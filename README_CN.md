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
- 支持本地审计日志、隐私 preset、导出、Markdown 报告、证据包、团队 showback。
- 单 Go 二进制，内嵌静态 UI，SQLite 存储。

## 快速开始

```bash
git clone https://github.com/zhenzhis/agent-ledger.git
cd agent-ledger
go build -o agent-ledger .
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
./agent-ledger workload create --goal "review strategy engine" --source codex --project quant
./agent-ledger workload start-run --workload-id wl_... --source codex --agent-name codex
./agent-ledger workload heartbeat --run-id run_... --status working --phase testing --progress 0.5
./agent-ledger workload liveness --max-age 10m --stale-only
./agent-ledger workload state --workload-id wl_... --max-age 10m
./agent-ledger workload feed --severity warning --max-age 10m
./agent-ledger workload evaluation --workload-id wl_... --run-id run_... --status pass --score 0.97 --signal unit-tests
./agent-ledger run --goal "debug ingestion" --agent codex -- codex
./agent-ledger event schema
./agent-ledger event examples --type model.call
./agent-ledger event validate --file event.json
./agent-ledger event ingest --file event.json
./agent-ledger adapter conformance --kind provider --strict --file fixture.json
./agent-ledger discovery
./agent-ledger integrations
./agent-ledger runtime
./agent-ledger notify webhook --dry-run --severity warning
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
./agent-ledger policy approvals
./agent-ledger policy enforcement --privacy
./agent-ledger policy resolve --id apr_... --status approved
./agent-ledger audit --action pricing --role operator --format markdown --privacy
./agent-ledger pricing sync
./agent-ledger wrapped
./agent-ledger mcp
```

仓库内 `examples/adapter-fixtures/` 提供 canonical events、provider usage envelope、OpenTelemetry GenAI span 与 A2A task snapshot 的 strict conformance 样例。

## 配置

配置搜索顺序：

1. `--config path/to/config.yaml`
2. `/etc/agent-ledger/config.yaml`
3. `./config.yaml`

核心配置：

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
  include_stream_usage: true
  max_body_bytes: 4194304
  max_response_bytes: 33554432
  timeout: 120s
