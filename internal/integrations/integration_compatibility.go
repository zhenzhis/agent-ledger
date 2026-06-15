package integrations

import (
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// IntegrationCompatibilityRequest filters the static ecosystem compatibility
// matrix. It is metadata-only and never carries prompt, response, path, or
// credential values.
type IntegrationCompatibilityRequest struct {
	AgentProfileID    string `json:"agent_profile_id,omitempty"`
	ProviderProfileID string `json:"provider_profile_id,omitempty"`
	Surface           string `json:"surface,omitempty"`
	MinConfidence     string `json:"min_confidence,omitempty"`
}

// IntegrationCompatibilityReport summarizes which Agent Ledger integration
// surfaces fit each agent/provider family before an adapter is enabled.
type IntegrationCompatibilityReport struct {
	Product               string                          `json:"product"`
	Contract              string                          `json:"contract"`
	Version               string                          `json:"version"`
	LocalFirst            bool                            `json:"local_first"`
	ReadOnlySafe          bool                            `json:"read_only_safe"`
	WritesLocalState      bool                            `json:"writes_local_state"`
	PrivacyPolicy         string                          `json:"privacy_policy"`
	Request               IntegrationCompatibilityRequest `json:"request"`
	AgentProfilesHash     string                          `json:"agent_profiles_hash"`
	ProviderProfilesHash  string                          `json:"provider_profiles_hash"`
	RecommendationHash    string                          `json:"integration_recommendation_hash"`
	ConformanceMatrixHash string                          `json:"conformance_matrix_hash"`
	CanonicalSchemaHash   string                          `json:"canonical_schema_hash"`
	CompatibilityHash     string                          `json:"compatibility_hash"`
	Summary               IntegrationCompatibilitySummary `json:"summary"`
	Rows                  []IntegrationCompatibilityRow   `json:"rows"`
	QualityGates          []string                        `json:"quality_gates"`
	OperationalGuidance   []string                        `json:"operational_guidance"`
}

// IntegrationCompatibilitySummary captures stable rollout counts for wrapper CI.
type IntegrationCompatibilitySummary struct {
	AgentProfiles        int `json:"agent_profiles"`
	ProviderProfiles     int `json:"provider_profiles"`
	Rows                 int `json:"rows"`
	Ready                int `json:"ready"`
	ReviewRequired       int `json:"review_required"`
	Limited              int `json:"limited"`
	HighConfidence       int `json:"high_confidence"`
	StrictCIAvailable    int `json:"strict_ci_available"`
	LocalOnlySurfaces    int `json:"local_only_surfaces"`
	OutboundSurfaces     int `json:"outbound_surfaces"`
	UnpricedRiskProfiles int `json:"unpriced_risk_profiles"`
}

// IntegrationCompatibilityRow is one agent/provider compatibility decision.
type IntegrationCompatibilityRow struct {
	AgentProfileID       string   `json:"agent_profile_id"`
	AgentLabel           string   `json:"agent_label"`
	ProviderProfileID    string   `json:"provider_profile_id"`
	ProviderLabel        string   `json:"provider_label"`
	RecommendedSurface   string   `json:"recommended_surface"`
	CandidateSurfaces    []string `json:"candidate_surfaces"`
	Status               string   `json:"status"`
	RiskLevel            string   `json:"risk_level"`
	Confidence           float64  `json:"confidence"`
	StrictCI             []string `json:"strict_ci"`
	Validation           []string `json:"validation"`
	ExpectedEventTypes   []string `json:"expected_event_types"`
	RequiredSignals      []string `json:"required_signals"`
	ConformanceKinds     []string `json:"conformance_kinds,omitempty"`
	CompatibilityReasons []string `json:"compatibility_reasons"`
	Limitations          []string `json:"limitations,omitempty"`
	NextSteps            []string `json:"next_steps"`
}

// IntegrationCompatibilityFromValues builds a stable filter request from REST
// query parameters or MCP resource arguments.
func IntegrationCompatibilityFromValues(values url.Values) IntegrationCompatibilityRequest {
	req := IntegrationCompatibilityRequest{
		AgentProfileID: firstNonEmptyRecommendation(
			values.Get("agent_profile_id"),
			values.Get("agent"),
			values.Get("profile"),
			values.Get("framework"),
		),
		ProviderProfileID: firstNonEmptyRecommendation(
			values.Get("provider_profile_id"),
			values.Get("provider"),
			values.Get("runtime"),
		),
		Surface:       firstNonEmptyRecommendation(values.Get("surface"), values.Get("ingest"), values.Get("kind")),
		MinConfidence: strings.TrimSpace(values.Get("min_confidence")),
	}
	return NormalizeIntegrationCompatibilityRequest(req)
}

// NormalizeIntegrationCompatibilityRequest returns a deterministic request.
func NormalizeIntegrationCompatibilityRequest(req IntegrationCompatibilityRequest) IntegrationCompatibilityRequest {
	req.AgentProfileID = normalizeRecommendationToken(req.AgentProfileID)
	req.ProviderProfileID = normalizeRecommendationToken(req.ProviderProfileID)
	req.Surface = normalizeRecommendationSurface(req.Surface)
	req.MinConfidence = strings.TrimSpace(req.MinConfidence)
	return req
}

// IntegrationCompatibilityReportFor returns a static privacy-safe ecosystem
// matrix. It never reads SQLite, local source files, or network resources.
func IntegrationCompatibilityReportFor(req IntegrationCompatibilityRequest) IntegrationCompatibilityReport {
	req = NormalizeIntegrationCompatibilityRequest(req)
	agents := AgentFrameworkProfiles().Profiles
	providers := ProviderProfiles().Profiles
	rows := []IntegrationCompatibilityRow{}
	minConfidence := compatibilityMinConfidence(req.MinConfidence)
	for _, agent := range agents {
		if req.AgentProfileID != "" && agent.ID != req.AgentProfileID && !containsString(agent.Families, req.AgentProfileID) {
			continue
		}
		for _, provider := range providers {
			if req.ProviderProfileID != "" && provider.ID != req.ProviderProfileID && !containsString(provider.Families, req.ProviderProfileID) {
				continue
			}
			row := integrationCompatibilityRow(agent, provider, req.Surface)
			if row.Confidence < minConfidence {
				continue
			}
			rows = append(rows, row)
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].AgentProfileID == rows[j].AgentProfileID {
			if rows[i].ProviderProfileID == rows[j].ProviderProfileID {
				return rows[i].RecommendedSurface < rows[j].RecommendedSurface
			}
			return rows[i].ProviderProfileID < rows[j].ProviderProfileID
		}
		return rows[i].AgentProfileID < rows[j].AgentProfileID
	})
	report := IntegrationCompatibilityReport{
		Product:               "Agent Ledger",
		Contract:              "agent-ledger.integration-compatibility",
		Version:               "v1",
		LocalFirst:            true,
		ReadOnlySafe:          true,
		WritesLocalState:      false,
		PrivacyPolicy:         "Integration compatibility is computed from static agent/provider profiles, conformance metadata, and advisor rules only. It excludes usage rows, prompt content, response content, message history, local paths, secrets, account identifiers, machine names, authors, and native session identifiers.",
		Request:               req,
		AgentProfilesHash:     AgentFrameworkProfilesFingerprint(),
		ProviderProfilesHash:  ProviderProfilesFingerprint(),
		RecommendationHash:    IntegrationRecommendationContractFingerprint(),
		ConformanceMatrixHash: AdapterConformanceMatrixFingerprint(),
		CanonicalSchemaHash:   storage.CanonicalEventSchemaFingerprint(),
		Rows:                  rows,
		QualityGates: []string{
			"treat compatibility as a preflight and still run strict conformance fixtures before enabling write ingest",
			"prefer provider-stream only when final usage chunks are available and privacy filters exclude stream deltas",
			"configure local pricing overrides for relay, enterprise contract, local runtime, and edge model billing",
			"keep gateway and outbound notification surfaces disabled until admission, policy, and deployment smoke checks pass",
			"do not persist prompt, response, message, artifact body, header, credential, local path, or raw account metadata in adapters",
		},
		OperationalGuidance: []string{
			"use this matrix to choose adapter fixture families for wrapper CI",
			"pin agent/provider/recommendation/conformance hashes when shipping third-party adapters",
			"start with canonical-events for new frameworks, then add provider envelope or stream telemetry for exact model calls",
			"use A2A or workload heartbeats when the runtime executes asynchronous goal/context workloads",
			"use OpenTelemetry GenAI when the runtime already emits spans and can omit content-bearing attributes",
		},
	}
	report.Summary = summarizeIntegrationCompatibility(agents, providers, rows)
	report.CompatibilityHash = IntegrationCompatibilityFingerprintFrom(report)
	return report
}

