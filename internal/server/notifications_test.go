package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/notifications"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestWebhookNotificationEndpointSendsRedactedPayload(t *testing.T) {
	db := testServerDB(t)
	now := time.Now().UTC()
	workloadID := createStaleWebhookWorkload(t, db, now)
	var payload notifications.WebhookPayload
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected webhook request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode webhook payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer receiver.Close()

	srv := New(db, "", Options{Webhooks: config.WebhookConfig{Enabled: true, URL: receiver.URL, Timeout: time.Second, MaxEvents: 10}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/notifications/webhook?from="+now.AddDate(0, 0, -1).Format("2006-01-02")+"&to="+now.Format("2006-01-02")+"&severity=warning&max_age=10m", nil)
	rr := httptest.NewRecorder()
	srv.handleWebhookNotification(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("notify status=%d body=%s", rr.Code, rr.Body.String())
	}
	if payload.Summary.Total != 1 || len(payload.Events) != 1 {
		t.Fatalf("unexpected payload: %+v", payload)
	}
	event := payload.Events[0]
	if event.WorkloadID == workloadID || event.Goal != "<redacted>" || event.Project != "<redacted>" || event.Repo != "<redacted>" || event.GitBranch != "<redacted>" || event.Team != "<redacted>" {
		t.Fatalf("webhook payload leaked sensitive data: %+v", event)
	}
	audit, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if !strings.Contains(string(rawAudit), "notification.webhook") || strings.Contains(string(rawAudit), receiver.URL) || strings.Contains(string(rawAudit), "private-project") {
		t.Fatalf("unexpected audit log: %s", string(rawAudit))
	}
}

func TestWebhookNotificationEndpointFailsWhenDisabled(t *testing.T) {
	db := testServerDB(t)
	now := time.Now().UTC()
	createStaleWebhookWorkload(t, db, now)
	srv := New(db, "", Options{Webhooks: config.WebhookConfig{Enabled: false}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/notifications/webhook?from="+now.AddDate(0, 0, -1).Format("2006-01-02")+"&to="+now.Format("2006-01-02")+"&severity=warning", nil)
	rr := httptest.NewRecorder()
	srv.handleWebhookNotification(rr, req)
	if rr.Code != http.StatusBadRequest || !strings.Contains(rr.Body.String(), "webhook disabled") {
		t.Fatalf("expected disabled failure, status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestWebhookNotificationEndpointDryRunIncludesApprovals(t *testing.T) {
	db := testServerDB(t)
	now := time.Now().UTC()
	if _, err := db.CreateApprovalRequest(storage.ApprovalRequest{
		RequestID:        "apr-private",
		WorkloadID:       "wl-private-approval",
		RunID:            "run-private-approval",
		Source:           "codex",
		Model:            "gpt-5.5",
		Project:          "private-project",
		Action:           "model.call",
		Target:           "C:/private/workspace",
		ActorRole:        "operator",
		Status:           "pending",
		ApproverHint:     "desk-lead",
		EscalationTarget: "research-head",
		DueAt:            now.Add(30 * time.Minute).Format(time.RFC3339Nano),
		Reason:           "private approval reason",
		RequestPayload:   `{"prompt":"do-not-send"}`,
	}); err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}
	srv := New(db, "", Options{RBAC: config.RBACConfig{ReadOnly: true}, Webhooks: config.WebhookConfig{Enabled: false, MaxEvents: 10}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/notifications/webhook?dry_run=1&from="+now.AddDate(0, 0, -1).Format("2006-01-02")+"&to="+now.Format("2006-01-02"), nil)
	rr := httptest.NewRecorder()
	srv.handleWebhookNotification(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("dry-run notify status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Result  notifications.DeliveryResult `json:"result"`
		Payload notifications.WebhookPayload `json:"payload"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode dry-run body: %v", err)
	}
	if !body.Result.DryRun || body.Result.ApprovalCount != 1 || body.Result.ApprovalRouteCount != 1 || body.Payload.Summary.PendingApprovals != 1 || body.Payload.Summary.ApprovalRoutes != 1 || len(body.Payload.Approvals) != 1 || body.Payload.Routes == nil || len(body.Payload.Routes.Routes) != 1 {
		t.Fatalf("expected approval notification summary: %+v", body)
	}
	approval := body.Payload.Approvals[0]
	if approval.WorkloadID == "wl-private-approval" || approval.RunID == "run-private-approval" || approval.Project != "<redacted>" || approval.Target != "<redacted>" || approval.Reason != "<redacted>" {
		t.Fatalf("approval notification was not redacted: %+v", approval)
	}
	raw, _ := json.Marshal(body)
	if strings.Contains(string(raw), "private-project") || strings.Contains(string(raw), "C:/private/workspace") || strings.Contains(string(raw), "do-not-send") || strings.Contains(string(raw), "desk-lead") || strings.Contains(string(raw), "research-head") {
		t.Fatalf("approval notification leaked sensitive data: %s", string(raw))
	}
	audit, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatalf("audit: %v", err)
	}
	if len(audit) != 0 {
		t.Fatalf("read-only dry-run should not append audit log: %+v", audit)
	}
	sendReq := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/notifications/webhook?from="+now.AddDate(0, 0, -1).Format("2006-01-02")+"&to="+now.Format("2006-01-02"), nil)
	sendRR := httptest.NewRecorder()
	srv.handleWebhookNotification(sendRR, sendReq)
	if sendRR.Code != http.StatusForbidden || !strings.Contains(sendRR.Body.String(), "read-only mode") {
		t.Fatalf("read-only send should be forbidden, status=%d body=%s", sendRR.Code, sendRR.Body.String())
	}
}

func createStaleWebhookWorkload(t *testing.T, db *storage.DB, now time.Time) string {
	t.Helper()
	workloadID, err := db.CreateWorkload("private webhook goal", "codex", "private-project", "zhenzhis/private-project", "feature/private", "", "research", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "codex", "codex", "C:/private/workspace")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-webhook-stale", runID, "working", "testing", "private message", 0.6, nil, now.Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("RecordAgentRunHeartbeat: %v", err)
	}
	return workloadID
}
