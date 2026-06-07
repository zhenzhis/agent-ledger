package integrations

import (
	"sort"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// Source describes one local collector source without exposing raw paths.
type Source struct {
	Source    string `json:"source"`
	Enabled   bool   `json:"enabled"`
	PathCount int    `json:"path_count"`
}

// Options controls runtime-specific capability flags for the registry.
type Options struct {
	Sources         []Source `json:"sources"`
	PricingMode     string   `json:"pricing_mode"`
	PoliciesEnabled bool     `json:"policies_enabled"`
	RBACEnabled     bool     `json:"rbac_enabled"`
	QuotaEnabled    bool     `json:"quota_enabled"`
	WebhooksEnabled bool     `json:"webhooks_enabled"`
}

// Capability describes one supported or planned integration surface.
type Capability struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	Protocol       string   `json:"protocol"`
	Direction      string   `json:"direction"`
	Status         string   `json:"status"`
	Maturity       string   `json:"maturity"`
	Enabled        bool     `json:"enabled"`
	Privacy        string   `json:"privacy"`
	EventTypes     []string `json:"event_types,omitempty"`
	Endpoints      []string `json:"endpoints,omitempty"`
	Commands       []string `json:"commands,omitempty"`
	Tools          []string `json:"tools,omitempty"`
	DataClasses    []string `json:"data_classes,omitempty"`
	Limitations    []string `json:"limitations,omitempty"`
	NextMilestones []string `json:"next_milestones,omitempty"`
}

// Summary captures high-level registry counts.
type Summary struct {
	Implemented       int `json:"implemented"`
	Experimental      int `json:"experimental"`
	Planned           int `json:"planned"`
	EnabledCollectors int `json:"enabled_collectors"`
}

// Catalog is the public Agent Ledger ecosystem capability contract.
type Catalog struct {
	Product        string       `json:"product"`
	Contract       string       `json:"contract"`
	Version        string       `json:"version"`
	PrivacyDefault string       `json:"privacy_default"`
	Summary        Summary      `json:"summary"`
	Capabilities   []Capability `json:"capabilities"`
}

// OptionsFromConfig builds privacy-safe registry options from runtime config.
func OptionsFromConfig(cfg *config.Config) Options {
	if cfg == nil {
		return Options{}
	}
	return Options{
		Sources: []Source{
			sourceFromConfig("claude", cfg.Collectors.Claude),
			sourceFromConfig("codex", cfg.Collectors.Codex),
			sourceFromConfig("openclaw", cfg.Collectors.OpenClaw),
			sourceFromConfig("opencode", cfg.Collectors.OpenCode),
			sourceFromConfig("kiro", cfg.Collectors.Kiro),
			sourceFromConfig("pi", cfg.Collectors.Pi),
		},
		PricingMode:     cfg.Pricing.Mode,
		PoliciesEnabled: cfg.Policies.Enabled,
		RBACEnabled:     cfg.RBAC.Enabled,
		QuotaEnabled:    cfg.Quota.Enabled,
		WebhooksEnabled: cfg.Webhooks.Enabled,
	}
}

