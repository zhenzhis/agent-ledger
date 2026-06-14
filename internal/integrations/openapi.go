package integrations

import (
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

const (
	defaultJSONBodyLimitBytes   = 1 << 20
	defaultIngestBodyLimitBytes = 4 << 20
	defaultBundleBodyLimitBytes = 32 << 20
)

// OpenAPISpecFor returns a compact OpenAPI 3.1 description for the stable
// metadata-only control-plane surfaces. It intentionally describes contracts
// and envelope shapes instead of local files, prompt content, or secrets.
func OpenAPISpecFor(opts Options, runtime *storage.RuntimeStatus) map[string]interface{} {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	catalog := Registry(opts)
	discovery := Discovery(opts)
	spec := map[string]interface{}{
		"openapi": "3.1.0",
		"info": map[string]interface{}{
			"title":       "Agent Ledger Control Plane API",
			"summary":     "Local-first metadata-only AgentOps and FinOps control-plane API.",
			"description": "Stable REST contract surfaces for discovery, canonical events, adapter conformance, workload state, and runtime probes. Prompt and response content are outside this API contract.",
			"version":     "v1",
		},
		"servers": []map[string]string{
			{"url": "/", "description": "Same-origin local Agent Ledger server"},
		},
		"tags": []map[string]string{
			{"name": "contracts", "description": "Discovery, contract bundle, OpenAPI, runtime, and capability metadata"},
			{"name": "canonical-events", "description": "Metadata-only canonical event schema, validation, and ingest"},
			{"name": "adapter-conformance", "description": "Adapter contract and dry-run fixture validation"},
			{"name": "dashboard", "description": "Read-only usage, cost, token, session, and workload analytics"},
			{"name": "operations", "description": "Local-only scan, repair, and projection maintenance operations"},
			{"name": "finops", "description": "Pricing governance, budgets, quota estimates, chargeback, reconciliation, and model routing economics"},
			{"name": "diagnostics", "description": "Data quality, doctor, cost intelligence, cache diagnostics, anomalies, watchdog, and audit evidence"},
			{"name": "governance", "description": "Policy evaluation, enforcement evidence, approval queues, and redacted notifications"},
			{"name": "reports", "description": "Privacy-aware exports, reports, bundles, wrapped summaries, and SVG badges"},
			{"name": "workload-control", "description": "Retry-safe workload and agent-run control-plane writes"},
			{"name": "workload-feed", "description": "Cursor-stable workload state feed for local monitors and routers"},
			{"name": "ecosystem-ingest", "description": "Metadata-only telemetry and provider usage ingestion for agent frameworks and observability protocols"},
			{"name": "provider-gateway", "description": "Optional local provider proxy surfaces that meter usage without persisting prompt or response content"},
		},
		"x-agent-ledger": map[string]interface{}{
			"contract":                "agent-ledger.control-plane-openapi",
			"version":                 "v1",
			"local_first":             true,
			"privacy_default":         catalog.PrivacyDefault,
			"read_only":               opts.ReadOnly,
			"prompt_content_stored":   false,
			"usage_data_uploaded":     false,
			"discovery_hash":          hashJSONPayload(discovery),
			"capability_catalog_hash": CatalogFingerprintFrom(catalog),
			"runtime_status_hash":     hashJSONPayload(runtime),
			"canonical_schema_hash":   storage.CanonicalEventSchemaFingerprint(),
			"adapter_spec_hash":       AdapterContractFingerprint(),
		},
		"paths": map[string]interface{}{
			"/.well-known/agent-ledger.json":      getOperation("contracts", "Get discovery manifest", "Privacy-safe local discovery manifest.", "DiscoveryManifest"),
			"/api/discovery":                      getOperation("contracts", "Get discovery manifest", "Same discovery manifest under the API namespace.", "DiscoveryManifest"),
			"/api/contracts":                      getOperation("contracts", "Get contract bundle", "One-shot contract index with document hashes, revalidation semantics, CLI commands, and MCP entrypoints.", "ContractBundle"),
			"/api/contracts/verify":               getOperation("contracts", "Verify control-plane contracts", "Machine-readable self-check for discovery, contract bundle, OpenAPI, schema, adapter, runtime, and privacy invariants.", "ContractVerificationReport"),
			"/api/openapi.json":                   getOperation("contracts", "Get OpenAPI document", "OpenAPI 3.1 control-plane contract document.", "OpenAPI"),
			"/api/integrations":                   getOperation("contracts", "Get integration catalog", "Privacy-safe integration capability catalog.", "CapabilityCatalog"),
			"/api/runtime/status":                 getOperation("contracts", "Get runtime status", "Process-local observer/control-plane mode and compatibility hashes.", "RuntimeStatus"),
			"/api/config/status":                  getOperation("contracts", "Get config status", "Privacy-safe deployment configuration status without paths, secrets, webhook URLs, prompt content, or session ids.", "ConfigStatusReport"),
			"/api/readiness":                      getOperation("contracts", "Get readiness", "Privacy-safe control-plane readiness for wrappers, routers, CI, and deployment checks.", "ReadinessReport"),
			"/api/admission/check":                getOperation("contracts", "Check operation admission", "Privacy-safe dry-run for HTTP, CLI, and MCP operation access in the current runtime.", "AdmissionDecision"),
			"/api/event-schema":                   getOperation("canonical-events", "Get canonical event schema", "Metadata-only canonical event contract and supported event types.", "CanonicalEventSchema"),
			"/api/event-examples":                 eventExamplesOperation(),
			"/api/events/validate":                canonicalEventPostOperation("canonical-events", "Validate canonical events", "Validate one or more canonical events without writing SQLite.", false),
			"/api/events":                         canonicalEventPostOperation("canonical-events", "Ingest canonical events", "Ingest one or more metadata-only canonical events.", true),
			"/api/integrations/adapter-spec":      getOperation("adapter-conformance", "Get adapter contract", "Machine-readable adapter contract for privacy-safe integrations.", "AdapterContract"),
			"/api/integrations/conformance":       adapterConformanceOperation(),
			"/api/stats":                          filteredReadOperation("dashboard", "Get dashboard stats", "Read aggregate tokens, cost, sessions, prompts, cache, and runtime totals for one scope.", "DashboardStats", scopedTimeParams()),
			"/api/dashboard":                      filteredReadOperation("dashboard", "Get dashboard bundle", "Read the aggregate dashboard bundle. Dashboard queries prefer aggregate tables and return consistency metadata.", "DashboardBundle", append(scopedTimeParams(), queryParam("granularity", "Chart bucket size such as 1h, 1d, or auto."))),
			"/api/cost-by-model":                  filteredReadOperation("dashboard", "Get cost by model", "Read cost distribution by model for a scoped time window.", "CostByModelRows", scopedTimeParams()),
			"/api/cost-over-time":                 filteredReadOperation("dashboard", "Get cost over time", "Read aggregate cost trend points for a scoped time window and granularity.", "CostTrendRows", append(scopedTimeParams(), queryParam("granularity", "Chart bucket size such as 1h, 1d, or auto."))),
			"/api/tokens-over-time":               filteredReadOperation("dashboard", "Get tokens over time", "Read aggregate token trend points for a scoped time window and granularity.", "TokenTrendRows", append(scopedTimeParams(), queryParam("granularity", "Chart bucket size such as 1h, 1d, or auto."))),
			"/api/sessions":                       filteredReadOperation("dashboard", "List session ledger", "Server-side paginated session ledger. Privacy filters may hash session ids and hide project metadata.", "SessionPage", append(scopedTimeParams(), paginationAndSortParams()...)),
			"/api/session-detail":                 filteredReadOperation("dashboard", "Get session detail", "Read one scoped session detail by source and session_id.", "SessionDetail", append([]map[string]interface{}{queryParam("source", "Source owning the session id."), queryParam("session_id", "Required native session id.")}, privacyParams()...)),
			"/api/session-replay":                 filteredReadOperation("dashboard", "Get session cost replay", "Read per-call token/cost replay points for one scoped session.", "SessionReplay", append([]map[string]interface{}{queryParam("source", "Source owning the session id."), queryParam("session_id", "Required native session id."), intQueryParam("limit", "Maximum replay points.")}, privacyParams()...)),
			"/api/workloads":                      workloadsOperation(),
			"/api/workloads/close":                workloadCloseOperation(),
			"/api/workloads/link":                 workloadLinkOperation(),
			"/api/workloads/claim-next":           workloadClaimNextOperation(),
			"/api/workloads/queue":                workloadQueueOperation(),
			"/api/workloads/lease":                workloadLeaseAcquireOperation(),
			"/api/workloads/lease/renew":          workloadLeaseRenewOperation(),
			"/api/workloads/lease/release":        workloadLeaseReleaseOperation(),
			"/api/workloads/leases":               workloadLeasesOperation(),
			"/api/agent-runs":                     agentRunsOperation(),
			"/api/agent-runs/heartbeat":           agentRunHeartbeatOperation(),
			"/api/agent-runs/liveness":            agentRunLivenessOperation(),
			"/api/workload-detail":                workloadDetailOperation(),
			"/api/workload-graph":                 workloadGraphOperation(),
			"/api/workload-timeline":              workloadTimelineOperation(),
			"/api/workload-state":                 workloadStateOperation(),
			"/api/workload-events":                workloadEventsOperation(false),
			"/api/workload-events/stream":         workloadEventsOperation(true),
			"/api/fleet-attribution":              filteredReadOperation("dashboard", "Get fleet attribution", "Read heuristic parent/child and parallel agent attribution with privacy filters.", "FleetAttributionReport", append(scopedTimeParams(), intQueryParam("limit", "Maximum fleet rows."))),
			"/api/otel/genai":                     ecosystemIngestOperation("Ingest OpenTelemetry GenAI spans", "Convert OpenTelemetry GenAI JSON spans into metadata-only canonical events.", "OTelGenAIRequest", "EcosystemIngestResponse", false),
			"/api/otlp/v1/traces":                 otlpTracesOperation(),
			"/v1/traces":                          otlpTracesOperation(),
			"/api/a2a/tasks":                      ecosystemIngestOperation("Ingest A2A task telemetry", "Convert A2A JSON task snapshots/events into workload, run, artifact, evaluation, and policy metadata events.", "A2ATaskRequest", "EcosystemIngestResponse", false),
			"/api/provider/calls":                 ecosystemIngestOperation("Ingest provider usage envelopes", "Convert OpenAI-compatible, Anthropic-style, and LiteLLM-style usage envelopes into metadata-only model.call events.", "ProviderUsageRequest", "EcosystemIngestResponse", false),
			"/gateway/openai/v1/chat/completions": gatewayOperation("Proxy OpenAI-compatible chat completions", "Optional local OpenAI-compatible Chat Completions JSON/SSE proxy. Prompt content is forwarded in memory only; the ledger persists usage metadata only."),
			"/gateway/openai/v1/responses":        gatewayOperation("Proxy OpenAI Responses", "Optional local OpenAI Responses JSON/SSE proxy. Prompt content is forwarded in memory only; the ledger persists usage metadata only."),
			"/gateway/anthropic/v1/messages":      gatewayOperation("Proxy Anthropic Messages", "Optional local Anthropic Messages JSON/SSE proxy. Prompt content is forwarded in memory only; the ledger persists usage metadata only."),
			"/api/health/ingestion":               filteredReadOperation("operations", "Get ingestion health", "Read collector health, path reachability summaries, scan watermarks, row counts, and last errors.", "IngestionHealthRows", []map[string]interface{}{}),
			"/api/scan":                           localQueryWriteOperation("operations", "Run manual scan", "Run a local collector scan for all sources or one source. reset=true requires a source and clears only that source state before scanning.", "OperationResult", []map[string]interface{}{queryParam("source", "Optional source to scan."), boolQueryParam("reset", "Dangerous source-scoped cleanup before rescan.")}),
			"/api/recalculate-costs":              localQueryWriteOperation("operations", "Recalculate costs", "Recalculate local usage record costs from current pricing rules.", "OperationResult", []map[string]interface{}{queryParam("mode", "zero or all. Empty defaults to zero.")}),
			"/api/projections/repair":             localQueryWriteOperation("operations", "Repair usage projections", "Idempotently repair canonical-event to usage projection drift for one scoped time window.", "ProjectionRepairResult", scopedTimeParams()),
			"/api/pricing/status":                 filteredReadOperation("finops", "Get pricing status", "Read pricing source freshness, effective rule summary, confidence mix, and unpriced model groups.", "PricingStatus", []map[string]interface{}{}),
			"/api/pricing/sync":                   localQueryWriteOperation("finops", "Sync pricing", "Run configured official pricing adapters, LiteLLM fallback, and local overrides. Writes local pricing metadata and audit events.", "OperationResult", []map[string]interface{}{}),
			"/api/pricing/recalculate":            localQueryWriteOperation("finops", "Recalculate pricing", "Recalculate zero-cost or all local records from current pricing rules.", "OperationResult", []map[string]interface{}{queryParam("mode", "zero or all. Empty defaults to zero.")}),
			"/api/pricing/audit":                  filteredReadOperation("finops", "List pricing audit", "Read local pricing match, stale, fuzzy, unpriced, and source provenance audit rows.", "PricingAuditRows", []map[string]interface{}{intQueryParam("limit", "Maximum audit rows.")}),
			"/api/budgets/status":                 filteredReadOperation("finops", "Get budget status", "Read configured budget rules, current usage ratio, severity, and budget event state.", "BudgetStatusResponse", []map[string]interface{}{}),
			"/api/quota/status":                   volatileReadOperation("finops", "Get quota estimate", "Read local 5h/day/week/month quota and burn-rate estimates. Subscription quota is an estimate, not provider billing.", "QuotaStatus", []map[string]interface{}{}),
			"/api/data-quality":                   filteredReadOperation("diagnostics", "Get data quality", "Read trust and completeness diagnostics, unpriced models, malformed rows, duplicates, and provenance confidence.", "DataQualityReport", []map[string]interface{}{}),
			"/api/doctor":                         doctorOperation(),
			"/api/model-calls":                    filteredReadOperation("diagnostics", "Get model call analytics", "Read call counts by source, model, and project for detecting loops and unexpected model use.", "ModelCallRows", append(scopedTimeParams(), intQueryParam("limit", "Maximum model call groups."))),
			"/api/model-registry":                 filteredReadOperation("finops", "Get model registry", "Read model pricing governance registry with stale/unpriced/fuzzy state.", "ModelRegistryRows", []map[string]interface{}{intQueryParam("limit", "Maximum model registry rows.")}),
			"/api/cost-intelligence":              filteredReadOperation("diagnostics", "Get cost intelligence", "Read expensive session explanations and cost drivers without inspecting prompt content.", "CostIntelligenceRows", append(scopedTimeParams(), intQueryParam("limit", "Maximum insight rows."))),
			"/api/cache/doctor":                   filteredReadOperation("diagnostics", "Get cache doctor", "Read cache hit/write/read diagnostics and estimated cache miss risk by source/model/project.", "CacheDoctorRows", append(scopedTimeParams(), intQueryParam("limit", "Maximum cache diagnostic rows."))),
			"/api/anomalies":                      derivedReadOperation("diagnostics", "Get anomalies", "Read robust-statistics anomaly events. In control-plane mode the endpoint may update derived local anomaly rows; observer mode stays read-only.", "InsightEventRows", append(scopedTimeParams(), intQueryParam("limit", "Maximum anomaly rows."))),
			"/api/watchdog/events":                derivedReadOperation("diagnostics", "Get watchdog events", "Read local runaway, call-density, cache-miss-risk, and non-working-hour watchdog events.", "InsightEventRows", append(scopedTimeParams(), intQueryParam("limit", "Maximum watchdog rows."))),
			"/api/notifications/webhook":          localQueryWriteOperation("governance", "Send redacted webhook notification", "Send or dry-run a redacted local notification summary. Webhooks are disabled unless explicitly configured.", "WebhookNotificationResult", append(scopedTimeParams(), queryParam("dry_run", "true or 1 to return the redacted payload without sending."), queryParam("max_age", "Workload feed stale threshold."), queryParam("approval_due_within", "Approval route deadline window."))),
			"/api/audit-log":                      filteredReadOperation("diagnostics", "List audit log", "Read local audit rows for scans, exports, pricing sync, policy decisions, and control-plane operations.", "AuditLogRows", append([]map[string]interface{}{queryParam("from", "YYYY-MM-DD lower bound."), queryParam("to", "YYYY-MM-DD upper bound."), queryParam("actor", "Actor filter."), queryParam("role", "Role filter."), queryParam("action", "Action filter."), queryParam("target", "Target filter."), intQueryParam("limit", "Maximum audit rows.")}, privacyParams()...)),
			"/api/reconciliation/status":          filteredReadOperation("finops", "List reconciliation imports", "Read provider invoice/import reconciliation rows.", "ReconciliationRows", []map[string]interface{}{intQueryParam("limit", "Maximum reconciliation imports.")}),
			"/api/reconciliation/import":          flexibleWriteOperation("finops", "Import provider reconciliation", "Import provider CSV/JSON or manual balance information for local reconciliation.", "ReconciliationImportRequest", "ReconciliationImportResponse", []map[string]interface{}{queryParam("provider", "Provider name."), queryParam("format", "csv, json, or provider-specific parser."), queryParam("from", "Optional local cost window lower bound."), queryParam("to", "Optional local cost window upper bound.")}, []string{"application/json", "text/csv", "text/plain"}),
			"/api/router/simulate":                filteredReadOperation("finops", "Simulate model routing", "Estimate savings from replacing some calls from one model with another using local historical usage.", "RouterSimulationReport", append(scopedTimeParams(), queryParam("to_model", "Required target model."), queryParam("from_model", "Optional source model."), queryParam("ratio", "Replacement ratio from 0 to 1."), intQueryParam("limit", "Maximum rows."))),
			"/api/preflight/estimate":             filteredReadOperation("finops", "Estimate preflight cost", "Estimate likely cost, tokens, and duration for a task type from local historical sessions.", "PreflightEstimateReport", append(scopedTimeParams(), queryParam("task", "Task type such as refactor, debug, review, docs, or custom."), intQueryParam("limit", "Maximum historical rows."))),
			"/api/chargeback":                     filteredReadOperation("finops", "Get chargeback", "Read team/project/model/source showback and chargeback rows using local attribution rules.", "ChargebackRows", append(scopedTimeParams(), intQueryParam("limit", "Maximum chargeback rows."))),
			"/api/wrapped":                        wrappedOperation(),
			"/api/badge/repo.svg":                 badgeOperation(),
			"/api/evidence-bundle":                evidenceBundleOperation(),
			"/api/offline-bundle/export":          offlineBundleExportOperation(),
			"/api/offline-bundle/import":          flexibleWriteOperation("reports", "Import offline bundle", "Import a local offline usage bundle. Optional signature verification uses AGENT_LEDGER_BUNDLE_KEY from the environment.", "OfflineBundleImportRequest", "OfflineBundleImportResponse", []map[string]interface{}{boolQueryParam("verify", "Require bundle signature verification.")}, []string{"application/json"}),
			"/api/policies/status":                filteredReadOperation("governance", "Get policy status", "Read local policy configuration summary and advisory enforcement posture.", "PolicyStatus", []map[string]interface{}{}),
			"/api/policy/evaluate":                policyEvaluateOperation(),
			"/api/policy/audit":                   filteredReadOperation("governance", "Audit policy candidates", "Read policy audit findings over usage, tool, and workload candidates.", "PolicyAuditReport", append(scopedTimeParams(), intQueryParam("limit", "Maximum policy findings."))),
			"/api/policy/enforcement":             filteredReadOperation("governance", "Get policy enforcement evidence", "Read local policy enforcement decisions and approval evidence.", "PolicyEnforcementReport", []map[string]interface{}{intQueryParam("limit", "Maximum enforcement rows.")}),
			"/api/policy/decisions":               filteredReadOperation("governance", "List policy decisions", "Read policy decisions by workload id.", "PolicyDecisionRows", []map[string]interface{}{queryParam("workload_id", "Optional workload id filter."), intQueryParam("limit", "Maximum policy decisions.")}),
			"/api/policy/approvals":               policyApprovalsOperation(),
			"/api/policy/approval-routes":         filteredReadOperation("governance", "Get policy approval routes", "Read due approval route summary for operators and notification dry-runs.", "ApprovalRouteSummary", []map[string]interface{}{queryParam("due_within", "Duration from 1ns to 720h."), intQueryParam("limit", "Maximum route rows.")}),
			"/api/export":                         exportOperation(),
			"/api/report":                         reportOperation(),
		},
		"components": map[string]interface{}{
			"securitySchemes": map[string]interface{}{
				"AgentLedgerBearer": map[string]interface{}{
					"type":         "http",
					"scheme":       "bearer",
					"bearerFormat": "opaque local token",
					"description":  "Optional local bearer token. Required when Agent Ledger is configured with auth_token, admin_token, or viewer_token; localhost-only deployments may run without bearer auth.",
				},
			},
			"schemas": map[string]interface{}{
				"Hash": map[string]interface{}{
					"type":        "string",
					"pattern":     "^sha256:[a-f0-9]{64}$",
					"description": "Stable SHA-256 fingerprint.",
				},
				"DiscoveryManifest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
					"required":             []string{"contract", "version", "local_first", "contract_bundle_uri", "capability_catalog_hash", "canonical_schema_hash", "adapter_spec_hash"},
					"properties": map[string]interface{}{
						"contract":                constSchema("agent-ledger.discovery"),
						"version":                 stringSchema(),
						"contract_bundle_uri":     stringSchema(),
						"capability_catalog_hash": refSchema("Hash"),
						"canonical_schema_hash":   refSchema("Hash"),
						"adapter_spec_hash":       refSchema("Hash"),
						"prompt_content_stored":   boolSchema(),
						"usage_data_uploaded":     boolSchema(),
					},
				},
				"ContractBundle": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
					"required":             []string{"contract", "version", "bundle_hash", "documents"},
					"properties": map[string]interface{}{
						"contract":    constSchema("agent-ledger.contract-bundle"),
						"version":     stringSchema(),
						"bundle_hash": refSchema("Hash"),
						"documents": map[string]interface{}{
							"type":  "array",
							"items": refSchema("ContractDocument"),
						},
					},
				},
				"ContractDocument": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
					"required":             []string{"id", "contract", "version", "hash", "primary_uri", "read_only_safe", "writes_local_state"},
					"properties": map[string]interface{}{
						"id":                 stringSchema(),
						"name":               stringSchema(),
						"contract":           stringSchema(),
						"version":            stringSchema(),
						"hash":               refSchema("Hash"),
						"primary_uri":        stringSchema(),
						"read_only_safe":     boolSchema(),
						"writes_local_state": boolSchema(),
					},
				},
				"ContractVerificationReport": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": true,
					"required":             []string{"contract", "version", "ok", "checked", "failed", "bundle_hash", "openapi_hash", "checks"},
					"properties": map[string]interface{}{
						"contract":     constSchema("agent-ledger.contract-verification"),
						"version":      stringSchema(),
						"ok":           boolSchema(),
						"checked":      map[string]interface{}{"type": "integer", "minimum": 0},
						"failed":       map[string]interface{}{"type": "integer", "minimum": 0},
						"bundle_hash":  refSchema("Hash"),
						"openapi_hash": refSchema("Hash"),
						"read_only":    boolSchema(),
						"privacy":      stringSchema(),
						"checks": map[string]interface{}{
							"type":  "array",
							"items": refSchema("ContractVerificationCheck"),
						},
					},
				},
				"ContractVerificationCheck": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"name", "ok", "severity", "message"},
					"properties": map[string]interface{}{
						"name":     stringSchema(),
						"ok":       boolSchema(),
						"severity": stringSchema(),
						"message":  stringSchema(),
						"expected": stringSchema(),
						"actual":   stringSchema(),
					},
				},
				"CapabilityCatalog":    looseObjectSchema("Integration capability catalog."),
				"RuntimeStatus":        looseObjectSchema("Process-local runtime mode and compatibility hashes."),
				"ConfigStatusReport":   looseObjectSchema("Privacy-safe deployment configuration status."),
				"ReadinessReport":      looseObjectSchema("Privacy-safe control-plane readiness report."),
				"AdmissionDecision":    looseObjectSchema("Privacy-safe control-plane admission decision."),
				"CanonicalEventSchema": looseObjectSchema("Canonical event contract metadata."),
				"AdapterContract":      looseObjectSchema("Machine-readable adapter contract."),
				"CanonicalEvent": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"source", "event_type", "payload"},
					"properties": map[string]interface{}{
						"source":          stringSchema(),
						"event_type":      stringSchema(),
						"event_id":        stringSchema(),
						"schema_version":  stringSchema(),
						"source_version":  stringSchema(),
						"parser_version":  stringSchema(),
						"source_event_id": stringSchema(),
						"raw_ref":         stringSchema(),
						"match_type":      stringSchema(),
						"workload_id":     stringSchema(),
						"agent_run_id":    stringSchema(),
						"session_id":      stringSchema(),
						"model":           stringSchema(),
						"project":         stringSchema(),
						"git_branch":      stringSchema(),
						"timestamp":       stringSchema(),
						"confidence":      numberSchema(),
						"payload":         looseObjectSchema("Metadata-only event payload. Raw prompt/content fields are rejected by the server."),
					},
				},
				"CanonicalEventRequest": map[string]interface{}{
					"oneOf": []map[string]interface{}{
						refSchema("CanonicalEvent"),
						{"type": "array", "items": refSchema("CanonicalEvent"), "maxItems": 500},
					},
				},
				"ValidationResponse": looseObjectSchema("Validation result for one or more canonical events."),
				"IngestResponse":     looseObjectSchema("Ingest result for one or more canonical events."),
				"WorkloadCreateRequest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"goal"},
					"properties": map[string]interface{}{
						"goal":            stringSchema(),
						"source":          stringSchema(),
						"project":         stringSchema(),
						"repo":            stringSchema(),
						"git_branch":      stringSchema(),
						"owner":           stringSchema(),
						"team":            stringSchema(),
						"budget_usd":      numberSchema(),
						"idempotency_key": stringSchema(),
					},
				},
				"WorkloadCreateResponse": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"ok", "workload_id", "idempotent_replay"},
					"properties": map[string]interface{}{
						"ok":                boolSchema(),
						"workload_id":       stringSchema(),
						"idempotent_replay": boolSchema(),
					},
				},
				"WorkloadCloseRequest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"workload_id"},
					"properties": map[string]interface{}{
						"workload_id": stringSchema(),
						"status":      stringSchema(),
						"outcome":     stringSchema(),
					},
				},
				"WorkloadCloseResponse": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"ok", "workload_id", "status"},
					"properties": map[string]interface{}{
						"ok":          boolSchema(),
						"workload_id": stringSchema(),
						"status":      stringSchema(),
					},
				},
				"WorkloadLinkRequest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"source_workload_id", "target_workload_id"},
					"properties": map[string]interface{}{
						"source_workload_id": stringSchema(),
						"target_workload_id": stringSchema(),
						"relation":           stringSchema(),
						"reason":             stringSchema(),
						"created_by":         stringSchema(),
						"confidence":         numberSchema(),
					},
				},
				"WorkloadLinkResponse": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"ok", "link_id", "source_workload_id", "target_workload_id"},
					"properties": map[string]interface{}{
						"ok":                 boolSchema(),
						"link_id":            stringSchema(),
						"source_workload_id": stringSchema(),
						"target_workload_id": stringSchema(),
					},
				},
				"WorkloadLeaseAcquireRequest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"workload_id", "holder"},
					"properties": map[string]interface{}{
						"workload_id": stringSchema(),
						"holder":      stringSchema(),
						"purpose":     stringSchema(),
						"ttl":         stringSchema(),
						"ttl_seconds": map[string]interface{}{"type": "integer", "minimum": 1},
					},
				},
				"WorkloadClaimNextRequest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"holder"},
					"properties": map[string]interface{}{
						"holder":      stringSchema(),
						"purpose":     stringSchema(),
						"ttl":         stringSchema(),
						"ttl_seconds": map[string]interface{}{"type": "integer", "minimum": 1},
						"source":      stringSchema(),
						"project":     stringSchema(),
						"repo":        stringSchema(),
						"team":        stringSchema(),
						"owner":       stringSchema(),
						"status":      stringSchema(),
						"q":           stringSchema(),
					},
				},
				"WorkloadLeaseRenewRequest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"lease_id", "lease_token"},
					"properties": map[string]interface{}{
						"lease_id":    stringSchema(),
						"lease_token": stringSchema(),
						"ttl":         stringSchema(),
						"ttl_seconds": map[string]interface{}{"type": "integer", "minimum": 1},
					},
				},
				"WorkloadLeaseReleaseRequest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"lease_id", "lease_token"},
					"properties": map[string]interface{}{
						"lease_id":    stringSchema(),
						"lease_token": stringSchema(),
					},
				},
				"WorkloadLease": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"properties": map[string]interface{}{
						"lease_id":        stringSchema(),
						"workload_id":     stringSchema(),
						"holder":          stringSchema(),
						"purpose":         stringSchema(),
						"status":          stringSchema(),
						"acquired_at":     stringSchema(),
						"expires_at":      stringSchema(),
						"last_renewed_at": stringSchema(),
						"released_at":     stringSchema(),
						"expired":         boolSchema(),
						"ttl_seconds":     map[string]interface{}{"type": "integer"},
						"lease_token":     stringSchema(),
					},
				},
				"WorkloadLeaseResponse": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"ok", "lease"},
					"properties": map[string]interface{}{
						"ok":    boolSchema(),
						"lease": refSchema("WorkloadLease"),
					},
				},
				"WorkloadClaimNextResponse": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"ok", "empty"},
					"properties": map[string]interface{}{
						"ok":          boolSchema(),
						"empty":       boolSchema(),
						"workload_id": stringSchema(),
						"workload":    looseObjectSchema("Claimed workload summary."),
						"lease":       refSchema("WorkloadLease"),
					},
				},
				"WorkloadQueueStats": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"ok", "generated_at", "claim_statuses", "claimable", "non_terminal", "active_leases", "expired_leases", "by_status"},
					"properties": map[string]interface{}{
						"ok":                   boolSchema(),
						"generated_at":         stringSchema(),
						"claim_statuses":       map[string]interface{}{"type": "array", "items": stringSchema()},
						"claimable":            map[string]interface{}{"type": "integer"},
						"non_terminal":         map[string]interface{}{"type": "integer"},
						"active_leases":        map[string]interface{}{"type": "integer"},
						"expired_leases":       map[string]interface{}{"type": "integer"},
						"by_status":            map[string]interface{}{"type": "object", "additionalProperties": map[string]interface{}{"type": "integer"}},
						"oldest_claimable_at":  stringSchema(),
						"next_lease_expiry_at": stringSchema(),
					},
				},
				"WorkloadLeaseListResponse": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"rows", "include_inactive"},
					"properties": map[string]interface{}{
						"rows":             map[string]interface{}{"type": "array", "items": refSchema("WorkloadLease")},
						"include_inactive": boolSchema(),
					},
				},
				"AgentRunStartRequest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"workload_id"},
					"properties": map[string]interface{}{
						"workload_id":     stringSchema(),
						"source":          stringSchema(),
						"agent_name":      stringSchema(),
						"command":         stringSchema(),
						"cwd":             stringSchema(),
						"idempotency_key": stringSchema(),
					},
				},
				"AgentRunStartResponse": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"ok", "workload_id", "run_id", "status", "idempotent_replay"},
					"properties": map[string]interface{}{
						"ok":                boolSchema(),
						"workload_id":       stringSchema(),
						"run_id":            stringSchema(),
						"status":            stringSchema(),
						"idempotent_replay": boolSchema(),
					},
				},
				"AgentRunHeartbeatRequest": map[string]interface{}{
					"type":                 "object",
					"additionalProperties": false,
					"required":             []string{"run_id"},
					"properties": map[string]interface{}{
						"event_id":  stringSchema(),
						"run_id":    stringSchema(),
						"status":    stringSchema(),
						"phase":     stringSchema(),
						"message":   stringSchema(),
						"progress":  numberSchema(),
						"metrics":   map[string]interface{}{"type": "object", "additionalProperties": true},
						"timestamp": stringSchema(),
					},
				},
				"AgentRunHeartbeatResponse":        looseObjectSchema("Recorded metadata-only agent run heartbeat."),
				"AgentRunLivenessResponse":         looseObjectSchema("Active async agent run liveness rows with privacy filters applied by the server."),
				"WorkloadDetail":                   looseObjectSchema("Full workload ledger detail with privacy filters applied by the server."),
				"WorkloadGraph":                    looseObjectSchema("Compact workload dependency and activity graph."),
				"WorkloadTimelineResponse":         looseObjectSchema("Chronological metadata-only workload audit timeline."),
				"WorkloadState":                    looseObjectSchema("Derived terminal-state snapshot for one async agent workload."),
				"WorkloadEventFeed":                looseObjectSchema("Cursor-stable workload state feed."),
				"DashboardStats":                   dashboardStatsSchema(),
				"DashboardConsistencyIssue":        dashboardConsistencyIssueSchema(),
				"CostByModel":                      costByModelSchema(),
				"TimeSeriesPoint":                  timeSeriesPointSchema(),
				"TokenTimeSeriesPoint":             tokenTimeSeriesPointSchema(),
				"SessionInfo":                      sessionInfoSchema(),
				"DashboardBundle":                  dashboardBundleSchema(),
				"CostByModelRows":                  costByModelRowsSchema(),
				"CostTrendRows":                    costTrendRowsSchema(),
				"TokenTrendRows":                   tokenTrendRowsSchema(),
				"SessionPage":                      sessionPageSchema(),
				"SessionDetailRow":                 sessionDetailRowSchema(),
				"SessionDetail":                    sessionDetailRowsSchema(),
				"SessionReplayPoint":               sessionReplayPointSchema(),
				"SessionReplay":                    sessionReplaySchema(),
				"FleetAttributionReport":           looseObjectSchema("Heuristic sub-agent, parent/child, and parallel-run attribution report."),
				"IngestionPathStatus":              ingestionPathStatusSchema(),
				"IngestionHealth":                  ingestionHealthSchema(),
				"IngestionHealthRows":              ingestionHealthRowsSchema(),
				"OperationResult":                  looseObjectSchema("Local operation acknowledgement."),
				"ProjectionRepairResult":           projectionRepairResultSchema(),
				"PricingSourceStatus":              pricingSourceStatusSchema(),
				"PricingRuleSummary":               pricingRuleSummarySchema(),
				"PricingStatus":                    pricingStatusSchema(),
				"PricingAuditRow":                  pricingAuditRowSchema(),
				"PricingAuditRows":                 pricingAuditRowsSchema(),
				"BudgetStatus":                     budgetStatusSchema(),
				"BudgetStatusResponse":             budgetStatusResponseSchema(),
				"QuotaWindow":                      quotaWindowSchema(),
				"QuotaStatus":                      quotaStatusSchema(),
				"UnpricedModel":                    unpricedModelSchema(),
				"QualitySource":                    qualitySourceSchema(),
				"ProvenanceQuality":                provenanceQualitySchema(),
				"ProjectionQuality":                projectionQualitySchema(),
				"DataQualityReport":                dataQualityReportSchema(),
				"DoctorCheck":                      doctorCheckSchema(),
				"ControlIdempotencyOperationStats": controlIdempotencyOperationStatsSchema(),
				"ControlIdempotencyStats":          controlIdempotencyStatsSchema(),
				"WorkloadLeaseStats":               workloadLeaseStatsSchema(),
				"DoctorReport":                     doctorReportSchema(),
				"ModelCallRow":                     modelCallRowSchema(),
				"ModelCallRows":                    modelCallRowsSchema(),
				"ModelRegistryRow":                 modelRegistryRowSchema(),
				"ModelRegistryRows":                modelRegistryRowsSchema(),
				"CostInsightRow":                   costInsightRowSchema(),
				"CostIntelligenceRows":             costIntelligenceRowsSchema(),
				"CacheDoctorRow":                   cacheDoctorRowSchema(),
				"CacheDoctorRows":                  cacheDoctorRowsSchema(),
				"InsightEvent":                     insightEventSchema(),
				"InsightEventRows":                 insightEventRowsSchema(),
				"WebhookNotificationResult":        looseObjectSchema("Redacted webhook delivery or dry-run result."),
				"AuditEvent":                       auditEventSchema(),
				"AuditLogRows":                     auditLogRowsSchema(),
				"ReconciliationImport":             reconciliationImportSchema(),
				"ReconciliationRows":               reconciliationRowsSchema(),
				"ReconciliationImportRequest": looseObjectSchema(
					"Provider CSV/JSON statement or manual reconciliation summary. Payload hashes are persisted; secrets and prompt content are not accepted.",
				),
				"ReconciliationImportResponse": reconciliationImportResponseSchema(),
				"RouterSimulationRow":          routerSimulationRowSchema(),
				"RouterSimulationSummary":      routerSimulationSummarySchema(),
				"RouterSimulationReport":       routerSimulationReportSchema(),
				"PreflightEstimateValues":      preflightEstimateValuesSchema(),
				"PreflightEstimateReport":      preflightEstimateReportSchema(),
				"ChargebackRow":                chargebackRowSchema(),
				"ChargebackRows":               chargebackRowsSchema(),
				"WrappedProject":               wrappedProjectSchema(),
				"WrappedDay":                   wrappedDaySchema(),
				"WrappedHighlight":             wrappedHighlightSchema(),
				"AgentWrappedReport":           agentWrappedReportSchema(),
				"WorkloadSummary":              workloadSummarySchema(),
				"EvidenceDashboard":            evidenceDashboardSchema(),
				"EvidenceBundle":               evidenceBundleSchema(),
				"OfflineBundleData":            offlineBundleDataSchema(),
				"OfflineBundleIntegrity":       offlineBundleIntegritySchema(),
				"OfflineBundle":                offlineBundleSchema(),
				"OfflineBundleImportRequest":   looseObjectSchema("Offline bundle JSON. Optional signature verification uses local environment key material only."),
				"OfflineBundleImportResult":    offlineBundleImportResultSchema(),
				"OfflineBundleImportResponse":  offlineBundleImportResponseSchema(),
				"PolicyStatus":                 looseObjectSchema("Policy configuration and enforcement posture summary."),
				"PolicyEvaluationRequest":      looseObjectSchema("Policy evaluation request for local advisory rules."),
				"PolicyEvaluationResponse":     looseObjectSchema("Policy evaluation decision result."),
				"PolicyAuditReport":            looseObjectSchema("Policy audit findings over usage, tool, and workload candidates."),
				"PolicyEnforcementReport":      looseObjectSchema("Policy enforcement evidence and local approval state."),
				"PolicyDecisionRows":           looseObjectSchema("Recorded policy decision rows."),
				"PolicyApprovalRows":           looseObjectSchema("Policy approval request rows."),
				"PolicyApprovalVoteRequest":    looseObjectSchema("Approval vote payload with request_id, status, note, voter, and required approvals."),
				"PolicyApprovalVoteResponse":   looseObjectSchema("Approval vote result."),
				"ApprovalRouteSummary":         looseObjectSchema("Pending approval route summary for operators and notifications."),
				"OTelGenAIRequest":             looseObjectSchema("OpenTelemetry GenAI JSON span export or span array. Prompt and completion message attributes are ignored by conversion."),
				"OTLPTraceRequest":             looseObjectSchema("OTLP HTTP JSON/protobuf trace batch. Protobuf requests use application/x-protobuf or application/protobuf."),
				"A2ATaskRequest":               looseObjectSchema("A2A task snapshot/event payload. Message history, message parts, and artifact parts are excluded from persistence."),
				"ProviderUsageRequest":         looseObjectSchema("OpenAI-compatible, Anthropic-style, or LiteLLM-style usage envelope without request/response message content."),
				"EcosystemIngestResponse":      looseObjectSchema("Metadata-only ingest result with accepted row counts and canonical event projection results."),
				"GatewayRequest":               looseObjectSchema("Provider-compatible request body. Prompt content is proxied in memory only and not persisted by Agent Ledger."),
				"GatewayResponse":              looseObjectSchema("Provider-compatible upstream response or SSE stream with Agent Ledger metering headers."),
				"Error": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{"error": stringSchema()},
				},
				"OpenAPI": looseObjectSchema("OpenAPI 3.1 document."),
			},
		},
	}
	addOperationIDs(spec)
	addAuthResponsesAndSecurity(spec)
	addMethodNotAllowedResponses(spec)
	return spec
}

func OpenAPIFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	return hashJSONPayload(OpenAPISpecFor(opts, runtime))
}

func addOperationIDs(spec map[string]interface{}) {
	paths, ok := spec["paths"].(map[string]interface{})
	if !ok {
		return
	}
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok {
				continue
			}
			if _, exists := operation["operationId"]; exists {
				continue
			}
			operation["operationId"] = openAPIOperationID(method, path)
		}
	}
}

func openAPIOperationID(method, path string) string {
	var b strings.Builder
	b.WriteString(method)
	lastWasUnderscore := false
	for _, ch := range path {
		switch {
		case ch >= 'a' && ch <= 'z':
			b.WriteRune(ch)
			lastWasUnderscore = false
		case ch >= 'A' && ch <= 'Z':
			b.WriteRune(ch + ('a' - 'A'))
			lastWasUnderscore = false
		case ch >= '0' && ch <= '9':
			b.WriteRune(ch)
			lastWasUnderscore = false
		default:
			if b.Len() > 0 && !lastWasUnderscore {
				b.WriteByte('_')
				lastWasUnderscore = true
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return method + "_operation"
	}
	return out
}

func addMethodNotAllowedResponses(spec map[string]interface{}) {
	paths, ok := spec["paths"].(map[string]interface{})
	if !ok {
		return
	}
	methodOrder := []struct {
		key   string
		label string
	}{
		{"get", "GET"},
		{"post", "POST"},
		{"put", "PUT"},
		{"patch", "PATCH"},
		{"delete", "DELETE"},
	}
	for _, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		allowed := []string{}
		for _, method := range methodOrder {
			if _, ok := pathItem[method.key]; ok {
				allowed = append(allowed, method.label)
			}
		}
		if len(allowed) == 0 {
			continue
		}
		allowValue := joinOpenAPIMethods(allowed)
		for _, method := range methodOrder {
			operation, ok := pathItem[method.key].(map[string]interface{})
			if !ok {
				continue
			}
			responses, ok := operation["responses"].(map[string]interface{})
			if !ok {
				continue
			}
			if _, exists := responses["405"]; !exists {
				responses["405"] = methodNotAllowedResponse(allowValue)
			}
		}
	}
}

func addAuthResponsesAndSecurity(spec map[string]interface{}) {
	paths, ok := spec["paths"].(map[string]interface{})
	if !ok {
		return
	}
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			continue
		}
		for _, method := range []string{"get", "post", "put", "patch", "delete"} {
			operation, ok := pathItem[method].(map[string]interface{})
			if !ok {
				continue
			}
			operation["security"] = []map[string][]string{{"AgentLedgerBearer": {}}}
			meta, ok := operation["x-agent-ledger"].(map[string]interface{})
			if !ok {
				meta = map[string]interface{}{}
				operation["x-agent-ledger"] = meta
			}
			meta["auth"] = "localhost or bearer token when configured; prompt content and secrets are never part of auth decisions"
			addOperationAdmissionMetadata(path, method, meta)
			responses, ok := operation["responses"].(map[string]interface{})
			if !ok {
				continue
			}
			if _, exists := responses["401"]; !exists {
				responses["401"] = jsonResponse("Error")
			}
		}
	}
}

