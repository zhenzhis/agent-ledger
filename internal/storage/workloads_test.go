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

func TestControlOperationIdempotencyForWorkloadAndRun(t *testing.T) {
	db := tempDB(t)
	workloadID, replayed, err := db.CreateWorkloadIdempotent("workload-key-1", "ship idempotent control", "codex", "repo-a", "repo-a", "main", "alice", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkloadIdempotent first: %v", err)
	}
	if replayed || workloadID == "" {
		t.Fatalf("unexpected first workload result id=%q replayed=%v", workloadID, replayed)
	}
	againID, replayed, err := db.CreateWorkloadIdempotent("workload-key-1", "ship idempotent control", "codex", "repo-a", "repo-a", "main", "alice", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkloadIdempotent replay: %v", err)
	}
	if !replayed || againID != workloadID {
		t.Fatalf("expected replay of %s, got id=%s replayed=%v", workloadID, againID, replayed)
	}
	if _, _, err := db.CreateWorkloadIdempotent("workload-key-1", "different goal", "codex", "repo-a", "repo-a", "main", "alice", "research", 0); !IsIdempotencyConflict(err) {
		t.Fatalf("expected workload idempotency conflict, got %v", err)
	}
	var workloadCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM workloads`).Scan(&workloadCount); err != nil {
		t.Fatalf("count workloads: %v", err)
	}
	if workloadCount != 1 {
		t.Fatalf("expected one workload, got %d", workloadCount)
	}

	runID, runReplayed, err := db.StartAgentRunIdempotent("run-key-1", workloadID, "codex", "codex", "codex exec", "/home/user/repo-a")
	if err != nil {
		t.Fatalf("StartAgentRunIdempotent first: %v", err)
	}
	if runReplayed || runID == "" {
		t.Fatalf("unexpected first run result id=%q replayed=%v", runID, runReplayed)
	}
	againRunID, runReplayed, err := db.StartAgentRunIdempotent("run-key-1", workloadID, "codex", "codex", "codex exec", "/home/user/repo-a")
	if err != nil {
		t.Fatalf("StartAgentRunIdempotent replay: %v", err)
	}
	if !runReplayed || againRunID != runID {
		t.Fatalf("expected run replay of %s, got id=%s replayed=%v", runID, againRunID, runReplayed)
	}
	if _, _, err := db.StartAgentRunIdempotent("run-key-1", workloadID, "codex", "codex", "codex different", "/home/user/repo-a"); !IsIdempotencyConflict(err) {
		t.Fatalf("expected run idempotency conflict, got %v", err)
	}
	if err := db.CloseWorkload(workloadID, "completed", "done"); err != nil {
		t.Fatalf("CloseWorkload: %v", err)
	}
	closedReplayID, closedReplay, err := db.StartAgentRunIdempotent("run-key-1", workloadID, "codex", "codex", "codex exec", "/home/user/repo-a")
	if err != nil {
		t.Fatalf("closed workload replay should succeed: %v", err)
	}
	if !closedReplay || closedReplayID != runID {
		t.Fatalf("expected closed workload replay of %s, got id=%s replayed=%v", runID, closedReplayID, closedReplay)
	}
	if _, _, err := db.StartAgentRunIdempotent("run-key-2", workloadID, "codex", "codex", "codex exec", "/home/user/repo-a"); err == nil || IsIdempotencyConflict(err) {
		t.Fatalf("expected closed workload rejection for new key, got %v", err)
	}
	var runCount int
	if err := db.db.QueryRow(`SELECT COUNT(*) FROM agent_runs`).Scan(&runCount); err != nil {
		t.Fatalf("count agent runs: %v", err)
	}
	if runCount != 1 {
		t.Fatalf("expected one agent run, got %d", runCount)
	}
	stats, err := db.GetControlIdempotencyStats()
	if err != nil {
		t.Fatalf("GetControlIdempotencyStats: %v", err)
	}
	if stats.TotalKeys != 2 || stats.ReplayedKeys != 2 || stats.ReplayCount != 3 || len(stats.Operations) != 2 {
		t.Fatalf("unexpected idempotency stats: %+v", stats)
	}
}

func TestWorkloadLeaseLifecycle(t *testing.T) {
	db := tempDB(t)
	workloadID, err := db.CreateWorkload("lease workload", "codex", "repo-a", "repo-a", "main", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	lease, err := db.AcquireWorkloadLease(workloadID, "router-a", "execute", time.Minute)
	if err != nil {
		t.Fatalf("AcquireWorkloadLease: %v", err)
	}
	if lease.LeaseID == "" || lease.LeaseToken == "" || lease.WorkloadID != workloadID || lease.Status != "active" {
		t.Fatalf("unexpected lease: %+v", lease)
	}
	var tokenHash string
	if err := db.db.QueryRow(`SELECT token_hash FROM workload_leases WHERE lease_id=?`, lease.LeaseID).Scan(&tokenHash); err != nil {
		t.Fatalf("token_hash: %v", err)
	}
	if tokenHash == lease.LeaseToken || !strings.HasPrefix(tokenHash, "sha256:") {
		t.Fatalf("lease token was not stored as a sha256 hash: token=%q hash=%q", lease.LeaseToken, tokenHash)
	}
	if _, err := db.AcquireWorkloadLease(workloadID, "router-b", "execute", time.Minute); !IsWorkloadLeaseConflict(err) {
		t.Fatalf("expected lease conflict, got %v", err)
	}
	if _, err := db.RenewWorkloadLease(lease.LeaseID, "wrong-token", time.Minute); err == nil {
		t.Fatal("expected wrong token renewal rejection")
	}
	renewed, err := db.RenewWorkloadLease(lease.LeaseID, lease.LeaseToken, 2*time.Minute)
	if err != nil {
		t.Fatalf("RenewWorkloadLease: %v", err)
	}
	if renewed.LastRenewedAt == "" || renewed.TTLSeconds <= 0 {
		t.Fatalf("unexpected renewed lease: %+v", renewed)
	}
	active, err := db.ListWorkloadLeases(false, 10)
	if err != nil {
		t.Fatalf("ListWorkloadLeases active: %v", err)
	}
	if len(active) != 1 || active[0].LeaseToken != "" {
		t.Fatalf("unexpected active leases: %+v", active)
	}
	released, err := db.ReleaseWorkloadLease(lease.LeaseID, lease.LeaseToken)
	if err != nil {
		t.Fatalf("ReleaseWorkloadLease: %v", err)
	}
	if released.Status != "released" || released.ReleasedAt == "" {
		t.Fatalf("unexpected released lease: %+v", released)
	}
	next, err := db.AcquireWorkloadLease(workloadID, "router-b", "resume", time.Minute)
	if err != nil {
		t.Fatalf("reacquire after release: %v", err)
	}
	if next.LeaseID == lease.LeaseID {
		t.Fatalf("expected new lease id after release, got %s", next.LeaseID)
	}
	stats, err := db.GetWorkloadLeaseStats()
	if err != nil {
		t.Fatalf("GetWorkloadLeaseStats: %v", err)
	}
	if stats.Active != 1 || stats.Released != 1 || stats.Total != 2 {
		t.Fatalf("unexpected lease stats: %+v", stats)
	}
}

func TestClaimNextWorkloadLifecycle(t *testing.T) {
	db := tempDB(t)
	firstID, err := db.CreateWorkload("queued first", "codex", "repo-a", "repo-a", "main", "alice", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload first: %v", err)
	}
	secondID, err := db.CreateWorkload("queued second", "opencode", "repo-b", "repo-b", "main", "bob", "infra", 0)
	if err != nil {
		t.Fatalf("CreateWorkload second: %v", err)
	}
	closedID, err := db.CreateWorkload("closed claim candidate", "codex", "repo-a", "repo-a", "main", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload closed: %v", err)
	}
	if err := db.CloseWorkload(closedID, "completed", "done"); err != nil {
		t.Fatalf("CloseWorkload: %v", err)
	}
	first, err := db.ClaimNextWorkload("router-a", "execute private task", time.Minute, WorkloadClaimFilter{Source: "codex"})
	if err != nil {
		t.Fatalf("ClaimNextWorkload first: %v", err)
	}
	if !first.OK || first.Empty || first.WorkloadID != firstID || first.Lease == nil || first.Lease.LeaseToken == "" || first.Workload == nil {
		t.Fatalf("unexpected first claim: %+v", first)
	}
	var tokenHash string
	if err := db.db.QueryRow(`SELECT token_hash FROM workload_leases WHERE lease_id=?`, first.Lease.LeaseID).Scan(&tokenHash); err != nil {
		t.Fatalf("token_hash: %v", err)
	}
	if tokenHash == first.Lease.LeaseToken || !strings.HasPrefix(tokenHash, "sha256:") {
		t.Fatalf("lease token was not hashed: token=%q hash=%q", first.Lease.LeaseToken, tokenHash)
	}
	emptyCodex, err := db.ClaimNextWorkload("router-b", "execute", time.Minute, WorkloadClaimFilter{Source: "codex"})
	if err != nil {
		t.Fatalf("ClaimNextWorkload empty codex: %v", err)
	}
	if !emptyCodex.OK || !emptyCodex.Empty || emptyCodex.Lease != nil || emptyCodex.WorkloadID != "" {
		t.Fatalf("expected codex queue to be empty after active lease and terminal exclusion: %+v", emptyCodex)
	}
	second, err := db.ClaimNextWorkload("router-c", "execute", time.Minute, WorkloadClaimFilter{Team: "infra"})
	if err != nil {
		t.Fatalf("ClaimNextWorkload second: %v", err)
	}
	if second.WorkloadID != secondID {
		t.Fatalf("expected team-filtered second workload, got %+v", second)
	}
	if _, err := db.ReleaseWorkloadLease(first.Lease.LeaseID, first.Lease.LeaseToken); err != nil {
		t.Fatalf("ReleaseWorkloadLease first: %v", err)
	}
	reclaimed, err := db.ClaimNextWorkload("router-d", "resume", time.Minute, WorkloadClaimFilter{Status: "any", Query: "first"})
	if err != nil {
		t.Fatalf("ClaimNextWorkload reclaimed: %v", err)
	}
	if reclaimed.WorkloadID != firstID || reclaimed.Lease == nil || reclaimed.Lease.LeaseID == first.Lease.LeaseID {
		t.Fatalf("expected released workload to be reclaimable with a new lease: %+v", reclaimed)
	}
	if _, err := db.ClaimNextWorkload("router-e", "bad", time.Minute, WorkloadClaimFilter{Status: "completed"}); err == nil {
		t.Fatal("expected terminal status claim rejection")
	}
}

func TestWorkloadLeaseReadPathsDoNotExpireByWriteback(t *testing.T) {
	db := tempDB(t)
	workloadID, err := db.CreateWorkload("expired lease workload", "codex", "repo-a", "repo-a", "main", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	lease, err := db.AcquireWorkloadLease(workloadID, "router-a", "execute", time.Minute)
	if err != nil {
		t.Fatalf("AcquireWorkloadLease: %v", err)
	}
	if _, err := db.db.Exec(`UPDATE workload_leases SET expires_at=? WHERE lease_id=?`, time.Now().UTC().Add(-time.Minute), lease.LeaseID); err != nil {
		t.Fatalf("force expire lease: %v", err)
	}
	stats, err := db.GetWorkloadLeaseStats()
	if err != nil {
		t.Fatalf("GetWorkloadLeaseStats: %v", err)
	}
	if stats.Active != 0 || stats.Expired != 1 {
		t.Fatalf("unexpected derived stats: %+v", stats)
	}
	active, err := db.ListWorkloadLeases(false, 10)
	if err != nil {
		t.Fatalf("ListWorkloadLeases active: %v", err)
	}
	if len(active) != 0 {
		t.Fatalf("expired active lease should not be listed as active: %+v", active)
	}
	all, err := db.ListWorkloadLeases(true, 10)
	if err != nil {
		t.Fatalf("ListWorkloadLeases all: %v", err)
	}
	if len(all) != 1 || all[0].Status != "expired" || !all[0].Expired {
		t.Fatalf("expected derived expired row: %+v", all)
	}
	var storedStatus string
	if err := db.db.QueryRow(`SELECT status FROM workload_leases WHERE lease_id=?`, lease.LeaseID).Scan(&storedStatus); err != nil {
		t.Fatalf("stored status: %v", err)
	}
	if storedStatus != "active" {
		t.Fatalf("read path mutated stored status: %s", storedStatus)
	}
}

func TestLinkWorkloadsCreatesDependencyEdge(t *testing.T) {
	db := tempDB(t)
	childID, err := db.CreateWorkload("child goal", "codex", "repo-a", "repo-a", "main", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload child: %v", err)
	}
	parentID, err := db.CreateWorkload("parent goal", "codex", "repo-a", "repo-a", "main", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload parent: %v", err)
	}
	linkID, err := db.LinkWorkloads(childID, parentID, "depends-on", "requires parent evidence", "local-test", 0.8)
	if err != nil {
		t.Fatalf("LinkWorkloads: %v", err)
	}
	dupID, err := db.LinkWorkloads(childID, parentID, "depends_on", "updated reason", "local-test", 0.9)
	if err != nil {
		t.Fatalf("LinkWorkloads duplicate: %v", err)
	}
	if dupID != linkID {
		t.Fatalf("expected stable duplicate link id, got %s want %s", dupID, linkID)
	}
	links, err := db.GetWorkloadLinks(childID)
	if err != nil {
		t.Fatalf("GetWorkloadLinks: %v", err)
	}
	if len(links) != 1 || links[0].Relation != "depends_on" || links[0].Reason != "updated reason" || links[0].Confidence != 0.9 {
		t.Fatalf("unexpected links: %+v", links)
	}
	if _, err := db.LinkWorkloads(childID, childID, "relates_to", "", "", 1); err == nil {
		t.Fatal("expected self-link rejection")
	}
	if _, err := db.LinkWorkloads(childID, "missing", "relates_to", "", "", 1); err == nil {
		t.Fatal("expected missing target rejection")
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
	if feed.GeneratedAt == "" || feed.Cursor == "" || !strings.HasPrefix(feed.Cursor, "sha256:") {
		t.Fatalf("feed missing generated_at or cursor: %+v", feed)
	}
	if !feedHasSeverity(feed.Rows, blockedID, "critical") || !feedHasSeverity(feed.Rows, staleID, "warning") {
		t.Fatalf("unexpected feed severities: %+v", feed.Rows)
	}
	sameFeed, err := db.GetWorkloadEventFeed(now.AddDate(0, 0, -1), now.AddDate(0, 0, 1), "codex", "", "", "", "", 10, 10*time.Minute)
	if err != nil {
		t.Fatalf("GetWorkloadEventFeed same: %v", err)
	}
	if sameFeed.Cursor != feed.Cursor {
		t.Fatalf("same feed rows should keep stable cursor: %q != %q", sameFeed.Cursor, feed.Cursor)
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
