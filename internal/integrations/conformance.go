package integrations

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// AdapterConformanceResult is one canonical event contract check.
type AdapterConformanceResult struct {
	Index       int      `json:"index"`
	EventID     string   `json:"event_id,omitempty"`
	EventType   string   `json:"event_type,omitempty"`
	Source      string   `json:"source,omitempty"`
	Status      string   `json:"status"`
	Error       string   `json:"error,omitempty"`
	Warnings    []string `json:"warnings,omitempty"`
	PayloadHash string   `json:"payload_hash,omitempty"`
}

// AdapterConformanceReport summarizes adapter or protocol fixture compatibility
// with the canonical metadata-only event contract.
type AdapterConformanceReport struct {
	Contract        string                     `json:"contract"`
	Version         string                     `json:"version"`
	SchemaVersion   string                     `json:"schema_version"`
	SchemaHash      string                     `json:"schema_hash"`
	InputKind       string                     `json:"input_kind"`
	Status          string                     `json:"status"`
	OK              bool                       `json:"ok"`
	DecodedEvents   int                        `json:"decoded_events"`
	ValidEvents     int                        `json:"valid_events"`
	WarningEvents   int                        `json:"warning_events"`
	FailedEvents    int                        `json:"failed_events"`
	PrivacyPolicy   string                     `json:"privacy_policy"`
	Recommendations []string                   `json:"recommendations,omitempty"`
	Results         []AdapterConformanceResult `json:"results"`
}

// AdapterConformanceOptions controls dry-run validation behavior.
type AdapterConformanceOptions struct {
	Kind   string `json:"kind"`
	Strict bool   `json:"strict"`
}

// RunAdapterConformance converts a supported adapter input into canonical events
// and dry-runs the same validation used by ingest without writing local state.
func RunAdapterConformance(kind string, raw []byte) (AdapterConformanceReport, error) {
	return RunAdapterConformanceWithOptions(AdapterConformanceOptions{Kind: kind}, raw)
}

// RunAdapterConformanceWithOptions runs adapter conformance with optional CI
// strictness. Strict mode treats provenance warnings as failures.
func RunAdapterConformanceWithOptions(opts AdapterConformanceOptions, raw []byte) (AdapterConformanceReport, error) {
	normalized, events, err := DecodeAdapterConformanceEvents(opts.Kind, raw)
	report := AdapterConformanceReport{
		Contract:      "agent-ledger.adapter-conformance",
		Version:       "v1",
		SchemaVersion: storage.CanonicalEventSchemaVersion,
		SchemaHash:    storage.CanonicalEventSchemaFingerprint(),
		InputKind:     normalized,
		Status:        "fail",
		PrivacyPolicy: "metadata-only canonical payloads; prompt/content/message keys are rejected",
	}
	if err != nil {
		return report, err
	}
	report.DecodedEvents = len(events)
	if len(events) == 0 {
		report.FailedEvents = 1
		report.Results = append(report.Results, AdapterConformanceResult{Index: 0, Status: "error", Error: "input decoded successfully but produced no canonical events"})
		report.Recommendations = append(report.Recommendations, "emit at least one supported canonical event or a supported provider/OTel/A2A usage envelope")
		return report, nil
	}
	for i, event := range events {
		result := AdapterConformanceResult{Index: i}
		validation, err := storage.ValidateCanonicalEvent(event)
		if err != nil {
			result.Status = "error"
			result.EventID = event.EventID
			result.EventType = event.EventType
			result.Source = event.Source
			result.Error = err.Error()
			report.FailedEvents++
			report.Results = append(report.Results, result)
			continue
		}
		result.EventID = validation.EventID
		result.EventType = validation.EventType
		result.Source = validation.Source
		result.Status = validation.Status
		result.Warnings = validation.Warnings
		result.PayloadHash = validation.PayloadHash
		if validation.Status == "valid_with_warnings" {
			report.WarningEvents++
		} else {
			report.ValidEvents++
		}
		report.Results = append(report.Results, result)
	}
	report.Status = "pass"
	report.OK = true
	if report.FailedEvents > 0 {
		report.Status = "fail"
		report.OK = false
		report.Recommendations = append(report.Recommendations, "fix all event errors before enabling ingest or CI release gates")
	} else if report.WarningEvents > 0 {
		report.Status = "pass_with_warnings"
		report.Recommendations = append(report.Recommendations, "add source_version, parser_version, raw_ref, and match_type for stronger provenance quality")
	}
	if opts.Strict && report.WarningEvents > 0 {
		report.Status = "fail"
		report.OK = false
		report.Recommendations = append(report.Recommendations, "strict mode treats provenance warnings as failures")
	}
	return report, nil
}

