package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestAgentRunHeartbeatAPI(t *testing.T) {
	db := testServerDB(t)
	workloadID, err := db.CreateWorkload("async api workload", "codex", "agent-ledger", "agent-ledger", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "codex", "codex", "C:/work/agent-ledger")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	body, _ := json.Marshal(map[string]interface{}{
		"run_id":   runID,
		"status":   "working",
		"phase":    "testing",
		"progress": 0.75,
		"message":  "running tests",
		"metrics":  map[string]interface{}{"tests_seen": 12},
	})
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/agent-runs/heartbeat", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handleAgentRunHeartbeat(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("heartbeat status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		OK        bool                     `json:"ok"`
		Heartbeat storage.AgentRunEventRow `json:"heartbeat"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.OK || response.Heartbeat.RunID != runID || response.Heartbeat.Progress != 0.75 {
		t.Fatalf("unexpected heartbeat response: %+v", response)
	}
	detail, err := db.GetWorkloadDetail(workloadID)
	if err != nil {
		t.Fatalf("GetWorkloadDetail: %v", err)
	}
	if len(detail.Runs) != 1 || detail.Runs[0].HeartbeatCount != 1 || detail.Runs[0].Phase != "testing" {
		t.Fatalf("run snapshot not updated: %+v", detail.Runs)
	}
	audit, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if !strings.Contains(string(rawAudit), "agent_run.heartbeat") || strings.Contains(string(rawAudit), "running tests") {
		t.Fatalf("unexpected audit log: %s", string(rawAudit))
	}
}

func TestAgentRunStartAPI(t *testing.T) {
	db := testServerDB(t)
	workloadID, err := db.CreateWorkload("start api workload", "codex", "agent-ledger", "agent-ledger", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	body, _ := json.Marshal(map[string]interface{}{
		"workload_id": workloadID,
		"source":      "codex",
		"agent_name":  "codex",
		"command":     "codex --token secret-value",
		"cwd":         "C:/private/workspace",
	})
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/agent-runs", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handleAgentRuns(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("start run status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		OK         bool   `json:"ok"`
		WorkloadID string `json:"workload_id"`
		RunID      string `json:"run_id"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.OK || response.WorkloadID != workloadID || response.RunID == "" || response.Status != "running" {
		t.Fatalf("unexpected response: %+v", response)
	}
	detail, err := db.GetWorkloadDetail(workloadID)
	if err != nil {
		t.Fatalf("GetWorkloadDetail: %v", err)
	}
	if len(detail.Runs) != 1 || detail.Runs[0].RunID != response.RunID {
		t.Fatalf("run not attached: %+v", detail.Runs)
	}
	audit, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if !strings.Contains(string(rawAudit), "agent_run.start") || strings.Contains(string(rawAudit), "secret-value") || strings.Contains(string(rawAudit), "C:/private") {
		t.Fatalf("unexpected audit log: %s", string(rawAudit))
	}
}

