package config

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

// ConfigStatusReport is a privacy-safe deployment configuration report. It is
// safe to expose through local control-plane APIs because it never includes raw
// filesystem paths, auth tokens, API keys, webhook URLs, machine names, authors,
// or prompt/session content.
type ConfigStatusReport struct {
	Product             string                  `json:"product"`
	Slug                string                  `json:"slug"`
	Contract            string                  `json:"contract"`
	Version             string                  `json:"version"`
	LocalFirst          bool                    `json:"local_first"`
	PrivacyDefault      string                  `json:"privacy_default"`
	PromptContentStored bool                    `json:"prompt_content_stored"`
	UsageDataUploaded   bool                    `json:"usage_data_uploaded"`
	PathValuesExposed   bool                    `json:"path_values_exposed"`
	SecretValuesExposed bool                    `json:"secret_values_exposed"`
	Bind                ConfigBindStatus        `json:"bind"`
	Auth                ConfigAuthStatus        `json:"auth"`
	Storage             ConfigStorageStatus     `json:"storage"`
	Collectors          []ConfigCollectorStatus `json:"collectors"`
	Pricing             ConfigPricingStatus     `json:"pricing"`
	Privacy             ConfigPrivacyStatus     `json:"privacy"`
	Features            ConfigFeatureStatus     `json:"features"`
	Outbound            ConfigOutboundStatus    `json:"outbound"`
	Teams               ConfigTeamStatus        `json:"teams"`
	Summary             ConfigStatusSummary     `json:"summary"`
	Issues              []ConfigStatusIssue     `json:"issues"`
	PrivacyNote         string                  `json:"privacy_note"`
}

type ConfigBindStatus struct {
	Address       string `json:"address"`
	Port          int    `json:"port"`
	LoopbackOnly  bool   `json:"loopback_only"`
	PubliclyBound bool   `json:"publicly_bound"`
}

type ConfigAuthStatus struct {
	AuthTokenConfigured   bool `json:"auth_token_configured"`
	AdminTokenConfigured  bool `json:"admin_token_configured"`
	ViewerTokenConfigured bool `json:"viewer_token_configured"`
	AnyTokenConfigured    bool `json:"any_token_configured"`
	RBACEnabled           bool `json:"rbac_enabled"`
	ReadOnly              bool `json:"read_only"`
}

type ConfigStorageStatus struct {
	PathConfigured bool `json:"path_configured"`
}

type ConfigCollectorStatus struct {
	Source       string `json:"source"`
	Enabled      bool   `json:"enabled"`
	PathCount    int    `json:"path_count"`
	ScanInterval string `json:"scan_interval"`
}

type ConfigPricingStatus struct {
	Mode          string `json:"mode"`
	SyncInterval  string `json:"sync_interval"`
	StaleAfter    string `json:"stale_after"`
	OverrideCount int    `json:"override_count"`
}

type ConfigPrivacyStatus struct {
	RedactPaths      bool   `json:"redact_paths"`
	HashSessionIDs   bool   `json:"hash_session_ids"`
	HideProjectNames bool   `json:"hide_project_names"`
	ScreenshotMode   bool   `json:"screenshot_mode"`
	DefaultPreset    string `json:"default_preset"`
}

type ConfigFeatureStatus struct {
	BudgetsEnabled           bool `json:"budgets_enabled"`
	BudgetRuleCount          int  `json:"budget_rule_count"`
	QuotaEnabled             bool `json:"quota_enabled"`
	WatchdogEnabled          bool `json:"watchdog_enabled"`
	PoliciesEnabled          bool `json:"policies_enabled"`
	PolicyRuleCount          int  `json:"policy_rule_count"`
	OTLPReceiverEnabled      bool `json:"otlp_receiver_enabled"`
	OTLPReceiverGRPCEnabled  bool `json:"otlp_receiver_grpc_enabled"`
	GatewayEnabled           bool `json:"gateway_enabled"`
	GatewayFallbackEnabled   bool `json:"gateway_fallback_enabled"`
	GatewayFallbackRuleCount int  `json:"gateway_fallback_rule_count"`
}

type ConfigOutboundStatus struct {
	WebhooksEnabled              bool     `json:"webhooks_enabled"`
	WebhookURLConfigured         bool     `json:"webhook_url_configured"`
	GatewayEnabled               bool     `json:"gateway_enabled"`
	GatewayFallbackEnabled       bool     `json:"gateway_fallback_enabled"`
	GatewayFallbackSeverity      string   `json:"gateway_fallback_severity"`
	GatewayFallbackRuleCount     int      `json:"gateway_fallback_rule_count"`
	GatewayUpstreamConfigured    bool     `json:"gateway_upstream_configured"`
	GatewayAPIKeyEnvConfigured   bool     `json:"gateway_api_key_env_configured"`
	AnthropicUpstreamConfigured  bool     `json:"anthropic_upstream_configured"`
	AnthropicAPIKeyEnvConfigured bool     `json:"anthropic_api_key_env_configured"`
	OutboundSurfaces             []string `json:"outbound_surfaces"`
}

