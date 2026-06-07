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
	if !hasTool(tools, "ledger.start_workload") || !hasTool(tools, "ledger.start_run") || !hasTool(tools, "ledger.link_workloads") || !hasTool(tools, "ledger.get_policy") || !hasTool(tools, "ledger.policy_audit") || !hasTool(tools, "ledger.audit_log") || !hasTool(tools, "ledger.workload_timeline") || !hasTool(tools, "ledger.workload_state") || !hasTool(tools, "ledger.record_tool_call") || !hasTool(tools, "ledger.record_context") || !hasTool(tools, "ledger.record_evaluation") || !hasTool(tools, "ledger.record_event") || !hasTool(tools, "ledger.validate_event") || !hasTool(tools, "ledger.event_schema") || !hasTool(tools, "ledger.event_examples") || !hasTool(tools, "ledger.adapter_conformance") || !hasTool(tools, "ledger.integrations") {
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

func TestMCPResourcesAndPrompts(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = true
	srv := New(db, cfg)
	srv.now = func() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"agent-ledger://schema/canonical-events"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"prompts/list"}`,
		`{"jsonrpc":"2.0","id":5,"method":"prompts/get","params":{"name":"agent-ledger/workload-brief","arguments":{"goal":"ship router","project":"quant","constraints":"privacy strict"}}}`,
	)
	caps := out[0]["result"].(map[string]interface{})["capabilities"].(map[string]interface{})
	if caps["resources"] == nil || caps["prompts"] == nil {
		t.Fatalf("missing resource/prompt capabilities: %#v", caps)
	}
	resources := out[1]["result"].(map[string]interface{})["resources"].([]interface{})
	if !hasResource(resources, "agent-ledger://schema/canonical-events") || !hasResource(resources, "agent-ledger://schema/canonical-event-examples") || !hasResource(resources, "agent-ledger://budget/current") {
		t.Fatalf("expected core resources, got %#v", resources)
	}
	resourceText := resourceTextPayload(t, out[2])
	if !strings.Contains(resourceText, "workload.started") || !strings.Contains(resourceText, "rejected_payload_keys") {
		t.Fatalf("unexpected schema resource text: %s", resourceText)
	}
	prompts := out[3]["result"].(map[string]interface{})["prompts"].([]interface{})
	if !hasPrompt(prompts, "agent-ledger/workload-brief") || !hasPrompt(prompts, "agent-ledger/cost-review") {
		t.Fatalf("expected prompts, got %#v", prompts)
	}
	promptText := promptTextPayload(t, out[4])
	if !strings.Contains(promptText, "ship router") || !strings.Contains(promptText, "privacy strict") {
		t.Fatalf("prompt did not interpolate arguments: %s", promptText)
	}
	if strings.Contains(strings.ToLower(promptText), "raw file contents") && !strings.Contains(promptText, "avoid") {
		t.Fatalf("prompt should warn against durable sensitive content: %s", promptText)
	}
}

func TestMCPRecentWorkloadsResourceIncludesState(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	workloadID, err := db.CreateWorkload("resource state workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}

	out := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"agent-ledger://workloads/recent"}}`)
	var payload map[string]interface{}
	text := resourceTextPayload(t, out[0])
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode workload resource: %v\n%s", err, text)
	}
	rows, ok := payload["rows"].([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("resource missing workload rows: %#v", payload)
	}
	states, ok := payload["states"].([]interface{})
	if !ok || len(states) == 0 {
		t.Fatalf("resource missing workload states: %#v", payload)
	}
	state := states[0].(map[string]interface{})
	if state["workload_id"] != workloadID || state["phase"] != "planned" || state["terminal"] != false {
		t.Fatalf("unexpected resource state: %#v", state)
	}
	if payload["stale_after_seconds"] == nil {
		t.Fatalf("resource missing stale threshold: %#v", payload)
	}
}

