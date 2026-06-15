package integrations

import (
	"net/url"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// IntegrationEvidenceKitRequest selects one static adapter release evidence
// target. It is metadata-only and mirrors rollout-plan filters.
type IntegrationEvidenceKitRequest struct {
	AgentProfileID    string `json:"agent_profile_id,omitempty"`
	ProviderProfileID string `json:"provider_profile_id,omitempty"`
	Surface           string `json:"surface,omitempty"`
	MinConfidence     string `json:"min_confidence,omitempty"`
}

// IntegrationEvidenceKitReport is a privacy-safe manifest that tells adapter
// authors exactly which static evidence to attach before enabling a new
// wrapper, provider, gateway, router, relay, local runtime, or edge runtime.
type IntegrationEvidenceKitReport struct {
	Product             string                         `json:"product"`
	Contract            string                         `json:"contract"`
	Version             string                         `json:"version"`
	LocalFirst          bool                           `json:"local_first"`
	ReadOnlySafe        bool                           `json:"read_only_safe"`
	WritesLocalState    bool                           `json:"writes_local_state"`
	PrivacyPolicy       string                         `json:"privacy_policy"`
	Request             IntegrationEvidenceKitRequest  `json:"request"`
	KitHash             string                         `json:"kit_hash"`
	Hashes              IntegrationEvidenceKitHashes   `json:"hashes"`
	Summary             IntegrationEvidenceKitSummary  `json:"summary"`
	Target              IntegrationRolloutTarget       `json:"target"`
	EvidenceItems       []IntegrationEvidenceItem      `json:"evidence_items"`
	FixtureEvidence     []IntegrationRolloutFixture    `json:"fixture_evidence"`
	CICommands          []string                       `json:"ci_commands"`
	ReviewerChecklist   []IntegrationEvidenceChecklist `json:"reviewer_checklist"`
	RedactionRules      []string                       `json:"redaction_rules"`
	OperationalGuidance []string                       `json:"operational_guidance"`
}

// IntegrationEvidenceKitHashes exposes the stable contract witnesses an
// adapter PR or internal release ticket should pin.
type IntegrationEvidenceKitHashes struct {
	CapabilityCatalogHash    string `json:"capability_catalog_hash"`
	ProviderProfilesHash     string `json:"provider_profiles_hash"`
	AgentProfilesHash        string `json:"agent_profiles_hash"`
	SignalTaxonomyHash       string `json:"signal_taxonomy_hash"`
	SignalCoverageHash       string `json:"signal_coverage_hash"`
	IntegrationReadinessHash string `json:"integration_readiness_hash"`
	IntegrationSmokeHash     string `json:"integration_smoke_hash"`
	CompatibilityHash        string `json:"integration_compatibility_hash"`
	RolloutPlanHash          string `json:"integration_rollout_plan_hash"`
	RecommendationHash       string `json:"integration_recommendation_hash"`
	ConformanceMatrixHash    string `json:"conformance_matrix_hash"`
	AdapterSpecHash          string `json:"adapter_spec_hash"`
	CanonicalSchemaHash      string `json:"canonical_schema_hash"`
	OpenAPISmokeHash         string `json:"openapi_smoke_hash"`
	RuntimeStatusHash        string `json:"runtime_status_hash"`
}

// IntegrationEvidenceKitSummary captures stable counts for release gates.
type IntegrationEvidenceKitSummary struct {
	Status                 string `json:"status"`
	EvidenceItems          int    `json:"evidence_items"`
	RequiredItems          int    `json:"required_items"`
	FixtureEvidence        int    `json:"fixture_evidence"`
	StrictFixtures         int    `json:"strict_fixtures"`
	CICommands             int    `json:"ci_commands"`
	ReviewerChecks         int    `json:"reviewer_checks"`
	Warnings               int    `json:"warnings"`
	RequiresPricingReview  bool   `json:"requires_pricing_review"`
	RequiresOutboundReview bool   `json:"requires_outbound_review"`
}

// IntegrationEvidenceItem is one release-ticket or CI attachment requirement.
type IntegrationEvidenceItem struct {
	ID       string   `json:"id"`
	Category string   `json:"category"`
	Title    string   `json:"title"`
	Kind     string   `json:"kind"`
	Command  string   `json:"command,omitempty"`
	Endpoint string   `json:"endpoint,omitempty"`
	MCPTool  string   `json:"mcp_tool,omitempty"`
	Resource string   `json:"resource,omitempty"`
	Hash     string   `json:"hash,omitempty"`
	Required bool     `json:"required"`
	Gate     string   `json:"gate"`
	Privacy  string   `json:"privacy"`
	Evidence []string `json:"evidence,omitempty"`
}

// IntegrationEvidenceChecklist is a human review lane for adapter rollout.
type IntegrationEvidenceChecklist struct {
	ID       string   `json:"id"`
	Owner    string   `json:"owner"`
	Title    string   `json:"title"`
	Required bool     `json:"required"`
	Checks   []string `json:"checks"`
	Privacy  string   `json:"privacy"`
}

func IntegrationEvidenceKitFromValues(values url.Values) IntegrationEvidenceKitRequest {
	return NormalizeIntegrationEvidenceKitRequest(IntegrationEvidenceKitRequest{
		AgentProfileID:    firstNonEmptyRecommendation(values.Get("agent_profile_id"), values.Get("agent"), values.Get("profile"), values.Get("framework")),
		ProviderProfileID: firstNonEmptyRecommendation(values.Get("provider_profile_id"), values.Get("provider"), values.Get("runtime")),
		Surface:           firstNonEmptyRecommendation(values.Get("surface"), values.Get("ingest"), values.Get("kind")),
		MinConfidence:     strings.TrimSpace(values.Get("min_confidence")),
	})
}

func NormalizeIntegrationEvidenceKitRequest(req IntegrationEvidenceKitRequest) IntegrationEvidenceKitRequest {
	req.AgentProfileID = normalizeRecommendationToken(req.AgentProfileID)
	req.ProviderProfileID = normalizeRecommendationToken(req.ProviderProfileID)
	req.Surface = normalizeRecommendationSurface(req.Surface)
	req.MinConfidence = strings.TrimSpace(req.MinConfidence)
	return req
}

// IntegrationEvidenceKitFor returns a deterministic static evidence manifest.
// It never reads SQLite usage rows, fixture bodies, local files, or network
// resources.
func IntegrationEvidenceKitFor(opts Options, runtime *storage.RuntimeStatus, req IntegrationEvidenceKitRequest) IntegrationEvidenceKitReport {
	req = NormalizeIntegrationEvidenceKitRequest(req)
	rolloutReq := IntegrationRolloutRequest{
		AgentProfileID:    req.AgentProfileID,
		ProviderProfileID: req.ProviderProfileID,
		Surface:           req.Surface,
		MinConfidence:     req.MinConfidence,
	}
	compatReq := IntegrationCompatibilityRequest{
		AgentProfileID:    req.AgentProfileID,
		ProviderProfileID: req.ProviderProfileID,
		Surface:           req.Surface,
		MinConfidence:     req.MinConfidence,
	}
	rollout := IntegrationRolloutPlanFor(rolloutReq)
	compatibility := IntegrationCompatibilityReportFor(compatReq)
	hashes := IntegrationEvidenceKitHashes{
		CapabilityCatalogHash:    CatalogFingerprint(opts),
		ProviderProfilesHash:     ProviderProfilesFingerprint(),
		AgentProfilesHash:        AgentFrameworkProfilesFingerprint(),
		SignalTaxonomyHash:       SignalTaxonomyFingerprint(),
		SignalCoverageHash:       SignalCoverageFingerprint(),
		IntegrationReadinessHash: IntegrationReadinessFingerprint(opts),
		IntegrationSmokeHash:     IntegrationSmokeFingerprint(opts, runtime),
		CompatibilityHash:        compatibility.CompatibilityHash,
		RolloutPlanHash:          rollout.RolloutHash,
		RecommendationHash:       IntegrationRecommendationContractFingerprint(),
		ConformanceMatrixHash:    AdapterConformanceMatrixFingerprint(),
		AdapterSpecHash:          AdapterContractFingerprint(),
		CanonicalSchemaHash:      storage.CanonicalEventSchemaFingerprint(),
		OpenAPISmokeHash:         OpenAPISmokeFingerprint(opts, runtime),
		RuntimeStatusHash:        hashJSONPayload(runtime),
	}
	items := integrationEvidenceItems(rollout, hashes)
	report := IntegrationEvidenceKitReport{
		Product:          "Agent Ledger",
		Contract:         "agent-ledger.integration-evidence-kit",
		Version:          "v1",
		LocalFirst:       true,
		ReadOnlySafe:     true,
		WritesLocalState: false,
		PrivacyPolicy:    "Integration evidence kits are generated from static contracts, compatibility, rollout, conformance, schema, and runtime-status metadata only. They exclude SQLite usage rows, fixture bodies, prompt content, response content, message history, local paths, secrets, account identifiers, machine names, authors, native session identifiers, and webhook URLs.",
		Request:          req,
		Hashes:           hashes,
		Target:           rollout.Target,
		EvidenceItems:    items,
		FixtureEvidence:  rollout.Fixtures,
		CICommands:       integrationEvidenceCICommands(rollout, items),
		ReviewerChecklist: []IntegrationEvidenceChecklist{
			integrationEvidenceChecklist("privacy", "security", "Confirm metadata-only persistence", true, []string{
				"adapter strips prompt, response, message, artifact body, raw header, credential, account, machine, author, and native session values before persistence",
				"fixture files used in CI contain metadata-only examples or synthetic placeholders",
				"exported screenshots and release tickets use privacy preset output only",
			}),
			integrationEvidenceChecklist("pricing", "finops", "Confirm price provenance", true, []string{
				"official seed, LiteLLM fallback, source-reported cost, and local override precedence are explicit",
				"relay, local, edge, enterprise contract, or regional pricing requires a local override before cost claims are treated as exact",
				"unpriced and fuzzy model matches are visible in pricing status or release limitations",
			}),
			integrationEvidenceChecklist("policy", "platform", "Confirm access and policy posture", true, []string{
				"admission dry-run matches the intended role and read-only behavior",
				"gateway, webhook, OTLP, relay, and outbound surfaces remain disabled unless explicitly approved",
				"policy decisions are advisory by default unless a local operator configured enforcement",
			}),
			integrationEvidenceChecklist("operations", "owner", "Confirm rollout and rollback readiness", true, []string{
				"strict conformance commands pass in adapter CI",
				"rollout plan status is ready or review-required with documented human acceptance",
				"rollback uses source-scoped disablement, projection quality checks, and optional admin repair rather than whole-database deletion",
			}),
		},
		RedactionRules: []string{
			"never attach prompts, responses, transcripts, message history, raw headers, secrets, webhook URLs, or credential-bearing logs",
			"do not attach absolute local paths, machine names, account identifiers, git authors, native session identifiers, or provider account ids",
			"prefer hashes, counts, endpoint names, contract ids, fixture paths, and validation command output",
			"use privacy presets before sharing evidence outside the local operator machine",
		},
		OperationalGuidance: []string{
			"attach this kit to adapter pull requests and internal release tickets before enabling write ingest",
			"pin the listed hashes in CI to detect drift across Agent Ledger releases",
			"run the listed CI commands locally or in a privacy-safe CI environment with synthetic fixtures",
			"treat this kit as release evidence, not as proof that live provider costs or subscription quotas match invoices",
		},
	}
	report.Summary = summarizeIntegrationEvidenceKit(report, rollout)
	report.KitHash = IntegrationEvidenceKitFingerprintFrom(report)
	return report
}

func IntegrationEvidenceKitFingerprint(opts Options, runtime *storage.RuntimeStatus, req IntegrationEvidenceKitRequest) string {
	return IntegrationEvidenceKitFingerprintFrom(IntegrationEvidenceKitFor(opts, runtime, req))
}

// IntegrationEvidenceKitOpenAPIFingerprint is a non-recursive witness hash for
// OpenAPI metadata. It intentionally avoids building the full evidence kit so
// OpenAPI generation stays cheap and does not pull in rollout/smoke report
// bodies recursively.
func IntegrationEvidenceKitOpenAPIFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	return hashJSONPayload(map[string]interface{}{
		"contract":                   "agent-ledger.integration-evidence-kit",
		"version":                    "v1",
		"default_uri":                "/api/integrations/evidence-kit",
		"capability_catalog_hash":    CatalogFingerprint(opts),
		"provider_profiles_hash":     ProviderProfilesFingerprint(),
		"agent_profiles_hash":        AgentFrameworkProfilesFingerprint(),
		"signal_taxonomy_hash":       SignalTaxonomyFingerprint(),
		"signal_coverage_hash":       SignalCoverageFingerprint(),
		"integration_readiness_hash": IntegrationReadinessFingerprint(opts),
		"conformance_matrix_hash":    AdapterConformanceMatrixFingerprint(),
		"adapter_spec_hash":          AdapterContractFingerprint(),
		"canonical_schema_hash":      storage.CanonicalEventSchemaFingerprint(),
		"runtime_status_hash":        hashJSONPayload(runtime),
		"privacy":                    "metadata-only OpenAPI witness; full endpoint ETag is returned by /api/integrations/evidence-kit",
	})
}

