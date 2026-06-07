package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CanonicalEvent is the stable event contract for wrappers, gateways, MCP tools,
// and future agent protocols. Payload must contain metadata only, never prompt text.
type CanonicalEvent struct {
	EventID       string          `json:"event_id"`
	Source        string          `json:"source"`
	EventType     string          `json:"event_type"`
	SchemaVersion string          `json:"schema_version,omitempty"`
	SourceVersion string          `json:"source_version,omitempty"`
	ParserVersion string          `json:"parser_version,omitempty"`
	SourceEventID string          `json:"source_event_id"`
	RawRef        string          `json:"raw_ref,omitempty"`
	MatchType     string          `json:"match_type,omitempty"`
	WorkloadID    string          `json:"workload_id"`
	AgentRunID    string          `json:"agent_run_id"`
	SessionID     string          `json:"session_id"`
	Model         string          `json:"model"`
	Project       string          `json:"project"`
	GitBranch     string          `json:"git_branch"`
	Timestamp     time.Time       `json:"timestamp"`
	PayloadHash   string          `json:"payload_hash"`
	Payload       json.RawMessage `json:"payload"`
	Confidence    float64         `json:"confidence"`
}

// CanonicalEventResult describes an ingest outcome.
type CanonicalEventResult struct {
	EventID    string   `json:"event_id"`
	Status     string   `json:"status"`
	EventType  string   `json:"event_type"`
	WorkloadID string   `json:"workload_id,omitempty"`
	RunID      string   `json:"run_id,omitempty"`
	Derived    []string `json:"derived,omitempty"`
}

// CanonicalEventValidation describes a dry-run contract validation outcome.
type CanonicalEventValidation struct {
	EventID     string   `json:"event_id"`
	Status      string   `json:"status"`
	EventType   string   `json:"event_type"`
	Source      string   `json:"source"`
	PayloadHash string   `json:"payload_hash"`
	Warnings    []string `json:"warnings,omitempty"`
}

// CanonicalEventTypeInfo describes one supported canonical event type.
type CanonicalEventTypeInfo struct {
	EventType     string            `json:"event_type"`
	Description   string            `json:"description"`
	Required      []string          `json:"required"`
	PayloadFields map[string]string `json:"payload_fields"`
}

// CanonicalEventExample is a privacy-safe template for adapter authors.
type CanonicalEventExample struct {
	EventType   string         `json:"event_type"`
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Event       CanonicalEvent `json:"event"`
}

// CanonicalEventSchema returns the public metadata-only event contract.
func CanonicalEventSchema() map[string]interface{} {
	return map[string]interface{}{
		"version": "v1",
		"privacy": map[string]interface{}{
			"payload_policy": "metadata-only",
			"rejected_payload_keys": []string{
				"prompt", "prompts", "messages", "content", "input_text", "output_text", "completion", "transcript",
			},
		},
		"envelope_fields": map[string]string{
			"event_id":        "Optional stable idempotency key. Deterministic hash is generated when omitted.",
			"source":          "Required source name such as codex, claude, opencode, gateway, or local.",
			"event_type":      "Required canonical event type.",
			"schema_version":  "Optional canonical envelope version. Defaults to v1.",
			"source_version":  "Optional upstream agent, CLI, provider, or protocol version.",
			"parser_version":  "Optional source adapter/parser version that produced this event.",
			"source_event_id": "Optional native upstream event id.",
			"raw_ref":         "Optional privacy-safe native reference such as file hash, row id, trace id, or byte offset. Do not send raw prompt/content.",
			"match_type":      "Optional provenance match label such as exact, estimated, reconstructed, source_reported, or fuzzy.",
			"workload_id":     "Optional Agent Ledger workload id. Required for most child events unless payload.goal is provided.",
			"agent_run_id":    "Optional Agent Ledger run id.",
			"session_id":      "Optional source-scoped local session id.",
			"model":           "Optional model name; required for model.call unless payload.model is provided.",
			"project":         "Optional project/workspace name.",
			"git_branch":      "Optional normalized branch name.",
			"timestamp":       "Optional RFC3339 timestamp; current UTC time is used when omitted.",
			"payload_hash":    "Optional sha256 payload hash. Generated when omitted.",
			"payload":         "Required JSON object for event-specific metadata. Raw prompt/model output is rejected.",
			"confidence":      "Optional parser confidence in [0,1]. Defaults to 1.",
		},
		"event_types":  CanonicalEventTypes(),
		"examples_uri": "/api/event-examples",
	}
}

