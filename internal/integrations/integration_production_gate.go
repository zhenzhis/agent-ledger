package integrations

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// IntegrationProductionGateRequest controls how strict the production
// enablement decision should be. It is metadata-only and never refers to local
// paths, prompts, accounts, or provider credentials.
type IntegrationProductionGateRequest struct {
	Strict        bool `json:"strict"`
	AllowPreview  bool `json:"allow_preview"`
	AllowOutbound bool `json:"allow_outbound"`
}

// IntegrationProductionGateReport is the last local release gate before an
// operator enables local-preview, gateway, OTLP, webhook, or write-ingest
// surfaces in a shared or production-like deployment.
type IntegrationProductionGateReport struct {
	Product             string                            `json:"product"`
	Contract            string                            `json:"contract"`
	Version             string                            `json:"version"`
	LocalFirst          bool                              `json:"local_first"`
	ReadOnlySafe        bool                              `json:"read_only_safe"`
	WritesLocalState    bool                              `json:"writes_local_state"`
	PrivacyPolicy       string                            `json:"privacy_policy"`
	Request             IntegrationProductionGateRequest  `json:"request"`
	GateHash            string                            `json:"gate_hash"`
	Hashes              IntegrationProductionGateHashes   `json:"hashes"`
	Runtime             IntegrationSmokeRuntime           `json:"runtime"`
	Decision            IntegrationProductionGateDecision `json:"decision"`
	Summary             IntegrationProductionGateSummary  `json:"summary"`
	Checks              []IntegrationProductionGateCheck  `json:"checks"`
	CICommands          []string                          `json:"ci_commands"`
	RequiredArtifacts   []string                          `json:"required_artifacts"`
	OperationalGuidance []string                          `json:"operational_guidance"`
	RedactionRules      []string                          `json:"redaction_rules"`
}

type IntegrationProductionGateHashes struct {
	CapabilityCatalogHash    string `json:"capability_catalog_hash"`
	IntegrationReadinessHash string `json:"integration_readiness_hash"`
	IntegrationSmokeHash     string `json:"integration_smoke_hash"`
	IntegrationUpgradeHash   string `json:"integration_upgrade_gate_hash"`
	IntegrationEvidenceHash  string `json:"integration_evidence_kit_hash"`
	OpenAPISmokeHash         string `json:"openapi_smoke_hash"`
	RuntimeStatusHash        string `json:"runtime_status_hash"`
}

type IntegrationProductionGateDecision struct {
	Status                    string `json:"status"`
	Severity                  string `json:"severity"`
	Reason                    string `json:"reason"`
	RecommendedCIExitCode     int    `json:"recommended_ci_exit_code"`
	AllowProductionEnablement bool   `json:"allow_production_enablement"`
	RequiresHumanReview       bool   `json:"requires_human_review"`
	RequiresSmoke             bool   `json:"requires_smoke"`
}

type IntegrationProductionGateSummary struct {
	TotalChecks      int `json:"total_checks"`
	Passed           int `json:"passed"`
	Review           int `json:"review"`
	Blocked          int `json:"blocked"`
	SmokeWarnings    int `json:"smoke_warnings"`
	SmokeFailures    int `json:"smoke_failures"`
	ReadinessBlocked int `json:"readiness_blocked"`
	ReadinessReview  int `json:"readiness_review_required"`
	PreviewEnabled   int `json:"preview_enabled"`
	OutboundEnabled  int `json:"outbound_enabled"`
	WriteEnabled     int `json:"write_enabled"`
	DisabledByConfig int `json:"disabled_by_config"`
	RecommendedExit  int `json:"recommended_ci_exit_code"`
}

type IntegrationProductionGateCheck struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Evidence    string `json:"evidence"`
	Remediation string `json:"remediation"`
	Privacy     string `json:"privacy"`
	Command     string `json:"command,omitempty"`
}

func IntegrationProductionGateFromValues(values url.Values) IntegrationProductionGateRequest {
	return NormalizeIntegrationProductionGateRequest(IntegrationProductionGateRequest{
		Strict:        valuesBool(values, "strict"),
		AllowPreview:  firstBool(values, "allow_preview", "allow-preview", "preview_ok", "preview-ok"),
		AllowOutbound: firstBool(values, "allow_outbound", "allow-outbound", "outbound_ok", "outbound-ok"),
	})
}

