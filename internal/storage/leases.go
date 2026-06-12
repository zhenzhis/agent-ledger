package storage

import (
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	defaultWorkloadLeaseTTL = 30 * time.Minute
	maxWorkloadLeaseTTL     = 24 * time.Hour
)

// WorkloadLease is a privacy-safe workload execution lease. LeaseToken is only
// returned on acquire and is never stored in SQLite.
type WorkloadLease struct {
	LeaseID       string `json:"lease_id"`
	WorkloadID    string `json:"workload_id"`
	Holder        string `json:"holder"`
	Purpose       string `json:"purpose"`
	Status        string `json:"status"`
	AcquiredAt    string `json:"acquired_at"`
	ExpiresAt     string `json:"expires_at"`
	LastRenewedAt string `json:"last_renewed_at,omitempty"`
	ReleasedAt    string `json:"released_at,omitempty"`
	Expired       bool   `json:"expired"`
	TTLSeconds    int64  `json:"ttl_seconds"`
	LeaseToken    string `json:"lease_token,omitempty"`
}

// WorkloadLeaseStats summarizes local workload leases without exposing tokens.
type WorkloadLeaseStats struct {
	Active       int    `json:"active"`
	Expired      int    `json:"expired"`
	Released     int    `json:"released"`
	Total        int    `json:"total"`
	NextExpiryAt string `json:"next_expiry_at"`
}

