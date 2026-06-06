# Agent Ledger

Agent Ledger 是本地优先的 AI Agent FinOps、额度、价格、审计与生产力洞察控制台，支持 Claude Code、Codex、OpenCode、OpenClaw、kiro、Pi 等本地 coding agent。

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
./agent-ledger pricing sync
./agent-ledger wrapped
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
collectors -> SQLite raw usage -> pricing governance -> cost recalculation
           -> aggregate tables -> REST API -> embedded dashboard / CLI
```

核心表：

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
| `GET /api/sessions` | 服务端分页会话账本 |
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
| `GET /api/export?type=sessions&format=csv` | CSV/JSON 导出 |
| `GET /api/report?format=markdown` | Markdown 报告 |

手动扫描、清理重扫、价格同步、导入和费用重算默认只允许本机访问；暴露到网络前必须配置 auth token 或反向代理访问控制。

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

## License

Apache-2.0。详见 [LICENSE](LICENSE)。
