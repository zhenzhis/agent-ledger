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
	if !strings.Contains(body, "event: workload_events") || !strings.Contains(body, `"rows"`) {
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
