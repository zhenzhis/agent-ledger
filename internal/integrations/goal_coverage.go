package integrations

import (
	"sort"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// GoalCoverageReport is a privacy-safe implementation coverage contract for
// the Agent Ledger product goal. It maps the long-form product plan to stable
// REST, CLI, MCP, schema, docs, and verification evidence without inspecting
// local usage data or exposing paths.
type GoalCoverageReport struct {
	Product               string                 `json:"product"`
	Slug                  string                 `json:"slug"`
	Contract              string                 `json:"contract"`
	Version               string                 `json:"version"`
	Status                string                 `json:"status"`
	LocalFirst            bool                   `json:"local_first"`
	ReadOnly              bool                   `json:"read_only"`
	PromptContentStored   bool                   `json:"prompt_content_stored"`
	UsageDataUploaded     bool                   `json:"usage_data_uploaded"`
	PrivacyDefault        string                 `json:"privacy_default"`
	CapabilityCatalogHash string                 `json:"capability_catalog_hash"`
	ProviderProfilesHash  string                 `json:"provider_profiles_hash"`
	AgentProfilesHash     string                 `json:"agent_profiles_hash"`
	OpenAPIHash           string                 `json:"openapi_hash"`
	ContractBundleHash    string                 `json:"contract_bundle_hash"`
	CanonicalSchemaHash   string                 `json:"canonical_schema_hash"`
	AdapterSpecHash       string                 `json:"adapter_spec_hash"`
	CoverageHash          string                 `json:"coverage_hash"`
	Summary               GoalCoverageSummary    `json:"summary"`
	CompletionAudit       GoalCompletionAudit    `json:"completion_audit"`
	Sections              []GoalCoverageSection  `json:"sections"`
	ExternalDependencies  []GoalCoverageExternal `json:"external_dependencies,omitempty"`
	Verification          []string               `json:"verification"`
	Privacy               string                 `json:"privacy"`
}

type GoalCoverageSummary struct {
	TotalSections        int     `json:"total_sections"`
	Implemented          int     `json:"implemented"`
	Experimental         int     `json:"experimental"`
	ExternalDependencies int     `json:"external_dependencies"`
	Gaps                 int     `json:"gaps"`
	CompletionRatio      float64 `json:"completion_ratio"`
	NextAction           string  `json:"next_action"`
}

type GoalCoverageSection struct {
	ID            string               `json:"id"`
	Title         string               `json:"title"`
	Category      string               `json:"category"`
	Status        string               `json:"status"`
	Maturity      string               `json:"maturity"`
	Objective     string               `json:"objective"`
	CapabilityIDs []string             `json:"capability_ids,omitempty"`
	Evidence      GoalCoverageEvidence `json:"evidence"`
	Privacy       string               `json:"privacy"`
	Limitations   []string             `json:"limitations,omitempty"`
	Remaining     []string             `json:"remaining,omitempty"`
}

type GoalCoverageEvidence struct {
	Endpoints    []string `json:"endpoints,omitempty"`
	Commands     []string `json:"commands,omitempty"`
	MCPTools     []string `json:"mcp_tools,omitempty"`
	MCPResources []string `json:"mcp_resources,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Tables       []string `json:"tables,omitempty"`
	Tests        []string `json:"tests,omitempty"`
	Docs         []string `json:"docs,omitempty"`
}

type GoalCoverageExternal struct {
	ID          string   `json:"id"`
	Dependency  string   `json:"dependency"`
	Reason      string   `json:"reason"`
	LocalStatus string   `json:"local_status"`
	Evidence    []string `json:"evidence"`
}

// GoalCompletionAudit is the conservative completion gate for the persistent
// Agent Ledger product goal. It deliberately treats coverage as necessary but
// not sufficient: experimental surfaces, guarded rollout needs, external
// dependencies, remaining verification work, or missing verification evidence
// keep completion in review-required or incomplete states.
type GoalCompletionAudit struct {
	Contract                string                     `json:"contract"`
	Version                 string                     `json:"version"`
	Status                  string                     `json:"status"`
	ReadyToMarkGoalComplete bool                       `json:"ready_to_mark_goal_complete"`
	RecommendedCIExitCode   int                        `json:"recommended_ci_exit_code"`
	Reason                  string                     `json:"reason"`
	Summary                 GoalCompletionAuditSummary `json:"summary"`
	Checks                  []GoalCompletionAuditCheck `json:"checks"`
	RequiredVerification    []string                   `json:"required_verification"`
	Privacy                 string                     `json:"privacy"`
}

type GoalCompletionAuditSummary struct {
	TotalChecks           int `json:"total_checks"`
	Passed                int `json:"passed"`
	Review                int `json:"review"`
	Blocked               int `json:"blocked"`
	CoverageGaps          int `json:"coverage_gaps"`
	ExperimentalSections  int `json:"experimental_sections"`
	ExternalDependencies  int `json:"external_dependencies"`
	SectionsWithRemaining int `json:"sections_with_remaining"`
	VerificationCommands  int `json:"verification_commands"`
}

type GoalCompletionAuditCheck struct {
	ID           string   `json:"id"`
	Status       string   `json:"status"`
	Severity     string   `json:"severity"`
	Message      string   `json:"message"`
	Evidence     string   `json:"evidence"`
	Requirement  string   `json:"requirement"`
	Remediation  string   `json:"remediation"`
	Verification []string `json:"verification,omitempty"`
}

// GoalCoverageReportFor returns a stable coverage contract for the current
// build/runtime options. Status is derived from the integration catalog where
// possible, so the report fails visibly when expected capability IDs drift.
func GoalCoverageReportFor(opts Options, runtime *storage.RuntimeStatus) GoalCoverageReport {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	bundle := ContractBundleFor(opts, runtime)
	report := goalCoverageReportFor(opts, runtime, bundle.BundleHash)
	report.CoverageHash = GoalCoverageFingerprintFrom(report)
	return report
}

func goalCoverageReportFor(opts Options, runtime *storage.RuntimeStatus, contractBundleHash string) GoalCoverageReport {
	catalog := Registry(opts)
	catalogHash := CatalogFingerprintFrom(catalog)
	openAPIHash := OpenAPIFingerprint(opts, runtime)
	capabilities := map[string]Capability{}
	for _, cap := range catalog.Capabilities {
		capabilities[cap.ID] = cap
	}
	sections := goalCoverageSections(capabilities)
	summary := goalCoverageSummary(sections)
	external := []GoalCoverageExternal{
		{
			ID:          "native_mcp_push_subscription",
			Dependency:  "MCP host/client support for native resource subscription push transport",
			Reason:      "Agent Ledger already exposes cursor-stable resources and local polling subscription notifications; true host push cannot be claimed until host clients support that transport.",
			LocalStatus: "implemented_local_polling",
			Evidence:    []string{"agent-ledger://workloads/feed", "agent-ledger://workload/state", "MCP resources/subscribe local polling"},
		},
	}
	summary.ExternalDependencies = len(external)
	if summary.Gaps == 0 && summary.Experimental == 0 && summary.ExternalDependencies > 0 {
		summary.NextAction = "keep external dependencies disclosed until accepted or removed from completion scope"
	}
	status := "implemented"
	if summary.Gaps > 0 {
		status = "gaps"
	} else if summary.Experimental > 0 {
		status = "implemented-with-experimental-surfaces"
	} else if summary.ExternalDependencies > 0 {
		status = "implemented-with-external-dependencies"
	}
	report := GoalCoverageReport{
		Product:               "Agent Ledger",
		Slug:                  "agent-ledger",
		Contract:              "agent-ledger.goal-coverage",
		Version:               "v1",
		Status:                status,
		LocalFirst:            true,
		ReadOnly:              opts.ReadOnly,
		PromptContentStored:   false,
		UsageDataUploaded:     false,
		PrivacyDefault:        catalog.PrivacyDefault,
		CapabilityCatalogHash: catalogHash,
		ProviderProfilesHash:  ProviderProfilesFingerprint(),
		AgentProfilesHash:     AgentFrameworkProfilesFingerprint(),
		OpenAPIHash:           openAPIHash,
		ContractBundleHash:    contractBundleHash,
		CanonicalSchemaHash:   storage.CanonicalEventSchemaFingerprint(),
		AdapterSpecHash:       AdapterContractFingerprint(),
		Summary:               summary,
		Sections:              sections,
		ExternalDependencies:  external,
		Verification: []string{
			"go test ./...",
			"go vet ./...",
			"govulncheck ./...",
			"node --check internal/server/static/app.js",
			"agent-ledger ui check",
			"git diff --check",
			"agent-ledger contracts verify",
			"agent-ledger integrations",
			"agent-ledger goal coverage",
			"agent-ledger goal audit",
		},
		Privacy: "Coverage uses static contracts, catalog metadata, endpoint names, table names, and documentation paths only; it does not read prompt content, responses, local paths, session ids, machine names, authors, or secrets.",
	}
	report.CompletionAudit = GoalCompletionAuditFor(report)
	return report
}

func GoalCoverageFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	return GoalCoverageFingerprintFrom(GoalCoverageReportFor(opts, runtime))
}

func GoalCoverageContractFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	return GoalCoverageFingerprintFrom(goalCoverageReportFor(opts, runtime, ""))
}

func GoalCoverageFingerprintFrom(report GoalCoverageReport) string {
	report.CoverageHash = ""
	return hashJSONPayload(report)
}

func goalCoverageSections(capabilities map[string]Capability) []GoalCoverageSection {
	sections := []GoalCoverageSection{
		{
			ID:        "identity_release_governance",
			Title:     "Product Identity And Release Governance",
			Category:  "repo",
			Status:    "implemented",
			Maturity:  "stable-v1",
			Objective: "Agent Ledger is a standalone local-first product line while retaining attribution to the upstream fork.",
			Evidence: GoalCoverageEvidence{
				Commands: []string{"agent-ledger version", "agent-ledger contracts", "agent-ledger integrations"},
				Docs:     []string{"README.md", "README_CN.md", "CHANGELOG.md", "CONTRIBUTING.md", "SECURITY.md", "RELEASE.md", "docker-compose.example.yml", ".goreleaser.yaml"},
			},
			Privacy: "Repository metadata only; no usage data or local paths.",
		},
		{
			ID:            "contract_discovery_control_plane",
			Title:         "Discovery, Contracts, OpenAPI, Admission And Readiness",
			Category:      "control-plane",
			Maturity:      "stable-v1",
			Objective:     "Wrappers, routers, CI, and local operators can discover capabilities, verify contracts, and dry-run access before calling write surfaces.",
			CapabilityIDs: []string{"protocol.discovery_manifest", "protocol.contract_bundle", "protocol.contract_verification", "protocol.goal_coverage", "protocol.openapi", "protocol.runtime_status", "protocol.config_status", "protocol.readiness", "protocol.admission_check", "protocol.schema_evolution_gate"},
			Evidence: GoalCoverageEvidence{
				Endpoints:    []string{"GET /.well-known/agent-ledger.json", "GET /api/contracts", "GET /api/contracts/verify", "GET /api/goal-coverage", "GET /api/openapi.json", "GET /api/schema/evolution-gate", "GET /api/readiness", "GET /api/admission/check"},
				Commands:     []string{"agent-ledger discovery", "agent-ledger contracts verify", "agent-ledger goal coverage", "agent-ledger schema-gate", "agent-ledger readiness", "agent-ledger admission check"},
				MCPTools:     []string{"ledger.discovery", "ledger.contracts", "ledger.contracts_verify", "ledger.goal_coverage", "ledger.openapi", "ledger.schema_evolution_gate", "ledger.runtime_status", "ledger.config_status", "ledger.readiness", "ledger.admission_check"},
				MCPResources: []string{"agent-ledger://discovery/manifest", "agent-ledger://contracts/bundle", "agent-ledger://contracts/verification", "agent-ledger://goal/coverage", "agent-ledger://contracts/openapi", "agent-ledger://schema/evolution-gate", "agent-ledger://readiness", "agent-ledger://admission/check"},
				Tests:        []string{"internal/server/integrations_test.go", "internal/integrations/registry_test.go", "internal/controlplane/admission_test.go", "internal/controlplane/readiness_test.go"},
			},
			Privacy: "Control-plane contracts expose only metadata, hashes, counts, role requirements, and remediation hints.",
		},
		{
			ID:            "canonical_event_workload_ledger",
			Title:         "Canonical Event Schema And Workload Ledger",
			Category:      "agentops",
			Maturity:      "stable-v1",
			Objective:     "Represent agent goal/context/run/model/tool/artifact/evaluation/policy workload metadata without storing prompt or response bodies.",
			CapabilityIDs: []string{"protocol.canonical_events.http", "protocol.canonical_events.cli", "protocol.adapter_conformance", "protocol.schema_evolution_gate", "protocol.workload_event_feed", "protocol.mcp_stdio"},
			Evidence: GoalCoverageEvidence{
				Endpoints:    []string{"GET /api/event-schema", "GET /api/schema/evolution-gate", "GET /api/event-examples", "POST /api/events/validate", "POST /api/events", "GET /api/workloads", "GET /api/workload-events", "GET /api/workload-events/stream"},
				Commands:     []string{"agent-ledger event schema", "agent-ledger event schema-gate", "agent-ledger event validate", "agent-ledger event ingest", "agent-ledger workload queue", "agent-ledger workload feed"},
				MCPTools:     []string{"ledger.start_workload", "ledger.start_run", "ledger.heartbeat_run", "ledger.record_tool_call", "ledger.record_context", "ledger.record_artifact", "ledger.record_evaluation", "ledger.schema_evolution_gate", "ledger.workload_feed"},
				MCPResources: []string{"agent-ledger://schema/canonical-events", "agent-ledger://schema/evolution-gate", "agent-ledger://workloads/recent", "agent-ledger://workloads/queue", "agent-ledger://workloads/feed", "agent-ledger://workload/state", "agent-ledger://workload/timeline"},
				Tables:       []string{"canonical_events", "workloads", "agent_runs", "agent_run_events", "model_calls", "tool_calls", "context_refs", "artifacts", "evaluations", "policy_decisions", "workload_links", "workload_leases"},
				Tests:        []string{"internal/storage/canonical_events_test.go", "internal/storage/workloads_test.go", "internal/server/workloads_test.go", "internal/mcp/server_test.go"},
			},
			Privacy: "Canonical payload validation rejects raw prompt/content keys; artifact and context evidence are stored by metadata and hashes.",
		},
		{
			ID:            "ecosystem_adapters_and_gateway",
			Title:         "Ecosystem Adapters, Protocols And Provider Gateway",
			Category:      "ecosystem",
			Maturity:      "guarded-v1",
			Objective:     "Support current and future agent CLIs, provider envelopes, OpenTelemetry GenAI, OTLP, A2A, provider streams, provider/runtime profiles, and optional local gateways.",
			CapabilityIDs: []string{"collector.claude", "collector.codex", "collector.openclaw", "collector.opencode", "collector.kiro", "collector.pi", "protocol.provider_profiles", "protocol.agent_profiles", "protocol.signal_taxonomy", "protocol.signal_coverage", "protocol.integration_readiness", "protocol.integration_smoke", "protocol.integration_compatibility", "protocol.integration_rollout_plan", "protocol.integration_evidence_kit", "protocol.integration_drift", "protocol.integration_lockfile", "protocol.integration_upgrade_gate", "protocol.integration_production_gate", "protocol.integration_recommendation", "protocol.opentelemetry_genai", "protocol.otlp_receiver", "protocol.a2a", "gateway.provider_api", "gateway.provider_live_proxy"},
			Evidence: GoalCoverageEvidence{
				Endpoints:    []string{"GET /api/provider-profiles", "GET /api/agent-profiles", "GET /api/signal-taxonomy", "GET /api/integrations/signal-coverage", "GET /api/integrations/readiness", "GET /api/integrations/smoke", "GET /api/integrations/compatibility", "GET /api/integrations/rollout-plan", "GET /api/integrations/evidence-kit", "GET /api/integrations/drift", "GET /api/integrations/lockfile", "GET /api/integrations/upgrade-gate", "GET /api/integrations/production-gate", "GET /api/integrations/recommendation", "POST /api/otel/genai", "POST /api/otlp/v1/traces", "POST /api/a2a/tasks", "POST /api/provider/calls", "POST /gateway/openai/v1/chat/completions", "POST /gateway/openai/v1/responses", "POST /gateway/anthropic/v1/messages"},
				Commands:     []string{"agent-ledger provider profiles", "agent-ledger agent profiles", "agent-ledger signals", "agent-ledger signal-coverage", "agent-ledger integrations readiness", "agent-ledger integrations smoke", "agent-ledger integrations compatibility", "agent-ledger integrations rollout-plan", "agent-ledger integrations evidence-kit", "agent-ledger integrations drift", "agent-ledger integrations lockfile", "agent-ledger integrations upgrade-gate", "agent-ledger integrations production-gate", "agent-ledger agent recommend", "agent-ledger adapter conformance", "agent-ledger otel convert", "agent-ledger a2a convert", "agent-ledger provider convert"},
				MCPTools:     []string{"ledger.provider_profiles", "ledger.agent_profiles", "ledger.signal_taxonomy", "ledger.signal_coverage", "ledger.integration_readiness", "ledger.integration_smoke", "ledger.integration_compatibility", "ledger.integration_rollout_plan", "ledger.integration_evidence_kit", "ledger.integration_drift", "ledger.integration_lockfile", "ledger.integration_upgrade_gate", "ledger.integration_production_gate", "ledger.integration_recommendation", "ledger.adapter_conformance"},
				MCPResources: []string{"agent-ledger://integrations/provider-profiles", "agent-ledger://integrations/agent-profiles", "agent-ledger://integrations/signal-taxonomy", "agent-ledger://integrations/signal-coverage", "agent-ledger://integrations/readiness", "agent-ledger://integrations/smoke", "agent-ledger://integrations/compatibility", "agent-ledger://integrations/rollout-plan", "agent-ledger://integrations/evidence-kit", "agent-ledger://integrations/drift", "agent-ledger://integrations/lockfile", "agent-ledger://integrations/upgrade-gate", "agent-ledger://integrations/production-gate", "agent-ledger://integrations/recommendation", "agent-ledger://integrations/adapter-contract"},
				Tests:        []string{"internal/collector/*_test.go", "internal/integrations/conformance_test.go", "internal/integrations/provider_profiles_test.go", "internal/integrations/agent_profiles_test.go", "internal/integrations/signal_taxonomy_test.go", "internal/integrations/signal_coverage_test.go", "internal/integrations/integration_readiness_test.go", "internal/integrations/integration_smoke_test.go", "internal/integrations/integration_compatibility_test.go", "internal/integrations/integration_rollout_test.go", "internal/integrations/integration_evidence_test.go", "internal/integrations/integration_drift_test.go", "internal/integrations/integration_lockfile_test.go", "internal/integrations/integration_upgrade_gate_test.go", "internal/integrations/integration_production_gate_test.go", "internal/integrations/recommendations_test.go", "internal/server/otel_test.go", "internal/server/otel_grpc_test.go", "internal/server/provider_test.go", "internal/server/gateway_test.go"},
				Docs:         []string{"examples/adapter-fixtures", "examples/otel-collector/README.md"},
			},
			Privacy:     "Adapters map metadata and token fields; request/response messages, headers, prompts, and secrets are excluded from persistence.",
			Limitations: []string{"OTLP receiver and live provider gateway are implemented but disabled by default and require production-gate approval before shared deployment."},
		},
		{
			ID:            "pricing_cost_accuracy",
			Title:         "Pricing Governance And Cost Accuracy",
			Category:      "finops",
			Maturity:      "production-local",
			Objective:     "Use official seeds, LiteLLM fallback, local overrides, pricing provenance, stale/unpriced/fuzzy diagnostics, and explicit cost recalculation.",
			CapabilityIDs: []string{"governance.pricing"},
			Evidence: GoalCoverageEvidence{
				Endpoints: []string{"GET /api/pricing/status", "POST /api/pricing/sync", "POST /api/pricing/recalculate", "GET /api/pricing/audit", "GET /api/model-registry"},
				Commands:  []string{"agent-ledger pricing sync"},
				Tables:    []string{"pricing", "pricing_sources", "pricing_snapshots", "pricing_rules", "pricing_audit_events"},
				Tests:     []string{"internal/pricing/pricing_test.go", "internal/storage/costs_test.go", "internal/storage/governance_test.go"},
			},
			Privacy: "Pricing sync may fetch public model prices; local usage data is not uploaded.",
		},
		{
			ID:        "data_quality_performance_foundation",
			Title:     "Data Trust, Aggregates And Performance Foundation",
			Category:  "performance",
			Status:    "implemented",
			Maturity:  "production-local",
			Objective: "Prefer aggregate-backed dashboard reads, source-scoped dedup, cursor/page bounds, projection repair, health diagnostics, and one-click doctor checks.",
			Evidence: GoalCoverageEvidence{
				Endpoints: []string{"GET /api/dashboard", "GET /api/data-quality", "GET /api/doctor", "GET /api/health/ingestion", "GET /api/sessions", "POST /api/projections/repair"},
				Commands:  []string{"agent-ledger doctor", "agent-ledger projection quality", "agent-ledger projection repair"},
				Tables:    []string{"hourly_usage_aggregate", "daily_usage_aggregate", "model_usage_aggregate", "project_usage_aggregate", "ingestion_health", "file_state", "prompt_events"},
				Tests:     []string{"internal/storage/storage_test.go", "internal/storage/doctor_test.go", "internal/server/insights_test.go"},
			},
			Privacy: "Diagnostics report counts, provenance, confidence, and remediation hints rather than raw paths or prompt content.",
		},
		{
			ID:        "budget_quota_anomaly_watchdog",
			Title:     "Budget, Quota, Burn Rate, Anomaly And Watchdog",
			Category:  "finops",
			Status:    "implemented",
			Maturity:  "local-preview",
			Objective: "Estimate quota windows, budget burn, local budget events, robust anomaly signals, and runaway-agent watchdog risks without blocking agents by default.",
			Evidence: GoalCoverageEvidence{
				Endpoints:    []string{"GET /api/budgets/status", "GET /api/quota/status", "GET /api/anomalies", "GET /api/watchdog/events"},
				Commands:     []string{"agent-ledger battery"},
				MCPResources: []string{"agent-ledger://budget/current"},
				Tables:       []string{"budget_events", "insight_events"},
				Tests:        []string{"internal/server/budget.go", "internal/server/insights_test.go", "internal/storage/governance_test.go"},
			},
			Privacy: "Quota and budgets are local estimates; subscription quota is not presented as provider invoice truth.",
		},
		{
			ID:        "cost_intelligence_productivity",
			Title:     "Cost Intelligence, Cache Doctor And Productivity Insights",
			Category:  "productivity",
			Status:    "implemented",
			Maturity:  "local-preview",
			Objective: "Explain expensive sessions, cache behavior, model call counts, replay costs, estimate preflight work, simulate model routing, generate badges, wrapped reports, and fleet attribution.",
			Evidence: GoalCoverageEvidence{
				Endpoints: []string{"GET /api/cost-intelligence", "GET /api/cache/doctor", "GET /api/model-calls", "GET /api/session-replay", "GET /api/preflight/estimate", "GET /api/router/simulate", "GET /api/badge/repo.svg", "GET /api/wrapped", "GET /api/fleet-attribution"},
				Commands:  []string{"agent-ledger top", "agent-ledger replay", "agent-ledger preflight", "agent-ledger router simulate", "agent-ledger badge", "agent-ledger wrapped", "agent-ledger fleet"},
				MCPTools:  []string{"ledger.explain_cost", "ledger.find_similar_workloads"},
				Tests:     []string{"internal/storage/session_replay_test.go", "internal/storage/preflight_test.go", "internal/storage/router_simulator_test.go", "internal/storage/wrapped_test.go", "internal/storage/fleet_test.go", "internal/server/badge_test.go"},
			},
			Privacy: "Insights use token/cost/session metadata only and do not analyze prompt content.",
		},
		{
			ID:            "team_finops_audit_policy_notifications",
			Title:         "Team FinOps, Audit, Policy, RBAC And Notifications",
			Category:      "enterprise",
			Maturity:      "local-preview",
			Objective:     "Support local RBAC/read-only mode, advisory policy rules, approvals, audit logs, chargeback/showback, webhook dry-runs, redacted outbound notifications, and desktop adapter payloads.",
			CapabilityIDs: []string{"governance.policy_evaluator", "notification.redacted_webhook", "notification.desktop_adapter"},
			Evidence: GoalCoverageEvidence{
				Endpoints: []string{"GET /api/audit-log", "GET /api/chargeback", "GET /api/policies/status", "POST /api/policy/evaluate", "GET /api/policy/audit", "GET /api/policy/enforcement", "GET /api/policy/approvals", "GET /api/policy/approval-routes", "POST /api/notifications/webhook", "GET /api/notifications/desktop"},
				Commands:  []string{"agent-ledger chargeback", "agent-ledger audit", "agent-ledger policy evaluate", "agent-ledger policy approvals", "agent-ledger policy routes", "agent-ledger notify webhook --dry-run", "agent-ledger notify desktop"},
				MCPTools:  []string{"ledger.get_policy", "ledger.policy_audit", "ledger.approvals", "ledger.approval_routes", "ledger.resolve_approval", "ledger.audit_log"},
				Tables:    []string{"audit_events", "policy_decisions", "policy_approval_requests", "policy_approval_votes"},
				Tests:     []string{"internal/server/policy_test.go", "internal/server/audit_test.go", "internal/notifications/webhook_test.go", "internal/controlplane/admission_test.go"},
			},
			Privacy: "Webhook and desktop payloads are redacted; webhook delivery is disabled by default and never includes prompts, paths, project names, branch names, secrets, or raw IDs.",
		},
		{
			ID:            "reports_reconciliation_offline_evidence",
			Title:         "Reports, Reconciliation, Evidence And Offline Bundles",
			Category:      "reports",
			Maturity:      "local-preview",
			Objective:     "Export CSV/JSON, produce Markdown reports, reconcile provider statements, generate privacy-safe evidence, and import/export signed offline bundles for air-gapped aggregation.",
			CapabilityIDs: []string{"finops.provider_reconciliation", "protocol.offline_bundle"},
			Evidence: GoalCoverageEvidence{
				Endpoints: []string{"GET /api/export", "GET /api/report", "GET /api/reconciliation/status", "POST /api/reconciliation/import", "GET /api/evidence-bundle", "GET /api/offline-bundle/export", "POST /api/offline-bundle/import"},
				Commands:  []string{"agent-ledger export", "agent-ledger reconcile import", "agent-ledger reconcile status", "agent-ledger bundle export --privacy --signed", "agent-ledger bundle import --verify"},
				Tests:     []string{"internal/reconciliation/parser_test.go", "internal/storage/offline_bundle_test.go", "internal/server/insights_test.go"},
			},
			Privacy: "Evidence and bundles support redaction and signature/hash integrity; raw provider rows and prompt bodies are not persisted.",
		},
		{
			ID:        "ui_ux_static_dashboard",
			Title:     "Embedded Black/White/Gray Data-Dense UI",
			Category:  "ui",
			Status:    "implemented",
			Maturity:  "local-preview",
			Objective: "Serve a no-framework embedded dashboard with scoped filters, operations, privacy mode, data quality, cost intelligence, workload views, and paginated ledgers.",
			Evidence: GoalCoverageEvidence{
				Endpoints: []string{"/"},
				Docs:      []string{"internal/server/static/index.html", "internal/server/static/app.js", "internal/server/static/styles.css"},
				Commands:  []string{"agent-ledger ui check"},
				Tests:     []string{"internal/ui/contract_test.go", "node --check internal/server/static/app.js", "agent-ledger ui check viewport matrix at 375/768/1024/1440px"},
			},
			Privacy: "UI privacy mode hides sensitive identifiers in screenshots and shareable views.",
		},
	}
	for i := range sections {
		if sections[i].Status == "" {
			sections[i].Status = deriveCoverageStatus(sections[i].CapabilityIDs, capabilities)
		}
		sections[i].Evidence.Capabilities = append([]string{}, sections[i].CapabilityIDs...)
		sort.Strings(sections[i].Evidence.Capabilities)
	}
	return sections
}

func deriveCoverageStatus(ids []string, capabilities map[string]Capability) string {
	if len(ids) == 0 {
		return "implemented"
	}
	missing := 0
	experimental := 0
	for _, id := range ids {
		cap, ok := capabilities[id]
		if !ok {
			missing++
			continue
		}
		switch cap.Status {
		case "implemented":
		case "experimental":
			experimental++
		default:
			missing++
		}
	}
	if missing > 0 {
		return "gap"
	}
	if experimental > 0 {
		return "experimental"
	}
	return "implemented"
}

func goalCoverageSummary(sections []GoalCoverageSection) GoalCoverageSummary {
	summary := GoalCoverageSummary{TotalSections: len(sections)}
	for _, section := range sections {
		switch section.Status {
		case "implemented":
			summary.Implemented++
		case "experimental":
			summary.Experimental++
		case "external_dependency":
			summary.ExternalDependencies++
		default:
			summary.Gaps++
		}
	}
	if summary.TotalSections > 0 {
		summary.CompletionRatio = float64(summary.Implemented+summary.Experimental+summary.ExternalDependencies) / float64(summary.TotalSections)
	}
	switch {
	case summary.Gaps > 0:
		summary.NextAction = "close sections marked gap before claiming goal completion"
	case summary.Experimental > 0:
		summary.NextAction = "keep experimental surfaces disabled by default and require explicit smoke tests before production enablement"
	default:
		summary.NextAction = "maintain verification suite and rerun coverage after every control-plane change"
	}
	return summary
}

func GoalCoverageHasGap(report GoalCoverageReport) bool {
	return report.Summary.Gaps > 0 || strings.EqualFold(report.Status, "gaps")
}

func GoalCompletionAuditFor(report GoalCoverageReport) GoalCompletionAudit {
	checks := []GoalCompletionAuditCheck{
		goalCompletionCheck("local_first_privacy", passBlock(report.LocalFirst && !report.PromptContentStored && !report.UsageDataUploaded), "critical", "local-first privacy invariant is explicit", "local_first="+boolString(report.LocalFirst)+",prompt_content_stored="+boolString(report.PromptContentStored)+",usage_data_uploaded="+boolString(report.UsageDataUploaded), "Agent Ledger must remain local-first and metadata-only before goal completion can be claimed.", "repair privacy flags and rerun goal coverage", []string{"agent-ledger goal coverage"}),
		goalCompletionCheck("coverage_gaps", passBlock(!GoalCoverageHasGap(report)), "critical", "goal coverage has no actionable gaps", "gaps="+intString(report.Summary.Gaps)+",status="+report.Status, "Every goal section must be implemented, experimental with review controls, or an explicit external dependency.", "close gap sections before marking the goal complete", []string{"agent-ledger goal coverage", "agent-ledger contracts verify"}),
		goalCompletionCheck("verification_commands", passBlock(len(report.Verification) >= 8), "critical", "goal coverage declares a complete verification command set", "commands="+intString(len(report.Verification)), "Completion requires repeatable verification commands, not only implementation evidence.", "add missing verification commands and rerun the suite", report.Verification),
	}

	experimental := report.Summary.Experimental
	if experimental > 0 {
		checks = append(checks, goalCompletionCheck("experimental_surfaces", "review", "warning", "experimental or local-preview sections still require operator acceptance", "experimental_sections="+intString(experimental), "Experimental surfaces may be implemented locally but are not equivalent to fully completed production readiness.", "keep preview/outbound/write-ingest surfaces behind production-gate approval or promote them only after stronger smoke evidence", []string{"agent-ledger integrations production-gate --strict", "agent-ledger integrations readiness", "agent-ledger integrations smoke"}))
	} else {
		checks = append(checks, goalCompletionCheck("experimental_surfaces", "pass", "warning", "no experimental sections remain", "experimental_sections=0", "All sections are stable or externalized.", "no action required", []string{"agent-ledger goal coverage"}))
	}

	if len(report.ExternalDependencies) > 0 {
		checks = append(checks, goalCompletionCheck("external_dependencies", "review", "warning", "external dependencies remain outside local implementation control", "external_dependencies="+intString(len(report.ExternalDependencies)), "The goal cannot claim externally controlled behavior as implemented by this repository.", "keep dependencies disclosed and avoid marking the goal complete until accepted or removed from scope", []string{"agent-ledger goal coverage"}))
	} else {
		checks = append(checks, goalCompletionCheck("external_dependencies", "pass", "warning", "no external dependencies remain", "external_dependencies=0", "All requirements are locally verifiable.", "no action required", []string{"agent-ledger goal coverage"}))
	}

	remainingSections := sectionsWithRemaining(report.Sections)
	if remainingSections > 0 {
		checks = append(checks, goalCompletionCheck("remaining_items", "review", "warning", "some sections still disclose remaining verification work", "sections_with_remaining="+intString(remainingSections), "Remaining items are allowed during active goal progress but must be resolved or explicitly accepted before completion.", "complete remaining verification or document accepted residual risk", []string{"agent-ledger goal coverage", "agent-ledger ui check"}))
	} else {
		checks = append(checks, goalCompletionCheck("remaining_items", "pass", "warning", "no sections disclose remaining work", "sections_with_remaining=0", "Coverage sections do not list residual work.", "no action required", []string{"agent-ledger goal coverage"}))
	}

	summary := summarizeGoalCompletionAudit(checks, report)
	status, reason, ready, exitCode := goalCompletionDecision(summary)
	return GoalCompletionAudit{
		Contract:                "agent-ledger.goal-completion-audit",
		Version:                 "v1",
		Status:                  status,
		ReadyToMarkGoalComplete: ready,
		RecommendedCIExitCode:   exitCode,
		Reason:                  reason,
		Summary:                 summary,
		Checks:                  checks,
		RequiredVerification:    append([]string{}, report.Verification...),
		Privacy:                 "Completion audit uses only goal coverage metadata, counts, section ids, command names, and external dependency labels; it does not inspect SQLite rows, prompts, responses, local paths, session ids, machine names, authors, or secrets.",
	}
}

func summarizeGoalCompletionAudit(checks []GoalCompletionAuditCheck, report GoalCoverageReport) GoalCompletionAuditSummary {
	summary := GoalCompletionAuditSummary{
		TotalChecks:           len(checks),
		CoverageGaps:          report.Summary.Gaps,
		ExperimentalSections:  report.Summary.Experimental,
		ExternalDependencies:  len(report.ExternalDependencies),
		SectionsWithRemaining: sectionsWithRemaining(report.Sections),
		VerificationCommands:  len(report.Verification),
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
	return summary
}

func goalCompletionDecision(summary GoalCompletionAuditSummary) (string, string, bool, int) {
	if summary.Blocked > 0 {
		return "incomplete", "one or more critical completion checks are blocked", false, 1
	}
	if summary.Review > 0 {
		reasons := []string{}
		if summary.ExperimentalSections > 0 {
			reasons = append(reasons, "experimental surfaces")
		}
		if summary.ExternalDependencies > 0 {
			reasons = append(reasons, "external dependencies")
		}
		if summary.SectionsWithRemaining > 0 {
			reasons = append(reasons, "remaining verification work")
		}
		if len(reasons) == 0 {
			reasons = append(reasons, "review items")
		}
		return "review-required", "coverage is implemented, but " + strings.Join(reasons, ", ") + " require explicit acceptance before marking the goal complete", false, 2
	}
	return "complete", "all completion checks passed and no review items remain", true, 0
}

func goalCompletionCheck(id, status, severity, message, evidence, requirement, remediation string, verification []string) GoalCompletionAuditCheck {
	return GoalCompletionAuditCheck{
		ID:           id,
		Status:       status,
		Severity:     severity,
		Message:      message,
		Evidence:     evidence,
		Requirement:  requirement,
		Remediation:  remediation,
		Verification: append([]string{}, verification...),
	}
}

func passBlock(ok bool) string {
	if ok {
		return "pass"
	}
	return "block"
}

func sectionsWithRemaining(sections []GoalCoverageSection) int {
	count := 0
	for _, section := range sections {
		if len(section.Remaining) > 0 {
			count++
		}
	}
	return count
}
