package integrations

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
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
	assertCapability(t, catalog, "protocol.contract_bundle", "implemented", true)
	assertCapability(t, catalog, "protocol.contract_verification", "implemented", true)
	assertCapability(t, catalog, "protocol.goal_coverage", "implemented", true)
	assertCapability(t, catalog, "protocol.provider_profiles", "implemented", true)
	assertCapability(t, catalog, "protocol.signal_taxonomy", "implemented", true)
	assertCapability(t, catalog, "protocol.signal_coverage", "implemented", true)
	assertCapability(t, catalog, "protocol.integration_readiness", "implemented", true)
	assertCapability(t, catalog, "protocol.integration_smoke", "implemented", true)
	assertCapability(t, catalog, "protocol.integration_evidence_kit", "implemented", true)
	assertCapability(t, catalog, "protocol.integration_drift", "implemented", true)
	assertCapability(t, catalog, "protocol.integration_lockfile", "implemented", true)
	assertCapability(t, catalog, "protocol.integration_upgrade_gate", "implemented", true)
	assertCapability(t, catalog, "protocol.schema_evolution_gate", "implemented", true)
	assertCapability(t, catalog, "protocol.openapi", "implemented", true)
	assertCapability(t, catalog, "protocol.runtime_status", "implemented", true)
	assertCapability(t, catalog, "protocol.config_status", "implemented", true)
	assertCapability(t, catalog, "protocol.readiness", "implemented", true)
	assertCapability(t, catalog, "protocol.admission_check", "implemented", true)
	assertCapability(t, catalog, "protocol.workload_event_feed", "implemented", true)
	assertCapability(t, catalog, "protocol.opentelemetry_genai", "implemented", true)
	assertCapability(t, catalog, "protocol.otlp_receiver", "experimental", false)
	assertCapability(t, catalog, "protocol.a2a", "implemented", true)
	assertCapability(t, catalog, "gateway.provider_api", "implemented", true)
	assertCapability(t, catalog, "gateway.provider_live_proxy", "experimental", false)
	assertCapability(t, catalog, "finops.provider_reconciliation", "implemented", true)
	assertCapability(t, catalog, "governance.policy_evaluator", "implemented", true)
	assertCapability(t, catalog, "notification.redacted_webhook", "implemented", false)
	assertCapability(t, catalog, "notification.desktop_adapter", "implemented", true)
	assertCapabilityCommand(t, catalog, "protocol.adapter_conformance", "agent-ledger adapter spec")
	assertCapabilityCommand(t, catalog, "protocol.adapter_conformance", "agent-ledger adapter matrix")
	assertCapabilityTool(t, catalog, "protocol.adapter_conformance", "ledger.conformance_matrix")
	assertCapabilityResource(t, catalog, "protocol.adapter_conformance", "agent-ledger://integrations/conformance-matrix")
	assertCapabilityCommand(t, catalog, "protocol.discovery_manifest", "agent-ledger discovery")
	assertCapabilityCommand(t, catalog, "protocol.contract_bundle", "agent-ledger contracts")
	assertCapabilityCommand(t, catalog, "protocol.contract_verification", "agent-ledger contracts verify")
	assertCapabilityCommand(t, catalog, "protocol.goal_coverage", "agent-ledger goal coverage")
	assertCapabilityCommand(t, catalog, "protocol.provider_profiles", "agent-ledger provider profiles")
	assertCapabilityTool(t, catalog, "protocol.provider_profiles", "ledger.provider_profiles")
	assertCapabilityResource(t, catalog, "protocol.provider_profiles", "agent-ledger://integrations/provider-profiles")
	assertCapabilityCommand(t, catalog, "protocol.agent_profiles", "agent-ledger agent profiles")
	assertCapabilityTool(t, catalog, "protocol.agent_profiles", "ledger.agent_profiles")
	assertCapabilityResource(t, catalog, "protocol.agent_profiles", "agent-ledger://integrations/agent-profiles")
	assertCapabilityCommand(t, catalog, "protocol.signal_taxonomy", "agent-ledger signals")
	assertCapabilityTool(t, catalog, "protocol.signal_taxonomy", "ledger.signal_taxonomy")
	assertCapabilityResource(t, catalog, "protocol.signal_taxonomy", "agent-ledger://integrations/signal-taxonomy")
	assertCapabilityCommand(t, catalog, "protocol.signal_coverage", "agent-ledger signal-coverage")
	assertCapabilityTool(t, catalog, "protocol.signal_coverage", "ledger.signal_coverage")
	assertCapabilityResource(t, catalog, "protocol.signal_coverage", "agent-ledger://integrations/signal-coverage")
	assertCapabilityCommand(t, catalog, "protocol.integration_readiness", "agent-ledger integrations readiness")
	assertCapabilityTool(t, catalog, "protocol.integration_readiness", "ledger.integration_readiness")
	assertCapabilityResource(t, catalog, "protocol.integration_readiness", "agent-ledger://integrations/readiness")
	assertCapabilityCommand(t, catalog, "protocol.integration_smoke", "agent-ledger integrations smoke")
	assertCapabilityTool(t, catalog, "protocol.integration_smoke", "ledger.integration_smoke")
	assertCapabilityResource(t, catalog, "protocol.integration_smoke", "agent-ledger://integrations/smoke")
	assertCapabilityCommand(t, catalog, "protocol.integration_evidence_kit", "agent-ledger integrations evidence-kit")
	assertCapabilityTool(t, catalog, "protocol.integration_evidence_kit", "ledger.integration_evidence_kit")
	assertCapabilityResource(t, catalog, "protocol.integration_evidence_kit", "agent-ledger://integrations/evidence-kit")
	assertCapabilityCommand(t, catalog, "protocol.integration_drift", "agent-ledger integrations drift")
	assertCapabilityTool(t, catalog, "protocol.integration_drift", "ledger.integration_drift")
	assertCapabilityResource(t, catalog, "protocol.integration_drift", "agent-ledger://integrations/drift")
	assertCapabilityCommand(t, catalog, "protocol.integration_lockfile", "agent-ledger integrations lockfile")
	assertCapabilityTool(t, catalog, "protocol.integration_lockfile", "ledger.integration_lockfile")
	assertCapabilityResource(t, catalog, "protocol.integration_lockfile", "agent-ledger://integrations/lockfile")
	assertCapabilityCommand(t, catalog, "protocol.integration_upgrade_gate", "agent-ledger integrations upgrade-gate")
	assertCapabilityTool(t, catalog, "protocol.integration_upgrade_gate", "ledger.integration_upgrade_gate")
	assertCapabilityResource(t, catalog, "protocol.integration_upgrade_gate", "agent-ledger://integrations/upgrade-gate")
	assertCapabilityCommand(t, catalog, "protocol.schema_evolution_gate", "agent-ledger schema-gate")
	assertCapabilityCommand(t, catalog, "protocol.schema_evolution_gate", "agent-ledger event schema-gate")
	assertCapabilityTool(t, catalog, "protocol.schema_evolution_gate", "ledger.schema_evolution_gate")
	assertCapabilityResource(t, catalog, "protocol.schema_evolution_gate", "agent-ledger://schema/evolution-gate")
	assertCapabilityCommand(t, catalog, "protocol.integration_recommendation", "agent-ledger agent recommend --profile codex-cli --provider openai-official --surface provider-stream --signals model,usage,cache")
	assertCapabilityTool(t, catalog, "protocol.integration_recommendation", "ledger.integration_recommendation")
	assertCapabilityResource(t, catalog, "protocol.integration_recommendation", "agent-ledger://integrations/recommendation")
	assertCapabilityCommand(t, catalog, "protocol.openapi", "agent-ledger openapi")
	assertCapabilityCommand(t, catalog, "protocol.runtime_status", "agent-ledger runtime")
	assertCapabilityCommand(t, catalog, "protocol.config_status", "agent-ledger config status")
	assertCapabilityCommand(t, catalog, "protocol.readiness", "agent-ledger readiness")
	assertCapabilityDataClass(t, catalog, "protocol.readiness", "workload queue claimability counts")
	assertCapabilityDataClass(t, catalog, "protocol.readiness", "workload lease pressure buckets")
	assertCapabilityDataClass(t, catalog, "protocol.readiness", "agent run active/stale counts")
	assertCapabilityCommand(t, catalog, "protocol.admission_check", "agent-ledger admission check")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.contracts")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.contracts_verify")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.goal_coverage")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.discovery")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.openapi")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.runtime_status")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.config_status")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.readiness")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.admission_check")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.provider_profiles")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.agent_profiles")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.signal_taxonomy")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.signal_coverage")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.integration_readiness")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.integration_smoke")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.integration_evidence_kit")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.integration_drift")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.integration_lockfile")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.integration_upgrade_gate")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.schema_evolution_gate")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.conformance_matrix")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.claim_next_workload")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.workload_queue")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.acquire_workload_lease")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.renew_workload_lease")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.release_workload_lease")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.workload_leases")
	assertCapabilityCommand(t, catalog, "notification.desktop_adapter", "agent-ledger notify desktop --severity warning --approval-due-within 24h")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://contracts/bundle")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://contracts/verification")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://goal/coverage")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://discovery/manifest")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://contracts/openapi")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/provider-profiles")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/agent-profiles")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/signal-taxonomy")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/signal-coverage")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/readiness")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/smoke")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/evidence-kit")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/drift")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/lockfile")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/upgrade-gate")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://schema/evolution-gate")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://integrations/conformance-matrix")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://runtime/status")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://config/status")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://readiness")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://admission/check")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://workloads/queue")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://workloads/leases")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://workload/state")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://workload/timeline")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://agent-runs/liveness")

	cfg.Integrations.OTLPReceiver.Enabled = true
	cfg.Gateway.Enabled = true
	cfg.Webhooks.Enabled = true
	enabledCatalog := Registry(OptionsFromConfig(cfg))
	assertCapability(t, enabledCatalog, "protocol.otlp_receiver", "experimental", true)
	assertCapability(t, enabledCatalog, "gateway.provider_live_proxy", "experimental", true)
	assertCapability(t, enabledCatalog, "notification.redacted_webhook", "implemented", true)
}

