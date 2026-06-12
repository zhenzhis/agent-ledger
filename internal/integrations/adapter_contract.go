package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// AdapterContract is the machine-readable contract for building Agent Ledger
// adapters without relying on README prose or product-specific assumptions.
type AdapterContract struct {
	Product              string                           `json:"product"`
	Contract             string                           `json:"contract"`
	Version              string                           `json:"version"`
	Purpose              string                           `json:"purpose"`
	SchemaVersion        string                           `json:"schema_version"`
	SchemaHash           string                           `json:"schema_hash"`
	PrivacyPolicy        string                           `json:"privacy_policy"`
	SupportedInputKinds  []AdapterInputKind               `json:"supported_input_kinds"`
	CanonicalEventTypes  []storage.CanonicalEventTypeInfo `json:"canonical_event_types"`
	RequiredEnvelope     []string                         `json:"required_envelope"`
	RecommendedEnvelope  []string                         `json:"recommended_envelope"`
	ForbiddenPayloadKeys []string                         `json:"forbidden_payload_keys"`
	TokenSemantics       []string                         `json:"token_semantics"`
	QualityGates         []string                         `json:"quality_gates"`
	Validation           AdapterValidationContract        `json:"validation"`
	Ingest               AdapterIngestContract            `json:"ingest"`
	RoadmapCompatibility []string                         `json:"roadmap_compatibility"`
}

// AdapterInputKind describes one currently accepted adapter fixture family.
type AdapterInputKind struct {
	Kind            string   `json:"kind"`
	Description     string   `json:"description"`
	ConformanceKind string   `json:"conformance_kind"`
	ConvertCommand  string   `json:"convert_command,omitempty"`
	IngestCommand   string   `json:"ingest_command,omitempty"`
	Endpoint        string   `json:"endpoint,omitempty"`
	RequiredSignals []string `json:"required_signals"`
	PrivacyNotes    []string `json:"privacy_notes"`
}

// AdapterValidationContract describes dry-run validation entrypoints.
type AdapterValidationContract struct {
	HTTP     string   `json:"http"`
	CLI      string   `json:"cli"`
	MCPTool  string   `json:"mcp_tool"`
	StrictCI []string `json:"strict_ci"`
}

// AdapterIngestContract describes local ingest entrypoints.
type AdapterIngestContract struct {
	HTTP     []string `json:"http"`
	CLI      []string `json:"cli"`
	MCPTools []string `json:"mcp_tools"`
}