func IntegrationCompatibilityFingerprint(req IntegrationCompatibilityRequest) string {
	report := IntegrationCompatibilityReportFor(req)
	return IntegrationCompatibilityFingerprintFrom(report)
}

func IntegrationCompatibilityFingerprintFrom(report IntegrationCompatibilityReport) string {
	report.CompatibilityHash = ""
	return hashJSONPayload(report)
}

func integrationCompatibilityRow(agent AgentFrameworkProfile, provider ProviderProfile, requestedSurface string) IntegrationCompatibilityRow {
	candidates := compatibilityCandidateSurfaces(agent, provider)
	surface := requestedSurface
	if surface == "" || !containsString(candidates, surface) {
		surface = compatibilityPreferredSurface(candidates, agent, provider)
	}
	recommendation := IntegrationRecommendation(IntegrationRecommendationRequest{
		AgentProfileID:    agent.ID,
		ProviderProfileID: provider.ID,
		Surface:           surface,
	})
	status := "ready"
	if recommendation.Confidence < 0.80 || surface == "gateway" {
		status = "review-required"
	}
	if recommendation.Confidence < 0.65 || len(recommendation.StrictCI) == 0 {
		status = "limited"
	}
	reasons := []string{
		"agent kind " + agent.Kind,
		"provider kind " + provider.Kind,
		"surface " + surface,
		"confidence " + strconv.FormatFloat(recommendation.Confidence, 'f', 2, 64),
	}
	if len(recommendation.StrictCI) > 0 {
		reasons = append(reasons, "strict conformance available")
	}
	if containsString(agent.CollectorSources, agent.ID) || surface == "local-collector" {
		reasons = append(reasons, "local collector available")
	}
	if containsString(provider.Families, "relay") || containsString(provider.Families, "local-model") || containsString(provider.Families, "edge-model") {
		reasons = append(reasons, "pricing override likely required")
	}
	return IntegrationCompatibilityRow{
		AgentProfileID:       agent.ID,
		AgentLabel:           agent.Label,
		ProviderProfileID:    provider.ID,
		ProviderLabel:        provider.Label,
		RecommendedSurface:   surface,
		CandidateSurfaces:    candidates,
		Status:               status,
		RiskLevel:            compatibilityRiskLevel(surface, provider),
		Confidence:           recommendation.Confidence,
		StrictCI:             recommendation.StrictCI,
		Validation:           recommendation.Validation,
		ExpectedEventTypes:   recommendation.ExpectedEventTypes,
		RequiredSignals:      recommendation.RequiredSignals,
		ConformanceKinds:     recommendation.RelatedProfiles.ConformanceKinds,
		CompatibilityReasons: uniqueSortedStrings(reasons),
		Limitations:          recommendation.Limitations,
		NextSteps:            recommendation.NextSteps,
	}
}

