package integrations

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// SchemaEvolutionGateRequest lets adapters, wrappers, routers, and CI pin the
// canonical event schema contract they were built against before accepting an
// Agent Ledger upgrade.
type SchemaEvolutionGateRequest struct {
	Strict               bool     `json:"strict"`
	ExpectedVersion      string   `json:"expected_version,omitempty"`
	ExpectedSchemaHash   string   `json:"expected_schema_hash,omitempty"`
	RequiredEventTypes   []string `json:"required_event_types,omitempty"`
	RequiredRejectedKeys []string `json:"required_rejected_keys,omitempty"`
}

// SchemaEvolutionGateReport is a metadata-only compatibility decision for the
// canonical event schema. It never reads SQLite usage rows, fixture bodies,
// prompts, responses, local paths, native sessions, or secrets.
type SchemaEvolutionGateReport struct {
	Product           string                     `json:"product"`
	Contract          string                     `json:"contract"`
	Version           string                     `json:"version"`
	LocalFirst        bool                       `json:"local_first"`
	ReadOnlySafe      bool                       `json:"read_only_safe"`
	WritesLocalState  bool                       `json:"writes_local_state"`
	PrivacyPolicy     string                     `json:"privacy_policy"`
	Request           SchemaEvolutionGateRequest `json:"request"`
	GateHash          string                     `json:"gate_hash"`
	Decision          SchemaEvolutionDecision    `json:"decision"`
	Current           SchemaEvolutionCurrent     `json:"current"`
	Summary           SchemaEvolutionSummary     `json:"summary"`
	Checks            []SchemaEvolutionCheck     `json:"checks"`
	EventRows         []SchemaEvolutionRow       `json:"event_rows"`
	RejectedKeyRows   []SchemaEvolutionRow       `json:"rejected_key_rows"`
	CICommands        []string                   `json:"ci_commands"`
	RequiredArtifacts []string                   `json:"required_artifacts"`
	MigrationGuidance []string                   `json:"migration_guidance"`
	RedactionRules    []string                   `json:"redaction_rules"`
}

type SchemaEvolutionCurrent struct {
	SchemaVersion       string   `json:"schema_version"`
	SchemaHash          string   `json:"schema_hash"`
	SupportedVersions   []string `json:"supported_versions"`
	EventTypes          []string `json:"event_types"`
	RejectedPayloadKeys []string `json:"rejected_payload_keys"`
	AdapterSpecHash     string   `json:"adapter_spec_hash"`
}

type SchemaEvolutionDecision struct {
	Status                  string `json:"status"`
	Severity                string `json:"severity"`
	Reason                  string `json:"reason"`
	RecommendedCIExitCode   int    `json:"recommended_ci_exit_code"`
	AllowAdapterIngest      bool   `json:"allow_adapter_ingest"`
	RequiresMigration       bool   `json:"requires_migration"`
	RequiresHumanReview     bool   `json:"requires_human_review"`
	RequiresLockfileRefresh bool   `json:"requires_lockfile_refresh"`
}

type SchemaEvolutionSummary struct {
	Status                    string `json:"status"`
	KnownEventTypes           int    `json:"known_event_types"`
	RequiredEventTypes        int    `json:"required_event_types"`
	MatchedEventTypes         int    `json:"matched_event_types"`
	MissingEventTypes         int    `json:"missing_event_types"`
	KnownRejectedKeys         int    `json:"known_rejected_keys"`
	RequiredRejectedKeys      int    `json:"required_rejected_keys"`
	MatchedRejectedKeys       int    `json:"matched_rejected_keys"`
	MissingRejectedKeys       int    `json:"missing_rejected_keys"`
	VersionMatches            bool   `json:"version_matches"`
	SchemaHashMatches         bool   `json:"schema_hash_matches"`
	ExpectedSchemaHashPresent bool   `json:"expected_schema_hash_present"`
	Warnings                  int    `json:"warnings"`
}

type SchemaEvolutionCheck struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	Message     string `json:"message"`
	Evidence    string `json:"evidence"`
	Remediation string `json:"remediation"`
	Privacy     string `json:"privacy"`
}