func TestCatalogEndpointsMatchOpenAPIContract(t *testing.T) {
	catalog := Registry(Options{})
	spec := OpenAPISpecFor(Options{}, nil)
	paths, ok := spec["paths"].(map[string]interface{})
	if !ok || len(paths) == 0 {
		t.Fatalf("OpenAPI paths missing: %#v", spec["paths"])
	}
	for _, capability := range catalog.Capabilities {
		for _, endpoint := range capability.Endpoints {
			methods, path, ok := parseCatalogEndpoint(endpoint)
			if !ok {
				t.Fatalf("invalid endpoint declaration for %s: %q", capability.ID, endpoint)
			}
			rawPathItem, ok := paths[path]
			if !ok {
				t.Fatalf("catalog endpoint path missing from OpenAPI: capability=%s endpoint=%q path=%s", capability.ID, endpoint, path)
			}
			pathItem, ok := rawPathItem.(map[string]interface{})
			if !ok {
				t.Fatalf("OpenAPI path item invalid for %s: %#v", path, rawPathItem)
			}
			for _, method := range methods {
				if _, ok := pathItem[strings.ToLower(method)]; !ok {
					t.Fatalf("catalog endpoint method missing from OpenAPI: capability=%s endpoint=%q method=%s path=%s", capability.ID, endpoint, method, path)
				}
			}
		}
	}
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

func parseCatalogEndpoint(endpoint string) ([]string, string, bool) {
	fields := strings.Fields(endpoint)
	if len(fields) < 2 {
		return nil, "", false
	}
	methods := strings.Split(fields[0], "/")
	path := fields[1]
	if idx := strings.Index(path, "?"); idx >= 0 {
		path = path[:idx]
	}
	if path == "" || !strings.HasPrefix(path, "/") {
		return nil, "", false
	}
	for _, method := range methods {
		switch method {
		case "GET", "POST":
		default:
			return nil, "", false
		}
	}
	return methods, path, true
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
	assertRuntimeCapability(t, catalog, "protocol.contract_bundle", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.contract_verification", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.discovery_manifest", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.openapi", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.signal_taxonomy", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.signal_coverage", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.integration_readiness", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.integration_smoke", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.integration_evidence_kit", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.integration_drift", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.integration_lockfile", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.integration_upgrade_gate", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.runtime_status", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.config_status", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.readiness", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.admission_check", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.mcp_stdio", true, true, true)
	assertRuntimeCapability(t, catalog, "protocol.offline_bundle", true, true, true)
	assertRuntimeCapability(t, catalog, "notification.desktop_adapter", true, false, true)
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
		manifest.ContractBundleURI != "/api/contracts" ||
		manifest.OpenAPIURI != "/api/openapi.json" ||
		manifest.RuntimeStatusURI != "/api/runtime/status" || manifest.CanonicalSchemaURI != "/api/event-schema" ||
		manifest.EventExamplesURI != "/api/event-examples" || manifest.AdapterSpecURI != "/api/integrations/adapter-spec" ||
		manifest.AdapterConformanceURI != "/api/integrations/conformance" ||
		manifest.SignalTaxonomyURI != "/api/signal-taxonomy" ||
		manifest.SignalTaxonomyHash != SignalTaxonomyFingerprint() ||
		manifest.SignalCoverageURI != "/api/integrations/signal-coverage" ||
		manifest.SignalCoverageHash != SignalCoverageFingerprint() ||
		manifest.IntegrationReadinessURI != "/api/integrations/readiness" ||
		manifest.IntegrationReadinessHash != IntegrationReadinessFingerprint(OptionsFromConfig(cfg)) ||
		manifest.IntegrationSmokeURI != "/api/integrations/smoke" ||
		manifest.IntegrationSmokeHash != IntegrationSmokeFingerprint(OptionsFromConfig(cfg), nil) ||
		manifest.EvidenceKitURI != "/api/integrations/evidence-kit" ||
		manifest.EvidenceKitHash != IntegrationEvidenceKitFingerprint(OptionsFromConfig(cfg), nil, IntegrationEvidenceKitRequest{}) ||
		manifest.DriftURI != "/api/integrations/drift" ||
		manifest.DriftHash != IntegrationDriftOpenAPIFingerprint(OptionsFromConfig(cfg), nil) ||
		manifest.LockfileURI != "/api/integrations/lockfile" ||
		manifest.LockfileHash != IntegrationLockfileOpenAPIFingerprint(OptionsFromConfig(cfg), nil) ||
		manifest.UpgradeGateURI != "/api/integrations/upgrade-gate" ||
		manifest.UpgradeGateHash != IntegrationUpgradeGateOpenAPIFingerprint(OptionsFromConfig(cfg), nil) ||
		manifest.RecommendationURI != "/api/integrations/recommendation" ||
		manifest.RecommendationHash != IntegrationRecommendationContractFingerprint() ||
		manifest.ConformanceMatrixURI != "/api/integrations/conformance-matrix" ||
		manifest.ConformanceMatrixHash != AdapterConformanceMatrixFingerprint() {
		t.Fatalf("discovery missing entrypoints: %#v", manifest)
	}
	if manifest.A2A.Endpoint != "/api/a2a/tasks" || manifest.A2A.ConformanceKind != "a2a" ||
		manifest.A2A.FullServer || manifest.A2A.AvailableInReadOnly ||
		manifest.A2A.MessageContentStored || manifest.A2A.PromptContentStored || manifest.A2A.ArtifactPartContentStored ||
		!manifest.A2A.SupportsDelegatedLineage || !manifest.A2A.SupportsEvidenceReferences || !manifest.A2A.SupportsParentPlaceholders ||
		manifest.A2A.AdapterSpecHash != AdapterContractFingerprint() {
		t.Fatalf("discovery missing privacy-safe A2A metadata: %#v", manifest.A2A)
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
	if !hasDiscoveryProtocol(manifest, "protocol.discovery_manifest") || !hasDiscoveryProtocol(manifest, "protocol.contract_bundle") || !hasDiscoveryProtocol(manifest, "protocol.contract_verification") || !hasDiscoveryProtocol(manifest, "protocol.openapi") || !hasDiscoveryProtocol(manifest, "protocol.mcp_stdio") || !hasDiscoveryProtocol(manifest, "protocol.runtime_status") || !hasDiscoveryProtocol(manifest, "protocol.config_status") || !hasDiscoveryProtocol(manifest, "protocol.readiness") || !hasDiscoveryProtocol(manifest, "protocol.admission_check") || !hasDiscoveryProtocol(manifest, "protocol.signal_taxonomy") || !hasDiscoveryProtocol(manifest, "protocol.signal_coverage") || !hasDiscoveryProtocol(manifest, "protocol.integration_readiness") || !hasDiscoveryProtocol(manifest, "protocol.integration_smoke") || !hasDiscoveryProtocol(manifest, "protocol.integration_evidence_kit") || !hasDiscoveryProtocol(manifest, "protocol.integration_drift") || !hasDiscoveryProtocol(manifest, "protocol.integration_lockfile") || !hasDiscoveryProtocol(manifest, "protocol.integration_upgrade_gate") || !hasDiscoveryProtocol(manifest, "protocol.integration_recommendation") || !hasDiscoveryProtocol(manifest, "protocol.workload_event_feed") {
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

func TestContractBundleIndexesCoreContracts(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Collectors.Claude.Enabled = true
	cfg.Collectors.Claude.Paths = []string{"C:/Users/example/.claude/projects"}
	runtime := EnrichRuntimeStatus(&storage.RuntimeStatus{
		Mode:            "control-plane",
		ReadOnly:        false,
		WriteOperations: "enabled",
		BackgroundTasks: "enabled",
		Message:         "test runtime",
	}, OptionsFromConfig(cfg))
	bundle := ContractBundleFor(OptionsFromConfig(cfg), runtime)
	if bundle.Contract != "agent-ledger.contract-bundle" || bundle.Version != "v1" || !bundle.LocalFirst || bundle.BundleHash == "" || !strings.HasPrefix(bundle.BundleHash, "sha256:") {
		t.Fatalf("unexpected contract bundle identity: %#v", bundle)
	}
	for _, id := range []string{"discovery", "contract-bundle", "openapi", "capability-catalog", "provider-profiles", "agent-profiles", "signal-taxonomy", "signal-coverage", "integration-readiness", "integration-smoke", "integration-evidence-kit", "integration-drift", "integration-lockfile", "integration-upgrade-gate", "integration-recommendation", "runtime-status", "admission-check", "canonical-event-schema", "adapter-contract", "adapter-conformance-matrix", "a2a-discovery"} {
		if !contractBundleHasDocument(bundle, id) {
			t.Fatalf("contract bundle missing %s: %#v", id, bundle.Documents)
		}
	}
	for _, doc := range bundle.Documents {
		if doc.Hash == "" || !strings.HasPrefix(doc.Hash, "sha256:") || doc.PrimaryURI == "" || doc.Revalidation == "" || !doc.ReadOnlySafe || doc.WritesLocalState {
			t.Fatalf("contract document missing stable metadata: %#v", doc)
		}
		for _, value := range append(append(append([]string{doc.PrimaryURI, doc.Privacy}, doc.AlternateURIs...), doc.CLICommands...), doc.MCPResources...) {
			if value == "C:/Users/example/.claude/projects" {
				t.Fatalf("raw path leaked in contract bundle document: %#v", doc)
			}
		}
	}
}

func TestOpenAPISpecIndexesStableControlPlane(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	runtime := EnrichRuntimeStatus(&storage.RuntimeStatus{
		Mode:             "observer",
		ReadOnly:         true,
		WriteOperations:  "disabled",
		BackgroundTasks:  "disabled",
		DisabledFeatures: []string{"background collectors"},
		Message:          "test read-only runtime",
	}, OptionsFromConfig(cfg))
	spec := OpenAPISpecFor(OptionsFromConfig(cfg), runtime)
	if spec["openapi"] != "3.1.0" || OpenAPIFingerprint(OptionsFromConfig(cfg), runtime) == "" {
		t.Fatalf("unexpected OpenAPI identity: %#v", spec)
	}
	meta := spec["x-agent-ledger"].(map[string]interface{})
	if meta["contract"] != "agent-ledger.control-plane-openapi" || meta["read_only"] != true ||
		meta["prompt_content_stored"] != false || meta["usage_data_uploaded"] != false ||
		meta["canonical_schema_hash"] == "" || meta["adapter_spec_hash"] == "" ||
		meta["integration_lockfile_hash"] != IntegrationLockfileOpenAPIFingerprint(OptionsFromConfig(cfg), runtime) ||
		meta["integration_upgrade_gate_hash"] != IntegrationUpgradeGateOpenAPIFingerprint(OptionsFromConfig(cfg), runtime) {
		t.Fatalf("unexpected OpenAPI metadata: %#v", meta)
	}
	paths := spec["paths"].(map[string]interface{})
	for _, path := range OpenAPIContractPaths() {
		if paths[path] == nil {
			t.Fatalf("OpenAPI missing path %s: %#v", path, paths)
		}
	}
	if !openAPIHasBearerSecurityScheme(spec) {
		t.Fatalf("OpenAPI missing AgentLedgerBearer security scheme: %#v", spec["components"])
	}
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI path %s has invalid item: %#v", path, rawPathItem)
		}
		for _, method := range []string{"get", "post"} {
			rawOperation, ok := pathItem[method]
			if !ok {
				continue
			}
			operation, ok := rawOperation.(map[string]interface{})
			if !ok {
				t.Fatalf("OpenAPI %s %s has invalid operation: %#v", method, path, rawOperation)
			}
			if !openAPIOperationHasBearerSecurity(operation) || !openAPIMethodHasResponse(paths, path, method, "401") {
				t.Fatalf("OpenAPI %s %s missing auth contract: %#v", method, path, operation)
			}
			if !openAPIOperationHasAdmissionMetadata(operation) {
				t.Fatalf("OpenAPI %s %s missing admission metadata: %#v", method, path, operation)
			}
		}
	}
	for _, path := range []string{"/api/dashboard", "/api/pricing/status", "/api/anomalies", "/api/policy/enforcement", "/api/workloads", "/api/workloads/leases", "/api/event-examples"} {
		if !openAPIGetHasResponse(paths, path, "304") {
			t.Fatalf("OpenAPI path %s should advertise 304 revalidation: %#v", path, paths[path])
		}
	}
	for _, path := range []string{"/api/quota/status", "/api/evidence-bundle", "/api/offline-bundle/export"} {
		if openAPIGetHasResponse(paths, path, "304") {
			t.Fatalf("OpenAPI path %s must not advertise 304 revalidation: %#v", path, paths[path])
		}
	}
	for _, path := range []string{"/api/dashboard", "/api/openapi.json", "/api/export", "/api/workload-events"} {
		if !openAPIMethodHasResponse(paths, path, "get", "405") || openAPIMethodAllowHeader(paths, path, "get") != "GET" {
			t.Fatalf("OpenAPI path %s should advertise GET-only 405 Allow: %#v", path, paths[path])
		}
	}
	for _, path := range []string{"/api/events", "/api/pricing/sync", "/gateway/openai/v1/responses", "/api/workloads/lease"} {
		if !openAPIMethodHasResponse(paths, path, "post", "405") || openAPIMethodAllowHeader(paths, path, "post") != "POST" {
			t.Fatalf("OpenAPI path %s should advertise POST-only 405 Allow: %#v", path, paths[path])
		}
	}
	for _, path := range []string{"/api/workloads", "/api/policy/approvals"} {
		if !openAPIMethodHasResponse(paths, path, "get", "405") || openAPIMethodAllowHeader(paths, path, "get") != "GET, POST" {
			t.Fatalf("OpenAPI path %s should advertise mixed-method 405 Allow on GET: %#v", path, paths[path])
		}
		if !openAPIMethodHasResponse(paths, path, "post", "405") || openAPIMethodAllowHeader(paths, path, "post") != "GET, POST" {
			t.Fatalf("OpenAPI path %s should advertise mixed-method 405 Allow on POST: %#v", path, paths[path])
		}
	}
	rawSpec, _ := json.Marshal(spec)
	for _, needle := range []string{"Idempotency-Key", "WorkloadCreateRequest", "WorkloadCloseRequest", "WorkloadLinkRequest", "WorkloadLeaseAcquireRequest", "WorkloadLeaseRenewRequest", "WorkloadLeaseReleaseRequest", "AgentRunStartRequest", "AgentRunHeartbeatRequest", "DashboardBundle", "SessionPage", "PricingStatus", "BudgetStatusResponse", "QuotaStatus", "DataQualityReport", "DoctorReport", "ModelRegistryRows", "CostIntelligenceRows", "CacheDoctorRows", "InsightEventRows", "EvidenceBundle", "IntegrationDriftReport", "IntegrationLockfileReport", "IntegrationUpgradeGateReport", "PolicyEvaluationRequest", "PolicyApprovalVoteRequest", "OTelGenAIRequest", "OTLPTraceRequest", "A2ATaskRequest", "ProviderUsageRequest", "GatewayRequest", `"409"`} {
		if !strings.Contains(string(rawSpec), needle) {
			t.Fatalf("OpenAPI missing %q: %s", needle, string(rawSpec))
		}
	}
	raw := hashJSONPayload(spec)
	if !strings.HasPrefix(raw, "sha256:") {
		t.Fatalf("OpenAPI hash missing: %q", raw)
	}
}

func TestOpenAPICostIntelligenceSchemaExposesTrustFields(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	rawSchema, ok := schemas["CostIntelligenceRows"].(map[string]interface{})
	if !ok {
		t.Fatalf("CostIntelligenceRows schema missing: %#v", schemas)
	}
	if rawSchema["type"] != "array" {
		t.Fatalf("CostIntelligenceRows should be an array schema: %#v", rawSchema)
	}
	items, ok := rawSchema["items"].(map[string]interface{})
	if !ok {
		t.Fatalf("CostIntelligenceRows missing item schema: %#v", rawSchema)
	}
	properties, ok := items["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("CostIntelligenceRows item missing properties: %#v", items)
	}
	for _, field := range []string{
		"input_tokens",
		"cache_read_tokens",
		"cache_write_tokens",
		"output_tokens",
		"reasoning_tokens",
		"cost_per_call",
		"cost_per_prompt",
		"tokens_per_prompt",
		"pricing_sources",
		"pricing_confidences",
		"fallback_priced_calls",
		"fuzzy_priced_calls",
		"source_reported_calls",
		"unpriced_calls",
		"unknown_pricing_calls",
		"reasons",
		"advice",
	} {
		if properties[field] == nil {
			t.Fatalf("CostIntelligenceRows schema missing field %q: %#v", field, properties)
		}
	}
	for _, field := range []string{"pricing_sources", "pricing_confidences", "reasons", "advice"} {
		schema, ok := properties[field].(map[string]interface{})
		if !ok || schema["type"] != "array" {
			t.Fatalf("CostIntelligenceRows field %q should be an array: %#v", field, properties[field])
		}
	}
}

func TestOpenAPIDashboardSchemasExposeDataSourceAndPagination(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})

	statsSchema, ok := schemas["DashboardStats"].(map[string]interface{})
	if !ok || statsSchema["type"] != "object" {
		t.Fatalf("DashboardStats should be an object schema: %#v", schemas["DashboardStats"])
	}
	statsProps, ok := statsSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("DashboardStats missing properties: %#v", statsSchema)
	}
	for _, field := range []string{"total_cost", "total_tokens", "total_sessions", "total_prompts", "total_calls", "cache_hit_rate"} {
		if statsProps[field] == nil {
			t.Fatalf("DashboardStats schema missing field %q: %#v", field, statsProps)
		}
	}

	bundleSchema, ok := schemas["DashboardBundle"].(map[string]interface{})
	if !ok || bundleSchema["type"] != "object" {
		t.Fatalf("DashboardBundle should be an object schema: %#v", schemas["DashboardBundle"])
	}
	bundleProps, ok := bundleSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("DashboardBundle missing properties: %#v", bundleSchema)
	}
	for _, field := range []string{"generated_at", "from", "to", "granularity", "data_source", "stats", "cost_by_model", "cost_over_time", "tokens_over_time", "consistency"} {
		if bundleProps[field] == nil {
			t.Fatalf("DashboardBundle schema missing field %q: %#v", field, bundleProps)
		}
	}
	if bundleProps["data_source"].(map[string]interface{})["type"] != "string" {
		t.Fatalf("DashboardBundle data_source should be string: %#v", bundleProps["data_source"])
	}
	if bundleProps["stats"].(map[string]interface{})["$ref"] != "#/components/schemas/DashboardStats" {
		t.Fatalf("DashboardBundle stats should reference DashboardStats: %#v", bundleProps["stats"])
	}
	for field, ref := range map[string]string{
		"cost_by_model":    "#/components/schemas/CostByModel",
		"cost_over_time":   "#/components/schemas/TimeSeriesPoint",
		"tokens_over_time": "#/components/schemas/TokenTimeSeriesPoint",
		"consistency":      "#/components/schemas/DashboardConsistencyIssue",
	} {
		arraySchema, ok := bundleProps[field].(map[string]interface{})
		if !ok || arraySchema["type"] != "array" {
			t.Fatalf("DashboardBundle field %q should be array: %#v", field, bundleProps[field])
		}
		items, ok := arraySchema["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("DashboardBundle field %q should reference %s: %#v", field, ref, arraySchema["items"])
		}
	}

	sessionPageSchema, ok := schemas["SessionPage"].(map[string]interface{})
	if !ok || sessionPageSchema["type"] != "object" {
		t.Fatalf("SessionPage should be an object schema: %#v", schemas["SessionPage"])
	}
	sessionProps, ok := sessionPageSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("SessionPage missing properties: %#v", sessionPageSchema)
	}
	for _, field := range []string{"rows", "total", "limit", "offset", "next_cursor"} {
		if sessionProps[field] == nil {
			t.Fatalf("SessionPage schema missing field %q: %#v", field, sessionProps)
		}
	}
	rowArray, ok := sessionProps["rows"].(map[string]interface{})
	if !ok || rowArray["type"] != "array" {
		t.Fatalf("SessionPage rows should be array: %#v", sessionProps["rows"])
	}
	rowItems, ok := rowArray["items"].(map[string]interface{})
	if !ok || rowItems["$ref"] != "#/components/schemas/SessionInfo" {
		t.Fatalf("SessionPage rows should reference SessionInfo: %#v", rowArray["items"])
	}
	sessionInfo, ok := schemas["SessionInfo"].(map[string]interface{})
	if !ok {
		t.Fatalf("SessionInfo schema missing: %#v", schemas)
	}
	sessionInfoProps, ok := sessionInfo["properties"].(map[string]interface{})
	if !ok || sessionInfoProps["session_id"] == nil || sessionInfoProps["tokens"] == nil || sessionInfoProps["total_cost"] == nil {
		t.Fatalf("SessionInfo missing ledger fields: %#v", sessionInfo)
	}
	if sessionInfoProps["tokens"].(map[string]interface{})["type"] != "integer" || sessionInfoProps["total_cost"].(map[string]interface{})["type"] != "number" {
		t.Fatalf("SessionInfo tokens/cost types are wrong: %#v", sessionInfoProps)
	}
}

