package integrations

import (
	"sort"
	"strings"

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
	ReadOnly            bool     `json:"read_only"`
	QuotaEnabled        bool     `json:"quota_enabled"`
	WebhooksEnabled     bool     `json:"webhooks_enabled"`
	OTLPReceiverEnabled bool     `json:"otlp_receiver_enabled"`
	GatewayEnabled      bool     `json:"gateway_enabled"`
}

// Capability describes one supported or planned integration surface.
type Capability struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Category            string   `json:"category"`
	Protocol            string   `json:"protocol"`
	Direction           string   `json:"direction"`
	Status              string   `json:"status"`
	Maturity            string   `json:"maturity"`
	Enabled             bool     `json:"enabled"`
	WritesLocalState    bool     `json:"writes_local_state"`
	AvailableInReadOnly bool     `json:"available_in_read_only"`
	RuntimeStatus       string   `json:"runtime_status"`
	Privacy             string   `json:"privacy"`
	EventTypes          []string `json:"event_types,omitempty"`
	Endpoints           []string `json:"endpoints,omitempty"`
	Commands            []string `json:"commands,omitempty"`
	Tools               []string `json:"tools,omitempty"`
	Resources           []string `json:"resources,omitempty"`
	Prompts             []string `json:"prompts,omitempty"`
	DataClasses         []string `json:"data_classes,omitempty"`
	Limitations         []string `json:"limitations,omitempty"`
	NextMilestones      []string `json:"next_milestones,omitempty"`
}

