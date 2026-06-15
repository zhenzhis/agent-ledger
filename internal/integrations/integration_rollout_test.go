package integrations

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestIntegrationRolloutPlanDefault(t *testing.T) {
	report := IntegrationRolloutPlanFor(IntegrationRolloutRequest{})
	if report.Contract != "agent-ledger.integration-rollout-plan" || !report.ReadOnlySafe || report.WritesLocalState {
		t.Fatalf("unexpected rollout identity: %+v", report)
	}
	if report.CompatibilityHash == "" ||
		report.RecommendationHash != IntegrationRecommendationContractFingerprint() ||
		report.ConformanceMatrixHash != AdapterConformanceMatrixFingerprint() ||
		report.CanonicalSchemaHash == "" {
		t.Fatalf("unexpected rollout hash chain: %+v", report)
	}
	if report.Summary.Phases != len(report.Phases) || report.Summary.Steps == 0 ||
		report.Summary.RequiredSteps == 0 || report.Summary.ReleaseGates == 0 ||
		report.Target.Surface == "" {
		t.Fatalf("unexpected rollout summary: summary=%+v target=%+v", report.Summary, report.Target)
	}
	if report.RolloutHash == "" || !strings.HasPrefix(report.RolloutHash, "sha256:") ||
		report.RolloutHash != IntegrationRolloutFingerprintFrom(report) {
		t.Fatalf("missing stable rollout hash: %s", report.RolloutHash)
	}
	if !rolloutHasPhase(report, "contract-discovery") || !rolloutHasPhase(report, "fixture-conformance") ||
		!rolloutHasStep(report, "admission") {
		t.Fatalf("rollout plan missing core phases or admission step: %+v", report.Phases)
	}
}

func TestIntegrationRolloutPlanForCodexOpenAI(t *testing.T) {
	values := url.Values{}
	values.Set("agent", "codex-cli")
	values.Set("provider", "openai-official")
	values.Set("surface", "provider-stream")
	values.Set("min_confidence", "0.8")
	report := IntegrationRolloutPlanFor(IntegrationRolloutFromValues(values))
	if report.Target.AgentProfileID != "codex-cli" || report.Target.ProviderProfileID != "openai-official" ||
		report.Target.Surface != "provider-stream" || report.Summary.StrictFixtures == 0 ||
		report.Summary.Status != "ready" {
		t.Fatalf("unexpected Codex/OpenAI rollout: summary=%+v target=%+v fixtures=%+v", report.Summary, report.Target, report.Fixtures)
	}
	if !rolloutFixturePathContains(report, "provider-openai-chat-stream.sse") {
		t.Fatalf("expected OpenAI stream fixture: %+v", report.Fixtures)
	}
	seen := map[string]bool{}
	for _, phase := range report.Phases {
		for _, step := range phase.Steps {
			if seen[step.ID] {
				t.Fatalf("duplicate rollout step id %q in phases: %+v", step.ID, report.Phases)
			}
			seen[step.ID] = true
		}
	}
	if !rolloutHasStep(report, "projection-quality") || !rolloutHasStep(report, "projection-repair") {
		t.Fatalf("expected explicit projection quality and repair rollout steps: %+v", report.Phases)
	}
	if step := rolloutFindStep(report, "projection-quality"); step.Endpoint != "" || step.Required != true {
		t.Fatalf("projection quality should be read-only CLI guidance, got %+v", step)
	}
	if step := rolloutFindStep(report, "projection-repair"); step.Endpoint != "POST /api/projections/repair" || step.Required != false {
		t.Fatalf("projection repair should be optional admin write guidance, got %+v", step)
	}
}

func TestIntegrationRolloutPlanFlagsRelayReview(t *testing.T) {
	report := IntegrationRolloutPlanFor(IntegrationRolloutRequest{
		AgentProfileID:    "codex-cli",
		ProviderProfileID: "openrouter-relay",
		Surface:           "provider-stream",
		MinConfidence:     "0.7",
	})
	if report.Target.RiskLevel != "high" || report.Summary.Status != "review-required" ||
		!report.Summary.RequiresOutboundReview || !report.Summary.RequiresPricingOverride {
		t.Fatalf("relay rollout should require review and pricing override: summary=%+v target=%+v", report.Summary, report.Target)
	}
}

func TestIntegrationRolloutPrivacySafe(t *testing.T) {
	raw, err := json.Marshal(IntegrationRolloutPlanFor(IntegrationRolloutRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("rollout plan leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func rolloutHasPhase(report IntegrationRolloutPlanReport, id string) bool {
	for _, phase := range report.Phases {
		if phase.ID == id {
			return true
		}
	}
	return false
}

func rolloutHasStep(report IntegrationRolloutPlanReport, id string) bool {
	return rolloutFindStep(report, id).ID != ""
}

func rolloutFindStep(report IntegrationRolloutPlanReport, id string) IntegrationRolloutStep {
	for _, phase := range report.Phases {
		for _, step := range phase.Steps {
			if step.ID == id {
				return step
			}
		}
	}
	return IntegrationRolloutStep{}
}

func rolloutFixturePathContains(report IntegrationRolloutPlanReport, fragment string) bool {
	for _, fixture := range report.Fixtures {
		if strings.Contains(fixture.Path, fragment) {
			return true
		}
	}
	return false
}
