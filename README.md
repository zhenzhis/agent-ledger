# Agent Ledger

Private local AI Agent FinOps, workload ledger, quota, pricing, audit, and productivity console for Claude Code, Codex, OpenCode, OpenClaw, kiro, Pi, and related local coding agents.

[中文文档](README_CN.md)

![Agent Ledger dashboard](docs/dashboard.png)

## Fork And Credits

Agent Ledger is an independent ZhenZhi second-development project based on [briqt/agent-usage](https://github.com/briqt/agent-usage). We keep the local-first collector foundation and thank the original author and contributors for the clean single-binary design.

The project has been renamed from `agent-usage` to `agent-ledger`. Old local databases and configs are not deleted automatically.

## What It Does

- Collects local usage records from Claude Code, Codex, OpenCode, OpenClaw, kiro, and Pi.
- Calculates token cost with local pricing governance: local overrides, official OpenAI/Anthropic seeds, and LiteLLM fallback.
- Explains expensive sessions without reading prompt content.
- Tracks budgets, burn rate, local quota estimates, cache health, model call counts, anomalies, and source health.
- Promotes raw sessions into a canonical Workload Ledger with goal, run, model-call, tool-call, artifact, evaluation, and policy-decision tables.
- Surfaces pending policy approvals in the web console with approver identity, quorum progress, and approve/reject actions.
- Provides local audit logs, privacy presets, exports, reports, evidence bundles, and team showback data.
- Runs as one Go binary with embedded static UI and SQLite.

## Quick Start

```bash
git clone https://github.com/zhenzhis/agent-ledger.git
cd agent-ledger
go build -o agent-ledger .
cp config.example.yaml config.yaml
./agent-ledger
```

Open [http://127.0.0.1:9800](http://127.0.0.1:9800).

Docker:

```bash
docker compose up -d --build
```

CLI:

```bash
./agent-ledger today
./agent-ledger top
./agent-ledger doctor
./agent-ledger battery
./agent-ledger workload list
./agent-ledger workload create --goal "review strategy engine" --source codex --project quant --idempotency-key router-task-001
./agent-ledger workload start-run --workload-id wl_... --source codex --agent-name codex --idempotency-key router-run-001
./agent-ledger workload lease acquire --workload-id wl_... --holder codex-router --ttl 30m
./agent-ledger workload lease renew --lease-id lease_... --lease-token lt_... --ttl 30m
./agent-ledger workload lease release --lease-id lease_... --lease-token lt_...
./agent-ledger workload heartbeat --run-id run_... --status working --phase testing --progress 0.5
./agent-ledger workload liveness --max-age 10m --stale-only
./agent-ledger workload state --workload-id wl_... --max-age 10m
./agent-ledger workload feed --severity warning --max-age 10m
./agent-ledger workload link --from wl_child --to wl_parent --relation depends_on
./agent-ledger workload evaluation --workload-id wl_... --run-id run_... --status pass --score 0.97 --signal unit-tests
./agent-ledger run --goal "debug ingestion" --agent codex -- codex
./agent-ledger event schema
./agent-ledger event examples --type model.call
./agent-ledger event validate --file event.json
./agent-ledger event ingest --file event.json
./agent-ledger adapter spec
./agent-ledger adapter conformance --kind provider --strict --file fixture.json
./agent-ledger discovery
./agent-ledger contracts
./agent-ledger contracts verify
./agent-ledger openapi
./agent-ledger integrations
./agent-ledger runtime
./agent-ledger config status --format markdown
./agent-ledger readiness --format markdown
./agent-ledger admission check --surface http --method POST --path /api/events --role operator
./agent-ledger notify webhook --dry-run --severity warning --approval-due-within 24h
./agent-ledger otel convert --file spans.json
./agent-ledger otel ingest --file spans.json
./agent-ledger a2a convert --file task.json
./agent-ledger a2a ingest --file task.json
./agent-ledger provider convert --file response.json
./agent-ledger provider ingest --file response.json
./agent-ledger projection quality
./agent-ledger projection repair --source gateway --from 2026-06-07 --to 2026-06-07
./agent-ledger reconcile parse --file provider-bill.csv --format csv
./agent-ledger reconcile import --file provider-bill.csv --format csv --provider openai
./agent-ledger reconcile status
./agent-ledger router simulate --to-model gpt-5-mini --from-model gpt-5 --ratio 0.5
./agent-ledger replay --source codex --session-id <id>
./agent-ledger badge --project repo-name --metric cost --out agent-ledger.svg
./agent-ledger preflight --task refactor --project repo-name
./agent-ledger bundle export --privacy --signed --out usage-bundle.json
./agent-ledger bundle import --file usage-bundle.json --verify
./agent-ledger policy evaluate --model gpt-5.5 --action model.call
./agent-ledger policy approvals --privacy
./agent-ledger policy enforcement --privacy
./agent-ledger policy routes --due-within 24h --privacy
./agent-ledger policy resolve --id apr_... --status approved
./agent-ledger audit --action pricing --role operator --format markdown --privacy
./agent-ledger pricing sync
./agent-ledger wrapped
./agent-ledger mcp
```

## Control-Plane Idempotency

Workload and run write operations support stable idempotency keys for async agent routers, wrappers, CI jobs, and retrying clients:

- HTTP: pass `Idempotency-Key` or `X-Idempotency-Key` on `POST /api/workloads` and `POST /api/agent-runs`.
- JSON: pass `idempotency_key` in the request body for the same endpoints.
- CLI: pass `--idempotency-key` to `agent-ledger workload create` or `agent-ledger workload start-run`.
- MCP: pass `idempotency_key` to `ledger.start_workload` or `ledger.start_run`.
- OpenAPI: `GET /api/openapi.json` describes the workload/run request schemas, idempotency headers, and `409 Conflict` response.

The first request writes the workload/run and records only the operation, key scope, request hash, result type, result id, and timestamps in SQLite. A retry with the same key and same normalized request returns the original id with `idempotent_replay: true`. Reusing the same key with different input fails explicitly with HTTP `409 Conflict` or a CLI/MCP error. Agent Ledger never stores the raw idempotency request body in the idempotency table.

## Workload Leases

Async routers and long-running agents can acquire a short-lived workload lease before executing a workload:

- REST: `POST /api/workloads/claim-next`, `POST /api/workloads/lease`, `POST /api/workloads/lease/renew`, `POST /api/workloads/lease/release`, and `GET /api/workloads/leases`.
- CLI: `agent-ledger workload queue`, `agent-ledger workload claim-next --holder router-a`, plus `agent-ledger workload lease acquire|renew|release|list`.
- MCP: `ledger.workload_queue`, `ledger.claim_next_workload`, `ledger.acquire_workload_lease`, `ledger.renew_workload_lease`, `ledger.release_workload_lease`, and `ledger.workload_leases`.
- OpenAPI: `GET /api/openapi.json` includes lease request/response schemas and `409 Conflict` for an already active lease.

`claim-next` atomically selects one queue-eligible workload and creates the lease in the same SQLite transaction, so multiple local workers do not need to list and race. By default it claims `queued` or `active` workloads; pass `status=any` or a comma-separated non-terminal status list only when a router intentionally handles stalled or blocked work.

`GET /api/workloads/queue`, `agent-ledger workload queue`, and MCP `ledger.workload_queue` are read-only queue probes. They report claimable workload count, non-terminal status distribution, active/expired lease pressure, oldest claimable workload time, and next lease expiry without mutating expired lease rows.

Only one active lease is allowed per workload. `lease_token` is returned only from acquire/claim responses; list, renew, release, readiness, doctor, audit, and contract surfaces never expose it. SQLite stores a SHA-256 token hash, not the plaintext token. Read paths derive expired status without mutating SQLite, so observer/read-only mode remains read-only.

Strict adapter fixtures are available in `examples/adapter-fixtures/` for canonical events, OpenAI Responses, OpenAI Chat Completions, Anthropic Messages, provider SSE streams, OpenTelemetry GenAI spans, and A2A task snapshots.

## Configuration

Config search order:

1. `--config path/to/config.yaml`
2. `/etc/agent-ledger/config.yaml`
3. `./config.yaml`

Start from [config.example.yaml](config.example.yaml). It is local-first, contains no secrets, keeps outbound webhooks and the provider gateway disabled by default, and documents every enterprise control-plane section. Minimal example:

```yaml
server:
  port: 9800
  bind_address: "127.0.0.1"

storage:
  path: "./agent-ledger.db"

pricing:
  sync_interval: 1h
  stale_after: 24h
  mode: official-plus-litellm
  overrides: []

privacy:
  default_preset: normal
  redact_paths: false
  hash_session_ids: false
  hide_project_names: false
  screenshot_mode: false

rbac:
  enabled: false
  read_only: false

policies:
  enabled: false
  require_privacy_export: false
  rules:
    # - name: expensive-model-review
    #   scope: model
    #   match: gpt-5.5
    #   action: require_approval
    #   message: Review expensive model usage
    #   required_approvals: 2
    #   approvers: ["desk-lead", "risk"]
    #   escalate_after: 30m
    #   escalate_to: ["research-head"]

webhooks:
  enabled: false
  url: ""
  timeout: 10s
  max_events: 20

integrations:
  otlp_receiver:
    enabled: false
    max_body_bytes: 4194304
    max_spans: 1000

gateway:
  enabled: false
  upstream_base_url: "https://api.openai.com"
  api_key_env: "OPENAI_API_KEY"
  anthropic_upstream_base_url: "https://api.anthropic.com"
  anthropic_api_key_env: "ANTHROPIC_API_KEY"
  include_stream_usage: true
  max_body_bytes: 4194304
  max_response_bytes: 33554432
  timeout: 120s
```

Run `agent-ledger config status --format markdown` or open `GET /api/config/status` to inspect a privacy-safe deployment report. It exposes bind safety, auth presence, collector path counts, pricing mode, outbound surfaces, privacy flags, and remediation hints without printing raw paths, auth tokens, API keys, webhook URLs, machine names, authors, prompts, responses, or session ids.

Use `pricing.overrides` for enterprise contracts, relay pricing, regional multipliers, or provider-specific discounts.

The optional gateway is a local provider proxy for OpenAI-compatible Chat Completions, OpenAI Responses, and Anthropic Messages. It is disabled by default, supports OpenAI-compatible Chat Completions JSON/SSE, OpenAI Responses JSON/SSE, and Anthropic Messages JSON/SSE, reads upstream API keys from configured environment variables, and records token usage plus audit metadata without storing request messages or response content. OpenAI Responses streaming usage is recorded from final `response.completed` events. Anthropic Messages streaming usage is merged from `message_start` and `message_delta` SSE events. For OpenAI Chat Completions streaming calls, `include_stream_usage: true` asks compatible upstreams for a final usage chunk when the client did not explicitly set `stream_options.include_usage`; set it to `false` for relays that reject that option.

Webhook notifications are disabled by default. When explicitly enabled, `POST /api/notifications/webhook` and `agent-ledger notify webhook` send only bounded workload-event, pending-approval, and approval-route summaries. Goals, projects, repos, branches, teams, approver routes, escalation targets, approval targets, approval reasons, event ids, workload ids, run ids, and approval request ids are redacted or hashed. Use `--dry-run` or `dry_run=1` to inspect the outgoing payload without sending.

Gateway requests can attach ledger context through query parameters or request `metadata`: `agent_ledger.project`, `agent_ledger.goal`, `agent_ledger.workload_id`, `agent_ledger.agent_run_id`, `agent_ledger.session_id`, and `agent_ledger.git_branch`. This lets wrappers, MCP tools, and async agents bind live model calls to an existing workload/run without exposing prompt content.

## Pricing Model

Agent Ledger stores non-overlapping token components:

```text
total = input_tokens
      + cache_creation_input_tokens
      + cache_read_input_tokens
      + output_tokens
```

Cost formula:

```text
cost = input_tokens * input_price
     + cache_creation_input_tokens * cache_write_price
     + cache_read_input_tokens * cache_read_price
     + output_tokens * output_price
```

Pricing priority:

1. Local override.
2. Official OpenAI/Anthropic seed rows.
3. LiteLLM fallback from `model_prices_and_context_window.json`.
4. Source-reported cost, preserved for sources such as OpenCode when present.

Every priced record can expose pricing source, matched model, match type, and confidence. Unknown or stale prices are surfaced as data quality issues instead of being hidden.

References:

- [OpenAI API pricing](https://openai.com/api/pricing/)
- [Anthropic Claude pricing](https://platform.claude.com/docs/en/about-claude/pricing)
- [LiteLLM model price data](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json)

## Architecture

```text
collectors / CLI wrapper / MCP tools -> canonical events -> workload ledger
                                     -> raw usage + pricing governance -> aggregates
                                     -> REST API -> embedded dashboard / CLI
```

Core tables:

- `canonical_events`: normalized event stream for future collectors, MCP, A2A, and gateways, including schema/source/parser provenance and privacy-safe native references.
- `workloads`, `agent_runs`, `agent_run_events`, `model_calls`, `tool_calls`: goal/run/heartbeat/call ledger.
- `control_idempotency`: retry-safe workload/run write ledger with request hashes only.
- `workload_leases`: short-lived async execution claims with plaintext lease tokens excluded from SQLite.
- `workload_sessions`: compatibility link from old source-scoped sessions to workloads.
- `context_refs`, `artifacts`, `evaluations`, `policy_decisions`, `workload_links`: privacy-safe AgentOps context, artifact, outcome, governance, dependency, and lineage records.
- `usage_records`: raw API-call token and cost data.
- `sessions`: source-scoped session metadata.
- `prompt_events`: prompt timestamps for time-accurate prompt counts.
- `pricing`, `pricing_sources`, `pricing_snapshots`: effective price rules and source health.
- `hourly_usage_aggregate`, `daily_usage_aggregate`: dashboard rollups.
- `reconciliation_imports`: local provider bill comparisons with payload hash and statement window.
- `ingestion_health`, `insight_events`, `audit_log`: operations and quality evidence.

## API Surface

Common filters: `from`, `to`, `source`, `model`, `project`, `privacy`.

| Endpoint | Purpose |
|---|---|
| `GET /.well-known/agent-ledger.json` | Privacy-safe local discovery manifest for agents, wrappers, and routers |
| `GET /api/discovery` | Same discovery manifest under the API namespace |
| `GET /api/contracts` | One-shot contract bundle with stable document URIs, hashes, cache semantics, CLI commands, and MCP entrypoints |
| `GET /api/contracts/verify` | Machine-readable control-plane self-check for discovery, bundle, OpenAPI, schema, adapter, runtime, and privacy invariants |
| `GET /api/openapi.json` | Metadata-only OpenAPI 3.1 control-plane contract for wrapper, router, and CI integration |
| `GET /api/runtime/status` | Runtime mode, read-only state, background/write status, and compatibility hashes |
| `GET /api/config/status` | Privacy-safe deployment configuration report with risk checks and remediation hints |
| `GET /api/readiness` | Privacy-safe control-plane readiness report for probes, wrappers, routers, and CI |
| `GET /api/admission/check` | Privacy-safe dry-run of HTTP, CLI, or MCP operation access under current read-only/RBAC rules |
| `GET /api/dashboard` | Consistent KPI, token, cost, and model bundle for the web dashboard |
| `GET /api/stats` | Summary stats |
| `GET /api/workloads` | Server-side paginated workload ledger |
| `POST /api/workloads` | Create a local workload |
| `POST /api/workloads/close` | Close a workload with status/outcome |
| `POST /api/workloads/link` | Create a metadata-only dependency or lineage edge between workloads |
| `POST /api/workloads/claim-next` | Atomically claim the next queue-eligible workload and return a lease |
| `GET /api/workloads/queue` | Read-only queue claimability and lease pressure stats |
| `POST /api/workloads/lease` | Acquire one short-lived execution lease for a workload |
| `POST /api/workloads/lease/renew` | Renew an active workload lease using its lease token |
| `POST /api/workloads/lease/release` | Release an active workload lease using its lease token |
| `GET /api/workloads/leases` | List workload leases without returning lease tokens |
| `POST /api/agent-runs` | Start a run attached to an existing workload |
| `POST /api/agent-runs/heartbeat` | Append metadata-only async run liveness/progress |
| `GET /api/agent-runs/liveness` | List active runs and stale heartbeat state |
| `GET /api/workload-detail` | Workload runs, model calls, tools, sessions, policies |
| `GET /api/workload-graph` | Compact workload graph |
| `GET /api/workload-timeline` | Chronological workload audit timeline |
| `GET /api/workload-state` | Derived terminal-state snapshot for one async agent workload |
| `GET /api/workload-events` | Derived local workload state feed for monitors, routers, and notification adapters; returns `cursor`, `generated_at`, and `ETag` for incremental polling |
| `GET /api/workload-events/stream` | Local SSE workload state stream for polling monitors and router subscriptions; emits the feed cursor as the SSE `id` |
| `POST /api/notifications/webhook?approval_due_within=24h` | Explicitly send redacted workload-event, approval, and approval-route summaries to the configured webhook |
| `GET /api/integrations` | Privacy-safe integration capability catalog |
| `GET /api/integrations/adapter-spec` | Machine-readable adapter contract for future agent CLIs, frameworks, gateways, OTel, A2A, and provider integrations |
| `POST /api/integrations/conformance` | Validate adapter fixtures for canonical, provider, provider-stream, OpenTelemetry GenAI, or A2A compatibility without writing SQLite; `strict=true` turns provenance warnings into failures |
| `GET /api/event-schema` | Canonical event schema and supported event types |
| `GET /api/event-examples` | Privacy-safe canonical event templates, filterable by `type` or `event_type` |
| `POST /api/events/validate` | Validate canonical metadata-only events without writing to SQLite |
| `POST /api/events` | Ingest metadata-only canonical events |
| `POST /api/otel/genai` | Convert OpenTelemetry GenAI JSON spans into canonical model-call events |
| `POST /v1/traces` | Optional local OTLP HTTP JSON/protobuf traces receiver when `integrations.otlp_receiver.enabled` is true |
| `POST /api/otlp/v1/traces` | Same receiver under the API namespace for local reverse proxies |
| `POST /api/a2a/tasks` | Convert A2A JSON task snapshots/events into workload/run/artifact/evaluation events |
| `POST /api/provider/calls` | Convert provider response usage envelopes into canonical model-call events |
| `POST /gateway/openai/v1/chat/completions` | Optional OpenAI-compatible gateway for JSON and SSE streaming when `gateway.enabled` is true |
| `POST /gateway/openai/v1/responses` | Optional OpenAI Responses gateway for JSON and SSE streaming when `gateway.enabled` is true |
| `POST /gateway/anthropic/v1/messages` | Optional Anthropic Messages gateway for JSON and SSE streaming when `gateway.enabled` is true |
| `GET /api/reconciliation/status` | Recent local/provider bill comparisons |
| `POST /api/reconciliation/import` | Import manual summary or provider CSV/JSON statement for local reconciliation |
| `GET /api/router/simulate?to_model=gpt-5-mini&ratio=0.5` | Simulate cost impact of model-routing changes without mutating the ledger |
| `GET /api/preflight/estimate?task=refactor&project=repo-name` | Estimate likely cost/tokens before starting an agent workload |
| `GET /api/chargeback` | Team/project/source/model showback using usage records joined with workload metadata |
| `GET /api/fleet-attribution` | Sub-agent, parent run, and parallel-run cost attribution |
| `GET /api/wrapped?period=monthly&format=markdown` | Monthly/weekly/yearly Agent Wrapped summary without prompt analysis |
| `POST /api/policy/evaluate` | Evaluate local advisory policy rules and optionally record decisions |
| `GET /api/policy/audit` | Audit historical usage, tool calls, and workloads against local policy rules |
| `GET /api/policy/enforcement` | Local policy enforcement evidence across decisions, approvals, and audit events |
| `GET /api/policy/approvals?status=pending&privacy=1` | List local pending, approved, rejected, or all policy approval requests; `privacy=1` hashes request/workload/run IDs and redacts project, target, approver, escalation, reason, payload, and decision note |
| `POST /api/policy/approvals` | Cast an approve/reject vote for a local policy approval request; supports `required_approvals` quorum and records local audit metadata without storing note text in audit params |
| `GET /api/policy/approval-routes?due_within=24h` | Summarize pending approval routes for local notification adapters |
| `GET /api/audit-log?action=pricing&role=operator` | Filter local operational audit events; supports `from`, `to`, `actor`, `role`, `action`, `target`, `limit`, and privacy mode |
| `GET /api/sessions` | Server-side paginated session ledger |
| `GET /api/session-replay?source=codex&session_id=...` | Chronological per-call token/cost replay for one session |
| `GET /api/badge/repo.svg?project=repo-name&metric=cost` | Local SVG repo cost/tokens/cache badge |
| `GET /api/model-registry` | Pricing and model governance registry |
| `GET /api/pricing/status` | Pricing freshness, source state, effective rule summary, unpriced models |
| `POST /api/pricing/sync` | Sync pricing |
| `POST /api/pricing/recalculate?mode=zero|all` | Recalculate costs |
| `POST /api/projections/repair` | Repair canonical `model_calls` to `usage_records` projection drift and rebuild aggregates |
| `GET /api/cost-intelligence` | Expensive session explanations |
| `GET /api/cache/doctor` | Cache hit/write/read diagnostics |
| `GET /api/doctor?format=markdown` | One-click local diagnostics for usage, ingestion, pricing, data quality, and workload state |
| `GET /api/data-quality` | Trust and completeness report |
| `GET /api/model-calls` | Calls by model/source/project |
| `GET /api/quota/status` | Local quota and burn-rate estimates |
| `GET /api/anomalies` | Robust-statistics anomaly events |
| `GET /api/watchdog/events` | Scoped runaway, call-density, cache-miss-risk, and non-working-hour watchdog events |
| `GET /api/evidence-bundle` | Redacted support/audit bundle with health, pricing, consistency, anomaly, and workload-state evidence |
| `GET /api/offline-bundle/export` | Export signed/hashed offline bundle |
| `POST /api/offline-bundle/import` | Import offline bundle canonical events |
| `GET /api/export?type=workloads&format=csv` | CSV/JSON exports |
| `GET /api/export?type=chargeback&format=csv` | Team showback CSV export |
| `GET /api/report?format=markdown` | Markdown report |

Manual scan, reset, pricing sync, imports, and recalculation require localhost access unless auth tokens are configured. Set `rbac.read_only: true` for observer deployments; Agent Ledger then rejects REST/CLI write operations, disables background scans, pricing sync, cost recalculation, and suppresses derived writebacks from GET endpoints.

When a policy returns `require_approval`, Agent Ledger records a local pending approval request and returns its id. Admins can approve or reject it through `POST /api/policy/approvals` or `agent-ledger policy resolve`; the default quorum is one approval, and `required_approvals` enables multi-actor local approval before the original operation can be retried with `approval_id=<id>` or `X-Agent-Ledger-Approval: <id>`. Approval reuse matches action/target and, when available, the original source, model, and project so a live gateway approval cannot be reused for a different model. Policy rules can also carry `approvers`, `escalate_after`, and `escalate_to`; these are stored as local routing metadata with `due_at` and `overdue` evidence for the dashboard, webhook summaries, and enforcement reports.

## MCP Tool Surface

`agent-ledger mcp` starts a local stdio JSON-RPC tool server for agent frameworks and wrappers. The implementation is intentionally local and privacy-preserving: tools can create or close workloads, link workload dependencies, acquire/renew/release workload leases, start runs on existing workloads, append run heartbeats, check run liveness and terminal-state snapshots, read the queue probe and cursor-stable workload event feed, record tool-call metadata, context refs, hashed artifacts, and quality/evaluation signals, ask for advisory policy decisions, list and vote on local approval requests, read approval route rollups, query local budget state, explain cost, and find similar workloads. Resources expose metadata-only schema, integration, budget, workload, queue, feed, terminal-state, and policy context; resource URIs support query parameters for scoped reads such as `agent-ledger://workloads/queue?source=codex&team=infra`, `agent-ledger://workloads/feed?severity=warning&source=codex&project=agent-ledger&limit=50`, `agent-ledger://policy/approvals?status=pending&privacy=1`, and `agent-ledger://policy/approval-routes?due_within=24h&privacy=1`. `resources/subscribe` watches the exact subscribed URI locally and emits `notifications/resources/updated` with the latest cursor/hash when that scoped resource changes. Prompts provide reusable workload/cost-review/evidence templates. It does not read prompt content and does not send data to a remote MCP host by itself. MCP, REST, and CLI policy evaluation share the same local evaluator so advisory decisions are consistent across integrations.

MCP `tools/list` includes standard-style `annotations.readOnlyHint` plus `_meta.agent_ledger` fields: `writes_local_state`, `write_mode` (`none`, `always`, or `conditional`), `available_in_read_only`, and `read_only_behavior`. Routers and multi-agent frameworks should use these fields before calling tools in observer deployments.

`GET /api/integrations`, `GET /.well-known/agent-ledger.json`, `agent-ledger integrations`, MCP `ledger.discovery`, MCP `ledger.integrations`, and `agent-ledger://discovery/manifest` expose runtime capability fields: `writes_local_state`, `available_in_read_only`, and `runtime_status`. The discovery manifest also exposes first-class `contract_bundle_uri`, `openapi_uri`, `capability_catalog_hash`, `runtime_status_uri`, `canonical_schema_uri`, `canonical_schema_hash`, `event_examples_uri`, `adapter_spec_uri`, `adapter_spec_hash`, and `adapter_conformance_uri` fields for lightweight wrappers. `GET /api/contracts`, `agent-ledger contracts`, MCP `ledger.contracts`, and `agent-ledger://contracts/bundle` expose a one-shot contract bundle with document URIs, hashes, cache semantics, CLI commands, and MCP entrypoints. `GET /api/contracts/verify`, `agent-ledger contracts verify`, MCP `ledger.contracts_verify`, and `agent-ledger://contracts/verification` expose a machine-readable self-check report that validates discovery, bundle, OpenAPI, schema, adapter, runtime, read-only, and privacy invariants. `GET /api/openapi.json`, `agent-ledger openapi`, MCP `ledger.openapi`, and `agent-ledger://contracts/openapi` expose a metadata-only OpenAPI 3.1 document for stable REST control-plane endpoints. `GET /api/integrations/adapter-spec`, `agent-ledger adapter spec`, MCP `ledger.adapter_contract`, and `agent-ledger://integrations/adapter-contract` expose the same machine-readable adapter contract. `GET /api/runtime/status` and `agent-ledger runtime` provide the same process-level observer/control-plane status for probes. `GET /api/config/status`, `agent-ledger config status`, MCP `ledger.config_status`, and `agent-ledger://config/status` provide a privacy-safe deployment configuration report for wrappers, CI, and operator checks. `GET /api/readiness`, `agent-ledger readiness`, MCP `ledger.readiness`, and `agent-ledger://readiness` provide a privacy-safe readiness report that summarizes database, config, runtime, contract, ingestion, and pricing evidence without leaking local data. `GET /api/admission/check`, `agent-ledger admission check`, MCP `ledger.admission_check`, and `agent-ledger://admission/check` let wrappers and routers dry-run HTTP, CLI, and MCP operations against the current role/read-only rules before attempting a mutation. The REST discovery, contract bundle, contract verification, OpenAPI, catalog, runtime status, config status, readiness, admission, adapter spec, and event schema endpoints emit strong `ETag` headers and honor `If-None-Match` with `304 Not Modified`, so wrappers can revalidate contracts without reparsing unchanged JSON. Agent routers and wrappers should use these fields instead of hardcoding endpoint assumptions, especially when `rbac.read_only` is enabled.

Current tools:

- `ledger.current_budget`
- `ledger.discovery`
- `ledger.contracts`
- `ledger.contracts_verify`
- `ledger.openapi`
- `ledger.runtime_status`
- `ledger.config_status`
- `ledger.readiness`
- `ledger.admission_check`
- `ledger.start_workload`
- `ledger.start_run`
- `ledger.close_workload`
- `ledger.link_workloads`
- `ledger.claim_next_workload`
- `ledger.workload_queue`
- `ledger.acquire_workload_lease`
- `ledger.renew_workload_lease`
- `ledger.release_workload_lease`
- `ledger.workload_leases`
- `ledger.heartbeat_run`
- `ledger.run_liveness`
- `ledger.workload_timeline`
- `ledger.workload_state`
- `ledger.workload_feed`
- `ledger.record_tool_call`
- `ledger.record_context`
- `ledger.record_artifact`
- `ledger.record_evaluation`
- `ledger.record_event`
- `ledger.validate_event`
- `ledger.event_schema`
- `ledger.event_examples`
- `ledger.adapter_contract`
- `ledger.adapter_conformance`
- `ledger.integrations`
- `ledger.get_policy`
- `ledger.policy_audit`
- `ledger.approval_routes`
- `ledger.approvals`
- `ledger.resolve_approval`
- `ledger.audit_log`
- `ledger.explain_cost`
- `ledger.find_similar_workloads`

Current resources:

- `agent-ledger://discovery/manifest`
- `agent-ledger://contracts/bundle`
- `agent-ledger://contracts/verification`
- `agent-ledger://contracts/openapi`
- `agent-ledger://schema/canonical-events`
- `agent-ledger://schema/canonical-event-examples`
- `agent-ledger://integrations/catalog`
- `agent-ledger://integrations/adapter-contract`
- `agent-ledger://runtime/status`
- `agent-ledger://config/status`
- `agent-ledger://readiness`
- `agent-ledger://admission/check`
- `agent-ledger://budget/current` with optional `window`, `source`, `model`, and `project` query parameters
- `agent-ledger://workloads/recent` with summary rows and derived terminal-state snapshots; supports `from`, `to`, `source`, `model`, `project`, `status`, `q`, `limit`, `offset`, and `stale_after`
- `agent-ledger://workloads/queue` with read-only queue claimability and lease pressure stats; supports `source`, `project`, `repo`, `team`, `owner`, `status`, and `q`
- `agent-ledger://workloads/feed` with cursor-stable workload state events for local monitors and routers; supports `from`, `to`, `source`, `model`, `project`, `phase`, `severity`, `limit`, and `stale_after`
- `agent-ledger://policies/status`
- `agent-ledger://policy/approvals` with local approval queue rows; supports `status`, `limit`, and `privacy`
- `agent-ledger://policy/approval-routes` with pending approval route rollups; supports `due_within`, `limit`, and `privacy`

Current prompts:

- `agent-ledger/workload-brief`
- `agent-ledger/cost-review`
- `agent-ledger/incident-evidence`

Canonical event ingest supports workload, workload-link, run, run-heartbeat, model-call, tool-call, context-ref, artifact, evaluation, and policy-decision events. Payloads are metadata-only; raw prompt/content keys are rejected instead of silently persisted. The current event-envelope contract accepts only `schema_version: "v1"`; unknown versions fail validation explicitly. `GET /api/event-examples`, `agent-ledger event examples`, and MCP `ledger.event_examples` return privacy-safe templates for every supported event type. `GET /api/integrations/adapter-spec`, `agent-ledger adapter spec`, MCP `ledger.adapter_contract`, and `agent-ledger://integrations/adapter-contract` expose a machine-readable adapter contract with supported input kinds, required envelope fields, forbidden payload keys, token semantics, quality gates, validation commands, and ingest entrypoints. `POST /api/events/validate` and `agent-ledger event validate` run the same contract checks without writing to SQLite, which is useful for direct canonical event checks. `POST /api/integrations/conformance` and `agent-ledger adapter conformance` also convert provider JSON, provider SSE, OpenTelemetry GenAI, and A2A fixtures before validation, so wrapper CI can prove compatibility before enabling ingest. The envelope also carries adapter provenance fields such as `schema_version`, `source_version`, `parser_version`, `raw_ref`, and `match_type`, so future collectors can explain whether an event was exact, estimated, reconstructed, source-reported, or fuzzy without storing prompt content. Canonical `model.call` events are also projected into `usage_records` so dashboard, budget, export, and preflight APIs use one token source where possible. `workload.linked` records dependency and lineage edges for async goal graphs without storing prompt content. `agent.run.heartbeat` appends liveness/progress rows and updates the run snapshot so long-running async agents can be monitored without reading prompts; liveness queries identify active runs whose heartbeat or start time is older than a configured threshold. `GET /api/integrations`, `agent-ledger integrations`, and `ledger.integrations` expose the current connector/protocol capability catalog without leaking local source paths. `POST /api/otel/genai` and `agent-ledger otel ingest` accept OpenTelemetry GenAI JSON spans and persist only selected metadata/token fields. When explicitly enabled, `POST /v1/traces` and `POST /api/otlp/v1/traces` accept OTLP HTTP JSON or HTTP protobuf trace batches with body and span-count limits; OTLP gRPC remains planned until receiver lifecycle and conformance tests are added. `POST /api/a2a/tasks` and `agent-ledger a2a ingest` accept A2A task snapshots/events and persist task lifecycle metadata while excluding message/history/artifact-part content. `POST /api/provider/calls` and `agent-ledger provider ingest` accept OpenAI-compatible, Anthropic-style, and LiteLLM-style usage envelopes while excluding request/response message content. When explicitly enabled, `POST /gateway/openai/v1/chat/completions` proxies OpenAI-compatible JSON or SSE streaming requests in memory, `POST /gateway/openai/v1/responses` proxies OpenAI Responses JSON or SSE streaming requests in memory, and `POST /gateway/anthropic/v1/messages` proxies Anthropic Messages JSON or SSE streaming requests in memory; all apply local policy checks, write usage/audit metadata, and expose metering state through response headers without storing prompt or response content. `POST /api/reconciliation/import` and `agent-ledger reconcile import` accept local provider CSV/JSON billing exports, store only summary totals, statement hash, window, and warnings, and compare them with the local ledger for the same window.

## Troubleshooting Data Accuracy

Start with the one-click doctor:

```bash
agent-ledger doctor --format markdown
```

Or open `GET /api/doctor?format=markdown&privacy=1`. The report checks the selected time range, collector health, path existence/readability, last scan errors, pricing freshness, unpriced models, empty usage windows, canonical-to-usage projection consistency, control idempotency health, workload lease health, and workload terminal-state issues such as stale runs or blocked policy decisions.

If Codex, OpenCode, or another source shows no data:

- Confirm the source is enabled and the configured path exists.
- Run `POST /api/scan?source=codex` or the UI Scan Source action.
- Check `GET /api/health/ingestion`; `last_error` is intentionally explicit.
- For Docker, mount only real agent directories. Docker creates missing host paths as root, which can break later agent writes.

If KPI totals and charts disagree:

- The web UI uses `GET /api/dashboard` for KPI, token, cost, and model panels so they are read from one storage window.
- The Data Quality panel surfaces dashboard consistency issues directly with metric, expected/actual delta, and severity.
- Run `POST /api/recalculate-costs?mode=zero` after pricing changes.
- If Doctor reports canonical-to-usage projection drift, run `agent-ledger projection repair` or `POST /api/projections/repair` with the same `from`/`to`/`source`/`model`/`project` scope. The repair is idempotent, backfills missing projected usage rows, realigns cache/cost metadata, and rebuilds aggregates.
- Run `agent-ledger doctor --format markdown` and inspect projection, dashboard consistency, or pricing warnings if a mismatch persists.

If costs differ from a provider invoice:

- Run `POST /api/pricing/sync`, then `POST /api/pricing/recalculate?mode=all` if you intentionally want all historical rows repriced.
- Use local pricing overrides for enterprise contract prices or third-party relay prices.
- Import the provider CSV/JSON statement through `POST /api/reconciliation/import` or `agent-ledger reconcile import`; reconciliation stores only totals, hashes, windows, and warnings.

## Security Model

- Binds to `127.0.0.1` by default.
- `agent-ledger config status`, `GET /api/config/status`, and MCP `ledger.config_status` are designed for deployment checks and never expose raw paths, auth tokens, API keys, webhook URLs, machine names, authors, prompts, responses, or session ids.
- `agent-ledger readiness`, `GET /api/readiness`, MCP `ledger.readiness`, and `agent-ledger://readiness` are designed for control-plane probes and expose only status, counts, check identifiers, and remediation hints, including privacy-safe control idempotency key/replay counts and workload lease counts.
- `agent-ledger admission check`, `GET /api/admission/check`, MCP `ledger.admission_check`, and `agent-ledger://admission/check` expose operation access decisions only; request bodies, full CLI arguments, raw paths, tokens, prompts, sessions, projects, branches, machine names, and authors are excluded.
- Reads local agent logs and databases; it does not upload usage data.
- Pricing sync is the expected outbound request.
- Manual operations are localhost-only by default.
- Optional RBAC supports `viewer`, `operator`, and `admin` tokens.
- `rbac.read_only: true` turns the process into an observer: POST/PUT-style write APIs are rejected, CLI/MCP write commands fail explicitly, automatic collectors and pricing sync do not run, and reports/exports/anomaly views do not append audit, budget, insight, or bundle records.
- Policy rules can match `global`, `source`, `model`, `project`, `repo`, `git_branch`, `team`, `action`, `target`, and `role`.
- MCP `ledger.get_policy` remains available for advisory reads in observer mode, but calls with `workload_id` would record a policy decision and are rejected as writes.
- Policy approval requests are local metadata records. They authorize only matching action/target retries and do not include prompt content.
- Approval queue privacy mode redacts approval routing metadata and payloads across REST, CLI, and MCP. Approval vote audit rows store quorum/vote facts plus `note_present`, not the note text.
- The optional provider gateway is disabled by default. It forwards prompt content only to the configured upstream in memory, reads API keys from environment variables, and stores usage metadata rather than message content.
- Run commands are stored as metadata, but common command-line secret patterns such as `API_KEY=...`, `--token ...`, `--api-key=...`, and `Bearer ...` are best-effort redacted before persistence. Prefer environment variables or a secret manager instead of durable command arguments.
- Privacy presets can hide paths, project names, branches, machine names, and session IDs.
- Webhooks are disabled by default and send only redacted workload-event, pending-approval, and approval-route summaries.
- Offline bundles are local JSON exports. Set `AGENT_LEDGER_BUNDLE_KEY` and pass `signed=1` / `--signed` to add an HMAC-SHA256 signature; use `verify=1` / `--verify` on import to require signature verification.

## Development

```bash
go test ./...
go vet ./...
node --check internal/server/static/app.js
docker compose up -d --build
```

On hosts without Go installed:

```bash
docker run --rm -v "$PWD:/src" -w /src golang:1.25.11-alpine sh -c "gofmt -w . && go test ./..."
```

## Release Governance

Releases use GoReleaser for platform archives and GitHub Actions for GHCR images. Release archives are configured to include Syft-generated SBOM files, while the Docker workflow publishes GHCR images with BuildKit SBOM attestations and `mode=max` provenance. See [RELEASE.md](RELEASE.md) for the release checklist and required artifact verification before announcing supply-chain claims.

## Roadmap

Implemented foundation: canonical workload schema, metadata-only canonical event ingest, machine-readable adapter contract, workload dependency/lineage links, retry-safe control operation idempotency for workload/run writes, short-lived workload leases for async execution claims, async run start/heartbeat/liveness ledger, derived workload terminal-state snapshots and local workload event feed/SSE stream, explicit workload evaluation signals, disabled-by-default redacted workload and approval webhook notifications, privacy-safe discovery manifest, contract bundle index, OpenAPI control-plane contract, runtime status probe, privacy-safe config status probe, control-plane readiness probe, operation admission dry-run, canonical-to-usage projection plus repair, OpenTelemetry GenAI JSON span mapping, optional local OTLP HTTP JSON/protobuf traces receiver, A2A task telemetry mapping, provider usage envelope mapping, optional local OpenAI-compatible Chat Completions JSON/SSE, OpenAI Responses JSON/SSE, and Anthropic Messages JSON/SSE gateway, provider bill reconciliation import, model router simulation, preflight cost estimates, session cost replay, repo cost badges, integration capability catalog, signed offline bundle export/import, legacy session backfill, workload API, workload CSV export, local policy approval requests, quorum-based approval votes, approval routing/escalation metadata, approval route summaries and enforcement evidence, CLI workload/event/policy/router/replay/badge/preflight/projection/config/readiness/admission commands, CLI run wrapper, and local MCP stdio tools/resources/resource-subscriptions/prompts.

Planned integrations: OTLP gRPC receiver conformance, provider-native gateway adapters, Postgres team mode, OIDC/SSO, native MCP subscription transport when host clients support it, and external approval notification adapters.

## License

Apache-2.0. See [LICENSE](LICENSE).
