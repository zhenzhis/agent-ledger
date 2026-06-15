package integrations

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestIntegrationDriftReportBaseline(t *testing.T) {
	runtime := &storage.RuntimeStatus{Mode: "control-plane", ReadOnly: false, WriteOperations: "enabled"}
	report := IntegrationDriftReportFor(Options{}, runtime, IntegrationDriftRequest{})
	if report.Contract != "agent-ledger.integration-drift" || !report.ReadOnlySafe ||
		report.WritesLocalState || report.Summary.Status != "review" ||
		report.Summary.KnownHashes == 0 || report.Summary.MissingExpected == 0 ||
		report.DriftHash == "" || !strings.HasPrefix(report.DriftHash, "sha256:") {
		t.Fatalf("unexpected drift baseline: %+v", report)
	}
	if report.Current["adapter_spec_hash"] != AdapterContractFingerprint() ||
		report.Current["canonical_schema_hash"] != storage.CanonicalEventSchemaFingerprint() {
		t.Fatalf("unexpected current hashes: %+v", report.Current)
	}
	if !containsString(report.CICommands, "agent-ledger integrations drift --strict") {
		t.Fatalf("drift report missing CI command: %+v", report.CICommands)
	}
	if report.DriftHash != IntegrationDriftFingerprintFrom(report) {
		t.Fatalf("unstable drift hash: %s", report.DriftHash)
	}
	if witness := IntegrationDriftOpenAPIFingerprint(Options{}, runtime); witness == "" || !strings.HasPrefix(witness, "sha256:") {
		t.Fatalf("missing OpenAPI drift witness hash: %q", witness)
	}
}

func TestIntegrationDriftReportMatchesPinnedHashes(t *testing.T) {
	runtime := &storage.RuntimeStatus{Mode: "control-plane", ReadOnly: false, WriteOperations: "enabled"}
	current := IntegrationDriftCurrentHashes(Options{}, runtime)
	report := IntegrationDriftReportFor(Options{}, runtime, IntegrationDriftRequest{
		Strict: true,
		Expected: map[string]string{
			"adapter-spec-hash":     current["adapter_spec_hash"],
			"canonical_schema_hash": current["canonical_schema_hash"],
		},
	})
	if report.Summary.Status != "drift" || report.Summary.Matched != 2 || report.Summary.Drifted != 0 ||
		report.Summary.MissingExpected == 0 {
		t.Fatalf("strict report should match provided hashes but fail on missing expected hashes: %+v", report.Summary)
	}
	if !integrationDriftHasRow(report, "adapter_spec_hash", "match") ||
		!integrationDriftHasRow(report, "canonical_schema_hash", "match") {
		t.Fatalf("expected matching drift rows: %+v", report.Rows)
	}
}

func TestIntegrationDriftReportFlagsDriftAndUnknown(t *testing.T) {
	values := url.Values{}
	values.Set("strict", "true")
	values.Set("adapter-spec-hash", "sha256:not-current")
	values.Add("hash", "future_hash=sha256:future")
	report := IntegrationDriftReportFor(Options{}, nil, IntegrationDriftFromValues(values))
	if report.Summary.Status != "drift" || report.Summary.Drifted != 1 ||
		report.Summary.UnknownExpected != 1 || report.Summary.Warnings == 0 {
		t.Fatalf("expected drift and unknown warning: %+v", report.Summary)
	}
	if !integrationDriftHasRow(report, "adapter_spec_hash", "drift") ||
		!integrationDriftHasRow(report, "future_hash", "unknown-expected") {
		t.Fatalf("missing drift rows: %+v", report.Rows)
	}
}

func TestIntegrationDriftPrivacySafe(t *testing.T) {
	raw, err := json.Marshal(IntegrationDriftReportFor(Options{}, nil, IntegrationDriftRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("drift report leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func integrationDriftHasRow(report IntegrationDriftReport, id, status string) bool {
	for _, row := range report.Rows {
		if row.ID == id && row.Status == status {
			return true
		}
	}
	return false
}
