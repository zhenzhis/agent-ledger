# Contributing To Agent Ledger

Agent Ledger is a local-first AI Agent FinOps and audit console. Contributions should preserve privacy, explicit failures, and verifiable cost accounting.

## Project Origin

This project is an independent ZhenZhi second-development fork of [briqt/agent-usage](https://github.com/briqt/agent-usage). Keep upstream credit intact when editing docs, release notes, or package metadata.

## Development Setup

```bash
git clone https://github.com/zhenzhis/agent-ledger.git
cd agent-ledger
go build -o agent-ledger .
./agent-ledger
```

Docker:

```bash
docker compose up -d --build
```

## Quality Gate

Run these before submitting changes:

```bash
go test ./...
go vet ./...
node --check internal/server/static/app.js
git diff --check
```

If Go is not installed locally:

```bash
docker run --rm -v "$PWD:/src" -w /src golang:1.25.11-alpine sh -c "gofmt -w . && go test ./... && go vet ./..."
```

## Engineering Rules

- Keep collector changes source-scoped and covered by fixtures.
- New collectors, wrappers, MCP/A2A bridges, or gateways should normalize into canonical workload events before adding product-specific analytics.
- MCP tools must remain local-first, avoid prompt-content access, return explicit JSON-RPC errors, and add tests under `internal/mcp`.
- Preserve compatibility for existing `usage_records`, `sessions`, and `/api/sessions`; Workload Ledger is an additive layer, not a breaking replacement.
- Do not read, store, or analyze prompt content for insights.
- Do not hide pricing failures. Mark records as `unpriced`, `stale`, `fallback`, `fuzzy`, or `source-reported`.
- Do not add default webhooks, telemetry, or cloud sync.
- Preserve localhost-first deployment defaults.
- Dangerous operations must be explicit and auditable.
- UI changes must remain black/white/gray, responsive, keyboard-accessible, and free of horizontal overflow.

## Commit Style

Use conventional commits:

- `feat:`
- `fix:`
- `perf:`
- `refactor:`
- `docs:`
- `test:`
- `chore:`

## Pull Request Checklist

- Tests added or updated for behavior changes.
- Documentation updated for public APIs, config, or deployment changes.
- Screenshots use synthetic or privacy-safe data.
- No secrets, private paths, raw prompts, or durable sensitive logs.
- Manual verification notes are included when tests cannot cover the change.
