package integrations

import (
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/config"
)

func TestRegistryReportsImplementedAndPlannedCapabilities(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Collectors.Claude.Enabled = true
	cfg.Collectors.Claude.Paths = []string{"~/.claude/projects"}
	cfg.Collectors.Codex.Enabled = false
	cfg.Policies.Enabled = true
	catalog := Registry(OptionsFromConfig(cfg))

	if catalog.Contract != "agent-ledger.integration-capability-catalog" || catalog.Version != "v1" {
		t.Fatalf("unexpected catalog identity: %#v", catalog)
	}
	if catalog.Summary.Implemented == 0 || catalog.Summary.Experimental == 0 {
		t.Fatalf("expected implemented and experimental capabilities: %#v", catalog.Summary)
	}
	if catalog.Summary.EnabledCollectors == 0 {
		t.Fatalf("expected enabled collector count: %#v", catalog.Summary)
	}
	assertCapability(t, catalog, "protocol.canonical_events.http", "implemented", true)
	assertCapability(t, catalog, "protocol.opentelemetry_genai", "implemented", true)
	assertCapability(t, catalog, "protocol.otlp_receiver", "experimental", false)
	assertCapability(t, catalog, "protocol.a2a", "implemented", true)
	assertCapability(t, catalog, "gateway.provider_api", "implemented", true)
	assertCapability(t, catalog, "gateway.provider_live_proxy", "experimental", false)
	assertCapability(t, catalog, "finops.provider_reconciliation", "implemented", true)
	assertCapability(t, catalog, "governance.policy_evaluator", "implemented", true)

	cfg.Integrations.OTLPReceiver.Enabled = true
	cfg.Gateway.Enabled = true
	enabledCatalog := Registry(OptionsFromConfig(cfg))
	assertCapability(t, enabledCatalog, "protocol.otlp_receiver", "experimental", true)
	assertCapability(t, enabledCatalog, "gateway.provider_live_proxy", "experimental", true)
}

func TestCollectorCapabilitiesDoNotExposeRawPaths(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Collectors.Claude.Enabled = true
	cfg.Collectors.Claude.Paths = []string{"C:/Users/example/.claude/projects"}
	opts := OptionsFromConfig(cfg)
	if got := opts.Sources[0].PathCount; got != 1 {
		t.Fatalf("path count=%d", got)
	}
	catalog := Registry(opts)
	for _, cap := range catalog.Capabilities {
		if cap.ID == "collector.claude" {
			for _, field := range append(append([]string{}, cap.DataClasses...), cap.Limitations...) {
				if field == "C:/Users/example/.claude/projects" {
					t.Fatalf("raw path leaked in capability: %#v", cap)
				}
			}
			return
		}
	}
	t.Fatal("collector.claude capability missing")
}

func assertCapability(t *testing.T, catalog Catalog, id, status string, enabled bool) {
	t.Helper()
	for _, cap := range catalog.Capabilities {
		if cap.ID == id {
			if cap.Status != status || cap.Enabled != enabled {
				t.Fatalf("%s status/enabled=%s/%v want %s/%v", id, cap.Status, cap.Enabled, status, enabled)
			}
			return
		}
	}
	t.Fatalf("capability %s missing", id)
}
