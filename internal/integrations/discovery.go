package integrations

import "github.com/zhenzhis/agent-ledger/internal/storage"

// DiscoveryProtocol is a compact protocol entry for automatic local discovery.
type DiscoveryProtocol struct {
	ID                  string   `json:"id"`
	Name                string   `json:"name"`
	Category            string   `json:"category"`
	Protocol            string   `json:"protocol"`
	Direction           string   `json:"direction"`
	Status              string   `json:"status"`
	Maturity            string   `json:"maturity"`
	Enabled             bool     `json:"enabled"`
	WritesLocalState    bool     `json:"writes_local_state"`
	AvailableInReadOnly bool     `json:"available_in_read_only"`
	RuntimeStatus       string   `json:"runtime_status"`
	Privacy             string   `json:"privacy"`
	Endpoints           []string `json:"endpoints,omitempty"`
	Commands            []string `json:"commands,omitempty"`
	Tools               []string `json:"tools,omitempty"`
	Resources           []string `json:"resources,omitempty"`
	EventTypes          []string `json:"event_types,omitempty"`
	DataClasses         []string `json:"data_classes,omitempty"`
}

// DiscoveryManifest is the privacy-safe well-known contract for local agents and wrappers.
type DiscoveryManifest struct {
	Product                  string               `json:"product"`
	Slug                     string               `json:"slug"`
	Contract                 string               `json:"contract"`
	Version                  string               `json:"version"`
	PrivacyDefault           string               `json:"privacy_default"`
	LocalFirst               bool                 `json:"local_first"`
	PromptContentStored      bool                 `json:"prompt_content_stored"`
	UsageDataUploaded        bool                 `json:"usage_data_uploaded"`
	APIBasePath              string               `json:"api_base_path"`
	WellKnownURI             string               `json:"well_known_uri"`
	CapabilityCatalogURI     string               `json:"capability_catalog_uri"`
	CapabilityCatalogHash    string               `json:"capability_catalog_hash"`
	ProviderProfilesURI      string               `json:"provider_profiles_uri"`
	ProviderProfilesHash     string               `json:"provider_profiles_hash"`
	AgentProfilesURI         string               `json:"agent_profiles_uri"`
	AgentProfilesHash        string               `json:"agent_profiles_hash"`
	SignalTaxonomyURI        string               `json:"signal_taxonomy_uri"`
	SignalTaxonomyHash       string               `json:"signal_taxonomy_hash"`
	SignalCoverageURI        string               `json:"signal_coverage_uri"`
	SignalCoverageHash       string               `json:"signal_coverage_hash"`
	IntegrationReadinessURI  string               `json:"integration_readiness_uri"`
	IntegrationReadinessHash string               `json:"integration_readiness_hash"`
	RecommendationURI        string               `json:"integration_recommendation_uri"`
	RecommendationHash       string               `json:"integration_recommendation_hash"`
	ContractBundleURI        string               `json:"contract_bundle_uri"`
	OpenAPIURI               string               `json:"openapi_uri"`
	RuntimeStatusURI         string               `json:"runtime_status_uri"`
	CanonicalSchemaURI       string               `json:"canonical_schema_uri"`
	CanonicalSchemaHash      string               `json:"canonical_schema_hash"`
	EventExamplesURI         string               `json:"event_examples_uri"`
	AdapterSpecURI           string               `json:"adapter_spec_uri"`
	AdapterSpecHash          string               `json:"adapter_spec_hash"`
	AdapterConformanceURI    string               `json:"adapter_conformance_uri"`
	ConformanceMatrixURI     string               `json:"conformance_matrix_uri"`
	ConformanceMatrixHash    string               `json:"conformance_matrix_hash"`
	MCPCommand               string               `json:"mcp_command"`
	Auth                     string               `json:"auth"`
	ReadOnly                 bool                 `json:"read_only"`
	Summary                  Summary              `json:"summary"`
	Protocols                []DiscoveryProtocol  `json:"protocols"`
	A2A                      A2ADiscoveryMetadata `json:"a2a"`
}

// A2ADiscoveryMetadata describes the local A2A telemetry surface without
// claiming to be a full remote Agent2Agent server.
type A2ADiscoveryMetadata struct {
	Mode                       string   `json:"mode"`
	Protocol                   string   `json:"protocol"`
	FullServer                 bool     `json:"full_server"`
	Endpoint                   string   `json:"endpoint"`
	HTTPMethods                []string `json:"http_methods"`
	RequiredRole               string   `json:"required_role"`
	AvailableInReadOnly        bool     `json:"available_in_read_only"`
	MaxBodyBytes               int64    `json:"max_body_bytes"`
	AdapterSpecURI             string   `json:"adapter_spec_uri"`
	AdapterSpecHash            string   `json:"adapter_spec_hash"`
	ConformanceURI             string   `json:"conformance_uri"`
	ConformanceKind            string   `json:"conformance_kind"`
	StrictFixture              string   `json:"strict_fixture"`
	SupportedTaskShapes        []string `json:"supported_task_shapes"`
	CanonicalEventTypes        []string `json:"canonical_event_types"`
	SupportsDelegatedLineage   bool     `json:"supports_delegated_lineage"`
	SupportsEvidenceReferences bool     `json:"supports_evidence_references"`
	SupportsParentPlaceholders bool     `json:"supports_parent_placeholders"`
	MessageContentStored       bool     `json:"message_content_stored"`
	ArtifactPartContentStored  bool     `json:"artifact_part_content_stored"`
	PromptContentStored        bool     `json:"prompt_content_stored"`
	Privacy                    string   `json:"privacy"`
	Limitations                []string `json:"limitations"`
}

