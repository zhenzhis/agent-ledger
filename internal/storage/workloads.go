package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// WorkloadSummary is the canonical goal-level ledger row exposed by the API.
type WorkloadSummary struct {
	WorkloadID   string  `json:"workload_id"`
	Goal         string  `json:"goal"`
	Status       string  `json:"status"`
	Source       string  `json:"source"`
	Project      string  `json:"project"`
	Repo         string  `json:"repo"`
	GitBranch    string  `json:"git_branch"`
	Owner        string  `json:"owner"`
	Team         string  `json:"team"`
	BudgetUSD    float64 `json:"budget_usd"`
	Outcome      string  `json:"outcome"`
	Confidence   float64 `json:"confidence"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
	ClosedAt     string  `json:"closed_at"`
	Runs         int     `json:"runs"`
	ModelCalls   int     `json:"model_calls"`
	ToolCalls    int     `json:"tool_calls"`
	Sessions     int     `json:"sessions"`
	Tokens       int64   `json:"tokens"`
	CostUSD      float64 `json:"cost_usd"`
	LastActivity string  `json:"last_activity"`
}

// WorkloadPage is a paginated workload ledger response.
type WorkloadPage struct {
	Rows       []WorkloadSummary `json:"rows"`
	Total      int               `json:"total"`
	Limit      int               `json:"limit"`
	Offset     int               `json:"offset"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// AgentRunRow is one agent execution attached to a workload.
type AgentRunRow struct {
	RunID        string  `json:"run_id"`
	WorkloadID   string  `json:"workload_id"`
	ParentRunID  string  `json:"parent_run_id"`
	Source       string  `json:"source"`
	AgentName    string  `json:"agent_name"`
	AgentVersion string  `json:"agent_version"`
	Command      string  `json:"command"`
	CWD          string  `json:"cwd"`
	Status       string  `json:"status"`
	ExitCode     int     `json:"exit_code"`
	Error        string  `json:"error"`
	StartedAt    string  `json:"started_at"`
	EndedAt      string  `json:"ended_at"`
	DurationMS   int64   `json:"duration_ms"`
	Confidence   float64 `json:"confidence"`
}

// ModelCallDetail summarizes canonical model calls by source/model/session.
type ModelCallDetail struct {
	CallID            string  `json:"call_id,omitempty"`
	RunID             string  `json:"run_id,omitempty"`
	Source            string  `json:"source"`
	SessionID         string  `json:"session_id"`
	Provider          string  `json:"provider"`
	Model             string  `json:"model"`
	Calls             int     `json:"calls"`
	InputTokens       int64   `json:"input_tokens"`
	OutputTokens      int64   `json:"output_tokens"`
	CacheRead         int64   `json:"cache_read"`
	CacheCreate       int64   `json:"cache_create"`
	Reasoning         int64   `json:"reasoning"`
	Tokens            int64   `json:"tokens"`
	CostUSD           float64 `json:"cost_usd"`
	PricingSource     string  `json:"pricing_source"`
	PricingConfidence string  `json:"pricing_confidence"`
	FirstAt           string  `json:"first_at"`
	LastAt            string  `json:"last_at"`
	Confidence        float64 `json:"confidence"`
}

// ToolCallRow represents one canonical tool call.
type ToolCallRow struct {
	ToolCallID string  `json:"tool_call_id"`
	WorkloadID string  `json:"workload_id"`
	RunID      string  `json:"run_id"`
	Source     string  `json:"source"`
	ToolName   string  `json:"tool_name"`
	ToolType   string  `json:"tool_type"`
	Status     string  `json:"status"`
	ErrorClass string  `json:"error_class"`
	DurationMS int64   `json:"duration_ms"`
	Timestamp  string  `json:"timestamp"`
	Confidence float64 `json:"confidence"`
}

// ArtifactRow represents one privacy-safe workload artifact reference.
type ArtifactRow struct {
	ArtifactID   string  `json:"artifact_id"`
	WorkloadID   string  `json:"workload_id"`
	RunID        string  `json:"run_id"`
	ArtifactType string  `json:"artifact_type"`
	Label        string  `json:"label"`
	PathHash     string  `json:"path_hash"`
	SHA256       string  `json:"sha256"`
	Metadata     string  `json:"metadata"`
	CreatedAt    string  `json:"created_at"`
	Confidence   float64 `json:"confidence"`
}

// EvaluationRow captures outcome and quality signals for a workload.
type EvaluationRow struct {
	EvaluationID string  `json:"evaluation_id"`
	WorkloadID   string  `json:"workload_id"`
	Evaluator    string  `json:"evaluator"`
	Status       string  `json:"status"`
	Score        float64 `json:"score"`
	Signal       string  `json:"signal"`
	Notes        string  `json:"notes"`
	CreatedAt    string  `json:"created_at"`
}

// PolicyDecisionRow captures local policy decisions.
type PolicyDecisionRow struct {
	DecisionID string `json:"decision_id"`
	WorkloadID string `json:"workload_id"`
	RunID      string `json:"run_id"`
	RuleID     string `json:"rule_id"`
	Action     string `json:"action"`
	Reason     string `json:"reason"`
	ActorRole  string `json:"actor_role"`
	CreatedAt  string `json:"created_at"`
}

// WorkloadDetail contains the workload plus child ledger rows.
type WorkloadDetail struct {
	Summary     WorkloadSummary     `json:"summary"`
	Runs        []AgentRunRow       `json:"runs"`
	ModelCalls  []ModelCallDetail   `json:"model_calls"`
	ToolCalls   []ToolCallRow       `json:"tool_calls"`
	Artifacts   []ArtifactRow       `json:"artifacts"`
	Evaluations []EvaluationRow     `json:"evaluations"`
	Policies    []PolicyDecisionRow `json:"policy_decisions"`
	Sessions    []SessionInfo       `json:"sessions"`
}

// WorkloadGraph is a compact graph for UI timelines and dependency views.
type WorkloadGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// GraphNode is a workload graph node.
type GraphNode struct {
	ID    string            `json:"id"`
	Kind  string            `json:"kind"`
	Label string            `json:"label"`
	Meta  map[string]string `json:"meta,omitempty"`
}

// GraphEdge is a workload graph edge.
type GraphEdge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Label string `json:"label"`
}

