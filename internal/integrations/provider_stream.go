package integrations

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// DecodeProviderStream decodes a privacy-safe SSE transcript into one provider
// usage call. It reads only data lines and ignores text/content deltas.
func DecodeProviderStream(raw []byte) ([]ProviderCall, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty provider stream input")
	}
	var state providerStreamState
	scanner := bufio.NewScanner(bytes.NewReader(trimmed))
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "" || data == "[DONE]" {
			continue
		}
		state.ObserveData([]byte(data))
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(state.Usage) == 0 {
		return nil, fmt.Errorf("provider stream usage object is required")
	}
	usage, schema := providerUsage(map[string]interface{}{"usage": state.Usage})
	if schema == "" {
		return nil, fmt.Errorf("provider stream usage object is invalid")
	}
	id := firstNonEmpty(state.ID, deterministicLedgerID("stream", string(hashRef(string(trimmed)))))
	provider := firstNonEmpty(state.Provider, "provider-stream")
	metadata := map[string]interface{}{
		"provider_usage_schema":        firstNonEmpty(state.Schema, schema),
		"agent_ledger.source":          "provider-stream",
		"agent_ledger.source_version":  provider + "-sse-transcript",
		"agent_ledger.parser_version":  "agent-ledger-provider-stream@v1",
		"agent_ledger.raw_ref":         "provider-stream:" + provider + ":" + id,
		"agent_ledger.match_type":      "source_reported",
		"agent_ledger.goal":            "Provider stream model call " + id,
		"agent_ledger.project":         "adapter-fixture",
		"agent_ledger.provider_schema": firstNonEmpty(state.Schema, schema),
	}
	return []ProviderCall{{
		ID:       id,
		Provider: provider,
		Model:    state.Model,
		Project:  "adapter-fixture",
		Usage:    usage,
		Metadata: metadata,
	}}, nil
}

type providerStreamState struct {
	ID       string
	Provider string
	Model    string
	Schema   string
	Usage    map[string]interface{}
}

func (s *providerStreamState) ObserveData(raw []byte) {
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return
	}
	eventType := strings.TrimSpace(stringValue(obj["type"]))
	s.ID = firstNonEmpty(stringFromObject(obj, "id", "response_id", "request_id"), s.ID)
	s.Model = firstNonEmpty(stringFromObject(obj, "model", "model_id"), s.Model)
	if _, ok := obj["choices"]; ok {
		s.Provider = firstNonEmpty(s.Provider, "openai")
		s.Schema = firstNonEmpty(s.Schema, "openai-chat-completions")
	}
	if rawResponse, ok := objectValue(obj["response"]); ok {
		s.Provider = firstNonEmpty(s.Provider, "openai")
		s.Schema = firstNonEmpty(s.Schema, "openai-responses")
		s.ID = firstNonEmpty(stringFromObject(rawResponse, "id", "response_id"), s.ID)
		s.Model = firstNonEmpty(stringFromObject(rawResponse, "model", "model_id"), s.Model)
		s.MergeUsage(rawResponse["usage"])
	}
	if rawMessage, ok := objectValue(obj["message"]); ok {
		s.Provider = firstNonEmpty(s.Provider, "anthropic")
		s.Schema = firstNonEmpty(s.Schema, "anthropic")
		s.ID = firstNonEmpty(stringFromObject(rawMessage, "id", "response_id"), s.ID)
		s.Model = firstNonEmpty(stringFromObject(rawMessage, "model", "model_id"), s.Model)
		s.MergeUsage(rawMessage["usage"])
	}
	if rawDelta, ok := objectValue(obj["delta"]); ok {
		if strings.HasPrefix(eventType, "message_") {
			s.Provider = firstNonEmpty(s.Provider, "anthropic")
			s.Schema = firstNonEmpty(s.Schema, "anthropic")
		}
		s.MergeUsage(rawDelta["usage"])
	}
	if strings.HasPrefix(eventType, "message_") {
		s.Provider = firstNonEmpty(s.Provider, "anthropic")
		s.Schema = firstNonEmpty(s.Schema, "anthropic")
	}
	s.MergeUsage(obj["usage"])
}

func (s *providerStreamState) MergeUsage(value interface{}) {
	usage, ok := objectValue(value)
	if !ok {
		return
	}
	if s.Usage == nil {
		s.Usage = map[string]interface{}{}
	}
	for key, item := range usage {
		if item != nil {
			s.Usage[key] = item
		}
	}
}

func objectValue(value interface{}) (map[string]interface{}, bool) {
	if value == nil {
		return nil, false
	}
	typed, ok := value.(map[string]interface{})
	return typed, ok
}

func stringFromObject(obj map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if text := stringValue(obj[key]); text != "" {
			return text
		}
	}
	return ""
}

func stringValue(value interface{}) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}