```

企业合同价、三方中转价、地区倍率和内部折扣请通过 `pricing.overrides` 配置。

可选 gateway 是本地 OpenAI-compatible Chat Completions 代理。它默认关闭，支持 JSON 响应和 SSE streaming，只从配置的环境变量读取上游 API key，并只记录 token usage 与审计元数据，不保存 request messages 或 response content。对 streaming 请求，`include_stream_usage: true` 会在客户端没有显式设置 `stream_options.include_usage` 时请求兼容上游返回最终 usage chunk；如果三方中转拒绝该选项，可设为 `false`。

Webhook 通知默认关闭。显式开启后，`POST /api/notifications/webhook` 与 `agent-ledger notify webhook` 只发送有上限的 workload-event 脱敏摘要；goal、project、repo、branch、team、event id、workload id 都会被隐藏或 hash。可用 `--dry-run` 或 `dry_run=1` 检查即将发送的 payload，不进行外发。

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

1. 本地 override。
2. OpenAI/Anthropic 官方 seed。
3. LiteLLM fallback。
4. OpenCode 等来源自带费用，默认保留为该来源事实。

每条记录可追踪价格来源、匹配模型、匹配方式和 confidence。未知价格、过期价格和 fuzzy 匹配会进入数据质量中心，不会被静默隐藏。

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
- `workload_sessions`：旧 session 与 workload 的兼容映射。
- `context_refs`、`artifacts`、`evaluations`、`policy_decisions`：隐私安全的 AgentOps 上下文、产物、结果和治理记录。
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
| `GET /api/runtime/status` | 运行模式、只读状态、后台任务与写操作状态 |
| `GET /api/dashboard` | Web dashboard 的一致性 KPI、token、费用与模型数据包 |
| `GET /api/stats` | 总览 |
| `GET /api/workloads` | 服务端分页工作负载账本 |
| `POST /api/workloads` | 创建本地 workload |
| `POST /api/workloads/close` | 关闭 workload 并记录结果 |
| `POST /api/agent-runs` | 在已有 workload 下启动一个 run |
| `POST /api/agent-runs/heartbeat` | 写入 metadata-only 异步 run 存活/进度心跳 |
| `GET /api/agent-runs/liveness` | 列出 active run 与心跳 stale 状态 |
| `GET /api/workload-detail` | workload 的 run、model call、tool、session、policy 明细 |
| `GET /api/workload-graph` | workload 图谱 |
| `GET /api/workload-timeline` | 按时间排序的 workload 审计时间线 |
| `GET /api/workload-state` | 单个异步 agent workload 的 terminal-state 派生快照 |
| `GET /api/workload-events` | 面向 monitor、router、通知适配器的本地 workload 状态事件 feed |
| `GET /api/workload-events/stream` | 面向本地轮询 monitor 与 router subscription 的 SSE workload 状态流 |
| `POST /api/notifications/webhook` | 显式发送脱敏 workload-event 摘要到配置的 webhook |
| `GET /api/integrations` | 隐私安全的集成能力目录 |
| `POST /api/integrations/conformance` | 校验 canonical、provider、OpenTelemetry GenAI 或 A2A adapter fixture，但不写入 SQLite；`strict=true` 会把 provenance warning 视为失败 |
| `GET /api/event-schema` | Canonical event schema 与支持的事件类型 |
| `GET /api/event-examples` | 隐私安全的 canonical event 模板，可用 `type` 或 `event_type` 过滤 |
| `POST /api/events/validate` | 校验 canonical metadata-only events，但不写入 SQLite |
| `POST /api/events` | 写入 metadata-only canonical events |
| `POST /api/otel/genai` | 将 OpenTelemetry GenAI JSON span 转成 canonical model-call events |
| `POST /v1/traces` | 显式开启 `integrations.otlp_receiver.enabled` 后可用的本地 OTLP HTTP/JSON traces receiver |
| `POST /api/otlp/v1/traces` | 同一 receiver 的 API 命名空间路径，便于本地反向代理 |
| `POST /api/a2a/tasks` | 将 A2A JSON task snapshot/event 转成 workload/run/artifact/evaluation events |
| `POST /api/provider/calls` | 将 provider response usage envelope 转成 canonical model-call events |
| `POST /gateway/openai/v1/chat/completions` | 显式开启 `gateway.enabled` 后可用的 JSON/SSE OpenAI-compatible gateway |
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
| `GET /api/policy/approvals?status=pending` | 查看本地 pending、approved、rejected 或全部策略审批请求 |
| `POST /api/policy/approvals` | 批准或拒绝本地策略审批请求 |
| `GET /api/audit-log?action=pricing&role=operator` | 过滤本地操作审计事件；支持 `from`、`to`、`actor`、`role`、`action`、`target`、`limit` 与隐私模式 |
| `GET /api/sessions` | 服务端分页会话账本 |
| `GET /api/session-replay?source=codex&session_id=...` | 单个 session 的调用级 token/cost 时间回放 |
| `GET /api/badge/repo.svg?project=repo-name&metric=cost` | 本地 SVG repo 成本、token 或 cache badge |
| `GET /api/model-registry` | 模型与价格治理注册表 |
| `GET /api/pricing/status` | 价格源、新鲜度、有效规则摘要、未计价模型 |
| `POST /api/pricing/sync` | 同步价格 |
| `POST /api/pricing/recalculate?mode=zero|all` | 重算费用 |
| `POST /api/projections/repair` | 修复 canonical `model_calls` 到 `usage_records` 的投影漂移，并重建 aggregates |
| `GET /api/cost-intelligence` | 昂贵会话解释 |
| `GET /api/cache/doctor` | cache 命中、写入、读取诊断 |
| `GET /api/doctor?format=markdown` | 一键本地诊断 usage、采集、价格、数据质量与 workload 状态 |
| `GET /api/data-quality` | 数据可信度报告 |
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

当策略返回 `require_approval` 时，Agent Ledger 会写入本地 pending 审批请求并返回 id。Admin 可通过 `POST /api/policy/approvals` 或 `agent-ledger policy resolve` 批准/拒绝；原操作之后可带 `approval_id=<id>` 或 `X-Agent-Ledger-Approval: <id>` 重试。

## MCP 工具接口

`agent-ledger mcp` 会启动本地 stdio JSON-RPC 工具服务，供 agent 框架或 wrapper 接入。当前实现保持本地优先和隐私优先：工具可以创建或关闭 workload、在已有 workload 下启动 run、写入 run heartbeat、查询 run liveness 与 terminal-state 快照、记录 tool-call 元数据、context ref、hash 后的 artifact 与质量/evaluation 信号、查询本地策略建议、查询预算状态、解释成本、查找相似 workload。Resources 提供 metadata-only 的 schema、integration、budget、workload、terminal-state、policy 上下文；prompts 提供可复用的 workload、成本复盘、证据包模板。它不会读取 prompt 内容，也不会主动把数据发送到远程 MCP host。MCP、REST 与 CLI 的 policy evaluation 共用同一个本地 evaluator，确保不同接入方式得到一致的 advisory 决策。

`GET /api/integrations`、`GET /.well-known/agent-ledger.json`、`agent-ledger integrations` 和 MCP `ledger.integrations` 会暴露运行时能力字段：`writes_local_state`、`available_in_read_only`、`runtime_status`。Discovery manifest 还会以一等字段暴露 `runtime_status_uri`、`canonical_schema_uri`、`canonical_schema_hash`、`event_examples_uri`、`adapter_conformance_uri`，便于轻量 wrapper 自动接入。`GET /api/runtime/status` 与 `agent-ledger runtime` 提供同一个进程级 observer/control-plane 状态，适合探针使用。Agent router 和 wrapper 应读取这些字段，而不是硬编码 endpoint 假设，尤其是在启用 `rbac.read_only` 时。

当前工具：

- `ledger.current_budget`
- `ledger.start_workload`
- `ledger.start_run`
- `ledger.close_workload`
- `ledger.heartbeat_run`
- `ledger.run_liveness`
- `ledger.workload_timeline`
- `ledger.workload_state`
- `ledger.record_tool_call`
- `ledger.record_context`
- `ledger.record_artifact`
- `ledger.record_evaluation`
- `ledger.record_event`
- `ledger.validate_event`
- `ledger.event_schema`
- `ledger.event_examples`
- `ledger.adapter_conformance`
- `ledger.integrations`
- `ledger.get_policy`
- `ledger.policy_audit`
- `ledger.audit_log`
- `ledger.explain_cost`
- `ledger.find_similar_workloads`

当前 resources：

- `agent-ledger://schema/canonical-events`
- `agent-ledger://schema/canonical-event-examples`
- `agent-ledger://integrations/catalog`
- `agent-ledger://budget/current`
- `agent-ledger://workloads/recent`，包含 workload summary rows 与派生 terminal-state snapshots
- `agent-ledger://policies/status`

