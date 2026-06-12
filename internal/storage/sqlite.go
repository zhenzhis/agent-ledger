package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// DB wraps a SQLite database connection with a mutex for safe concurrent access.
type DB struct {
	db             *sql.DB
	mu             sync.Mutex
	projectAliases map[string]string
	projectExclude []string
}

// UsageRecord represents a single API call's token usage and cost.
type UsageRecord struct {
	ID                       int64
	Source                   string // "claude" or "codex"
	SessionID                string
	Model                    string
	InputTokens              int64
	OutputTokens             int64
	CacheCreationInputTokens int64
	CacheReadInputTokens     int64
	ReasoningOutputTokens    int64
	CostUSD                  float64
	Timestamp                time.Time
	Project                  string
	GitBranch                string
	PricingSource            string
	PricingModel             string
	PricingConfidence        string
	PricingNote              string
}

// SessionRecord represents metadata for a coding agent session.
type SessionRecord struct {
	ID        int64
	Source    string
	SessionID string
	Project   string
	CWD       string
	Version   string
	GitBranch string
	StartTime time.Time
	Prompts   int
}

// PromptEvent represents a single user prompt with its timestamp.
type PromptEvent struct {
	Source    string
	SessionID string
	Model     string
	Project   string
	Timestamp time.Time
}

// PathStatus describes whether a configured collector path is usable.
type PathStatus struct {
	Path     string `json:"path"`
	Exists   bool   `json:"exists"`
	Readable bool   `json:"readable"`
	Error    string `json:"error,omitempty"`
}

// IngestionHealth records the most recent scan status for one source.
type IngestionHealth struct {
	Source          string       `json:"source"`
	Enabled         bool         `json:"enabled"`
	Paths           []string     `json:"paths"`
	PathStatus      []PathStatus `json:"path_status"`
	LastScanAt      string       `json:"last_scan_at"`
	DurationMS      int64        `json:"duration_ms"`
	Watermark       string       `json:"watermark"`
	FilesSeen       int          `json:"files_seen"`
	RecordsInserted int          `json:"records_inserted"`
	PromptsInserted int          `json:"prompts_inserted"`
	SkippedRows     int          `json:"skipped_rows"`
	LastError       string       `json:"last_error"`
}

// BudgetEvent records the latest budget state for a rule and period.
type BudgetEvent struct {
	EventKey  string
	RuleName  string
	Period    string
	Scope     string
	Match     string
	Metric    string
	Value     float64
	Limit     float64
	Severity  string
	Message   string
	CreatedAt time.Time
}

// PricingSourceStatus records health and freshness for one pricing source.
type PricingSourceStatus struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Priority    int    `json:"priority"`
	URL         string `json:"url"`
	LastFetchAt string `json:"last_fetch_at"`
	ETag        string `json:"etag"`
	SHA256      string `json:"sha256"`
	ModelCount  int    `json:"model_count"`
	Status      string `json:"status"`
	LastError   string `json:"last_error"`
	Stale       bool   `json:"stale"`
}

// PricingAuditRow describes an effective pricing rule.
type PricingAuditRow struct {
	Model                  string  `json:"model"`
	PricingSource          string  `json:"pricing_source"`
	MatchedModel           string  `json:"matched_model"`
	MatchType              string  `json:"match_type"`
	Priority               int     `json:"priority"`
	InputCostPerToken      float64 `json:"input_cost_per_token"`
	OutputCostPerToken     float64 `json:"output_cost_per_token"`
	CacheReadCostPerToken  float64 `json:"cache_read_input_token_cost"`
	CacheWriteCostPerToken float64 `json:"cache_creation_input_token_cost"`
	EffectiveAt            string  `json:"effective_at"`
	UpdatedAt              string  `json:"updated_at"`
	Confidence             string  `json:"confidence"`
}

// PricingRuleSummary summarizes the effective pricing rule set.
type PricingRuleSummary struct {
	TotalRules      int            `json:"total_rules"`
	BySource        map[string]int `json:"by_source"`
	ByConfidence    map[string]int `json:"by_confidence"`
	OverrideRules   int            `json:"override_rules"`
	OfficialRules   int            `json:"official_rules"`
	FallbackRules   int            `json:"fallback_rules"`
	OldestUpdatedAt string         `json:"oldest_updated_at"`
	NewestUpdatedAt string         `json:"newest_updated_at"`
}

// AuditEvent is a local immutable operational audit event.
type AuditEvent struct {
	ID        int64  `json:"id"`
	Actor     string `json:"actor"`
	Role      string `json:"role"`
	Action    string `json:"action"`
	Target    string `json:"target"`
	Params    string `json:"params"`
	CreatedAt string `json:"created_at"`
}

// AuditLogFilter constrains local operational audit log queries.
type AuditLogFilter struct {
	From   time.Time
	To     time.Time
	Actor  string
	Role   string
	Action string
	Target string
	Limit  int
}

