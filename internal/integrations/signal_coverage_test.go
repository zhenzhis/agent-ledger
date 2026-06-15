package integrations

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSignalCoverageMapsAdaptersProfilesAndTaxonomy(t *testing.T) {
	report := SignalCoverage()
	if report.Contract != "agent-ledger.signal-coverage" || !report.LocalFirst || !report.ReadOnlySafe || report.WritesLocalState {
		t.Fatalf("unexpected signal coverage identity: %+v", report)
	}
	if report.TaxonomyHash != SignalTaxonomyFingerprint() ||
		report.AdapterSpecHash != AdapterContractFingerprint() ||
		report.ConformanceMatrixHash != AdapterConformanceMatrixFingerprint() ||
		report.ProviderProfilesHash != ProviderProfilesFingerprint() ||
		report.AgentProfilesHash != AgentFrameworkProfilesFingerprint() {
		t.Fatalf("signal coverage hash chain mismatch: %+v", report)
	}
	if report.Summary.TaxonomySignals < 11 || report.Summary.AdapterKinds != len(SupportedAdapterConformanceKinds()) ||
		report.Summary.ProviderProfiles < 6 || report.Summary.AgentProfiles < 10 ||
		report.Summary.UnknownSignalReferences != 0 || report.Summary.SignalsWithoutAdapterCoverage != 0 ||
		report.Summary.Gaps != 0 {
		t.Fatalf("unexpected signal coverage summary: %+v gaps=%+v", report.Summary, report.Gaps)
	}
	for _, id := range []string{"usage.tokens", "model.identity", "pricing.provenance", "workload.identity", "agent.run.lifecycle", "tool.call.metadata"} {
		signal, ok := signalCoverageByID(report, id)
		if !ok {
			t.Fatalf("signal coverage missing %q", id)
		}
		if len(signal.RequiredByAdapterKinds) == 0 {
			t.Fatalf("signal %q lacks adapter coverage: %+v", id, signal)
		}
	}
	if SignalCoverageFingerprint() == "" || !strings.HasPrefix(SignalCoverageFingerprint(), "sha256:") {
		t.Fatalf("missing signal coverage fingerprint")
	}
}

func TestAdapterRequiredSignalsAreTaxonomyIDs(t *testing.T) {
	known := map[string]bool{}
	for _, signal := range SignalTaxonomy().Signals {
		known[signal.ID] = true
	}
	for _, input := range AdapterContractSpec().SupportedInputKinds {
		for _, signalID := range input.RequiredSignals {
			if !known[signalID] {
				t.Fatalf("adapter input %s uses non-taxonomy signal %q", input.Kind, signalID)
			}
		}
	}
	for _, kind := range AdapterConformanceMatrixSpec().Kinds {
		for _, signalID := range kind.RequiredSignals {
			if !known[signalID] {
				t.Fatalf("conformance kind %s uses non-taxonomy signal %q", kind.ConformanceKind, signalID)
			}
		}
	}
}

func TestSignalCoveragePrivacySafe(t *testing.T) {
	raw, err := json.Marshal(SignalCoverage())
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("signal coverage leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func signalCoverageByID(report SignalCoverageReport, id string) (SignalCoverageSignal, bool) {
	for _, signal := range report.Signals {
		if signal.ID == id {
			return signal, true
		}
	}
	return SignalCoverageSignal{}, false
}