func IntegrationEvidenceKitFingerprintFrom(report IntegrationEvidenceKitReport) string {
	report.KitHash = ""
	return hashJSONPayload(report)
}

func integrationEvidenceItems(rollout IntegrationRolloutPlanReport, hashes IntegrationEvidenceKitHashes) []IntegrationEvidenceItem {
	items := []IntegrationEvidenceItem{
		integrationEvidenceItem("contracts-verify", "contract", "Control-plane contract verification", "self-check", "agent-ledger contracts verify", "GET /api/contracts/verify", "ledger.contracts_verify", "agent-ledger://contracts/verification", "", true, "contract verification returns ok=true", []string{"contract bundle", "OpenAPI", "privacy invariants"}),
		integrationEvidenceItem("openapi", "contract", "OpenAPI control-plane witness", "schema", "agent-ledger openapi", "GET /api/openapi.json", "ledger.openapi", "agent-ledger://contracts/openapi", hashes.OpenAPISmokeHash, true, "OpenAPI smoke hash is pinned without recursive document hashing", []string{"operation ids", "read-only metadata", "body limits"}),
		integrationEvidenceItem("capability-catalog", "contract", "Capability catalog", "catalog", "agent-ledger integrations", "GET /api/integrations", "ledger.integrations", "agent-ledger://integrations/catalog", hashes.CapabilityCatalogHash, true, "catalog lists the target surface and runtime posture", []string{rollout.Target.Surface, rollout.Target.Status}),
		integrationEvidenceItem("compatibility", "planning", "Integration compatibility row", "matrix", rolloutCompatibilityCommand(rollout.Request), "GET /api/integrations/compatibility", "ledger.integration_compatibility", "agent-ledger://integrations/compatibility", hashes.CompatibilityHash, true, "compatibility row is reviewed for confidence, risk, and limitations", []string{rollout.Target.AgentProfileID, rollout.Target.ProviderProfileID, rollout.Target.Surface}),
		integrationEvidenceItem("rollout-plan", "planning", "Rollout checklist", "checklist", rolloutPlanCommand(rollout.Request), "GET /api/integrations/rollout-plan", "ledger.integration_rollout_plan", "agent-ledger://integrations/rollout-plan", hashes.RolloutPlanHash, true, "rollout plan documents release gates and rollback steps", []string{rollout.Summary.Status, rollout.Target.RiskLevel}),
		integrationEvidenceItem("readiness", "runtime", "Integration readiness gates", "readiness", "agent-ledger integrations readiness", "GET /api/integrations/readiness", "ledger.integration_readiness", "agent-ledger://integrations/readiness", hashes.IntegrationReadinessHash, true, "readiness exposes blocked, warning, and disabled-by-default surfaces", []string{"activation gates", "runtime flags"}),
		integrationEvidenceItem("smoke", "runtime", "Integration smoke report", "smoke", "agent-ledger integrations smoke", "GET /api/integrations/smoke", "ledger.integration_smoke", "agent-ledger://integrations/smoke", hashes.IntegrationSmokeHash, true, "smoke report has no failed active gates", []string{"contract", "conformance", "recommendation"}),
		integrationEvidenceItem("signal-coverage", "adapter", "Signal taxonomy coverage", "coverage", "agent-ledger signal-coverage", "GET /api/integrations/signal-coverage", "ledger.signal_coverage", "agent-ledger://integrations/signal-coverage", hashes.SignalCoverageHash, true, "required signals map to adapter/provider coverage", rollout.Target.RequiredSignals),
		integrationEvidenceItem("conformance-matrix", "adapter", "Adapter conformance matrix", "matrix", "agent-ledger adapter matrix", "GET /api/integrations/conformance-matrix", "ledger.conformance_matrix", "agent-ledger://integrations/conformance-matrix", hashes.ConformanceMatrixHash, true, "strict fixture families are documented", rollout.Target.ConformanceKinds),
		integrationEvidenceItem("adapter-spec", "adapter", "Adapter contract", "schema", "agent-ledger adapter spec", "GET /api/integrations/adapter-spec", "ledger.adapter_contract", "agent-ledger://integrations/adapter-contract", hashes.AdapterSpecHash, true, "adapter contract forbids content-bearing payload fields", rollout.Target.ExpectedEventTypes),
	}
	for _, phase := range rollout.Phases {
		for _, step := range phase.Steps {
			if step.ID == "admission" || step.ID == "pricing" || step.ID == "policy" || strings.HasPrefix(step.ID, "projection-") {
				items = append(items, integrationEvidenceItem(step.ID, "governance", step.Title, "gate", step.Command, step.Endpoint, step.MCPTool, "", "", step.Required, step.Gate, step.Evidence))
			}
		}
	}
	for _, fixture := range rollout.Fixtures {
		items = append(items, integrationEvidenceItem("fixture-"+fixture.ConformanceKind+"-"+strings.TrimSuffix(strings.TrimPrefix(fixture.Path, "examples/adapter-fixtures/"), "."+fixture.Format), "fixture", "Strict fixture: "+fixture.Scenario, "fixture", fixture.Command, "POST /api/integrations/conformance?kind="+fixture.ConformanceKind+"&strict=true", "ledger.adapter_conformance", "", "", fixture.Strict, "strict conformance passes without provenance warnings", append([]string{fixture.Path, fixture.Format}, fixture.ExpectedEventTypes...)))
	}
	return items
}

