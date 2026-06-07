package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
)

func TestGatewayDisabledByDefault(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/gateway/openai/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOpenAIChatGateway(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("expected disabled gateway 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGatewayStreamsAndRecordsUsage(t *testing.T) {
	db := testServerDB(t)
	t.Setenv("AGENT_LEDGER_TEST_OPENAI_KEY", "sk-test")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var upstreamBody map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&upstreamBody); err != nil {
			t.Fatalf("decode upstream body: %v", err)
		}
		streamOptions, ok := upstreamBody["stream_options"].(map[string]interface{})
		if !ok || streamOptions["include_usage"] != true {
			t.Fatalf("gateway did not request final stream usage chunk: %+v", upstreamBody["stream_options"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl_stream_test\",\"model\":\"gpt-5.5\",\"choices\":[{\"delta\":{\"content\":\"secret streamed response must not persist\"}}],\"usage\":null}\n\n"))
		_, _ = w.Write([]byte("data: {\"id\":\"chatcmpl_stream_test\",\"model\":\"gpt-5.5\",\"choices\":[],\"usage\":{\"prompt_tokens\":30,\"completion_tokens\":7,\"prompt_tokens_details\":{\"cached_tokens\":5}}}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer upstream.Close()
	srv := New(db, "", Options{Gateway: testGatewayConfig(upstream.URL)})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/gateway/openai/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5","stream":true,"messages":[{"role":"user","content":"secret streamed prompt must not persist"}],"metadata":{"agent_ledger.project":"gateway-project","agent_ledger.goal":"gateway stream"}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOpenAIChatGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "secret streamed response must not persist") {
		t.Fatalf("stream body was not proxied: %s", rr.Body.String())
	}
	usageRows, err := db.GetModelCalls(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "gateway", "gpt-5.5", "gateway-project", 10)
	if err != nil {
		t.Fatalf("GetModelCalls: %v", err)
	}
	if len(usageRows) != 1 || usageRows[0].Calls != 1 || usageRows[0].Tokens != 37 {
		t.Fatalf("unexpected streamed usage projection: %+v", usageRows)
	}
	audit, err := db.GetAuditLog(20)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if strings.Contains(string(rawAudit), "secret streamed prompt") || strings.Contains(string(rawAudit), "secret streamed response") || strings.Contains(string(rawAudit), "sk-test") {
		t.Fatalf("sensitive streamed gateway data leaked into audit log: %s", string(rawAudit))
	}
}

func TestPrepareOpenAIChatGatewayBodyStreamUsage(t *testing.T) {
	baseConfig := testGatewayConfig("http://127.0.0.1")
	tests := []struct {
		name      string
		raw       string
		cfg       config.GatewayConfig
		requested bool
		wantUsage interface{}
	}{
		{
			name:      "injects when streaming and absent",
			raw:       `{"model":"gpt-5.5","stream":true,"messages":[]}`,
			cfg:       baseConfig,
			requested: true,
			wantUsage: true,
		},
		{
			name:      "preserves explicit false",
			raw:       `{"model":"gpt-5.5","stream":true,"stream_options":{"include_usage":false},"messages":[]}`,
			cfg:       baseConfig,
			requested: false,
			wantUsage: false,
		},
		{
			name:      "preserves explicit true",
			raw:       `{"model":"gpt-5.5","stream":true,"stream_options":{"include_usage":true},"messages":[]}`,
			cfg:       baseConfig,
			requested: false,
			wantUsage: true,
		},
		{
			name: "does not inject when config disabled",
			raw:  `{"model":"gpt-5.5","stream":true,"messages":[]}`,
			cfg: func() config.GatewayConfig {
				cfg := baseConfig
				cfg.IncludeStreamUsage = false
				return cfg
			}(),
			requested: false,
			wantUsage: nil,
		},
		{
			name:      "does not inject for non-streaming",
			raw:       `{"model":"gpt-5.5","messages":[]}`,
			cfg:       baseConfig,
			requested: false,
			wantUsage: nil,
		},
		{
			name:      "preserves non-object stream options",
			raw:       `{"model":"gpt-5.5","stream":true,"stream_options":"custom","messages":[]}`,
			cfg:       baseConfig,
			requested: false,
			wantUsage: nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var payload openAIChatGatewayRequest
			if err := json.Unmarshal([]byte(tc.raw), &payload); err != nil {
				t.Fatalf("unmarshal payload: %v", err)
			}
			out, requested, err := prepareOpenAIChatGatewayBody([]byte(tc.raw), payload, tc.cfg)
			if err != nil {
				t.Fatalf("prepare body: %v", err)
			}
			if requested != tc.requested {
				t.Fatalf("requested=%v want %v", requested, tc.requested)
			}
			var body map[string]interface{}
			if err := json.Unmarshal(out, &body); err != nil {
				t.Fatalf("unmarshal output: %v", err)
			}
			streamOptions, _ := body["stream_options"].(map[string]interface{})
			var got interface{}
			if streamOptions != nil {
				got = streamOptions["include_usage"]
			}
			if got != tc.wantUsage {
				t.Fatalf("include_usage=%v want %v body=%s", got, tc.wantUsage, string(out))
			}
		})
	}
}

func TestGatewayProxiesAndRecordsUsage(t *testing.T) {
	db := testServerDB(t)
	t.Setenv("AGENT_LEDGER_TEST_OPENAI_KEY", "sk-test")
	upstreamAuth := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/chat/completions" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_gateway_test",
			"model":"gpt-5.5",
			"choices":[{"message":{"role":"assistant","content":"secret response text must not persist"}}],
			"usage":{"prompt_tokens":20,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":4}}
		}`))
	}))
	defer upstream.Close()

	srv := New(db, "", Options{Gateway: testGatewayConfig(upstream.URL)})
	body := `{"model":"gpt-5.5","messages":[{"role":"user","content":"secret user prompt must not persist"}],"metadata":{"agent_ledger.project":"gateway-project","agent_ledger.goal":"gateway smoke"}}`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/gateway/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOpenAIChatGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamAuth != "Bearer sk-test" {
		t.Fatalf("upstream auth header was not set correctly")
	}
	if rr.Header().Get("X-Agent-Ledger-Usage-Recorded") != "true" {
		t.Fatalf("usage recorded header=%q", rr.Header().Get("X-Agent-Ledger-Usage-Recorded"))
	}
	page, err := db.GetWorkloadsPage(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "gateway", "", "gateway-project", "", "", 10, 0)
	if err != nil {
		t.Fatalf("GetWorkloadsPage: %v", err)
	}
	if page.Total != 1 || len(page.Rows) != 1 || page.Rows[0].Tokens != 25 {
		t.Fatalf("unexpected gateway workload page: %+v", page)
	}
	usageRows, err := db.GetModelCalls(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "gateway", "gpt-5.5", "gateway-project", 10)
	if err != nil {
		t.Fatalf("GetModelCalls: %v", err)
	}
	if len(usageRows) != 1 || usageRows[0].Calls != 1 || usageRows[0].Tokens != 25 {
		t.Fatalf("unexpected gateway usage projection: %+v", usageRows)
	}
	audit, err := db.GetAuditLog(20)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if strings.Contains(string(rawAudit), "secret user prompt") || strings.Contains(string(rawAudit), "secret response text") || strings.Contains(string(rawAudit), "sk-test") {
		t.Fatalf("sensitive gateway data leaked into audit log: %s", string(rawAudit))
	}
}

func TestOpenAIResponsesGatewayProxiesAndRecordsUsage(t *testing.T) {
	db := testServerDB(t)
	t.Setenv("AGENT_LEDGER_TEST_OPENAI_KEY", "sk-test")
	upstreamAuth := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamAuth = r.Header.Get("Authorization")
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"resp_gateway_test",
			"model":"gpt-5.5",
			"output":[{"type":"message","content":[{"type":"output_text","text":"secret responses output must not persist"}]}],
			"usage":{"input_tokens":50,"input_tokens_details":{"cached_tokens":5},"output_tokens":12,"output_tokens_details":{"reasoning_tokens":2}}
		}`))
	}))
	defer upstream.Close()

	srv := New(db, "", Options{Gateway: testGatewayConfig(upstream.URL)})
	body := `{"model":"gpt-5.5","input":"secret responses prompt must not persist","metadata":{"agent_ledger.project":"responses-gateway-project","agent_ledger.goal":"responses gateway smoke"}}`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/gateway/openai/v1/responses", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOpenAIResponsesGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamAuth != "Bearer sk-test" {
		t.Fatalf("upstream auth header was not set correctly")
	}
	if rr.Header().Get("X-Agent-Ledger-Usage-Recorded") != "true" {
		t.Fatalf("usage recorded header=%q", rr.Header().Get("X-Agent-Ledger-Usage-Recorded"))
	}
	usageRows, err := db.GetModelCalls(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "gateway", "gpt-5.5", "responses-gateway-project", 10)
	if err != nil {
		t.Fatalf("GetModelCalls: %v", err)
	}
	if len(usageRows) != 1 || usageRows[0].Calls != 1 || usageRows[0].Tokens != 62 {
		t.Fatalf("unexpected responses gateway usage projection: %+v", usageRows)
	}
	audit, err := db.GetAuditLog(20)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if strings.Contains(string(rawAudit), "secret responses prompt") || strings.Contains(string(rawAudit), "secret responses output") || strings.Contains(string(rawAudit), "sk-test") {
		t.Fatalf("sensitive responses gateway data leaked into audit log: %s", string(rawAudit))
	}
}

