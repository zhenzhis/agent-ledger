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

type openAIResponsesGatewayRequest struct {
	Model    string                 `json:"model"`
	Stream   bool                   `json:"stream"`
	Metadata map[string]interface{} `json:"metadata"`
}

type anthropicMessagesGatewayRequest struct {
	Model    string                 `json:"model"`
	Stream   bool                   `json:"stream"`
	Metadata map[string]interface{} `json:"metadata"`
}

type gatewayLedgerContext struct {
	Project    string
	Goal       string
	WorkloadID string
	AgentRunID string
	SessionID  string
	GitBranch  string
}

const defaultAnthropicVersion = "2023-06-01"

const (
	gatewayUsageWarningMissingNonStream = "missing_or_unparseable_upstream_usage"
	gatewayUsageWarningMissingStream    = "missing_upstream_stream_usage"
)

func (s *Server) handleOpenAIChatGateway(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodPost) {
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
	ledgerCtx := gatewayContext(r, payload.Metadata, model)
	if !s.evaluateOperationPolicy(w, r, "model.call", "gateway", model, ledgerCtx.Project, "openai-chat-completions") {
		return
	}
	upstreamBody, streamUsageRequested, err := prepareOpenAIChatGatewayBody(raw, payload, cfg)
	if err != nil {
		badRequest(w, err)
		return
	}
	apiKey := strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
	if apiKey == "" {
		http.Error(w, "gateway upstream API key env var is not set", http.StatusServiceUnavailable)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.chat.config_error", model, map[string]string{"api_key_env": cfg.APIKeyEnv})
		return
	}
	upstreamURL := strings.TrimRight(cfg.UpstreamBaseURL, "/") + "/v1/chat/completions"
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(upstreamBody))
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
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.chat.upstream_error", model, map[string]string{"error": err.Error(), "project": ledgerCtx.Project})
		return
	}
	defer resp.Body.Close()
	if payload.Stream {
		s.handleOpenAIChatGatewayStream(w, resp, cfg, model, ledgerCtx, started, streamUsageRequested)
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
		recorded, eventCount = s.recordOpenAIChatGatewayUsage(respBody, model, ledgerCtx, started)
	} else {
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.chat.upstream_status", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
	}

	w.Header().Set("Content-Type", firstNonEmpty(resp.Header.Get("Content-Type"), "application/json"))
	w.Header().Set("X-Agent-Ledger-Usage-Recorded", fmt.Sprint(recorded))
	w.Header().Set("X-Agent-Ledger-Usage-Events", fmt.Sprint(eventCount))
	w.Header().Set("X-Agent-Ledger-Stream-Usage-Requested", fmt.Sprint(streamUsageRequested))
	w.Header().Set("X-Agent-Ledger-Upstream-Status", fmt.Sprint(resp.StatusCode))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && !recorded {
		w.Header().Set("X-Agent-Ledger-Usage-Warning", gatewayUsageWarningMissingNonStream)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func (s *Server) handleAnthropicMessagesGateway(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodPost) {
		return
	}
	cfg := normalizedAnthropicGatewayConfig(s.options.Gateway)
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
	var payload anthropicMessagesGatewayRequest
	if err := json.Unmarshal(raw, &payload); err != nil {
		badRequest(w, err)
		return
	}
	model := strings.TrimSpace(payload.Model)
	if model == "" {
		badRequest(w, fmt.Errorf("model is required"))
		return
	}
	ledgerCtx := gatewayContext(r, payload.Metadata, model)
	if !s.evaluateOperationPolicy(w, r, "model.call", "gateway", model, ledgerCtx.Project, "anthropic-messages") {
		return
	}
	apiKey := strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
	if apiKey == "" {
		http.Error(w, "gateway upstream API key env var is not set", http.StatusServiceUnavailable)
		s.appendAuditLog("local", s.roleFor(r), "gateway.anthropic.messages.config_error", model, map[string]string{"api_key_env": cfg.APIKeyEnv})
		return
	}
	upstreamURL := strings.TrimRight(cfg.UpstreamBaseURL, "/") + "/v1/messages"
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(raw))
	if err != nil {
		serverError(w, err)
		return
	}
	upReq.Header.Set("X-API-Key", apiKey)
	upReq.Header.Set("Content-Type", "application/json")
	if payload.Stream {
		upReq.Header.Set("Accept", "text/event-stream")
	} else {
		upReq.Header.Set("Accept", "application/json")
	}
	upReq.Header.Set("User-Agent", "agent-ledger-gateway")
	upReq.Header.Set("Anthropic-Version", firstNonEmpty(r.Header.Get("Anthropic-Version"), defaultAnthropicVersion))
	if beta := strings.TrimSpace(r.Header.Get("Anthropic-Beta")); beta != "" {
		upReq.Header.Set("Anthropic-Beta", beta)
	}

	started := time.Now().UTC()
	resp, err := (&http.Client{Timeout: cfg.Timeout}).Do(upReq)
	if err != nil {
		http.Error(w, "gateway upstream request failed", http.StatusBadGateway)
		s.appendAuditLog("local", s.roleFor(r), "gateway.anthropic.messages.upstream_error", model, map[string]string{"error": err.Error(), "project": ledgerCtx.Project})
		return
	}
	defer resp.Body.Close()
	if payload.Stream {
		s.handleAnthropicMessagesGatewayStream(w, resp, cfg, model, ledgerCtx, started)
		return
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, cfg.MaxResponseBytes+1))
	if err != nil {
		http.Error(w, "gateway upstream response read failed", http.StatusBadGateway)
		s.appendAuditLog("local", s.roleFor(r), "gateway.anthropic.messages.upstream_read_error", model, map[string]string{"error": err.Error(), "status": fmt.Sprint(resp.StatusCode)})
		return
	}
	if int64(len(respBody)) > cfg.MaxResponseBytes {
		http.Error(w, "gateway upstream response exceeded max_response_bytes", http.StatusBadGateway)
		s.appendAuditLog("local", s.roleFor(r), "gateway.anthropic.messages.response_too_large", model, map[string]string{"status": fmt.Sprint(resp.StatusCode)})
		return
	}
	recorded := false
	eventCount := 0
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		recorded, eventCount = s.recordAnthropicMessagesGatewayUsage(respBody, model, ledgerCtx, started)
	} else {
		s.appendAuditLog("local", s.roleFor(r), "gateway.anthropic.messages.upstream_status", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
	}
	w.Header().Set("Content-Type", firstNonEmpty(resp.Header.Get("Content-Type"), "application/json"))
	w.Header().Set("X-Agent-Ledger-Usage-Recorded", fmt.Sprint(recorded))
	w.Header().Set("X-Agent-Ledger-Usage-Events", fmt.Sprint(eventCount))
	w.Header().Set("X-Agent-Ledger-Upstream-Status", fmt.Sprint(resp.StatusCode))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && !recorded {
		w.Header().Set("X-Agent-Ledger-Usage-Warning", gatewayUsageWarningMissingNonStream)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func (s *Server) handleOpenAIResponsesGateway(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodPost) {
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
	var payload openAIResponsesGatewayRequest
	if err := json.Unmarshal(raw, &payload); err != nil {
		badRequest(w, err)
		return
	}
	model := strings.TrimSpace(payload.Model)
	if model == "" {
		badRequest(w, fmt.Errorf("model is required"))
		return
	}
	ledgerCtx := gatewayContext(r, payload.Metadata, model)
	if !s.evaluateOperationPolicy(w, r, "model.call", "gateway", model, ledgerCtx.Project, "openai-responses") {
		return
	}
	apiKey := strings.TrimSpace(os.Getenv(cfg.APIKeyEnv))
	if apiKey == "" {
		http.Error(w, "gateway upstream API key env var is not set", http.StatusServiceUnavailable)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.responses.config_error", model, map[string]string{"api_key_env": cfg.APIKeyEnv})
		return
	}
	upstreamURL := strings.TrimRight(cfg.UpstreamBaseURL, "/") + "/v1/responses"
	upReq, err := http.NewRequestWithContext(r.Context(), http.MethodPost, upstreamURL, bytes.NewReader(raw))
	if err != nil {
		serverError(w, err)
		return
	}
	upReq.Header.Set("Authorization", "Bearer "+apiKey)
	upReq.Header.Set("Content-Type", "application/json")
	if payload.Stream {
		upReq.Header.Set("Accept", "text/event-stream")
	} else {
		upReq.Header.Set("Accept", "application/json")
	}
	upReq.Header.Set("User-Agent", "agent-ledger-gateway")

	started := time.Now().UTC()
	resp, err := (&http.Client{Timeout: cfg.Timeout}).Do(upReq)
	if err != nil {
		http.Error(w, "gateway upstream request failed", http.StatusBadGateway)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.responses.upstream_error", model, map[string]string{"error": err.Error(), "project": ledgerCtx.Project})
		return
	}
	defer resp.Body.Close()
	if payload.Stream {
		s.handleOpenAIResponsesGatewayStream(w, resp, cfg, model, ledgerCtx, started)
		return
	}
	respBody, err := io.ReadAll(io.LimitReader(resp.Body, cfg.MaxResponseBytes+1))
	if err != nil {
		http.Error(w, "gateway upstream response read failed", http.StatusBadGateway)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.responses.upstream_read_error", model, map[string]string{"error": err.Error(), "status": fmt.Sprint(resp.StatusCode)})
		return
	}
	if int64(len(respBody)) > cfg.MaxResponseBytes {
		http.Error(w, "gateway upstream response exceeded max_response_bytes", http.StatusBadGateway)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.responses.response_too_large", model, map[string]string{"status": fmt.Sprint(resp.StatusCode)})
		return
	}
	recorded := false
	eventCount := 0
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		recorded, eventCount = s.recordOpenAIResponsesGatewayUsage(respBody, model, ledgerCtx, started)
	} else {
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "gateway.openai.responses.upstream_status", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
	}
	w.Header().Set("Content-Type", firstNonEmpty(resp.Header.Get("Content-Type"), "application/json"))
	w.Header().Set("X-Agent-Ledger-Usage-Recorded", fmt.Sprint(recorded))
	w.Header().Set("X-Agent-Ledger-Usage-Events", fmt.Sprint(eventCount))
	w.Header().Set("X-Agent-Ledger-Upstream-Status", fmt.Sprint(resp.StatusCode))
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && !recorded {
		w.Header().Set("X-Agent-Ledger-Usage-Warning", gatewayUsageWarningMissingNonStream)
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func prepareOpenAIChatGatewayBody(raw []byte, payload openAIChatGatewayRequest, cfg config.GatewayConfig) ([]byte, bool, error) {
	if !payload.Stream || !cfg.IncludeStreamUsage {
		return raw, false, nil
	}
	var obj map[string]interface{}
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false, err
	}
	var streamOptions map[string]interface{}
	if rawOptions, exists := obj["stream_options"]; exists && rawOptions != nil {
		typedOptions, ok := rawOptions.(map[string]interface{})
		if !ok {
			return raw, false, nil
		}
		streamOptions = typedOptions
	}
	if streamOptions == nil {
		streamOptions = map[string]interface{}{}
	}
	if _, ok := streamOptions["include_usage"]; ok {
		return raw, false, nil
	}
	streamOptions["include_usage"] = true
	obj["stream_options"] = streamOptions
	out, err := json.Marshal(obj)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

func (s *Server) recordOpenAIChatGatewayUsage(raw []byte, model string, ledgerCtx gatewayLedgerContext, started time.Time) (bool, int) {
	return s.recordOpenAIGatewayUsage(raw, model, ledgerCtx, started, "openai-compatible-gateway", "agent-ledger-openai-gateway@v1", "gateway.openai.chat")
}

func (s *Server) recordOpenAIResponsesGatewayUsage(raw []byte, model string, ledgerCtx gatewayLedgerContext, started time.Time) (bool, int) {
	return s.recordOpenAIGatewayUsage(raw, model, ledgerCtx, started, "openai-responses-gateway", "agent-ledger-openai-responses-gateway@v1", "gateway.openai.responses")
}

func (s *Server) recordOpenAIGatewayUsage(raw []byte, model string, ledgerCtx gatewayLedgerContext, started time.Time, sourceVersion, parserVersion, auditPrefix string) (bool, int) {
	calls, err := integrations.DecodeProviderCalls(raw)
	if err != nil {
		log.Printf("gateway usage decode failed: %v", err)
		_ = s.db.AppendAuditLog("local", "gateway", auditPrefix+".usage_decode_error", model, map[string]string{"error": err.Error(), "project": ledgerCtx.Project})
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
			calls[i].Project = ledgerCtx.Project
		}
		if strings.TrimSpace(calls[i].SessionID) == "" {
			calls[i].SessionID = ledgerCtx.SessionID
		}
		if calls[i].Timestamp.IsZero() {
			calls[i].Timestamp = started
		}
		if calls[i].Metadata == nil {
			calls[i].Metadata = map[string]interface{}{}
		}
		calls[i].Metadata["agent_ledger.source"] = "gateway"
		calls[i].Metadata["agent_ledger.source_version"] = sourceVersion
		calls[i].Metadata["agent_ledger.parser_version"] = parserVersion
		calls[i].Metadata["agent_ledger.match_type"] = "source_reported"
		calls[i].Metadata["agent_ledger.goal"] = ledgerCtx.Goal
		calls[i].Metadata["agent_ledger.project"] = ledgerCtx.Project
		calls[i].Metadata["agent_ledger.latency_ms"] = int64(time.Since(started) / time.Millisecond)
		setGatewayMetadata(calls[i].Metadata, "agent_ledger.workload_id", ledgerCtx.WorkloadID)
		setGatewayMetadata(calls[i].Metadata, "agent_ledger.agent_run_id", ledgerCtx.AgentRunID)
		setGatewayMetadata(calls[i].Metadata, "agent_ledger.git_branch", ledgerCtx.GitBranch)
		if calls[i].Usage.CostUSD == 0 {
			calls[i].Metadata["pricing_source"] = "unpriced"
			calls[i].Metadata["pricing_confidence"] = "needs-local-pricing"
		}
	}
	events, err := integrations.ConvertProviderCalls(calls)
	if err != nil {
		log.Printf("gateway usage conversion failed: %v", err)
		_ = s.db.AppendAuditLog("local", "gateway", auditPrefix+".usage_convert_error", model, map[string]string{"error": err.Error(), "project": ledgerCtx.Project})
		return false, 0
	}
	inserted := 0
	for _, event := range events {
		result, err := s.db.IngestCanonicalEvent(event)
		if err != nil {
			log.Printf("gateway usage ingest failed: %v", err)
			_ = s.db.AppendAuditLog("local", "gateway", auditPrefix+".usage_ingest_error", model, map[string]string{"error": err.Error(), "project": ledgerCtx.Project})
			return false, inserted
		}
		if result != nil && result.Status == "inserted" {
			inserted++
		}
	}
	_ = s.db.AppendAuditLog("local", "gateway", auditPrefix, model, map[string]string{"calls": fmt.Sprint(len(calls)), "events": fmt.Sprint(len(events)), "inserted": fmt.Sprint(inserted), "project": ledgerCtx.Project, "workload_id": ledgerCtx.WorkloadID, "run_id": ledgerCtx.AgentRunID})
	return len(events) > 0, len(events)
}

func (s *Server) recordAnthropicMessagesGatewayUsage(raw []byte, model string, ledgerCtx gatewayLedgerContext, started time.Time) (bool, int) {
	calls, err := integrations.DecodeProviderCalls(raw)
	if err != nil {
		log.Printf("anthropic gateway usage decode failed: %v", err)
		s.appendAuditLog("local", "gateway", "gateway.anthropic.messages.usage_decode_error", model, map[string]string{"error": err.Error(), "project": ledgerCtx.Project})
		return false, 0
	}
	for i := range calls {
		if strings.TrimSpace(calls[i].Provider) == "" {
			calls[i].Provider = "anthropic"
		}
		if strings.TrimSpace(calls[i].Model) == "" {
			calls[i].Model = model
		}
		if strings.TrimSpace(calls[i].Project) == "" {
			calls[i].Project = ledgerCtx.Project
		}
		if strings.TrimSpace(calls[i].SessionID) == "" {
			calls[i].SessionID = ledgerCtx.SessionID
		}
		if calls[i].Timestamp.IsZero() {
			calls[i].Timestamp = started
		}
		if calls[i].Metadata == nil {
			calls[i].Metadata = map[string]interface{}{}
		}
		calls[i].Metadata["agent_ledger.source"] = "gateway"
		calls[i].Metadata["agent_ledger.source_version"] = "anthropic-messages-gateway"
		calls[i].Metadata["agent_ledger.parser_version"] = "agent-ledger-anthropic-gateway@v1"
		calls[i].Metadata["agent_ledger.match_type"] = "source_reported"
		calls[i].Metadata["agent_ledger.goal"] = ledgerCtx.Goal
		calls[i].Metadata["agent_ledger.project"] = ledgerCtx.Project
		calls[i].Metadata["agent_ledger.latency_ms"] = int64(time.Since(started) / time.Millisecond)
		setGatewayMetadata(calls[i].Metadata, "agent_ledger.workload_id", ledgerCtx.WorkloadID)
		setGatewayMetadata(calls[i].Metadata, "agent_ledger.agent_run_id", ledgerCtx.AgentRunID)
		setGatewayMetadata(calls[i].Metadata, "agent_ledger.git_branch", ledgerCtx.GitBranch)
		if calls[i].Usage.CostUSD == 0 {
			calls[i].Metadata["pricing_source"] = "unpriced"
			calls[i].Metadata["pricing_confidence"] = "needs-local-pricing"
		}
	}
	events, err := integrations.ConvertProviderCalls(calls)
	if err != nil {
		log.Printf("anthropic gateway usage conversion failed: %v", err)
		s.appendAuditLog("local", "gateway", "gateway.anthropic.messages.usage_convert_error", model, map[string]string{"error": err.Error(), "project": ledgerCtx.Project})
		return false, 0
	}
	inserted := 0
	for _, event := range events {
		result, err := s.db.IngestCanonicalEvent(event)
		if err != nil {
			log.Printf("anthropic gateway usage ingest failed: %v", err)
			s.appendAuditLog("local", "gateway", "gateway.anthropic.messages.usage_ingest_error", model, map[string]string{"error": err.Error(), "project": ledgerCtx.Project})
			return false, inserted
		}
		if result != nil && result.Status == "inserted" {
			inserted++
		}
	}
	s.appendAuditLog("local", "gateway", "gateway.anthropic.messages", model, map[string]string{"calls": fmt.Sprint(len(calls)), "events": fmt.Sprint(len(events)), "inserted": fmt.Sprint(inserted), "project": ledgerCtx.Project, "workload_id": ledgerCtx.WorkloadID, "run_id": ledgerCtx.AgentRunID})
	return len(events) > 0, len(events)
}

func (s *Server) handleOpenAIChatGatewayStream(w http.ResponseWriter, resp *http.Response, cfg config.GatewayConfig, model string, ledgerCtx gatewayLedgerContext, started time.Time, streamUsageRequested bool) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this response writer", http.StatusInternalServerError)
		return
	}
	contentType := firstNonEmpty(resp.Header.Get("Content-Type"), "text/event-stream")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", firstNonEmpty(resp.Header.Get("Cache-Control"), "no-cache"))
	w.Header().Set("X-Agent-Ledger-Upstream-Status", fmt.Sprint(resp.StatusCode))
	w.Header().Set("X-Agent-Ledger-Stream-Usage-Requested", fmt.Sprint(streamUsageRequested))
	w.Header().Set("Trailer", "X-Agent-Ledger-Usage-Recorded, X-Agent-Ledger-Usage-Events, X-Agent-Ledger-Usage-Warning")
	w.WriteHeader(resp.StatusCode)

	var usage json.RawMessage
	responseID := ""
	responseModel := model
	var streamed int64
	usageWarning := ""
	reader := bufio.NewReader(io.LimitReader(resp.Body, cfg.MaxResponseBytes+1))
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			streamed += int64(len(line))
			if streamed > cfg.MaxResponseBytes {
				_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.stream_too_large", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
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
			_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.stream_read_error", model, map[string]string{"error": err.Error(), "status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
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
				recorded, eventCount = s.recordOpenAIChatGatewayUsage(body, responseModel, ledgerCtx, started)
			}
		} else {
			_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.stream_usage_missing", model, map[string]string{"project": ledgerCtx.Project})
			usageWarning = gatewayUsageWarningMissingStream
		}
	} else {
		_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.chat.upstream_status", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
	}
	w.Header().Set("X-Agent-Ledger-Usage-Recorded", fmt.Sprint(recorded))
	w.Header().Set("X-Agent-Ledger-Usage-Events", fmt.Sprint(eventCount))
	if usageWarning != "" {
		w.Header().Set("X-Agent-Ledger-Usage-Warning", usageWarning)
	}
}