// ModelRegistryRow is a canonical model registry view derived from pricing and usage.
type ModelRegistryRow struct {
	Model              string  `json:"model"`
	Vendor             string  `json:"vendor"`
	Family             string  `json:"family"`
	PricingSource      string  `json:"pricing_source"`
	MatchedModel       string  `json:"matched_model"`
	MatchType          string  `json:"match_type"`
	Confidence         string  `json:"confidence"`
	InputCostPerToken  float64 `json:"input_cost_per_token"`
	OutputCostPerToken float64 `json:"output_cost_per_token"`
	CacheReadCost      float64 `json:"cache_read_input_token_cost"`
	CacheWriteCost     float64 `json:"cache_creation_input_token_cost"`
	Calls              int     `json:"calls"`
	Tokens             int64   `json:"tokens"`
	CostUSD            float64 `json:"cost_usd"`
	UpdatedAt          string  `json:"updated_at"`
	Stale              bool    `json:"stale"`
}

// BackfillWorkloadsFromUsage derives canonical workload rows from legacy usage/session data.
// It is idempotent and keeps existing manually created workloads untouched.
func (d *DB) BackfillWorkloadsFromUsage(from, to time.Time) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT u.source,u.session_id,
		COALESCE(NULLIF(MAX(s.project),''),MAX(u.project),''), COALESCE(MAX(s.cwd),''),
		COALESCE(NULLIF(MAX(s.git_branch),''),MAX(u.git_branch),'unknown'),
		COALESCE(MIN(s.start_time),MIN(u.timestamp)), MAX(u.timestamp), COUNT(*),
		COALESCE(SUM(u.input_tokens+u.cache_read_input_tokens+u.cache_creation_input_tokens+u.output_tokens),0),
		COALESCE(SUM(u.cost_usd),0), COUNT(DISTINCT u.model), COALESCE(MAX(s.prompts),0)
		FROM usage_records u LEFT JOIN sessions s ON u.source=s.source AND u.session_id=s.session_id
		WHERE u.timestamp >= ? AND u.timestamp < ?
		GROUP BY u.source,u.session_id`, from, to)
	if err != nil {
		return err
	}
	type legacySession struct {
		source, sessionID, project, cwd, branch, firstAt, lastAt string
		calls, models, prompts                                   int
		tokens                                                   int64
		cost                                                     float64
	}
	var legacy []legacySession
	for rows.Next() {
		var s legacySession
		if err := rows.Scan(&s.source, &s.sessionID, &s.project, &s.cwd, &s.branch, &s.firstAt, &s.lastAt, &s.calls, &s.tokens, &s.cost, &s.models, &s.prompts); err != nil {
			rows.Close()
			return err
		}
		legacy = append(legacy, s)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	now := time.Now().UTC()
	for _, s := range legacy {
		workloadID := stableID("wl", s.source+"\x00"+s.sessionID)
		runID := stableID("run", s.source+"\x00"+s.sessionID)
		repo := deriveRepo(s.project, s.cwd)
		goal := fmt.Sprintf("%s session %s", s.source, shortID(s.sessionID))
		if _, err := tx.Exec(`INSERT INTO workloads(workload_id,goal,status,source,project,repo,git_branch,budget_usd,outcome,confidence,created_at,updated_at,closed_at)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(workload_id) DO UPDATE SET
				source=excluded.source,
				project=CASE WHEN workloads.project='' THEN excluded.project ELSE workloads.project END,
				repo=CASE WHEN workloads.repo='' THEN excluded.repo ELSE workloads.repo END,
				git_branch=CASE WHEN workloads.git_branch='' OR workloads.git_branch='unknown' THEN excluded.git_branch ELSE workloads.git_branch END,
				updated_at=CASE WHEN excluded.updated_at > workloads.updated_at THEN excluded.updated_at ELSE workloads.updated_at END,
				closed_at=CASE WHEN workloads.status='completed' AND (workloads.closed_at IS NULL OR excluded.closed_at > workloads.closed_at) THEN excluded.closed_at ELSE workloads.closed_at END`,
			workloadID, goal, "completed", s.source, s.project, repo, normalizeBranch(s.branch), 0, "legacy-session-derived", 0.65, s.firstAt, s.lastAt, s.lastAt); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO workload_sessions(workload_id,source,session_id,confidence,created_at) VALUES(?,?,?,?,?)`,
			workloadID, s.source, s.sessionID, 0.9, now); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO agent_runs(run_id,workload_id,source,agent_name,cwd,status,started_at,ended_at,duration_ms,confidence)
			VALUES(?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT(run_id) DO UPDATE SET
				cwd=CASE WHEN agent_runs.cwd='' THEN excluded.cwd ELSE agent_runs.cwd END,
				status=CASE WHEN agent_runs.status='running' THEN excluded.status ELSE agent_runs.status END,
				ended_at=CASE WHEN agent_runs.ended_at IS NULL OR excluded.ended_at > agent_runs.ended_at THEN excluded.ended_at ELSE agent_runs.ended_at END`,
			runID, workloadID, s.source, s.source, s.cwd, "completed", s.firstAt, s.lastAt, durationMillis(s.firstAt, s.lastAt), 0.65); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO model_calls(call_id,workload_id,run_id,source,session_id,model,input_tokens,output_tokens,
				cache_read_input_tokens,cache_creation_input_tokens,reasoning_output_tokens,cost_usd,pricing_source,pricing_confidence,timestamp,confidence)
			SELECT 'usage:' || id, ?, ?, source, session_id, model, input_tokens, output_tokens, cache_read_input_tokens,
				cache_creation_input_tokens, reasoning_output_tokens, cost_usd, pricing_source, pricing_confidence, timestamp, 0.95
			FROM usage_records WHERE source=? AND session_id=? AND timestamp >= ? AND timestamp < ?`,
			workloadID, runID, s.source, s.sessionID, from, to); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO canonical_events(event_id,source,event_type,source_event_id,workload_id,agent_run_id,session_id,model,project,git_branch,timestamp,payload_hash,payload,confidence,created_at)
			SELECT 'usage:' || id, source, 'model_call', CAST(id AS TEXT), ?, ?, session_id, model, project, git_branch, timestamp,
				'usage:' || id, '{"legacy_table":"usage_records","usage_record_id":' || id || '}', 0.95, ?
			FROM usage_records WHERE source=? AND session_id=? AND timestamp >= ? AND timestamp < ?`,
			workloadID, runID, now, s.source, s.sessionID, from, to); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// CreateWorkload creates a manually scoped workload for CLI/MCP/API callers.
func (d *DB) CreateWorkload(goal, source, project, repo, branch, owner, team string, budgetUSD float64) (string, error) {
	if strings.TrimSpace(goal) == "" {
		return "", fmt.Errorf("goal is required")
	}
	id := generatedID("wl")
	now := time.Now().UTC()
	_, err := d.db.Exec(`INSERT INTO workloads(workload_id,goal,status,source,project,repo,git_branch,owner,team,budget_usd,outcome,confidence,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		id, strings.TrimSpace(goal), "active", source, project, repo, normalizeBranch(branch), owner, team, budgetUSD, "", 1.0, now, now)
	if err != nil {
		return "", err
	}
	return id, nil
}