// CanonicalEventTypes lists event types that are projected into the workload ledger.
func CanonicalEventTypes() []CanonicalEventTypeInfo {
	return []CanonicalEventTypeInfo{
		{
			EventType:   "workload.started",
			Description: "Create or upsert a goal-level workload.",
			Required:    []string{"source", "event_type", "payload.goal"},
			PayloadFields: map[string]string{
				"goal":       "Human-readable goal or workload objective.",
				"project":    "Project/workspace name.",
				"repo":       "Repository slug or local repo alias.",
				"git_branch": "Branch name; empty values normalize to unknown.",
				"owner":      "Optional owner or user alias.",
				"team":       "Optional team/showback group.",
				"budget_usd": "Optional local budget for this workload.",
			},
		},
		{
			EventType:   "workload.closed",
			Description: "Close an existing workload.",
			Required:    []string{"source", "event_type", "workload_id"},
			PayloadFields: map[string]string{
				"status":  "completed, failed, partial, or abandoned.",
				"outcome": "Short outcome summary without prompt content.",
			},
		},
		{
			EventType:   "agent.run.started",
			Description: "Attach an agent execution to a workload.",
			Required:    []string{"source", "event_type", "workload_id or payload.goal"},
			PayloadFields: map[string]string{
				"run_id":        "Optional stable run id.",
				"parent_run_id": "Optional parent run for sub-agent/fleet attribution.",
				"agent_name":    "Agent name, e.g. codex, claude-code, opencode.",
				"agent_version": "Agent version if available.",
				"command":       "Optional command string; redact when sensitive.",
				"cwd":           "Optional working directory; privacy modes can redact it.",
				"status":        "Usually running.",
			},
		},
		{
			EventType:   "agent.run.finished",
			Description: "Mark an agent run complete or failed.",
			Required:    []string{"source", "event_type", "agent_run_id"},
			PayloadFields: map[string]string{
				"status":      "completed or failed.",
				"exit_code":   "Process exit code when applicable.",
				"error":       "Short error class/message without secrets.",
				"duration_ms": "Run duration in milliseconds.",
			},
		},
		{
			EventType:   "agent.run.heartbeat",
			Description: "Append a metadata-only liveness/progress signal for an async agent run.",
			Required:    []string{"source", "event_type", "agent_run_id"},
			PayloadFields: map[string]string{
				"event_id": "Optional stable heartbeat event id.",
				"status":   "running, working, waiting_approval, blocked, evaluating, or stalled.",
				"phase":    "Short phase label such as planning, editing, testing, or waiting.",
				"progress": "Optional progress in [0,1].",
				"message":  "Short metadata-only status message; do not include prompt or response content.",
				"metrics":  "Small JSON object with counters or hashes; prompt/content keys are rejected.",
			},
		},
		{
			EventType:   "model.call",
			Description: "Record one model call with non-overlapping token components.",
			Required:    []string{"source", "event_type", "workload_id or payload.goal", "model or payload.model"},
			PayloadFields: map[string]string{
				"call_id":                     "Optional stable call id.",
				"provider":                    "Model provider.",
				"model":                       "Model name when envelope.model is omitted.",
				"model_alias":                 "Provider or gateway alias.",
				"input_tokens":                "Non-cached input tokens.",
				"cache_read_input_tokens":     "Input tokens served from cache.",
				"cache_creation_input_tokens": "Input tokens written to cache.",
				"output_tokens":               "Output tokens.",
				"reasoning_output_tokens":     "Reasoning tokens, informational subset of output.",
				"cost_usd":                    "Local or provider-reported cost.",
				"latency_ms":                  "Model call latency.",
				"finish_reason":               "Stop reason.",
				"pricing_source":              "official, fallback, override, source-reported, or unpriced.",
				"pricing_confidence":          "exact, fuzzy, stale, fallback, source-reported, or unpriced.",
			},
		},
		{
			EventType:   "tool.call",
			Description: "Record one tool invocation without raw parameters.",
			Required:    []string{"source", "event_type", "workload_id or payload.goal"},
			PayloadFields: map[string]string{
				"tool_call_id": "Optional stable tool call id.",
				"tool_name":    "Tool name.",
				"tool_type":    "shell, file, browser, mcp, api, or other.",
				"status":       "ok, failed, skipped, or blocked.",
				"error_class":  "Short error class.",
				"duration_ms":  "Tool duration.",
				"params_hash":  "Hash of parameters; do not send raw params.",
			},
		},
		{
			EventType:   "context.ref",
			Description: "Attach a hashed or metadata-only context reference.",
			Required:    []string{"source", "event_type", "workload_id or payload.goal"},
			PayloadFields: map[string]string{
				"ref_type":      "repo, file, diff, issue, pr, doc, memory, or other.",
				"ref_hash":      "Stable hash of the referenced context.",
				"label":         "Short non-sensitive label.",
				"repo":          "Repository alias.",
				"git_branch":    "Branch name.",
				"commit_sha":    "Commit SHA if relevant.",
				"privacy_label": "local, team-share, strict, or public.",
			},
		},
		{
			EventType:   "artifact.created",
			Description: "Record a privacy-safe output artifact reference.",
			Required:    []string{"source", "event_type", "workload_id or payload.goal"},
			PayloadFields: map[string]string{
				"artifact_id":   "Optional stable artifact id.",
				"artifact_type": "patch, report, bundle, screenshot, metric, or other.",
				"label":         "Short label.",
				"path_hash":     "Hash of local path.",
				"sha256":        "Artifact content hash.",
				"metadata":      "Small JSON metadata object without raw content.",
			},
		},
		{
			EventType:   "evaluation.recorded",
			Description: "Record local quality or outcome signal.",
			Required:    []string{"source", "event_type", "workload_id or payload.goal"},
			PayloadFields: map[string]string{
				"evaluator": "local evaluator name.",
				"status":    "pass, fail, unknown, or skipped.",
				"score":     "Numeric score.",
				"signal":    "tests, review, lint, user, perf, or custom signal.",
				"notes":     "Short metadata-only note.",
			},
		},
		{
			EventType:   "policy.decision",
			Description: "Record advisory or enforced local policy decision.",
			Required:    []string{"source", "event_type", "workload_id or payload.goal"},
			PayloadFields: map[string]string{
				"decision_id": "Optional stable decision id.",
				"rule_id":     "Policy rule id.",
				"action":      "allow, warn, require_approval, or block.",
				"reason":      "Short policy reason.",
				"actor_role":  "viewer, operator, admin, agent, or gateway.",
			},
		},
	}
}