func addOperationAdmissionMetadata(path, method string, meta map[string]interface{}) {
	role, writeMode, availableInReadOnly := operationAdmissionMetadata(path, method)
	meta["required_role"] = role
	meta["write_mode"] = writeMode
	meta["available_in_read_only"] = availableInReadOnly
}

func operationAdmissionMetadata(path, method string) (requiredRole, writeMode string, availableInReadOnly bool) {
	if method == "get" {
		return "viewer", "none", true
	}
	if method != "post" {
		return "viewer", "unknown", false
	}
	switch path {
	case "/api/events/validate", "/api/integrations/conformance":
		return "viewer", "none", true
	case "/api/policy/evaluate":
		return "operator", "conditional", true
	case "/api/notifications/webhook":
		return "operator", "conditional", false
	case "/api/pricing/sync", "/api/pricing/recalculate", "/api/recalculate-costs", "/api/projections/repair":
		return "admin", "always", false
	case "/api/policy/approvals":
		return "admin", "always", false
	default:
		return "operator", "always", false
	}
}

func methodNotAllowedResponse(allowValue string) map[string]interface{} {
	return map[string]interface{}{
		"description": "Method not allowed for this endpoint.",
		"headers": map[string]interface{}{
			"Allow": map[string]interface{}{
				"description": "Comma-separated HTTP methods allowed by this path.",
				"schema":      constSchema(allowValue),
			},
		},
	}
}

func joinOpenAPIMethods(methods []string) string {
	out := ""
	for i, method := range methods {
		if i > 0 {
			out += ", "
		}
		out += method
	}
	return out
}

// OpenAPIContractPaths returns the stable REST paths that the control-plane
// OpenAPI contract must expose. Keep this hard-coded so tests catch generator
// drift instead of deriving the list from the generated document itself.
func OpenAPIContractPaths() []string {
	return []string{
		"/.well-known/agent-ledger.json",
		"/api/discovery",
		"/api/stats",
		"/api/dashboard",
		"/api/cost-by-model",
		"/api/cost-over-time",
		"/api/tokens-over-time",
		"/api/sessions",
		"/api/session-detail",
		"/api/session-replay",
		"/api/workloads",
		"/api/workloads/close",
		"/api/workloads/link",
		"/api/workloads/claim-next",
		"/api/workloads/queue",
		"/api/workloads/lease",
		"/api/workloads/lease/renew",
		"/api/workloads/lease/release",
		"/api/workloads/leases",
		"/api/agent-runs",
		"/api/agent-runs/heartbeat",
		"/api/agent-runs/liveness",
		"/api/workload-detail",
		"/api/workload-graph",
		"/api/workload-timeline",
		"/api/workload-state",
		"/api/workload-events",
		"/api/workload-events/stream",
		"/api/fleet-attribution",
		"/api/integrations",
		"/api/contracts",
		"/api/contracts/verify",
		"/api/openapi.json",
		"/api/integrations/adapter-spec",
		"/api/integrations/conformance",
		"/api/runtime/status",
		"/api/config/status",
		"/api/readiness",
		"/api/admission/check",
		"/api/event-schema",
		"/api/event-examples",
		"/api/events/validate",
		"/api/events",
		"/api/otel/genai",
		"/api/otlp/v1/traces",
		"/v1/traces",
		"/api/a2a/tasks",
		"/api/provider/calls",
		"/gateway/openai/v1/chat/completions",
		"/gateway/openai/v1/responses",
		"/gateway/anthropic/v1/messages",
		"/api/health/ingestion",
		"/api/scan",
		"/api/recalculate-costs",
		"/api/projections/repair",
		"/api/pricing/status",
		"/api/pricing/sync",
		"/api/pricing/recalculate",
		"/api/pricing/audit",
		"/api/budgets/status",
		"/api/quota/status",
		"/api/data-quality",
		"/api/doctor",
		"/api/model-calls",
		"/api/model-registry",
		"/api/cost-intelligence",
		"/api/cache/doctor",
		"/api/anomalies",
		"/api/watchdog/events",
		"/api/notifications/webhook",
		"/api/audit-log",
		"/api/reconciliation/status",
		"/api/reconciliation/import",
		"/api/router/simulate",
		"/api/preflight/estimate",
		"/api/chargeback",
		"/api/wrapped",
		"/api/badge/repo.svg",
		"/api/evidence-bundle",
		"/api/offline-bundle/export",
		"/api/offline-bundle/import",
		"/api/policies/status",
		"/api/policy/evaluate",
		"/api/policy/audit",
		"/api/policy/enforcement",
		"/api/policy/decisions",
		"/api/policy/approvals",
		"/api/policy/approval-routes",
		"/api/export",
		"/api/report",
	}
}