func NormalizeIntegrationProductionGateRequest(req IntegrationProductionGateRequest) IntegrationProductionGateRequest {
	return req
}

func IntegrationProductionGateFor(opts Options, runtime *storage.RuntimeStatus, req IntegrationProductionGateRequest) IntegrationProductionGateReport {
	req = NormalizeIntegrationProductionGateRequest(req)
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	catalog := Registry(opts)
	readiness := IntegrationReadiness(opts)
	smoke := IntegrationSmokeReportFor(opts, runtime)
	surfaceCounts := productionGateSurfaceCounts(catalog)
	hashes := IntegrationProductionGateHashes{
		CapabilityCatalogHash:    CatalogFingerprintFrom(catalog),
		IntegrationReadinessHash: IntegrationReadinessFingerprint(opts),
		IntegrationSmokeHash:     IntegrationSmokeFingerprintFrom(smoke),
		IntegrationUpgradeHash:   IntegrationUpgradeGateOpenAPIFingerprint(opts, runtime),
		IntegrationEvidenceHash:  IntegrationEvidenceKitOpenAPIFingerprint(opts, runtime),
		OpenAPISmokeHash:         OpenAPISmokeFingerprint(opts, runtime),
		RuntimeStatusHash:        hashJSONPayload(runtime),
	}
	checks := integrationProductionGateChecks(req, readiness, smoke, surfaceCounts, hashes, opts)
	summary := summarizeIntegrationProductionGate(checks, readiness, smoke, surfaceCounts)
	decision := integrationProductionGateDecision(summary, req)
	report := IntegrationProductionGateReport{
		Product:          "Agent Ledger",
		Contract:         "agent-ledger.integration-production-gate",
		Version:          "v1",
		LocalFirst:       !opts.GatewayEnabled && !opts.WebhooksEnabled,
		ReadOnlySafe:     true,
		WritesLocalState: false,
		PrivacyPolicy:    "Integration production gates evaluate static contracts, readiness, smoke, runtime flags, release evidence, and operator intent only. They exclude SQLite usage rows, fixture bodies, prompt content, response content, message history, local paths, secrets, account identifiers, machine names, authors, native session identifiers, and webhook URLs.",
		Request:          req,
		Hashes:           hashes,
		Runtime: IntegrationSmokeRuntime{
			ReadOnly:                opts.ReadOnly,
			RBACEnabled:             opts.RBACEnabled,
			WebhooksEnabled:         opts.WebhooksEnabled,
			OTLPReceiverEnabled:     opts.OTLPReceiverEnabled,
			OTLPReceiverGRPCEnabled: opts.OTLPReceiverGRPCEnabled,
			GatewayEnabled:          opts.GatewayEnabled,
			PricingMode:             strings.TrimSpace(opts.PricingMode),
		},
		Decision: decision,
		Summary:  summary,
		Checks:   checks,
		CICommands: []string{
			"agent-ledger contracts verify",
			"agent-ledger integrations smoke",
			"agent-ledger integrations readiness",
			"agent-ledger integrations evidence-kit",
			"agent-ledger integrations production-gate --strict",
			"agent-ledger admission check --surface http --method POST --path /api/events --role operator",
		},
		RequiredArtifacts: []string{
			"contract verification report with failed=0",
			"integration smoke report with no failed checks",
			"readiness report showing blocked=0",
			"integration evidence kit for each enabled adapter or provider surface",
			"operator approval for local-preview, gateway, OTLP, webhook, or outbound behavior",
			"deployment smoke evidence for bind address, auth mode, read-only posture, and privacy preset",
		},
		OperationalGuidance: []string{
			"use pass status for production enablement continuation",
			"use review-required status to require an operator approval record before enabling preview or outbound surfaces",
			"use blocked status as a release stopper until contract, readiness, smoke, or explicit approval gaps are resolved",
			"keep this gate in wrapper, router, and deployment CI before switching from observer mode to write ingest",
		},
		RedactionRules: []string{
			"attach only hashes, contract ids, endpoint names, command names, check ids, and redacted evidence summaries to release tickets",
			"do not attach prompts, responses, transcripts, raw headers, credentials, webhook URLs, local paths, account names, machine names, authors, native session ids, or provider account ids",
			"use privacy presets and synthetic data when sharing screenshots or production-readiness evidence outside the local operator machine",
		},
	}
	report.GateHash = IntegrationProductionGateFingerprintFrom(report)
	return report
}

