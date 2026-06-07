# Adapter Fixtures

Privacy-safe fixtures for adapter CI and local wrapper development.

Validate them with:

```bash
agent-ledger adapter conformance --kind canonical --strict --file examples/adapter-fixtures/canonical-workload.json
agent-ledger adapter conformance --kind provider --strict --file examples/adapter-fixtures/provider-openai-response.json
agent-ledger adapter conformance --kind otel --strict --file examples/adapter-fixtures/otel-genai-span.json
agent-ledger adapter conformance --kind a2a --strict --file examples/adapter-fixtures/a2a-task.json
```

These examples intentionally contain metadata, counters, hashes, and lifecycle fields only. Do not add prompt, response, message, transcript, or raw artifact content to fixtures.
