package storage

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// CreateApprovalRequest stores a pending local approval request.
func (d *DB) CreateApprovalRequest(req ApprovalRequest) (string, error) {
	if strings.TrimSpace(req.Action) == "" {
		return "", fmt.Errorf("approval action is required")
	}
	now := time.Now().UTC()
	if req.RequestID == "" {
		req.RequestID = generatedID("apr")
	}
	if req.Status == "" {
		req.Status = "pending"
	}
	if req.RequiredApprovals <= 0 {
		req.RequiredApprovals = 1
	}
	if req.EscalationAfterSeconds < 0 {
		req.EscalationAfterSeconds = 0
	}
	if strings.TrimSpace(req.DueAt) == "" && req.EscalationAfterSeconds > 0 {
		req.DueAt = now.Add(time.Duration(req.EscalationAfterSeconds) * time.Second).Format(time.RFC3339Nano)
	}
	_, err := d.db.Exec(`INSERT INTO approval_requests(request_id,policy_decision_id,workload_id,run_id,source,model,project,action,target,actor_role,status,required_approvals,approver_hint,escalation_target,escalation_after_seconds,due_at,reason,request_payload,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		req.RequestID, req.PolicyDecisionID, req.WorkloadID, req.RunID, req.Source, req.Model, req.Project, req.Action, req.Target,
		req.ActorRole, normalizeApprovalStatus(req.Status), req.RequiredApprovals, req.ApproverHint, req.EscalationTarget, req.EscalationAfterSeconds, req.DueAt, req.Reason, req.RequestPayload, now, now)
	if err != nil {
		return "", err
	}
	return req.RequestID, nil
}

// ListApprovalRequests returns recent approval requests by status.
func (d *DB) ListApprovalRequests(status string, limit int) ([]ApprovalRequest, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT request_id,policy_decision_id,workload_id,run_id,source,model,project,action,target,actor_role,status,
		COALESCE(required_approvals,1),
		(SELECT COUNT(*) FROM approval_votes av WHERE av.request_id=approval_requests.request_id AND av.status='approved'),
		(SELECT COUNT(*) FROM approval_votes av WHERE av.request_id=approval_requests.request_id AND av.status='rejected'),
		approver_hint,escalation_target,COALESCE(escalation_after_seconds,0),COALESCE(due_at,''),
		reason,request_payload,created_at,updated_at,COALESCE(decided_at,''),decided_by,decision_note FROM approval_requests`
	args := []interface{}{}
	if status != "" && status != "all" {
		q += ` WHERE status=?`
		args = append(args, normalizeApprovalStatus(status))
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ApprovalRequest
	for rows.Next() {
		var r ApprovalRequest
		if err := rows.Scan(&r.RequestID, &r.PolicyDecisionID, &r.WorkloadID, &r.RunID, &r.Source, &r.Model, &r.Project,
			&r.Action, &r.Target, &r.ActorRole, &r.Status, &r.RequiredApprovals, &r.ApprovalVotes, &r.RejectionVotes, &r.ApproverHint, &r.EscalationTarget, &r.EscalationAfterSeconds, &r.DueAt, &r.Reason, &r.RequestPayload, &r.CreatedAt, &r.UpdatedAt,
			&r.DecidedAt, &r.DecidedBy, &r.DecisionNote); err != nil {
			return nil, err
		}
		r.Overdue = approvalOverdue(r.Status, r.DueAt, time.Now().UTC())
		out = append(out, r)
	}
	if out == nil {
		out = []ApprovalRequest{}
	}
	return out, rows.Err()
}

func approvalOverdue(status, dueAt string, now time.Time) bool {
	if normalizeApprovalStatus(status) != "pending" || strings.TrimSpace(dueAt) == "" {
		return false
	}
	due, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(dueAt))
	if err != nil {
		due, err = time.Parse(time.RFC3339, strings.TrimSpace(dueAt))
	}
	return err == nil && !due.After(now)
}

// CastApprovalVote records or updates one local actor vote and resolves the request when quorum is reached.
func (d *DB) CastApprovalVote(requestID, status, voter, role, note string, requiredApprovals int) (*ApprovalVoteResult, error) {
	if requestID == "" {
		return nil, fmt.Errorf("request_id is required")
	}
	status = normalizeApprovalStatus(status)
	if status != "approved" && status != "rejected" {
		return nil, fmt.Errorf("approval status must be approved or rejected")
	}
	voter = strings.TrimSpace(voter)
	if voter == "" {
		voter = firstNonEmpty(role, "local")
	}
	if requiredApprovals <= 0 {
		requiredApprovals = 0
	}
	now := time.Now().UTC()
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var currentStatus string
	var storedRequired int
	err = tx.QueryRow(`SELECT status, COALESCE(required_approvals,1) FROM approval_requests WHERE request_id=?`, requestID).Scan(&currentStatus, &storedRequired)
	if err != nil {
		return nil, err
	}
	if currentStatus != "pending" {
		return nil, sql.ErrNoRows
	}
	if requiredApprovals <= 0 {
		requiredApprovals = storedRequired
	}
	if requiredApprovals <= 0 {
		requiredApprovals = 1
	}
	if requiredApprovals > 20 {
		requiredApprovals = 20
	}
	if _, err := tx.Exec(`UPDATE approval_requests SET required_approvals=?, updated_at=? WHERE request_id=?`, requiredApprovals, now, requestID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO approval_votes(request_id,voter,role,status,note,created_at)
		VALUES(?,?,?,?,?,?)
		ON CONFLICT(request_id,voter) DO UPDATE SET role=excluded.role,status=excluded.status,note=excluded.note,created_at=excluded.created_at`,
		requestID, voter, role, status, note, now); err != nil {
		return nil, err
	}
	var approvals, rejections int
	if err := tx.QueryRow(`SELECT
		COALESCE(SUM(CASE WHEN status='approved' THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN status='rejected' THEN 1 ELSE 0 END),0)
		FROM approval_votes WHERE request_id=?`, requestID).Scan(&approvals, &rejections); err != nil {
		return nil, err
	}
	finalStatus := "pending"
	if rejections >= requiredApprovals {
		finalStatus = "rejected"
	} else if approvals >= requiredApprovals {
		finalStatus = "approved"
	}
	if finalStatus != "pending" {
		voters, err := approvalVoters(tx, requestID, finalStatus)
		if err != nil {
			return nil, err
		}
		decisionNote := fmt.Sprintf("%s quorum reached: %s", finalStatus, strings.Join(voters, ","))
		if strings.TrimSpace(note) != "" {
			decisionNote = decisionNote + " - " + strings.TrimSpace(note)
		}
		if _, err := tx.Exec(`UPDATE approval_requests SET status=?,updated_at=?,decided_at=?,decided_by=?,decision_note=? WHERE request_id=?`,
			finalStatus, now, now, strings.Join(voters, ","), decisionNote, requestID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &ApprovalVoteResult{
		RequestID:         requestID,
		Status:            finalStatus,
		RequiredApprovals: requiredApprovals,
		ApprovalVotes:     approvals,
		RejectionVotes:    rejections,
		Decided:           finalStatus != "pending",
	}, nil
}

// ResolveApprovalRequest marks a request approved or rejected.
func (d *DB) ResolveApprovalRequest(requestID, status, decidedBy, note string) error {
	_, err := d.CastApprovalVote(requestID, status, firstNonEmpty(decidedBy, "admin"), decidedBy, note, 0)
	return err
}

// ApprovalAllows reports whether a previously approved request can authorize an operation retry.
func (d *DB) ApprovalAllows(requestID, action, target string) (bool, error) {
	if requestID == "" {
		return false, nil
	}
	var storedAction, storedTarget, status string
	err := d.db.QueryRow(`SELECT action,target,status FROM approval_requests WHERE request_id=?`, requestID).Scan(&storedAction, &storedTarget, &status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status != "approved" {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(storedAction), strings.TrimSpace(action)) {
		return false, nil
	}
	if storedTarget != "" && target != "" && storedTarget != target {
		return false, nil
	}
	return true, nil
}

// ApprovalAllowsOperation reports whether a previously approved request can authorize a matching operation retry.
func (d *DB) ApprovalAllowsOperation(op ApprovalOperation) (bool, error) {
	if strings.TrimSpace(op.RequestID) == "" {
		return false, nil
	}
	var storedAction, storedTarget, storedSource, storedModel, storedProject, status string
	err := d.db.QueryRow(`SELECT action,target,source,model,project,status FROM approval_requests WHERE request_id=?`, op.RequestID).Scan(&storedAction, &storedTarget, &storedSource, &storedModel, &storedProject, &status)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if status != "approved" {
		return false, nil
	}
	if !strings.EqualFold(strings.TrimSpace(storedAction), strings.TrimSpace(op.Action)) {
		return false, nil
	}
	if !approvalContextMatches(storedTarget, op.Target, false) {
		return false, nil
	}
	if !approvalContextMatches(storedSource, op.Source, true) {
		return false, nil
	}
	if !approvalContextMatches(storedModel, op.Model, true) {
		return false, nil
	}
	if !approvalContextMatches(storedProject, op.Project, false) {
		return false, nil
	}
	return true, nil
}

func approvalContextMatches(stored, incoming string, caseInsensitive bool) bool {
	stored = strings.TrimSpace(stored)
	incoming = strings.TrimSpace(incoming)
	if stored == "" {
		return true
	}
	if incoming == "" {
		return false
	}
	if caseInsensitive {
		return strings.EqualFold(stored, incoming)
	}
	return stored == incoming
}

func approvalVoters(tx *sql.Tx, requestID, status string) ([]string, error) {
	rows, err := tx.Query(`SELECT voter FROM approval_votes WHERE request_id=? AND status=? ORDER BY created_at, voter`, requestID, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var voter string
		if err := rows.Scan(&voter); err != nil {
			return nil, err
		}
		out = append(out, voter)
	}
	if len(out) == 0 {
		out = []string{"local"}
	}
	return out, rows.Err()
}

func normalizeApprovalStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "approved", "approve", "allow":
		return "approved"
	case "rejected", "reject", "denied", "deny":
		return "rejected"
	case "all":
		return "all"
	default:
		return "pending"
	}
}
