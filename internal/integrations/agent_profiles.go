package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// AgentFrameworkProfileCatalog is a static ecosystem map for agent CLIs,
// wrappers, routers, framework runtimes, and observability bridges.
//
// It is intentionally metadata-only: it describes integration posture and
// expected signals, but never carries prompt content, response content, local paths,
// account identifiers, machine identifiers, or durable secrets.
type AgentFrameworkProfileCatalog struct {
	Product          string                  `json:"product"`
	Contract         string                  `json:"contract"`
	Version          string                  `json:"version"`
	GeneratedFrom    string                  `json:"generated_from"`
	LocalFirst       bool                    `json:"local_first"`
	ReadOnlySafe     bool                    `json:"read_only_safe"`
	WritesLocalState bool                    `json:"writes_local_state"`
	PrivacyPolicy    string                  `json:"privacy_policy"`
	Summary          AgentFrameworkSummary   `json:"summary"`
	Profiles         []AgentFrameworkProfile `json:"profiles"`
	QualityGates     []string                `json:"quality_gates"`
	RoutingGuidance  []string                `json:"routing_guidance"`
}

// AgentFrameworkSummary captures high-level agent ecosystem coverage counts.
type AgentFrameworkSummary struct {
	Profiles               int `json:"profiles"`
	LocalCollectors        int `json:"local_collectors"`
	CanonicalEventIngest   int `json:"canonical_event_ingest"`
	ProviderEnvelopeIngest int `json:"provider_envelope_ingest"`
	ProviderStreamIngest   int `json:"provider_stream_ingest"`
	MCPTooling             int `json:"mcp_tooling"`
	A2ATelemetry           int `json:"a2a_telemetry"`
	OTelTelemetry          int `json:"otel_telemetry"`
	WorkloadHeartbeats     int `json:"workload_heartbeats"`
	MultiAgentRouters      int `json:"multi_agent_routers"`
}

// AgentFrameworkProfile describes one agent/framework family without raw user
// data. It helps wrappers and CI decide which already-supported Agent Ledger
// surface to use for telemetry and cost attribution.
type AgentFrameworkProfile struct {
	ID                  string   `json:"id"`
	Label               string   `json:"label"`
	Kind                string   `json:"kind"`
	Families            []string `json:"families"`
	SupportedSurfaces   []string `json:"supported_surfaces"`
	RecommendedIngest   string   `json:"recommended_ingest"`
	FallbackIngest      []string `json:"fallback_ingest,omitempty"`
	AvailableSignals    []string `json:"available_signals"`
	CanonicalEventTypes []string `json:"canonical_event_types"`
	CollectorSources    []string `json:"collector_sources,omitempty"`
	AdapterInputKinds   []string `json:"adapter_input_kinds,omitempty"`
	MCPTools            []string `json:"mcp_tools,omitempty"`
	MCPResources        []string `json:"mcp_resources,omitempty"`
	HTTPEndpoints       []string `json:"http_endpoints,omitempty"`
	CLICommands         []string `json:"cli_commands,omitempty"`
	PrivacyNotes        []string `json:"privacy_notes"`
	QualityGates        []string `json:"quality_gates"`
	Limitations         []string `json:"limitations,omitempty"`
}

