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
./agent-ledger run --goal "debug ingestion" --agent codex -- codex
./agent-ledger pricing sync
./agent-ledger wrapped
./agent-ledger mcp
```

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
```

企业合同价、三方中转价、地区倍率和内部折扣请通过 `pricing.overrides` 配置。

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

- `canonical_events`：面向未来 collector、MCP、A2A、gateway 的规范事件流。
- `workloads`、`agent_runs`、`model_calls`、`tool_calls`：goal/run/call 级账本。
- `workload_sessions`：旧 session 与 workload 的兼容映射。
- `artifacts`、`evaluations`、`policy_decisions`、`context_refs`：AgentOps 扩展记录。
- `usage_records`：API 调用级 token 与费用。
- `sessions`：source-scoped 会话元数据。
- `prompt_events`：按时间统计 prompt。
- `pricing`、`pricing_sources`、`pricing_snapshots`：价格规则和价格源健康。
- `hourly_usage_aggregate`、`daily_usage_aggregate`：Dashboard rollup。
- `ingestion_health`、`insight_events`、`audit_log`：运维和质量证据。

## API

常用过滤参数：`from`、`to`、`source`、`model`、`project`、`privacy`。

| Endpoint | 用途 |
|---|---|
| `GET /api/stats` | 总览 |
| `GET /api/workloads` | 服务端分页工作负载账本 |
| `POST /api/workloads` | 创建本地 workload |
| `POST /api/workloads/close` | 关闭 workload 并记录结果 |
| `GET /api/workload-detail` | workload 的 run、model call、tool、session、policy 明细 |
| `GET /api/workload-graph` | workload 图谱 |
| `GET /api/sessions` | 服务端分页会话账本 |
| `GET /api/model-registry` | 模型与价格治理注册表 |
| `GET /api/pricing/status` | 价格源、新鲜度、未计价模型 |
| `POST /api/pricing/sync` | 同步价格 |
| `POST /api/pricing/recalculate?mode=zero|all` | 重算费用 |
| `GET /api/cost-intelligence` | 昂贵会话解释 |
| `GET /api/cache/doctor` | cache 命中、写入、读取诊断 |
| `GET /api/data-quality` | 数据可信度报告 |
| `GET /api/model-calls` | 模型调用次数 |
| `GET /api/quota/status` | 本地 quota 和 burn-rate 估算 |
| `GET /api/anomalies` | 异常检测事件 |
| `GET /api/evidence-bundle` | 脱敏证据包 |
| `GET /api/export?type=workloads&format=csv` | CSV/JSON 导出 |
| `GET /api/report?format=markdown` | Markdown 报告 |

手动扫描、清理重扫、价格同步、导入和费用重算默认只允许本机访问；暴露到网络前必须配置 auth token 或反向代理访问控制。

## MCP 工具接口

`agent-ledger mcp` 会启动本地 stdio JSON-RPC 工具服务，供 agent 框架或 wrapper 接入。当前实现保持本地优先和隐私优先：工具可以创建或关闭 workload、记录 hash 后的 artifact、查询本地策略建议、查询预算状态、解释成本、查找相似 workload。它不会读取 prompt 内容，也不会主动把数据发送到远程 MCP host。

当前工具：

- `ledger.current_budget`
- `ledger.start_workload`
- `ledger.close_workload`
- `ledger.record_artifact`
- `ledger.get_policy`
- `ledger.explain_cost`
- `ledger.find_similar_workloads`

## 安全模型

- 默认绑定 `127.0.0.1`。
- 只读取本地 agent 日志和数据库，不上传 usage 数据。
- pricing sync 是默认唯一出站请求。
- 副作用操作默认 localhost-only。
- 可选 RBAC：`viewer`、`operator`、`admin`。
- 隐私 preset 可隐藏路径、项目、分支、机器名和 session id。
- Webhook 默认关闭，只应发送脱敏摘要。

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

## Roadmap

已落地基础：canonical workload schema、旧 session 自动 backfill、workload API、workload CSV 导出、CLI workload 命令、CLI run wrapper 和本地 MCP stdio tools。

后续路线：A2A task telemetry、OpenTelemetry GenAI mapping、可选 provider/API gateway、Postgres 团队模式、signed offline bundle import、OIDC/SSO、更完整的 MCP resources/prompts、企业策略审批流。

## License

Apache-2.0。详见 [LICENSE](LICENSE)。