func getOperation(tag, summary, description, schema string) map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{tag},
			"summary":     summary,
			"description": description,
			"responses": map[string]interface{}{
				"200": jsonResponse(schema),
				"304": map[string]interface{}{"description": "Not modified when If-None-Match matches the current ETag."},
			},
		},
	}
}

func eventExamplesOperation() map[string]interface{} {
	op := getOperation("canonical-events", "Get canonical event examples", "Privacy-safe canonical event examples.", "CanonicalEventSchema")
	op["get"].(map[string]interface{})["parameters"] = []map[string]interface{}{
		queryParam("type", "Filter examples by event type."),
		queryParam("event_type", "Alias for type."),
	}
	return op
}

func filteredReadOperation(tag, summary, description, schema string, params []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{tag},
			"summary":     summary,
			"description": description,
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": false,
				"read_only_safe":     true,
				"prompt_content":     false,
			},
			"parameters": params,
			"responses": map[string]interface{}{
				"200": jsonResponse(schema),
				"304": map[string]interface{}{"description": "Not modified when If-None-Match matches the stable response ETag."},
				"400": jsonResponse("Error"),
				"403": jsonResponse("Error"),
			},
		},
	}
}

func volatileReadOperation(tag, summary, description, schema string, params []map[string]interface{}) map[string]interface{} {
	op := filteredReadOperation(tag, summary, description, schema, params)
	delete(op["get"].(map[string]interface{})["responses"].(map[string]interface{}), "304")
	setOperationETagPolicy(op["get"].(map[string]interface{}), "not emitted because response includes time-sensitive local estimates")
	return op
}

func derivedReadOperation(tag, summary, description, schema string, params []map[string]interface{}) map[string]interface{} {
	op := filteredReadOperation(tag, summary, description, schema, params)
	meta := op["get"].(map[string]interface{})["x-agent-ledger"].(map[string]interface{})
	meta["writes_local_state"] = "control-plane mode may upsert derived rows; observer/read-only mode does not mutate local state"
	meta["read_only_safe"] = "true in observer/read-only mode"
	return op
}

func localQueryWriteOperation(tag, summary, description, responseSchema string, params []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"post": map[string]interface{}{
			"tags":        []string{tag},
			"summary":     summary,
			"description": description,
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": true,
				"read_only_safe":     false,
				"prompt_content":     false,
				"local_or_auth":      true,
			},
			"parameters": params,
			"responses": map[string]interface{}{
				"200": jsonResponse(responseSchema),
				"400": jsonResponse("Error"),
				"403": jsonResponse("Error"),
				"503": jsonResponse("Error"),
			},
		},
	}
}

func flexibleWriteOperation(tag, summary, description, requestSchema, responseSchema string, params []map[string]interface{}, contentTypes []string) map[string]interface{} {
	content := map[string]interface{}{}
	for _, contentType := range contentTypes {
		schema := interface{}(refSchema(requestSchema))
		if contentType == "text/csv" || contentType == "text/plain" || contentType == "application/x-ndjson" {
			schema = stringSchema()
		}
		content[contentType] = map[string]interface{}{"schema": schema}
	}
	maxBodyBytes := defaultIngestBodyLimitBytes
	if requestSchema == "OfflineBundleImportRequest" {
		maxBodyBytes = defaultBundleBodyLimitBytes
	}
	return map[string]interface{}{
		"post": map[string]interface{}{
			"tags":        []string{tag},
			"summary":     summary,
			"description": description,
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": true,
				"read_only_safe":     false,
				"prompt_content":     false,
				"local_or_auth":      true,
				"max_body_bytes":     maxBodyBytes,
			},
			"parameters": params,
			"requestBody": map[string]interface{}{
				"required": true,
				"content":  content,
			},
			"responses": map[string]interface{}{
				"200": jsonResponse(responseSchema),
				"400": jsonResponse("Error"),
				"403": jsonResponse("Error"),
				"413": jsonResponse("Error"),
			},
		},
	}
}

func doctorOperation() map[string]interface{} {
	op := filteredReadOperation("diagnostics", "Run local doctor", "Read one-click diagnostics for usage, ingestion, pricing, data quality, projection drift, idempotency, leases, workload state, and runtime posture.", "DoctorReport", append(scopedTimeParams(), queryParam("format", "json or markdown.")))
	get := op["get"].(map[string]interface{})
	get["responses"] = map[string]interface{}{
		"200": map[string]interface{}{
			"description": "JSON diagnostic report or Markdown diagnostic report.",
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{"schema": refSchema("DoctorReport")},
				"text/markdown":    map[string]interface{}{"schema": stringSchema()},
			},
		},
		"304": map[string]interface{}{"description": "Not modified when JSON If-None-Match matches the stable diagnostic ETag."},
		"400": jsonResponse("Error"),
		"403": jsonResponse("Error"),
	}
	return op
}

func wrappedOperation() map[string]interface{} {
	op := filteredReadOperation("reports", "Get Agent Wrapped", "Read private monthly/yearly/custom usage summary. Markdown output honors privacy filters.", "AgentWrappedReport", append(scopedTimeParams(), queryParam("period", "month, year, or custom."), queryParam("format", "json or markdown.")))
	get := op["get"].(map[string]interface{})
	get["responses"] = map[string]interface{}{
		"200": map[string]interface{}{
			"description": "JSON wrapped report or Markdown wrapped report.",
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{"schema": refSchema("AgentWrappedReport")},
				"text/markdown":    map[string]interface{}{"schema": stringSchema()},
			},
		},
		"304": map[string]interface{}{"description": "Not modified when JSON If-None-Match matches the stable wrapped report ETag."},
		"400": jsonResponse("Error"),
		"403": jsonResponse("Error"),
	}
	return op
}

func badgeOperation() map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"reports"},
			"summary":     "Get repo AI cost badge",
			"description": "Render a local black/white SVG badge for repo monthly cost, tokens, sessions, or cache hit rate.",
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": false,
				"read_only_safe":     true,
				"prompt_content":     false,
				"etag":               "not emitted because SVG badges are generated attachments",
			},
			"parameters": append(scopedTimeParams(), queryParam("label", "Optional badge label."), queryParam("metric", "cost, tokens, sessions, or cache.")),
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "SVG badge.",
					"content": map[string]interface{}{
						"image/svg+xml": map[string]interface{}{"schema": stringSchema()},
					},
				},
				"400": jsonResponse("Error"),
				"403": jsonResponse("Error"),
			},
		},
	}
}

func evidenceBundleOperation() map[string]interface{} {
	op := filteredReadOperation("reports", "Export incident evidence bundle", "Export a privacy-redacted evidence bundle with health, pricing audit, data quality, dashboard consistency, anomalies, watchdog, and workload state.", "EvidenceBundle", append(scopedTimeParams(), queryParam("granularity", "Dashboard evidence chart granularity.")))
	get := op["get"].(map[string]interface{})
	get["x-agent-ledger"].(map[string]interface{})["writes_local_state"] = "control-plane mode may record the exported bundle and audit event"
	setOperationETagPolicy(get, "not emitted because evidence exports may record local audit metadata")
	get["responses"].(map[string]interface{})["200"] = map[string]interface{}{
		"description": "Privacy-redacted JSON evidence bundle attachment.",
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{"schema": refSchema("EvidenceBundle")},
		},
	}
	delete(get["responses"].(map[string]interface{}), "304")
	return op
}

func offlineBundleExportOperation() map[string]interface{} {
	op := filteredReadOperation("reports", "Export offline bundle", "Export a local offline usage bundle for air-gapped aggregation. Optional signing uses AGENT_LEDGER_BUNDLE_KEY from the environment.", "OfflineBundle", append(scopedTimeParams(), boolQueryParam("signed", "Sign bundle with local environment key."), queryParam("key_id", "Optional signing key identifier."), intQueryParam("limit", "Maximum bundle rows.")))
	get := op["get"].(map[string]interface{})
	get["x-agent-ledger"].(map[string]interface{})["writes_local_state"] = "control-plane mode may record export metadata and audit event"
	setOperationETagPolicy(get, "not emitted because offline bundle exports may record local audit metadata")
	get["responses"].(map[string]interface{})["200"] = map[string]interface{}{
		"description": "Offline JSON bundle attachment.",
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{"schema": refSchema("OfflineBundle")},
		},
	}
	delete(get["responses"].(map[string]interface{}), "304")
	return op
}

func policyApprovalsOperation() map[string]interface{} {
	get := filteredReadOperation("governance", "List policy approvals", "List pending or historical local policy approval requests.", "PolicyApprovalRows", []map[string]interface{}{queryParam("status", "Approval status. Empty defaults to pending."), intQueryParam("limit", "Maximum approval rows.")})["get"].(map[string]interface{})
	post := simpleWriteOperation("governance", "Cast policy approval vote", "Approve or reject one local policy approval request. Writes audit metadata without prompt content.", "PolicyApprovalVoteRequest", "PolicyApprovalVoteResponse")
	return map[string]interface{}{
		"get":  get,
		"post": post,
	}
}

func policyEvaluateOperation() map[string]interface{} {
	op := flexibleWriteOperation("governance", "Evaluate policy", "Evaluate local advisory policy rules for an operation. Optional record=true or workload_id writes local policy decision metadata.", "PolicyEvaluationRequest", "PolicyEvaluationResponse", []map[string]interface{}{}, []string{"application/json"})
	meta := op["post"].(map[string]interface{})["x-agent-ledger"].(map[string]interface{})
	meta["writes_local_state"] = "conditional: record=true or workload_id writes local policy decision metadata"
	meta["read_only_safe"] = "true when record=false and no workload_id is supplied"
	return op
}

func exportOperation() map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"reports"},
			"summary":     "Export ledger data",
			"description": "Export sessions, workloads, daily totals, model costs, model calls, chargeback, audit, or data quality rows as CSV or JSON. Policy may require privacy=1.",
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": "control-plane mode records export audit metadata",
				"read_only_safe":     true,
				"prompt_content":     false,
				"etag":               "not emitted because export responses are generated attachments and may record audit metadata",
			},
			"parameters": append(scopedTimeParams(), queryParam("type", "sessions, workloads, daily, models, model-calls, chargeback, audit, or quality."), queryParam("format", "csv or json.")),
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "CSV or JSON export attachment.",
					"content": map[string]interface{}{
						"application/json": map[string]interface{}{"schema": looseObjectSchema("JSON export payload.")},
						"text/csv":         map[string]interface{}{"schema": stringSchema()},
					},
				},
				"400": jsonResponse("Error"),
				"403": jsonResponse("Error"),
			},
		},
	}
}

func reportOperation() map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"reports"},
			"summary":     "Generate Markdown report",
			"description": "Generate a local Markdown cost, token, session, model, and budget report for the selected scope.",
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": "control-plane mode records report audit metadata",
				"read_only_safe":     true,
				"prompt_content":     false,
				"etag":               "not emitted because report responses are generated attachments and may record audit metadata",
			},
			"parameters": scopedTimeParams(),
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Markdown report.",
					"content": map[string]interface{}{
						"text/markdown": map[string]interface{}{"schema": stringSchema()},
					},
				},
				"400": jsonResponse("Error"),
				"403": jsonResponse("Error"),
			},
		},
	}
}

func canonicalEventPostOperation(tag, summary, description string, writes bool) map[string]interface{} {
	op := map[string]interface{}{
		"post": map[string]interface{}{
			"tags":        []string{tag},
			"summary":     summary,
			"description": description,
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": writes,
				"read_only_safe":     !writes,
				"max_events":         500,
				"max_body_bytes":     defaultIngestBodyLimitBytes,
			},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{"schema": refSchema("CanonicalEventRequest")},
				},
			},
			"responses": map[string]interface{}{
				"200": jsonResponse(map[bool]string{true: "IngestResponse", false: "ValidationResponse"}[writes]),
				"400": jsonResponse("Error"),
				"413": jsonResponse("Error"),
			},
		},
	}
	return op
}

func adapterConformanceOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": map[string]interface{}{
			"tags":        []string{"adapter-conformance"},
			"summary":     "Validate adapter fixture",
			"description": "Validate canonical, provider, provider-stream, OpenTelemetry GenAI, or A2A adapter fixture without writing SQLite.",
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": false,
				"read_only_safe":     true,
				"max_body_bytes":     defaultIngestBodyLimitBytes,
			},
			"parameters": []map[string]interface{}{
				queryParam("kind", "auto, canonical, provider, provider-stream, otel, or a2a."),
				boolQueryParam("strict", "Treat provenance warnings as validation failures."),
			},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json":     map[string]interface{}{"schema": looseObjectSchema("Adapter fixture JSON.")},
					"text/event-stream":    map[string]interface{}{"schema": stringSchema()},
					"application/x-ndjson": map[string]interface{}{"schema": stringSchema()},
				},
			},
			"responses": map[string]interface{}{
				"200": jsonResponse("ValidationResponse"),
				"400": jsonResponse("Error"),
				"413": jsonResponse("Error"),
			},
		},
	}
}

func workloadsOperation() map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"workload-control"},
			"summary":     "List workloads",
			"description": "Server-side paginated workload ledger for local dashboards, wrappers, and routers.",
			"parameters": []map[string]interface{}{
				queryParam("from", "YYYY-MM-DD lower bound."),
				queryParam("to", "YYYY-MM-DD upper bound."),
				queryParam("source", "Optional source filter."),
				queryParam("model", "Optional model filter."),
				queryParam("project", "Optional project filter."),
				queryParam("status", "Optional workload status filter."),
				queryParam("q", "Optional text filter."),
				intQueryParam("limit", "Maximum rows."),
				intQueryParam("offset", "Offset for pagination."),
				queryParam("cursor", "Cursor alias for offset."),
			},
			"responses": map[string]interface{}{
				"200": jsonResponse(looseObjectSchema("Paginated workload rows.")),
				"304": map[string]interface{}{"description": "Not modified when If-None-Match matches the current workload page ETag."},
				"400": jsonResponse("Error"),
			},
		},
		"post": idempotentWriteOperation("workload-control", "Create workload", "Create one local workload. Retries with the same normalized request and idempotency key return the original workload id.", "WorkloadCreateRequest", "WorkloadCreateResponse"),
	}
}

func workloadCloseOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": simpleWriteOperation("workload-control", "Close workload", "Mark one workload as terminal with a status and optional outcome. The operation writes local metadata and audit rows only.", "WorkloadCloseRequest", "WorkloadCloseResponse"),
	}
}

func workloadLinkOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": simpleWriteOperation("workload-control", "Link workloads", "Create a local metadata-only dependency or lineage edge between two workloads.", "WorkloadLinkRequest", "WorkloadLinkResponse"),
	}
}

func workloadLeaseAcquireOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": workloadLeaseWriteOperation(
			"Acquire workload lease",
			"Acquire a short-lived local execution lease before an async router, wrapper, or agent starts work on a workload. The lease token is returned once and is never stored in plaintext.",
			"WorkloadLeaseAcquireRequest",
			"WorkloadLeaseResponse",
		),
	}
}

func workloadClaimNextOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": workloadLeaseWriteOperation(
			"Claim next workload",
			"Atomically select the next queue-eligible local workload and create a short-lived execution lease in the same SQLite transaction. Empty queues return ok=true and empty=true without a lease.",
			"WorkloadClaimNextRequest",
			"WorkloadClaimNextResponse",
		),
	}
}

