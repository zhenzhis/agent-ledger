package integrations

import (
	"encoding/json"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestConvertOTelGenAISpanToCanonicalModelCall(t *testing.T) {
	raw := []byte(`{
		"trace_id":"trace-1",
		"span_id":"span-1",
		"name":"chat gpt-5.5",
		"start_time":"2026-06-07T10:00:00Z",
		"end_time":"2026-06-07T10:00:02Z",
		"attributes":{
			"gen_ai.provider.name":"openai",
			"gen_ai.request.model":"gpt-5.5",
			"gen_ai.usage.input_tokens":1200,
			"gen_ai.usage.cache_read.input_tokens":200,
			"gen_ai.usage.cache_creation.input_tokens":100,
			"gen_ai.usage.output_tokens":300,
			"gen_ai.usage.reasoning.output_tokens":80,
			"gen_ai.input.messages":[{"role":"user","content":"must not persist"}],
			"agent_ledger.goal":"ship otel ingest",
			"agent_ledger.project":"agent-ledger"
		}
	}`)
	spans, err := DecodeOTelGenAISpans(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	events, err := ConvertOTelGenAISpans(spans)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("events=%d", len(events))
	}
	event := findEvent(t, events, "model.call")
	if event.EventType != "model.call" || event.Source != "opentelemetry" || event.Model != "gpt-5.5" || event.Project != "agent-ledger" {
		t.Fatalf("unexpected event: %#v", event)
	}
	if event.ParserVersion != "agent-ledger-otel-genai@v1" || event.RawRef == "" || event.MatchType != "source_reported" {
		t.Fatalf("OTel provenance missing: %#v", event)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload["input_tokens"].(float64) != 900 || payload["cache_read_input_tokens"].(float64) != 200 || payload["cache_creation_input_tokens"].(float64) != 100 {
		t.Fatalf("non-overlap token mapping failed: %#v", payload)
	}
	if _, ok := payload["gen_ai.input.messages"]; ok {
		t.Fatalf("sensitive OTel message leaked: %#v", payload)
	}
	contextEvent := findEvent(t, events, "context.ref")
	if event.WorkloadID == "" || contextEvent.WorkloadID != event.WorkloadID {
		t.Fatalf("events must share deterministic workload: model=%s context=%s", event.WorkloadID, contextEvent.WorkloadID)
	}
	var contextPayload map[string]interface{}
	if err := json.Unmarshal(contextEvent.Payload, &contextPayload); err != nil {
		t.Fatalf("context payload: %v", err)
	}
	if contextPayload["ref_type"] != "otel_span" || contextPayload["ref_hash"] == "" {
		t.Fatalf("unexpected context payload: %#v", contextPayload)
	}
	if contextEvent.ParserVersion != event.ParserVersion || contextEvent.MatchType != "reconstructed" {
		t.Fatalf("OTel context provenance missing: %#v", contextEvent)
	}
}

func TestDecodeOTLPResourceSpans(t *testing.T) {
	raw := []byte(`{
		"resourceSpans":[{
			"resource":{"attributes":[{"key":"service.namespace","value":{"stringValue":"quant"}}]},
			"scopeSpans":[{
				"scope":{"name":"agent-ledger-test"},
				"spans":[{
					"traceId":"abc",
					"spanId":"def",
					"startTimeUnixNano":"1780836000000000000",
					"attributes":[
						{"key":"gen_ai.provider.name","value":{"stringValue":"anthropic"}},
						{"key":"gen_ai.request.model","value":{"stringValue":"claude-opus-4-7"}},
						{"key":"gen_ai.usage.input_tokens","value":{"intValue":"10"}},
						{"key":"gen_ai.usage.output_tokens","value":{"intValue":"5"}}
					]
				}]
			}]
		}]
	}`)
	spans, err := DecodeOTelGenAISpans(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	events, err := ConvertOTelGenAISpans(spans)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	modelEvent := findEvent(t, events, "model.call")
	if len(events) != 2 || modelEvent.Project != "quant" || modelEvent.Confidence < 0.8 {
		t.Fatalf("unexpected events: %#v", events)
	}
}

func TestConvertOpenInferenceSpanToCanonicalModelCall(t *testing.T) {
	raw := []byte(`{
		"trace_id":"trace-openinference-1",
		"span_id":"span-openinference-1",
		"name":"OpenInference LLM span",
		"start_time":"2026-06-07T11:00:00Z",
		"end_time":"2026-06-07T11:00:01Z",
		"attributes":{
			"openinference.span.kind":"LLM",
			"llm.provider":"google",
			"llm.model_name":"gemini-2.5-pro",
			"llm.token_count.prompt":160,
			"llm.token_count.prompt_details.cache_read":40,
			"llm.token_count.prompt_details.cache_write":10,
			"llm.token_count.completion":55,
			"llm.token_count.completion_details.reasoning":12,
			"llm.finish_reason":"stop",
			"agent_ledger.goal":"openinference fixture",
			"agent_ledger.project":"agent-ledger"
		}
	}`)
	spans, err := DecodeOTelGenAISpans(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(spans) != 1 || spans[0].SourceConvention != "openinference" {
		t.Fatalf("unexpected span convention: %#v", spans)
	}
	events, err := ConvertOTelGenAISpans(spans)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	event := findEvent(t, events, "model.call")
	if event.Model != "gemini-2.5-pro" || event.Project != "agent-ledger" {
		t.Fatalf("unexpected event identity: %#v", event)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(event.Payload, &payload); err != nil {
		t.Fatalf("payload: %v", err)
	}
	if payload["provider"] != "google" || payload["operation"] != "LLM" || payload["otel_convention"] != "openinference" {
		t.Fatalf("unexpected OpenInference payload identity: %#v", payload)
	}
	if payload["input_tokens"].(float64) != 110 || payload["cache_read_input_tokens"].(float64) != 40 || payload["cache_creation_input_tokens"].(float64) != 10 || payload["output_tokens"].(float64) != 55 || payload["reasoning_output_tokens"].(float64) != 12 {
		t.Fatalf("unexpected OpenInference token mapping: %#v", payload)
	}
	if containsAny(string(event.Payload), "prompt", "message", "content") {
		t.Fatalf("OpenInference content leaked: %s", string(event.Payload))
	}
}

func findEvent(t *testing.T, events []storage.CanonicalEvent, eventType string) storage.CanonicalEvent {
	t.Helper()
	for _, event := range events {
		if event.EventType == eventType {
			return event
		}
	}
	t.Fatalf("event type %s missing in %#v", eventType, events)
	return storage.CanonicalEvent{}
}
