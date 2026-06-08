package integrations

import (
	"strings"
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
	if fingerprint := CatalogFingerprintFrom(catalog); fingerprint == "" || !strings.HasPrefix(fingerprint, "sha256:") {
		t.Fatalf("catalog fingerprint missing: %q", fingerprint)
	}
	assertCapability(t, catalog, "protocol.canonical_events.http", "implemented", true)
	assertCapability(t, catalog, "protocol.adapter_conformance", "implemented", true)
	assertCapability(t, catalog, "protocol.discovery_manifest", "implemented", true)
	assertCapability(t, catalog, "protocol.runtime_status", "implemented", true)
	assertCapability(t, catalog, "protocol.workload_event_feed", "implemented", true)
	assertCapability(t, catalog, "protocol.opentelemetry_genai", "implemented", true)
	assertCapability(t, catalog, "protocol.otlp_receiver", "experimental", false)
	assertCapability(t, catalog, "protocol.a2a", "implemented", true)
	assertCapability(t, catalog, "gateway.provider_api", "implemented", true)
	assertCapability(t, catalog, "gateway.provider_live_proxy", "experimental", false)
	assertCapability(t, catalog, "finops.provider_reconciliation", "implemented", true)
	assertCapability(t, catalog, "governance.policy_evaluator", "implemented", true)
	assertCapability(t, catalog, "notification.redacted_webhook", "implemented", false)
	assertCapabilityCommand(t, catalog, "protocol.adapter_conformance", "agent-ledger adapter spec")
	assertCapabilityCommand(t, catalog, "protocol.discovery_manifest", "agent-ledger discovery")
	assertCapabilityCommand(t, catalog, "protocol.runtime_status", "agent-ledger runtime")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.discovery")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.runtime_status")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://discovery/manifest")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://runtime/status")

	cfg.Integrations.OTLPReceiver.Enabled = true
	cfg.Gateway.Enabled = true
	cfg.Webhooks.Enabled = true
	enabledCatalog := Registry(OptionsFromConfig(cfg))
	assertCapability(t, enabledCatalog, "protocol.otlp_receiver", "experimental", true)
	assertCapability(t, enabledCatalog, "gateway.provider_live_proxy", "experimental", true)
	assertCapability(t, enabledCatalog, "notification.redacted_webhook", "implemented", true)
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