// Registry returns a stable, metadata-only capability catalog.
func Registry(opts Options) Catalog {
	eventTypes := canonicalEventNames()
	capabilities := []Capability{
		{
			ID:         "protocol.canonical_events.http",
			Name:       "Canonical Events HTTP Ingest",
			Category:   "protocol",
			Protocol:   "HTTP JSON",
			Direction:  "ingest",
			Status:     "implemented",
			Maturity:   "stable-v1",
			Enabled:    true,
			Privacy:    "metadata-only payloads; raw prompt and content keys are rejected",
			EventTypes: eventTypes,
			Endpoints:  []string{"GET /api/event-schema", "POST /api/events"},
			DataClasses: []string{
				"workload metadata", "model call usage", "tool call metadata", "context references", "artifact hashes", "evaluation signals", "policy decisions",
			},
		},
		{
			ID:         "protocol.canonical_events.cli",
			Name:       "Canonical Events CLI Ingest",
			Category:   "protocol",
			Protocol:   "CLI JSON",
			Direction:  "ingest",
			Status:     "implemented",
			Maturity:   "stable-v1",
			Enabled:    true,
			Privacy:    "metadata-only payloads; stdin and file inputs are size limited",
			EventTypes: eventTypes,
			Commands:   []string{"agent-ledger event schema", "agent-ledger event ingest --file event.json"},
		},
		{
			ID:        "protocol.mcp_stdio",
			Name:      "Local MCP Tool Server",
			Category:  "agent-tool",
			Protocol:  "MCP-compatible stdio JSON-RPC",
			Direction: "bidirectional",
			Status:    "implemented",
			Maturity:  "local-preview",
			Enabled:   true,
			Privacy:   "local stdio only; does not connect to remote MCP hosts by itself",
			Tools:     []string{"ledger.current_budget", "ledger.start_workload", "ledger.close_workload", "ledger.record_artifact", "ledger.record_event", "ledger.event_schema", "ledger.integrations", "ledger.get_policy", "ledger.explain_cost", "ledger.find_similar_workloads"},
		},
		{
			ID:        "protocol.offline_bundle",
			Name:      "Signed Offline Bundle",
			Category:  "air-gapped-sync",
			Protocol:  "JSON bundle with SHA-256 and optional HMAC-SHA256",
			Direction: "export-import",
			Status:    "implemented",
			Maturity:  "local-preview",
			Enabled:   true,
			Privacy:   "supports redacted exports; imports replay canonical metadata events",
			Endpoints: []string{"GET /api/offline-bundle/export", "POST /api/offline-bundle/import"},
			Commands:  []string{"agent-ledger bundle export --privacy --signed", "agent-ledger bundle import --verify"},
		},
		{
			ID:        "governance.policy_evaluator",
			Name:      "Local Policy Evaluator",
			Category:  "governance",
			Protocol:  "Config rules",
			Direction: "advisory",
			Status:    "implemented",
			Maturity:  "local-preview",
			Enabled:   opts.PoliciesEnabled,
			Privacy:   "records rule metadata only; enforcement is delegated to wrappers or gateways",
			Endpoints: []string{"GET /api/policies/status", "POST /api/policy/evaluate", "GET /api/policy/decisions"},
			Commands:  []string{"agent-ledger policy evaluate --model gpt-5.5 --action model.call"},
		},
		{
			ID:          "governance.pricing",
			Name:        "Pricing Governance",
			Category:    "finops",
			Protocol:    "official seeds + LiteLLM fallback + local overrides",
			Direction:   "sync",
			Status:      "implemented",
			Maturity:    "production-local",
			Enabled:     true,
			Privacy:     "pricing sync is outbound; usage data is not uploaded",
			Endpoints:   []string{"GET /api/pricing/status", "POST /api/pricing/sync", "POST /api/pricing/recalculate"},
			Commands:    []string{"agent-ledger pricing sync"},
			DataClasses: []string{"model prices", "pricing source health", "unpriced model diagnostics"},
			Limitations: []string{"official seed rows are embedded snapshots; enterprise contract prices should use local overrides"},
		},
		{
			ID:             "protocol.opentelemetry_genai",
			Name:           "OpenTelemetry GenAI Mapping",
			Category:       "protocol",
			Protocol:       "OpenTelemetry JSON spans",
			Direction:      "ingest",
			Status:         "implemented",
			Maturity:       "local-preview",
			Enabled:        true,
			Privacy:        "metadata-only mapping; prompt/completion message attributes are intentionally excluded",
			EventTypes:     eventTypes,
			Endpoints:      []string{"POST /api/otel/genai"},
			Commands:       []string{"agent-ledger otel convert --file spans.json", "agent-ledger otel ingest --file spans.json"},
			Limitations:    []string{"local JSON mapper, not a full OTLP collector", "prompt and completion message attributes are intentionally not persisted"},
			NextMilestones: []string{"support trace_id/span_id context references", "add OTLP receiver mode behind explicit config", "expand fixture-based conformance tests"},
		},
		{
			ID:             "protocol.a2a",
			Name:           "Agent-to-Agent Task Telemetry",
			Category:       "protocol",
			Protocol:       "A2A task events",
			Direction:      "ingest",
			Status:         "planned",
			Maturity:       "roadmap",
			Enabled:        false,
			Privacy:        "planned adapter should record task metadata, not message content",
			EventTypes:     []string{"workload.started", "agent.run.started", "agent.run.finished", "artifact.created", "evaluation.recorded", "policy.decision"},
			NextMilestones: []string{"map task lifecycle to workloads", "map delegated agents to parent/child runs", "support evidence bundle references"},
		},
		{
			ID:             "gateway.provider_api",
			Name:           "Provider Gateway Adapter",
			Category:       "gateway",
			Protocol:       "OpenAI-compatible and provider-native APIs",
			Direction:      "observe",
			Status:         "planned",
			Maturity:       "roadmap",
			Enabled:        false,
			Privacy:        "gateway must redact or hash prompt content before ledger writes",
			EventTypes:     []string{"model.call", "policy.decision"},
			NextMilestones: []string{"request/response metadata wrapper", "budget-aware advisory policies", "provider reconciliation hooks"},
		},
	}
	capabilities = append(capabilities, collectorCapabilities(opts.Sources)...)
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i].ID < capabilities[j].ID })
	return Catalog{
		Product:        "Agent Ledger",
		Contract:       "agent-ledger.integration-capability-catalog",
		Version:        "v1",
		PrivacyDefault: "local-first metadata-only; no prompt content required",
		Summary:        summarize(capabilities),
		Capabilities:   capabilities,
	}
}

