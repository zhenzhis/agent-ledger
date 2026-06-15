package integrations

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// IntegrationUpgradeGateRequest carries a pinned hash baseline that adapter,
// wrapper, router, relay, or CI jobs want to evaluate before upgrading Agent
// Ledger or enabling a new integration surface.
type IntegrationUpgradeGateRequest struct {
	Strict   bool              `json:"strict"`
	Expected map[string]string `json:"expected,omitempty"`
}

// IntegrationUpgradeGateReport turns lockfile/drift metadata into a CI-friendly
// pass/review/block decision without reading local usage rows, fixture bodies,
// prompts, responses, local paths, or secrets.
type IntegrationUpgradeGateReport struct {
	Product             string                         `json:"product"`
	Contract            string                         `json:"contract"`
	Version             string                         `json:"version"`
	LocalFirst          bool                           `json:"local_first"`
	ReadOnlySafe        bool                           `json:"read_only_safe"`
	WritesLocalState    bool                           `json:"writes_local_state"`
	PrivacyPolicy       string                         `json:"privacy_policy"`
	Request             IntegrationUpgradeGateRequest  `json:"request"`
	GateHash            string                         `json:"gate_hash"`
	Decision            IntegrationUpgradeGateDecision `json:"decision"`
	DriftSummary        IntegrationDriftSummary        `json:"drift_summary"`
	Checks              []IntegrationUpgradeGateCheck  `json:"checks"`
	BlockingRows        []IntegrationDriftRow          `json:"blocking_rows"`
	ReviewRows          []IntegrationDriftRow          `json:"review_rows"`
	CICommands          []string                       `json:"ci_commands"`
	RequiredArtifacts   []string                       `json:"required_artifacts"`
	OperationalGuidance []string                       `json:"operational_guidance"`
	RedactionRules      []string                       `json:"redaction_rules"`
}

type IntegrationUpgradeGateDecision struct {
	Status                  string `json:"status"`
	Severity                string `json:"severity"`
	Reason                  string `json:"reason"`
	RecommendedCIExitCode   int    `json:"recommended_ci_exit_code"`
	AllowWriteIngest        bool   `json:"allow_write_ingest"`
	RequiresHumanReview     bool   `json:"requires_human_review"`
	RequiresEvidenceRefresh bool   `json:"requires_evidence_refresh"`
}

type IntegrationUpgradeGateCheck struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Evidence    string `json:"evidence"`
	Remediation string `json:"remediation"`
	Privacy     string `json:"privacy"`
}

func IntegrationUpgradeGateFromValues(values url.Values) IntegrationUpgradeGateRequest {
	drift := IntegrationDriftFromValues(values)
	return NormalizeIntegrationUpgradeGateRequest(IntegrationUpgradeGateRequest{
		Strict:   drift.Strict,
		Expected: drift.Expected,
	})
}

func NormalizeIntegrationUpgradeGateRequest(req IntegrationUpgradeGateRequest) IntegrationUpgradeGateRequest {
	drift := NormalizeIntegrationDriftRequest(IntegrationDriftRequest{
		Strict:   req.Strict,
		Expected: req.Expected,
	})
	return IntegrationUpgradeGateRequest{
		Strict:   drift.Strict,
		Expected: drift.Expected,
	}
}

func IntegrationUpgradeGateFor(opts Options, runtime *storage.RuntimeStatus, req IntegrationUpgradeGateRequest) IntegrationUpgradeGateReport {
	req = NormalizeIntegrationUpgradeGateRequest(req)
	drift := IntegrationDriftReportFor(opts, runtime, IntegrationDriftRequest{Strict: req.Strict, Expected: req.Expected})
	checks := integrationUpgradeGateChecks(drift, runtime, req)
	blockingRows, reviewRows := integrationUpgradeGateRows(drift, req)
	decision := summarizeIntegrationUpgradeGate(checks, drift, req)
	report := IntegrationUpgradeGateReport{
		Product:          "Agent Ledger",
		Contract:         "agent-ledger.integration-upgrade-gate",
		Version:          "v1",
		LocalFirst:       true,
		ReadOnlySafe:     true,
		WritesLocalState: false,
		PrivacyPolicy:    "Integration upgrade gates evaluate static control-plane hashes, drift status, and release commands only. They exclude SQLite usage rows, fixture bodies, prompt content, response content, message history, local paths, secrets, account identifiers, machine names, authors, native session identifiers, and webhook URLs.",
		Request:          req,
		Decision:         decision,
		DriftSummary:     drift.Summary,
		Checks:           checks,
		BlockingRows:     blockingRows,
		ReviewRows:       reviewRows,
		CICommands: []string{
			"agent-ledger integrations lockfile",
			"agent-ledger integrations drift --strict",
			"agent-ledger integrations upgrade-gate --strict",
			"agent-ledger integrations evidence-kit",
			"agent-ledger contracts verify",
		},
		RequiredArtifacts: []string{
			"integration lockfile JSON generated from a verified Agent Ledger binary",
			"strict drift report for the pinned release baseline",
			"contract verification report with failed=0",
			"adapter conformance output for every enabled ingest surface",
			"privacy review confirmation when write ingest, gateway, webhook, or provider-profile changes are enabled",
		},
		OperationalGuidance: []string{
			"use pass status for automated release continuation",
			"use review status to require human approval before enabling write ingest or provider gateway surfaces",
			"use block status as a release stopper until drift, missing lockfile pins, or unknown hash ids are resolved",
			"regenerate lockfiles after conformance, pricing, policy, and evidence-kit checks pass",
		},
		RedactionRules: []string{
			"attach only hashes, contract ids, endpoint names, command names, check ids, and redacted evidence summaries to CI logs",
			"do not attach prompts, responses, transcripts, raw headers, credentials, webhook URLs, local paths, account names, machine names, authors, native session ids, or provider account ids",
			"share evidence-kit metadata instead of fixture bodies when a gate requires review",
		},
	}
	report.GateHash = IntegrationUpgradeGateFingerprintFrom(report)
	return report
}