func TestMCPUnknownResourceReturnsError(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	responses := serveRawLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"agent-ledger://unknown"}}`)
	if responses[0]["error"] == nil {
		t.Fatalf("expected unknown resource error: %#v", responses[0])
	}
}

func TestMCPReadOnlyAllowsReadToolsAndRejectsWriteTools(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	srv := New(db, cfg)
	srv.now = func() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

	readResponses := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.current_budget","arguments":{"window":"day"}}}`,
	)
	if toolTextPayload(t, readResponses[0])["method"] != "local-estimate" {
		t.Fatalf("read-only read tool returned unexpected payload: %#v", readResponses[0])
	}

	writeResponses := serveRawLines(t, srv,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ledger.start_workload","arguments":{"goal":"blocked","source":"codex"}}}`,
	)
	if writeResponses[0]["error"] == nil {
		t.Fatalf("expected read-only write tool error: %#v", writeResponses[0])
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
	parentID, err := db.CreateWorkload("parent workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("CreateWorkload parent: %v", err)
	}

	startRunLine := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ledger.start_run","arguments":{"workload_id":"` + workloadID + `","source":"codex","agent_name":"codex-worker","command":"codex worker","cwd":"C:/work"}}}`
	linkLine := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ledger.link_workloads","arguments":{"source_workload_id":"` + workloadID + `","target_workload_id":"` + parentID + `","relation":"depends-on","reason":"privacy-safe parent dependency","created_by":"mcp-test"}}}`
	policyLine := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ledger.get_policy","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","model":"gpt-5.5","role":"operator"}}}`
	toolLine := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"ledger.record_tool_call","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","source":"codex","tool_call_id":"tool-mcp","tool_name":"shell","tool_type":"command","status":"ok","duration_ms":120,"params_hash":"sha256:params"}}}`
	contextLine := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"ledger.record_context","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","source":"codex","context_ref_id":"ctx-mcp","ref_type":"repo","ref_hash":"sha256:context","label":"privacy-safe-context","repo":"zhenzhis/agent-ledger","git_branch":"main","privacy_label":"synthetic"}}}`
	artifactLine := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"ledger.record_artifact","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","artifact_type":"report","label":"privacy-safe-summary","path_hash":"sha256:abc","sha256":"def","metadata":{"format":"markdown"}}}}`
	evaluationLine := `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"ledger.record_evaluation","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","source":"codex","evaluation_id":"eval-mcp","evaluator":"ci","status":"pass","score":0.97,"signal":"unit-tests","notes":"privacy-safe acceptance signal"}}}`
	closeLine := `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"ledger.close_workload","arguments":{"workload_id":"` + workloadID + `","status":"completed","outcome":"accepted"}}}`
	timelineLine := `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"ledger.workload_timeline","arguments":{"workload_id":"` + workloadID + `","limit":20}}}`
	stateLine := `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"ledger.workload_state","arguments":{"workload_id":"` + workloadID + `","max_age":"10m"}}}`
	responses := serveLines(t, srv, startRunLine, linkLine, policyLine, toolLine, contextLine, artifactLine, evaluationLine, closeLine, timelineLine, stateLine)
	startedRun := toolTextPayload(t, responses[0])
	if startedRun["run_id"] == "" || startedRun["workload_id"] != workloadID {
		t.Fatalf("start run payload=%#v", startedRun)
	}
	link := toolTextPayload(t, responses[1])
	if link["link_id"] == "" || link["target_workload_id"] != parentID {
		t.Fatalf("missing link result: %#v", link)
	}
	policy := toolTextPayload(t, responses[2])
	if policy["action"] != "warn" {
		t.Fatalf("policy action=%#v", policy["action"])
	}
	tool := toolTextPayload(t, responses[3])
	if tool["status"] != "inserted" {
		t.Fatalf("missing tool result: %#v", tool)
	}
	context := toolTextPayload(t, responses[4])
	if context["status"] != "inserted" {
		t.Fatalf("missing context result: %#v", context)
	}
	artifact := toolTextPayload(t, responses[5])
	if artifact["artifact_id"] == "" {
		t.Fatalf("missing artifact id: %#v", artifact)
	}
	evaluation := toolTextPayload(t, responses[6])
	if evaluation["status"] != "inserted" {
		t.Fatalf("missing evaluation result: %#v", evaluation)
	}
	closed := toolTextPayload(t, responses[7])
	if closed["status"] != "completed" {
		t.Fatalf("close payload=%#v", closed)
	}
	timeline := toolTextPayload(t, responses[8])
	rows, ok := timeline["rows"].([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("timeline payload=%#v", timeline)
	}
	if !timelineHasKind(rows, "evaluation") {
		t.Fatalf("timeline missing evaluation: %#v", timeline)
	}
	if !timelineHasKind(rows, "workload_link") {
		t.Fatalf("timeline missing workload link: %#v", timeline)
	}
	state := toolTextPayload(t, responses[9])
	if state["phase"] != "accepted" || state["terminal"] != true {
		t.Fatalf("unexpected workload state: %#v", state)
	}

	detail, err := db.GetWorkloadDetail(workloadID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Summary.Status != "completed" {
		t.Fatalf("status=%s", detail.Summary.Status)
	}
	if len(detail.Runs) != 2 {
		t.Fatalf("runs=%#v", detail.Runs)
	}
	if len(detail.ToolCalls) != 1 || detail.ToolCalls[0].ToolCallID != "tool-mcp" || detail.ToolCalls[0].ToolName != "shell" {
		t.Fatalf("tool_calls=%#v", detail.ToolCalls)
	}
	if len(detail.ContextRefs) != 1 || detail.ContextRefs[0].ContextRefID != "ctx-mcp" || detail.ContextRefs[0].RefHash != "sha256:context" {
		t.Fatalf("context_refs=%#v", detail.ContextRefs)
	}
	if len(detail.Policies) != 1 || detail.Policies[0].Action != "warn" {
		t.Fatalf("policy decisions=%#v", detail.Policies)
	}
	if len(detail.Links) != 1 || detail.Links[0].TargetWorkloadID != parentID || detail.Links[0].Relation != "depends_on" {
		t.Fatalf("links=%#v", detail.Links)
	}
	if len(detail.Artifacts) != 1 || detail.Artifacts[0].Label != "privacy-safe-summary" {
		t.Fatalf("artifacts=%#v", detail.Artifacts)
	}
	if len(detail.Evaluations) != 1 || detail.Evaluations[0].EvaluationID != "eval-mcp" || detail.Evaluations[0].Status != "pass" || detail.Evaluations[0].Signal != "unit-tests" {
		t.Fatalf("evaluations=%#v", detail.Evaluations)
	}
}

func timelineHasKind(rows []interface{}, kind string) bool {
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if ok && m["kind"] == kind {
			return true
		}
	}
	return false
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

func TestMCPPolicyAudit(t *testing.T) {
	db := openTestDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&storage.UsageRecord{
		Source:       "codex",
		SessionID:    "sess-mcp-policy",
		Model:        "gpt-5.5",
		InputTokens:  100,
		OutputTokens: 25,
		CostUSD:      0.5,
		Timestamp:    ts,
		Project:      "agent-ledger",
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = true
	cfg.Policies.Rules = []config.PolicyRule{{Name: "warn-gpt", Scope: "model", Match: "gpt-5.5", Action: "warn"}}
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.policy_audit","arguments":{"from":"2026-06-07","to":"2026-06-08","limit":10}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["matches"] != float64(1) {
		t.Fatalf("unexpected policy audit payload: %#v", payload)
	}
}

func TestMCPPolicyAgentOpsScopes(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = true
	cfg.Policies.Rules = []config.PolicyRule{{Name: "branch-block", Scope: "git_branch", Match: "release", Action: "block"}}
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.get_policy","arguments":{"git_branch":"release/2026w23","action":"model.call","target":"gpt-5.5"}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["action"] != "block" {
		t.Fatalf("unexpected policy payload: %#v", payload)
	}
}

func TestMCPAuditLog(t *testing.T) {
	db := openTestDB(t)
	if err := db.AppendAuditLog("local", "operator", "pricing.sync", "openai", map[string]string{"project": "private"}); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.audit_log","arguments":{"action":"pricing","privacy":true,"limit":10}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["count"] != float64(1) {
		t.Fatalf("unexpected audit payload: %#v", payload)
	}
	rows := payload["rows"].([]interface{})
	row := rows[0].(map[string]interface{})
	if row["target"] != "<redacted>" || row["params"] != "<redacted>" {
		t.Fatalf("audit privacy redaction failed: %#v", row)
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

func TestMCPValidateAndConformanceAreReadOnly(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	srv := New(db, cfg)

	validateResp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.validate_event","arguments":{"source":"codex","event_type":"workload.started","payload":{"goal":"mcp validate only"}}}}`)[0]
	validatePayload := toolTextPayload(t, validateResp)
	if validatePayload["status"] != "valid_with_warnings" || validatePayload["event_id"] == "" {
		t.Fatalf("unexpected validate payload: %#v", validatePayload)
	}

	request, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "ledger.adapter_conformance",
			"arguments": map[string]interface{}{
				"kind":     "canonical",
				"strict":   true,
				"raw_json": `{"source":"codex","event_type":"workload.started","payload":{"goal":"mcp conformance only"}}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	conformanceResp := serveLines(t, srv, string(request))[0]
	conformancePayload := toolTextPayload(t, conformanceResp)
	if conformancePayload["ok"] != false || conformancePayload["status"] != "fail" {
		t.Fatalf("unexpected conformance payload: %#v", conformancePayload)
	}
	quality, err := db.GetDataQuality(time.Hour)
	if err != nil {
		t.Fatalf("GetDataQuality: %v", err)
	}
	if quality.Provenance == nil || quality.Provenance.Events != 0 {
		t.Fatalf("read-only validation tools wrote events: %#v", quality.Provenance)
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
	responses := serveRawLines(t, srv, lines...)
	for _, resp := range responses {
		if errObj, ok := resp["error"]; ok {
			t.Fatalf("rpc error: %#v", errObj)
		}
	}
	return responses
}

func serveRawLines(t *testing.T, srv *Server, lines ...string) []map[string]interface{} {
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

func resourceTextPayload(t *testing.T, resp map[string]interface{}) string {
	t.Helper()
	result := resp["result"].(map[string]interface{})
	contents := result["contents"].([]interface{})
	return contents[0].(map[string]interface{})["text"].(string)
}

func promptTextPayload(t *testing.T, resp map[string]interface{}) string {
	t.Helper()
	result := resp["result"].(map[string]interface{})
	messages := result["messages"].([]interface{})
	content := messages[0].(map[string]interface{})["content"].(map[string]interface{})
	return content["text"].(string)
}

func hasResource(resources []interface{}, uri string) bool {
	for _, resource := range resources {
		if resource.(map[string]interface{})["uri"] == uri {
			return true
		}
	}
	return false
}

func hasPrompt(prompts []interface{}, name string) bool {
	for _, prompt := range prompts {
		if prompt.(map[string]interface{})["name"] == name {
			return true
		}
	}
	return false
}
