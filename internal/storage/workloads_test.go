package storage

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBackfillWorkloadsFromUsageIsIdempotent(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	if err := db.UpsertSession(&SessionRecord{
		Source:    "codex",
		SessionID: "sess-1",
		Project:   "repo-a",
		CWD:       "/home/user/repo-a",
		GitBranch: "main",
		StartTime: ts,
		Prompts:   2,
	}); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	for i := 0; i < 2; i++ {
		if err := db.InsertUsage(&UsageRecord{
			Source:                   "codex",
			SessionID:                "sess-1",
			Model:                    "gpt-5",
			InputTokens:              int64(100 + i),
			OutputTokens:             50,
			CacheReadInputTokens:     20,
			CacheCreationInputTokens: 10,
			CostUSD:                  0.25,
			Timestamp:                ts.Add(time.Duration(i) * time.Minute),
			Project:                  "repo-a",
			GitBranch:                "main",
			PricingSource:            "test",
			PricingConfidence:        "official",
		}); err != nil {
			t.Fatalf("InsertUsage: %v", err)
		}
	}

	from := ts.Add(-time.Hour)
	to := ts.Add(time.Hour)
	for i := 0; i < 2; i++ {
		if err := db.BackfillWorkloadsFromUsage(from, to); err != nil {
			t.Fatalf("BackfillWorkloadsFromUsage pass %d: %v", i, err)
		}
	}

	page, err := db.GetWorkloadsPage(from, to, "", "", "", "", "", 10, 0)
	if err != nil {
		t.Fatalf("GetWorkloadsPage: %v", err)
	}
	if page.Total != 1 || len(page.Rows) != 1 {
		t.Fatalf("expected one workload, total=%d rows=%d", page.Total, len(page.Rows))
	}
	row := page.Rows[0]
	if row.ModelCalls != 2 {
		t.Fatalf("expected 2 model calls, got %d", row.ModelCalls)
	}
	if row.Runs != 1 || row.Sessions != 1 {
		t.Fatalf("expected one run/session, got runs=%d sessions=%d", row.Runs, row.Sessions)
	}
	wantTokens := int64((100 + 50 + 20 + 10) + (101 + 50 + 20 + 10))
	if row.Tokens != wantTokens {
		t.Fatalf("tokens mismatch: got %d want %d", row.Tokens, wantTokens)
	}
	if row.CostUSD != 0.5 {
		t.Fatalf("cost mismatch: got %f", row.CostUSD)
	}
}

func TestManualWorkloadAndRunDetail(t *testing.T) {
	db := tempDB(t)
	id, err := db.CreateWorkload("ship agent telemetry", "codex", "repo-a", "repo-a", "main", "alice", "research", 12.5)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(id, "codex", "codex", "codex exec", "/home/user/repo-a")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if err := db.FinishAgentRun(runID, "completed", 0, "", 1234); err != nil {
		t.Fatalf("FinishAgentRun: %v", err)
	}
	if err := db.CloseWorkload(id, "completed", "tests-passed"); err != nil {
		t.Fatalf("CloseWorkload: %v", err)
	}

	detail, err := db.GetWorkloadDetail(id)
	if err != nil {
		t.Fatalf("GetWorkloadDetail: %v", err)
	}
	if detail.Summary.WorkloadID != id {
		t.Fatalf("id mismatch: got %s want %s", detail.Summary.WorkloadID, id)
	}
	if detail.Summary.Status != "completed" || detail.Summary.Outcome != "tests-passed" {
		t.Fatalf("bad status/outcome: %s/%s", detail.Summary.Status, detail.Summary.Outcome)
	}
	if len(detail.Runs) != 1 || detail.Runs[0].RunID != runID {
		t.Fatalf("run detail missing: %+v", detail.Runs)
	}
}

