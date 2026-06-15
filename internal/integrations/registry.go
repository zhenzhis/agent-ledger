package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
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
	Sources                 []Source `json:"sources"`
	PricingMode             string   `json:"pricing_mode"`
	PoliciesEnabled         bool     `json:"policies_enabled"`
	RBACEnabled             bool     `json:"rbac_enabled"`
	ReadOnly                bool     `json:"read_only"`
	QuotaEnabled            bool     `json:"quota_enabled"`
	WebhooksEnabled         bool     `json:"webhooks_enabled"`
	OTLPReceiverEnabled     bool     `json:"otlp_receiver_enabled"`
	OTLPReceiverGRPCEnabled bool     `json:"otlp_receiver_grpc_enabled"`
	GatewayEnabled          bool     `json:"gateway_enabled"`
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
		PricingMode:             cfg.Pricing.Mode,
		PoliciesEnabled:         cfg.Policies.Enabled,
		RBACEnabled:             cfg.RBAC.Enabled,
		ReadOnly:                cfg.RBAC.ReadOnly,
		QuotaEnabled:            cfg.Quota.Enabled,
		WebhooksEnabled:         cfg.Webhooks.Enabled,
		OTLPReceiverEnabled:     cfg.Integrations.OTLPReceiver.Enabled,
		OTLPReceiverGRPCEnabled: cfg.Integrations.OTLPReceiver.GRPCEnabled,
		GatewayEnabled:          cfg.Gateway.Enabled,
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
			Endpoints:   []string{"GET /api/integrations/adapter-spec", "GET /api/integrations/conformance-matrix", "POST /api/integrations/conformance?kind=canonical|provider|provider-stream|otel|a2a&strict=true"},
			Commands:    []string{"agent-ledger adapter spec", "agent-ledger adapter matrix", "agent-ledger adapter conformance --kind provider-stream --strict --file fixture.sse"},
			Tools:       []string{"ledger.adapter_contract", "ledger.conformance_matrix", "ledger.adapter_conformance"},
			Resources:   []string{"agent-ledger://integrations/adapter-contract", "agent-ledger://integrations/conformance-matrix"},
			DataClasses: []string{"adapter fixture metadata", "canonical event validation result", "provenance warnings"},
		},
		{
			ID:          "protocol.provider_profiles",
			Name:        "Provider Profile Catalog",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "static provider/runtime metadata only; no API credentials, endpoints with secrets, local paths, prompts, responses, sessions, machine names, authors, or usage rows",
			Endpoints:   []string{"GET /api/provider-profiles"},
			Commands:    []string{"agent-ledger provider profiles"},
			Tools:       []string{"ledger.provider_profiles"},
			Resources:   []string{"agent-ledger://integrations/provider-profiles"},
			DataClasses: []string{"provider families", "usage schema names", "gateway route names", "pricing strategy labels", "privacy notes", "conformance fixture paths"},
		},
		{
			ID:          "protocol.agent_profiles",
			Name:        "Agent Framework Profile Catalog",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "static agent CLI, framework, wrapper, router, protocol, and observability metadata only; no raw local paths, prompts, responses, sessions, machine names, authors, account ids, or usage rows",
			Endpoints:   []string{"GET /api/agent-profiles"},
			Commands:    []string{"agent-ledger agent profiles"},
			Tools:       []string{"ledger.agent_profiles"},
			Resources:   []string{"agent-ledger://integrations/agent-profiles"},
			DataClasses: []string{"agent framework families", "recommended ingest surfaces", "available signal names", "canonical event type names", "collector source names", "quality gates", "limitations"},
		},
		{
			ID:          "protocol.integration_recommendation",
			Name:        "Integration Recommendation Advisor",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "static advisor metadata only; request parameters are used to choose ingest, validation, privacy, and quality gates without reading local usage rows",
			Endpoints:   []string{"GET /api/integrations/recommendation"},
			Commands:    []string{"agent-ledger agent recommend --profile codex-cli --provider openai-official --surface provider-stream --signals model,usage,cache"},
			Tools:       []string{"ledger.integration_recommendation"},
			Resources:   []string{"agent-ledger://integrations/recommendation"},
			DataClasses: []string{"profile ids", "surface names", "signal names", "validation commands", "contract hashes", "privacy checklists", "quality gates"},
			Limitations: []string{"recommendations are advisory and metadata-only; wrappers still need fixture validation and admission checks before enabling write ingest"},
		},
		{
			ID:          "protocol.signal_taxonomy",
			Name:        "Signal Taxonomy Catalog",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "static metadata-only signal dictionary; no usage rows, prompts, responses, local paths, sessions, machine names, authors, or secrets",
			Endpoints:   []string{"GET /api/signal-taxonomy"},
			Commands:    []string{"agent-ledger signals"},
			Tools:       []string{"ledger.signal_taxonomy"},
			Resources:   []string{"agent-ledger://integrations/signal-taxonomy"},
			DataClasses: []string{"signal ids", "canonical event families", "recommended field names", "precision labels", "privacy classes", "quality gates"},
		},
		{
			ID:          "protocol.signal_coverage",
			Name:        "Signal Coverage Report",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "static metadata-only coverage report; no usage rows, prompts, responses, local paths, sessions, machine names, authors, or secrets",
			Endpoints:   []string{"GET /api/integrations/signal-coverage"},
			Commands:    []string{"agent-ledger signal-coverage", "agent-ledger signals coverage"},
			Tools:       []string{"ledger.signal_coverage"},
			Resources:   []string{"agent-ledger://integrations/signal-coverage"},
			DataClasses: []string{"taxonomy signal ids", "adapter kind coverage", "provider profile ids", "agent profile ids", "coverage gap metadata"},
		},
		{
			ID:          "protocol.discovery_manifest",
			Name:        "Local Discovery Manifest",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "privacy-safe manifest; exposes capability counts, entrypoint URIs, schema hashes, and runtime flags without local paths or prompt content",
			Endpoints:   []string{"GET /.well-known/agent-ledger.json", "GET /api/discovery"},
			Commands:    []string{"agent-ledger discovery"},
			Tools:       []string{"ledger.discovery"},
			Resources:   []string{"agent-ledger://discovery/manifest"},
			DataClasses: []string{"entrypoint URIs", "runtime flags", "schema hashes", "adapter contract hash", "capability summary"},
		},
		{
			ID:          "protocol.contract_bundle",
			Name:        "Contract Bundle Index",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "metadata-only contract index; exposes URIs, hashes, cache semantics, and read-only/write-state flags without paths, prompts, sessions, or secrets",
			Endpoints:   []string{"GET /api/contracts"},
			Commands:    []string{"agent-ledger contracts"},
			Tools:       []string{"ledger.contracts"},
			Resources:   []string{"agent-ledger://contracts/bundle"},
			DataClasses: []string{"contract URIs", "schema hashes", "runtime hash", "ETag semantics", "CLI/MCP entrypoints"},
		},
		{
			ID:          "protocol.contract_verification",
			Name:        "Contract Verification Report",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "metadata-only self-check for contract hashes, stable paths, read-only semantics, and privacy flags; excludes prompts, sessions, secrets, and local paths",
			Endpoints:   []string{"GET /api/contracts/verify"},
			Commands:    []string{"agent-ledger contracts verify"},
			Tools:       []string{"ledger.contracts_verify"},
			Resources:   []string{"agent-ledger://contracts/verification"},
			DataClasses: []string{"contract check names", "expected hashes", "actual hashes", "read-only/write-state flags", "privacy invariant status"},
		},
		{
			ID:          "protocol.goal_coverage",
			Name:        "Agent Ledger Goal Coverage",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "metadata-only coverage report; exposes planned product sections, capability ids, endpoint names, commands, docs, tests, and external dependencies without local data",
			Endpoints:   []string{"GET /api/goal-coverage"},
			Commands:    []string{"agent-ledger goal coverage"},
			Tools:       []string{"ledger.goal_coverage"},
			Resources:   []string{"agent-ledger://goal/coverage"},
			DataClasses: []string{"goal section ids", "capability ids", "contract hashes", "verification commands", "external dependency notes"},
		},
		{
			ID:          "protocol.openapi",
			Name:        "Control Plane OpenAPI",
			Category:    "control-plane",
			Protocol:    "OpenAPI 3.1 + HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "metadata-only OpenAPI document for stable local control-plane endpoints; excludes prompt content, response content, secrets, sessions, and local paths",
			Endpoints:   []string{"GET /api/openapi.json"},
			Commands:    []string{"agent-ledger openapi"},
			Tools:       []string{"ledger.openapi"},
			Resources:   []string{"agent-ledger://contracts/openapi"},
			DataClasses: []string{"OpenAPI paths", "query parameters", "request envelopes", "response schemas", "read-only/write-state metadata"},
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
			Tools:     []string{"ledger.current_budget", "ledger.discovery", "ledger.contracts", "ledger.contracts_verify", "ledger.goal_coverage", "ledger.openapi", "ledger.runtime_status", "ledger.config_status", "ledger.readiness", "ledger.admission_check", "ledger.start_workload", "ledger.start_run", "ledger.close_workload", "ledger.link_workloads", "ledger.claim_next_workload", "ledger.workload_queue", "ledger.acquire_workload_lease", "ledger.renew_workload_lease", "ledger.release_workload_lease", "ledger.workload_leases", "ledger.heartbeat_run", "ledger.run_liveness", "ledger.workload_timeline", "ledger.workload_state", "ledger.workload_feed", "ledger.record_tool_call", "ledger.record_context", "ledger.record_artifact", "ledger.record_evaluation", "ledger.record_event", "ledger.validate_event", "ledger.event_schema", "ledger.event_examples", "ledger.adapter_contract", "ledger.adapter_conformance", "ledger.conformance_matrix", "ledger.integrations", "ledger.provider_profiles", "ledger.agent_profiles", "ledger.signal_taxonomy", "ledger.signal_coverage", "ledger.integration_recommendation", "ledger.get_policy", "ledger.policy_audit", "ledger.approval_routes", "ledger.approvals", "ledger.resolve_approval", "ledger.audit_log", "ledger.explain_cost", "ledger.find_similar_workloads"},
			Resources: []string{"agent-ledger://discovery/manifest", "agent-ledger://contracts/bundle", "agent-ledger://contracts/verification", "agent-ledger://goal/coverage", "agent-ledger://contracts/openapi", "agent-ledger://schema/canonical-events", "agent-ledger://schema/canonical-event-examples", "agent-ledger://integrations/catalog", "agent-ledger://integrations/provider-profiles", "agent-ledger://integrations/agent-profiles", "agent-ledger://integrations/signal-taxonomy", "agent-ledger://integrations/signal-coverage", "agent-ledger://integrations/recommendation", "agent-ledger://integrations/adapter-contract", "agent-ledger://integrations/conformance-matrix", "agent-ledger://runtime/status", "agent-ledger://config/status", "agent-ledger://readiness", "agent-ledger://admission/check", "agent-ledger://budget/current", "agent-ledger://workloads/recent", "agent-ledger://workloads/queue", "agent-ledger://workloads/leases", "agent-ledger://workloads/feed", "agent-ledger://workload/state", "agent-ledger://workload/timeline", "agent-ledger://agent-runs/liveness", "agent-ledger://policies/status", "agent-ledger://policy/approvals", "agent-ledger://policy/approval-routes"},
			Prompts:   []string{"agent-ledger/workload-brief", "agent-ledger/cost-review", "agent-ledger/incident-evidence"},
		},
		{
			ID:          "protocol.runtime_status",
			Name:        "Runtime Status Probe",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "process-level mode only; does not expose paths, prompts, sessions, API credentials, or webhook URLs",
			Endpoints:   []string{"GET /api/runtime/status"},
			Commands:    []string{"agent-ledger runtime"},
			Tools:       []string{"ledger.runtime_status"},
			Resources:   []string{"agent-ledger://runtime/status"},
			DataClasses: []string{"runtime mode", "read-only status", "write-operation status", "background task status", "compatibility hashes", "disabled feature names"},
			Limitations: []string{"reports process-local mode only; it is not a cluster health endpoint"},
		},
		{
			ID:          "protocol.config_status",
			Name:        "Privacy-Safe Config Status",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "deployment configuration report with counts, booleans, risk checks, and remediation hints; raw paths, tokens, webhook URLs, machine names, authors, prompts, responses, and session ids are excluded",
			Endpoints:   []string{"GET /api/config/status"},
			Commands:    []string{"agent-ledger config status", "agent-ledger config status --format markdown"},
			Tools:       []string{"ledger.config_status"},
			Resources:   []string{"agent-ledger://config/status"},
			DataClasses: []string{"bind safety", "auth token presence", "collector path counts", "pricing mode", "outbound surface flags", "privacy settings", "configuration issues"},
		},
		{
			ID:          "protocol.readiness",
			Name:        "Control Plane Readiness",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "aggregated readiness checks and counts only; excludes raw paths, URLs, secrets, prompts, responses, sessions, projects, branches, machine names, and authors",
			Endpoints:   []string{"GET /api/readiness"},
			Commands:    []string{"agent-ledger readiness", "agent-ledger readiness --format markdown"},
			Tools:       []string{"ledger.readiness"},
			Resources:   []string{"agent-ledger://readiness"},
			DataClasses: []string{"database query status", "configuration issue counts", "contract verification result", "runtime mode", "ingestion health counts", "pricing source counts", "workload queue claimability counts", "workload lease pressure buckets", "agent run active/stale counts"},
		},
		{
			ID:          "protocol.admission_check",
			Name:        "Operation Admission Check",
			Category:    "control-plane",
			Protocol:    "HTTP JSON + CLI JSON + MCP resource/tool",
			Direction:   "query",
			Status:      "implemented",
			Maturity:    "stable-v1",
			Enabled:     true,
			Privacy:     "operation metadata, role requirements, read-only behavior, and remediation hints only; excludes request bodies, raw paths, prompts, secrets, sessions, projects, branches, machine names, and authors",
			Endpoints:   []string{"GET /api/admission/check"},
			Commands:    []string{"agent-ledger admission check", "agent-ledger admission check --surface http --method POST --path /api/events --role operator"},
			Tools:       []string{"ledger.admission_check"},
			Resources:   []string{"agent-ledger://admission/check"},
			DataClasses: []string{"operation surface", "method/path/tool/command summary", "role requirement", "read-only behavior", "write-mode metadata"},
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
			Limitations: []string{"derived snapshot feed; SSE stream is a local polling subscription", "MCP resource subscriptions support parameterized local polling URIs, not native host push transport"},
			NextMilestones: []string{
				"add native MCP subscription transport when host clients support it",
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
			Endpoints: []string{"GET /api/policies/status", "POST /api/policy/evaluate", "GET /api/policy/audit", "GET /api/policy/enforcement", "GET /api/policy/decisions", "GET/POST /api/policy/approvals", "GET /api/policy/approval-routes"},
			Commands:  []string{"agent-ledger policy evaluate --model gpt-5.5 --action model.call", "agent-ledger policy audit", "agent-ledger policy enforcement --privacy", "agent-ledger policy routes --due-within 24h --privacy", "MCP ledger.approval_routes", "MCP ledger.approvals", "MCP ledger.resolve_approval", "agent-ledger policy approvals --privacy", "agent-ledger policy resolve --id apr_... --status approved --voter alice --required-approvals 2"},
			DataClasses: []string{
				"policy rule metadata", "policy decisions", "approval requests", "approval routing metadata", "approval route summaries", "approval due/overdue evidence", "quorum approval votes", "redacted audit evidence",
			},
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
			Privacy:     "disabled by default; sends bounded redacted workload-event, approval, and approval-route summaries only",
			Endpoints:   []string{"POST /api/notifications/webhook"},
			Commands:    []string{"agent-ledger notify webhook --dry-run --approval-due-within 24h", "agent-ledger notify webhook --severity warning"},
			DataClasses: []string{"redacted workload ids", "phase", "severity", "next action", "risk metadata", "redacted approval ids", "hashed approval route metadata"},
			Limitations: []string{"requires explicit webhooks.enabled and webhooks.url", "does not send prompt content, local paths, project names, branch names, or webhook URL in audit logs"},
		},
		{
			ID:          "notification.desktop_adapter",
			Name:        "Desktop Notification Adapter Payload",
			Category:    "notification",
			Protocol:    "HTTP JSON + CLI JSON",
			Direction:   "local-adapter",
			Status:      "implemented",
			Maturity:    "local-preview",
			Enabled:     true,
			Privacy:     "read-only redacted payload for local desktop clients; no outbound delivery and no prompt content",
			Endpoints:   []string{"GET /api/notifications/desktop"},
			Commands:    []string{"agent-ledger notify desktop --severity warning --approval-due-within 24h"},
			DataClasses: []string{"redacted workload state", "severity", "phase", "pending approval counts", "approval route counts"},
			Limitations: []string{"adapter payload only; OS-specific toast rendering is delegated to a local desktop client"},
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
			ID:          "protocol.opentelemetry_genai",
			Name:        "OpenTelemetry GenAI Mapping",
			Category:    "protocol",
			Protocol:    "OpenTelemetry GenAI and OpenInference JSON spans",
			Direction:   "ingest",
			Status:      "implemented",
			Maturity:    "local-preview",
			Enabled:     true,
			Privacy:     "metadata-only mapping; prompt/completion message attributes are intentionally excluded",
			EventTypes:  eventTypes,
			Endpoints:   []string{"POST /api/otel/genai"},
			Commands:    []string{"agent-ledger otel convert --file spans.json", "agent-ledger otel ingest --file spans.json"},
			Limitations: []string{"local JSON mapper, not a full OTLP collector", "OpenInference llm.token_count fixtures are supported for conformance; prompt and completion message attributes are intentionally not persisted"},
		},
		{
			ID:          "protocol.a2a",
			Name:        "Agent-to-Agent Task Telemetry",
			Category:    "protocol",
			Protocol:    "A2A JSON task snapshots/events with delegated lineage, evidence references, and discovery metadata",
			Direction:   "ingest",
			Status:      "implemented",
			Maturity:    "local-preview",
			Enabled:     true,
			Privacy:     "metadata-only task mapping; message history, message parts, and artifact parts are intentionally excluded",
			EventTypes:  []string{"workload.started", "workload.linked", "agent.run.started", "agent.run.heartbeat", "agent.run.finished", "context.ref", "artifact.created", "evaluation.recorded", "policy.decision"},
			Endpoints:   []string{"GET /.well-known/agent-ledger.json", "GET /api/discovery", "POST /api/a2a/tasks"},
			Commands:    []string{"agent-ledger a2a convert --file task.json", "agent-ledger a2a ingest --file task.json"},
			DataClasses: []string{"task lifecycle metadata", "parent/child lineage metadata", "evidence reference hashes", "artifact metadata", "evaluation metadata", "policy metadata"},
			Limitations: []string{"local telemetry adapter, not a full A2A task execution server", "parent workload placeholders are reconstructed when delegated task metadata references a parent not present in the local ledger", "does not persist A2A message/history content"},
		},
		{
			ID:         "gateway.provider_api",
			Name:       "Provider Usage Envelope Adapter",
			Category:   "gateway",
			Protocol:   "OpenAI-compatible, Anthropic-style, LiteLLM-style, usageMetadata relay usage JSON, and request/response metadata wrappers with provider bill hooks",
			Direction:  "ingest",
			Status:     "implemented",
			Maturity:   "local-preview",
			Enabled:    true,
			Privacy:    "metadata-only usage mapping; request/response message content, headers, and secrets are intentionally excluded",
			EventTypes: []string{"model.call", "context.ref"},
			Endpoints:  []string{"POST /api/provider/calls"},
			Commands:   []string{"agent-ledger provider convert --file response.json", "agent-ledger provider ingest --file response.json"},
			Limitations: []string{
				"local usage envelope mapper, not a live reverse proxy",
				"does not perform request routing or API key handling",
				"budget advisories are local post-ingest signals and do not block provider imports",
				"provider reconciliation hooks store hashed bill/invoice references only",
			},
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
			ID:          "gateway.provider_live_proxy",
			Name:        "Live Provider Gateway Mode",
			Category:    "gateway",
			Protocol:    "OpenAI-compatible Chat Completions HTTP JSON/SSE, OpenAI Responses HTTP JSON/SSE, Anthropic Messages HTTP JSON/SSE proxy, generic relay SSE fixture conformance, and explicit budget fallback routing",
			Direction:   "observe-and-route",
			Status:      "experimental",
			Maturity:    "local-preview",
			Enabled:     opts.GatewayEnabled,
			Privacy:     "forwards prompt content only in-memory to the configured upstream; ledger writes usage metadata only; API credentials are read from env and not persisted",
			EventTypes:  []string{"model.call", "policy.decision", "context.ref"},
			Endpoints:   []string{"POST /gateway/openai/v1/chat/completions", "POST /gateway/openai/v1/responses", "POST /gateway/anthropic/v1/messages"},
			Limitations: []string{"disabled by default", "automatic fallback is disabled by default and only rewrites explicitly mapped models after configured budget severity is reached", "OpenAI Chat Completions streaming usage requires upstream final usage chunks; gateway requests them by default when compatible", "OpenAI Responses streaming records usage from response.completed events", "Anthropic Messages streaming records usage from message_start/message_delta events", "generic relay usageMetadata SSE is covered by adapter conformance fixtures before runtime proxy enablement", "does not persist prompt or response content"},
		},
		{
			ID:          "protocol.otlp_receiver",
			Name:        "OpenTelemetry OTLP Receiver",
			Category:    "protocol",
			Protocol:    "OTLP HTTP JSON/protobuf traces and optional loopback-only OTLP/gRPC TraceService",
			Direction:   "ingest",
			Status:      "experimental",
			Maturity:    "local-preview",
			Enabled:     opts.OTLPReceiverEnabled,
			Privacy:     "metadata-only span projection; prompt/message attributes are intentionally not persisted",
			EventTypes:  []string{"model.call", "context.ref"},
			Endpoints:   []string{"POST /v1/traces", "POST /api/otlp/v1/traces"},
			Limitations: []string{"disabled by default", "gRPC receiver requires integrations.otlp_receiver.enabled and integrations.otlp_receiver.grpc_enabled", "gRPC receiver refuses non-loopback bind addresses", "HTTP receiver accepts uncompressed or gzip JSON/protobuf batches and rejects over-limit compressed bodies, decoded bodies, or span batches explicitly with per-request backpressure headers/body fields/audit metadata", "collector exporter and Docker Compose smoke examples are provided under examples/otel-collector"},
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

// CatalogFingerprint returns a stable hash for the privacy-safe capability
// catalog so wrappers can cheaply detect tool, protocol, or runtime changes.
func CatalogFingerprint(opts Options) string {
	return CatalogFingerprintFrom(Registry(opts))
}

func CatalogFingerprintFrom(catalog Catalog) string {
	raw, err := json.Marshal(catalog)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
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
