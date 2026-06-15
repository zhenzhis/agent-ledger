package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// AdapterConformanceMatrix is a static, privacy-safe index of the adapter
// fixture families Agent Ledger can validate before ingest is enabled.
type AdapterConformanceMatrix struct {
	Product              string                          `json:"product"`
	Contract             string                          `json:"contract"`
	Version              string                          `json:"version"`
	LocalFirst           bool                            `json:"local_first"`
	ReadOnlySafe         bool                            `json:"read_only_safe"`
	WritesLocalState     bool                            `json:"writes_local_state"`
	SchemaVersion        string                          `json:"schema_version"`
	SchemaHash           string                          `json:"schema_hash"`
	AdapterSpecHash      string                          `json:"adapter_spec_hash"`
	ProviderProfilesHash string                          `json:"provider_profiles_hash"`
	PrivacyPolicy        string                          `json:"privacy_policy"`
	Summary              AdapterConformanceMatrixSummary `json:"summary"`
	Kinds                []AdapterConformanceMatrixKind  `json:"kinds"`
	QualityGates         []string                        `json:"quality_gates"`
	RoutingGuidance      []string                        `json:"routing_guidance"`
}

// AdapterConformanceMatrixSummary contains stable counts for wrappers and CI.
type AdapterConformanceMatrixSummary struct {
	InputKinds             int `json:"input_kinds"`
	Fixtures               int `json:"fixtures"`
	StrictFixtures         int `json:"strict_fixtures"`
	ProviderFixtures       int `json:"provider_fixtures"`
	ProviderStreamFixtures int `json:"provider_stream_fixtures"`
	OTelFixtures           int `json:"otel_fixtures"`
	A2AFixtures            int `json:"a2a_fixtures"`
	CanonicalFixtures      int `json:"canonical_fixtures"`
}

// AdapterConformanceMatrixKind describes one accepted adapter input family.
type AdapterConformanceMatrixKind struct {
	Kind               string                      `json:"kind"`
	ConformanceKind    string                      `json:"conformance_kind"`
	Description        string                      `json:"description"`
	Status             string                      `json:"status"`
	Maturity           string                      `json:"maturity"`
	Endpoint           string                      `json:"endpoint"`
	CLICommand         string                      `json:"cli_command"`
	MCPTool            string                      `json:"mcp_tool"`
	StrictCICommand    string                      `json:"strict_ci_command"`
	ConvertCommand     string                      `json:"convert_command,omitempty"`
	IngestCommand      string                      `json:"ingest_command,omitempty"`
	AcceptedFormats    []string                    `json:"accepted_formats"`
	RequiredSignals    []string                    `json:"required_signals"`
	PrivacyNotes       []string                    `json:"privacy_notes"`
	ExpectedEventTypes []string                    `json:"expected_event_types"`
	Fixtures           []AdapterConformanceFixture `json:"fixtures"`
}

// AdapterConformanceFixture is one privacy-safe fixture path and its expected
// canonical event family. Paths are repository-relative examples only.
type AdapterConformanceFixture struct {
	Path               string   `json:"path"`
	Format             string   `json:"format"`
	Scenario           string   `json:"scenario"`
	Strict             bool     `json:"strict"`
	Command            string   `json:"command"`
	ProviderProfileIDs []string `json:"provider_profile_ids,omitempty"`
	ExpectedEventTypes []string `json:"expected_event_types"`
	Privacy            string   `json:"privacy"`
}

