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
./agent-ledger event schema
./agent-ledger integrations
printf '{"trace_id":"t","span_id":"s","attributes":{"gen_ai.request.model":"gpt-5.5","gen_ai.usage.input_tokens":10,"gen_ai.usage.output_tokens":5,"agent_ledger.goal":"release smoke"}}\n' | ./agent-ledger otel convert
printf '{"id":"task-release","contextId":"ctx-release","status":{"state":"completed"},"metadata":{"agent_ledger.goal":"release smoke"}}\n' | ./agent-ledger a2a convert
printf '{"id":"resp-release","provider":"openai","model":"gpt-5.5","usage":{"input_tokens":10,"output_tokens":5},"metadata":{"agent_ledger.goal":"release smoke"}}\n' | ./agent-ledger provider convert
printf 'provider,date,currency,amount_usd\nopenai,2026-06-06,USD,1.25\n' | ./agent-ledger reconcile parse --format csv
printf '{"jsonrpc":"2.0","id":1,"method":"tools/list"}\n' | ./agent-ledger mcp
printf '{"source":"local","event_type":"workload.started","payload":{"goal":"release smoke"}}\n' | ./agent-ledger event ingest
AGENT_LEDGER_BUNDLE_KEY=test-key ./agent-ledger bundle export --signed --privacy > /tmp/agent-ledger-bundle.json
AGENT_LEDGER_BUNDLE_KEY=test-key ./agent-ledger bundle import --verify < /tmp/agent-ledger-bundle.json
./agent-ledger policy evaluate --model gpt-5.5 --action model.call
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

The release pipeline is configured to produce supply-chain evidence:

- GHCR images are built by `.github/workflows/docker.yml` with BuildKit SBOM attestations and `mode=max` provenance.
- GoReleaser archives are cataloged by Syft through the `sboms` section in `.goreleaser.yaml`; the release workflow installs Syft before running GoReleaser.

Release verification must confirm:

- The GitHub Release contains one `.sbom.json` file for each published archive.
- `checksums.txt` covers the release archives.
- GHCR shows SBOM/provenance attestations for the pushed image digest.
- Release notes do not claim signing, SLSA level, or vulnerability attestation unless those artifacts are present in the release run.