// CanonicalEventExamples returns stable metadata-only templates for adapter
// authors. eventType is optional and uses the same normalized names as ingest.
func CanonicalEventExamples(eventType string) []CanonicalEventExample {
	filter := normalizeCanonicalEventType(eventType)
	examples := []CanonicalEventExample{
		canonicalExample("workload.started", "Start a workload", "Create a goal-level workload before runs and calls.", "", "", examplePayload(map[string]interface{}{
			"goal":       "Ship metadata-only adapter integration",
			"project":    "agent-ledger",
			"repo":       "zhenzhis/agent-ledger",
			"git_branch": "main",
			"team":       "platform",
		})),
		canonicalExample("agent.run.started", "Start an agent run", "Attach one agent execution to a workload.", "wl_example_adapter", "run_example_001", examplePayload(map[string]interface{}{
			"run_id":        "run_example_001",
			"agent_name":    "example-agent",
			"agent_version": "1.0.0",
			"status":        "running",
			"phase":         "planning",
		})),
		canonicalExample("agent.run.heartbeat", "Run heartbeat", "Append async liveness and progress metadata.", "wl_example_adapter", "run_example_001", examplePayload(map[string]interface{}{
			"status":   "working",
			"phase":    "testing",
			"progress": 0.6,
			"message":  "metadata-only status",
			"metrics": map[string]interface{}{
				"files_changed": 2,
				"tests_seen":    18,
			},
		})),
		canonicalExample("model.call", "Model call usage", "Record one non-overlapping token usage event.", "wl_example_adapter", "run_example_001", examplePayload(map[string]interface{}{
			"call_id":                     "call_example_001",
			"provider":                    "openai",
			"model":                       "gpt-5.5",
			"input_tokens":                1200,
			"cache_read_input_tokens":     400,
			"cache_creation_input_tokens": 100,
			"output_tokens":               300,
			"reasoning_output_tokens":     80,
			"latency_ms":                  2400,
			"pricing_source":              "official",
			"pricing_confidence":          "exact",
		})),
		canonicalExample("tool.call", "Tool call metadata", "Record one tool invocation without raw params.", "wl_example_adapter", "run_example_001", examplePayload(map[string]interface{}{
			"tool_call_id": "tool_example_001",
			"tool_name":    "shell",
			"tool_type":    "shell",
			"status":       "ok",
			"duration_ms":  180,
			"params_hash":  "sha256:examplehash",
		})),
		canonicalExample("context.ref", "Context reference", "Attach hashed context metadata.", "wl_example_adapter", "run_example_001", examplePayload(map[string]interface{}{
			"context_ref_id": "ctx_example_001",
			"ref_type":       "repo",
			"ref_hash":       "sha256:examplerepohash",
			"label":          "repo alias",
			"repo":           "zhenzhis/agent-ledger",
			"git_branch":     "main",
			"privacy_label":  "team-share",
		})),
		canonicalExample("artifact.created", "Artifact reference", "Record output artifact identity without content.", "wl_example_adapter", "run_example_001", examplePayload(map[string]interface{}{
			"artifact_id":   "artifact_example_001",
			"artifact_type": "patch",
			"label":         "adapter patch",
			"path_hash":     "sha256:examplepathhash",
			"sha256":        "sha256:exampleartifacthash",
		})),
		canonicalExample("evaluation.recorded", "Evaluation signal", "Record a local quality or outcome signal.", "wl_example_adapter", "run_example_001", examplePayload(map[string]interface{}{
			"evaluation_id": "eval_example_001",
			"evaluator":     "local-ci",
			"status":        "pass",
			"score":         0.98,
			"signal":        "unit-tests",
			"notes":         "metadata-only summary",
		})),
		canonicalExample("policy.decision", "Policy decision", "Record advisory policy metadata.", "wl_example_adapter", "run_example_001", examplePayload(map[string]interface{}{
			"decision_id": "policy_example_001",
			"rule_id":     "budget.warn",
			"action":      "warn",
			"reason":      "budget threshold near limit",
			"actor_role":  "agent",
		})),
		canonicalExample("agent.run.finished", "Finish an agent run", "Mark one agent run complete or failed.", "wl_example_adapter", "run_example_001", examplePayload(map[string]interface{}{
			"run_id":      "run_example_001",
			"status":      "completed",
			"exit_code":   0,
			"duration_ms": 125000,
		})),
		canonicalExample("workload.closed", "Close workload", "Close a workload with a short outcome.", "wl_example_adapter", "", examplePayload(map[string]interface{}{
			"status":  "completed",
			"outcome": "adapter integration validated",
		})),
	}
	if filter == "" {
		return examples
	}
	filtered := make([]CanonicalEventExample, 0, 1)
	for _, example := range examples {
		if example.EventType == filter {
			filtered = append(filtered, example)
		}
	}
	return filtered
}

func canonicalExample(eventType, name, description, workloadID, runID string, payload json.RawMessage) CanonicalEventExample {
	eventIDType := strings.ReplaceAll(eventType, ".", "_")
	event := CanonicalEvent{
		EventID:       "example:" + eventIDType,
		Source:        "example-adapter",
		EventType:     eventType,
		SchemaVersion: "v1",
		SourceVersion: "example-agent@1.0.0",
		ParserVersion: "example-adapter@v1",
		SourceEventID: "native:" + eventIDType,
		RawRef:        "example:" + eventIDType + ":row:1",
		MatchType:     "source_reported",
		WorkloadID:    workloadID,
		AgentRunID:    runID,
		SessionID:     "session_example_001",
		Model:         "",
		Project:       "agent-ledger",
		GitBranch:     "main",
		Timestamp:     time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC),
		Payload:       payload,
		Confidence:    0.95,
	}
	if eventType == "model.call" {
		event.Model = "gpt-5.5"
	}
	return CanonicalEventExample{EventType: eventType, Name: name, Description: description, Event: event}
}

