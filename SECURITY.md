# Security Policy

## Reporting

Report vulnerabilities through GitHub Security Advisories:

https://github.com/zhenzhis/agent-ledger/security/advisories/new

Do not disclose exploitable issues publicly until a fix is available.

## Security Model

Agent Ledger runs locally and reads local agent usage files. The default deployment is intentionally private:

- HTTP bind address is `127.0.0.1`.
- SQLite storage is local.
- Pricing sync is the expected outbound request.
- Manual scan, reset, pricing sync, import, and recalculation are localhost-only unless auth is configured.
- `agent-ledger mcp` is a local stdio tool surface. Treat the launching agent as the operator and do not connect it to untrusted hosts without sandboxing and policy review.
- MCP resources and prompts expose metadata-only local context and templates; they must not be extended to include raw prompts, transcripts, secrets, or file contents.
- The OTLP HTTP/JSON receiver is disabled by default, rejects OTLP protobuf/gRPC, and still requires localhost access or configured auth.
- The optional OpenAI-compatible provider gateway is disabled by default. It forwards request bodies only in memory to the configured upstream, reads API keys only from environment variables, and records usage/audit metadata instead of prompt or response content.
- Local policy evaluation is advisory unless your wrapper or gateway enforces it. Policy decisions record rule metadata, role, workload ID, and action, but must not record prompt text, secrets, or raw tool output.
- Policy approval requests are local metadata records and authorize only matching action/target retries through an explicit approval id.
- Webhooks are disabled by default.
- Usage data, prompts, local paths, and session IDs are not uploaded by default.

## Sensitive Data

Agent Ledger should not store prompt content, secrets, API keys, webhook URLs, private keys, or raw secret-bearing logs. Audit logs should record operation metadata, not raw prompt or file content.

Canonical event ingest accepts metadata only and rejects obvious raw prompt/content payload keys. Canonical model-call projection writes token metadata into `usage_records`; it must not include prompt text, transcripts, model output, or API keys. Integrations should send hashes, IDs, counts, timings, and status instead of raw prompts, transcripts, or model output.

Offline bundles are local JSON files. They always include a payload SHA-256 hash and can include an HMAC-SHA256 signature when `AGENT_LEDGER_BUNDLE_KEY` is set. Do not put signing keys in config files, reports, screenshots, or durable logs.

Use privacy presets before sharing screenshots, reports, evidence bundles, or CSV exports from sensitive workspaces.

## Network Exposure

If binding to `0.0.0.0`, place the service behind a trusted reverse proxy and configure auth tokens or RBAC. Do not expose write endpoints to untrusted networks.

## Supported Versions

Security fixes target the latest `main` branch and the latest released version.