type ConfigTeamStatus struct {
	MachineNameConfigured bool `json:"machine_name_configured"`
	GitAuthorConfigured   bool `json:"git_author_configured"`
	GroupMappingCount     int  `json:"group_mapping_count"`
}

type ConfigStatusSummary struct {
	EnabledCollectors  int `json:"enabled_collectors"`
	DisabledCollectors int `json:"disabled_collectors"`
	CollectorPathCount int `json:"collector_path_count"`
	CriticalIssues     int `json:"critical_issues"`
	WarningIssues      int `json:"warning_issues"`
	InfoIssues         int `json:"info_issues"`
}

type ConfigStatusIssue struct {
	Name     string `json:"name"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"`
}

// StatusReport returns a deterministic, privacy-safe configuration report.
func StatusReport(cfg *Config) *ConfigStatusReport {
	if cfg == nil {
		cfg = DefaultConfig()
		report := StatusReport(cfg)
		report.addIssue("config.nil", "warning", "configuration was nil; defaults were used for status reporting", "load an explicit config file before serving production traffic")
		report.recountIssues()
		return report
	}

	bind := bindStatus(cfg.Server)
	auth := ConfigAuthStatus{
		AuthTokenConfigured:   strings.TrimSpace(cfg.Server.AuthToken) != "",
		AdminTokenConfigured:  strings.TrimSpace(cfg.Server.AdminToken) != "",
		ViewerTokenConfigured: strings.TrimSpace(cfg.Server.ViewerToken) != "",
		RBACEnabled:           cfg.RBAC.Enabled,
		ReadOnly:              cfg.RBAC.ReadOnly,
	}
	auth.AnyTokenConfigured = auth.AuthTokenConfigured || auth.AdminTokenConfigured || auth.ViewerTokenConfigured
	collectors := collectorStatuses(cfg.Collectors)
	outbound := outboundStatus(cfg)

	report := &ConfigStatusReport{
		Product:             "Agent Ledger",
		Slug:                "agent-ledger",
		Contract:            "agent-ledger.config-status",
		Version:             "v1",
		LocalFirst:          bind.LoopbackOnly && len(outbound.OutboundSurfaces) == 0,
		PrivacyDefault:      firstNonEmpty(cfg.Privacy.DefaultPreset, "normal"),
		PromptContentStored: false,
		UsageDataUploaded:   false,
		PathValuesExposed:   false,
		SecretValuesExposed: false,
		Bind:                bind,
		Auth:                auth,
		Storage: ConfigStorageStatus{
			PathConfigured: strings.TrimSpace(cfg.Storage.Path) != "",
		},
		Collectors: collectors,
		Pricing: ConfigPricingStatus{
			Mode:          firstNonEmpty(cfg.Pricing.Mode, "official-plus-litellm"),
			SyncInterval:  durationString(cfg.Pricing.SyncInterval),
			StaleAfter:    durationString(cfg.Pricing.StaleAfter),
			OverrideCount: len(cfg.Pricing.Overrides),
		},
		Privacy: ConfigPrivacyStatus{
			RedactPaths:      cfg.Privacy.RedactPaths,
			HashSessionIDs:   cfg.Privacy.HashSessionIDs,
			HideProjectNames: cfg.Privacy.HideProjectNames,
			ScreenshotMode:   cfg.Privacy.ScreenshotMode,
			DefaultPreset:    firstNonEmpty(cfg.Privacy.DefaultPreset, "normal"),
		},
		Features: ConfigFeatureStatus{
			BudgetsEnabled:           cfg.Budgets.Enabled,
			BudgetRuleCount:          len(cfg.Budgets.Rules),
			QuotaEnabled:             cfg.Quota.Enabled,
			WatchdogEnabled:          cfg.Watchdog.Enabled,
			PoliciesEnabled:          cfg.Policies.Enabled,
			PolicyRuleCount:          len(cfg.Policies.Rules),
			OTLPReceiverEnabled:      cfg.Integrations.OTLPReceiver.Enabled,
			OTLPReceiverGRPCEnabled:  cfg.Integrations.OTLPReceiver.GRPCEnabled,
			GatewayEnabled:           cfg.Gateway.Enabled,
			GatewayFallbackEnabled:   cfg.Gateway.FallbackEnabled,
			GatewayFallbackRuleCount: len(cfg.Gateway.FallbackModels),
		},
		Outbound: outbound,
		Teams: ConfigTeamStatus{
			MachineNameConfigured: strings.TrimSpace(cfg.Teams.MachineName) != "",
			GitAuthorConfigured:   strings.TrimSpace(cfg.Teams.GitAuthor) != "",
			GroupMappingCount:     len(cfg.Teams.Groups),
		},
		PrivacyNote: "This report intentionally exposes counts and booleans only; raw paths, auth tokens, API keys, webhook URLs, machine names, git authors, prompts, responses, and session ids are excluded.",
	}

	report.Summary = collectorSummary(collectors)
	report.addValidationIssues(cfg)
	report.recountIssues()
	return report
}