func IntegrationUpgradeGateFingerprint(opts Options, runtime *storage.RuntimeStatus, req IntegrationUpgradeGateRequest) string {
	return IntegrationUpgradeGateFingerprintFrom(IntegrationUpgradeGateFor(opts, runtime, req))
}

// IntegrationUpgradeGateOpenAPIFingerprint is a non-recursive witness hash for
// discovery, OpenAPI metadata, and contract-bundle indexes.
func IntegrationUpgradeGateOpenAPIFingerprint(opts Options, runtime *storage.RuntimeStatus) string {
	if runtime == nil {
		runtime = defaultRuntimeStatus(opts)
	}
	return hashJSONPayload(map[string]interface{}{
		"contract":                  "agent-ledger.integration-upgrade-gate",
		"version":                   "v1",
		"default_uri":               "/api/integrations/upgrade-gate",
		"accepted_hash_ids":         IntegrationDriftHashIDs(),
		"integration_drift_hash":    IntegrationDriftOpenAPIFingerprint(opts, runtime),
		"integration_lockfile_hash": IntegrationLockfileOpenAPIFingerprint(opts, runtime),
		"adapter_spec_hash":         AdapterContractFingerprint(),
		"canonical_schema_hash":     storage.CanonicalEventSchemaFingerprint(),
		"runtime_status_hash":       hashJSONPayload(runtime),
		"privacy":                   "metadata-only OpenAPI witness; full endpoint ETag is returned by /api/integrations/upgrade-gate",
	})
}

func IntegrationUpgradeGateFingerprintFrom(report IntegrationUpgradeGateReport) string {
	report.GateHash = ""
	return hashJSONPayload(report)
}

func integrationUpgradeGateChecks(drift IntegrationDriftReport, runtime *storage.RuntimeStatus, req IntegrationUpgradeGateRequest) []IntegrationUpgradeGateCheck {
	checks := []IntegrationUpgradeGateCheck{
		{
			ID:          "metadata_only",
			Status:      "pass",
			Severity:    "info",
			Message:     "upgrade gate is metadata-only and read-only",
			Evidence:    "writes_local_state=false, read_only_safe=true",
			Remediation: "keep prompt, response, transcript, raw header, local path, and secret material out of CI artifacts",
			Privacy:     "no usage rows, local files, prompt content, response content, local paths, or secrets are read",
		},
	}
	expected := len(req.Expected)
	switch {
	case expected == 0 && req.Strict:
		checks = append(checks, integrationUpgradeGateCheck("lockfile_present", "block", "critical", "strict upgrade gates require pinned expected hashes", "expected_hashes=0, strict=true", "generate a lockfile with agent-ledger integrations lockfile and pass its hashes to this gate"))
	case expected == 0:
		checks = append(checks, integrationUpgradeGateCheck("lockfile_present", "review", "warning", "no pinned lockfile hashes were supplied", "expected_hashes=0", "generate and pin an integration lockfile before using this gate for CI release decisions"))
	default:
		checks = append(checks, integrationUpgradeGateCheck("lockfile_present", "pass", "info", "pinned lockfile hashes supplied", "expected_hashes="+strconv.Itoa(expected), "no action required"))
	}
	if drift.Summary.Drifted > 0 {
		checks = append(checks, integrationUpgradeGateCheck("hash_drift", "block", "critical", "one or more pinned integration hashes drifted", "drifted="+strconv.Itoa(drift.Summary.Drifted), "rerun conformance, evidence-kit, pricing, policy, and rollout checks before refreshing the lockfile"))
	} else {
		checks = append(checks, integrationUpgradeGateCheck("hash_drift", "pass", "info", "no drift found among supplied hashes", "drifted=0", "no action required"))
	}
	if drift.Summary.MissingExpected > 0 {
		status := "review"
		severity := "warning"
		message := "some known hash ids are not pinned"
		remediation := "refresh the lockfile so every known hash id is pinned"
		if req.Strict {
			status = "block"
			severity = "critical"
			message = "strict gate blocks missing expected hashes"
			remediation = "pin every known hash id or run without strict mode for exploratory review"
		}
		checks = append(checks, integrationUpgradeGateCheck("missing_expected", status, severity, message, "missing_expected="+strconv.Itoa(drift.Summary.MissingExpected), remediation))
	} else {
		checks = append(checks, integrationUpgradeGateCheck("missing_expected", "pass", "info", "all known hash ids are pinned by the request", "missing_expected=0", "no action required"))
	}
	if drift.Summary.UnknownExpected > 0 {
		status := "review"
		severity := "warning"
		if req.Strict {
			status = "block"
			severity = "critical"
		}
		checks = append(checks, integrationUpgradeGateCheck("unknown_expected", status, severity, "unknown expected hash ids require compatibility review", "unknown_expected="+strconv.Itoa(drift.Summary.UnknownExpected), "remove unknown hash ids or upgrade Agent Ledger if they belong to a newer contract"))
	} else {
		checks = append(checks, integrationUpgradeGateCheck("unknown_expected", "pass", "info", "no unknown expected hash ids supplied", "unknown_expected=0", "no action required"))
	}
	runtimeEvidence := "runtime=default"
	if runtime != nil {
		runtimeEvidence = "read_only=" + boolString(runtime.ReadOnly) + ",write_operations=" + strings.TrimSpace(runtime.WriteOperations)
	}
	checks = append(checks, integrationUpgradeGateCheck("runtime_contract", "pass", "info", "runtime status is included as a hash witness", runtimeEvidence, "review runtime_status_hash drift before enabling write ingest"))
	if drift.Summary.Drifted > 0 || drift.Summary.MissingExpected > 0 || drift.Summary.UnknownExpected > 0 {
		checks = append(checks, integrationUpgradeGateCheck("evidence_refresh", "review", "warning", "release evidence should be refreshed before rollout", "drift_status="+drift.Summary.Status, "run agent-ledger integrations evidence-kit and attach updated adapter conformance evidence"))
	} else {
		checks = append(checks, integrationUpgradeGateCheck("evidence_refresh", "pass", "info", "no evidence refresh required by current hash comparison", "drift_status="+drift.Summary.Status, "keep normal release evidence attached"))
	}
	return checks
}

