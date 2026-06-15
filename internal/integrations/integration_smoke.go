package integrations

import (
	"strconv"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// IntegrationSmokeReport is a deterministic, privacy-safe rollout smoke report
// for ecosystem adapters. It combines static contract, readiness, signal, and
// fixture declaration checks without reading local usage rows or calling out.
type IntegrationSmokeReport struct {
	Product                  string                          `json:"product"`
	Contract                 string                          `json:"contract"`
	Version                  string                          `json:"version"`
	LocalFirst               bool                            `json:"local_first"`
	ReadOnlySafe             bool                            `json:"read_only_safe"`
	WritesLocalState         bool                            `json:"writes_local_state"`
	PrivacyPolicy            string                          `json:"privacy_policy"`
	CatalogHash              string                          `json:"catalog_hash"`
	SignalCoverageHash       string                          `json:"signal_coverage_hash"`
	IntegrationReadinessHash string                          `json:"integration_readiness_hash"`
	RecommendationHash       string                          `json:"integration_recommendation_hash"`
	ConformanceMatrixHash    string                          `json:"conformance_matrix_hash"`
	OpenAPIHash              string                          `json:"openapi_hash"`
	RuntimeHash              string                          `json:"runtime_hash"`
	Runtime                  IntegrationSmokeRuntime         `json:"runtime"`
	Summary                  IntegrationSmokeSummary         `json:"summary"`
	FixtureCoverage          IntegrationSmokeFixtureCoverage `json:"fixture_coverage"`
	Checks                   []IntegrationSmokeCheck         `json:"checks"`
	CICommands               []string                        `json:"ci_commands"`
	QualityGates             []string                        `json:"quality_gates"`
	OperationalGuidance      []string                        `json:"operational_guidance"`
}

// IntegrationSmokeRuntime exposes only feature flags that affect adapter rollout.
type IntegrationSmokeRuntime struct {
	ReadOnly                bool   `json:"read_only"`
	RBACEnabled             bool   `json:"rbac_enabled"`
	WebhooksEnabled         bool   `json:"webhooks_enabled"`
	OTLPReceiverEnabled     bool   `json:"otlp_receiver_enabled"`
	OTLPReceiverGRPCEnabled bool   `json:"otlp_receiver_grpc_enabled"`
	GatewayEnabled          bool   `json:"gateway_enabled"`
	PricingMode             string `json:"pricing_mode"`
}

// IntegrationSmokeSummary contains stable pass/fail counts for CI and wrappers.
type IntegrationSmokeSummary struct {
	Status           string `json:"status"`
	TotalChecks      int    `json:"total_checks"`
	Passed           int    `json:"passed"`
	Warnings         int    `json:"warnings"`
	Failed           int    `json:"failed"`
	ReviewRequired   int    `json:"review_required"`
	DisabledByConfig int    `json:"disabled_by_config"`
}

// IntegrationSmokeFixtureCoverage mirrors the conformance matrix fixture family
// counts in a compact shape for rollout gates.
type IntegrationSmokeFixtureCoverage struct {
	InputKinds             int      `json:"input_kinds"`
	Fixtures               int      `json:"fixtures"`
	StrictFixtures         int      `json:"strict_fixtures"`
	ProviderFixtures       int      `json:"provider_fixtures"`
	ProviderStreamFixtures int      `json:"provider_stream_fixtures"`
	OTelFixtures           int      `json:"otel_fixtures"`
	A2AFixtures            int      `json:"a2a_fixtures"`
	CanonicalFixtures      int      `json:"canonical_fixtures"`
	ExpectedKinds          []string `json:"expected_kinds"`
}

// IntegrationSmokeCheck is one metadata-only smoke assertion.
type IntegrationSmokeCheck struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Evidence    string `json:"evidence"`
	Remediation string `json:"remediation,omitempty"`
	Command     string `json:"command,omitempty"`
}

