package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"sort"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// IntegrationRecommendationRequest describes a read-only advisor query for
// adapter authors, wrappers, routers, and framework integrations.
type IntegrationRecommendationRequest struct {
	AgentProfileID    string   `json:"agent_profile_id,omitempty"`
	ProviderProfileID string   `json:"provider_profile_id,omitempty"`
	Surface           string   `json:"surface,omitempty"`
	Signals           []string `json:"signals,omitempty"`
	RuntimeMode       string   `json:"runtime_mode,omitempty"`
	ReadOnly          bool     `json:"read_only"`
}

// IntegrationRecommendationReport is a deterministic, metadata-only advisor
// response. It does not read SQLite and does not write local state.
type IntegrationRecommendationReport struct {
	Product                string                              `json:"product"`
	Contract               string                              `json:"contract"`
	Version                string                              `json:"version"`
	LocalFirst             bool                                `json:"local_first"`
	ReadOnlySafe           bool                                `json:"read_only_safe"`
	WritesLocalState       bool                                `json:"writes_local_state"`
	PrivacyPolicy          string                              `json:"privacy_policy"`
	Request                IntegrationRecommendationRequest    `json:"request"`
	MatchedAgentProfile    *AgentFrameworkProfile              `json:"matched_agent_profile,omitempty"`
	MatchedProviderProfile *ProviderProfile                    `json:"matched_provider_profile,omitempty"`
	RecommendedSurface     string                              `json:"recommended_surface"`
	Recommendation         string                              `json:"recommendation"`
	Confidence             float64                             `json:"confidence"`
	IngestPath             string                              `json:"ingest_path"`
	FallbackPaths          []string                            `json:"fallback_paths,omitempty"`
	HTTP                   []string                            `json:"http,omitempty"`
	CLI                    []string                            `json:"cli,omitempty"`
	MCPTools               []string                            `json:"mcp_tools,omitempty"`
	MCPResources           []string                            `json:"mcp_resources,omitempty"`
	Validation             []string                            `json:"validation"`
	StrictCI               []string                            `json:"strict_ci"`
	ExpectedEventTypes     []string                            `json:"expected_event_types"`
	RequiredSignals        []string                            `json:"required_signals"`
	MissingSignals         []string                            `json:"missing_signals,omitempty"`
	PrivacyChecklist       []string                            `json:"privacy_checklist"`
	QualityGates           []string                            `json:"quality_gates"`
	Limitations            []string                            `json:"limitations,omitempty"`
	Hashes                 map[string]string                   `json:"hashes"`
	NextSteps              []string                            `json:"next_steps"`
	RelatedProfiles        IntegrationRecommendationProfileRef `json:"related_profiles"`
}

// IntegrationRecommendationProfileRef keeps the payload compact while still
// letting callers pin the source catalogs used by the advisor.
type IntegrationRecommendationProfileRef struct {
	AgentProfileIDs    []string `json:"agent_profile_ids,omitempty"`
	ProviderProfileIDs []string `json:"provider_profile_ids,omitempty"`
	ConformanceKinds   []string `json:"conformance_kinds,omitempty"`
}

// IntegrationRecommendationFromValues builds a request from REST or MCP query
// parameters. Aliases are accepted to make wrapper integration easier.
func IntegrationRecommendationFromValues(values url.Values) IntegrationRecommendationRequest {
	signals := splitRecommendationSignals(firstNonEmptyRecommendation(
		values.Get("signals"),
		values.Get("signal"),
		values.Get("available_signals"),
	))
	req := IntegrationRecommendationRequest{
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
		Surface: firstNonEmptyRecommendation(
			values.Get("surface"),
			values.Get("ingest"),
			values.Get("kind"),
		),
		Signals:     signals,
		RuntimeMode: firstNonEmptyRecommendation(values.Get("runtime_mode"), values.Get("mode")),
		ReadOnly:    recommendationBool(values, "read_only") || recommendationBool(values, "readonly"),
	}
	return NormalizeIntegrationRecommendationRequest(req)
}

