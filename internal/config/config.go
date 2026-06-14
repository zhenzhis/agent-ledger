package config

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the top-level application configuration.
type Config struct {
	Server       ServerConfig       `yaml:"server"`
	Collectors   CollectorConfigs   `yaml:"collectors"`
	Storage      StorageConfig      `yaml:"storage"`
	Pricing      PricingConfig      `yaml:"pricing"`
	Privacy      PrivacyConfig      `yaml:"privacy"`
	Projects     ProjectsConfig     `yaml:"projects"`
	Budgets      BudgetConfig       `yaml:"budgets"`
	Quota        QuotaConfig        `yaml:"quota"`
	Watchdog     WatchdogConfig     `yaml:"watchdog"`
	RBAC         RBACConfig         `yaml:"rbac"`
	Policies     PolicyConfig       `yaml:"policies"`
	Webhooks     WebhookConfig      `yaml:"webhooks"`
	Teams        TeamsConfig        `yaml:"teams"`
	Integrations IntegrationsConfig `yaml:"integrations"`
	Gateway      GatewayConfig      `yaml:"gateway"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Port        int    `yaml:"port"`
	BindAddress string `yaml:"bind_address"`
	AuthToken   string `yaml:"auth_token"`
	AdminToken  string `yaml:"admin_token"`
	ViewerToken string `yaml:"viewer_token"`
}

// CollectorConfigs groups configuration for all data source collectors.
type CollectorConfigs struct {
	Claude   CollectorConfig `yaml:"claude"`
	Codex    CollectorConfig `yaml:"codex"`
	OpenClaw CollectorConfig `yaml:"openclaw"`
	OpenCode CollectorConfig `yaml:"opencode"`
	Kiro     CollectorConfig `yaml:"kiro"`
	Pi       CollectorConfig `yaml:"pi"`
}

// CollectorConfig holds settings for a single data source collector.
type CollectorConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Paths        []string      `yaml:"paths"`
	ScanInterval time.Duration `yaml:"scan_interval"`
}

// StorageConfig holds SQLite database settings.
type StorageConfig struct {
	Path string `yaml:"path"`
}

// PricingConfig holds model pricing sync settings.
type PricingConfig struct {
	SyncInterval time.Duration `yaml:"sync_interval"`
	StaleAfter   time.Duration `yaml:"stale_after"`
	Mode         string        `yaml:"mode"`
	Overrides    []PriceRule   `yaml:"overrides"`
}

// PriceRule defines a local or contract price override for one model.
type PriceRule struct {
	Model                  string  `yaml:"model"`
	InputCostPerToken      float64 `yaml:"input_cost_per_token"`
	OutputCostPerToken     float64 `yaml:"output_cost_per_token"`
	CacheReadCostPerToken  float64 `yaml:"cache_read_input_token_cost"`
	CacheWriteCostPerToken float64 `yaml:"cache_creation_input_token_cost"`
	Source                 string  `yaml:"source"`
	EffectiveAt            string  `yaml:"effective_at"`
}

// PrivacyConfig controls optional output redaction for UI, reports, and exports.
type PrivacyConfig struct {
	RedactPaths      bool   `yaml:"redact_paths"`
	HashSessionIDs   bool   `yaml:"hash_session_ids"`
	HideProjectNames bool   `yaml:"hide_project_names"`
	ScreenshotMode   bool   `yaml:"screenshot_mode"`
	DefaultPreset    string `yaml:"default_preset"`
}

// ProjectsConfig controls local project naming and exclusion.
type ProjectsConfig struct {
	Aliases map[string]string `yaml:"aliases"`
	Exclude []string          `yaml:"exclude"`
}

// BudgetConfig groups local budget and alert rules.
type BudgetConfig struct {
	Enabled bool         `yaml:"enabled"`
	Rules   []BudgetRule `yaml:"rules"`
}

// BudgetRule defines a local token, cost, or prompt budget threshold.
type BudgetRule struct {
	Name      string  `yaml:"name"`
	Period    string  `yaml:"period"`
	Scope     string  `yaml:"scope"`
	Match     string  `yaml:"match"`
	Metric    string  `yaml:"metric"`
	Limit     float64 `yaml:"limit"`
	WarnRatio float64 `yaml:"warn_ratio"`
}

// QuotaConfig controls local plan and reset estimates.
type QuotaConfig struct {
	Enabled       bool          `yaml:"enabled"`
	Plan          string        `yaml:"plan"`
	MonthlyBudget float64       `yaml:"monthly_budget"`
	TokenBudget   int64         `yaml:"token_budget"`
	PromptBudget  int64         `yaml:"prompt_budget"`
	ResetDay      int           `yaml:"reset_day"`
	Window5H      bool          `yaml:"window_5h"`
	CustomWindows []QuotaWindow `yaml:"custom_windows"`
}

// QuotaWindow defines an additional local usage window.
type QuotaWindow struct {
	Name        string  `yaml:"name"`
	Duration    string  `yaml:"duration"`
	CostLimit   float64 `yaml:"cost_limit"`
	TokenLimit  int64   `yaml:"token_limit"`
	PromptLimit int64   `yaml:"prompt_limit"`
}

// WatchdogConfig controls local anomaly and runaway detection.
type WatchdogConfig struct {
	Enabled              bool    `yaml:"enabled"`
	TokenSpikeMultiplier float64 `yaml:"token_spike_multiplier"`
	MinCalls             int     `yaml:"min_calls"`
	NightStartHour       int     `yaml:"night_start_hour"`
	NightEndHour         int     `yaml:"night_end_hour"`
}

// RBACConfig enables coarse local roles for API operations.
type RBACConfig struct {
	Enabled  bool `yaml:"enabled"`
	ReadOnly bool `yaml:"read_only"`
}

// PolicyConfig groups local policy rules.
type PolicyConfig struct {
	Enabled              bool         `yaml:"enabled"`
	RequirePrivacyExport bool         `yaml:"require_privacy_export"`
	Rules                []PolicyRule `yaml:"rules"`
}

// PolicyRule describes a local advisory policy.
type PolicyRule struct {
	Name              string        `yaml:"name"`
	Scope             string        `yaml:"scope"`
	Match             string        `yaml:"match"`
	Action            string        `yaml:"action"`
	Message           string        `yaml:"message"`
	RequiredApprovals int           `yaml:"required_approvals"`
	Approvers         []string      `yaml:"approvers"`
	EscalateAfter     time.Duration `yaml:"escalate_after"`
	EscalateTo        []string      `yaml:"escalate_to"`
}

// WebhookConfig is disabled by default and only sends redacted summaries.
type WebhookConfig struct {
	Enabled   bool          `yaml:"enabled"`
	URL       string        `yaml:"url"`
	Timeout   time.Duration `yaml:"timeout"`
	MaxEvents int           `yaml:"max_events"`
}

// TeamsConfig maps projects, paths, or authors to showback groups.
type TeamsConfig struct {
	MachineName string            `yaml:"machine_name"`
	GitAuthor   string            `yaml:"git_author"`
	Groups      map[string]string `yaml:"groups"`
}

// IntegrationsConfig controls optional protocol adapters and receivers.
type IntegrationsConfig struct {
	OTLPReceiver OTLPReceiverConfig `yaml:"otlp_receiver"`
}

// GatewayConfig controls the optional local provider gateway.
type GatewayConfig struct {
	Enabled                  bool              `yaml:"enabled"`
	UpstreamBaseURL          string            `yaml:"upstream_base_url"`
	APIKeyEnv                string            `yaml:"api_key_env"`
	AnthropicUpstreamBaseURL string            `yaml:"anthropic_upstream_base_url"`
	AnthropicAPIKeyEnv       string            `yaml:"anthropic_api_key_env"`
	IncludeStreamUsage       bool              `yaml:"include_stream_usage"`
	FallbackEnabled          bool              `yaml:"fallback_enabled"`
	FallbackOnBudgetSeverity string            `yaml:"fallback_on_budget_severity"`
	FallbackModels           map[string]string `yaml:"fallback_models"`
	MaxBodyBytes             int64             `yaml:"max_body_bytes"`
	MaxResponseBytes         int64             `yaml:"max_response_bytes"`
	Timeout                  time.Duration     `yaml:"timeout"`
}

// OTLPReceiverConfig controls the local OTLP HTTP JSON/protobuf traces receiver.
type OTLPReceiverConfig struct {
	Enabled         bool   `yaml:"enabled"`
	GRPCEnabled     bool   `yaml:"grpc_enabled"`
	GRPCBindAddress string `yaml:"grpc_bind_address"`
	GRPCPort        int    `yaml:"grpc_port"`
	MaxBodyBytes    int64  `yaml:"max_body_bytes"`
	MaxSpans        int    `yaml:"max_spans"`
}

func expandPath(p string) string {
	if strings.HasPrefix(p, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, p[2:])
	}
	return p
}

// DefaultConfig returns a Config with sensible defaults for all fields.
func DefaultConfig() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Server: ServerConfig{Port: 9800, BindAddress: "127.0.0.1"},
		Collectors: CollectorConfigs{
			Claude: CollectorConfig{
				Enabled:      true,
				Paths:        []string{filepath.Join(home, ".claude", "projects")},
				ScanInterval: 60 * time.Second,
			},
			Codex: CollectorConfig{
				Enabled:      true,
				Paths:        []string{filepath.Join(home, ".codex", "sessions")},
				ScanInterval: 60 * time.Second,
			},
			OpenClaw: CollectorConfig{
				Enabled:      true,
				Paths:        []string{filepath.Join(home, ".openclaw", "agents")},
				ScanInterval: 60 * time.Second,
			},
			OpenCode: CollectorConfig{
				Enabled:      true,
				Paths:        []string{filepath.Join(home, ".local", "share", "opencode", "opencode.db")},
				ScanInterval: 60 * time.Second,
			},
			Kiro: CollectorConfig{
				Enabled: true,
				Paths: []string{
					filepath.Join(home, ".local", "share", "kiro-cli", "data.sqlite3"),
					filepath.Join(home, ".kiro", "sessions", "cli"),
				},
				ScanInterval: 60 * time.Second,
			},
			Pi: CollectorConfig{
				Enabled:      true,
				Paths:        []string{filepath.Join(home, ".pi", "agent", "sessions")},
				ScanInterval: 60 * time.Second,
			},
		},
		Storage: StorageConfig{Path: "./agent-ledger.db"},
		Pricing: PricingConfig{SyncInterval: time.Hour, StaleAfter: 24 * time.Hour, Mode: "official-plus-litellm"},
		Privacy: PrivacyConfig{DefaultPreset: "normal"},
		Projects: ProjectsConfig{
			Aliases: map[string]string{},
			Exclude: []string{},
		},
		Budgets:  BudgetConfig{Enabled: false},
		Quota:    QuotaConfig{Enabled: false, Plan: "custom", ResetDay: 1, Window5H: true},
		Watchdog: WatchdogConfig{Enabled: true, TokenSpikeMultiplier: 4, MinCalls: 8, NightStartHour: 22, NightEndHour: 6},
		RBAC:     RBACConfig{Enabled: false, ReadOnly: false},
		Policies: PolicyConfig{Enabled: false},
		Webhooks: WebhookConfig{Enabled: false, Timeout: 10 * time.Second, MaxEvents: 20},
		Teams:    TeamsConfig{Groups: map[string]string{}},
		Integrations: IntegrationsConfig{
			OTLPReceiver: OTLPReceiverConfig{Enabled: false, GRPCEnabled: false, GRPCBindAddress: "127.0.0.1", GRPCPort: 4317, MaxBodyBytes: 4 << 20, MaxSpans: 1000},
		},
		Gateway: GatewayConfig{
			Enabled:                  false,
			UpstreamBaseURL:          "https://api.openai.com",
			APIKeyEnv:                "OPENAI_API_KEY",
			AnthropicUpstreamBaseURL: "https://api.anthropic.com",
			AnthropicAPIKeyEnv:       "ANTHROPIC_API_KEY",
			IncludeStreamUsage:       true,
			FallbackEnabled:          false,
			FallbackOnBudgetSeverity: "critical",
			FallbackModels:           map[string]string{},
			MaxBodyBytes:             4 << 20,
			MaxResponseBytes:         32 << 20,
			Timeout:                  120 * time.Second,
		},
	}
}

// ResolveConfigPath returns the config file path to use, checking in order:
// 1. Explicit path from --config flag (if non-empty)
// 2. /etc/agent-ledger/config.yaml (Docker / system-wide)
// 3. ./config.yaml (local default)
func ResolveConfigPath(flagPath string) string {
	if flagPath != "" {
		return flagPath
	}
	if _, err := os.Stat("/etc/agent-ledger/config.yaml"); err == nil {
		return "/etc/agent-ledger/config.yaml"
	}
	return "config.yaml"
}

// Load reads configuration from the given YAML file path, falling back to
// defaults for any missing fields. If the file does not exist, defaults are used.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// Expand ~ in paths
	for i, p := range cfg.Collectors.Claude.Paths {
		cfg.Collectors.Claude.Paths[i] = expandPath(p)
	}
	for i, p := range cfg.Collectors.Codex.Paths {
		cfg.Collectors.Codex.Paths[i] = expandPath(p)
	}
	for i, p := range cfg.Collectors.OpenClaw.Paths {
		cfg.Collectors.OpenClaw.Paths[i] = expandPath(p)
	}
	for i, p := range cfg.Collectors.OpenCode.Paths {
		cfg.Collectors.OpenCode.Paths[i] = expandPath(p)
	}
	for i, p := range cfg.Collectors.Kiro.Paths {
		cfg.Collectors.Kiro.Paths[i] = expandPath(p)
	}
	for i, p := range cfg.Collectors.Pi.Paths {
		cfg.Collectors.Pi.Paths[i] = expandPath(p)
	}
	cfg.Storage.Path = expandPath(cfg.Storage.Path)
	if cfg.Projects.Aliases == nil {
		cfg.Projects.Aliases = map[string]string{}
	}
	if cfg.Teams.Groups == nil {
		cfg.Teams.Groups = map[string]string{}
	}
	if cfg.Integrations.OTLPReceiver.MaxBodyBytes <= 0 {
		cfg.Integrations.OTLPReceiver.MaxBodyBytes = 4 << 20
	}
	if cfg.Integrations.OTLPReceiver.MaxSpans <= 0 {
		cfg.Integrations.OTLPReceiver.MaxSpans = 1000
	}
	if cfg.Integrations.OTLPReceiver.GRPCBindAddress == "" {
		cfg.Integrations.OTLPReceiver.GRPCBindAddress = "127.0.0.1"
	}
	if cfg.Integrations.OTLPReceiver.GRPCPort <= 0 {
		cfg.Integrations.OTLPReceiver.GRPCPort = 4317
	}
	if cfg.Gateway.UpstreamBaseURL == "" {
		cfg.Gateway.UpstreamBaseURL = "https://api.openai.com"
	}
	if cfg.Gateway.APIKeyEnv == "" {
		cfg.Gateway.APIKeyEnv = "OPENAI_API_KEY"
	}
	if cfg.Gateway.AnthropicUpstreamBaseURL == "" {
		cfg.Gateway.AnthropicUpstreamBaseURL = "https://api.anthropic.com"
	}
	if cfg.Gateway.AnthropicAPIKeyEnv == "" {
		cfg.Gateway.AnthropicAPIKeyEnv = "ANTHROPIC_API_KEY"
	}
	if cfg.Gateway.MaxBodyBytes <= 0 {
		cfg.Gateway.MaxBodyBytes = 4 << 20
	}
	if cfg.Gateway.MaxResponseBytes <= 0 {
		cfg.Gateway.MaxResponseBytes = 32 << 20
	}
	if cfg.Gateway.Timeout <= 0 {
		cfg.Gateway.Timeout = 120 * time.Second
	}
	if cfg.Pricing.StaleAfter <= 0 {
		cfg.Pricing.StaleAfter = 24 * time.Hour
	}
	if cfg.Watchdog.TokenSpikeMultiplier <= 0 {
		cfg.Watchdog.TokenSpikeMultiplier = 4
	}
	if cfg.Watchdog.MinCalls <= 0 {
		cfg.Watchdog.MinCalls = 8
	}
	if cfg.Webhooks.Timeout <= 0 {
		cfg.Webhooks.Timeout = 10 * time.Second
	}
	if cfg.Webhooks.MaxEvents <= 0 || cfg.Webhooks.MaxEvents > 100 {
		cfg.Webhooks.MaxEvents = 20
	}
	return cfg, nil
}