func IntegrationProductionGateFingerprint(opts Options, runtime *storage.RuntimeStatus, req IntegrationProductionGateRequest) string {
	return IntegrationProductionGateFingerprintFrom(IntegrationProductionGateFor(opts, runtime, req))
}

// IntegrationProductionGateOpenAPIFingerprint is a non-recursive witness hash
// for discovery, OpenAPI metadata, and contract-bundle indexes.
func IntegrationProductionGateOpenAPIFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	return hashJSONPayload(map[string]interface{}{
		"contract":                        "agent-ledger.integration-production-gate",
		"version":                         "v1",
		"default_uri":                     "/api/integrations/production-gate",
		"capability_catalog_hash":         CatalogFingerprint(opts),
		"integration_readiness_hash":      IntegrationReadinessFingerprint(opts),
		"integration_upgrade_gate_hash":   IntegrationUpgradeGateOpenAPIFingerprint(opts, runtime),
		"runtime_status_hash":             hashJSONPayload(runtime),
		"privacy":                         "metadata-only OpenAPI witness; full endpoint ETag is returned by /api/integrations/production-gate",
		"operator_approval_query_options": []string{"strict", "allow_preview", "allow_outbound"},
	})
}

func IntegrationProductionGateFingerprintFrom(report IntegrationProductionGateReport) string {
	report.GateHash = ""
	return hashJSONPayload(report)
}

type productionGateSurfaceSummary struct {
	PreviewEnabled  int
	OutboundEnabled int
	WriteEnabled    int
}

func productionGateSurfaceCounts(catalog Catalog) productionGateSurfaceSummary {
	summary := productionGateSurfaceSummary{}
	for _, cap := range catalog.Capabilities {
		if !cap.Enabled {
			continue
		}
		if cap.Status == "experimental" || strings.EqualFold(cap.Maturity, "local-preview") {
			summary.PreviewEnabled++
		}
		if cap.Direction == "outbound" || cap.Category == "gateway" {
			summary.OutboundEnabled++
		}
		if cap.WritesLocalState {
			summary.WriteEnabled++
		}
	}
	return summary
}

