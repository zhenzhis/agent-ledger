package integrations

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/config"
)

func TestIntegrationSmokeDefaultIsPrivacySafeAndNonFailing(t *testing.T) {
	cfg := config.DefaultConfig()
	opts := OptionsFromConfig(cfg)
	report := IntegrationSmokeReportFor(opts, nil)
	if report.Contract != "agent-ledger.integration-smoke" || !report.ReadOnlySafe || report.WritesLocalState {
		t.Fatalf("unexpected integration smoke identity: %+v", report)
	}
	if report.CatalogHash != CatalogFingerprint(opts) ||
		report.SignalCoverageHash != SignalCoverageFingerprint() ||
		report.IntegrationReadinessHash != IntegrationReadinessFingerprint(opts) ||
		report.RecommendationHash != IntegrationRecommendationContractFingerprint() ||
		report.ConformanceMatrixHash != AdapterConformanceMatrixFingerprint() ||
		report.OpenAPIHash != OpenAPISmokeFingerprint(opts, nil) {
		t.Fatalf("unexpected smoke hash chain: %+v", report)
	}
	if report.Summary.TotalChecks != len(report.Checks) || report.Summary.Failed != 0 ||
		report.Summary.Passed == 0 || report.Summary.DisabledByConfig == 0 ||
		report.FixtureCoverage.InputKinds != len(SupportedAdapterConformanceKinds()) ||
		report.FixtureCoverage.Fixtures < 10 {
		t.Fatalf("unexpected smoke summary: summary=%+v fixtures=%+v", report.Summary, report.FixtureCoverage)
	}
	if !integrationSmokeHasCheck(report, "conformance.fixture_declarations", "pass") ||
		!integrationSmokeHasCheck(report, "signal.coverage", "pass") ||
		!integrationSmokeHasCheck(report, "readiness.guarded_review_visible", "pass") ||
		!integrationSmokeHasCheck(report, "recommendation.codex_provider_stream", "pass") {
		t.Fatalf("missing core smoke checks: %+v", report.Checks)
	}
	if IntegrationSmokeFingerprint(opts, nil) == "" || !strings.HasPrefix(IntegrationSmokeFingerprint(opts, nil), "sha256:") {
		t.Fatalf("missing smoke fingerprint")
	}
}

func TestIntegrationSmokeFlagsInconsistentOTLPGRPC(t *testing.T) {
	opts := OptionsFromConfig(config.DefaultConfig())
	opts.OTLPReceiverEnabled = false
	opts.OTLPReceiverGRPCEnabled = true
	report := IntegrationSmokeReportFor(opts, nil)
	if report.Summary.Failed == 0 || report.Summary.Status != "fail" {
		t.Fatalf("inconsistent OTLP gRPC should fail smoke: %+v", report.Summary)
	}
	if !integrationSmokeHasCheck(report, "runtime.otlp_grpc_parent_gate", "fail") ||
		!integrationSmokeHasCheck(report, "readiness.no_blocked_gates", "fail") {
		t.Fatalf("missing OTLP smoke failures: %+v", report.Checks)
	}
}

func TestIntegrationSmokeWarnsWhenOutboundEnabled(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.Enabled = true
	cfg.Webhooks.Enabled = true
	opts := OptionsFromConfig(cfg)
	report := IntegrationSmokeReportFor(opts, nil)
	if report.LocalFirst {
		t.Fatalf("gateway/webhook-enabled smoke should not claim local_first: %+v", report.Runtime)
	}
	if !integrationSmokeHasCheck(report, "runtime.outbound_disabled_by_default", "warning") {
		t.Fatalf("expected outbound review warning: %+v", report.Checks)
	}
}

func TestIntegrationSmokePrivacySafe(t *testing.T) {
	raw, err := json.Marshal(IntegrationSmokeReportFor(OptionsFromConfig(config.DefaultConfig()), nil))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("integration smoke leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func integrationSmokeHasCheck(report IntegrationSmokeReport, id, status string) bool {
	for _, check := range report.Checks {
		if check.ID == id && check.Status == status {
			return true
		}
	}
	return false
}
