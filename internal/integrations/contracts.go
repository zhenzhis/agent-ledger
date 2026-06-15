package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// ContractDocument describes one stable control-plane document that external
// wrappers, routers, and adapter CI can pin by hash.
type ContractDocument struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Contract         string   `json:"contract"`
	Version          string   `json:"version"`
	Hash             string   `json:"hash"`
	PrimaryURI       string   `json:"primary_uri"`
	AlternateURIs    []string `json:"alternate_uris,omitempty"`
	HTTPMethods      []string `json:"http_methods,omitempty"`
	CLICommands      []string `json:"cli_commands,omitempty"`
	MCPTools         []string `json:"mcp_tools,omitempty"`
	MCPResources     []string `json:"mcp_resources,omitempty"`
	Revalidation     string   `json:"revalidation"`
	Privacy          string   `json:"privacy"`
	ReadOnlySafe     bool     `json:"read_only_safe"`
	WritesLocalState bool     `json:"writes_local_state"`
}

// ContractBundle is a privacy-safe one-shot handshake document for agent
// ecosystems. It indexes contract URIs, hashes, cache semantics, CLI commands,
// and MCP entrypoints without exposing paths, prompts, sessions, or secrets.
type ContractBundle struct {
	Product        string             `json:"product"`
	Slug           string             `json:"slug"`
	Contract       string             `json:"contract"`
	Version        string             `json:"version"`
	BundleHash     string             `json:"bundle_hash"`
	LocalFirst     bool               `json:"local_first"`
	PrivacyDefault string             `json:"privacy_default"`
	ReadOnly       bool               `json:"read_only"`
	Revalidation   string             `json:"revalidation"`
	Documents      []ContractDocument `json:"documents"`
}

type ContractVerificationReport struct {
	Contract    string                      `json:"contract"`
	Version     string                      `json:"version"`
	OK          bool                        `json:"ok"`
	Checked     int                         `json:"checked"`
	Failed      int                         `json:"failed"`
	BundleHash  string                      `json:"bundle_hash"`
	OpenAPIHash string                      `json:"openapi_hash"`
	ReadOnly    bool                        `json:"read_only"`
	Privacy     string                      `json:"privacy"`
	Checks      []ContractVerificationCheck `json:"checks"`
}

type ContractVerificationCheck struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Expected string `json:"expected,omitempty"`
	Actual   string `json:"actual,omitempty"`
}

