package storage

import (
	"time"

	ledgerpolicy "github.com/zhenzhis/agent-ledger/internal/policy"
)

// GetPolicyAuditCandidates returns metadata-only historical rows that can be
// evaluated with the shared local policy evaluator.
func (d *DB) GetPolicyAuditCandidates(from, to time.Time, source, model, project string, limit int) ([]ledgerpolicy.AuditCandidate, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	out := []ledgerpolicy.AuditCandidate{}
	usage, err := d.policyAuditUsageCandidates(from, to, source, model, project, limit)
	if err != nil {
		return nil, err
	}
	out = append(out, usage...)
	if model != "" {
		return out, nil
	}
	remaining := limit - len(out)
	if remaining <= 0 {
		return out, nil
	}
	tools, err := d.policyAuditToolCandidates(from, to, source, project, remaining)
	if err != nil {
		return nil, err
	}
	out = append(out, tools...)
	remaining = limit - len(out)
	if remaining <= 0 {
		return out, nil
	}
	workloads, err := d.policyAuditWorkloadCandidates(from, to, source, project, remaining)
	if err != nil {
		return nil, err
	}
	out = append(out, workloads...)
	return out, nil
}

func (d *DB) policyAuditUsageCandidates(from, to time.Time, source, model, project string, limit int) ([]ledgerpolicy.AuditCandidate, error) {
	q := `SELECT source,model,project,session_id,
		COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens),0),
		COALESCE(SUM(cost_usd),0),COALESCE(MAX(timestamp),'')
		FROM usage_records WHERE timestamp >= ? AND timestamp < ?`
	args := []interface{}{from, to}
	if source != "" {
		q += ` AND source=?`
		args = append(args, source)
	}
	if model != "" {
		q += ` AND model=?`
		args = append(args, model)
	}
	if project != "" {
		q += ` AND project=?`
		args = append(args, project)
	}
	q += ` GROUP BY source,model,project,session_id ORDER BY MAX(timestamp) DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ledgerpolicy.AuditCandidate{}
	for rows.Next() {
		var c ledgerpolicy.AuditCandidate
		if err := rows.Scan(&c.Source, &c.Model, &c.Project, &c.SessionID, &c.Tokens, &c.CostUSD, &c.Timestamp); err != nil {
			return nil, err
		}
		c.Kind = "usage_session"
		c.Action = "model.call"
		c.Evidence = "usage_records"
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *DB) policyAuditToolCandidates(from, to time.Time, source, project string, limit int) ([]ledgerpolicy.AuditCandidate, error) {
	q := `SELECT tc.tool_call_id,tc.workload_id,tc.run_id,tc.source,COALESCE(w.project,''),tc.timestamp,tc.tool_name,tc.status
		FROM tool_calls tc LEFT JOIN workloads w ON tc.workload_id=w.workload_id
		WHERE tc.timestamp >= ? AND tc.timestamp < ?`
	args := []interface{}{from, to}
	if source != "" {
		q += ` AND tc.source=?`
		args = append(args, source)
	}
	if project != "" {
		q += ` AND w.project=?`
		args = append(args, project)
	}
	q += ` ORDER BY tc.timestamp DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ledgerpolicy.AuditCandidate{}
	for rows.Next() {
		var id, toolName, status string
		var c ledgerpolicy.AuditCandidate
		if err := rows.Scan(&id, &c.WorkloadID, &c.RunID, &c.Source, &c.Project, &c.Timestamp, &toolName, &status); err != nil {
			return nil, err
		}
		c.Kind = "tool_call"
		c.Action = "tool.call"
		c.Evidence = "tool_calls:" + id + ":" + toolName + ":" + status
		out = append(out, c)
	}
	return out, rows.Err()
}

func (d *DB) policyAuditWorkloadCandidates(from, to time.Time, source, project string, limit int) ([]ledgerpolicy.AuditCandidate, error) {
	q := `SELECT workload_id,source,project,created_at,status FROM workloads WHERE created_at >= ? AND created_at < ?`
	args := []interface{}{from, to}
	if source != "" {
		q += ` AND source=?`
		args = append(args, source)
	}
	if project != "" {
		q += ` AND project=?`
		args = append(args, project)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ledgerpolicy.AuditCandidate{}
	for rows.Next() {
		var status string
		var c ledgerpolicy.AuditCandidate
		if err := rows.Scan(&c.WorkloadID, &c.Source, &c.Project, &c.Timestamp, &status); err != nil {
			return nil, err
		}
		c.Kind = "workload"
		c.Action = "workload"
		c.Evidence = "workloads:" + status
		out = append(out, c)
	}
	return out, rows.Err()
}
