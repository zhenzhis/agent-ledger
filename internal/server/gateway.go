package server

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
)

type openAIChatGatewayRequest struct {
	Model    string                 `json:"model"`
	Stream   bool                   `json:"stream"`
	Metadata map[string]interface{} `json:"metadata"`
}

func (s *Server) handleOpenAIChatGateway(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg := normalizedGatewayConfig(s.options.Gateway)
	if !cfg.Enabled {
		http.Error(w, "gateway is disabled; set gateway.enabled=true", http.StatusNotFound)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	if contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0])); contentType != "application/json" {
		http.Error(w, "gateway requires application/json requests", http.StatusUnsupportedMediaType)
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes))
	if err != nil {
		badRequest(w, err)
		return
	}
	var payload openAIChatGatewayRequest
	if err := json.Unmarshal(raw, &payload); err != nil {
		badRequest(w, err)
		return
	}
	model := strings.TrimSpace(payload.Model)
	if model == "" {
		badRequest(w, fmt.Errorf("model is required"))
		return
	}
	project := firstNonEmpty(r.URL.Query().Get("project"), gatewayMetadataString(payload.Metadata, "agent_ledger.project", "project"))
	goal := firstNonEmpty(r.URL.Query().Get("goal"), gatewayMetadataString(payload.Metadata, "agent_ledger.goal", "goal"), "Gateway model call "+model)
	if !s.evaluateOperationPolicy(w, r, "model.call", "gateway", model, project, "openai-chat-completions") {
		return
	}
	apiKey := strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
	if apiKey == "" {
		http.Error(w, "gateway upstream API key env var is not set", http.StatusServiceUnavailable)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.chat.config_error", model, map[string]string{"api_key_env": cfg.APIKeyEnv})
		return
	}
	upstreamURL := strings.TrimRight(cfg.UpstreamBaseURL, "/") + "/v1/chat/completions"
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(raw))
	if err != nil {
		serverError(w, err)
		return
	}
	upReq.Header.Set("Authorization", "Bearer "+apiKey)
	upReq.Header.Set("Content-Type", "application/json")
	upReq.Header.Set("Accept", "application/json")
	upReq.Header.Set("User-Agent", "agent-ledger-gateway")

	started := time.Now().UTC()
	resp, err := (&http.Client{Timeout: cfg.Timeout}).Do(upReq)
	if err != nil {
		http.Error(w, "gateway upstream request failed", http.StatusBadGateway)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.chat.upstream_error", model, map[string]string{"error": err.Error(), "project": project})
		return
	}
	defer resp.Body.Close()
	if payload.Stream {
		s.handleOpenAIChatGatewayStream(w, resp, cfg, model, project, goal, started)
		return
	}

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, cfg.MaxResponseBytes+1))
	if err != nil {
		http.Error(w, "gateway upstream response read failed", http.StatusBadGateway)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.chat.upstream_read_error", model, map[string]string{"error": err.Error(), "status": fmt.Sprint(resp.StatusCode)})
		return
	}
	if int64(len(respBody)) > cfg.MaxResponseBytes {
		http.Error(w, "gateway upstream response exceeded max_response_bytes", http.StatusBadGateway)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.chat.response_too_large", model, map[string]string{"status": fmt.Sprint(resp.StatusCode)})
		return
	}

	recorded := false
	eventCount := 0
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		recorded, eventCount = s.recordOpenAIChatGatewayUsage(respBody, model, project, goal, started)
	} else {
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.chat.upstream_status", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": project})
	}

	w.Header().Set("Content-Type", firstNonEmpty(resp.Header.Get("Content-Type"), "application/json"))
	w.Header().Set("X-Agent-Ledger-Usage-Recorded", fmt.Sprint(recorded))
	w.Header().Set("X-Agent-Ledger-Usage-Events", fmt.Sprint(eventCount))
	w.Header().Set("X-Agent-Ledger-Upstream-Status", fmt.Sprint(resp.StatusCode))
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func (s *Server) recordOpenAIChatGatewayUsage(raw []byte, model, project, goal string, started time.Time) (bool, int) {
	calls, err := integrations.DecodeProviderCalls(raw)
	if err != nil {
		log.Printf("gateway usage decode failed: %v", err)
		_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.usage_decode_error", model, map[string]string{"error": err.Error(), "project": project})
		return false, 0
	}
	for i := range calls {
		if strings.TrimSpace(calls[i].Provider) == "" {
			calls[i].Provider = "openai"
		}
		if strings.TrimSpace(calls[i].Model) == "" {
			calls[i].Model = model
		}
		if strings.TrimSpace(calls[i].Project) == "" {
			calls[i].Project = project
		}
		if calls[i].Timestamp.IsZero() {
			calls[i].Timestamp = started
		}
		if calls[i].Metadata == nil {
			calls[i].Metadata = map[string]interface{}{}
		}
		calls[i].Metadata["agent_ledger.source"] = "gateway"
		calls[i].Metadata["agent_ledger.goal"] = goal
		calls[i].Metadata["agent_ledger.project"] = project
		calls[i].Metadata["agent_ledger.latency_ms"] = int64(time.Since(started) / time.Millisecond)
		if calls[i].Usage.CostUSD == 0 {
			calls[i].Metadata["pricing_source"] = "unpriced"
			calls[i].Metadata["pricing_confidence"] = "needs-local-pricing"
		}
	}
	events, err := integrations.ConvertProviderCalls(calls)
	if err != nil {
		log.Printf("gateway usage conversion failed: %v", err)
		_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.usage_convert_error", model, map[string]string{"error": err.Error(), "project": project})
		return false, 0
	}
	inserted := 0
	for _, event := range events {
		result, err := s.db.IngestCanonicalEvent(event)
		if err != nil {
			log.Printf("gateway usage ingest failed: %v", err)
			_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.usage_ingest_error", model, map[string]string{"error": err.Error(), "project": project})
			return false, inserted
		}
		if result != nil && result.Status == "inserted" {
			inserted++
		}
	}
	_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat", model, map[string]string{"calls": fmt.Sprint(len(calls)), "events": fmt.Sprint(len(events)), "inserted": fmt.Sprint(inserted), "project": project})
	return len(events) > 0, len(events)
}

