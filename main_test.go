package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestCLICommandRequiresWriteForNotifyDryRun(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{name: "notify webhook sends", args: []string{"notify", "webhook"}, want: true},
		{name: "notify webhook dry run is read-only", args: []string{"notify", "webhook", "--dry-run"}, want: false},
		{name: "notify webhook dry run with filters is read-only", args: []string{"notify", "webhook", "--severity", "warning", "--dry-run"}, want: false},
		{name: "notify without subcommand remains write", args: []string{"notify"}, want: true},
		{name: "notify other subcommand remains write", args: []string{"notify", "other", "--dry-run"}, want: true},
		{name: "policy routes is read-only", args: []string{"policy", "routes"}, want: false},
		{name: "policy routes privacy is read-only", args: []string{"policy", "routes", "--privacy"}, want: false},
		{name: "policy approvals is read-only", args: []string{"policy", "approvals"}, want: false},
		{name: "policy approvals privacy is read-only", args: []string{"policy", "approvals", "--privacy"}, want: false},
		{name: "policy resolve writes", args: []string{"policy", "resolve", "--id", "apr_1", "--status", "approved"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cliCommandRequiresWrite(tc.args); got != tc.want {
				t.Fatalf("cliCommandRequiresWrite(%v)=%v want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestPolicyRoutesCLIOutputsRedactedSummary(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	if _, err := db.CreateApprovalRequest(storage.ApprovalRequest{
		Source:           "gateway",
		Model:            "gpt-5.5",
		Project:          "private-project",
		Action:           "model.call",
		Target:           "openai-chat-completions",
		Status:           "pending",
		ApproverHint:     "desk-lead",
		EscalationTarget: "research-head",
		DueAt:            time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runPolicyCLI([]string{"routes", "--due-within", "1h", "--privacy"}, config.DefaultConfig(), db)
	})
	if err != nil {
		t.Fatalf("runPolicyCLI routes: %v", err)
	}
	for _, want := range []string{`"pending":1`, `redacted`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
	for _, forbidden := range []string{"private-project", "desk-lead", "research-head"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("output leaked %q: %s", forbidden, out)
		}
	}
}

func TestPolicyApprovalsCLIPrivacyAndResolveAudit(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	requestID, err := db.CreateApprovalRequest(storage.ApprovalRequest{
		PolicyDecisionID:       "dec-cli-private",
		WorkloadID:             "wl-cli-private",
		RunID:                  "run-cli-private",
		Source:                 "gateway",
		Model:                  "gpt-5.5",
		Project:                "private-project",
		Action:                 "model.call",
		Target:                 "openai-chat-completions",
		ActorRole:              "operator",
		Status:                 "pending",
		RequiredApprovals:      1,
		ApproverHint:           "desk-lead",
		EscalationTarget:       "research-head",
		EscalationAfterSeconds: 1800,
		DueAt:                  time.Now().UTC().Add(30 * time.Minute).Format(time.RFC3339Nano),
		Reason:                 "private approval reason",
		RequestPayload:         `{"prompt":"do-not-persist"}`,
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}

	out, err := captureStdout(t, func() error {
		return runPolicyCLI([]string{"approvals", "--status", "pending", "--privacy"}, config.DefaultConfig(), db)
	})
	if err != nil {
		t.Fatalf("runPolicyCLI approvals: %v", err)
	}
	for _, want := range []string{`"status":"pending"`, `redacted`} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q: %s", want, out)
		}
	}
	for _, forbidden := range []string{
		requestID,
		"dec-cli-private",
		"wl-cli-private",
		"run-cli-private",
		"private-project",
		"openai-chat-completions",
		"desk-lead",
		"research-head",
		"private approval reason",
		"do-not-persist",
	} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("approval privacy output leaked %q: %s", forbidden, out)
		}
	}

	if _, err := captureStdout(t, func() error {
		return runPolicyCLI([]string{"resolve", "--id", requestID, "--status", "approved", "--voter", "alice", "--note", "private approval note"}, config.DefaultConfig(), db)
	}); err != nil {
		t.Fatalf("runPolicyCLI resolve: %v", err)
	}
	allowed, err := db.ApprovalAllowsOperation(storage.ApprovalOperation{
		RequestID: requestID,
		Action:    "model.call",
		Target:    "openai-chat-completions",
		Source:    "gateway",
		Model:     "gpt-5.5",
		Project:   "private-project",
	})
	if err != nil {
		t.Fatalf("ApprovalAllowsOperation: %v", err)
	}
	if !allowed {
		t.Fatal("resolved CLI approval should authorize the matching operation")
	}

	audit, err := db.QueryAuditLog(storage.AuditLogFilter{Action: "policy.approval", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAuditLog: %v", err)
	}
	rawAudit, err := json.Marshal(audit)
	if err != nil {
		t.Fatalf("Marshal audit: %v", err)
	}
	raw := string(rawAudit)
	for _, want := range []string{"policy.approval.approved", requestID, "note_present", "true", "alice"} {
		if !strings.Contains(raw, want) {
			t.Fatalf("audit output missing %q: %s", want, raw)
		}
	}
	if strings.Contains(raw, "private approval note") {
		t.Fatalf("audit output leaked note text: %s", raw)
	}
}

func captureStdout(t *testing.T, fn func() error) (string, error) {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("Pipe: %v", err)
	}
	os.Stdout = w
	runErr := fn()
	if closeErr := w.Close(); closeErr != nil && runErr == nil {
		runErr = closeErr
	}
	os.Stdout = old
	out, readErr := io.ReadAll(r)
	if readErr != nil && runErr == nil {
		runErr = readErr
	}
	return string(out), runErr
}
