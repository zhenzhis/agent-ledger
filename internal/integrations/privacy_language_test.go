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
		"openapi":           OpenAPISpecFor(Options{}, nil),
		"provider_profiles": ProviderProfiles(),
		"registry":          Registry(Options{}),
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
