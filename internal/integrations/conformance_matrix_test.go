package integrations

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAdapterConformanceMatrixCoversDecoderKinds(t *testing.T) {
	matrix := AdapterConformanceMatrixSpec()
	if matrix.Contract != "agent-ledger.adapter-conformance-matrix" || !matrix.LocalFirst || !matrix.ReadOnlySafe || matrix.WritesLocalState {
		t.Fatalf("unexpected matrix identity: %+v", matrix)
	}
	if matrix.SchemaHash == "" || matrix.AdapterSpecHash != AdapterContractFingerprint() || matrix.ProviderProfilesHash != ProviderProfilesFingerprint() {
		t.Fatalf("matrix hashes drifted: %+v", matrix)
	}
	if matrix.Summary.InputKinds != len(SupportedAdapterConformanceKinds()) || matrix.Summary.Fixtures != 13 || matrix.Summary.StrictFixtures != matrix.Summary.Fixtures {
		t.Fatalf("unexpected matrix summary: %+v", matrix.Summary)
	}
	for _, kind := range SupportedAdapterConformanceKinds() {
		entry := matrixKindByConformanceKind(matrix, kind)
		if entry == nil {
			t.Fatalf("matrix missing conformance kind %q: %+v", kind, matrix.Kinds)
		}
		if entry.Endpoint == "" || entry.CLICommand == "" || entry.MCPTool != "ledger.adapter_conformance" || len(entry.Fixtures) == 0 {
			t.Fatalf("matrix kind incomplete for %s: %+v", kind, entry)
		}
		for _, fixture := range entry.Fixtures {
			if fixture.Path == "" || fixture.Command == "" || !fixture.Strict || len(fixture.ExpectedEventTypes) == 0 {
				t.Fatalf("fixture incomplete for %s: %+v", kind, fixture)
			}
		}
	}
}

func TestAdapterConformanceMatrixPrivacy(t *testing.T) {
	raw, err := json.Marshal(AdapterConformanceMatrixSpec())
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"api_key", "sk-", "c:/users", "\\users\\", "session_fixture_001", "bearer "} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("conformance matrix leaked %q: %s", forbidden, raw)
		}
	}
}

func matrixKindByConformanceKind(matrix AdapterConformanceMatrix, kind string) *AdapterConformanceMatrixKind {
	for i := range matrix.Kinds {
		if matrix.Kinds[i].ConformanceKind == kind {
			return &matrix.Kinds[i]
		}
	}
	return nil
}
