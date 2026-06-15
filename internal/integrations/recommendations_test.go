package integrations

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestIntegrationRecommendationForCodexProviderStream(t *testing.T) {
	report := IntegrationRecommendation(IntegrationRecommendationRequest{
		AgentProfileID:    "codex-cli",
		ProviderProfileID: "openai-official",
		Surface:           "provider-stream",
		Signals:           []string{"model", "usage", "cache"},
	})
	if report.Contract != "agent-ledger.integration-recommendation" || !report.LocalFirst || !report.ReadOnlySafe || report.WritesLocalState {
		t.Fatalf("unexpected recommendation identity: %#v", report)
	}
	if report.RecommendedSurface != "provider-stream" || report.MatchedAgentProfile == nil || report.MatchedAgentProfile.ID != "codex-cli" {
		t.Fatalf("unexpected codex match: %#v", report)
	}
	if report.MatchedProviderProfile == nil || report.MatchedProviderProfile.ID != "openai-official" {
		t.Fatalf("unexpected provider match: %#v", report.MatchedProviderProfile)
	}
	if report.Confidence < 0.80 {
		t.Fatalf("expected high confidence, got %.2f: %#v", report.Confidence, report)
	}
	assertContainsString(t, report.StrictCI, "agent-ledger adapter conformance --kind provider-stream --strict --file <fixture>")
	assertContainsString(t, report.ExpectedEventTypes, "model.call")
	if report.Hashes["recommendation_contract"] != IntegrationRecommendationContractFingerprint() {
		t.Fatalf("missing recommendation hash: %#v", report.Hashes)
	}
}

func TestIntegrationRecommendationForClaudeCollector(t *testing.T) {
	report := IntegrationRecommendation(IntegrationRecommendationRequest{AgentProfileID: "claude-code", Surface: "collector"})
	if report.RecommendedSurface != "local-collector" {
		t.Fatalf("expected local collector surface: %#v", report)
	}
	if report.MatchedAgentProfile == nil || report.MatchedAgentProfile.ID != "claude-code" {
		t.Fatalf("expected claude profile: %#v", report.MatchedAgentProfile)
	}
	assertContainsString(t, report.HTTP, "GET /api/health/ingestion")
	assertContainsString(t, report.Validation, "agent-ledger doctor --format markdown")
}

func TestIntegrationRecommendationForMCPWrapper(t *testing.T) {
	report := IntegrationRecommendation(IntegrationRecommendationRequest{AgentProfileID: "mcp-wrapper", Surface: "mcp"})
	if report.RecommendedSurface != "mcp-stdio" {
		t.Fatalf("expected mcp-stdio surface: %#v", report)
	}
	assertContainsString(t, report.MCPTools, "ledger.integration_recommendation")
	assertContainsString(t, report.MCPResources, "agent-ledger://integrations/recommendation")
}

func TestIntegrationRecommendationUnknownProfileIsExplicit(t *testing.T) {
	report := IntegrationRecommendation(IntegrationRecommendationRequest{
		AgentProfileID:    "unknown-agent",
		ProviderProfileID: "unknown-provider",
		Surface:           "canonical",
	})
	if report.MatchedAgentProfile != nil || report.MatchedProviderProfile != nil {
		t.Fatalf("unknown profiles should not match: %#v", report)
	}
	if report.Confidence >= 0.80 {
		t.Fatalf("unknown recommendation should not look high confidence: %.2f", report.Confidence)
	}
	if len(report.Limitations) == 0 || !strings.Contains(strings.Join(report.Limitations, " "), "not found") {
		t.Fatalf("unknown profiles should produce explicit limitations: %#v", report.Limitations)
	}
}

func TestIntegrationRecommendationFromValuesNormalizesAliases(t *testing.T) {
	values := mustParseValues(t, "agent=codex&provider=openai&surface=sse&signals=model,usage,cache&read_only=1")
	req := IntegrationRecommendationFromValues(values)
	if req.AgentProfileID != "codex" || req.ProviderProfileID != "openai" || req.Surface != "provider-stream" || !req.ReadOnly {
		t.Fatalf("unexpected normalized request: %#v", req)
	}
	if len(req.Signals) != 3 {
		t.Fatalf("signals not normalized: %#v", req.Signals)
	}
}

func TestIntegrationRecommendationIsPrivacySafe(t *testing.T) {
	report := IntegrationRecommendation(IntegrationRecommendationRequest{
		AgentProfileID:    "codex-cli",
		ProviderProfileID: "openai-official",
		Surface:           "provider-stream",
	})
	rawBytes, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	raw := string(rawBytes)
	for _, forbidden := range []string{"C:/Users", "\\Users\\", "sk-proj-", "sk_live_", "Bearer ", "api_key", "API keys", "session_id", "prompt text", "response text", "output text", "transcript text", "prompt, output"} {
		if strings.Contains(raw, forbidden) {
			t.Fatalf("recommendation leaked %q: %s", forbidden, raw)
		}
	}
}

func TestIntegrationRecommendationContractFingerprint(t *testing.T) {
	hash := IntegrationRecommendationContractFingerprint()
	if !strings.HasPrefix(hash, "sha256:") || len(hash) != len("sha256:")+64 {
		t.Fatalf("unexpected recommendation fingerprint: %q", hash)
	}
	if hash != IntegrationRecommendationContractFingerprint() {
		t.Fatal("recommendation fingerprint must be deterministic")
	}
}

func assertContainsString(t *testing.T, values []string, want string) {
	t.Helper()
	for _, value := range values {
		if value == want {
			return
		}
	}
	t.Fatalf("missing %q in %#v", want, values)
}

func mustParseValues(t *testing.T, raw string) url.Values {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	return values
}