// NormalizeIntegrationRecommendationRequest returns a stable request shape.
func NormalizeIntegrationRecommendationRequest(req IntegrationRecommendationRequest) IntegrationRecommendationRequest {
	req.AgentProfileID = normalizeRecommendationToken(req.AgentProfileID)
	req.ProviderProfileID = normalizeRecommendationToken(req.ProviderProfileID)
	req.Surface = normalizeRecommendationSurface(req.Surface)
	req.RuntimeMode = strings.TrimSpace(strings.ToLower(req.RuntimeMode))
	req.Signals = normalizeRecommendationSignals(req.Signals)
	return req
}

// IntegrationRecommendation returns a deterministic advisor report. It is pure
// function logic over static catalogs and can be used in read-only deployments.
func IntegrationRecommendation(req IntegrationRecommendationRequest) IntegrationRecommendationReport {
	req = NormalizeIntegrationRecommendationRequest(req)
	agent, agentOK := findAgentProfile(req.AgentProfileID)
	provider, providerOK := findProviderProfile(req.ProviderProfileID)
	surface := selectRecommendationSurface(req.Surface, agent, agentOK, provider, providerOK)
	matrixKind, matrixOK := findConformanceMatrixKind(surface)

	expectedEvents := []string{}
	requiredSignals := []string{}
	validation := []string{}
	strictCI := []string{}
	http := []string{}
	cli := []string{}
	mcpTools := []string{}
	mcpResources := []string{}
	fallbackPaths := []string{}
	qualityGates := []string{
		"validate fixtures before enabling write ingest",
		"keep raw request and response content outside Agent Ledger persistence",
		"preserve non-overlapping token counters and explicit confidence labels",
	}
	privacyChecklist := []string{
		"do not persist prompt content, response content, message histories, artifact bodies, raw headers, or durable secrets",
		"use project aliases or hashes before sharing reports outside the machine",
		"treat native identifiers as scoped metadata and redact them in screenshot or team-share mode",
		"keep outbound notification adapters disabled unless explicitly configured",
	}
	limitations := []string{}

	if agentOK {
		expectedEvents = append(expectedEvents, agent.CanonicalEventTypes...)
		requiredSignals = append(requiredSignals, agent.AvailableSignals...)
		fallbackPaths = append(fallbackPaths, agent.FallbackIngest...)
		http = append(http, agent.HTTPEndpoints...)
		cli = append(cli, agent.CLICommands...)
		mcpTools = append(mcpTools, agent.MCPTools...)
		mcpResources = append(mcpResources, agent.MCPResources...)
		qualityGates = append(qualityGates, agent.QualityGates...)
		privacyChecklist = append(privacyChecklist, agent.PrivacyNotes...)
		limitations = append(limitations, agent.Limitations...)
	} else if req.AgentProfileID != "" {
		limitations = append(limitations, "agent profile was not found; recommendation uses generic adapter surfaces with lower confidence")
	}

	if providerOK {
		requiredSignals = append(requiredSignals, provider.UsageSchemas...)
		fallbackPaths = append(fallbackPaths, "provider profile "+provider.ID+": "+provider.PricingStrategy)
		privacyChecklist = append(privacyChecklist, provider.PrivacyNotes...)
		limitations = append(limitations, provider.Limitations...)
	} else if req.ProviderProfileID != "" {
		limitations = append(limitations, "provider profile was not found; configure local pricing overrides and conformance fixtures before production use")
	}

	if matrixOK {
		expectedEvents = append(expectedEvents, matrixKind.ExpectedEventTypes...)
		requiredSignals = append(requiredSignals, matrixKind.RequiredSignals...)
		validation = append(validation, matrixKind.Endpoint, matrixKind.CLICommand)
		strictCI = append(strictCI, matrixKind.StrictCICommand)
		if matrixKind.ConvertCommand != "" {
			cli = append(cli, matrixKind.ConvertCommand)
		}
		if matrixKind.IngestCommand != "" {
			cli = append(cli, matrixKind.IngestCommand)
		}
		qualityGates = append(qualityGates, matrixKind.PrivacyNotes...)
	} else {
		validation = append(validation, "agent-ledger event validate")
		strictCI = append(strictCI, "agent-ledger event validate --file event.json")
		limitations = append(limitations, "selected surface has no dedicated conformance matrix kind; use canonical event validation and profile quality gates")
	}

	surfacePlan := recommendationSurfacePlan(surface, agent, agentOK, provider, providerOK)
	http = append(http, surfacePlan.HTTP...)
	cli = append(cli, surfacePlan.CLI...)
	mcpTools = append(mcpTools, surfacePlan.MCPTools...)
	mcpResources = append(mcpResources, surfacePlan.MCPResources...)
	validation = append(validation, surfacePlan.Validation...)
	strictCI = append(strictCI, surfacePlan.StrictCI...)
	fallbackPaths = append(fallbackPaths, surfacePlan.FallbackPaths...)
	expectedEvents = append(expectedEvents, surfacePlan.ExpectedEventTypes...)
	requiredSignals = append(requiredSignals, surfacePlan.RequiredSignals...)
	qualityGates = append(qualityGates, surfacePlan.QualityGates...)
	limitations = append(limitations, surfacePlan.Limitations...)

	expectedEvents = uniqueSortedStrings(expectedEvents)
	requiredSignals = uniqueSortedStrings(requiredSignals)
	validation = uniqueSortedStrings(validation)
	strictCI = uniqueSortedStrings(strictCI)
	http = uniqueSortedStrings(http)
	cli = uniqueSortedStrings(cli)
	mcpTools = uniqueSortedStrings(mcpTools)
	mcpResources = uniqueSortedStrings(mcpResources)
	fallbackPaths = uniqueSortedStrings(fallbackPaths)
	privacyChecklist = uniqueSortedStrings(privacyChecklist)
	qualityGates = uniqueSortedStrings(qualityGates)
	limitations = uniqueSortedStrings(limitations)
	fallbackPaths = sanitizeRecommendationStrings(fallbackPaths)
	privacyChecklist = sanitizeRecommendationStrings(privacyChecklist)
	qualityGates = sanitizeRecommendationStrings(qualityGates)
	limitations = sanitizeRecommendationStrings(limitations)

	missingSignals := recommendationMissingSignals(req.Signals, requiredSignals)
	confidence := recommendationConfidence(agentOK, providerOK, matrixOK, surface, req.Signals, missingSignals)
	recommendation := sanitizeRecommendationText(recommendationSummary(surface, agent, agentOK, provider, providerOK, matrixOK))
	ingestPath := sanitizeRecommendationText(recommendationIngestPath(surface, agent, agentOK, matrixKind, matrixOK))

	related := IntegrationRecommendationProfileRef{
		ConformanceKinds: conformanceKindsForSurface(surface),
	}
	if agentOK {
		related.AgentProfileIDs = append(related.AgentProfileIDs, agent.ID)
	}
	if providerOK {
		related.ProviderProfileIDs = append(related.ProviderProfileIDs, provider.ID)
	}

	return IntegrationRecommendationReport{
		Product:                "Agent Ledger",
		Contract:               "agent-ledger.integration-recommendation",
		Version:                "v1",
		LocalFirst:             true,
		ReadOnlySafe:           true,
		WritesLocalState:       false,
		PrivacyPolicy:          "Integration recommendations are computed from static metadata catalogs and request parameters only. They do not read or write usage rows and do not include prompt content, response content, message histories, raw local paths, secrets, account identifiers, machine names, or authors.",
		Request:                req,
		MatchedAgentProfile:    sanitizedAgentPointer(agent, agentOK),
		MatchedProviderProfile: sanitizedProviderPointer(provider, providerOK),
		RecommendedSurface:     surface,
		Recommendation:         recommendation,
		Confidence:             confidence,
		IngestPath:             ingestPath,
		FallbackPaths:          fallbackPaths,
		HTTP:                   http,
		CLI:                    cli,
		MCPTools:               mcpTools,
		MCPResources:           mcpResources,
		Validation:             validation,
		StrictCI:               strictCI,
		ExpectedEventTypes:     expectedEvents,
		RequiredSignals:        requiredSignals,
		MissingSignals:         missingSignals,
		PrivacyChecklist:       privacyChecklist,
		QualityGates:           qualityGates,
		Limitations:            limitations,
		Hashes: map[string]string{
			"agent_profiles":          AgentFrameworkProfilesFingerprint(),
			"provider_profiles":       ProviderProfilesFingerprint(),
			"adapter_contract":        AdapterContractFingerprint(),
			"conformance_matrix":      AdapterConformanceMatrixFingerprint(),
			"canonical_event_schema":  storage.CanonicalEventSchemaFingerprint(),
			"recommendation_contract": IntegrationRecommendationContractFingerprint(),
		},
		NextSteps:       sanitizeRecommendationStrings(recommendationNextSteps(surface, matrixOK, agentOK, providerOK, len(missingSignals) == 0)),
		RelatedProfiles: related,
	}
}