func TestOpenAPIPricingStatusSchemaExposesSourceFreshness(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	statusSchema, ok := schemas["PricingStatus"].(map[string]interface{})
	if !ok {
		t.Fatalf("PricingStatus schema missing: %#v", schemas)
	}
	if statusSchema["type"] != "object" {
		t.Fatalf("PricingStatus should be an object schema: %#v", statusSchema)
	}
	statusProps, ok := statusSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("PricingStatus missing properties: %#v", statusSchema)
	}
	for _, field := range []string{"sources", "unpriced_models", "confidence_mix", "rules", "mode", "stale_after"} {
		if statusProps[field] == nil {
			t.Fatalf("PricingStatus schema missing field %q: %#v", field, statusProps)
		}
	}
	unpricedModels, ok := statusProps["unpriced_models"].(map[string]interface{})
	if !ok || unpricedModels["type"] != "array" {
		t.Fatalf("PricingStatus unpriced_models should be an array: %#v", statusProps["unpriced_models"])
	}
	unpricedItems, ok := unpricedModels["items"].(map[string]interface{})
	if !ok || unpricedItems["$ref"] != "#/components/schemas/UnpricedModel" {
		t.Fatalf("PricingStatus unpriced_models should reference UnpricedModel: %#v", unpricedModels["items"])
	}
	sourceSchema, ok := schemas["PricingSourceStatus"].(map[string]interface{})
	if !ok {
		t.Fatalf("PricingSourceStatus schema missing: %#v", schemas)
	}
	sourceProps, ok := sourceSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("PricingSourceStatus missing properties: %#v", sourceSchema)
	}
	for _, field := range []string{"name", "kind", "priority", "url", "last_fetch_at", "sha256", "model_count", "status", "freshness_kind", "freshness_note", "stale"} {
		if sourceProps[field] == nil {
			t.Fatalf("PricingSourceStatus schema missing field %q: %#v", field, sourceProps)
		}
	}
	if sourceProps["stale"].(map[string]interface{})["type"] != "boolean" {
		t.Fatalf("PricingSourceStatus stale should be boolean: %#v", sourceProps["stale"])
	}
}

func TestOpenAPIDataQualitySchemaExposesTrustFields(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	reportSchema, ok := schemas["DataQualityReport"].(map[string]interface{})
	if !ok {
		t.Fatalf("DataQualityReport schema missing: %#v", schemas)
	}
	if reportSchema["type"] != "object" {
		t.Fatalf("DataQualityReport should be an object schema: %#v", reportSchema)
	}
	reportProps, ok := reportSchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("DataQualityReport missing properties: %#v", reportSchema)
	}
	for _, field := range []string{"generated_at", "pricing_sources", "source_quality", "unpriced_models", "confidence_mix", "provenance", "projection", "issues"} {
		if reportProps[field] == nil {
			t.Fatalf("DataQualityReport schema missing field %q: %#v", field, reportProps)
		}
	}
	sourceQuality, ok := reportProps["source_quality"].(map[string]interface{})
	if !ok || sourceQuality["type"] != "array" {
		t.Fatalf("DataQualityReport source_quality should be an array: %#v", reportProps["source_quality"])
	}
	sourceItems, ok := sourceQuality["items"].(map[string]interface{})
	if !ok || sourceItems["$ref"] != "#/components/schemas/QualitySource" {
		t.Fatalf("DataQualityReport source_quality should reference QualitySource: %#v", sourceQuality["items"])
	}
	pricingSources, ok := reportProps["pricing_sources"].(map[string]interface{})
	if !ok || pricingSources["type"] != "array" {
		t.Fatalf("DataQualityReport pricing_sources should be an array: %#v", reportProps["pricing_sources"])
	}
	pricingItems, ok := pricingSources["items"].(map[string]interface{})
	if !ok || pricingItems["$ref"] != "#/components/schemas/PricingSourceStatus" {
		t.Fatalf("DataQualityReport pricing_sources should reference PricingSourceStatus: %#v", pricingSources["items"])
	}
	unpricedModels, ok := reportProps["unpriced_models"].(map[string]interface{})
	if !ok || unpricedModels["type"] != "array" {
		t.Fatalf("DataQualityReport unpriced_models should be an array: %#v", reportProps["unpriced_models"])
	}
	unpricedItems, ok := unpricedModels["items"].(map[string]interface{})
	if !ok || unpricedItems["$ref"] != "#/components/schemas/UnpricedModel" {
		t.Fatalf("DataQualityReport unpriced_models should reference UnpricedModel: %#v", unpricedModels["items"])
	}
	issues, ok := reportProps["issues"].(map[string]interface{})
	if !ok || issues["type"] != "array" {
		t.Fatalf("DataQualityReport issues should be an array: %#v", reportProps["issues"])
	}
	issueItems, ok := issues["items"].(map[string]interface{})
	if !ok || issueItems["$ref"] != "#/components/schemas/InsightEvent" {
		t.Fatalf("DataQualityReport issues should reference InsightEvent: %#v", issues["items"])
	}
	qualitySchema, ok := schemas["QualitySource"].(map[string]interface{})
	if !ok {
		t.Fatalf("QualitySource schema missing: %#v", schemas)
	}
	qualityProps, ok := qualitySchema["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("QualitySource missing properties: %#v", qualitySchema)
	}
	for _, field := range []string{"source", "records", "sessions", "unpriced_records", "estimated_aggregate_records", "cache_aware_records", "confidence", "message"} {
		if qualityProps[field] == nil {
			t.Fatalf("QualitySource schema missing field %q: %#v", field, qualityProps)
		}
	}
	if qualityProps["estimated_aggregate_records"].(map[string]interface{})["type"] != "integer" {
		t.Fatalf("QualitySource estimated_aggregate_records should be integer: %#v", qualityProps["estimated_aggregate_records"])
	}
	for _, name := range []string{"UnpricedModel", "ProvenanceQuality", "ProjectionQuality"} {
		if _, ok := schemas[name].(map[string]interface{}); !ok {
			t.Fatalf("%s schema missing: %#v", name, schemas)
		}
	}
}

func TestOpenAPIDiagnosticsSchemasExposeControlPlaneFields(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})

	schema := func(name string) map[string]interface{} {
		t.Helper()
		raw, ok := schemas[name].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing: %#v", name, schemas[name])
		}
		return raw
	}
	props := func(name string) map[string]interface{} {
		t.Helper()
		raw := schema(name)
		properties, ok := raw["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing properties: %#v", name, raw)
		}
		return properties
	}
	expectFields := func(name string, fields ...string) map[string]interface{} {
		t.Helper()
		properties := props(name)
		for _, field := range fields {
			if properties[field] == nil {
				t.Fatalf("%s schema missing field %q: %#v", name, field, properties)
			}
		}
		return properties
	}
	expectArrayRef := func(name, ref string) {
		t.Helper()
		raw := schema(name)
		if raw["type"] != "array" {
			t.Fatalf("%s should be an array schema: %#v", name, raw)
		}
		items, ok := raw["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("%s items should reference %s: %#v", name, ref, raw["items"])
		}
	}
	expectArrayPropertyRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		arraySchema, ok := properties[field].(map[string]interface{})
		if !ok || arraySchema["type"] != "array" {
			t.Fatalf("%s.%s should be an array: %#v", name, field, properties[field])
		}
		items, ok := arraySchema["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("%s.%s items should reference %s: %#v", name, field, ref, arraySchema["items"])
		}
	}
	expectType := func(name, field, kind string) {
		t.Helper()
		properties := props(name)
		fieldSchema, ok := properties[field].(map[string]interface{})
		if !ok || fieldSchema["type"] != kind {
			t.Fatalf("%s.%s should be %s: %#v", name, field, kind, properties[field])
		}
	}

	expectArrayRef("SessionDetail", "#/components/schemas/SessionDetailRow")
	expectFields("SessionDetailRow", "model", "calls", "input_tokens", "output_tokens", "cache_read", "cache_create", "cost_usd")
	expectType("SessionDetailRow", "calls", "integer")
	expectType("SessionDetailRow", "cost_usd", "number")

	expectFields("SessionReplay", "source", "session_id", "start_time", "end_time", "calls", "total_tokens", "total_cost_usd", "peak_tokens_per_call", "truncated", "points")
	expectType("SessionReplay", "truncated", "boolean")
	expectType("SessionReplay", "total_tokens", "integer")
	expectType("SessionReplay", "total_cost_usd", "number")
	expectArrayPropertyRef("SessionReplay", "points", "#/components/schemas/SessionReplayPoint")
	expectFields("SessionReplayPoint", "timestamp", "source", "session_id", "model", "input_tokens", "output_tokens", "cache_read", "cache_create", "reasoning_output_tokens", "tokens", "cost_usd", "cumulative_tokens", "cumulative_cost_usd", "cumulative_calls", "pricing_source", "pricing_model", "pricing_confidence")

	expectArrayRef("IngestionHealthRows", "#/components/schemas/IngestionHealth")
	expectFields("IngestionHealth", "source", "enabled", "paths", "path_status", "last_scan_at", "duration_ms", "watermark", "files_seen", "records_inserted", "prompts_inserted", "skipped_rows", "last_error")
	expectArrayPropertyRef("IngestionHealth", "path_status", "#/components/schemas/IngestionPathStatus")
	expectFields("IngestionPathStatus", "path", "exists", "readable", "error")
	expectType("IngestionPathStatus", "exists", "boolean")

	expectFields("BudgetStatusResponse", "enabled", "rules")
	expectType("BudgetStatusResponse", "enabled", "boolean")
	expectArrayPropertyRef("BudgetStatusResponse", "rules", "#/components/schemas/BudgetStatus")
	expectFields("BudgetStatus", "name", "period", "scope", "match", "metric", "value", "limit", "ratio", "severity", "message", "period_key")

	expectFields("QuotaStatus", "enabled", "plan", "reset_day", "windows", "method")
	expectArrayPropertyRef("QuotaStatus", "windows", "#/components/schemas/QuotaWindow")
	expectFields("QuotaWindow", "name", "from", "to", "cost_usd", "tokens", "prompts", "cost_limit", "token_limit", "remaining_cost", "remaining_tokens", "burn_rate_per_hour", "projected_cost_usd", "projected_tokens", "reset_at", "time_to_limit_hours")
	expectType("QuotaWindow", "burn_rate_per_hour", "number")

	expectFields("DoctorReport", "generated_at", "from", "to", "stats", "ingestion", "quality", "projection", "pricing_sources", "checks", "summary", "runtime")
	if props("DoctorReport")["stats"].(map[string]interface{})["$ref"] != "#/components/schemas/DashboardStats" {
		t.Fatalf("DoctorReport.stats should reference DashboardStats: %#v", props("DoctorReport")["stats"])
	}
	expectArrayPropertyRef("DoctorReport", "ingestion", "#/components/schemas/IngestionHealth")
	expectArrayPropertyRef("DoctorReport", "pricing_sources", "#/components/schemas/PricingSourceStatus")
	expectArrayPropertyRef("DoctorReport", "checks", "#/components/schemas/DoctorCheck")
	expectFields("DoctorCheck", "name", "source", "status", "severity", "message", "action")
	expectFields("ControlIdempotencyStats", "total_keys", "replayed_keys", "replay_count", "last_seen_at", "operations")
	expectArrayPropertyRef("ControlIdempotencyStats", "operations", "#/components/schemas/ControlIdempotencyOperationStats")
	expectFields("WorkloadLeaseStats", "active", "expired", "released", "total", "next_expiry_at")

	expectArrayRef("CacheDoctorRows", "#/components/schemas/CacheDoctorRow")
	expectFields("CacheDoctorRow", "source", "model", "project", "calls", "input_tokens", "cache_read_tokens", "cache_write_tokens", "output_tokens", "cost_usd", "cache_hit_rate", "estimated_lost_saving", "message")
	expectType("CacheDoctorRow", "cache_hit_rate", "number")

	expectArrayRef("InsightEventRows", "#/components/schemas/InsightEvent")
	expectFields("InsightEvent", "id", "kind", "severity", "source", "model", "project", "session_id", "metric", "value", "baseline", "message", "created_at")
	expectType("InsightEvent", "id", "integer")
	expectType("InsightEvent", "value", "number")
}