func examplePayload(v map[string]interface{}) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return raw
}

// IngestCanonicalEvent stores one canonical event and applies supported ledger projections.
func (d *DB) IngestCanonicalEvent(event CanonicalEvent) (*CanonicalEventResult, error) {
	payload, payloadJSON, err := prepareCanonicalEvent(&event)
	if err != nil {
		return nil, err
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	now := time.Now().UTC()
	res, err := tx.Exec(`INSERT INTO canonical_events(event_id,source,event_type,schema_version,source_version,parser_version,source_event_id,raw_ref,match_type,workload_id,agent_run_id,session_id,model,project,git_branch,timestamp,payload_hash,payload,confidence,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(event_id) DO NOTHING`,
		event.EventID, event.Source, event.EventType, event.SchemaVersion, event.SourceVersion, event.ParserVersion, event.SourceEventID, event.RawRef, event.MatchType, event.WorkloadID, event.AgentRunID, event.SessionID, event.Model,
		event.Project, normalizeBranch(event.GitBranch), event.Timestamp, event.PayloadHash, payloadJSON, event.Confidence, now)
	if err != nil {
		return nil, err
	}
	inserted, _ := res.RowsAffected()
	if inserted == 0 {
		existing := CanonicalEventResult{EventID: event.EventID, Status: "duplicate", EventType: event.EventType}
		_ = tx.QueryRow(`SELECT workload_id,agent_run_id,event_type FROM canonical_events WHERE event_id=?`, event.EventID).Scan(&existing.WorkloadID, &existing.RunID, &existing.EventType)
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return &existing, nil
	}

	derived, err := d.applyCanonicalEvent(tx, &event, payload, now)
	if err != nil {
		return nil, err
	}
	if event.WorkloadID != "" || event.AgentRunID != "" {
		if _, err := tx.Exec(`UPDATE canonical_events SET workload_id=?, agent_run_id=? WHERE event_id=?`, event.WorkloadID, event.AgentRunID, event.EventID); err != nil {
			return nil, err
		}
	}
	if event.WorkloadID != "" && event.SessionID != "" {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO workload_sessions(workload_id,source,session_id,confidence,created_at) VALUES(?,?,?,?,?)`,
			event.WorkloadID, event.Source, event.SessionID, event.Confidence, now); err != nil {
			return nil, err
		}
		sessionProject := firstNonEmptyStorage(event.Project, payloadString(payload, "project"))
		sessionBranch := normalizeBranch(firstNonEmptyStorage(event.GitBranch, payloadString(payload, "git_branch")))
		if _, err := tx.Exec(`INSERT INTO sessions(source,session_id,project,git_branch,start_time,prompts)
			VALUES(?,?,?,?,?,0)
			ON CONFLICT(source,session_id) DO UPDATE SET
				project=CASE WHEN excluded.project!='' THEN excluded.project ELSE project END,
				git_branch=CASE WHEN excluded.git_branch!='' THEN excluded.git_branch ELSE git_branch END,
				start_time=CASE WHEN start_time IS NULL OR excluded.start_time < start_time THEN excluded.start_time ELSE start_time END`,
			event.Source, event.SessionID, sessionProject, sessionBranch, event.Timestamp); err != nil {
			return nil, err
		}
	}
	if event.WorkloadID != "" {
		_, _ = tx.Exec(`UPDATE workloads SET updated_at=? WHERE workload_id=?`, now, event.WorkloadID)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &CanonicalEventResult{EventID: event.EventID, Status: "inserted", EventType: event.EventType, WorkloadID: event.WorkloadID, RunID: event.AgentRunID, Derived: derived}, nil
}

// ValidateCanonicalEvent performs the same metadata-only envelope validation as
// ingest without writing to SQLite.
func ValidateCanonicalEvent(event CanonicalEvent) (*CanonicalEventValidation, error) {
	if _, _, err := prepareCanonicalEvent(&event); err != nil {
		return nil, err
	}
	warnings := canonicalEventWarnings(event)
	status := "valid"
	if len(warnings) > 0 {
		status = "valid_with_warnings"
	}
	return &CanonicalEventValidation{
		EventID:     event.EventID,
		Status:      status,
		EventType:   event.EventType,
		Source:      event.Source,
		PayloadHash: event.PayloadHash,
		Warnings:    warnings,
	}, nil
}

func prepareCanonicalEvent(event *CanonicalEvent) (map[string]interface{}, string, error) {
	event.Source = strings.TrimSpace(event.Source)
	event.EventType = normalizeCanonicalEventType(event.EventType)
	if event.Source == "" {
		return nil, "", fmt.Errorf("source is required")
	}
	if event.EventType == "" {
		return nil, "", fmt.Errorf("event_type is required")
	}
	if !isSupportedCanonicalEventType(event.EventType) {
		return nil, "", fmt.Errorf("unsupported event_type %q", event.EventType)
	}
	if err := normalizeCanonicalEventProvenance(event); err != nil {
		return nil, "", err
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	}
	if event.Confidence <= 0 {
		event.Confidence = 1
	}
	payload, payloadJSON, err := canonicalPayload(event.Payload)
	if err != nil {
		return nil, "", err
	}
	if err := validateCanonicalEventRequirements(*event, payload); err != nil {
		return nil, "", err
	}
	if event.PayloadHash == "" {
		event.PayloadHash = hashPayload(payloadJSON)
	}
	if event.EventID == "" {
		event.EventID = deterministicEventID(*event, event.PayloadHash)
	}
	return payload, payloadJSON, nil
}

func validateCanonicalEventRequirements(event CanonicalEvent, payload map[string]interface{}) error {
	hasWorkload := event.WorkloadID != "" || payloadString(payload, "workload_id") != "" || payloadString(payload, "goal") != ""
	switch event.EventType {
	case "workload.started":
		if payloadString(payload, "goal") == "" {
			return fmt.Errorf("payload.goal is required for %s", event.EventType)
		}
	case "workload.closed":
		if event.WorkloadID == "" && payloadString(payload, "workload_id") == "" {
			return fmt.Errorf("workload_id is required for %s", event.EventType)
		}
	case "agent.run.started":
		if !hasWorkload {
			return fmt.Errorf("workload_id or payload.goal is required for %s", event.EventType)
		}
	case "agent.run.finished", "agent.run.heartbeat":
		if event.AgentRunID == "" && firstPayloadString(payload, "run_id", "agent_run_id") == "" {
			return fmt.Errorf("agent_run_id is required for %s", event.EventType)
		}
	case "model.call":
		if !hasWorkload {
			return fmt.Errorf("workload_id or payload.goal is required for %s", event.EventType)
		}
		if event.Model == "" && payloadString(payload, "model") == "" {
			return fmt.Errorf("model or payload.model is required for %s", event.EventType)
		}
	case "tool.call", "context.ref", "artifact.created", "evaluation.recorded", "policy.decision":
		if !hasWorkload {
			return fmt.Errorf("workload_id or payload.goal is required for %s", event.EventType)
		}
	}
	return nil
}

func canonicalEventWarnings(event CanonicalEvent) []string {
	var warnings []string
	if event.SourceVersion == "" {
		warnings = append(warnings, "source_version is missing")
	}
	if event.ParserVersion == "" {
		warnings = append(warnings, "parser_version is missing")
	}
	if event.RawRef == "" {
		warnings = append(warnings, "raw_ref is missing")
	}
	if event.MatchType == "" {
		warnings = append(warnings, "match_type is missing")
	}
	return warnings
}

func (d *DB) applyCanonicalEvent(tx *sql.Tx, event *CanonicalEvent, payload map[string]interface{}, now time.Time) ([]string, error) {
	switch event.EventType {
	case "workload.started":
		if err := ensureEventWorkload(tx, event, payload, now); err != nil {
			return nil, err
		}
		return []string{"workload"}, nil
	case "workload.closed":
		if err := closeEventWorkload(tx, event, payload, now); err != nil {
			return nil, err
		}
		return []string{"workload"}, nil
	case "agent.run.started":
		if err := ensureEventWorkload(tx, event, payload, now); err != nil {
			return nil, err
		}
		if event.AgentRunID == "" {
			event.AgentRunID = firstPayloadString(payload, "run_id", "agent_run_id")
		}
		if event.AgentRunID == "" {
			event.AgentRunID = generatedID("run")
		}
		_, err := tx.Exec(`INSERT OR IGNORE INTO agent_runs(run_id,workload_id,parent_run_id,source,agent_name,agent_version,command,cwd,status,started_at,confidence)
			VALUES(?,?,?,?,?,?,?,?,?,?,?)`, event.AgentRunID, event.WorkloadID, payloadString(payload, "parent_run_id"), event.Source,
			firstNonEmptyStorage(payloadString(payload, "agent_name"), event.Source), payloadString(payload, "agent_version"),
			redactCommandSecrets(payloadString(payload, "command")), payloadString(payload, "cwd"), firstNonEmptyStorage(payloadString(payload, "status"), "running"), event.Timestamp, event.Confidence)
		if err != nil {
			return nil, err
		}
		return []string{"agent_run"}, nil
	case "agent.run.finished":
		if event.AgentRunID == "" {
			event.AgentRunID = firstPayloadString(payload, "run_id", "agent_run_id")
		}
		if event.AgentRunID == "" {
			return nil, fmt.Errorf("agent_run_id is required for %s", event.EventType)
		}
		status := firstNonEmptyStorage(payloadString(payload, "status"), "completed")
		res, err := tx.Exec(`UPDATE agent_runs SET status=?, exit_code=?, error=?, ended_at=?, duration_ms=? WHERE run_id=?`,
			status, payloadInt(payload, "exit_code"), payloadString(payload, "error"), event.Timestamp, payloadInt64(payload, "duration_ms"), event.AgentRunID)
		if err != nil {
			return nil, err
		}
		affected, _ := res.RowsAffected()
		if affected == 0 {
			return nil, sql.ErrNoRows
		}
		return []string{"agent_run"}, nil
	case "agent.run.heartbeat":
		if event.AgentRunID == "" {
			event.AgentRunID = firstPayloadString(payload, "run_id", "agent_run_id")
		}
		if event.AgentRunID == "" {
			return nil, fmt.Errorf("agent_run_id is required for %s", event.EventType)
		}
		metrics := map[string]interface{}{}
		if rawMetrics, ok := payload["metrics"]; ok && rawMetrics != nil {
			typedMetrics, ok := rawMetrics.(map[string]interface{})
			if !ok {
				return nil, fmt.Errorf("heartbeat metrics must be a JSON object")
			}
			metrics = typedMetrics
		}
		row, err := recordAgentRunHeartbeatTx(tx, agentRunHeartbeatInput{
			EventID:    firstNonEmptyStorage(payloadString(payload, "event_id"), event.SourceEventID),
			RunID:      event.AgentRunID,
			WorkloadID: event.WorkloadID,
			Source:     event.Source,
			Status:     payloadString(payload, "status"),
			Phase:      payloadString(payload, "phase"),
			Message:    payloadString(payload, "message"),
			Progress:   payloadFloat(payload, "progress"),
			Metrics:    metrics,
			Timestamp:  event.Timestamp,
			Confidence: event.Confidence,
		})
		if err != nil {
			return nil, err
		}
		event.WorkloadID = row.WorkloadID
		return []string{"agent_run_heartbeat"}, nil
	case "model.call":
		if err := ensureEventWorkload(tx, event, payload, now); err != nil {
			return nil, err
		}
		if err := hydrateEventWorkloadMetadata(tx, event); err != nil {
			return nil, err
		}
		if event.SessionID == "" {
			event.SessionID = payloadString(payload, "session_id")
		}
		if event.Model == "" {
			event.Model = payloadString(payload, "model")
		}
		if event.Model == "" {
			return nil, fmt.Errorf("model is required for %s", event.EventType)
		}
		callID := firstNonEmptyStorage(payloadString(payload, "call_id"), event.SourceEventID, generatedID("call"))
		sessionID := firstNonEmptyStorage(event.SessionID, payloadString(payload, "session_id"), callID)
		_, err := tx.Exec(`INSERT OR IGNORE INTO model_calls(call_id,workload_id,run_id,source,session_id,provider,model,model_alias,input_tokens,output_tokens,cache_read_input_tokens,cache_creation_input_tokens,reasoning_output_tokens,cost_usd,latency_ms,finish_reason,pricing_source,pricing_confidence,timestamp,confidence)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, callID, event.WorkloadID, event.AgentRunID, event.Source,
			sessionID, payloadString(payload, "provider"), event.Model, payloadString(payload, "model_alias"),
			payloadInt64(payload, "input_tokens"), payloadInt64(payload, "output_tokens"), payloadInt64(payload, "cache_read_input_tokens"),
			payloadInt64(payload, "cache_creation_input_tokens"), payloadInt64(payload, "reasoning_output_tokens"), payloadFloat(payload, "cost_usd"),
			payloadInt64(payload, "latency_ms"), payloadString(payload, "finish_reason"), payloadString(payload, "pricing_source"),
			payloadString(payload, "pricing_confidence"), event.Timestamp, event.Confidence)
		if err != nil {
			return nil, err
		}
		event.SessionID = sessionID
		_, err = tx.Exec(`INSERT OR IGNORE INTO usage_records(source,session_id,model,input_tokens,output_tokens,cache_creation_input_tokens,cache_read_input_tokens,reasoning_output_tokens,cost_usd,timestamp,project,git_branch,pricing_source,pricing_model,pricing_confidence,pricing_note)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			event.Source, sessionID, event.Model, payloadInt64(payload, "input_tokens"), payloadInt64(payload, "output_tokens"),
			payloadInt64(payload, "cache_creation_input_tokens"), payloadInt64(payload, "cache_read_input_tokens"), payloadInt64(payload, "reasoning_output_tokens"),
			payloadFloat(payload, "cost_usd"), event.Timestamp, event.Project, normalizeBranch(event.GitBranch), payloadString(payload, "pricing_source"),
			firstNonEmptyStorage(payloadString(payload, "matched_model"), payloadString(payload, "model_alias")), payloadString(payload, "pricing_confidence"), "canonical model.call projection")
		if err != nil {
			return nil, err
		}
		return []string{"model_call"}, nil
	case "tool.call":
		if err := ensureEventWorkload(tx, event, payload, now); err != nil {
			return nil, err
		}
		toolID := firstNonEmptyStorage(payloadString(payload, "tool_call_id"), event.SourceEventID, generatedID("tool"))
		_, err := tx.Exec(`INSERT OR IGNORE INTO tool_calls(tool_call_id,workload_id,run_id,source,tool_name,tool_type,status,error_class,duration_ms,params_hash,timestamp,confidence)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, toolID, event.WorkloadID, event.AgentRunID, event.Source, payloadString(payload, "tool_name"),
			payloadString(payload, "tool_type"), payloadString(payload, "status"), payloadString(payload, "error_class"),
			payloadInt64(payload, "duration_ms"), payloadString(payload, "params_hash"), event.Timestamp, event.Confidence)
		if err != nil {
			return nil, err
		}
		return []string{"tool_call"}, nil
	case "context.ref":
		if err := ensureEventWorkload(tx, event, payload, now); err != nil {
			return nil, err
		}
		refID := firstNonEmptyStorage(payloadString(payload, "context_ref_id"), event.SourceEventID, generatedID("ctx"))
		_, err := tx.Exec(`INSERT OR IGNORE INTO context_refs(context_ref_id,workload_id,run_id,ref_type,ref_hash,label,repo,git_branch,commit_sha,privacy_label,created_at,confidence)
			VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, refID, event.WorkloadID, event.AgentRunID, payloadString(payload, "ref_type"), payloadString(payload, "ref_hash"),
			payloadString(payload, "label"), payloadString(payload, "repo"), normalizeBranch(firstNonEmptyStorage(payloadString(payload, "git_branch"), event.GitBranch)),
			payloadString(payload, "commit_sha"), firstNonEmptyStorage(payloadString(payload, "privacy_label"), "local"), event.Timestamp, event.Confidence)
		if err != nil {
			return nil, err
		}
		return []string{"context_ref"}, nil
	case "artifact.created":
		if err := ensureEventWorkload(tx, event, payload, now); err != nil {
			return nil, err
		}
		artifactID := firstNonEmptyStorage(payloadString(payload, "artifact_id"), event.SourceEventID, generatedID("art"))
		_, err := tx.Exec(`INSERT OR IGNORE INTO artifacts(artifact_id,workload_id,run_id,artifact_type,label,path_hash,sha256,metadata,created_at,confidence)
			VALUES(?,?,?,?,?,?,?,?,?,?)`, artifactID, event.WorkloadID, event.AgentRunID, payloadString(payload, "artifact_type"),
			payloadString(payload, "label"), payloadString(payload, "path_hash"), payloadString(payload, "sha256"), payloadMetadataJSON(payload), event.Timestamp, event.Confidence)
		if err != nil {
			return nil, err
		}
		return []string{"artifact"}, nil
	case "evaluation.recorded":
		if err := ensureEventWorkload(tx, event, payload, now); err != nil {
			return nil, err
		}
		evaluationID := firstNonEmptyStorage(payloadString(payload, "evaluation_id"), event.SourceEventID, generatedID("eval"))
		_, err := tx.Exec(`INSERT OR IGNORE INTO evaluations(evaluation_id,workload_id,evaluator,status,score,signal,notes,created_at)
			VALUES(?,?,?,?,?,?,?,?)`, evaluationID, event.WorkloadID, firstNonEmptyStorage(payloadString(payload, "evaluator"), "local"),
			payloadString(payload, "status"), payloadFloat(payload, "score"), payloadString(payload, "signal"), payloadString(payload, "notes"), event.Timestamp)
		if err != nil {
			return nil, err
		}
		return []string{"evaluation"}, nil
	case "policy.decision":
		if err := ensureEventWorkload(tx, event, payload, now); err != nil {
			return nil, err
		}
		decisionID := firstNonEmptyStorage(payloadString(payload, "decision_id"), event.SourceEventID, generatedID("pol"))
		action := firstNonEmptyStorage(payloadString(payload, "action"), "allow")
		_, err := tx.Exec(`INSERT OR IGNORE INTO policy_decisions(decision_id,workload_id,run_id,rule_id,action,reason,actor_role,created_at)
			VALUES(?,?,?,?,?,?,?,?)`, decisionID, event.WorkloadID, event.AgentRunID, payloadString(payload, "rule_id"), action,
			payloadString(payload, "reason"), payloadString(payload, "actor_role"), event.Timestamp)
		if err != nil {
			return nil, err
		}
		return []string{"policy_decision"}, nil
	default:
		return nil, fmt.Errorf("unsupported event_type %q", event.EventType)
	}
}

func ensureEventWorkload(tx *sql.Tx, event *CanonicalEvent, payload map[string]interface{}, now time.Time) error {
	if event.WorkloadID == "" {
		event.WorkloadID = firstPayloadString(payload, "workload_id")
	}
	if event.WorkloadID == "" {
		goal := payloadString(payload, "goal")
		if goal == "" {
			return fmt.Errorf("workload_id or payload.goal is required for %s", event.EventType)
		}
		event.WorkloadID = generatedID("wl")
	}
	goal := firstNonEmptyStorage(payloadString(payload, "goal"), event.WorkloadID)
	source := firstNonEmptyStorage(payloadString(payload, "source"), event.Source)
	project := firstNonEmptyStorage(payloadString(payload, "project"), event.Project)
	branch := normalizeBranch(firstNonEmptyStorage(payloadString(payload, "git_branch"), event.GitBranch))
	_, err := tx.Exec(`INSERT OR IGNORE INTO workloads(workload_id,goal,status,source,project,repo,git_branch,owner,team,budget_usd,outcome,confidence,created_at,updated_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, event.WorkloadID, goal, "active", source, project, payloadString(payload, "repo"), branch,
		payloadString(payload, "owner"), payloadString(payload, "team"), payloadFloat(payload, "budget_usd"), "", event.Confidence, now, now)
	return err
}

func hydrateEventWorkloadMetadata(tx *sql.Tx, event *CanonicalEvent) error {
	if event.WorkloadID == "" {
		return nil
	}
	if event.Project != "" && normalizeBranch(event.GitBranch) != "unknown" {
		return nil
	}
	var project, branch string
	err := tx.QueryRow(`SELECT COALESCE(project,''),COALESCE(git_branch,'') FROM workloads WHERE workload_id=?`, event.WorkloadID).Scan(&project, &branch)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil
		}
		return err
	}
	if event.Project == "" {
		event.Project = project
	}
	if normalizeBranch(event.GitBranch) == "unknown" && branch != "" {
		event.GitBranch = branch
	}
	return nil
}