// WorkloadClaimFilter scopes queue-style workload claims for async routers.
type WorkloadClaimFilter struct {
	Source  string `json:"source,omitempty"`
	Project string `json:"project,omitempty"`
	Repo    string `json:"repo,omitempty"`
	Team    string `json:"team,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Status  string `json:"status,omitempty"`
	Query   string `json:"q,omitempty"`
}

// WorkloadClaimResult is returned by queue-style claim operations.
type WorkloadClaimResult struct {
	OK         bool             `json:"ok"`
	Empty      bool             `json:"empty"`
	WorkloadID string           `json:"workload_id,omitempty"`
	Workload   *WorkloadSummary `json:"workload,omitempty"`
	Lease      *WorkloadLease   `json:"lease,omitempty"`
}

// WorkloadLeaseConflictError reports that a workload has a non-expired active lease.
type WorkloadLeaseConflictError struct {
	WorkloadID string
	ExpiresAt  string
}

func (e *WorkloadLeaseConflictError) Error() string {
	return fmt.Sprintf("workload_id %s already has an active lease until %s", e.WorkloadID, e.ExpiresAt)
}

// IsWorkloadLeaseConflict reports whether err is an active lease conflict.
func IsWorkloadLeaseConflict(err error) bool {
	var target *WorkloadLeaseConflictError
	return errors.As(err, &target)
}

// AcquireWorkloadLease claims one workload for a local router/agent holder.
func (d *DB) AcquireWorkloadLease(workloadID, holder, purpose string, ttl time.Duration) (*WorkloadLease, error) {
	workloadID = strings.TrimSpace(workloadID)
	holder = strings.TrimSpace(holder)
	purpose = strings.TrimSpace(purpose)
	if workloadID == "" {
		return nil, fmt.Errorf("workload_id is required")
	}
	if holder == "" {
		return nil, fmt.Errorf("holder is required")
	}
	if err := validateShortMetadata("holder", holder, 200); err != nil {
		return nil, err
	}
	if err := validateShortMetadata("purpose", purpose, 256); err != nil {
		return nil, err
	}
	ttl = normalizeWorkloadLeaseTTL(ttl)
	now := time.Now().UTC()
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := expireWorkloadLeasesTx(tx, now); err != nil {
		return nil, err
	}
	var status string
	if err := tx.QueryRow(`SELECT status FROM workloads WHERE workload_id=?`, workloadID).Scan(&status); err != nil {
		return nil, err
	}
	if terminalWorkloadStatus(status) {
		return nil, fmt.Errorf("workload_id %s is already %s; lease acquire rejected", workloadID, status)
	}
	var existing WorkloadLease
	err = tx.QueryRow(`SELECT lease_id,expires_at FROM workload_leases WHERE workload_id=? AND status='active'`, workloadID).Scan(&existing.LeaseID, &existing.ExpiresAt)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	if err == nil {
		return nil, &WorkloadLeaseConflictError{WorkloadID: workloadID, ExpiresAt: existing.ExpiresAt}
	}
	lease, err := insertWorkloadLeaseTx(tx, workloadID, holder, purpose, ttl, now)
	if err != nil {
		return nil, err
	}
	_, _ = tx.Exec(`UPDATE workloads SET updated_at=? WHERE workload_id=?`, now, workloadID)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return lease, nil
}

// ClaimNextWorkload atomically selects the next claimable workload and creates
// a short-lived execution lease. It is intended for local async routers and
// workers that need queue semantics without a list-then-acquire race.
func (d *DB) ClaimNextWorkload(holder, purpose string, ttl time.Duration, filter WorkloadClaimFilter) (*WorkloadClaimResult, error) {
	holder = strings.TrimSpace(holder)
	purpose = strings.TrimSpace(purpose)
	if holder == "" {
		return nil, fmt.Errorf("holder is required")
	}
	if err := validateShortMetadata("holder", holder, 200); err != nil {
		return nil, err
	}
	if err := validateShortMetadata("purpose", purpose, 256); err != nil {
		return nil, err
	}
	if err := validateWorkloadClaimFilter(filter); err != nil {
		return nil, err
	}
	statuses, err := workloadClaimStatuses(filter.Status)
	if err != nil {
		return nil, err
	}
	ttl = normalizeWorkloadLeaseTTL(ttl)
	now := time.Now().UTC()
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := expireWorkloadLeasesTx(tx, now); err != nil {
		return nil, err
	}
	where := []string{`w.status IN (` + placeholders(len(statuses)) + `)`, `NOT EXISTS (
			SELECT 1 FROM workload_leases wl
			WHERE wl.workload_id=w.workload_id AND wl.status='active' AND wl.expires_at > ?
		)`}
	args := make([]interface{}, 0, len(statuses)+8)
	for _, status := range statuses {
		args = append(args, status)
	}
	args = append(args, now)
	filter.Source = strings.TrimSpace(filter.Source)
	filter.Project = strings.TrimSpace(filter.Project)
	filter.Repo = strings.TrimSpace(filter.Repo)
	filter.Team = strings.TrimSpace(filter.Team)
	filter.Owner = strings.TrimSpace(filter.Owner)
	filter.Query = strings.TrimSpace(filter.Query)
	if filter.Source != "" {
		where = append(where, "w.source=?")
		args = append(args, filter.Source)
	}
	if filter.Project != "" {
		where = append(where, "w.project=?")
		args = append(args, filter.Project)
	}
	if filter.Repo != "" {
		where = append(where, "w.repo=?")
		args = append(args, filter.Repo)
	}
	if filter.Team != "" {
		where = append(where, "w.team=?")
		args = append(args, filter.Team)
	}
	if filter.Owner != "" {
		where = append(where, "w.owner=?")
		args = append(args, filter.Owner)
	}
	if filter.Query != "" {
		like := "%" + strings.ToLower(filter.Query) + "%"
		where = append(where, `(LOWER(w.goal) LIKE ? OR LOWER(w.project) LIKE ? OR LOWER(w.repo) LIKE ? OR LOWER(w.team) LIKE ? OR LOWER(w.owner) LIKE ?)`)
		args = append(args, like, like, like, like, like)
	}
	var workloadID string
	err = tx.QueryRow(`SELECT w.workload_id
		FROM workloads w
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY CASE w.status
			WHEN 'queued' THEN 0
			WHEN 'active' THEN 1
			WHEN 'stalled' THEN 2
			WHEN 'blocked' THEN 3
			WHEN 'waiting_approval' THEN 4
			WHEN 'evaluating' THEN 5
			WHEN 'running' THEN 6
			ELSE 9
		END, w.created_at ASC, w.updated_at ASC, w.workload_id ASC
		LIMIT 1`, args...).Scan(&workloadID)
	if err == sql.ErrNoRows {
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &WorkloadClaimResult{OK: true, Empty: true}, nil
	}
	if err != nil {
		return nil, err
	}
	lease, err := insertWorkloadLeaseTx(tx, workloadID, holder, purpose, ttl, now)
	if err != nil {
		return nil, err
	}
	_, _ = tx.Exec(`UPDATE workloads SET updated_at=? WHERE workload_id=?`, now, workloadID)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	summary, err := d.getWorkloadSummaryByID(workloadID)
	if err != nil {
		return nil, err
	}
	return &WorkloadClaimResult{OK: true, Empty: false, WorkloadID: workloadID, Workload: summary, Lease: lease}, nil
}

// RenewWorkloadLease extends an active lease after validating its token.
func (d *DB) RenewWorkloadLease(leaseID, leaseToken string, ttl time.Duration) (*WorkloadLease, error) {
	leaseID = strings.TrimSpace(leaseID)
	leaseToken = strings.TrimSpace(leaseToken)
	if leaseID == "" {
		return nil, fmt.Errorf("lease_id is required")
	}
	if leaseToken == "" {
		return nil, fmt.Errorf("lease_token is required")
	}
	ttl = normalizeWorkloadLeaseTTL(ttl)
	now := time.Now().UTC()
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := expireWorkloadLeasesTx(tx, now); err != nil {
		return nil, err
	}
	lease, tokenHash, err := getWorkloadLeaseForUpdateTx(tx, leaseID)
	if err != nil {
		return nil, err
	}
	if lease.Status != "active" || lease.Expired {
		return nil, fmt.Errorf("lease_id %s is not active", leaseID)
	}
	if !workloadLeaseTokenMatches(tokenHash, leaseToken) {
		return nil, fmt.Errorf("lease_token does not match lease_id %s", leaseID)
	}
	expires := now.Add(ttl)
	if _, err := tx.Exec(`UPDATE workload_leases SET expires_at=?, last_renewed_at=? WHERE lease_id=?`, expires, now, leaseID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	lease.ExpiresAt = expires.Format(time.RFC3339Nano)
	lease.LastRenewedAt = now.Format(time.RFC3339Nano)
	lease.TTLSeconds = int64(ttl.Seconds())
	return lease, nil
}

// ReleaseWorkloadLease releases an active lease after validating its token.
func (d *DB) ReleaseWorkloadLease(leaseID, leaseToken string) (*WorkloadLease, error) {
	leaseID = strings.TrimSpace(leaseID)
	leaseToken = strings.TrimSpace(leaseToken)
	if leaseID == "" {
		return nil, fmt.Errorf("lease_id is required")
	}
	if leaseToken == "" {
		return nil, fmt.Errorf("lease_token is required")
	}
	now := time.Now().UTC()
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if err := expireWorkloadLeasesTx(tx, now); err != nil {
		return nil, err
	}
	lease, tokenHash, err := getWorkloadLeaseForUpdateTx(tx, leaseID)
	if err != nil {
		return nil, err
	}
	if !workloadLeaseTokenMatches(tokenHash, leaseToken) {
		return nil, fmt.Errorf("lease_token does not match lease_id %s", leaseID)
	}
	if lease.Status == "active" {
		if _, err := tx.Exec(`UPDATE workload_leases SET status='released', released_at=? WHERE lease_id=?`, now, leaseID); err != nil {
			return nil, err
		}
		lease.Status = "released"
		lease.ReleasedAt = now.Format(time.RFC3339Nano)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return lease, nil
}

// ListWorkloadLeases lists recent leases. includeInactive includes expired and released rows.
func (d *DB) ListWorkloadLeases(includeInactive bool, limit int) ([]WorkloadLease, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now().UTC()
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	where := `status='active' AND expires_at > ?`
	args := []interface{}{now}
	if includeInactive {
		where = `1=1`
		args = nil
	}
	args = append(args, limit)
	rows, err := d.db.Query(`SELECT lease_id,workload_id,holder,purpose,status,acquired_at,expires_at,COALESCE(last_renewed_at,''),COALESCE(released_at,'')
		FROM workload_leases WHERE `+where+` ORDER BY expires_at ASC, acquired_at DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	leases := []WorkloadLease{}
	for rows.Next() {
		lease, err := scanWorkloadLease(rows, now)
		if err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}
	return leases, rows.Err()
}

