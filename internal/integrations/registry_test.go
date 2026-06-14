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
	assertCapabilityCommand(t, catalog, "protocol.adapter_conformance", "agent-ledger adapter spec")
	assertCapabilityCommand(t, catalog, "protocol.discovery_manifest", "agent-ledger discovery")
	assertCapabilityCommand(t, catalog, "protocol.contract_bundle", "agent-ledger contracts")
	assertCapabilityCommand(t, catalog, "protocol.contract_verification", "agent-ledger contracts verify")
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
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.discovery")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.openapi")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.runtime_status")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.config_status")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.readiness")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.admission_check")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.claim_next_workload")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.workload_queue")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.acquire_workload_lease")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.renew_workload_lease")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.release_workload_lease")
	assertCapabilityTool(t, catalog, "protocol.mcp_stdio", "ledger.workload_leases")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://contracts/bundle")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://contracts/verification")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://discovery/manifest")
	assertCapabilityResource(t, catalog, "protocol.mcp_stdio", "agent-ledger://contracts/openapi")
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
	assertRuntimeCapability(t, catalog, "protocol.contract_bundle", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.contract_verification", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.discovery_manifest", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.openapi", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.runtime_status", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.config_status", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.readiness", true, false, true)
	assertRuntimeCapability(t, catalog, "protocol.admission_check", true, false, true)
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
		manifest.ContractBundleURI != "/api/contracts" ||
		manifest.OpenAPIURI != "/api/openapi.json" ||
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
	if !hasDiscoveryProtocol(manifest, "protocol.discovery_manifest") || !hasDiscoveryProtocol(manifest, "protocol.contract_bundle") || !hasDiscoveryProtocol(manifest, "protocol.contract_verification") || !hasDiscoveryProtocol(manifest, "protocol.openapi") || !hasDiscoveryProtocol(manifest, "protocol.mcp_stdio") || !hasDiscoveryProtocol(manifest, "protocol.runtime_status") || !hasDiscoveryProtocol(manifest, "protocol.config_status") || !hasDiscoveryProtocol(manifest, "protocol.readiness") || !hasDiscoveryProtocol(manifest, "protocol.admission_check") || !hasDiscoveryProtocol(manifest, "protocol.workload_event_feed") {
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
	for _, id := range []string{"discovery", "contract-bundle", "openapi", "capability-catalog", "runtime-status", "admission-check", "canonical-event-schema", "adapter-contract"} {
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
		meta["canonical_schema_hash"] == "" || meta["adapter_spec_hash"] == "" {
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
	for _, needle := range []string{"Idempotency-Key", "WorkloadCreateRequest", "WorkloadCloseRequest", "WorkloadLinkRequest", "WorkloadLeaseAcquireRequest", "WorkloadLeaseRenewRequest", "WorkloadLeaseReleaseRequest", "AgentRunStartRequest", "AgentRunHeartbeatRequest", "DashboardBundle", "SessionPage", "PricingStatus", "BudgetStatusResponse", "QuotaStatus", "DataQualityReport", "DoctorReport", "ModelRegistryRows", "CostIntelligenceRows", "CacheDoctorRows", "InsightEventRows", "EvidenceBundle", "PolicyEvaluationRequest", "PolicyApprovalVoteRequest", "OTelGenAIRequest", "OTLPTraceRequest", "A2ATaskRequest", "ProviderUsageRequest", "GatewayRequest", `"409"`} {
		if !strings.Contains(string(rawSpec), needle) {
			t.Fatalf("OpenAPI missing %q: %s", needle, string(rawSpec))
		}
	}
	raw := hashJSONPayload(spec)
	if !strings.HasPrefix(raw, "sha256:") {
		t.Fatalf("OpenAPI hash missing: %q", raw)
	}
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
	for _, name := range []string{"discovery.contract_bundle_uri", "bundle.document.openapi", "openapi.path./api/contracts/verify", "openapi.privacy", "openapi.auth_scheme", "openapi.operation_auth", "openapi.operation_ids", "openapi.operation_admission", "openapi.operation_methods", "openapi.request_body_limits", "openapi.get_revalidation"} {
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
