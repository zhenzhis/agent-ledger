package storage

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var commandSecretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(\b[A-Z0-9_]*(?:API_KEY|TOKEN|PASSWORD|SECRET|ACCESS_KEY|PRIVATE_KEY)[A-Z0-9_]*=)("[^"]*"|'[^']*'|\S+)`),
	regexp.MustCompile(`(?i)(--(?:api[-_]?key|token|password|secret|access[-_]?token|auth[-_]?token|private[-_]?key)(?:=|\s+))("[^"]*"|'[^']*'|\S+)`),
	regexp.MustCompile(`(?i)(\bBearer\s+)[A-Za-z0-9._~+/\-=]+`),
}

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
	RunID           string  `json:"run_id"`
	WorkloadID      string  `json:"workload_id"`
	ParentRunID     string  `json:"parent_run_id"`
	Source          string  `json:"source"`
	AgentName       string  `json:"agent_name"`
	AgentVersion    string  `json:"agent_version"`
	Command         string  `json:"command"`
	CWD             string  `json:"cwd"`
	Status          string  `json:"status"`
	ExitCode        int     `json:"exit_code"`
	Error           string  `json:"error"`
	StartedAt       string  `json:"started_at"`
	EndedAt         string  `json:"ended_at"`
	DurationMS      int64   `json:"duration_ms"`
	LastHeartbeatAt string  `json:"last_heartbeat_at"`
	HeartbeatCount  int     `json:"heartbeat_count"`
	Phase           string  `json:"phase"`
	Progress        float64 `json:"progress"`
	StatusMessage   string  `json:"status_message"`
	Confidence      float64 `json:"confidence"`
}

// AgentRunEventRow is an append-only metadata event for async agent run state.
type AgentRunEventRow struct {
	EventID    string  `json:"event_id"`
	RunID      string  `json:"run_id"`
	WorkloadID string  `json:"workload_id"`
	Source     string  `json:"source"`
	EventType  string  `json:"event_type"`
	Status     string  `json:"status"`
	Phase      string  `json:"phase"`
	Progress   float64 `json:"progress"`
	Message    string  `json:"message"`
	Metrics    string  `json:"metrics"`
	Timestamp  string  `json:"timestamp"`
	Confidence float64 `json:"confidence"`
}

// AgentRunLivenessRow reports whether an async run is still sending heartbeats.
type AgentRunLivenessRow struct {
	RunID           string  `json:"run_id"`
	WorkloadID      string  `json:"workload_id"`
	Goal            string  `json:"goal"`
	Source          string  `json:"source"`
	AgentName       string  `json:"agent_name"`
	Status          string  `json:"status"`
	Project         string  `json:"project"`
	Repo            string  `json:"repo"`
	GitBranch       string  `json:"git_branch"`
	Phase           string  `json:"phase"`
	Progress        float64 `json:"progress"`
	StartedAt       string  `json:"started_at"`
	LastHeartbeatAt string  `json:"last_heartbeat_at"`
	LastActivity    string  `json:"last_activity"`
	HeartbeatCount  int     `json:"heartbeat_count"`
	StatusMessage   string  `json:"status_message"`
	AgeSeconds      int64   `json:"age_seconds"`
	Stale           bool    `json:"stale"`
}