// AdapterConformanceMatrixSpec returns a stable adapter conformance capability
// matrix for CI, wrappers, routers, and future agent framework integrations.
func AdapterConformanceMatrixSpec() AdapterConformanceMatrix {
	spec := AdapterContractSpec()
	kinds := []AdapterConformanceMatrixKind{
		matrixKind(spec, "canonical", []string{"json"}, []string{"workload.started"}, []AdapterConformanceFixture{
			matrixFixture("canonical", "examples/adapter-fixtures/canonical-workload.json", "json", "native canonical workload event", []string{"workload.started"}, nil),
		}),
		matrixKind(spec, "provider", []string{"json"}, []string{"model.call", "context.ref"}, []AdapterConformanceFixture{
			matrixFixture("provider", "examples/adapter-fixtures/provider-openai-response.json", "json", "OpenAI Responses usage envelope", []string{"model.call"}, []string{"openai-official"}),
			matrixFixture("provider", "examples/adapter-fixtures/provider-openai-chat-completion.json", "json", "OpenAI-compatible Chat Completions usage envelope", []string{"model.call"}, []string{"openai-official", "openrouter-relay", "litellm-proxy"}),
			matrixFixture("provider", "examples/adapter-fixtures/provider-anthropic-message.json", "json", "Anthropic Messages usage envelope", []string{"model.call"}, []string{"anthropic-official"}),
		}),
		matrixKind(spec, "provider-stream", []string{"sse", "text/event-stream"}, []string{"model.call"}, []AdapterConformanceFixture{
			matrixFixture("provider-stream", "examples/adapter-fixtures/provider-openai-chat-stream.sse", "sse", "OpenAI-compatible Chat Completions final usage chunk", []string{"model.call"}, []string{"openai-official", "openrouter-relay", "litellm-proxy"}),
			matrixFixture("provider-stream", "examples/adapter-fixtures/provider-openai-responses-stream.sse", "sse", "OpenAI Responses response.completed usage event", []string{"model.call"}, []string{"openai-official"}),
			matrixFixture("provider-stream", "examples/adapter-fixtures/provider-anthropic-message-stream.sse", "sse", "Anthropic Messages usage events", []string{"model.call"}, []string{"anthropic-official"}),
			matrixFixture("provider-stream", "examples/adapter-fixtures/provider-generic-usage-metadata-stream.sse", "sse", "generic relay usageMetadata stream", []string{"model.call"}, []string{"google-gemini", "openrouter-relay", "litellm-proxy"}),
		}),
		matrixKind(spec, "otel", []string{"json", "otlp-json", "otlp-protobuf"}, []string{"model.call", "context.ref"}, []AdapterConformanceFixture{
			matrixFixture("otel", "examples/adapter-fixtures/otel-genai-span.json", "json", "OpenTelemetry GenAI span", []string{"model.call", "context.ref"}, nil),
			matrixFixture("otel", "examples/adapter-fixtures/otel-openinference-span.json", "json", "OpenInference token-count span", []string{"model.call", "context.ref"}, nil),
			matrixFixture("otel", "examples/adapter-fixtures/otlp-resource-spans.json", "json", "OTLP resourceSpans trace batch", []string{"model.call", "context.ref"}, nil),
		}),
		matrixKind(spec, "a2a", []string{"json"}, []string{"workload.started", "workload.linked", "agent.run.started", "agent.run.heartbeat", "agent.run.finished", "context.ref", "artifact.created", "evaluation.recorded", "policy.decision"}, []AdapterConformanceFixture{
			matrixFixture("a2a", "examples/adapter-fixtures/a2a-task.json", "json", "single A2A task snapshot", []string{"workload.started", "agent.run.started", "context.ref", "artifact.created", "agent.run.finished", "workload.closed", "evaluation.recorded"}, nil),
			matrixFixture("a2a", "examples/adapter-fixtures/a2a-delegated-task.json", "json", "delegated A2A task with lineage and evidence references", []string{"workload.started", "agent.run.started", "workload.linked", "context.ref"}, nil),
		}),
	}
	matrix := AdapterConformanceMatrix{
		Product:              "Agent Ledger",
		Contract:             "agent-ledger.adapter-conformance-matrix",
		Version:              "v1",
		LocalFirst:           true,
		ReadOnlySafe:         true,
		WritesLocalState:     false,
		SchemaVersion:        storage.CanonicalEventSchemaVersion,
		SchemaHash:           storage.CanonicalEventSchemaFingerprint(),
		AdapterSpecHash:      AdapterContractFingerprint(),
		ProviderProfilesHash: ProviderProfilesFingerprint(),
		PrivacyPolicy:        "Conformance matrix contains fixture paths, validation entrypoints, expected metadata event families, and privacy notes only; it never includes prompts, responses, secrets, local paths, session ids, machine names, authors, or usage rows.",
		Kinds:                kinds,
		QualityGates: []string{
			"all shipped fixtures should pass agent-ledger adapter conformance --strict before adapter ingest is enabled",
			"new provider/runtime adapters must add at least one privacy-safe fixture and list the expected canonical event types",
			"fixtures must not contain request content, generation content, transcript bodies, raw headers, API credentials, or local filesystem paths",
			"streaming adapters must prove that usage can be recovered without storing stream deltas",
			"estimated or fuzzy fields must be represented through match_type, confidence, or provenance warnings",
		},
		RoutingGuidance: []string{
			"wrappers should read this matrix before selecting a conformance kind",
			"CI should prefer strict=true and fail on provenance warnings for release fixtures",
			"gateways should use provider-stream for SSE usage events and provider for completed JSON usage envelopes",
			"agent protocol bridges should use a2a or canonical events for workload/run lineage rather than provider usage envelopes",
		},
	}
	matrix.Summary = summarizeConformanceMatrix(kinds)
	return matrix
}

