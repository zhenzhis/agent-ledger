# Changelog

## Unreleased

### Added

- Official Agent Ledger naming across module path, binary, Docker, release metadata, and documentation.
- Pricing governance with local override, official OpenAI/Anthropic seed rows, LiteLLM fallback, pricing source health, snapshots, audit events, and per-record pricing confidence.
- Cost Intelligence, Cache Doctor, Data Quality Center, Model Call Analytics, Quota Status, Watchdog events, evidence bundles, reconciliation imports, audit log, policy status, and expanded export types.
- Hourly and daily usage aggregate tables with dashboard aggregate fallback.
- CLI commands: `today`, `top`, `doctor`, `battery`, `export`, `pricing sync`, and `wrapped`.
- Cursor-compatible session pagination via `next_cursor`.
- Black/white/gray data-dense dashboard panels for pricing, quota, quality, model calls, cache, watchdog, and cost intelligence.

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
