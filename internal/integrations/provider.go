package integrations

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// ProviderCall is a metadata-only provider response usage envelope.
type ProviderCall struct {
	ID        string                 `json:"id"`
	Provider  string                 `json:"provider"`
	Model     string                 `json:"model"`
	Project   string                 `json:"project,omitempty"`
	SessionID string                 `json:"session_id,omitempty"`
	Timestamp time.Time              `json:"timestamp,omitempty"`
	Usage     ProviderUsage          `json:"usage"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ProviderUsage uses Agent Ledger non-overlapping token semantics.
type ProviderUsage struct {
	InputTokens              int     `json:"input_tokens"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	ReasoningOutputTokens    int     `json:"reasoning_output_tokens"`
	CostUSD                  float64 `json:"cost_usd,omitempty"`
}

// DecodeProviderCalls decodes OpenAI-style, Anthropic-style, LiteLLM-style, or generic usage response envelopes.
func DecodeProviderCalls(raw []byte) ([]ProviderCall, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty provider input")
	}
	if trimmed[0] == '[' {
		var entries []json.RawMessage
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return nil, err
		}
		return decodeProviderEntries(entries)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, err
	}
	for _, key := range []string{"responses", "calls", "items"} {
		if rawEntries, ok := obj[key]; ok {
			var entries []json.RawMessage
			if err := json.Unmarshal(rawEntries, &entries); err != nil {
				return nil, err
			}
			return decodeProviderEntries(entries)
		}
	}
	call, err := decodeProviderEntry(trimmed)
	if err != nil {
		return nil, err
	}
	return []ProviderCall{call}, nil
}

// ConvertProviderCalls projects provider usage envelopes into canonical model.call and context.ref events.
func ConvertProviderCalls(calls []ProviderCall) ([]storage.CanonicalEvent, error) {
	events := []storage.CanonicalEvent{}
	for _, call := range calls {
		if call.Model == "" {
			return nil, fmt.Errorf("provider call model is required")
		}
		callID := firstNonEmpty(call.ID, deterministicLedgerID("call", "provider-call:"+call.Provider+":"+call.Model+":"+call.Timestamp.String()))
		source := firstNonEmpty(metadataString(call.Metadata, "agent_ledger.source", "source"), "provider")
		goal := firstNonEmpty(metadataString(call.Metadata, "agent_ledger.goal", "goal"), "Provider model call "+callID)
		workloadID := metadataString(call.Metadata, "agent_ledger.workload_id", "workload_id")
		if workloadID == "" && goal != "" {
			workloadID = deterministicLedgerID("wl", "provider-workload:"+goal+":"+firstNonEmpty(call.SessionID, callID))
		}
		runID := metadataString(call.Metadata, "agent_ledger.agent_run_id", "agent_run_id")
		timestamp := firstTime(call.Timestamp, time.Now().UTC())
		payload := map[string]interface{}{
			"goal":                         goal,
			"call_id":                      callID,
			"provider":                     call.Provider,
			"model":                        call.Model,
			"model_alias":                  metadataString(call.Metadata, "model_alias", "agent_ledger.model_alias"),
			"input_tokens":                 call.Usage.InputTokens,
			"cache_read_input_tokens":      call.Usage.CacheReadInputTokens,
			"cache_creation_input_tokens":  call.Usage.CacheCreationInputTokens,
			"output_tokens":                call.Usage.OutputTokens,
			"reasoning_output_tokens":      call.Usage.ReasoningOutputTokens,
			"cost_usd":                     call.Usage.CostUSD,
			"pricing_source":               firstNonEmpty(metadataString(call.Metadata, "pricing_source"), "provider-reported"),
			"pricing_confidence":           firstNonEmpty(metadataString(call.Metadata, "pricing_confidence"), "provider-usage"),
			"finish_reason":                metadataString(call.Metadata, "finish_reason"),
			"provider_response_id":         call.ID,
			"provider_usage_schema":        metadataString(call.Metadata, "provider_usage_schema"),
			"agent_ledger_provider_mapper": "v1",
		}
		payloadJSON, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		modelEvent := storage.CanonicalEvent{
			EventID:       "provider:" + callID,
			Source:        source,
			EventType:     "model.call",
			SourceEventID: callID,
			WorkloadID:    workloadID,
			AgentRunID:    runID,
			SessionID:     call.SessionID,
			Model:         call.Model,
			Project:       call.Project,
			GitBranch:     metadataString(call.Metadata, "agent_ledger.git_branch", "git_branch"),
			Timestamp:     timestamp,
			Payload:       payloadJSON,
			Confidence:    providerConfidence(call),
		}
		events = append(events, modelEvent)
		contextJSON, err := json.Marshal(map[string]interface{}{
			"context_ref_id": "providerctx:" + callID,
			"ref_type":       "provider_response",
			"ref_hash":       hashRef("provider:" + callID),
			"label":          firstNonEmpty(call.Provider, "provider") + " response " + callID,
			"privacy_label":  "local",
			"goal":           goal,
		})
		if err != nil {
			return nil, err
		}
		events = append(events, storage.CanonicalEvent{
			EventID:       "providerctx:" + callID,
			Source:        source,
			EventType:     "context.ref",
			SourceEventID: "providerctx:" + callID,
			WorkloadID:    workloadID,
			AgentRunID:    runID,
			SessionID:     call.SessionID,
			Model:         call.Model,
			Project:       call.Project,
			GitBranch:     modelEvent.GitBranch,
			Timestamp:     timestamp,
			Payload:       contextJSON,
			Confidence:    modelEvent.Confidence,
		})
	}
	return events, nil
}