func integrationEvidenceItem(id, category, title, kind, command, endpoint, tool, resource, hash string, required bool, gate string, evidence []string) IntegrationEvidenceItem {
	return IntegrationEvidenceItem{
		ID:       sanitizeEvidenceID(id),
		Category: category,
		Title:    title,
		Kind:     kind,
		Command:  command,
		Endpoint: endpoint,
		MCPTool:  tool,
		Resource: resource,
		Hash:     hash,
		Required: required,
		Gate:     gate,
		Privacy:  "metadata-only; prompts, responses, messages, artifact bodies, raw headers, credentials, local paths, accounts, machines, authors, native sessions, and webhook URLs are excluded",
		Evidence: uniqueSortedStrings(evidence),
	}
}

func integrationEvidenceChecklist(id, owner, title string, required bool, checks []string) IntegrationEvidenceChecklist {
	return IntegrationEvidenceChecklist{
		ID:       id,
		Owner:    owner,
		Title:    title,
		Required: required,
		Checks:   uniqueSortedStrings(checks),
		Privacy:  "review checklist uses policy, pricing, and metadata posture only; it does not request prompt, response, path, account, machine, author, or session data",
	}
}

func integrationEvidenceCICommands(rollout IntegrationRolloutPlanReport, items []IntegrationEvidenceItem) []string {
	commands := []string{}
	for _, item := range items {
		if item.Command != "" && (item.Required || item.Category == "fixture") {
			commands = append(commands, item.Command)
		}
	}
	commands = append(commands, rolloutCompatibilityCommand(rollout.Request))
	return uniqueSortedStrings(commands)
}