// Summary captures high-level registry counts.
type Summary struct {
	Implemented       int `json:"implemented"`
	Experimental      int `json:"experimental"`
	Planned           int `json:"planned"`
	EnabledCollectors int `json:"enabled_collectors"`
	ReadOnlyLimited   int `json:"read_only_limited"`
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
		ReadOnly:            cfg.RBAC.ReadOnly,
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
			Endpoints:  []string{"GET /api/event-schema", "GET /api/event-examples", "POST /api/events/validate", "POST /api/events"},
			DataClasses: []string{
				"workload metadata", "adapter provenance", "model call usage", "tool call metadata", "context references", "artifact hashes", "evaluation signals", "policy decisions",
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
			Privacy:    "metadata-only payloads; stdin and file inputs are size limited; provenance fields should use hashes, row ids, or offsets instead of raw prompt content",
			EventTypes: eventTypes,
			Commands:   []string{"agent-ledger event schema", "agent-ledger event examples --type model.call", "agent-ledger event validate --file event.json", "agent-ledger event ingest --file event.json"},
		},
		{
			ID:          "protocol.adapter_conformance",
			Name:        "Adapter Conformance Validation",
			Category:    "protocol",
			Protocol:    "HTTP JSON + CLI JSON",
			Direction:   "validate",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "dry-run only; validates converted metadata-only events without writing SQLite",
			EventTypes:  eventTypes,
			Endpoints:   []string{"GET /api/integrations/adapter-spec", "POST /api/integrations/conformance?kind=canonical|provider|provider-stream|otel|a2a&strict=true"},
			Commands:    []string{"agent-ledger adapter spec", "agent-ledger adapter conformance --kind provider-stream --strict --file fixture.sse"},
			DataClasses: []string{"adapter fixture metadata", "canonical event validation result", "provenance warnings"},
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
			Tools:     []string{"ledger.current_budget", "ledger.start_workload", "ledger.start_run", "ledger.close_workload", "ledger.link_workloads", "ledger.heartbeat_run", "ledger.run_liveness", "ledger.workload_timeline", "ledger.workload_state", "ledger.record_tool_call", "ledger.record_context", "ledger.record_artifact", "ledger.record_evaluation", "ledger.record_event", "ledger.validate_event", "ledger.event_schema", "ledger.event_examples", "ledger.adapter_contract", "ledger.adapter_conformance", "ledger.integrations", "ledger.get_policy", "ledger.policy_audit", "ledger.audit_log", "ledger.explain_cost", "ledger.find_similar_workloads"},
			Resources: []string{"agent-ledger://schema/canonical-events", "agent-ledger://schema/canonical-event-examples", "agent-ledger://integrations/catalog", "agent-ledger://integrations/adapter-contract", "agent-ledger://budget/current", "agent-ledger://workloads/recent", "agent-ledger://policies/status"},
			Prompts:   []string{"agent-ledger/workload-brief", "agent-ledger/cost-review", "agent-ledger/incident-evidence"},
		},
		{
			ID:          "protocol.workload_event_feed",
			Name:        "Local Workload Event Feed",
			Category:    "agent-observability",
			Protocol:    "HTTP JSON + SSE + CLI JSON",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "local-preview",
			Enabled:     true,
			Privacy:     "derived metadata-only state feed; supports privacy redaction and does not include prompt content",
			EventTypes:  []string{"workload.state.planned", "workload.state.running", "workload.state.stale", "workload.state.blocked", "workload.state.needs_approval", "workload.state.needs_evaluation", "workload.state.accepted", "workload.state.terminal"},
			Endpoints:   []string{"GET /api/workload-events", "GET /api/workload-events/stream"},
			Commands:    []string{"agent-ledger workload feed --severity warning --max-age 10m"},
			DataClasses: []string{"workload state", "phase", "severity", "next action", "risk metadata"},
			Limitations: []string{"derived snapshot feed; SSE stream is a local polling subscription"},
			NextMilestones: []string{
				"add native MCP subscription transport when host clients support it",
				"allow desktop notification adapters to consume the same event schema",
			},
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
			Endpoints: []string{"GET /api/policies/status", "POST /api/policy/evaluate", "GET /api/policy/audit", "GET /api/policy/enforcement", "GET /api/policy/decisions", "GET/POST /api/policy/approvals"},
			Commands:  []string{"agent-ledger policy evaluate --model gpt-5.5 --action model.call", "agent-ledger policy audit", "agent-ledger policy enforcement --privacy", "agent-ledger policy approvals", "agent-ledger policy resolve --id apr_... --status approved"},
		},
		{
			ID:          "notification.redacted_webhook",
			Name:        "Redacted Webhook Notifications",
			Category:    "notification",
			Protocol:    "HTTP JSON webhook",
			Direction:   "outbound",
			Status:      "implemented",
			Maturity:    "local-preview",
			Enabled:     opts.WebhooksEnabled,
			Privacy:     "disabled by default; sends bounded redacted workload-event summaries only",
			Endpoints:   []string{"POST /api/notifications/webhook"},
			Commands:    []string{"agent-ledger notify webhook --dry-run", "agent-ledger notify webhook --severity warning"},
			DataClasses: []string{"redacted workload ids", "phase", "severity", "next action", "risk metadata"},
			Limitations: []string{"requires explicit webhooks.enabled and webhooks.url", "does not send prompt content, local paths, project names, branch names, or webhook URL in audit logs"},
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
			EventTypes:     []string{"workload.started", "agent.run.started", "agent.run.heartbeat", "agent.run.finished", "artifact.created", "evaluation.recorded", "policy.decision"},
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
			Protocol:       "OpenAI-compatible Chat Completions HTTP JSON/SSE, OpenAI Responses HTTP JSON/SSE, and Anthropic Messages HTTP JSON/SSE proxy",
			Direction:      "observe-and-route",
			Status:         "experimental",
			Maturity:       "local-preview",
			Enabled:        opts.GatewayEnabled,
			Privacy:        "forwards prompt content only in-memory to the configured upstream; ledger writes usage metadata only; API keys are read from env and not persisted",
			EventTypes:     []string{"model.call", "policy.decision", "context.ref"},
			Endpoints:      []string{"POST /gateway/openai/v1/chat/completions", "POST /gateway/openai/v1/responses", "POST /gateway/anthropic/v1/messages"},
			Limitations:    []string{"disabled by default", "OpenAI Chat Completions streaming usage requires upstream final usage chunks; gateway requests them by default when compatible", "OpenAI Responses streaming records usage from response.completed events", "Anthropic Messages streaming records usage from message_start/message_delta events", "does not persist prompt or response content"},
			NextMilestones: []string{"budget-aware routing decisions", "streaming conformance fixtures for more providers"},
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
	annotateRuntimeCapabilities(capabilities, opts)
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

func annotateRuntimeCapabilities(capabilities []Capability, opts Options) {
	for i := range capabilities {
		cap := &capabilities[i]
		cap.WritesLocalState = capabilityWritesLocalState(cap.ID)
		cap.AvailableInReadOnly = !cap.WritesLocalState || capabilityReadOnlyPartial(cap.ID)
		switch {
		case opts.ReadOnly && cap.WritesLocalState && cap.AvailableInReadOnly:
			cap.RuntimeStatus = "read-only limited: query/export surfaces remain available; write/import/sync operations are disabled"
		case opts.ReadOnly && cap.WritesLocalState:
			cap.Enabled = false
			cap.RuntimeStatus = "disabled by read-only mode"
		case cap.Enabled:
			cap.RuntimeStatus = "enabled"
		default:
			cap.RuntimeStatus = "disabled by config"
		}
	}
}

func capabilityWritesLocalState(id string) bool {
	if strings.HasPrefix(id, "collector.") {
		return true
	}
	switch id {
	case "protocol.canonical_events.http",
		"protocol.canonical_events.cli",
		"protocol.mcp_stdio",
		"protocol.offline_bundle",
		"governance.policy_evaluator",
		"notification.redacted_webhook",
		"governance.pricing",
		"protocol.opentelemetry_genai",
		"protocol.a2a",
		"gateway.provider_api",
		"finops.provider_reconciliation",
		"gateway.provider_live_proxy",
		"protocol.otlp_receiver":
		return true
	default:
		return false
	}
}

func capabilityReadOnlyPartial(id string) bool {
	switch id {
	case "protocol.mcp_stdio",
		"protocol.offline_bundle",
		"governance.policy_evaluator",
		"governance.pricing":
		return true
	default:
		return false
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
		if cap.WritesLocalState && cap.AvailableInReadOnly && strings.HasPrefix(cap.RuntimeStatus, "read-only") {
			summary.ReadOnlyLimited++
		}
	}
	return summary
}
