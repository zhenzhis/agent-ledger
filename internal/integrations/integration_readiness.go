package integrations

import (
	"sort"
	"strings"
)

// IntegrationReadinessReport is a static, privacy-safe activation checklist
// for local protocol, gateway, notification, and collector surfaces.
type IntegrationReadinessReport struct {
	Product             string                           `json:"product"`
	Contract            string                           `json:"contract"`
	Version             string                           `json:"version"`
	LocalFirst          bool                             `json:"local_first"`
	ReadOnlySafe        bool                             `json:"read_only_safe"`
	WritesLocalState    bool                             `json:"writes_local_state"`
	CatalogHash         string                           `json:"catalog_hash"`
	SignalCoverageHash  string                           `json:"signal_coverage_hash"`
	Runtime             IntegrationReadinessRuntime      `json:"runtime"`
	PrivacyPolicy       string                           `json:"privacy_policy"`
	Summary             IntegrationReadinessSummary      `json:"summary"`
	Capabilities        []IntegrationReadinessCapability `json:"capabilities"`
	QualityGates        []string                         `json:"quality_gates"`
	OperationalGuidance []string                         `json:"operational_guidance"`
}

// IntegrationReadinessRuntime exposes runtime flags without secrets or paths.
type IntegrationReadinessRuntime struct {
	ReadOnly                bool   `json:"read_only"`
	RBACEnabled             bool   `json:"rbac_enabled"`
	PoliciesEnabled         bool   `json:"policies_enabled"`
	QuotaEnabled            bool   `json:"quota_enabled"`
	WebhooksEnabled         bool   `json:"webhooks_enabled"`
	OTLPReceiverEnabled     bool   `json:"otlp_receiver_enabled"`
	OTLPReceiverGRPCEnabled bool   `json:"otlp_receiver_grpc_enabled"`
	GatewayEnabled          bool   `json:"gateway_enabled"`
	PricingMode             string `json:"pricing_mode"`
}

// IntegrationReadinessSummary captures stable readiness counts.
type IntegrationReadinessSummary struct {
	TotalCapabilities int `json:"total_capabilities"`
	Enabled           int `json:"enabled"`
	Disabled          int `json:"disabled"`
	Experimental      int `json:"experimental"`
	DisabledByConfig  int `json:"disabled_by_config"`
	Ready             int `json:"ready"`
	ReviewRequired    int `json:"review_required"`
	Blocked           int `json:"blocked"`
	Warnings          int `json:"warnings"`
	Failures          int `json:"failures"`
}

// IntegrationReadinessCapability describes activation state for one surface.
type IntegrationReadinessCapability struct {
	ID                  string                     `json:"id"`
	Name                string                     `json:"name"`
	Category            string                     `json:"category"`
	Status              string                     `json:"status"`
	Maturity            string                     `json:"maturity"`
	Direction           string                     `json:"direction"`
	Enabled             bool                       `json:"enabled"`
	RuntimeStatus       string                     `json:"runtime_status"`
	WritesLocalState    bool                       `json:"writes_local_state"`
	AvailableInReadOnly bool                       `json:"available_in_read_only"`
	ActivationState     string                     `json:"activation_state"`
	RiskLevel           string                     `json:"risk_level"`
	Gates               []IntegrationReadinessGate `json:"gates"`
	Evidence            []string                   `json:"evidence"`
	Actions             []string                   `json:"actions"`
}