// DecodeAdapterConformanceEvents maps a raw fixture into canonical events without
// persisting anything. kind supports auto, canonical, provider, provider-stream,
// otel, a2a.
func DecodeAdapterConformanceEvents(kind string, raw []byte) (string, []storage.CanonicalEvent, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return normalizeConformanceKind(kind), nil, fmt.Errorf("empty conformance input")
	}
	normalized := normalizeConformanceKind(kind)
	if normalized == "auto" {
		normalized = inferConformanceKind(trimmed)
	}
	switch normalized {
	case "canonical":
		events, err := DecodeCanonicalEvents(trimmed)
		return normalized, events, err
	case "provider":
		calls, err := DecodeProviderCalls(trimmed)
		if err != nil {
			return normalized, nil, err
		}
		events, err := ConvertProviderCalls(calls)
		return normalized, events, err
	case "provider-stream":
		calls, err := DecodeProviderStream(trimmed)
		if err != nil {
			return normalized, nil, err
		}
		events, err := ConvertProviderCalls(calls)
		return normalized, events, err
	case "otel":
		spans, err := DecodeOTelGenAISpans(trimmed)
		if err != nil {
			return normalized, nil, err
		}
		events, err := ConvertOTelGenAISpans(spans)
		return normalized, events, err
	case "a2a":
		tasks, err := DecodeA2ATasks(trimmed)
		if err != nil {
			return normalized, nil, err
		}
		events, err := ConvertA2ATasks(tasks)
		return normalized, events, err
	default:
		return normalized, nil, fmt.Errorf("unsupported conformance kind %q: use auto, canonical, provider, provider-stream, otel, or a2a", kind)
	}
}

// DecodeCanonicalEvents decodes a single event, an array, or {"events":[...]}.
func DecodeCanonicalEvents(raw []byte) ([]storage.CanonicalEvent, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty canonical event input")
	}
	if trimmed[0] == '[' {
		var events []storage.CanonicalEvent
		if err := json.Unmarshal(trimmed, &events); err != nil {
			return nil, err
		}
		return events, nil
	}
	var envelope struct {
		Events []storage.CanonicalEvent `json:"events"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err == nil && len(envelope.Events) > 0 {
		return envelope.Events, nil
	}
	var event storage.CanonicalEvent
	if err := json.Unmarshal(trimmed, &event); err != nil {
		return nil, err
	}
	return []storage.CanonicalEvent{event}, nil
}

func normalizeConformanceKind(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "", "auto", "detect":
		return "auto"
	case "canonical", "event", "events", "canonical-events":
		return "canonical"
	case "provider", "usage", "openai", "anthropic", "litellm":
		return "provider"
	case "provider-stream", "stream", "sse", "provider-sse", "openai-stream", "anthropic-stream":
		return "provider-stream"
	case "otel", "opentelemetry", "otlp", "genai":
		return "otel"
	case "a2a", "agent2agent", "agent-to-agent":
		return "a2a"
	default:
		return strings.ToLower(strings.TrimSpace(kind))
	}
}

func inferConformanceKind(raw []byte) string {
	if looksLikeSSETranscript(raw) {
		return "provider-stream"
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		if bytes.HasPrefix(raw, []byte("[")) {
			return "canonical"
		}
		return "auto"
	}
	if _, ok := obj["event_type"]; ok {
		return "canonical"
	}
	if eventsRaw, ok := obj["events"]; ok && arrayLooksCanonical(eventsRaw) {
		return "canonical"
	}
	if _, ok := obj["resourceSpans"]; ok {
		return "otel"
	}
	if _, ok := obj["spans"]; ok {
		return "otel"
	}
	if _, ok := obj["trace_id"]; ok {
		return "otel"
	}
	if _, ok := obj["traceId"]; ok {
		return "otel"
	}
	if _, ok := obj["usage"]; ok {
		return "provider"
	}
	for _, key := range []string{"responses", "calls", "items"} {
		if _, ok := obj[key]; ok {
			return "provider"
		}
	}
	for _, key := range []string{"task", "tasks", "taskId", "task_id", "status", "result"} {
		if _, ok := obj[key]; ok {
			return "a2a"
		}
	}
	return "canonical"
}

func looksLikeSSETranscript(raw []byte) bool {
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		return strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "data:")
	}
	return false
}

func arrayLooksCanonical(raw json.RawMessage) bool {
	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil || len(entries) == 0 {
		return false
	}
	_, ok := entries[0]["event_type"]
	return ok
}
