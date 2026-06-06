# agent-usage

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)]()
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-blue?logo=docker)](https://ghcr.io/zhenzhis/agent-usage)

Private local AI coding agent usage, cost, budget, and health console for Claude Code, Codex, OpenCode, Claude-compatible agents, and related local coding tools.

Single binary, SQLite storage, embedded web UI, localhost-first deployment.

[中文文档](README_CN.md)

## Fork Notice

This repository is a second-development fork by ZhenZhi based on [briqt/agent-usage](https://github.com/briqt/agent-usage).

We keep the upstream collection and pricing model compatible, and add source-scoped accounting, local deployment hardening, ingestion health, local budgets, export/report APIs, privacy mode, server-side pagination, and a monochrome operations dashboard.

Thanks to the original author and contributors of [briqt/agent-usage](https://github.com/briqt/agent-usage/) for the clean local-first foundation.

![Dashboard](docs/dashboard.png)

Screenshot is captured with privacy mode enabled. Local paths, project names, branches, and session identifiers are redacted.

## Features

- Local collectors for Claude Code, Codex CLI, OpenCode, OpenClaw, kiro, and Pi.
- SQLite database with source-scoped session identity: `(source, session_id)`.
- Incremental scanning with file offsets and parser context recovery.
- Cost calculation from litellm model pricing with local backfill.
- Ingestion health for each source: path status, last scan, duration, watermark, inserted rows, and errors.
- Local budgets by day, week, or month for global, source, model, or project scopes.
- CSV/JSON export and Markdown daily/weekly reports.
- Privacy mode for screenshots and shared reports.
- Server-side session pagination for large local databases.
- Docker compose defaults to `127.0.0.1:9800` and read-only session mounts.

## Quick Start

```bash
mkdir -p ./data
docker compose up --build -d
```

Open:

```bash
http://127.0.0.1:9800
```

Default Docker mounts:

| Source | Host path | Container path |
| --- | --- | --- |
| Claude Code | `~/.claude/projects` | `/sessions/claude` |
| Codex CLI | `~/.codex/sessions` | `/sessions/codex` |
| OpenCode | `~/.local/share/opencode` | `/sessions/opencode` |

The mounts are read-only. Missing paths fail explicitly instead of being created by Docker.

## Configuration

Minimal example:

```yaml
server:
  port: 9800
  bind_address: "127.0.0.1"
  # auth_token: "change-me"

collectors:
  claude:
    enabled: true
    paths: ["~/.claude/projects"]
    scan_interval: 60s
  codex:
    enabled: true
    paths: ["~/.codex/sessions"]
    scan_interval: 30s
  opencode:
    enabled: true
    paths: ["~/.local/share/opencode/opencode.db"]
    scan_interval: 30s

storage:
  path: "./agent-usage.db"

pricing:
  sync_interval: 1h

privacy:
  redact_paths: false
  hash_session_ids: false
  hide_project_names: false
  screenshot_mode: false

projects:
  aliases: {}
  exclude: []

budgets:
  enabled: false
  rules: []
```

Config search order:

1. `--config`
2. `/etc/agent-usage/config.yaml`
3. `./config.yaml`

## Data Accuracy

The storage model uses non-overlapping token components:

```text
input_tokens                 non-cached input
cache_read_input_tokens      cached input read
cache_creation_input_tokens  cached input written
output_tokens                output tokens
reasoning_output_tokens      reasoning output subset, informational only
```

Total tokens:

```text
total_tokens = input_tokens
             + cache_read_input_tokens
             + cache_creation_input_tokens
             + output_tokens
```

Collectors normalize source-specific formats before writing to SQLite. If a source reports input as total input including cache, the collector subtracts cache tokens first so the stored fields remain non-overlapping.

Deduplication is source-scoped:

```text
usage_records: (source, session_id, model, timestamp, input_tokens, output_tokens)
sessions:      (source, session_id)
prompt_events: (source, session_id, timestamp)
```

Time filters use half-open intervals internally:

```text
[from, to_next_day)
```

## Pricing And Cost Calculation

Pricing is fetched from the same source used by the upstream project:

[BerriAI/litellm model_prices_and_context_window.json](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json)

The fetcher stores:

- `input_cost_per_token`
- `output_cost_per_token`
- `cache_read_input_token_cost`
- `cache_creation_input_token_cost`

The runtime cost formula is:

```text
cost = input_tokens * input_price
     + cache_creation_input_tokens * cache_creation_price
     + cache_read_input_tokens * cache_read_price
     + output_tokens * output_price
```

This formula matches the upstream implementation. The ZhenZhi fork only hardens the pricing fetch with HTTP status validation, a User-Agent, a 30 second timeout, and an 8 MiB response limit.

Costs are recalculated for zero-cost rows after pricing sync. OpenCode source-reported costs are preserved when present.

## API

Read endpoints accept `from`, `to`, `source`, `model`, `project`, and `privacy=1`. Time-series endpoints also accept `granularity`.

| Endpoint | Description |
| --- | --- |
| `GET /api/stats` | Summary totals |
| `GET /api/cost-by-model` | Cost grouped by model |
| `GET /api/cost-over-time` | Cost time series |
| `GET /api/tokens-over-time` | Token time series |
| `GET /api/sessions?limit=100&offset=0` | Paginated sessions |
| `GET /api/session-detail?source=codex&session_id=ID` | Per-session model breakdown |
| `GET /api/health/ingestion` | Collector health |
| `GET /api/budgets/status` | Budget status |
| `GET /api/export?type=sessions&format=csv` | CSV/JSON export |
| `GET /api/report?period=daily&format=markdown` | Markdown report |
| `POST /api/scan?source=codex` | Manual scan |
| `POST /api/scan?source=codex&reset=true` | Clear one source and rescan |
| `POST /api/recalculate-costs` | Rebuild zero-cost records |

Manual scan and reset endpoints require localhost access unless `server.auth_token` is configured.

## Supported Sources

| Source | Location | Format |
| --- | --- | --- |
| Claude Code | `~/.claude/projects/<project>/<session>.jsonl` | JSONL |
| Codex CLI | `~/.codex/sessions/<year>/<month>/<day>/<session>.jsonl` | JSONL |
| OpenCode | `~/.local/share/opencode/opencode.db` | SQLite |
| OpenClaw | `~/.openclaw/agents/<agentId>/sessions/<sessionId>.jsonl` | JSONL |
| kiro | `~/.local/share/kiro-cli/data.sqlite3` and `~/.kiro/sessions/cli` | SQLite/JSON |
| Pi | `~/.pi/agent/sessions/<workspace>/<session>.jsonl` | JSONL |

## Build

```bash
git clone https://github.com/zhenzhis/agent-usage.git
cd agent-usage
go build -o agent-usage .
./agent-usage
```

Docker:

```bash
docker build -t agent-usage:local .
docker run --rm -p 127.0.0.1:9800:9800 agent-usage:local
```

For GHCR-based deployments, see `docker-compose.example.yml`. SBOM and provenance publication are planned for the release workflow and should not be claimed until enabled.

## Verification

Recommended checks before release:

```bash
go test ./...
go vet ./...
govulncheck ./...
docker build -t agent-usage:local .
```

The current CI and local verification cover storage migrations, source-scoped identity, pagination, collector fixtures, OpenCode prompt deduplication, pricing matching, and cost formula behavior.

## Security Model

- Local-first by default.
- Docker examples bind to localhost.
- Session mounts are read-only.
- No usage data is uploaded by the application.
- Pricing sync is the expected outbound request.
- Static frontend assets are embedded and do not depend on runtime CDNs.
- Optional bearer token protects API access when configured.

## License

[Apache 2.0](LICENSE)