func compatibilityCandidateSurfaces(agent AgentFrameworkProfile, provider ProviderProfile) []string {
	candidates := []string{}
	for _, surface := range agent.SupportedSurfaces {
		norm := normalizeRecommendationSurface(surface)
		if norm != "" {
			candidates = append(candidates, norm)
		}
	}
	for _, kind := range provider.AcceptedInputKinds {
		switch normalizeRecommendationSurface(kind) {
		case "provider-envelope":
			if containsString(agent.AdapterInputKinds, "provider") || containsString(candidates, "provider-envelope") {
				candidates = append(candidates, "provider-envelope")
			}
		case "provider-stream":
			if containsString(agent.AdapterInputKinds, "provider-stream") || containsString(candidates, "provider-stream") {
				candidates = append(candidates, "provider-stream")
			}
		case "opentelemetry":
			if containsString(agent.AdapterInputKinds, "otel") || containsString(candidates, "opentelemetry") {
				candidates = append(candidates, "opentelemetry")
			}
		}
	}
	if len(provider.GatewayRoutes) > 0 && (containsString(candidates, "provider-envelope") || containsString(candidates, "provider-stream")) {
		candidates = append(candidates, "gateway")
	}
	if len(agent.CollectorSources) > 0 {
		candidates = append(candidates, "local-collector")
	}
	if containsString(agent.CanonicalEventTypes, "workload.started") || containsString(agent.SupportedSurfaces, "canonical-events") {
		candidates = append(candidates, "canonical-events")
	}
	return uniqueSortedStrings(candidates)
}