// ApprovalRequest is a local policy approval request.
type ApprovalRequest struct {
	RequestID              string `json:"request_id"`
	PolicyDecisionID       string `json:"policy_decision_id"`
	WorkloadID             string `json:"workload_id"`
	RunID                  string `json:"run_id"`
	Source                 string `json:"source"`
	Model                  string `json:"model"`
	Project                string `json:"project"`
	Action                 string `json:"action"`
	Target                 string `json:"target"`
	ActorRole              string `json:"actor_role"`
	Status                 string `json:"status"`
	RequiredApprovals      int    `json:"required_approvals"`
	ApprovalVotes          int    `json:"approval_votes"`
	RejectionVotes         int    `json:"rejection_votes"`
	ApproverHint           string `json:"approver_hint"`
	EscalationTarget       string `json:"escalation_target"`
	EscalationAfterSeconds int64  `json:"escalation_after_seconds"`
	DueAt                  string `json:"due_at"`
	Overdue                bool   `json:"overdue"`
	Reason                 string `json:"reason"`
	RequestPayload         string `json:"request_payload"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
	DecidedAt              string `json:"decided_at"`
	DecidedBy              string `json:"decided_by"`
	DecisionNote           string `json:"decision_note"`
}

// ApprovalOperation describes the operation context an approved request must match.
type ApprovalOperation struct {
	RequestID string
	Action    string
	Target    string
	Source    string
	Model     string
	Project   string
}

// ApprovalVote is one local actor decision on an approval request.
type ApprovalVote struct {
	RequestID string `json:"request_id"`
	Voter     string `json:"voter"`
	Role      string `json:"role"`
	Status    string `json:"status"`
	Note      string `json:"note"`
	CreatedAt string `json:"created_at"`
}

// ApprovalVoteResult summarizes the request state after recording a vote.
type ApprovalVoteResult struct {
	RequestID         string `json:"request_id"`
	Status            string `json:"status"`
	RequiredApprovals int    `json:"required_approvals"`
	ApprovalVotes     int    `json:"approval_votes"`
	RejectionVotes    int    `json:"rejection_votes"`
	Decided           bool   `json:"decided"`
}

// InsightEvent describes a local anomaly, watchdog, or quality signal.
type InsightEvent struct {
	ID        int64   `json:"id"`
	Kind      string  `json:"kind"`
	Severity  string  `json:"severity"`
	Source    string  `json:"source"`
	Model     string  `json:"model"`
	Project   string  `json:"project"`
	SessionID string  `json:"session_id"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Baseline  float64 `json:"baseline"`
	Message   string  `json:"message"`
	CreatedAt string  `json:"created_at"`
}

// Open creates or opens a SQLite database at the given path, enables WAL mode,
// and runs schema migrations.
func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(wal)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		return nil, err
	}
	return &DB{db: db}, nil
}

// Close closes the underlying database connection.
func (d *DB) Close() error { return d.db.Close() }