func workloadQueueOperation() map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"workload-control"},
			"summary":     "Get workload queue stats",
			"description": "Return read-only queue claimability and lease pressure stats for local routers and operators. This endpoint does not mutate expired lease rows.",
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": false,
				"read_only_safe":     true,
				"prompt_content":     false,
				"lease_tokens":       "not_returned",
			},
			"parameters": []map[string]interface{}{
				queryParam("source", "Optional source filter."),
				queryParam("project", "Optional project filter."),
				queryParam("repo", "Optional repo filter."),
				queryParam("team", "Optional team filter."),
				queryParam("owner", "Optional owner filter."),
				queryParam("status", "Claim status set. Empty defaults to queued,active; use any for non-terminal statuses."),
				queryParam("q", "Optional text filter."),
			},
			"responses": map[string]interface{}{
				"200": jsonResponse("WorkloadQueueStats"),
				"304": map[string]interface{}{"description": "Not modified when If-None-Match matches the stable queue ETag."},
				"400": jsonResponse("Error"),
			},
		},
	}
}

func workloadLeaseRenewOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": workloadLeaseWriteOperation(
			"Renew workload lease",
			"Renew an active workload lease using the lease token. Renewal fails explicitly when the token does not match or the lease is no longer active.",
			"WorkloadLeaseRenewRequest",
			"WorkloadLeaseResponse",
		),
	}
}

func workloadLeaseReleaseOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": workloadLeaseWriteOperation(
			"Release workload lease",
			"Release a workload lease using the lease token. Release is local-only and writes an audit event without storing the token.",
			"WorkloadLeaseReleaseRequest",
			"WorkloadLeaseResponse",
		),
	}
}

func workloadLeasesOperation() map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"workload-control"},
			"summary":     "List workload leases",
			"description": "List active workload leases for local routers and operators. Lease tokens are never returned by this list endpoint.",
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": false,
				"read_only_safe":     true,
				"prompt_content":     false,
				"lease_tokens":       "not_returned",
			},
			"parameters": []map[string]interface{}{
				boolQueryParam("include_inactive", "Include expired and released leases."),
				intQueryParam("limit", "Maximum rows."),
			},
			"responses": map[string]interface{}{
				"200": jsonResponse("WorkloadLeaseListResponse"),
				"304": map[string]interface{}{"description": "Not modified when If-None-Match matches the current lease list ETag."},
				"400": jsonResponse("Error"),
			},
		},
	}
}

func agentRunsOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": idempotentWriteOperation("workload-control", "Start agent run", "Start a run attached to an existing workload. Retries with the same normalized request and idempotency key return the original run id.", "AgentRunStartRequest", "AgentRunStartResponse"),
	}
}

func agentRunHeartbeatOperation() map[string]interface{} {
	return map[string]interface{}{
		"post": simpleWriteOperation("workload-control", "Record agent run heartbeat", "Append one metadata-only liveness/progress heartbeat to an active async agent run.", "AgentRunHeartbeatRequest", "AgentRunHeartbeatResponse"),
	}
}

func agentRunLivenessOperation() map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"workload-control"},
			"summary":     "Get agent run liveness",
			"description": "Return active async agent run liveness rows and stale heartbeat state. The endpoint honors privacy filters and emits a stable ETag that ignores age_seconds render ticks.",
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": false,
				"read_only_safe":     true,
				"prompt_content":     false,
			},
			"parameters": []map[string]interface{}{
				queryParam("max_age", "Heartbeat age threshold such as 10m."),
				queryParam("stale_only", "Return only stale active runs when true or 1."),
				queryParam("source", "Optional source filter."),
				queryParam("project", "Optional project or repo filter."),
				queryParam("limit", "Maximum rows to return."),
			},
			"responses": map[string]interface{}{
				"200": jsonResponse("AgentRunLivenessResponse"),
				"304": map[string]interface{}{"description": "Not modified when If-None-Match matches the stable liveness ETag."},
				"400": jsonResponse("Error"),
			},
		},
	}
}

func workloadDetailOperation() map[string]interface{} {
	return workloadReadOperation("Get workload detail", "Return a full workload ledger detail for one workload. Privacy filters are applied by the server.", "WorkloadDetail", []map[string]interface{}{
		queryParam("workload_id", "Required workload id."),
	})
}

func workloadGraphOperation() map[string]interface{} {
	return workloadReadOperation("Get workload graph", "Return a compact dependency and activity graph for one workload.", "WorkloadGraph", []map[string]interface{}{
		queryParam("workload_id", "Required workload id."),
	})
}

func workloadTimelineOperation() map[string]interface{} {
	return workloadReadOperation("Get workload timeline", "Return a chronological metadata-only audit timeline for one workload.", "WorkloadTimelineResponse", []map[string]interface{}{
		queryParam("workload_id", "Required workload id."),
		queryParam("limit", "Maximum timeline rows to return."),
	})
}

func workloadStateOperation() map[string]interface{} {
	return workloadReadOperation("Get workload state", "Return a derived terminal-state snapshot for one async agent workload.", "WorkloadState", []map[string]interface{}{
		queryParam("workload_id", "Required workload id."),
		queryParam("max_age", "Heartbeat stale threshold such as 10m."),
	})
}

func workloadReadOperation(summary, description, schema string, params []map[string]interface{}) map[string]interface{} {
	return map[string]interface{}{
		"get": map[string]interface{}{
			"tags":        []string{"workload-control"},
			"summary":     summary,
			"description": description,
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state": false,
				"read_only_safe":     true,
				"prompt_content":     false,
			},
			"parameters": params,
			"responses": map[string]interface{}{
				"200": jsonResponse(schema),
				"304": map[string]interface{}{"description": "Not modified when If-None-Match matches the current ETag."},
				"400": jsonResponse("Error"),
			},
		},
	}
}

func workloadLeaseWriteOperation(summary, description, requestSchema, responseSchema string) map[string]interface{} {
	return map[string]interface{}{
		"tags":        []string{"workload-control"},
		"summary":     summary,
		"description": description,
		"x-agent-ledger": map[string]interface{}{
			"writes_local_state": true,
			"read_only_safe":     false,
			"prompt_content":     false,
			"lease_tokens":       "plaintext accepted only in request body and returned only from acquire; SQLite stores sha256 hashes",
			"max_body_bytes":     defaultJSONBodyLimitBytes,
		},
		"requestBody": map[string]interface{}{
			"required": true,
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{"schema": refSchema(requestSchema)},
			},
		},
		"responses": map[string]interface{}{
			"200": jsonResponse(responseSchema),
			"400": jsonResponse("Error"),
			"409": jsonResponse("Error"),
			"413": jsonResponse("Error"),
		},
	}
}

func simpleWriteOperation(tag, summary, description, requestSchema, responseSchema string) map[string]interface{} {
	return map[string]interface{}{
		"tags":        []string{tag},
		"summary":     summary,
		"description": description,
		"x-agent-ledger": map[string]interface{}{
			"writes_local_state": true,
			"read_only_safe":     false,
			"prompt_content":     false,
			"max_body_bytes":     defaultJSONBodyLimitBytes,
		},
		"requestBody": map[string]interface{}{
			"required": true,
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{"schema": refSchema(requestSchema)},
			},
		},
		"responses": map[string]interface{}{
			"200": jsonResponse(responseSchema),
			"400": jsonResponse("Error"),
			"413": jsonResponse("Error"),
		},
	}
}

func idempotentWriteOperation(tag, summary, description, requestSchema, responseSchema string) map[string]interface{} {
	return map[string]interface{}{
		"tags":        []string{tag},
		"summary":     summary,
		"description": description,
		"x-agent-ledger": map[string]interface{}{
			"writes_local_state": true,
			"read_only_safe":     false,
			"idempotency":        "Idempotency-Key header, X-Idempotency-Key header, or idempotency_key JSON field. Same key with different input fails with 409.",
			"prompt_content":     false,
			"max_body_bytes":     defaultJSONBodyLimitBytes,
		},
		"parameters": []map[string]interface{}{
			headerParam("Idempotency-Key", "Stable retry key for this write operation."),
			headerParam("X-Idempotency-Key", "Alternative stable retry key."),
		},
		"requestBody": map[string]interface{}{
			"required": true,
			"content": map[string]interface{}{
				"application/json": map[string]interface{}{"schema": refSchema(requestSchema)},
			},
		},
		"responses": map[string]interface{}{
			"200": jsonResponse(responseSchema),
			"400": jsonResponse("Error"),
			"409": jsonResponse("Error"),
			"413": jsonResponse("Error"),
		},
	}
}

func workloadEventsOperation(stream bool) map[string]interface{} {
	method := map[string]interface{}{
		"tags":        []string{"workload-feed"},
		"summary":     "Get workload event feed",
		"description": "Cursor-stable metadata-only workload state feed for local monitors and agent routers.",
		"parameters": []map[string]interface{}{
			queryParam("from", "YYYY-MM-DD lower bound."),
			queryParam("to", "YYYY-MM-DD upper bound."),
			queryParam("source", "Optional source filter."),
			queryParam("model", "Optional model filter."),
			queryParam("project", "Optional project filter."),
			queryParam("phase", "Optional workload phase filter."),
			queryParam("severity", "Optional severity filter."),
			queryParam("cursor", "Previously returned feed cursor."),
			queryParam("stale_after", "Duration such as 10m."),
			intQueryParam("limit", "Maximum rows."),
		},
		"responses": map[string]interface{}{
			"200": jsonResponse("WorkloadEventFeed"),
			"304": map[string]interface{}{"description": "Not modified when cursor or If-None-Match matches."},
			"400": jsonResponse("Error"),
		},
	}
	if stream {
		method["summary"] = "Stream workload event feed"
		setOperationETagPolicy(method, "not emitted because SSE streams are long-lived event feeds")
		method["responses"] = map[string]interface{}{
			"200": map[string]interface{}{
				"description": "SSE stream that emits workload_events messages with the feed cursor as SSE id.",
				"content": map[string]interface{}{
					"text/event-stream": map[string]interface{}{"schema": stringSchema()},
				},
			},
			"400": jsonResponse("Error"),
		}
	}
	return map[string]interface{}{"get": method}
}

func setOperationETagPolicy(operation map[string]interface{}, policy string) {
	meta, ok := operation["x-agent-ledger"].(map[string]interface{})
	if !ok {
		meta = map[string]interface{}{}
		operation["x-agent-ledger"] = meta
	}
	meta["etag"] = policy
}

func ecosystemIngestOperation(summary, description, requestSchema, responseSchema string, disabledByDefault bool) map[string]interface{} {
	meta := map[string]interface{}{
		"writes_local_state": true,
		"read_only_safe":     false,
		"prompt_content":     false,
		"max_body_bytes":     defaultIngestBodyLimitBytes,
	}
	if disabledByDefault {
		meta["disabled_by_default"] = true
	}
	return map[string]interface{}{
		"post": map[string]interface{}{
			"tags":           []string{"ecosystem-ingest"},
			"summary":        summary,
			"description":    description,
			"x-agent-ledger": meta,
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{"schema": refSchema(requestSchema)},
				},
			},
			"responses": map[string]interface{}{
				"200": jsonResponse(responseSchema),
				"400": jsonResponse("Error"),
				"403": jsonResponse("Error"),
				"413": jsonResponse("Error"),
			},
		},
	}
}

func otlpTracesOperation() map[string]interface{} {
	op := ecosystemIngestOperation(
		"Ingest OTLP traces",
		"Optional local OTLP HTTP JSON/protobuf trace receiver. Disabled by default and restricted to localhost or authenticated operators.",
		"OTLPTraceRequest",
		"EcosystemIngestResponse",
		true,
	)
	post := op["post"].(map[string]interface{})
	post["requestBody"] = map[string]interface{}{
		"required": true,
		"content": map[string]interface{}{
			"application/json":       map[string]interface{}{"schema": refSchema("OTLPTraceRequest")},
			"application/x-protobuf": map[string]interface{}{"schema": stringSchema()},
			"application/protobuf":   map[string]interface{}{"schema": stringSchema()},
		},
	}
	post["responses"].(map[string]interface{})["404"] = jsonResponse("Error")
	post["responses"].(map[string]interface{})["413"] = jsonResponse("Error")
	return op
}

func gatewayOperation(summary, description string) map[string]interface{} {
	return map[string]interface{}{
		"post": map[string]interface{}{
			"tags":        []string{"provider-gateway"},
			"summary":     summary,
			"description": description,
			"x-agent-ledger": map[string]interface{}{
				"writes_local_state":                 true,
				"read_only_safe":                     false,
				"prompt_content":                     false,
				"prompt_content_forwarded_in_memory": true,
				"disabled_by_default":                true,
				"upstream_api_keys":                  "read from environment variables; not persisted",
				"usage_metadata_persisted":           true,
				"response_content_persisted":         false,
				"max_body_bytes":                     defaultIngestBodyLimitBytes,
			},
			"requestBody": map[string]interface{}{
				"required": true,
				"content": map[string]interface{}{
					"application/json": map[string]interface{}{"schema": refSchema("GatewayRequest")},
				},
			},
			"responses": map[string]interface{}{
				"200": map[string]interface{}{
					"description": "Provider-compatible JSON response or SSE stream.",
					"content": map[string]interface{}{
						"application/json":  map[string]interface{}{"schema": refSchema("GatewayResponse")},
						"text/event-stream": map[string]interface{}{"schema": stringSchema()},
					},
				},
				"400": jsonResponse("Error"),
				"403": jsonResponse("Error"),
				"404": jsonResponse("Error"),
				"413": jsonResponse("Error"),
				"415": jsonResponse("Error"),
			},
		},
	}
}

func jsonResponse(schema interface{}) map[string]interface{} {
	ref := schema
	if name, ok := schema.(string); ok {
		ref = refSchema(name)
	}
	return map[string]interface{}{
		"description": "JSON response.",
		"content": map[string]interface{}{
			"application/json": map[string]interface{}{"schema": ref},
		},
	}
}

func queryParam(name, description string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"in":          "query",
		"description": description,
		"required":    false,
		"schema":      stringSchema(),
	}
}

func headerParam(name, description string) map[string]interface{} {
	return map[string]interface{}{
		"name":        name,
		"in":          "header",
		"description": description,
		"required":    false,
		"schema":      stringSchema(),
	}
}

func boolQueryParam(name, description string) map[string]interface{} {
	param := queryParam(name, description)
	param["schema"] = boolSchema()
	return param
}

func intQueryParam(name, description string) map[string]interface{} {
	param := queryParam(name, description)
	param["schema"] = map[string]interface{}{"type": "integer", "minimum": 1}
	return param
}

func scopedTimeParams() []map[string]interface{} {
	params := []map[string]interface{}{
		queryParam("from", "YYYY-MM-DD lower bound. Time filters use half-open [from,to) semantics."),
		queryParam("to", "YYYY-MM-DD upper bound. Time filters use half-open [from,to) semantics."),
		queryParam("source", "Optional source filter."),
		queryParam("model", "Optional model filter."),
		queryParam("project", "Optional project/workspace filter."),
		queryParam("privacy", "Set to 1 or true to apply privacy filters where supported."),
	}
	return params
}

func privacyParams() []map[string]interface{} {
	return []map[string]interface{}{
		queryParam("privacy", "Set to 1 or true to hash session ids and hide sensitive local metadata where supported."),
	}
}

func paginationAndSortParams() []map[string]interface{} {
	return []map[string]interface{}{
		queryParam("q", "Optional text search."),
		queryParam("sort", "Server-side sort key."),
		queryParam("dir", "Sort direction asc or desc."),
		intQueryParam("limit", "Maximum rows."),
		intQueryParam("offset", "Offset for pagination."),
		queryParam("cursor", "Cursor alias for offset."),
	}
}

func refSchema(name string) map[string]interface{} {
	return map[string]interface{}{"$ref": "#/components/schemas/" + name}
}

func constSchema(value string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "const": value}
}

func stringSchema() map[string]interface{} {
	return map[string]interface{}{"type": "string"}
}

