package storage

import (
	"fmt"
	"time"
)

// FleetAttributionReport summarizes sub-agent and parallel-run amplification.
type FleetAttributionReport struct {
	GeneratedAt   string                `json:"generated_at"`
	From          string                `json:"from"`
	To            string                `json:"to"`
	Runs          int                   `json:"runs"`
	SubAgentRuns  int                   `json:"sub_agent_runs"`
	MaxConcurrent int                   `json:"max_concurrent_runs"`
	ModelCalls    int                   `json:"model_calls"`
	Tokens        int64                 `json:"tokens"`
	CostUSD       float64               `json:"cost_usd"`
	Rows          []FleetAttributionRow `json:"rows"`
}

// FleetAttributionRow is one canonical agent run with cost attribution.
type FleetAttributionRow struct {
	WorkloadID     string  `json:"workload_id"`
	Goal           string  `json:"goal"`
	Source         string  `json:"source"`
	Project        string  `json:"project"`
	Repo           string  `json:"repo"`
	GitBranch      string  `json:"git_branch"`
	Team           string  `json:"team"`
	RunID          string  `json:"run_id"`
	ParentRunID    string  `json:"parent_run_id"`
	AgentName      string  `json:"agent_name"`
	Status         string  `json:"status"`
	StartedAt      string  `json:"started_at"`
	EndedAt        string  `json:"ended_at"`
	FirstCallAt    string  `json:"first_call_at"`
	LastCallAt     string  `json:"last_call_at"`
	DurationMS     int64   `json:"duration_ms"`
	ModelCalls     int     `json:"model_calls"`
	Tokens         int64   `json:"tokens"`
	CostUSD        float64 `json:"cost_usd"`
	ChildRuns      int     `json:"child_runs"`
	ConcurrentRuns int     `json:"concurrent_runs"`
	Attribution    string  `json:"attribution"`
	Confidence     float64 `json:"confidence"`
	Evidence       string  `json:"evidence"`
	start          time.Time
	end            time.Time
}

