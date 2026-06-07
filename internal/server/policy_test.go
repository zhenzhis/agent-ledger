package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestExportPolicyBlock(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{Policies: config.PolicyConfig{
		Enabled: true,
		Rules: []config.PolicyRule{{
			Name: "block-export", Scope: "action", Match: "export", Action: "block", Message: "exports disabled",
		}},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/export?type=sessions&format=csv&from=2026-06-07&to=2026-06-07", nil)
	rr := httptest.NewRecorder()
	srv.handleExport(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestExportPolicyWarnRecordsAudit(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{Policies: config.PolicyConfig{
		Enabled: true,
		Rules: []config.PolicyRule{{
			Name: "warn-export", Scope: "action", Match: "export", Action: "warn", Message: "audit export",
		}},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/export?type=sessions&format=csv&from=2026-06-07&to=2026-06-07", nil)
	rr := httptest.NewRecorder()
	srv.handleExport(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	events, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	seenPolicy, seenExport := false, false
	for _, event := range events {
		if event.Action == "policy.evaluate" {
			seenPolicy = true
		}
		if event.Action == "export" && event.Target == "sessions" {
			seenExport = true
		}
	}
	if !seenPolicy || !seenExport {
		t.Fatalf("expected policy and export audit rows, got %+v", events)
	}
}

func TestExportPolicyRequireApprovalCreatesRequestAndApprovedRequestAllows(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{Policies: config.PolicyConfig{
		Enabled: true,
		Rules: []config.PolicyRule{{
			Name: "approve-export", Scope: "action", Match: "export", Action: "require_approval", Message: "review export",
		}},
	}})
	req := httptest.NewRequest(http.MethodGet, "/api/export?type=sessions&format=csv&from=2026-06-07&to=2026-06-07", nil)
	rr := httptest.NewRecorder()
	srv.handleExport(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d body=%s", rr.Code, rr.Body.String())
	}
	rows, err := db.ListApprovalRequests("pending", 10)
	if err != nil {
		t.Fatalf("approvals: %v", err)
	}
	if len(rows) != 1 || rows[0].Action != "export" || rows[0].Target != "sessions" {
		t.Fatalf("unexpected approvals: %+v", rows)
	}
	body, _ := json.Marshal(map[string]string{"request_id": rows[0].RequestID, "status": "approved", "note": "ok"})
	resolveReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/policy/approvals", bytes.NewReader(body))
	resolveRR := httptest.NewRecorder()
	srv.handlePolicyApprovals(resolveRR, resolveReq)
	if resolveRR.Code != http.StatusOK {
		t.Fatalf("resolve approval status=%d body=%s", resolveRR.Code, resolveRR.Body.String())
	}
	retry := httptest.NewRequest(http.MethodGet, "/api/export?type=sessions&format=csv&from=2026-06-07&to=2026-06-07&approval_id="+rows[0].RequestID, nil)
	retryRR := httptest.NewRecorder()
	srv.handleExport(retryRR, retry)
	if retryRR.Code != http.StatusOK {
		t.Fatalf("approved retry status=%d body=%s", retryRR.Code, retryRR.Body.String())
	}
}

func TestPolicyAuditAPIReportsAndRedactsMatches(t *testing.T) {
	db := testServerDB(t)
	ts := time.Date(2026, 6, 7, 13, 30, 0, 0, time.UTC)
	if err := db.InsertUsage(&storage.UsageRecord{
		Source:       "codex",
		SessionID:    "sess-private",
		Model:        "gpt-5.5",
		InputTokens:  100,
		OutputTokens: 25,
		CostUSD:      0.5,
		Timestamp:    ts,
		Project:      "private-project",
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	srv := New(db, "", Options{Policies: config.PolicyConfig{
		Enabled: true,
		Rules: []config.PolicyRule{{
			Name: "warn-gpt", Scope: "model", Match: "gpt-5.5", Action: "warn", Message: "review model spend",
		}},
	}, Privacy: config.PrivacyConfig{ScreenshotMode: true}})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/policy/audit?from=2026-06-07&to=2026-06-07&privacy=1", nil)
	rr := httptest.NewRecorder()
	srv.handlePolicyAudit(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("policy audit status=%d body=%s", rr.Code, rr.Body.String())
	}
	var report struct {
		Matches int `json:"matches"`
		Rows    []struct {
			Project   string `json:"project"`
			SessionID string `json:"session_id"`
			Evidence  string `json:"evidence"`
		} `json:"rows"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if report.Matches != 1 || len(report.Rows) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Rows[0].Project != "<redacted>" || report.Rows[0].SessionID == "sess-private" || report.Rows[0].Evidence != "<redacted>" {
		t.Fatalf("privacy redaction failed: %+v", report.Rows[0])
	}
}

func TestRepairProjectionAPI(t *testing.T) {
	db := testServerDB(t)
	ts := time.Date(2026, 6, 7, 13, 30, 0, 0, time.UTC)
	payload, err := json.Marshal(map[string]interface{}{
		"goal":                    "api repair projection",
		"call_id":                 "call-api-repair",
		"input_tokens":            20,
		"cache_read_input_tokens": 7,
		"output_tokens":           5,
		"cost_usd":                1.25,
		"pricing_source":          "openai-official",
		"pricing_confidence":      "official",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.IngestCanonicalEvent(storage.CanonicalEvent{
		EventID:   "evt-api-repair",
		Source:    "gateway",
		EventType: "model.call",
		SessionID: "sess-api-repair",
		Model:     "gpt-5",
		Project:   "agent-ledger",
		Timestamp: ts,
		Payload:   payload,
	}); err != nil {
		t.Fatalf("IngestCanonicalEvent: %v", err)
	}
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/projections/repair?from=2026-06-07&to=2026-06-07&source=gateway&model=gpt-5&project=agent-ledger", nil)
	rr := httptest.NewRecorder()
	srv.handleRepairProjections(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("repair status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		OK     bool                           `json:"ok"`
		Result storage.ProjectionRepairResult `json:"result"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if !body.OK || body.Result.After.MissingUsageProjection != 0 {
		t.Fatalf("unexpected repair response: %+v", body)
	}
	events, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	found := false
	for _, event := range events {
		if event.Action == "projections.repair" {
			found = true
		}
	}
	if !found {
		t.Fatalf("repair audit event missing: %+v", events)
	}
}

func testServerDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
