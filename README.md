# agent-usage

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-blue)]()
[![Docker](https://img.shields.io/badge/Docker-ghcr.io-blue?logo=docker)](https://ghcr.io/zhenzhis/agent-usage)

Private local AI coding agent usage, cost, budget, and health console for teams that want token accounting without external infrastructure.

Single binary + SQLite — local-first, auditable, and safe to run behind a localhost-only dashboard.

**[中文文档](README_CN.md)**

Collects local session data from Claude Code, Codex, OpenClaw, OpenCode, kiro, and Pi, calculates costs automatically, surfaces ingestion health, budget status, exports, and session details through a monochrome operations dashboard.

## Fork Notice

This repository is a second-development fork by **ZhenZhi** based on [briqt/agent-usage](https://github.com/briqt/agent-usage). We keep the core collection model close to the upstream project, and extend it with safer local deployment defaults, source-scoped accounting, ingestion health, local budgets, export/report APIs, privacy mode, pinned CI, vendored frontend assets, and a monochrome operations dashboard.

Thanks to the original author and contributors of [briqt/agent-usage](https://github.com/briqt/agent-usage/) for the clean single-binary foundation.

![Dashboard](docs/dashboard.png)

Screenshot is captured with privacy mode enabled; local paths, project names, branches, and session identifiers are redacted.

## Features

- 📁 **Local file parsing** — reads Claude Code, Codex CLI, OpenClaw, Pi session files, OpenCode SQLite database, and kiro session/database files directly
- 💰 **Automatic cost calculation** — fetches model pricing from [litellm](https://github.com/BerriAI/litellm), supports backfill when prices update
- 🗄️ **SQLite storage** — single file, zero ops, data is correctable
- 📊 **Web dashboard** — dark-themed UI with ECharts: cost breakdown, token trends, session list
- 🔄 **Incremental scanning** — watches for new sessions, deduplicates automatically
- 🧭 **Ingestion health** — source path checks, last scan time, duration, watermark, inserted row counts, and last error
- 🚦 **Local budgets** — day/week/month thresholds by global/source/model/project for tokens, prompts, or cost
- 📤 **Exports & reports** — CSV/JSON exports and Markdown daily/weekly reports using the active filters
- 🔒 **Privacy mode** — hide paths, hash session IDs, and redact project/branch names for screenshots
- 📦 **Single binary** — `go:embed` packs the web UI into the executable
- 🖥️ **Cross-platform** — Linux, macOS, Windows

## ZhenZhi Edition Optimizations

- **Local-only Docker default** — compose publishes `127.0.0.1:9800`, builds the local checkout, and mounts Claude/Codex/OpenCode read-only by default.
- **HTTP hardening** — explicit server timeouts plus CSP, frame, content-type, referrer, and permissions headers.
- **Frontend supply-chain reduction** — ECharts is vendored into the embedded static bundle; no CDN scripts or Google Fonts are loaded at runtime.
- **Scanner integrity** — JSONL collectors check scanner errors before inserting records or advancing file offsets, preventing silent truncation on oversized or unreadable lines.
- **OpenCode source cost preservation** — OpenCode-reported per-message costs are stored directly before pricing backfill, so custom providers such as GLM or DeepSeek do not appear as `$0`.
- **Source-scoped identity** — usage, session, and prompt deduplication now keys on `(source, session_id)` to avoid cross-agent collisions.
- **Operational controls** — manual scan, cost rebuild, source reset/rescan, ingestion health, budget status, export, report, and privacy mode.
- **Server-side pagination** — the session ledger fetches bounded pages instead of loading the full database into the browser.
- **Bounded pricing sync** — litellm pricing fetch checks HTTP status, uses a User-Agent, and caps response size.
- **Pinned automation** — release/docker actions are pinned by SHA; CI runs tests, vet, and `govulncheck@v1.3.0`.
- **Monochrome operations UI** — black/white/gray, dense, operation-oriented dashboard with activity matrix, token throughput, model allocation, cost trend, and a sortable session ledger.

## Quick Start (Docker)

```bash
# One command to start
mkdir -p ./data && docker compose up --build -d

# Open dashboard
open http://localhost:9800
```

The default `docker-compose.yml` builds the local checkout and publishes the dashboard only on `127.0.0.1:9800`. Keep that localhost binding unless you add your own reverse proxy or authentication layer. It mounts Claude, Codex, and OpenCode session data read-only by default:

- `~/.claude/projects` → `/sessions/claude`
- `~/.codex/sessions` → `/sessions/codex`
- `~/.local/share/opencode` → `/sessions/opencode`

These mounts use `create_host_path: false`, so missing host paths fail explicitly instead of being silently created by Docker. To collect OpenClaw, kiro, or Pi usage, uncomment the matching read-only volume in `docker-compose.yml` and set that collector to `enabled: true` in `config.docker.yaml` or your custom config. Data persists in `./data/`.

> **Note:** Only enable mounts for agents you actually use. Docker creates missing host directories as root, which can interfere with tools like `npx skills add` that detect installed agents by directory existence.

The container uses `config.docker.yaml` by default (binds to `0.0.0.0` inside the container, stores data in `/data/`). Host exposure is controlled by the compose port binding above. To override, mount your own config:

```yaml
# In docker-compose.yml, uncomment:
volumes:
  - ./config.yaml:/etc/agent-usage/config.yaml:ro
```

See [Docker Details](#docker-details) for UID/GID permissions and local builds.

## Query Usage from Agent Conversations

The skill works standalone — no need to install or run the agent-usage server. It parses local JSONL session files directly. If the agent-usage server is detected, it automatically switches to API queries for more accurate cost data.

```bash
# Installed via vercel-labs/skills, supports Claude Code, Cursor, kiro, and 40+ agents
npx skills add zhenzhis/agent-usage -y
```

Once installed, try: `查下 agent usage`、`agent usage 统计` or `check agent usage`. See [`skills/agent-usage/SKILL.md`](skills/agent-usage/SKILL.md) for details.

## Configuration

```yaml
server:
  port: 9800
  bind_address: "127.0.0.1"  # use "0.0.0.0" for remote access
  # auth_token: "change-me"  # optional bearer token for API access

collectors:
  claude:
    enabled: true
    paths:
      - "~/.claude/projects"
    scan_interval: 60s
  codex:
    enabled: true
    paths:
      - "~/.codex/sessions"
    scan_interval: 60s
  openclaw:
    enabled: true
    paths:
      - "~/.openclaw/agents"
    scan_interval: 60s
  opencode:
    enabled: true
    paths:
      - "~/.local/share/opencode/opencode.db"
    scan_interval: 60s
  kiro:
    enabled: true
    paths:
      - "~/.local/share/kiro-cli/data.sqlite3"
    scan_interval: 60s

storage:
  path: "./agent-usage.db"

pricing:
  sync_interval: 1h  # fetched from GitHub; set HTTPS_PROXY env var if this fails

privacy:
  redact_paths: false
  hash_session_ids: false
  hide_project_names: false
  screenshot_mode: false

projects:
  aliases:
    # "/Users/me/work/agent-usage": "agent-usage"
  exclude:
    # - "/tmp"

budgets:
  enabled: false
  rules:
    # - name: daily-global-cost
    #   period: day       # day | week | month
    #   scope: global     # global | source | model | project
    #   metric: cost_usd  # cost_usd | tokens | prompts
    #   limit: 25
    #   warn_ratio: 0.8
```

Config search order: `--config` flag > `/etc/agent-usage/config.yaml` > `./config.yaml`.

## Build from Source

```bash
# Clone
git clone https://github.com/zhenzhis/agent-usage.git
cd agent-usage

# Build
go build -o agent-usage .

# Edit config
cp config.yaml config.local.yaml
# Adjust paths if needed

# Run
./agent-usage

# Open dashboard
open http://localhost:9800
```

## Supported Data Sources

| Source | Session Location | Format |
|--------|-----------------|--------|
| [Claude Code](https://docs.anthropic.com/en/docs/claude-code) | `~/.claude/projects/<project>/<session>.jsonl` | JSONL |
| [Codex CLI](https://github.com/openai/codex) | `~/.codex/sessions/<year>/<month>/<day>/<session>.jsonl` | JSONL |
| [OpenClaw](https://github.com/openclaw/openclaw) | `~/.openclaw/agents/<agentId>/sessions/<sessionId>.jsonl` | JSONL |
| [OpenCode](https://github.com/anomalyco/opencode) | `~/.local/share/opencode/opencode.db` | SQLite |
| [kiro](https://kiro.dev) | `~/.local/share/kiro-cli/data.sqlite3` | SQLite |
| [Pi](https://pi.dev) | `~/.pi/agent/sessions/<workspace>/<session>.jsonl` | JSONL |

### Adding New Sources

Each source needs a collector that:
1. Scans session directories for JSONL files
2. Parses entries and extracts token usage per API call
3. Writes records to SQLite via the storage layer

See `internal/collector/claude.go` as a reference implementation.

## Dashboard

The web dashboard provides:

- **Control surface** — time presets, date range, granularity, source/model filters, theme, language, and auto-refresh
- **Control actions** — refresh, scan now, rebuild costs, reset/rescan current source, CSV export, Markdown report, and privacy mode
- **KPI strip** — total tokens, cost, sessions, prompts, budget state, health state, calls, cache rate, and per-call metrics
- **Ingestion health** — configured paths, readability, last scan, duration, watermark, inserted rows, and errors
- **Budget status** — local day/week/month thresholds with warning/critical states
- **Activity matrix** — commit-heatmap inspired token activity by input/output/cache channel
- **Token throughput** — stacked token bars for input, output, cache read, and cache write
- **Cost trend** — stacked cost bars by model with stable grayscale series
- **Model allocation** — horizontal ranking for top model spend
- **Session ledger** — sortable, filterable table with expandable per-model detail
- **Dark/Light theme** — monochrome dark-first default with manual toggle
- **i18n** — English and Chinese
- **Timezone handling** — all timestamps are stored in UTC; the frontend automatically converts to your browser's local timezone for date pickers, chart X-axis labels, and session timestamps

## Architecture

The application stays intentionally small: collectors read local agent artifacts, storage normalizes usage into SQLite, pricing enriches records, and the embedded HTTP server serves both REST endpoints and the dashboard.

```
agent-usage
├── main.go                     # Entry point, orchestrates components
├── config.yaml                 # Configuration
├── internal/
│   ├── config/                 # YAML config loader
│   ├── collector/
│   │   ├── collector.go        # Collector interface
│   │   ├── jsonl_scanner.go    # Shared bounded JSONL scanner config
│   │   ├── claude.go           # Claude Code session scanner
│   │   ├── claude_process.go   # Claude Code JSONL parser
│   │   ├── codex.go            # Codex CLI JSONL parser
│   │   ├── openclaw.go         # OpenClaw session scanner
│   │   ├── openclaw_process.go # OpenClaw JSONL parser
│   │   ├── opencode.go         # OpenCode SQLite collector
│   │   ├── kiro.go             # kiro scanner
│   │   ├── kiro_process.go     # kiro SQLite parser
│   │   ├── pi.go               # Pi coding agent session scanner
│   │   └── pi_process.go       # Pi coding agent JSONL parser
│   ├── pricing/                # litellm price fetcher + cost formula
│   ├── storage/
│   │   ├── sqlite.go           # DB init + migrations
│   │   ├── api.go              # Query types + read operations
│   │   ├── queries.go          # Write operations
│   │   ├── ops.go              # Ingestion health, reset, budget events
│   │   └── costs.go            # Cost recalculation + backfill
│   └── server/
│       ├── server.go           # HTTP server + REST API
│       ├── budget.go           # Local budget evaluation
│       ├── ops.go              # Scan/export/report handlers
│       ├── privacy.go          # Redaction helpers
│       └── static/             # Embedded dashboard, CSS, JS, vendored ECharts
└── agent-usage.db              # SQLite database (generated at runtime)
```

Security boundaries:

- Session directories are mounted read-only in Docker examples.
- The dashboard binds to localhost by default. If `server.auth_token` is configured, API calls require `Authorization: Bearer <token>`.
- Manual scan/reset endpoints require localhost access when no auth token is configured.
- Static assets are embedded; runtime UI does not fetch third-party scripts or fonts.
- Privacy mode can hash session IDs and redact paths/project/branch names in API responses, exports, and screenshots.
- Pricing sync is the only expected outbound request during normal operation.

## Cost Calculation

Pricing is fetched from [litellm's model price database](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json) and stored locally.

```
cost = (input - cache_read - cache_creation) × input_price
     + cache_creation × cache_creation_price
     + cache_read × cache_read_price
     + output × output_price
```

When prices update, historical records are automatically backfilled.

## API Endpoints

All read endpoints accept `from` and `to` (YYYY-MM-DD) query parameters. Optional: `source` (`claude`, `codex`, `openclaw`, `opencode`, `kiro`, `pi`), `model`, `project`, and `privacy=1`. Time-series endpoints also accept `granularity` (`1m`, `30m`, `1h`, `6h`, `12h`, `1d`, `1w`, `1M`). Time ranges use half-open intervals internally: `[from, to_next_day)`.

| Endpoint | Description |
|----------|-------------|
| `GET /api/stats` | Summary: total cost, tokens, sessions, prompts, API calls |
| `GET /api/cost-by-model` | Cost grouped by model |
| `GET /api/cost-over-time` | Cost time series (supports `granularity`) |
| `GET /api/tokens-over-time` | Token usage time series (supports `granularity`) |
| `GET /api/sessions?limit=100&offset=0` | Paginated session list with cost/token totals |
| `GET /api/session-detail?source=codex&session_id=ID` | Per-model breakdown for a source/session pair |
| `GET /api/health/ingestion` | Per-source collector health |
| `GET /api/budgets/status` | Local budget status |
| `GET /api/export?type=sessions&format=csv` | CSV/JSON export for sessions, daily, or models |
| `GET /api/report?format=markdown` | Markdown usage report |
| `POST /api/scan?source=codex` | Manual scan; omit source to scan all enabled sources |
| `POST /api/scan?source=codex&reset=true` | Clear one source's scan state/usage, then rescan |
| `POST /api/recalculate-costs` | Rebuild zero-cost records from local pricing |

Invalid date formats or reversed date ranges return a `400` JSON error with a descriptive message.

## Tech Stack

- **Go** — pure Go, no CGO required
- **SQLite** via [`modernc.org/sqlite`](https://pkg.go.dev/modernc.org/sqlite) — pure Go SQLite driver
- **ECharts** — charting library
- **`go:embed`** — single binary deployment

## Docker Details

The default compose file builds from source so local security fixes are included. Release workflows publish multi-arch images (amd64 + arm64) to this repository's GHCR namespace.

Use `docker-compose.example.yml` when you want a pinned template for GHCR-based deployments. SBOM/provenance publication is planned for the release workflow; until it is enabled, release notes must not claim signed provenance.

The default `docker-compose.yml` runs as UID 1000. If your host user has a different UID, edit the `user:` field:

```bash
# Check your UID/GID
id -u  # e.g. 1000
id -g  # e.g. 1000

# Edit docker-compose.yml: user: "YOUR_UID:YOUR_GID"
```

This is required because `~/.claude/projects` is mode 700 — only the owning UID can read it.

### Building locally

```bash
docker build -t agent-usage:local .

# For China mainland, use GOPROXY:
docker build --build-arg GOPROXY=https://goproxy.cn,direct -t agent-usage:local .
```

When using Docker directly, bind only to localhost unless you have added access control:

```bash
docker run --rm -p 127.0.0.1:9800:9800 agent-usage:local
```

## Community

Join the discussion at [Linux.do](https://linux.do/t/topic/1922004).

## License

[Apache 2.0](LICENSE)