func sourceFromConfig(name string, cfg config.CollectorConfig) Source {
	return Source{Source: name, Enabled: cfg.Enabled, PathCount: len(cfg.Paths)}
}

func collectorCapabilities(sources []Source) []Capability {
	out := make([]Capability, 0, len(sources))
	for _, source := range sources {
		out = append(out, Capability{
			ID:          "collector." + source.Source,
			Name:        source.Source + " local collector",
			Category:    "collector",
			Protocol:    collectorProtocol(source.Source),
			Direction:   "ingest",
			Status:      "implemented",
			Maturity:    "source-specific",
			Enabled:     source.Enabled,
			Privacy:     "reads configured local paths only; registry exposes path counts, not raw paths",
			EventTypes:  []string{"model.call", "workload.started", "agent.run.started"},
			DataClasses: []string{"usage tokens", "session metadata", "project metadata", "model names"},
			Limitations: collectorLimitations(source),
		})
	}
	return out
}

func collectorProtocol(source string) string {
	switch source {
	case "opencode":
		return "local SQLite"
	case "kiro":
		return "local SQLite + JSON session files"
	default:
		return "local JSONL/session files"
	}
}

func collectorLimitations(source Source) []string {
	limits := []string{}
	if !source.Enabled {
		limits = append(limits, "disabled in current config")
	}
	if source.PathCount == 0 {
		limits = append(limits, "no configured paths")
	}
	if source.Source == "kiro" {
		limits = append(limits, "some token values are estimated when native usage fields are unavailable")
	}
	return limits
}

func canonicalEventNames() []string {
	types := storage.CanonicalEventTypes()
	names := make([]string, 0, len(types))
	for _, t := range types {
		names = append(names, t.EventType)
	}
	sort.Strings(names)
	return names
}

func summarize(capabilities []Capability) Summary {
	var summary Summary
	for _, cap := range capabilities {
		switch cap.Status {
		case "implemented":
			summary.Implemented++
		case "experimental":
			summary.Experimental++
		case "planned":
			summary.Planned++
		}
		if cap.Category == "collector" && cap.Enabled {
			summary.EnabledCollectors++
		}
	}
	return summary
}
