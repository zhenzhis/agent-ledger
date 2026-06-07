package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestMCPToolsListAndBudget(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Quota.Enabled = true
	cfg.Quota.Plan = "custom"
	cfg.Quota.MonthlyBudget = 300
	cfg.Quota.TokenBudget = 3_000_000
	srv := New(db, cfg)
	srv.now = func() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ledger.current_budget","arguments":{"window":"day"}}}`,
	)
	if len(out) != 2 {
		t.Fatalf("responses=%d want 2", len(out))
	}
	tools := out[0]["result"].(map[string]interface{})["tools"].([]interface{})
	if !hasTool(tools, "ledger.start_workload") || !hasTool(tools, "ledger.get_policy") || !hasTool(tools, "ledger.record_event") || !hasTool(tools, "ledger.event_schema") {
		t.Fatalf("expected workload and policy tools, got %#v", tools)
	}
	payload := toolTextPayload(t, out[1])
	if payload["plan"] != "custom" || payload["method"] != "local-estimate" {
		t.Fatalf("unexpected budget payload: %#v", payload)
	}
	windows := payload["windows"].([]interface{})
	if len(windows) != 1 || windows[0].(map[string]interface{})["name"] != "day" {
		t.Fatalf("unexpected windows: %#v", windows)
	}
}

func TestMCPWorkloadLifecycleArtifactAndPolicy(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = true
	cfg.Policies.Rules = []config.PolicyRule{{
		Name:    "warn-high-model",
		Scope:   "model",
		Match:   "gpt-5.5",
		Action:  "warn",
		Message: "use with budget awareness",
	}}
	srv := New(db, cfg)

	create := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.start_workload","arguments":{"goal":"ship allocator audit","source":"codex","project":"agent-ledger","repo":"zhenzhis/agent-ledger","git_branch":"main","agent_name":"codex","command":"codex run","cwd":"C:/work"}}}`)[0]
	created := toolTextPayload(t, create)
	workloadID, _ := created["workload_id"].(string)
	runID, _ := created["run_id"].(string)
	if workloadID == "" || runID == "" {
		t.Fatalf("missing ids: %#v", created)
	}

	policyLine := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ledger.get_policy","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","model":"gpt-5.5","role":"operator"}}}`
	artifactLine := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ledger.record_artifact","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","artifact_type":"report","label":"privacy-safe-summary","path_hash":"sha256:abc","sha256":"def","metadata":{"format":"markdown"}}}}`
	closeLine := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ledger.close_workload","arguments":{"workload_id":"` + workloadID + `","status":"completed","outcome":"accepted"}}}`
	responses := serveLines(t, srv, policyLine, artifactLine, closeLine)
	policy := toolTextPayload(t, responses[0])
	if policy["action"] != "warn" {
		t.Fatalf("policy action=%#v", policy["action"])
	}
	artifact := toolTextPayload(t, responses[1])
	if artifact["artifact_id"] == "" {
		t.Fatalf("missing artifact id: %#v", artifact)
	}
	closed := toolTextPayload(t, responses[2])
	if closed["status"] != "completed" {
		t.Fatalf("close payload=%#v", closed)
	}

	detail, err := db.GetWorkloadDetail(workloadID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Summary.Status != "completed" {
		t.Fatalf("status=%s", detail.Summary.Status)
	}
	if len(detail.Policies) != 1 || detail.Policies[0].Action != "warn" {
		t.Fatalf("policy decisions=%#v", detail.Policies)
	}
	if len(detail.Artifacts) != 1 || detail.Artifacts[0].Label != "privacy-safe-summary" {
		t.Fatalf("artifacts=%#v", detail.Artifacts)
	}
}

func TestMCPPolicyDisabledDoesNotEvaluateRules(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = false
	cfg.Policies.Rules = []config.PolicyRule{{
		Name:    "disabled-block",
		Scope:   "model",
		Match:   "gpt-5.5",
		Action:  "block",
		Message: "should not run",
	}}
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.get_policy","arguments":{"model":"gpt-5.5"}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["enabled"] != false || payload["action"] != "allow" {
		t.Fatalf("unexpected disabled policy payload: %#v", payload)
	}
	decisions := payload["decisions"].([]interface{})
	if len(decisions) != 0 {
		t.Fatalf("disabled policy returned decisions: %#v", decisions)
	}
}

func TestMCPRecordEvent(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.record_event","arguments":{"event_id":"evt-mcp-record","source":"codex","event_type":"workload.started","payload":{"goal":"mcp event bridge","project":"agent-ledger"}}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["status"] != "inserted" || payload["workload_id"] == "" {
		t.Fatalf("unexpected record event payload: %#v", payload)
	}
	detail, err := db.GetWorkloadDetail(payload["workload_id"].(string))
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Summary.Goal != "mcp event bridge" {
		t.Fatalf("summary=%#v", detail.Summary)
	}
}

func openTestDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func serveLines(t *testing.T, srv *Server, lines ...string) []map[string]interface{} {
	t.Helper()
	var input strings.Builder
	for _, line := range lines {
		input.WriteString(line)
		input.WriteByte('\n')
	}
	var output bytes.Buffer
	if err := srv.Serve(strings.NewReader(input.String()), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}
	scanner := bufio.NewScanner(&output)
	var responses []map[string]interface{}
	for scanner.Scan() {
		var resp map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			t.Fatalf("decode response %q: %v", scanner.Text(), err)
		}
		if errObj, ok := resp["error"]; ok {
			t.Fatalf("rpc error: %#v", errObj)
		}
		responses = append(responses, resp)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}
	return responses
}

func toolTextPayload(t *testing.T, resp map[string]interface{}) map[string]interface{} {
	t.Helper()
	result := resp["result"].(map[string]interface{})
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode tool text: %v\n%s", err, text)
	}
	return payload
}

func hasTool(tools []interface{}, name string) bool {
	for _, tool := range tools {
		if tool.(map[string]interface{})["name"] == name {
			return true
		}
	}
	return false
}
