# Changelog

## Unreleased

### Added

- Official Agent Ledger naming across module path, binary, Docker, release metadata, and documentation.
- Pricing governance with local override, official OpenAI/Anthropic seed rows, LiteLLM fallback, pricing source health, snapshots, audit events, and per-record pricing confidence.
- Cost Intelligence, Cache Doctor, Data Quality Center, Model Call Analytics, Quota Status, Watchdog events, evidence bundles, reconciliation imports, audit log, policy status, and expanded export types.
- Canonical Workload Ledger foundation with `canonical_events`, `workloads`, `agent_runs`, `model_calls`, `tool_calls`, `context_refs`, `artifacts`, `evaluations`, and `policy_decisions`.
- Legacy session backfill into workload/run/model-call records for immediate compatibility with existing local data.
- Workload APIs: list/create/close/detail/graph, model registry, policy decisions, and workload CSV/JSON export.
- Local MCP stdio JSON-RPC tools for budget lookup, workload lifecycle, privacy-safe artifacts, advisory policy decisions, cost explanation, and similar workload search.
- Hourly and daily usage aggregate tables with dashboard aggregate fallback.
- CLI commands: `today`, `top`, `doctor`, `battery`, `workload list/create/show/close`, `run --goal ... -- <command>`, `export`, `pricing sync`, and `wrapped`.
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