func (s *Server) handleOpenAIResponsesGatewayStream(w http.ResponseWriter, resp *http.Response, cfg config.GatewayConfig, model string, ledgerCtx gatewayLedgerContext, started time.Time) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this response writer", http.StatusInternalServerError)
		return
	}
	contentType := firstNonEmpty(resp.Header.Get("Content-Type"), "text/event-stream")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", firstNonEmpty(resp.Header.Get("Cache-Control"), "no-cache"))
	w.Header().Set("X-Agent-Ledger-Upstream-Status", fmt.Sprint(resp.StatusCode))
	w.Header().Set("X-Agent-Ledger-Stream-Usage-Requested", "false")
	w.Header().Set("Trailer", "X-Agent-Ledger-Usage-Recorded, X-Agent-Ledger-Usage-Events, X-Agent-Ledger-Usage-Warning")
	w.WriteHeader(resp.StatusCode)

	var usage json.RawMessage
	responseID := ""
	responseModel := model
	var streamed int64
	usageWarning := ""
	reader := bufio.NewReader(io.LimitReader(resp.Body, cfg.MaxResponseBytes+1))
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			streamed += int64(len(line))
			if streamed > cfg.MaxResponseBytes {
				_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.responses.stream_too_large", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
				break
			}
			_, _ = w.Write(line)
			flusher.Flush()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				if id, chunkModel, chunkUsage := openAIResponsesStreamUsage(line); len(chunkUsage) > 0 {
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
			_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.responses.stream_read_error", model, map[string]string{"error": err.Error(), "status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
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
				recorded, eventCount = s.recordOpenAIResponsesGatewayUsage(body, responseModel, ledgerCtx, started)
			}
		} else {
			_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.responses.stream_usage_missing", model, map[string]string{"project": ledgerCtx.Project})
			usageWarning = gatewayUsageWarningMissingStream
		}
	} else {
		_ = s.db.AppendAuditLog("local", "gateway", "gateway.openai.responses.upstream_status", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
	}
	w.Header().Set("X-Agent-Ledger-Usage-Recorded", fmt.Sprint(recorded))
	w.Header().Set("X-Agent-Ledger-Usage-Events", fmt.Sprint(eventCount))
	if usageWarning != "" {
		w.Header().Set("X-Agent-Ledger-Usage-Warning", usageWarning)
	}
}