当前 prompts：

- `agent-ledger/workload-brief`
- `agent-ledger/cost-review`
- `agent-ledger/incident-evidence`

Canonical event ingest 支持 workload、run、run heartbeat、model call、tool call、context ref、artifact、evaluation、policy decision 事件。Payload 只允许元数据；如果出现 raw prompt/content 相关键会直接失败，不会静默持久化。当前 event-envelope contract 只接受 `schema_version: "v1"`；未知版本会明确校验失败。`GET /api/event-examples`、`agent-ledger event examples` 与 MCP `ledger.event_examples` 会返回每类事件的隐私安全模板。`POST /api/events/validate` 与 `agent-ledger event validate` 会执行同一套契约校验，但不写入 SQLite，适合直接 canonical event 检查。`POST /api/integrations/conformance` 与 `agent-ledger adapter conformance` 还会先转换 provider、OpenTelemetry GenAI 与 A2A fixture 再校验，让 wrapper CI 在开启 ingest 前证明兼容性。Envelope 同时携带 `schema_version`、`source_version`、`parser_version`、`raw_ref`、`match_type` 等 adapter provenance 字段，让未来 collector 能说明事件是 exact、estimated、reconstructed、source-reported 还是 fuzzy，同时不保存 prompt 内容。Canonical `model.call` 也会投影到 `usage_records`，让 dashboard、预算、导出和 preflight 尽量使用同一个 token 来源。`agent.run.heartbeat` 会写入存活/进度时间线并更新 run 快照，使长时间异步 agent 可被监控而不需要读取 prompt；liveness 查询会标记心跳或启动时间超过阈值的 active run。`GET /api/integrations`、`agent-ledger integrations` 与 `ledger.integrations` 会暴露当前 connector/protocol 能力目录，但不会泄露本地 source 原始路径。`POST /api/otel/genai` 与 `agent-ledger otel ingest` 支持 OpenTelemetry GenAI JSON span，并只保留经过挑选的元数据和 token 字段。显式开启后，`POST /v1/traces` 与 `POST /api/otlp/v1/traces` 可接收 OTLP HTTP/JSON trace batch，并有 body 与 span 数量上限；OTLP protobuf/gRPC 在加入 conformance tests 前会被明确拒绝。`POST /api/a2a/tasks` 与 `agent-ledger a2a ingest` 支持 A2A task snapshot/event，只保留任务生命周期元数据，不保存 message/history/artifact part 内容。`POST /api/provider/calls` 与 `agent-ledger provider ingest` 支持 OpenAI-compatible、Anthropic-style、LiteLLM-style usage envelope，不保存 request/response message 内容。显式开启后，`POST /gateway/openai/v1/chat/completions` 会在内存中代理 OpenAI-compatible JSON 或 SSE streaming 请求，执行本地 policy 检查，写入 usage/audit 元数据，并通过 headers/trailers 暴露记账状态。`POST /api/reconciliation/import` 与 `agent-ledger reconcile import` 支持导入本地 provider CSV/JSON 账单，只保存汇总金额、账单 hash、窗口和 warning，并与相同窗口的本地账本做差异比较。

