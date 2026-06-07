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
- Provides local audit logs, privacy presets, exports, reports, evidence bundles, and team showback data.
- Runs as one Go binary with embedded static UI and SQLite.

## Quick Start

```bash
git clone https://github.com/zhenzhis/agent-ledger.git
cd agent-ledger
go build -o agent-ledger .
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
./agent-ledger workload create --goal "review strategy engine" --source codex --project quant
./agent-ledger run --goal "debug ingestion" --agent codex -- codex
./agent-ledger event schema
./agent-ledger event ingest --file event.json
./agent-ledger integrations
./agent-ledger otel convert --file spans.json
./agent-ledger otel ingest --file spans.json
./agent-ledger a2a convert --file task.json
./agent-ledger a2a ingest --file task.json
./agent-ledger provider convert --file response.json
./agent-ledger provider ingest --file response.json
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
./agent-ledger pricing sync
./agent-ledger wrapped
./agent-ledger mcp
```

## Configuration

Config search order:

1. `--config path/to/config.yaml`
2. `/etc/agent-ledger/config.yaml`
3. `./config.yaml`

Minimal example:

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
```

Use `pricing.overrides` for enterprise contracts, relay pricing, regional multipliers, or provider-specific discounts.

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

- `canonical_events`: normalized event stream for future collectors, MCP, A2A, and gateways.
- `workloads`, `agent_runs`, `model_calls`, `tool_calls`: goal/run/call ledger.
- `workload_sessions`: compatibility link from old source-scoped sessions to workloads.
- `artifacts`, `evaluations`, `policy_decisions`, `context_refs`: future-proof AgentOps records.
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
| `GET /api/dashboard` | Consistent KPI, token, cost, and model bundle for the web dashboard |
| `GET /api/stats` | Summary stats |
| `GET /api/workloads` | Server-side paginated workload ledger |
| `POST /api/workloads` | Create a local workload |
| `POST /api/workloads/close` | Close a workload with status/outcome |
| `GET /api/workload-detail` | Workload runs, model calls, tools, sessions, policies |
| `GET /api/workload-graph` | Compact workload graph |
| `GET /api/integrations` | Privacy-safe integration capability catalog |
| `GET /api/event-schema` | Canonical event schema and supported event types |
| `POST /api/events` | Ingest metadata-only canonical events |
| `POST /api/otel/genai` | Convert OpenTelemetry GenAI JSON spans into canonical model-call events |
| `POST /api/a2a/tasks` | Convert A2A JSON task snapshots/events into workload/run/artifact/evaluation events |
| `POST /api/provider/calls` | Convert provider response usage envelopes into canonical model-call events |
| `GET /api/reconciliation/status` | Recent local/provider bill comparisons |
| `POST /api/reconciliation/import` | Import manual summary or provider CSV/JSON statement for local reconciliation |
| `GET /api/router/simulate?to_model=gpt-5-mini&ratio=0.5` | Simulate cost impact of model-routing changes without mutating the ledger |
| `GET /api/preflight/estimate?task=refactor&project=repo-name` | Estimate likely cost/tokens before starting an agent workload |
| `GET /api/chargeback` | Team/project/source/model showback using raw usage first, canonical model calls as fallback |
| `GET /api/fleet-attribution` | Sub-agent, parent run, and parallel-run cost attribution |
| `GET /api/wrapped?period=monthly&format=markdown` | Monthly/weekly/yearly Agent Wrapped summary without prompt analysis |
| `POST /api/policy/evaluate` | Evaluate local advisory policy rules and optionally record decisions |
| `GET /api/sessions` | Server-side paginated session ledger |
| `GET /api/session-replay?source=codex&session_id=...` | Chronological per-call token/cost replay for one session |
| `GET /api/badge/repo.svg?project=repo-name&metric=cost` | Local SVG repo cost/tokens/cache badge |
| `GET /api/model-registry` | Pricing and model governance registry |
| `GET /api/pricing/status` | Pricing freshness, source state, unpriced models |
| `POST /api/pricing/sync` | Sync pricing |
| `POST /api/pricing/recalculate?mode=zero|all` | Recalculate costs |
| `GET /api/cost-intelligence` | Expensive session explanations |
| `GET /api/cache/doctor` | Cache hit/write/read diagnostics |
| `GET /api/doctor?format=markdown` | One-click local diagnostics for usage, ingestion, pricing, and data quality |
| `GET /api/data-quality` | Trust and completeness report |
| `GET /api/model-calls` | Calls by model/source/project |
| `GET /api/quota/status` | Local quota and burn-rate estimates |
| `GET /api/anomalies` | Robust-statistics anomaly events |
| `GET /api/evidence-bundle` | Redacted support/audit bundle |
| `GET /api/offline-bundle/export` | Export signed/hashed offline bundle |
| `POST /api/offline-bundle/import` | Import offline bundle canonical events |
| `GET /api/export?type=workloads&format=csv` | CSV/JSON exports |
| `GET /api/export?type=chargeback&format=csv` | Team showback CSV export |
| `GET /api/report?format=markdown` | Markdown report |

Manual scan, reset, pricing sync, imports, and recalculation require localhost access unless auth tokens are configured.

## MCP Tool Surface

`agent-ledger mcp` starts a local stdio JSON-RPC tool server for agent frameworks and wrappers. The first implementation is intentionally local and privacy-preserving: tools can create or close workloads, record hashed artifacts, ask for advisory policy decisions, query local budget state, explain cost, and find similar workloads. It does not read prompt content and does not send data to a remote MCP host by itself. MCP, REST, and CLI policy evaluation share the same local evaluator so advisory decisions are consistent across integrations.

Current tools:

- `ledger.current_budget`
- `ledger.start_workload`
- `ledger.close_workload`
- `ledger.record_artifact`
- `ledger.record_event`
- `ledger.event_schema`
- `ledger.integrations`
- `ledger.get_policy`
- `ledger.explain_cost`
- `ledger.find_similar_workloads`

Canonical event ingest supports workload, run, model-call, tool-call, context-ref, artifact, evaluation, and policy-decision events. Payloads are metadata-only; raw prompt/content keys are rejected instead of silently persisted. `GET /api/integrations`, `agent-ledger integrations`, and `ledger.integrations` expose the current connector/protocol capability catalog without leaking local source paths. `POST /api/otel/genai` and `agent-ledger otel ingest` accept OpenTelemetry GenAI JSON spans and persist only selected metadata/token fields. `POST /api/a2a/tasks` and `agent-ledger a2a ingest` accept A2A task snapshots/events and persist task lifecycle metadata while excluding message/history/artifact-part content. `POST /api/provider/calls` and `agent-ledger provider ingest` accept OpenAI-compatible, Anthropic-style, and LiteLLM-style usage envelopes while excluding request/response message content. `POST /api/reconciliation/import` and `agent-ledger reconcile import` accept local provider CSV/JSON billing exports, store only summary totals, statement hash, window, and warnings, and compare them with the local ledger for the same window.

## Troubleshooting Data Accuracy

Start with the one-click doctor:

```bash
agent-ledger doctor --format markdown
```

Or open `GET /api/doctor?format=markdown&privacy=1`. The report checks the selected time range, collector health, path existence/readability, last scan errors, pricing freshness, unpriced models, and empty usage windows.

If Codex, OpenCode, or another source shows no data:

- Confirm the source is enabled and the configured path exists.
- Run `POST /api/scan?source=codex` or the UI Scan Source action.
- Check `GET /api/health/ingestion`; `last_error` is intentionally explicit.
- For Docker, mount only real agent directories. Docker creates missing host paths as root, which can break later agent writes.

If KPI totals and charts disagree:

- The web UI uses `GET /api/dashboard` for KPI, token, cost, and model panels so they are read from one storage window.
- Run `POST /api/recalculate-costs?mode=zero` after pricing changes.
- Run `agent-ledger doctor --format markdown` and inspect `dashboard consistency` or pricing warnings if a mismatch persists.

If costs differ from a provider invoice:

- Run `POST /api/pricing/sync`, then `POST /api/pricing/recalculate?mode=all` if you intentionally want all historical rows repriced.
- Use local pricing overrides for enterprise contract prices or third-party relay prices.
- Import the provider CSV/JSON statement through `POST /api/reconciliation/import` or `agent-ledger reconcile import`; reconciliation stores only totals, hashes, windows, and warnings.

## Security Model

- Binds to `127.0.0.1` by default.
- Reads local agent logs and databases; it does not upload usage data.
- Pricing sync is the expected outbound request.
- Manual operations are localhost-only by default.
- Optional RBAC supports `viewer`, `operator`, and `admin` tokens.
- Privacy presets can hide paths, project names, branches, machine names, and session IDs.
- Webhooks are disabled by default and should only send redacted summaries.
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

## Roadmap

Implemented foundation: canonical workload schema, metadata-only canonical event ingest, OpenTelemetry GenAI JSON span mapping, A2A task telemetry mapping, provider usage envelope mapping, provider bill reconciliation import, model router simulation, preflight cost estimates, session cost replay, repo cost badges, integration capability catalog, signed offline bundle export/import, legacy session backfill, workload API, workload CSV export, CLI workload/event/policy/router/replay/badge/preflight commands, CLI run wrapper, and local MCP stdio tools.

Planned integrations: full OTLP receiver mode, live provider/API gateway mode, Postgres team mode, OIDC/SSO, richer MCP resources/prompts, and enterprise policy approval flows.

## License

Apache-2.0. See [LICENSE](LICENSE).