// CloseWorkload marks a workload complete, failed, partial, or abandoned.
func (d *DB) CloseWorkload(workloadID, status, outcome string) error {
	if workloadID == "" {
		return fmt.Errorf("workload_id is required")
	}
	if status == "" {
		status = "completed"
	}
	if !validWorkloadStatus(status) {
		return fmt.Errorf("unsupported workload status %q", status)
	}
	now := time.Now().UTC()
	res, err := d.db.Exec(`UPDATE workloads SET status=?, outcome=?, updated_at=?, closed_at=? WHERE workload_id=?`,
		status, outcome, now, now, workloadID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// StartAgentRun records a run attached to an existing workload.
func (d *DB) StartAgentRun(workloadID, source, agentName, command, cwd string) (string, error) {
	if workloadID == "" {
		return "", fmt.Errorf("workload_id is required")
	}
	id := generatedID("run")
	now := time.Now().UTC()
	if _, err := d.db.Exec(`INSERT INTO agent_runs(run_id,workload_id,source,agent_name,command,cwd,status,started_at,confidence)
		VALUES(?,?,?,?,?,?,?,?,?)`, id, workloadID, source, agentName, command, cwd, "running", now, 1.0); err != nil {
		return "", err
	}
	_, _ = d.db.Exec(`UPDATE workloads SET updated_at=? WHERE workload_id=?`, now, workloadID)
	return id, nil
}

// FinishAgentRun closes a run and records command outcome.
func (d *DB) FinishAgentRun(runID, status string, exitCode int, errText string, durationMS int64) error {
	if runID == "" {
		return fmt.Errorf("run_id is required")
	}
	if status == "" {
		status = "completed"
	}
	now := time.Now().UTC()
	_, err := d.db.Exec(`UPDATE agent_runs SET status=?, exit_code=?, error=?, ended_at=?, duration_ms=? WHERE run_id=?`,
		status, exitCode, errText, now, durationMS, runID)
	return err
}

// GetWorkloadsPage returns workload summaries derived from canonical model calls.
func (d *DB) GetWorkloadsPage(from, to time.Time, source, model, project, status, query string, limit, offset int) (*WorkloadPage, error) {
	if err := d.BackfillWorkloadsFromUsage(from, to); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	where, args := workloadWhere(source, model, project, status, query, from, to)
	countArgs := append([]interface{}{}, args...)
	var total int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM workloads w WHERE `+where, countArgs...).Scan(&total); err != nil {
		return nil, err
	}
	queryArgs := append([]interface{}{from, to}, args...)
	queryArgs = append(queryArgs, limit, offset)
	rows, err := d.db.Query(`SELECT w.workload_id,w.goal,w.status,w.source,w.project,w.repo,w.git_branch,w.owner,w.team,
		w.budget_usd,w.outcome,w.confidence,w.created_at,w.updated_at,COALESCE(w.closed_at,''),
		COALESCE(ar.runs,0), COALESCE(mc.model_calls,0), COALESCE(tc.tool_calls,0),
		COALESCE(ws.sessions,0), COALESCE(mc.tokens,0), COALESCE(mc.cost_usd,0), COALESCE(mc.last_activity,w.updated_at)
		FROM workloads w
		LEFT JOIN (SELECT workload_id,COUNT(*) AS runs FROM agent_runs GROUP BY workload_id) ar ON w.workload_id=ar.workload_id
		LEFT JOIN (SELECT workload_id,COUNT(*) AS sessions FROM workload_sessions GROUP BY workload_id) ws ON w.workload_id=ws.workload_id
		LEFT JOIN (SELECT workload_id,COUNT(*) AS tool_calls FROM tool_calls GROUP BY workload_id) tc ON w.workload_id=tc.workload_id
		LEFT JOIN (SELECT workload_id,COUNT(*) AS model_calls,
				COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens),0) AS tokens,
				COALESCE(SUM(cost_usd),0) AS cost_usd, MAX(timestamp) AS last_activity
			FROM model_calls WHERE timestamp >= ? AND timestamp < ? GROUP BY workload_id) mc ON w.workload_id=mc.workload_id
		WHERE `+where+`
		ORDER BY COALESCE(mc.last_activity,w.updated_at) DESC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkloadSummary
	for rows.Next() {
		var w WorkloadSummary
		if err := rows.Scan(&w.WorkloadID, &w.Goal, &w.Status, &w.Source, &w.Project, &w.Repo, &w.GitBranch, &w.Owner, &w.Team,
			&w.BudgetUSD, &w.Outcome, &w.Confidence, &w.CreatedAt, &w.UpdatedAt, &w.ClosedAt, &w.Runs, &w.ModelCalls, &w.ToolCalls,
			&w.Sessions, &w.Tokens, &w.CostUSD, &w.LastActivity); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if out == nil {
		out = []WorkloadSummary{}
	}
	next := ""
	if offset+limit < total {
		next = fmt.Sprintf("%d", offset+limit)
	}
	return &WorkloadPage{Rows: out, Total: total, Limit: limit, Offset: offset, NextCursor: next}, nil
}

// GetWorkloadDetail returns a full workload ledger detail.
func (d *DB) GetWorkloadDetail(workloadID string) (*WorkloadDetail, error) {
	if workloadID == "" {
		return nil, fmt.Errorf("workload_id is required")
	}
	summary, err := d.getWorkloadSummaryByID(workloadID)
	if err != nil {
		return nil, err
	}
	detail := &WorkloadDetail{Summary: *summary}
	if detail.Runs, err = d.getAgentRuns(workloadID); err != nil {
		return nil, err
	}
	if detail.ModelCalls, err = d.getModelCallDetails(workloadID); err != nil {
		return nil, err
	}
	if detail.ToolCalls, err = d.getToolCalls(workloadID); err != nil {
		return nil, err
	}
	if detail.Artifacts, err = d.getArtifacts(workloadID); err != nil {
		return nil, err
	}
	if detail.Evaluations, err = d.getEvaluations(workloadID); err != nil {
		return nil, err
	}
	if detail.Policies, err = d.GetPolicyDecisions(workloadID, 200); err != nil {
		return nil, err
	}
	if detail.Sessions, err = d.getWorkloadSessions(workloadID); err != nil {
		return nil, err
	}
	return detail, nil
}

// GetWorkloadGraph returns a compact workload graph.
func (d *DB) GetWorkloadGraph(workloadID string) (*WorkloadGraph, error) {
	detail, err := d.GetWorkloadDetail(workloadID)
	if err != nil {
		return nil, err
	}
	graph := &WorkloadGraph{}
	graph.Nodes = append(graph.Nodes, GraphNode{ID: detail.Summary.WorkloadID, Kind: "workload", Label: detail.Summary.Goal})
	for _, r := range detail.Runs {
		graph.Nodes = append(graph.Nodes, GraphNode{ID: r.RunID, Kind: "agent_run", Label: firstNonEmpty(r.AgentName, r.Source)})
		graph.Edges = append(graph.Edges, GraphEdge{From: detail.Summary.WorkloadID, To: r.RunID, Label: "runs"})
	}
	seenModels := map[string]bool{}
	for _, c := range detail.ModelCalls {
		id := "model:" + c.Source + ":" + c.Model
		if !seenModels[id] {
			graph.Nodes = append(graph.Nodes, GraphNode{ID: id, Kind: "model", Label: c.Model, Meta: map[string]string{"source": c.Source}})
			seenModels[id] = true
		}
		if c.RunID != "" {
			graph.Edges = append(graph.Edges, GraphEdge{From: c.RunID, To: id, Label: "calls"})
		}
	}
	for _, t := range detail.ToolCalls {
		graph.Nodes = append(graph.Nodes, GraphNode{ID: t.ToolCallID, Kind: "tool", Label: t.ToolName})
		if t.RunID != "" {
			graph.Edges = append(graph.Edges, GraphEdge{From: t.RunID, To: t.ToolCallID, Label: "uses"})
		}
	}
	return graph, nil
}

func (d *DB) getWorkloadSummaryByID(workloadID string) (*WorkloadSummary, error) {
	row := d.db.QueryRow(`SELECT w.workload_id,w.goal,w.status,w.source,w.project,w.repo,w.git_branch,w.owner,w.team,
		w.budget_usd,w.outcome,w.confidence,w.created_at,w.updated_at,COALESCE(w.closed_at,''),
		COALESCE(ar.runs,0), COALESCE(mc.model_calls,0), COALESCE(tc.tool_calls,0),
		COALESCE(ws.sessions,0), COALESCE(mc.tokens,0), COALESCE(mc.cost_usd,0), COALESCE(mc.last_activity,w.updated_at)
		FROM workloads w
		LEFT JOIN (SELECT workload_id,COUNT(*) AS runs FROM agent_runs GROUP BY workload_id) ar ON w.workload_id=ar.workload_id
		LEFT JOIN (SELECT workload_id,COUNT(*) AS sessions FROM workload_sessions GROUP BY workload_id) ws ON w.workload_id=ws.workload_id
		LEFT JOIN (SELECT workload_id,COUNT(*) AS tool_calls FROM tool_calls GROUP BY workload_id) tc ON w.workload_id=tc.workload_id
		LEFT JOIN (SELECT workload_id,COUNT(*) AS model_calls,
				COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens),0) AS tokens,
				COALESCE(SUM(cost_usd),0) AS cost_usd, MAX(timestamp) AS last_activity
			FROM model_calls GROUP BY workload_id) mc ON w.workload_id=mc.workload_id
		WHERE w.workload_id=?`, workloadID)
	var w WorkloadSummary
	if err := row.Scan(&w.WorkloadID, &w.Goal, &w.Status, &w.Source, &w.Project, &w.Repo, &w.GitBranch, &w.Owner, &w.Team,
		&w.BudgetUSD, &w.Outcome, &w.Confidence, &w.CreatedAt, &w.UpdatedAt, &w.ClosedAt, &w.Runs, &w.ModelCalls, &w.ToolCalls,
		&w.Sessions, &w.Tokens, &w.CostUSD, &w.LastActivity); err != nil {
		return nil, err
	}
	return &w, nil
}

// GetModelRegistry returns pricing governance joined with usage counts.
func (d *DB) GetModelRegistry(staleAfter time.Duration, limit int) ([]ModelRegistryRow, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := d.db.Query(`SELECT p.model,COALESCE(p.pricing_source,''),COALESCE(p.matched_model,''),COALESCE(p.match_type,''),
		COALESCE(p.confidence,''),p.input_cost_per_token,p.output_cost_per_token,p.cache_read_input_token_cost,
		p.cache_creation_input_token_cost,COALESCE(p.updated_at,''),
		COUNT(u.id),COALESCE(SUM(u.input_tokens+u.cache_read_input_tokens+u.cache_creation_input_tokens+u.output_tokens),0),
		COALESCE(SUM(u.cost_usd),0)
		FROM pricing p LEFT JOIN usage_records u ON p.model=u.model
		GROUP BY p.model
		ORDER BY COUNT(u.id) DESC, p.model
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().UTC()
	var out []ModelRegistryRow
	for rows.Next() {
		var r ModelRegistryRow
		if err := rows.Scan(&r.Model, &r.PricingSource, &r.MatchedModel, &r.MatchType, &r.Confidence,
			&r.InputCostPerToken, &r.OutputCostPerToken, &r.CacheReadCost, &r.CacheWriteCost, &r.UpdatedAt,
			&r.Calls, &r.Tokens, &r.CostUSD); err != nil {
			return nil, err
		}
		r.Vendor, r.Family = inferVendorFamily(r.Model)
		if t, ok := parseDBTime(r.UpdatedAt); !ok || now.Sub(t) > staleAfter {
			r.Stale = true
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetPolicyDecisions returns policy decisions for a workload or recent global decisions.
func (d *DB) GetPolicyDecisions(workloadID string, limit int) ([]PolicyDecisionRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT decision_id,workload_id,run_id,rule_id,action,reason,actor_role,created_at FROM policy_decisions`
	args := []interface{}{}
	if workloadID != "" {
		q += ` WHERE workload_id=?`
		args = append(args, workloadID)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PolicyDecisionRow
	for rows.Next() {
		var r PolicyDecisionRow
		if err := rows.Scan(&r.DecisionID, &r.WorkloadID, &r.RunID, &r.RuleID, &r.Action, &r.Reason, &r.ActorRole, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecordArtifact stores a privacy-safe artifact reference for a workload.
func (d *DB) RecordArtifact(workloadID, runID, artifactType, label, pathHash, sha, metadata string, confidence float64) (string, error) {
	if workloadID == "" {
		return "", fmt.Errorf("workload_id is required")
	}
	if confidence <= 0 {
		confidence = 1
	}
	id := generatedID("art")
	now := time.Now().UTC()
	_, err := d.db.Exec(`INSERT INTO artifacts(artifact_id,workload_id,run_id,artifact_type,label,path_hash,sha256,metadata,created_at,confidence)
		VALUES(?,?,?,?,?,?,?,?,?,?)`, id, workloadID, runID, artifactType, label, pathHash, sha, metadata, now, confidence)
	if err != nil {
		return "", err
	}
	_, _ = d.db.Exec(`UPDATE workloads SET updated_at=? WHERE workload_id=?`, now, workloadID)
	return id, nil
}

// RecordPolicyDecision stores a local policy decision.
func (d *DB) RecordPolicyDecision(workloadID, runID, ruleID, action, reason, actorRole string) (string, error) {
	if action == "" {
		action = "allow"
	}
	id := generatedID("pol")
	_, err := d.db.Exec(`INSERT INTO policy_decisions(decision_id,workload_id,run_id,rule_id,action,reason,actor_role,created_at)
		VALUES(?,?,?,?,?,?,?,?)`, id, workloadID, runID, ruleID, action, reason, actorRole, time.Now().UTC())
	if err != nil {
		return "", err
	}
	return id, nil
}

func (d *DB) getAgentRuns(workloadID string) ([]AgentRunRow, error) {
	rows, err := d.db.Query(`SELECT run_id,workload_id,parent_run_id,source,agent_name,agent_version,command,cwd,status,exit_code,error,
		started_at,COALESCE(ended_at,''),duration_ms,confidence FROM agent_runs WHERE workload_id=? ORDER BY started_at`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentRunRow
	for rows.Next() {
		var r AgentRunRow
		if err := rows.Scan(&r.RunID, &r.WorkloadID, &r.ParentRunID, &r.Source, &r.AgentName, &r.AgentVersion, &r.Command, &r.CWD,
			&r.Status, &r.ExitCode, &r.Error, &r.StartedAt, &r.EndedAt, &r.DurationMS, &r.Confidence); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) getModelCallDetails(workloadID string) ([]ModelCallDetail, error) {
	rows, err := d.db.Query(`SELECT run_id,source,session_id,provider,model,COUNT(*),
		COALESCE(SUM(input_tokens),0),COALESCE(SUM(output_tokens),0),COALESCE(SUM(cache_read_input_tokens),0),
		COALESCE(SUM(cache_creation_input_tokens),0),COALESCE(SUM(reasoning_output_tokens),0),
		COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens),0),
		COALESCE(SUM(cost_usd),0),COALESCE(MAX(pricing_source),''),COALESCE(MAX(pricing_confidence),''),
		MIN(timestamp),MAX(timestamp),AVG(confidence)
		FROM model_calls WHERE workload_id=? GROUP BY run_id,source,session_id,provider,model ORDER BY MAX(timestamp) DESC`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelCallDetail
	for rows.Next() {
		var r ModelCallDetail
		if err := rows.Scan(&r.RunID, &r.Source, &r.SessionID, &r.Provider, &r.Model, &r.Calls, &r.InputTokens, &r.OutputTokens,
			&r.CacheRead, &r.CacheCreate, &r.Reasoning, &r.Tokens, &r.CostUSD, &r.PricingSource, &r.PricingConfidence,
			&r.FirstAt, &r.LastAt, &r.Confidence); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) getToolCalls(workloadID string) ([]ToolCallRow, error) {
	rows, err := d.db.Query(`SELECT tool_call_id,workload_id,run_id,source,tool_name,tool_type,status,error_class,duration_ms,timestamp,confidence
		FROM tool_calls WHERE workload_id=? ORDER BY timestamp DESC LIMIT 500`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ToolCallRow
	for rows.Next() {
		var r ToolCallRow
		if err := rows.Scan(&r.ToolCallID, &r.WorkloadID, &r.RunID, &r.Source, &r.ToolName, &r.ToolType, &r.Status, &r.ErrorClass, &r.DurationMS, &r.Timestamp, &r.Confidence); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) getArtifacts(workloadID string) ([]ArtifactRow, error) {
	rows, err := d.db.Query(`SELECT artifact_id,workload_id,run_id,artifact_type,label,path_hash,sha256,metadata,created_at,confidence
		FROM artifacts WHERE workload_id=? ORDER BY created_at DESC LIMIT 500`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ArtifactRow
	for rows.Next() {
		var r ArtifactRow
		if err := rows.Scan(&r.ArtifactID, &r.WorkloadID, &r.RunID, &r.ArtifactType, &r.Label, &r.PathHash, &r.SHA256, &r.Metadata, &r.CreatedAt, &r.Confidence); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) getEvaluations(workloadID string) ([]EvaluationRow, error) {
	rows, err := d.db.Query(`SELECT evaluation_id,workload_id,evaluator,status,score,signal,notes,created_at
		FROM evaluations WHERE workload_id=? ORDER BY created_at DESC LIMIT 200`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []EvaluationRow
	for rows.Next() {
		var r EvaluationRow
		if err := rows.Scan(&r.EvaluationID, &r.WorkloadID, &r.Evaluator, &r.Status, &r.Score, &r.Signal, &r.Notes, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) getWorkloadSessions(workloadID string) ([]SessionInfo, error) {
	rows, err := d.db.Query(`SELECT s.session_id,s.source,s.project,s.cwd,s.git_branch,COALESCE(s.start_time,''),COALESCE(MAX(u.timestamp),''),
		COALESCE(s.prompts,0),COALESCE(SUM(u.cost_usd),0),COALESCE(SUM(u.input_tokens+u.cache_read_input_tokens+u.cache_creation_input_tokens+u.output_tokens),0)
		FROM workload_sessions ws JOIN sessions s ON ws.source=s.source AND ws.session_id=s.session_id
		LEFT JOIN usage_records u ON s.source=u.source AND s.session_id=u.session_id
		WHERE ws.workload_id=? GROUP BY s.source,s.session_id ORDER BY MAX(u.timestamp) DESC`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SessionInfo
	for rows.Next() {
		var r SessionInfo
		if err := rows.Scan(&r.SessionID, &r.Source, &r.Project, &r.CWD, &r.GitBranch, &r.StartTime, &r.LastActivity, &r.Prompts, &r.TotalCost, &r.Tokens); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func workloadWhere(source, model, project, status, query string, from, to time.Time) (string, []interface{}) {
	base := `(EXISTS (SELECT 1 FROM model_calls mx WHERE mx.workload_id=w.workload_id AND mx.timestamp >= ? AND mx.timestamp < ?`
	args := []interface{}{from, to}
	if source != "" {
		base += ` AND mx.source=?`
		args = append(args, source)
	}
	if model != "" {
		base += ` AND mx.model=?`
		args = append(args, model)
	}
	base += `)`
	if model == "" {
		base += ` OR (NOT EXISTS (SELECT 1 FROM model_calls m0 WHERE m0.workload_id=w.workload_id) AND w.updated_at >= ? AND w.updated_at < ?`
		args = append(args, from, to)
		if source != "" {
			base += ` AND w.source=?`
			args = append(args, source)
		}
		base += `)`
	}
	base += `)`
	clauses := []string{base}
	if project != "" {
		clauses = append(clauses, `w.project=?`)
		args = append(args, project)
	}
	if status != "" {
		clauses = append(clauses, `w.status=?`)
		args = append(args, status)
	}
	query = strings.TrimSpace(query)
	if query != "" {
		pattern := "%" + escapeLike(query) + "%"
		clauses = append(clauses, `(w.workload_id=? OR w.goal LIKE ? ESCAPE '\' OR w.project LIKE ? ESCAPE '\' OR w.repo LIKE ? ESCAPE '\' OR w.git_branch LIKE ? ESCAPE '\')`)
		args = append(args, query, pattern, pattern, pattern, pattern)
	}
	return strings.Join(clauses, " AND "), args
}

func stableID(prefix, value string) string {
	sum := sha256.Sum256([]byte(value))
	return prefix + "_" + hex.EncodeToString(sum[:])[:20]
}

func generatedID(prefix string) string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return stableID(prefix, fmt.Sprintf("%s:%d", prefix, time.Now().UnixNano()))
	}
	return prefix + "_" + hex.EncodeToString(b[:])
}

func shortID(id string) string {
	id = strings.TrimSpace(id)
	if len(id) <= 12 {
		return id
	}
	return id[:12]
}

func deriveRepo(project, cwd string) string {
	if project != "" {
		return project
	}
	cwd = strings.TrimRight(strings.ReplaceAll(cwd, "\\", "/"), "/")
	if cwd == "" {
		return ""
	}
	parts := strings.Split(cwd, "/")
	return parts[len(parts)-1]
}

func durationMillis(first, last string) int64 {
	start, ok1 := parseDBTime(first)
	end, ok2 := parseDBTime(last)
	if !ok1 || !ok2 || end.Before(start) {
		return 0
	}
	return end.Sub(start).Milliseconds()
}

func validWorkloadStatus(status string) bool {
	switch status {
	case "active", "queued", "running", "waiting_approval", "evaluating", "completed", "partial", "failed", "abandoned", "merged", "deployed", "reverted":
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return "-"
}

func inferVendorFamily(model string) (string, string) {
	m := strings.ToLower(model)
	switch {
	case strings.Contains(m, "gpt") || strings.Contains(m, "o1") || strings.Contains(m, "o3") || strings.Contains(m, "o4"):
		return "openai", "gpt"
	case strings.Contains(m, "claude"):
		return "anthropic", "claude"
	case strings.Contains(m, "gemini"):
		return "google", "gemini"
	case strings.Contains(m, "qwen"):
		return "alibaba", "qwen"
	case strings.Contains(m, "deepseek"):
		return "deepseek", "deepseek"
	case strings.Contains(m, "llama"):
		return "meta", "llama"
	case strings.Contains(m, "glm"):
		return "zhipu", "glm"
	default:
		return "unknown", "unknown"
	}
}

func jsonString(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}