func closeEventWorkload(tx *sql.Tx, event *CanonicalEvent, payload map[string]interface{}, now time.Time) error {
	if event.WorkloadID == "" {
		event.WorkloadID = firstPayloadString(payload, "workload_id")
	}
	if event.WorkloadID == "" {
		return fmt.Errorf("workload_id is required for %s", event.EventType)
	}
	status := firstNonEmptyStorage(payloadString(payload, "status"), "completed")
	if !validWorkloadStatus(status) {
		return fmt.Errorf("unsupported workload status %q", status)
	}
	res, err := tx.Exec(`UPDATE workloads SET status=?, outcome=?, updated_at=?, closed_at=? WHERE workload_id=?`,
		status, payloadString(payload, "outcome"), now, now, event.WorkloadID)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func canonicalPayload(raw json.RawMessage) (map[string]interface{}, string, error) {
	if len(raw) == 0 {
		return map[string]interface{}{}, "{}", nil
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, "", fmt.Errorf("payload must be a JSON object: %w", err)
	}
	if payload == nil {
		payload = map[string]interface{}{}
	}
	if containsPromptContentKey(payload) {
		return nil, "", fmt.Errorf("payload appears to contain prompt/content text; store hashes or metadata only")
	}
	stable, err := json.Marshal(payload)
	if err != nil {
		return nil, "", err
	}
	return payload, string(stable), nil
}

func containsPromptContentKey(v interface{}) bool {
	switch x := v.(type) {
	case map[string]interface{}:
		for k, child := range x {
			switch strings.ToLower(k) {
			case "prompt", "prompts", "messages", "content", "input_text", "output_text", "completion", "transcript":
				return true
			}
			if containsPromptContentKey(child) {
				return true
			}
		}
	case []interface{}:
		for _, child := range x {
			if containsPromptContentKey(child) {
				return true
			}
		}
	}
	return false
}

func normalizeCanonicalEventProvenance(event *CanonicalEvent) error {
	event.SchemaVersion = strings.TrimSpace(event.SchemaVersion)
	if event.SchemaVersion == "" {
		event.SchemaVersion = "v1"
	}
	event.SourceVersion = strings.TrimSpace(event.SourceVersion)
	event.ParserVersion = strings.TrimSpace(event.ParserVersion)
	event.RawRef = strings.TrimSpace(event.RawRef)
	event.MatchType = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(event.MatchType, "-", "_")))
	if err := validateProvenanceField("schema_version", event.SchemaVersion, 64); err != nil {
		return err
	}
	if err := validateProvenanceField("source_version", event.SourceVersion, 128); err != nil {
		return err
	}
	if err := validateProvenanceField("parser_version", event.ParserVersion, 128); err != nil {
		return err
	}
	if err := validateProvenanceField("raw_ref", event.RawRef, 512); err != nil {
		return err
	}
	if err := validateProvenanceField("match_type", event.MatchType, 64); err != nil {
		return err
	}
	return nil
}

