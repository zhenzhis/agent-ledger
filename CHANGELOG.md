# Changelog

## Unreleased

### Added

- Official Agent Ledger naming across module path, binary, Docker, release metadata, and documentation.
- Pricing governance with local override, official OpenAI/Anthropic seed rows, LiteLLM fallback, pricing source health, snapshots, audit events, and per-record pricing confidence.
- Pricing sync now still applies official seed rows and local overrides when LiteLLM fallback fetch fails, and pricing status exposes effective rule counts by source and confidence.
- Cost Intelligence, Cache Doctor, Data Quality Center, Model Call Analytics, Quota Status, Watchdog events, evidence bundles, reconciliation imports, audit log, policy status, and expanded export types.
- Scoped Watchdog detection for runaway token/call density, calls per prompt, low output ratio, cache-miss risk, cost spikes, and non-working-hour usage, with stable insight-event upsert keys to avoid duplicate rows during dashboard refresh.
- Canonical Workload Ledger foundation with `canonical_events`, `workloads`, `agent_runs`, `model_calls`, `tool_calls`, `context_refs`, `artifacts`, `evaluations`, and `policy_decisions`.
- Workload dependency and lineage edges with `workload_links`, canonical `workload.linked` events, `POST /api/workloads/link`, `agent-ledger workload link`, and graph/timeline/detail projection for async goal DAGs.
- Queue-style workload claiming with `POST /api/workloads/claim-next`, `agent-ledger workload claim-next`, and MCP `ledger.claim_next_workload`; routers can atomically select the next claimable workload and create a lease in one SQLite transaction.
- Read-only workload queue probes with `GET /api/workloads/queue`, `agent-ledger workload queue`, and MCP `ledger.workload_queue`, reporting claimable counts, non-terminal status distribution, active/expired lease pressure, and next lease expiry without mutating SQLite.
- MCP resource `agent-ledger://workloads/queue` for read-only queue stats in agent context windows and router discovery flows.
- Control-plane readiness now includes privacy-safe workload queue claimability and lease pressure buckets for router and deployment probes.
- Capability catalog readiness metadata now declares workload queue claimability and lease pressure data classes for integration discovery.
- MCP resource subscription cursors now ignore volatile `generated_at` fields, preventing queue/readiness subscriptions from emitting no-op router wakeups.
- `GET /api/workloads/queue` now emits stable `ETag` values and honors `If-None-Match` with `304 Not Modified` when only `generated_at` changes.
- Short-lived workload leases with `workload_leases`, `POST /api/workloads/lease`, `POST /api/workloads/lease/renew`, `POST /api/workloads/lease/release`, `GET /api/workloads/leases`, `agent-ledger workload lease`, and MCP `ledger.acquire_workload_lease` / `ledger.renew_workload_lease` / `ledger.release_workload_lease` / `ledger.workload_leases`; plaintext lease tokens are returned only on acquire and stored as SHA-256 hashes.
- Async run start, heartbeat, and liveness ledger with `agent_run_events`, run snapshot fields, `agent.run.heartbeat` canonical events, `POST /api/agent-runs`, `POST /api/agent-runs/heartbeat`, `GET /api/agent-runs/liveness`, `agent-ledger workload start-run|heartbeat|liveness`, and MCP `ledger.start_run` / `ledger.heartbeat_run` / `ledger.run_liveness`.
- Workload event feeds now include stable content cursors, `generated_at`, HTTP `ETag` / `304 Not Modified` support, and SSE `id` values so local routers and monitors can consume state changes incrementally.
- Metadata-only canonical event schema and ingest through storage, `GET /api/event-schema`, `POST /api/events`, `agent-ledger event schema/ingest`, and `ledger.event_schema` / `ledger.record_event`.
- Privacy-safe canonical event templates through `GET /api/event-examples`, `agent-ledger event examples`, MCP `ledger.event_examples`, and `agent-ledger://schema/canonical-event-examples`.
- Canonical event schema version gating now accepts only `v1`; unknown event-envelope versions fail validation explicitly.
- Canonical event dry-run validation through `POST /api/events/validate` and `agent-ledger event validate`, sharing ingest validation without mutating SQLite.
- Machine-readable adapter contract through `GET /api/integrations/adapter-spec`, `agent-ledger adapter spec`, MCP `ledger.adapter_contract`, and `agent-ledger://integrations/adapter-contract`, defining supported input kinds, privacy-forbidden fields, token semantics, quality gates, validation commands, and ingest entrypoints.
- Adapter conformance validation through `POST /api/integrations/conformance` and `agent-ledger adapter conformance`, covering canonical, provider, OpenTelemetry GenAI, and A2A fixtures without writing SQLite.
- Adapter conformance strict mode via `strict=true` or `--strict` treats provenance warnings as CI failures.
- Discovery manifests now expose first-class runtime, canonical schema, event examples, adapter spec, and adapter conformance URIs for wrappers and routers.
- Canonical event schema now includes a stable `schema_hash`, also surfaced in conformance reports and discovery manifests.
- Adapter fixture examples under `examples/adapter-fixtures/` cover canonical, provider, OpenTelemetry GenAI, and A2A strict conformance.
- Provider adapter fixtures now separately cover OpenAI Responses, OpenAI Chat Completions, and Anthropic Messages usage envelopes in CI.
- CI now builds the binary, checks embedded UI JavaScript syntax, and runs strict conformance against public adapter fixtures.
- MCP now exposes read-only `ledger.validate_event`, `ledger.adapter_contract`, and `ledger.adapter_conformance` tools so agents can inspect contracts and verify events or fixtures before writing.
- Canonical event provenance fields for future adapters: `schema_version`, `source_version`, `parser_version`, `raw_ref`, and `match_type`, persisted locally and included in offline bundle exports.
- Data Quality and Doctor provenance checks for canonical events, including schema/source/parser coverage, raw reference coverage, match-type mix, confidence, and UI panel visibility.
- Provenance metadata is now populated by OpenTelemetry, A2A, provider usage, and gateway adapters so downstream quality checks can distinguish source-reported data from reconstructed references.
- Privacy-safe discovery manifest through `GET /.well-known/agent-ledger.json`, `GET /api/discovery`, and `agent-ledger discovery`, exposing local protocol entrypoints without prompt content, secrets, or raw source paths.
- Privacy-safe integration capability catalog through `GET /api/integrations`, `agent-ledger integrations`, and `ledger.integrations`, covering implemented collectors/protocols plus experimental provider gateway surfaces.
- OpenTelemetry GenAI JSON span mapping through `POST /api/otel/genai` and `agent-ledger otel convert|ingest`, projecting token metadata into canonical `model.call` plus hashed `context.ref` events while excluding prompt/completion message attributes.
- Optional local OTLP HTTP/JSON traces receiver through `POST /v1/traces` and `POST /api/otlp/v1/traces`, gated by `integrations.otlp_receiver.enabled` with body and span-count limits.
- A2A task telemetry mapping through `POST /api/a2a/tasks` and `agent-ledger a2a convert|ingest`, projecting task snapshots/events into workload, run, context, artifact, close, and evaluation events while excluding message/history/artifact-part content.
- Provider usage envelope mapping through `POST /api/provider/calls` and `agent-ledger provider convert|ingest`, supporting OpenAI-compatible, Anthropic-style, and LiteLLM-style usage objects while excluding request/response message content.
- Optional local OpenAI-compatible gateway through `POST /gateway/openai/v1/chat/completions`, gated by `gateway.enabled`, supporting JSON and SSE streaming responses, using environment-variable API keys, local policy checks, usage response metering, and audit metadata without storing prompt or response content.
- Optional local OpenAI Responses gateway through `POST /gateway/openai/v1/responses`, gated by `gateway.enabled`, supporting JSON and SSE streaming responses, environment-variable API keys, local policy checks, usage response metering from final `response.completed` events, and audit metadata without storing prompt or response content.
- Optional local Anthropic Messages gateway through `POST /gateway/anthropic/v1/messages`, gated by `gateway.enabled`, supporting non-streaming JSON responses, environment-variable API keys, local policy checks, usage response metering, and audit metadata without storing prompt or response content.
- Gateway streaming requests now ask compatible upstreams for final usage chunks by default through `gateway.include_stream_usage`, while preserving explicit client `stream_options.include_usage` settings.
- Gateway ledger-context attribution for project, goal, workload id, run id, session id, and branch via query parameters or request metadata.
- Provider bill reconciliation import through `POST /api/reconciliation/import` and `agent-ledger reconcile import/status`, parsing local CSV/JSON statements into summary totals, statement hash, window, warnings, and local/provider cost diff.
- Model Router Simulator through `GET /api/router/simulate` and `agent-ledger router simulate`, estimating target-model routing savings from existing token components and pricing governance metadata without mutating ledger records.
- Preflight Cost Estimate through `GET /api/preflight/estimate` and `agent-ledger preflight`, estimating likely task cost/tokens/calls from local historical session medians and task-type multipliers.
- Team Chargeback/Showback through `GET /api/chargeback`, `GET /api/export?type=chargeback`, and `agent-ledger chargeback`, using raw usage as the authoritative source and canonical model calls only as fallback.
- Fleet Attribution through `GET /api/fleet-attribution` and `agent-ledger fleet`, reporting explicit sub-agent parent links, overlapping parallel runs, model-call totals, tokens, and costs.
- Agent Wrapped through `GET /api/wrapped` and `agent-ledger wrapped`, producing monthly/weekly/yearly Markdown or JSON summaries from metadata-only usage signals.
- One-click Doctor through `GET /api/doctor` and `agent-ledger doctor`, combining usage, ingestion health, pricing freshness, and data-quality checks into actionable JSON/Markdown diagnostics.
- Doctor and evidence bundles now include bounded workload terminal-state snapshots, surfacing stale runs, blocked policy decisions, approval waits, missing evaluations, and budget-exhausted workloads.
- Evidence bundles now include redacted ingestion health, pricing source/rule status, dashboard consistency, anomaly events, and watchdog events for data discrepancy investigations.
- Consistent dashboard bundle through `GET /api/dashboard`, so KPI, token, cost, and model panels read one storage snapshot and avoid mixed old/new states during scans.
- Canonical `model.call` events now project into `usage_records`, while legacy session backfill skips sessions already owned by canonical workloads to avoid duplicate workload rows.
- Pricing recalculation now updates canonical `model_calls` as well as `usage_records`, keeping workload detail costs aligned with dashboard and export totals.
- Doctor and Data Quality reports now include canonical-to-usage projection consistency checks for missing projections, cost mismatches, and duplicate legacy/canonical session owners.
- Canonical-to-usage projection repair through `POST /api/projections/repair` and `agent-ledger projection repair`, backfilling missing projected usage rows, realigning cache/cost metadata, and rebuilding usage aggregates.
- Advanced dashboard action for projection repair using the current time/source/model/project filters.
- Quota/Battery forecasts now include reset time, projected window usage, and estimated time-to-limit based on local burn rate.
- Session Cost Replay through `GET /api/session-replay` and `agent-ledger replay`, returning chronological per-call token/cost accumulation without reading prompt content.
- Repo AI Cost Badge through `GET /api/badge/repo.svg` and `agent-ledger badge`, rendering local black/white SVG cost, token, session, or cache badges.
- Offline bundle export/import for air-gapped multi-machine aggregation, with metadata-only canonical event replay, payload SHA-256 integrity, optional HMAC-SHA256 signing, CLI commands, and REST endpoints.
- Shared local policy evaluator across MCP, `POST /api/policy/evaluate`, and `agent-ledger policy evaluate`, with stable action severity and optional decision recording.
- Policy rules can now match AgentOps dimensions including target, repo, git branch, and team, while preserving existing source/model/project/action/role rules.
- Policy-aware export/report governance, recording policy decisions for export/report/evidence/offline-bundle operations and blocking configured `block` or `require_approval` actions.
- Historical policy audit through `GET /api/policy/audit`, `agent-ledger policy audit`, and MCP `ledger.policy_audit`, applying the same local evaluator to usage sessions, tool calls, and workloads.
- Policy enforcement evidence through `GET /api/policy/enforcement` and `agent-ledger policy enforcement`, combining recent policy decisions, approval requests, and policy audit events with privacy redaction.
- Policy audit summary in the Data Quality panel so matched warnings, approvals, and blocks are visible during normal dashboard review.
- Filterable operational audit log through `GET /api/audit-log`, `agent-ledger audit`, MCP `ledger.audit_log`, privacy-aware audit export, and a dashboard Audit Log panel.
- Local policy approval requests with `GET/POST /api/policy/approvals`, `agent-ledger policy approvals`, and `agent-ledger policy resolve`, allowing approved action/target retries by `approval_id`.
- Legacy session backfill into workload/run/model-call records for immediate compatibility with existing local data.
- Workload APIs: list/create/close/start-run/heartbeat/liveness/detail/graph/timeline/state, model registry, policy decisions, and workload CSV/JSON export.
- Workload detail and graph now expose `context_refs`, making context/worktree/protocol references visible alongside runs, model calls, tools, artifacts, evaluations, and policies.
- Workload audit timeline through `GET /api/workload-timeline`, `agent-ledger workload timeline`, and MCP `ledger.workload_timeline`, merging runs, heartbeats, model calls, tool calls, context refs, artifacts, evaluations, and policies.
- Workload terminal-state snapshots through `GET /api/workload-state`, `agent-ledger workload state`, and MCP `ledger.workload_state`, deriving running/stale/blocked/needs-evaluation/accepted phases from local metadata only.
- MCP `agent-ledger://workloads/recent` now includes derived terminal-state snapshots alongside recent workload summary rows for read-only agent context.
- Local workload event feed through `GET /api/workload-events`, `GET /api/workload-events/stream`, and `agent-ledger workload feed`, deriving metadata-only phase/severity/next-action events for monitors, routers, and notification adapters.
- Disabled-by-default redacted webhook notifications through `POST /api/notifications/webhook` and `agent-ledger notify webhook`, with dry-run support, bounded event and pending-approval counts, audit logging, hashed request/workload/run ids, and forced redaction of goals, projects, repos, branches, teams, approval targets, approval reasons, and request payloads.
- `agent-ledger notify webhook --dry-run` is treated as a read-only CLI operation, allowing observer deployments to inspect redacted notification payloads without sending webhooks or mutating state.
- Read-only HTTP observer mode now allows notification dry-runs while still rejecting real webhook sends, and notification audit writes now respect the read-only guard.
- Read-only HTTP observer mode now allows policy evaluation without recording decisions, while `record=true` remains explicitly forbidden.
- Workload detail UI now shows the recent audit timeline while isolating timeline API failures from the core detail view.
- Explicit tool-call entrypoints through `agent-ledger workload tool` and MCP `ledger.record_tool_call`, recording tool metadata without command parameters or tool input content.
- Explicit context-reference entrypoints through `agent-ledger workload context` and MCP `ledger.record_context`, both backed by canonical `context.ref` events.
- Explicit evaluation entrypoints through `agent-ledger workload evaluation` and MCP `ledger.record_evaluation`, recording test, review, quality, or acceptance signals without prompt or artifact content.
- Local MCP stdio JSON-RPC tools for budget lookup, workload lifecycle, workload lease lifecycle, run start/heartbeat/liveness, cursor-stable workload feed access, privacy-safe artifacts, advisory policy decisions, cost explanation, and similar workload search.
- Local MCP resources, resource subscriptions, and prompts for metadata-only schema/catalog/budget/workload/feed/policy context plus workload, cost-review, and incident-evidence templates.
- Hourly and daily usage aggregate tables with dashboard aggregate fallback.
- CLI commands: `today`, `top`, `doctor`, `battery`, `discovery`, `notify webhook`, `workload list/create/show/timeline/state/feed/close/start-run/heartbeat/liveness/tool/context/evaluation`, `run --goal ... -- <command>`, `export`, `pricing sync`, `policy enforcement`, `router simulate`, `preflight`, `replay`, `badge`, and `wrapped`.
- Cursor-compatible session pagination via `next_cursor`.
- Black/white/gray data-dense dashboard panels for workloads, pricing, quota, quality, model calls, cache, watchdog, and cost intelligence.
- Read-only observer mode through `rbac.read_only`, disabling background collectors, pricing sync, cost recalculation, REST/CLI writes, and GET-derived audit/insight/bundle writebacks.
- Runtime capability contract fields in integration catalog and discovery manifests: `writes_local_state`, `available_in_read_only`, and `runtime_status`.
- Observer/control-plane runtime status in dashboard, Doctor reports, evidence bundles, and the web UI header, with write controls disabled in read-only mode.
- Lightweight runtime status probes through `GET /api/runtime/status` and `agent-ledger runtime`.

### Changed

- Default database name is `agent-ledger.db`.
- Default system config path is `/etc/agent-ledger/config.yaml`.
- Docker runtime binary is `/agent-ledger`.
- Go module path is `github.com/zhenzhis/agent-ledger`.

### Security

- Added RBAC configuration fields and role checks for side-effectful governance APIs.
- Added a read-only control plane mode for observer deployments that must not mutate local SQLite state.
- Added local audit logging for scan, pricing sync, recalculation, and reconciliation import operations.
- Exports can be forced into privacy mode by policy.
- Agent run commands are best-effort redacted for common API key, token, password, secret, and bearer patterns before persistence, including canonical run events.
- Release workflows now configure Syft SBOMs for GoReleaser archives and BuildKit SBOM/provenance attestations for GHCR images.

### Credits

- Agent Ledger remains based on the local-first foundation from [briqt/agent-usage](https://github.com/briqt/agent-usage). Thanks to the original author and contributors.
