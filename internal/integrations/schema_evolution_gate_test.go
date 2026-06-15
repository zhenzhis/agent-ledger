package integrations

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestSchemaEvolutionGatePassesPinnedSchema(t *testing.T) {
	report := SchemaEvolutionGateFor(SchemaEvolutionGateRequest{
		Strict:               true,
		ExpectedVersion:      storage.CanonicalEventSchemaVersion,
		ExpectedSchemaHash:   storage.CanonicalEventSchemaFingerprint(),
		RequiredEventTypes:   []string{"workload.started", "model.call", "tool.call"},
		RequiredRejectedKeys: []string{"prompt", "messages", "content"},
	})
	if report.Contract != "agent-ledger.schema-evolution-gate" || !report.ReadOnlySafe ||
		report.WritesLocalState || report.GateHash == "" || !strings.HasPrefix(report.GateHash, "sha256:") {
		t.Fatalf("unexpected schema gate identity: %+v", report)
	}
	if report.Decision.Status != "pass" || report.Decision.RecommendedCIExitCode != 0 ||
		!report.Decision.AllowAdapterIngest || report.Summary.MissingEventTypes != 0 ||
		report.Summary.MissingRejectedKeys != 0 || !report.Summary.SchemaHashMatches {
		t.Fatalf("expected passing schema gate: %+v", report)
	}
	if report.GateHash != SchemaEvolutionGateFingerprintFrom(report) {
		t.Fatalf("unstable schema gate hash: %s", report.GateHash)
	}
	if witness := SchemaEvolutionGateOpenAPIFingerprint(); witness == "" || !strings.HasPrefix(witness, "sha256:") {
		t.Fatalf("missing schema gate witness hash: %q", witness)
	}
}

func TestSchemaEvolutionGateBlocksSchemaHashDrift(t *testing.T) {
	report := SchemaEvolutionGateFor(SchemaEvolutionGateRequest{
		Strict:             true,
		ExpectedVersion:    storage.CanonicalEventSchemaVersion,
		ExpectedSchemaHash: "sha256:not-current",
		RequiredEventTypes: []string{"model.call"},
	})
	if report.Decision.Status != "block" || report.Decision.RecommendedCIExitCode != 2 ||
		report.Summary.SchemaHashMatches || !schemaEvolutionHasCheck(report, "schema_hash_pin", "block") {
		t.Fatalf("expected schema hash drift to block: %+v", report)
	}
}

func TestSchemaEvolutionGateReviewsOrBlocksMissingPins(t *testing.T) {
	review := SchemaEvolutionGateFor(SchemaEvolutionGateRequest{})
	if review.Decision.Status != "review" || review.Decision.RecommendedCIExitCode != 1 ||
		!schemaEvolutionHasCheck(review, "schema_hash_pin", "review") ||
		!schemaEvolutionHasCheck(review, "required_event_types", "review") {
		t.Fatalf("expected missing pins to require review: %+v", review)
	}
	block := SchemaEvolutionGateFor(SchemaEvolutionGateRequest{Strict: true})
	if block.Decision.Status != "block" || block.Decision.RecommendedCIExitCode != 2 ||
		!schemaEvolutionHasCheck(block, "schema_hash_pin", "block") {
		t.Fatalf("expected strict missing pins to block: %+v", block)
	}
}

func TestSchemaEvolutionGateParsesValuesAndMissingEventTypes(t *testing.T) {
	values := url.Values{}
	values.Set("strict", "true")
	values.Set("schema-version", storage.CanonicalEventSchemaVersion)
	values.Set("schema-hash", storage.CanonicalEventSchemaFingerprint())
	values.Add("event-type", "model.call, future.event")
	values.Add("required-rejected-key", "prompt")
	report := SchemaEvolutionGateFor(SchemaEvolutionGateFromValues(values))
	if report.Decision.Status != "block" || report.Summary.MissingEventTypes != 1 ||
		len(report.EventRows) != 2 || !schemaEvolutionHasCheck(report, "required_event_types", "block") {
		t.Fatalf("expected strict missing event type to block: %+v", report)
	}
}

func TestSchemaEvolutionGatePrivacySafe(t *testing.T) {
	raw, err := json.Marshal(SchemaEvolutionGateFor(SchemaEvolutionGateRequest{Strict: true}))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("schema gate leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func schemaEvolutionHasCheck(report SchemaEvolutionGateReport, id, status string) bool {
	for _, check := range report.Checks {
		if check.ID == id && check.Status == status {
			return true
		}
	}
	return false
}