func integrationProductionGateChecks(req IntegrationProductionGateRequest, readiness IntegrationReadinessReport, smoke IntegrationSmokeReport, surfaces productionGateSurfaceSummary, hashes IntegrationProductionGateHashes, opts Options) []IntegrationProductionGateCheck {
	checks := []IntegrationProductionGateCheck{
		productionGateCheck("metadata_only", "pass", "info", "production gate is metadata-only and read-only", "writes_local_state=false, read_only_safe=true", "keep release evidence free of prompt, response, transcript, raw header, local path, and secret material", "agent-ledger integrations production-gate"),
		productionGateCheck("contracts_present", passOrReview(hashes.CapabilityCatalogHash != "" && hashes.IntegrationSmokeHash != "" && hashes.IntegrationReadinessHash != ""), "critical", "production gate pins catalog, smoke, readiness, evidence, OpenAPI, upgrade, and runtime hashes", "catalog="+shortHashEvidence(hashes.CapabilityCatalogHash)+",smoke="+shortHashEvidence(hashes.IntegrationSmokeHash)+",readiness="+shortHashEvidence(hashes.IntegrationReadinessHash), "rerun contract generation before using this gate for release decisions", "agent-ledger contracts verify"),
	}
	if smoke.Summary.Failed > 0 {
		checks = append(checks, productionGateCheck("smoke.failures", "block", "critical", "integration smoke has failed checks", "failed="+strconv.Itoa(smoke.Summary.Failed), "resolve smoke failures before production enablement", "agent-ledger integrations smoke"))
	} else {
		checks = append(checks, productionGateCheck("smoke.failures", "pass", "critical", "integration smoke has no failed checks", "failed=0", "no action required", "agent-ledger integrations smoke"))
	}
	if smoke.Summary.Warnings > 0 {
		status := "review"
		if req.Strict {
			status = "block"
		}
		checks = append(checks, productionGateCheck("smoke.warnings", status, "warning", "integration smoke has warning checks that require review", "warnings="+strconv.Itoa(smoke.Summary.Warnings), "review smoke warnings or run with explicit allow flags after acceptance", "agent-ledger integrations smoke"))
	} else {
		checks = append(checks, productionGateCheck("smoke.warnings", "pass", "warning", "integration smoke has no warning checks", "warnings=0", "no action required", "agent-ledger integrations smoke"))
	}
	if readiness.Summary.Blocked > 0 {
		checks = append(checks, productionGateCheck("readiness.blocked", "block", "critical", "readiness has blocked activation gates", "blocked="+strconv.Itoa(readiness.Summary.Blocked), "resolve blocked readiness gates before production enablement", "agent-ledger integrations readiness"))
	} else {
		checks = append(checks, productionGateCheck("readiness.blocked", "pass", "critical", "readiness has no blocked activation gates", "blocked=0", "no action required", "agent-ledger integrations readiness"))
	}
	if readiness.Summary.ReviewRequired > 0 {
		status := "review"
		if req.Strict && !req.AllowPreview {
			status = "block"
		}
		checks = append(checks, productionGateCheck("readiness.review_required", status, "warning", "readiness has capabilities requiring operator review", "review_required="+strconv.Itoa(readiness.Summary.ReviewRequired), "attach operator approval or keep the surfaces disabled in production", "agent-ledger integrations readiness"))
	}
	if surfaces.PreviewEnabled > 0 && !req.AllowPreview {
		status := "review"
		if req.Strict {
			status = "block"
		}
		checks = append(checks, productionGateCheck("preview.explicit_approval", status, "warning", "enabled local-preview surfaces require explicit production approval", "preview_enabled="+strconv.Itoa(surfaces.PreviewEnabled)+",allow_preview=false", "rerun with allow_preview only after deployment smoke evidence is attached", "agent-ledger integrations production-gate --allow-preview"))
	} else {
		checks = append(checks, productionGateCheck("preview.explicit_approval", "pass", "warning", "preview surface approval posture is explicit", "preview_enabled="+strconv.Itoa(surfaces.PreviewEnabled)+",allow_preview="+boolString(req.AllowPreview), "no action required", "agent-ledger integrations production-gate"))
	}
	outboundRuntimeEnabled := opts.GatewayEnabled || opts.WebhooksEnabled || opts.OTLPReceiverEnabled || opts.OTLPReceiverGRPCEnabled
	if (surfaces.OutboundEnabled > 0 || outboundRuntimeEnabled) && !req.AllowOutbound {
		status := "review"
		if req.Strict {
			status = "block"
		}
		checks = append(checks, productionGateCheck("outbound.explicit_approval", status, "critical", "enabled outbound or receiver surfaces require explicit approval", "outbound_enabled="+strconv.Itoa(surfaces.OutboundEnabled)+",runtime_outbound="+boolString(outboundRuntimeEnabled)+",allow_outbound=false", "rerun with allow_outbound only after loopback/auth/redaction review is attached", "agent-ledger integrations production-gate --allow-outbound"))
	} else {
		checks = append(checks, productionGateCheck("outbound.explicit_approval", "pass", "critical", "outbound approval posture is explicit", "outbound_enabled="+strconv.Itoa(surfaces.OutboundEnabled)+",allow_outbound="+boolString(req.AllowOutbound), "no action required", "agent-ledger integrations production-gate"))
	}
	checks = append(checks,
		productionGateCheck("upgrade_gate.witness", passOrReview(strings.HasPrefix(hashes.IntegrationUpgradeHash, "sha256:")), "critical", "upgrade gate witness hash is available", hashes.IntegrationUpgradeHash, "repair integration upgrade gate contract before release", "agent-ledger integrations upgrade-gate --strict"),
		productionGateCheck("evidence_kit.witness", passOrReview(strings.HasPrefix(hashes.IntegrationEvidenceHash, "sha256:")), "critical", "evidence kit witness hash is available", hashes.IntegrationEvidenceHash, "repair integration evidence kit contract before release", "agent-ledger integrations evidence-kit"),
		productionGateCheck("runtime.local_first", passOrReview(req.AllowOutbound || (!opts.GatewayEnabled && !opts.WebhooksEnabled)), "warning", "local-first runtime posture is explicit", "gateway_enabled="+boolString(opts.GatewayEnabled)+",webhooks_enabled="+boolString(opts.WebhooksEnabled)+",allow_outbound="+boolString(req.AllowOutbound), "keep outbound behavior disabled or attach explicit approval", "agent-ledger config status --format markdown"),
	)
	return checks
}

