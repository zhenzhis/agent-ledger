package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// OTelSpan is a compact representation of a GenAI-related OpenTelemetry span.
type OTelSpan struct {
	TraceID          string                 `json:"trace_id"`
	SpanID           string                 `json:"span_id"`
	ParentSpanID     string                 `json:"parent_span_id,omitempty"`
	Name             string                 `json:"name,omitempty"`
	StartTime        time.Time              `json:"start_time,omitempty"`
	EndTime          time.Time              `json:"end_time,omitempty"`
	Attributes       map[string]interface{} `json:"attributes"`
	ResourceAttrs    map[string]interface{} `json:"resource_attributes,omitempty"`
	Instrumentation  string                 `json:"instrumentation,omitempty"`
	SourceConvention string                 `json:"source_convention"`
}

// DecodeOTelGenAISpans decodes common OTel JSON span shapes, including OTLP JSON.
func DecodeOTelGenAISpans(raw []byte) ([]OTelSpan, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty OpenTelemetry input")
	}
	if trimmed[0] == '[' {
		var entries []json.RawMessage
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, err
		}
		return decodeSpanList(entries, nil)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, err
	}
	if rawSpans, ok := obj["spans"]; ok {
		var entries []json.RawMessage
		if err := json.Unmarshal(rawSpans, &entries); err != nil {
			return nil, err
		}
		return decodeSpanList(entries, nil)
	}
	if rawResourceSpans, ok := obj["resourceSpans"]; ok {
		return decodeOTLPResourceSpans(rawResourceSpans)
	}
	span, err := decodeSpanObject(trimmed, nil)
	if err != nil {
		return nil, err
	}
	return []OTelSpan{span}, nil
}