func matrixKind(spec AdapterContract, kind string, formats, expectedEvents []string, fixtures []AdapterConformanceFixture) AdapterConformanceMatrixKind {
	input := adapterInputKindByConformanceKind(spec, kind)
	return AdapterConformanceMatrixKind{
		Kind:               input.Kind,
		ConformanceKind:    kind,
		Description:        input.Description,
		Status:             "implemented",
		Maturity:           "stable-v1",
		Endpoint:           "POST /api/integrations/conformance?kind=" + kind + "&strict=true",
		CLICommand:         "agent-ledger adapter conformance --kind " + kind + " --strict --file fixture.json",
		MCPTool:            "ledger.adapter_conformance",
		StrictCICommand:    "agent-ledger adapter conformance --kind " + kind + " --strict --file <fixture>",
		ConvertCommand:     input.ConvertCommand,
		IngestCommand:      input.IngestCommand,
		AcceptedFormats:    formats,
		RequiredSignals:    input.RequiredSignals,
		PrivacyNotes:       input.PrivacyNotes,
		ExpectedEventTypes: expectedEvents,
		Fixtures:           fixtures,
	}
}

func matrixFixture(kind, path, format, scenario string, expectedEvents, profileIDs []string) AdapterConformanceFixture {
	return AdapterConformanceFixture{
		Path:               path,
		Format:             format,
		Scenario:           scenario,
		Strict:             true,
		Command:            "agent-ledger adapter conformance --kind " + kind + " --strict --file " + path,
		ProviderProfileIDs: profileIDs,
		ExpectedEventTypes: expectedEvents,
		Privacy:            "metadata-only fixture; prompt, response, message, transcript, raw header, secret, and local path content are excluded",
	}
}

func adapterInputKindByConformanceKind(spec AdapterContract, kind string) AdapterInputKind {
	for _, input := range spec.SupportedInputKinds {
		if input.ConformanceKind == kind {
			return input
		}
	}
	return AdapterInputKind{Kind: kind, ConformanceKind: kind}
}

func summarizeConformanceMatrix(kinds []AdapterConformanceMatrixKind) AdapterConformanceMatrixSummary {
	out := AdapterConformanceMatrixSummary{InputKinds: len(kinds)}
	for _, kind := range kinds {
		for _, fixture := range kind.Fixtures {
			out.Fixtures++
			if fixture.Strict {
				out.StrictFixtures++
			}
			switch kind.ConformanceKind {
			case "canonical":
				out.CanonicalFixtures++
			case "provider":
				out.ProviderFixtures++
			case "provider-stream":
				out.ProviderStreamFixtures++
			case "otel":
				out.OTelFixtures++
			case "a2a":
				out.A2AFixtures++
			}
		}
	}
	return out
}

// AdapterConformanceMatrixFingerprint returns a stable hash for caching and
// contract verification.
func AdapterConformanceMatrixFingerprint() string {
	raw, err := json.Marshal(AdapterConformanceMatrixSpec())
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
