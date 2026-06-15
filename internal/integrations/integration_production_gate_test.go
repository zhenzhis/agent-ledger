package integrations

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestIntegrationProductionGateRequiresReviewByDefault(t *testing.T) {
	runtime := &storage.RuntimeStatus{Mode: "control-plane", ReadOnly: false, WriteOperations: "enabled"}
	report := IntegrationProductionGateFor(Options{}, runtime, IntegrationProductionGateRequest{})
	if report.Contract != "agent-ledger.integration-production-gate" || !report.ReadOnlySafe ||
		report.WritesLocalState || report.GateHash == "" || !strings.HasPrefix(report.GateHash, "sha256:") {
		t.Fatalf("unexpected production gate identity: %+v", report)
	}
	if report.Decision.Status != "review-required" || report.Decision.AllowProductionEnablement {
		t.Fatalf("default production gate should require review: %+v", report.Decision)
	}
	if report.Summary.TotalChecks == 0 || report.Summary.Review == 0 || report.Summary.RecommendedExit != 2 {
		t.Fatalf("unexpected production gate summary: %+v", report.Summary)
	}
	if report.Hashes.IntegrationUpgradeHash != IntegrationUpgradeGateOpenAPIFingerprint(Options{}, runtime) ||
		report.Hashes.IntegrationEvidenceHash != IntegrationEvidenceKitOpenAPIFingerprint(Options{}, runtime) ||
		report.Hashes.IntegrationSmokeHash != IntegrationSmokeFingerprint(Options{}, runtime) {
		t.Fatalf("unexpected production gate hashes: %+v", report.Hashes)
	}
	for _, command := range []string{
		"agent-ledger integrations production-gate --strict",
		"agent-ledger integrations evidence-kit",
		"agent-ledger integrations smoke",
		"agent-ledger contracts verify",
	} {
		if !containsString(report.CICommands, command) {
			t.Fatalf("production gate missing command %q: %+v", command, report.CICommands)
		}
	}
	if report.GateHash != IntegrationProductionGateFingerprintFrom(report) {
		t.Fatalf("unstable production gate hash: %s", report.GateHash)
	}
}

func TestIntegrationProductionGateStrictBlocksReviewItems(t *testing.T) {
	report := IntegrationProductionGateFor(Options{}, nil, IntegrationProductionGateRequest{Strict: true})
	if report.Decision.Status != "blocked" || report.Decision.RecommendedCIExitCode != 1 ||
		report.Summary.Blocked == 0 || report.Decision.AllowProductionEnablement {
		t.Fatalf("strict production gate should block review items: decision=%+v summary=%+v", report.Decision, report.Summary)
	}
}

func TestIntegrationProductionGatePrivacySafe(t *testing.T) {
	raw, err := json.Marshal(IntegrationProductionGateFor(Options{}, nil, IntegrationProductionGateRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("production gate leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}