func (s *Server) handleAnthropicMessagesGatewayStream(w http.ResponseWriter, resp *http.Response, cfg config.GatewayConfig, model string, ledgerCtx gatewayLedgerContext, started time.Time) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming is not supported by this response writer", http.StatusInternalServerError)
		return
	}
	contentType := firstNonEmpty(resp.Header.Get("Content-Type"), "text/event-stream")
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", firstNonEmpty(resp.Header.Get("Cache-Control"), "no-cache"))
	w.Header().Set("X-Agent-Ledger-Upstream-Status", fmt.Sprint(resp.StatusCode))
	w.Header().Set("X-Agent-Ledger-Stream-Usage-Requested", "false")
	w.Header().Set("Trailer", "X-Agent-Ledger-Usage-Recorded, X-Agent-Ledger-Usage-Events, X-Agent-Ledger-Usage-Warning")
	w.WriteHeader(resp.StatusCode)

	var state anthropicStreamUsageState
	var streamed int64
	usageWarning := ""
	reader := bufio.NewReader(io.LimitReader(resp.Body, cfg.MaxResponseBytes+1))
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			streamed += int64(len(line))
			if streamed > cfg.MaxResponseBytes {
				s.appendAuditLog("local", "gateway", "gateway.anthropic.messages.stream_too_large", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
				break
			}
			_, _ = w.Write(line)
			flusher.Flush()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				state.Observe(line)
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			s.appendAuditLog("local", "gateway", "gateway.anthropic.messages.stream_read_error", model, map[string]string{"error": err.Error(), "status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
			break
		}
	}

	recorded := false
	eventCount := 0
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if body, ok := state.ProviderBody(model, started); ok {
			recorded, eventCount = s.recordAnthropicMessagesGatewayUsage(body, firstNonEmpty(state.Model, model), ledgerCtx, started)
		} else {
			s.appendAuditLog("local", "gateway", "gateway.anthropic.messages.stream_usage_missing", model, map[string]string{"project": ledgerCtx.Project})
			usageWarning = gatewayUsageWarningMissingStream
		}
	} else {
		s.appendAuditLog("local", "gateway", "gateway.anthropic.messages.upstream_status", model, map[string]string{"status": fmt.Sprint(resp.StatusCode), "project": ledgerCtx.Project})
	}
	w.Header().Set("X-Agent-Ledger-Usage-Recorded", fmt.Sprint(recorded))
	w.Header().Set("X-Agent-Ledger-Usage-Events", fmt.Sprint(eventCount))
	if usageWarning != "" {
		w.Header().Set("X-Agent-Ledger-Usage-Warning", usageWarning)
	}
}

