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
		{name: "workload lease list is read-only", args: []string{"workload", "lease", "list"}, want: false},
		{name: "workload leases default list is read-only", args: []string{"workload", "leases"}, want: false},
		{name: "workload leases with flags is read-only", args: []string{"workload", "leases", "--limit", "50"}, want: false},
		{name: "workload lease acquire writes", args: []string{"workload", "lease", "acquire", "--workload-id", "wl_1"}, want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cliCommandRequiresWrite(tc.args); got != tc.want {
				t.Fatalf("cliCommandRequiresWrite(%v)=%v want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestRuntimeCLIOutputsCompatibilityHashes(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"runtime"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI runtime: %v", err)
	}
	var status storage.RuntimeStatus
	if err := json.Unmarshal([]byte(out), &status); err != nil {
		t.Fatalf("decode runtime output: %v\n%s", err, out)
	}
	if status.Contract != "agent-ledger.runtime-status" || !status.ReadOnly || status.Mode != "observer" ||
		status.CapabilityCatalogHash == "" || status.CanonicalSchemaHash == "" || status.AdapterSpecHash == "" {
		t.Fatalf("unexpected runtime output: %+v", status)
	}
}

func TestContractsCLIOutputsBundle(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"contracts"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI contracts: %v", err)
	}
	var bundle map[string]interface{}
	if err := json.Unmarshal([]byte(out), &bundle); err != nil {
		t.Fatalf("decode contracts output: %v\n%s", err, out)
	}
	if bundle["contract"] != "agent-ledger.contract-bundle" || bundle["bundle_hash"] == "" || bundle["read_only"] != true {
		t.Fatalf("unexpected contracts output: %+v", bundle)
	}
	docs, ok := bundle["documents"].([]interface{})
	if !ok || len(docs) < 5 {
		t.Fatalf("contracts output missing documents: %+v", bundle)
	}
}

func TestContractsCLIVerifyOutputsReport(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"contracts", "verify"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI contracts verify: %v", err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode contracts verify output: %v\n%s", err, out)
	}
	if report["contract"] != "agent-ledger.contract-verification" || report["ok"] != true ||
		report["failed"] != float64(0) || report["read_only"] != true || report["bundle_hash"] == "" || report["openapi_hash"] == "" {
		t.Fatalf("unexpected contracts verify output: %+v", report)
	}
	checks, ok := report["checks"].([]interface{})
	if !ok || len(checks) == 0 {
		t.Fatalf("contracts verify output missing checks: %+v", report)
	}
}

func TestOpenAPICLIOutputsControlPlaneSpec(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"openapi"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI openapi: %v", err)
	}
	var spec map[string]interface{}
	if err := json.Unmarshal([]byte(out), &spec); err != nil {
		t.Fatalf("decode openapi output: %v\n%s", err, out)
	}
	if spec["openapi"] != "3.1.0" || spec["x-agent-ledger"] == nil {
		t.Fatalf("unexpected openapi output: %+v", spec)
	}
	paths := spec["paths"].(map[string]interface{})
	if paths["/api/openapi.json"] == nil || paths["/api/contracts/verify"] == nil || paths["/api/config/status"] == nil || paths["/api/readiness"] == nil || paths["/api/admission/check"] == nil || paths["/api/events/validate"] == nil || paths["/api/workloads"] == nil || paths["/api/workloads/lease"] == nil || paths["/api/workloads/lease/renew"] == nil || paths["/api/workloads/lease/release"] == nil || paths["/api/workloads/leases"] == nil || paths["/api/agent-runs"] == nil {
		t.Fatalf("openapi output missing expected paths: %+v", paths)
	}
}