func summarizeIntegrationProductionGate(checks []IntegrationProductionGateCheck, readiness IntegrationReadinessReport, smoke IntegrationSmokeReport, surfaces productionGateSurfaceSummary) IntegrationProductionGateSummary {
	summary := IntegrationProductionGateSummary{
		TotalChecks:      len(checks),
		SmokeWarnings:    smoke.Summary.Warnings,
		SmokeFailures:    smoke.Summary.Failed,
		ReadinessBlocked: readiness.Summary.Blocked,
		ReadinessReview:  readiness.Summary.ReviewRequired,
		PreviewEnabled:   surfaces.PreviewEnabled,
		OutboundEnabled:  surfaces.OutboundEnabled,
		WriteEnabled:     surfaces.WriteEnabled,
		DisabledByConfig: readiness.Summary.DisabledByConfig,
	}
	for _, check := range checks {
		switch check.Status {
		case "block":
			summary.Blocked++
		case "review":
			summary.Review++
		default:
			summary.Passed++
		}
	}
	switch {
	case summary.Blocked > 0:
		summary.RecommendedExit = 1
	case summary.Review > 0:
		summary.RecommendedExit = 2
	default:
		summary.RecommendedExit = 0
	}
	return summary
}

func integrationProductionGateDecision(summary IntegrationProductionGateSummary, req IntegrationProductionGateRequest) IntegrationProductionGateDecision {
	if summary.Blocked > 0 {
		return IntegrationProductionGateDecision{
			Status:                    "blocked",
			Severity:                  "critical",
			Reason:                    "one or more production gate checks are blocked",
			RecommendedCIExitCode:     1,
			AllowProductionEnablement: false,
			RequiresHumanReview:       true,
			RequiresSmoke:             true,
		}
	}
	if summary.Review > 0 {
		return IntegrationProductionGateDecision{
			Status:                    "review-required",
			Severity:                  "warning",
			Reason:                    "production enablement requires explicit operator approval for warnings, preview, or outbound posture",
			RecommendedCIExitCode:     2,
			AllowProductionEnablement: false,
			RequiresHumanReview:       true,
			RequiresSmoke:             true,
		}
	}
	return IntegrationProductionGateDecision{
		Status:                    "pass",
		Severity:                  "info",
		Reason:                    "all production gate checks passed with the supplied operator intent",
		RecommendedCIExitCode:     0,
		AllowProductionEnablement: true,
		RequiresHumanReview:       req.AllowPreview || req.AllowOutbound,
		RequiresSmoke:             false,
	}
}

func productionGateCheck(id, status, severity, message, evidence, remediation, command string) IntegrationProductionGateCheck {
	return IntegrationProductionGateCheck{
		ID:          id,
		Status:      status,
		Severity:    severity,
		Message:     message,
		Evidence:    evidence,
		Remediation: remediation,
		Privacy:     "metadata-only; no usage rows, fixture bodies, prompts, responses, local paths, accounts, machines, authors, native sessions, credentials, or webhook URLs are read or emitted",
		Command:     command,
	}
}

func passOrReview(ok bool) string {
	if ok {
		return "pass"
	}
	return "review"
}

func shortHashEvidence(hash string) string {
	if len(hash) <= 19 {
		return hash
	}
	return hash[:19]
}

func firstBool(values url.Values, keys ...string) bool {
	for _, key := range keys {
		if valuesBool(values, key) {
			return true
		}
	}
	return false
}

func valuesBool(values url.Values, key string) bool {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return false
	}
	parsed, err := strconv.ParseBool(raw)
	return err == nil && parsed
}