func TestOpenAIResponsesGatewayStreamsAndRecordsUsage(t *testing.T) {
	db := testServerDB(t)
	t.Setenv("AGENT_LEDGER_TEST_OPENAI_KEY", "sk-test")
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/responses" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		if r.Header.Get("Accept") != "text/event-stream" {
			t.Fatalf("gateway did not request event stream: %q", r.Header.Get("Accept"))
		}
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("event: response.output_text.delta\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.output_text.delta\",\"delta\":\"secret streamed responses output must not persist\"}\n\n"))
		_, _ = w.Write([]byte("event: response.completed\n"))
		_, _ = w.Write([]byte("data: {\"type\":\"response.completed\",\"response\":{\"id\":\"resp_stream_gateway_test\",\"model\":\"gpt-5.5\",\"usage\":{\"input_tokens\":30,\"input_tokens_details\":{\"cached_tokens\":3},\"output_tokens\":9}}}\n\n"))
	}))
	defer upstream.Close()
	srv := New(db, "", Options{Gateway: testGatewayConfig(upstream.URL)})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/gateway/openai/v1/responses", strings.NewReader(`{"model":"gpt-5.5","stream":true,"input":"secret streamed responses prompt must not persist","metadata":{"agent_ledger.project":"responses-stream-project","agent_ledger.goal":"responses stream"}}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOpenAIResponsesGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "secret streamed responses output must not persist") {
		t.Fatalf("stream body was not proxied: %s", rr.Body.String())
	}
	usageRows, err := db.GetModelCalls(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "gateway", "gpt-5.5", "responses-stream-project", 10)
	if err != nil {
		t.Fatalf("GetModelCalls: %v", err)
	}
	if len(usageRows) != 1 || usageRows[0].Calls != 1 || usageRows[0].Tokens != 39 {
		t.Fatalf("unexpected responses stream usage projection: %+v", usageRows)
	}
	audit, err := db.GetAuditLog(20)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if strings.Contains(string(rawAudit), "secret streamed responses prompt") || strings.Contains(string(rawAudit), "secret streamed responses output") || strings.Contains(string(rawAudit), "sk-test") {
		t.Fatalf("sensitive responses stream data leaked into audit log: %s", string(rawAudit))
	}
}

