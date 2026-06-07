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
