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
	_, _ = tx.Exec(`UPDATE workloads SET updated_at=? WHERE workload_id=?`, now, workloadID)
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return lease, nil
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

func normalizeWorkloadLeaseTTL(ttl time.Duration) time.Duration {
	if ttl <= 0 {
		return defaultWorkloadLeaseTTL
	}
	if ttl > maxWorkloadLeaseTTL {
		return maxWorkloadLeaseTTL
	}
	return ttl
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