// GetWorkloadLeaseStats returns privacy-safe lease counts for probes.
func (d *DB) GetWorkloadLeaseStats() (*WorkloadLeaseStats, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	now := time.Now().UTC()
	stats := &WorkloadLeaseStats{}
	if err := d.db.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN status='active' AND expires_at > ? THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='expired' OR (status='active' AND expires_at <= ?) THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='released' THEN 1 ELSE 0 END),0),
		COUNT(*),
		COALESCE(MIN(CASE WHEN status='active' AND expires_at > ? THEN expires_at END),'')
		FROM workload_leases`, now, now, now).Scan(&stats.Active, &stats.Expired, &stats.Released, &stats.Total, &stats.NextExpiryAt); err != nil {
		return nil, err
	}
	return stats, nil
}

func insertWorkloadLeaseTx(tx *sql.Tx, workloadID, holder, purpose string, ttl time.Duration, now time.Time) (*WorkloadLease, error) {
	token := generatedID("lt")
	lease := &WorkloadLease{
		LeaseID:    generatedID("lease"),
		WorkloadID: workloadID,
		Holder:     holder,
		Purpose:    purpose,
		Status:     "active",
		AcquiredAt: now.Format(time.RFC3339Nano),
		ExpiresAt:  now.Add(ttl).Format(time.RFC3339Nano),
		TTLSeconds: int64(ttl.Seconds()),
		LeaseToken: token,
	}
	if _, err := tx.Exec(`INSERT INTO workload_leases(lease_id,workload_id,holder,purpose,status,token_hash,acquired_at,expires_at,confidence)
		VALUES(?,?,?,?,?,?,?,?,?)`, lease.LeaseID, workloadID, holder, purpose, "active", workloadLeaseTokenHash(token), now, now.Add(ttl), 1.0); err != nil {
		return nil, err
	}
	return lease, nil
}

func normalizeWorkloadLeaseTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return defaultWorkloadLeaseTTL
	}
	if ttl > maxWorkloadLeaseTTL {
		return maxWorkloadLeaseTTL
	}
	return ttl
}

func workloadClaimStatuses(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{"queued", "active"}, nil
	}
	if raw == "*" || strings.EqualFold(raw, "any") || strings.EqualFold(raw, "nonterminal") {
		return []string{"queued", "active", "stalled", "blocked", "waiting_approval", "evaluating", "running"}, nil
	}
	seen := map[string]bool{}
	var statuses []string
	for _, part := range strings.Split(raw, ",") {
		status := strings.TrimSpace(part)
		if status == "" {
			continue
		}
		if !validWorkloadStatus(status) {
			return nil, fmt.Errorf("unsupported workload status %q", status)
		}
		if terminalWorkloadStatus(status) {
			return nil, fmt.Errorf("terminal workload status %q is not claimable", status)
		}
		if !seen[status] {
			seen[status] = true
			statuses = append(statuses, status)
		}
	}
	if len(statuses) == 0 {
		return nil, fmt.Errorf("at least one claimable status is required")
	}
	return statuses, nil
}

func validateWorkloadClaimFilter(filter WorkloadClaimFilter) error {
	fields := []struct {
		name  string
		value string
		limit int
	}{
		{"source", filter.Source, 128},
		{"project", filter.Project, 512},
		{"repo", filter.Repo, 512},
		{"team", filter.Team, 256},
		{"owner", filter.Owner, 256},
		{"q", filter.Query, 512},
	}
	for _, field := range fields {
		if err := validateShortMetadata(field.name, field.value, field.limit); err != nil {
			return err
		}
	}
	return nil
}

func placeholders(n int) string {
	if n <= 0 {
		return ""
	}
	parts := make([]string, n)
	for i := range parts {
		parts[i] = "?"
	}
	return strings.Join(parts, ",")
}

func workloadLeaseTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func workloadLeaseTokenMatches(expectedHash, token string) bool {
	actualHash := workloadLeaseTokenHash(token)
	return subtle.ConstantTimeCompare([]byte(expectedHash), []byte(actualHash)) == 1
}

func expireWorkloadLeasesTx(tx *sql.Tx, now time.Time) error {
	_, err := tx.Exec(`UPDATE workload_leases SET status='expired' WHERE status='active' AND expires_at <= ?`, now)
	return err
}

func getWorkloadLeaseForUpdateTx(tx *sql.Tx, leaseID string) (*WorkloadLease, string, error) {
	var tokenHash string
	row := tx.QueryRow(`SELECT lease_id,workload_id,holder,purpose,status,token_hash,acquired_at,expires_at,COALESCE(last_renewed_at,''),COALESCE(released_at,'')
		FROM workload_leases WHERE lease_id=?`, leaseID)
	lease, err := scanWorkloadLeaseWithToken(row, time.Now().UTC(), &tokenHash)
	return &lease, tokenHash, err
}

type leaseScanner interface {
	Scan(dest ...interface{}) error
}

func scanWorkloadLease(scanner leaseScanner, now time.Time) (WorkloadLease, error) {
	var lease WorkloadLease
	if err := scanner.Scan(&lease.LeaseID, &lease.WorkloadID, &lease.Holder, &lease.Purpose, &lease.Status, &lease.AcquiredAt, &lease.ExpiresAt, &lease.LastRenewedAt, &lease.ReleasedAt); err != nil {
		return lease, err
	}
	lease.applyLeaseDerived(now)
	return lease, nil
}

func scanWorkloadLeaseWithToken(scanner leaseScanner, now time.Time, tokenHash *string) (WorkloadLease, error) {
	var lease WorkloadLease
	if err := scanner.Scan(&lease.LeaseID, &lease.WorkloadID, &lease.Holder, &lease.Purpose, &lease.Status, tokenHash, &lease.AcquiredAt, &lease.ExpiresAt, &lease.LastRenewedAt, &lease.ReleasedAt); err != nil {
		return lease, err
	}
	lease.applyLeaseDerived(now)
	return lease, nil
}

func (l *WorkloadLease) applyLeaseDerived(now time.Time) {
	if t, ok := parseDBTime(l.ExpiresAt); ok {
		l.Expired = l.Status == "active" && !t.After(now)
		if l.Expired {
			l.Status = "expired"
			l.TTLSeconds = 0
			return
		}
		ttl := t.Sub(now).Seconds()
		if ttl > 0 {
			l.TTLSeconds = int64(ttl)
		}
	}
}