func TestOpenAPIEnterpriseReportSchemasExposeLedgerFields(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})

	schema := func(name string) map[string]interface{} {
		t.Helper()
		raw, ok := schemas[name].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing: %#v", name, schemas[name])
		}
		return raw
	}
	props := func(name string) map[string]interface{} {
		t.Helper()
		raw := schema(name)
		properties, ok := raw["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing properties: %#v", name, raw)
		}
		return properties
	}
	expectFields := func(name string, fields ...string) map[string]interface{} {
		t.Helper()
		properties := props(name)
		for _, field := range fields {
			if properties[field] == nil {
				t.Fatalf("%s schema missing field %q: %#v", name, field, properties)
			}
		}
		return properties
	}
	expectArrayRef := func(name, ref string) {
		t.Helper()
		raw := schema(name)
		if raw["type"] != "array" {
			t.Fatalf("%s should be an array schema: %#v", name, raw)
		}
		items, ok := raw["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("%s items should reference %s: %#v", name, ref, raw["items"])
		}
	}
	expectArrayPropertyRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		arraySchema, ok := properties[field].(map[string]interface{})
		if !ok || arraySchema["type"] != "array" {
			t.Fatalf("%s.%s should be an array: %#v", name, field, properties[field])
		}
		items, ok := arraySchema["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("%s.%s items should reference %s: %#v", name, field, ref, arraySchema["items"])
		}
	}
	expectRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		fieldSchema, ok := properties[field].(map[string]interface{})
		if !ok || fieldSchema["$ref"] != ref {
			t.Fatalf("%s.%s should reference %s: %#v", name, field, ref, properties[field])
		}
	}
	expectType := func(name, field, kind string) {
		t.Helper()
		properties := props(name)
		fieldSchema, ok := properties[field].(map[string]interface{})
		if !ok || fieldSchema["type"] != kind {
			t.Fatalf("%s.%s should be %s: %#v", name, field, kind, properties[field])
		}
	}
	expectOneOfFields := func(name string, variants ...[]string) {
		t.Helper()
		raw := schema(name)
		oneOf, ok := raw["oneOf"].([]map[string]interface{})
		if !ok || len(oneOf) != len(variants) {
			t.Fatalf("%s should expose %d oneOf variants: %#v", name, len(variants), raw)
		}
		for idx, fields := range variants {
			properties, ok := oneOf[idx]["properties"].(map[string]interface{})
			if !ok {
				t.Fatalf("%s oneOf[%d] missing properties: %#v", name, idx, oneOf[idx])
			}
			for _, field := range fields {
				if properties[field] == nil {
					t.Fatalf("%s oneOf[%d] missing field %q: %#v", name, idx, field, properties)
				}
			}
		}
	}
	expectOneOfArrayOrRef := func(name string, refs ...string) {
		t.Helper()
		raw := schema(name)
		oneOf, ok := raw["oneOf"].([]map[string]interface{})
		if !ok {
			t.Fatalf("%s should expose oneOf variants: %#v", name, raw)
		}
		for _, ref := range refs {
			found := false
			for _, variant := range oneOf {
				if variant["$ref"] == ref {
					found = true
					break
				}
				if variant["type"] != "array" {
					continue
				}
				items, _ := variant["items"].(map[string]interface{})
				if items["$ref"] == ref {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("%s oneOf missing %s: %#v", name, ref, oneOf)
			}
		}
	}
	expectPathResponseRef := func(path, method, ref string) {
		t.Helper()
		paths := spec["paths"].(map[string]interface{})
		pathItem, ok := paths[path].(map[string]interface{})
		if !ok {
			t.Fatalf("path %s missing: %#v", path, paths[path])
		}
		operation, ok := pathItem[method].(map[string]interface{})
		if !ok {
			t.Fatalf("%s %s operation missing: %#v", method, path, pathItem)
		}
		responses := operation["responses"].(map[string]interface{})
		response := responses["200"].(map[string]interface{})
		content := response["content"].(map[string]interface{})
		jsonContent := content["application/json"].(map[string]interface{})
		responseSchema := jsonContent["schema"].(map[string]interface{})
		if responseSchema["$ref"] != ref {
			t.Fatalf("%s %s response should reference %s: %#v", method, path, ref, responseSchema)
		}
	}
	expectPathRequestRef := func(path, method, contentType, ref string) {
		t.Helper()
		paths := spec["paths"].(map[string]interface{})
		pathItem, ok := paths[path].(map[string]interface{})
		if !ok {
			t.Fatalf("path %s missing: %#v", path, paths[path])
		}
		operation, ok := pathItem[method].(map[string]interface{})
		if !ok {
			t.Fatalf("%s %s operation missing: %#v", method, path, pathItem)
		}
		body := operation["requestBody"].(map[string]interface{})
		content := body["content"].(map[string]interface{})
		bodyContent, ok := content[contentType].(map[string]interface{})
		if !ok {
			t.Fatalf("%s %s missing request content type %s: %#v", method, path, contentType, content)
		}
		requestSchema := bodyContent["schema"].(map[string]interface{})
		if requestSchema["$ref"] != ref {
			t.Fatalf("%s %s request should reference %s: %#v", method, path, ref, requestSchema)
		}
	}

	expectArrayRef("PricingAuditRows", "#/components/schemas/PricingAuditRow")
	expectFields("PricingAuditRow", "model", "pricing_source", "matched_model", "match_type", "priority", "input_cost_per_token", "output_cost_per_token", "cache_read_input_token_cost", "cache_creation_input_token_cost", "effective_at", "updated_at", "confidence")
	expectType("PricingAuditRow", "priority", "integer")
	expectType("PricingAuditRow", "input_cost_per_token", "number")
	expectFields("ProjectionRepairResult", "before", "after", "inserted", "updated", "from", "to", "aggregates_note")
	expectRef("ProjectionRepairResult", "before", "#/components/schemas/ProjectionQuality")

	expectArrayRef("ModelCallRows", "#/components/schemas/ModelCallRow")
	expectFields("ModelCallRow", "source", "model", "project", "calls", "tokens", "cost_usd", "avg_tokens_per_call", "cost_per_call", "unpriced_calls")
	expectArrayRef("ModelRegistryRows", "#/components/schemas/ModelRegistryRow")
	expectFields("ModelRegistryRow", "model", "vendor", "family", "pricing_source", "matched_model", "match_type", "confidence", "input_cost_per_token", "output_cost_per_token", "cache_read_input_token_cost", "cache_creation_input_token_cost", "calls", "tokens", "cost_usd", "updated_at", "stale")
	expectType("ModelRegistryRow", "stale", "boolean")

	expectArrayRef("AuditLogRows", "#/components/schemas/AuditEvent")
	expectFields("AuditEvent", "id", "actor", "role", "action", "target", "params", "created_at")
	expectArrayRef("ReconciliationRows", "#/components/schemas/ReconciliationImport")
	expectFields("ReconciliationImport", "id", "provider", "format", "currency", "local_cost_usd", "provider_cost_usd", "diff_usd", "rows_seen", "payload_sha256", "window_start", "window_end", "status", "notes", "warnings", "imported_at")
	expectOneOfFields("ReconciliationImportRequest",
		[]string{"provider", "format", "provider_cost_usd", "local_cost_usd", "rows_seen", "notes"},
		[]string{"provider", "format", "raw", "notes"},
	)
	expectFields("ReconciliationImportResponse", "ok", "import")
	expectRef("ReconciliationImportResponse", "import", "#/components/schemas/ReconciliationImport")
	expectPathRequestRef("/api/reconciliation/import", "post", "application/json", "#/components/schemas/ReconciliationImportRequest")

	expectFields("RouterSimulationReport", "generated_at", "from", "to", "to_model", "replacement_ratio", "target_pricing", "status", "summary", "rows")
	expectRef("RouterSimulationReport", "target_pricing", "#/components/schemas/PricingAuditRow")
	expectRef("RouterSimulationReport", "summary", "#/components/schemas/RouterSimulationSummary")
	expectArrayPropertyRef("RouterSimulationReport", "rows", "#/components/schemas/RouterSimulationRow")
	expectFields("RouterSimulationRow", "source", "from_model", "to_model", "project", "calls", "tokens", "current_cost_usd", "simulated_cost_usd", "delta_usd", "savings_pct", "replacement_ratio", "target_pricing_source", "target_confidence")
	expectFields("RouterSimulationSummary", "calls", "tokens", "current_cost_usd", "simulated_cost_usd", "delta_usd", "savings_pct", "groups", "unpriced_current_calls")

	expectFields("PreflightEstimateReport", "generated_at", "from", "to", "task", "method", "samples", "confidence", "factor", "baseline", "estimate", "p75")
	expectRef("PreflightEstimateReport", "estimate", "#/components/schemas/PreflightEstimateValues")
	expectFields("PreflightEstimateValues", "cost_usd", "tokens", "calls", "prompts", "duration_minutes")
	expectArrayRef("ChargebackRows", "#/components/schemas/ChargebackRow")
	expectFields("ChargebackRow", "team", "project", "source", "model", "calls", "sessions", "tokens", "cost_usd", "avg_tokens_per_call", "cost_per_call", "unpriced_calls", "mapping_source", "data_source", "confidence")

	expectPathResponseRef("/api/export", "get", "#/components/schemas/ExportJSONResponse")
	expectOneOfArrayOrRef("ExportJSONResponse",
		"#/components/schemas/WorkloadSummary",
		"#/components/schemas/SessionInfo",
		"#/components/schemas/TokenTimeSeriesPoint",
		"#/components/schemas/CostByModel",
		"#/components/schemas/ModelCallRow",
		"#/components/schemas/ChargebackRow",
		"#/components/schemas/AuditEvent",
		"#/components/schemas/DataQualityReport",
	)

	expectPathResponseRef("/api/fleet-attribution", "get", "#/components/schemas/FleetAttributionReport")
	expectFields("FleetAttributionReport", "generated_at", "from", "to", "runs", "sub_agent_runs", "max_concurrent_runs", "model_calls", "tokens", "cost_usd", "rows")
	expectArrayPropertyRef("FleetAttributionReport", "rows", "#/components/schemas/FleetAttributionRow")
	expectFields("FleetAttributionRow", "workload_id", "goal", "source", "project", "repo", "git_branch", "team", "run_id", "parent_run_id", "agent_name", "status", "started_at", "ended_at", "first_call_at", "last_call_at", "duration_ms", "model_calls", "tokens", "cost_usd", "child_runs", "concurrent_runs", "attribution", "confidence", "evidence")
	expectType("FleetAttributionRow", "confidence", "number")

	expectFields("AgentWrappedReport", "generated_at", "period", "from", "to", "stats", "top_model", "top_project", "most_active_day", "best_cache_day", "most_expensive_session", "highlights", "issues")
	expectRef("AgentWrappedReport", "most_expensive_session", "#/components/schemas/CostInsightRow")
	expectArrayPropertyRef("AgentWrappedReport", "highlights", "#/components/schemas/WrappedHighlight")
	expectFields("CostInsightRow", "source", "session_id", "tokens", "cost_usd", "pricing_sources", "pricing_confidences", "reasons", "advice")

	expectFields("EvidenceBundle", "product", "generated_at", "window", "privacy", "runtime", "quality", "ingestion_health", "pricing_sources", "pricing_rules", "pricing_audit", "dashboard", "anomaly_events", "watchdog_events", "cost_intelligence", "workload_states")
	expectRef("EvidenceBundle", "pricing_rules", "#/components/schemas/PricingRuleSummary")
	expectRef("EvidenceBundle", "dashboard", "#/components/schemas/EvidenceDashboard")
	expectArrayPropertyRef("EvidenceBundle", "pricing_audit", "#/components/schemas/PricingAuditRow")
	expectArrayPropertyRef("EvidenceBundle", "cost_intelligence", "#/components/schemas/CostInsightRow")

	expectFields("OfflineBundle", "schema_version", "product", "bundle_id", "generated_at", "window", "filters", "privacy", "data", "integrity")
	expectRef("OfflineBundle", "data", "#/components/schemas/OfflineBundleData")
	expectRef("OfflineBundle", "integrity", "#/components/schemas/OfflineBundleIntegrity")
	expectArrayPropertyRef("OfflineBundleData", "canonical_events", "#/components/schemas/CanonicalEvent")
	expectArrayPropertyRef("OfflineBundleData", "workloads", "#/components/schemas/WorkloadSummary")
	expectArrayPropertyRef("OfflineBundleData", "model_calls", "#/components/schemas/ModelCallRow")
	expectFields("OfflineBundleIntegrity", "hash_algorithm", "payload_sha256", "signature_algorithm", "signature", "key_id")
	expectFields("OfflineBundleImportRequest", "schema_version", "product", "bundle_id", "generated_at", "window", "filters", "privacy", "data", "integrity")
	expectRef("OfflineBundleImportRequest", "data", "#/components/schemas/OfflineBundleData")
	expectRef("OfflineBundleImportRequest", "integrity", "#/components/schemas/OfflineBundleIntegrity")
	expectPathRequestRef("/api/offline-bundle/import", "post", "application/json", "#/components/schemas/OfflineBundleImportRequest")
	expectFields("OfflineBundleImportResult", "bundle_id", "events_seen", "events_inserted", "events_duplicate", "payload_sha256", "signature_verified")
	expectFields("OfflineBundleImportResponse", "ok", "result")
	expectRef("OfflineBundleImportResponse", "result", "#/components/schemas/OfflineBundleImportResult")
}

func TestOpenAPIPolicyGovernanceSchemasExposeControlFields(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})

	schema := func(name string) map[string]interface{} {
		t.Helper()
		raw, ok := schemas[name].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing: %#v", name, schemas[name])
		}
		return raw
	}
	props := func(name string) map[string]interface{} {
		t.Helper()
		raw := schema(name)
		properties, ok := raw["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing properties: %#v", name, raw)
		}
		return properties
	}
	expectFields := func(name string, fields ...string) map[string]interface{} {
		t.Helper()
		properties := props(name)
		for _, field := range fields {
			if properties[field] == nil {
				t.Fatalf("%s schema missing field %q: %#v", name, field, properties)
			}
		}
		return properties
	}
	expectArrayRef := func(name, ref string) {
		t.Helper()
		raw := schema(name)
		if raw["type"] != "array" {
			t.Fatalf("%s should be an array schema: %#v", name, raw)
		}
		items, ok := raw["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("%s items should reference %s: %#v", name, ref, raw["items"])
		}
	}
	expectArrayPropertyRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		arraySchema, ok := properties[field].(map[string]interface{})
		if !ok || arraySchema["type"] != "array" {
			t.Fatalf("%s.%s should be an array: %#v", name, field, properties[field])
		}
		items, ok := arraySchema["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("%s.%s items should reference %s: %#v", name, field, ref, arraySchema["items"])
		}
	}
	expectRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		fieldSchema, ok := properties[field].(map[string]interface{})
		if !ok || fieldSchema["$ref"] != ref {
			t.Fatalf("%s.%s should reference %s: %#v", name, field, ref, properties[field])
		}
	}
	expectType := func(name, field, kind string) {
		t.Helper()
		properties := props(name)
		fieldSchema, ok := properties[field].(map[string]interface{})
		if !ok || fieldSchema["type"] != kind {
			t.Fatalf("%s.%s should be %s: %#v", name, field, kind, properties[field])
		}
	}

	expectFields("PolicyStatus", "enabled", "read_only", "require_privacy_export", "rules", "webhooks_enabled")
	expectArrayPropertyRef("PolicyStatus", "rules", "#/components/schemas/PolicyRuleConfig")
	expectFields("PolicyRuleConfig", "Name", "Scope", "Match", "Action", "Message", "RequiredApprovals", "Approvers", "EscalateAfter", "EscalateTo")
	expectType("PolicyStatus", "require_privacy_export", "boolean")

	expectFields("PolicyEvaluationRequest", "workload_id", "run_id", "source", "model", "project", "repo", "git_branch", "team", "action", "target", "role", "record")
	expectFields("PolicyEvaluationResponse", "enabled", "action", "decisions", "webhooks", "privacy_export")
	expectArrayPropertyRef("PolicyEvaluationResponse", "decisions", "#/components/schemas/PolicyDecision")
	expectFields("PolicyDecision", "decision_id", "rule", "scope", "match", "action", "message", "required_approvals", "approvers", "escalate_after_seconds", "escalate_to")

	expectFields("PolicyAuditReport", "enabled", "checked", "matches", "blocks", "approvals", "warnings", "rows", "scope", "window_from", "window_to")
	expectArrayPropertyRef("PolicyAuditReport", "rows", "#/components/schemas/PolicyAuditRow")
	expectFields("PolicyAuditRow", "kind", "workload_id", "run_id", "session_id", "source", "model", "project", "repo", "git_branch", "team", "action", "target", "role", "tokens", "cost_usd", "timestamp", "evidence", "effective_action", "decisions")
	expectArrayPropertyRef("PolicyAuditRow", "decisions", "#/components/schemas/PolicyDecision")

	expectArrayRef("PolicyDecisionRows", "#/components/schemas/PolicyDecisionRow")
	expectFields("PolicyDecisionRow", "decision_id", "workload_id", "run_id", "rule_id", "action", "reason", "actor_role", "created_at")
	expectFields("PolicyEnforcementReport", "generated_at", "summary", "decisions", "approval_requests", "audit_events")
	expectRef("PolicyEnforcementReport", "summary", "#/components/schemas/PolicyEnforcementSummary")
	expectArrayPropertyRef("PolicyEnforcementReport", "decisions", "#/components/schemas/PolicyDecisionRow")
	expectArrayPropertyRef("PolicyEnforcementReport", "approval_requests", "#/components/schemas/ApprovalRequest")
	expectArrayPropertyRef("PolicyEnforcementReport", "audit_events", "#/components/schemas/AuditEvent")
	expectFields("PolicyEnforcementSummary", "decisions", "blocks", "warnings", "approvals_required", "approval_requests", "pending_approvals", "approved_approvals", "rejected_approvals", "approval_votes", "rejection_votes", "overdue_approvals", "policy_audit_events")

	expectFields("PolicyApprovalRows", "rows", "status")
	expectArrayPropertyRef("PolicyApprovalRows", "rows", "#/components/schemas/ApprovalRequest")
	expectFields("ApprovalRequest", "request_id", "policy_decision_id", "workload_id", "run_id", "source", "model", "project", "action", "target", "actor_role", "status", "required_approvals", "approval_votes", "rejection_votes", "approver_hint", "escalation_target", "escalation_after_seconds", "due_at", "overdue", "reason", "request_payload", "created_at", "updated_at", "decided_at", "decided_by", "decision_note")
	expectFields("PolicyApprovalVoteRequest", "request_id", "status", "note", "voter", "required_approvals")
	expectFields("PolicyApprovalVoteResponse", "ok", "result")
	expectRef("PolicyApprovalVoteResponse", "result", "#/components/schemas/PolicyApprovalVoteResult")
	expectFields("PolicyApprovalVoteResult", "request_id", "status", "required_approvals", "approval_votes", "rejection_votes", "decided")
	expectType("PolicyApprovalVoteResult", "decided", "boolean")

	expectFields("ApprovalRouteSummary", "generated_at", "due_within", "summary", "routes")
	expectRef("ApprovalRouteSummary", "summary", "#/components/schemas/ApprovalRouteSummaryStats")
	expectArrayPropertyRef("ApprovalRouteSummary", "routes", "#/components/schemas/ApprovalRouteRow")
	expectFields("ApprovalRouteSummaryStats", "routes", "pending", "overdue", "due_soon", "unassigned")
	expectFields("ApprovalRouteRow", "route_key", "approver", "escalation_target", "pending", "overdue", "due_soon", "approval_votes", "rejection_votes", "max_required_approvals", "due_next", "sources", "models", "projects", "actions")
}

