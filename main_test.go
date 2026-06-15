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
	"github.com/zhenzhis/agent-ledger/internal/controlplane"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	"github.com/zhenzhis/agent-ledger/internal/server"
	"github.com/zhenzhis/agent-ledger/internal/storage"
	"github.com/zhenzhis/agent-ledger/internal/ui"
)

func openTestDB(t *testing.T) *storage.DB {
	t.Helper()
	dir, err := os.MkdirTemp("", strings.ReplaceAll(t.Name(), "/", "_")+"-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	db, err := storage.Open(filepath.Join(dir, "agent-ledger.db"))
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
		for i := 0; i < 10; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(time.Duration(i+1) * 25 * time.Millisecond)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("RemoveAll %s: %v", dir, err)
		}
	})
	return db
}

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
		{name: "ui check is read-only", args: []string{"ui", "check"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := cliCommandRequiresWrite(tc.args); got != tc.want {
				t.Fatalf("cliCommandRequiresWrite(%v)=%v want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestCLIReadOnlyGateMatchesAdmission(t *testing.T) {
	cases := [][]string{
		{"today"},
		{"top"},
		{"doctor", "--privacy"},
		{"export", "--privacy"},
		{"pricing"},
		{"pricing", "sync"},
		{"projection", "quality"},
		{"projection", "repair"},
		{"event", "validate", "--file", "event.json"},
		{"event", "ingest", "--file", "event.json"},
		{"bundle", "export", "--privacy"},
		{"bundle", "import", "--file", "bundle.json"},
		{"reconcile", "status"},
		{"reconcile", "import", "--file", "provider.csv"},
		{"provider", "profiles"},
		{"signals"},
		{"integrations", "readiness"},
		{"integrations", "smoke"},
		{"integrations", "drift"},
		{"integration-readiness"},
		{"integration-smoke"},
		{"integration-drift"},
		{"adapter", "matrix"},
		{"provider", "convert", "--file", "provider.json"},
		{"provider", "ingest", "--file", "provider.json"},
		{"otel", "convert", "--file", "spans.json"},
		{"otel", "ingest", "--file", "spans.json"},
		{"a2a", "convert", "--file", "task.json"},
		{"a2a", "ingest", "--file", "task.json"},
		{"policy", "audit"},
		{"policy", "evaluate", "--workload-id", "wl_1"},
		{"policy", "evaluate", "--workload-id", "wl_1", "--record"},
		{"policy", "resolve", "--id", "apr_1", "--status", "approved"},
		{"notify", "webhook", "--dry-run"},
		{"notify", "webhook"},
		{"ui", "check"},
		{"workload", "feed"},
		{"workload", "heartbeat", "--run-id", "run_1"},
		{"workload", "lease", "list"},
		{"workload", "lease", "renew", "--lease-id", "lease_1"},
		{"mcp"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			requiresWrite := cliCommandRequiresWrite(args)
			access := controlplane.CLICommandAccessFor("agent-ledger "+strings.Join(args, " "), admissionInputForCLIArgs(args))
			if !access.Known {
				t.Fatalf("admission does not know supported CLI command %v: %+v", args, access)
			}
			if access.AvailableInReadOnly == requiresWrite {
				t.Fatalf("read-only gate/admission mismatch for %v: cliRequiresWrite=%t admission=%+v", args, requiresWrite, access)
			}
		})
	}
}

func TestRuntimeCLIOutputsCompatibilityHashes(t *testing.T) {
	db := openTestDB(t)
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
	db := openTestDB(t)
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
	db := openTestDB(t)
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
	db := openTestDB(t)
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
	if paths["/api/integrations/conformance-matrix"] == nil {
		t.Fatalf("openapi output missing conformance matrix path: %+v", paths)
	}
	if paths["/api/agent-profiles"] == nil {
		t.Fatalf("openapi output missing agent profile path: %+v", paths)
	}
	if paths["/api/signal-taxonomy"] == nil {
		t.Fatalf("openapi output missing signal taxonomy path: %+v", paths)
	}
	if paths["/api/integrations/signal-coverage"] == nil {
		t.Fatalf("openapi output missing signal coverage path: %+v", paths)
	}
	if paths["/api/integrations/readiness"] == nil {
		t.Fatalf("openapi output missing integration readiness path: %+v", paths)
	}
	if paths["/api/integrations/smoke"] == nil {
		t.Fatalf("openapi output missing integration smoke path: %+v", paths)
	}
}

func TestUICLICheckOutputsContractReport(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"ui", "check"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI ui check: %v", err)
	}
	var report ui.ContractReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode ui check output: %v\n%s", err, out)
	}
	if report.Contract != ui.ContractName || !report.OK || report.ContractHash == "" || report.Checked == 0 || report.Failed != 0 {
		t.Fatalf("unexpected ui contract output: %+v", report)
	}

	md, err := captureStdout(t, func() error {
		return runCLI([]string{"ui", "check", "--format", "markdown"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI ui check markdown: %v", err)
	}
	if !strings.Contains(md, "Agent Ledger UI Contract") || !strings.Contains(md, "charts.accessible") {
		t.Fatalf("unexpected ui contract markdown: %s", md)
	}
}

func TestProviderProfilesCLIOutputsReadOnlyCatalog(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"provider", "profiles"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI provider profiles: %v", err)
	}
	var catalog integrations.ProviderProfileCatalog
	if err := json.Unmarshal([]byte(out), &catalog); err != nil {
		t.Fatalf("decode provider profiles output: %v\n%s", err, out)
	}
	if catalog.Contract != "agent-ledger.provider-profile-catalog" || catalog.Summary.Profiles == 0 {
		t.Fatalf("unexpected provider profile catalog: %+v", catalog)
	}
	if !strings.Contains(out, "ollama-local") || !strings.Contains(out, "openrouter-relay") {
		t.Fatalf("provider profile catalog missing local/relay coverage: %s", out)
	}
}

func TestAgentProfilesCLIOutputsReadOnlyCatalog(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"agent", "profiles"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI agent profiles: %v", err)
	}
	var catalog integrations.AgentFrameworkProfileCatalog
	if err := json.Unmarshal([]byte(out), &catalog); err != nil {
		t.Fatalf("decode agent profiles output: %v\n%s", err, out)
	}
	if catalog.Contract != "agent-ledger.agent-framework-profile-catalog" || catalog.Summary.Profiles < 10 {
		t.Fatalf("unexpected agent profile catalog: %+v", catalog)
	}
	if !strings.Contains(out, "codex-cli") || !strings.Contains(out, "mcp-wrapper") || !strings.Contains(out, "a2a-task-runtime") {
		t.Fatalf("agent profile catalog missing agent ecosystem coverage: %s", out)
	}
}

func TestSignalTaxonomyCLIOutputsReadOnlyCatalog(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"signals"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI signals: %v", err)
	}
	var catalog integrations.SignalTaxonomyCatalog
	if err := json.Unmarshal([]byte(out), &catalog); err != nil {
		t.Fatalf("decode signal taxonomy output: %v\n%s", err, out)
	}
	if catalog.Contract != "agent-ledger.signal-taxonomy" || !catalog.ReadOnlySafe || catalog.WritesLocalState || catalog.Summary.Signals < 11 {
		t.Fatalf("unexpected signal taxonomy catalog: %+v", catalog)
	}
	if !strings.Contains(out, "usage.tokens") || !strings.Contains(out, "workload.identity") || !strings.Contains(out, "pricing.provenance") {
		t.Fatalf("signal taxonomy missing core signal coverage: %s", out)
	}
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "sk_test_", "C:/Users", "session_id", "prompt text", "response text"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(forbidden)) {
			t.Fatalf("signal taxonomy leaked %q: %s", forbidden, out)
		}
	}
}