// FormatStatusMarkdown renders the config status report for CLI and docs usage.
func FormatStatusMarkdown(report *ConfigStatusReport) string {
	if report == nil {
		report = StatusReport(nil)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent Ledger Config Status\n\n")
	fmt.Fprintf(&b, "- Contract: `%s`\n", report.Contract)
	fmt.Fprintf(&b, "- Local first: `%t`\n", report.LocalFirst)
	fmt.Fprintf(&b, "- Bind: `%s:%d` loopback_only=`%t`\n", report.Bind.Address, report.Bind.Port, report.Bind.LoopbackOnly)
	fmt.Fprintf(&b, "- Auth: rbac=`%t` any_token=`%t` read_only=`%t`\n", report.Auth.RBACEnabled, report.Auth.AnyTokenConfigured, report.Auth.ReadOnly)
	fmt.Fprintf(&b, "- Collectors: enabled=`%d` disabled=`%d` paths=`%d`\n", report.Summary.EnabledCollectors, report.Summary.DisabledCollectors, report.Summary.CollectorPathCount)
	fmt.Fprintf(&b, "- Pricing: mode=`%s` stale_after=`%s` overrides=`%d`\n", report.Pricing.Mode, report.Pricing.StaleAfter, report.Pricing.OverrideCount)
	fmt.Fprintf(&b, "- Outbound surfaces: `%s`\n", strings.Join(report.Outbound.OutboundSurfaces, ","))
	fmt.Fprintf(&b, "- Privacy: paths_exposed=`%t` secrets_exposed=`%t`\n\n", report.PathValuesExposed, report.SecretValuesExposed)
	if len(report.Issues) == 0 {
		b.WriteString("## Issues\n\nNo issues detected.\n")
		return b.String()
	}
	b.WriteString("## Issues\n\n")
	for _, issue := range report.Issues {
		fmt.Fprintf(&b, "- `%s` %s: %s", issue.Severity, issue.Name, issue.Message)
		if issue.Action != "" {
			fmt.Fprintf(&b, " Action: %s", issue.Action)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

func (r *ConfigStatusReport) addValidationIssues(cfg *Config) {
	if r.Bind.PubliclyBound && !r.Auth.AnyTokenConfigured && !r.Auth.RBACEnabled {
		r.addIssue("server.public_bind_without_auth", "critical", "server is bound outside loopback without auth or RBAC configured", "bind to 127.0.0.1 or configure auth/RBAC before exposing the service")
	}
	if !r.Storage.PathConfigured {
		r.addIssue("storage.path_missing", "critical", "storage path is not configured", "set storage.path to a local SQLite file")
	}
	for _, collector := range r.Collectors {
		if collector.Enabled && collector.PathCount == 0 {
			r.addIssue("collector."+collector.Source+".paths_missing", "warning", "collector is enabled but has no configured paths", "configure collector paths or disable this source")
		}
		if collector.Enabled && collector.ScanInterval == "0s" {
			r.addIssue("collector."+collector.Source+".scan_interval_zero", "warning", "collector scan interval is zero", "set a positive scan interval for background collection")
		}
	}
	if strings.TrimSpace(r.Pricing.Mode) == "" {
		r.addIssue("pricing.mode_missing", "warning", "pricing mode is empty", "use official-plus-litellm or a documented local pricing mode")
	}
	if cfg.Pricing.StaleAfter <= 0 {
		r.addIssue("pricing.stale_after_invalid", "warning", "pricing stale_after is not positive", "set pricing.stale_after to a positive duration")
	}
	if cfg.Pricing.SyncInterval <= 0 {
		r.addIssue("pricing.sync_interval_invalid", "warning", "pricing sync_interval is not positive", "set pricing.sync_interval to a positive duration")
	}
	if cfg.Webhooks.Enabled && strings.TrimSpace(cfg.Webhooks.URL) == "" {
		r.addIssue("webhooks.url_missing", "warning", "webhooks are enabled but no webhook URL is configured", "set webhooks.url or disable webhooks")
	}
	if cfg.Webhooks.Enabled {
		r.addIssue("webhooks.enabled", "info", "webhooks are enabled and may send redacted summaries outside this process", "confirm outbound notification policy before enabling in shared environments")
	}
	if cfg.Gateway.Enabled {
		r.addIssue("gateway.enabled", "info", "provider gateway is enabled and can proxy model traffic to upstream providers", "confirm upstream provider and data-handling policy before routing prompts through the gateway")
		if strings.TrimSpace(cfg.Gateway.UpstreamBaseURL) == "" && strings.TrimSpace(cfg.Gateway.AnthropicUpstreamBaseURL) == "" {
			r.addIssue("gateway.upstream_missing", "warning", "gateway is enabled without upstream base URLs", "configure at least one upstream provider base URL")
		}
		if strings.TrimSpace(cfg.Gateway.APIKeyEnv) == "" && strings.TrimSpace(cfg.Gateway.AnthropicAPIKeyEnv) == "" {
			r.addIssue("gateway.api_key_env_missing", "warning", "gateway is enabled without API key environment variable names", "configure provider API key environment variable names")
		}
		if cfg.Gateway.FallbackEnabled {
			r.addIssue("gateway.fallback_enabled", "info", "gateway budget fallback can rewrite configured model requests before proxying upstream", "confirm fallback model policy and budget thresholds before enabling for shared teams")
			if len(cfg.Gateway.FallbackModels) == 0 {
				r.addIssue("gateway.fallback_models_missing", "warning", "gateway fallback is enabled but no fallback model mappings are configured", "set gateway.fallback_models or disable gateway.fallback_enabled")
			}
		}
	}
	if cfg.Policies.Enabled && !cfg.RBAC.Enabled {
		r.addIssue("policies.enabled_without_rbac", "info", "policies are enabled while RBAC is disabled; policy checks remain advisory", "enable RBAC if role-aware policy enforcement is required")
	}
	if cfg.Integrations.OTLPReceiver.GRPCEnabled {
		if !cfg.Integrations.OTLPReceiver.Enabled {
			r.addIssue("otlp.grpc_without_receiver", "warning", "OTLP gRPC receiver is enabled while OTLP receiver is disabled", "set integrations.otlp_receiver.enabled=true or disable grpc_enabled")
		}
		if !isLoopbackBind(cfg.Integrations.OTLPReceiver.GRPCBindAddress) {
			r.addIssue("otlp.grpc_non_loopback", "critical", "OTLP gRPC receiver bind address is not loopback-only", "bind OTLP gRPC to 127.0.0.1 or ::1 unless a separate authenticated transport is added")
		}
	}
	if cfg.Policies.RequirePrivacyExport && !cfg.Privacy.RedactPaths && !cfg.Privacy.HideProjectNames && !cfg.Privacy.HashSessionIDs {
		r.addIssue("policies.privacy_export_without_redaction", "warning", "privacy export policy is enabled but no redaction toggles are active", "enable a privacy preset or explicit redaction options")
	}
	if cfg.Integrations.OTLPReceiver.Enabled && cfg.Integrations.OTLPReceiver.MaxBodyBytes <= 0 {
		r.addIssue("integrations.otlp.max_body_invalid", "warning", "OTLP receiver max body size is not positive", "set integrations.otlp_receiver.max_body_bytes to a positive limit")
	}
	if cfg.Integrations.OTLPReceiver.Enabled && cfg.Integrations.OTLPReceiver.MaxSpans <= 0 {
		r.addIssue("integrations.otlp.max_spans_invalid", "warning", "OTLP receiver max spans is not positive", "set integrations.otlp_receiver.max_spans to a positive limit")
	}
}

func (r *ConfigStatusReport) addIssue(name, severity, message, action string) {
	r.Issues = append(r.Issues, ConfigStatusIssue{
		Name:     name,
		Severity: severity,
		Message:  message,
		Action:   action,
	})
	sort.SliceStable(r.Issues, func(i, j int) bool {
		left := severityRank(r.Issues[i].Severity)
		right := severityRank(r.Issues[j].Severity)
		if left != right {
			return left < right
		}
		return r.Issues[i].Name < r.Issues[j].Name
	})
}

func (r *ConfigStatusReport) recountIssues() {
	r.Summary.CriticalIssues = 0
	r.Summary.WarningIssues = 0
	r.Summary.InfoIssues = 0
	for _, issue := range r.Issues {
		switch strings.ToLower(issue.Severity) {
		case "critical":
			r.Summary.CriticalIssues++
		case "warning":
			r.Summary.WarningIssues++
		default:
			r.Summary.InfoIssues++
		}
	}
}

func bindStatus(server ServerConfig) ConfigBindStatus {
	rawAddress := strings.TrimSpace(server.BindAddress)
	if rawAddress == "" {
		rawAddress = "127.0.0.1"
	}
	loopback := isLoopbackBind(rawAddress)
	return ConfigBindStatus{
		Address:       classifiedBindAddress(rawAddress),
		Port:          server.Port,
		LoopbackOnly:  loopback,
		PubliclyBound: !loopback,
	}
}

func classifiedBindAddress(address string) string {
	address = strings.Trim(strings.TrimSpace(address), "[]")
	switch {
	case address == "", strings.EqualFold(address, "localhost"):
		return "localhost"
	case address == "0.0.0.0", address == "::", address == "*":
		return "public-wildcard"
	}
	ip := net.ParseIP(address)
	if ip == nil {
		return "non-loopback-hostname"
	}
	if ip.IsLoopback() {
		return ip.String()
	}
	return "non-loopback-ip"
}

func isLoopbackBind(address string) bool {
	address = strings.Trim(strings.TrimSpace(address), "[]")
	if address == "" || strings.EqualFold(address, "localhost") {
		return true
	}
	if address == "0.0.0.0" || address == "::" || address == "*" {
		return false
	}
	ip := net.ParseIP(address)
	if ip == nil {
		return false
	}
	return ip.IsLoopback()
}

func collectorStatuses(cfg CollectorConfigs) []ConfigCollectorStatus {
	out := []ConfigCollectorStatus{
		collectorStatus("claude", cfg.Claude),
		collectorStatus("codex", cfg.Codex),
		collectorStatus("openclaw", cfg.OpenClaw),
		collectorStatus("opencode", cfg.OpenCode),
		collectorStatus("kiro", cfg.Kiro),
		collectorStatus("pi", cfg.Pi),
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Source < out[j].Source })
	return out
}

func collectorStatus(source string, cfg CollectorConfig) ConfigCollectorStatus {
	return ConfigCollectorStatus{
		Source:       source,
		Enabled:      cfg.Enabled,
		PathCount:    len(cfg.Paths),
		ScanInterval: durationString(cfg.ScanInterval),
	}
}

func collectorSummary(collectors []ConfigCollectorStatus) ConfigStatusSummary {
	summary := ConfigStatusSummary{}
	for _, collector := range collectors {
		if collector.Enabled {
			summary.EnabledCollectors++
		} else {
			summary.DisabledCollectors++
		}
		summary.CollectorPathCount += collector.PathCount
	}
	return summary
}

func outboundStatus(cfg *Config) ConfigOutboundStatus {
	status := ConfigOutboundStatus{
		WebhooksEnabled:              cfg.Webhooks.Enabled,
		WebhookURLConfigured:         strings.TrimSpace(cfg.Webhooks.URL) != "",
		GatewayEnabled:               cfg.Gateway.Enabled,
		GatewayFallbackEnabled:       cfg.Gateway.FallbackEnabled,
		GatewayFallbackSeverity:      firstNonEmpty(cfg.Gateway.FallbackOnBudgetSeverity, "critical"),
		GatewayFallbackRuleCount:     len(cfg.Gateway.FallbackModels),
		GatewayUpstreamConfigured:    strings.TrimSpace(cfg.Gateway.UpstreamBaseURL) != "",
		GatewayAPIKeyEnvConfigured:   strings.TrimSpace(cfg.Gateway.APIKeyEnv) != "",
		AnthropicUpstreamConfigured:  strings.TrimSpace(cfg.Gateway.AnthropicUpstreamBaseURL) != "",
		AnthropicAPIKeyEnvConfigured: strings.TrimSpace(cfg.Gateway.AnthropicAPIKeyEnv) != "",
	}
	if cfg.Webhooks.Enabled {
		status.OutboundSurfaces = append(status.OutboundSurfaces, "webhooks")
	}
	if cfg.Gateway.Enabled {
		status.OutboundSurfaces = append(status.OutboundSurfaces, "gateway")
	}
	sort.Strings(status.OutboundSurfaces)
	return status
}

func durationString(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	return d.String()
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func severityRank(severity string) int {
	switch strings.ToLower(severity) {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}