func TestOpenAPIWorkloadLedgerSchemasExposeRunAndFeedFields(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	paths := spec["paths"].(map[string]interface{})

	schema := func(name string) map[string]interface{} {
		t.Helper()
		raw, ok := schemas[name].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing: %#v", name, schemas[name])
		}
		return raw
	}
	props := func(name string) map[string]interface{} {
		t.Helper()
		raw := schema(name)
		properties, ok := raw["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing properties: %#v", name, raw)
		}
		return properties
	}
	expectFields := func(name string, fields ...string) map[string]interface{} {
		t.Helper()
		properties := props(name)
		for _, field := range fields {
			if properties[field] == nil {
				t.Fatalf("%s schema missing field %q: %#v", name, field, properties)
			}
		}
		return properties
	}
	expectArrayPropertyRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		arraySchema, ok := properties[field].(map[string]interface{})
		if !ok || arraySchema["type"] != "array" {
			t.Fatalf("%s.%s should be an array: %#v", name, field, properties[field])
		}
		items, ok := arraySchema["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("%s.%s items should reference %s: %#v", name, field, ref, arraySchema["items"])
		}
	}
	expectRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		fieldSchema, ok := properties[field].(map[string]interface{})
		if !ok || fieldSchema["$ref"] != ref {
			t.Fatalf("%s.%s should reference %s: %#v", name, field, ref, properties[field])
		}
	}
	expectType := func(name, field, kind string) {
		t.Helper()
		properties := props(name)
		fieldSchema, ok := properties[field].(map[string]interface{})
		if !ok || fieldSchema["type"] != kind {
			t.Fatalf("%s.%s should be %s: %#v", name, field, kind, properties[field])
		}
	}

	workloadsGet := paths["/api/workloads"].(map[string]interface{})["get"].(map[string]interface{})
	workloadsSchema := workloadsGet["responses"].(map[string]interface{})["200"].(map[string]interface{})["content"].(map[string]interface{})["application/json"].(map[string]interface{})["schema"].(map[string]interface{})
	if workloadsSchema["$ref"] != "#/components/schemas/WorkloadPage" {
		t.Fatalf("/api/workloads should return WorkloadPage: %#v", workloadsSchema)
	}
	expectFields("WorkloadPage", "rows", "total", "limit", "offset", "next_cursor")
	expectArrayPropertyRef("WorkloadPage", "rows", "#/components/schemas/WorkloadSummary")

	expectFields("AgentRunHeartbeatResponse", "ok", "heartbeat")
	expectRef("AgentRunHeartbeatResponse", "heartbeat", "#/components/schemas/AgentRunEventRow")
	expectFields("AgentRunEventRow", "event_id", "run_id", "workload_id", "source", "event_type", "status", "phase", "progress", "message", "metrics", "timestamp", "confidence")
	expectType("AgentRunEventRow", "progress", "number")

	expectFields("AgentRunLivenessResponse", "rows", "max_age", "stale_only")
	expectArrayPropertyRef("AgentRunLivenessResponse", "rows", "#/components/schemas/AgentRunLivenessRow")
	expectFields("AgentRunLivenessRow", "run_id", "workload_id", "goal", "source", "agent_name", "status", "project", "repo", "git_branch", "phase", "progress", "started_at", "last_heartbeat_at", "last_activity", "heartbeat_count", "status_message", "age_seconds", "stale")
	expectType("AgentRunLivenessRow", "stale", "boolean")

	expectFields("WorkloadDetail", "summary", "runs", "run_events", "model_calls", "tool_calls", "context_refs", "artifacts", "evaluations", "policy_decisions", "links", "sessions")
	expectRef("WorkloadDetail", "summary", "#/components/schemas/WorkloadSummary")
	expectArrayPropertyRef("WorkloadDetail", "runs", "#/components/schemas/AgentRunRow")
	expectArrayPropertyRef("WorkloadDetail", "run_events", "#/components/schemas/AgentRunEventRow")
	expectArrayPropertyRef("WorkloadDetail", "model_calls", "#/components/schemas/ModelCallDetail")
	expectArrayPropertyRef("WorkloadDetail", "tool_calls", "#/components/schemas/ToolCallRow")
	expectArrayPropertyRef("WorkloadDetail", "context_refs", "#/components/schemas/ContextRefRow")
	expectArrayPropertyRef("WorkloadDetail", "artifacts", "#/components/schemas/ArtifactRow")
	expectArrayPropertyRef("WorkloadDetail", "evaluations", "#/components/schemas/EvaluationRow")
	expectArrayPropertyRef("WorkloadDetail", "policy_decisions", "#/components/schemas/PolicyDecisionRow")
	expectArrayPropertyRef("WorkloadDetail", "links", "#/components/schemas/WorkloadLinkRow")
	expectArrayPropertyRef("WorkloadDetail", "sessions", "#/components/schemas/SessionInfo")

	expectFields("AgentRunRow", "run_id", "workload_id", "source", "agent_name", "command", "cwd", "status", "duration_ms", "last_heartbeat_at", "heartbeat_count", "phase", "progress", "status_message", "confidence")
	expectFields("ModelCallDetail", "source", "session_id", "provider", "model", "calls", "input_tokens", "output_tokens", "cache_read", "cache_create", "reasoning", "tokens", "cost_usd", "pricing_source", "pricing_confidence", "first_at", "last_at", "confidence")
	expectFields("ToolCallRow", "tool_call_id", "workload_id", "run_id", "source", "tool_name", "tool_type", "status", "error_class", "duration_ms", "timestamp", "confidence")
	expectFields("ContextRefRow", "context_ref_id", "workload_id", "run_id", "ref_type", "ref_hash", "label", "repo", "git_branch", "commit_sha", "privacy_label", "created_at", "confidence")
	expectFields("ArtifactRow", "artifact_id", "workload_id", "run_id", "artifact_type", "label", "path_hash", "sha256", "metadata", "created_at", "confidence")
	expectFields("EvaluationRow", "evaluation_id", "workload_id", "evaluator", "status", "score", "signal", "notes", "created_at")
	expectFields("WorkloadLinkRow", "link_id", "source_workload_id", "target_workload_id", "relation", "reason", "created_by", "created_at", "confidence")

	expectFields("WorkloadGraph", "nodes", "edges")
	expectArrayPropertyRef("WorkloadGraph", "nodes", "#/components/schemas/GraphNode")
	expectArrayPropertyRef("WorkloadGraph", "edges", "#/components/schemas/GraphEdge")
	expectFields("GraphNode", "id", "kind", "label", "meta")
	expectFields("GraphEdge", "from", "to", "label")

	expectFields("WorkloadTimelineResponse", "workload_id", "rows")
	expectArrayPropertyRef("WorkloadTimelineResponse", "rows", "#/components/schemas/WorkloadTimelineRow")
	expectFields("WorkloadTimelineRow", "kind", "id", "run_id", "source", "label", "status", "detail", "tokens", "cost_usd", "duration_ms", "timestamp", "confidence")

	expectFields("WorkloadState", "workload_id", "goal", "status", "source", "phase", "terminal", "stale", "readiness_score", "progress", "next_action", "reasons", "risks", "project", "repo", "git_branch", "team", "last_activity", "stale_after_seconds", "runs", "active_runs", "stale_runs", "completed_runs", "failed_runs", "model_calls", "tool_calls", "context_refs", "artifacts", "evaluations", "positive_evaluations", "negative_evaluations", "policy_blocks", "policy_approvals_required", "budget_usd", "cost_usd", "tokens", "estimated_remaining_budget", "estimated_budget_exhausted")
	expectType("WorkloadState", "terminal", "boolean")
	expectType("WorkloadState", "readiness_score", "number")

	expectFields("WorkloadEventFeed", "rows", "total", "limit", "generated_at", "cursor", "from", "to", "stale_after_seconds")
	expectArrayPropertyRef("WorkloadEventFeed", "rows", "#/components/schemas/WorkloadFeedEvent")
	expectFields("WorkloadFeedEvent", "event_id", "event_type", "workload_id", "goal", "source", "project", "repo", "git_branch", "team", "phase", "severity", "message", "next_action", "timestamp", "terminal", "stale", "readiness_score", "progress", "tokens", "cost_usd", "reasons", "risks")
	expectType("WorkloadFeedEvent", "terminal", "boolean")
}

