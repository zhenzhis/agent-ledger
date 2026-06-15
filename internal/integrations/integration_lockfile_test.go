package integrations

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestIntegrationLockfileBaseline(t *testing.T) {
	runtime := &storage.RuntimeStatus{Mode: "control-plane", ReadOnly: false, WriteOperations: "enabled"}
	report := IntegrationLockfileFor(Options{}, runtime)
	if report.Contract != "agent-ledger.integration-lockfile" || report.Format != "agent-ledger.integration-lockfile.v1" ||
		!report.ReadOnlySafe || report.WritesLocalState || report.LockfileHash == "" ||
		!strings.HasPrefix(report.LockfileHash, "sha256:") || len(report.Hashes) != len(IntegrationDriftHashIDs()) {
		t.Fatalf("unexpected lockfile baseline: %+v", report)
	}
	if report.Hashes["adapter_spec_hash"] != AdapterContractFingerprint() ||
		report.Hashes["canonical_schema_hash"] != storage.CanonicalEventSchemaFingerprint() {
		t.Fatalf("unexpected lockfile hashes: %+v", report.Hashes)
	}
	if !containsString(report.RefreshCommands, "agent-ledger integrations lockfile") ||
		report.DriftCommand != "agent-ledger integrations drift --strict" {
		t.Fatalf("lockfile missing refresh/drift commands: %+v", report)
	}
	if report.LockfileHash != IntegrationLockfileFingerprintFrom(report) {
		t.Fatalf("unstable lockfile hash: %s", report.LockfileHash)
	}
	if witness := IntegrationLockfileOpenAPIFingerprint(Options{}, runtime); witness == "" || !strings.HasPrefix(witness, "sha256:") {
		t.Fatalf("missing OpenAPI lockfile witness hash: %q", witness)
	}
}

func TestIntegrationLockfilePrivacySafe(t *testing.T) {
	raw, err := json.Marshal(IntegrationLockfileFor(Options{}, nil))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("lockfile leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}
