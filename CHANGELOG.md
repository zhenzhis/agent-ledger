# Changelog

## Unreleased

### Added

- Official Agent Ledger naming across module path, binary, Docker, release metadata, and documentation.
- Pricing governance with local override, official OpenAI/Anthropic seed rows, LiteLLM fallback, pricing source health, snapshots, audit events, and per-record pricing confidence.
- Cost Intelligence, Cache Doctor, Data Quality Center, Model Call Analytics, Quota Status, Watchdog events, evidence bundles, reconciliation imports, audit log, policy status, and expanded export types.
- Canonical Workload Ledger foundation with `canonical_events`, `workloads`, `agent_runs`, `model_calls`, `tool_calls`, `context_refs`, `artifacts`, `evaluations`, and `policy_decisions`.
- Metadata-only canonical event schema and ingest through storage, `GET /api/event-schema`, `POST /api/events`, `agent-ledger event schema/ingest`, and `ledger.event_schema` / `ledger.record_event`.
- Privacy-safe integration capability catalog through `GET /api/integrations`, `agent-ledger integrations`, and `ledger.integrations`, covering implemented collectors/protocols plus planned provider gateway surfaces.
- OpenTelemetry GenAI JSON span mapping through `POST /api/otel/genai` and `agent-ledger otel convert|ingest`, projecting token metadata into canonical `model.call` plus hashed `context.ref` events while excluding prompt/completion message attributes.
- A2A task telemetry mapping through `POST /api/a2a/tasks` and `agent-ledger a2a convert|ingest`, projecting task snapshots/events into workload, run, context, artifact, close, and evaluation events while excluding message/history/artifact-part content.
- Provider usage envelope mapping through `POST /api/provider/calls` and `agent-ledger provider convert|ingest`, supporting OpenAI-compatible, Anthropic-style, and LiteLLM-style usage objects while excluding request/response message content.
- Provider bill reconciliation import through `POST /api/reconciliation/import` and `agent-ledger reconcile import/status`, parsing local CSV/JSON statements into summary totals, statement hash, window, warnings, and local/provider cost diff.
- Model Router Simulator through `GET /api/router/simulate` and `agent-ledger router simulate`, estimating target-model routing savings from existing token components and pricing governance metadata without mutating ledger records.
- Offline bundle export/import for air-gapped multi-machine aggregation, with metadata-only canonical event replay, payload SHA-256 integrity, optional HMAC-SHA256 signing, CLI commands, and REST endpoints.
- Shared local policy evaluator across MCP, `POST /api/policy/evaluate`, and `agent-ledger policy evaluate`, with stable action severity and optional decision recording.
- Legacy session backfill into workload/run/model-call records for immediate compatibility with existing local data.
- Workload APIs: list/create/close/detail/graph, model registry, policy decisions, and workload CSV/JSON export.
- Local MCP stdio JSON-RPC tools for budget lookup, workload lifecycle, privacy-safe artifacts, advisory policy decisions, cost explanation, and similar workload search.
- Hourly and daily usage aggregate tables with dashboard aggregate fallback.
- CLI commands: `today`, `top`, `doctor`, `battery`, `workload list/create/show/close`, `run --goal ... -- <command>`, `export`, `pricing sync`, `router simulate`, and `wrapped`.
- Cursor-compatible session pagination via `next_cursor`.
- Black/white/gray data-dense dashboard panels for workloads, pricing, quota, quality, model calls, cache, watchdog, and cost intelligence.

### Changed

- Default database name is `agent-ledger.db`.
- Default system config path is `/etc/agent-ledger/config.yaml`.
- Docker runtime binary is `/agent-ledger`.
- Go module path is `github.com/zhenzhis/agent-ledger`.

### Security

- Added RBAC configuration fields and role checks for side-effectful governance APIs.
- Added local audit logging for scan, pricing sync, recalculation, and reconciliation import operations.
- Exports can be forced into privacy mode by policy.

### Credits

- Agent Ledger remains based on the local-first foundation from [briqt/agent-usage](https://github.com/briqt/agent-usage). Thanks to the original author and contributors.