func TestOpenAPICoreControlPlaneSchemasExposeContractFields(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	paths := spec["paths"].(map[string]interface{})

	schema := func(name string) map[string]interface{} {
		t.Helper()
		raw, ok := schemas[name].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing: %#v", name, schemas[name])
		}
		return raw
	}
	props := func(name string) map[string]interface{} {
		t.Helper()
		raw := schema(name)
		properties, ok := raw["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing properties: %#v", name, raw)
		}
		return properties
	}
	expectFields := func(name string, fields ...string) map[string]interface{} {
		t.Helper()
		properties := props(name)
		for _, field := range fields {
			if properties[field] == nil {
				t.Fatalf("%s schema missing field %q: %#v", name, field, properties)
			}
		}
		return properties
	}
	expectRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		fieldSchema, ok := properties[field].(map[string]interface{})
		if !ok || fieldSchema["$ref"] != ref {
			t.Fatalf("%s.%s should reference %s: %#v", name, field, ref, properties[field])
		}
	}
	expectArrayRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		arraySchema, ok := properties[field].(map[string]interface{})
		if !ok || arraySchema["type"] != "array" {
			t.Fatalf("%s.%s should be an array: %#v", name, field, properties[field])
		}
		items, ok := arraySchema["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("%s.%s items should reference %s: %#v", name, field, ref, arraySchema["items"])
		}
	}
	expectPathResponseRef := func(path, method, ref string) {
		t.Helper()
		operation := paths[path].(map[string]interface{})[method].(map[string]interface{})
		response := operation["responses"].(map[string]interface{})["200"].(map[string]interface{})
		schema := response["content"].(map[string]interface{})["application/json"].(map[string]interface{})["schema"].(map[string]interface{})
		if schema["$ref"] != ref {
			t.Fatalf("%s %s should return %s: %#v", method, path, ref, schema)
		}
	}

	expectPathResponseRef("/api/integrations", "get", "#/components/schemas/CapabilityCatalog")
	expectPathResponseRef("/api/runtime/status", "get", "#/components/schemas/RuntimeStatus")
	expectPathResponseRef("/api/config/status", "get", "#/components/schemas/ConfigStatusReport")
	expectPathResponseRef("/api/readiness", "get", "#/components/schemas/ReadinessReport")
	expectPathResponseRef("/api/admission/check", "get", "#/components/schemas/AdmissionDecision")
	expectPathResponseRef("/api/goal-coverage", "get", "#/components/schemas/GoalCoverageReport")
	expectPathResponseRef("/api/event-schema", "get", "#/components/schemas/CanonicalEventSchema")
	expectPathResponseRef("/api/agent-profiles", "get", "#/components/schemas/AgentFrameworkProfileCatalog")
	expectPathResponseRef("/api/signal-taxonomy", "get", "#/components/schemas/SignalTaxonomyCatalog")
	expectPathResponseRef("/api/integrations/signal-coverage", "get", "#/components/schemas/SignalCoverageReport")
	expectPathResponseRef("/api/integrations/readiness", "get", "#/components/schemas/IntegrationReadinessReport")
	expectPathResponseRef("/api/integrations/smoke", "get", "#/components/schemas/IntegrationSmokeReport")
	expectPathResponseRef("/api/integrations/drift", "get", "#/components/schemas/IntegrationDriftReport")
	expectPathResponseRef("/api/integrations/lockfile", "get", "#/components/schemas/IntegrationLockfileReport")
	expectPathResponseRef("/api/integrations/upgrade-gate", "get", "#/components/schemas/IntegrationUpgradeGateReport")
	expectPathResponseRef("/api/integrations/recommendation", "get", "#/components/schemas/IntegrationRecommendationReport")
	expectPathResponseRef("/api/integrations/adapter-spec", "get", "#/components/schemas/AdapterContract")
	expectPathResponseRef("/api/integrations/conformance-matrix", "get", "#/components/schemas/AdapterConformanceMatrix")
	expectPathResponseRef("/api/events/validate", "post", "#/components/schemas/ValidationResponse")
	expectPathResponseRef("/api/events", "post", "#/components/schemas/IngestResponse")
	expectPathResponseRef("/api/notifications/webhook", "post", "#/components/schemas/WebhookNotificationResult")
	expectPathResponseRef("/api/notifications/desktop", "get", "#/components/schemas/DesktopNotificationPayload")

	expectFields("CapabilityCatalog", "product", "contract", "version", "privacy_default", "summary", "capabilities")
	expectRef("CapabilityCatalog", "summary", "#/components/schemas/CapabilitySummary")
	expectArrayRef("CapabilityCatalog", "capabilities", "#/components/schemas/IntegrationCapability")
	expectFields("CapabilitySummary", "implemented", "experimental", "planned", "enabled_collectors", "read_only_limited")
	expectFields("IntegrationCapability", "id", "name", "category", "protocol", "direction", "status", "maturity", "enabled", "writes_local_state", "available_in_read_only", "runtime_status", "privacy", "event_types", "endpoints", "commands", "tools", "resources", "prompts", "data_classes", "limitations", "next_milestones")
	expectFields("SignalTaxonomyCatalog", "product", "contract", "version", "generated_from", "local_first", "read_only_safe", "writes_local_state", "privacy_policy", "summary", "signals", "quality_gates", "routing_guidance")
	expectRef("SignalTaxonomyCatalog", "summary", "#/components/schemas/SignalTaxonomySummary")
	expectArrayRef("SignalTaxonomyCatalog", "signals", "#/components/schemas/SignalTaxonomySignal")
	expectFields("SignalTaxonomySummary", "signals", "event_signals", "usage_signals", "workload_signals", "policy_signals", "exact_preferred", "estimated_allowed")
	expectFields("SignalTaxonomySignal", "id", "label", "category", "description", "canonical_event_types", "recommended_fields", "accepted_aliases", "precision", "required_for", "privacy_class", "content_safe", "quality_gates")
	expectFields("SignalCoverageReport", "product", "contract", "version", "generated_from", "local_first", "read_only_safe", "writes_local_state", "taxonomy_hash", "adapter_spec_hash", "conformance_matrix_hash", "provider_profiles_hash", "agent_profiles_hash", "privacy_policy", "summary", "signals", "adapter_kinds", "gaps", "quality_gates", "routing_guidance")
	expectRef("SignalCoverageReport", "summary", "#/components/schemas/SignalCoverageSummary")
	expectArrayRef("SignalCoverageReport", "signals", "#/components/schemas/SignalCoverageSignal")
	expectArrayRef("SignalCoverageReport", "adapter_kinds", "#/components/schemas/SignalCoverageAdapter")
	expectArrayRef("SignalCoverageReport", "gaps", "#/components/schemas/SignalCoverageGap")
	expectFields("SignalCoverageSummary", "taxonomy_signals", "adapter_kinds", "provider_profiles", "agent_profiles", "covered_signals", "required_signal_references", "unknown_signal_references", "signals_without_adapter_coverage", "gaps")
	expectFields("SignalCoverageSignal", "id", "label", "category", "canonical_event_types", "required_by_adapter_kinds", "provider_profile_ids", "agent_profile_ids", "status")
	expectFields("SignalCoverageAdapter", "kind", "conformance_kind", "required_signals", "unknown_signals", "expected_event_types", "fixtures", "status")
	expectFields("SignalCoverageGap", "severity", "surface", "reference", "message", "remediation")
	expectFields("IntegrationReadinessReport", "product", "contract", "version", "local_first", "read_only_safe", "writes_local_state", "catalog_hash", "signal_coverage_hash", "runtime", "privacy_policy", "summary", "capabilities", "quality_gates", "operational_guidance")
	expectRef("IntegrationReadinessReport", "runtime", "#/components/schemas/IntegrationReadinessRuntime")
	expectRef("IntegrationReadinessReport", "summary", "#/components/schemas/IntegrationReadinessSummary")
	expectArrayRef("IntegrationReadinessReport", "capabilities", "#/components/schemas/IntegrationReadinessCapability")
	expectFields("IntegrationReadinessRuntime", "read_only", "rbac_enabled", "policies_enabled", "quota_enabled", "webhooks_enabled", "otlp_receiver_enabled", "otlp_receiver_grpc_enabled", "gateway_enabled", "pricing_mode")
	expectFields("IntegrationReadinessSummary", "total_capabilities", "enabled", "disabled", "experimental", "disabled_by_config", "ready", "review_required", "blocked", "warnings", "failures")
	expectFields("IntegrationReadinessCapability", "id", "name", "category", "status", "maturity", "direction", "enabled", "runtime_status", "writes_local_state", "available_in_read_only", "activation_state", "risk_level", "gates", "evidence", "actions")
	expectArrayRef("IntegrationReadinessCapability", "gates", "#/components/schemas/IntegrationReadinessGate")
	expectFields("IntegrationReadinessGate", "id", "severity", "status", "message", "remediation")
	expectFields("IntegrationSmokeReport", "product", "contract", "version", "local_first", "read_only_safe", "writes_local_state", "privacy_policy", "catalog_hash", "signal_coverage_hash", "integration_readiness_hash", "integration_recommendation_hash", "conformance_matrix_hash", "openapi_hash", "runtime_hash", "runtime", "summary", "fixture_coverage", "checks", "ci_commands", "quality_gates", "operational_guidance")
	expectRef("IntegrationSmokeReport", "runtime", "#/components/schemas/IntegrationSmokeRuntime")
	expectRef("IntegrationSmokeReport", "summary", "#/components/schemas/IntegrationSmokeSummary")
	expectRef("IntegrationSmokeReport", "fixture_coverage", "#/components/schemas/IntegrationSmokeFixtureCoverage")
	expectArrayRef("IntegrationSmokeReport", "checks", "#/components/schemas/IntegrationSmokeCheck")
	expectFields("IntegrationSmokeRuntime", "read_only", "rbac_enabled", "webhooks_enabled", "otlp_receiver_enabled", "otlp_receiver_grpc_enabled", "gateway_enabled", "pricing_mode")
	expectFields("IntegrationSmokeSummary", "status", "total_checks", "passed", "warnings", "failed", "review_required", "disabled_by_config")
	expectFields("IntegrationSmokeFixtureCoverage", "input_kinds", "fixtures", "strict_fixtures", "provider_fixtures", "provider_stream_fixtures", "otel_fixtures", "a2a_fixtures", "canonical_fixtures", "expected_kinds")
	expectFields("IntegrationSmokeCheck", "id", "category", "status", "severity", "message", "evidence", "remediation", "command")
	expectFields("IntegrationEvidenceKitReport", "product", "contract", "version", "local_first", "read_only_safe", "writes_local_state", "privacy_policy", "request", "kit_hash", "hashes", "summary", "target", "evidence_items", "fixture_evidence", "ci_commands", "reviewer_checklist", "redaction_rules", "operational_guidance")
	expectRef("IntegrationEvidenceKitReport", "request", "#/components/schemas/IntegrationEvidenceKitRequest")
	expectRef("IntegrationEvidenceKitReport", "hashes", "#/components/schemas/IntegrationEvidenceKitHashes")
	expectRef("IntegrationEvidenceKitReport", "summary", "#/components/schemas/IntegrationEvidenceKitSummary")
	expectRef("IntegrationEvidenceKitReport", "target", "#/components/schemas/IntegrationRolloutTarget")
	expectArrayRef("IntegrationEvidenceKitReport", "evidence_items", "#/components/schemas/IntegrationEvidenceItem")
	expectArrayRef("IntegrationEvidenceKitReport", "fixture_evidence", "#/components/schemas/IntegrationRolloutFixture")
	expectArrayRef("IntegrationEvidenceKitReport", "reviewer_checklist", "#/components/schemas/IntegrationEvidenceChecklist")
	expectFields("IntegrationEvidenceKitRequest", "agent_profile_id", "provider_profile_id", "surface", "min_confidence")
	expectFields("IntegrationEvidenceKitHashes", "capability_catalog_hash", "provider_profiles_hash", "agent_profiles_hash", "signal_taxonomy_hash", "signal_coverage_hash", "integration_readiness_hash", "integration_smoke_hash", "integration_compatibility_hash", "integration_rollout_plan_hash", "integration_recommendation_hash", "conformance_matrix_hash", "adapter_spec_hash", "canonical_schema_hash", "openapi_smoke_hash", "runtime_status_hash")
	expectFields("IntegrationEvidenceKitSummary", "status", "evidence_items", "required_items", "fixture_evidence", "strict_fixtures", "ci_commands", "reviewer_checks", "warnings", "requires_pricing_review", "requires_outbound_review")
	expectFields("IntegrationEvidenceItem", "id", "category", "title", "kind", "command", "endpoint", "mcp_tool", "resource", "hash", "required", "gate", "privacy", "evidence")
	expectFields("IntegrationEvidenceChecklist", "id", "owner", "title", "required", "checks", "privacy")
	expectFields("IntegrationDriftReport", "product", "contract", "version", "local_first", "read_only_safe", "writes_local_state", "privacy_policy", "request", "drift_hash", "current", "summary", "rows", "ci_commands", "operational_guidance", "redaction_rules")
	expectRef("IntegrationDriftReport", "request", "#/components/schemas/IntegrationDriftRequest")
	expectRef("IntegrationDriftReport", "summary", "#/components/schemas/IntegrationDriftSummary")
	expectArrayRef("IntegrationDriftReport", "rows", "#/components/schemas/IntegrationDriftRow")
	expectFields("IntegrationDriftRequest", "strict", "expected")
	expectFields("IntegrationDriftSummary", "status", "known_hashes", "expected_hashes", "checked", "matched", "drifted", "missing_expected", "unknown_expected", "warnings")
	expectFields("IntegrationDriftRow", "id", "title", "expected", "current", "status", "severity", "action", "privacy")
	expectFields("IntegrationLockfileReport", "product", "contract", "version", "format", "local_first", "read_only_safe", "writes_local_state", "privacy_policy", "lockfile_hash", "hash_ids", "hashes", "drift_command", "refresh_commands", "operational_guidance", "redaction_rules")
	expectFields("IntegrationUpgradeGateReport", "product", "contract", "version", "local_first", "read_only_safe", "writes_local_state", "privacy_policy", "request", "gate_hash", "decision", "drift_summary", "checks", "blocking_rows", "review_rows", "ci_commands", "required_artifacts", "operational_guidance", "redaction_rules")
	expectRef("IntegrationUpgradeGateReport", "request", "#/components/schemas/IntegrationUpgradeGateRequest")
	expectRef("IntegrationUpgradeGateReport", "decision", "#/components/schemas/IntegrationUpgradeGateDecision")
	expectRef("IntegrationUpgradeGateReport", "drift_summary", "#/components/schemas/IntegrationDriftSummary")
	expectArrayRef("IntegrationUpgradeGateReport", "checks", "#/components/schemas/IntegrationUpgradeGateCheck")
	expectArrayRef("IntegrationUpgradeGateReport", "blocking_rows", "#/components/schemas/IntegrationDriftRow")
	expectArrayRef("IntegrationUpgradeGateReport", "review_rows", "#/components/schemas/IntegrationDriftRow")
	expectFields("IntegrationUpgradeGateRequest", "strict", "expected")
	expectFields("IntegrationUpgradeGateDecision", "status", "severity", "reason", "recommended_ci_exit_code", "allow_write_ingest", "requires_human_review", "requires_evidence_refresh")
	expectFields("IntegrationUpgradeGateCheck", "id", "status", "severity", "message", "evidence", "remediation", "privacy")
	expectFields("SchemaEvolutionGateReport", "product", "contract", "version", "local_first", "read_only_safe", "writes_local_state", "privacy_policy", "request", "gate_hash", "decision", "current", "summary", "checks", "event_rows", "rejected_key_rows", "ci_commands", "required_artifacts", "migration_guidance", "redaction_rules")
	expectRef("SchemaEvolutionGateReport", "request", "#/components/schemas/SchemaEvolutionGateRequest")
	expectRef("SchemaEvolutionGateReport", "decision", "#/components/schemas/SchemaEvolutionDecision")
	expectRef("SchemaEvolutionGateReport", "current", "#/components/schemas/SchemaEvolutionCurrent")
	expectRef("SchemaEvolutionGateReport", "summary", "#/components/schemas/SchemaEvolutionSummary")
	expectArrayRef("SchemaEvolutionGateReport", "checks", "#/components/schemas/SchemaEvolutionCheck")
	expectArrayRef("SchemaEvolutionGateReport", "event_rows", "#/components/schemas/SchemaEvolutionRow")
	expectArrayRef("SchemaEvolutionGateReport", "rejected_key_rows", "#/components/schemas/SchemaEvolutionRow")
	expectFields("SchemaEvolutionGateRequest", "strict", "expected_version", "expected_schema_hash", "required_event_types", "required_rejected_keys")
	expectFields("SchemaEvolutionCurrent", "schema_version", "schema_hash", "supported_versions", "event_types", "rejected_payload_keys", "adapter_spec_hash")
	expectFields("SchemaEvolutionDecision", "status", "severity", "reason", "recommended_ci_exit_code", "allow_adapter_ingest", "requires_migration", "requires_human_review", "requires_lockfile_refresh")
	expectFields("SchemaEvolutionSummary", "status", "known_event_types", "required_event_types", "matched_event_types", "missing_event_types", "known_rejected_keys", "required_rejected_keys", "matched_rejected_keys", "missing_rejected_keys", "version_matches", "schema_hash_matches", "expected_schema_hash_present", "warnings")
	expectFields("SchemaEvolutionCheck", "id", "status", "severity", "message", "evidence", "remediation", "privacy")
	expectFields("SchemaEvolutionRow", "id", "kind", "expected", "current", "status", "severity", "action", "privacy")

	expectFields("GoalCoverageReport", "product", "slug", "contract", "version", "status", "local_first", "read_only", "prompt_content_stored", "usage_data_uploaded", "privacy_default", "capability_catalog_hash", "provider_profiles_hash", "agent_profiles_hash", "openapi_hash", "contract_bundle_hash", "canonical_schema_hash", "adapter_spec_hash", "coverage_hash", "summary", "sections", "external_dependencies", "verification", "privacy")
	expectRef("GoalCoverageReport", "summary", "#/components/schemas/GoalCoverageSummary")
	expectArrayRef("GoalCoverageReport", "sections", "#/components/schemas/GoalCoverageSection")
	expectArrayRef("GoalCoverageReport", "external_dependencies", "#/components/schemas/GoalCoverageExternal")
	expectFields("GoalCoverageSummary", "total_sections", "implemented", "experimental", "external_dependencies", "gaps", "completion_ratio", "next_action")
	expectFields("GoalCoverageSection", "id", "title", "category", "status", "maturity", "objective", "capability_ids", "evidence", "privacy", "limitations", "remaining")
	expectRef("GoalCoverageSection", "evidence", "#/components/schemas/GoalCoverageEvidence")
	expectFields("GoalCoverageEvidence", "endpoints", "commands", "mcp_tools", "mcp_resources", "capabilities", "tables", "tests", "docs")
	expectFields("GoalCoverageExternal", "id", "dependency", "reason", "local_status", "evidence")

	expectFields("RuntimeStatus", "contract", "version", "mode", "read_only", "write_operations", "background_tasks", "capability_catalog_hash", "canonical_schema_hash", "adapter_spec_hash", "disabled_features", "message")
	expectFields("ConfigStatusReport", "product", "slug", "contract", "version", "local_first", "privacy_default", "prompt_content_stored", "usage_data_uploaded", "path_values_exposed", "secret_values_exposed", "bind", "auth", "storage", "collectors", "pricing", "privacy", "features", "outbound", "teams", "summary", "issues", "privacy_note")
	expectRef("ConfigStatusReport", "bind", "#/components/schemas/ConfigBindStatus")
	expectRef("ConfigStatusReport", "auth", "#/components/schemas/ConfigAuthStatus")
	expectArrayRef("ConfigStatusReport", "collectors", "#/components/schemas/ConfigCollectorStatus")
	expectArrayRef("ConfigStatusReport", "issues", "#/components/schemas/ConfigStatusIssue")
	expectFields("ConfigFeatureStatus", "budgets_enabled", "budget_rule_count", "quota_enabled", "watchdog_enabled", "policies_enabled", "policy_rule_count", "otlp_receiver_enabled", "otlp_receiver_grpc_enabled", "gateway_enabled", "gateway_fallback_enabled", "gateway_fallback_rule_count")
	expectFields("ConfigOutboundStatus", "webhooks_enabled", "webhook_url_configured", "gateway_enabled", "gateway_fallback_enabled", "gateway_fallback_severity", "gateway_fallback_rule_count", "gateway_upstream_configured", "gateway_api_key_env_configured", "anthropic_upstream_configured", "anthropic_api_key_env_configured", "outbound_surfaces")

	expectFields("ReadinessReport", "product", "slug", "contract", "version", "generated_at", "status", "mode", "read_only", "accepts_writes", "local_first", "prompt_content_stored", "usage_data_uploaded", "summary", "checks", "privacy_note")
	expectRef("ReadinessReport", "summary", "#/components/schemas/ReadinessSummary")
	expectArrayRef("ReadinessReport", "checks", "#/components/schemas/ReadinessCheck")
	expectFields("ReadinessSummary", "total_checks", "passing_checks", "critical_failures", "warnings", "info", "usage_records", "prompt_events", "idempotency_keys", "idempotency_replays", "queue_claimable", "queue_non_terminal", "queue_oldest_claimable_age", "queue_next_lease_expiry", "active_leases", "expired_leases", "released_leases", "active_runs", "stale_runs", "oldest_run_age", "health_sources", "health_errors", "pricing_sources", "pricing_stale", "pricing_errors", "config_issues", "contract_checks", "contract_failures", "recommendation")
	expectFields("ReadinessCheck", "name", "ok", "severity", "message", "action")

	expectFields("AdmissionDecision", "product", "slug", "contract", "version", "generated_at", "status", "allowed", "surface", "operation", "role", "required_role", "rbac_enabled", "auth_configured", "read_only", "known_operation", "writes_local_state", "write_mode", "available_in_read_only", "local_or_auth_required", "prompt_content_stored", "usage_data_uploaded", "reason", "action", "privacy_note")
	expectFields("CanonicalEventSchema", "version", "supported_versions", "schema_hash", "privacy", "envelope_fields", "event_types", "examples_uri")
	expectRef("CanonicalEventSchema", "privacy", "#/components/schemas/CanonicalEventPrivacy")
	expectArrayRef("CanonicalEventSchema", "event_types", "#/components/schemas/CanonicalEventTypeInfo")
	expectFields("CanonicalEventTypeInfo", "event_type", "description", "required", "payload_fields")
	expectFields("CanonicalEventValidation", "event_id", "status", "event_type", "source", "payload_hash", "warnings")
	expectFields("CanonicalEventResult", "event_id", "status", "event_type", "workload_id", "run_id", "derived")
	expectArrayRef("ValidationResponse", "results", "#/components/schemas/CanonicalEventValidation")
	expectArrayRef("IngestResponse", "results", "#/components/schemas/CanonicalEventResult")

	expectFields("AdapterContract", "product", "contract", "version", "purpose", "schema_version", "schema_hash", "privacy_policy", "supported_input_kinds", "canonical_event_types", "required_envelope", "recommended_envelope", "forbidden_payload_keys", "token_semantics", "quality_gates", "validation", "ingest", "roadmap_compatibility")
	expectArrayRef("AdapterContract", "supported_input_kinds", "#/components/schemas/AdapterInputKind")
	expectArrayRef("AdapterContract", "canonical_event_types", "#/components/schemas/CanonicalEventTypeInfo")
	expectRef("AdapterContract", "validation", "#/components/schemas/AdapterValidationContract")
	expectRef("AdapterContract", "ingest", "#/components/schemas/AdapterIngestContract")
	expectFields("AdapterInputKind", "kind", "description", "conformance_kind", "convert_command", "ingest_command", "endpoint", "required_signals", "privacy_notes")
	expectFields("AdapterValidationContract", "http", "cli", "mcp_tool", "strict_ci")
	expectFields("AdapterIngestContract", "http", "cli", "mcp_tools")
	expectFields("AdapterConformanceMatrix", "product", "contract", "version", "local_first", "read_only_safe", "writes_local_state", "schema_version", "schema_hash", "adapter_spec_hash", "provider_profiles_hash", "privacy_policy", "summary", "kinds", "quality_gates", "routing_guidance")
	expectRef("AdapterConformanceMatrix", "summary", "#/components/schemas/AdapterConformanceSummary")
	expectArrayRef("AdapterConformanceMatrix", "kinds", "#/components/schemas/AdapterConformanceKind")
	expectFields("AdapterConformanceSummary", "input_kinds", "fixtures", "strict_fixtures", "provider_fixtures", "provider_stream_fixtures", "otel_fixtures", "a2a_fixtures", "canonical_fixtures")
	expectFields("AdapterConformanceKind", "kind", "conformance_kind", "description", "status", "maturity", "endpoint", "cli_command", "mcp_tool", "strict_ci_command", "convert_command", "ingest_command", "accepted_formats", "required_signals", "privacy_notes", "expected_event_types", "fixtures")
	expectArrayRef("AdapterConformanceKind", "fixtures", "#/components/schemas/AdapterConformanceFixture")
	expectFields("AdapterConformanceFixture", "path", "format", "scenario", "strict", "command", "provider_profile_ids", "expected_event_types", "privacy")
	expectFields("AgentFrameworkProfileCatalog", "product", "contract", "version", "generated_from", "local_first", "read_only_safe", "writes_local_state", "privacy_policy", "summary", "profiles", "quality_gates", "routing_guidance")
	expectRef("AgentFrameworkProfileCatalog", "summary", "#/components/schemas/AgentFrameworkSummary")
	expectArrayRef("AgentFrameworkProfileCatalog", "profiles", "#/components/schemas/AgentFrameworkProfile")
	expectFields("AgentFrameworkSummary", "profiles", "local_collectors", "canonical_event_ingest", "provider_envelope_ingest", "provider_stream_ingest", "mcp_tooling", "a2a_telemetry", "otel_telemetry", "workload_heartbeats", "multi_agent_routers")
	expectFields("AgentFrameworkProfile", "id", "label", "kind", "families", "supported_surfaces", "recommended_ingest", "fallback_ingest", "available_signals", "canonical_event_types", "collector_sources", "adapter_input_kinds", "mcp_tools", "mcp_resources", "http_endpoints", "cli_commands", "privacy_notes", "quality_gates", "limitations")
	expectFields("IntegrationRecommendationReport", "product", "contract", "version", "local_first", "read_only_safe", "writes_local_state", "privacy_policy", "request", "matched_agent_profile", "matched_provider_profile", "recommended_surface", "recommendation", "confidence", "ingest_path", "fallback_paths", "http", "cli", "mcp_tools", "mcp_resources", "validation", "strict_ci", "expected_event_types", "required_signals", "missing_signals", "privacy_checklist", "quality_gates", "limitations", "hashes", "next_steps", "related_profiles")
	expectRef("IntegrationRecommendationReport", "request", "#/components/schemas/IntegrationRecommendationRequest")
	expectRef("IntegrationRecommendationReport", "matched_agent_profile", "#/components/schemas/AgentFrameworkProfile")
	expectRef("IntegrationRecommendationReport", "matched_provider_profile", "#/components/schemas/ProviderProfile")
	expectRef("IntegrationRecommendationReport", "related_profiles", "#/components/schemas/IntegrationRecommendationProfileRef")
	expectFields("IntegrationRecommendationRequest", "agent_profile_id", "provider_profile_id", "surface", "signals", "runtime_mode", "read_only")
	expectFields("IntegrationRecommendationProfileRef", "agent_profile_ids", "provider_profile_ids", "conformance_kinds")

	expectFields("OperationResult", "ok", "source", "reset", "mode", "result")
	expectRef("OperationResult", "result", "#/components/schemas/ProjectionRepairResult")
	expectFields("WebhookNotificationResult", "result", "payload")
	expectRef("WebhookNotificationResult", "result", "#/components/schemas/WebhookDeliveryResult")
	expectRef("WebhookNotificationResult", "payload", "#/components/schemas/WebhookNotificationPayload")
	expectFields("WebhookDeliveryResult", "enabled", "dry_run", "event_count", "approval_count", "approval_route_count", "status_code", "message")
	expectFields("WebhookNotificationPayload", "product", "kind", "generated_at", "summary", "events", "approvals", "approval_routes")
	expectRef("WebhookNotificationPayload", "summary", "#/components/schemas/WebhookNotificationSummary")
	expectArrayRef("WebhookNotificationPayload", "events", "#/components/schemas/WorkloadFeedEvent")
	expectArrayRef("WebhookNotificationPayload", "approvals", "#/components/schemas/WebhookNotificationApproval")
	expectFields("WebhookNotificationSummary", "total", "pending_approvals", "approval_routes", "by_phase", "by_severity")
	expectFields("WebhookNotificationApproval", "request_id", "policy_decision_id", "workload_id", "run_id", "source", "model", "project", "action", "target", "actor_role", "status", "required_approvals", "approval_votes", "rejection_votes", "escalation_after_seconds", "due_at", "overdue", "reason", "created_at", "updated_at")
	expectFields("WebhookNotificationApprovalRoutes", "generated_at", "due_within", "summary", "routes")
	expectArrayRef("WebhookNotificationApprovalRoutes", "routes", "#/components/schemas/WebhookNotificationApprovalRoute")
	expectFields("WebhookNotificationApprovalRoute", "route_key_hash", "approver_hash", "escalation_target_hash", "pending", "overdue", "due_soon", "approval_votes", "rejection_votes", "max_required_approvals", "due_next", "sources", "models", "projects", "actions")
	expectFields("DesktopNotificationPayload", "product", "kind", "generated_at", "title", "body", "severity", "summary", "notifications")
	expectRef("DesktopNotificationPayload", "summary", "#/components/schemas/WebhookNotificationSummary")
	expectArrayRef("DesktopNotificationPayload", "notifications", "#/components/schemas/DesktopNotificationItem")
	expectFields("DesktopNotificationItem", "title", "body", "severity", "phase", "source", "model", "action", "timestamp", "next_action")
}