type anthropicStreamUsageState struct {
	ID    string
	Model string
	Usage map[string]interface{}
}

func (s *anthropicStreamUsageState) Observe(line []byte) {
	text := strings.TrimSpace(string(line))
	if !strings.HasPrefix(text, "data:") {
		return
	}
	data := strings.TrimSpace(strings.TrimPrefix(text, "data:"))
	if data == "" || data == "[DONE]" {
		return
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(data), &obj); err != nil {
		return
	}
	s.ID = firstNonEmpty(rawJSONText(obj["id"]), s.ID)
	s.Model = firstNonEmpty(rawJSONText(obj["model"]), s.Model)
	if rawMessage, ok := obj["message"]; ok {
		var message map[string]json.RawMessage
		if err := json.Unmarshal(rawMessage, &message); err == nil {
			s.ID = firstNonEmpty(rawJSONText(message["id"]), s.ID)
			s.Model = firstNonEmpty(rawJSONText(message["model"]), s.Model)
			s.MergeUsage(message["usage"])
		}
	}
	if rawDelta, ok := obj["delta"]; ok {
		var delta map[string]json.RawMessage
		if err := json.Unmarshal(rawDelta, &delta); err == nil {
			s.MergeUsage(delta["usage"])
		}
	}
	s.MergeUsage(obj["usage"])
}

