package integrations

import (
	"net/url"
	"path"
	"sort"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// IntegrationRolloutRequest selects one static adapter rollout target.
type IntegrationRolloutRequest struct {
	AgentProfileID    string `json:"agent_profile_id,omitempty"`
	ProviderProfileID string `json:"provider_profile_id,omitempty"`
	Surface           string `json:"surface,omitempty"`
	MinConfidence     string `json:"min_confidence,omitempty"`
}

// IntegrationRolloutPlanReport is a metadata-only release checklist for
// enabling an Agent Ledger ecosystem adapter, wrapper, provider, or gateway.
type IntegrationRolloutPlanReport struct {
	Product               string                      `json:"product"`
	Contract              string                      `json:"contract"`
	Version               string                      `json:"version"`
	LocalFirst            bool                        `json:"local_first"`
	ReadOnlySafe          bool                        `json:"read_only_safe"`
	WritesLocalState      bool                        `json:"writes_local_state"`
	PrivacyPolicy         string                      `json:"privacy_policy"`
	Request               IntegrationRolloutRequest   `json:"request"`
	CompatibilityHash     string                      `json:"compatibility_hash"`
	RecommendationHash    string                      `json:"integration_recommendation_hash"`
	ConformanceMatrixHash string                      `json:"conformance_matrix_hash"`
	CanonicalSchemaHash   string                      `json:"canonical_schema_hash"`
	RolloutHash           string                      `json:"rollout_hash"`
	Summary               IntegrationRolloutSummary   `json:"summary"`
	Target                IntegrationRolloutTarget    `json:"target"`
	Phases                []IntegrationRolloutPhase   `json:"phases"`
	Fixtures              []IntegrationRolloutFixture `json:"fixtures"`
	ReleaseGates          []string                    `json:"release_gates"`
	RollbackPlan          []string                    `json:"rollback_plan"`
	OperationalGuidance   []string                    `json:"operational_guidance"`
}

// IntegrationRolloutSummary captures stable checklist counts for CI.
type IntegrationRolloutSummary struct {
	Status                  string `json:"status"`
	Phases                  int    `json:"phases"`
	Steps                   int    `json:"steps"`
	RequiredSteps           int    `json:"required_steps"`
	StrictFixtures          int    `json:"strict_fixtures"`
	AdmissionChecks         int    `json:"admission_checks"`
	ReleaseGates            int    `json:"release_gates"`
	Warnings                int    `json:"warnings"`
	RequiresPricingOverride bool   `json:"requires_pricing_override"`
	RequiresOutboundReview  bool   `json:"requires_outbound_review"`
}

// IntegrationRolloutTarget identifies the selected compatibility row.
type IntegrationRolloutTarget struct {
	AgentProfileID     string   `json:"agent_profile_id"`
	AgentLabel         string   `json:"agent_label"`
	ProviderProfileID  string   `json:"provider_profile_id"`
	ProviderLabel      string   `json:"provider_label"`
	Surface            string   `json:"surface"`
	Status             string   `json:"status"`
	RiskLevel          string   `json:"risk_level"`
	Confidence         float64  `json:"confidence"`
	CandidateSurfaces  []string `json:"candidate_surfaces"`
	ConformanceKinds   []string `json:"conformance_kinds,omitempty"`
	ExpectedEventTypes []string `json:"expected_event_types"`
	RequiredSignals    []string `json:"required_signals"`
}

// IntegrationRolloutPhase groups ordered rollout steps.
type IntegrationRolloutPhase struct {
	ID       string                   `json:"id"`
	Title    string                   `json:"title"`
	Purpose  string                   `json:"purpose"`
	Required bool                     `json:"required"`
	Status   string                   `json:"status"`
	Steps    []IntegrationRolloutStep `json:"steps"`
}

// IntegrationRolloutStep is one metadata-only action in the rollout checklist.
type IntegrationRolloutStep struct {
	ID       string   `json:"id"`
	Title    string   `json:"title"`
	Command  string   `json:"command,omitempty"`
	Endpoint string   `json:"endpoint,omitempty"`
	MCPTool  string   `json:"mcp_tool,omitempty"`
	Required bool     `json:"required"`
	Gate     string   `json:"gate"`
	Privacy  string   `json:"privacy"`
	Evidence []string `json:"evidence,omitempty"`
}

// IntegrationRolloutFixture describes repository-relative validation fixtures.
type IntegrationRolloutFixture struct {
	ConformanceKind    string   `json:"conformance_kind"`
	Path               string   `json:"path"`
	Format             string   `json:"format"`
	Scenario           string   `json:"scenario"`
	Strict             bool     `json:"strict"`
	Command            string   `json:"command"`
	ExpectedEventTypes []string `json:"expected_event_types"`
	Privacy            string   `json:"privacy"`
}

// IntegrationRolloutFromValues builds a stable request from REST or MCP query
// parameters.
func IntegrationRolloutFromValues(values url.Values) IntegrationRolloutRequest {
	return NormalizeIntegrationRolloutRequest(IntegrationRolloutRequest{
		AgentProfileID:    firstNonEmptyRecommendation(values.Get("agent_profile_id"), values.Get("agent"), values.Get("profile"), values.Get("framework")),
		ProviderProfileID: firstNonEmptyRecommendation(values.Get("provider_profile_id"), values.Get("provider"), values.Get("runtime")),
		Surface:           firstNonEmptyRecommendation(values.Get("surface"), values.Get("ingest"), values.Get("kind")),
		MinConfidence:     strings.TrimSpace(values.Get("min_confidence")),
	})
}

func NormalizeIntegrationRolloutRequest(req IntegrationRolloutRequest) IntegrationRolloutRequest {
	req.AgentProfileID = normalizeRecommendationToken(req.AgentProfileID)
	req.ProviderProfileID = normalizeRecommendationToken(req.ProviderProfileID)
	req.Surface = normalizeRecommendationSurface(req.Surface)
	req.MinConfidence = strings.TrimSpace(req.MinConfidence)
	return req
}

// IntegrationRolloutPlanFor returns a deterministic, privacy-safe rollout plan.
func IntegrationRolloutPlanFor(req IntegrationRolloutRequest) IntegrationRolloutPlanReport {
	req = NormalizeIntegrationRolloutRequest(req)
	compatReq := IntegrationCompatibilityRequest{
		AgentProfileID:    req.AgentProfileID,
		ProviderProfileID: req.ProviderProfileID,
		Surface:           req.Surface,
		MinConfidence:     req.MinConfidence,
	}
	compatibility := IntegrationCompatibilityReportFor(compatReq)
	row := selectRolloutRow(compatibility.Rows)
	fixtures := rolloutFixtures(row)
	phases := rolloutPhases(row, fixtures, req)
	report := IntegrationRolloutPlanReport{
		Product:               "Agent Ledger",
		Contract:              "agent-ledger.integration-rollout-plan",
		Version:               "v1",
		LocalFirst:            true,
		ReadOnlySafe:          true,
		WritesLocalState:      false,
		PrivacyPolicy:         "Integration rollout plans are generated from static compatibility, recommendation, conformance, and schema metadata only. They exclude usage rows, fixture bodies, prompt content, response content, message history, local paths, secrets, account identifiers, machine names, authors, and native session identifiers.",
		Request:               req,
		CompatibilityHash:     compatibility.CompatibilityHash,
		RecommendationHash:    IntegrationRecommendationContractFingerprint(),
		ConformanceMatrixHash: AdapterConformanceMatrixFingerprint(),
		CanonicalSchemaHash:   storage.CanonicalEventSchemaFingerprint(),
		Target:                rolloutTargetFromRow(row),
		Phases:                phases,
		Fixtures:              fixtures,
		ReleaseGates: []string{
			"contract verification, integration smoke, compatibility, and rollout plan hashes are pinned in adapter CI",
			"strict conformance passes for every selected fixture without provenance warnings",
			"admission checks allow only the intended read or write surface for the intended role",
			"pricing source, stale state, unpriced model handling, and local override requirements are explicit",
			"privacy review confirms no prompt, response, message, artifact body, header, credential, raw path, account, machine, or author values are persisted",
			"gateway, webhook, OTLP, and outbound surfaces remain disabled unless explicitly configured and reviewed",
		},
		RollbackPlan: []string{
			"disable the adapter, collector, gateway, or wrapper integration flag before retrying ingest",
			"keep already-ingested metadata rows for audit unless an operator explicitly performs source-scoped cleanup",
			"rerun projection quality and data quality reports after parser or pricing fixes",
			"record the failed rollout as a local audit event without prompt or secret material",
		},
		OperationalGuidance: []string{
			"use this plan as a CI checklist before enabling a new agent, provider, router, relay, local model, or edge runtime",
			"prefer dry-run validation and read-only resources until conformance and admission pass",
			"attach this plan to internal adapter PRs together with fixture files and contract verification output",
			"treat warning status as a human review requirement rather than a runtime block",
		},
	}
	report.Summary = summarizeIntegrationRollout(report)
	report.RolloutHash = IntegrationRolloutFingerprintFrom(report)
	return report
}

func IntegrationRolloutFingerprint(req IntegrationRolloutRequest) string {
	return IntegrationRolloutFingerprintFrom(IntegrationRolloutPlanFor(req))
}

func IntegrationRolloutFingerprintFrom(report IntegrationRolloutPlanReport) string {
	report.RolloutHash = ""
	return hashJSONPayload(report)
}

func selectRolloutRow(rows []IntegrationCompatibilityRow) IntegrationCompatibilityRow {
	if len(rows) == 0 {
		return IntegrationCompatibilityRow{
			RecommendedSurface:   "canonical-events",
			Status:               "limited",
			RiskLevel:            "low",
			CompatibilityReasons: []string{"no matching compatibility row; use canonical-events as a metadata-only fallback"},
			NextSteps:            []string{"select a supported agent and provider profile before enabling write ingest"},
		}
	}
	best := rows[0]
	for _, row := range rows[1:] {
		if rolloutRowRank(row) > rolloutRowRank(best) {
			best = row
		}
	}
	return best
}

func rolloutRowRank(row IntegrationCompatibilityRow) int {
	score := int(row.Confidence * 100)
	if row.Status == "ready" {
		score += 50
	}
	if len(row.StrictCI) > 0 {
		score += 20
	}
	if row.RiskLevel == "high" {
		score -= 15
	}
	return score
}

func rolloutTargetFromRow(row IntegrationCompatibilityRow) IntegrationRolloutTarget {
	return IntegrationRolloutTarget{
		AgentProfileID:     row.AgentProfileID,
		AgentLabel:         row.AgentLabel,
		ProviderProfileID:  row.ProviderProfileID,
		ProviderLabel:      row.ProviderLabel,
		Surface:            row.RecommendedSurface,
		Status:             row.Status,
		RiskLevel:          row.RiskLevel,
		Confidence:         row.Confidence,
		CandidateSurfaces:  row.CandidateSurfaces,
		ConformanceKinds:   row.ConformanceKinds,
		ExpectedEventTypes: row.ExpectedEventTypes,
		RequiredSignals:    row.RequiredSignals,
	}
}

func rolloutFixtures(row IntegrationCompatibilityRow) []IntegrationRolloutFixture {
	matrix := AdapterConformanceMatrixSpec()
	providerID := row.ProviderProfileID
	kinds := row.ConformanceKinds
	if len(kinds) == 0 {
		kinds = conformanceKindsForSurface(row.RecommendedSurface)
	}
	out := []IntegrationRolloutFixture{}
	for _, kind := range matrix.Kinds {
		if len(kinds) > 0 && !containsString(kinds, kind.ConformanceKind) {
			continue
		}
		for _, fixture := range kind.Fixtures {
			if providerID != "" && len(fixture.ProviderProfileIDs) > 0 && !containsString(fixture.ProviderProfileIDs, providerID) {
				continue
			}
			out = append(out, IntegrationRolloutFixture{
				ConformanceKind:    kind.ConformanceKind,
				Path:               fixture.Path,
				Format:             fixture.Format,
				Scenario:           fixture.Scenario,
				Strict:             fixture.Strict,
				Command:            fixture.Command,
				ExpectedEventTypes: fixture.ExpectedEventTypes,
				Privacy:            fixture.Privacy,
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ConformanceKind == out[j].ConformanceKind {
			return out[i].Path < out[j].Path
		}
		return out[i].ConformanceKind < out[j].ConformanceKind
	})
	return out
}

func rolloutPhases(row IntegrationCompatibilityRow, fixtures []IntegrationRolloutFixture, req IntegrationRolloutRequest) []IntegrationRolloutPhase {
	return []IntegrationRolloutPhase{
		{
			ID:       "contract-discovery",
			Title:    "Contract Discovery",
			Purpose:  "Pin the static Agent Ledger contracts that the adapter depends on.",
			Required: true,
			Status:   "required",
			Steps: []IntegrationRolloutStep{
				rolloutStep("discovery", "Fetch local discovery manifest", "agent-ledger discovery", "GET /.well-known/agent-ledger.json", "ledger.discovery", true, "discovery manifest exposes stable hashes", []string{"agent-ledger.discovery"}),
				rolloutStep("contracts", "Verify control-plane contracts", "agent-ledger contracts verify", "GET /api/contracts/verify", "ledger.contracts_verify", true, "contract verification must pass", []string{"agent-ledger.contract-verification"}),
				rolloutStep("compatibility", "Generate compatibility row", rolloutCompatibilityCommand(req), "GET /api/integrations/compatibility", "ledger.integration_compatibility", true, "compatibility row must be reviewed", []string{row.RecommendedSurface, row.Status}),
			},
		},
		{
			ID:       "fixture-conformance",
			Title:    "Fixture Conformance",
			Purpose:  "Prove adapter output maps into canonical metadata-only events before ingest is enabled.",
			Required: true,
			Status:   phaseStatus(len(fixtures) > 0),
			Steps:    rolloutFixtureSteps(fixtures),
		},
		{
			ID:       "privacy-pricing-policy",
			Title:    "Privacy, Pricing And Policy",
			Purpose:  "Confirm content exclusion, pricing provenance, and policy/admission behavior.",
			Required: true,
			Status:   "required",
			Steps: []IntegrationRolloutStep{
				rolloutStep("pricing", "Check pricing governance", "agent-ledger pricing", "GET /api/pricing/status", "", true, "pricing source and stale/unpriced state are explicit", []string{"official seed", "LiteLLM fallback", "local override"}),
				rolloutStep("policy", "Evaluate advisory model policy", "agent-ledger policy evaluate --action model.call", "POST /api/policy/evaluate", "ledger.get_policy", true, "policy must be advisory or explicitly approved", row.RequiredSignals),
				rolloutStep("admission", "Dry-run target write/read operation", rolloutAdmissionCommand(row), "GET /api/admission/check", "ledger.admission_check", true, "admission must match intended role and read-only behavior", []string{row.RecommendedSurface, row.RiskLevel}),
			},
		},
		{
			ID:       "runtime-smoke",
			Title:    "Runtime Smoke",
			Purpose:  "Run local smoke checks and diagnostics before release or internal rollout.",
			Required: true,
			Status:   "required",
			Steps: []IntegrationRolloutStep{
				rolloutStep("integration-smoke", "Run integration smoke", "agent-ledger integrations smoke", "GET /api/integrations/smoke", "ledger.integration_smoke", true, "smoke report must have no failures", []string{"agent-ledger.integration-smoke"}),
				rolloutStep("readiness", "Inspect integration readiness", "agent-ledger integrations readiness", "GET /api/integrations/readiness", "ledger.integration_readiness", true, "readiness must show no unexpected blocked gate", []string{row.Status, row.RiskLevel}),
				rolloutStep("doctor", "Run local doctor after ingest is enabled", "agent-ledger doctor --format markdown", "GET /api/doctor", "", false, "doctor explains missing data, pricing, and time-window issues", []string{"data quality", "pricing", "collector health"}),
			},
		},
		{
			ID:       "release-operations",
			Title:    "Release Operations",
			Purpose:  "Define release gates, monitoring, and rollback for the adapter rollout.",
			Required: true,
			Status:   "review-required",
			Steps: []IntegrationRolloutStep{
				rolloutStep("release-note", "Document scope and limitations", "agent-ledger goal coverage", "GET /api/goal-coverage", "ledger.goal_coverage", true, "release notes must disclose experimental or heuristic limitations", row.Limitations),
				rolloutStep("monitoring", "Monitor data quality and workload feed", "agent-ledger workload feed --severity warning --max-age 10m", "GET /api/workload-events/stream", "ledger.workload_feed", false, "monitoring remains metadata-only", []string{"workload feed", "data quality"}),
				rolloutStep("projection-quality", "Check projection quality before rollback", "agent-ledger projection quality", "", "", true, "rollback decisions start with read-only projection diagnostics", []string{"projection quality", "source-scoped diagnosis"}),
				rolloutStep("projection-repair", "Prepare admin-only projection repair", "agent-ledger projection repair", "POST /api/projections/repair", "", false, "repair is source-scoped and avoids whole-database deletion", []string{"source-scoped repair", "admin-only operation"}),
			},
		},
	}
}

func rolloutStep(id, title, command, endpoint, tool string, required bool, gate string, evidence []string) IntegrationRolloutStep {
	return IntegrationRolloutStep{
		ID:       id,
		Title:    title,
		Command:  command,
		Endpoint: endpoint,
		MCPTool:  tool,
		Required: required,
		Gate:     gate,
		Privacy:  "metadata-only; prompt, response, message, artifact body, raw header, credential, local path, account, machine, author, and native session values are excluded",
		Evidence: uniqueSortedStrings(evidence),
	}
}

func rolloutFixtureSteps(fixtures []IntegrationRolloutFixture) []IntegrationRolloutStep {
	if len(fixtures) == 0 {
		return []IntegrationRolloutStep{
			rolloutStep("fixture-missing", "Add a privacy-safe fixture before release", "agent-ledger adapter matrix", "GET /api/integrations/conformance-matrix", "ledger.conformance_matrix", true, "no matching strict fixture is available for this target", []string{"add fixture", "strict conformance"}),
		}
	}
	steps := make([]IntegrationRolloutStep, 0, len(fixtures))
	for _, fixture := range fixtures {
		steps = append(steps, rolloutStep(rolloutFixtureStepID(fixture), "Validate "+fixture.Scenario, fixture.Command, "POST /api/integrations/conformance?kind="+fixture.ConformanceKind+"&strict=true", "ledger.adapter_conformance", true, "strict fixture must pass", append([]string{fixture.Path, fixture.Format}, fixture.ExpectedEventTypes...)))
	}
	return steps
}

func rolloutFixtureStepID(fixture IntegrationRolloutFixture) string {
	name := strings.TrimSuffix(path.Base(fixture.Path), path.Ext(fixture.Path))
	name = strings.Trim(strings.ToLower(name), "-_ .")
	var b strings.Builder
	for _, r := range fixture.ConformanceKind + "-" + name {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteByte('-')
		}
	}
	id := strings.Trim(b.String(), "-")
	if id == "" {
		id = strings.Trim(fixture.ConformanceKind, "-")
	}
	return "fixture-" + id
}

func rolloutCompatibilityCommand(req IntegrationRolloutRequest) string {
	parts := []string{"agent-ledger integrations compatibility"}
	if req.AgentProfileID != "" {
		parts = append(parts, "--agent "+req.AgentProfileID)
	}
	if req.ProviderProfileID != "" {
		parts = append(parts, "--provider "+req.ProviderProfileID)
	}
	if req.Surface != "" {
		parts = append(parts, "--surface "+req.Surface)
	}
	if req.MinConfidence != "" {
		parts = append(parts, "--min-confidence "+req.MinConfidence)
	}
	return strings.Join(parts, " ")
}

func rolloutAdmissionCommand(row IntegrationCompatibilityRow) string {
	switch row.RecommendedSurface {
	case "local-collector":
		return "agent-ledger admission check --surface http --method POST --path /api/scan --role operator"
	case "provider-envelope", "provider-stream":
		return "agent-ledger admission check --surface http --method POST --path /api/provider/calls --role operator"
	case "opentelemetry":
		return "agent-ledger admission check --surface http --method POST --path /api/otel/genai --role operator"
	case "a2a":
		return "agent-ledger admission check --surface http --method POST --path /api/a2a/tasks --role operator"
	case "gateway":
		return "agent-ledger admission check --surface http --method POST --path /gateway/openai/v1/chat/completions --role operator"
	default:
		return "agent-ledger admission check --surface http --method POST --path /api/events --role operator"
	}
}

func phaseStatus(ok bool) string {
	if ok {
		return "required"
	}
	return "blocked"
}

func summarizeIntegrationRollout(report IntegrationRolloutPlanReport) IntegrationRolloutSummary {
	summary := IntegrationRolloutSummary{
		Status:       "ready",
		Phases:       len(report.Phases),
		ReleaseGates: len(report.ReleaseGates),
	}
	if report.Target.Status != "ready" {
		summary.Warnings++
		summary.Status = "review-required"
	}
	if report.Target.RiskLevel == "high" || report.Target.Surface == "gateway" {
		summary.RequiresOutboundReview = true
		summary.Warnings++
		summary.Status = "review-required"
	}
	for _, fixture := range report.Fixtures {
		if fixture.Strict {
			summary.StrictFixtures++
		}
	}
	for _, phase := range report.Phases {
		if phase.Status == "blocked" {
			summary.Status = "blocked"
			summary.Warnings++
		}
		for _, step := range phase.Steps {
			summary.Steps++
			if step.Required {
				summary.RequiredSteps++
			}
			if strings.Contains(step.ID, "admission") {
				summary.AdmissionChecks++
			}
			if strings.Contains(strings.ToLower(strings.Join(step.Evidence, " ")), "pricing override") {
				summary.RequiresPricingOverride = true
			}
		}
	}
	providerRisk := strings.ToLower(report.Target.ProviderProfileID + " " + strings.Join(report.Target.RequiredSignals, " "))
	if strings.Contains(providerRisk, "relay") || strings.Contains(providerRisk, "local") || strings.Contains(providerRisk, "edge") {
		summary.RequiresPricingOverride = true
	}
	return summary
}