func integrationUpgradeGateCheck(id, status, severity, message, evidence, remediation string) IntegrationUpgradeGateCheck {
	return IntegrationUpgradeGateCheck{
		ID:          id,
		Status:      status,
		Severity:    severity,
		Message:     message,
		Evidence:    evidence,
		Remediation: remediation,
		Privacy:     "metadata-only check; prompts, responses, messages, artifact bodies, raw headers, credentials, local paths, accounts, machines, authors, native sessions, and webhook URLs are excluded",
	}
}

func integrationUpgradeGateRows(drift IntegrationDriftReport, req IntegrationUpgradeGateRequest) ([]IntegrationDriftRow, []IntegrationDriftRow) {
	blocking := []IntegrationDriftRow{}
	review := []IntegrationDriftRow{}
	for _, row := range drift.Rows {
		switch row.Status {
		case "drift":
			blocking = append(blocking, row)
		case "missing-expected":
			if req.Strict {
				blocking = append(blocking, row)
			} else {
				review = append(review, row)
			}
		case "unknown-expected":
			if req.Strict {
				blocking = append(blocking, row)
			} else {
				review = append(review, row)
			}
		}
	}
	return blocking, review
}

func summarizeIntegrationUpgradeGate(checks []IntegrationUpgradeGateCheck, drift IntegrationDriftReport, req IntegrationUpgradeGateRequest) IntegrationUpgradeGateDecision {
	blocked := 0
	review := 0
	for _, check := range checks {
		switch check.Status {
		case "block":
			blocked++
		case "review":
			review++
		}
	}
	switch {
	case blocked > 0:
		return IntegrationUpgradeGateDecision{
			Status:                  "block",
			Severity:                "critical",
			Reason:                  "release gate blocked by drift, missing strict lockfile pins, or unknown expected hash ids",
			RecommendedCIExitCode:   2,
			AllowWriteIngest:        false,
			RequiresHumanReview:     true,
			RequiresEvidenceRefresh: true,
		}
	case review > 0:
		return IntegrationUpgradeGateDecision{
			Status:                  "review",
			Severity:                "warning",
			Reason:                  "release gate needs human review before enabling write ingest or provider gateway surfaces",
			RecommendedCIExitCode:   1,
			AllowWriteIngest:        false,
			RequiresHumanReview:     true,
			RequiresEvidenceRefresh: drift.Summary.Status != "match" || len(req.Expected) == 0,
		}
	default:
		return IntegrationUpgradeGateDecision{
			Status:                  "pass",
			Severity:                "info",
			Reason:                  "pinned integration hashes match current Agent Ledger control-plane witnesses",
			RecommendedCIExitCode:   0,
			AllowWriteIngest:        true,
			RequiresHumanReview:     false,
			RequiresEvidenceRefresh: false,
		}
	}
}