func (s *Server) handleOpenAIChatGatewayStream(w http.ResponseWriter, resp *http.Response, cfg config.GatewayConfig, model, project, goal string, started time.Time) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this response writer", http.StatusInternalServerError)
		return
	}
	contentType := firstNonEmpty(resp.Header.Get("Content-Type"), "text/event-stream")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", firstNonEmpty(resp.Header.Get("Cache-Control"), "no-cache"))
	w.Header().Set("X-Agent-Ledger-Upstream-Status", fmt.Sprint(resp.StatusCode))
	w.Header().Set("Trailer", "X-Agent-Ledger-Usage-Recorded, X-Agent-Ledger-Usage-Events")
	w.WriteHeader(resp.StatusCode)

	var usage json.RawMessage
	responseID := ""
	responseModel := model
	var streamed int64
	reader := bufio.NewReader(io.LimitReader(resp.Body, cfg.MaxResponseBytes+1))
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			streamed += int64(len(line))
			if streamed > cfg.MaxResponseBytes {
				_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.stream_too_large", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": project})
				break
			}
			_, _ = w.Write(line)
			flusher.Flush()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if id, chunkModel, chunkUsage := openAIStreamUsage(line); len(chunkUsage) > 0 {
					usage = chunkUsage
					responseID = firstNonEmpty(id, responseID)
					responseModel = firstNonEmpty(chunkModel, responseModel)
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.stream_read_error", model, map[string]string{"error": err.Error(), "status": fmt.Sprint(resp.StatusCode), "project": project})
			break
		}
	}

	recorded := false
	eventCount := 0
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if len(usage) > 0 {
			body, err := json.Marshal(map[string]interface{}{
				"id":    firstNonEmpty(responseID, "stream-"+started.Format("20060102T150405.000000000Z")),
				"model": responseModel,
				"usage": json.RawMessage(usage),
			})
			if err == nil {
				recorded, eventCount = s.recordOpenAIChatGatewayUsage(body, responseModel, project, goal, started)
			}
		} else {
			_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.stream_usage_missing", model, map[string]string{"project": project})
		}
	} else {
		_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.upstream_status", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": project})
	}
	w.Header().Set("X-Agent-Ledger-Usage-Recorded", fmt.Sprint(recorded))
	w.Header().Set("X-Agent-Ledger-Usage-Events", fmt.Sprint(eventCount))
}

func openAIStreamUsage(line []byte) (id, model string, usage json.RawMessage) {
	text := strings.TrimSpace(string(line))
	if !strings.HasPrefix(text, "data:") {
		return "", "", nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(text, "data:"))
	if data == "" || data == "[DONE]" {
		return "", "", nil
	}
	var chunk struct {
		ID    string          `json:"id"`
		Model string          `json:"model"`
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return "", "", nil
	}
	if len(chunk.Usage) == 0 || string(chunk.Usage) == "null" {
		return "", "", nil
	}
	return chunk.ID, chunk.Model, chunk.Usage
}

func normalizedGatewayConfig(cfg config.GatewayConfig) config.GatewayConfig {
	if cfg.UpstreamBaseURL == "" {
		cfg.UpstreamBaseURL = "https://api.openai.com"
	}
	if cfg.APIKeyEnv == "" {
		cfg.APIKeyEnv = "OPENAI_API_KEY"
	}
	if cfg.MaxBodyBytes <= 0 {
		cfg.MaxBodyBytes = 4 << 20
	}
	if cfg.MaxResponseBytes <= 0 {
		cfg.MaxResponseBytes = 32 << 20
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 120 * time.Second
	}
	return cfg
}

func gatewayMetadataString(metadata map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := metadata[key]
		if !ok || value == nil {
			continue
		}
		text := strings.TrimSpace(fmt.Sprint(value))
		if text != "" && text != "<nil>" {
			return text
		}
	}
	return ""
}
