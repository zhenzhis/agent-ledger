package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/config"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

func TestOTLPReceiverDisabledByDefault(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/traces", strings.NewReader(otlpPayload("span-1")))
	rr := httptest.NewRecorder()
	srv.handleOTLPTraces(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected disabled receiver 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestOTLPReceiverReadOnlyRejectsWrite(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{
		RBAC: config.RBACConfig{ReadOnly: true},
		Integrations: config.IntegrationsConfig{
			OTLPReceiver: config.OTLPReceiverConfig{Enabled: true, MaxBodyBytes: 1 << 20, MaxSpans: 10},
		},
	})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/traces", strings.NewReader(otlpPayload("span-1")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOTLPTraces(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected read-only 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "read-only mode") {
		t.Fatalf("expected explicit read-only error, got %s", rr.Body.String())
	}
	rows, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("read-only OTLP receiver should not write audit rows: %+v", rows)
	}
}

func TestOTLPReceiverRejectsOversizedBody(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{Integrations: config.IntegrationsConfig{
		OTLPReceiver: config.OTLPReceiverConfig{Enabled: true, MaxBodyBytes: 32, MaxSpans: 10},
	}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/traces", strings.NewReader(otlpPayload("span-1")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOTLPTraces(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected JSON error content type, got %q", got)
	}
	if rr.Header().Get("X-Agent-Ledger-OTLP-Backpressure") != "body_limit_exceeded" {
		t.Fatalf("expected body limit backpressure header, got %q", rr.Header().Get("X-Agent-Ledger-OTLP-Backpressure"))
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	backpressure := body["backpressure"].(map[string]interface{})
	if backpressure["status"] != "body_limit_exceeded" || int(backpressure["max_body_bytes"].(float64)) != 32 {
		t.Fatalf("unexpected body limit backpressure payload: %#v", backpressure)
	}
	events, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	for _, event := range events {
		if event.Action == "otlp.receiver.backpressure" && event.Target == "body_limit_exceeded" {
			return
		}
	}
	t.Fatalf("missing body limit backpressure audit event: %+v", events)
}

func TestOTLPReceiverIngestsJSONSpans(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{Integrations: config.IntegrationsConfig{
		OTLPReceiver: config.OTLPReceiverConfig{Enabled: true, MaxBodyBytes: 1 << 20, MaxSpans: 10},
	}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/traces", strings.NewReader(otlpPayload("span-1")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOTLPTraces(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if int(body["spans"].(float64)) != 1 || int(body["events"].(float64)) != 2 {
		t.Fatalf("unexpected receiver body: %#v", body)
	}
	if rr.Header().Get("X-Agent-Ledger-OTLP-Backpressure") != "accepted" ||
		rr.Header().Get("X-Agent-Ledger-OTLP-Spans") != "1" ||
		rr.Header().Get("X-Agent-Ledger-OTLP-Events") != "2" ||
		rr.Header().Get("X-Agent-Ledger-OTLP-Max-Spans") != "10" {
		t.Fatalf("unexpected OTLP backpressure headers: status=%q spans=%q events=%q max_spans=%q",
			rr.Header().Get("X-Agent-Ledger-OTLP-Backpressure"),
			rr.Header().Get("X-Agent-Ledger-OTLP-Spans"),
			rr.Header().Get("X-Agent-Ledger-OTLP-Events"),
			rr.Header().Get("X-Agent-Ledger-OTLP-Max-Spans"))
	}
	backpressure := body["backpressure"].(map[string]interface{})
	if backpressure["status"] != "accepted" || int(backpressure["spans_seen"].(float64)) != 1 || int(backpressure["events_produced"].(float64)) != 2 {
		t.Fatalf("unexpected accepted backpressure payload: %#v", backpressure)
	}
	events, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	for _, event := range events {
		if event.Action == "otlp.receiver.ingest" && event.Target == "2" {
			return
		}
	}
	t.Fatalf("missing otlp audit event: %+v", events)
}