func rolloutPlanCommand(req IntegrationRolloutRequest) string {
	command := strings.Replace(rolloutCompatibilityCommand(req), "agent-ledger integrations compatibility", "agent-ledger integrations rollout-plan", 1)
	if command == "" {
		return "agent-ledger integrations rollout-plan"
	}
	return command
}

func summarizeIntegrationEvidenceKit(report IntegrationEvidenceKitReport, rollout IntegrationRolloutPlanReport) IntegrationEvidenceKitSummary {
	summary := IntegrationEvidenceKitSummary{
		Status:          rollout.Summary.Status,
		EvidenceItems:   len(report.EvidenceItems),
		FixtureEvidence: len(report.FixtureEvidence),
		CICommands:      len(report.CICommands),
		ReviewerChecks:  len(report.ReviewerChecklist),
		Warnings:        rollout.Summary.Warnings,
	}
	for _, item := range report.EvidenceItems {
		if item.Required {
			summary.RequiredItems++
		}
	}
	for _, fixture := range report.FixtureEvidence {
		if fixture.Strict {
			summary.StrictFixtures++
		}
	}
	summary.RequiresPricingReview = rollout.Summary.RequiresPricingOverride
	summary.RequiresOutboundReview = rollout.Summary.RequiresOutboundReview
	if report.Target.RiskLevel == "high" {
		summary.RequiresOutboundReview = true
	}
	if report.Target.Status != "ready" && summary.Status == "ready" {
		summary.Status = "review-required"
	}
	return summary
}

func sanitizeEvidenceID(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	var b strings.Builder
	lastHyphen := false
	for _, r := range raw {
		writeHyphen := false
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
			lastHyphen = false
		case r >= '0' && r <= '9':
			b.WriteRune(r)
			lastHyphen = false
		case r == '-' || r == '_' || r == '/' || r == '.' || r == ' ':
			writeHyphen = true
		}
		if writeHyphen && !lastHyphen && b.Len() > 0 {
			b.WriteByte('-')
			lastHyphen = true
		}
	}
	return strings.Trim(b.String(), "-")
}
