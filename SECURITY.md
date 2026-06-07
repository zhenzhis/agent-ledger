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
- Webhooks are disabled by default.
- Usage data, prompts, local paths, and session IDs are not uploaded by default.

## Sensitive Data

Agent Ledger should not store prompt content, secrets, API keys, webhook URLs, private keys, or raw secret-bearing logs. Audit logs should record operation metadata, not raw prompt or file content.

Use privacy presets before sharing screenshots, reports, evidence bundles, or CSV exports from sensitive workspaces.

## Network Exposure

If binding to `0.0.0.0`, place the service behind a trusted reverse proxy and configure auth tokens or RBAC. Do not expose write endpoints to untrusted networks.

## Supported Versions

Security fixes target the latest `main` branch and the latest released version.
