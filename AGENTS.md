# AGENTS.md

This file provides guidance to AI coding agents when working with code in this repository.

## Build & Run

```bash
go build -o agent-ledger .                 # build binary
./agent-ledger                             # run server (reads config.yaml by default)
./agent-ledger --config path/to/config.yaml
./agent-ledger version                     # print version info
./agent-ledger doctor --format markdown    # local diagnostics
./agent-ledger today                       # CLI summary
./agent-ledger workload start-run --workload-id <id> --source codex --agent-name codex --idempotency-key <stable-key>
./agent-ledger workload queue --source codex
./agent-ledger workload claim-next --holder codex-router --ttl 30m
./agent-ledger workload lease acquire --workload-id <id> --holder codex-router --ttl 30m
./agent-ledger workload lease list
./agent-ledger workload heartbeat --run-id <id> --status working --phase testing --progress 0.5
./agent-ledger workload liveness --max-age 10m --stale-only
./agent-ledger workload feed --severity warning --max-age 10m
./agent-ledger event schema                # print canonical event schema
./agent-ledger event examples --type model.call # print privacy-safe canonical event templates
./agent-ledger event validate < event.json # validate canonical events without writing SQLite
./agent-ledger event ingest < event.json   # ingest metadata-only canonical event(s)
./agent-ledger adapter spec                # print machine-readable adapter contract
./agent-ledger adapter matrix              # print adapter conformance matrix for fixture CI
./agent-ledger adapter conformance --kind provider-stream --strict --file fixture.sse # validate adapter fixture output without writing SQLite
./agent-ledger discovery                   # print local discovery manifest
./agent-ledger contracts                   # print REST/CLI/MCP contract bundle with hashes and cache semantics
./agent-ledger contracts verify            # print control-plane contract self-check report
./agent-ledger goal coverage               # print requirement-level product goal coverage report
./agent-ledger openapi                     # print metadata-only OpenAPI 3.1 control-plane contract
./agent-ledger integrations                # print privacy-safe integration capability catalog
./agent-ledger signals                     # print privacy-safe signal taxonomy catalog
./agent-ledger signal-coverage             # print taxonomy-to-adapter/provider/agent coverage report
./agent-ledger integrations readiness      # print integration activation readiness gates
./agent-ledger integrations production-gate --strict # block/review/pass production enablement gate
./agent-ledger runtime                     # print runtime mode and read-only/write status
./agent-ledger ui check                    # verify embedded dashboard UI/UX static contract
./agent-ledger config status --format markdown # print privacy-safe deployment config report
./agent-ledger readiness --format markdown # print privacy-safe control-plane readiness report
./agent-ledger admission check --surface http --method POST --path /api/events --role operator # dry-run operation access
./agent-ledger notify webhook --dry-run --severity warning --approval-due-within 24h # inspect redacted notification payload
./agent-ledger otel convert/ingest         # map OpenTelemetry GenAI JSON spans to canonical events
./agent-ledger a2a convert/ingest          # map A2A task snapshots/events to canonical events
./agent-ledger provider profiles           # print privacy-safe provider/runtime profile catalog
./agent-ledger agent profiles              # print privacy-safe agent/framework profile catalog
./agent-ledger agent recommend --profile codex-cli --provider openai-official --surface provider-stream --signals model,usage,cache # print read-only integration recommendation advisor report
./agent-ledger provider convert/ingest     # map provider usage envelopes to canonical events
./agent-ledger projection quality/repair   # diagnose and repair canonical-to-usage projection drift
./agent-ledger reconcile import/status     # import provider CSV/JSON bills for local reconciliation
./agent-ledger router simulate --to-model gpt-5-mini --ratio 0.5  # local what-if model routing estimate
./agent-ledger replay --source codex --session-id <id>  # per-call session token/cost replay
./agent-ledger badge --project repo --metric cost       # local SVG repo AI cost badge
./agent-ledger preflight --task refactor --project repo # estimate cost before starting a task
./agent-ledger chargeback --format csv                  # team/project showback report
./agent-ledger fleet --limit 50                         # sub-agent and parallel-run attribution
./agent-ledger wrapped --period month --format markdown # private period summary
./agent-ledger bundle export/import        # offline JSON bundle for air-gapped aggregation
./agent-ledger policy evaluate --model gpt-5.5 --action model.call  # local advisory policy evaluation
./agent-ledger policy approvals --privacy # pending approval queue without sensitive metadata
./agent-ledger policy enforcement --privacy # local policy enforcement evidence
./agent-ledger policy routes --due-within 24h --privacy # pending approval route summary
./agent-ledger mcp                         # local stdio JSON-RPC tool surface
```

## Testing

```bash
go test ./...                             # all tests
go test ./internal/collector/...          # single package
go test ./internal/storage/... -run TestDedup  # single test
```

No CGO required — the SQLite driver (`modernc.org/sqlite`) is pure Go.

## Docker

```bash
docker compose up -d                      # start with default compose
docker build -t agent-ledger:local .       # local image build
docker build --build-arg GOPROXY=https://goproxy.cn,direct -t agent-ledger:local .  # China proxy
```

Container runs as UID 1000 by default; adjust `user:` in docker-compose.yml if your host UID differs (needed because `~/.claude/projects` is mode 700).

**Important:** Only mount directories for agents you have actually installed. Docker automatically creates missing host directories as root, which causes two problems: (1) agent tools that detect installation by checking directory existence (e.g. Kiro CLI, `npx skills add`) will think the agent is installed when it isn't; (2) the root-owned directory prevents the real agent from writing to it later, breaking session recording.

## Architecture

Single-binary Go application that collects AI coding agent token usage from local session files and local agent databases, stores it in SQLite, and serves a web dashboard plus CLI.