func validateProvenanceField(name, value string, maxLen int) error {
	if value == "" {
		return nil
	}
	if len(value) > maxLen {
		return fmt.Errorf("%s is too long: max %d bytes", name, maxLen)
	}
	for _, r := range value {
		if r < 0x20 || r == 0x7f {
			return fmt.Errorf("%s contains control characters", name)
		}
	}
	return nil
}

func normalizeCanonicalEventType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "workload_started", "workload.start", "workload.started":
		return "workload.started"
	case "workload_closed", "workload.close", "workload.closed":
		return "workload.closed"
	case "agent_run_started", "run.started", "agent.run.start", "agent.run.started":
		return "agent.run.started"
	case "agent_run_finished", "run.finished", "agent.run.finish", "agent.run.finished":
		return "agent.run.finished"
	case "agent_run_heartbeat", "run.heartbeat", "agent.run.ping", "agent.run.heartbeat":
		return "agent.run.heartbeat"
	case "model_call", "model.call":
		return "model.call"
	case "tool_call", "tool.call":
		return "tool.call"
	case "context_ref", "context.ref":
		return "context.ref"
	case "artifact_created", "artifact.created":
		return "artifact.created"
	case "evaluation_recorded", "evaluation.recorded":
		return "evaluation.recorded"
	case "policy_decision", "policy.decision":
		return "policy.decision"
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

