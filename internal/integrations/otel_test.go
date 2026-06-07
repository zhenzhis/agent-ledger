package integrations

import (
	"encoding/json"
	"testing"
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
	if len(events) != 1 {
		t.Fatalf("events=%d", len(events))
	}
	event := events[0]
	if event.EventType != "model.call" || event.Source != "opentelemetry" || event.Model != "gpt-5.5" || event.Project != "agent-ledger" {
		t.Fatalf("unexpected event: %#v", event)
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
	if len(events) != 1 || events[0].Project != "quant" || events[0].Confidence < 0.8 {
		t.Fatalf("unexpected events: %#v", events)
	}
}