func TestOpenAPIEcosystemIngestSchemasExposeTelemetryFields(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	schemas := spec["components"].(map[string]interface{})["schemas"].(map[string]interface{})
	paths := spec["paths"].(map[string]interface{})

	schema := func(name string) map[string]interface{} {
		t.Helper()
		raw, ok := schemas[name].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing: %#v", name, schemas[name])
		}
		return raw
	}
	props := func(name string) map[string]interface{} {
		t.Helper()
		raw := schema(name)
		properties, ok := raw["properties"].(map[string]interface{})
		if !ok {
			t.Fatalf("%s schema missing properties: %#v", name, raw)
		}
		return properties
	}
	expectFields := func(name string, fields ...string) map[string]interface{} {
		t.Helper()
		properties := props(name)
		for _, field := range fields {
			if properties[field] == nil {
				t.Fatalf("%s schema missing field %q: %#v", name, field, properties)
			}
		}
		return properties
	}
	expectRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		fieldSchema, ok := properties[field].(map[string]interface{})
		if !ok || fieldSchema["$ref"] != ref {
			t.Fatalf("%s.%s should reference %s: %#v", name, field, ref, properties[field])
		}
	}
	expectArrayRef := func(name, field, ref string) {
		t.Helper()
		properties := props(name)
		arraySchema, ok := properties[field].(map[string]interface{})
		if !ok || arraySchema["type"] != "array" {
			t.Fatalf("%s.%s should be an array: %#v", name, field, properties[field])
		}
		items, ok := arraySchema["items"].(map[string]interface{})
		if !ok || items["$ref"] != ref {
			t.Fatalf("%s.%s items should reference %s: %#v", name, field, ref, arraySchema["items"])
		}
	}
	expectOneOfRefs := func(name string, refs ...string) {
		t.Helper()
		raw := schema(name)
		oneOf, ok := raw["oneOf"].([]map[string]interface{})
		if !ok {
			t.Fatalf("%s should expose oneOf variants: %#v", name, raw)
		}
		seen := map[string]bool{}
		for _, variant := range oneOf {
			if ref, ok := variant["$ref"].(string); ok {
				seen[ref] = true
			}
			if items, ok := variant["items"].(map[string]interface{}); ok {
				if ref, ok := items["$ref"].(string); ok {
					seen[ref] = true
				}
			}
		}
		for _, ref := range refs {
			if !seen[ref] {
				t.Fatalf("%s oneOf missing %s: %#v", name, ref, oneOf)
			}
		}
	}
	expectPathResponseRef := func(path, method, ref string) {
		t.Helper()
		operation := paths[path].(map[string]interface{})[method].(map[string]interface{})
		response := operation["responses"].(map[string]interface{})["200"].(map[string]interface{})
		schema := response["content"].(map[string]interface{})["application/json"].(map[string]interface{})["schema"].(map[string]interface{})
		if schema["$ref"] != ref {
			t.Fatalf("%s %s should return %s: %#v", method, path, ref, schema)
		}
	}

	expectPathResponseRef("/api/otel/genai", "post", "#/components/schemas/EcosystemIngestResponse")
	expectPathResponseRef("/api/a2a/tasks", "post", "#/components/schemas/EcosystemIngestResponse")
	expectPathResponseRef("/api/provider/calls", "post", "#/components/schemas/EcosystemIngestResponse")
	expectPathResponseRef("/gateway/openai/v1/chat/completions", "post", "#/components/schemas/GatewayResponse")
	expectPathResponseRef("/gateway/openai/v1/responses", "post", "#/components/schemas/GatewayResponse")
	expectPathResponseRef("/gateway/anthropic/v1/messages", "post", "#/components/schemas/GatewayResponse")

	expectOneOfRefs("OTelGenAIRequest", "#/components/schemas/OTelSpan", "#/components/schemas/OTelSpanEnvelope", "#/components/schemas/OTelResourceSpansEnvelope")
	expectFields("OTelSpan", "trace_id", "traceId", "span_id", "spanId", "parent_span_id", "parentSpanId", "name", "start_time", "startTime", "startTimeUnixNano", "end_time", "endTime", "endTimeUnixNano", "attributes", "resource_attributes")
	expectFields("OTelAttribute", "key", "value")
	expectRef("OTelAttribute", "value", "#/components/schemas/OTelAttributeValue")
	expectFields("OTelSpanEnvelope", "spans")
	expectArrayRef("OTelSpanEnvelope", "spans", "#/components/schemas/OTelSpan")
	expectFields("OTelResourceSpansEnvelope", "resourceSpans")
	expectOneOfRefs("OTLPTraceRequest", "#/components/schemas/OTelResourceSpansEnvelope", "#/components/schemas/OTelSpanEnvelope")

	expectFields("DiscoveryManifest", "contract_bundle_uri", "openapi_uri", "capability_catalog_hash", "provider_profiles_uri", "provider_profiles_hash", "agent_profiles_uri", "agent_profiles_hash", "signal_taxonomy_uri", "signal_taxonomy_hash", "signal_coverage_uri", "signal_coverage_hash", "integration_readiness_uri", "integration_readiness_hash", "integration_smoke_uri", "integration_smoke_hash", "integration_compatibility_uri", "integration_compatibility_hash", "integration_rollout_plan_uri", "integration_rollout_plan_hash", "integration_evidence_kit_uri", "integration_evidence_kit_hash", "integration_drift_uri", "integration_drift_hash", "integration_lockfile_uri", "integration_lockfile_hash", "integration_upgrade_gate_uri", "integration_upgrade_gate_hash", "schema_evolution_gate_uri", "schema_evolution_gate_hash", "integration_recommendation_uri", "integration_recommendation_hash", "conformance_matrix_uri", "conformance_matrix_hash", "runtime_status_uri", "canonical_schema_uri", "canonical_schema_hash", "event_examples_uri", "adapter_spec_uri", "adapter_spec_hash", "adapter_conformance_uri", "a2a")
	expectRef("DiscoveryManifest", "a2a", "#/components/schemas/A2ADiscoveryMetadata")
	expectFields("A2ADiscoveryMetadata", "mode", "protocol", "full_server", "endpoint", "http_methods", "required_role", "available_in_read_only", "max_body_bytes", "adapter_spec_uri", "adapter_spec_hash", "conformance_uri", "conformance_kind", "strict_fixture", "supported_task_shapes", "canonical_event_types", "supports_delegated_lineage", "supports_evidence_references", "supports_parent_placeholders", "message_content_stored", "artifact_part_content_stored", "prompt_content_stored", "privacy", "limitations")
	expectOneOfRefs("A2ATaskRequest", "#/components/schemas/A2ATask", "#/components/schemas/A2ATaskEnvelope")
	expectFields("A2AStatus", "state", "timestamp")
	expectFields("A2AArtifact", "artifact_id", "artifactId", "id", "name", "description", "parts", "metadata")
	expectFields("A2AEvidenceRef", "id", "evidence_id", "context_ref_id", "ref_id", "ref_type", "type", "kind", "ref_hash", "sha256", "hash", "label", "name", "title", "repo", "repository", "git_branch", "branch", "commit_sha", "commit", "privacy_label", "privacy", "confidence")
	expectFields("A2ATask", "id", "taskId", "task_id", "contextId", "context_id", "parentTaskId", "parent_task_id", "delegatedByTaskId", "delegated_by_task_id", "parentWorkloadId", "parent_workload_id", "kind", "status", "artifact", "artifacts", "evidence_refs", "evidenceReferences", "metadata")
	expectRef("A2ATask", "status", "#/components/schemas/A2AStatus")
	expectArrayRef("A2ATask", "artifacts", "#/components/schemas/A2AArtifact")
	expectArrayRef("A2ATask", "evidence_refs", "#/components/schemas/A2AEvidenceRef")
	expectArrayRef("A2ATask", "evidenceReferences", "#/components/schemas/A2AEvidenceRef")
	expectFields("A2ATaskEnvelope", "task", "result", "tasks", "events")
	expectArrayRef("A2ATaskEnvelope", "tasks", "#/components/schemas/A2ATask")

	expectOneOfRefs("ProviderUsageRequest", "#/components/schemas/ProviderCall", "#/components/schemas/ProviderUsageEnvelope", "#/components/schemas/ProviderMetadataWrapper")
	expectFields("ProviderUsage", "input_tokens", "prompt_tokens", "cache_read_input_tokens", "cache_read_tokens", "cache_creation_input_tokens", "cache_write_input_tokens", "cache_write_tokens", "output_tokens", "completion_tokens", "reasoning_output_tokens", "input_tokens_details", "prompt_tokens_details", "output_tokens_details", "completion_tokens_details", "cost_usd", "total_cost", "cost")
	expectFields("ProviderCall", "id", "response_id", "completion_id", "request_id", "provider", "system", "gen_ai.provider.name", "model", "model_id", "modelID", "project", "session_id", "created_at", "created", "timestamp", "finish_reason", "stop_reason", "usage", "request", "response", "request_metadata", "response_metadata", "reconciliation", "metadata")
	expectRef("ProviderCall", "usage", "#/components/schemas/ProviderUsage")
	expectRef("ProviderCall", "metadata", "#/components/schemas/GatewayLedgerMetadata")
	expectFields("ProviderUsageEnvelope", "responses", "calls", "items")
	expectArrayRef("ProviderUsageEnvelope", "calls", "#/components/schemas/ProviderCall")
	expectFields("ProviderMetadataWrapper", "provider", "model", "request", "response", "request_metadata", "response_metadata", "reconciliation", "metadata")
	expectRef("ProviderMetadataWrapper", "response", "#/components/schemas/ProviderCall")

	expectFields("EcosystemIngestResponse", "ok", "spans", "calls", "tasks", "events", "warning", "budget_advisories", "budget_warning", "reconciliation_hooks", "results")
	expectArrayRef("EcosystemIngestResponse", "results", "#/components/schemas/CanonicalEventResult")
	expectFields("GatewayLedgerMetadata", "agent_ledger.project", "agent_ledger.goal", "agent_ledger.workload_id", "agent_ledger.agent_run_id", "agent_ledger.session_id", "agent_ledger.git_branch", "project", "goal", "workload_id", "agent_run_id", "run_id", "session_id", "git_branch", "branch")
	expectFields("GatewayRequest", "model", "stream", "metadata", "max_tokens", "temperature", "stream_options", "tools")
	expectRef("GatewayRequest", "metadata", "#/components/schemas/GatewayLedgerMetadata")
	expectFields("GatewayResponse", "id", "object", "type", "model", "usage", "choices", "output")
	expectRef("GatewayResponse", "usage", "#/components/schemas/ProviderUsage")
}

