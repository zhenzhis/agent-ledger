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
		);
		CREATE INDEX IF NOT EXISTS idx_usage_timestamp ON usage_records(timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_session ON usage_records(source, session_id);
		CREATE INDEX IF NOT EXISTS idx_usage_source ON usage_records(source);
		CREATE INDEX IF NOT EXISTS idx_usage_source_timestamp ON usage_records(source, timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_source_model_timestamp ON usage_records(source, model, timestamp);
		CREATE INDEX IF NOT EXISTS idx_usage_project_timestamp ON usage_records(project, timestamp);
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
			updated_at DATETIME
		);

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
	return nil
}