func IntegrationRecommendationContractFingerprint() string {
	payload := map[string]interface{}{
		"contract":    "agent-ledger.integration-recommendation",
		"version":     "v1",
		"entrypoints": []string{"/api/integrations/recommendation", "agent-ledger agent recommend", "ledger.integration_recommendation", "agent-ledger://integrations/recommendation"},
		"inputs":      []string{"agent_profile_id", "provider_profile_id", "surface", "signals", "runtime_mode", "read_only"},
		"hashes":      []string{"agent_profiles", "provider_profiles", "adapter_contract", "conformance_matrix", "canonical_event_schema"},
		"privacy":     "static metadata and request parameters only; no local usage rows, prompts, responses, secrets, raw paths, machine names, or authors",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

type recommendationSurfaceDetails struct {
	HTTP               []string
	CLI                []string
	MCPTools           []string
	MCPResources       []string
	Validation         []string
	StrictCI           []string
	FallbackPaths      []string
	ExpectedEventTypes []string
	RequiredSignals    []string
	QualityGates       []string
	Limitations        []string
}

func recommendationSurfacePlan(surface string, agent AgentFrameworkProfile, agentOK bool, provider ProviderProfile, providerOK bool) recommendationSurfaceDetails {
	switch surface {
	case "local-collector":
		sources := []string{"<source>"}
		if agentOK && len(agent.CollectorSources) > 0 {
			sources = agent.CollectorSources
		}
		commands := []string{"agent-ledger doctor", "agent-ledger readiness"}
		for _, source := range sources {
			commands = append(commands, "agent-ledger admission check --surface http --method POST --path /api/scan --role operator")
			commands = append(commands, "agent-ledger agent recommend --profile "+firstNonEmptyRecommendation(agent.ID, "agent")+" --surface local-collector")
			_ = source
		}
		return recommendationSurfaceDetails{
			HTTP:               []string{"GET /api/health/ingestion", "POST /api/scan?source=<source>"},
			CLI:                commands,
			Validation:         []string{"agent-ledger doctor --format markdown", "agent-ledger readiness --format markdown"},
			StrictCI:           []string{"go test ./internal/collector/..."},
			ExpectedEventTypes: []string{"model.call", "context.ref"},
			RequiredSignals:    []string{"usage counters", "timestamp", "model name", "project attribution"},
			QualityGates:       []string{"collector health must expose path reachability, watermark, inserted rows, and last error"},
		}
	case "provider-stream":
		return recommendationSurfaceDetails{
			HTTP:               []string{"POST /api/integrations/conformance?kind=provider-stream&strict=true", "POST /api/provider/calls"},
			CLI:                []string{"agent-ledger adapter conformance --kind provider-stream --strict --file fixture.sse", "agent-ledger provider convert --file response.json"},
			Validation:         []string{"agent-ledger adapter conformance --kind provider-stream --strict --file fixture.sse"},
			StrictCI:           []string{"agent-ledger adapter conformance --kind provider-stream --strict --file examples/adapter-fixtures/provider-openai-chat-stream.sse"},
			ExpectedEventTypes: []string{"model.call"},
			RequiredSignals:    []string{"model name", "usage counters", "final usage chunk"},
			QualityGates:       []string{"streaming adapters must recover final usage without persisting stream deltas"},
		}
	case "provider-envelope":
		extra := []string{}
		if providerOK {
			extra = append(extra, "configure pricing profile for "+provider.ID)
		}
		return recommendationSurfaceDetails{
			HTTP:               []string{"POST /api/integrations/conformance?kind=provider&strict=true", "POST /api/provider/calls"},
			CLI:                append([]string{"agent-ledger provider convert --file response.json", "agent-ledger adapter conformance --kind provider --strict --file fixture.json"}, extra...),
			Validation:         []string{"agent-ledger adapter conformance --kind provider --strict --file fixture.json"},
			StrictCI:           []string{"agent-ledger adapter conformance --kind provider --strict --file examples/adapter-fixtures/provider-openai-chat-completion.json"},
			ExpectedEventTypes: []string{"model.call", "context.ref"},
			RequiredSignals:    []string{"model name", "usage counters"},
			QualityGates:       []string{"source-reported cost is evidence; pricing governance remains authoritative"},
		}
	case "opentelemetry":
		return recommendationSurfaceDetails{
			HTTP:               []string{"POST /api/otel/genai", "POST /api/integrations/conformance?kind=otel&strict=true"},
			CLI:                []string{"agent-ledger otel convert --file spans.json", "agent-ledger adapter conformance --kind otel --strict --file spans.json"},
			Validation:         []string{"agent-ledger adapter conformance --kind otel --strict --file spans.json"},
			StrictCI:           []string{"agent-ledger adapter conformance --kind otel --strict --file examples/adapter-fixtures/otel-genai-span.json"},
			ExpectedEventTypes: []string{"model.call", "context.ref"},
			RequiredSignals:    []string{"span timing", "model name", "usage counters"},
			QualityGates:       []string{"GenAI span attributes carrying prompt or completion content must be rejected or ignored"},
		}
	case "a2a":
		return recommendationSurfaceDetails{
			HTTP:               []string{"POST /api/a2a/tasks", "GET /.well-known/agent-ledger.json", "POST /api/integrations/conformance?kind=a2a&strict=true"},
			CLI:                []string{"agent-ledger a2a convert --file task.json", "agent-ledger adapter conformance --kind a2a --strict --file task.json"},
			Validation:         []string{"agent-ledger adapter conformance --kind a2a --strict --file task.json"},
			StrictCI:           []string{"agent-ledger adapter conformance --kind a2a --strict --file examples/adapter-fixtures/a2a-delegated-task.json"},
			ExpectedEventTypes: []string{"workload.started", "workload.linked", "agent.run.started", "agent.run.heartbeat", "agent.run.finished", "context.ref", "artifact.created", "evaluation.recorded"},
			RequiredSignals:    []string{"task lifecycle", "delegated parent reference", "context reference", "artifact metadata"},
			QualityGates:       []string{"Agent Ledger is a local telemetry adapter, not a remote task execution server"},
		}
	case "mcp-stdio":
		return recommendationSurfaceDetails{
			CLI:                []string{"agent-ledger mcp", "agent-ledger admission check --surface mcp --tool ledger.discovery --role viewer"},
			MCPTools:           []string{"ledger.discovery", "ledger.contracts", "ledger.agent_profiles", "ledger.provider_profiles", "ledger.integration_recommendation", "ledger.admission_check"},
			MCPResources:       []string{"agent-ledger://discovery/manifest", "agent-ledger://integrations/agent-profiles", "agent-ledger://integrations/provider-profiles", "agent-ledger://integrations/recommendation"},
			Validation:         []string{"agent-ledger contracts verify", "agent-ledger admission check --surface cli --command \"agent-ledger mcp\" --role operator"},
			StrictCI:           []string{"agent-ledger contracts verify"},
			ExpectedEventTypes: []string{"workload.started", "agent.run.started", "agent.run.heartbeat", "model.call", "tool.call", "context.ref", "artifact.created", "evaluation.recorded"},
			RequiredSignals:    []string{"runtime status", "admission result", "contract hashes"},
			QualityGates:       []string{"MCP write tools must be denied in read-only mode"},
		}
	case "gateway":
		return recommendationSurfaceDetails{
			HTTP:               []string{"POST /gateway/openai/v1/chat/completions", "POST /gateway/openai/v1/responses", "POST /gateway/anthropic/v1/messages"},
			CLI:                []string{"agent-ledger provider profiles", "agent-ledger policy evaluate --action model.call"},
			Validation:         []string{"agent-ledger adapter conformance --kind provider --strict --file fixture.json"},
			StrictCI:           []string{"agent-ledger contracts verify"},
			ExpectedEventTypes: []string{"model.call", "policy.decision"},
			RequiredSignals:    []string{"model name", "usage counters", "pricing source"},
			QualityGates:       []string{"gateway forwarding must be explicitly enabled and should remain loopback-bound"},
			Limitations:        []string{"live gateway surfaces may contact upstream providers when enabled; the recommendation report itself does not"},
		}
	default:
		return recommendationSurfaceDetails{
			HTTP:               []string{"POST /api/events/validate", "POST /api/events"},
			CLI:                []string{"agent-ledger event validate --file event.json", "agent-ledger event ingest --file event.json"},
			Validation:         []string{"agent-ledger event validate --file event.json"},
			StrictCI:           []string{"agent-ledger event validate --file event.json"},
			ExpectedEventTypes: []string{"workload.started", "agent.run.started", "model.call", "tool.call", "context.ref", "artifact.created", "evaluation.recorded"},
			RequiredSignals:    []string{"source", "event type", "timestamp", "payload metadata"},
			QualityGates:       []string{"canonical payload validation must reject content-bearing keys before ingest"},
		}
	}
}

func findAgentProfile(id string) (AgentFrameworkProfile, bool) {
	if id == "" {
		return AgentFrameworkProfile{}, false
	}
	for _, profile := range AgentFrameworkProfiles().Profiles {
		if profile.ID == id || containsString(profile.Families, id) {
			return profile, true
		}
	}
	return AgentFrameworkProfile{}, false
}

func findProviderProfile(id string) (ProviderProfile, bool) {
	if id == "" {
		return ProviderProfile{}, false
	}
	for _, profile := range ProviderProfiles().Profiles {
		if profile.ID == id || containsString(profile.Families, id) {
			return profile, true
		}
		for _, example := range profile.ModelNameExamples {
			if strings.Contains(normalizeRecommendationToken(example), id) {
				return profile, true
			}
		}
	}
	return ProviderProfile{}, false
}

func findConformanceMatrixKind(surface string) (AdapterConformanceMatrixKind, bool) {
	kind := conformanceKindForSurface(surface)
	if kind == "" {
		return AdapterConformanceMatrixKind{}, false
	}
	for _, entry := range AdapterConformanceMatrixSpec().Kinds {
		if entry.ConformanceKind == kind {
			return entry, true
		}
	}
	return AdapterConformanceMatrixKind{}, false
}

func selectRecommendationSurface(requested string, agent AgentFrameworkProfile, agentOK bool, provider ProviderProfile, providerOK bool) string {
	if requested != "" {
		return requested
	}
	if agentOK {
		switch {
		case len(agent.CollectorSources) > 0:
			return "local-collector"
		case containsString(agent.AdapterInputKinds, "provider-stream"):
			return "provider-stream"
		case containsString(agent.AdapterInputKinds, "provider"):
			return "provider-envelope"
		case containsString(agent.AdapterInputKinds, "otel"):
			return "opentelemetry"
		case containsString(agent.AdapterInputKinds, "a2a"):
			return "a2a"
		case len(agent.MCPTools) > 0:
			return "mcp-stdio"
		}
	}
	if providerOK {
		if containsString(provider.AcceptedInputKinds, "provider-stream") {
			return "provider-stream"
		}
		if containsString(provider.AcceptedInputKinds, "provider") {
			return "provider-envelope"
		}
		if containsString(provider.AcceptedInputKinds, "otel") {
			return "opentelemetry"
		}
	}
	return "canonical-events"
}

func conformanceKindForSurface(surface string) string {
	switch normalizeRecommendationSurface(surface) {
	case "canonical-events":
		return "canonical"
	case "provider-envelope", "gateway":
		return "provider"
	case "provider-stream":
		return "provider-stream"
	case "opentelemetry":
		return "otel"
	case "a2a":
		return "a2a"
	default:
		return ""
	}
}

func conformanceKindsForSurface(surface string) []string {
	if kind := conformanceKindForSurface(surface); kind != "" {
		return []string{kind}
	}
	return nil
}

func recommendationIngestPath(surface string, agent AgentFrameworkProfile, agentOK bool, matrixKind AdapterConformanceMatrixKind, matrixOK bool) string {
	if agentOK && agent.RecommendedIngest != "" && surface == "local-collector" {
		return agent.RecommendedIngest
	}
	if matrixOK {
		return matrixKind.Endpoint
	}
	switch surface {
	case "mcp-stdio":
		return "agent-ledger mcp local stdio resources and tools"
	case "local-collector":
		return "local collector scan plus ingestion health diagnostics"
	default:
		return "canonical metadata-only event validation and ingest"
	}
}

func recommendationSummary(surface string, agent AgentFrameworkProfile, agentOK bool, provider ProviderProfile, providerOK, matrixOK bool) string {
	parts := []string{"use " + surface + " as the primary Agent Ledger integration surface"}
	if agentOK {
		parts = append(parts, "agent profile "+agent.ID+" matched")
	}
	if providerOK {
		parts = append(parts, "provider profile "+provider.ID+" matched")
	}
	if matrixOK {
		parts = append(parts, "strict conformance is available before enabling ingest")
	} else {
		parts = append(parts, "use canonical validation because no dedicated conformance kind exists")
	}
	return strings.Join(parts, "; ")
}

func recommendationNextSteps(surface string, matrixOK, agentOK, providerOK, noMissing bool) []string {
	steps := []string{
		"pin the returned contract hashes in wrapper CI",
		"run the validation command with a privacy-safe fixture",
		"enable write ingest only after admission and strict checks pass",
	}
	if !agentOK {
		steps = append(steps, "select a supported agent profile or add a static profile before release")
	}
	if !providerOK && (surface == "provider-envelope" || surface == "provider-stream" || surface == "gateway") {
		steps = append(steps, "select a provider profile and configure local price overrides for non-official billing")
	}
	if !matrixOK {
		steps = append(steps, "add a conformance fixture if this surface becomes a stable adapter")
	}
	if !noMissing {
		steps = append(steps, "add or map missing signals before marking usage exact")
	}
	return uniqueSortedStrings(steps)
}

func recommendationConfidence(agentOK, providerOK, matrixOK bool, surface string, signals, missing []string) float64 {
	confidence := 0.45
	if agentOK {
		confidence += 0.20
	}
	if providerOK {
		confidence += 0.12
	}
	if matrixOK {
		confidence += 0.18
	}
	if surface != "" {
		confidence += 0.05
	}
	if len(signals) > 0 && len(missing) == 0 {
		confidence += 0.05
	}
	if len(missing) > 0 {
		confidence -= 0.10
	}
	if confidence < 0.10 {
		confidence = 0.10
	}
	if confidence > 0.98 {
		confidence = 0.98
	}
	return confidence
}

func recommendationMissingSignals(signals, required []string) []string {
	if len(signals) == 0 || len(required) == 0 {
		return nil
	}
	available := map[string]bool{}
	for _, signal := range signals {
		available[signal] = true
	}
	missing := []string{}
	for _, requiredSignal := range required {
		norm := normalizeRecommendationSignal(requiredSignal)
		if norm == "" || available[norm] {
			continue
		}
		if recommendationSignalCovered(norm, available) {
			continue
		}
		missing = append(missing, requiredSignal)
	}
	return uniqueSortedStrings(missing)
}

func recommendationSignalCovered(required string, available map[string]bool) bool {
	for signal := range available {
		if strings.Contains(required, signal) || strings.Contains(signal, required) {
			return true
		}
	}
	return false
}

func normalizeRecommendationSignals(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		for _, part := range strings.Split(value, ",") {
			norm := normalizeRecommendationSignal(part)
			if norm != "" {
				out = append(out, norm)
			}
		}
	}
	return uniqueSortedStrings(out)
}

func splitRecommendationSignals(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return normalizeRecommendationSignals(strings.Split(raw, ","))
}

func normalizeRecommendationSignal(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.Join(strings.Fields(value), " ")
	switch value {
	case "usage", "usage row", "usage rows", "token", "tokens", "token usage", "usage counter", "usage counters":
		return "usage counters"
	case "model", "model name", "models":
		return "model name"
	case "cache", "cache token", "cache tokens", "cache read", "cache write":
		return "cache counters"
	case "project", "project attribution", "repo", "repository":
		return "project attribution"
	case "timestamp", "time", "span timing":
		return "timestamp"
	case "final usage chunk", "stream usage":
		return "final usage chunk"
	default:
		return value
	}
}

func normalizeRecommendationToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.Join(strings.Fields(value), "-")
	return value
}