func TestStartAgentRunRejectsClosedWorkload(t *testing.T) {
	db := tempDB(t)
	id, err := db.CreateWorkload("closed workload", "codex", "repo-a", "repo-a", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	if err := db.CloseWorkload(id, "completed", "done"); err != nil {
		t.Fatalf("CloseWorkload: %v", err)
	}
	if _, err := db.StartAgentRun(id, "codex", "codex", "codex exec", "/home/user/repo-a"); err == nil {
		t.Fatal("expected closed workload to reject new run")
	}
}

func TestStartAgentRunRedactsCommandSecrets(t *testing.T) {
	db := tempDB(t)
	id, err := db.CreateWorkload("secret command", "codex", "repo-a", "repo-a", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(id, "codex", "codex", "OPENAI_API_KEY=sk-test codex --token secret-value --password='hidden' -m gpt-5 -H 'Authorization: Bearer abc123'", "/home/user/repo-a")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	detail, err := db.GetWorkloadDetail(id)
	if err != nil {
		t.Fatalf("GetWorkloadDetail: %v", err)
	}
	if len(detail.Runs) != 1 || detail.Runs[0].RunID != runID {
		t.Fatalf("run detail missing: %+v", detail.Runs)
	}
	command := detail.Runs[0].Command
	for _, leaked := range []string{"sk-test", "secret-value", "hidden", "Bearer abc123"} {
		if strings.Contains(command, leaked) {
			t.Fatalf("command leaked %q: %s", leaked, command)
		}
	}
	if !strings.Contains(command, "<redacted>") {
		t.Fatalf("expected redacted marker in command: %s", command)
	}
}

func TestAgentRunLivenessReportsStaleActiveRuns(t *testing.T) {
	db := tempDB(t)
	id, err := db.CreateWorkload("ship async goal", "codex", "repo-a", "repo-a", "main", "alice", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(id, "codex", "codex", "codex exec", "/home/user/repo-a")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	oldHeartbeat := time.Now().UTC().Add(-20 * time.Minute)
	if _, err := db.RecordAgentRunHeartbeat("evt-stale-run", runID, "working", "testing", "waiting on tests", 0.5, map[string]interface{}{"files_touched": 2}, oldHeartbeat, 1); err != nil {
		t.Fatalf("RecordAgentRunHeartbeat: %v", err)
	}

	freshID, err := db.CreateWorkload("finished goal", "codex", "repo-b", "repo-b", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload fresh: %v", err)
	}
	finishedRunID, err := db.StartAgentRun(freshID, "codex", "codex", "codex exec", "/home/user/repo-b")
	if err != nil {
		t.Fatalf("StartAgentRun fresh: %v", err)
	}
	if err := db.FinishAgentRun(finishedRunID, "completed", 0, "", 10); err != nil {
		t.Fatalf("FinishAgentRun: %v", err)
	}

	rows, err := db.GetAgentRunLiveness(10*time.Minute, false, 10)
	if err != nil {
		t.Fatalf("GetAgentRunLiveness: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one active run, got %+v", rows)
	}
	row := rows[0]
	if row.RunID != runID || !row.Stale || row.HeartbeatCount != 1 || row.Phase != "testing" || row.Progress != 0.5 {
		t.Fatalf("unexpected liveness row: %+v", row)
	}
	if row.LastHeartbeatAt == "" || row.LastActivity != row.LastHeartbeatAt || row.AgeSeconds < int64((10*time.Minute).Seconds()) {
		t.Fatalf("bad activity fields: %+v", row)
	}

	staleRows, err := db.GetAgentRunLiveness(10*time.Minute, true, 10)
	if err != nil {
		t.Fatalf("GetAgentRunLiveness stale: %v", err)
	}
	if len(staleRows) != 1 || staleRows[0].RunID != runID {
		t.Fatalf("unexpected stale rows: %+v", staleRows)
	}
	filteredRows, err := db.GetAgentRunLiveness(10*time.Minute, false, 10, "codex", "repo-a")
	if err != nil {
		t.Fatalf("GetAgentRunLiveness filtered: %v", err)
	}
	if len(filteredRows) != 1 || filteredRows[0].RunID != runID {
		t.Fatalf("unexpected filtered rows: %+v", filteredRows)
	}
	emptyRows, err := db.GetAgentRunLiveness(10*time.Minute, false, 10, "codex", "repo-missing")
	if err != nil {
		t.Fatalf("GetAgentRunLiveness empty filter: %v", err)
	}
	if len(emptyRows) != 0 {
		t.Fatalf("expected empty filtered rows, got %+v", emptyRows)
	}
}

func TestWorkloadStateDerivesAsyncTerminalSignals(t *testing.T) {
	db := tempDB(t)
	id, err := db.CreateWorkload("ship terminal state", "codex", "repo-a", "repo-a", "main", "alice", "research", 10)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(id, "codex", "codex", "codex exec", "/home/user/repo-a")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-state-stale", runID, "working", "testing", "waiting on tests", 0.4, nil, time.Now().UTC().Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("RecordAgentRunHeartbeat: %v", err)
	}
	state, err := db.GetWorkloadState(id, 10*time.Minute)
	if err != nil {
		t.Fatalf("GetWorkloadState stale: %v", err)
	}
	if state.Phase != "stale" || !state.Stale || state.StaleRuns != 1 || state.NextAction == "" || len(state.Risks) == 0 {
		t.Fatalf("unexpected stale state: %+v", state)
	}

	if err := db.FinishAgentRun(runID, "completed", 0, "", 1200); err != nil {
		t.Fatalf("FinishAgentRun: %v", err)
	}
	if _, err := db.IngestCanonicalEvent(CanonicalEvent{
		Source:     "test",
		EventType:  "evaluation.recorded",
		WorkloadID: id,
		AgentRunID: runID,
		Timestamp:  time.Now().UTC(),
		Payload:    json.RawMessage(`{"evaluation_id":"eval-state","evaluator":"ci","status":"pass","score":0.95,"signal":"unit-tests"}`),
		Confidence: 1,
	}); err != nil {
		t.Fatalf("IngestCanonicalEvent evaluation: %v", err)
	}
	if err := db.CloseWorkload(id, "completed", "accepted"); err != nil {
		t.Fatalf("CloseWorkload: %v", err)
	}
	state, err = db.GetWorkloadState(id, 10*time.Minute)
	if err != nil {
		t.Fatalf("GetWorkloadState accepted: %v", err)
	}
	if !state.Terminal || state.Phase != "accepted" || state.Progress != 1 || state.PositiveEvaluations != 1 || state.CompletedRuns != 1 || state.Stale {
		t.Fatalf("unexpected accepted state: %+v", state)
	}

	blockedID, err := db.CreateWorkload("blocked policy", "codex", "repo-b", "repo-b", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload blocked: %v", err)
	}
	if _, err := db.RecordPolicyDecision(blockedID, "", "deny-model", "block", "model not allowed", "operator"); err != nil {
		t.Fatalf("RecordPolicyDecision: %v", err)
	}
	blockedState, err := db.GetWorkloadState(blockedID, 10*time.Minute)
	if err != nil {
		t.Fatalf("GetWorkloadState blocked: %v", err)
	}
	if blockedState.Phase != "blocked" || blockedState.PolicyBlocks != 1 || len(blockedState.Risks) == 0 {
		t.Fatalf("unexpected blocked state: %+v", blockedState)
	}
}

func TestWorkloadEventFeedDerivesSeverity(t *testing.T) {
	db := tempDB(t)
	now := time.Now().UTC()
	staleID, err := db.CreateWorkload("stale feed", "codex", "repo-a", "repo-a", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("CreateWorkload stale: %v", err)
	}
	runID, err := db.StartAgentRun(staleID, "codex", "codex", "codex exec", "/home/user/repo-a")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-feed-stale", runID, "working", "testing", "waiting", 0.4, nil, now.Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("RecordAgentRunHeartbeat: %v", err)
	}
	blockedID, err := db.CreateWorkload("blocked feed", "codex", "repo-b", "repo-b", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("CreateWorkload blocked: %v", err)
	}
	if _, err := db.RecordPolicyDecision(blockedID, "", "deny-model", "block", "model not allowed", "operator"); err != nil {
		t.Fatalf("RecordPolicyDecision: %v", err)
	}

	feed, err := db.GetWorkloadEventFeed(now.AddDate(0, 0, -1), now.AddDate(0, 0, 1), "codex", "", "", "", "", 10, 10*time.Minute)
	if err != nil {
		t.Fatalf("GetWorkloadEventFeed: %v", err)
	}
	if feed.Total != 2 || len(feed.Rows) != 2 {
		t.Fatalf("unexpected feed: %+v", feed)
	}
	if !feedHasSeverity(feed.Rows, blockedID, "critical") || !feedHasSeverity(feed.Rows, staleID, "warning") {
		t.Fatalf("unexpected feed severities: %+v", feed.Rows)
	}
	warnings, err := db.GetWorkloadEventFeed(now.AddDate(0, 0, -1), now.AddDate(0, 0, 1), "codex", "", "", "", "warning", 10, 10*time.Minute)
	if err != nil {
		t.Fatalf("GetWorkloadEventFeed warning: %v", err)
	}
	if warnings.Total != 1 || warnings.Rows[0].WorkloadID != staleID || warnings.Rows[0].Phase != "stale" {
		t.Fatalf("unexpected warning feed: %+v", warnings)
	}
}

func feedHasSeverity(rows []WorkloadFeedEvent, workloadID, severity string) bool {
	for _, row := range rows {
		if row.WorkloadID == workloadID && row.Severity == severity {
			return true
		}
	}
	return false
}

func TestAgentRunHeartbeatRejectsTerminalRun(t *testing.T) {
	db := tempDB(t)
	id, err := db.CreateWorkload("terminal run", "codex", "repo-a", "repo-a", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(id, "codex", "codex", "codex exec", "/home/user/repo-a")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	first, err := db.RecordAgentRunHeartbeat("evt-terminal-dup", runID, "working", "testing", "", 0.5, nil, time.Now().UTC(), 1)
	if err != nil {
		t.Fatalf("RecordAgentRunHeartbeat: %v", err)
	}
	if err := db.FinishAgentRun(runID, "completed", 0, "", 10); err != nil {
		t.Fatalf("FinishAgentRun: %v", err)
	}
	dup, err := db.RecordAgentRunHeartbeat("evt-terminal-dup", runID, "working", "testing", "", 0.5, nil, time.Now().UTC(), 1)
	if err != nil {
		t.Fatalf("duplicate terminal heartbeat should remain idempotent: %v", err)
	}
	if dup.EventID != first.EventID || dup.RunID != runID {
		t.Fatalf("unexpected duplicate row: %+v", dup)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-terminal-new", runID, "working", "testing", "", 0.5, nil, time.Now().UTC(), 1); err == nil {
		t.Fatal("expected new heartbeat on terminal run to fail")
	}
	rows, err := db.GetAgentRunLiveness(10*time.Minute, false, 10)
	if err != nil {
		t.Fatalf("GetAgentRunLiveness: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("terminal run should not be live: %+v", rows)
	}
}