func TestOpenAPIRequestBodyOperationsAdvertiseBodyLimits(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	paths := spec["paths"].(map[string]interface{})
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI path %s has invalid item: %#v", path, rawPathItem)
		}
		for _, method := range []string{"post", "put", "patch"} {
			rawOperation, ok := pathItem[method]
			if !ok {
				continue
			}
			operation, ok := rawOperation.(map[string]interface{})
			if !ok {
				t.Fatalf("OpenAPI %s %s has invalid operation: %#v", method, path, rawOperation)
			}
			if _, hasBody := operation["requestBody"]; !hasBody {
				continue
			}
			meta, ok := operation["x-agent-ledger"].(map[string]interface{})
			if !ok {
				t.Fatalf("OpenAPI %s %s with requestBody missing x-agent-ledger metadata: %#v", method, path, operation)
			}
			limit, ok := meta["max_body_bytes"].(int)
			if !ok || limit <= 0 {
				t.Fatalf("OpenAPI %s %s missing positive max_body_bytes: %#v", method, path, meta)
			}
			if !openAPIMethodHasResponse(paths, path, method, "413") {
				t.Fatalf("OpenAPI %s %s with requestBody should advertise 413: %#v", method, path, operation)
			}
		}
	}
}

func TestOpenAPIGetOperationsDeclareRevalidationPolicy(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	paths := spec["paths"].(map[string]interface{})
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI path %s has invalid item: %#v", path, rawPathItem)
		}
		rawOperation, ok := pathItem["get"]
		if !ok {
			continue
		}
		operation, ok := rawOperation.(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI GET %s has invalid operation: %#v", path, rawOperation)
		}
		if openAPIMethodHasResponse(paths, path, "get", "304") || openAPIOperationETagPolicy(operation) != "" {
			continue
		}
		t.Fatalf("OpenAPI GET %s missing 304 or explicit x-agent-ledger.etag policy: %#v", path, operation)
	}
}

func TestOpenAPIOperationsExposeStableUniqueOperationIDs(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	paths := spec["paths"].(map[string]interface{})
	seen := map[string]string{}
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI path %s has invalid item: %#v", path, rawPathItem)
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			rawOperation, ok := pathItem[method]
			if !ok {
				continue
			}
			operation, ok := rawOperation.(map[string]interface{})
			if !ok {
				t.Fatalf("OpenAPI %s %s has invalid operation: %#v", method, path, rawOperation)
			}
			id, _ := operation["operationId"].(string)
			if !contractOpenAPIOperationIDValid(id) {
				t.Fatalf("OpenAPI %s %s has invalid operationId %q: %#v", method, path, id, operation)
			}
			if previous, ok := seen[id]; ok {
				t.Fatalf("OpenAPI operationId %q reused by %s %s and %s", id, method, path, previous)
			}
			seen[id] = method + " " + path
		}
	}
	if len(seen) == 0 {
		t.Fatal("OpenAPI operationId check did not inspect any operations")
	}
}

func TestOpenAPIIdempotentOperationsAdvertiseRetryContract(t *testing.T) {
	spec := OpenAPISpecFor(Options{}, nil)
	paths := spec["paths"].(map[string]interface{})
	checked := 0
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI path %s has invalid item: %#v", path, rawPathItem)
		}
		for _, method := range []string{"post", "put", "patch"} {
			rawOperation, ok := pathItem[method]
			if !ok {
				continue
			}
			operation, ok := rawOperation.(map[string]interface{})
			if !ok {
				t.Fatalf("OpenAPI %s %s has invalid operation: %#v", method, path, rawOperation)
			}
			if contractOpenAPIOperationIdempotency(operation) == "" {
				continue
			}
			checked++
			if !contractOpenAPIOperationHasHeaderParam(operation, "Idempotency-Key") ||
				!contractOpenAPIOperationHasHeaderParam(operation, "X-Idempotency-Key") {
				t.Fatalf("OpenAPI %s %s idempotent operation missing retry key headers: %#v", method, path, operation)
			}
			if _, hasBody := operation["requestBody"]; !hasBody {
				t.Fatalf("OpenAPI %s %s idempotent operation missing requestBody: %#v", method, path, operation)
			}
			if !contractOpenAPIOperationHasBodyLimit(operation) {
				t.Fatalf("OpenAPI %s %s idempotent operation missing body limit: %#v", method, path, operation)
			}
			if !openAPIMethodHasResponse(paths, path, method, "409") {
				t.Fatalf("OpenAPI %s %s idempotent operation missing 409 conflict: %#v", method, path, operation)
			}
		}
	}
	if checked < 2 {
		t.Fatalf("expected workload and run idempotent operations, checked=%d", checked)
	}
	for _, schema := range []string{"WorkloadCreateRequest", "AgentRunStartRequest"} {
		if !openAPIComponentSchemaHasProperty(spec, schema, "idempotency_key") {
			t.Fatalf("OpenAPI schema %s missing JSON idempotency_key property", schema)
		}
	}
}

func TestContractVerificationReportIsOKAndPrivacySafe(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Collectors.Claude.Enabled = true
	cfg.Collectors.Claude.Paths = []string{"C:/Users/example/.claude/projects"}
	runtime := EnrichRuntimeStatus(&storage.RuntimeStatus{
		Mode:            "control-plane",
		ReadOnly:        false,
		WriteOperations: "enabled",
		BackgroundTasks: "enabled",
		Message:         "test runtime",
	}, OptionsFromConfig(cfg))
	report := ContractVerificationReportFor(OptionsFromConfig(cfg), runtime)
	if report.Contract != "agent-ledger.contract-verification" || report.Version != "v1" || !report.OK || report.Failed != 0 || report.Checked == 0 {
		t.Fatalf("unexpected verification report: %#v", report)
	}
	if report.BundleHash == "" || report.OpenAPIHash == "" || !strings.HasPrefix(report.BundleHash, "sha256:") || !strings.HasPrefix(report.OpenAPIHash, "sha256:") {
		t.Fatalf("verification report missing hashes: %#v", report)
	}
	for _, name := range []string{"discovery.contract_bundle_uri", "discovery.agent_profiles", "discovery.signal_taxonomy", "discovery.signal_coverage", "discovery.integration_readiness", "discovery.integration_smoke", "discovery.integration_evidence_kit", "discovery.integration_drift", "discovery.integration_lockfile", "discovery.integration_upgrade_gate", "discovery.schema_evolution_gate", "discovery.integration_recommendation", "discovery.conformance_matrix", "discovery.a2a_metadata", "bundle.document.openapi", "bundle.document.agent-profiles", "bundle.document.signal-taxonomy", "bundle.document.signal-coverage", "bundle.document.integration-readiness", "bundle.document.integration-smoke", "bundle.document.integration-evidence-kit", "bundle.document.integration-drift", "bundle.document.integration-lockfile", "bundle.document.integration-upgrade-gate", "bundle.document.schema-evolution-gate", "bundle.document.integration-recommendation", "bundle.document.adapter-conformance-matrix", "bundle.document.a2a-discovery", "canonical.examples", "adapter.schema_alignment", "adapter.input_kinds", "adapter.conformance_matrix", "adapter.signal_coverage", "privacy.public_metadata_language", "openapi.agent_profiles_hash", "openapi.signal_taxonomy_hash", "openapi.signal_coverage_hash", "openapi.integration_readiness_hash", "openapi.integration_smoke_hash", "openapi.integration_evidence_kit_hash", "openapi.integration_drift_hash", "openapi.integration_lockfile_hash", "openapi.integration_upgrade_gate_hash", "openapi.schema_evolution_gate_hash", "openapi.integration_recommendation_hash", "openapi.conformance_matrix_hash", "openapi.path./api/contracts/verify", "openapi.path./api/signal-taxonomy", "openapi.path./api/integrations/signal-coverage", "openapi.path./api/integrations/readiness", "openapi.path./api/integrations/smoke", "openapi.path./api/integrations/drift", "openapi.path./api/integrations/lockfile", "openapi.path./api/integrations/upgrade-gate", "openapi.path./api/schema/evolution-gate", "openapi.path./api/integrations/recommendation", "openapi.privacy", "openapi.auth_scheme", "openapi.operation_auth", "openapi.operation_ids", "openapi.operation_admission", "openapi.operation_methods", "openapi.request_body_limits", "openapi.idempotency", "openapi.get_revalidation"} {
		if !verificationReportHasCheck(report, name) {
			t.Fatalf("verification report missing check %q: %#v", name, report.Checks)
		}
	}
	first := ContractVerificationFingerprintFrom(report)
	second := ContractVerificationFingerprint(OptionsFromConfig(cfg), runtime)
	if first == "" || first != second {
		t.Fatalf("verification fingerprint unstable: %q vs %q", first, second)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	for _, forbidden := range []string{"C:/Users/example", ".claude/projects"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("verification report leaked path %q: %s", forbidden, string(raw))
		}
	}
	for _, check := range report.Checks {
		if !check.OK {
			t.Fatalf("verification check failed: %#v", check)
		}
	}
}

func verificationReportHasCheck(report ContractVerificationReport, name string) bool {
	for _, check := range report.Checks {
		if check.Name == name {
			return true
		}
	}
	return false
}

func contractBundleHasDocument(bundle ContractBundle, id string) bool {
	for _, doc := range bundle.Documents {
		if doc.ID == id {
			return true
		}
	}
	return false
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
	for _, kind := range SupportedAdapterConformanceKinds() {
		if !adapterContractHasKind(spec, kind) {
			t.Fatalf("adapter contract missing %s kind: %#v", kind, spec.SupportedInputKinds)
		}
	}
	if !stringSliceHas(spec.ForbiddenPayloadKeys, "prompt") || !stringSliceHas(spec.ForbiddenPayloadKeys, "messages") {
		t.Fatalf("adapter contract missing forbidden payload keys: %#v", spec.ForbiddenPayloadKeys)
	}
	if spec.Validation.HTTP == "" || spec.Validation.CLI == "" || spec.Ingest.HTTP == nil || spec.Ingest.CLI == nil {
		t.Fatalf("adapter contract missing validation or ingest entrypoints: %#v", spec)
	}
	if ok, actual := contractAdapterSchemaStatus(spec); !ok {
		t.Fatalf("adapter contract is not aligned with canonical schema: %s", actual)
	}
	if ok, actual := contractAdapterInputKindStatus(spec); !ok {
		t.Fatalf("adapter contract is not aligned with conformance decoder: %s", actual)
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

func openAPIGetHasResponse(paths map[string]interface{}, path, status string) bool {
	return openAPIMethodHasResponse(paths, path, "get", status)
}

func openAPIMethodHasResponse(paths map[string]interface{}, path, method, status string) bool {
	pathItem, ok := paths[path].(map[string]interface{})
	if !ok {
		return false
	}
	operation, ok := pathItem[method].(map[string]interface{})
	if !ok {
		return false
	}
	responses, ok := operation["responses"].(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = responses[status]
	return ok
}

func openAPIMethodAllowHeader(paths map[string]interface{}, path, method string) string {
	pathItem, ok := paths[path].(map[string]interface{})
	if !ok {
		return ""
	}
	operation, ok := pathItem[method].(map[string]interface{})
	if !ok {
		return ""
	}
	responses, ok := operation["responses"].(map[string]interface{})
	if !ok {
		return ""
	}
	response, ok := responses["405"].(map[string]interface{})
	if !ok {
		return ""
	}
	headers, ok := response["headers"].(map[string]interface{})
	if !ok {
		return ""
	}
	allow, ok := headers["Allow"].(map[string]interface{})
	if !ok {
		return ""
	}
	schema, ok := allow["schema"].(map[string]interface{})
	if !ok {
		return ""
	}
	value, _ := schema["const"].(string)
	return value
}

func openAPIHasBearerSecurityScheme(spec map[string]interface{}) bool {
	components, ok := spec["components"].(map[string]interface{})
	if !ok {
		return false
	}
	schemes, ok := components["securitySchemes"].(map[string]interface{})
	if !ok {
		return false
	}
	scheme, ok := schemes["AgentLedgerBearer"].(map[string]interface{})
	if !ok {
		return false
	}
	return scheme["type"] == "http" && scheme["scheme"] == "bearer"
}

func openAPIOperationHasBearerSecurity(operation map[string]interface{}) bool {
	security, ok := operation["security"].([]map[string][]string)
	if !ok {
		return false
	}
	for _, requirement := range security {
		if _, ok := requirement["AgentLedgerBearer"]; ok {
			return true
		}
	}
	return false
}

func openAPIOperationHasAdmissionMetadata(operation map[string]interface{}) bool {
	meta, ok := operation["x-agent-ledger"].(map[string]interface{})
	if !ok {
		return false
	}
	role, roleOK := meta["required_role"].(string)
	writeMode, writeModeOK := meta["write_mode"].(string)
	_, readOnlyOK := meta["available_in_read_only"].(bool)
	return roleOK && role != "" && writeModeOK && writeMode != "" && readOnlyOK
}

func openAPIOperationETagPolicy(operation map[string]interface{}) string {
	meta, ok := operation["x-agent-ledger"].(map[string]interface{})
	if !ok {
		return ""
	}
	policy, _ := meta["etag"].(string)
	return policy
}

func openAPIComponentSchemaHasProperty(spec map[string]interface{}, schemaName, propertyName string) bool {
	components, ok := spec["components"].(map[string]interface{})
	if !ok {
		return false
	}
	schemas, ok := components["schemas"].(map[string]interface{})
	if !ok {
		return false
	}
	schema, ok := schemas[schemaName].(map[string]interface{})
	if !ok {
		return false
	}
	properties, ok := schema["properties"].(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = properties[propertyName]
	return ok
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

func assertCapabilityDataClass(t *testing.T, catalog Catalog, id, dataClass string) {
	t.Helper()
	for _, cap := range catalog.Capabilities {
		if cap.ID == id {
			if !stringSliceHas(cap.DataClasses, dataClass) {
				t.Fatalf("%s missing data class %q: %#v", id, dataClass, cap.DataClasses)
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