// IntegrationSmokeReportFor returns an installable-binary-safe smoke report. It
// does not read repository fixture files at runtime; fixture execution remains a
// CI/test concern and this report exposes the exact commands to run.
func IntegrationSmokeReportFor(opts Options, runtime *storage.RuntimeStatus) IntegrationSmokeReport {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	catalog := Registry(opts)
	catalogHash := CatalogFingerprintFrom(catalog)
	coverage := SignalCoverage()
	readiness := IntegrationReadiness(opts)
	matrix := AdapterConformanceMatrixSpec()
	openAPIHash := OpenAPISmokeFingerprint(opts, runtime)
	runtimeHash := hashJSONPayload(runtime)
	recommendation := IntegrationRecommendation(IntegrationRecommendationRequest{
		AgentProfileID:    "codex-cli",
		ProviderProfileID: "openai-official",
		Surface:           "provider-stream",
		Signals:           []string{"model", "usage", "cache"},
	})
	a2aRecommendation := IntegrationRecommendation(IntegrationRecommendationRequest{
		AgentProfileID: "a2a-task-runtime",
		Surface:        "a2a",
		Signals:        []string{"task lifecycle", "context reference", "delegated parent reference"},
	})

	checks := []IntegrationSmokeCheck{}
	addCheck := func(id, category, severity string, ok bool, message, evidence, remediation, command string) {
		status := "pass"
		if !ok {
			status = "warning"
			if severity == "critical" {
				status = "fail"
			}
		}
		checks = append(checks, IntegrationSmokeCheck{
			ID:          id,
			Category:    category,
			Status:      status,
			Severity:    severity,
			Message:     message,
			Evidence:    evidence,
			Remediation: remediation,
			Command:     command,
		})
	}

	addCheck("contracts.catalog_hash", "contracts", "critical", catalogHash != "" && strings.HasPrefix(catalogHash, "sha256:"), "capability catalog has a stable hash", catalogHash, "regenerate the integration catalog before publishing contracts", "agent-ledger integrations")
	addCheck("conformance.kind_coverage", "adapter-conformance", "critical", matrix.Summary.InputKinds == len(SupportedAdapterConformanceKinds()), "conformance matrix covers every supported decoder family", "matrix_kinds="+strconv.Itoa(matrix.Summary.InputKinds)+",supported="+strconv.Itoa(len(SupportedAdapterConformanceKinds())), "add missing adapter kind coverage before enabling new ecosystem ingest", "agent-ledger adapter matrix")
	addCheck("conformance.fixture_declarations", "adapter-conformance", "critical", matrix.Summary.Fixtures >= 10 && matrix.Summary.StrictFixtures == matrix.Summary.Fixtures, "all shipped fixture declarations are strict CI candidates", "fixtures="+strconv.Itoa(matrix.Summary.Fixtures)+",strict="+strconv.Itoa(matrix.Summary.StrictFixtures), "mark release fixtures strict and include commands for every kind", "agent-ledger adapter matrix")
	fixturePrivacyOK, fixturePrivacyEvidence := smokeFixturePrivacyStatus(matrix)
	addCheck("conformance.fixture_privacy", "adapter-conformance", "critical", fixturePrivacyOK, "fixture declarations preserve metadata-only privacy language", fixturePrivacyEvidence, "remove local paths, credentials, request bodies, and generation bodies from fixtures", "go test ./internal/integrations -run TestAdapterConformance")
	addCheck("signal.coverage", "signal-coverage", "critical", coverage.Summary.UnknownSignalReferences == 0 && coverage.Summary.SignalsWithoutAdapterCoverage == 0, "adapter required signals are taxonomy-backed and covered", signalCoverageCheckSummary(coverage), "add missing taxonomy or adapter coverage before release", "agent-ledger signal-coverage")
	addCheck("readiness.no_blocked_gates", "readiness", "critical", readiness.Summary.Blocked == 0, "current runtime has no active blocked integration activation gates", "blocked="+strconv.Itoa(readiness.Summary.Blocked)+",failures="+strconv.Itoa(readiness.Summary.Failures), "resolve blocked readiness gates before production rollout", "agent-ledger integrations readiness")
	addCheck("readiness.experimental_review_visible", "readiness", "warning", readiness.Summary.Experimental > 0 && readiness.Summary.ReviewRequired > 0, "experimental surfaces are visible as review-required instead of silently ready", "experimental="+strconv.Itoa(readiness.Summary.Experimental)+",review_required="+strconv.Itoa(readiness.Summary.ReviewRequired), "keep local-preview surfaces behind explicit smoke and operator review", "agent-ledger integrations readiness")
	addCheck("readiness.optional_disabled_visible", "readiness", "warning", readiness.Summary.DisabledByConfig > 0, "disabled optional surfaces remain visible as disabled-by-config", "disabled_by_config="+strconv.Itoa(readiness.Summary.DisabledByConfig), "enable optional gateway, webhook, or OTLP surfaces only after local smoke review", "agent-ledger integrations readiness")
	addCheck("recommendation.codex_provider_stream", "recommendation", "critical", recommendation.RecommendedSurface == "provider-stream" && recommendation.Confidence >= 0.75 && len(recommendation.StrictCI) > 0, "Codex/OpenAI provider-stream recommendation returns strict validation steps", "surface="+recommendation.RecommendedSurface+",confidence="+smokeFloat(recommendation.Confidence)+",strict_ci="+strconv.Itoa(len(recommendation.StrictCI)), "repair provider, agent, or conformance profiles before recommending provider-stream ingest", "agent-ledger agent recommend --profile codex-cli --provider openai-official --surface provider-stream --signals model,usage,cache")
	addCheck("recommendation.a2a_lineage", "recommendation", "critical", a2aRecommendation.RecommendedSurface == "a2a" && len(a2aRecommendation.StrictCI) > 0 && stringSliceContains(a2aRecommendation.ExpectedEventTypes, "workload.started"), "A2A recommendation exposes lineage-capable validation", "surface="+a2aRecommendation.RecommendedSurface+",events="+strconv.Itoa(len(a2aRecommendation.ExpectedEventTypes))+",strict_ci="+strconv.Itoa(len(a2aRecommendation.StrictCI)), "repair A2A profile or conformance matrix before advertising agent-to-agent telemetry", "agent-ledger agent recommend --profile a2a-task-runtime --surface a2a")
	addCheck("runtime.outbound_disabled_by_default", "runtime", "warning", !opts.GatewayEnabled && !opts.WebhooksEnabled, "outbound gateway/webhook surfaces are disabled by default", "gateway_enabled="+boolString(opts.GatewayEnabled)+",webhooks_enabled="+boolString(opts.WebhooksEnabled), "review policy, auth, loopback binding, and redaction before enabling outbound behavior", "agent-ledger config status --format markdown")
	addCheck("runtime.otlp_grpc_parent_gate", "runtime", "critical", !opts.OTLPReceiverGRPCEnabled || opts.OTLPReceiverEnabled, "OTLP gRPC cannot be enabled without the parent OTLP receiver", "otlp_enabled="+boolString(opts.OTLPReceiverEnabled)+",grpc_enabled="+boolString(opts.OTLPReceiverGRPCEnabled), "enable integrations.otlp_receiver.enabled or disable grpc_enabled", "agent-ledger integrations readiness")
	addCheck("openapi.hash_present", "contracts", "critical", strings.HasPrefix(openAPIHash, "sha256:"), "OpenAPI control-plane surface has a stable non-recursive smoke hash", openAPIHash, "regenerate OpenAPI metadata before release", "agent-ledger openapi")
	addCheck("runtime.report_read_only_contract", "runtime", "critical", true, "integration smoke report is read-only and does not write local state", "read_only_safe=true,writes_local_state=false", "", "agent-ledger integrations smoke")

	report := IntegrationSmokeReport{
		Product:                  "Agent Ledger",
		Contract:                 "agent-ledger.integration-smoke",
		Version:                  "v1",
		LocalFirst:               !opts.GatewayEnabled && !opts.WebhooksEnabled,
		ReadOnlySafe:             true,
		WritesLocalState:         false,
		PrivacyPolicy:            "Integration smoke is metadata-only. It contains contract hashes, capability ids, fixture declaration counts, validation commands, and activation gate summaries only; it excludes usage rows, request content, generation content, local paths, secrets, account identifiers, machine names, authors, and native session identifiers.",
		CatalogHash:              catalogHash,
		SignalCoverageHash:       SignalCoverageFingerprint(),
		IntegrationReadinessHash: IntegrationReadinessFingerprint(opts),
		RecommendationHash:       IntegrationRecommendationContractFingerprint(),
		ConformanceMatrixHash:    AdapterConformanceMatrixFingerprint(),
		OpenAPIHash:              openAPIHash,
		RuntimeHash:              runtimeHash,
		Runtime: IntegrationSmokeRuntime{
			ReadOnly:                opts.ReadOnly,
			RBACEnabled:             opts.RBACEnabled,
			WebhooksEnabled:         opts.WebhooksEnabled,
			OTLPReceiverEnabled:     opts.OTLPReceiverEnabled,
			OTLPReceiverGRPCEnabled: opts.OTLPReceiverGRPCEnabled,
			GatewayEnabled:          opts.GatewayEnabled,
			PricingMode:             strings.TrimSpace(opts.PricingMode),
		},
		FixtureCoverage: IntegrationSmokeFixtureCoverage{
			InputKinds:             matrix.Summary.InputKinds,
			Fixtures:               matrix.Summary.Fixtures,
			StrictFixtures:         matrix.Summary.StrictFixtures,
			ProviderFixtures:       matrix.Summary.ProviderFixtures,
			ProviderStreamFixtures: matrix.Summary.ProviderStreamFixtures,
			OTelFixtures:           matrix.Summary.OTelFixtures,
			A2AFixtures:            matrix.Summary.A2AFixtures,
			CanonicalFixtures:      matrix.Summary.CanonicalFixtures,
			ExpectedKinds:          SupportedAdapterConformanceKinds(),
		},
		Checks: checks,
		CICommands: []string{
			"agent-ledger contracts verify",
			"agent-ledger integrations smoke",
			"agent-ledger adapter matrix",
			"agent-ledger signal-coverage",
			"agent-ledger integrations readiness",
			"go test ./internal/integrations ./internal/server ./internal/mcp ./internal/controlplane",
		},
		QualityGates: []string{
			"do not enable write ingest for a new adapter until strict conformance, signal coverage, readiness, and admission checks pass",
			"do not enable gateway, webhook, or OTLP receiver surfaces without explicit config and operator review",
			"keep runtime smoke checks metadata-only and independent of source checkout paths",
			"fixture CI must validate repository examples separately from this installable-binary report",
		},
		OperationalGuidance: []string{
			"use this report as the single local preflight for wrapper, router, and adapter rollout",
			"treat warnings as review items and failures as release blockers",
			"pin the returned hashes in integration CI when building external adapters",
			"run source-specific fixture tests in CI before claiming exact support for a new provider or agent framework",
		},
	}
	report.Summary = summarizeIntegrationSmoke(checks, readiness)
	return report
}

