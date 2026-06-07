package integrations

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunAdapterConformanceAutoDetectsProviderUsage(t *testing.T) {
	report, err := RunAdapterConformance("auto", []byte(`{
		"id":"resp_conf_1",
		"provider":"openai",
		"model":"gpt-5.5",
		"input":[{"role":"user","content":"must not persist"}],
		"usage":{"input_tokens":100,"input_tokens_details":{"cached_tokens":25},"output_tokens":40},
		"metadata":{"agent_ledger.goal":"conformance provider","agent_ledger.project":"agent-ledger"}
	}`))
	if err != nil {
		t.Fatalf("conformance: %v", err)
	}
	if !report.OK || report.Status != "pass" || report.InputKind != "provider" || report.DecodedEvents != 2 || report.FailedEvents != 0 {
		t.Fatalf("unexpected report: %#v", report)
	}
	if report.SchemaVersion != "v1" || report.SchemaHash == "" {
		t.Fatalf("missing schema identity: %#v", report)
	}
	for _, result := range report.Results {
		if result.EventID == "" || result.PayloadHash == "" || result.EventType == "" {
			t.Fatalf("result missing validation identity: %#v", result)
		}
	}
}

func TestRunAdapterConformanceReportsCanonicalPrivacyFailure(t *testing.T) {
	report, err := RunAdapterConformance("canonical", []byte(`{
		"source":"test-adapter",
		"event_type":"workload.started",
		"payload":{"goal":"safe","messages":[{"content":"must fail"}]}
	}`))
	if err != nil {
		t.Fatalf("conformance should return a report for validation failures: %v", err)
	}
	if report.OK || report.Status != "fail" || report.FailedEvents != 1 {
		t.Fatalf("expected failed report: %#v", report)
	}
	if len(report.Results) != 1 || report.Results[0].Error == "" {
		t.Fatalf("expected result error: %#v", report.Results)
	}
}

func TestRunAdapterConformanceAutoDetectsProviderStreamUsage(t *testing.T) {
	report, err := RunAdapterConformance("auto", []byte(`event: message_start
data: {"type":"message_start","message":{"id":"msg_stream_conf_1","model":"claude-opus-4-7","usage":{"input_tokens":100,"cache_read_input_tokens":10,"cache_creation_input_tokens":5,"output_tokens":1}}}

event: content_block_delta
data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"must not persist"}}

event: message_delta
data: {"type":"message_delta","usage":{"output_tokens":25}}

event: message_stop
data: {"type":"message_stop"}
`))
	if err != nil {
		t.Fatalf("conformance: %v", err)
	}
	if !report.OK || report.Status != "pass" || report.InputKind != "provider-stream" || report.DecodedEvents != 2 || report.FailedEvents != 0 {
		t.Fatalf("unexpected provider-stream report: %#v", report)
	}
}

func TestRunAdapterConformanceCanonicalWarnings(t *testing.T) {
	report, err := RunAdapterConformance("canonical", []byte(`{
		"source":"test-adapter",
		"event_type":"workload.started",
		"payload":{"goal":"missing provenance"}
	}`))
	if err != nil {
		t.Fatalf("conformance: %v", err)
	}
	if !report.OK || report.Status != "pass_with_warnings" || report.WarningEvents != 1 || len(report.Recommendations) == 0 {
		t.Fatalf("expected provenance warning report: %#v", report)
	}
	strict, err := RunAdapterConformanceWithOptions(AdapterConformanceOptions{Kind: "canonical", Strict: true}, []byte(`{
		"source":"test-adapter",
		"event_type":"workload.started",
		"payload":{"goal":"missing provenance"}
	}`))
	if err != nil {
		t.Fatalf("strict conformance: %v", err)
	}
	if strict.OK || strict.Status != "fail" || strict.WarningEvents != 1 || strict.FailedEvents != 0 {
		t.Fatalf("expected strict warning failure: %#v", strict)
	}
}

func TestAdapterFixtureFilesPassStrictConformance(t *testing.T) {
	fixtures := []struct {
		kind string
		file string
	}{
		{"canonical", "canonical-workload.json"},
		{"provider", "provider-openai-response.json"},
		{"provider", "provider-openai-chat-completion.json"},
		{"provider", "provider-anthropic-message.json"},
		{"provider-stream", "provider-openai-chat-stream.sse"},
		{"provider-stream", "provider-openai-responses-stream.sse"},
		{"provider-stream", "provider-anthropic-message-stream.sse"},
		{"otel", "otel-genai-span.json"},
		{"a2a", "a2a-task.json"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.file, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join("..", "..", "examples", "adapter-fixtures", fixture.file))
			if err != nil {
				t.Fatalf("read fixture: %v", err)
			}
			report, err := RunAdapterConformanceWithOptions(AdapterConformanceOptions{Kind: fixture.kind, Strict: true}, raw)
			if err != nil {
				t.Fatalf("conformance: %v", err)
			}
			if !report.OK || report.Status != "pass" || report.FailedEvents != 0 || report.WarningEvents != 0 {
				t.Fatalf("fixture failed strict conformance: %#v", report)
			}
		})
	}
}
