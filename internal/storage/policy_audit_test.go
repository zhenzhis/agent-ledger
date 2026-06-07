package storage

import (
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	ledgerpolicy "github.com/zhenzhis/agent-ledger/internal/policy"
)

func TestPolicyAuditCandidatesCoverUsageToolsAndWorkloads(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source:       "codex",
		SessionID:    "sess-policy",
		Model:        "gpt-5.5",
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      1.25,
		Timestamp:    ts,
		Project:      "agent-ledger",
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	workloadID, err := db.CreateWorkload("policy audit", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE workloads SET created_at=?,updated_at=? WHERE workload_id=?`, ts, ts, workloadID); err != nil {
		t.Fatalf("update workload time: %v", err)
	}
	if _, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:    "evt-policy-tool",
		Source:     "codex",
		EventType:  "tool.call",
		WorkloadID: workloadID,
		Timestamp:  ts.Add(time.Minute),
		Payload:    rawJSON(t, map[string]interface{}{"tool_name": "shell", "tool_type": "command", "status": "ok"}),
	}); err != nil {
		t.Fatalf("tool event: %v", err)
	}
	candidates, err := db.GetPolicyAuditCandidates(ts.Add(-time.Hour), ts.Add(time.Hour), "", "", "", 20)
	if err != nil {
		t.Fatalf("GetPolicyAuditCandidates: %v", err)
	}
	kinds := map[string]bool{}
	for _, c := range candidates {
		kinds[c.Kind] = true
	}
	for _, kind := range []string{"usage_session", "tool_call", "workload"} {
		if !kinds[kind] {
			t.Fatalf("missing %s candidates=%#v", kind, candidates)
		}
	}
	report := ledgerpolicy.Audit(config.PolicyConfig{Enabled: true, Rules: []config.PolicyRule{
		{Name: "warn-gpt", Scope: "model", Match: "gpt-5.5", Action: "warn"},
		{Name: "review-tools", Scope: "action", Match: "tool.call", Action: "require_approval"},
	}}, candidates, 20)
	if report.Matches != 2 || report.Warnings != 1 || report.Approvals != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}

	modelCandidates, err := db.GetPolicyAuditCandidates(ts.Add(-time.Hour), ts.Add(time.Hour), "", "gpt-5.5", "", 20)
	if err != nil {
		t.Fatalf("GetPolicyAuditCandidates with model: %v", err)
	}
	if len(modelCandidates) != 1 || modelCandidates[0].Kind != "usage_session" {
		t.Fatalf("model filter should only return usage candidates: %#v", modelCandidates)
	}
}