func TestRegistryAnnotatesReadOnlyRuntimeCapabilities(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	cfg.Policies.Enabled = true
	cfg.Gateway.Enabled = true
	catalog := Registry(OptionsFromConfig(cfg))
	if catalog.Summary.ReadOnlyLimited == 0 {
		t.Fatalf("expected read-only limited capability count: %#v", catalog.Summary)
	}
	assertRuntimeCapability(t, catalog, "protocol.canonical_events.http", false, true, false)
	assertRuntimeCapability(t, catalog, "collector.codex", false, true, false)
	assertRuntimeCapability(t, catalog, "protocol.adapter_conformance", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.discovery_manifest", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.runtime_status", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.mcp_stdio", true, true, true)
	assertRuntimeCapability(t, catalog, "protocol.offline_bundle", true, true, true)
	assertRuntimeCapability(t, catalog, "governance.policy_evaluator", true, true, true)
	assertRuntimeCapability(t, catalog, "governance.pricing", true, true, true)
	assertRuntimeCapability(t, catalog, "gateway.provider_live_proxy", false, true, false)
}

func TestDiscoveryManifestIsPrivacySafe(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Collectors.Claude.Enabled = true
	cfg.Collectors.Claude.Paths = []string{"C:/Users/example/.claude/projects"}
	cfg.RBAC.Enabled = true
	manifest := Discovery(OptionsFromConfig(cfg))
	if manifest.Contract != "agent-ledger.discovery" || manifest.Version != "v1" || !manifest.LocalFirst {
		t.Fatalf("unexpected discovery identity: %#v", manifest)
	}
	if manifest.PromptContentStored || manifest.UsageDataUploaded {
		t.Fatalf("discovery must keep privacy defaults explicit: %#v", manifest)
	}
	if manifest.Auth == "" || manifest.MCPCommand != "agent-ledger mcp" || manifest.CapabilityCatalogURI != "/api/integrations" ||
		manifest.RuntimeStatusURI != "/api/runtime/status" || manifest.CanonicalSchemaURI != "/api/event-schema" ||
		manifest.EventExamplesURI != "/api/event-examples" || manifest.AdapterSpecURI != "/api/integrations/adapter-spec" ||
		manifest.AdapterConformanceURI != "/api/integrations/conformance" {
		t.Fatalf("discovery missing entrypoints: %#v", manifest)
	}
	if manifest.CapabilityCatalogHash == "" || !strings.HasPrefix(manifest.CapabilityCatalogHash, "sha256:") || manifest.CapabilityCatalogHash != CatalogFingerprint(OptionsFromConfig(cfg)) {
		t.Fatalf("discovery missing catalog hash: %#v", manifest)
	}
	if manifest.CanonicalSchemaHash == "" || !strings.HasPrefix(manifest.CanonicalSchemaHash, "sha256:") {
		t.Fatalf("discovery missing schema hash: %#v", manifest)
	}
	if manifest.AdapterSpecHash == "" || !strings.HasPrefix(manifest.AdapterSpecHash, "sha256:") || manifest.AdapterSpecHash != AdapterContractFingerprint() {
		t.Fatalf("discovery missing adapter contract hash: %#v", manifest)
	}
	if !hasDiscoveryProtocol(manifest, "protocol.discovery_manifest") || !hasDiscoveryProtocol(manifest, "protocol.mcp_stdio") || !hasDiscoveryProtocol(manifest, "protocol.runtime_status") || !hasDiscoveryProtocol(manifest, "protocol.workload_event_feed") {
		t.Fatalf("discovery missing agent protocols: %#v", manifest.Protocols)
	}
	for _, protocol := range manifest.Protocols {
		for _, value := range append(append(append([]string{}, protocol.Endpoints...), protocol.Commands...), protocol.DataClasses...) {
			if value == "C:/Users/example/.claude/projects" {
				t.Fatalf("raw path leaked in discovery protocol: %#v", protocol)
			}
		}
	}
}

func TestAdapterContractSpecIsMachineReadableAndPrivacySafe(t *testing.T) {
	spec := AdapterContractSpec()
	if spec.Contract != "agent-ledger.adapter-contract" || spec.Version != "v1" || spec.SchemaHash == "" {
		t.Fatalf("unexpected adapter contract identity: %#v", spec)
	}
	if fingerprint := AdapterContractFingerprint(); fingerprint == "" || !strings.HasPrefix(fingerprint, "sha256:") {
		t.Fatalf("adapter contract fingerprint missing: %q", fingerprint)
	}
	if len(spec.SupportedInputKinds) < 5 || len(spec.CanonicalEventTypes) == 0 {
		t.Fatalf("adapter contract missing supported kinds or event types: %#v", spec)
	}
	if !adapterContractHasKind(spec, "provider-stream") {
		t.Fatalf("adapter contract missing provider-stream kind: %#v", spec.SupportedInputKinds)
	}
	if !stringSliceHas(spec.ForbiddenPayloadKeys, "prompt") || !stringSliceHas(spec.ForbiddenPayloadKeys, "messages") {
		t.Fatalf("adapter contract missing forbidden payload keys: %#v", spec.ForbiddenPayloadKeys)
	}
	if spec.Validation.HTTP == "" || spec.Validation.CLI == "" || spec.Ingest.HTTP == nil || spec.Ingest.CLI == nil {
		t.Fatalf("adapter contract missing validation or ingest entrypoints: %#v", spec)
	}
}

func adapterContractHasKind(spec AdapterContract, kind string) bool {
	for _, item := range spec.SupportedInputKinds {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func TestDiscoveryManifestCarriesReadOnlyRuntimeStatus(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	manifest := Discovery(OptionsFromConfig(cfg))
	if !manifest.ReadOnly || manifest.Auth == "" {
		t.Fatalf("expected read-only discovery status: %#v", manifest)
	}
	for _, protocol := range manifest.Protocols {
		if protocol.ID == "protocol.canonical_events.http" {
			if protocol.Enabled || !protocol.WritesLocalState || protocol.AvailableInReadOnly || protocol.RuntimeStatus == "" {
				t.Fatalf("unexpected canonical event read-only protocol: %#v", protocol)
			}
			return
		}
	}
	t.Fatalf("canonical event protocol missing: %#v", manifest.Protocols)
}

func stringSliceHas(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func assertCapabilityCommand(t *testing.T, catalog Catalog, id, command string) {
	t.Helper()
	for _, cap := range catalog.Capabilities {
		if cap.ID == id {
			if !stringSliceHas(cap.Commands, command) {
				t.Fatalf("%s missing command %q: %#v", id, command, cap.Commands)
			}
			return
		}
	}
	t.Fatalf("capability %s missing", id)
}

func assertCapabilityTool(t *testing.T, catalog Catalog, id, tool string) {
	t.Helper()
	for _, cap := range catalog.Capabilities {
		if cap.ID == id {
			if !stringSliceHas(cap.Tools, tool) {
				t.Fatalf("%s missing tool %q: %#v", id, tool, cap.Tools)
			}
			return
		}
	}
	t.Fatalf("capability %s missing", id)
}

func assertCapabilityResource(t *testing.T, catalog Catalog, id, resource string) {
	t.Helper()
	for _, cap := range catalog.Capabilities {
		if cap.ID == id {
			if !stringSliceHas(cap.Resources, resource) {
				t.Fatalf("%s missing resource %q: %#v", id, resource, cap.Resources)
			}
			return
		}
	}
	t.Fatalf("capability %s missing", id)
}

func hasDiscoveryProtocol(manifest DiscoveryManifest, id string) bool {
	for _, protocol := range manifest.Protocols {
		if protocol.ID == id {
			return true
		}
	}
	return false
}

func assertRuntimeCapability(t *testing.T, catalog Catalog, id string, enabled, writes, availableInReadOnly bool) {
	t.Helper()
	for _, cap := range catalog.Capabilities {
		if cap.ID == id {
			if cap.Enabled != enabled || cap.WritesLocalState != writes || cap.AvailableInReadOnly != availableInReadOnly || cap.RuntimeStatus == "" {
				t.Fatalf("%s runtime=%v/%v/%v/%q want %v/%v/%v/non-empty", id, cap.Enabled, cap.WritesLocalState, cap.AvailableInReadOnly, cap.RuntimeStatus, enabled, writes, availableInReadOnly)
			}
			return
		}
	}
	t.Fatalf("capability %s missing", id)
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
