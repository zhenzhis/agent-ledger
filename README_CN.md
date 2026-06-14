# Agent Ledger

Agent Ledger 是本地优先的 AI Agent FinOps、工作负载账本、额度、价格、审计与生产力洞察控制台，支持 Claude Code、Codex、OpenCode、OpenClaw、kiro、Pi 等本地 coding agent。

[English README](README.md)

![Agent Ledger dashboard](docs/dashboard.png)

## Fork 与致谢

Agent Ledger 是 ZhenZhi 基于 [briqt/agent-usage](https://github.com/briqt/agent-usage) 的独立二次开发项目。我们保留上游本地采集和单二进制基础，并感谢原作者与贡献者。

项目已从 `agent-usage` 正式更名为 `agent-ledger`。旧数据库和配置不会被自动删除。

## 功能

- 从 Claude Code、Codex、OpenCode、OpenClaw、kiro、Pi 采集本地用量。
- 使用本地 override、OpenAI/Anthropic 官方 seed、LiteLLM fallback 进行价格治理。
- 不读取 prompt 内容，只基于 token、cache、模型、时间和会话元数据解释昂贵 session。
- 提供预算、burn rate、本地 quota 估算、cache doctor、模型调用次数、异常检测、采集健康。
- 将底层 session 自动提升为 canonical Workload Ledger，记录 goal、run、model call、tool call、artifact、evaluation 与 policy decision。
- Web 控制台展示待审批策略请求，支持审批人身份、quorum 进度和批准/拒绝操作。
- 支持本地审计日志、隐私 preset、导出、Markdown 报告、证据包、团队 showback。
- 单 Go 二进制，内嵌静态 UI，SQLite 存储。

## 快速开始

```bash
git clone https://github.com/zhenzhis/agent-ledger.git
cd agent-ledger
go build -o agent-ledger .
cp config.example.yaml config.yaml
./agent-ledger
```

打开 [http://127.0.0.1:9800](http://127.0.0.1:9800)。

Docker：

```bash
docker compose up -d --build
```

CLI：

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

## 控制面幂等

Workload 与 run 写操作支持稳定幂等键，面向异步 agent router、wrapper、CI job 与重试客户端：

- HTTP：在 `POST /api/workloads` 与 `POST /api/agent-runs` 上传入 `Idempotency-Key` 或 `X-Idempotency-Key`。
- JSON：同一批 endpoint 也可在 body 中传入 `idempotency_key`。
- CLI：`agent-ledger workload create` 与 `agent-ledger workload start-run` 支持 `--idempotency-key`。
- MCP：`ledger.start_workload` 与 `ledger.start_run` 支持 `idempotency_key`。
- OpenAPI：`GET /api/openapi.json` 会描述 workload/run 请求 schema、幂等 headers 与 `409 Conflict` 响应。

第一次请求会写入 workload/run，并且只在 SQLite 中记录 operation、key scope、request hash、result type、result id 与时间戳。同一 key 与同一归一化请求重试时返回原 ID，并带 `idempotent_replay: true`。同一 key 复用到不同输入会明确失败，HTTP 返回 `409 Conflict`，CLI/MCP 返回错误。Agent Ledger 不会在幂等表中保存原始请求体。

## Workload Lease

异步 router 和长时间运行的 agent 可以在执行 workload 前获取短期 lease：

- REST：`POST /api/workloads/claim-next`、`POST /api/workloads/lease`、`POST /api/workloads/lease/renew`、`POST /api/workloads/lease/release`、`GET /api/workloads/leases`。
- CLI：`agent-ledger workload queue`、`agent-ledger workload claim-next --holder router-a`，以及 `agent-ledger workload lease acquire|renew|release|list`。
- MCP：`ledger.workload_queue`、`ledger.claim_next_workload`、`ledger.acquire_workload_lease`、`ledger.renew_workload_lease`、`ledger.release_workload_lease`、`ledger.workload_leases`。
- OpenAPI：`GET /api/openapi.json` 会描述 lease 请求/响应 schema，以及已有 active lease 时的 `409 Conflict`。

`claim-next` 会在同一个 SQLite 事务中原子选择一个可领取 workload 并创建 lease，多个本地 worker 不需要先 list 再抢占。默认只领取 `queued` 或 `active` workload；只有 router 明确要处理 stalled/blocked 工作时，才传 `status=any` 或逗号分隔的非终态 status。

`GET /api/workloads/queue`、`agent-ledger workload queue` 与 MCP `ledger.workload_queue` 是只读 queue 探针。它们会返回可领取 workload 数、非终态分布、active/expired lease 压力、最老可领取 workload 时间和下一次 lease 过期时间，不会回写过期 lease 行。REST 端点会返回忽略 `generated_at` 的稳定 `ETag`，HTTP monitor 可用 `If-None-Match` 在 queue 状态未变化时获得 `304 Not Modified`。

`GET /api/agent-runs/liveness` 同样会返回稳定 `ETag`；纯展示用的 `age_seconds` 跳动会被忽略，但 stale 状态、phase、status 与 heartbeat 变化仍会让响应失效。Readiness 报告会加入 active/stale run 数与最老 active run 的时间桶，不暴露 run id、workload id、goal、project、repo、branch、command 或 status message。

Workload detail、graph、timeline 与 terminal-state snapshot 读端点都是 GET-only viewer reads，并返回 `ETag`，方便本地 monitor 对单个 workload 做轮询。

同一个 workload 同时只允许一个 active lease。`lease_token` 只在 acquire/claim 响应中返回；list、renew、release、readiness、doctor、audit 和 contract surface 都不会返回它。SQLite 只保存 SHA-256 token hash，不保存明文 token。读路径只派生过期状态，不写回 SQLite，因此 observer/read-only 模式仍保持只读。

仓库内 `examples/adapter-fixtures/` 提供 canonical events、OpenAI Responses、OpenAI Chat Completions、Anthropic Messages、provider SSE stream、OpenTelemetry GenAI span 与 A2A task snapshot 的 strict conformance 样例。

## 配置

配置搜索顺序：

1. `--config path/to/config.yaml`
2. `/etc/agent-ledger/config.yaml`
3. `./config.yaml`

建议从 [config.example.yaml](config.example.yaml) 开始。该模板默认本地优先、不包含密钥、默认关闭外发 webhook 与 provider gateway，并覆盖所有企业控制面配置段。核心配置：

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

运行 `agent-ledger config status --format markdown` 或打开 `GET /api/config/status`，可以查看隐私安全的部署配置状态报告。它只展示绑定安全性、auth 是否存在、collector 路径数量、价格模式、外发通道、隐私开关和修复建议，不打印原始路径、auth token、API key、webhook URL、机器名、作者、prompt、response 或 session id。

企业合同价、三方中转价、地区倍率和内部折扣请通过 `pricing.overrides` 配置。

可选 gateway 是本地 provider 代理，支持 OpenAI-compatible Chat Completions、OpenAI Responses 与 Anthropic Messages。它默认关闭，支持 OpenAI-compatible Chat Completions JSON/SSE、OpenAI Responses JSON/SSE，以及 Anthropic Messages JSON/SSE，只从配置的环境变量读取上游 API key，并只记录 token usage 与审计元数据，不保存 request messages 或 response content。OpenAI Responses streaming usage 会从最终 `response.completed` 事件记录。Anthropic Messages streaming usage 会合并 `message_start` 与 `message_delta` SSE 事件中的 usage。对 OpenAI Chat Completions streaming 请求，`include_stream_usage: true` 会在客户端没有显式设置 `stream_options.include_usage` 时请求兼容上游返回最终 usage chunk；如果三方中转拒绝该选项，可设为 `false`。如果成功的上游响应没有可解析 usage，Agent Ledger 会保留原始上游响应，不伪造 usage row，并返回 `X-Agent-Ledger-Usage-Warning`。

Webhook 通知默认关闭。显式开启后，`POST /api/notifications/webhook` 与 `agent-ledger notify webhook` 只发送有上限的 workload-event、pending approval 与 approval route 脱敏摘要；goal、project、repo、branch、team、approver route、escalation target、approval target、approval reason、event id、workload id、run id、approval request id 都会被隐藏或 hash。可用 `--dry-run` 或 `dry_run=1` 检查即将发送的 payload，不进行外发。

Gateway 请求可以通过 query 参数或 request `metadata` 附加账本上下文：`agent_ledger.project`、`agent_ledger.goal`、`agent_ledger.workload_id`、`agent_ledger.agent_run_id`、`agent_ledger.session_id`、`agent_ledger.git_branch`。这样 wrapper、MCP 工具和异步 agent 可以把实时模型调用绑定到已有 workload/run，而无需暴露 prompt 内容。

## 价格与成本

Agent Ledger 使用非重叠 token 字段：

```text
total = input_tokens
      + cache_creation_input_tokens
      + cache_read_input_tokens
      + output_tokens
```

成本公式：

```text
cost = input_tokens * input_price
     + cache_creation_input_tokens * cache_write_price
     + cache_read_input_tokens * cache_read_price
     + output_tokens * output_price
```

价格优先级：

1. 本地 override，用于企业合同价、第三方中转价或地区价格 profile。
2. 内置 OpenAI/Anthropic 官方 seed。这是随 Agent Ledger 发布的 curated overlay，不是实时抓取 provider 页面。
3. 从 `model_prices_and_context_window.json` 获取的 LiteLLM fallback。
4. OpenCode 等来源自带费用，默认保留为该来源事实。

每条记录可追踪价格来源、匹配模型、匹配方式和 confidence。`GET /api/pricing/status` 会区分 `seeded`、`fetched`、`configured` 三种 freshness provenance：只有远端 fetch 价格源会按时间窗口标记 stale；内置官方 seed 与本地 override 通过 hash 与 source status 审计。未知价格、过期价格、fuzzy 匹配、fallback、source-reported 与未计价记录都会进入数据质量中心，不会被静默隐藏。

参考：

- [OpenAI API pricing](https://openai.com/api/pricing/)
- [Anthropic Claude pricing](https://platform.claude.com/docs/en/about-claude/pricing)
- [LiteLLM model price data](https://github.com/BerriAI/litellm/blob/main/model_prices_and_context_window.json)

## 架构

```text
collectors / CLI wrapper / MCP tools -> canonical events -> workload ledger
                                     -> raw usage + pricing governance -> aggregates
                                     -> REST API -> embedded dashboard / CLI
```

核心表：

- `canonical_events`：面向未来 collector、MCP、A2A、gateway 的规范事件流，包含 schema/source/parser provenance 与隐私安全的原生引用。
- `workloads`、`agent_runs`、`agent_run_events`、`model_calls`、`tool_calls`：goal/run/heartbeat/call 级账本。
- `control_idempotency`：只保存 request hash 的 workload/run retry-safe 写入账本。
- `workload_leases`：短期异步执行声明；SQLite 不保存明文 lease token。
- `workload_sessions`：旧 session 与 workload 的兼容映射。
- `context_refs`、`artifacts`、`evaluations`、`policy_decisions`、`workload_links`：隐私安全的 AgentOps 上下文、产物、结果、治理、依赖和 lineage 记录。
- `usage_records`：API 调用级 token 与费用。
- `sessions`：source-scoped 会话元数据。
- `prompt_events`：按时间统计 prompt。
- `pricing`、`pricing_sources`、`pricing_snapshots`：价格规则和价格源健康。
- `hourly_usage_aggregate`、`daily_usage_aggregate`：Dashboard rollup。
- `reconciliation_imports`：本地账本与 provider 账单的对账记录，包含 payload hash 和账单窗口。
- `ingestion_health`、`insight_events`、`audit_log`：运维和质量证据。

## API

常用过滤参数：`from`、`to`、`source`、`model`、`project`、`privacy`。

| Endpoint | 用途 |
|---|---|
| `GET /.well-known/agent-ledger.json` | 面向 agent、wrapper、router 的隐私安全本地 discovery manifest |
| `GET /api/discovery` | API 命名空间下的同一 discovery manifest |
| `GET /api/contracts` | 单次握手的契约 bundle，包含稳定文档 URI、hash、缓存语义、CLI 命令和 MCP 入口 |
| `GET /api/contracts/verify` | 机器可读控制面自检报告，校验 discovery、bundle、OpenAPI、schema、adapter、runtime 与隐私不变量 |
| `GET /api/openapi.json` | metadata-only OpenAPI 3.1 控制面契约，供 wrapper、router 与 CI 集成 |
| `GET /api/runtime/status` | 运行模式、只读状态、后台/写操作状态与兼容性 hash |
| `GET /api/config/status` | 隐私安全部署配置报告，包含风险检查与修复建议 |
| `GET /api/readiness` | 面向探针、wrapper、router 与 CI 的隐私安全控制面就绪报告 |
| `GET /api/admission/check` | 在当前 read-only/RBAC 规则下对 HTTP、CLI 或 MCP 操作做隐私安全 dry-run |
| `GET /api/dashboard` | Web dashboard 的一致性 KPI、token、费用与模型数据包 |
| `GET /api/stats` | 总览 |
| `GET /api/workloads` | 服务端分页工作负载账本 |
| `POST /api/workloads` | 创建本地 workload |
| `POST /api/workloads/close` | 关闭 workload 并记录结果 |
| `POST /api/workloads/link` | 创建 metadata-only 的 workload 依赖或 lineage 边 |
| `POST /api/workloads/claim-next` | 原子领取下一个可执行 workload 并返回 lease |
| `GET /api/workloads/queue` | 只读 queue 可领取状态和 lease 压力统计 |
| `POST /api/workloads/lease` | 为 workload 获取一个短期执行 lease |
| `POST /api/workloads/lease/renew` | 使用 lease token 续期 active workload lease |
| `POST /api/workloads/lease/release` | 使用 lease token 释放 active workload lease |
| `GET /api/workloads/leases` | 列出 workload leases，但不返回 lease token |
| `POST /api/agent-runs` | 在已有 workload 下启动一个 run |
| `POST /api/agent-runs/heartbeat` | 写入 metadata-only 异步 run 存活/进度心跳 |
| `GET /api/agent-runs/liveness` | 列出 active run 与心跳 stale 状态 |
| `GET /api/workload-detail` | workload 的 run、model call、tool、session、policy 明细 |
| `GET /api/workload-graph` | workload 图谱 |
| `GET /api/workload-timeline` | 按时间排序的 workload 审计时间线 |
| `GET /api/workload-state` | 单个异步 agent workload 的 terminal-state 派生快照 |
| `GET /api/workload-events` | 面向 monitor、router、通知适配器的本地 workload 状态事件 feed；返回 `cursor`、`generated_at` 与 `ETag`，支持增量轮询 |
| `GET /api/workload-events/stream` | 面向本地轮询 monitor 与 router subscription 的 SSE workload 状态流；SSE `id` 使用 feed cursor |
| `POST /api/notifications/webhook?approval_due_within=24h` | 显式发送脱敏 workload-event、approval 与 approval-route 摘要到配置的 webhook |
| `GET /api/integrations` | 隐私安全的集成能力目录 |
| `GET /api/integrations/adapter-spec` | 面向未来 Agent CLI、框架、gateway、OTel、A2A 与 provider 集成的机器可读 adapter 契约 |
| `POST /api/integrations/conformance` | 校验 canonical、provider、provider-stream、OpenTelemetry GenAI 或 A2A adapter fixture，但不写入 SQLite；`strict=true` 会把 provenance warning 视为失败 |
| `GET /api/event-schema` | Canonical event schema 与支持的事件类型 |
| `GET /api/event-examples` | 隐私安全的 canonical event 模板，可用 `type` 或 `event_type` 过滤 |
| `POST /api/events/validate` | 校验 canonical metadata-only events，但不写入 SQLite |
| `POST /api/events` | 写入 metadata-only canonical events |
| `POST /api/otel/genai` | 将 OpenTelemetry GenAI JSON span 转成 canonical model-call events |
| `POST /v1/traces` | 显式开启 `integrations.otlp_receiver.enabled` 后可用的本地 OTLP HTTP JSON/protobuf traces receiver |
| `POST /api/otlp/v1/traces` | 同一 receiver 的 API 命名空间路径，便于本地反向代理 |
| `POST /api/a2a/tasks` | 将 A2A JSON task snapshot/event 转成 workload/run/artifact/evaluation events |
| `POST /api/provider/calls` | 将 provider response usage envelope 转成 canonical model-call events |
| `POST /gateway/openai/v1/chat/completions` | 显式开启 `gateway.enabled` 后可用的 JSON/SSE OpenAI-compatible gateway |
| `POST /gateway/openai/v1/responses` | 显式开启 `gateway.enabled` 后可用的 OpenAI Responses JSON/SSE gateway |
| `POST /gateway/anthropic/v1/messages` | 显式开启 `gateway.enabled` 后可用的 Anthropic Messages JSON/SSE gateway |
| `GET /api/reconciliation/status` | 查看最近本地账本与 provider 账单对账 |
| `POST /api/reconciliation/import` | 导入手动 summary 或 provider CSV/JSON 账单并做本地对账 |
| `GET /api/router/simulate?to_model=gpt-5-mini&ratio=0.5` | 模拟模型路由调整的费用影响，不修改账本 |
| `GET /api/preflight/estimate?task=refactor&project=repo-name` | 开始 agent 任务前估算可能费用、token 与调用量 |
| `GET /api/chargeback` | 团队/项目/source/model showback；使用 usage records 并关联 workload metadata |
| `GET /api/fleet-attribution` | sub-agent、parent run 与并行 run 成本归因 |
| `GET /api/wrapped?period=monthly&format=markdown` | 月度/周度/年度 Agent Wrapped 摘要，不分析 prompt 内容 |
| `POST /api/policy/evaluate` | 评估本地 advisory policy，并可选择写入 policy decision |
| `GET /api/policy/audit` | 使用本地 policy rules 审计历史 usage、tool call 和 workload |
| `GET /api/policy/enforcement` | 汇总本地 policy decision、approval request 与 audit event 的执行证据 |
| `GET /api/policy/approvals?status=pending&privacy=1` | 查看本地 pending、approved、rejected 或全部策略审批请求；`privacy=1` 会 hash request/workload/run id，并隐藏 project、target、approver、escalation、reason、payload 与 decision note |
| `POST /api/policy/approvals` | 为本地策略审批请求投批准/拒绝票；支持 `required_approvals` 法定人数，并只在本地审计中记录投票/quorum 元数据，不保存 note 明文 |
| `GET /api/policy/approval-routes?due_within=24h` | 汇总 pending 审批路由，供本地通知适配器使用 |
| `GET /api/audit-log?action=pricing&role=operator` | 过滤本地操作审计事件；支持 `from`、`to`、`actor`、`role`、`action`、`target`、`limit` 与隐私模式 |
| `GET /api/sessions` | 服务端分页会话账本 |
| `GET /api/session-replay?source=codex&session_id=...` | 单个 session 的调用级 token/cost 时间回放 |
| `GET /api/badge/repo.svg?project=repo-name&metric=cost` | 本地 SVG repo 成本、token 或 cache badge |
| `GET /api/model-registry` | 模型与价格治理注册表 |
| `GET /api/pricing/status` | 价格源 provenance、新鲜度、有效规则摘要、未计价模型 |
| `POST /api/pricing/sync` | 同步价格 |
| `POST /api/pricing/recalculate?mode=zero|all` | 重算费用 |
| `POST /api/projections/repair` | 修复 canonical `model_calls` 到 `usage_records` 的投影漂移，并重建 aggregates |
| `GET /api/cost-intelligence` | 昂贵会话解释，包含 input/output/cache/reasoning token 构成、每调用/每 Prompt 费用、价格来源与可信状态、未计价/fallback/模糊匹配/source-reported 计数、原因和建议 |
| `GET /api/cache/doctor` | cache 命中、写入、读取诊断 |
| `GET /api/doctor?format=markdown` | 一键本地诊断 usage、采集、价格、数据质量与 workload 状态 |
| `GET /api/data-quality` | 数据可信度报告，包含未计价、source-reported、fallback 与 aggregate-estimated 记录 |
| `GET /api/model-calls` | 模型调用次数 |
| `GET /api/quota/status` | 本地 quota 和 burn-rate 估算 |
| `GET /api/anomalies` | 异常检测事件 |
| `GET /api/watchdog/events` | 按筛选范围返回 runaway、调用密度、cache miss risk、非工作时段 watchdog 事件 |
| `GET /api/evidence-bundle` | 脱敏证据包，包含 health、pricing、consistency、异常与 workload 状态证据 |
| `GET /api/offline-bundle/export` | 导出带 hash/可选签名的离线包 |
| `POST /api/offline-bundle/import` | 导入离线包中的 canonical events |
| `GET /api/export?type=workloads&format=csv` | CSV/JSON 导出 |
| `GET /api/export?type=chargeback&format=csv` | 团队 showback CSV 导出 |
| `GET /api/report?format=markdown` | Markdown 报告 |

手动扫描、清理重扫、价格同步、导入和费用重算默认只允许本机访问；暴露到网络前必须配置 auth token 或反向代理访问控制。

当策略返回 `require_approval` 时，Agent Ledger 会写入本地 pending 审批请求并返回 id。Admin 可通过 `POST /api/policy/approvals` 或 `agent-ledger policy resolve` 投批准/拒绝票；默认法定人数为 1，`required_approvals` 可要求多名审批人达成 quorum 后，原操作再带 `approval_id=<id>` 或 `X-Agent-Ledger-Approval: <id>` 重试。审批复用会匹配 action/target；当原请求带有 source、model、project 时也会一并校验，因此实时 gateway 的某个模型审批不能被其他模型复用。Policy rule 还可以携带 `approvers`、`escalate_after` 与 `escalate_to`；这些字段会作为本地审批路由元数据保存，并生成 `due_at` 与 `overdue` 证据，供 Dashboard、Webhook 摘要和执行报告使用。

## MCP 工具接口

`agent-ledger mcp` 会启动本地 stdio JSON-RPC 工具服务，供 agent 框架或 wrapper 接入。当前实现保持本地优先和隐私优先：工具可以创建或关闭 workload、关联 workload 依赖、获取/续期/释放 workload lease、在已有 workload 下启动 run、写入 run heartbeat、查询 run liveness 与 terminal-state 快照、读取 queue 探针和 cursor-stable workload event feed、记录 tool-call 元数据、context ref、hash 后的 artifact 与质量/evaluation 信号、查询本地策略建议、列出并投票处理本地审批请求、读取审批路由聚合、查询预算状态、解释成本、查找相似 workload。Resources 提供 metadata-only 的 schema、integration、budget、workload、queue、隐私裁剪 lease、run liveness、feed、terminal-state、policy 上下文；resource URI 支持查询参数，例如 `agent-ledger://workloads/queue?source=codex&team=infra`、`agent-ledger://workloads/leases?include_inactive=1&limit=25`、`agent-ledger://workload/state?workload_id=...&max_age=10m`、`agent-ledger://workload/timeline?workload_id=...&limit=50`、`agent-ledger://agent-runs/liveness?max_age=10m&stale_only=1&limit=25`、`agent-ledger://workloads/feed?severity=warning&source=codex&project=agent-ledger&limit=50`、`agent-ledger://policy/approvals?status=pending&privacy=1` 与 `agent-ledger://policy/approval-routes?due_within=24h&privacy=1`。`resources/subscribe` 会在本地观察精确订阅的 URI，并在该 scope 变化时用 `notifications/resources/updated` 返回最新 cursor/hash；hash cursor 会忽略 volatile `generated_at` 与 `age_seconds` 字段，避免只有渲染时间变化时唤醒 router。Prompts 提供可复用的 workload、成本复盘、证据包模板。它不会读取 prompt 内容，也不会主动把数据发送到远程 MCP host。MCP、REST 与 CLI 的 policy evaluation 共用同一个本地 evaluator，确保不同接入方式得到一致的 advisory 决策。

MCP `tools/list` 会返回标准风格的 `annotations.readOnlyHint`，以及 `_meta.agent_ledger` 字段：`writes_local_state`、`write_mode`（`none`、`always` 或 `conditional`）、`available_in_read_only`、`read_only_behavior`。Router 和多智能体框架应在观测部署中调用工具前读取这些字段。

`GET /api/integrations`、`GET /.well-known/agent-ledger.json`、`agent-ledger integrations`、MCP `ledger.discovery`、MCP `ledger.integrations` 和 `agent-ledger://discovery/manifest` 会暴露运行时能力字段：`writes_local_state`、`available_in_read_only`、`runtime_status`。Discovery manifest 还会以一等字段暴露 `contract_bundle_uri`、`openapi_uri`、`capability_catalog_hash`、`runtime_status_uri`、`canonical_schema_uri`、`canonical_schema_hash`、`event_examples_uri`、`adapter_spec_uri`、`adapter_spec_hash`、`adapter_conformance_uri`，便于轻量 wrapper 自动接入。`GET /api/contracts`、`agent-ledger contracts`、MCP `ledger.contracts` 和 `agent-ledger://contracts/bundle` 会暴露单次握手的 contract bundle，包含文档 URI、hash、缓存语义、CLI 命令与 MCP 入口。`GET /api/contracts/verify`、`agent-ledger contracts verify`、MCP `ledger.contracts_verify` 和 `agent-ledger://contracts/verification` 会暴露机器可读自检报告，用于校验 discovery、bundle、OpenAPI、schema、adapter、runtime、只读语义与隐私不变量。`GET /api/openapi.json`、`agent-ledger openapi`、MCP `ledger.openapi` 和 `agent-ledger://contracts/openapi` 会暴露 metadata-only OpenAPI 3.1 文档，用于稳定 REST 控制面接口。`GET /api/integrations/adapter-spec`、`agent-ledger adapter spec`、MCP `ledger.adapter_contract` 和 `agent-ledger://integrations/adapter-contract` 会暴露同一份机器可读 adapter 契约。`GET /api/runtime/status` 与 `agent-ledger runtime` 提供同一个进程级 observer/control-plane 状态，适合探针使用。`GET /api/config/status`、`agent-ledger config status`、MCP `ledger.config_status` 与 `agent-ledger://config/status` 提供隐私安全的部署配置状态报告，适合 wrapper、CI 和运维检查。`GET /api/readiness`、`agent-ledger readiness`、MCP `ledger.readiness` 与 `agent-ledger://readiness` 提供隐私安全的就绪报告，汇总数据库、配置、运行模式、契约、采集和价格证据，但不泄露本地数据。`GET /api/admission/check`、`agent-ledger admission check`、MCP `ledger.admission_check` 与 `agent-ledger://admission/check` 可让 wrapper 和 router 在真正调用前，按当前 role/read-only 规则 dry-run HTTP、CLI 与 MCP 操作。REST discovery、contract bundle、contract verification、OpenAPI、catalog、runtime status、config status、readiness、admission、adapter spec 和 event schema 端点会返回强 `ETag`，并支持 `If-None-Match` 返回 `304 Not Modified`，让 wrapper 不必重复解析未变化的契约 JSON。Agent router 和 wrapper 应读取这些字段，而不是硬编码 endpoint 假设，尤其是在启用 `rbac.read_only` 时。

当前工具：

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

当前 resources：

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
- `agent-ledger://budget/current`，支持 `window`、`source`、`model`、`project` 查询参数
- `agent-ledger://workloads/recent`，包含 workload summary rows 与派生 terminal-state snapshots，支持 `from`、`to`、`source`、`model`、`project`、`status`、`q`、`limit`、`offset`、`stale_after`
- `agent-ledger://workloads/queue`，包含只读 queue 可领取状态和 lease 压力统计，支持 `source`、`project`、`repo`、`team`、`owner`、`status`、`q`
- `agent-ledger://workloads/leases`，包含面向 router context 的隐私裁剪 lease rows，支持 `include_inactive` 与 `limit`；不会暴露 workload id、holder、purpose 或 lease token
- `agent-ledger://workloads/feed`，包含供本地 monitor 和 router 使用的 cursor-stable workload state events，支持 `from`、`to`、`source`、`model`、`project`、`phase`、`severity`、`limit`、`stale_after`
- `agent-ledger://workload/state`，包含隐私安全的单 workload terminal-state 快照；要求 `workload_id`，支持 `max_age`，不会暴露 workload id、goal、project、repo、branch、team、owner 或 status message
- `agent-ledger://workload/timeline`，包含隐私安全的单 workload timeline event kind；要求 `workload_id`，支持 `limit`，不会暴露 event id、run id、label、detail、goal、project metadata、status message 或 raw identifier
- `agent-ledger://agent-runs/liveness`，包含面向异步 monitor 的隐私安全 run liveness rows，支持 `max_age`、`stale_only`、`source`、`project`、`limit`；不会暴露 run id、workload id、goal、project、repo、branch 或 status message
- `agent-ledger://policies/status`
- `agent-ledger://policy/approvals`，包含本地审批队列行，支持 `status`、`limit`、`privacy`
- `agent-ledger://policy/approval-routes`，包含 pending 审批路由聚合，支持 `due_within`、`limit`、`privacy`

当前 prompts：

- `agent-ledger/workload-brief`
- `agent-ledger/cost-review`
- `agent-ledger/incident-evidence`

Canonical event ingest 支持 workload、workload link、run、run heartbeat、model call、tool call、context ref、artifact、evaluation、policy decision 事件。Payload 只允许元数据；如果出现 raw prompt/content 相关键会直接失败，不会静默持久化。当前 event-envelope contract 只接受 `schema_version: "v1"`；未知版本会明确校验失败。`GET /api/event-examples`、`agent-ledger event examples` 与 MCP `ledger.event_examples` 会返回每类事件的隐私安全模板。`GET /api/integrations/adapter-spec`、`agent-ledger adapter spec`、MCP `ledger.adapter_contract` 与 `agent-ledger://integrations/adapter-contract` 会暴露机器可读 adapter 契约，包含支持的输入类型、必需 envelope 字段、禁止 payload key、token 语义、质量门槛、验证命令和 ingest 入口。`POST /api/events/validate` 与 `agent-ledger event validate` 会执行同一套契约校验，但不写入 SQLite，适合直接 canonical event 检查。`POST /api/integrations/conformance` 与 `agent-ledger adapter conformance` 还会先转换 provider JSON、provider SSE、OpenTelemetry GenAI 与 A2A fixture 再校验，让 wrapper CI 在开启 ingest 前证明兼容性。Envelope 同时携带 `schema_version`、`source_version`、`parser_version`、`raw_ref`、`match_type` 等 adapter provenance 字段，让未来 collector 能说明事件是 exact、estimated、reconstructed、source-reported 还是 fuzzy，同时不保存 prompt 内容。Canonical `model.call` 也会投影到 `usage_records`，让 dashboard、预算、导出和 preflight 尽量使用同一个 token 来源。`workload.linked` 会记录异步 goal 图的依赖和 lineage 边，但不保存 prompt 内容。`agent.run.heartbeat` 会写入存活/进度时间线并更新 run 快照，使长时间异步 agent 可被监控而不需要读取 prompt；liveness 查询会标记心跳或启动时间超过阈值的 active run。`GET /api/integrations`、`agent-ledger integrations` 与 `ledger.integrations` 会暴露当前 connector/protocol 能力目录，但不会泄露本地 source 原始路径。`POST /api/otel/genai` 与 `agent-ledger otel ingest` 支持 OpenTelemetry GenAI JSON span，并只保留经过挑选的元数据和 token 字段。显式开启后，`POST /v1/traces` 与 `POST /api/otlp/v1/traces` 可接收 OTLP HTTP JSON 或 HTTP protobuf trace batch，并有 body 与 span 数量上限；OTLP gRPC receiver 仍待补齐生命周期与 conformance 测试后再开放。`POST /api/a2a/tasks` 与 `agent-ledger a2a ingest` 支持 A2A task snapshot/event，只保留任务生命周期元数据，不保存 message/history/artifact part 内容。`POST /api/provider/calls` 与 `agent-ledger provider ingest` 支持 OpenAI-compatible、Anthropic-style、LiteLLM-style usage envelope，不保存 request/response message 内容。显式开启后，`POST /gateway/openai/v1/chat/completions` 会在内存中代理 OpenAI-compatible JSON 或 SSE streaming 请求，`POST /gateway/openai/v1/responses` 会在内存中代理 OpenAI Responses JSON 或 SSE streaming 请求，`POST /gateway/anthropic/v1/messages` 会在内存中代理 Anthropic Messages JSON 或 SSE streaming 请求；三者都会执行本地 policy 检查，写入 usage/audit 元数据，并通过响应 headers 暴露记账状态，不保存 prompt 或 response content。`POST /api/reconciliation/import` 与 `agent-ledger reconcile import` 支持导入本地 provider CSV/JSON 账单，只保存汇总金额、账单 hash、窗口和 warning，并与相同窗口的本地账本做差异比较。

## 数据准确性排障

优先运行一键诊断：

```bash
agent-ledger doctor --format markdown
```

也可以打开 `GET /api/doctor?format=markdown&privacy=1`。诊断报告会检查当前时间窗口、collector health、路径是否存在/可读、最近扫描错误、价格新鲜度、未计价模型、空 usage 窗口、canonical-to-usage projection 一致性、control idempotency 健康、workload lease 健康，以及 stale run、policy block 等 workload terminal-state 问题。

如果 Codex、OpenCode 或其他来源没有数据：

- 确认 source 已启用，配置路径真实存在。
- 执行 `POST /api/scan?source=codex` 或 UI 的 Scan Source。
- 查看 `GET /api/health/ingestion`；`last_error` 会明确暴露失败原因。
- Codex collector 会先读取 `sessions/**/*.jsonl`。同时会自动探测相邻的 `state_*.sqlite`，例如 `~/.codex/state_5.sqlite`，用于兼容较新的 Codex 版本，因为它们可能比 JSONL usage event 更频繁更新 SQLite thread state。
- Codex SQLite fallback 会把 `threads.tokens_used` 记录为 `estimated-aggregate`：token 总量会进入账本，但费用重算会故意跳过这些行，因为缺少 input/output/cache 拆分，不能当成逐调用精确账单。
- Docker 部署时只挂载真实存在的 agent 目录。Docker 会把缺失的 host path 创建成 root-owned 目录，可能破坏后续 agent 写入。

如果 KPI 和图表总数不一致：

- Web UI 使用 `GET /api/dashboard` 作为 KPI、token、费用、模型面板的一致性读取入口。
- Data Quality 面板会直接显示 dashboard consistency 问题，包括 metric、expected/actual 差值和 severity。
- 价格变更后执行 `POST /api/recalculate-costs?mode=zero`。
- 如果 Doctor 报告 canonical-to-usage projection 漂移，使用相同 `from`/`to`/`source`/`model`/`project` 范围运行 `agent-ledger projection repair` 或 `POST /api/projections/repair`。该修复是幂等的，会补回缺失投影、对齐 cache/cost 元数据，并重建 aggregates。
- 如果差异持续，运行 `agent-ledger doctor --format markdown`，查看 projection、dashboard consistency 或 pricing warning。

如果费用与 provider 账单不一致：

- 执行 `POST /api/pricing/sync`；如果需要重算历史数据，再执行 `POST /api/pricing/recalculate?mode=all`。
- 企业合同价或第三方中转价应使用本地 pricing override。
- 通过 `POST /api/reconciliation/import` 或 `agent-ledger reconcile import` 导入 provider CSV/JSON 账单；对账只保存汇总金额、hash、窗口和 warning。

## 安全模型

- 默认绑定 `127.0.0.1`。
- `agent-ledger config status`、`GET /api/config/status` 和 MCP `ledger.config_status` 用于部署检查，不暴露原始路径、auth token、API key、webhook URL、机器名、作者、prompt、response 或 session id。
- `agent-ledger readiness`、`GET /api/readiness`、MCP `ledger.readiness` 与 `agent-ledger://readiness` 用于控制面探针，只暴露状态、计数、检查标识和修复建议，包括隐私安全的 control idempotency key/replay 计数、workload queue 可领取状态、workload lease 计数，以及 active/stale agent run 计数。
- `agent-ledger admission check`、`GET /api/admission/check`、MCP `ledger.admission_check` 与 `agent-ledger://admission/check` 只暴露操作访问决策；不暴露 request body、完整 CLI 参数、原始路径、token、prompt、session、项目、分支、机器名或作者。
- 只读取本地 agent 日志和数据库，不上传 usage 数据。
- pricing sync 是默认唯一出站请求。
- 副作用操作默认 localhost-only。
- 可选 RBAC：`viewer`、`operator`、`admin`。
- `rbac.read_only: true` 会把进程切到观测模式：拒绝 REST/CLI/MCP 写操作，关闭后台 collector、pricing sync 和费用重算，并且报告、导出、异常视图等 GET 端点不会追加 audit、budget、insight 或 bundle 记录。
- Policy rule 可匹配 `global`、`source`、`model`、`project`、`repo`、`git_branch`、`team`、`action`、`target` 和 `role`。
- MCP `ledger.get_policy` 在观测模式下仍可做 advisory read；但一旦传入 `workload_id`，该调用会记录 policy decision，因此会按写操作拒绝。
- 策略审批请求只保存本地 metadata。批准后只授权相同 action/target 的重试，不包含 prompt 内容。
- 审批队列隐私模式会在 REST、CLI 与 MCP 中隐藏审批路由元数据和 payload。审批投票审计只记录 quorum/投票事实和 `note_present`，不记录 note 明文。
- 可选 provider gateway 默认关闭。它只在内存中把 prompt content 转发给配置的上游，只从环境变量读取 API key，并只保存 usage 元数据而不是消息内容。
- Run command 会作为 metadata 保存，但常见命令行密钥模式，例如 `API_KEY=...`、`--token ...`、`--api-key=...`、`Bearer ...`，会在持久化前做 best-effort 脱敏。敏感值仍建议使用环境变量或密钥管理器，不要放进长期命令参数。
- 隐私 preset 可隐藏路径、项目、分支、机器名和 session id。
- Webhook 默认关闭，只发送脱敏 workload-event、pending approval 与 approval-route 摘要。
- Offline bundle 是本地 JSON 导出。设置 `AGENT_LEDGER_BUNDLE_KEY` 并使用 `signed=1` / `--signed` 可加入 HMAC-SHA256 签名；导入时使用 `verify=1` / `--verify` 可强制验证签名。

## 开发验证

```bash
go test ./...
go vet ./...
node --check internal/server/static/app.js
docker compose up -d --build
```

主机没有 Go 时：

```bash
docker run --rm -v "$PWD:/src" -w /src golang:1.25.11-alpine sh -c "gofmt -w . && go test ./..."
```

## 发布治理

Release 使用 GoReleaser 构建多平台归档，使用 GitHub Actions 发布 GHCR 镜像。归档产物已配置 Syft SBOM；Docker workflow 会为 GHCR 镜像发布 BuildKit SBOM attestation 和 `mode=max` provenance。发布前请按 [RELEASE.md](RELEASE.md) 核验每个归档的 `.sbom.json`、`checksums.txt`、镜像 digest 的 SBOM/provenance，再对外声明供应链证据。

## Roadmap

已落地基础：canonical workload schema、metadata-only canonical event ingest、机器可读 adapter contract、workload 依赖/lineage links、workload/run 写操作的 retry-safe control operation idempotency、面向异步执行声明的短期 workload lease、异步 run start/heartbeat/liveness 账本、workload terminal-state 派生快照与本地 workload event feed/SSE stream、显式 workload evaluation 信号、默认关闭的 workload 与 approval 脱敏 webhook 通知、隐私安全 discovery manifest、contract bundle index、OpenAPI control-plane contract、runtime status probe、隐私安全 config status probe、control-plane readiness probe、operation admission dry-run、canonical-to-usage projection 与 repair、OpenTelemetry GenAI JSON span mapping、可选本地 OTLP HTTP JSON/protobuf traces receiver、A2A task telemetry mapping、provider usage envelope mapping、可选本地 OpenAI-compatible Chat Completions JSON/SSE、OpenAI Responses JSON/SSE 与 Anthropic Messages JSON/SSE gateway、provider 账单导入对账、model router simulation、preflight cost estimates、session cost replay、repo cost badge、integration capability catalog、signed offline bundle export/import、旧 session 自动 backfill、workload API、workload CSV 导出、本地策略审批请求、quorum-based approval votes、审批路由/升级元数据、审批路由摘要与执行证据、CLI workload/event/policy/router/replay/badge/preflight/projection/config/readiness/admission 命令、CLI run wrapper 和本地 MCP stdio tools/resources/resource-subscriptions/prompts。

后续路线：OTLP gRPC receiver conformance、provider-native gateway adapters、Postgres 团队模式、OIDC/SSO、host client 支持后接入原生 MCP subscription transport、外部审批通知适配器。

## License

Apache-2.0。详见 [LICENSE](LICENSE)。
