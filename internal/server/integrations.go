package server

import (
	"bytes"
	"net/http"
	"strconv"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/controlplane"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
)

func (s *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	catalog := integrations.Registry(s.integrationOptions())
	writeJSONWithETag(w, r, catalog, integrations.CatalogFingerprintFrom(catalog))
}

func (s *Server) handleProviderProfiles(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	writeJSONWithETag(w, r, integrations.ProviderProfiles(), integrations.ProviderProfilesFingerprint())
}

func (s *Server) handleAgentProfiles(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	writeJSONWithETag(w, r, integrations.AgentFrameworkProfiles(), integrations.AgentFrameworkProfilesFingerprint())
}

func (s *Server) handleSignalTaxonomy(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	writeJSONWithETag(w, r, integrations.SignalTaxonomy(), integrations.SignalTaxonomyFingerprint())
}

func (s *Server) handleSignalCoverage(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	writeJSONWithETag(w, r, integrations.SignalCoverage(), integrations.SignalCoverageFingerprint())
}

func (s *Server) handleIntegrationReadiness(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	opts := s.integrationOptions()
	writeJSONWithETag(w, r, integrations.IntegrationReadiness(opts), integrations.IntegrationReadinessFingerprint(opts))
}

func (s *Server) handleIntegrationSmoke(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	opts := s.integrationOptions()
	runtime := s.runtimeStatus()
	report := integrations.IntegrationSmokeReportFor(opts, runtime)
	writeJSONWithETag(w, r, report, integrations.IntegrationSmokeFingerprintFrom(report))
}

func (s *Server) handleIntegrationCompatibility(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	req := integrations.IntegrationCompatibilityFromValues(r.URL.Query())
	report := integrations.IntegrationCompatibilityReportFor(req)
	writeJSONWithETag(w, r, report, integrations.IntegrationCompatibilityFingerprintFrom(report))
}

func (s *Server) handleIntegrationRolloutPlan(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	req := integrations.IntegrationRolloutFromValues(r.URL.Query())
	report := integrations.IntegrationRolloutPlanFor(req)
	writeJSONWithETag(w, r, report, integrations.IntegrationRolloutFingerprintFrom(report))
}

func (s *Server) handleIntegrationEvidenceKit(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	req := integrations.IntegrationEvidenceKitFromValues(r.URL.Query())
	report := integrations.IntegrationEvidenceKitFor(s.integrationOptions(), s.runtimeStatus(), req)
	writeJSONWithETag(w, r, report, integrations.IntegrationEvidenceKitFingerprintFrom(report))
}

func (s *Server) handleIntegrationDrift(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	req := integrations.IntegrationDriftFromValues(r.URL.Query())
	report := integrations.IntegrationDriftReportFor(s.integrationOptions(), s.runtimeStatus(), req)
	writeJSONWithETag(w, r, report, integrations.IntegrationDriftFingerprintFrom(report))
}

func (s *Server) handleIntegrationLockfile(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	report := integrations.IntegrationLockfileFor(s.integrationOptions(), s.runtimeStatus())
	writeJSONWithETag(w, r, report, integrations.IntegrationLockfileFingerprintFrom(report))
}

func (s *Server) handleIntegrationUpgradeGate(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	req := integrations.IntegrationUpgradeGateFromValues(r.URL.Query())
	report := integrations.IntegrationUpgradeGateFor(s.integrationOptions(), s.runtimeStatus(), req)
	writeJSONWithETag(w, r, report, integrations.IntegrationUpgradeGateFingerprintFrom(report))
}

func (s *Server) handleSchemaEvolutionGate(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	req := integrations.SchemaEvolutionGateFromValues(r.URL.Query())
	report := integrations.SchemaEvolutionGateFor(req)
	writeJSONWithETag(w, r, report, integrations.SchemaEvolutionGateFingerprintFrom(report))
}

func (s *Server) handleIntegrationRecommendation(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	req := integrations.IntegrationRecommendationFromValues(r.URL.Query())
	report := integrations.IntegrationRecommendation(req)
	etag, err := jsonPayloadETag(report)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSONWithETag(w, r, report, etag)
}

func (s *Server) handleConformanceMatrix(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	writeJSONWithETag(w, r, integrations.AdapterConformanceMatrixSpec(), integrations.AdapterConformanceMatrixFingerprint())
}

func (s *Server) handleGoalCoverage(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	report := integrations.GoalCoverageReportFor(s.integrationOptions(), s.runtimeStatus())
	writeJSONWithETag(w, r, report, integrations.GoalCoverageFingerprintFrom(report))
}

func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	manifest := integrations.Discovery(s.integrationOptions())
	etag, err := jsonPayloadETag(manifest)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSONWithETag(w, r, manifest, etag)
}

func (s *Server) handleContracts(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	opts := s.integrationOptions()
	runtime := s.runtimeStatus()
	bundle := integrations.ContractBundleFor(opts, runtime)
	writeJSONWithETag(w, r, bundle, bundle.BundleHash)
}

func (s *Server) handleContractVerification(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	opts := s.integrationOptions()
	runtime := s.runtimeStatus()
	report := integrations.ContractVerificationReportFor(opts, runtime)
	writeJSONWithETag(w, r, report, integrations.ContractVerificationFingerprintFrom(report))
}

func (s *Server) handleOpenAPI(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	opts := s.integrationOptions()
	runtime := s.runtimeStatus()
	spec := integrations.OpenAPISpecFor(opts, runtime)
	writeJSONWithETag(w, r, spec, integrations.OpenAPIFingerprint(opts, runtime))
}