func boolSchema() map[string]interface{} {
	return map[string]interface{}{"type": "boolean"}
}

func numberSchema() map[string]interface{} {
	return map[string]interface{}{"type": "number"}
}

func integerSchema() map[string]interface{} {
	return map[string]interface{}{"type": "integer"}
}

func stringArraySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":  "array",
		"items": stringSchema(),
	}
}

func dashboardStatsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Aggregate usage, token, cost, prompt, session, call, and cache totals for one scoped time window.",
		"additionalProperties": true,
		"required":             []string{"total_cost", "total_tokens", "total_sessions", "total_prompts", "total_calls", "cache_hit_rate"},
		"properties": map[string]interface{}{
			"total_cost":     numberSchema(),
			"total_tokens":   integerSchema(),
			"total_sessions": integerSchema(),
			"total_prompts":  integerSchema(),
			"total_calls":    integerSchema(),
			"cache_hit_rate": numberSchema(),
		},
	}
}

func dashboardConsistencyIssueSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "A visible mismatch between dashboard modules, usually indicating stale aggregates or a rebuild requirement.",
		"additionalProperties": true,
		"required":             []string{"metric", "expected", "actual", "delta", "severity", "message"},
		"properties": map[string]interface{}{
			"metric":   stringSchema(),
			"expected": numberSchema(),
			"actual":   numberSchema(),
			"delta":    numberSchema(),
			"severity": stringSchema(),
			"message":  stringSchema(),
		},
	}
}

func costByModelSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Cost total for one model in the scoped time window.",
		"additionalProperties": true,
		"required":             []string{"model", "cost"},
		"properties": map[string]interface{}{
			"model": stringSchema(),
			"cost":  numberSchema(),
		},
	}
}

func timeSeriesPointSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "One cost trend point, optionally grouped by model.",
		"additionalProperties": true,
		"required":             []string{"date", "value"},
		"properties": map[string]interface{}{
			"date":  stringSchema(),
			"value": numberSchema(),
			"model": stringSchema(),
		},
	}
}

func tokenTimeSeriesPointSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "One token trend point using non-overlapping token components.",
		"additionalProperties": true,
		"required":             []string{"date", "input_tokens", "output_tokens", "cache_read", "cache_create"},
		"properties": map[string]interface{}{
			"date":          stringSchema(),
			"input_tokens":  integerSchema(),
			"output_tokens": integerSchema(),
			"cache_read":    integerSchema(),
			"cache_create":  integerSchema(),
		},
	}
}

func sessionInfoSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "One server-side aggregated session ledger row after privacy filters are applied.",
		"additionalProperties": true,
		"required":             []string{"session_id", "source", "project", "cwd", "git_branch", "start_time", "last_activity", "prompts", "total_cost", "tokens"},
		"properties": map[string]interface{}{
			"session_id":    stringSchema(),
			"source":        stringSchema(),
			"project":       stringSchema(),
			"cwd":           stringSchema(),
			"git_branch":    stringSchema(),
			"start_time":    stringSchema(),
			"last_activity": stringSchema(),
			"prompts":       integerSchema(),
			"total_cost":    numberSchema(),
			"tokens":        integerSchema(),
		},
	}
}

func dashboardBundleSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Full dashboard bundle with aggregate/raw data source disclosure, chart rows, consistency metadata, and optional runtime state.",
		"additionalProperties": true,
		"required": []string{
			"generated_at", "from", "to", "granularity", "data_source", "stats", "cost_by_model", "cost_over_time", "tokens_over_time", "consistency",
		},
		"properties": map[string]interface{}{
			"generated_at": stringSchema(),
			"from":         stringSchema(),
			"to":           stringSchema(),
			"granularity":  stringSchema(),
			"data_source":  stringSchema(),
			"source":       stringSchema(),
			"model":        stringSchema(),
			"project":      stringSchema(),
			"stats":        refSchema("DashboardStats"),
			"cost_by_model": map[string]interface{}{
				"type":  "array",
				"items": refSchema("CostByModel"),
			},
			"cost_over_time": map[string]interface{}{
				"type":  "array",
				"items": refSchema("TimeSeriesPoint"),
			},
			"tokens_over_time": map[string]interface{}{
				"type":  "array",
				"items": refSchema("TokenTimeSeriesPoint"),
			},
			"consistency": map[string]interface{}{
				"type":  "array",
				"items": refSchema("DashboardConsistencyIssue"),
			},
			"runtime": refSchema("RuntimeStatus"),
		},
	}
}

func costByModelRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Cost distribution rows grouped by model.",
		"items":       refSchema("CostByModel"),
	}
}

func costTrendRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Aggregate cost trend points.",
		"items":       refSchema("TimeSeriesPoint"),
	}
}

func tokenTrendRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Aggregate token trend points.",
		"items":       refSchema("TokenTimeSeriesPoint"),
	}
}

func sessionPageSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Server-side paginated session ledger page. The next_cursor is an offset-compatible cursor string.",
		"additionalProperties": true,
		"required":             []string{"rows", "total", "limit", "offset"},
		"properties": map[string]interface{}{
			"rows": map[string]interface{}{
				"type":  "array",
				"items": refSchema("SessionInfo"),
			},
			"total":       integerSchema(),
			"limit":       integerSchema(),
			"offset":      integerSchema(),
			"next_cursor": stringSchema(),
		},
	}
}

func pricingSourceStatusSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "One pricing source with explicit provenance. Embedded official seeds and local overrides are not treated as live fetches.",
		"additionalProperties": true,
		"required": []string{
			"name", "kind", "priority", "url", "last_fetch_at", "sha256", "model_count", "status", "freshness_kind", "stale",
		},
		"properties": map[string]interface{}{
			"name":           stringSchema(),
			"kind":           stringSchema(),
			"priority":       integerSchema(),
			"url":            stringSchema(),
			"last_fetch_at":  stringSchema(),
			"etag":           stringSchema(),
			"sha256":         stringSchema(),
			"model_count":    integerSchema(),
			"status":         stringSchema(),
			"last_error":     stringSchema(),
			"freshness_kind": stringSchema(),
			"freshness_note": stringSchema(),
			"stale":          boolSchema(),
		},
	}
}

func pricingStatusSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Pricing source provenance, freshness, rule summary, confidence mix, and unpriced model groups.",
		"additionalProperties": true,
		"required":             []string{"sources", "unpriced_models", "confidence_mix", "rules", "mode", "stale_after"},
		"properties": map[string]interface{}{
			"sources": map[string]interface{}{
				"type":  "array",
				"items": refSchema("PricingSourceStatus"),
			},
			"unpriced_models": map[string]interface{}{
				"type":        "array",
				"description": "Models with usage records that could not be priced.",
				"items":       looseObjectSchema("Unpriced model group with source, model, and record count."),
			},
			"confidence_mix": map[string]interface{}{
				"type":                 "object",
				"description":          "Record counts grouped by pricing confidence such as official, override, fallback, fuzzy, source-reported, or unpriced.",
				"additionalProperties": integerSchema(),
			},
			"rules": map[string]interface{}{
				"type":                 "object",
				"description":          "Effective pricing rule counts grouped by source priority and confidence.",
				"additionalProperties": true,
				"properties": map[string]interface{}{
					"total_rules":     integerSchema(),
					"override_rules":  integerSchema(),
					"official_rules":  integerSchema(),
					"fallback_rules":  integerSchema(),
					"unknown_rules":   integerSchema(),
					"oldest_updated":  stringSchema(),
					"newest_updated":  stringSchema(),
					"confidence_mix":  map[string]interface{}{"type": "object", "additionalProperties": integerSchema()},
					"pricing_sources": map[string]interface{}{"type": "object", "additionalProperties": integerSchema()},
				},
			},
			"mode":        stringSchema(),
			"stale_after": stringSchema(),
		},
	}
}

func stringIntMapSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          description,
		"additionalProperties": integerSchema(),
	}
}

func unpricedModelSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "One source/model group with usage records that could not be priced.",
		"additionalProperties": true,
		"required":             []string{"source", "model", "records"},
		"properties": map[string]interface{}{
			"source":  stringSchema(),
			"model":   stringSchema(),
			"records": integerSchema(),
		},
	}
}

func qualitySourceSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Source-level trust signals for pricing completeness, aggregate estimates, cache awareness, and confidence.",
		"additionalProperties": true,
		"required": []string{
			"source", "records", "sessions", "unpriced_records", "estimated_aggregate_records", "cache_aware_records", "confidence", "message",
		},
		"properties": map[string]interface{}{
			"source":                      stringSchema(),
			"records":                     integerSchema(),
			"sessions":                    integerSchema(),
			"unpriced_records":            integerSchema(),
			"estimated_aggregate_records": integerSchema(),
			"cache_aware_records":         integerSchema(),
			"confidence":                  numberSchema(),
			"message":                     stringSchema(),
		},
	}
}

func provenanceQualitySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Traceability diagnostics for canonical events and source adapter provenance. Raw prompt and response content are not included.",
		"additionalProperties": true,
		"required": []string{
			"events", "missing_schema_version", "missing_source_version", "missing_parser_version", "missing_raw_ref", "missing_match_type", "schema_version_mix", "match_type_mix", "confidence", "message",
		},
		"properties": map[string]interface{}{
			"events":                 integerSchema(),
			"missing_schema_version": integerSchema(),
			"missing_source_version": integerSchema(),
			"missing_parser_version": integerSchema(),
			"missing_raw_ref":        integerSchema(),
			"missing_match_type":     integerSchema(),
			"schema_version_mix":     stringIntMapSchema("Canonical event counts grouped by schema version."),
			"match_type_mix":         stringIntMapSchema("Canonical event counts grouped by raw-source match type."),
			"confidence":             numberSchema(),
			"message":                stringSchema(),
		},
	}
}

func projectionQualitySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Alignment diagnostics between canonical model calls and projected usage records.",
		"additionalProperties": true,
		"required": []string{
			"model_calls", "projected_usage_records", "missing_usage_projection", "cost_mismatch_records", "cost_delta_usd", "duplicate_session_owners", "confidence", "message",
		},
		"properties": map[string]interface{}{
			"model_calls":              integerSchema(),
			"projected_usage_records":  integerSchema(),
			"missing_usage_projection": integerSchema(),
			"cost_mismatch_records":    integerSchema(),
			"cost_delta_usd":           numberSchema(),
			"duplicate_session_owners": integerSchema(),
			"confidence":               numberSchema(),
			"message":                  stringSchema(),
		},
	}
}

func dataQualityReportSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Trust and completeness diagnostics for usage, pricing, provenance, canonical projection, source confidence, and malformed rows.",
		"additionalProperties": true,
		"required": []string{
			"generated_at", "pricing_sources", "source_quality", "unpriced_models", "confidence_mix", "provenance", "projection", "issues",
		},
		"properties": map[string]interface{}{
			"generated_at": stringSchema(),
			"pricing_sources": map[string]interface{}{
				"type":  "array",
				"items": refSchema("PricingSourceStatus"),
			},
			"source_quality": map[string]interface{}{
				"type":  "array",
				"items": refSchema("QualitySource"),
			},
			"unpriced_models": map[string]interface{}{
				"type":  "array",
				"items": refSchema("UnpricedModel"),
			},
			"confidence_mix": stringIntMapSchema("Usage record counts grouped by pricing confidence."),
			"provenance":     refSchema("ProvenanceQuality"),
			"projection":     refSchema("ProjectionQuality"),
			"issues": map[string]interface{}{
				"type":        "array",
				"description": "Anomaly, watchdog, or diagnostic issues. Privacy filters redact paths, projects, branches, and session ids where requested.",
				"items":       looseObjectSchema("Insight event row."),
			},
		},
	}
}

func costIntelligenceRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Expensive session explanations, token composition, cache behavior, pricing provenance, and low-confidence pricing counters. Prompt and response content are never included.",
		"items": map[string]interface{}{
			"type":                 "object",
			"additionalProperties": true,
			"required": []string{
				"source", "session_id", "calls", "tokens", "cost_usd", "cache_hit_rate", "pricing_sources", "pricing_confidences", "quality_score", "reasons", "advice",
			},
			"properties": map[string]interface{}{
				"source":                stringSchema(),
				"session_id":            stringSchema(),
				"project":               stringSchema(),
				"git_branch":            stringSchema(),
				"models":                integerSchema(),
				"calls":                 integerSchema(),
				"prompts":               integerSchema(),
				"input_tokens":          integerSchema(),
				"cache_read_tokens":     integerSchema(),
				"cache_write_tokens":    integerSchema(),
				"output_tokens":         integerSchema(),
				"reasoning_tokens":      integerSchema(),
				"tokens":                integerSchema(),
				"cost_usd":              numberSchema(),
				"cost_per_call":         numberSchema(),
				"cost_per_prompt":       numberSchema(),
				"tokens_per_prompt":     numberSchema(),
				"cache_hit_rate":        numberSchema(),
				"output_ratio":          numberSchema(),
				"pricing_sources":       stringArraySchema(),
				"pricing_confidences":   stringArraySchema(),
				"official_priced_calls": integerSchema(),
				"override_priced_calls": integerSchema(),
				"fallback_priced_calls": integerSchema(),
				"fuzzy_priced_calls":    integerSchema(),
				"source_reported_calls": integerSchema(),
				"unpriced_calls":        integerSchema(),
				"unknown_pricing_calls": integerSchema(),
				"quality_score":         numberSchema(),
				"reasons":               stringArraySchema(),
				"advice":                stringArraySchema(),
				"last_activity":         stringSchema(),
			},
		},
	}
}

func projectionRepairResultSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Canonical-to-usage projection repair summary with before/after quality evidence.",
		"additionalProperties": true,
		"required":             []string{"before", "after", "inserted", "updated", "from", "to", "aggregates_note"},
		"properties": map[string]interface{}{
			"before":          refSchema("ProjectionQuality"),
			"after":           refSchema("ProjectionQuality"),
			"inserted":        integerSchema(),
			"updated":         integerSchema(),
			"from":            stringSchema(),
			"to":              stringSchema(),
			"source":          stringSchema(),
			"model":           stringSchema(),
			"project":         stringSchema(),
			"aggregates_note": stringSchema(),
		},
	}
}

func pricingRuleSummarySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Effective pricing rule counts grouped by source priority and confidence.",
		"additionalProperties": true,
		"required":             []string{"total_rules", "by_source", "by_confidence", "override_rules", "official_rules", "fallback_rules", "oldest_updated_at", "newest_updated_at"},
		"properties": map[string]interface{}{
			"total_rules":       integerSchema(),
			"by_source":         stringIntMapSchema("Pricing rule counts grouped by pricing source."),
			"by_confidence":     stringIntMapSchema("Pricing rule counts grouped by confidence."),
			"override_rules":    integerSchema(),
			"official_rules":    integerSchema(),
			"fallback_rules":    integerSchema(),
			"oldest_updated_at": stringSchema(),
			"newest_updated_at": stringSchema(),
		},
	}
}

func pricingAuditRowSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "One effective pricing rule with source, matched model, match type, token prices, and confidence.",
		"additionalProperties": true,
		"required": []string{
			"model", "pricing_source", "matched_model", "match_type", "priority", "input_cost_per_token", "output_cost_per_token", "cache_read_input_token_cost", "cache_creation_input_token_cost", "effective_at", "updated_at", "confidence",
		},
		"properties": map[string]interface{}{
			"model":                           stringSchema(),
			"pricing_source":                  stringSchema(),
			"matched_model":                   stringSchema(),
			"match_type":                      stringSchema(),
			"priority":                        integerSchema(),
			"input_cost_per_token":            numberSchema(),
			"output_cost_per_token":           numberSchema(),
			"cache_read_input_token_cost":     numberSchema(),
			"cache_creation_input_token_cost": numberSchema(),
			"effective_at":                    stringSchema(),
			"updated_at":                      stringSchema(),
			"confidence":                      stringSchema(),
		},
	}
}

func pricingAuditRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Pricing audit rows for official, fallback, override, stale, fuzzy, and unpriced matches.",
		"items":       refSchema("PricingAuditRow"),
	}
}

func modelCallRowSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Model call analytics grouped by source, model, and project.",
		"additionalProperties": true,
		"required":             []string{"source", "model", "project", "calls", "tokens", "cost_usd", "avg_tokens_per_call", "cost_per_call", "unpriced_calls"},
		"properties": map[string]interface{}{
			"source":              stringSchema(),
			"model":               stringSchema(),
			"project":             stringSchema(),
			"calls":               integerSchema(),
			"tokens":              integerSchema(),
			"cost_usd":            numberSchema(),
			"avg_tokens_per_call": numberSchema(),
			"cost_per_call":       numberSchema(),
			"unpriced_calls":      integerSchema(),
		},
	}
}

func modelCallRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Model call analytics grouped by source, model, project, and session.",
		"items":       refSchema("ModelCallRow"),
	}
}

func modelRegistryRowSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Canonical model registry row derived from pricing governance and observed usage.",
		"additionalProperties": true,
		"required": []string{
			"model", "vendor", "family", "pricing_source", "matched_model", "match_type", "confidence", "input_cost_per_token", "output_cost_per_token", "cache_read_input_token_cost", "cache_creation_input_token_cost", "calls", "tokens", "cost_usd", "updated_at", "stale",
		},
		"properties": map[string]interface{}{
			"model":                           stringSchema(),
			"vendor":                          stringSchema(),
			"family":                          stringSchema(),
			"pricing_source":                  stringSchema(),
			"matched_model":                   stringSchema(),
			"match_type":                      stringSchema(),
			"confidence":                      stringSchema(),
			"input_cost_per_token":            numberSchema(),
			"output_cost_per_token":           numberSchema(),
			"cache_read_input_token_cost":     numberSchema(),
			"cache_creation_input_token_cost": numberSchema(),
			"calls":                           integerSchema(),
			"tokens":                          integerSchema(),
			"cost_usd":                        numberSchema(),
			"updated_at":                      stringSchema(),
			"stale":                           boolSchema(),
		},
	}
}

func modelRegistryRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Model pricing and provenance registry rows.",
		"items":       refSchema("ModelRegistryRow"),
	}
}

func costInsightRowSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "High-impact session explanation with token composition, pricing provenance, and advice. Prompt and response content are never included.",
		"additionalProperties": true,
		"required": []string{
			"source", "session_id", "calls", "tokens", "cost_usd", "cache_hit_rate", "pricing_sources", "pricing_confidences", "quality_score", "reasons", "advice",
		},
		"properties": map[string]interface{}{
			"source":                stringSchema(),
			"session_id":            stringSchema(),
			"project":               stringSchema(),
			"git_branch":            stringSchema(),
			"models":                integerSchema(),
			"calls":                 integerSchema(),
			"prompts":               integerSchema(),
			"input_tokens":          integerSchema(),
			"cache_read_tokens":     integerSchema(),
			"cache_write_tokens":    integerSchema(),
			"output_tokens":         integerSchema(),
			"reasoning_tokens":      integerSchema(),
			"tokens":                integerSchema(),
			"cost_usd":              numberSchema(),
			"cost_per_call":         numberSchema(),
			"cost_per_prompt":       numberSchema(),
			"tokens_per_prompt":     numberSchema(),
			"cache_hit_rate":        numberSchema(),
			"output_ratio":          numberSchema(),
			"pricing_sources":       stringArraySchema(),
			"pricing_confidences":   stringArraySchema(),
			"official_priced_calls": integerSchema(),
			"override_priced_calls": integerSchema(),
			"fallback_priced_calls": integerSchema(),
			"fuzzy_priced_calls":    integerSchema(),
			"source_reported_calls": integerSchema(),
			"unpriced_calls":        integerSchema(),
			"unknown_pricing_calls": integerSchema(),
			"quality_score":         numberSchema(),
			"reasons":               stringArraySchema(),
			"advice":                stringArraySchema(),
			"last_activity":         stringSchema(),
		},
	}
}

func auditEventSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Local immutable operational audit event. Params are redacted by privacy filters and do not contain prompts or secrets.",
		"additionalProperties": true,
		"required":             []string{"id", "actor", "role", "action", "target", "params", "created_at"},
		"properties": map[string]interface{}{
			"id":         integerSchema(),
			"actor":      stringSchema(),
			"role":       stringSchema(),
			"action":     stringSchema(),
			"target":     stringSchema(),
			"params":     stringSchema(),
			"created_at": stringSchema(),
		},
	}
}

func auditLogRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Local audit log rows with privacy filters applied by the server.",
		"items":       refSchema("AuditEvent"),
	}
}

func reconciliationImportSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Imported provider bill comparison summary. Payload hashes are persisted; raw provider statements are not echoed.",
		"additionalProperties": true,
		"required": []string{
			"id", "provider", "format", "currency", "local_cost_usd", "provider_cost_usd", "diff_usd", "rows_seen", "payload_sha256", "window_start", "window_end", "status", "notes", "warnings", "imported_at",
		},
		"properties": map[string]interface{}{
			"id":                integerSchema(),
			"provider":          stringSchema(),
			"format":            stringSchema(),
			"currency":          stringSchema(),
			"local_cost_usd":    numberSchema(),
			"provider_cost_usd": numberSchema(),
			"diff_usd":          numberSchema(),
			"rows_seen":         integerSchema(),
			"payload_sha256":    stringSchema(),
			"window_start":      stringSchema(),
			"window_end":        stringSchema(),
			"status":            stringSchema(),
			"notes":             stringSchema(),
			"warnings":          stringSchema(),
			"imported_at":       stringSchema(),
		},
	}
}

func reconciliationRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Provider reconciliation import rows.",
		"items":       refSchema("ReconciliationImport"),
	}
}

func reconciliationImportResponseSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Provider reconciliation import result.",
		"additionalProperties": true,
		"required":             []string{"ok", "import"},
		"properties": map[string]interface{}{
			"ok":     boolSchema(),
			"import": refSchema("ReconciliationImport"),
		},
	}
}

func routerSimulationRowSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Cost impact estimate for routing one source/model/project group to a different model.",
		"additionalProperties": true,
		"required": []string{
			"source", "from_model", "to_model", "project", "calls", "input_tokens", "output_tokens", "cache_creation_input_tokens", "cache_read_input_tokens", "tokens", "current_cost_usd", "simulated_cost_usd", "delta_usd", "savings_pct", "replacement_ratio", "unpriced_current_calls", "target_pricing_source", "target_pricing_model", "target_match_type", "target_confidence",
		},
		"properties": map[string]interface{}{
			"source":                      stringSchema(),
			"from_model":                  stringSchema(),
			"to_model":                    stringSchema(),
			"project":                     stringSchema(),
			"calls":                       integerSchema(),
			"input_tokens":                integerSchema(),
			"output_tokens":               integerSchema(),
			"cache_creation_input_tokens": integerSchema(),
			"cache_read_input_tokens":     integerSchema(),
			"tokens":                      integerSchema(),
			"current_cost_usd":            numberSchema(),
			"simulated_cost_usd":          numberSchema(),
			"delta_usd":                   numberSchema(),
			"savings_pct":                 numberSchema(),
			"replacement_ratio":           numberSchema(),
			"unpriced_current_calls":      integerSchema(),
			"target_pricing_source":       stringSchema(),
			"target_pricing_model":        stringSchema(),
			"target_match_type":           stringSchema(),
			"target_confidence":           stringSchema(),
			"note":                        stringSchema(),
		},
	}
}

func routerSimulationSummarySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Aggregate modeled cost impact across router simulation rows.",
		"additionalProperties": true,
		"required":             []string{"calls", "tokens", "current_cost_usd", "simulated_cost_usd", "delta_usd", "savings_pct", "groups", "unpriced_current_calls"},
		"properties": map[string]interface{}{
			"calls":                  integerSchema(),
			"tokens":                 integerSchema(),
			"current_cost_usd":       numberSchema(),
			"simulated_cost_usd":     numberSchema(),
			"delta_usd":              numberSchema(),
			"savings_pct":            numberSchema(),
			"groups":                 integerSchema(),
			"unpriced_current_calls": integerSchema(),
		},
	}
}

func routerSimulationReportSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Model router what-if savings estimate. It never mutates ledger records.",
		"additionalProperties": true,
		"required":             []string{"generated_at", "from", "to", "to_model", "replacement_ratio", "target_pricing", "status", "summary", "rows"},
		"properties": map[string]interface{}{
			"generated_at":      stringSchema(),
			"from":              stringSchema(),
			"to":                stringSchema(),
			"source":            stringSchema(),
			"from_model":        stringSchema(),
			"to_model":          stringSchema(),
			"project":           stringSchema(),
			"replacement_ratio": numberSchema(),
			"target_pricing":    refSchema("PricingAuditRow"),
			"status":            stringSchema(),
			"issues":            stringArraySchema(),
			"summary":           refSchema("RouterSimulationSummary"),
			"rows":              map[string]interface{}{"type": "array", "items": refSchema("RouterSimulationRow")},
		},
	}
}

func preflightEstimateValuesSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Estimated workload cost, token, call, prompt, and duration values.",
		"additionalProperties": true,
		"required":             []string{"cost_usd", "tokens", "calls", "prompts", "duration_minutes"},
		"properties": map[string]interface{}{
			"cost_usd":         numberSchema(),
			"tokens":           integerSchema(),
			"calls":            integerSchema(),
			"prompts":          integerSchema(),
			"duration_minutes": numberSchema(),
		},
	}
}

func preflightEstimateReportSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Historical preflight task cost, token, call, prompt, and duration estimate based only on metadata.",
		"additionalProperties": true,
		"required":             []string{"generated_at", "from", "to", "task", "method", "samples", "confidence", "factor", "baseline", "estimate", "p75"},
		"properties": map[string]interface{}{
			"generated_at": stringSchema(),
			"from":         stringSchema(),
			"to":           stringSchema(),
			"task":         stringSchema(),
			"source":       stringSchema(),
			"model":        stringSchema(),
			"project":      stringSchema(),
			"method":       stringSchema(),
			"samples":      integerSchema(),
			"confidence":   stringSchema(),
			"factor":       numberSchema(),
			"baseline":     refSchema("PreflightEstimateValues"),
			"estimate":     refSchema("PreflightEstimateValues"),
			"p75":          refSchema("PreflightEstimateValues"),
			"issues":       stringArraySchema(),
		},
	}
}

func chargebackRowSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Team/project/model/source showback row with confidence and mapping provenance.",
		"additionalProperties": true,
		"required":             []string{"team", "project", "source", "model", "calls", "sessions", "tokens", "cost_usd", "avg_tokens_per_call", "cost_per_call", "unpriced_calls", "mapping_source", "data_source", "confidence"},
		"properties": map[string]interface{}{
			"team":                stringSchema(),
			"project":             stringSchema(),
			"source":              stringSchema(),
			"model":               stringSchema(),
			"calls":               integerSchema(),
			"sessions":            integerSchema(),
			"tokens":              integerSchema(),
			"cost_usd":            numberSchema(),
			"avg_tokens_per_call": numberSchema(),
			"cost_per_call":       numberSchema(),
			"unpriced_calls":      integerSchema(),
			"mapping_source":      stringSchema(),
			"data_source":         stringSchema(),
			"confidence":          numberSchema(),
		},
	}
}

func chargebackRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Team/project/model/source showback and chargeback rows.",
		"items":       refSchema("ChargebackRow"),
	}
}

func wrappedProjectSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Project-level Agent Wrapped highlight.",
		"additionalProperties": true,
		"required":             []string{"project", "sessions", "calls", "tokens", "cost_usd"},
		"properties": map[string]interface{}{
			"project":  stringSchema(),
			"sessions": integerSchema(),
			"calls":    integerSchema(),
			"tokens":   integerSchema(),
			"cost_usd": numberSchema(),
		},
	}
}

func wrappedDaySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Day-level Agent Wrapped activity/cache highlight.",
		"additionalProperties": true,
		"required":             []string{"date", "calls", "tokens", "cost_usd", "cache_hit_rate"},
		"properties": map[string]interface{}{
			"date":           stringSchema(),
			"calls":          integerSchema(),
			"tokens":         integerSchema(),
			"cost_usd":       numberSchema(),
			"cache_hit_rate": numberSchema(),
		},
	}
}

func wrappedHighlightSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Concise narrative-safe Agent Wrapped fact.",
		"additionalProperties": true,
		"required":             []string{"label", "value", "detail"},
		"properties": map[string]interface{}{
			"label":  stringSchema(),
			"value":  stringSchema(),
			"detail": stringSchema(),
		},
	}
}

func agentWrappedReportSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Private period summary with top models, projects, sessions, cache days, and efficiency facts.",
		"additionalProperties": true,
		"required":             []string{"generated_at", "period", "from", "to", "stats", "top_model", "top_project", "most_active_day", "best_cache_day", "most_expensive_session", "highlights", "issues"},
		"properties": map[string]interface{}{
			"generated_at":           stringSchema(),
			"period":                 stringSchema(),
			"from":                   stringSchema(),
			"to":                     stringSchema(),
			"stats":                  refSchema("DashboardStats"),
			"top_model":              refSchema("CostByModel"),
			"top_project":            refSchema("WrappedProject"),
			"most_active_day":        refSchema("WrappedDay"),
			"best_cache_day":         refSchema("WrappedDay"),
			"most_expensive_session": refSchema("CostInsightRow"),
			"highlights":             map[string]interface{}{"type": "array", "items": refSchema("WrappedHighlight")},
			"issues":                 stringArraySchema(),
		},
	}
}

func workloadSummarySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Canonical goal-level workload ledger summary row.",
		"additionalProperties": true,
		"required": []string{
			"workload_id", "goal", "status", "source", "project", "repo", "git_branch", "owner", "team", "budget_usd", "outcome", "confidence", "created_at", "updated_at", "closed_at", "runs", "model_calls", "tool_calls", "sessions", "tokens", "cost_usd", "last_activity",
		},
		"properties": map[string]interface{}{
			"workload_id":   stringSchema(),
			"goal":          stringSchema(),
			"status":        stringSchema(),
			"source":        stringSchema(),
			"project":       stringSchema(),
			"repo":          stringSchema(),
			"git_branch":    stringSchema(),
			"owner":         stringSchema(),
			"team":          stringSchema(),
			"budget_usd":    numberSchema(),
			"outcome":       stringSchema(),
			"confidence":    numberSchema(),
			"created_at":    stringSchema(),
			"updated_at":    stringSchema(),
			"closed_at":     stringSchema(),
			"runs":          integerSchema(),
			"model_calls":   integerSchema(),
			"tool_calls":    integerSchema(),
			"sessions":      integerSchema(),
			"tokens":        integerSchema(),
			"cost_usd":      numberSchema(),
			"last_activity": stringSchema(),
		},
	}
}

func evidenceDashboardSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Privacy-redacted dashboard evidence summary included in incident bundles.",
		"additionalProperties": true,
		"properties": map[string]interface{}{
			"generated_at": stringSchema(),
			"from":         stringSchema(),
			"to":           stringSchema(),
			"granularity":  stringSchema(),
			"source":       stringSchema(),
			"model":        stringSchema(),
			"project":      stringSchema(),
			"stats":        refSchema("DashboardStats"),
			"consistency":  map[string]interface{}{"type": "array", "items": refSchema("DashboardConsistencyIssue")},
			"runtime":      refSchema("RuntimeStatus"),
		},
	}
}

func evidenceBundleSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Privacy-redacted incident evidence bundle for local audit, issue reports, and support.",
		"additionalProperties": true,
		"required":             []string{"product", "generated_at", "window", "privacy", "runtime", "quality", "ingestion_health", "pricing_sources", "pricing_rules", "pricing_audit", "dashboard", "anomaly_events", "watchdog_events", "cost_intelligence", "workload_states"},
		"properties": map[string]interface{}{
			"product":          stringSchema(),
			"generated_at":     stringSchema(),
			"window":           map[string]interface{}{"type": "object", "additionalProperties": stringSchema()},
			"privacy":          stringSchema(),
			"runtime":          refSchema("RuntimeStatus"),
			"quality":          refSchema("DataQualityReport"),
			"ingestion_health": map[string]interface{}{"type": "array", "items": refSchema("IngestionHealth")},
			"pricing_sources":  map[string]interface{}{"type": "array", "items": refSchema("PricingSourceStatus")},
			"pricing_rules":    refSchema("PricingRuleSummary"),
			"pricing_audit":    map[string]interface{}{"type": "array", "items": refSchema("PricingAuditRow")},
			"dashboard":        refSchema("EvidenceDashboard"),
			"anomaly_events":   map[string]interface{}{"type": "array", "items": refSchema("InsightEvent")},
			"watchdog_events":  map[string]interface{}{"type": "array", "items": refSchema("InsightEvent")},
			"cost_intelligence": map[string]interface{}{
				"type":  "array",
				"items": refSchema("CostInsightRow"),
			},
			"workload_states": map[string]interface{}{"type": "array", "items": refSchema("WorkloadState")},
		},
	}
}

func offlineBundleDataSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Offline bundle data payload containing ingestible canonical events and summary snapshots.",
		"additionalProperties": true,
		"required":             []string{"canonical_events", "stats", "workloads", "model_calls", "daily"},
		"properties": map[string]interface{}{
			"canonical_events": map[string]interface{}{"type": "array", "items": refSchema("CanonicalEvent")},
			"stats":            refSchema("DashboardStats"),
			"workloads":        map[string]interface{}{"type": "array", "items": refSchema("WorkloadSummary")},
			"model_calls":      map[string]interface{}{"type": "array", "items": refSchema("ModelCallRow")},
			"daily":            map[string]interface{}{"type": "array", "items": refSchema("TokenTimeSeriesPoint")},
			"quality":          refSchema("DataQualityReport"),
		},
	}
}

func offlineBundleIntegritySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Payload hash and optional HMAC signature metadata for an offline bundle.",
		"additionalProperties": true,
		"required":             []string{"hash_algorithm", "payload_sha256"},
		"properties": map[string]interface{}{
			"hash_algorithm":      stringSchema(),
			"payload_sha256":      stringSchema(),
			"signature_algorithm": stringSchema(),
			"signature":           stringSchema(),
			"key_id":              stringSchema(),
		},
	}
}

func offlineBundleSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Offline signed or unsigned local usage bundle for air-gapped aggregation.",
		"additionalProperties": true,
		"required":             []string{"schema_version", "product", "bundle_id", "generated_at", "window", "filters", "privacy", "data", "integrity"},
		"properties": map[string]interface{}{
			"schema_version": stringSchema(),
			"product":        stringSchema(),
			"bundle_id":      stringSchema(),
			"generated_at":   stringSchema(),
			"window":         map[string]interface{}{"type": "object", "additionalProperties": stringSchema()},
			"filters":        map[string]interface{}{"type": "object", "additionalProperties": stringSchema()},
			"privacy":        stringSchema(),
			"data":           refSchema("OfflineBundleData"),
			"integrity":      refSchema("OfflineBundleIntegrity"),
		},
	}
}

func offlineBundleImportResultSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Offline bundle import and merge outcome.",
		"additionalProperties": true,
		"required":             []string{"bundle_id", "events_seen", "events_inserted", "events_duplicate", "payload_sha256", "signature_verified"},
		"properties": map[string]interface{}{
			"bundle_id":          stringSchema(),
			"events_seen":        integerSchema(),
			"events_inserted":    integerSchema(),
			"events_duplicate":   integerSchema(),
			"payload_sha256":     stringSchema(),
			"signature_verified": boolSchema(),
		},
	}
}

func offlineBundleImportResponseSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Offline bundle import result.",
		"additionalProperties": true,
		"required":             []string{"ok", "result"},
		"properties": map[string]interface{}{
			"ok":     boolSchema(),
			"result": refSchema("OfflineBundleImportResult"),
		},
	}
}

func sessionDetailRowSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Per-model token and cost breakdown for one scoped session.",
		"additionalProperties": true,
		"required":             []string{"model", "calls", "input_tokens", "output_tokens", "cache_read", "cache_create", "cost_usd"},
		"properties": map[string]interface{}{
			"model":         stringSchema(),
			"calls":         integerSchema(),
			"input_tokens":  integerSchema(),
			"output_tokens": integerSchema(),
			"cache_read":    integerSchema(),
			"cache_create":  integerSchema(),
			"cost_usd":      numberSchema(),
		},
	}
}

func sessionDetailRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Per-model usage breakdown for one scoped source/session pair.",
		"items":       refSchema("SessionDetailRow"),
	}
}

func sessionReplayPointSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "One chronological model-call replay point with cumulative token and cost counters.",
		"additionalProperties": true,
		"required": []string{
			"timestamp", "source", "session_id", "model", "input_tokens", "output_tokens", "cache_read", "cache_create", "reasoning_output_tokens", "tokens", "cost_usd", "cumulative_tokens", "cumulative_cost_usd", "cumulative_calls", "pricing_source", "pricing_model", "pricing_confidence",
		},
		"properties": map[string]interface{}{
			"timestamp":               stringSchema(),
			"source":                  stringSchema(),
			"session_id":              stringSchema(),
			"model":                   stringSchema(),
			"input_tokens":            integerSchema(),
			"output_tokens":           integerSchema(),
			"cache_read":              integerSchema(),
			"cache_create":            integerSchema(),
			"reasoning_output_tokens": integerSchema(),
			"tokens":                  integerSchema(),
			"cost_usd":                numberSchema(),
			"cumulative_tokens":       integerSchema(),
			"cumulative_cost_usd":     numberSchema(),
			"cumulative_calls":        integerSchema(),
			"pricing_source":          stringSchema(),
			"pricing_model":           stringSchema(),
			"pricing_confidence":      stringSchema(),
		},
	}
}

func sessionReplaySchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Bounded, chronological per-call replay for one source/session. Prompt and response content are not included.",
		"additionalProperties": true,
		"required":             []string{"source", "session_id", "start_time", "end_time", "calls", "total_tokens", "total_cost_usd", "peak_tokens_per_call", "truncated", "points"},
		"properties": map[string]interface{}{
			"source":               stringSchema(),
			"session_id":           stringSchema(),
			"start_time":           stringSchema(),
			"end_time":             stringSchema(),
			"calls":                integerSchema(),
			"total_tokens":         integerSchema(),
			"total_cost_usd":       numberSchema(),
			"peak_tokens_per_call": integerSchema(),
			"truncated":            boolSchema(),
			"points": map[string]interface{}{
				"type":  "array",
				"items": refSchema("SessionReplayPoint"),
			},
		},
	}
}

func ingestionPathStatusSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Reachability and readability status for one configured collector path. Privacy filters may redact the path value in future responses.",
		"additionalProperties": true,
		"required":             []string{"path", "exists", "readable"},
		"properties": map[string]interface{}{
			"path":     stringSchema(),
			"exists":   boolSchema(),
			"readable": boolSchema(),
			"error":    stringSchema(),
		},
	}
}

func ingestionHealthSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Latest collector health, path reachability, watermark, scan duration, inserted rows, skipped rows, and last error for one source.",
		"additionalProperties": true,
		"required":             []string{"source", "enabled", "paths", "path_status", "last_scan_at", "duration_ms", "watermark", "files_seen", "records_inserted", "prompts_inserted", "skipped_rows", "last_error"},
		"properties": map[string]interface{}{
			"source":           stringSchema(),
			"enabled":          boolSchema(),
			"paths":            stringArraySchema(),
			"path_status":      map[string]interface{}{"type": "array", "items": refSchema("IngestionPathStatus")},
			"last_scan_at":     stringSchema(),
			"duration_ms":      integerSchema(),
			"watermark":        stringSchema(),
			"files_seen":       integerSchema(),
			"records_inserted": integerSchema(),
			"prompts_inserted": integerSchema(),
			"skipped_rows":     integerSchema(),
			"last_error":       stringSchema(),
		},
	}
}

func ingestionHealthRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Collector health rows with path, scan, watermark, duration, insert, skip, and error summaries.",
		"items":       refSchema("IngestionHealth"),
	}
}

func budgetStatusSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Current consumption against one configured budget rule.",
		"additionalProperties": true,
		"required":             []string{"name", "period", "scope", "match", "metric", "value", "limit", "ratio", "severity", "message", "period_key"},
		"properties": map[string]interface{}{
			"name":       stringSchema(),
			"period":     stringSchema(),
			"scope":      stringSchema(),
			"match":      stringSchema(),
			"metric":     stringSchema(),
			"value":      numberSchema(),
			"limit":      numberSchema(),
			"ratio":      numberSchema(),
			"severity":   stringSchema(),
			"message":    stringSchema(),
			"period_key": stringSchema(),
		},
	}
}

func budgetStatusResponseSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Budget feature state and evaluated local budget rules. Events are persisted locally only when enabled and severity is not ok.",
		"additionalProperties": true,
		"required":             []string{"enabled", "rules"},
		"properties": map[string]interface{}{
			"enabled": boolSchema(),
			"rules": map[string]interface{}{
				"type":  "array",
				"items": refSchema("BudgetStatus"),
			},
		},
	}
}

func quotaWindowSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Local estimated quota window. Subscription quota is an estimate and is not provider invoice billing.",
		"additionalProperties": true,
		"required": []string{
			"name", "from", "to", "cost_usd", "tokens", "prompts", "cost_limit", "token_limit", "remaining_cost", "remaining_tokens", "burn_rate_per_hour", "projected_cost_usd", "projected_tokens", "reset_at", "time_to_limit_hours",
		},
		"properties": map[string]interface{}{
			"name":                stringSchema(),
			"from":                stringSchema(),
			"to":                  stringSchema(),
			"cost_usd":            numberSchema(),
			"tokens":              integerSchema(),
			"prompts":             integerSchema(),
			"cost_limit":          numberSchema(),
			"token_limit":         integerSchema(),
			"remaining_cost":      numberSchema(),
			"remaining_tokens":    integerSchema(),
			"burn_rate_per_hour":  numberSchema(),
			"projected_cost_usd":  numberSchema(),
			"projected_tokens":    integerSchema(),
			"reset_at":            stringSchema(),
			"time_to_limit_hours": numberSchema(),
		},
	}
}

func quotaStatusSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Local estimated plan, reset calendar, burn-rate, and remaining quota windows. It does not claim to match provider billing exactly.",
		"additionalProperties": true,
		"required":             []string{"enabled", "plan", "reset_day", "windows", "method"},
		"properties": map[string]interface{}{
			"enabled":   boolSchema(),
			"plan":      stringSchema(),
			"reset_day": integerSchema(),
			"windows": map[string]interface{}{
				"type":  "array",
				"items": refSchema("QuotaWindow"),
			},
			"method": stringSchema(),
		},
	}
}

func doctorCheckSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "One diagnostic finding with a visible remediation action.",
		"additionalProperties": true,
		"required":             []string{"name", "status", "severity", "message", "action"},
		"properties": map[string]interface{}{
			"name":     stringSchema(),
			"source":   stringSchema(),
			"status":   stringSchema(),
			"severity": stringSchema(),
			"message":  stringSchema(),
			"action":   stringSchema(),
		},
	}
}

func controlIdempotencyOperationStatsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Retry-safe control write counts for one operation. Raw idempotency keys and hashes are never exposed.",
		"additionalProperties": true,
		"required":             []string{"operation", "keys", "replays", "last_seen_at"},
		"properties": map[string]interface{}{
			"operation":    stringSchema(),
			"keys":         integerSchema(),
			"replays":      integerSchema(),
			"last_seen_at": stringSchema(),
		},
	}
}

func controlIdempotencyStatsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Privacy-safe retry and replay statistics for idempotent control-plane writes.",
		"additionalProperties": true,
		"required":             []string{"total_keys", "replayed_keys", "replay_count", "last_seen_at", "operations"},
		"properties": map[string]interface{}{
			"total_keys":    integerSchema(),
			"replayed_keys": integerSchema(),
			"replay_count":  integerSchema(),
			"last_seen_at":  stringSchema(),
			"operations": map[string]interface{}{
				"type":  "array",
				"items": refSchema("ControlIdempotencyOperationStats"),
			},
		},
	}
}

func workloadLeaseStatsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Privacy-safe workload lease counts without exposing lease tokens.",
		"additionalProperties": true,
		"required":             []string{"active", "expired", "released", "total", "next_expiry_at"},
		"properties": map[string]interface{}{
			"active":         integerSchema(),
			"expired":        integerSchema(),
			"released":       integerSchema(),
			"total":          integerSchema(),
			"next_expiry_at": stringSchema(),
		},
	}
}

func doctorReportSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "One-click local diagnostic report. Privacy filters redact paths, projects, branches, and session ids where supported.",
		"additionalProperties": true,
		"required":             []string{"generated_at", "from", "to", "stats", "ingestion", "quality", "projection", "pricing_sources", "checks", "summary"},
		"properties": map[string]interface{}{
			"generated_at": stringSchema(),
			"from":         stringSchema(),
			"to":           stringSchema(),
			"stats":        refSchema("DashboardStats"),
			"ingestion": map[string]interface{}{
				"type":  "array",
				"items": refSchema("IngestionHealth"),
			},
			"quality":    refSchema("DataQualityReport"),
			"projection": refSchema("ProjectionQuality"),
			"workload_states": map[string]interface{}{
				"type":  "array",
				"items": refSchema("WorkloadState"),
			},
			"pricing_sources": map[string]interface{}{
				"type":  "array",
				"items": refSchema("PricingSourceStatus"),
			},
			"idempotency": refSchema("ControlIdempotencyStats"),
			"leases":      refSchema("WorkloadLeaseStats"),
			"checks": map[string]interface{}{
				"type":  "array",
				"items": refSchema("DoctorCheck"),
			},
			"summary": stringSchema(),
			"runtime": refSchema("RuntimeStatus"),
		},
	}
}

func cacheDoctorRowSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Cache hit, cache write/read, and estimated cache miss diagnostic row for one source/model/project group.",
		"additionalProperties": true,
		"required":             []string{"source", "model", "project", "calls", "input_tokens", "cache_read_tokens", "cache_write_tokens", "output_tokens", "cost_usd", "cache_hit_rate", "estimated_lost_saving", "message"},
		"properties": map[string]interface{}{
			"source":                stringSchema(),
			"model":                 stringSchema(),
			"project":               stringSchema(),
			"calls":                 integerSchema(),
			"input_tokens":          integerSchema(),
			"cache_read_tokens":     integerSchema(),
			"cache_write_tokens":    integerSchema(),
			"output_tokens":         integerSchema(),
			"cost_usd":              numberSchema(),
			"cache_hit_rate":        numberSchema(),
			"estimated_lost_saving": numberSchema(),
			"message":               stringSchema(),
		},
	}
}

func cacheDoctorRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Cache diagnostic rows grouped by source, model, and project.",
		"items":       refSchema("CacheDoctorRow"),
	}
}

func insightEventSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          "Local anomaly, watchdog, or quality signal. Privacy filters may hash session_id and redact project metadata.",
		"additionalProperties": true,
		"required":             []string{"id", "kind", "severity", "source", "model", "project", "session_id", "metric", "value", "baseline", "message", "created_at"},
		"properties": map[string]interface{}{
			"id":         integerSchema(),
			"kind":       stringSchema(),
			"severity":   stringSchema(),
			"source":     stringSchema(),
			"model":      stringSchema(),
			"project":    stringSchema(),
			"session_id": stringSchema(),
			"metric":     stringSchema(),
			"value":      numberSchema(),
			"baseline":   numberSchema(),
			"message":    stringSchema(),
			"created_at": stringSchema(),
		},
	}
}

func insightEventRowsSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "Anomaly or watchdog insight event rows.",
		"items":       refSchema("InsightEvent"),
	}
}

func looseObjectSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          description,
		"additionalProperties": true,
	}
}