// Discovery returns a compact local discovery document for agent ecosystems.
func Discovery(opts Options) DiscoveryManifest {
	catalog := Registry(opts)
	protocols := make([]DiscoveryProtocol, 0, len(catalog.Capabilities))
	for _, cap := range catalog.Capabilities {
		if cap.Status == "planned" {
			continue
		}
		protocols = append(protocols, DiscoveryProtocol{
			ID:                  cap.ID,
			Name:                cap.Name,
			Category:            cap.Category,
			Protocol:            cap.Protocol,
			Direction:           cap.Direction,
			Status:              cap.Status,
			Maturity:            cap.Maturity,
			Enabled:             cap.Enabled,
			WritesLocalState:    cap.WritesLocalState,
			AvailableInReadOnly: cap.AvailableInReadOnly,
			RuntimeStatus:       cap.RuntimeStatus,
			Privacy:             cap.Privacy,
			Endpoints:           cap.Endpoints,
			Commands:            cap.Commands,
			Tools:               cap.Tools,
			Resources:           cap.Resources,
			EventTypes:          cap.EventTypes,
			DataClasses:         cap.DataClasses,
		})
	}
	return DiscoveryManifest{
		Product:                  catalog.Product,
		Slug:                     "agent-ledger",
		Contract:                 "agent-ledger.discovery",
		Version:                  "v1",
		PrivacyDefault:           catalog.PrivacyDefault,
		LocalFirst:               true,
		PromptContentStored:      false,
		UsageDataUploaded:        false,
		APIBasePath:              "/api",
		WellKnownURI:             "/.well-known/agent-ledger.json",
		CapabilityCatalogURI:     "/api/integrations",
		CapabilityCatalogHash:    CatalogFingerprintFrom(catalog),
		ProviderProfilesURI:      "/api/provider-profiles",
		ProviderProfilesHash:     ProviderProfilesFingerprint(),
		AgentProfilesURI:         "/api/agent-profiles",
		AgentProfilesHash:        AgentFrameworkProfilesFingerprint(),
		SignalTaxonomyURI:        "/api/signal-taxonomy",
		SignalTaxonomyHash:       SignalTaxonomyFingerprint(),
		SignalCoverageURI:        "/api/integrations/signal-coverage",
		SignalCoverageHash:       SignalCoverageFingerprint(),
		IntegrationReadinessURI:  "/api/integrations/readiness",
		IntegrationReadinessHash: IntegrationReadinessFingerprint(opts),
		RecommendationURI:        "/api/integrations/recommendation",
		RecommendationHash:       IntegrationRecommendationContractFingerprint(),
		ContractBundleURI:        "/api/contracts",
		OpenAPIURI:               "/api/openapi.json",
		RuntimeStatusURI:         "/api/runtime/status",
		CanonicalSchemaURI:       "/api/event-schema",
		CanonicalSchemaHash:      storage.CanonicalEventSchemaFingerprint(),
		EventExamplesURI:         "/api/event-examples",
		AdapterSpecURI:           "/api/integrations/adapter-spec",
		AdapterSpecHash:          AdapterContractFingerprint(),
		AdapterConformanceURI:    "/api/integrations/conformance",
		ConformanceMatrixURI:     "/api/integrations/conformance-matrix",
		ConformanceMatrixHash:    AdapterConformanceMatrixFingerprint(),
		MCPCommand:               "agent-ledger mcp",
		Auth:                     discoveryAuth(opts),
		ReadOnly:                 opts.ReadOnly,
		Summary:                  catalog.Summary,
		Protocols:                protocols,
		A2A:                      A2ADiscovery(),
	}
}

// A2ADiscovery returns the stable, privacy-safe discovery block for local A2A
// telemetry ingestion.
func A2ADiscovery() A2ADiscoveryMetadata {
	return A2ADiscoveryMetadata{
		Mode:                       "telemetry_adapter",
		Protocol:                   "A2A JSON task snapshots/events",
		FullServer:                 false,
		Endpoint:                   "/api/a2a/tasks",
		HTTPMethods:                []string{"POST"},
		RequiredRole:               "operator",
		AvailableInReadOnly:        false,
		MaxBodyBytes:               4 << 20,
		AdapterSpecURI:             "/api/integrations/adapter-spec",
		AdapterSpecHash:            AdapterContractFingerprint(),
		ConformanceURI:             "/api/integrations/conformance?kind=a2a&strict=true",
		ConformanceKind:            "a2a",
		StrictFixture:              "examples/adapter-fixtures/a2a-delegated-task.json",
		SupportedTaskShapes:        []string{"task", "task_array", "tasks_envelope", "events_envelope", "jsonrpc_result", "task_event"},
		CanonicalEventTypes:        []string{"workload.started", "workload.linked", "agent.run.started", "agent.run.heartbeat", "agent.run.finished", "context.ref", "artifact.created", "evaluation.recorded", "policy.decision"},
		SupportsDelegatedLineage:   true,
		SupportsEvidenceReferences: true,
		SupportsParentPlaceholders: true,
		MessageContentStored:       false,
		ArtifactPartContentStored:  false,
		PromptContentStored:        false,
		Privacy:                    "metadata-only task telemetry; message history, message parts, prompt content, response content, artifact bodies, and raw evidence refs are excluded",
		Limitations:                []string{"local telemetry adapter, not a full A2A task execution server", "parent workloads may be reconstructed with lower confidence when only parent task metadata is provided", "evidence references require ref_hash or a locally hashable stable reference"},
	}
}

func discoveryAuth(opts Options) string {
	if opts.ReadOnly {
		return "read-only mode; write operations, background scans, pricing sync, and derived writebacks are disabled"
	}
	if opts.RBACEnabled {
		return "rbac tokens configured; viewer/operator/admin roles may apply"
	}
	return "localhost access by default; optional configured auth tokens may apply"
}