## 数据准确性排障

优先运行一键诊断：

```bash
agent-ledger doctor --format markdown
```

也可以打开 `GET /api/doctor?format=markdown&privacy=1`。诊断报告会检查当前时间窗口、collector health、路径是否存在/可读、最近扫描错误、价格新鲜度、未计价模型、空 usage 窗口、canonical-to-usage projection 一致性，以及 stale run、policy block 等 workload terminal-state 问题。

如果 Codex、OpenCode 或其他来源没有数据：

- 确认 source 已启用，配置路径真实存在。
- 执行 `POST /api/scan?source=codex` 或 UI 的 Scan Source。
- 查看 `GET /api/health/ingestion`；`last_error` 会明确暴露失败原因。
- Docker 部署时只挂载真实存在的 agent 目录。Docker 会把缺失的 host path 创建成 root-owned 目录，可能破坏后续 agent 写入。

如果 KPI 和图表总数不一致：

- Web UI 使用 `GET /api/dashboard` 作为 KPI、token、费用、模型面板的一致性读取入口。
- 价格变更后执行 `POST /api/recalculate-costs?mode=zero`。
- 如果 Doctor 报告 canonical-to-usage projection 漂移，使用相同 `from`/`to`/`source`/`model`/`project` 范围运行 `agent-ledger projection repair` 或 `POST /api/projections/repair`。该修复是幂等的，会补回缺失投影、对齐 cache/cost 元数据，并重建 aggregates。
- 如果差异持续，运行 `agent-ledger doctor --format markdown`，查看 projection、dashboard consistency 或 pricing warning。

如果费用与 provider 账单不一致：

- 执行 `POST /api/pricing/sync`；如果需要重算历史数据，再执行 `POST /api/pricing/recalculate?mode=all`。
- 企业合同价或第三方中转价应使用本地 pricing override。
- 通过 `POST /api/reconciliation/import` 或 `agent-ledger reconcile import` 导入 provider CSV/JSON 账单；对账只保存汇总金额、hash、窗口和 warning。

## 安全模型

- 默认绑定 `127.0.0.1`。
- 只读取本地 agent 日志和数据库，不上传 usage 数据。
- pricing sync 是默认唯一出站请求。
- 副作用操作默认 localhost-only。
- 可选 RBAC：`viewer`、`operator`、`admin`。
- `rbac.read_only: true` 会把进程切到观测模式：拒绝 REST/CLI 写操作，关闭后台 collector、pricing sync 和费用重算，并且报告、导出、异常视图等 GET 端点不会追加 audit、budget、insight 或 bundle 记录。
- Policy rule 可匹配 `global`、`source`、`model`、`project`、`repo`、`git_branch`、`team`、`action`、`target` 和 `role`。
- 策略审批请求只保存本地 metadata。批准后只授权相同 action/target 的重试，不包含 prompt 内容。
- 可选 provider gateway 默认关闭。它只在内存中把 prompt content 转发给配置的上游，只从环境变量读取 API key，并只保存 usage 元数据而不是消息内容。
- Run command 会作为 metadata 保存，但常见命令行密钥模式，例如 `API_KEY=...`、`--token ...`、`--api-key=...`、`Bearer ...`，会在持久化前做 best-effort 脱敏。敏感值仍建议使用环境变量或密钥管理器，不要放进长期命令参数。
- 隐私 preset 可隐藏路径、项目、分支、机器名和 session id。
- Webhook 默认关闭，只应发送脱敏摘要。
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

已落地基础：canonical workload schema、metadata-only canonical event ingest、异步 run start/heartbeat/liveness 账本、workload terminal-state 派生快照与本地 workload event feed/SSE stream、显式 workload evaluation 信号、默认关闭的脱敏 webhook 通知、隐私安全 discovery manifest、canonical-to-usage projection 与 repair、OpenTelemetry GenAI JSON span mapping、可选本地 OTLP HTTP/JSON traces receiver、A2A task telemetry mapping、provider usage envelope mapping、可选 JSON/SSE 本地 OpenAI-compatible gateway、provider 账单导入对账、model router simulation、preflight cost estimates、session cost replay、repo cost badge、integration capability catalog、signed offline bundle export/import、旧 session 自动 backfill、workload API、workload CSV 导出、本地策略审批请求与执行证据、CLI workload/event/policy/router/replay/badge/preflight/projection 命令、CLI run wrapper 和本地 MCP stdio tools/resources/prompts。

后续路线：OTLP protobuf/gRPC conformance、provider-native gateway adapters、Postgres 团队模式、OIDC/SSO、更完整的 MCP subscriptions、多操作者审批通知。

## License

Apache-2.0。详见 [LICENSE](LICENSE)。
