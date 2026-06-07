# Agent Ledger Release Process

## Versioning

Use semantic versions: `vMAJOR.MINOR.PATCH`.

## Pre-Release Checks

```bash
go test ./...
go vet ./...
node --check internal/server/static/app.js
git diff --check
docker compose up -d --build
printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n' | ./agent-ledger mcp
```

If available, also run:

```bash
govulncheck ./...
```

## Artifacts

GoReleaser builds:

| Platform | Architecture | Artifact |
|---|---|---|
| Linux | amd64 | `agent-ledger_<ver>_linux_amd64.tar.gz` |
| Linux | arm64 | `agent-ledger_<ver>_linux_arm64.tar.gz` |
| macOS | amd64 | `agent-ledger_<ver>_darwin_amd64.tar.gz` |
| macOS | arm64 | `agent-ledger_<ver>_darwin_arm64.tar.gz` |
| Windows | amd64 | `agent-ledger_<ver>_windows_amd64.zip` |
| Windows | arm64 | `agent-ledger_<ver>_windows_arm64.zip` |

GHCR image target:

```text
ghcr.io/zhenzhis/agent-ledger:<version>
```

## Release Notes

Release notes must include:

- Summary of user-facing changes.
- Pricing source or cost algorithm changes.
- Database migrations.
- Security or privacy changes.
- Known limitations and planned items.

## SBOM And Provenance

SBOM/provenance are planned release-governance items unless the active GoReleaser and CI workflow explicitly produce them in the release run. Do not claim SBOM availability before verifying generated artifacts.