// GetFleetAttribution returns run-level attribution for multi-agent/fleet work.
func (d *DB) GetFleetAttribution(from, to time.Time, source, model, project string, limit int) (*FleetAttributionReport, error) {
	from, to = utcRange(from, to)
	if err := d.BackfillWorkloadsFromUsage(from, to); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	filter, filterArgs := fleetFilter(source, model, project)
	args := append([]interface{}{from, to}, filterArgs...)
	args = append(args, limit)
	rows, err := d.db.Query(`SELECT w.workload_id,COALESCE(w.goal,''),COALESCE(w.source,''),COALESCE(w.project,''),COALESCE(w.repo,''),
		COALESCE(w.git_branch,''),COALESCE(w.team,''),ar.run_id,COALESCE(ar.parent_run_id,''),COALESCE(ar.agent_name,''),
		COALESCE(ar.status,''),COALESCE(ar.started_at,''),COALESCE(ar.ended_at,''),COALESCE(ar.duration_ms,0),COALESCE(ar.confidence,0),
		COUNT(mc.call_id) AS model_calls,
		COALESCE(SUM(mc.input_tokens+mc.cache_read_input_tokens+mc.cache_creation_input_tokens+mc.output_tokens),0) AS tokens,
		COALESCE(SUM(mc.cost_usd),0) AS cost_usd,
		COALESCE(MIN(mc.timestamp),''),COALESCE(MAX(mc.timestamp),'')
		FROM model_calls mc
		JOIN agent_runs ar ON mc.run_id=ar.run_id
		LEFT JOIN workloads w ON mc.workload_id=w.workload_id
		WHERE mc.timestamp >= ? AND mc.timestamp < ?`+filter+`
		GROUP BY ar.run_id
		ORDER BY cost_usd DESC, tokens DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	report := &FleetAttributionReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		From:        from.Format(time.RFC3339),
		To:          to.Format(time.RFC3339),
	}
	for rows.Next() {
		var row FleetAttributionRow
		if err := rows.Scan(&row.WorkloadID, &row.Goal, &row.Source, &row.Project, &row.Repo, &row.GitBranch, &row.Team,
			&row.RunID, &row.ParentRunID, &row.AgentName, &row.Status, &row.StartedAt, &row.EndedAt, &row.DurationMS,
			&row.Confidence, &row.ModelCalls, &row.Tokens, &row.CostUSD, &row.FirstCallAt, &row.LastCallAt); err != nil {
			return nil, err
		}
		row.start, row.end = fleetWindow(row)
		report.Rows = append(report.Rows, row)
		report.ModelCalls += row.ModelCalls
		report.Tokens += row.Tokens
		report.CostUSD += row.CostUSD
		if row.ParentRunID != "" {
			report.SubAgentRuns++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if report.Rows == nil {
		report.Rows = []FleetAttributionRow{}
	}
	report.Runs = len(report.Rows)
	annotateFleetRows(report.Rows)
	for _, row := range report.Rows {
		if row.ConcurrentRuns > report.MaxConcurrent {
			report.MaxConcurrent = row.ConcurrentRuns
		}
	}
	return report, nil
}

func fleetFilter(source, model, project string) (string, []interface{}) {
	var filter string
	var args []interface{}
	if source != "" {
		filter += " AND mc.source=?"
		args = append(args, source)
	}
	if model != "" {
		filter += " AND mc.model=?"
		args = append(args, model)
	}
	if project != "" {
		filter += " AND COALESCE(w.project,'')=?"
		args = append(args, project)
	}
	return filter, args
}

func annotateFleetRows(rows []FleetAttributionRow) {
	childCounts := map[string]int{}
	for _, row := range rows {
		if row.ParentRunID != "" {
			childCounts[row.ParentRunID]++
		}
	}
	for i := range rows {
		rows[i].ChildRuns = childCounts[rows[i].RunID]
		concurrent := 1
		for j := range rows {
			if i == j || rows[i].WorkloadID != rows[j].WorkloadID {
				continue
			}
			if fleetOverlaps(rows[i], rows[j]) {
				concurrent++
			}
		}
		rows[i].ConcurrentRuns = concurrent
		rows[i].Attribution, rows[i].Evidence = fleetAttributionLabel(rows[i])
	}
}

func fleetAttributionLabel(row FleetAttributionRow) (string, string) {
	switch {
	case row.ParentRunID != "":
		return "sub-agent", "explicit parent_run_id"
	case row.ChildRuns > 0:
		return "parent-agent", fmt.Sprintf("%d child run(s)", row.ChildRuns)
	case row.ConcurrentRuns > 1:
		return "parallel-run", fmt.Sprintf("%d overlapping run(s) in workload", row.ConcurrentRuns)
	case row.Confidence < 0.7:
		return "legacy-heuristic", "derived from legacy session backfill"
	default:
		return "single-run", "canonical run metadata"
	}
}

func fleetWindow(row FleetAttributionRow) (time.Time, time.Time) {
	start, ok := parseFleetTime(firstNonEmptyStorage(row.StartedAt, row.FirstCallAt))
	if !ok {
		start, _ = parseFleetTime(row.FirstCallAt)
	}
	end, ok := parseFleetTime(firstNonEmptyStorage(row.EndedAt, row.LastCallAt))
	if !ok {
		end = start
	}
	if end.Before(start) {
		end = start
	}
	return start, end
}

func fleetOverlaps(a, b FleetAttributionRow) bool {
	if a.start.IsZero() || b.start.IsZero() {
		return false
	}
	aEnd := a.end
	bEnd := b.end
	if aEnd.Equal(a.start) {
		aEnd = a.start.Add(time.Millisecond)
	}
	if bEnd.Equal(b.start) {
		bEnd = b.start.Add(time.Millisecond)
	}
	return a.start.Before(bEnd) && b.start.Before(aEnd)
}

func parseFleetTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05.999999999Z07:00",
		"2006-01-02 15:04:05",
	} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}