func isSupportedCanonicalEventType(eventType string) bool {
	for _, info := range CanonicalEventTypes() {
		if info.EventType == eventType {
			return true
		}
	}
	return false
}

func deterministicEventID(event CanonicalEvent, payloadHash string) string {
	parts := []string{
		event.Source, event.EventType, event.SourceEventID, event.WorkloadID, event.AgentRunID,
		event.SessionID, event.Model, event.Project, event.GitBranch, event.Timestamp.UTC().Format(time.RFC3339Nano), payloadHash,
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "evt_" + hex.EncodeToString(sum[:16])
}

func hashPayload(payload string) string {
	sum := sha256.Sum256([]byte(payload))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func payloadString(payload map[string]interface{}, key string) string {
	v, ok := payload[key]
	if !ok || v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case fmt.Stringer:
		return strings.TrimSpace(x.String())
	default:
		return strings.TrimSpace(fmt.Sprint(x))
	}
}

func firstPayloadString(payload map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		if value := payloadString(payload, key); value != "" {
			return value
		}
	}
	return ""
}

func payloadInt(payload map[string]interface{}, key string) int {
	return int(payloadInt64(payload, key))
}

func payloadInt64(payload map[string]interface{}, key string) int64 {
	v, ok := payload[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case json.Number:
		n, _ := x.Int64()
		return n
	case string:
		n, _ := strconv.ParseInt(strings.TrimSpace(x), 10, 64)
		return n
	default:
		return 0
	}
}

func payloadFloat(payload map[string]interface{}, key string) float64 {
	v, ok := payload[key]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		n, _ := x.Float64()
		return n
	case string:
		n, _ := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return n
	default:
		return 0
	}
}

func payloadMetadataJSON(payload map[string]interface{}) string {
	if metadata, ok := payload["metadata"]; ok && metadata != nil {
		raw, _ := json.Marshal(metadata)
		return string(raw)
	}
	return "{}"
}

func firstNonEmptyStorage(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