func TestOTLPReceiverRejectsOversizedSpanBatch(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{Integrations: config.IntegrationsConfig{
		OTLPReceiver: config.OTLPReceiverConfig{Enabled: true, MaxBodyBytes: 1 << 20, MaxSpans: 1},
	}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/otlp/v1/traces", strings.NewReader(otlpPayload("span-1", "span-2")))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOTLPTraces(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Header().Get("X-Agent-Ledger-OTLP-Backpressure") != "span_limit_exceeded" ||
		rr.Header().Get("X-Agent-Ledger-OTLP-Spans") != "2" ||
		rr.Header().Get("X-Agent-Ledger-OTLP-Max-Spans") != "1" {
		t.Fatalf("unexpected span limit backpressure headers: status=%q spans=%q max_spans=%q",
			rr.Header().Get("X-Agent-Ledger-OTLP-Backpressure"),
			rr.Header().Get("X-Agent-Ledger-OTLP-Spans"),
			rr.Header().Get("X-Agent-Ledger-OTLP-Max-Spans"))
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	backpressure := body["backpressure"].(map[string]interface{})
	if backpressure["status"] != "span_limit_exceeded" || int(backpressure["spans_seen"].(float64)) != 2 {
		t.Fatalf("unexpected span limit backpressure payload: %#v", backpressure)
	}
	events, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	for _, event := range events {
		if event.Action == "otlp.receiver.backpressure" && event.Target == "span_limit_exceeded" {
			return
		}
	}
	t.Fatalf("missing span limit backpressure audit event: %+v", events)
}

func TestOTLPReceiverIngestsProtobufSpans(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{Integrations: config.IntegrationsConfig{
		OTLPReceiver: config.OTLPReceiverConfig{Enabled: true, MaxBodyBytes: 1 << 20, MaxSpans: 10},
	}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/v1/traces", bytes.NewReader(otlpProtoPayload(t)))
	req.Header.Set("Content-Type", "application/x-protobuf")
	rr := httptest.NewRecorder()
	srv.handleOTLPTraces(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if int(body["spans"].(float64)) != 1 || int(body["events"].(float64)) != 2 {
		t.Fatalf("unexpected protobuf receiver body: %#v", body)
	}
	if rr.Header().Get("X-Agent-Ledger-OTLP-Backpressure") != "accepted" ||
		rr.Header().Get("X-Agent-Ledger-OTLP-Spans") != "1" ||
		rr.Header().Get("X-Agent-Ledger-OTLP-Events") != "2" {
		t.Fatalf("unexpected protobuf backpressure headers: status=%q spans=%q events=%q",
			rr.Header().Get("X-Agent-Ledger-OTLP-Backpressure"),
			rr.Header().Get("X-Agent-Ledger-OTLP-Spans"),
			rr.Header().Get("X-Agent-Ledger-OTLP-Events"))
	}
	events, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	for _, event := range events {
		if event.Action == "otlp.receiver.ingest" && event.Target == "2" {
			return
		}
	}
	t.Fatalf("missing otlp protobuf audit event: %+v", events)
}

func otlpPayload(spanIDs ...string) string {
	spans := make([]string, 0, len(spanIDs))
	for _, spanID := range spanIDs {
		spans = append(spans, `{
			"traceId":"trace-otlp",
			"spanId":"`+spanID+`",
			"name":"genai.chat",
			"startTimeUnixNano":"1780836000000000000",
			"endTimeUnixNano":"1780836001000000000",
			"attributes":[
				{"key":"gen_ai.request.model","value":{"stringValue":"gpt-5.5"}},
				{"key":"gen_ai.provider.name","value":{"stringValue":"openai"}},
				{"key":"gen_ai.usage.input_tokens","value":{"intValue":"10"}},
				{"key":"gen_ai.usage.output_tokens","value":{"intValue":"5"}},
				{"key":"agent_ledger.goal","value":{"stringValue":"receiver smoke"}},
				{"key":"agent_ledger.source","value":{"stringValue":"otlp-test"}}
			]
		}`)
	}
	return `{
		"resourceSpans":[{
			"resource":{"attributes":[{"key":"service.namespace","value":{"stringValue":"quant"}}]},
			"scopeSpans":[{"scope":{"name":"agent-ledger-test"},"spans":[` + strings.Join(spans, ",") + `]}]
		}]
	}`
}

func otlpProtoPayload(t *testing.T) []byte {
	t.Helper()
	req := otlpProtoRequest(t, "span-1")
	raw, err := proto.Marshal(req)
	if err != nil {
		t.Fatalf("marshal OTLP protobuf: %v", err)
	}
	return raw
}

func otlpProtoRequest(t *testing.T, spanIDs ...string) *collectortracepb.ExportTraceServiceRequest {
	t.Helper()
	if len(spanIDs) == 0 {
		spanIDs = []string{"span-1"}
	}
	spans := make([]*tracepb.Span, 0, len(spanIDs))
	for i, spanID := range spanIDs {
		spans = append(spans, &tracepb.Span{
			TraceId:           []byte{0x10, 0x20, 0x30, 0x40, 0x50, 0x60, 0x70, 0x80, 0x90, 0xa0, 0xb0, 0xc0, 0xd0, 0xe0, 0xf0, 0x01},
			SpanId:            otlpSpanIDBytes(i),
			Name:              "genai.chat",
			StartTimeUnixNano: 1780836000000000000,
			EndTimeUnixNano:   1780836001000000000,
			Attributes: []*commonpb.KeyValue{
				otelKVString("gen_ai.request.model", "gpt-5.5"),
				otelKVString("gen_ai.provider.name", "openai"),
				otelKVInt("gen_ai.usage.input_tokens", 10),
				otelKVInt("gen_ai.usage.output_tokens", 5),
				otelKVString("agent_ledger.goal", "receiver protobuf smoke "+spanID),
				otelKVString("agent_ledger.source", "otlp-protobuf-test"),
			},
		})
	}
	return &collectortracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
				otelKVString("service.namespace", "quant"),
			}},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Scope: &commonpb.InstrumentationScope{Name: "agent-ledger-test"},
				Spans: spans,
			}},
		}},
	}
}

func otlpSpanIDBytes(i int) []byte {
	return []byte{0x01, 0x23, 0x45, 0x67, 0x89, 0xab, 0xcd, byte(0xef - i)}
}

func otelKVString(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}},
	}
}

func otelKVInt(key string, value int64) *commonpb.KeyValue {
	return &commonpb.KeyValue{
		Key:   key,
		Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_IntValue{IntValue: value}},
	}
}