func (s *anthropicStreamUsageState) MergeUsage(raw json.RawMessage) {
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	var usage map[string]interface{}
	if err := json.Unmarshal(raw, &usage); err != nil {
		return
	}
	if s.Usage == nil {
		s.Usage = map[string]interface{}{}
	}
	for key, value := range usage {
		if value != nil {
			s.Usage[key] = value
		}
	}
}

func (s anthropicStreamUsageState) ProviderBody(fallbackModel string, started time.Time) ([]byte, bool) {
	if len(s.Usage) == 0 {
		return nil, false
	}
	body, err := json.Marshal(map[string]interface{}{
		"id":    firstNonEmpty(s.ID, "stream-"+started.Format("20060102T150405.000000000Z")),
		"type":  "message",
		"model": firstNonEmpty(s.Model, fallbackModel),
		"usage": s.Usage,
	})
	if err != nil {
		return nil, false
	}
	return body, true
}

func rawJSONText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return strings.TrimSpace(text)
	}
	return ""
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

func openAIResponsesStreamUsage(line []byte) (id, model string, usage json.RawMessage) {
	text := strings.TrimSpace(string(line))
	if !strings.HasPrefix(text, "data:") {
		return "", "", nil
	}
	data := strings.TrimSpace(strings.TrimPrefix(text, "data:"))
	if data == "" || data == "[DONE]" {
		return "", "", nil
	}
	var chunk struct {
		ID       string          `json:"id"`
		Model    string          `json:"model"`
		Usage    json.RawMessage `json:"usage"`
		Response struct {
			ID    string          `json:"id"`
			Model string          `json:"model"`
			Usage json.RawMessage `json:"usage"`
		} `json:"response"`
	}
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return "", "", nil
	}
	if len(chunk.Usage) > 0 && string(chunk.Usage) != "null" {
		return chunk.ID, chunk.Model, chunk.Usage
	}
	if len(chunk.Response.Usage) > 0 && string(chunk.Response.Usage) != "null" {
		return chunk.Response.ID, chunk.Response.Model, chunk.Response.Usage
	}
	return "", "", nil
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

