package integrations

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestPublicMetadataUsesContentSafePrivacyLanguage(t *testing.T) {
	docs := map[string]interface{}{
		"agent_profiles":             AgentFrameworkProfiles(),
		"adapter_contract":           AdapterContractSpec(),
		"adapter_conformance_matrix": AdapterConformanceMatrixSpec(),
		"discovery":                  Discovery(Options{}),
		"a2a_discovery":              A2ADiscovery(),
		"goal_coverage":              GoalCoverageReportFor(Options{}, nil),
		"integration_recommendation": IntegrationRecommendation(IntegrationRecommendationRequest{
			AgentProfileID:    "codex-cli",
			ProviderProfileID: "openai-official",
			Surface:           "provider-stream",
			Signals:           []string{"model", "usage", "cache"},
		}),
		"integration_readiness": IntegrationReadiness(Options{}),
		"integration_smoke":     IntegrationSmokeReportFor(Options{}, nil),
		"integration_compatibility": IntegrationCompatibilityReportFor(IntegrationCompatibilityRequest{
			AgentProfileID:    "codex-cli",
			ProviderProfileID: "openai-official",
			Surface:           "provider-stream",
		}),
		"integration_rollout_plan": IntegrationRolloutPlanFor(IntegrationRolloutRequest{
			AgentProfileID:    "codex-cli",
			ProviderProfileID: "openai-official",
			Surface:           "provider-stream",
		}),
		"openapi":           OpenAPISpecFor(Options{}, nil),
		"provider_profiles": ProviderProfiles(),
		"registry":          Registry(Options{}),
		"signal_coverage":   SignalCoverage(),
		"signal_taxonomy":   SignalTaxonomy(),
	}
	ok, actual := contractPublicPrivacyLanguageStatus(docs)
	if !ok {
		t.Fatalf("contract privacy language status failed: %s", actual)
	}
	for name, doc := range docs {
		raw, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("marshal %s: %v", name, err)
		}
		lower := strings.ToLower(string(raw))
		for _, forbidden := range []string{
			"api keys",
			"prompt text",
			"response text",
			"output text",
			"model output text",
			"transcript text",
			"prompt, output",
		} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("%s uses unsafe privacy phrase %q: %s", name, forbidden, raw)
			}
		}
	}
}

func TestPublicMetadataPrivacyLanguageStatusFailsUnsafePhrase(t *testing.T) {
	ok, actual := contractPublicPrivacyLanguageStatus(map[string]interface{}{
		"unsafe": map[string]string{"privacy": "do not store prompt text"},
	})
	if ok || !strings.Contains(actual, "unsafe:prompt text") {
		t.Fatalf("expected unsafe phrase failure, ok=%v actual=%q", ok, actual)
	}
}