// ContractBundleFor returns the current control-plane contract bundle.
func ContractBundleFor(opts Options, runtime *storage.RuntimeStatus) ContractBundle {
	catalog := Registry(opts)
	discovery := Discovery(opts)
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	bundle := ContractBundle{
		Product:        catalog.Product,
		Slug:           "agent-ledger",
		Contract:       "agent-ledger.contract-bundle",
		Version:        "v1",
		LocalFirst:     true,
		PrivacyDefault: catalog.PrivacyDefault,
		ReadOnly:       opts.ReadOnly,
		Revalidation:   "REST documents emit strong ETag values and honor If-None-Match with 304 Not Modified",
		Documents: []ContractDocument{
			{
				ID:               "discovery",
				Name:             "Discovery Manifest",
				Contract:         discovery.Contract,
				Version:          discovery.Version,
				Hash:             hashJSONPayload(discovery),
				PrimaryURI:       discovery.WellKnownURI,
				AlternateURIs:    []string{"/api/discovery"},
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger discovery"},
				MCPTools:         []string{"ledger.discovery"},
				MCPResources:     []string{"agent-ledger://discovery/manifest"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "entrypoint URIs, runtime flags, schema hashes, and capability summary only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "contract-bundle",
				Name:             "Contract Bundle",
				Contract:         "agent-ledger.contract-bundle",
				Version:          "v1",
				PrimaryURI:       "/api/contracts",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger contracts"},
				MCPTools:         []string{"ledger.contracts"},
				MCPResources:     []string{"agent-ledger://contracts/bundle"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "hashes, URIs, cache semantics, and read-only/write-local-state metadata only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "goal-coverage",
				Name:             "Goal Coverage",
				Contract:         "agent-ledger.goal-coverage",
				Version:          "v1",
				Hash:             GoalCoverageContractFingerprint(opts, runtime),
				PrimaryURI:       "/api/goal-coverage",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger goal coverage"},
				MCPTools:         []string{"ledger.goal_coverage"},
				MCPResources:     []string{"agent-ledger://goal/coverage"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "requirement-level evidence, hashes, endpoint names, command names, docs, tests, and external dependency notes only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "openapi",
				Name:             "Control Plane OpenAPI",
				Contract:         "agent-ledger.control-plane-openapi",
				Version:          "v1",
				Hash:             OpenAPIFingerprint(opts, runtime),
				PrimaryURI:       "/api/openapi.json",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger openapi"},
				MCPTools:         []string{"ledger.openapi"},
				MCPResources:     []string{"agent-ledger://contracts/openapi"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "OpenAPI 3.1 metadata-only REST contract for stable control-plane endpoints",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "capability-catalog",
				Name:             "Integration Capability Catalog",
				Contract:         catalog.Contract,
				Version:          catalog.Version,
				Hash:             CatalogFingerprintFrom(catalog),
				PrimaryURI:       "/api/integrations",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger integrations"},
				MCPTools:         []string{"ledger.integrations"},
				MCPResources:     []string{"agent-ledger://integrations/catalog"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "privacy-safe capability metadata without collector paths or prompt content",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "provider-profiles",
				Name:             "Provider Profile Catalog",
				Contract:         "agent-ledger.provider-profile-catalog",
				Version:          "v1",
				Hash:             ProviderProfilesFingerprint(),
				PrimaryURI:       "/api/provider-profiles",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger provider profiles"},
				MCPTools:         []string{"ledger.provider_profiles"},
				MCPResources:     []string{"agent-ledger://integrations/provider-profiles"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "static provider/runtime metadata, usage schema names, gateway route names, pricing strategy labels, and privacy notes only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "agent-profiles",
				Name:             "Agent Framework Profile Catalog",
				Contract:         "agent-ledger.agent-framework-profile-catalog",
				Version:          "v1",
				Hash:             AgentFrameworkProfilesFingerprint(),
				PrimaryURI:       "/api/agent-profiles",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger agent profiles"},
				MCPTools:         []string{"ledger.agent_profiles"},
				MCPResources:     []string{"agent-ledger://integrations/agent-profiles"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "static agent CLI, framework, wrapper, router, protocol, and observability profile metadata only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "signal-taxonomy",
				Name:             "Signal Taxonomy Catalog",
				Contract:         "agent-ledger.signal-taxonomy",
				Version:          "v1",
				Hash:             SignalTaxonomyFingerprint(),
				PrimaryURI:       "/api/signal-taxonomy",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger signals"},
				MCPTools:         []string{"ledger.signal_taxonomy"},
				MCPResources:     []string{"agent-ledger://integrations/signal-taxonomy"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "static signal ids, canonical event families, field names, precision labels, privacy classes, and quality gates only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "integration-recommendation",
				Name:             "Integration Recommendation Advisor",
				Contract:         "agent-ledger.integration-recommendation",
				Version:          "v1",
				Hash:             IntegrationRecommendationContractFingerprint(),
				PrimaryURI:       "/api/integrations/recommendation",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger agent recommend"},
				MCPTools:         []string{"ledger.integration_recommendation"},
				MCPResources:     []string{"agent-ledger://integrations/recommendation"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "static recommendation metadata, request parameter echo, profile ids, validation commands, quality gates, and contract hashes only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "runtime-status",
				Name:             "Runtime Status",
				Contract:         runtime.Contract,
				Version:          runtime.Version,
				Hash:             hashJSONPayload(runtime),
				PrimaryURI:       "/api/runtime/status",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger runtime"},
				MCPTools:         []string{"ledger.runtime_status"},
				MCPResources:     []string{"agent-ledger://runtime/status"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "process mode, read-only state, disabled features, and compatibility hashes only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "admission-check",
				Name:             "Operation Admission Check",
				Contract:         "agent-ledger.admission-check",
				Version:          "v1",
				Hash:             admissionCheckContractHash(),
				PrimaryURI:       "/api/admission/check",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger admission check"},
				MCPTools:         []string{"ledger.admission_check"},
				MCPResources:     []string{"agent-ledger://admission/check"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "operation metadata, role requirements, read-only behavior, and remediation hints only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "canonical-event-schema",
				Name:             "Canonical Event Schema",
				Contract:         "agent-ledger.canonical-event-schema",
				Version:          storage.CanonicalEventSchemaVersion,
				Hash:             storage.CanonicalEventSchemaFingerprint(),
				PrimaryURI:       "/api/event-schema",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger event schema"},
				MCPTools:         []string{"ledger.event_schema"},
				MCPResources:     []string{"agent-ledger://schema/canonical-events"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "schema metadata, rejected payload keys, and event type definitions only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "adapter-contract",
				Name:             "Adapter Contract",
				Contract:         "agent-ledger.adapter-contract",
				Version:          "v1",
				Hash:             AdapterContractFingerprint(),
				PrimaryURI:       "/api/integrations/adapter-spec",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger adapter spec"},
				MCPTools:         []string{"ledger.adapter_contract"},
				MCPResources:     []string{"agent-ledger://integrations/adapter-contract"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "adapter input kinds, forbidden payload keys, token semantics, quality gates, and entrypoints only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "adapter-conformance-matrix",
				Name:             "Adapter Conformance Matrix",
				Contract:         "agent-ledger.adapter-conformance-matrix",
				Version:          "v1",
				Hash:             AdapterConformanceMatrixFingerprint(),
				PrimaryURI:       "/api/integrations/conformance-matrix",
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger adapter matrix"},
				MCPTools:         []string{"ledger.conformance_matrix"},
				MCPResources:     []string{"agent-ledger://integrations/conformance-matrix"},
				Revalidation:     "ETag + If-None-Match",
				Privacy:          "adapter fixture paths, validation commands, expected metadata event families, and privacy notes only",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
			{
				ID:               "a2a-discovery",
				Name:             "A2A Telemetry Discovery",
				Contract:         "agent-ledger.a2a-discovery",
				Version:          "v1",
				Hash:             hashJSONPayload(discovery.A2A),
				PrimaryURI:       "/.well-known/agent-ledger.json#a2a",
				AlternateURIs:    []string{"/api/discovery#a2a"},
				HTTPMethods:      []string{"GET"},
				CLICommands:      []string{"agent-ledger discovery"},
				MCPTools:         []string{"ledger.discovery"},
				MCPResources:     []string{"agent-ledger://discovery/manifest"},
				Revalidation:     "ETag + If-None-Match on the discovery document",
				Privacy:          "A2A endpoint, role, conformance, capability, and metadata-only privacy flags",
				ReadOnlySafe:     true,
				WritesLocalState: false,
			},
		},
	}
	bundle.BundleHash = ContractBundleFingerprint(bundle)
	for i := range bundle.Documents {
		if bundle.Documents[i].ID == "contract-bundle" {
			bundle.Documents[i].Hash = bundle.BundleHash
			break
		}
	}
	return bundle
}

func ContractVerificationReportFor(opts Options, runtime *storage.RuntimeStatus) ContractVerificationReport {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	catalog := Registry(opts)
	catalogHash := CatalogFingerprintFrom(catalog)
	discovery := Discovery(opts)
	adapter := AdapterContractSpec()
	openAPI := OpenAPISpecFor(opts, runtime)
	openAPIHash := OpenAPIFingerprint(opts, runtime)
	bundle := ContractBundleFor(opts, runtime)
	checks := []ContractVerificationCheck{}
	addCheck := func(name string, ok bool, severity, message, expected, actual string) {
		checks = append(checks, ContractVerificationCheck{
			Name:     name,
			OK:       ok,
			Severity: severity,
			Message:  message,
			Expected: expected,
			Actual:   actual,
		})
	}

	addCheck("discovery.contract_bundle_uri", discovery.ContractBundleURI == "/api/contracts", "critical", "discovery points to contract bundle", "/api/contracts", discovery.ContractBundleURI)
	addCheck("discovery.openapi_uri", discovery.OpenAPIURI == "/api/openapi.json", "critical", "discovery points to OpenAPI contract", "/api/openapi.json", discovery.OpenAPIURI)
	addCheck("discovery.catalog_hash", discovery.CapabilityCatalogHash == catalogHash, "critical", "discovery catalog hash matches generated catalog", catalogHash, discovery.CapabilityCatalogHash)
	addCheck("discovery.provider_profiles", discovery.ProviderProfilesURI == "/api/provider-profiles" && discovery.ProviderProfilesHash == ProviderProfilesFingerprint(), "critical", "discovery points to provider profile catalog", "/api/provider-profiles "+ProviderProfilesFingerprint(), discovery.ProviderProfilesURI+" "+discovery.ProviderProfilesHash)
	addCheck("discovery.agent_profiles", discovery.AgentProfilesURI == "/api/agent-profiles" && discovery.AgentProfilesHash == AgentFrameworkProfilesFingerprint(), "critical", "discovery points to agent framework profile catalog", "/api/agent-profiles "+AgentFrameworkProfilesFingerprint(), discovery.AgentProfilesURI+" "+discovery.AgentProfilesHash)
	addCheck("discovery.signal_taxonomy", discovery.SignalTaxonomyURI == "/api/signal-taxonomy" && discovery.SignalTaxonomyHash == SignalTaxonomyFingerprint(), "critical", "discovery points to signal taxonomy catalog", "/api/signal-taxonomy "+SignalTaxonomyFingerprint(), discovery.SignalTaxonomyURI+" "+discovery.SignalTaxonomyHash)
	addCheck("discovery.integration_recommendation", discovery.RecommendationURI == "/api/integrations/recommendation" && discovery.RecommendationHash == IntegrationRecommendationContractFingerprint(), "critical", "discovery points to integration recommendation advisor", "/api/integrations/recommendation "+IntegrationRecommendationContractFingerprint(), discovery.RecommendationURI+" "+discovery.RecommendationHash)
	addCheck("discovery.conformance_matrix", discovery.ConformanceMatrixURI == "/api/integrations/conformance-matrix" && discovery.ConformanceMatrixHash == AdapterConformanceMatrixFingerprint(), "critical", "discovery points to adapter conformance matrix", "/api/integrations/conformance-matrix "+AdapterConformanceMatrixFingerprint(), discovery.ConformanceMatrixURI+" "+discovery.ConformanceMatrixHash)
	addCheck("discovery.schema_hash", discovery.CanonicalSchemaHash == storage.CanonicalEventSchemaFingerprint(), "critical", "discovery canonical schema hash matches generated schema", storage.CanonicalEventSchemaFingerprint(), discovery.CanonicalSchemaHash)
	addCheck("discovery.adapter_hash", discovery.AdapterSpecHash == AdapterContractFingerprint(), "critical", "discovery adapter hash matches generated adapter contract", AdapterContractFingerprint(), discovery.AdapterSpecHash)
	addCheck("discovery.a2a_metadata", discovery.A2A.Endpoint == "/api/a2a/tasks" && discovery.A2A.ConformanceKind == "a2a" && !discovery.A2A.FullServer && !discovery.A2A.MessageContentStored && !discovery.A2A.PromptContentStored, "critical", "discovery exposes privacy-safe A2A telemetry metadata without claiming full server behavior", "endpoint=/api/a2a/tasks,kind=a2a,full_server=false,prompt/message_content_stored=false", a2aDiscoveryCheckSummary(discovery.A2A))
	examplesOK, examplesActual := contractCanonicalEventExamplesStatus()
	addCheck("canonical.examples", examplesOK, "critical", "canonical event examples cover every event type and validate without warnings", "one valid metadata-only example per canonical event type", examplesActual)
	adapterSchemaOK, adapterSchemaActual := contractAdapterSchemaStatus(adapter)
	addCheck("adapter.schema_alignment", adapterSchemaOK, "critical", "adapter contract matches canonical schema version, hash, event types, and privacy keys", "adapter schema/version/event_types/forbidden_keys match canonical schema", adapterSchemaActual)
	adapterKindsOK, adapterKindsActual := contractAdapterInputKindStatus(adapter)
	addCheck("adapter.input_kinds", adapterKindsOK, "critical", "adapter contract supported input kinds match conformance decoder families", "adapter input kinds cover all supported conformance kinds with signals and privacy notes", adapterKindsActual)
	addCheck("adapter.provider_profiles", adapter.ProviderProfilesURI == "/api/provider-profiles" && adapter.ProviderProfilesHash == ProviderProfilesFingerprint(), "critical", "adapter contract links provider profile catalog", "/api/provider-profiles "+ProviderProfilesFingerprint(), adapter.ProviderProfilesURI+" "+adapter.ProviderProfilesHash)
	matrix := AdapterConformanceMatrixSpec()
	addCheck("adapter.conformance_matrix", matrix.AdapterSpecHash == AdapterContractFingerprint() && matrix.ProviderProfilesHash == ProviderProfilesFingerprint() && len(matrix.Kinds) == len(SupportedAdapterConformanceKinds()), "critical", "adapter conformance matrix links schema, provider profiles, and every decoder kind", AdapterContractFingerprint()+" "+ProviderProfilesFingerprint()+" kinds="+strconv.Itoa(len(SupportedAdapterConformanceKinds())), matrix.AdapterSpecHash+" "+matrix.ProviderProfilesHash+" kinds="+strconv.Itoa(len(matrix.Kinds)))
	privacyLanguageOK, privacyLanguageActual := contractPublicPrivacyLanguageStatus(map[string]interface{}{
		"adapter_contract":           adapter,
		"adapter_conformance_matrix": matrix,
		"agent_profiles":             AgentFrameworkProfiles(),
		"a2a_discovery":              discovery.A2A,
		"capability_catalog":         catalog,
		"contract_bundle":            bundle,
		"discovery":                  discovery,
		"goal_coverage":              GoalCoverageReportFor(opts, runtime),
		"integration_recommendation": IntegrationRecommendation(IntegrationRecommendationRequest{
			AgentProfileID:    "codex-cli",
			ProviderProfileID: "openai-official",
			Surface:           "provider-stream",
			Signals:           []string{"model", "usage", "cache"},
		}),
		"openapi":           openAPI,
		"provider_profiles": ProviderProfiles(),
		"signal_taxonomy":   SignalTaxonomy(),
	})
	addCheck("privacy.public_metadata_language", privacyLanguageOK, "critical", "public metadata uses content-safe privacy language", "no unsafe content-capture phrases in public metadata documents", privacyLanguageActual)
	addCheck("bundle.hash", bundle.BundleHash == ContractBundleFingerprint(bundle), "critical", "contract bundle hash matches deterministic fingerprint", ContractBundleFingerprint(bundle), bundle.BundleHash)

	requiredDocs := []struct {
		id   string
		hash string
	}{
		{id: "discovery", hash: hashJSONPayload(discovery)},
		{id: "contract-bundle", hash: bundle.BundleHash},
		{id: "goal-coverage", hash: GoalCoverageContractFingerprint(opts, runtime)},
		{id: "openapi", hash: openAPIHash},
		{id: "capability-catalog", hash: catalogHash},
		{id: "provider-profiles", hash: ProviderProfilesFingerprint()},
		{id: "agent-profiles", hash: AgentFrameworkProfilesFingerprint()},
		{id: "signal-taxonomy", hash: SignalTaxonomyFingerprint()},
		{id: "integration-recommendation", hash: IntegrationRecommendationContractFingerprint()},
		{id: "adapter-conformance-matrix", hash: AdapterConformanceMatrixFingerprint()},
		{id: "runtime-status", hash: hashJSONPayload(runtime)},
		{id: "admission-check", hash: admissionCheckContractHash()},
		{id: "canonical-event-schema", hash: storage.CanonicalEventSchemaFingerprint()},
		{id: "adapter-contract", hash: AdapterContractFingerprint()},
		{id: "a2a-discovery", hash: hashJSONPayload(discovery.A2A)},
	}
	for _, required := range requiredDocs {
		doc, ok := contractDocument(bundle, required.id)
		actual := ""
		if ok {
			actual = doc.Hash
		}
		addCheck("bundle.document."+required.id, ok && actual == required.hash, "critical", "bundle document is present and hash matches generated payload", required.hash, actual)
		if ok {
			addCheck("bundle.document."+required.id+".read_only", doc.ReadOnlySafe && !doc.WritesLocalState, "critical", "contract document is read-only and does not write local state", "read_only_safe=true,writes_local_state=false", contractDocAccess(doc))
		}
	}

	meta, _ := openAPI["x-agent-ledger"].(map[string]interface{})
	addCheck("openapi.identity", openAPI["openapi"] == "3.1.0" && meta["contract"] == "agent-ledger.control-plane-openapi", "critical", "OpenAPI document has expected identity", "3.1.0 agent-ledger.control-plane-openapi", openAPIIdentity(openAPI, meta))
	addCheck("openapi.privacy", meta["prompt_content_stored"] == false && meta["usage_data_uploaded"] == false, "critical", "OpenAPI metadata preserves local-first privacy flags", "prompt_content_stored=false,usage_data_uploaded=false", openAPIPrivacy(meta))
	addCheck("openapi.catalog_hash", contractStringValue(meta["capability_catalog_hash"]) == catalogHash, "critical", "OpenAPI catalog hash matches generated catalog", catalogHash, contractStringValue(meta["capability_catalog_hash"]))
	addCheck("openapi.provider_profiles_hash", contractStringValue(meta["provider_profiles_hash"]) == ProviderProfilesFingerprint(), "critical", "OpenAPI provider profile hash matches generated catalog", ProviderProfilesFingerprint(), contractStringValue(meta["provider_profiles_hash"]))
	addCheck("openapi.agent_profiles_hash", contractStringValue(meta["agent_profiles_hash"]) == AgentFrameworkProfilesFingerprint(), "critical", "OpenAPI agent framework profile hash matches generated catalog", AgentFrameworkProfilesFingerprint(), contractStringValue(meta["agent_profiles_hash"]))
	addCheck("openapi.signal_taxonomy_hash", contractStringValue(meta["signal_taxonomy_hash"]) == SignalTaxonomyFingerprint(), "critical", "OpenAPI signal taxonomy hash matches generated catalog", SignalTaxonomyFingerprint(), contractStringValue(meta["signal_taxonomy_hash"]))
	addCheck("openapi.integration_recommendation_hash", contractStringValue(meta["integration_recommendation_hash"]) == IntegrationRecommendationContractFingerprint(), "critical", "OpenAPI integration recommendation hash matches generated contract", IntegrationRecommendationContractFingerprint(), contractStringValue(meta["integration_recommendation_hash"]))
	addCheck("openapi.conformance_matrix_hash", contractStringValue(meta["conformance_matrix_hash"]) == AdapterConformanceMatrixFingerprint(), "critical", "OpenAPI conformance matrix hash matches generated matrix", AdapterConformanceMatrixFingerprint(), contractStringValue(meta["conformance_matrix_hash"]))
	addCheck("openapi.schema_hash", contractStringValue(meta["canonical_schema_hash"]) == storage.CanonicalEventSchemaFingerprint(), "critical", "OpenAPI schema hash matches generated schema", storage.CanonicalEventSchemaFingerprint(), contractStringValue(meta["canonical_schema_hash"]))
	addCheck("openapi.adapter_hash", contractStringValue(meta["adapter_spec_hash"]) == AdapterContractFingerprint(), "critical", "OpenAPI adapter hash matches generated adapter contract", AdapterContractFingerprint(), contractStringValue(meta["adapter_spec_hash"]))

	paths, _ := openAPI["paths"].(map[string]interface{})
	authSchemeOK := openAPIAuthSchemeOK(openAPI)
	addCheck("openapi.auth_scheme", authSchemeOK, "critical", "OpenAPI declares the local bearer authentication scheme", "AgentLedgerBearer type=http scheme=bearer", boolString(authSchemeOK))
	authOK, authActual := contractOpenAPIOperationAuthStatus(paths)
	addCheck("openapi.operation_auth", authOK, "critical", "OpenAPI operations declare bearer auth and 401 responses", "all operations include AgentLedgerBearer security and 401 response", authActual)
	operationIDOK, operationIDActual := contractOpenAPIOperationIDStatus(paths)
	addCheck("openapi.operation_ids", operationIDOK, "critical", "OpenAPI operations expose stable unique operationId values", "all operations include valid unique operationId", operationIDActual)
	admissionOK, admissionActual := contractOpenAPIOperationAdmissionStatus(paths)
	addCheck("openapi.operation_admission", admissionOK, "critical", "OpenAPI operations expose admission role, write-mode, and read-only metadata", "all operations include required_role, write_mode, available_in_read_only", admissionActual)
	methodOK, methodActual := contractOpenAPIOperationMethodStatus(paths)
	addCheck("openapi.operation_methods", methodOK, "critical", "OpenAPI operations expose method-not-allowed contracts", "all operations include 405 response", methodActual)
	bodyLimitOK, bodyLimitActual := contractOpenAPIRequestBodyLimitStatus(paths)
	addCheck("openapi.request_body_limits", bodyLimitOK, "critical", "OpenAPI request body operations expose body limits and 413 responses", "all requestBody operations include max_body_bytes and 413 response", bodyLimitActual)
	idempotencyOK, idempotencyActual := contractOpenAPIIdempotencyStatus(openAPI, paths)
	addCheck("openapi.idempotency", idempotencyOK, "critical", "OpenAPI idempotent writes declare retry keys and conflict behavior", "idempotent writes include Idempotency-Key, X-Idempotency-Key, idempotency_key request field, body limit, and 409 response", idempotencyActual)
	revalidationOK, revalidationActual := contractOpenAPIGetRevalidationStatus(paths)
	addCheck("openapi.get_revalidation", revalidationOK, "critical", "OpenAPI GET operations declare ETag revalidation or an explicit no-ETag reason", "all GET operations include 304 response or x-agent-ledger.etag", revalidationActual)
	for _, path := range OpenAPIContractPaths() {
		_, ok := paths[path]
		addCheck("openapi.path."+path, ok, "warning", "OpenAPI exposes stable control-plane path", path, boolString(ok))
	}

	failed := 0
	for _, check := range checks {
		if !check.OK {
			failed++
		}
	}
	return ContractVerificationReport{
		Contract:    "agent-ledger.contract-verification",
		Version:     "v1",
		OK:          failed == 0,
		Checked:     len(checks),
		Failed:      failed,
		BundleHash:  bundle.BundleHash,
		OpenAPIHash: openAPIHash,
		ReadOnly:    opts.ReadOnly,
		Privacy:     "metadata-only self-check; no prompts, responses, sessions, secrets, or local paths",
		Checks:      checks,
	}
}

func ContractVerificationFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	return hashJSONPayload(ContractVerificationReportFor(opts, runtime))
}

func ContractVerificationFingerprintFrom(report ContractVerificationReport) string {
	return hashJSONPayload(report)
}

func contractDocument(bundle ContractBundle, id string) (ContractDocument, bool) {
	for _, doc := range bundle.Documents {
		if doc.ID == id {
			return doc, true
		}
	}
	return ContractDocument{}, false
}

func contractDocAccess(doc ContractDocument) string {
	return "read_only_safe=" + boolString(doc.ReadOnlySafe) + ",writes_local_state=" + boolString(doc.WritesLocalState)
}

func a2aDiscoveryCheckSummary(meta A2ADiscoveryMetadata) string {
	return "endpoint=" + meta.Endpoint +
		",kind=" + meta.ConformanceKind +
		",full_server=" + boolString(meta.FullServer) +
		",prompt_content_stored=" + boolString(meta.PromptContentStored) +
		",message_content_stored=" + boolString(meta.MessageContentStored)
}

func admissionCheckContractHash() string {
	return hashJSONPayload(map[string]interface{}{
		"contract":    "agent-ledger.admission-check",
		"version":     "v1",
		"surfaces":    []string{"http", "cli", "mcp"},
		"entrypoints": []string{"/api/admission/check", "agent-ledger admission check", "ledger.admission_check", "agent-ledger://admission/check"},
		"privacy":     "operation metadata, role requirements, read-only behavior, and remediation hints only",
	})
}

func contractCanonicalEventExamplesStatus() (bool, string) {
	eventTypes := storage.CanonicalEventTypes()
	examples := storage.CanonicalEventExamples("")
	seen := map[string]bool{}
	invalid, duplicate := 0, 0
	for _, example := range examples {
		if seen[example.EventType] {
			duplicate++
		}
		seen[example.EventType] = true
		result, err := storage.ValidateCanonicalEvent(example.Event)
		if err != nil || result.Status != "valid" || result.EventType != example.EventType {
			invalid++
		}
	}
	missing := 0
	for _, info := range eventTypes {
		if !seen[info.EventType] {
			missing++
			continue
		}
		filtered := storage.CanonicalEventExamples(info.EventType)
		if len(filtered) != 1 || filtered[0].EventType != info.EventType {
			invalid++
		}
	}
	ok := len(eventTypes) > 0 && len(examples) == len(eventTypes) && missing == 0 && invalid == 0 && duplicate == 0
	actual := "event_types=" + intString(len(eventTypes)) +
		",examples=" + intString(len(examples)) +
		",missing=" + intString(missing) +
		",invalid=" + intString(invalid) +
		",duplicates=" + intString(duplicate)
	return ok, actual
}

func contractPublicPrivacyLanguageStatus(docs map[string]interface{}) (bool, string) {
	forbidden := []string{
		"api keys",
		"model output text",
		"output text",
		"prompt text",
		"prompt, output",
		"response text",
		"transcript text",
	}
	failures := []string{}
	for name, doc := range docs {
		raw, err := json.Marshal(doc)
		if err != nil {
			failures = append(failures, name+":marshal_error")
			continue
		}
		lower := strings.ToLower(string(raw))
		for _, phrase := range forbidden {
			if strings.Contains(lower, phrase) {
				failures = append(failures, name+":"+phrase)
			}
		}
	}
	if len(failures) == 0 {
		return true, "checked=" + intString(len(docs)) + ",failures=0"
	}
	return false, "checked=" + intString(len(docs)) + ",failures=" + intString(len(failures)) + ",first=" + strings.Join(firstContractStrings(failures, 5), "|")
}

func contractAdapterSchemaStatus(adapter AdapterContract) (bool, string) {
	eventTypesMatch := contractEventTypesMatch(adapter.CanonicalEventTypes, storage.CanonicalEventTypes())
	forbiddenKeysMatch := stringSetEqual(adapter.ForbiddenPayloadKeys, contractCanonicalRejectedPayloadKeys())
	schemaVersionMatch := adapter.SchemaVersion == storage.CanonicalEventSchemaVersion
	schemaHashMatch := adapter.SchemaHash == storage.CanonicalEventSchemaFingerprint()
	ok := schemaVersionMatch && schemaHashMatch && eventTypesMatch && forbiddenKeysMatch
	actual := "schema_version_match=" + boolString(schemaVersionMatch) +
		",schema_hash_match=" + boolString(schemaHashMatch) +
		",event_types=" + intString(len(adapter.CanonicalEventTypes)) +
		",event_types_match=" + boolString(eventTypesMatch) +
		",forbidden_keys=" + intString(len(adapter.ForbiddenPayloadKeys)) +
		",forbidden_keys_match=" + boolString(forbiddenKeysMatch)
	return ok, actual
}

func contractAdapterInputKindStatus(adapter AdapterContract) (bool, string) {
	expected := SupportedAdapterConformanceKinds()
	seen := map[string]bool{}
	incomplete, unsupported := 0, 0
	for _, kind := range adapter.SupportedInputKinds {
		if kind.Kind == "" || kind.ConformanceKind == "" || len(kind.RequiredSignals) == 0 || len(kind.PrivacyNotes) == 0 {
			incomplete++
		}
		if !stringSliceContains(expected, kind.ConformanceKind) {
			unsupported++
			continue
		}
		seen[kind.ConformanceKind] = true
	}
	missing := 0
	for _, kind := range expected {
		if !seen[kind] {
			missing++
		}
	}
	ok := missing == 0 && incomplete == 0 && unsupported == 0
	actual := "expected=" + intString(len(expected)) +
		",declared=" + intString(len(adapter.SupportedInputKinds)) +
		",missing=" + intString(missing) +
		",unsupported=" + intString(unsupported) +
		",incomplete=" + intString(incomplete)
	return ok, actual
}

func contractEventTypesMatch(got, want []storage.CanonicalEventTypeInfo) bool {
	gotNames := make([]string, 0, len(got))
	for _, item := range got {
		gotNames = append(gotNames, item.EventType)
	}
	wantNames := make([]string, 0, len(want))
	for _, item := range want {
		wantNames = append(wantNames, item.EventType)
	}
	return stringSetEqual(gotNames, wantNames)
}

func contractCanonicalRejectedPayloadKeys() []string {
	schema := storage.CanonicalEventSchema()
	privacy, ok := schema["privacy"].(map[string]interface{})
	if !ok {
		return nil
	}
	switch keys := privacy["rejected_payload_keys"].(type) {
	case []string:
		return keys
	case []interface{}:
		out := make([]string, 0, len(keys))
		for _, key := range keys {
			value, ok := key.(string)
			if ok {
				out = append(out, value)
			}
		}
		return out
	default:
		return nil
	}
}

func openAPIIdentity(openAPI map[string]interface{}, meta map[string]interface{}) string {
	return contractStringValue(openAPI["openapi"]) + " " + contractStringValue(meta["contract"])
}

func openAPIPrivacy(meta map[string]interface{}) string {
	return "prompt_content_stored=" + boolString(meta["prompt_content_stored"] == true) + ",usage_data_uploaded=" + boolString(meta["usage_data_uploaded"] == true)
}

func openAPIAuthSchemeOK(openAPI map[string]interface{}) bool {
	components, ok := openAPI["components"].(map[string]interface{})
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

func contractOpenAPIOperationAuthStatus(paths map[string]interface{}) (bool, string) {
	checked, missing := 0, 0
	for _, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok {
				continue
			}
			checked++
			if !contractOpenAPIOperationHasBearerSecurity(operation) || !contractOpenAPIOperationHasResponse(operation, "401") {
				missing++
			}
		}
	}
	return checked > 0 && missing == 0, "checked=" + intString(checked) + ",missing=" + intString(missing)
}

func contractOpenAPIOperationIDStatus(paths map[string]interface{}) (bool, string) {
	checked, missing, duplicate := 0, 0, 0
	seen := map[string]bool{}
	for _, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok {
				continue
			}
			checked++
			id, _ := operation["operationId"].(string)
			if !contractOpenAPIOperationIDValid(id) {
				missing++
				continue
			}
			if seen[id] {
				duplicate++
				continue
			}
			seen[id] = true
		}
	}
	return checked > 0 && missing == 0 && duplicate == 0, "checked=" + intString(checked) + ",missing=" + intString(missing) + ",duplicates=" + intString(duplicate)
}

func contractOpenAPIOperationAdmissionStatus(paths map[string]interface{}) (bool, string) {
	checked, missing := 0, 0
	for _, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok {
				continue
			}
			checked++
			if !contractOpenAPIOperationHasAdmissionMetadata(operation) {
				missing++
			}
		}
	}
	return checked > 0 && missing == 0, "checked=" + intString(checked) + ",missing=" + intString(missing)
}

func contractOpenAPIOperationMethodStatus(paths map[string]interface{}) (bool, string) {
	checked, missing := 0, 0
	for _, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok {
				continue
			}
			checked++
			if !contractOpenAPIOperationHasResponse(operation, "405") {
				missing++
			}
		}
	}
	return checked > 0 && missing == 0, "checked=" + intString(checked) + ",missing=" + intString(missing)
}

func contractOpenAPIRequestBodyLimitStatus(paths map[string]interface{}) (bool, string) {
	checked, missing := 0, 0
	for _, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range []string{"post", "put", "patch"} {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok {
				continue
			}
			if _, hasBody := operation["requestBody"]; !hasBody {
				continue
			}
			checked++
			if !contractOpenAPIOperationHasBodyLimit(operation) || !contractOpenAPIOperationHasResponse(operation, "413") {
				missing++
			}
		}
	}
	return checked > 0 && missing == 0, "checked=" + intString(checked) + ",missing=" + intString(missing)
}

func contractOpenAPIIdempotencyStatus(openAPI map[string]interface{}, paths map[string]interface{}) (bool, string) {
	checked, missing := 0, 0
	for _, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range []string{"post", "put", "patch"} {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok || contractOpenAPIOperationIdempotency(operation) == "" {
				continue
			}
			checked++
			if !contractOpenAPIOperationHasHeaderParam(operation, "Idempotency-Key") ||
				!contractOpenAPIOperationHasHeaderParam(operation, "X-Idempotency-Key") ||
				!contractOpenAPIOperationHasResponse(operation, "409") ||
				!contractOpenAPIOperationHasBodyLimit(operation) {
				missing++
			}
			if _, hasBody := operation["requestBody"]; !hasBody {
				missing++
			}
			if !contractOpenAPIIdempotencyRequestSchemaOK(openAPI, operation) {
				missing++
			}
		}
	}
	return checked > 0 && missing == 0, "checked=" + intString(checked) + ",missing=" + intString(missing)
}

func contractOpenAPIGetRevalidationStatus(paths map[string]interface{}) (bool, string) {
	checked, missing := 0, 0
	for _, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		operation, ok := pathItem["get"].(map[string]interface{})
		if !ok {
			continue
		}
		checked++
		if !contractOpenAPIOperationHasResponse(operation, "304") && contractOpenAPIOperationETagPolicy(operation) == "" {
			missing++
		}
	}
	return checked > 0 && missing == 0, "checked=" + intString(checked) + ",missing=" + intString(missing)
}

func contractOpenAPIOperationHasBearerSecurity(operation map[string]interface{}) bool {
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

func contractOpenAPIOperationHasResponse(operation map[string]interface{}, status string) bool {
	responses, ok := operation["responses"].(map[string]interface{})
	if !ok {
		return false
	}
	_, ok = responses[status]
	return ok
}

func contractOpenAPIOperationIDValid(id string) bool {
	if id == "" {
		return false
	}
	for i, ch := range id {
		switch {
		case ch >= 'a' && ch <= 'z':
		case ch >= 'A' && ch <= 'Z':
		case ch == '_':
		case ch >= '0' && ch <= '9' && i > 0:
		default:
			return false
		}
	}
	return true
}

func contractOpenAPIOperationHasAdmissionMetadata(operation map[string]interface{}) bool {
	meta, ok := operation["x-agent-ledger"].(map[string]interface{})
	if !ok {
		return false
	}
	role, roleOK := meta["required_role"].(string)
	writeMode, writeModeOK := meta["write_mode"].(string)
	_, readOnlyOK := meta["available_in_read_only"].(bool)
	return roleOK && role != "" && writeModeOK && writeMode != "" && readOnlyOK
}

func contractOpenAPIOperationHasBodyLimit(operation map[string]interface{}) bool {
	meta, ok := operation["x-agent-ledger"].(map[string]interface{})
	if !ok {
		return false
	}
	switch value := meta["max_body_bytes"].(type) {
	case int:
		return value > 0
	case int64:
		return value > 0
	case float64:
		return value > 0
	default:
		return false
	}
}

func contractOpenAPIOperationIdempotency(operation map[string]interface{}) string {
	meta, ok := operation["x-agent-ledger"].(map[string]interface{})
	if !ok {
		return ""
	}
	idempotency, _ := meta["idempotency"].(string)
	return idempotency
}

func contractOpenAPIOperationHasHeaderParam(operation map[string]interface{}, name string) bool {
	params, ok := operation["parameters"].([]map[string]interface{})
	if !ok {
		return false
	}
	for _, param := range params {
		if param["in"] == "header" && param["name"] == name {
			return true
		}
	}
	return false
}

func contractOpenAPIIdempotencyRequestSchemaOK(openAPI map[string]interface{}, operation map[string]interface{}) bool {
	schemaName := contractOpenAPIJSONRequestSchemaRef(operation)
	if schemaName == "" {
		return false
	}
	return contractOpenAPIComponentSchemaHasProperty(openAPI, schemaName, "idempotency_key")
}

func contractOpenAPIJSONRequestSchemaRef(operation map[string]interface{}) string {
	requestBody, ok := operation["requestBody"].(map[string]interface{})
	if !ok {
		return ""
	}
	content, ok := requestBody["content"].(map[string]interface{})
	if !ok {
		return ""
	}
	jsonContent, ok := content["application/json"].(map[string]interface{})
	if !ok {
		return ""
	}
	schema, ok := jsonContent["schema"].(map[string]interface{})
	if !ok {
		return ""
	}
	ref, _ := schema["$ref"].(string)
	return strings.TrimPrefix(ref, "#/components/schemas/")
}

func contractOpenAPIComponentSchemaHasProperty(openAPI map[string]interface{}, schemaName, propertyName string) bool {
	if schemaName == "" || propertyName == "" {
		return false
	}
	components, ok := openAPI["components"].(map[string]interface{})
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

func contractOpenAPIOperationETagPolicy(operation map[string]interface{}) string {
	meta, ok := operation["x-agent-ledger"].(map[string]interface{})
	if !ok {
		return ""
	}
	policy, _ := meta["etag"].(string)
	return policy
}

func contractStringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func intString(v int) string {
	return strconv.Itoa(v)
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

func firstContractStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) == 0 {
		return nil
	}
	if len(values) <= limit {
		return values
	}
	return values[:limit]
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func stringSetEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, value := range a {
		seen[value]++
	}
	for _, value := range b {
		seen[value]--
		if seen[value] < 0 {
			return false
		}
	}
	for _, count := range seen {
		if count != 0 {
			return false
		}
	}
	return true
}

func defaultRuntimeStatus(opts Options) *storage.RuntimeStatus {
	if opts.ReadOnly {
		return EnrichRuntimeStatus(&storage.RuntimeStatus{
			Mode:             "observer",
			ReadOnly:         true,
			WriteOperations:  "disabled",
			BackgroundTasks:  "disabled",
			DisabledFeatures: []string{"background collectors", "pricing sync", "cost recalculation", "manual scans", "imports", "write APIs", "write MCP tools", "derived GET writebacks"},
			Message:          "read-only observer mode: local state is not mutated by this process",
		}, opts)
	}
	return EnrichRuntimeStatus(&storage.RuntimeStatus{
		Mode:            "control-plane",
		ReadOnly:        false,
		WriteOperations: "enabled",
		BackgroundTasks: "enabled",
		Message:         "write operations and background collectors are enabled",
	}, opts)
}

func ContractBundleFingerprint(bundle ContractBundle) string {
	bundle.Documents = append([]ContractDocument(nil), bundle.Documents...)
	bundle.BundleHash = ""
	for i := range bundle.Documents {
		if bundle.Documents[i].ID == "contract-bundle" {
			bundle.Documents[i].Hash = ""
		}
	}
	return hashJSONPayload(bundle)
}

func hashJSONPayload(v interface{}) string {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