// IntegrationReadinessGate is one activation prerequisite or invariant.
type IntegrationReadinessGate struct {
	ID          string `json:"id"`
	Severity    string `json:"severity"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Remediation string `json:"remediation,omitempty"`
}

// IntegrationReadiness returns a deterministic report for operators and
// wrapper CI. It reads only the in-process capability catalog and runtime flags.
func IntegrationReadiness(opts Options) IntegrationReadinessReport {
	catalog := Registry(opts)
	capabilities := make([]IntegrationReadinessCapability, 0, len(catalog.Capabilities))
	summary := IntegrationReadinessSummary{TotalCapabilities: len(catalog.Capabilities)}
	for _, cap := range catalog.Capabilities {
		item := integrationReadinessCapability(cap, opts)
		if item.Enabled {
			summary.Enabled++
		} else {
			summary.Disabled++
		}
		if item.Status == "experimental" {
			summary.Experimental++
		}
		switch item.ActivationState {
		case "ready":
			summary.Ready++
		case "review-required", "read-only-limited":
			summary.ReviewRequired++
		case "disabled-by-config":
			summary.DisabledByConfig++
		case "blocked":
			summary.Blocked++
		}
		for _, gate := range item.Gates {
			switch gate.Status {
			case "warning":
				summary.Warnings++
			case "blocked":
				summary.Failures++
			}
		}
		capabilities = append(capabilities, item)
	}
	sort.Slice(capabilities, func(i, j int) bool { return capabilities[i].ID < capabilities[j].ID })
	return IntegrationReadinessReport{
		Product:            "Agent Ledger",
		Contract:           "agent-ledger.integration-readiness",
		Version:            "v1",
		LocalFirst:         !opts.GatewayEnabled && !opts.WebhooksEnabled,
		ReadOnlySafe:       true,
		WritesLocalState:   false,
		CatalogHash:        CatalogFingerprintFrom(catalog),
		SignalCoverageHash: SignalCoverageFingerprint(),
		Runtime: IntegrationReadinessRuntime{
			ReadOnly:                opts.ReadOnly,
			RBACEnabled:             opts.RBACEnabled,
			PoliciesEnabled:         opts.PoliciesEnabled,
			QuotaEnabled:            opts.QuotaEnabled,
			WebhooksEnabled:         opts.WebhooksEnabled,
			OTLPReceiverEnabled:     opts.OTLPReceiverEnabled,
			OTLPReceiverGRPCEnabled: opts.OTLPReceiverGRPCEnabled,
			GatewayEnabled:          opts.GatewayEnabled,
			PricingMode:             strings.TrimSpace(opts.PricingMode),
		},
		PrivacyPolicy: "Integration readiness is catalog metadata only. It contains no usage rows, prompt content, response content, raw paths, secrets, machine names, authors, account ids, webhook URLs, or session ids.",
		Summary:       summary,
		Capabilities:  capabilities,
		QualityGates: []string{
			"guarded, experimental, or outbound surfaces must be explicitly enabled before runtime use",
			"read-only observer mode must keep write-capable surfaces blocked or limited to dry-run/read paths",
			"gateway and webhook surfaces must never persist prompt, response, credential, or webhook URL values",
			"OTLP gRPC must remain loopback-only unless an authenticated transport is added",
			"new adapters must pass conformance, signal coverage, and admission checks before enabling ingest",
		},
		OperationalGuidance: []string{
			"use this report in wrapper CI before enabling new agent or provider integrations",
			"treat disabled optional capabilities as acceptable until an operator explicitly opts in",
			"treat review-required capabilities as locally usable but not production-hardened without deployment-specific smoke tests",
			"use config status and admission check together with this report for deployment rollout decisions",
		},
	}
}

// IntegrationReadinessFingerprint returns a stable hash for the current runtime
// option view.
func IntegrationReadinessFingerprint(opts Options) string {
	return hashJSONPayload(IntegrationReadiness(opts))
}

func integrationReadinessCapability(cap Capability, opts Options) IntegrationReadinessCapability {
	gates := []IntegrationReadinessGate{
		{
			ID:          "catalog.privacy_statement",
			Severity:    "critical",
			Status:      passOrBlocked(cap.Privacy != ""),
			Message:     "capability has a privacy statement in the public catalog",
			Remediation: "add a metadata-only privacy statement before exposing the capability",
		},
		{
			ID:          "catalog.runtime_status",
			Severity:    "critical",
			Status:      passOrBlocked(cap.RuntimeStatus != ""),
			Message:     "capability has an explicit runtime status",
			Remediation: "run registry runtime annotation before publishing readiness",
		},
	}
	if strings.Contains(strings.ToLower(cap.Privacy), "prompt") || strings.Contains(strings.ToLower(cap.Privacy), "message") {
		gates = append(gates, IntegrationReadinessGate{
			ID:          "privacy.content_boundary",
			Severity:    "critical",
			Status:      "pass",
			Message:     "capability documents prompt/message content handling without claiming persistence",
			Remediation: "ensure implementation stores metadata only and rejects or excludes raw prompt and response bodies",
		})
	}
	if opts.ReadOnly && cap.WritesLocalState && !cap.AvailableInReadOnly {
		gates = append(gates, IntegrationReadinessGate{
			ID:          "runtime.read_only_block",
			Severity:    "critical",
			Status:      "blocked",
			Message:     "read-only observer mode blocks this write-capable surface",
			Remediation: "restart without read-only mode or use a validation/dry-run/read endpoint",
		})
	} else if opts.ReadOnly && cap.WritesLocalState {
		gates = append(gates, IntegrationReadinessGate{
			ID:          "runtime.read_only_limited",
			Severity:    "warning",
			Status:      "warning",
			Message:     "read-only observer mode leaves only read or dry-run behavior available",
			Remediation: "do not rely on write/import/sync behavior until read-only mode is disabled",
		})
	}
	if cap.Status == "experimental" || strings.EqualFold(cap.Maturity, "local-preview") {
		status := "warning"
		if !cap.Enabled {
			status = "info"
		}
		gates = append(gates, IntegrationReadinessGate{
			ID:          "maturity.preview_review",
			Severity:    "warning",
			Status:      status,
			Message:     "local-preview or experimental surface requires explicit smoke tests before production enablement",
			Remediation: "run the documented conformance, admission, privacy, and deployment smoke checks before rollout",
		})
	}
	switch cap.ID {
	case "gateway.provider_live_proxy":
		status := "info"
		if opts.GatewayEnabled {
			status = "warning"
		}
		gates = append(gates, IntegrationReadinessGate{
			ID:          "gateway.explicit_enablement",
			Severity:    "critical",
			Status:      status,
			Message:     "live provider gateway must be explicitly enabled and deployment-reviewed",
			Remediation: "set gateway.enabled only after upstream credentials, RBAC/local bind, policy, and usage smoke tests are verified",
		})
	case "protocol.otlp_receiver":
		status := "info"
		if opts.OTLPReceiverEnabled {
			status = "warning"
		}
		gates = append(gates, IntegrationReadinessGate{
			ID:          "otlp.explicit_enablement",
			Severity:    "critical",
			Status:      status,
			Message:     "OTLP receiver must be explicitly enabled and bounded before ingesting telemetry",
			Remediation: "set integrations.otlp_receiver.enabled only after body/span limits and loopback binding are verified",
		})
		if opts.OTLPReceiverGRPCEnabled && !opts.OTLPReceiverEnabled {
			gates = append(gates, IntegrationReadinessGate{
				ID:          "otlp.grpc_requires_receiver",
				Severity:    "critical",
				Status:      "blocked",
				Message:     "OTLP gRPC is enabled while the parent OTLP receiver is disabled",
				Remediation: "enable integrations.otlp_receiver.enabled or disable grpc_enabled",
			})
		}
	case "notification.redacted_webhook":
		status := "info"
		if opts.WebhooksEnabled {
			status = "warning"
		}
		gates = append(gates, IntegrationReadinessGate{
			ID:          "webhook.explicit_enablement",
			Severity:    "warning",
			Status:      status,
			Message:     "webhook delivery is optional outbound behavior and must remain redacted",
			Remediation: "use dry-run payload review before enabling webhooks in shared environments",
		})
	}
	actions := readinessActions(cap, opts)
	evidence := append(append([]string{}, cap.Endpoints...), cap.Commands...)
	evidence = append(evidence, cap.Tools...)
	evidence = append(evidence, cap.Resources...)
	sort.Strings(evidence)
	return IntegrationReadinessCapability{
		ID:                  cap.ID,
		Name:                cap.Name,
		Category:            cap.Category,
		Status:              cap.Status,
		Maturity:            cap.Maturity,
		Direction:           cap.Direction,
		Enabled:             cap.Enabled,
		RuntimeStatus:       cap.RuntimeStatus,
		WritesLocalState:    cap.WritesLocalState,
		AvailableInReadOnly: cap.AvailableInReadOnly,
		ActivationState:     activationState(cap, gates, opts),
		RiskLevel:           readinessRiskLevel(cap),
		Gates:               gates,
		Evidence:            evidence,
		Actions:             actions,
	}
}

func activationState(cap Capability, gates []IntegrationReadinessGate, opts Options) string {
	for _, gate := range gates {
		if gate.Status == "blocked" && cap.Enabled {
			return "blocked"
		}
		if gate.ID == "otlp.grpc_requires_receiver" && gate.Status == "blocked" {
			return "blocked"
		}
	}
	if !cap.Enabled {
		return "disabled-by-config"
	}
	if opts.ReadOnly && cap.WritesLocalState && cap.AvailableInReadOnly {
		return "read-only-limited"
	}
	if hasReadinessGateStatus(gates, "warning") {
		return "review-required"
	}
	if cap.Status == "experimental" || strings.EqualFold(cap.Maturity, "local-preview") || cap.Category == "gateway" || cap.Direction == "outbound" {
		return "review-required"
	}
	return "ready"
}

func readinessRiskLevel(cap Capability) string {
	if cap.Category == "gateway" || cap.Direction == "outbound" {
		return "high"
	}
	if cap.WritesLocalState || cap.Status == "experimental" {
		return "medium"
	}
	return "low"
}

func hasReadinessGateStatus(gates []IntegrationReadinessGate, status string) bool {
	for _, gate := range gates {
		if gate.Status == status {
			return true
		}
	}
	return false
}

func readinessActions(cap Capability, opts Options) []string {
	actions := []string{}
	if !cap.Enabled {
		actions = append(actions, "enable explicitly only when this surface is required")
	}
	if opts.ReadOnly && cap.WritesLocalState {
		actions = append(actions, "disable read-only mode before expecting writes")
	}
	if cap.Status == "experimental" || strings.EqualFold(cap.Maturity, "local-preview") || strings.EqualFold(cap.Maturity, "guarded-v1") {
		actions = append(actions, "run fixture, privacy, admission, and deployment smoke checks")
	}
	if cap.ID == "gateway.provider_live_proxy" {
		actions = append(actions, "verify upstream credentials through environment variables and keep prompt/response bodies out of durable logs")
	}
	if cap.ID == "protocol.otlp_receiver" {
		actions = append(actions, "verify body limits, span limits, gzip limits, and loopback gRPC bind before enabling")
	}
	if len(actions) == 0 {
		actions = append(actions, "keep contract verification in CI")
	}
	sort.Strings(actions)
	return actions
}

func passOrBlocked(ok bool) string {
	if ok {
		return "pass"
	}
	return "blocked"
}