// IntegrationSmokeFingerprint returns a stable hash for the current runtime
// option and runtime-status view.
func IntegrationSmokeFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	return hashJSONPayload(IntegrationSmokeReportFor(opts, runtime))
}

func IntegrationSmokeFingerprintFrom(report IntegrationSmokeReport) string {
	return hashJSONPayload(report)
}

func summarizeIntegrationSmoke(checks []IntegrationSmokeCheck, readiness IntegrationReadinessReport) IntegrationSmokeSummary {
	summary := IntegrationSmokeSummary{
		TotalChecks:      len(checks),
		ReviewRequired:   readiness.Summary.ReviewRequired,
		DisabledByConfig: readiness.Summary.DisabledByConfig,
		Status:           "pass",
	}
	for _, check := range checks {
		switch check.Status {
		case "fail":
			summary.Failed++
		case "warning":
			summary.Warnings++
		default:
			summary.Passed++
		}
	}
	if summary.Failed > 0 {
		summary.Status = "fail"
	} else if summary.Warnings > 0 {
		summary.Status = "pass_with_warnings"
	}
	return summary
}

func smokeFixturePrivacyStatus(matrix AdapterConformanceMatrix) (bool, string) {
	missingPrivacy := 0
	unsafe := 0
	fixtures := 0
	for _, kind := range matrix.Kinds {
		for _, fixture := range kind.Fixtures {
			fixtures++
			privacy := strings.ToLower(strings.TrimSpace(fixture.Privacy))
			if privacy == "" {
				missingPrivacy++
				continue
			}
			for _, marker := range []string{"sk-", "api_key", "bearer ", "c:/users", "\\users\\", "webhook_url"} {
				if strings.Contains(privacy, marker) {
					unsafe++
					break
				}
			}
		}
	}
	ok := fixtures > 0 && missingPrivacy == 0 && unsafe == 0
	return ok, "fixtures=" + strconv.Itoa(fixtures) + ",missing_privacy=" + strconv.Itoa(missingPrivacy) + ",unsafe_markers=" + strconv.Itoa(unsafe)
}

func smokeFloat(v float64) string {
	return strconv.FormatFloat(v, 'f', 2, 64)
}
