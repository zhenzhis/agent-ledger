package storage

import (
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