func migrate(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS usage_records (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL,
			model TEXT NOT NULL,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_creation_input_tokens INTEGER DEFAULT 0,
			cache_read_input_tokens INTEGER DEFAULT 0,
			reasoning_output_tokens INTEGER DEFAULT 0,
			cost_usd REAL DEFAULT 0,
			timestamp DATETIME NOT NULL,
			project TEXT DEFAULT '',
			git_branch TEXT DEFAULT ''
			,pricing_source TEXT DEFAULT ''
			,pricing_model TEXT DEFAULT ''
			,pricing_confidence TEXT DEFAULT ''
			,pricing_note TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON usage_records(timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_records(source, session_id);
		CREATE INDEX IF NOT EXISTS idx_usage_source ON usage_records(source);
		CREATE INDEX IF NOT EXISTS idx_usage_source_timestamp ON usage_records(source, timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_source_model_timestamp ON usage_records(source, model, timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_timestamp_session ON usage_records(timestamp, source, session_id);
		CREATE INDEX IF NOT EXISTS idx_usage_model_timestamp ON usage_records(model, timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_project_timestamp ON usage_records(project, timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_project_source_timestamp ON usage_records(project, source, timestamp);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_dedup ON usage_records(source, session_id, model, timestamp, input_tokens, output_tokens);

		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL,
			project TEXT DEFAULT '',
			cwd TEXT DEFAULT '',
			version TEXT DEFAULT '',
			git_branch TEXT DEFAULT '',
			start_time DATETIME,
			prompts INTEGER DEFAULT 0
		);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_source_session ON sessions(source, session_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_source_start ON sessions(source, start_time);
		CREATE INDEX IF NOT EXISTS idx_sessions_project_start ON sessions(project, start_time);

		CREATE TABLE IF NOT EXISTS prompt_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			session_id TEXT NOT NULL,
			model TEXT DEFAULT '',
			project TEXT DEFAULT '',
			timestamp DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_prompt_timestamp ON prompt_events(timestamp);
		CREATE INDEX IF NOT EXISTS idx_prompt_source_timestamp ON prompt_events(source, timestamp);
		CREATE INDEX IF NOT EXISTS idx_prompt_timestamp_session ON prompt_events(timestamp, source, session_id);
		CREATE UNIQUE INDEX IF NOT EXISTS idx_prompt_dedup ON prompt_events(source, session_id, timestamp);

		CREATE TABLE IF NOT EXISTS file_state (
			path TEXT PRIMARY KEY,
			size INTEGER DEFAULT 0,
			last_offset INTEGER DEFAULT 0,
			scan_context TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS pricing (
			model TEXT PRIMARY KEY,
			input_cost_per_token REAL DEFAULT 0,
			output_cost_per_token REAL DEFAULT 0,
			cache_read_input_token_cost REAL DEFAULT 0,
			cache_creation_input_token_cost REAL DEFAULT 0,
			updated_at DATETIME,
			pricing_source TEXT DEFAULT 'unknown',
			matched_model TEXT DEFAULT '',
			match_type TEXT DEFAULT 'direct',
			priority INTEGER DEFAULT 999,
			confidence TEXT DEFAULT 'unknown'
		);

		CREATE TABLE IF NOT EXISTS pricing_sources (
			name TEXT PRIMARY KEY,
			kind TEXT DEFAULT '',
			priority INTEGER DEFAULT 999,
			url TEXT DEFAULT '',
			last_fetch_at DATETIME,
			etag TEXT DEFAULT '',
			sha256 TEXT DEFAULT '',
			model_count INTEGER DEFAULT 0,
			status TEXT DEFAULT 'unknown',
			last_error TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS pricing_snapshots (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			source TEXT NOT NULL,
			sha256 TEXT NOT NULL,
			model_count INTEGER DEFAULT 0,
			raw_metadata TEXT DEFAULT '',
			fetched_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_pricing_snapshots_source_time ON pricing_snapshots(source, fetched_at);

		CREATE TABLE IF NOT EXISTS pricing_audit_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			source TEXT DEFAULT '',
			model TEXT DEFAULT '',
			message TEXT DEFAULT '',
			created_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_pricing_audit_created ON pricing_audit_events(created_at);

		CREATE TABLE IF NOT EXISTS meta (
			key TEXT PRIMARY KEY,
			value TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS ingestion_health (
			source TEXT PRIMARY KEY,
			enabled INTEGER DEFAULT 0,
			paths TEXT DEFAULT '[]',
			path_status TEXT DEFAULT '[]',
			last_scan_at DATETIME,
			duration_ms INTEGER DEFAULT 0,
			watermark TEXT DEFAULT '',
			files_seen INTEGER DEFAULT 0,
			records_inserted INTEGER DEFAULT 0,
			prompts_inserted INTEGER DEFAULT 0,
			skipped_rows INTEGER DEFAULT 0,
			last_error TEXT DEFAULT ''
		);

		CREATE TABLE IF NOT EXISTS budget_events (
			event_key TEXT PRIMARY KEY,
			rule_name TEXT NOT NULL,
			period TEXT NOT NULL,
			scope TEXT NOT NULL,
			match TEXT DEFAULT '',
			metric TEXT NOT NULL,
			value REAL DEFAULT 0,
			limit_value REAL DEFAULT 0,
			severity TEXT NOT NULL,
			message TEXT DEFAULT '',
			created_at DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS audit_log (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			actor TEXT DEFAULT 'local',
			role TEXT DEFAULT 'admin',
			action TEXT NOT NULL,
			target TEXT DEFAULT '',
			params TEXT DEFAULT '',
			created_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at);

		CREATE TABLE IF NOT EXISTS approval_requests (
			request_id TEXT PRIMARY KEY,
			policy_decision_id TEXT DEFAULT '',
			workload_id TEXT DEFAULT '',
			run_id TEXT DEFAULT '',
			source TEXT DEFAULT '',
			model TEXT DEFAULT '',
			project TEXT DEFAULT '',
			action TEXT DEFAULT '',
			target TEXT DEFAULT '',
			actor_role TEXT DEFAULT '',
			status TEXT DEFAULT 'pending',
			required_approvals INTEGER DEFAULT 1,
			approver_hint TEXT DEFAULT '',
			escalation_target TEXT DEFAULT '',
			escalation_after_seconds INTEGER DEFAULT 0,
			due_at DATETIME,
			reason TEXT DEFAULT '',
			request_payload TEXT DEFAULT '',
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			decided_at DATETIME,
			decided_by TEXT DEFAULT '',
			decision_note TEXT DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_approval_requests_status_created ON approval_requests(status, created_at);
		CREATE INDEX IF NOT EXISTS idx_approval_requests_action_target ON approval_requests(action, target);

		CREATE TABLE IF NOT EXISTS approval_votes (
			request_id TEXT NOT NULL,
			voter TEXT NOT NULL,
			role TEXT DEFAULT '',
			status TEXT NOT NULL,
			note TEXT DEFAULT '',
			created_at DATETIME NOT NULL,
			PRIMARY KEY(request_id, voter)
		);
		CREATE INDEX IF NOT EXISTS idx_approval_votes_request_status ON approval_votes(request_id, status);

		CREATE TABLE IF NOT EXISTS insight_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_key TEXT,
			kind TEXT NOT NULL,
			severity TEXT NOT NULL,
			source TEXT DEFAULT '',
			model TEXT DEFAULT '',
			project TEXT DEFAULT '',
			session_id TEXT DEFAULT '',
			metric TEXT DEFAULT '',
			value REAL DEFAULT 0,
			baseline REAL DEFAULT 0,
			message TEXT DEFAULT '',
			created_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_insight_kind_created ON insight_events(kind, created_at);

		CREATE TABLE IF NOT EXISTS hourly_usage_aggregate (
			bucket DATETIME NOT NULL,
			source TEXT NOT NULL,
			model TEXT NOT NULL,
			project TEXT DEFAULT '',
			git_branch TEXT DEFAULT '',
			calls INTEGER DEFAULT 0,
			prompts INTEGER DEFAULT 0,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_read_input_tokens INTEGER DEFAULT 0,
			cache_creation_input_tokens INTEGER DEFAULT 0,
			reasoning_output_tokens INTEGER DEFAULT 0,
			cost_usd REAL DEFAULT 0,
			PRIMARY KEY(bucket, source, model, project, git_branch)
		);
		CREATE INDEX IF NOT EXISTS idx_hourly_usage_bucket_source ON hourly_usage_aggregate(bucket, source);

		CREATE TABLE IF NOT EXISTS daily_usage_aggregate (
			bucket DATE NOT NULL,
			source TEXT NOT NULL,
			model TEXT NOT NULL,
			project TEXT DEFAULT '',
			git_branch TEXT DEFAULT '',
			calls INTEGER DEFAULT 0,
			prompts INTEGER DEFAULT 0,
			input_tokens INTEGER DEFAULT 0,
			output_tokens INTEGER DEFAULT 0,
			cache_read_input_tokens INTEGER DEFAULT 0,
			cache_creation_input_tokens INTEGER DEFAULT 0,
			reasoning_output_tokens INTEGER DEFAULT 0,
			cost_usd REAL DEFAULT 0,
			PRIMARY KEY(bucket, source, model, project, git_branch)
		);
		CREATE INDEX IF NOT EXISTS idx_daily_usage_bucket_source ON daily_usage_aggregate(bucket, source);

		CREATE TABLE IF NOT EXISTS reconciliation_imports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT DEFAULT '',
			format TEXT DEFAULT '',
			currency TEXT DEFAULT 'USD',
			local_cost_usd REAL DEFAULT 0,
			provider_cost_usd REAL DEFAULT 0,
			diff_usd REAL DEFAULT 0,
			rows_seen INTEGER DEFAULT 0,
			payload_sha256 TEXT DEFAULT '',
			window_start DATETIME,
			window_end DATETIME,
			status TEXT DEFAULT '',
			notes TEXT DEFAULT '',
			warnings TEXT DEFAULT '',
			imported_at DATETIME NOT NULL
		);

		CREATE TABLE IF NOT EXISTS offline_bundles (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			bundle_id TEXT NOT NULL,
			sha256 TEXT NOT NULL,
			format TEXT DEFAULT 'json',
			created_at DATETIME NOT NULL
		);

		DELETE FROM usage_records WHERE model = '<synthetic>';
		DELETE FROM usage_records WHERE model = 'delivery-mirror';
	`)
	if err != nil {
		return err
	}

	// Add scan_context column to file_state for existing DBs (idempotent).
	db.Exec("ALTER TABLE file_state ADD COLUMN scan_context TEXT DEFAULT ''")
	db.Exec("ALTER TABLE prompt_events ADD COLUMN model TEXT DEFAULT ''")
	db.Exec("ALTER TABLE prompt_events ADD COLUMN project TEXT DEFAULT ''")
	db.Exec("ALTER TABLE usage_records ADD COLUMN pricing_source TEXT DEFAULT ''")
	db.Exec("ALTER TABLE usage_records ADD COLUMN pricing_model TEXT DEFAULT ''")
	db.Exec("ALTER TABLE usage_records ADD COLUMN pricing_confidence TEXT DEFAULT ''")
	db.Exec("ALTER TABLE usage_records ADD COLUMN pricing_note TEXT DEFAULT ''")
	db.Exec("ALTER TABLE pricing ADD COLUMN pricing_source TEXT DEFAULT 'unknown'")
	db.Exec("ALTER TABLE pricing ADD COLUMN matched_model TEXT DEFAULT ''")
	db.Exec("ALTER TABLE pricing ADD COLUMN match_type TEXT DEFAULT 'direct'")
	db.Exec("ALTER TABLE pricing ADD COLUMN priority INTEGER DEFAULT 999")
	db.Exec("ALTER TABLE pricing ADD COLUMN confidence TEXT DEFAULT 'unknown'")
	db.Exec("ALTER TABLE reconciliation_imports ADD COLUMN currency TEXT DEFAULT 'USD'")
	db.Exec("ALTER TABLE reconciliation_imports ADD COLUMN payload_sha256 TEXT DEFAULT ''")
	db.Exec("ALTER TABLE reconciliation_imports ADD COLUMN window_start DATETIME")
	db.Exec("ALTER TABLE reconciliation_imports ADD COLUMN window_end DATETIME")
	db.Exec("ALTER TABLE reconciliation_imports ADD COLUMN warnings TEXT DEFAULT ''")
	db.Exec("ALTER TABLE agent_runs ADD COLUMN last_heartbeat_at DATETIME")
	db.Exec("ALTER TABLE agent_runs ADD COLUMN heartbeat_count INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE agent_runs ADD COLUMN phase TEXT DEFAULT ''")
	db.Exec("ALTER TABLE agent_runs ADD COLUMN progress REAL DEFAULT 0")
	db.Exec("ALTER TABLE agent_runs ADD COLUMN status_message TEXT DEFAULT ''")
	db.Exec("ALTER TABLE insight_events ADD COLUMN event_key TEXT")
	db.Exec("ALTER TABLE approval_requests ADD COLUMN required_approvals INTEGER DEFAULT 1")
	db.Exec("ALTER TABLE approval_requests ADD COLUMN approver_hint TEXT DEFAULT ''")
	db.Exec("ALTER TABLE approval_requests ADD COLUMN escalation_target TEXT DEFAULT ''")
	db.Exec("ALTER TABLE approval_requests ADD COLUMN escalation_after_seconds INTEGER DEFAULT 0")
	db.Exec("ALTER TABLE approval_requests ADD COLUMN due_at DATETIME")
	db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_insight_event_key ON insight_events(event_key)")

	// Versioned migrations: each runs once, tracked via meta table.
	migrations := []struct {
		id  string
		sql string
	}{
		{
			"001_fix_opencode_input_tokens", `
				DELETE FROM usage_records WHERE source = 'opencode';
				DELETE FROM file_state WHERE path LIKE '%opencode%';
				DELETE FROM sessions WHERE source = 'opencode';
			`,
		},
		{
			"002_input_tokens_non_overlapping", `
				DELETE FROM usage_records;
				DELETE FROM file_state;
				DELETE FROM sessions;
			`,
		},
		{
			"003_prompt_events_rescan", `
				DELETE FROM usage_records;
				DELETE FROM file_state;
				DELETE FROM sessions;
				DELETE FROM prompt_events;
			`,
		},
		{
			"004_file_state_scan_context", `
				DELETE FROM meta WHERE key LIKE 'file_scan_context:%';
				DELETE FROM file_state;
			`,
		},
		{
			"005_kiro_sqlite_only_rescan", `
				DELETE FROM usage_records WHERE source = 'kiro';
				DELETE FROM prompt_events WHERE source = 'kiro';
				DELETE FROM sessions WHERE source = 'kiro';
				DELETE FROM file_state WHERE path LIKE '%kiro%';
			`,
		},
		{
			"006_opencode_source_cost_rescan", `
				DELETE FROM usage_records WHERE source = 'opencode';
				DELETE FROM prompt_events WHERE source = 'opencode';
				DELETE FROM sessions WHERE source = 'opencode';
				DELETE FROM file_state WHERE path LIKE '%opencode%';
			`,
		},
		{
			"007_source_scoped_identity", `
				DROP INDEX IF EXISTS idx_usage_dedup;
				DROP INDEX IF EXISTS idx_prompt_dedup;
				DROP INDEX IF EXISTS idx_sessions_source_session;
				CREATE TABLE IF NOT EXISTS sessions_rebuild (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					source TEXT NOT NULL,
					session_id TEXT NOT NULL,
					project TEXT DEFAULT '',
					cwd TEXT DEFAULT '',
					version TEXT DEFAULT '',
					git_branch TEXT DEFAULT '',
					start_time DATETIME,
					prompts INTEGER DEFAULT 0
				);
				INSERT OR IGNORE INTO sessions_rebuild(id,source,session_id,project,cwd,version,git_branch,start_time,prompts)
					SELECT id,source,session_id,project,cwd,version,git_branch,start_time,prompts FROM sessions;
				DROP TABLE sessions;
				ALTER TABLE sessions_rebuild RENAME TO sessions;
				CREATE UNIQUE INDEX IF NOT EXISTS idx_sessions_source_session ON sessions(source, session_id);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_usage_dedup ON usage_records(source, session_id, model, timestamp, input_tokens, output_tokens);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_prompt_dedup ON prompt_events(source, session_id, timestamp);
				CREATE INDEX IF NOT EXISTS idx_usage_source_timestamp ON usage_records(source, timestamp);
				CREATE INDEX IF NOT EXISTS idx_usage_source_model_timestamp ON usage_records(source, model, timestamp);
				CREATE INDEX IF NOT EXISTS idx_usage_project_timestamp ON usage_records(project, timestamp);
				CREATE INDEX IF NOT EXISTS idx_prompt_source_timestamp ON prompt_events(source, timestamp);
				CREATE INDEX IF NOT EXISTS idx_sessions_source_start ON sessions(source, start_time);
			`,
		},
		{
			"008_large_dataset_indexes", `
				CREATE INDEX IF NOT EXISTS idx_usage_timestamp_session ON usage_records(timestamp, source, session_id);
				CREATE INDEX IF NOT EXISTS idx_usage_model_timestamp ON usage_records(model, timestamp);
				CREATE INDEX IF NOT EXISTS idx_usage_project_source_timestamp ON usage_records(project, source, timestamp);
				CREATE INDEX IF NOT EXISTS idx_prompt_timestamp_session ON prompt_events(timestamp, source, session_id);
				CREATE INDEX IF NOT EXISTS idx_sessions_project_start ON sessions(project, start_time);
			`,
		},
		{
			"009_agent_ledger_governance", `
				CREATE INDEX IF NOT EXISTS idx_usage_pricing_confidence ON usage_records(pricing_confidence);
				CREATE INDEX IF NOT EXISTS idx_usage_branch_timestamp ON usage_records(git_branch, timestamp);
				CREATE INDEX IF NOT EXISTS idx_usage_source_project_branch_time ON usage_records(source, project, git_branch, timestamp);
			`,
		},
		{
			"010_workload_ledger_foundation", `
				CREATE TABLE IF NOT EXISTS workloads (
					workload_id TEXT PRIMARY KEY,
					goal TEXT DEFAULT '',
					status TEXT DEFAULT 'active',
					source TEXT DEFAULT '',
					project TEXT DEFAULT '',
					repo TEXT DEFAULT '',
					git_branch TEXT DEFAULT '',
					owner TEXT DEFAULT '',
					team TEXT DEFAULT '',
					budget_usd REAL DEFAULT 0,
					outcome TEXT DEFAULT '',
					confidence REAL DEFAULT 1,
					created_at DATETIME NOT NULL,
					updated_at DATETIME NOT NULL,
					closed_at DATETIME
				);
				CREATE INDEX IF NOT EXISTS idx_workloads_updated ON workloads(updated_at);
				CREATE INDEX IF NOT EXISTS idx_workloads_source_updated ON workloads(source, updated_at);
				CREATE INDEX IF NOT EXISTS idx_workloads_project_updated ON workloads(project, updated_at);
				CREATE INDEX IF NOT EXISTS idx_workloads_status_updated ON workloads(status, updated_at);

				CREATE TABLE IF NOT EXISTS workload_sessions (
					workload_id TEXT NOT NULL,
					source TEXT NOT NULL,
					session_id TEXT NOT NULL,
					confidence REAL DEFAULT 1,
					created_at DATETIME NOT NULL,
					PRIMARY KEY(workload_id, source, session_id)
				);
				CREATE INDEX IF NOT EXISTS idx_workload_sessions_source_session ON workload_sessions(source, session_id);

				CREATE TABLE IF NOT EXISTS agent_runs (
					run_id TEXT PRIMARY KEY,
					workload_id TEXT NOT NULL,
					parent_run_id TEXT DEFAULT '',
					source TEXT DEFAULT '',
					agent_name TEXT DEFAULT '',
					agent_version TEXT DEFAULT '',
					command TEXT DEFAULT '',
					cwd TEXT DEFAULT '',
					status TEXT DEFAULT 'running',
					exit_code INTEGER DEFAULT 0,
					error TEXT DEFAULT '',
					started_at DATETIME NOT NULL,
					ended_at DATETIME,
					duration_ms INTEGER DEFAULT 0,
					last_heartbeat_at DATETIME,
					heartbeat_count INTEGER DEFAULT 0,
					phase TEXT DEFAULT '',
					progress REAL DEFAULT 0,
					status_message TEXT DEFAULT '',
					confidence REAL DEFAULT 1
				);
				CREATE INDEX IF NOT EXISTS idx_agent_runs_workload ON agent_runs(workload_id, started_at);
				CREATE INDEX IF NOT EXISTS idx_agent_runs_source_started ON agent_runs(source, started_at);

				CREATE TABLE IF NOT EXISTS model_calls (
					call_id TEXT PRIMARY KEY,
					workload_id TEXT NOT NULL,
					run_id TEXT DEFAULT '',
					source TEXT NOT NULL,
					session_id TEXT DEFAULT '',
					provider TEXT DEFAULT '',
					model TEXT NOT NULL,
					model_alias TEXT DEFAULT '',
					input_tokens INTEGER DEFAULT 0,
					output_tokens INTEGER DEFAULT 0,
					cache_read_input_tokens INTEGER DEFAULT 0,
					cache_creation_input_tokens INTEGER DEFAULT 0,
					reasoning_output_tokens INTEGER DEFAULT 0,
					cost_usd REAL DEFAULT 0,
					latency_ms INTEGER DEFAULT 0,
					finish_reason TEXT DEFAULT '',
					pricing_source TEXT DEFAULT '',
					pricing_confidence TEXT DEFAULT '',
					timestamp DATETIME NOT NULL,
					confidence REAL DEFAULT 1
				);
				CREATE INDEX IF NOT EXISTS idx_model_calls_workload_time ON model_calls(workload_id, timestamp);
				CREATE INDEX IF NOT EXISTS idx_model_calls_source_model_time ON model_calls(source, model, timestamp);

				CREATE TABLE IF NOT EXISTS tool_calls (
					tool_call_id TEXT PRIMARY KEY,
					workload_id TEXT NOT NULL,
					run_id TEXT DEFAULT '',
					source TEXT DEFAULT '',
					tool_name TEXT DEFAULT '',
					tool_type TEXT DEFAULT '',
					status TEXT DEFAULT '',
					error_class TEXT DEFAULT '',
					duration_ms INTEGER DEFAULT 0,
					params_hash TEXT DEFAULT '',
					timestamp DATETIME NOT NULL,
					confidence REAL DEFAULT 1
				);
				CREATE INDEX IF NOT EXISTS idx_tool_calls_workload_time ON tool_calls(workload_id, timestamp);

				CREATE TABLE IF NOT EXISTS context_refs (
					context_ref_id TEXT PRIMARY KEY,
					workload_id TEXT NOT NULL,
					run_id TEXT DEFAULT '',
					ref_type TEXT DEFAULT '',
					ref_hash TEXT DEFAULT '',
					label TEXT DEFAULT '',
					repo TEXT DEFAULT '',
					git_branch TEXT DEFAULT '',
					commit_sha TEXT DEFAULT '',
					privacy_label TEXT DEFAULT 'local',
					created_at DATETIME NOT NULL,
					confidence REAL DEFAULT 1
				);
				CREATE INDEX IF NOT EXISTS idx_context_refs_workload ON context_refs(workload_id);

				CREATE TABLE IF NOT EXISTS artifacts (
					artifact_id TEXT PRIMARY KEY,
					workload_id TEXT NOT NULL,
					run_id TEXT DEFAULT '',
					artifact_type TEXT DEFAULT '',
					label TEXT DEFAULT '',
					path_hash TEXT DEFAULT '',
					sha256 TEXT DEFAULT '',
					metadata TEXT DEFAULT '',
					created_at DATETIME NOT NULL,
					confidence REAL DEFAULT 1
				);
				CREATE INDEX IF NOT EXISTS idx_artifacts_workload ON artifacts(workload_id, created_at);

				CREATE TABLE IF NOT EXISTS evaluations (
					evaluation_id TEXT PRIMARY KEY,
					workload_id TEXT NOT NULL,
					evaluator TEXT DEFAULT 'local',
					status TEXT DEFAULT '',
					score REAL DEFAULT 0,
					signal TEXT DEFAULT '',
					notes TEXT DEFAULT '',
					created_at DATETIME NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_evaluations_workload ON evaluations(workload_id, created_at);

				CREATE TABLE IF NOT EXISTS policy_decisions (
					decision_id TEXT PRIMARY KEY,
					workload_id TEXT DEFAULT '',
					run_id TEXT DEFAULT '',
					rule_id TEXT DEFAULT '',
					action TEXT DEFAULT 'allow',
					reason TEXT DEFAULT '',
					actor_role TEXT DEFAULT '',
					created_at DATETIME NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_policy_decisions_created ON policy_decisions(created_at);
				CREATE INDEX IF NOT EXISTS idx_policy_decisions_workload ON policy_decisions(workload_id, created_at);

				CREATE TABLE IF NOT EXISTS canonical_events (
					id INTEGER PRIMARY KEY AUTOINCREMENT,
					event_id TEXT NOT NULL UNIQUE,
					source TEXT NOT NULL,
					event_type TEXT NOT NULL,
					source_event_id TEXT DEFAULT '',
					workload_id TEXT DEFAULT '',
					agent_run_id TEXT DEFAULT '',
					session_id TEXT DEFAULT '',
					model TEXT DEFAULT '',
					project TEXT DEFAULT '',
					git_branch TEXT DEFAULT '',
					timestamp DATETIME NOT NULL,
					payload_hash TEXT DEFAULT '',
					payload TEXT DEFAULT '',
					confidence REAL DEFAULT 1,
					created_at DATETIME NOT NULL
				);
				CREATE INDEX IF NOT EXISTS idx_canonical_events_source_time ON canonical_events(source, timestamp);
				CREATE INDEX IF NOT EXISTS idx_canonical_events_workload_time ON canonical_events(workload_id, timestamp);
				CREATE INDEX IF NOT EXISTS idx_canonical_events_type_time ON canonical_events(event_type, timestamp);
			`,
		},
		{
			"011_agent_run_heartbeat", `
				CREATE TABLE IF NOT EXISTS agent_run_events (
					event_id TEXT PRIMARY KEY,
					run_id TEXT NOT NULL,
					workload_id TEXT DEFAULT '',
					source TEXT DEFAULT '',
					event_type TEXT NOT NULL,
					status TEXT DEFAULT '',
					phase TEXT DEFAULT '',
					progress REAL DEFAULT 0,
					message TEXT DEFAULT '',
					metrics TEXT DEFAULT '{}',
					timestamp DATETIME NOT NULL,
					confidence REAL DEFAULT 1
				);
				CREATE INDEX IF NOT EXISTS idx_agent_run_events_run_time ON agent_run_events(run_id, timestamp);
				CREATE INDEX IF NOT EXISTS idx_agent_run_events_workload_time ON agent_run_events(workload_id, timestamp);
				CREATE INDEX IF NOT EXISTS idx_agent_run_events_type_time ON agent_run_events(event_type, timestamp);
			`,
		},
		{
			"012_workload_links", `
				CREATE TABLE IF NOT EXISTS workload_links (
					link_id TEXT PRIMARY KEY,
					source_workload_id TEXT NOT NULL,
					target_workload_id TEXT NOT NULL,
					relation TEXT NOT NULL,
					reason TEXT DEFAULT '',
					created_by TEXT DEFAULT '',
					created_at DATETIME NOT NULL,
					confidence REAL DEFAULT 1,
					UNIQUE(source_workload_id, target_workload_id, relation)
				);
				CREATE INDEX IF NOT EXISTS idx_workload_links_source ON workload_links(source_workload_id, created_at);
				CREATE INDEX IF NOT EXISTS idx_workload_links_target ON workload_links(target_workload_id, created_at);
				CREATE INDEX IF NOT EXISTS idx_workload_links_relation ON workload_links(relation, created_at);
			`,
		},
		{
			"013_control_idempotency", `
				CREATE TABLE IF NOT EXISTS control_idempotency (
					scope TEXT NOT NULL,
					idempotency_key TEXT NOT NULL,
					operation TEXT NOT NULL,
					request_hash TEXT NOT NULL,
					result_kind TEXT NOT NULL,
					result_id TEXT NOT NULL,
					created_at DATETIME NOT NULL,
					last_seen_at DATETIME NOT NULL,
					replay_count INTEGER NOT NULL DEFAULT 0,
					PRIMARY KEY(scope, idempotency_key)
				);
				CREATE INDEX IF NOT EXISTS idx_control_idempotency_operation_created ON control_idempotency(operation, created_at);
			`,
		},
		{
			"014_workload_leases", `
				CREATE TABLE IF NOT EXISTS workload_leases (
					lease_id TEXT PRIMARY KEY,
					workload_id TEXT NOT NULL,
					holder TEXT NOT NULL,
					purpose TEXT DEFAULT '',
					status TEXT NOT NULL DEFAULT 'active',
					token_hash TEXT NOT NULL,
					acquired_at DATETIME NOT NULL,
					expires_at DATETIME NOT NULL,
					last_renewed_at DATETIME,
					released_at DATETIME,
					confidence REAL DEFAULT 1
				);
				CREATE UNIQUE INDEX IF NOT EXISTS idx_workload_leases_active_unique ON workload_leases(workload_id) WHERE status='active';
				CREATE INDEX IF NOT EXISTS idx_workload_leases_status_expires ON workload_leases(status, expires_at);
				CREATE INDEX IF NOT EXISTS idx_workload_leases_workload ON workload_leases(workload_id, acquired_at);
			`,
		},
		{
			"015_workload_claim_indexes", `
				CREATE INDEX IF NOT EXISTS idx_workloads_claim_queue ON workloads(status, source, team, project, created_at, updated_at);
			`,
		},
	}
	for _, m := range migrations {
		var done string
		db.QueryRow("SELECT value FROM meta WHERE key=?", "migration_"+m.id).Scan(&done)
		if done == "done" {
			continue
		}
		if _, err := db.Exec(m.sql); err != nil {
			return fmt.Errorf("migration %s: %w", m.id, err)
		}
		db.Exec(`INSERT INTO meta(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
			"migration_"+m.id, "done")
	}
	db.Exec("ALTER TABLE canonical_events ADD COLUMN schema_version TEXT DEFAULT 'v1'")
	db.Exec("ALTER TABLE canonical_events ADD COLUMN source_version TEXT DEFAULT ''")
	db.Exec("ALTER TABLE canonical_events ADD COLUMN parser_version TEXT DEFAULT ''")
	db.Exec("ALTER TABLE canonical_events ADD COLUMN raw_ref TEXT DEFAULT ''")
	db.Exec("ALTER TABLE canonical_events ADD COLUMN match_type TEXT DEFAULT ''")
	db.Exec("CREATE INDEX IF NOT EXISTS idx_canonical_events_schema_type_time ON canonical_events(schema_version, event_type, timestamp)")
	return nil
}
