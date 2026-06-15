package integrations

import "github.com/zhenzhis/agent-ledger/internal/storage"

// IntegrationLockfileReport is the stable metadata-only baseline that adapters,
// wrappers, relays, routers, and internal CI can pin before using the drift
// report to compare future Agent Ledger releases.
type IntegrationLockfileReport struct {
	Product             string            `json:"product"`
	Contract            string            `json:"contract"`
	Version             string            `json:"version"`
	Format              string            `json:"format"`
	LocalFirst          bool              `json:"local_first"`
	ReadOnlySafe        bool              `json:"read_only_safe"`
	WritesLocalState    bool              `json:"writes_local_state"`
	PrivacyPolicy       string            `json:"privacy_policy"`
	LockfileHash        string            `json:"lockfile_hash"`
	HashIDs             []string          `json:"hash_ids"`
	Hashes              map[string]string `json:"hashes"`
	DriftCommand        string            `json:"drift_command"`
	RefreshCommands     []string          `json:"refresh_commands"`
	OperationalGuidance []string          `json:"operational_guidance"`
	RedactionRules      []string          `json:"redaction_rules"`
}

func IntegrationLockfileFor(opts Options, runtime *storage.RuntimeStatus) IntegrationLockfileReport {
	report := IntegrationLockfileReport{
		Product:          "Agent Ledger",
		Contract:         "agent-ledger.integration-lockfile",
		Version:          "v1",
		Format:           "agent-ledger.integration-lockfile.v1",
		LocalFirst:       true,
		ReadOnlySafe:     true,
		WritesLocalState: false,
		PrivacyPolicy:    "Integration lockfiles contain static control-plane hashes only. They exclude SQLite usage rows, fixture bodies, prompt content, response content, message history, local paths, secrets, account identifiers, machine names, authors, native session identifiers, and webhook URLs.",
		HashIDs:          IntegrationDriftHashIDs(),
		Hashes:           IntegrationDriftCurrentHashes(opts, runtime),
		DriftCommand:     "agent-ledger integrations drift --strict",
		RefreshCommands: []string{
			"agent-ledger integrations lockfile",
			"agent-ledger integrations drift --strict",
			"agent-ledger contracts verify",
			"agent-ledger integrations evidence-kit",
		},
		OperationalGuidance: []string{
			"commit this lockfile with adapter, wrapper, or router release artifacts after conformance and contract verification pass",
			"rerun the drift report before upgrading Agent Ledger or enabling a new integration surface",
			"treat strict drift failures as release blockers until conformance, pricing, policy, and rollout evidence is refreshed",
			"do not edit hashes manually; regenerate the lockfile from a verified Agent Ledger binary",
		},
		RedactionRules: []string{
			"lockfiles may be shared in CI artifacts because they contain only sha256 hashes, contract ids, and commands",
			"do not attach prompts, responses, transcripts, raw headers, credentials, webhook URLs, local paths, account names, machine names, authors, native session ids, or provider account ids",
			"pair lockfile changes with evidence-kit output when a release requires human review",
		},
	}
	report.LockfileHash = IntegrationLockfileFingerprintFrom(report)
	return report
}

func IntegrationLockfileFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	return IntegrationLockfileFingerprintFrom(IntegrationLockfileFor(opts, runtime))
}

// IntegrationLockfileOpenAPIFingerprint is a non-recursive witness hash for
// discovery, OpenAPI metadata, and contract-bundle indexes. It keeps contract
// verification fast while the full endpoint still returns an ETag based on the
// complete lockfile payload.
func IntegrationLockfileOpenAPIFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	return hashJSONPayload(map[string]interface{}{
		"contract":                "agent-ledger.integration-lockfile",
		"version":                 "v1",
		"format":                  "agent-ledger.integration-lockfile.v1",
		"default_uri":             "/api/integrations/lockfile",
		"accepted_hash_ids":       IntegrationDriftHashIDs(),
		"capability_catalog_hash": CatalogFingerprint(opts),
		"adapter_spec_hash":       AdapterContractFingerprint(),
		"canonical_schema_hash":   storage.CanonicalEventSchemaFingerprint(),
		"runtime_status_hash":     hashJSONPayload(runtime),
		"privacy":                 "metadata-only OpenAPI witness; full endpoint ETag is returned by /api/integrations/lockfile",
	})
}

func IntegrationLockfileFingerprintFrom(report IntegrationLockfileReport) string {
	report.LockfileHash = ""
	return hashJSONPayload(report)
}
