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
	Sources             []Source `json:"sources"`
	PricingMode         string   `json:"pricing_mode"`
	PoliciesEnabled     bool     `json:"policies_enabled"`
	RBACEnabled         bool     `json:"rbac_enabled"`
	QuotaEnabled        bool     `json:"quota_enabled"`
	WebhooksEnabled     bool     `json:"webhooks_enabled"`
	OTLPReceiverEnabled bool     `json:"otlp_receiver_enabled"`
	GatewayEnabled      bool     `json:"gateway_enabled"`
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
	Resources      []string `json:"resources,omitempty"`
	Prompts        []string `json:"prompts,omitempty"`
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
		PricingMode:         cfg.Pricing.Mode,
		PoliciesEnabled:     cfg.Policies.Enabled,
		RBACEnabled:         cfg.RBAC.Enabled,
		QuotaEnabled:        cfg.Quota.Enabled,
		WebhooksEnabled:     cfg.Webhooks.Enabled,
		OTLPReceiverEnabled: cfg.Integrations.OTLPReceiver.Enabled,
		GatewayEnabled:      cfg.Gateway.Enabled,
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
			Resources: []string{"agent-ledger://schema/canonical-events", "agent-ledger://integrations/catalog", "agent-ledger://budget/current", "agent-ledger://workloads/recent", "agent-ledger://policies/status"},
			Prompts:   []string{"agent-ledger/workload-brief", "agent-ledger/cost-review", "agent-ledger/incident-evidence"},
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
			Endpoints: []string{"GET /api/policies/status", "POST /api/policy/evaluate", "GET /api/policy/decisions", "GET/POST /api/policy/approvals"},
			Commands:  []string{"agent-ledger policy evaluate --model gpt-5.5 --action model.call", "agent-ledger policy approvals", "agent-ledger policy resolve --id apr_... --status approved"},
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
			NextMilestones: []string{"add OTLP receiver mode behind explicit config", "expand fixture-based conformance tests"},
		},
		{
			ID:             "protocol.a2a",
			Name:           "Agent-to-Agent Task Telemetry",
			Category:       "protocol",
			Protocol:       "A2A JSON task snapshots/events",
			Direction:      "ingest",
			Status:         "implemented",
			Maturity:       "local-preview",
			Enabled:        true,
			Privacy:        "metadata-only task mapping; message history, message parts, and artifact parts are intentionally excluded",
			EventTypes:     []string{"workload.started", "agent.run.started", "agent.run.finished", "artifact.created", "evaluation.recorded", "policy.decision"},
			Endpoints:      []string{"POST /api/a2a/tasks"},
			Commands:       []string{"agent-ledger a2a convert --file task.json", "agent-ledger a2a ingest --file task.json"},
			Limitations:    []string{"local JSON mapper, not a full A2A server", "does not persist A2A message/history content"},
			NextMilestones: []string{"map delegated agents to parent/child runs from richer metadata", "support evidence bundle references", "add A2A server discovery metadata"},
		},
		{
			ID:             "gateway.provider_api",
			Name:           "Provider Usage Envelope Adapter",
			Category:       "gateway",
			Protocol:       "OpenAI-compatible, Anthropic-style, and LiteLLM-style usage JSON",
			Direction:      "ingest",
			Status:         "implemented",
			Maturity:       "local-preview",
			Enabled:        true,
			Privacy:        "metadata-only usage mapping; request/response message content is intentionally excluded",
			EventTypes:     []string{"model.call", "context.ref"},
			Endpoints:      []string{"POST /api/provider/calls"},
			Commands:       []string{"agent-ledger provider convert --file response.json", "agent-ledger provider ingest --file response.json"},
			Limitations:    []string{"local usage envelope mapper, not a live reverse proxy", "does not perform request routing or API key handling"},
			NextMilestones: []string{"request/response metadata wrapper", "budget-aware advisory policies", "provider reconciliation hooks"},
		},
		{
			ID:          "finops.provider_reconciliation",
			Name:        "Provider Bill Reconciliation",
			Category:    "finops",
			Protocol:    "Local CSV/JSON statement import",
			Direction:   "import-compare",
			Status:      "implemented",
			Maturity:    "local-preview",
			Enabled:     true,
			Privacy:     "stores statement summary, SHA-256 hash, window, warnings, and cost diff; raw statement rows are not persisted",
			Endpoints:   []string{"GET /api/reconciliation/status", "POST /api/reconciliation/import"},
			Commands:    []string{"agent-ledger reconcile parse --file provider-bill.csv", "agent-ledger reconcile import --file provider-bill.csv", "agent-ledger reconcile status"},
			DataClasses: []string{"provider cost summary", "local cost summary", "statement integrity hash", "statement window", "currency warnings"},
			Limitations: []string{"non-USD rows are reported as warnings and ignored until an explicit conversion profile is configured"},
		},
		{
			ID:             "gateway.provider_live_proxy",
			Name:           "Live Provider Gateway Mode",
			Category:       "gateway",
			Protocol:       "OpenAI-compatible non-streaming HTTP JSON proxy",
			Direction:      "observe-and-route",
			Status:         "experimental",
			Maturity:       "local-preview",
			Enabled:        opts.GatewayEnabled,
			Privacy:        "forwards prompt content only in-memory to the configured upstream; ledger writes usage metadata only; API keys are read from env and not persisted",
			EventTypes:     []string{"model.call", "policy.decision", "context.ref"},
			Endpoints:      []string{"POST /gateway/openai/v1/chat/completions"},
			Limitations:    []string{"disabled by default", "non-streaming JSON requests only", "OpenAI-compatible chat completions MVP", "does not persist prompt or response content"},
			NextMilestones: []string{"streaming capture with conformance tests", "provider-native adapters", "budget-aware routing decisions"},
		},
		{
			ID:             "protocol.otlp_receiver",
			Name:           "OpenTelemetry OTLP Receiver",
			Category:       "protocol",
			Protocol:       "OTLP HTTP/JSON traces",
			Direction:      "ingest",
			Status:         "experimental",
			Maturity:       "local-preview",
			Enabled:        opts.OTLPReceiverEnabled,
			Privacy:        "metadata-only span projection; prompt/message attributes are intentionally not persisted",
			EventTypes:     []string{"model.call", "context.ref"},
			Endpoints:      []string{"POST /v1/traces", "POST /api/otlp/v1/traces"},
			Limitations:    []string{"JSON receiver only; OTLP protobuf and gRPC are not yet accepted", "disabled by default and restricted to localhost or authenticated operators"},
			NextMilestones: []string{"OTLP protobuf/gRPC conformance", "collector exporter examples", "backpressure metrics"},
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