func TestSignalCoverageCLIOutputsReadOnlyReport(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"signals", "coverage"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI signals coverage: %v", err)
	}
	var report integrations.SignalCoverageReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode signal coverage output: %v\n%s", err, out)
	}
	if report.Contract != "agent-ledger.signal-coverage" || !report.ReadOnlySafe || report.WritesLocalState ||
		report.Summary.UnknownSignalReferences != 0 || report.Summary.SignalsWithoutAdapterCoverage != 0 {
		t.Fatalf("unexpected signal coverage report: %+v", report)
	}
	if !strings.Contains(out, "provider-stream") || !strings.Contains(out, "usage.tokens") || !strings.Contains(out, "agent.run.lifecycle") {
		t.Fatalf("signal coverage missing core adapter/signal mapping: %s", out)
	}
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "sk_test_", "C:/Users", "session_id", "prompt text", "response text"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(forbidden)) {
			t.Fatalf("signal coverage leaked %q: %s", forbidden, out)
		}
	}
}

func TestIntegrationReadinessCLIOutputsReadOnlyReport(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	cfg.Integrations.OTLPReceiver.GRPCEnabled = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"integrations", "readiness"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI integrations readiness: %v", err)
	}
	var report integrations.IntegrationReadinessReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode integration readiness output: %v\n%s", err, out)
	}
	if report.Contract != "agent-ledger.integration-readiness" || !report.ReadOnlySafe || report.WritesLocalState ||
		!report.Runtime.ReadOnly || !report.Runtime.OTLPReceiverGRPCEnabled || report.Summary.Failures == 0 {
		t.Fatalf("unexpected integration readiness report: %+v", report)
	}
	if !strings.Contains(out, "gateway.provider_live_proxy") || !strings.Contains(out, "otlp.grpc_requires_receiver") {
		t.Fatalf("integration readiness missing activation gates: %s", out)
	}
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "sk_test_", "C:/Users", "session_id", "prompt text", "response text"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(forbidden)) {
			t.Fatalf("integration readiness leaked %q: %s", forbidden, out)
		}
	}
}