func (s *Server) handleRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	status := s.runtimeStatus()
	etag, err := jsonPayloadETag(status)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSONWithETag(w, r, status, etag)
}

func (s *Server) handleConfigStatus(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	status := s.configStatus()
	etag, err := jsonPayloadETag(status)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSONWithETag(w, r, status, etag)
}

func (s *Server) handleReadiness(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	report := s.readinessReport()
	writeJSONWithETag(w, r, report, controlplane.ReadinessFingerprint(report))
}

func (s *Server) handleAdmissionCheck(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	decision := controlplane.EvaluateAdmission(
		controlplane.AdmissionInputFromValues(r.URL.Query(), s.admissionDefaults()),
		time.Now().UTC(),
	)
	writeJSONWithETag(w, r, decision, controlplane.AdmissionFingerprint(decision))
}

func (s *Server) handleAdapterSpec(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodGet) {
		return
	}
	writeJSONWithETag(w, r, integrations.AdapterContractSpec(), integrations.AdapterContractFingerprint())
}

func (s *Server) handleAdapterConformance(w http.ResponseWriter, r *http.Request) {
	if !requireHTTPMethod(w, r, http.MethodPost) {
		return
	}
	if !s.requireLocalOrAuth(w, r) {
		return
	}
	raw := bytes.Buffer{}
	if _, err := raw.ReadFrom(http.MaxBytesReader(w, r.Body, 4<<20)); err != nil {
		badRequest(w, err)
		return
	}
	strict, err := strconv.ParseBool(r.URL.Query().Get("strict"))
	if r.URL.Query().Get("strict") == "" {
		strict = false
		err = nil
	}
	if err != nil {
		badRequest(w, err)
		return
	}
	report, err := integrations.RunAdapterConformanceWithOptions(integrations.AdapterConformanceOptions{
		Kind:   r.URL.Query().Get("kind"),
		Strict: strict,
	}, raw.Bytes())
	if err != nil {
		badRequest(w, err)
		return
	}
	writeJSON(w, report)
}

func (s *Server) integrationOptions() integrations.Options {
	sources := make([]integrations.Source, 0, len(s.options.Sources))
	for _, source := range s.options.Sources {
		sources = append(sources, integrations.Source{
			Source:    source.Source,
			Enabled:   source.Enabled,
			PathCount: len(source.Paths),
		})
	}
	return integrations.Options{
		Sources:                 sources,
		PricingMode:             s.options.Pricing.Mode,
		PoliciesEnabled:         s.options.Policies.Enabled,
		RBACEnabled:             s.options.RBAC.Enabled,
		ReadOnly:                s.options.RBAC.ReadOnly,
		QuotaEnabled:            s.options.Quota.Enabled,
		WebhooksEnabled:         s.options.Webhooks.Enabled,
		OTLPReceiverEnabled:     s.options.Integrations.OTLPReceiver.Enabled,
		OTLPReceiverGRPCEnabled: s.options.Integrations.OTLPReceiver.GRPCEnabled,
		GatewayEnabled:          s.options.Gateway.Enabled,
	}
}

func (s *Server) configStatus() *config.ConfigStatusReport {
	if s.options.ConfigStatus != nil {
		report := *s.options.ConfigStatus
		return &report
	}
	return config.StatusReport(s.statusConfig())
}

func (s *Server) readinessReport() *controlplane.ReadinessReport {
	runtime := s.runtimeStatus()
	return controlplane.BuildReadinessReport(
		s.db,
		s.statusConfig(),
		runtime,
		integrations.ContractVerificationReportFor(s.integrationOptions(), runtime),
		time.Now().UTC(),
	)
}

func (s *Server) statusConfig() *config.Config {
	if s.options.Config != nil {
		return s.options.Config
	}
	cfg := config.DefaultConfig()
	cfg.Server.AuthToken = s.options.AuthToken
	cfg.Server.AdminToken = s.options.AdminToken
	cfg.Server.ViewerToken = s.options.ViewerToken
	cfg.RBAC = s.options.RBAC
	cfg.Privacy = s.options.Privacy
	cfg.Budgets = s.options.Budgets
	cfg.Quota = s.options.Quota
	cfg.Watchdog = s.options.Watchdog
	cfg.Policies = s.options.Policies
	cfg.Webhooks = s.options.Webhooks
	cfg.Teams = s.options.Teams
	cfg.Integrations = s.options.Integrations
	cfg.Gateway = s.options.Gateway
	cfg.Pricing = s.options.Pricing
	cfg.Collectors = collectorsFromSourceOptions(s.options.Sources)
	return cfg
}

func (s *Server) admissionDefaults() controlplane.AdmissionInput {
	return controlplane.AdmissionInput{
		RBACEnabled:    s.options.RBAC.Enabled,
		AuthConfigured: s.options.AuthToken != "" || s.options.AdminToken != "" || s.options.ViewerToken != "",
		ReadOnly:       s.options.RBAC.ReadOnly,
	}
}

func collectorsFromSourceOptions(sources []SourceOption) config.CollectorConfigs {
	cfg := config.CollectorConfigs{}
	for _, source := range sources {
		collector := config.CollectorConfig{Enabled: source.Enabled, Paths: make([]string, len(source.Paths))}
		copy(collector.Paths, source.Paths)
		switch source.Source {
		case "claude":
			cfg.Claude = collector
		case "codex":
			cfg.Codex = collector
		case "openclaw":
			cfg.OpenClaw = collector
		case "opencode":
			cfg.OpenCode = collector
		case "kiro":
			cfg.Kiro = collector
		case "pi":
			cfg.Pi = collector
		}
	}
	return cfg
}