// WorkloadState is a derived terminal-state snapshot for async agent workloads.
type WorkloadState struct {
	WorkloadID               string   `json:"workload_id"`
	Goal                     string   `json:"goal"`
	Status                   string   `json:"status"`
	Source                   string   `json:"source"`
	Phase                    string   `json:"phase"`
	Terminal                 bool     `json:"terminal"`
	Stale                    bool     `json:"stale"`
	ReadinessScore           float64  `json:"readiness_score"`
	Progress                 float64  `json:"progress"`
	NextAction               string   `json:"next_action"`
	Reasons                  []string `json:"reasons"`
	Risks                    []string `json:"risks"`
	Project                  string   `json:"project"`
	Repo                     string   `json:"repo"`
	GitBranch                string   `json:"git_branch"`
	Team                     string   `json:"team"`
	LastActivity             string   `json:"last_activity"`
	StaleAfterSeconds        int64    `json:"stale_after_seconds"`
	Runs                     int      `json:"runs"`
	ActiveRuns               int      `json:"active_runs"`
	StaleRuns                int      `json:"stale_runs"`
	CompletedRuns            int      `json:"completed_runs"`
	FailedRuns               int      `json:"failed_runs"`
	ModelCalls               int      `json:"model_calls"`
	ToolCalls                int      `json:"tool_calls"`
	ContextRefs              int      `json:"context_refs"`
	Artifacts                int      `json:"artifacts"`
	Evaluations              int      `json:"evaluations"`
	PositiveEvaluations      int      `json:"positive_evaluations"`
	NegativeEvaluations      int      `json:"negative_evaluations"`
	PolicyBlocks             int      `json:"policy_blocks"`
	PolicyApprovalsRequired  int      `json:"policy_approvals_required"`
	BudgetUSD                float64  `json:"budget_usd"`
	CostUSD                  float64  `json:"cost_usd"`
	Tokens                   int64    `json:"tokens"`
	EstimatedRemainingBudget float64  `json:"estimated_remaining_budget"`
	EstimatedBudgetExhausted bool     `json:"estimated_budget_exhausted"`
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

// ContextRefRow represents one privacy-safe context reference attached to a workload.
type ContextRefRow struct {
	ContextRefID string  `json:"context_ref_id"`
	WorkloadID   string  `json:"workload_id"`
	RunID        string  `json:"run_id"`
	RefType      string  `json:"ref_type"`
	RefHash      string  `json:"ref_hash"`
	Label        string  `json:"label"`
	Repo         string  `json:"repo"`
	GitBranch    string  `json:"git_branch"`
	CommitSHA    string  `json:"commit_sha"`
	PrivacyLabel string  `json:"privacy_label"`
	CreatedAt    string  `json:"created_at"`
	Confidence   float64 `json:"confidence"`
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

// WorkloadLinkRow captures a metadata-only dependency or lineage edge between workloads.
type WorkloadLinkRow struct {
	LinkID           string  `json:"link_id"`
	SourceWorkloadID string  `json:"source_workload_id"`
	TargetWorkloadID string  `json:"target_workload_id"`
	Relation         string  `json:"relation"`
	Reason           string  `json:"reason"`
	CreatedBy        string  `json:"created_by"`
	CreatedAt        string  `json:"created_at"`
	Confidence       float64 `json:"confidence"`
}

// WorkloadDetail contains the workload plus child ledger rows.
type WorkloadDetail struct {
	Summary     WorkloadSummary     `json:"summary"`
	Runs        []AgentRunRow       `json:"runs"`
	RunEvents   []AgentRunEventRow  `json:"run_events"`
	ModelCalls  []ModelCallDetail   `json:"model_calls"`
	ToolCalls   []ToolCallRow       `json:"tool_calls"`
	ContextRefs []ContextRefRow     `json:"context_refs"`
	Artifacts   []ArtifactRow       `json:"artifacts"`
	Evaluations []EvaluationRow     `json:"evaluations"`
	Policies    []PolicyDecisionRow `json:"policy_decisions"`
	Links       []WorkloadLinkRow   `json:"links"`
	Sessions    []SessionInfo       `json:"sessions"`
}

// WorkloadGraph is a compact graph for UI timelines and dependency views.
type WorkloadGraph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

// WorkloadTimelineRow is a normalized chronological audit event for one workload.
type WorkloadTimelineRow struct {
	Kind       string  `json:"kind"`
	ID         string  `json:"id"`
	RunID      string  `json:"run_id,omitempty"`
	Source     string  `json:"source,omitempty"`
	Label      string  `json:"label"`
	Status     string  `json:"status,omitempty"`
	Detail     string  `json:"detail,omitempty"`
	Tokens     int64   `json:"tokens,omitempty"`
	CostUSD    float64 `json:"cost_usd,omitempty"`
	DurationMS int64   `json:"duration_ms,omitempty"`
	Timestamp  string  `json:"timestamp"`
	Confidence float64 `json:"confidence,omitempty"`
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
		var nonLegacyOwners int
		if err := tx.QueryRow(`SELECT COUNT(*)
			FROM workload_sessions ws JOIN workloads w ON ws.workload_id=w.workload_id
			WHERE ws.source=? AND ws.session_id=? AND ws.workload_id<>? AND COALESCE(w.outcome,'')<>'legacy-session-derived'`,
			s.source, s.sessionID, workloadID).Scan(&nonLegacyOwners); err != nil {
			return err
		}
		if nonLegacyOwners > 0 {
			continue
		}
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
	d.mu.Lock()
	defer d.mu.Unlock()
	if workloadID == "" {
		return "", fmt.Errorf("workload_id is required")
	}
	var workloadStatus string
	if err := d.db.QueryRow(`SELECT status FROM workloads WHERE workload_id=?`, workloadID).Scan(&workloadStatus); err != nil {
		return "", err
	}
	if terminalWorkloadStatus(workloadStatus) {
		return "", fmt.Errorf("workload_id %s is already %s; run start rejected", workloadID, workloadStatus)
	}
	id := generatedID("run")
	now := time.Now().UTC()
	if _, err := d.db.Exec(`INSERT INTO agent_runs(run_id,workload_id,source,agent_name,command,cwd,status,started_at,confidence)
		VALUES(?,?,?,?,?,?,?,?,?)`, id, workloadID, source, agentName, redactCommandSecrets(command), cwd, "running", now, 1.0); err != nil {
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

type agentRunHeartbeatInput struct {
	EventID    string
	RunID      string
	WorkloadID string
	Source     string
	Status     string
	Phase      string
	Message    string
	Progress   float64
	Metrics    map[string]interface{}
	Timestamp  time.Time
	Confidence float64
}

// RecordAgentRunHeartbeat appends a metadata-only heartbeat and updates the run snapshot.
func (d *DB) RecordAgentRunHeartbeat(eventID, runID, status, phase, message string, progress float64, metrics map[string]interface{}, timestamp time.Time, confidence float64) (*AgentRunEventRow, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	row, err := recordAgentRunHeartbeatTx(tx, agentRunHeartbeatInput{
		EventID:    eventID,
		RunID:      runID,
		Status:     status,
		Phase:      phase,
		Message:    message,
		Progress:   progress,
		Metrics:    metrics,
		Timestamp:  timestamp,
		Confidence: confidence,
	})
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return row, nil
}

// GetAgentRunLiveness returns active runs ordered by oldest heartbeat/activity first.
func (d *DB) GetAgentRunLiveness(maxAge time.Duration, staleOnly bool, limit int, filters ...string) ([]AgentRunLivenessRow, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if maxAge <= 0 {
		maxAge = 10 * time.Minute
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	source, project := "", ""
	if len(filters) > 0 {
		source = strings.TrimSpace(filters[0])
	}
	if len(filters) > 1 {
		project = strings.TrimSpace(filters[1])
	}
	clauses := []string{`ar.status IN ('queued','running','working','waiting_approval','blocked','evaluating','stalled')`}
	args := []interface{}{}
	if source != "" {
		clauses = append(clauses, `ar.source=?`)
		args = append(args, source)
	}
	if project != "" {
		clauses = append(clauses, `(w.project=? OR w.repo=?)`)
		args = append(args, project, project)
	}
	args = append(args, limit)
	rows, err := d.db.Query(`SELECT ar.run_id,ar.workload_id,w.goal,ar.source,ar.agent_name,ar.status,w.project,w.repo,w.git_branch,
		COALESCE(ar.phase,''),COALESCE(ar.progress,0),ar.started_at,COALESCE(ar.last_heartbeat_at,''),COALESCE(ar.heartbeat_count,0),COALESCE(ar.status_message,'')
		FROM agent_runs ar JOIN workloads w ON ar.workload_id=w.workload_id
		WHERE `+strings.Join(clauses, " AND ")+`
		ORDER BY COALESCE(ar.last_heartbeat_at, ar.started_at) ASC
		LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().UTC()
	var out []AgentRunLivenessRow
	for rows.Next() {
		var r AgentRunLivenessRow
		if err := rows.Scan(&r.RunID, &r.WorkloadID, &r.Goal, &r.Source, &r.AgentName, &r.Status, &r.Project, &r.Repo, &r.GitBranch,
			&r.Phase, &r.Progress, &r.StartedAt, &r.LastHeartbeatAt, &r.HeartbeatCount, &r.StatusMessage); err != nil {
			return nil, err
		}
		r.LastActivity = firstNonEmptyStorage(r.LastHeartbeatAt, r.StartedAt)
		if t, ok := parseDBTime(r.LastActivity); ok {
			r.AgeSeconds = int64(now.Sub(t).Seconds())
			r.Stale = now.Sub(t) > maxAge
		}
		if staleOnly && !r.Stale {
			continue
		}
		out = append(out, r)
	}
	if out == nil {
		out = []AgentRunLivenessRow{}
	}
	return out, rows.Err()
}

func recordAgentRunHeartbeatTx(tx *sql.Tx, in agentRunHeartbeatInput) (*AgentRunEventRow, error) {
	in.RunID = strings.TrimSpace(in.RunID)
	if in.RunID == "" {
		return nil, fmt.Errorf("run_id is required")
	}
	if in.Timestamp.IsZero() {
		in.Timestamp = time.Now().UTC()
	}
	if in.Confidence <= 0 {
		in.Confidence = 1
	}
	in.Status = strings.ToLower(strings.TrimSpace(in.Status))
	if in.Status == "" {
		in.Status = "running"
	}
	if !validAgentRunStatus(in.Status) {
		return nil, fmt.Errorf("unsupported agent run status %q", in.Status)
	}
	if in.Progress < 0 || in.Progress > 1 {
		return nil, fmt.Errorf("progress must be in [0,1]")
	}
	in.Phase = strings.TrimSpace(in.Phase)
	in.Message = strings.TrimSpace(in.Message)
	if len(in.Phase) > 128 {
		return nil, fmt.Errorf("phase is too long: max 128 bytes")
	}
	if len(in.Message) > 512 {
		return nil, fmt.Errorf("message is too long: max 512 bytes")
	}
	metricsJSON, err := runHeartbeatMetricsJSON(in.Metrics)
	if err != nil {
		return nil, err
	}
	var workloadID, source, currentStatus string
	err = tx.QueryRow(`SELECT workload_id,source,status FROM agent_runs WHERE run_id=?`, in.RunID).Scan(&workloadID, &source, &currentStatus)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(in.WorkloadID) != "" && strings.TrimSpace(in.WorkloadID) != workloadID {
		return nil, fmt.Errorf("run_id %s belongs to workload %s, not %s", in.RunID, workloadID, in.WorkloadID)
	}
	if strings.TrimSpace(in.Source) != "" {
		source = strings.TrimSpace(in.Source)
	}
	if terminalAgentRunStatus(currentStatus) {
		if strings.TrimSpace(in.EventID) != "" {
			row, err := getAgentRunEventTx(tx, in.EventID)
			if err == nil && row.RunID == in.RunID {
				return row, nil
			}
			if err != nil && err != sql.ErrNoRows {
				return nil, err
			}
		}
		return nil, fmt.Errorf("run_id %s is already %s; heartbeat rejected", in.RunID, currentStatus)
	}
	if in.EventID == "" {
		in.EventID = generatedID("runevt")
	}
	res, err := tx.Exec(`INSERT OR IGNORE INTO agent_run_events(event_id,run_id,workload_id,source,event_type,status,phase,progress,message,metrics,timestamp,confidence)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		in.EventID, in.RunID, workloadID, source, "agent.run.heartbeat", in.Status, in.Phase, in.Progress, in.Message, metricsJSON, in.Timestamp, in.Confidence)
	if err != nil {
		return nil, err
	}
	inserted, _ := res.RowsAffected()
	if inserted > 0 {
		if _, err := tx.Exec(`UPDATE agent_runs SET status=?, phase=?, progress=?, status_message=?, last_heartbeat_at=?, heartbeat_count=COALESCE(heartbeat_count,0)+1
			WHERE run_id=?`, in.Status, in.Phase, in.Progress, in.Message, in.Timestamp, in.RunID); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`UPDATE workloads SET updated_at=?, status=CASE WHEN status IN ('active','queued','running','waiting_approval','evaluating','blocked','stalled') THEN ? ELSE status END
			WHERE workload_id=?`, in.Timestamp, workloadStatusForRunHeartbeat(in.Status), workloadID); err != nil {
			return nil, err
		}
	}
	row, err := getAgentRunEventTx(tx, in.EventID)
	if err != nil {
		return nil, err
	}
	if row.RunID != in.RunID {
		return nil, fmt.Errorf("heartbeat event_id %s already belongs to run %s", in.EventID, row.RunID)
	}
	return row, nil
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
	if detail.RunEvents, err = d.getAgentRunEvents(workloadID); err != nil {
		return nil, err
	}
	if detail.ModelCalls, err = d.getModelCallDetails(workloadID); err != nil {
		return nil, err
	}
	if detail.ToolCalls, err = d.getToolCalls(workloadID); err != nil {
		return nil, err
	}
	if detail.ContextRefs, err = d.getContextRefs(workloadID); err != nil {
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
	if detail.Links, err = d.GetWorkloadLinks(workloadID); err != nil {
		return nil, err
	}
	if detail.Sessions, err = d.getWorkloadSessions(workloadID); err != nil {
		return nil, err
	}
	return detail, nil
}

// GetWorkloadState returns a derived terminal-state snapshot for one workload.
func (d *DB) GetWorkloadState(workloadID string, staleAfter time.Duration) (*WorkloadState, error) {
	if staleAfter <= 0 {
		staleAfter = 10 * time.Minute
	}
	detail, err := d.GetWorkloadDetail(workloadID)
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	state := &WorkloadState{
		WorkloadID:               detail.Summary.WorkloadID,
		Goal:                     detail.Summary.Goal,
		Status:                   detail.Summary.Status,
		Source:                   detail.Summary.Source,
		Project:                  detail.Summary.Project,
		Repo:                     detail.Summary.Repo,
		GitBranch:                detail.Summary.GitBranch,
		Team:                     detail.Summary.Team,
		BudgetUSD:                detail.Summary.BudgetUSD,
		CostUSD:                  detail.Summary.CostUSD,
		Tokens:                   detail.Summary.Tokens,
		Runs:                     len(detail.Runs),
		ModelCalls:               len(detail.ModelCalls),
		ToolCalls:                len(detail.ToolCalls),
		ContextRefs:              len(detail.ContextRefs),
		Artifacts:                len(detail.Artifacts),
		Evaluations:              len(detail.Evaluations),
		StaleAfterSeconds:        int64(staleAfter.Seconds()),
		EstimatedRemainingBudget: detail.Summary.BudgetUSD - detail.Summary.CostUSD,
	}
	if detail.Summary.BudgetUSD > 0 && detail.Summary.CostUSD >= detail.Summary.BudgetUSD {
		state.EstimatedBudgetExhausted = true
	}

	latest := latestWorkloadActivity(detail)
	if latest.IsZero() {
		latest = now
	}
	state.LastActivity = latest.UTC().Format(time.RFC3339Nano)
	state.Terminal = isTerminalWorkloadStatus(detail.Summary.Status) || strings.TrimSpace(detail.Summary.ClosedAt) != ""

	maxProgress := 0.0
	for _, run := range detail.Runs {
		if run.Progress > maxProgress {
			maxProgress = run.Progress
		}
		status := strings.ToLower(strings.TrimSpace(run.Status))
		switch {
		case isSuccessfulRunStatus(status):
			state.CompletedRuns++
		case isFailedRunStatus(status):
			state.FailedRuns++
		case isTerminalRunStatus(status):
			state.CompletedRuns++
		case state.Terminal:
			state.CompletedRuns++
		default:
			state.ActiveRuns++
			activity := staleRunActivity(run)
			if !activity.IsZero() && now.Sub(activity) > staleAfter {
				state.StaleRuns++
			}
		}
	}
	state.Stale = state.StaleRuns > 0

	for _, evaluation := range detail.Evaluations {
		status := strings.ToLower(strings.TrimSpace(evaluation.Status))
		switch {
		case isPositiveEvaluationStatus(status):
			state.PositiveEvaluations++
		case isNegativeEvaluationStatus(status):
			state.NegativeEvaluations++
		}
	}
	for _, decision := range detail.Policies {
		action := strings.ToLower(strings.TrimSpace(decision.Action))
		switch action {
		case "block", "blocked", "deny", "denied":
			state.PolicyBlocks++
		case "require_approval", "approval_required", "approval":
			state.PolicyApprovalsRequired++
		}
	}

	state.Phase, state.NextAction = deriveWorkloadPhase(state)
	state.Reasons = workloadStateReasons(state)
	state.Risks = workloadStateRisks(state)
	state.ReadinessScore = workloadReadinessScore(state)
	state.Progress = clamp01(maxProgress)
	if state.Terminal || state.PositiveEvaluations > 0 && state.Artifacts > 0 {
		state.Progress = 1
	} else if state.Progress == 0 {
		state.Progress = state.ReadinessScore
	}
	return state, nil
}

// GetWorkloadStates returns bounded terminal-state snapshots for workloads in a window.
func (d *DB) GetWorkloadStates(from, to time.Time, source, model, project string, limit int, staleAfter time.Duration) ([]WorkloadState, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	page, err := d.GetWorkloadsPage(from, to, source, model, project, "", "", limit, 0)
	if err != nil {
		return nil, err
	}
	states := make([]WorkloadState, 0, len(page.Rows))
	for _, row := range page.Rows {
		state, err := d.GetWorkloadState(row.WorkloadID, staleAfter)
		if err != nil {
			return nil, err
		}
		states = append(states, *state)
	}
	if states == nil {
		states = []WorkloadState{}
	}
	return states, nil
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
	for _, c := range detail.ContextRefs {
		label := firstNonEmpty(c.Label, c.RefType, c.RefHash)
		graph.Nodes = append(graph.Nodes, GraphNode{ID: c.ContextRefID, Kind: "context", Label: label, Meta: map[string]string{"type": c.RefType, "privacy": c.PrivacyLabel}})
		if c.RunID != "" {
			graph.Edges = append(graph.Edges, GraphEdge{From: c.RunID, To: c.ContextRefID, Label: "context"})
		} else {
			graph.Edges = append(graph.Edges, GraphEdge{From: detail.Summary.WorkloadID, To: c.ContextRefID, Label: "context"})
		}
	}
	for _, link := range detail.Links {
		otherID := link.TargetWorkloadID
		if otherID == detail.Summary.WorkloadID {
			otherID = link.SourceWorkloadID
		}
		graph.Nodes = append(graph.Nodes, GraphNode{ID: otherID, Kind: "workload", Label: otherID, Meta: map[string]string{"relation": link.Relation}})
		graph.Edges = append(graph.Edges, GraphEdge{From: link.SourceWorkloadID, To: link.TargetWorkloadID, Label: link.Relation})
	}
	return graph, nil
}

// GetWorkloadTimeline returns a chronological metadata-only event stream for audit and replay.
func (d *DB) GetWorkloadTimeline(workloadID string, limit int) ([]WorkloadTimelineRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	detail, err := d.GetWorkloadDetail(workloadID)
	if err != nil {
		return nil, err
	}
	rows := []WorkloadTimelineRow{{
		Kind:       "workload",
		ID:         detail.Summary.WorkloadID,
		Source:     detail.Summary.Source,
		Label:      detail.Summary.Goal,
		Status:     detail.Summary.Status,
		Detail:     firstNonEmpty(detail.Summary.Project, detail.Summary.Repo),
		Tokens:     detail.Summary.Tokens,
		CostUSD:    detail.Summary.CostUSD,
		Timestamp:  detail.Summary.CreatedAt,
		Confidence: detail.Summary.Confidence,
	}}
	for _, r := range detail.Runs {
		rows = append(rows, WorkloadTimelineRow{
			Kind:       "agent_run",
			ID:         r.RunID,
			RunID:      r.RunID,
			Source:     r.Source,
			Label:      firstNonEmpty(r.AgentName, r.Source, "agent"),
			Status:     r.Status,
			Detail:     r.Phase,
			DurationMS: r.DurationMS,
			Timestamp:  r.StartedAt,
			Confidence: r.Confidence,
		})
	}
	for _, e := range detail.RunEvents {
		rows = append(rows, WorkloadTimelineRow{
			Kind:       "run_event",
			ID:         e.EventID,
			RunID:      e.RunID,
			Source:     e.Source,
			Label:      firstNonEmpty(e.Phase, e.EventType),
			Status:     e.Status,
			Detail:     e.Message,
			Timestamp:  e.Timestamp,
			Confidence: e.Confidence,
		})
	}
	for _, c := range detail.ModelCalls {
		rows = append(rows, WorkloadTimelineRow{
			Kind:       "model_call",
			ID:         firstNonEmpty(c.CallID, "model:"+c.Source+":"+c.Model+":"+c.LastAt),
			RunID:      c.RunID,
			Source:     c.Source,
			Label:      c.Model,
			Status:     c.PricingConfidence,
			Detail:     c.Provider,
			Tokens:     c.Tokens,
			CostUSD:    c.CostUSD,
			Timestamp:  firstNonEmpty(c.LastAt, c.FirstAt),
			Confidence: c.Confidence,
		})
	}
	for _, t := range detail.ToolCalls {
		rows = append(rows, WorkloadTimelineRow{
			Kind:       "tool_call",
			ID:         t.ToolCallID,
			RunID:      t.RunID,
			Source:     t.Source,
			Label:      t.ToolName,
			Status:     t.Status,
			Detail:     firstNonEmpty(t.ToolType, t.ErrorClass),
			DurationMS: t.DurationMS,
			Timestamp:  t.Timestamp,
			Confidence: t.Confidence,
		})
	}
	for _, c := range detail.ContextRefs {
		rows = append(rows, WorkloadTimelineRow{
			Kind:       "context_ref",
			ID:         c.ContextRefID,
			RunID:      c.RunID,
			Label:      firstNonEmpty(c.Label, c.RefType, c.RefHash),
			Status:     c.PrivacyLabel,
			Detail:     firstNonEmpty(c.Repo, c.RefHash),
			Timestamp:  c.CreatedAt,
			Confidence: c.Confidence,
		})
	}
	for _, a := range detail.Artifacts {
		rows = append(rows, WorkloadTimelineRow{
			Kind:       "artifact",
			ID:         a.ArtifactID,
			RunID:      a.RunID,
			Label:      firstNonEmpty(a.Label, a.ArtifactType),
			Status:     a.ArtifactType,
			Detail:     firstNonEmpty(a.SHA256, a.PathHash),
			Timestamp:  a.CreatedAt,
			Confidence: a.Confidence,
		})
	}
	for _, e := range detail.Evaluations {
		rows = append(rows, WorkloadTimelineRow{
			Kind:      "evaluation",
			ID:        e.EvaluationID,
			Label:     firstNonEmpty(e.Signal, e.Evaluator, "evaluation"),
			Status:    e.Status,
			Detail:    e.Notes,
			Timestamp: e.CreatedAt,
		})
	}
	for _, p := range detail.Policies {
		rows = append(rows, WorkloadTimelineRow{
			Kind:      "policy",
			ID:        p.DecisionID,
			RunID:     p.RunID,
			Label:     firstNonEmpty(p.RuleID, p.Action),
			Status:    p.Action,
			Detail:    p.Reason,
			Timestamp: p.CreatedAt,
		})
	}
	for _, link := range detail.Links {
		rows = append(rows, WorkloadTimelineRow{
			Kind:       "workload_link",
			ID:         link.LinkID,
			Label:      link.Relation,
			Status:     link.TargetWorkloadID,
			Detail:     link.Reason,
			Timestamp:  link.CreatedAt,
			Confidence: link.Confidence,
		})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		ti, iok := parseDBTime(rows[i].Timestamp)
		tj, jok := parseDBTime(rows[j].Timestamp)
		if iok && jok && !ti.Equal(tj) {
			return ti.Before(tj)
		}
		if iok != jok {
			return iok
		}
		if rows[i].Timestamp != rows[j].Timestamp {
			return rows[i].Timestamp < rows[j].Timestamp
		}
		return rows[i].ID < rows[j].ID
	})
	if len(rows) > limit {
		rows = rows[len(rows)-limit:]
	}
	return rows, nil
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

// LinkWorkloads creates or updates a dependency/lineage edge between two workloads.
func (d *DB) LinkWorkloads(sourceWorkloadID, targetWorkloadID, relation, reason, createdBy string, confidence float64) (string, error) {
	sourceWorkloadID = strings.TrimSpace(sourceWorkloadID)
	targetWorkloadID = strings.TrimSpace(targetWorkloadID)
	if sourceWorkloadID == "" || targetWorkloadID == "" {
		return "", fmt.Errorf("source_workload_id and target_workload_id are required")
	}
	if sourceWorkloadID == targetWorkloadID {
		return "", fmt.Errorf("workload link cannot target itself")
	}
	relation = normalizeWorkloadRelation(relation)
	if !validWorkloadRelation(relation) {
		return "", fmt.Errorf("unsupported workload relation %q", relation)
	}
	if err := validateShortMetadata("reason", reason, 512); err != nil {
		return "", err
	}
	if err := validateShortMetadata("created_by", createdBy, 128); err != nil {
		return "", err
	}
	if confidence <= 0 {
		confidence = 1
	}
	if confidence > 1 {
		confidence = 1
	}
	var existing int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM workloads WHERE workload_id IN (?,?)`, sourceWorkloadID, targetWorkloadID).Scan(&existing); err != nil {
		return "", err
	}
	if existing != 2 {
		return "", fmt.Errorf("both source and target workloads must exist")
	}
	id := stableID("lnk", sourceWorkloadID+"\x00"+targetWorkloadID+"\x00"+relation)
	now := time.Now().UTC()
	_, err := d.db.Exec(`INSERT INTO workload_links(link_id,source_workload_id,target_workload_id,relation,reason,created_by,created_at,confidence)
		VALUES(?,?,?,?,?,?,?,?)
		ON CONFLICT(source_workload_id,target_workload_id,relation) DO UPDATE SET
			reason=excluded.reason,
			created_by=excluded.created_by,
			created_at=excluded.created_at,
			confidence=excluded.confidence`,
		id, sourceWorkloadID, targetWorkloadID, relation, strings.TrimSpace(reason), strings.TrimSpace(createdBy), now, confidence)
	if err != nil {
		return "", err
	}
	_, _ = d.db.Exec(`UPDATE workloads SET updated_at=? WHERE workload_id IN (?,?)`, now, sourceWorkloadID, targetWorkloadID)
	return id, nil
}

// GetWorkloadLinks returns incoming and outgoing workload dependency edges.
func (d *DB) GetWorkloadLinks(workloadID string) ([]WorkloadLinkRow, error) {
	if workloadID == "" {
		return nil, fmt.Errorf("workload_id is required")
	}
	rows, err := d.db.Query(`SELECT link_id,source_workload_id,target_workload_id,relation,reason,created_by,created_at,confidence
		FROM workload_links
		WHERE source_workload_id=? OR target_workload_id=?
		ORDER BY created_at DESC`, workloadID, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []WorkloadLinkRow
	for rows.Next() {
		var r WorkloadLinkRow
		if err := rows.Scan(&r.LinkID, &r.SourceWorkloadID, &r.TargetWorkloadID, &r.Relation, &r.Reason, &r.CreatedBy, &r.CreatedAt, &r.Confidence); err != nil {
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
		started_at,COALESCE(ended_at,''),duration_ms,COALESCE(last_heartbeat_at,''),COALESCE(heartbeat_count,0),COALESCE(phase,''),COALESCE(progress,0),COALESCE(status_message,''),confidence
		FROM agent_runs WHERE workload_id=? ORDER BY started_at`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentRunRow
	for rows.Next() {
		var r AgentRunRow
		if err := rows.Scan(&r.RunID, &r.WorkloadID, &r.ParentRunID, &r.Source, &r.AgentName, &r.AgentVersion, &r.Command, &r.CWD,
			&r.Status, &r.ExitCode, &r.Error, &r.StartedAt, &r.EndedAt, &r.DurationMS, &r.LastHeartbeatAt, &r.HeartbeatCount,
			&r.Phase, &r.Progress, &r.StatusMessage, &r.Confidence); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (d *DB) getAgentRunEvents(workloadID string) ([]AgentRunEventRow, error) {
	rows, err := d.db.Query(`SELECT event_id,run_id,workload_id,source,event_type,status,phase,progress,message,metrics,timestamp,confidence
		FROM agent_run_events WHERE workload_id=? ORDER BY timestamp DESC LIMIT 500`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AgentRunEventRow
	for rows.Next() {
		var r AgentRunEventRow
		if err := rows.Scan(&r.EventID, &r.RunID, &r.WorkloadID, &r.Source, &r.EventType, &r.Status, &r.Phase, &r.Progress,
			&r.Message, &r.Metrics, &r.Timestamp, &r.Confidence); err != nil {
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

func (d *DB) getContextRefs(workloadID string) ([]ContextRefRow, error) {
	rows, err := d.db.Query(`SELECT context_ref_id,workload_id,run_id,ref_type,ref_hash,label,repo,git_branch,commit_sha,privacy_label,created_at,confidence
		FROM context_refs WHERE workload_id=? ORDER BY created_at DESC LIMIT 500`, workloadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ContextRefRow
	for rows.Next() {
		var r ContextRefRow
		if err := rows.Scan(&r.ContextRefID, &r.WorkloadID, &r.RunID, &r.RefType, &r.RefHash, &r.Label, &r.Repo, &r.GitBranch,
			&r.CommitSHA, &r.PrivacyLabel, &r.CreatedAt, &r.Confidence); err != nil {
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
	case "active", "queued", "running", "waiting_approval", "evaluating", "blocked", "stalled", "completed", "partial", "failed", "abandoned", "cancelled", "merged", "deployed", "reverted":
		return true
	default:
		return false
	}
}

func validAgentRunStatus(status string) bool {
	switch status {
	case "queued", "running", "working", "waiting_approval", "blocked", "evaluating", "stalled", "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func terminalAgentRunStatus(status string) bool {
	switch status {
	case "completed", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func terminalWorkloadStatus(status string) bool {
	switch status {
	case "completed", "partial", "failed", "abandoned", "cancelled", "merged", "deployed", "reverted":
		return true
	default:
		return false
	}
}

func workloadStatusForRunHeartbeat(status string) string {
	switch status {
	case "working":
		return "running"
	case "completed", "failed", "cancelled":
		return status
	default:
		return status
	}
}

func runHeartbeatMetricsJSON(metrics map[string]interface{}) (string, error) {
	if metrics == nil {
		return "{}", nil
	}
	if containsPromptContentKey(metrics) {
		return "", fmt.Errorf("heartbeat metrics appear to contain prompt/content text; store hashes or metadata only")
	}
	raw, err := json.Marshal(metrics)
	if err != nil {
		return "", err
	}
	if len(raw) > 16<<10 {
		return "", fmt.Errorf("heartbeat metrics are too large: max 16 KiB")
	}
	return string(raw), nil
}

func redactCommandSecrets(command string) string {
	if strings.TrimSpace(command) == "" {
		return command
	}
	out := command
	for _, pattern := range commandSecretPatterns {
		out = pattern.ReplaceAllString(out, `${1}<redacted>`)
	}
	return out
}

func getAgentRunEventTx(tx *sql.Tx, eventID string) (*AgentRunEventRow, error) {
	row := AgentRunEventRow{}
	err := tx.QueryRow(`SELECT event_id,run_id,workload_id,source,event_type,status,phase,progress,message,metrics,timestamp,confidence
		FROM agent_run_events WHERE event_id=?`, eventID).Scan(&row.EventID, &row.RunID, &row.WorkloadID, &row.Source, &row.EventType, &row.Status,
		&row.Phase, &row.Progress, &row.Message, &row.Metrics, &row.Timestamp, &row.Confidence)
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func latestWorkloadActivity(detail *WorkloadDetail) time.Time {
	var latest time.Time
	consider := func(raw string) {
		if t, ok := parseDBTime(raw); ok && t.After(latest) {
			latest = t
		}
	}
	consider(detail.Summary.LastActivity)
	consider(detail.Summary.UpdatedAt)
	consider(detail.Summary.CreatedAt)
	consider(detail.Summary.ClosedAt)
	for _, run := range detail.Runs {
		if t := latestRunActivity(run); t.After(latest) {
			latest = t
		}
	}
	for _, event := range detail.RunEvents {
		consider(event.Timestamp)
	}
	for _, call := range detail.ModelCalls {
		consider(call.LastAt)
		consider(call.FirstAt)
	}
	for _, tool := range detail.ToolCalls {
		consider(tool.Timestamp)
	}
	for _, ref := range detail.ContextRefs {
		consider(ref.CreatedAt)
	}
	for _, artifact := range detail.Artifacts {
		consider(artifact.CreatedAt)
	}
	for _, evaluation := range detail.Evaluations {
		consider(evaluation.CreatedAt)
	}
	for _, policy := range detail.Policies {
		consider(policy.CreatedAt)
	}
	return latest
}

func latestRunActivity(run AgentRunRow) time.Time {
	var latest time.Time
	for _, raw := range []string{run.LastHeartbeatAt, run.EndedAt, run.StartedAt} {
		if t, ok := parseDBTime(raw); ok && t.After(latest) {
			latest = t
		}
	}
	return latest
}

func staleRunActivity(run AgentRunRow) time.Time {
	if run.HeartbeatCount > 0 && strings.TrimSpace(run.LastHeartbeatAt) != "" {
		t, _ := parseDBTime(run.LastHeartbeatAt)
		return t
	}
	t, _ := parseDBTime(run.StartedAt)
	return t
}

func deriveWorkloadPhase(state *WorkloadState) (string, string) {
	switch {
	case state.PolicyBlocks > 0:
		return "blocked", "resolve blocking policy decision"
	case state.PolicyApprovalsRequired > 0:
		return "needs_approval", "approve, reject, or revise the guarded action"
	case state.NegativeEvaluations > 0 && !state.Terminal:
		return "needs_revision", "address failing evaluation signal"
	case state.Terminal && state.PositiveEvaluations > 0:
		return "accepted", "archive, report, or use as precedent"
	case state.Terminal && state.NegativeEvaluations > 0:
		return "rejected", "review failure signal and reopen if needed"
	case state.Terminal:
		return "terminal", "archive or report workload"
	case state.StaleRuns > 0:
		return "stale", "inspect stale agent run heartbeat"
	case state.ActiveRuns > 0:
		return "running", "continue monitoring run heartbeat"
	case state.Artifacts > 0 && state.Evaluations == 0:
		return "needs_evaluation", "record review, test, or acceptance signal"
	case state.Runs == 0:
		return "planned", "start an agent run or attach an existing session"
	default:
		return "active", "continue workload instrumentation"
	}
}

func workloadStateReasons(state *WorkloadState) []string {
	reasons := []string{}
	if state.Runs > 0 {
		reasons = append(reasons, fmt.Sprintf("%d runs recorded", state.Runs))
	}
	if state.ModelCalls > 0 {
		reasons = append(reasons, fmt.Sprintf("%d model call groups", state.ModelCalls))
	}
	if state.ToolCalls > 0 {
		reasons = append(reasons, fmt.Sprintf("%d tool calls", state.ToolCalls))
	}
	if state.ContextRefs > 0 {
		reasons = append(reasons, fmt.Sprintf("%d context refs", state.ContextRefs))
	}
	if state.Artifacts > 0 {
		reasons = append(reasons, fmt.Sprintf("%d artifacts", state.Artifacts))
	}
	if state.PositiveEvaluations > 0 {
		reasons = append(reasons, fmt.Sprintf("%d positive evaluations", state.PositiveEvaluations))
	}
	if state.Terminal {
		reasons = append(reasons, "workload is terminal")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "workload exists")
	}
	return reasons
}

func workloadStateRisks(state *WorkloadState) []string {
	risks := []string{}
	if state.PolicyBlocks > 0 {
		risks = append(risks, fmt.Sprintf("%d blocking policy decisions", state.PolicyBlocks))
	}
	if state.PolicyApprovalsRequired > 0 {
		risks = append(risks, fmt.Sprintf("%d approval-required policy decisions", state.PolicyApprovalsRequired))
	}
	if state.StaleRuns > 0 {
		risks = append(risks, fmt.Sprintf("%d stale active runs", state.StaleRuns))
	}
	if state.FailedRuns > 0 {
		risks = append(risks, fmt.Sprintf("%d failed runs", state.FailedRuns))
	}
	if state.NegativeEvaluations > 0 {
		risks = append(risks, fmt.Sprintf("%d negative evaluations", state.NegativeEvaluations))
	}
	if state.EstimatedBudgetExhausted {
		risks = append(risks, "budget exhausted")
	}
	return risks
}

func workloadReadinessScore(state *WorkloadState) float64 {
	score := 0.15
	if state.Runs > 0 {
		score += 0.15
	}
	if state.ContextRefs > 0 {
		score += 0.10
	}
	if state.ModelCalls > 0 {
		score += 0.15
	}
	if state.ToolCalls > 0 {
		score += 0.10
	}
	if state.Artifacts > 0 {
		score += 0.15
	}
	if state.PositiveEvaluations > 0 {
		score += 0.20
	} else if state.Evaluations > 0 {
		score += 0.10
	}
	if state.Terminal {
		score += 0.10
	}
	if state.PolicyBlocks > 0 {
		score -= 0.25
	}
	if state.PolicyApprovalsRequired > 0 {
		score -= 0.15
	}
	if state.StaleRuns > 0 {
		score -= 0.20
	}
	if state.FailedRuns > 0 {
		score -= 0.10
	}
	if state.NegativeEvaluations > 0 {
		score -= 0.20
	}
	return clamp01(score)
}

func isTerminalWorkloadStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "closed", "failed", "failure", "error", "cancelled", "canceled", "accepted", "rejected", "merged", "deployed", "reverted", "abandoned", "partial":
		return true
	default:
		return false
	}
}

func isTerminalRunStatus(status string) bool {
	return isSuccessfulRunStatus(status) || isFailedRunStatus(status) || status == "cancelled" || status == "canceled"
}

func isSuccessfulRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "complete", "succeeded", "success", "done", "closed":
		return true
	default:
		return false
	}
}

func isFailedRunStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "failed", "failure", "error", "errored", "timeout", "timed_out":
		return true
	default:
		return false
	}
}

func isPositiveEvaluationStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "pass", "passed", "success", "succeeded", "accepted", "approved", "complete", "completed", "ok":
		return true
	default:
		return false
	}
}

func isNegativeEvaluationStatus(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "fail", "failed", "failure", "rejected", "blocked", "regressed", "needs_work", "needs-rework":
		return true
	default:
		return false
	}
}

func normalizeWorkloadRelation(relation string) string {
	relation = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(relation, "-", "_")))
	switch relation {
	case "", "related", "relates", "relates_to":
		return "relates_to"
	case "depends", "dependency", "depends_on":
		return "depends_on"
	case "blocks":
		return "blocks"
	case "blocked_by":
		return "blocked_by"
	case "spawns", "spawned", "spawned_by":
		return "spawned_by"
	case "supersedes", "replaces":
		return "supersedes"
	case "superseded_by", "replaced_by":
		return "superseded_by"
	case "duplicates", "duplicate_of":
		return "duplicates"
	case "parent", "parent_of":
		return "parent_of"
	case "child", "child_of":
		return "child_of"
	default:
		return relation
	}
}

func validWorkloadRelation(relation string) bool {
	switch relation {
	case "relates_to", "depends_on", "blocks", "blocked_by", "spawned_by", "supersedes", "superseded_by", "duplicates", "parent_of", "child_of":
		return true
	default:
		return false
	}
}

func validateShortMetadata(name, value string, maxLen int) error {
	value = strings.TrimSpace(value)
	if len(value) > maxLen {
		return fmt.Errorf("%s is too long: max %d bytes", name, maxLen)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
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