// AdapterContractSpec returns a stable, privacy-first adapter contract for
// current and future agent ecosystem integrations.
func AdapterContractSpec() AdapterContract {
	return AdapterContract{
		Product:       "Agent Ledger",
		Contract:      "agent-ledger.adapter-contract",
		Version:       "v1",
		Purpose:       "Build metadata-only adapters for agent CLIs, agent frameworks, provider gateways, OpenTelemetry GenAI spans, A2A task telemetry, and future multi-agent routing protocols.",
		SchemaVersion: storage.CanonicalEventSchemaVersion,
		SchemaHash:    storage.CanonicalEventSchemaFingerprint(),
		PrivacyPolicy: "Adapters must never persist prompt text, model output text, secrets, raw tool parameters, or raw transcripts. Use hashes, ids, offsets, and short metadata labels.",
		SupportedInputKinds: []AdapterInputKind{
			{
				Kind:            "canonical",
				Description:     "Native Agent Ledger canonical event envelope.",
				ConformanceKind: "canonical",
				ConvertCommand:  "agent-ledger event validate --file event.json",
				IngestCommand:   "agent-ledger event ingest --file event.json",
				Endpoint:        "POST /api/events/validate or POST /api/events",
				RequiredSignals: []string{"source", "event_type", "payload", "timestamp or generated UTC time"},
				PrivacyNotes:    []string{"payload must be a JSON object", "prompt/content/message-like keys are rejected"},
			},
			{
				Kind:            "provider",
				Description:     "Provider usage envelopes from OpenAI-compatible, Anthropic-compatible, LiteLLM, gateway, or billing-export sources.",
				ConformanceKind: "provider",
				ConvertCommand:  "agent-ledger provider convert --file response.json",
				IngestCommand:   "agent-ledger provider ingest --file response.json",
				Endpoint:        "POST /api/provider/calls",
				RequiredSignals: []string{"provider or source", "model", "usage tokens", "timestamp or source event time"},
				PrivacyNotes:    []string{"drop request/response bodies before conversion", "prefer provider request ids or hashes as raw_ref"},
			},
			{
				Kind:            "provider-stream",
				Description:     "Provider SSE usage transcripts from OpenAI-compatible Chat Completions, OpenAI Responses, or Anthropic Messages streaming responses.",
				ConformanceKind: "provider-stream",
				ConvertCommand:  "agent-ledger adapter conformance --kind provider-stream --file stream.sse",
				Endpoint:        "POST /api/integrations/conformance?kind=provider-stream",
				RequiredSignals: []string{"SSE data events", "provider response id when available", "model", "usage tokens"},
				PrivacyNotes:    []string{"stream deltas are ignored", "fixtures should include usage events only", "do not store prompt, output, or transcript text"},
			},
			{
				Kind:            "otel",
				Description:     "OpenTelemetry GenAI JSON spans or local OTLP HTTP JSON/protobuf traces.",
				ConformanceKind: "otel",
				ConvertCommand:  "agent-ledger otel convert --file spans.json",
				IngestCommand:   "agent-ledger otel ingest --file spans.json",
				Endpoint:        "POST /api/otel/genai or POST /api/otlp/v1/traces",
				RequiredSignals: []string{"trace/span id", "provider/model attributes", "usage token attributes", "start/end time"},
				PrivacyNotes:    []string{"message and prompt attributes are intentionally ignored", "span ids are acceptable raw_ref values", "OTLP protobuf is decoded into the same metadata-only span model as OTLP JSON"},
			},
			{
				Kind:            "a2a",
				Description:     "Agent-to-Agent task snapshots or events for multi-agent workload lineage.",
				ConformanceKind: "a2a",
				ConvertCommand:  "agent-ledger a2a convert --file task.json",
				IngestCommand:   "agent-ledger a2a ingest --file task.json",
				Endpoint:        "POST /api/a2a/tasks",
				RequiredSignals: []string{"task id", "state/status", "goal or project metadata", "agent/run metadata when available"},
				PrivacyNotes:    []string{"store task metadata and artifact hashes only", "do not store task messages or transcripts"},
			},
		},
		CanonicalEventTypes: storage.CanonicalEventTypes(),
		RequiredEnvelope: []string{
			"source", "event_type", "payload",
		},
		RecommendedEnvelope: []string{
			"event_id", "schema_version", "source_version", "parser_version", "source_event_id", "raw_ref", "match_type", "workload_id", "agent_run_id", "session_id", "model", "project", "git_branch", "timestamp", "confidence",
		},
		ForbiddenPayloadKeys: []string{"prompt", "prompts", "messages", "content", "input_text", "output_text", "completion", "transcript"},
		TokenSemantics: []string{
			"input_tokens is non-cached input only",
			"cache_read_input_tokens is cached input read from provider cache",
			"cache_creation_input_tokens is cached input written to provider cache",
			"output_tokens is total output; reasoning_output_tokens is informational subset only",
			"total_tokens = input_tokens + cache_read_input_tokens + cache_creation_input_tokens + output_tokens",
		},
		QualityGates: []string{
			"adapter conformance must pass before ingest is enabled",
			"strict mode must pass in CI for shipped fixtures",
			"timestamps must be RFC3339 or recoverable from upstream metadata",
			"model names should preserve provider spelling and may add model_alias",
			"cost_usd may be source-reported but local pricing governance remains authoritative for recomputation",
			"confidence below 1 must explain estimation or fuzzy matching through match_type or payload metadata",
		},
		Validation: AdapterValidationContract{
			HTTP:    "POST /api/integrations/conformance?kind=auto|canonical|provider|provider-stream|otel|a2a&strict=true",
			CLI:     "agent-ledger adapter conformance --kind auto --strict --file fixture.json",
			MCPTool: "ledger.adapter_conformance",
			StrictCI: []string{
				"agent-ledger adapter conformance --kind canonical --strict --file examples/adapter-fixtures/canonical-workload.json",
				"agent-ledger adapter conformance --kind provider --strict --file examples/adapter-fixtures/provider-openai-response.json",
				"agent-ledger adapter conformance --kind provider-stream --strict --file examples/adapter-fixtures/provider-anthropic-message-stream.sse",
				"agent-ledger adapter conformance --kind otel --strict --file examples/adapter-fixtures/otel-genai-span.json",
				"agent-ledger adapter conformance --kind a2a --strict --file examples/adapter-fixtures/a2a-task.json",
			},
		},
		Ingest: AdapterIngestContract{
			HTTP: []string{
				"POST /api/events",
				"POST /api/provider/calls",
				"POST /api/otel/genai",
				"POST /api/otlp/v1/traces",
				"POST /api/a2a/tasks",
			},
			CLI: []string{
				"agent-ledger event ingest --file event.json",
				"agent-ledger provider ingest --file response.json",
				"agent-ledger otel ingest --file spans.json",
				"agent-ledger a2a ingest --file task.json",
			},
			MCPTools: []string{"ledger.record_event", "ledger.start_workload", "ledger.start_run", "ledger.acquire_workload_lease", "ledger.renew_workload_lease", "ledger.release_workload_lease", "ledger.heartbeat_run", "ledger.record_tool_call", "ledger.record_context", "ledger.record_artifact", "ledger.record_evaluation"},
		},
		RoadmapCompatibility: []string{
			"new collectors should emit canonical events before product-specific analytics",
			"future provider-native gateways should preserve raw provider ids as raw_ref and avoid request bodies",
			"future multi-agent routing protocols should map task/run lineage to workload.linked and parent_run_id",
			"future team/server modes should keep the same metadata-only event envelope and add transport auth outside payload",
		},
	}
}

// AdapterContractFingerprint returns a stable hash for lightweight wrappers
// that need to cache or compare the machine-readable adapter contract.
func AdapterContractFingerprint() string {
	raw, err := json.Marshal(AdapterContractSpec())
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