func TestConfigCLIStatusOutputsPrivacySafeReport(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	cfg := config.DefaultConfig()
	cfg.Server.AuthToken = "secret-auth-token"
	cfg.Collectors.Codex.Paths = []string{"C:/Users/zhang/private/.codex/sessions"}
	cfg.Storage.Path = "C:/Users/zhang/private/agent-ledger.db"
	cfg.Webhooks.Enabled = true
	cfg.Webhooks.URL = "https://hooks.example.test/secret-webhook"
	cfg.Teams.MachineName = "private-machine"

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"config", "status"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI config status: %v", err)
	}
	var report config.ConfigStatusReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode config status output: %v\n%s", err, out)
	}
	if report.Contract != "agent-ledger.config-status" || !report.Auth.AnyTokenConfigured || !report.Outbound.WebhookURLConfigured {
		t.Fatalf("unexpected config status output: %+v", report)
	}
	for _, forbidden := range []string{"secret-auth-token", "secret-webhook", "C:/Users/zhang/private", "private-machine"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("config status leaked %q: %s", forbidden, out)
		}
	}

	md, err := captureStdout(t, func() error {
		return runCLI([]string{"config", "status", "--format", "markdown"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI config status markdown: %v", err)
	}
	if !strings.Contains(md, "Agent Ledger Config Status") || strings.Contains(md, "secret-auth-token") || strings.Contains(md, "C:/Users/zhang/private") {
		t.Fatalf("unexpected markdown output: %s", md)
	}
}

func TestReadinessCLIOutputsPrivacySafeReport(t *testing.T) {
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	cfg := config.DefaultConfig()
	cfg.Server.AuthToken = "secret-auth-token"
	cfg.Collectors.Codex.Paths = []string{"C:/Users/zhang/private/.codex/sessions"}
	cfg.Storage.Path = "C:/Users/zhang/private/agent-ledger.db"
	cfg.Webhooks.Enabled = true
	cfg.Webhooks.URL = "https://hooks.example.test/secret-webhook"

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"readiness"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI readiness: %v", err)
	}
	var report map[string]interface{}
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode readiness output: %v\n%s", err, out)
	}
	if report["contract"] != "agent-ledger.readiness" || report["prompt_content_stored"] != false || report["usage_data_uploaded"] != false {
		t.Fatalf("unexpected readiness output: %+v", report)
	}
	for _, forbidden := range []string{"secret-auth-token", "secret-webhook", "C:/Users/zhang/private"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("readiness leaked %q: %s", forbidden, out)
		}
	}

	md, err := captureStdout(t, func() error {
		return runCLI([]string{"readiness", "--format", "markdown"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI readiness markdown: %v", err)
	}
	if !strings.Contains(md, "Agent Ledger Readiness") || strings.Contains(md, "secret-auth-token") || strings.Contains(md, "C:/Users/zhang/private") {
		t.Fatalf("unexpected readiness markdown: %s", md)
	}
}

func TestAdmissionCLIOutputsPrivacySafeDecision(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RBAC.Enabled = true
	cfg.RBAC.ReadOnly = true
	cfg.Server.ViewerToken = "secret-viewer-token"
	out, err := captureStdout(t, func() error {
		return runCLI([]string{"admission", "check", "--surface", "cli", "--command", "agent-ledger event validate --file C:/Users/zhang/private/event.json --token secret", "--role", "viewer"}, cfg, nil)
	})
	if err != nil {
		t.Fatalf("runCLI admission: %v", err)
	}
	var decision map[string]interface{}
	if err := json.Unmarshal([]byte(out), &decision); err != nil {
		t.Fatalf("decode admission output: %v\n%s", err, out)
	}
	if decision["contract"] != "agent-ledger.admission-check" || decision["allowed"] != true || decision["writes_local_state"] != false {
		t.Fatalf("unexpected admission output: %+v", decision)
	}
	for _, forbidden := range []string{"secret-viewer-token", "C:/Users/zhang/private", "event.json", "--token secret"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("admission leaked %q: %s", forbidden, out)
		}
	}
	md, err := captureStdout(t, func() error {
		return runCLI([]string{"admission", "check", "--surface", "http", "--method", "POST", "--path", "/api/events", "--role", "operator", "--format", "markdown"}, cfg, nil)
	})
	if err != nil {
		t.Fatalf("runCLI admission markdown: %v", err)
	}
	if !strings.Contains(md, "Agent Ledger Admission") || !strings.Contains(md, "denied") {
		t.Fatalf("unexpected admission markdown: %s", md)
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
	outCh := make(chan struct {
		out []byte
		err error
	}, 1)
	go func() {
		out, readErr := io.ReadAll(r)
		outCh <- struct {
			out []byte
			err error
		}{out: out, err: readErr}
	}()
	os.Stdout = w
	runErr := fn()
	if closeErr := w.Close(); closeErr != nil && runErr == nil {
		runErr = closeErr
	}
	os.Stdout = old
	result := <-outCh
	if result.err != nil && runErr == nil {
		runErr = result.err
	}
	_ = r.Close()
	return string(result.out), runErr
}
