package integrations

import (
	"net/url"
	"sort"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// IntegrationDriftRequest carries expected contract hashes pinned by an
// adapter, wrapper, router, relay, or CI job. Expected values are metadata-only
// sha256 strings and never include local paths, prompts, responses, or secrets.
type IntegrationDriftRequest struct {
	Strict   bool              `json:"strict"`
	Expected map[string]string `json:"expected,omitempty"`
}

// IntegrationDriftReport compares caller-pinned integration contract hashes
// with the current Agent Ledger control-plane witnesses.
type IntegrationDriftReport struct {
	Product             string                  `json:"product"`
	Contract            string                  `json:"contract"`
	Version             string                  `json:"version"`
	LocalFirst          bool                    `json:"local_first"`
	ReadOnlySafe        bool                    `json:"read_only_safe"`
	WritesLocalState    bool                    `json:"writes_local_state"`
	PrivacyPolicy       string                  `json:"privacy_policy"`
	Request             IntegrationDriftRequest `json:"request"`
	DriftHash           string                  `json:"drift_hash"`
	Current             map[string]string       `json:"current"`
	Summary             IntegrationDriftSummary `json:"summary"`
	Rows                []IntegrationDriftRow   `json:"rows"`
	CICommands          []string                `json:"ci_commands"`
	OperationalGuidance []string                `json:"operational_guidance"`
	RedactionRules      []string                `json:"redaction_rules"`
}

// IntegrationDriftSummary captures stable counts for CI gates.
type IntegrationDriftSummary struct {
	Status          string `json:"status"`
	KnownHashes     int    `json:"known_hashes"`
	ExpectedHashes  int    `json:"expected_hashes"`
	Checked         int    `json:"checked"`
	Matched         int    `json:"matched"`
	Drifted         int    `json:"drifted"`
	MissingExpected int    `json:"missing_expected"`
	UnknownExpected int    `json:"unknown_expected"`
	Warnings        int    `json:"warnings"`
}

// IntegrationDriftRow is one hash comparison.
type IntegrationDriftRow struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Expected string `json:"expected,omitempty"`
	Current  string `json:"current,omitempty"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Action   string `json:"action"`
	Privacy  string `json:"privacy"`
}

type integrationDriftHashDef struct {
	ID     string
	Title  string
	Action string
}

var integrationDriftHashDefs = []integrationDriftHashDef{
	{"capability_catalog_hash", "Capability catalog", "rerun agent-ledger integrations and review capability/runtime metadata"},
	{"provider_profiles_hash", "Provider profile catalog", "review provider/runtime profile changes and pricing override requirements"},
	{"agent_profiles_hash", "Agent framework profile catalog", "review supported agent surfaces, required signals, and collector mappings"},
	{"signal_taxonomy_hash", "Signal taxonomy", "review signal semantics, privacy class, and required field changes"},
	{"signal_coverage_hash", "Signal coverage", "rerun agent-ledger signal-coverage and adapter conformance checks"},
	{"integration_readiness_hash", "Integration readiness", "rerun agent-ledger integrations readiness and inspect runtime gates"},
	{"integration_smoke_hash", "Integration smoke", "rerun agent-ledger integrations smoke before enabling write ingest"},
	{"integration_compatibility_hash", "Compatibility matrix", "rerun agent-ledger integrations compatibility for the target agent/provider/surface"},
	{"integration_rollout_plan_hash", "Rollout plan", "rerun agent-ledger integrations rollout-plan and review release gates"},
	{"integration_evidence_kit_hash", "Evidence kit", "rerun agent-ledger integrations evidence-kit and attach updated release evidence"},
	{"integration_recommendation_hash", "Recommendation advisor", "rerun agent-ledger agent recommend and review ingest path guidance"},
	{"conformance_matrix_hash", "Adapter conformance matrix", "rerun agent-ledger adapter matrix and strict fixture CI"},
	{"adapter_spec_hash", "Adapter contract", "rerun agent-ledger adapter spec and update wrapper contract bindings"},
	{"canonical_schema_hash", "Canonical event schema", "rerun agent-ledger event schema and update canonical event validators"},
	{"schema_evolution_gate_hash", "Schema evolution gate", "rerun agent-ledger schema-gate and review adapter migration compatibility"},
	{"openapi_smoke_hash", "OpenAPI smoke witness", "rerun agent-ledger openapi and verify operation metadata"},
	{"runtime_status_hash", "Runtime status", "review read-only/write/runtime feature flags for the local deployment"},
}

// IntegrationDriftHashIDs returns stable hash ids accepted by the drift API,
// CLI, and MCP tool.
func IntegrationDriftHashIDs() []string {
	out := make([]string, 0, len(integrationDriftHashDefs))
	for _, def := range integrationDriftHashDefs {
		out = append(out, def.ID)
	}
	return out
}

func IntegrationDriftFromValues(values url.Values) IntegrationDriftRequest {
	req := IntegrationDriftRequest{
		Strict:   parseIntegrationDriftBool(values.Get("strict")),
		Expected: map[string]string{},
	}
	for _, def := range integrationDriftHashDefs {
		for _, key := range []string{def.ID, strings.ReplaceAll(def.ID, "_", "-")} {
			if value := strings.TrimSpace(values.Get(key)); value != "" {
				req.Expected[def.ID] = value
				break
			}
		}
	}
	for _, raw := range append(values["hash"], values["expected"]...) {
		key, value, ok := strings.Cut(raw, "=")
		if ok {
			req.Expected[normalizeIntegrationDriftHashID(key)] = strings.TrimSpace(value)
		}
	}
	return NormalizeIntegrationDriftRequest(req)
}

func NormalizeIntegrationDriftRequest(req IntegrationDriftRequest) IntegrationDriftRequest {
	expected := map[string]string{}
	for key, value := range req.Expected {
		key = normalizeIntegrationDriftHashID(key)
		value = strings.TrimSpace(value)
		if key != "" && value != "" {
			expected[key] = value
		}
	}
	req.Expected = expected
	return req
}

// IntegrationDriftReportFor returns a deterministic drift report without
// reading SQLite usage rows, local session paths, fixture bodies, or network
// resources.
func IntegrationDriftReportFor(opts Options, runtime *storage.RuntimeStatus, req IntegrationDriftRequest) IntegrationDriftReport {
	req = NormalizeIntegrationDriftRequest(req)
	current := IntegrationDriftCurrentHashes(opts, runtime)
	rows := integrationDriftRows(current, req)
	report := IntegrationDriftReport{
		Product:          "Agent Ledger",
		Contract:         "agent-ledger.integration-drift",
		Version:          "v1",
		LocalFirst:       true,
		ReadOnlySafe:     true,
		WritesLocalState: false,
		PrivacyPolicy:    "Integration drift reports compare caller-pinned metadata hashes with current static contract witnesses only. They exclude SQLite usage rows, fixture bodies, prompt content, response content, message history, local paths, secrets, account identifiers, machine names, authors, native session identifiers, and webhook URLs.",
		Request:          req,
		Current:          current,
		Rows:             rows,
		CICommands: []string{
			"agent-ledger discovery",
			"agent-ledger contracts verify",
			"agent-ledger integrations evidence-kit",
			"agent-ledger integrations drift --strict",
		},
		OperationalGuidance: []string{
			"store the current hash map as an adapter lockfile or CI artifact after a successful release",
			"run this report before upgrading Agent Ledger, enabling a new adapter surface, or changing provider/runtime profiles",
			"treat strict drift failures as release blockers until conformance, pricing, policy, and rollout evidence is refreshed",
			"missing expected hashes are warnings in non-strict mode and failures in strict mode",
		},
		RedactionRules: []string{
			"only sha256 hashes, contract ids, endpoint names, and remediation commands should be attached to drift tickets",
			"do not attach prompts, responses, transcripts, raw headers, credentials, webhook URLs, local paths, account names, machine names, authors, native session ids, or provider account ids",
			"use the evidence kit when a drift row needs human review context",
		},
	}
	report.Summary = summarizeIntegrationDrift(report)
	report.DriftHash = IntegrationDriftFingerprintFrom(report)
	return report
}

func IntegrationDriftFingerprint(opts Options, runtime *storage.RuntimeStatus, req IntegrationDriftRequest) string {
	return IntegrationDriftFingerprintFrom(IntegrationDriftReportFor(opts, runtime, req))
}

// IntegrationDriftOpenAPIFingerprint is a non-recursive witness hash for
// OpenAPI metadata. It avoids building the full drift report, which itself
// carries hashes for evidence, rollout, smoke, compatibility, and OpenAPI smoke
// contracts.
func IntegrationDriftOpenAPIFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	return hashJSONPayload(map[string]interface{}{
		"contract":                "agent-ledger.integration-drift",
		"version":                 "v1",
		"default_uri":             "/api/integrations/drift",
		"accepted_hash_ids":       IntegrationDriftHashIDs(),
		"capability_catalog_hash": CatalogFingerprint(opts),
		"adapter_spec_hash":       AdapterContractFingerprint(),
		"canonical_schema_hash":   storage.CanonicalEventSchemaFingerprint(),
		"runtime_status_hash":     hashJSONPayload(runtime),
		"privacy":                 "metadata-only OpenAPI witness; full endpoint ETag is returned by /api/integrations/drift",
	})
}

func IntegrationDriftFingerprintFrom(report IntegrationDriftReport) string {
	report.DriftHash = ""
	return hashJSONPayload(report)
}

func IntegrationDriftCurrentHashes(opts Options, runtime *storage.RuntimeStatus) map[string]string {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	return map[string]string{
		"capability_catalog_hash":         CatalogFingerprint(opts),
		"provider_profiles_hash":          ProviderProfilesFingerprint(),
		"agent_profiles_hash":             AgentFrameworkProfilesFingerprint(),
		"signal_taxonomy_hash":            SignalTaxonomyFingerprint(),
		"signal_coverage_hash":            SignalCoverageFingerprint(),
		"integration_readiness_hash":      IntegrationReadinessFingerprint(opts),
		"integration_smoke_hash":          IntegrationSmokeFingerprint(opts, runtime),
		"integration_compatibility_hash":  integrationDriftCompatibilityWitnessFingerprint(),
		"integration_rollout_plan_hash":   integrationDriftRolloutPlanWitnessFingerprint(),
		"integration_evidence_kit_hash":   IntegrationEvidenceKitOpenAPIFingerprint(opts, runtime),
		"integration_recommendation_hash": IntegrationRecommendationContractFingerprint(),
		"conformance_matrix_hash":         AdapterConformanceMatrixFingerprint(),
		"adapter_spec_hash":               AdapterContractFingerprint(),
		"canonical_schema_hash":           storage.CanonicalEventSchemaFingerprint(),
		"schema_evolution_gate_hash":      SchemaEvolutionGateOpenAPIFingerprint(),
		"openapi_smoke_hash":              OpenAPISmokeFingerprint(opts, runtime),
		"runtime_status_hash":             hashJSONPayload(runtime),
	}
}

func integrationDriftCompatibilityWitnessFingerprint() string {
	return hashJSONPayload(map[string]interface{}{
		"contract":                "agent-ledger.integration-compatibility",
		"version":                 "v1",
		"default_uri":             "/api/integrations/compatibility",
		"agent_profiles_hash":     AgentFrameworkProfilesFingerprint(),
		"provider_profiles_hash":  ProviderProfilesFingerprint(),
		"recommendation_hash":     IntegrationRecommendationContractFingerprint(),
		"conformance_matrix_hash": AdapterConformanceMatrixFingerprint(),
		"canonical_schema_hash":   storage.CanonicalEventSchemaFingerprint(),
		"privacy":                 "metadata-only drift witness; full endpoint ETag is returned by /api/integrations/compatibility",
	})
}

func integrationDriftRolloutPlanWitnessFingerprint() string {
	return hashJSONPayload(map[string]interface{}{
		"contract":                       "agent-ledger.integration-rollout-plan",
		"version":                        "v1",
		"default_uri":                    "/api/integrations/rollout-plan",
		"integration_compatibility_hash": integrationDriftCompatibilityWitnessFingerprint(),
		"provider_profiles_hash":         ProviderProfilesFingerprint(),
		"agent_profiles_hash":            AgentFrameworkProfilesFingerprint(),
		"recommendation_hash":            IntegrationRecommendationContractFingerprint(),
		"conformance_matrix_hash":        AdapterConformanceMatrixFingerprint(),
		"canonical_schema_hash":          storage.CanonicalEventSchemaFingerprint(),
		"privacy":                        "metadata-only drift witness; full endpoint ETag is returned by /api/integrations/rollout-plan",
	})
}

func integrationDriftRows(current map[string]string, req IntegrationDriftRequest) []IntegrationDriftRow {
	rows := []IntegrationDriftRow{}
	seen := map[string]bool{}
	for _, def := range integrationDriftHashDefs {
		expected := req.Expected[def.ID]
		status := "missing-expected"
		severity := "info"
		action := "pin this hash after the next successful adapter release"
		if expected != "" {
			status = "match"
			severity = "info"
			action = "no action required"
			if expected != current[def.ID] {
				status = "drift"
				severity = "critical"
				action = def.Action
			}
		} else if req.Strict {
			severity = "warning"
			action = "add this expected hash to the adapter lockfile or disable strict drift gating"
		}
		rows = append(rows, IntegrationDriftRow{
			ID:       def.ID,
			Title:    def.Title,
			Expected: expected,
			Current:  current[def.ID],
			Status:   status,
			Severity: severity,
			Action:   action,
			Privacy:  "hash metadata only; prompts, responses, messages, artifact bodies, raw headers, credentials, local paths, accounts, machines, authors, native sessions, and webhook URLs are excluded",
		})
		seen[def.ID] = true
	}
	unknown := []string{}
	for key := range req.Expected {
		if !seen[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	for _, key := range unknown {
		rows = append(rows, IntegrationDriftRow{
			ID:       key,
			Title:    "Unknown expected hash",
			Expected: req.Expected[key],
			Status:   "unknown-expected",
			Severity: "warning",
			Action:   "remove the unknown key or upgrade Agent Ledger if this hash id belongs to a newer contract",
			Privacy:  "hash metadata only; unknown keys are reported without local data",
		})
	}
	return rows
}

func summarizeIntegrationDrift(report IntegrationDriftReport) IntegrationDriftSummary {
	summary := IntegrationDriftSummary{
		Status:         "baseline",
		KnownHashes:    len(report.Current),
		ExpectedHashes: len(report.Request.Expected),
	}
	for _, row := range report.Rows {
		switch row.Status {
		case "match":
			summary.Checked++
			summary.Matched++
		case "drift":
			summary.Checked++
			summary.Drifted++
			summary.Warnings++
		case "missing-expected":
			summary.MissingExpected++
			if report.Request.Strict {
				summary.Warnings++
			}
		case "unknown-expected":
			summary.UnknownExpected++
			summary.Warnings++
		}
	}
	switch {
	case summary.Drifted > 0 || (report.Request.Strict && summary.MissingExpected > 0):
		summary.Status = "drift"
	case summary.UnknownExpected > 0 || summary.MissingExpected > 0:
		summary.Status = "review"
	case summary.Checked > 0 && summary.Matched == summary.Checked:
		summary.Status = "match"
	}
	return summary
}

func normalizeIntegrationDriftHashID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	raw = strings.TrimPrefix(raw, "--")
	raw = strings.ReplaceAll(raw, "-", "_")
	return raw
}

func parseIntegrationDriftBool(raw string) bool {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "y", "strict":
		return true
	default:
		return false
	}
}