**Data flow:** Collectors scan session dirs → parse usage rows → write to SQLite (with dedup) → pricing governance syncs official seeds + LiteLLM fallback + local overrides → costs calculated → aggregate tables rebuilt → advisory policies evaluated from a shared local evaluator → served via REST API + embedded web UI + CLI + local MCP tools.

### Key packages

- `internal/collector` — Source-specific parsers. Each collector implements `Scan()` which walks session directories and calls `processFile()` for incremental parsing. File offsets tracked in `file_state` table to avoid re-reading. `claude.go`/`claude_process.go` is the reference implementation for adding new sources. Collectors must normalize token fields to match the non-overlapping semantics defined below. Collectors also extract individual user prompt events (with timestamps) into the `prompt_events` table for time-accurate prompt counting. The Kiro collector (`kiro.go`/`kiro_process.go`) supports **two data sources** simultaneously: (1) a SQLite database (`~/.local/share/kiro-cli/data.sqlite3`) containing `conversations_v2` with per-request metadata, and (2) JSON session files (`~/.kiro/sessions/cli/*.json` + companion `*.jsonl`) with aggregated turn metadata. `Scan()` auto-detects each path type and routes accordingly. Kiro does not expose actual token counts, so tokens are **estimated**: input from `context_usage_percentage × context_window_tokens`, output from `response_size / 4` (SQLite) or CJK-aware character heuristics on JSONL content (JSON source). Known limitations: (1) subagent sessions (null `session_state` in JSON) cannot be tracked; (2) token counts are estimates, not exact values; (3) the two sources have disjoint session IDs — no overlap or dedup concern. The Pi collector (`pi.go`/`pi_process.go`) shares the same JSONL format as OpenClaw (same underlying framework). It tracks `model_change` entries for mid-session model switching and derives project names from the session CWD (with workspace slug as fallback). Directory structure: `~/.pi/agent/sessions/<workspace-slug>/<file>.jsonl`.
- `internal/storage` — SQLite layer. `sqlite.go` has schema + versioned migrations (tracked via `meta` table with `migration_{id}` keys, each runs once), `queries.go` handles writes, `api.go` handles reads, `costs.go` does cost recalculation. All DB access serialized through a mutex (`DB.mu`). Key tables: `usage_records` (per-API-call token/cost data), `sessions` (session metadata), `prompt_events` (per-prompt timestamps for time-range queries), `pricing` (model prices), `file_state` (scan offsets and parser context for incremental scanning), `control_idempotency` (privacy-safe request hashes for retry-safe workload/run writes), and `workload_leases` (short-lived async execution claims; SQLite stores token hashes only). Control-plane writers should pass stable idempotency keys for async router, wrapper, or CI retries; same-key/different-request conflicts must stay explicit. Routers can call claim-next to atomically select the next queue-eligible workload and create a lease in one transaction, or acquire/renew/release explicit workload leases around actual execution to avoid concurrent workers claiming the same workload.
- `internal/pricing` — Syncs LiteLLM fallback prices and applies official OpenAI/Anthropic seed rows plus local overrides. Cost formula: `input × input_price + cache_creation × cache_creation_price + cache_read × cache_read_price + output × output_price`.
- `internal/server` — HTTP server with REST API endpoints (`/api/stats`, `/api/cost-intelligence`, `/api/pricing/status`, etc.) and `go:embed` static files (HTML + ECharts dashboard). Endpoints accept `from`, `to`, `source`, `model`, `project`, and privacy filters where applicable. Invalid dates or reversed ranges return `400` with a JSON error message.
- `internal/config` — YAML config loader and privacy-safe config status report. Search order: `--config` flag → `/etc/agent-ledger/config.yaml` → `./config.yaml`. Supports `~` expansion in paths. Config status must expose counts, booleans, and remediation hints only; never raw paths, tokens, API credentials, webhook URLs, machine names, authors, prompts, responses, or session ids.
- `internal/controlplane` — Privacy-safe control-plane readiness and admission checks for wrappers, CI, routers, and operators. Readiness reports summarize database queryability, config posture, runtime mode, contract verification, ingestion evidence, and pricing evidence. Admission checks dry-run REST/CLI/MCP operations against role and read-only rules. Neither surface emits raw paths, URLs, secrets, prompts, responses, sessions, projects, branches, machine names, or authors.

### Token semantics

All token fields are **non-overlapping components** that sum to the total:

```
input_tokens              — non-cached input (mutually exclusive with cache fields)
cache_read_input_tokens   — input tokens served from cache
cache_creation_input_tokens — input tokens written to cache
output_tokens             — total output tokens
reasoning_output_tokens   — reasoning tokens (subset of output, informational only)

total_input  = input_tokens + cache_read_input_tokens + cache_creation_input_tokens
total_output = output_tokens
total_tokens = total_input + total_output
```

If a data source reports `input_tokens` as the total (including cache), the collector must subtract cache tokens before storing. If a source reports non-cached input natively, store as-is.

### Deduplication

Usage records are deduped via a unique index on `(source, session_id, model, timestamp, input_tokens, output_tokens)` so the same native session id from different agents cannot collide. Incremental file scanning uses stored offsets in `file_state` to resume from where it left off. For data sources where session metadata only appears at the top of the file (Codex, OpenClaw), `file_state.scan_context` stores parser state (sessionID, cwd, version, model) as JSON so incremental scans can restore context without re-reading the file from the beginning.

## Conventions

- Conventional Commits (`feat:`, `fix:`, `refactor:`, etc.) — GoReleaser generates changelog from these.
- Releases built with GoReleaser; version/commit/date injected via ldflags.
- Web UI is embedded via `go:embed` in `internal/server/static/` — changes to frontend files require rebuilding the binary.