func normalizedAnthropicGatewayConfig(cfg config.GatewayConfig) config.GatewayConfig {
	if cfg.AnthropicUpstreamBaseURL == "" {
		cfg.AnthropicUpstreamBaseURL = "https://api.anthropic.com"
	}
	if cfg.AnthropicAPIKeyEnv == "" {
		cfg.AnthropicAPIKeyEnv = "ANTHROPIC_API_KEY"
	}
	cfg.UpstreamBaseURL = cfg.AnthropicUpstreamBaseURL
	cfg.APIKeyEnv = cfg.AnthropicAPIKeyEnv
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

func gatewayContext(r *http.Request, metadata map[string]interface{}, model string) gatewayLedgerContext {
	return gatewayLedgerContext{
		Project:    firstNonEmpty(gatewayQuery(r, "project"), gatewayMetadataString(metadata, "agent_ledger.project", "project")),
		Goal:       firstNonEmpty(gatewayQuery(r, "goal"), gatewayMetadataString(metadata, "agent_ledger.goal", "goal"), "Gateway model call "+model),
		WorkloadID: firstNonEmpty(gatewayQuery(r, "workload_id"), gatewayMetadataString(metadata, "agent_ledger.workload_id", "workload_id")),
		AgentRunID: firstNonEmpty(gatewayQuery(r, "agent_run_id"), gatewayQuery(r, "run_id"), gatewayMetadataString(metadata, "agent_ledger.agent_run_id", "agent_run_id", "run_id")),
		SessionID:  firstNonEmpty(gatewayQuery(r, "session_id"), gatewayMetadataString(metadata, "agent_ledger.session_id", "session_id")),
		GitBranch:  firstNonEmpty(gatewayQuery(r, "git_branch"), gatewayQuery(r, "branch"), gatewayMetadataString(metadata, "agent_ledger.git_branch", "git_branch", "branch")),
	}
}

func gatewayQuery(r *http.Request, key string) string {
	if r == nil {
		return ""
	}
	return strings.TrimSpace(r.URL.Query().Get(key))
}

func setGatewayMetadata(metadata map[string]interface{}, key, value string) {
	if strings.TrimSpace(value) != "" {
		metadata[key] = value
	}
}
