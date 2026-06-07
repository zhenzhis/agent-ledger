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
		Enabled:          true,
		UpstreamBaseURL:  upstream,
		APIKeyEnv:        "AGENT_LEDGER_TEST_OPENAI_KEY",
		MaxBodyBytes:     1 << 20,
		MaxResponseBytes: 1 << 20,
		Timeout:          time.Second,
	}
}
