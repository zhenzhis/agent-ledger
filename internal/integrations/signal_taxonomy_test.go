package integrations

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSignalTaxonomyCoversAgentOpsSignals(t *testing.T) {
	catalog := SignalTaxonomy()
	if catalog.Contract != "agent-ledger.signal-taxonomy" || !catalog.LocalFirst || !catalog.ReadOnlySafe || catalog.WritesLocalState {
		t.Fatalf("unexpected signal taxonomy identity: %+v", catalog)
	}
	for _, id := range []string{
		"agent.run.lifecycle",
		"artifact.reference",
		"context.reference",
		"evaluation.signal",
		"model.identity",
		"policy.decision",
		"pricing.provenance",
		"project.attribution",
		"tool.call.metadata",
		"usage.tokens",
		"workload.identity",
	} {
		if !signalTaxonomyHasSignal(catalog, id) {
			t.Fatalf("signal taxonomy missing %q: %+v", id, catalog.Signals)
		}
	}
	if catalog.Summary.Signals < 11 || catalog.Summary.EventSignals < 11 ||
		catalog.Summary.UsageSignals < 3 || catalog.Summary.WorkloadSignals < 3 ||
		catalog.Summary.PolicySignals < 1 || catalog.Summary.ExactPreferred < 6 ||
		catalog.Summary.EstimatedAllowed < 4 {
		t.Fatalf("signal taxonomy coverage too narrow: %+v", catalog.Summary)
	}
	for _, signal := range catalog.Signals {
		if signal.ID == "" || signal.Label == "" || signal.Category == "" || len(signal.CanonicalEventTypes) == 0 ||
			len(signal.RecommendedFields) == 0 || signal.Precision == "" || signal.PrivacyClass == "" || !signal.ContentSafe {
			t.Fatalf("signal taxonomy entry incomplete: %+v", signal)
		}
	}
	if SignalTaxonomyFingerprint() == "" || !strings.HasPrefix(SignalTaxonomyFingerprint(), "sha256:") {
		t.Fatalf("missing signal taxonomy fingerprint")
	}
}

func TestSignalTaxonomyPrivacySafe(t *testing.T) {
	raw, err := json.Marshal(SignalTaxonomy())
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("signal taxonomy leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func signalTaxonomyHasSignal(catalog SignalTaxonomyCatalog, id string) bool {
	for _, signal := range catalog.Signals {
		if signal.ID == id {
			return true
		}
	}
	return false
}
