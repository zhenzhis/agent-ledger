package notifications

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestWebhookDisabledFailsExplicitly(t *testing.T) {
	feed := sampleFeed()
	result, err := SendWebhook(context.Background(), config.WebhookConfig{Enabled: false}, feed, false)
	if err == nil {
		t.Fatal("expected disabled webhook to fail explicitly")
	}
	if result == nil || result.Enabled || result.Message != "webhook disabled" {
		t.Fatalf("unexpected result: %+v err=%v", result, err)
	}
}

func TestWebhookDryRunBuildsRedactedPayload(t *testing.T) {
	feed := sampleFeed()
	result, err := SendWebhook(context.Background(), config.WebhookConfig{Enabled: false, MaxEvents: 10}, feed, true)
	if err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if result == nil || !result.DryRun || result.EventCount != 1 {
		t.Fatalf("unexpected dry-run result: %+v", result)
	}
	payload := BuildWebhookPayload(feed, 10)
	if payload.Events[0].Goal != "<redacted>" || payload.Events[0].Project != "<redacted>" || payload.Events[0].Repo != "<redacted>" || payload.Events[0].GitBranch != "<redacted>" || payload.Events[0].Team != "<redacted>" {
		t.Fatalf("payload was not redacted: %+v", payload.Events[0])
	}
	if payload.Events[0].WorkloadID == "wl-private" || payload.Events[0].EventID == "evt-private" {
		t.Fatalf("ids were not hashed: %+v", payload.Events[0])
	}
	if payload.Summary.BySeverity["warning"] != 1 || payload.Summary.ByPhase["stale"] != 1 {
		t.Fatalf("unexpected summary: %+v", payload.Summary)
	}
}

func TestWebhookPayloadIncludesRedactedApprovalRequests(t *testing.T) {
	approvals := []storage.ApprovalRequest{{
		RequestID:        "apr-private",
		PolicyDecisionID: "pd-private",
		WorkloadID:       "wl-private",
		RunID:            "run-private",
		Source:           "codex",
		Model:            "gpt-5.5",
		Project:          "private-project",
		Action:           "model.call",
		Target:           "C:/private/path",
		ActorRole:        "operator",
		Status:           "pending",
		Reason:           "private approval reason",
		RequestPayload:   `{"secret":"do-not-send"}`,
		CreatedAt:        "2026-06-07T12:00:00Z",
		UpdatedAt:        "2026-06-07T12:00:00Z",
	}}
	payload := BuildWebhookPayloadWithApprovals(sampleFeed(), approvals, 10)
	if payload.Summary.Total != 2 || payload.Summary.PendingApprovals != 1 || len(payload.Approvals) != 1 {
		t.Fatalf("unexpected approval summary: %+v", payload)
	}
	approval := payload.Approvals[0]
	if approval.RequestID == "apr-private" || approval.PolicyDecisionID == "pd-private" || approval.WorkloadID == "wl-private" || approval.RunID == "run-private" {
		t.Fatalf("approval ids were not hashed: %+v", approval)
	}
	if approval.Project != "<redacted>" || approval.Target != "<redacted>" || approval.Reason != "<redacted>" {
		t.Fatalf("approval fields were not redacted: %+v", approval)
	}
	raw, _ := json.Marshal(payload)
	if strings.Contains(string(raw), "private-project") || strings.Contains(string(raw), "C:/private/path") || strings.Contains(string(raw), "do-not-send") {
		t.Fatalf("approval payload leaked sensitive data: %s", string(raw))
	}
}

func TestWebhookPayloadIncludesRedactedApprovalRoutes(t *testing.T) {
	routes := &storage.ApprovalRouteSummary{
		GeneratedAt: "2026-06-07T12:00:00Z",
		DueWithin:   "1h0m0s",
		Summary:     storage.ApprovalRouteSummaryStats{Routes: 1, Pending: 2, DueSoon: 1},
		Routes: []storage.ApprovalRouteRow{{
			RouteKey:         "desk-lead|research-head",
			Approver:         "desk-lead",
			EscalationTarget: "research-head",
			Pending:          2,
			DueSoon:          1,
			Sources:          []string{"codex"},
			Models:           []string{"gpt-5.5"},
			Projects:         []string{"private-project"},
			Actions:          []string{"model.call"},
		}},
	}
	payload := BuildWebhookPayloadWithApprovalRoutes(sampleFeed(), nil, routes, 10)
	if payload.Summary.ApprovalRoutes != 1 || payload.Routes == nil || len(payload.Routes.Routes) != 1 {
		t.Fatalf("expected route summary: %+v", payload)
	}
	route := payload.Routes.Routes[0]
	if route.RouteKeyHash == "" || route.RouteKeyHash == "desk-lead|research-head" || route.ApproverHash == "desk-lead" || route.EscalationTargetHash == "research-head" {
		t.Fatalf("route identifiers were not hashed: %+v", route)
	}
	if len(route.Projects) != 1 || route.Projects[0] != "<redacted>" {
		t.Fatalf("route projects were not redacted: %+v", route)
	}
	raw, _ := json.Marshal(payload)
	for _, forbidden := range []string{"desk-lead", "research-head", "private-project"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("route payload leaked %q: %s", forbidden, string(raw))
		}
	}
}

func TestWebhookSendsOnlyRedactedSummary(t *testing.T) {
	var payload WebhookPayload
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer ts.Close()

	result, err := SendWebhook(context.Background(), config.WebhookConfig{Enabled: true, URL: ts.URL, Timeout: time.Second, MaxEvents: 10}, sampleFeed(), false)
	if err != nil {
		t.Fatalf("send webhook: %v", err)
	}
	if result.StatusCode != http.StatusAccepted || result.EventCount != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if payload.Events[0].Goal != "<redacted>" || payload.Events[0].Project != "<redacted>" || payload.Events[0].WorkloadID == "wl-private" {
		t.Fatalf("sensitive data leaked: %+v", payload.Events[0])
	}
}

func sampleFeed() *storage.WorkloadEventFeed {
	return &storage.WorkloadEventFeed{Rows: []storage.WorkloadFeedEvent{{
		EventID:    "evt-private",
		WorkloadID: "wl-private",
		Goal:       "private goal",
		Source:     "codex",
		Project:    "private-project",
		Repo:       "zhenzhis/private-project",
		GitBranch:  "feature/private",
		Team:       "research",
		Phase:      "stale",
		Severity:   "warning",
		Message:    "workload has stale active agent runs",
		NextAction: "inspect stale agent run heartbeat",
		Timestamp:  time.Now().UTC().Format(time.RFC3339Nano),
		Stale:      true,
	}}}
}
