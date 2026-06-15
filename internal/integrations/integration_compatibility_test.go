package integrations

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestIntegrationCompatibilityDefaultMatrix(t *testing.T) {
	report := IntegrationCompatibilityReportFor(IntegrationCompatibilityRequest{})
	if report.Contract != "agent-ledger.integration-compatibility" || !report.ReadOnlySafe || report.WritesLocalState {
		t.Fatalf("unexpected compatibility identity: %+v", report)
	}
	if report.AgentProfilesHash != AgentFrameworkProfilesFingerprint() ||
		report.ProviderProfilesHash != ProviderProfilesFingerprint() ||
		report.RecommendationHash != IntegrationRecommendationContractFingerprint() ||
		report.ConformanceMatrixHash != AdapterConformanceMatrixFingerprint() {
		t.Fatalf("unexpected compatibility hash chain: %+v", report)
	}
	if report.Summary.AgentProfiles == 0 || report.Summary.ProviderProfiles == 0 ||
		report.Summary.Rows == 0 || report.Summary.Rows != len(report.Rows) ||
		report.Summary.StrictCIAvailable == 0 || report.Summary.HighConfidence == 0 {
		t.Fatalf("unexpected compatibility summary: %+v", report.Summary)
	}
	if report.CompatibilityHash == "" || !strings.HasPrefix(report.CompatibilityHash, "sha256:") ||
		report.CompatibilityHash != IntegrationCompatibilityFingerprintFrom(report) {
		t.Fatalf("missing stable compatibility hash: %s", report.CompatibilityHash)
	}
	if !compatibilityHasRow(report, "codex-cli", "openai-official", "provider-stream") {
		t.Fatalf("expected Codex/OpenAI provider-stream row")
	}
}

func TestIntegrationCompatibilityFiltersAndRisk(t *testing.T) {
	values := url.Values{}
	values.Set("agent", "codex-cli")
	values.Set("provider", "openrouter-relay")
	values.Set("surface", "provider-stream")
	values.Set("min_confidence", "0.75")
	report := IntegrationCompatibilityReportFor(IntegrationCompatibilityFromValues(values))
	if report.Summary.Rows != 1 {
		t.Fatalf("expected one filtered row: %+v", report.Summary)
	}
	row := report.Rows[0]
	if row.AgentProfileID != "codex-cli" || row.ProviderProfileID != "openrouter-relay" ||
		row.RecommendedSurface != "provider-stream" || row.RiskLevel != "high" ||
		row.Confidence < 0.75 {
		t.Fatalf("unexpected filtered row: %+v", row)
	}
	if len(row.StrictCI) == 0 || len(row.NextSteps) == 0 {
		t.Fatalf("expected validation guidance: %+v", row)
	}
}

func TestIntegrationCompatibilityPrivacySafe(t *testing.T) {
	raw, err := json.Marshal(IntegrationCompatibilityReportFor(IntegrationCompatibilityRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("compatibility report leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func compatibilityHasRow(report IntegrationCompatibilityReport, agentID, providerID, surface string) bool {
	for _, row := range report.Rows {
		if row.AgentProfileID == agentID && row.ProviderProfileID == providerID && row.RecommendedSurface == surface {
			return true
		}
	}
	return false
}
