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
				"AgentRunHeartbeatResponse": looseObjectSchema("Recorded metadata-only agent run heartbeat."),
				"AgentRunLivenessResponse":  looseObjectSchema("Active async agent run liveness rows with privacy filters applied by the server."),
				"WorkloadDetail":            looseObjectSchema("Full workload ledger detail with privacy filters applied by the server."),
				"WorkloadGraph":             looseObjectSchema("Compact workload dependency and activity graph."),
				"WorkloadTimelineResponse":  looseObjectSchema("Chronological metadata-only workload audit timeline."),
				"WorkloadState":             looseObjectSchema("Derived terminal-state snapshot for one async agent workload."),
				"WorkloadEventFeed":         looseObjectSchema("Cursor-stable workload state feed."),
				"DashboardStats":            looseObjectSchema("Aggregate usage, token, cost, prompt, session, cache, budget, and runtime totals."),
				"DashboardBundle":           looseObjectSchema("Full dashboard bundle with aggregate chart data, consistency metadata, and runtime state."),
				"CostByModelRows":           looseObjectSchema("Cost distribution rows grouped by model."),
				"CostTrendRows":             looseObjectSchema("Aggregate cost trend points."),
				"TokenTrendRows":            looseObjectSchema("Aggregate token trend points."),
				"SessionPage":               looseObjectSchema("Server-side paginated session ledger page."),
				"SessionDetail":             looseObjectSchema("Scoped session detail with records and prompt counts."),
				"SessionReplay":             looseObjectSchema("Per-call token and cost replay points for one session."),
				"FleetAttributionReport":    looseObjectSchema("Heuristic sub-agent, parent/child, and parallel-run attribution report."),
				"IngestionHealthRows":       looseObjectSchema("Collector health rows with path, scan, watermark, and error summaries."),
				"OperationResult":           looseObjectSchema("Local operation acknowledgement."),
				"ProjectionRepairResult":    looseObjectSchema("Canonical-to-usage projection repair result."),
				"PricingSourceStatus":       pricingSourceStatusSchema(),
				"PricingStatus":             pricingStatusSchema(),
				"PricingAuditRows":          looseObjectSchema("Pricing audit rows for official, fallback, override, stale, fuzzy, and unpriced matches."),
				"BudgetStatusResponse":      looseObjectSchema("Configured budget rules and current severity state."),
				"QuotaStatus":               looseObjectSchema("Local estimated quota windows, reset calendar, burn-rate, and remaining usage."),
				"DataQualityReport":         looseObjectSchema("Trust and completeness diagnostics for usage, pricing, provenance, duplicates, and malformed rows."),
				"DoctorReport":              looseObjectSchema("One-click local diagnostic report. Privacy filters redact paths, projects, branches, and session ids."),
				"ModelCallRows":             looseObjectSchema("Model call analytics grouped by source, model, project, and session."),
				"ModelRegistryRows":         looseObjectSchema("Model pricing and provenance registry rows."),
				"CostIntelligenceRows":      costIntelligenceRowsSchema(),
				"CacheDoctorRows":           looseObjectSchema("Cache hit, cache write/read, and cache miss diagnostic rows."),
				"InsightEventRows":          looseObjectSchema("Anomaly or watchdog insight event rows."),
				"WebhookNotificationResult": looseObjectSchema("Redacted webhook delivery or dry-run result."),
				"AuditLogRows":              looseObjectSchema("Local audit log rows with privacy filters applied by the server."),
				"ReconciliationRows":        looseObjectSchema("Provider reconciliation import rows."),
				"ReconciliationImportRequest": looseObjectSchema(
					"Provider CSV/JSON statement or manual reconciliation summary. Payload hashes are persisted; secrets and prompt content are not accepted.",
				),
				"ReconciliationImportResponse": looseObjectSchema("Provider reconciliation import result."),
				"RouterSimulationReport":       looseObjectSchema("Model router what-if savings estimate."),
				"PreflightEstimateReport":      looseObjectSchema("Historical preflight task cost, token, and duration estimate."),
				"ChargebackRows":               looseObjectSchema("Team/project/model/source showback and chargeback rows."),
				"AgentWrappedReport":           looseObjectSchema("Private period summary with top models, projects, sessions, cache days, and efficiency facts."),
				"EvidenceBundle":               looseObjectSchema("Privacy-redacted incident evidence bundle for local audit, issue reports, and support."),
				"OfflineBundle":                looseObjectSchema("Offline signed or unsigned local usage bundle."),
				"OfflineBundleImportRequest":   looseObjectSchema("Offline bundle JSON. Optional signature verification uses local environment key material only."),
				"OfflineBundleImportResponse":  looseObjectSchema("Offline bundle import result."),
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

func looseObjectSchema(description string) map[string]interface{} {
	return map[string]interface{}{
		"type":                 "object",
		"description":          description,
		"additionalProperties": true,
	}
}
