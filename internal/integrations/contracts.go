package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

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
	addCheck("discovery.schema_hash", discovery.CanonicalSchemaHash == storage.CanonicalEventSchemaFingerprint(), "critical", "discovery canonical schema hash matches generated schema", storage.CanonicalEventSchemaFingerprint(), discovery.CanonicalSchemaHash)
	addCheck("discovery.adapter_hash", discovery.AdapterSpecHash == AdapterContractFingerprint(), "critical", "discovery adapter hash matches generated adapter contract", AdapterContractFingerprint(), discovery.AdapterSpecHash)
	addCheck("bundle.hash", bundle.BundleHash == ContractBundleFingerprint(bundle), "critical", "contract bundle hash matches deterministic fingerprint", ContractBundleFingerprint(bundle), bundle.BundleHash)

	requiredDocs := []struct {
		id   string
		hash string
	}{
		{id: "discovery", hash: hashJSONPayload(discovery)},
		{id: "contract-bundle", hash: bundle.BundleHash},
		{id: "openapi", hash: openAPIHash},
		{id: "capability-catalog", hash: catalogHash},
		{id: "runtime-status", hash: hashJSONPayload(runtime)},
		{id: "admission-check", hash: admissionCheckContractHash()},
		{id: "canonical-event-schema", hash: storage.CanonicalEventSchemaFingerprint()},
		{id: "adapter-contract", hash: AdapterContractFingerprint()},
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
	addCheck("openapi.schema_hash", contractStringValue(meta["canonical_schema_hash"]) == storage.CanonicalEventSchemaFingerprint(), "critical", "OpenAPI schema hash matches generated schema", storage.CanonicalEventSchemaFingerprint(), contractStringValue(meta["canonical_schema_hash"]))
	addCheck("openapi.adapter_hash", contractStringValue(meta["adapter_spec_hash"]) == AdapterContractFingerprint(), "critical", "OpenAPI adapter hash matches generated adapter contract", AdapterContractFingerprint(), contractStringValue(meta["adapter_spec_hash"]))

	paths, _ := openAPI["paths"].(map[string]interface{})
	for _, path := range []string{"/api/contracts", "/api/contracts/verify", "/api/openapi.json", "/api/config/status", "/api/readiness", "/api/admission/check", "/api/event-schema", "/api/events/validate", "/api/integrations/conformance", "/api/workloads", "/api/workloads/close", "/api/workloads/link", "/api/workloads/claim-next", "/api/workloads/queue", "/api/workloads/lease", "/api/workloads/lease/renew", "/api/workloads/lease/release", "/api/workloads/leases", "/api/agent-runs", "/api/agent-runs/heartbeat", "/api/agent-runs/liveness", "/api/workload-detail", "/api/workload-graph", "/api/workload-timeline", "/api/workload-state", "/api/workload-events"} {
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

func admissionCheckContractHash() string {
	return hashJSONPayload(map[string]interface{}{
		"contract":    "agent-ledger.admission-check",
		"version":     "v1",
		"surfaces":    []string{"http", "cli", "mcp"},
		"entrypoints": []string{"/api/admission/check", "agent-ledger admission check", "ledger.admission_check", "agent-ledger://admission/check"},
		"privacy":     "operation metadata, role requirements, read-only behavior, and remediation hints only",
	})
}

func openAPIIdentity(openAPI map[string]interface{}, meta map[string]interface{}) string {
	return contractStringValue(openAPI["openapi"]) + " " + contractStringValue(meta["contract"])
}

func openAPIPrivacy(meta map[string]interface{}) string {
	return "prompt_content_stored=" + boolString(meta["prompt_content_stored"] == true) + ",usage_data_uploaded=" + boolString(meta["usage_data_uploaded"] == true)
}

func contractStringValue(v interface{}) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func boolString(v bool) string {
	if v {
		return "true"
	}
	return "false"
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
