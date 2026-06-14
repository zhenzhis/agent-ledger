package integrations

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAgentFrameworkProfilesCoverFutureAgentEcosystem(t *testing.T) {
	catalog := AgentFrameworkProfiles()
	if catalog.Contract != "agent-ledger.agent-framework-profile-catalog" || !catalog.LocalFirst ||
		!catalog.ReadOnlySafe || catalog.WritesLocalState || catalog.Summary.Profiles < 10 {
		t.Fatalf("unexpected agent framework profile catalog: %+v", catalog)
	}
	for _, id := range []string{
		"codex-cli",
		"claude-code",
		"opencode",
		"kiro-cli",
		"openclaw",
		"pi-agent",
		"mcp-wrapper",
		"a2a-task-runtime",
		"otel-genai-instrumented-app",
		"ci-router",
		"provider-gateway-wrapper",
		"agent-framework-runtime",
	} {
		if !agentFrameworkProfileExists(catalog, id) {
			t.Fatalf("missing agent framework profile %q: %+v", id, catalog.Profiles)
		}
	}
	if catalog.Summary.LocalCollectors < 6 || catalog.Summary.CanonicalEventIngest < 9 ||
		catalog.Summary.MCPTooling < 4 || catalog.Summary.A2ATelemetry < 2 ||
		catalog.Summary.OTelTelemetry < 3 || catalog.Summary.WorkloadHeartbeats < 4 ||
		catalog.Summary.ProviderEnvelopeIngest < 5 || catalog.Summary.ProviderStreamIngest < 3 ||
		catalog.Summary.MultiAgentRouters < 2 {
		t.Fatalf("agent framework profile coverage too narrow: %+v", catalog.Summary)
	}
	raw, err := json.Marshal(catalog)
	if err != nil {
		t.Fatalf("marshal catalog: %v", err)
	}
	for _, forbidden := range []string{
		"api_key",
		"sk-proj-",
		"sk_live_",
		"bearer ",
		"prompt text",
		"response text",
		"message history body",
		"C:/Users/",
		"\\Users\\",
		"session_id",
		"BEGIN PRIVATE KEY",
		"webhook",
	} {
		if strings.Contains(strings.ToLower(string(raw)), strings.ToLower(forbidden)) {
			t.Fatalf("agent framework profile catalog leaked forbidden marker %q: %s", forbidden, string(raw))
		}
	}
	if AgentFrameworkProfilesFingerprint() == "" || !strings.HasPrefix(AgentFrameworkProfilesFingerprint(), "sha256:") {
		t.Fatalf("missing agent framework profile fingerprint")
	}
}

func agentFrameworkProfileExists(catalog AgentFrameworkProfileCatalog, id string) bool {
	for _, profile := range catalog.Profiles {
		if profile.ID == id {
			return true
		}
	}
	return false
}