// ConvertOTelGenAISpans projects OTel GenAI spans into metadata-only canonical events.
func ConvertOTelGenAISpans(spans []OTelSpan) ([]storage.CanonicalEvent, error) {
	events := make([]storage.CanonicalEvent, 0, len(spans))
	for _, span := range spans {
		if !isGenAISpan(span) {
			continue
		}
		attrs := mergedAttrs(span)
		inputTotal := intAttr(attrs, "gen_ai.usage.input_tokens", "gen_ai.usage.prompt_tokens", "llm.usage.prompt_tokens")
		cacheRead := intAttr(attrs, "gen_ai.usage.cache_read.input_tokens", "gen_ai.usage.cache_read_input_tokens")
		cacheWrite := intAttr(attrs, "gen_ai.usage.cache_creation.input_tokens", "gen_ai.usage.cache_creation_input_tokens")
		nonCachedInput := inputTotal - cacheRead - cacheWrite
		if nonCachedInput < 0 {
			nonCachedInput = 0
		}
		output := intAttr(attrs, "gen_ai.usage.output_tokens", "gen_ai.usage.completion_tokens", "llm.usage.completion_tokens")
		reasoning := intAttr(attrs, "gen_ai.usage.reasoning.output_tokens", "gen_ai.usage.reasoning_output_tokens")
		model := stringAttr(attrs, "agent_ledger.model", "gen_ai.response.model", "gen_ai.request.model", "llm.request.model")
		provider := stringAttr(attrs, "gen_ai.provider.name", "gen_ai.system", "llm.system")
		payload := map[string]interface{}{
			"call_id":                     firstNonEmpty(span.SpanID, span.TraceID),
			"trace_id":                    span.TraceID,
			"span_id":                     span.SpanID,
			"parent_span_id":              span.ParentSpanID,
			"provider":                    provider,
			"model":                       model,
			"model_alias":                 stringAttr(attrs, "gen_ai.request.model", "llm.request.model"),
			"operation":                   stringAttr(attrs, "gen_ai.operation.name", "gen_ai.operation"),
			"input_tokens":                nonCachedInput,
			"cache_read_input_tokens":     cacheRead,
			"cache_creation_input_tokens": cacheWrite,
			"output_tokens":               output,
			"reasoning_output_tokens":     reasoning,
			"latency_ms":                  span.DurationMS(),
			"finish_reason":               stringAttr(attrs, "gen_ai.response.finish_reasons", "gen_ai.response.finish_reason"),
			"pricing_source":              "unpriced",
			"pricing_confidence":          "opentelemetry-metadata",
			"otel_span_name":              span.Name,
			"otel_convention":             span.SourceConvention,
		}
		if goal := stringAttr(attrs, "agent_ledger.goal", "agent.goal", "workload.goal"); goal != "" {
			payload["goal"] = goal
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		events = append(events, storage.CanonicalEvent{
			EventID:       firstNonEmpty(stringAttr(attrs, "agent_ledger.event_id"), "otel:"+span.TraceID+":"+span.SpanID),
			Source:        firstNonEmpty(stringAttr(attrs, "agent_ledger.source"), "opentelemetry"),
			EventType:     "model.call",
			SourceEventID: firstNonEmpty(stringAttr(attrs, "agent_ledger.source_event_id"), span.TraceID+":"+span.SpanID),
			WorkloadID:    stringAttr(attrs, "agent_ledger.workload_id"),
			AgentRunID:    stringAttr(attrs, "agent_ledger.agent_run_id"),
			SessionID:     stringAttr(attrs, "agent_ledger.session_id", "session.id"),
			Model:         model,
			Project:       stringAttr(attrs, "agent_ledger.project", "project", "service.namespace", "code.namespace"),
			GitBranch:     stringAttr(attrs, "agent_ledger.git_branch", "git.branch"),
			Timestamp:     firstTime(span.StartTime, time.Now().UTC()),
			Payload:       payloadJSON,
			Confidence:    otelConfidence(model, inputTotal, output),
		})
	}
	return events, nil
}

// DurationMS returns the span duration in milliseconds when timestamps are present.
func (s OTelSpan) DurationMS() int64 {
	if s.StartTime.IsZero() || s.EndTime.IsZero() || s.EndTime.Before(s.StartTime) {
		return 0
	}
	return s.EndTime.Sub(s.StartTime).Milliseconds()
}

func decodeSpanList(entries []json.RawMessage, resourceAttrs map[string]interface{}) ([]OTelSpan, error) {
	spans := make([]OTelSpan, 0, len(entries))
	for _, entry := range entries {
		span, err := decodeSpanObject(entry, resourceAttrs)
		if err != nil {
			return nil, err
		}
		spans = append(spans, span)
	}
	return spans, nil
}

func decodeOTLPResourceSpans(raw json.RawMessage) ([]OTelSpan, error) {
	var resourceSpans []struct {
		Resource struct {
			Attributes json.RawMessage `json:"attributes"`
		} `json:"resource"`
		ScopeSpans []struct {
			Scope struct {
				Name string `json:"name"`
			} `json:"scope"`
			Spans []json.RawMessage `json:"spans"`
		} `json:"scopeSpans"`
	}
	if err := json.Unmarshal(raw, &resourceSpans); err != nil {
		return nil, err
	}
	out := []OTelSpan{}
	for _, rs := range resourceSpans {
		resourceAttrs, err := decodeAttributes(rs.Resource.Attributes)
		if err != nil {
			return nil, err
		}
		for _, ss := range rs.ScopeSpans {
			spans, err := decodeSpanList(ss.Spans, resourceAttrs)
			if err != nil {
				return nil, err
			}
			for i := range spans {
				spans[i].Instrumentation = ss.Scope.Name
			}
			out = append(out, spans...)
		}
	}
	return out, nil
}

func decodeSpanObject(raw json.RawMessage, resourceAttrs map[string]interface{}) (OTelSpan, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return OTelSpan{}, err
	}
	attrs, err := decodeAttributes(obj["attributes"])
	if err != nil {
		return OTelSpan{}, err
	}
	span := OTelSpan{
		TraceID:          textField(obj, "trace_id", "traceId"),
		SpanID:           textField(obj, "span_id", "spanId"),
		ParentSpanID:     textField(obj, "parent_span_id", "parentSpanId"),
		Name:             textField(obj, "name"),
		StartTime:        timeField(obj, "start_time", "startTime", "startTimeUnixNano"),
		EndTime:          timeField(obj, "end_time", "endTime", "endTimeUnixNano"),
		Attributes:       attrs,
		ResourceAttrs:    cloneAttrs(resourceAttrs),
		SourceConvention: "opentelemetry.gen_ai",
	}
	return span, nil
}