func TestWorkloadAndRunAPIIdempotency(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	createBody, _ := json.Marshal(map[string]interface{}{
		"goal":    "idempotent api workload",
		"source":  "codex",
		"project": "agent-ledger",
	})
	first := httptest.NewRecorder()
	firstReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads", bytes.NewReader(createBody))
	firstReq.Header.Set("Idempotency-Key", "api-workload-key")
	srv.handleWorkloadCreate(first, firstReq)
	if first.Code != http.StatusOK {
		t.Fatalf("first workload status=%d body=%s", first.Code, first.Body.String())
	}
	var firstCreate struct {
		WorkloadID       string `json:"workload_id"`
		IdempotentReplay bool   `json:"idempotent_replay"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstCreate); err != nil {
		t.Fatalf("decode first workload: %v", err)
	}
	second := httptest.NewRecorder()
	secondReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads", bytes.NewReader(createBody))
	secondReq.Header.Set("Idempotency-Key", "api-workload-key")
	srv.handleWorkloadCreate(second, secondReq)
	if second.Code != http.StatusOK {
		t.Fatalf("second workload status=%d body=%s", second.Code, second.Body.String())
	}
	var secondCreate struct {
		WorkloadID       string `json:"workload_id"`
		IdempotentReplay bool   `json:"idempotent_replay"`
	}
	if err := json.Unmarshal(second.Body.Bytes(), &secondCreate); err != nil {
		t.Fatalf("decode second workload: %v", err)
	}
	if firstCreate.WorkloadID == "" || secondCreate.WorkloadID != firstCreate.WorkloadID || !secondCreate.IdempotentReplay {
		t.Fatalf("unexpected workload replay first=%+v second=%+v", firstCreate, secondCreate)
	}
	conflictBody, _ := json.Marshal(map[string]interface{}{"goal": "changed goal", "source": "codex", "project": "agent-ledger"})
	conflictResp := httptest.NewRecorder()
	conflictReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads", bytes.NewReader(conflictBody))
	conflictReq.Header.Set("Idempotency-Key", "api-workload-key")
	srv.handleWorkloadCreate(conflictResp, conflictReq)
	if conflictResp.Code != http.StatusConflict {
		t.Fatalf("expected conflict status, got %d body=%s", conflictResp.Code, conflictResp.Body.String())
	}

	runBody, _ := json.Marshal(map[string]interface{}{
		"workload_id": firstCreate.WorkloadID,
		"source":      "codex",
		"agent_name":  "codex",
		"command":     "codex exec",
	})
	runFirst := httptest.NewRecorder()
	runFirstReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/agent-runs", bytes.NewReader(runBody))
	runFirstReq.Header.Set("Idempotency-Key", "api-run-key")
	srv.handleAgentRuns(runFirst, runFirstReq)
	if runFirst.Code != http.StatusOK {
		t.Fatalf("first run status=%d body=%s", runFirst.Code, runFirst.Body.String())
	}
	var firstRun struct {
		RunID            string `json:"run_id"`
		IdempotentReplay bool   `json:"idempotent_replay"`
	}
	if err := json.Unmarshal(runFirst.Body.Bytes(), &firstRun); err != nil {
		t.Fatalf("decode first run: %v", err)
	}
	runSecond := httptest.NewRecorder()
	runSecondReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/agent-runs", bytes.NewReader(runBody))
	runSecondReq.Header.Set("Idempotency-Key", "api-run-key")
	srv.handleAgentRuns(runSecond, runSecondReq)
	if runSecond.Code != http.StatusOK {
		t.Fatalf("second run status=%d body=%s", runSecond.Code, runSecond.Body.String())
	}
	var secondRun struct {
		RunID            string `json:"run_id"`
		IdempotentReplay bool   `json:"idempotent_replay"`
	}
	if err := json.Unmarshal(runSecond.Body.Bytes(), &secondRun); err != nil {
		t.Fatalf("decode second run: %v", err)
	}
	if firstRun.RunID == "" || secondRun.RunID != firstRun.RunID || !secondRun.IdempotentReplay {
		t.Fatalf("unexpected run replay first=%+v second=%+v", firstRun, secondRun)
	}
}

func TestWorkloadLinkAPI(t *testing.T) {
	db := testServerDB(t)
	sourceID, err := db.CreateWorkload("source link workload", "codex", "agent-ledger", "agent-ledger", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload source: %v", err)
	}
	targetID, err := db.CreateWorkload("target link workload", "codex", "agent-ledger", "agent-ledger", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload target: %v", err)
	}
	body, _ := json.Marshal(map[string]interface{}{
		"source_workload_id": sourceID,
		"target_workload_id": targetID,
		"relation":           "spawned-by",
		"reason":             "private parent task summary",
		"created_by":         "local-wrapper",
	})
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads/link", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	srv.handleWorkloadLink(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("link status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		OK     bool   `json:"ok"`
		LinkID string `json:"link_id"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !response.OK || response.LinkID == "" {
		t.Fatalf("unexpected response: %+v", response)
	}
	detail, err := db.GetWorkloadDetail(sourceID)
	if err != nil {
		t.Fatalf("GetWorkloadDetail: %v", err)
	}
	if len(detail.Links) != 1 || detail.Links[0].TargetWorkloadID != targetID || detail.Links[0].Relation != "spawned_by" {
		t.Fatalf("link not attached: %+v", detail.Links)
	}
	audit, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if !strings.Contains(string(rawAudit), "workload.link") || strings.Contains(string(rawAudit), "private parent task summary") {
		t.Fatalf("unexpected audit log: %s", string(rawAudit))
	}
}

func TestWorkloadLeaseAPI(t *testing.T) {
	db := testServerDB(t)
	workloadID, err := db.CreateWorkload("lease api workload", "codex", "agent-ledger", "agent-ledger", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	srv := New(db, "", Options{})
	body, _ := json.Marshal(map[string]interface{}{
		"workload_id": workloadID,
		"holder":      "router-a",
		"purpose":     "execute private task",
		"ttl_seconds": 120,
	})
	first := httptest.NewRecorder()
	srv.handleWorkloadLeaseAcquire(first, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads/lease", bytes.NewReader(body)))
	if first.Code != http.StatusOK {
		t.Fatalf("lease acquire status=%d body=%s", first.Code, first.Body.String())
	}
	var acquire struct {
		OK    bool                  `json:"ok"`
		Lease storage.WorkloadLease `json:"lease"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &acquire); err != nil {
		t.Fatalf("decode acquire: %v", err)
	}
	if !acquire.OK || acquire.Lease.LeaseID == "" || acquire.Lease.LeaseToken == "" || acquire.Lease.WorkloadID != workloadID {
		t.Fatalf("unexpected acquire response: %+v", acquire)
	}
	conflictResp := httptest.NewRecorder()
	srv.handleWorkloadLeaseAcquire(conflictResp, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads/lease", bytes.NewReader(body)))
	if conflictResp.Code != http.StatusConflict {
		t.Fatalf("expected lease conflict, got %d body=%s", conflictResp.Code, conflictResp.Body.String())
	}
	renewBody, _ := json.Marshal(map[string]interface{}{"lease_id": acquire.Lease.LeaseID, "lease_token": acquire.Lease.LeaseToken, "ttl_seconds": 240})
	renewResp := httptest.NewRecorder()
	srv.handleWorkloadLeaseRenew(renewResp, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads/lease/renew", bytes.NewReader(renewBody)))
	if renewResp.Code != http.StatusOK {
		t.Fatalf("renew status=%d body=%s", renewResp.Code, renewResp.Body.String())
	}
	listReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workloads/leases?privacy=1", nil)
	listResp := httptest.NewRecorder()
	srv.handleWorkloadLeases(listResp, listReq)
	if listResp.Code != http.StatusOK {
		t.Fatalf("list leases status=%d body=%s", listResp.Code, listResp.Body.String())
	}
	if strings.Contains(listResp.Body.String(), acquire.Lease.LeaseToken) || strings.Contains(listResp.Body.String(), "router-a") || strings.Contains(listResp.Body.String(), "execute private task") {
		t.Fatalf("lease list leaked private fields: %s", listResp.Body.String())
	}
	releaseBody, _ := json.Marshal(map[string]interface{}{"lease_id": acquire.Lease.LeaseID, "lease_token": acquire.Lease.LeaseToken})
	releaseResp := httptest.NewRecorder()
	srv.handleWorkloadLeaseRelease(releaseResp, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads/lease/release", bytes.NewReader(releaseBody)))
	if releaseResp.Code != http.StatusOK {
		t.Fatalf("release status=%d body=%s", releaseResp.Code, releaseResp.Body.String())
	}
	audit, err := db.GetAuditLog(20)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if !strings.Contains(string(rawAudit), "workload.lease.acquire") || strings.Contains(string(rawAudit), acquire.Lease.LeaseToken) || strings.Contains(string(rawAudit), "execute private task") {
		t.Fatalf("unexpected lease audit log: %s", string(rawAudit))
	}
}

func TestWorkloadClaimNextAPI(t *testing.T) {
	db := testServerDB(t)
	workloadID, err := db.CreateWorkload("claim private workload", "codex", "agent-ledger", "secret/repo", "feature/private", "alice", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	srv := New(db, "", Options{})
	queueBefore := httptest.NewRecorder()
	srv.handleWorkloadQueue(queueBefore, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workloads/queue?source=codex", nil))
	if queueBefore.Code != http.StatusOK {
		t.Fatalf("queue status before claim=%d body=%s", queueBefore.Code, queueBefore.Body.String())
	}
	var beforeStats storage.WorkloadQueueStats
	if err := json.Unmarshal(queueBefore.Body.Bytes(), &beforeStats); err != nil {
		t.Fatalf("decode queue before: %v", err)
	}
	if beforeStats.Claimable != 1 || beforeStats.ActiveLeases != 0 {
		t.Fatalf("unexpected queue before claim: %+v", beforeStats)
	}
	queueBeforeETag := queueBefore.Header().Get("ETag")
	if queueBeforeETag == "" {
		t.Fatalf("queue response missing ETag")
	}
	queueCached := httptest.NewRecorder()
	queueCachedReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workloads/queue?source=codex", nil)
	queueCachedReq.Header.Set("If-None-Match", queueBeforeETag)
	srv.handleWorkloadQueue(queueCached, queueCachedReq)
	if queueCached.Code != http.StatusNotModified {
		t.Fatalf("queue If-None-Match status=%d body=%s", queueCached.Code, queueCached.Body.String())
	}
	body, _ := json.Marshal(map[string]interface{}{
		"holder":      "router-a",
		"purpose":     "private execution purpose",
		"source":      "codex",
		"ttl_seconds": 120,
	})
	rr := httptest.NewRecorder()
	srv.handleWorkloadClaimNext(rr, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads/claim-next", bytes.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("claim-next status=%d body=%s", rr.Code, rr.Body.String())
	}
	var claimed storage.WorkloadClaimResult
	if err := json.Unmarshal(rr.Body.Bytes(), &claimed); err != nil {
		t.Fatalf("decode claim-next: %v", err)
	}
	if !claimed.OK || claimed.Empty || claimed.WorkloadID != workloadID || claimed.Workload == nil || claimed.Lease == nil || claimed.Lease.LeaseToken == "" {
		t.Fatalf("unexpected claim-next response: %+v", claimed)
	}
	queueAfter := httptest.NewRecorder()
	srv.handleWorkloadQueue(queueAfter, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workloads/queue?source=codex", nil))
	if queueAfter.Code != http.StatusOK {
		t.Fatalf("queue status after claim=%d body=%s", queueAfter.Code, queueAfter.Body.String())
	}
	var afterStats storage.WorkloadQueueStats
	if err := json.Unmarshal(queueAfter.Body.Bytes(), &afterStats); err != nil {
		t.Fatalf("decode queue after: %v", err)
	}
	if afterStats.Claimable != 0 || afterStats.ActiveLeases != 1 || afterStats.NextLeaseExpiryAt == "" {
		t.Fatalf("unexpected queue after claim: %+v", afterStats)
	}
	if queueAfter.Header().Get("ETag") == "" || queueAfter.Header().Get("ETag") == queueBeforeETag {
		t.Fatalf("queue ETag did not change after claim: before=%q after=%q", queueBeforeETag, queueAfter.Header().Get("ETag"))
	}
	emptyResp := httptest.NewRecorder()
	srv.handleWorkloadClaimNext(emptyResp, httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/workloads/claim-next", bytes.NewReader(body)))
	if emptyResp.Code != http.StatusOK {
		t.Fatalf("empty claim-next status=%d body=%s", emptyResp.Code, emptyResp.Body.String())
	}
	var empty storage.WorkloadClaimResult
	if err := json.Unmarshal(emptyResp.Body.Bytes(), &empty); err != nil {
		t.Fatalf("decode empty claim-next: %v", err)
	}
	if !empty.OK || !empty.Empty || empty.Lease != nil || empty.WorkloadID != "" {
		t.Fatalf("expected empty queue response: %+v", empty)
	}
	audit, err := db.GetAuditLog(20)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if !strings.Contains(string(rawAudit), "workload.claim_next") || strings.Contains(string(rawAudit), claimed.Lease.LeaseToken) || strings.Contains(string(rawAudit), "private execution purpose") || strings.Contains(string(rawAudit), "secret/repo") {
		t.Fatalf("unexpected claim audit log: %s", string(rawAudit))
	}
}

func TestAgentRunLivenessAPI(t *testing.T) {
	db := testServerDB(t)
	workloadID, err := db.CreateWorkload("sensitive async goal", "codex", "agent-ledger", "zhenzhis/agent-ledger", "feature/private", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "codex", "codex", "C:/work/agent-ledger")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-api-liveness", runID, "working", "testing", "waiting on tests", 0.4, map[string]interface{}{"files_touched": 3}, time.Now().UTC().Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("RecordAgentRunHeartbeat: %v", err)
	}
	srv := New(db, "", Options{Privacy: config.PrivacyConfig{ScreenshotMode: true}})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/agent-runs/liveness?max_age=10m&stale_only=1", nil)
	rr := httptest.NewRecorder()
	srv.handleAgentRunLiveness(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("liveness status=%d body=%s", rr.Code, rr.Body.String())
	}
	var response struct {
		Rows      []storage.AgentRunLivenessRow `json:"rows"`
		MaxAge    string                        `json:"max_age"`
		StaleOnly bool                          `json:"stale_only"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.MaxAge != "10m0s" || !response.StaleOnly || len(response.Rows) != 1 {
		t.Fatalf("unexpected liveness response: %+v", response)
	}
	row := response.Rows[0]
	if row.RunID != runID || !row.Stale || row.Phase != "testing" || row.Progress != 0.4 {
		t.Fatalf("unexpected liveness row: %+v", row)
	}
	if row.Goal != "<redacted>" || row.StatusMessage != "<redacted>" || row.Project != "<redacted>" || row.Repo != "<redacted>" || row.GitBranch != "<redacted>" {
		t.Fatalf("privacy redaction failed: %+v", row)
	}
}

func TestWorkloadStateAPIPrivacy(t *testing.T) {
	db := testServerDB(t)
	workloadID, err := db.CreateWorkload("private terminal-state goal", "codex", "private-project", "zhenzhis/private-project", "feature/private", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "codex", "codex", "C:/private/workspace")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-api-state", runID, "working", "testing", "private message", 0.6, nil, time.Now().UTC().Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("RecordAgentRunHeartbeat: %v", err)
	}
	srv := New(db, "", Options{Privacy: config.PrivacyConfig{ScreenshotMode: true}})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workload-state?workload_id="+workloadID+"&max_age=10m&privacy=1", nil)
	rr := httptest.NewRecorder()
	srv.handleWorkloadState(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("state status=%d body=%s", rr.Code, rr.Body.String())
	}
	var state storage.WorkloadState
	if err := json.Unmarshal(rr.Body.Bytes(), &state); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if state.Phase != "stale" || !state.Stale || state.StaleRuns != 1 || state.Progress != 0.6 {
		t.Fatalf("unexpected state: %+v", state)
	}
	if state.Goal != "<redacted>" || state.Project != "<redacted>" || state.Repo != "<redacted>" || state.GitBranch != "<redacted>" || state.Team != "<redacted>" {
		t.Fatalf("privacy redaction failed: %+v", state)
	}
}

func TestWorkloadEventsAPIPrivacy(t *testing.T) {
	db := testServerDB(t)
	now := time.Now().UTC()
	workloadID, err := db.CreateWorkload("private event feed goal", "codex", "private-project", "zhenzhis/private-project", "feature/private", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "codex", "codex", "C:/private/workspace")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-api-feed", runID, "working", "testing", "private message", 0.6, nil, now.Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("RecordAgentRunHeartbeat: %v", err)
	}
	srv := New(db, "", Options{Privacy: config.PrivacyConfig{ScreenshotMode: true}})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workload-events?from="+now.AddDate(0, 0, -1).Format("2006-01-02")+"&to="+now.Format("2006-01-02")+"&max_age=10m&severity=warning&privacy=1", nil)
	rr := httptest.NewRecorder()
	srv.handleWorkloadEvents(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("events status=%d body=%s", rr.Code, rr.Body.String())
	}
	var feed storage.WorkloadEventFeed
	if err := json.Unmarshal(rr.Body.Bytes(), &feed); err != nil {
		t.Fatalf("decode feed: %v", err)
	}
	if feed.Total != 1 || len(feed.Rows) != 1 {
		t.Fatalf("unexpected feed: %+v", feed)
	}
	if feed.Cursor == "" || !strings.HasPrefix(feed.Cursor, "sha256:") || rr.Header().Get("ETag") == "" {
		t.Fatalf("feed missing cursor or ETag: cursor=%q etag=%q", feed.Cursor, rr.Header().Get("ETag"))
	}
	notModifiedReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workload-events?from="+now.AddDate(0, 0, -1).Format("2006-01-02")+"&to="+now.Format("2006-01-02")+"&max_age=10m&severity=warning&cursor="+feed.Cursor, nil)
	notModifiedRR := httptest.NewRecorder()
	srv.handleWorkloadEvents(notModifiedRR, notModifiedReq)
	if notModifiedRR.Code != http.StatusNotModified {
		t.Fatalf("cursor request status=%d body=%s", notModifiedRR.Code, notModifiedRR.Body.String())
	}
	etagReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workload-events?from="+now.AddDate(0, 0, -1).Format("2006-01-02")+"&to="+now.Format("2006-01-02")+"&max_age=10m&severity=warning", nil)
	etagReq.Header.Set("If-None-Match", rr.Header().Get("ETag"))
	etagRR := httptest.NewRecorder()
	srv.handleWorkloadEvents(etagRR, etagReq)
	if etagRR.Code != http.StatusNotModified {
		t.Fatalf("etag request status=%d body=%s", etagRR.Code, etagRR.Body.String())
	}
	row := feed.Rows[0]
	if row.Phase != "stale" || row.Severity != "warning" || !row.Stale {
		t.Fatalf("unexpected event row: %+v", row)
	}
	if row.WorkloadID == workloadID || row.Goal != "<redacted>" || row.Project != "<redacted>" || row.Repo != "<redacted>" || row.GitBranch != "<redacted>" || row.Team != "<redacted>" {
		t.Fatalf("privacy redaction failed: %+v", row)
	}
}

func TestWorkloadEventsStreamOncePrivacy(t *testing.T) {
	db := testServerDB(t)
	now := time.Now().UTC()
	workloadID, err := db.CreateWorkload("private stream goal", "codex", "private-project", "zhenzhis/private-project", "feature/private", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "codex", "codex", "C:/private/workspace")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-api-stream", runID, "working", "testing", "private message", 0.6, nil, now.Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("RecordAgentRunHeartbeat: %v", err)
	}
	srv := New(db, "", Options{Privacy: config.PrivacyConfig{ScreenshotMode: true}})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/workload-events/stream?from="+now.AddDate(0, 0, -1).Format("2006-01-02")+"&to="+now.Format("2006-01-02")+"&max_age=10m&severity=warning&privacy=1&once=1", nil)
	rr := httptest.NewRecorder()
	srv.handleWorkloadEventsStream(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("stream status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type=%s", got)
	}
	body := rr.Body.String()
	if !strings.Contains(body, "id: sha256:") || !strings.Contains(body, "event: workload_events") || !strings.Contains(body, `"rows"`) {
		t.Fatalf("unexpected SSE body: %s", body)
	}
	if strings.Contains(body, workloadID) || strings.Contains(body, "private-project") || strings.Contains(body, "private stream goal") || strings.Contains(body, "feature/private") {
		t.Fatalf("SSE privacy redaction failed: %s", body)
	}
}

func TestEvidenceBundleIncludesRedactedWorkloadState(t *testing.T) {
	db := testServerDB(t)
	now := time.Now().UTC()
	workloadID, err := db.CreateWorkload("private evidence goal", "codex", "private-project", "zhenzhis/private-project", "feature/private", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	if _, err := db.StartAgentRun(workloadID, "codex", "codex", "codex", "C:/private/workspace"); err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if err := db.UpsertIngestionHealth(storage.IngestionHealth{
		Source:  "codex",
		Enabled: true,
		Paths:   []string{"C:/Users/zhang/private-ledger-path"},
		PathStatus: []storage.PathStatus{{
			Path:     "C:/Users/zhang/private-ledger-path",
			Exists:   true,
			Readable: true,
		}},
	}); err != nil {
		t.Fatalf("UpsertIngestionHealth: %v", err)
	}
	if err := db.InsertUsage(&storage.UsageRecord{
		Source: "codex", SessionID: "private-session", Model: "gpt-5",
		InputTokens: 100000, OutputTokens: 500, CostUSD: 3.5, Timestamp: now, Project: "private-project", GitBranch: "feature/private",
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/evidence-bundle?from="+now.AddDate(0, 0, -1).Format("2006-01-02")+"&to="+now.AddDate(0, 0, 1).Format("2006-01-02"), nil)
	rr := httptest.NewRecorder()
	srv.handleEvidenceBundle(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("evidence status=%d body=%s", rr.Code, rr.Body.String())
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("decode bundle: %v", err)
	}
	rows, ok := bundle["workload_states"].([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("missing workload states: %#v", bundle["workload_states"])
	}
	for _, rawRow := range rows {
		row := rawRow.(map[string]interface{})
		if row["goal"] != "<redacted>" || row["project"] != "<redacted>" || row["repo"] != "<redacted>" || row["git_branch"] != "<redacted>" || row["team"] != "<redacted>" || row["workload_id"] == workloadID {
			t.Fatalf("workload state privacy failed: %#v", row)
		}
	}
	body := rr.Body.String()
	for _, key := range []string{`"dashboard"`, `"pricing_sources"`, `"pricing_rules"`, `"anomaly_events"`, `"watchdog_events"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("evidence bundle missing %s: %s", key, body)
		}
	}
	for _, secret := range []string{"C:/Users/zhang/private-ledger-path", "private-session", "private-project", "feature/private", workloadID} {
		if strings.Contains(body, secret) {
			t.Fatalf("evidence bundle leaked %s: %s", secret, body)
		}
	}
}

func TestWorkloadDetailPrivacyRedactsContextRefs(t *testing.T) {
	detail := &storage.WorkloadDetail{
		Summary: storage.WorkloadSummary{
			WorkloadID: "wl-sensitive",
			Project:    "private-project",
			Repo:       "zhenzhis/private-project",
			GitBranch:  "feature/private",
		},
		ContextRefs: []storage.ContextRefRow{{
			ContextRefID: "ctx-sensitive",
			Label:        "C:/Users/zhang/quant/private-project",
			Repo:         "zhenzhis/private-project",
			GitBranch:    "feature/private",
			CommitSHA:    "abc123",
			RefHash:      "sha256:safe",
		}},
	}
	applyWorkloadDetailPrivacy(detail, config.PrivacyConfig{ScreenshotMode: true})
	ctx := detail.ContextRefs[0]
	if ctx.Label != "<redacted>" || ctx.Repo != "<redacted>" || ctx.GitBranch != "<redacted>" || ctx.CommitSHA != "<redacted>" {
		t.Fatalf("context privacy redaction failed: %+v", ctx)
	}
	if ctx.RefHash != "sha256:safe" {
		t.Fatalf("ref hash should remain visible for evidence correlation: %+v", ctx)
	}
}

func TestWorkloadTimelinePrivacyRedactsSensitiveText(t *testing.T) {
	rows := []storage.WorkloadTimelineRow{
		{Kind: "context_ref", Label: "C:/Users/zhang/quant/private-project", Detail: "zhenzhis/private-project"},
		{Kind: "tool_call", Label: "shell", Detail: "command"},
	}
	applyWorkloadTimelinePrivacy(rows, config.PrivacyConfig{ScreenshotMode: true})
	for _, row := range rows {
		if row.Label != "<redacted>" || row.Detail != "<redacted>" {
			t.Fatalf("timeline privacy redaction failed: %+v", rows)
		}
	}
}
