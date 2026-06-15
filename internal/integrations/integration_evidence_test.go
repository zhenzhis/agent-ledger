package integrations

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestIntegrationEvidenceKitForCodexOpenAI(t *testing.T) {
	values := url.Values{}
	values.Set("agent", "codex-cli")
	values.Set("provider", "openai-official")
	values.Set("surface", "provider-stream")
	values.Set("min_confidence", "0.8")
	runtime := &storage.RuntimeStatus{Mode: "control-plane", ReadOnly: false, WriteOperations: "enabled"}
	report := IntegrationEvidenceKitFor(Options{}, runtime, IntegrationEvidenceKitFromValues(values))
	if report.Contract != "agent-ledger.integration-evidence-kit" || !report.LocalFirst ||
		!report.ReadOnlySafe || report.WritesLocalState || report.KitHash == "" ||
		!strings.HasPrefix(report.KitHash, "sha256:") {
		t.Fatalf("unexpected evidence kit identity: %+v", report)
	}
	if report.Target.AgentProfileID != "codex-cli" || report.Target.ProviderProfileID != "openai-official" ||
		report.Target.Surface != "provider-stream" || report.Summary.Status != "ready" ||
		report.Summary.RequiredItems == 0 || report.Summary.StrictFixtures == 0 ||
		report.Summary.CICommands == 0 || report.Summary.ReviewerChecks < 4 {
		t.Fatalf("unexpected evidence kit summary: summary=%+v target=%+v", report.Summary, report.Target)
	}
	if report.Hashes.RolloutPlanHash != IntegrationRolloutFingerprint(IntegrationRolloutRequest{
		AgentProfileID:    "codex-cli",
		ProviderProfileID: "openai-official",
		Surface:           "provider-stream",
		MinConfidence:     "0.8",
	}) || report.Hashes.AdapterSpecHash != AdapterContractFingerprint() ||
		report.Hashes.SignalCoverageHash != SignalCoverageFingerprint() ||
		report.Hashes.IntegrationDriftHash != IntegrationDriftOpenAPIFingerprint(Options{}, runtime) ||
		report.Hashes.IntegrationLockfileHash != IntegrationLockfileOpenAPIFingerprint(Options{}, runtime) ||
		report.Hashes.IntegrationUpgradeGateHash != IntegrationUpgradeGateOpenAPIFingerprint(Options{}, runtime) ||
		report.Hashes.IntegrationProductionGateHash != IntegrationProductionGateOpenAPIFingerprint(Options{}, runtime) ||
		report.Hashes.SchemaEvolutionGateHash != SchemaEvolutionGateOpenAPIFingerprint() {
		t.Fatalf("unexpected evidence hashes: %+v", report.Hashes)
	}
	for _, command := range []string{
		"agent-ledger contracts verify",
		"agent-ledger integrations smoke",
		"agent-ledger integrations drift --strict",
		"agent-ledger integrations lockfile",
		"agent-ledger integrations upgrade-gate --strict",
		"agent-ledger integrations production-gate --strict",
		"agent-ledger schema-gate",
		"agent-ledger integrations rollout-plan --agent codex-cli --provider openai-official --surface provider-stream --min-confidence 0.8",
		"agent-ledger adapter conformance --kind provider-stream --strict --file examples/adapter-fixtures/provider-openai-chat-stream.sse",
	} {
		if !containsString(report.CICommands, command) {
			t.Fatalf("CI commands missing %q: %+v", command, report.CICommands)
		}
	}
	for _, itemID := range []string{
		"integration-drift",
		"integration-lockfile",
		"integration-upgrade-gate",
		"integration-production-gate",
		"schema-evolution-gate",
		"projection-quality",
		"projection-repair",
	} {
		if !integrationEvidenceHasItem(report, itemID) {
			t.Fatalf("evidence kit missing %q: %+v", itemID, report.EvidenceItems)
		}
	}
	if report.KitHash != IntegrationEvidenceKitFingerprintFrom(report) {
		t.Fatalf("unstable kit hash: %s", report.KitHash)
	}
}

func TestIntegrationEvidenceKitFlagsRelayReview(t *testing.T) {
	report := IntegrationEvidenceKitFor(Options{}, nil, IntegrationEvidenceKitRequest{
		AgentProfileID:    "codex-cli",
		ProviderProfileID: "openrouter-relay",
		Surface:           "provider-stream",
		MinConfidence:     "0.7",
	})
	if report.Summary.Status != "review-required" || !report.Summary.RequiresPricingReview ||
		!report.Summary.RequiresOutboundReview {
		t.Fatalf("relay evidence kit should require pricing and outbound review: %+v", report.Summary)
	}
}

func TestIntegrationEvidenceKitPrivacySafe(t *testing.T) {
	raw, err := json.Marshal(IntegrationEvidenceKitFor(Options{}, nil, IntegrationEvidenceKitRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("evidence kit leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func integrationEvidenceHasItem(report IntegrationEvidenceKitReport, id string) bool {
	for _, item := range report.EvidenceItems {
		if item.ID == id {
			return true
		}
	}
	return false
}