type SchemaEvolutionRow struct {
	ID       string `json:"id"`
	Kind     string `json:"kind"`
	Expected string `json:"expected,omitempty"`
	Current  string `json:"current,omitempty"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Action   string `json:"action"`
	Privacy  string `json:"privacy"`
}

func SchemaEvolutionGateFromValues(values url.Values) SchemaEvolutionGateRequest {
	req := SchemaEvolutionGateRequest{
		Strict:               parseIntegrationDriftBool(values.Get("strict")),
		ExpectedVersion:      firstNonEmptyIntegrationValue(values.Get("schema_version"), values.Get("schema-version"), values.Get("version")),
		ExpectedSchemaHash:   firstNonEmptyIntegrationValue(values.Get("schema_hash"), values.Get("schema-hash"), values.Get("canonical_schema_hash"), values.Get("canonical-schema-hash"), values.Get("hash")),
		RequiredEventTypes:   appendCSVValues(values["event_type"], values["event-type"], values["event_types"], values["event-types"], values["required_event_type"], values["required-event-type"]),
		RequiredRejectedKeys: appendCSVValues(values["rejected_key"], values["rejected-key"], values["rejected_keys"], values["rejected-keys"], values["required_rejected_key"], values["required-rejected-key"]),
	}
	return NormalizeSchemaEvolutionGateRequest(req)
}

func NormalizeSchemaEvolutionGateRequest(req SchemaEvolutionGateRequest) SchemaEvolutionGateRequest {
	return SchemaEvolutionGateRequest{
		Strict:               req.Strict,
		ExpectedVersion:      strings.ToLower(strings.TrimSpace(req.ExpectedVersion)),
		ExpectedSchemaHash:   strings.TrimSpace(req.ExpectedSchemaHash),
		RequiredEventTypes:   normalizeSchemaEventTypes(req.RequiredEventTypes),
		RequiredRejectedKeys: normalizeSchemaRejectedKeys(req.RequiredRejectedKeys),
	}
}

func SchemaEvolutionGateFor(req SchemaEvolutionGateRequest) SchemaEvolutionGateReport {
	req = NormalizeSchemaEvolutionGateRequest(req)
	current := currentSchemaEvolution()
	eventRows := schemaEvolutionRows("event_type", req.RequiredEventTypes, current.EventTypes)
	rejectedKeyRows := schemaEvolutionRows("rejected_payload_key", req.RequiredRejectedKeys, current.RejectedPayloadKeys)
	summary := summarizeSchemaEvolution(req, current, eventRows, rejectedKeyRows)
	checks := schemaEvolutionChecks(req, current, summary)
	decision := summarizeSchemaEvolutionDecision(checks, summary, req)
	report := SchemaEvolutionGateReport{
		Product:          "Agent Ledger",
		Contract:         "agent-ledger.schema-evolution-gate",
		Version:          "v1",
		LocalFirst:       true,
		ReadOnlySafe:     true,
		WritesLocalState: false,
		PrivacyPolicy:    "Schema evolution gates compare canonical event schema metadata, event type ids, rejected payload keys, and hashes only. They exclude SQLite usage rows, fixture bodies, prompt content, response content, message history, local paths, secrets, account identifiers, machine names, authors, native session identifiers, and webhook URLs.",
		Request:          req,
		Decision:         decision,
		Current:          current,
		Summary:          summary,
		Checks:           checks,
		EventRows:        eventRows,
		RejectedKeyRows:  rejectedKeyRows,
		CICommands: []string{
			"agent-ledger event schema",
			"agent-ledger schema-gate --strict --schema-version v1 --schema-hash <pinned>",
			"agent-ledger adapter spec",
			"agent-ledger adapter matrix",
			"agent-ledger contracts verify",
		},
		RequiredArtifacts: []string{
			"pinned canonical schema version and schema hash from a verified Agent Ledger binary",
			"adapter conformance output for every event family the adapter emits",
			"privacy review for rejected payload key changes",
			"migration notes when supported schema versions or required event types change",
		},
		MigrationGuidance: []string{
			"treat schema hash drift as requiring adapter conformance refresh",
			"treat missing required event types as migration blockers for adapters that emit those events",
			"treat missing rejected payload keys as a privacy review blocker",
			"keep adapters accepting only supported_versions reported by the current schema endpoint",
		},
		RedactionRules: []string{
			"attach only schema versions, schema hashes, event type ids, rejected key ids, and CI command names to tickets",
			"do not attach prompts, responses, transcripts, raw headers, credentials, webhook URLs, local paths, account names, machine names, authors, native session ids, or provider account ids",
			"use synthetic metadata fixtures when reproducing schema compatibility failures",
		},
	}
	report.GateHash = SchemaEvolutionGateFingerprintFrom(report)
	return report
}

func SchemaEvolutionGateFingerprint(req SchemaEvolutionGateRequest) string {
	return SchemaEvolutionGateFingerprintFrom(SchemaEvolutionGateFor(req))
}

// SchemaEvolutionGateOpenAPIFingerprint is a lightweight witness hash for
// discovery, OpenAPI metadata, and contract-bundle indexes.
func SchemaEvolutionGateOpenAPIFingerprint() string {
	current := currentSchemaEvolution()
	return hashJSONPayload(map[string]interface{}{
		"contract":              "agent-ledger.schema-evolution-gate",
		"version":               "v1",
		"default_uri":           "/api/schema/evolution-gate",
		"schema_version":        current.SchemaVersion,
		"schema_hash":           current.SchemaHash,
		"event_type_count":      len(current.EventTypes),
		"rejected_key_count":    len(current.RejectedPayloadKeys),
		"adapter_spec_hash":     current.AdapterSpecHash,
		"canonical_schema_hash": storage.CanonicalEventSchemaFingerprint(),
		"privacy":               "metadata-only OpenAPI witness; full endpoint ETag is returned by /api/schema/evolution-gate",
	})
}

func SchemaEvolutionGateFingerprintFrom(report SchemaEvolutionGateReport) string {
	report.GateHash = ""
	return hashJSONPayload(report)
}

func currentSchemaEvolution() SchemaEvolutionCurrent {
	schema := storage.CanonicalEventSchema()
	return SchemaEvolutionCurrent{
		SchemaVersion:       storage.CanonicalEventSchemaVersion,
		SchemaHash:          storage.CanonicalEventSchemaFingerprint(),
		SupportedVersions:   schemaSupportedVersions(schema),
		EventTypes:          schemaEventTypeIDs(),
		RejectedPayloadKeys: schemaRejectedPayloadKeys(schema),
		AdapterSpecHash:     AdapterContractFingerprint(),
	}
}

func schemaSupportedVersions(schema map[string]interface{}) []string {
	switch raw := schema["supported_versions"].(type) {
	case []string:
		return uniqueSortedStrings(raw)
	case []interface{}:
		out := []string{}
		for _, value := range raw {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return uniqueSortedStrings(out)
	default:
		return []string{storage.CanonicalEventSchemaVersion}
	}
}

func schemaEventTypeIDs() []string {
	out := make([]string, 0, len(storage.CanonicalEventTypes()))
	for _, item := range storage.CanonicalEventTypes() {
		out = append(out, item.EventType)
	}
	return uniqueSortedStrings(out)
}

func schemaRejectedPayloadKeys(schema map[string]interface{}) []string {
	privacy, _ := schema["privacy"].(map[string]interface{})
	switch raw := privacy["rejected_payload_keys"].(type) {
	case []string:
		return normalizeSchemaRejectedKeys(raw)
	case []interface{}:
		out := []string{}
		for _, value := range raw {
			if text, ok := value.(string); ok {
				out = append(out, text)
			}
		}
		return normalizeSchemaRejectedKeys(out)
	default:
		return nil
	}
}

func schemaEvolutionRows(kind string, expected []string, current []string) []SchemaEvolutionRow {
	rows := []SchemaEvolutionRow{}
	currentSet := map[string]bool{}
	for _, value := range current {
		currentSet[value] = true
	}
	for _, value := range expected {
		status := "match"
		severity := "info"
		action := "no action required"
		currentValue := value
		if !currentSet[value] {
			status = "missing"
			severity = "critical"
			action = "update the adapter migration plan or keep using a compatible Agent Ledger release"
			currentValue = ""
		}
		rows = append(rows, SchemaEvolutionRow{
			ID:       kind + ":" + value,
			Kind:     kind,
			Expected: value,
			Current:  currentValue,
			Status:   status,
			Severity: severity,
			Action:   action,
			Privacy:  "schema metadata only; prompts, responses, messages, artifact bodies, raw headers, credentials, local paths, accounts, machines, authors, native sessions, and webhook URLs are excluded",
		})
	}
	return rows
}

func summarizeSchemaEvolution(req SchemaEvolutionGateRequest, current SchemaEvolutionCurrent, eventRows, rejectedKeyRows []SchemaEvolutionRow) SchemaEvolutionSummary {
	summary := SchemaEvolutionSummary{
		Status:                    "baseline",
		KnownEventTypes:           len(current.EventTypes),
		RequiredEventTypes:        len(req.RequiredEventTypes),
		KnownRejectedKeys:         len(current.RejectedPayloadKeys),
		RequiredRejectedKeys:      len(req.RequiredRejectedKeys),
		VersionMatches:            req.ExpectedVersion == "" || req.ExpectedVersion == current.SchemaVersion || stringSliceContains(current.SupportedVersions, req.ExpectedVersion),
		SchemaHashMatches:         req.ExpectedSchemaHash == "" || req.ExpectedSchemaHash == current.SchemaHash,
		ExpectedSchemaHashPresent: req.ExpectedSchemaHash != "",
	}
	for _, row := range eventRows {
		if row.Status == "match" {
			summary.MatchedEventTypes++
		} else {
			summary.MissingEventTypes++
			summary.Warnings++
		}
	}
	for _, row := range rejectedKeyRows {
		if row.Status == "match" {
			summary.MatchedRejectedKeys++
		} else {
			summary.MissingRejectedKeys++
			summary.Warnings++
		}
	}
	if !summary.VersionMatches {
		summary.Warnings++
	}
	if !summary.SchemaHashMatches {
		summary.Warnings++
	}
	if req.Strict && req.ExpectedSchemaHash == "" {
		summary.Warnings++
	}
	switch {
	case !summary.VersionMatches || !summary.SchemaHashMatches || summary.MissingRejectedKeys > 0:
		summary.Status = "block"
	case req.Strict && (req.ExpectedSchemaHash == "" || summary.MissingEventTypes > 0):
		summary.Status = "block"
	case req.ExpectedSchemaHash == "" || summary.MissingEventTypes > 0 || len(req.RequiredEventTypes) == 0:
		summary.Status = "review"
	default:
		summary.Status = "pass"
	}
	return summary
}

func schemaEvolutionChecks(req SchemaEvolutionGateRequest, current SchemaEvolutionCurrent, summary SchemaEvolutionSummary) []SchemaEvolutionCheck {
	checks := []SchemaEvolutionCheck{
		schemaEvolutionCheck("metadata_only", "pass", "info", "schema evolution gate is metadata-only and read-only", "writes_local_state=false, read_only_safe=true", "keep prompt content, response content, raw headers, local paths, and secrets out of schema CI artifacts"),
	}
	if req.ExpectedVersion == "" {
		status := "review"
		severity := "warning"
		if req.Strict {
			status = "block"
			severity = "critical"
		}
		checks = append(checks, schemaEvolutionCheck("schema_version_pin", status, severity, "schema version pin is missing", "expected_version=<empty>, current="+current.SchemaVersion, "pin the schema version from agent-ledger event schema"))
	} else if summary.VersionMatches {
		checks = append(checks, schemaEvolutionCheck("schema_version_pin", "pass", "info", "schema version is supported", "expected_version="+req.ExpectedVersion+", current="+current.SchemaVersion, "no action required"))
	} else {
		checks = append(checks, schemaEvolutionCheck("schema_version_pin", "block", "critical", "schema version is unsupported", "expected_version="+req.ExpectedVersion+", supported="+strings.Join(current.SupportedVersions, ","), "update adapter migrations or keep using a compatible Agent Ledger release"))
	}
	if req.ExpectedSchemaHash == "" {
		status := "review"
		severity := "warning"
		if req.Strict {
			status = "block"
			severity = "critical"
		}
		checks = append(checks, schemaEvolutionCheck("schema_hash_pin", status, severity, "schema hash pin is missing", "expected_schema_hash=<empty>", "pin schema_hash from agent-ledger event schema before using this gate as a release blocker"))
	} else if summary.SchemaHashMatches {
		checks = append(checks, schemaEvolutionCheck("schema_hash_pin", "pass", "info", "schema hash matches current canonical schema", "schema_hash="+current.SchemaHash, "no action required"))
	} else {
		checks = append(checks, schemaEvolutionCheck("schema_hash_pin", "block", "critical", "schema hash drifted from the adapter pin", "expected_schema_hash="+req.ExpectedSchemaHash+", current="+current.SchemaHash, "rerun adapter conformance and refresh schema migration evidence"))
	}
	if len(req.RequiredEventTypes) == 0 {
		checks = append(checks, schemaEvolutionCheck("required_event_types", "review", "warning", "adapter required event types were not supplied", "required_event_types=0,current_event_types="+strconv.Itoa(len(current.EventTypes)), "declare the event types emitted by the adapter so CI can detect missing schema support"))
	} else if summary.MissingEventTypes > 0 {
		status := "review"
		severity := "warning"
		if req.Strict {
			status = "block"
			severity = "critical"
		}
		checks = append(checks, schemaEvolutionCheck("required_event_types", status, severity, "one or more adapter event types are not supported by the current schema", "missing_event_types="+strconv.Itoa(summary.MissingEventTypes), "update the adapter or select a compatible Agent Ledger release"))
	} else {
		checks = append(checks, schemaEvolutionCheck("required_event_types", "pass", "info", "all adapter required event types are supported", "matched_event_types="+strconv.Itoa(summary.MatchedEventTypes), "no action required"))
	}
	if summary.MissingRejectedKeys > 0 {
		checks = append(checks, schemaEvolutionCheck("rejected_payload_keys", "block", "critical", "one or more required rejected payload keys are not enforced by the current schema", "missing_rejected_keys="+strconv.Itoa(summary.MissingRejectedKeys), "review privacy policy before accepting this schema"))
	} else if len(req.RequiredRejectedKeys) == 0 {
		checks = append(checks, schemaEvolutionCheck("rejected_payload_keys", "pass", "info", "no additional rejected payload key pins were requested", "current_rejected_keys="+strconv.Itoa(len(current.RejectedPayloadKeys)), "optionally pin rejected payload keys for privacy regression CI"))
	} else {
		checks = append(checks, schemaEvolutionCheck("rejected_payload_keys", "pass", "info", "all required rejected payload keys are enforced", "matched_rejected_keys="+strconv.Itoa(summary.MatchedRejectedKeys), "no action required"))
	}
	checks = append(checks, schemaEvolutionCheck("adapter_contract_alignment", "pass", "info", "adapter contract and schema fingerprint are linked", "adapter_spec_hash="+current.AdapterSpecHash+", schema_hash="+current.SchemaHash, "rerun agent-ledger adapter spec when schema hash changes"))
	return checks
}

func schemaEvolutionCheck(id, status, severity, message, evidence, remediation string) SchemaEvolutionCheck {
	return SchemaEvolutionCheck{
		ID:          id,
		Status:      status,
		Severity:    severity,
		Message:     message,
		Evidence:    evidence,
		Remediation: remediation,
		Privacy:     "metadata-only check; prompts, responses, messages, artifact bodies, raw headers, credentials, local paths, accounts, machines, authors, native sessions, and webhook URLs are excluded",
	}
}

func summarizeSchemaEvolutionDecision(checks []SchemaEvolutionCheck, summary SchemaEvolutionSummary, req SchemaEvolutionGateRequest) SchemaEvolutionDecision {
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
		return SchemaEvolutionDecision{
			Status:                  "block",
			Severity:                "critical",
			Reason:                  "schema gate blocked by version drift, schema hash drift, missing strict pins, missing event support, or privacy key regression",
			RecommendedCIExitCode:   2,
			AllowAdapterIngest:      false,
			RequiresMigration:       true,
			RequiresHumanReview:     true,
			RequiresLockfileRefresh: true,
		}
	case review > 0:
		return SchemaEvolutionDecision{
			Status:                  "review",
			Severity:                "warning",
			Reason:                  "schema gate needs human review before adapter release decisions",
			RecommendedCIExitCode:   1,
			AllowAdapterIngest:      false,
			RequiresMigration:       summary.MissingEventTypes > 0 || req.ExpectedSchemaHash == "",
			RequiresHumanReview:     true,
			RequiresLockfileRefresh: !summary.ExpectedSchemaHashPresent,
		}
	default:
		return SchemaEvolutionDecision{
			Status:                  "pass",
			Severity:                "info",
			Reason:                  "adapter schema pins match the current canonical event schema contract",
			RecommendedCIExitCode:   0,
			AllowAdapterIngest:      true,
			RequiresMigration:       false,
			RequiresHumanReview:     false,
			RequiresLockfileRefresh: false,
		}
	}
}

func normalizeSchemaEventTypes(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "_", ".")))
		if value != "" {
			out = append(out, value)
		}
	}
	return uniqueSortedStrings(out)
}

func normalizeSchemaRejectedKeys(values []string) []string {
	out := []string{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		value = strings.ReplaceAll(value, "-", "_")
		if value != "" {
			out = append(out, value)
		}
	}
	return uniqueSortedStrings(out)
}

func appendCSVValues(groups ...[]string) []string {
	out := []string{}
	for _, group := range groups {
		for _, raw := range group {
			for _, part := range strings.Split(raw, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					out = append(out, trimmed)
				}
			}
		}
	}
	return out
}

func firstNonEmptyIntegrationValue(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