func decodeAttributes(raw json.RawMessage) (map[string]interface{}, error) {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return map[string]interface{}{}, nil
	}
	if bytes.TrimSpace(raw)[0] == '[' {
		var attrs []struct {
			Key   string          `json:"key"`
			Value json.RawMessage `json:"value"`
		}
		if err := json.Unmarshal(raw, &attrs); err != nil {
			return nil, err
		}
		out := map[string]interface{}{}
		for _, attr := range attrs {
			out[attr.Key] = decodeOTelValue(attr.Value)
		}
		return out, nil
	}
	var attrs map[string]interface{}
	if err := json.Unmarshal(raw, &attrs); err != nil {
		return nil, err
	}
	return attrs, nil
}

func decodeOTelValue(raw json.RawMessage) interface{} {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		var plain interface{}
		_ = json.Unmarshal(raw, &plain)
		return plain
	}
	for _, key := range []string{"stringValue", "intValue", "doubleValue", "boolValue"} {
		if value, ok := obj[key]; ok {
			var out interface{}
			_ = json.Unmarshal(value, &out)
			return out
		}
	}
	if value, ok := obj["arrayValue"]; ok {
		var arr struct {
			Values []json.RawMessage `json:"values"`
		}
		_ = json.Unmarshal(value, &arr)
		out := make([]interface{}, 0, len(arr.Values))
		for _, item := range arr.Values {
			out = append(out, decodeOTelValue(item))
		}
		return out
	}
	return nil
}

func mergedAttrs(span OTelSpan) map[string]interface{} {
	out := cloneAttrs(span.ResourceAttrs)
	for k, v := range span.Attributes {
		out[k] = v
	}
	return out
}

func cloneAttrs(in map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{}
	for k, v := range in {
		out[k] = v
	}
	return out
}

func isGenAISpan(span OTelSpan) bool {
	for key := range mergedAttrs(span) {
		if strings.HasPrefix(key, "gen_ai.") || strings.HasPrefix(key, "llm.") || strings.HasPrefix(key, "agent_ledger.") {
			return true
		}
	}
	return false
}

func stringAttr(attrs map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if v, ok := attrs[key]; ok {
			switch typed := v.(type) {
			case string:
				if strings.TrimSpace(typed) != "" {
					return typed
				}
			case []interface{}:
				parts := make([]string, 0, len(typed))
				for _, item := range typed {
					if item != nil {
						parts = append(parts, fmt.Sprint(item))
					}
				}
				if len(parts) > 0 {
					return strings.Join(parts, ",")
				}
			default:
				if typed != nil {
					return fmt.Sprint(typed)
				}
			}
		}
	}
	return ""
}

func intAttr(attrs map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if v, ok := attrs[key]; ok {
			switch typed := v.(type) {
			case int:
				return typed
			case int64:
				return int(typed)
			case float64:
				return int(math.Round(typed))
			case json.Number:
				n, _ := typed.Int64()
				return int(n)
			case string:
				n, _ := strconv.Atoi(typed)
				return n
			}
		}
	}
	return 0
}

func textField(obj map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		if raw, ok := obj[key]; ok {
			var out string
			if err := json.Unmarshal(raw, &out); err == nil {
				return out
			}
			var number json.Number
			if err := json.Unmarshal(raw, &number); err == nil {
				return number.String()
			}
		}
	}
	return ""
}

func timeField(obj map[string]json.RawMessage, keys ...string) time.Time {
	for _, key := range keys {
		raw, ok := obj[key]
		if !ok {
			continue
		}
		var text string
		if err := json.Unmarshal(raw, &text); err == nil {
			if parsed, err := time.Parse(time.RFC3339Nano, text); err == nil {
				return parsed.UTC()
			}
			if n, err := strconv.ParseInt(text, 10, 64); err == nil {
				return unixNano(n)
			}
		}
		var n int64
		if err := json.Unmarshal(raw, &n); err == nil {
			return unixNano(n)
		}
	}
	return time.Time{}
}

func unixNano(n int64) time.Time {
	if n <= 0 {
		return time.Time{}
	}
	return time.Unix(0, n).UTC()
}

func firstTime(values ...time.Time) time.Time {
	for _, value := range values {
		if !value.IsZero() {
			return value
		}
	}
	return time.Time{}
}

func otelConfidence(model string, inputTokens, outputTokens int) float64 {
	if model != "" && (inputTokens > 0 || outputTokens > 0) {
		return 0.85
	}
	if model != "" {
		return 0.7
	}
	return 0.55
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