func TestIntegrationSmokeCLIOutputsReadOnlyReport(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"integrations", "smoke"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI integrations smoke: %v", err)
	}
	var report integrations.IntegrationSmokeReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode integration smoke output: %v\n%s", err, out)
	}
	if report.Contract != "agent-ledger.integration-smoke" || !report.ReadOnlySafe || report.WritesLocalState ||
		!report.Runtime.ReadOnly || report.Summary.Failed != 0 || report.FixtureCoverage.Fixtures < 10 {
		t.Fatalf("unexpected integration smoke report: %+v", report)
	}
	if !strings.Contains(out, "recommendation.codex_provider_stream") || !strings.Contains(out, "agent-ledger contracts verify") {
		t.Fatalf("integration smoke missing rollout checks: %s", out)
	}
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "sk_test_", "C:/Users", "session_id", "prompt text", "response text"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(forbidden)) {
			t.Fatalf("integration smoke leaked %q: %s", forbidden, out)
		}
	}
}

func TestIntegrationDriftCLIOutputsReadOnlyReport(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	current := integrations.IntegrationDriftCurrentHashes(integrations.OptionsFromConfig(cfg), server.RuntimeStatusFromConfig(cfg))

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"integrations", "drift", "--strict", "--adapter-spec-hash", current["adapter_spec_hash"], "--expected", "future_hash=sha256:future"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI integrations drift: %v", err)
	}
	var report integrations.IntegrationDriftReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode integration drift output: %v\n%s", err, out)
	}
	if report.Contract != "agent-ledger.integration-drift" || !report.ReadOnlySafe || report.WritesLocalState ||
		report.Summary.Matched == 0 || report.Summary.UnknownExpected != 1 || report.Summary.Status != "drift" {
		t.Fatalf("unexpected integration drift report: %+v", report)
	}
	if !strings.Contains(out, "agent-ledger integrations drift --strict") || !strings.Contains(out, "future_hash") {
		t.Fatalf("integration drift missing CI guidance: %s", out)
	}
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "sk_test_", "C:/Users", "session_id", "prompt text", "response text"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(forbidden)) {
			t.Fatalf("integration drift leaked %q: %s", forbidden, out)
		}
	}
}

func TestAgentRecommendCLIOutputsReadOnlyAdvisor(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"agent", "recommend", "--profile", "codex-cli", "--provider", "openai-official", "--surface", "provider-stream", "--signals", "model,usage,cache"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI agent recommend: %v", err)
	}
	var report integrations.IntegrationRecommendationReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("decode recommendation output: %v\n%s", err, out)
	}
	if report.Contract != "agent-ledger.integration-recommendation" || report.RecommendedSurface != "provider-stream" || !report.ReadOnlySafe || report.WritesLocalState {
		t.Fatalf("unexpected recommendation report: %+v", report)
	}
	if report.Hashes["recommendation_contract"] != integrations.IntegrationRecommendationContractFingerprint() {
		t.Fatalf("recommendation output missing contract hash: %+v", report.Hashes)
	}
}

func TestAdapterMatrixCLIOutputsReadOnlyCatalog(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true

	out, err := captureStdout(t, func() error {
		return runCLI([]string{"adapter", "matrix"}, cfg, db)
	})
	if err != nil {
		t.Fatalf("runCLI adapter matrix: %v", err)
	}
	var matrix integrations.AdapterConformanceMatrix
	if err := json.Unmarshal([]byte(out), &matrix); err != nil {
		t.Fatalf("decode adapter matrix output: %v\n%s", err, out)
	}
	if matrix.Contract != "agent-ledger.adapter-conformance-matrix" || !matrix.ReadOnlySafe ||
		matrix.WritesLocalState || matrix.Summary.Fixtures < 10 || matrix.AdapterSpecHash != integrations.AdapterContractFingerprint() {
		t.Fatalf("unexpected adapter conformance matrix: %+v", matrix)
	}
	if !strings.Contains(out, "provider-openai-response.json") || !strings.Contains(out, "a2a-delegated-task.json") {
		t.Fatalf("adapter matrix missing expected fixture coverage: %s", out)
	}
	for _, forbidden := range []string{"api_key", "sk-", "C:/Users", "session_id"} {
		if strings.Contains(strings.ToLower(out), strings.ToLower(forbidden)) {
			t.Fatalf("adapter matrix leaked %q: %s", forbidden, out)
		}
	}
}

func TestConfigCLIStatusOutputsPrivacySafeReport(t *testing.T) {
	db := openTestDB(t)
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
	db := openTestDB(t)
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

func admissionInputForCLIArgs(args []string) controlplane.AdmissionInput {
	input := controlplane.AdmissionInput{ReadOnly: true}
	for i, arg := range args {
		switch arg {
		case "--dry-run":
			input.DryRun = true
		case "--record":
			input.Record = true
		case "--workload-id", "--workload_id":
			if i+1 < len(args) && strings.TrimSpace(args[i+1]) != "" {
				input.HasWorkloadID = true
			}
		}
	}
	return input
}

func TestPolicyRoutesCLIOutputsRedactedSummary(t *testing.T) {
	db := openTestDB(t)

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
	db := openTestDB(t)

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