func normalizeRecommendationSurface(value string) string {
	value = normalizeRecommendationToken(value)
	switch value {
	case "", "auto":
		return ""
	case "collector", "local", "local-collector", "source-collector":
		return "local-collector"
	case "canonical", "canonical-event", "canonical-events", "events":
		return "canonical-events"
	case "provider", "provider-envelope", "provider-json", "envelope":
		return "provider-envelope"
	case "provider-stream", "stream", "sse", "streaming":
		return "provider-stream"
	case "otel", "opentelemetry", "otlp", "open-telemetry":
		return "opentelemetry"
	case "a2a", "agent-to-agent":
		return "a2a"
	case "mcp", "mcp-stdio", "mcp-tool", "resources":
		return "mcp-stdio"
	case "gateway", "provider-gateway", "router":
		return "gateway"
	default:
		return value
	}
}

func recommendationBool(values url.Values, key string) bool {
	switch strings.ToLower(strings.TrimSpace(values.Get(key))) {
	case "1", "true", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func firstNonEmptyRecommendation(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueSortedStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func agentPointer(profile AgentFrameworkProfile, ok bool) *AgentFrameworkProfile {
	if !ok {
		return nil
	}
	return &profile
}

func providerPointer(profile ProviderProfile, ok bool) *ProviderProfile {
	if !ok {
		return nil
	}
	return &profile
}

func sanitizedAgentPointer(profile AgentFrameworkProfile, ok bool) *AgentFrameworkProfile {
	if !ok {
		return nil
	}
	profile.RecommendedIngest = sanitizeRecommendationText(profile.RecommendedIngest)
	profile.FallbackIngest = sanitizeRecommendationStrings(profile.FallbackIngest)
	profile.AvailableSignals = sanitizeRecommendationStrings(profile.AvailableSignals)
	profile.PrivacyNotes = sanitizeRecommendationStrings(profile.PrivacyNotes)
	profile.QualityGates = sanitizeRecommendationStrings(profile.QualityGates)
	profile.Limitations = sanitizeRecommendationStrings(profile.Limitations)
	return &profile
}

func sanitizedProviderPointer(profile ProviderProfile, ok bool) *ProviderProfile {
	if !ok {
		return nil
	}
	profile.PricingStrategy = sanitizeRecommendationText(profile.PricingStrategy)
	profile.ReconciliationSupport = sanitizeRecommendationText(profile.ReconciliationSupport)
	profile.PrivacyNotes = sanitizeRecommendationStrings(profile.PrivacyNotes)
	profile.Limitations = sanitizeRecommendationStrings(profile.Limitations)
	return &profile
}

func sanitizeRecommendationStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if sanitized := sanitizeRecommendationText(value); sanitized != "" {
			out = append(out, sanitized)
		}
	}
	return uniqueSortedStrings(out)
}

func sanitizeRecommendationText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	replacements := []struct {
		from string
		to   string
	}{
		{"prompt, output, or transcript text", "prompt, output, or transcript content"},
		{"prompt, output, or transcript content", "request, generation, or transcript content"},
		{"prompt text", "prompt content"},
		{"response text", "response content"},
		{"output text", "output content"},
		{"transcript text", "transcript content"},
		{"session_id", "session identifier"},
		{"API keys", "API credentials"},
		{"api keys", "API credentials"},
	}
	for _, replacement := range replacements {
		value = strings.ReplaceAll(value, replacement.from, replacement.to)
	}
	return value
}
