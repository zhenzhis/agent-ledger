package integrations

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestIntegrationUpgradeGatePassesPinnedLockfile(t *testing.T) {
	runtime := &storage.RuntimeStatus{Mode: "control-plane", ReadOnly: false, WriteOperations: "enabled"}
	current := IntegrationDriftCurrentHashes(Options{}, runtime)
	report := IntegrationUpgradeGateFor(Options{}, runtime, IntegrationUpgradeGateRequest{
		Strict:   true,
		Expected: current,
	})
	if report.Contract != "agent-ledger.integration-upgrade-gate" || !report.ReadOnlySafe ||
		report.WritesLocalState || report.GateHash == "" || !strings.HasPrefix(report.GateHash, "sha256:") {
		t.Fatalf("unexpected gate identity: %+v", report)
	}
	if report.Decision.Status != "pass" || report.Decision.RecommendedCIExitCode != 0 ||
		!report.Decision.AllowWriteIngest || report.Decision.RequiresHumanReview ||
		report.DriftSummary.Matched != len(current) || len(report.BlockingRows) != 0 || len(report.ReviewRows) != 0 {
		t.Fatalf("expected passing gate for pinned lockfile: %+v", report)
	}
	if report.GateHash != IntegrationUpgradeGateFingerprintFrom(report) {
		t.Fatalf("unstable gate hash: %s", report.GateHash)
	}
	if witness := IntegrationUpgradeGateOpenAPIFingerprint(Options{}, runtime); witness == "" || !strings.HasPrefix(witness, "sha256:") {
		t.Fatalf("missing upgrade gate witness hash: %q", witness)
	}
}

func TestIntegrationUpgradeGateBlocksStrictMissingPins(t *testing.T) {
	report := IntegrationUpgradeGateFor(Options{}, nil, IntegrationUpgradeGateRequest{Strict: true})
	if report.Decision.Status != "block" || report.Decision.RecommendedCIExitCode != 2 ||
		report.DriftSummary.MissingExpected == 0 || len(report.BlockingRows) == 0 {
		t.Fatalf("expected strict missing pins to block: %+v", report)
	}
	if !integrationUpgradeGateHasCheck(report, "lockfile_present", "block") ||
		!integrationUpgradeGateHasCheck(report, "missing_expected", "block") {
		t.Fatalf("strict gate missing block checks: %+v", report.Checks)
	}
}

func TestIntegrationUpgradeGateBlocksDriftAndUnknownStrictHash(t *testing.T) {
	values := url.Values{}
	values.Set("strict", "true")
	values.Set("adapter-spec-hash", "sha256:not-current")
	values.Add("hash", "future_hash=sha256:future")
	report := IntegrationUpgradeGateFor(Options{}, nil, IntegrationUpgradeGateFromValues(values))
	if report.Decision.Status != "block" || report.DriftSummary.Drifted != 1 ||
		report.DriftSummary.UnknownExpected != 1 || len(report.BlockingRows) < 2 {
		t.Fatalf("expected drift/unknown strict gate block: %+v", report)
	}
	if !integrationUpgradeGateHasCheck(report, "hash_drift", "block") ||
		!integrationUpgradeGateHasCheck(report, "unknown_expected", "block") {
		t.Fatalf("gate missing drift/unknown checks: %+v", report.Checks)
	}
}

func TestIntegrationUpgradeGatePrivacySafe(t *testing.T) {
	raw, err := json.Marshal(IntegrationUpgradeGateFor(Options{}, nil, IntegrationUpgradeGateRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("upgrade gate leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func integrationUpgradeGateHasCheck(report IntegrationUpgradeGateReport, id, status string) bool {
	for _, check := range report.Checks {
		if check.ID == id && check.Status == status {
			return true
		}
	}
	return false
}