// AgentFrameworkProfiles returns the privacy-safe static profile catalog for
// current and future agent workload integrations.
func AgentFrameworkProfiles() AgentFrameworkProfileCatalog {
	commonLifecycleEvents := []string{
		"workload.started",
		"agent.run.started",
		"agent.run.heartbeat",
		"agent.run.finished",
		"model.call",
		"tool.call",
		"context.ref",
		"artifact.created",
		"evaluation.recorded",
	}
	profiles := []AgentFrameworkProfile{
		{
			ID:                "a2a-task-runtime",
			Label:             "A2A task telemetry runtime",
			Kind:              "agent-protocol-adapter",
			Families:          []string{"a2a", "task-runtime", "delegated-agent"},
			SupportedSurfaces: []string{"http-json", "adapter-conformance", "canonical-events"},
			RecommendedIngest: "POST /api/a2a/tasks for task snapshots or task events, followed by strict A2A conformance in wrapper CI",
			FallbackIngest:    []string{"agent-ledger a2a convert", "agent-ledger a2a ingest"},
			AvailableSignals:  []string{"task lifecycle", "delegated parent reference", "context reference", "artifact metadata", "evaluation metadata"},
			CanonicalEventTypes: []string{
				"workload.started", "workload.linked", "agent.run.started", "agent.run.heartbeat", "agent.run.finished", "context.ref", "artifact.created", "evaluation.recorded", "policy.decision",
			},
			AdapterInputKinds: []string{"a2a"},
			HTTPEndpoints:     []string{"POST /api/a2a/tasks", "POST /api/integrations/conformance?kind=a2a&strict=true"},
			CLICommands:       []string{"agent-ledger a2a convert", "agent-ledger a2a ingest", "agent-ledger adapter conformance --kind a2a --strict --file task.json"},
			PrivacyNotes:      []string{"message history and artifact part content are excluded", "evidence references should be stable hashes or opaque local handles", "task metadata may reconstruct lineage with confidence below 1 when parent context is incomplete"},
			QualityGates:      []string{"strict fixture must emit workload and run lifecycle events", "message and artifact body fields must be absent", "delegated lineage should use metadata identifiers or hashes"},
			Limitations:       []string{"Agent Ledger is a local telemetry adapter, not a full remote A2A execution server"},
		},
		{
			ID:                "agent-framework-runtime",
			Label:             "LangGraph, AutoGen, CrewAI, or custom multi-agent runtime",
			Kind:              "multi-agent-framework",
			Families:          []string{"langgraph", "autogen", "crewai", "custom-runtime", "multi-agent"},
			SupportedSurfaces: []string{"canonical-events", "mcp-stdio", "provider-envelope", "opentelemetry"},
			RecommendedIngest: "emit canonical workload/run/tool/model/evaluation events directly, or expose local MCP tools for routers that need admission checks",
			FallbackIngest:    []string{"provider usage envelopes for model calls", "OpenTelemetry GenAI spans for model call telemetry"},
			AvailableSignals:  []string{"workload graph", "agent run lifecycle", "tool call metadata", "model usage", "evaluation signal", "policy decision"},
			CanonicalEventTypes: []string{
				"workload.started", "workload.linked", "agent.run.started", "agent.run.heartbeat", "agent.run.finished", "model.call", "tool.call", "context.ref", "artifact.created", "evaluation.recorded", "policy.decision",
			},
			AdapterInputKinds: []string{"canonical", "provider", "provider-stream", "otel"},
			MCPTools:          []string{"ledger.start_workload", "ledger.start_run", "ledger.heartbeat_run", "ledger.record_tool_call", "ledger.record_context", "ledger.record_artifact", "ledger.record_evaluation", "ledger.get_policy"},
			MCPResources:      []string{"agent-ledger://workloads/queue", "agent-ledger://workloads/feed", "agent-ledger://readiness", "agent-ledger://admission/check"},
			HTTPEndpoints:     []string{"POST /api/events", "POST /api/events/validate", "POST /api/provider/calls", "POST /api/otel/genai"},
			CLICommands:       []string{"agent-ledger event validate", "agent-ledger event ingest", "agent-ledger provider convert", "agent-ledger otel convert"},
			PrivacyNotes:      []string{"framework memory, messages, prompts, responses, and artifact bodies must stay outside the ledger", "context and artifact references should be hashes or labels", "team, owner, and machine dimensions should be configured as privacy-safe aliases"},
			QualityGates:      []string{"canonical events must validate before ingest", "model calls must preserve non-overlapping token fields", "tool metadata must avoid command arguments that contain secrets"},
			Limitations:       []string{"framework-specific native fields are represented as metadata and confidence labels until a dedicated adapter is added"},
		},
		{
			ID:                  "ci-router",
			Label:               "CI, queue, or async router",
			Kind:                "agent-router",
			Families:            []string{"ci", "router", "queue", "async-agent", "goal-runner"},
			SupportedSurfaces:   []string{"workload-ledger", "leases", "mcp-stdio", "http-json", "admission"},
			RecommendedIngest:   "claim workload leases, append run heartbeats, and consult admission/policy before write operations",
			FallbackIngest:      []string{"read-only workload queue and liveness resources for observer deployments"},
			AvailableSignals:    []string{"claimable work", "lease pressure", "run liveness", "phase", "progress", "policy route", "readiness"},
			CanonicalEventTypes: []string{"workload.started", "agent.run.started", "agent.run.heartbeat", "agent.run.finished", "policy.decision"},
			MCPTools:            []string{"ledger.workload_queue", "ledger.claim_next_workload", "ledger.acquire_workload_lease", "ledger.renew_workload_lease", "ledger.release_workload_lease", "ledger.run_liveness", "ledger.admission_check"},
			MCPResources:        []string{"agent-ledger://workloads/queue", "agent-ledger://workloads/leases", "agent-ledger://agent-runs/liveness", "agent-ledger://policy/approval-routes"},
			HTTPEndpoints:       []string{"GET /api/workloads/queue", "POST /api/workloads/claim-next", "POST /api/workloads/lease", "POST /api/agent-runs/heartbeat", "GET /api/agent-runs/liveness", "GET /api/admission/check"},
			CLICommands:         []string{"agent-ledger workload queue", "agent-ledger workload claim-next", "agent-ledger workload lease list", "agent-ledger workload heartbeat", "agent-ledger readiness", "agent-ledger admission check"},
			PrivacyNotes:        []string{"lease tokens are returned only on acquire and stored as hashes", "queue and liveness resources redact workload and run identifiers where needed", "operator actions are auditable without recording prompts"},
			QualityGates:        []string{"routers should call admission before mutating state", "heartbeat writes require stable run identifiers and explicit status", "read-only deployments must use queue probes instead of claim calls"},
		},
		{
			ID:                  "claude-code",
			Label:               "Claude Code local CLI",
			Kind:                "ai-coding-cli",
			Families:            []string{"claude", "anthropic", "ai-coding-cli"},
			SupportedSurfaces:   []string{"local-collector", "canonical-events", "provider-envelope", "mcp-stdio"},
			RecommendedIngest:   "local collector for existing session files, plus canonical events or provider envelopes from wrappers for future run-level telemetry",
			FallbackIngest:      []string{"provider envelope ingest when routed through a local gateway", "OpenTelemetry GenAI spans when instrumented"},
			AvailableSignals:    []string{"usage rows", "prompt counts", "model name", "cache read/write tokens", "project attribution", "branch attribution"},
			CanonicalEventTypes: commonLifecycleEvents,
			CollectorSources:    []string{"claude"},
			AdapterInputKinds:   []string{"canonical", "provider", "provider-stream", "otel"},
			HTTPEndpoints:       []string{"POST /api/events", "POST /api/provider/calls", "POST /api/otel/genai"},
			CLICommands:         []string{"agent-ledger event ingest", "agent-ledger provider ingest", "agent-ledger otel ingest"},
			PrivacyNotes:        []string{"collector reads local metadata and usage rows only", "prompt and response content are never persisted", "cache diagnostics use token counters rather than message content"},
			QualityGates:        []string{"cache tokens must remain non-overlapping with input tokens", "damaged local rows should be visible in ingestion health", "wrapper events should include source and parser version"},
		},
		{
			ID:                  "codex-cli",
			Label:               "OpenAI Codex local CLI",
			Kind:                "ai-coding-cli",
			Families:            []string{"codex", "openai", "ai-coding-cli"},
			SupportedSurfaces:   []string{"local-collector", "canonical-events", "provider-envelope", "provider-stream", "mcp-stdio"},
			RecommendedIngest:   "local collector for Codex session logs, plus canonical workload/run heartbeats from wrappers when asynchronous goal execution is used",
			FallbackIngest:      []string{"provider envelope ingest when model calls are routed through a local gateway", "A2A task telemetry for delegated runs"},
			AvailableSignals:    []string{"usage rows", "model name", "project attribution", "branch attribution", "run liveness", "provider cost evidence"},
			CanonicalEventTypes: commonLifecycleEvents,
			CollectorSources:    []string{"codex"},
			AdapterInputKinds:   []string{"canonical", "provider", "provider-stream", "a2a", "otel"},
			MCPTools:            []string{"ledger.start_workload", "ledger.start_run", "ledger.heartbeat_run", "ledger.record_tool_call", "ledger.admission_check"},
			MCPResources:        []string{"agent-ledger://workloads/feed", "agent-ledger://admission/check", "agent-ledger://readiness"},
			HTTPEndpoints:       []string{"POST /api/events", "POST /api/provider/calls", "POST /api/a2a/tasks", "POST /api/otel/genai"},
			CLICommands:         []string{"agent-ledger event ingest", "agent-ledger provider ingest", "agent-ledger a2a ingest", "agent-ledger workload heartbeat"},
			PrivacyNotes:        []string{"absolute paths should be reduced to configured project aliases or hashes before sharing", "goal and context metadata should avoid prompt content", "subscription quota estimates are shown separately from API-style provider costs"},
			QualityGates:        []string{"timestamp windows must be half-open", "session identity must be source-scoped", "provider prices must expose source and freshness before recalculation"},
		},
		{
			ID:                  "kiro-cli",
			Label:               "Kiro CLI",
			Kind:                "ai-coding-cli",
			Families:            []string{"kiro", "ai-coding-cli"},
			SupportedSurfaces:   []string{"local-collector", "canonical-events"},
			RecommendedIngest:   "local collector for known local database and session metadata, with estimated token confidence surfaced explicitly",
			FallbackIngest:      []string{"canonical event ingest from wrappers when native token usage becomes available"},
			AvailableSignals:    []string{"conversation metadata", "project attribution", "estimated token usage", "model hints", "prompt counts"},
			CanonicalEventTypes: []string{"workload.started", "agent.run.started", "model.call", "context.ref"},
			CollectorSources:    []string{"kiro"},
			AdapterInputKinds:   []string{"canonical"},
			CLICommands:         []string{"agent-ledger event validate", "agent-ledger event ingest"},
			PrivacyNotes:        []string{"token estimates must be labeled and should not be mixed with exact usage without confidence", "local database paths are not exposed in profile metadata", "raw conversation bodies are excluded"},
			QualityGates:        []string{"estimated rows must carry confidence below exact collectors", "damaged database or JSON rows should remain visible in data quality", "future native usage fields should replace heuristics through a dedicated parser version"},
			Limitations:         []string{"exact tokens depend on native usage availability"},
		},
		{
			ID:                  "mcp-wrapper",
			Label:               "MCP wrapper or local agent tool surface",
			Kind:                "agent-tool-wrapper",
			Families:            []string{"mcp", "tool-wrapper", "local-agent"},
			SupportedSurfaces:   []string{"mcp-stdio", "canonical-events", "admission", "resources"},
			RecommendedIngest:   "use MCP read-only resources for context and MCP write tools only after admission allows the operation",
			FallbackIngest:      []string{"HTTP canonical event endpoints for non-MCP runtimes"},
			AvailableSignals:    []string{"budget state", "admission result", "workload lifecycle", "tool metadata", "context metadata", "artifact hash", "evaluation signal"},
			CanonicalEventTypes: commonLifecycleEvents,
			MCPTools:            []string{"ledger.discovery", "ledger.contracts", "ledger.agent_profiles", "ledger.admission_check", "ledger.start_workload", "ledger.record_tool_call", "ledger.record_context", "ledger.record_artifact", "ledger.record_evaluation"},
			MCPResources:        []string{"agent-ledger://discovery/manifest", "agent-ledger://integrations/agent-profiles", "agent-ledger://contracts/bundle", "agent-ledger://admission/check"},
			CLICommands:         []string{"agent-ledger mcp", "agent-ledger admission check", "agent-ledger agent profiles"},
			PrivacyNotes:        []string{"MCP resources are local stdio payloads and do not contact remote hosts by themselves", "tool inputs and command parameters should be represented by hashes or classes", "routers should use readOnlyHint and Agent Ledger access metadata"},
			QualityGates:        []string{"write tools must be blocked in read-only mode", "conditional policy tools must disclose when they record decisions", "resources should support stable cursors for monitor loops"},
		},
		{
			ID:                  "opencode",
			Label:               "OpenCode local CLI",
			Kind:                "ai-coding-cli",
			Families:            []string{"opencode", "ai-coding-cli"},
			SupportedSurfaces:   []string{"local-collector", "canonical-events", "provider-envelope"},
			RecommendedIngest:   "local collector for OpenCode local state, with prompt events de-duplicated by source and timestamp evidence",
			FallbackIngest:      []string{"provider usage envelope ingest when routed through OpenAI-compatible relays"},
			AvailableSignals:    []string{"usage rows", "prompt counts", "model name", "project attribution", "source-reported cost evidence"},
			CanonicalEventTypes: []string{"workload.started", "agent.run.started", "model.call", "context.ref"},
			CollectorSources:    []string{"opencode"},
			AdapterInputKinds:   []string{"canonical", "provider"},
			CLICommands:         []string{"agent-ledger event ingest", "agent-ledger provider ingest"},
			PrivacyNotes:        []string{"source-reported cost is retained as evidence but pricing governance remains authoritative for recalculation", "prompt counters are aggregated from de-duplicated prompt events", "raw prompt and response data stay out of the ledger"},
			QualityGates:        []string{"prompt counts must not be repeatedly accumulated during incremental scans", "source-scoped identity prevents collisions with other tools", "unpriced models should be surfaced in data quality"},
		},
		{
			ID:                  "openclaw",
			Label:               "OpenClaw local CLI",
			Kind:                "ai-coding-cli",
			Families:            []string{"openclaw", "ai-coding-cli"},
			SupportedSurfaces:   []string{"local-collector", "canonical-events"},
			RecommendedIngest:   "local collector for OpenClaw session metadata and model-call records",
			FallbackIngest:      []string{"canonical events from wrappers"},
			AvailableSignals:    []string{"usage rows", "model switch metadata", "project attribution", "branch attribution"},
			CanonicalEventTypes: []string{"workload.started", "agent.run.started", "model.call", "context.ref"},
			CollectorSources:    []string{"openclaw"},
			AdapterInputKinds:   []string{"canonical"},
			CLICommands:         []string{"agent-ledger event validate", "agent-ledger event ingest"},
			PrivacyNotes:        []string{"profile exposes source names, not native local directories", "model-switch attribution is metadata-only", "raw session transcript content is excluded"},
			QualityGates:        []string{"parser state should survive incremental scan offsets", "model changes should not split unrelated source-scoped sessions", "branch normalization should preserve detached or unknown state explicitly"},
		},
		{
			ID:                  "otel-genai-instrumented-app",
			Label:               "OpenTelemetry GenAI instrumented app",
			Kind:                "observability-bridge",
			Families:            []string{"opentelemetry", "otel-genai", "otlp", "observability"},
			SupportedSurfaces:   []string{"http-json", "otlp-http", "otlp-grpc", "adapter-conformance"},
			RecommendedIngest:   "POST /api/otel/genai for local JSON spans, or enable the local OTLP receiver after loopback and limit checks",
			FallbackIngest:      []string{"agent-ledger otel convert", "agent-ledger otel ingest"},
			AvailableSignals:    []string{"span timing", "model name", "usage counters", "service metadata", "trace correlation"},
			CanonicalEventTypes: []string{"model.call", "context.ref"},
			AdapterInputKinds:   []string{"otel"},
			HTTPEndpoints:       []string{"POST /api/otel/genai", "POST /api/otlp/v1/traces", "POST /v1/traces", "POST /api/integrations/conformance?kind=otel&strict=true"},
			CLICommands:         []string{"agent-ledger otel convert", "agent-ledger otel ingest", "agent-ledger adapter conformance --kind otel --strict --file spans.json"},
			PrivacyNotes:        []string{"prompt and completion message attributes are rejected or ignored", "trace identifiers should be treated as correlation metadata", "OTLP receivers are disabled by default and should stay loopback-bound"},
			QualityGates:        []string{"body and span counts must be bounded", "compressed payloads must be decoded under configured limits", "span attributes containing content must not be persisted"},
			Limitations:         []string{"native framework coverage depends on instrumentation quality"},
		},
		{
			ID:                  "pi-agent",
			Label:               "Pi Agent local CLI",
			Kind:                "ai-coding-cli",
			Families:            []string{"pi-agent", "openclaw-compatible", "ai-coding-cli"},
			SupportedSurfaces:   []string{"local-collector", "canonical-events"},
			RecommendedIngest:   "local collector for Pi session metadata, sharing the OpenClaw-style parser family where compatible",
			FallbackIngest:      []string{"canonical events from wrappers"},
			AvailableSignals:    []string{"usage rows", "model switch metadata", "project attribution", "workspace slug"},
			CanonicalEventTypes: []string{"workload.started", "agent.run.started", "model.call", "context.ref"},
			CollectorSources:    []string{"pi"},
			AdapterInputKinds:   []string{"canonical"},
			CLICommands:         []string{"agent-ledger event validate", "agent-ledger event ingest"},
			PrivacyNotes:        []string{"workspace slugs should be treated as aliases and can be hidden by privacy mode", "raw transcripts are excluded", "native local directories are not exposed"},
			QualityGates:        []string{"model change records should be preserved", "project fallback should be explicit", "parser compatibility with OpenClaw should be tested by fixtures"},
		},
		{
			ID:                  "provider-gateway-wrapper",
			Label:               "Provider gateway or relay wrapper",
			Kind:                "provider-gateway-wrapper",
			Families:            []string{"openai-compatible", "anthropic-compatible", "relay", "gateway", "model-router"},
			SupportedSurfaces:   []string{"provider-envelope", "provider-stream", "gateway", "pricing-governance", "reconciliation"},
			RecommendedIngest:   "emit provider envelopes or provider streams with explicit usage metadata, then recalculate through pricing governance",
			FallbackIngest:      []string{"source-reported cost as evidence when exact token prices are unavailable", "provider reconciliation import for invoice comparison"},
			AvailableSignals:    []string{"model name", "usage counters", "cache counters", "source cost evidence", "routing label", "pricing source"},
			CanonicalEventTypes: []string{"model.call", "context.ref"},
			AdapterInputKinds:   []string{"provider", "provider-stream"},
			HTTPEndpoints:       []string{"POST /api/provider/calls", "POST /api/integrations/conformance?kind=provider&strict=true", "POST /gateway/openai/v1/chat/completions", "POST /gateway/anthropic/v1/messages"},
			CLICommands:         []string{"agent-ledger provider convert", "agent-ledger provider ingest", "agent-ledger pricing sync", "agent-ledger projection quality"},
			PrivacyNotes:        []string{"request and response bodies are not persisted", "provider account identifiers should be hashed before persistence", "outbound notification and upstream secrets must stay in environment variables"},
			QualityGates:        []string{"local override prices must outrank official and fallback prices", "source-reported cost must not silently override governance recalculation", "streaming usage must be bounded and conformance-tested"},
		},
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	catalog := AgentFrameworkProfileCatalog{
		Product:          "Agent Ledger",
		Contract:         "agent-ledger.agent-framework-profile-catalog",
		Version:          "v1",
		GeneratedFrom:    "static privacy-safe agent/framework integration profiles",
		LocalFirst:       true,
		ReadOnlySafe:     true,
		WritesLocalState: false,
		PrivacyPolicy:    "Agent framework profiles are static metadata. They contain no raw local paths, API credentials, auth tokens, prompts, responses, message histories, artifact bodies, account ids, machine names, authors, or usage rows.",
		Profiles:         profiles,
		QualityGates: []string{
			"new framework profiles must map to at least one implemented read-only discovery, validation, or ingest surface",
			"profiles must identify whether usage is exact, estimated, source-reported, or reconstructed through existing confidence fields",
			"wrappers should validate canonical/provider/OTel/A2A fixtures before enabling write ingest",
			"workload routers must call admission and respect read-only metadata before invoking write tools",
			"prompt, response, message history, artifact body, raw local path, secret, and durable account identifiers must remain outside profile metadata",
		},
		RoutingGuidance: []string{
			"use local collectors for already-supported CLIs when exact native usage is present",
			"use canonical events for future agent frameworks that can emit workload, run, tool, context, artifact, evaluation, and model-call metadata",
			"use provider envelopes or streams when the integration point is a model gateway, relay, or local OpenAI-compatible runtime",
			"use OpenTelemetry GenAI when a framework already emits bounded GenAI spans and does not require prompt persistence",
			"use A2A task telemetry for delegated task snapshots and parent/child lineage, while keeping Agent Ledger as a telemetry adapter rather than a remote execution server",
			"use MCP read-only resources for router context and MCP write tools only after admission confirms the current role and runtime mode",
		},
	}
	catalog.Summary = summarizeAgentFrameworkProfiles(catalog.Profiles)
	return catalog
}

// AgentFrameworkProfilesFingerprint returns a stable hash for cache validators,
// discovery manifests, contract bundles, and wrapper CI.
func AgentFrameworkProfilesFingerprint() string {
	raw, err := json.Marshal(AgentFrameworkProfiles())
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func summarizeAgentFrameworkProfiles(profiles []AgentFrameworkProfile) AgentFrameworkSummary {
	var out AgentFrameworkSummary
	out.Profiles = len(profiles)
	for _, profile := range profiles {
		if len(profile.CollectorSources) > 0 {
			out.LocalCollectors++
		}
		if containsString(profile.AdapterInputKinds, "canonical") || containsString(profile.SupportedSurfaces, "canonical-events") {
			out.CanonicalEventIngest++
		}
		if containsString(profile.AdapterInputKinds, "provider") || containsString(profile.SupportedSurfaces, "provider-envelope") {
			out.ProviderEnvelopeIngest++
		}
		if containsString(profile.AdapterInputKinds, "provider-stream") || containsString(profile.SupportedSurfaces, "provider-stream") {
			out.ProviderStreamIngest++
		}
		if len(profile.MCPTools) > 0 || containsString(profile.SupportedSurfaces, "mcp-stdio") {
			out.MCPTooling++
		}
		if containsString(profile.AdapterInputKinds, "a2a") || containsString(profile.Families, "a2a") {
			out.A2ATelemetry++
		}
		if containsString(profile.AdapterInputKinds, "otel") || containsString(profile.Families, "opentelemetry") || containsString(profile.SupportedSurfaces, "opentelemetry") {
			out.OTelTelemetry++
		}
		if containsString(profile.CanonicalEventTypes, "agent.run.heartbeat") || containsString(profile.SupportedSurfaces, "workload-ledger") {
			out.WorkloadHeartbeats++
		}
		if profile.Kind == "multi-agent-framework" || profile.Kind == "agent-router" || containsString(profile.Families, "multi-agent") {
			out.MultiAgentRouters++
		}
	}
	return out
}