func compatibilityPreferredSurface(candidates []string, agent AgentFrameworkProfile, provider ProviderProfile) string {
	preferences := []string{"provider-stream", "provider-envelope", "opentelemetry", "a2a", "local-collector", "canonical-events", "mcp-stdio", "gateway"}
	if strings.Contains(provider.Kind, "local") {
		preferences = []string{"provider-envelope", "provider-stream", "opentelemetry", "local-collector", "canonical-events", "mcp-stdio", "gateway"}
	}
	if len(agent.CollectorSources) > 0 && !containsString(agent.AdapterInputKinds, "provider-stream") {
		preferences = []string{"local-collector", "provider-envelope", "opentelemetry", "canonical-events", "mcp-stdio", "gateway"}
	}
	for _, surface := range preferences {
		if containsString(candidates, surface) {
			return surface
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	return "canonical-events"
}

func compatibilityRiskLevel(surface string, provider ProviderProfile) string {
	if surface == "gateway" || strings.Contains(provider.Kind, "relay") {
		return "high"
	}
	if strings.Contains(provider.Kind, "local") || strings.Contains(provider.Kind, "edge") {
		return "medium"
	}
	if surface == "provider-stream" || surface == "opentelemetry" {
		return "medium"
	}
	return "low"
}

func summarizeIntegrationCompatibility(agents []AgentFrameworkProfile, providers []ProviderProfile, rows []IntegrationCompatibilityRow) IntegrationCompatibilitySummary {
	summary := IntegrationCompatibilitySummary{
		AgentProfiles:    len(agents),
		ProviderProfiles: len(providers),
		Rows:             len(rows),
	}
	for _, row := range rows {
		switch row.Status {
		case "ready":
			summary.Ready++
		case "review-required":
			summary.ReviewRequired++
		default:
			summary.Limited++
		}
		if row.Confidence >= 0.80 {
			summary.HighConfidence++
		}
		if len(row.StrictCI) > 0 {
			summary.StrictCIAvailable++
		}
		if row.RecommendedSurface == "local-collector" || row.RecommendedSurface == "canonical-events" || row.RecommendedSurface == "mcp-stdio" {
			summary.LocalOnlySurfaces++
		}
		if row.RecommendedSurface == "gateway" {
			summary.OutboundSurfaces++
		}
		if row.RiskLevel == "high" && (strings.Contains(row.ProviderProfileID, "relay") || strings.Contains(row.ProviderProfileID, "openrouter") || strings.Contains(row.ProviderProfileID, "litellm")) {
			summary.UnpricedRiskProfiles++
		}
	}
	return summary
}

func compatibilityMinConfidence(raw string) float64 {
	if strings.TrimSpace(raw) == "" {
		return 0
	}
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}