func decodeProviderEntries(entries []json.RawMessage) ([]ProviderCall, error) {
	out := make([]ProviderCall, 0, len(entries))
	for _, entry := range entries {
		call, err := decodeProviderEntry(entry)
		if err != nil {
			return nil, err
		}
		out = append(out, call)
	}
	return out, nil
}

func decodeProviderEntry(raw json.RawMessage) (ProviderCall, error) {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return ProviderCall{}, err
	}
	metadata := map[string]interface{}{}
	if rawMetadata, ok := obj["metadata"]; ok {
		if typed, ok := rawMetadata.(map[string]interface{}); ok {
			metadata = typed
		}
	}
	provider := firstNonNilString(obj, "provider", "system", "gen_ai.provider.name")
	usage, schema := providerUsage(obj)
	if schema == "" {
		return ProviderCall{}, fmt.Errorf("provider usage object is required")
	}
	metadata["provider_usage_schema"] = schema
	if finish := firstNonNilString(obj, "finish_reason", "stop_reason"); finish != "" {
		metadata["finish_reason"] = finish
	}
	return ProviderCall{
		ID:        firstNonNilString(obj, "id", "response_id", "completion_id", "request_id"),
		Provider:  provider,
		Model:     firstNonNilString(obj, "model", "model_id", "modelID"),
		Project:   firstNonEmpty(metadataString(metadata, "agent_ledger.project", "project"), firstNonNilString(obj, "project")),
		SessionID: firstNonEmpty(metadataString(metadata, "agent_ledger.session_id", "session_id"), firstNonNilString(obj, "session_id")),
		Timestamp: providerTimestamp(obj),
		Usage:     usage,
		Metadata:  metadata,
	}, nil
}

func providerUsage(obj map[string]interface{}) (ProviderUsage, string) {
	usageRaw, ok := obj["usage"]
	if !ok {
		return ProviderUsage{}, ""
	}
	usage, ok := usageRaw.(map[string]interface{})
	if !ok {
		return ProviderUsage{}, ""
	}
	inputTotal := intFromMap(usage, "input_tokens", "prompt_tokens")
	cacheRead := nestedInt(usage, "input_tokens_details", "cached_tokens") +
		nestedInt(usage, "prompt_tokens_details", "cached_tokens") +
		intFromMap(usage, "cache_read_input_tokens", "cache_read_tokens")
	cacheWrite := intFromMap(usage, "cache_creation_input_tokens", "cache_write_input_tokens", "cache_write_tokens")
	output := intFromMap(usage, "output_tokens", "completion_tokens")
	reasoning := nestedInt(usage, "output_tokens_details", "reasoning_tokens") +
		nestedInt(usage, "completion_tokens_details", "reasoning_tokens") +
		intFromMap(usage, "reasoning_output_tokens")
	nonCachedInput := inputTotal - cacheRead - cacheWrite
	if nonCachedInput < 0 {
		nonCachedInput = 0
	}
	schema := "generic"
	if _, ok := usage["prompt_tokens"]; ok {
		schema = "openai-chat-completions"
	}
	if _, ok := usage["input_tokens"]; ok {
		schema = "openai-responses-or-anthropic"
	}
	if _, ok := usage["cache_creation_input_tokens"]; ok {
		schema = "anthropic"
	}
	return ProviderUsage{
		InputTokens:              nonCachedInput,
		CacheReadInputTokens:     cacheRead,
		CacheCreationInputTokens: cacheWrite,
		OutputTokens:             output,
		ReasoningOutputTokens:    reasoning,
		CostUSD:                  floatFromMap(usage, "cost_usd", "total_cost", "cost"),
	}, schema
}

func providerTimestamp(obj map[string]interface{}) time.Time {
	for _, key := range []string{"created_at", "created", "timestamp"} {
		value, ok := obj[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if parsed, err := time.Parse(time.RFC3339Nano, typed); err == nil {
				return parsed.UTC()
			}
			if seconds, err := strconv.ParseInt(typed, 10, 64); err == nil {
				return time.Unix(seconds, 0).UTC()
			}
		case float64:
			return time.Unix(int64(typed), 0).UTC()
		}
	}
	return time.Time{}
}

func firstNonNilString(obj map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value, ok := obj[key]; ok && value != nil {
			text := fmt.Sprint(value)
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func intFromMap(obj map[string]interface{}, keys ...string) int {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			return intValue(value)
		}
	}
	return 0
}

func floatFromMap(obj map[string]interface{}, keys ...string) float64 {
	for _, key := range keys {
		if value, ok := obj[key]; ok {
			switch typed := value.(type) {
			case float64:
				return typed
			case string:
				parsed, _ := strconv.ParseFloat(typed, 64)
				return parsed
			}
		}
	}
	return 0
}

func nestedInt(obj map[string]interface{}, key, child string) int {
	if value, ok := obj[key]; ok {
		if nested, ok := value.(map[string]interface{}); ok {
			return intFromMap(nested, child)
		}
	}
	return 0
}

func intValue(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case string:
		parsed, _ := strconv.Atoi(typed)
		return parsed
	default:
		return 0
	}
}

func providerConfidence(call ProviderCall) float64 {
	if call.Model != "" && (call.Usage.InputTokens+call.Usage.CacheReadInputTokens+call.Usage.CacheCreationInputTokens+call.Usage.OutputTokens) > 0 {
		return 0.9
	}
	return 0.65
}
