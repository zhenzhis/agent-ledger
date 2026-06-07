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
- `ingestion_health`, `insight_events`, `audit_log`: operations and quality evidence.

## API Surface

Common filters: `from`, `to`, `source`, `model`, `project`, `privacy`.

| Endpoint | Purpose |
|---|---|
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
| `POST /api/policy/evaluate` | Evaluate local advisory policy rules and optionally record decisions |
| `GET /api/sessions` | Server-side paginated session ledger |
| `GET /api/model-registry` | Pricing and model governance registry |
| `GET /api/pricing/status` | Pricing freshness, source state, unpriced models |
| `POST /api/pricing/sync` | Sync pricing |
| `POST /api/pricing/recalculate?mode=zero|all` | Recalculate costs |
| `GET /api/cost-intelligence` | Expensive session explanations |
| `GET /api/cache/doctor` | Cache hit/write/read diagnostics |
| `GET /api/data-quality` | Trust and completeness report |
| `GET /api/model-calls` | Calls by model/source/project |
| `GET /api/quota/status` | Local quota and burn-rate estimates |
| `GET /api/anomalies` | Robust-statistics anomaly events |
| `GET /api/evidence-bundle` | Redacted support/audit bundle |
| `GET /api/offline-bundle/export` | Export signed/hashed offline bundle |
| `POST /api/offline-bundle/import` | Import offline bundle canonical events |
| `GET /api/export?type=workloads&format=csv` | CSV/JSON exports |
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

Canonical event ingest supports workload, run, model-call, tool-call, context-ref, artifact, evaluation, and policy-decision events. Payloads are metadata-only; raw prompt/content keys are rejected instead of silently persisted. `GET /api/integrations`, `agent-ledger integrations`, and `ledger.integrations` expose the current connector/protocol capability catalog without leaking local source paths. `POST /api/otel/genai` and `agent-ledger otel ingest` accept OpenTelemetry GenAI JSON spans and persist only selected metadata/token fields.

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

Implemented foundation: canonical workload schema, metadata-only canonical event ingest, OpenTelemetry GenAI JSON span mapping, integration capability catalog, signed offline bundle export/import, legacy session backfill, workload API, workload CSV export, CLI workload/event/policy commands, CLI run wrapper, and local MCP stdio tools.

Planned integrations: A2A task telemetry, full OTLP receiver mode, optional provider/API gateway, Postgres team mode, OIDC/SSO, richer MCP resources/prompts, and enterprise policy approval flows.

## License

Apache-2.0. See [LICENSE](LICENSE).