func TestAnthropicGatewayProxiesAndRecordsUsage(t *testing.T) {
	db := testServerDB(t)
	t.Setenv("AGENT_LEDGER_TEST_ANTHROPIC_KEY", "sk-ant-test")
	upstreamKey := ""
	upstreamVersion := ""
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamKey = r.Header.Get("X-API-Key")
		upstreamVersion = r.Header.Get("Anthropic-Version")
		if r.URL.Path != "/v1/messages" {
			t.Fatalf("unexpected upstream path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"msg_gateway_test",
			"type":"message",
			"model":"claude-opus-4-7",
			"content":[{"type":"text","text":"secret anthropic response must not persist"}],
			"usage":{"input_tokens":100,"cache_read_input_tokens":7,"cache_creation_input_tokens":3,"output_tokens":20}
		}`))
	}))
	defer upstream.Close()

	srv := New(db, "", Options{Gateway: testAnthropicGatewayConfig(upstream.URL)})
	body := `{"model":"claude-opus-4-7","max_tokens":128,"messages":[{"role":"user","content":"secret anthropic prompt must not persist"}],"metadata":{"agent_ledger.project":"anthropic-gateway-project","agent_ledger.goal":"anthropic gateway smoke"}}`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/gateway/anthropic/v1/messages", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleAnthropicMessagesGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if upstreamKey != "sk-ant-test" || upstreamVersion != defaultAnthropicVersion {
		t.Fatalf("unexpected upstream auth/version: key=%q version=%q", upstreamKey, upstreamVersion)
	}
	if rr.Header().Get("X-Agent-Ledger-Usage-Recorded") != "true" {
		t.Fatalf("usage recorded header=%q", rr.Header().Get("X-Agent-Ledger-Usage-Recorded"))
	}
	usageRows, err := db.GetModelCalls(time.Now().Add(-time.Hour), time.Now().Add(time.Hour), "gateway", "claude-opus-4-7", "anthropic-gateway-project", 10)
	if err != nil {
		t.Fatalf("GetModelCalls: %v", err)
	}
	if len(usageRows) != 1 || usageRows[0].Calls != 1 || usageRows[0].Tokens != 120 {
		t.Fatalf("unexpected anthropic gateway usage projection: %+v", usageRows)
	}
	audit, err := db.GetAuditLog(20)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if strings.Contains(string(rawAudit), "secret anthropic prompt") || strings.Contains(string(rawAudit), "secret anthropic response") || strings.Contains(string(rawAudit), "sk-ant-test") {
		t.Fatalf("sensitive anthropic gateway data leaked into audit log: %s", string(rawAudit))
	}
}

func TestAnthropicGatewayRejectsStreamingExplicitly(t *testing.T) {
	db := testServerDB(t)
	t.Setenv("AGENT_LEDGER_TEST_ANTHROPIC_KEY", "sk-ant-test")
	srv := New(db, "", Options{Gateway: testAnthropicGatewayConfig("http://127.0.0.1:1")})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/gateway/anthropic/v1/messages", strings.NewReader(`{"model":"claude-opus-4-7","stream":true,"messages":[]}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleAnthropicMessagesGateway(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "streaming gateway is not supported") {
		t.Fatalf("expected explicit streaming rejection, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestGatewayAttachesLedgerContext(t *testing.T) {
	db := testServerDB(t)
	t.Setenv("AGENT_LEDGER_TEST_OPENAI_KEY", "sk-test")
	workloadID, err := db.CreateWorkload("gateway attached workload", "gateway", "gateway-project", "agent-ledger", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "gateway", "codex", "codex run", "/workspace/agent-ledger")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"id":"chatcmpl_gateway_context",
			"model":"gpt-5.5",
			"usage":{"prompt_tokens":12,"completion_tokens":4}
		}`))
	}))
	defer upstream.Close()

	srv := New(db, "", Options{Gateway: testGatewayConfig(upstream.URL)})
	body := `{"model":"gpt-5.5","messages":[{"role":"user","content":"secret context prompt must not persist"}],"metadata":{"agent_ledger.project":"gateway-project","agent_ledger.goal":"gateway attached workload","agent_ledger.workload_id":"` + workloadID + `","agent_ledger.agent_run_id":"` + runID + `","agent_ledger.session_id":"gateway-session-1","agent_ledger.git_branch":"main"}}`
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/gateway/openai/v1/chat/completions", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOpenAIChatGateway(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	detail, err := db.GetWorkloadDetail(workloadID)
	if err != nil {
		t.Fatalf("GetWorkloadDetail: %v", err)
	}
	if len(detail.ModelCalls) != 1 || detail.ModelCalls[0].RunID != runID || detail.ModelCalls[0].SessionID != "gateway-session-1" {
		t.Fatalf("gateway usage was not attached to supplied workload/run/session: %+v", detail.ModelCalls)
	}
	if detail.Summary.ModelCalls != 1 || detail.Summary.Tokens != 16 || detail.Summary.GitBranch != "main" {
		t.Fatalf("unexpected workload summary after gateway attach: %+v", detail.Summary)
	}
}

func TestGatewayPolicyApprovalRequired(t *testing.T) {
	db := testServerDB(t)
	t.Setenv("AGENT_LEDGER_TEST_OPENAI_KEY", "sk-test")
	srv := New(db, "", Options{
		Gateway: testGatewayConfig("http://127.0.0.1:1"),
		Policies: config.PolicyConfig{Enabled: true, Rules: []config.PolicyRule{{
			Name: "approve-gateway", Scope: "action", Match: "model.call", Action: "require_approval", Message: "review live model call",
		}}},
	})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/gateway/openai/v1/chat/completions", strings.NewReader(`{"model":"gpt-5.5"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleOpenAIChatGateway(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected approval 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	rows, err := db.ListApprovalRequests("pending", 10)
	if err != nil {
		t.Fatalf("approvals: %v", err)
	}
	if len(rows) != 1 || rows[0].Action != "model.call" || rows[0].Target != "openai-chat-completions" || rows[0].Model != "gpt-5.5" {
		t.Fatalf("unexpected approvals: %+v", rows)
	}
}

func testGatewayConfig(upstream string) config.GatewayConfig {
	return config.GatewayConfig{
		Enabled:            true,
		UpstreamBaseURL:    upstream,
		APIKeyEnv:          "AGENT_LEDGER_TEST_OPENAI_KEY",
		IncludeStreamUsage: true,
		MaxBodyBytes:       1 << 20,
		MaxResponseBytes:   1 << 20,
		Timeout:            time.Second,
	}
}

func testAnthropicGatewayConfig(upstream string) config.GatewayConfig {
	return config.GatewayConfig{
		Enabled:                  true,
		AnthropicUpstreamBaseURL: upstream,
		AnthropicAPIKeyEnv:       "AGENT_LEDGER_TEST_ANTHROPIC_KEY",
		MaxBodyBytes:             1 << 20,
		MaxResponseBytes:         1 << 20,
		Timeout:                  time.Second,
	}
}
