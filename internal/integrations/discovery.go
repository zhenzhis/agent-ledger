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
	Product               string              `json:"product"`
	Slug                  string              `json:"slug"`
	Contract              string              `json:"contract"`
	Version               string              `json:"version"`
	PrivacyDefault        string              `json:"privacy_default"`
	LocalFirst            bool                `json:"local_first"`
	PromptContentStored   bool                `json:"prompt_content_stored"`
	UsageDataUploaded     bool                `json:"usage_data_uploaded"`
	APIBasePath           string              `json:"api_base_path"`
	WellKnownURI          string              `json:"well_known_uri"`
	CapabilityCatalogURI  string              `json:"capability_catalog_uri"`
	RuntimeStatusURI      string              `json:"runtime_status_uri"`
	CanonicalSchemaURI    string              `json:"canonical_schema_uri"`
	CanonicalSchemaHash   string              `json:"canonical_schema_hash"`
	EventExamplesURI      string              `json:"event_examples_uri"`
	AdapterSpecURI        string              `json:"adapter_spec_uri"`
	AdapterConformanceURI string              `json:"adapter_conformance_uri"`
	MCPCommand            string              `json:"mcp_command"`
	Auth                  string              `json:"auth"`
	ReadOnly              bool                `json:"read_only"`
	Summary               Summary             `json:"summary"`
	Protocols             []DiscoveryProtocol `json:"protocols"`
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
		Product:               catalog.Product,
		Slug:                  "agent-ledger",
		Contract:              "agent-ledger.discovery",
		Version:               "v1",
		PrivacyDefault:        catalog.PrivacyDefault,
		LocalFirst:            true,
		PromptContentStored:   false,
		UsageDataUploaded:     false,
		APIBasePath:           "/api",
		WellKnownURI:          "/.well-known/agent-ledger.json",
		CapabilityCatalogURI:  "/api/integrations",
		RuntimeStatusURI:      "/api/runtime/status",
		CanonicalSchemaURI:    "/api/event-schema",
		CanonicalSchemaHash:   storage.CanonicalEventSchemaFingerprint(),
		EventExamplesURI:      "/api/event-examples",
		AdapterSpecURI:        "/api/integrations/adapter-spec",
		AdapterConformanceURI: "/api/integrations/conformance",
		MCPCommand:            "agent-ledger mcp",
		Auth:                  discoveryAuth(opts),
		ReadOnly:              opts.ReadOnly,
		Summary:               catalog.Summary,
		Protocols:             protocols,
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
