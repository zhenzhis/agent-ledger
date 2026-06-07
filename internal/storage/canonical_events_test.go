package storage

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestIngestCanonicalEventBuildsWorkloadLedger(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)

	start, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:   "evt-start",
		Source:    "codex",
		EventType: "workload.started",
		Timestamp: ts,
		Payload: rawJSON(t, map[string]interface{}{
			"goal":       "ship event ledger",
			"project":    "agent-ledger",
			"repo":       "zhenzhis/agent-ledger",
			"git_branch": "main",
		}),
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if start.Status != "inserted" || start.WorkloadID == "" {
		t.Fatalf("unexpected start result: %#v", start)
	}
	dup, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:   "evt-start",
		Source:    "codex",
		EventType: "workload.started",
		Timestamp: ts,
		Payload:   rawJSON(t, map[string]interface{}{"goal": "ship event ledger"}),
	})
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if dup.Status != "duplicate" || dup.WorkloadID != start.WorkloadID {
		t.Fatalf("unexpected duplicate result: %#v", dup)
	}

	run, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:    "evt-run",
		Source:     "codex",
		EventType:  "agent.run.started",
		WorkloadID: start.WorkloadID,
		Timestamp:  ts.Add(time.Minute),
		Payload: rawJSON(t, map[string]interface{}{
			"run_id":     "run-event-ledger",
			"agent_name": "codex",
			"command":    "codex",
			"cwd":        "C:/work/agent-ledger",
		}),
	})
	if err != nil {
		t.Fatalf("run start: %v", err)
	}
	if run.RunID != "run-event-ledger" {
		t.Fatalf("run id=%q", run.RunID)
	}

	heartbeat, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:    "evt-heartbeat",
		Source:     "codex",
		EventType:  "agent.run.heartbeat",
		WorkloadID: start.WorkloadID,
		AgentRunID: run.RunID,
		Timestamp:  ts.Add(90 * time.Second),
		Payload: rawJSON(t, map[string]interface{}{
			"status":   "working",
			"phase":    "editing",
			"progress": 0.4,
			"message":  "editing tracked files",
			"metrics":  map[string]interface{}{"files_touched": 3},
		}),
	})
	if err != nil {
		t.Fatalf("heartbeat: %v", err)
	}
	if heartbeat.WorkloadID != start.WorkloadID || heartbeat.RunID != run.RunID {
		t.Fatalf("unexpected heartbeat result: %#v", heartbeat)
	}

	events := []CanonicalEvent{
		{
			EventID:    "evt-model",
			Source:     "codex",
			EventType:  "model.call",
			WorkloadID: start.WorkloadID,
			AgentRunID: run.RunID,
			SessionID:  "sess-event-ledger",
			Model:      "gpt-5.5",
			Timestamp:  ts.Add(2 * time.Minute),
			Payload: rawJSON(t, map[string]interface{}{
				"provider":                    "openai",
				"input_tokens":                1000,
				"cache_read_input_tokens":     200,
				"cache_creation_input_tokens": 50,
				"output_tokens":               300,
				"cost_usd":                    0.42,
				"pricing_source":              "official",
				"pricing_confidence":          "exact",
			}),
		},
		{
			EventID:    "evt-tool",
			Source:     "codex",
			EventType:  "tool.call",
			WorkloadID: start.WorkloadID,
			AgentRunID: run.RunID,
			Timestamp:  ts.Add(3 * time.Minute),
			Payload: rawJSON(t, map[string]interface{}{
				"tool_name":   "shell",
				"tool_type":   "command",
				"status":      "ok",
				"params_hash": "sha256:cmd",
			}),
		},
		{
			EventID:    "evt-context",
			Source:     "codex",
			EventType:  "context.ref",
			WorkloadID: start.WorkloadID,
			AgentRunID: run.RunID,
			Timestamp:  ts.Add(3*time.Minute + 30*time.Second),
			Payload: rawJSON(t, map[string]interface{}{
				"ref_type":      "repo",
				"ref_hash":      "sha256:context",
				"label":         "agent-ledger working tree",
				"repo":          "zhenzhis/agent-ledger",
				"git_branch":    "main",
				"commit_sha":    "abc123",
				"privacy_label": "local",
			}),
		},
		{
			EventID:    "evt-artifact",
			Source:     "codex",
			EventType:  "artifact.created",
			WorkloadID: start.WorkloadID,
			AgentRunID: run.RunID,
			Timestamp:  ts.Add(4 * time.Minute),
			Payload: rawJSON(t, map[string]interface{}{
				"artifact_type": "report",
				"label":         "summary",
				"path_hash":     "sha256:path",
				"sha256":        "sha256:file",
				"metadata":      map[string]interface{}{"format": "markdown"},
			}),
		},
		{
			EventID:    "evt-eval",
			Source:     "codex",
			EventType:  "evaluation.recorded",
			WorkloadID: start.WorkloadID,
			Timestamp:  ts.Add(5 * time.Minute),
			Payload: rawJSON(t, map[string]interface{}{
				"evaluator": "local",
				"status":    "pass",
				"score":     0.9,
				"signal":    "tests",
			}),
		},
		{
			EventID:    "evt-policy",
			Source:     "codex",
			EventType:  "policy.decision",
			WorkloadID: start.WorkloadID,
			AgentRunID: run.RunID,
			Timestamp:  ts.Add(6 * time.Minute),
			Payload: rawJSON(t, map[string]interface{}{
				"rule_id":    "budget-warn",
				"action":     "warn",
				"reason":     "near local budget",
				"actor_role": "operator",
			}),
		},
	}
	for _, event := range events {
		if _, err := db.IngestCanonicalEvent(event); err != nil {
			t.Fatalf("%s: %v", event.EventID, err)
		}
	}

	detail, err := db.GetWorkloadDetail(start.WorkloadID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Summary.Goal != "ship event ledger" || detail.Summary.Project != "agent-ledger" {
		t.Fatalf("summary=%#v", detail.Summary)
	}
	if len(detail.Runs) != 1 || len(detail.ModelCalls) != 1 || len(detail.ToolCalls) != 1 {
		t.Fatalf("runs=%d model_calls=%d tool_calls=%d", len(detail.Runs), len(detail.ModelCalls), len(detail.ToolCalls))
	}
	if len(detail.ContextRefs) != 1 || detail.ContextRefs[0].RefHash != "sha256:context" || detail.ContextRefs[0].Label != "agent-ledger working tree" {
		t.Fatalf("context refs=%#v", detail.ContextRefs)
	}
	if detail.Runs[0].HeartbeatCount != 1 || detail.Runs[0].Phase != "editing" || detail.Runs[0].Progress != 0.4 {
		t.Fatalf("run heartbeat snapshot not updated: %#v", detail.Runs[0])
	}
	if len(detail.RunEvents) != 1 || detail.RunEvents[0].EventType != "agent.run.heartbeat" || detail.RunEvents[0].Status != "working" {
		t.Fatalf("run heartbeat event missing: %#v", detail.RunEvents)
	}
	if detail.ModelCalls[0].Tokens != 1550 || detail.ModelCalls[0].CostUSD != 0.42 {
		t.Fatalf("model call=%#v", detail.ModelCalls[0])
	}
	callRows, err := db.GetModelCalls(ts.Add(-time.Hour), ts.Add(time.Hour), "codex", "", "", 10)
	if err != nil {
		t.Fatalf("model call analytics: %v", err)
	}
	if len(callRows) != 1 || callRows[0].Calls != 1 || callRows[0].Tokens != 1550 || callRows[0].CostUSD != 0.42 {
		t.Fatalf("usage projection missing from analytics: %+v", callRows)
	}
	if len(detail.Artifacts) != 1 || len(detail.Evaluations) != 1 || len(detail.Policies) != 1 {
		t.Fatalf("artifacts=%d evaluations=%d policies=%d", len(detail.Artifacts), len(detail.Evaluations), len(detail.Policies))
	}
	if len(detail.Sessions) != 1 || detail.Sessions[0].SessionID != "sess-event-ledger" {
		t.Fatalf("sessions=%#v", detail.Sessions)
	}
	graph, err := db.GetWorkloadGraph(start.WorkloadID)
	if err != nil {
		t.Fatalf("graph: %v", err)
	}
	var contextNode, contextEdge bool
	for _, node := range graph.Nodes {
		if node.Kind == "context" && node.Label == "agent-ledger working tree" {
			contextNode = true
		}
	}
	for _, edge := range graph.Edges {
		if edge.To == detail.ContextRefs[0].ContextRefID && edge.Label == "context" {
			contextEdge = true
		}
	}
	if !contextNode || !contextEdge {
		t.Fatalf("context missing from graph: nodes=%#v edges=%#v", graph.Nodes, graph.Edges)
	}
	timeline, err := db.GetWorkloadTimeline(start.WorkloadID, 100)
	if err != nil {
		t.Fatalf("timeline: %v", err)
	}
	seenKinds := map[string]bool{}
	for _, row := range timeline {
		seenKinds[row.Kind] = true
	}
	for _, kind := range []string{"workload", "agent_run", "run_event", "model_call", "tool_call", "context_ref", "artifact", "evaluation", "policy"} {
		if !seenKinds[kind] {
			t.Fatalf("timeline missing %s: %#v", kind, timeline)
		}
	}
}

func TestCanonicalEventSchemaListsCoreTypes(t *testing.T) {
	schema := CanonicalEventSchema()
	if schema["version"] != "v1" {
		t.Fatalf("schema version=%#v", schema["version"])
	}
	types, ok := schema["event_types"].([]CanonicalEventTypeInfo)
	if !ok || len(types) == 0 {
		t.Fatalf("missing event types: %#v", schema["event_types"])
	}
	seen := map[string]bool{}
	for _, info := range types {
		seen[info.EventType] = true
	}
	for _, eventType := range []string{"workload.started", "agent.run.started", "agent.run.heartbeat", "model.call", "tool.call", "policy.decision"} {
		if !seen[eventType] {
			t.Fatalf("schema missing %s", eventType)
		}
	}
}

func TestCanonicalEventExamplesValidate(t *testing.T) {
	examples := CanonicalEventExamples("")
	if len(examples) < 10 {
		t.Fatalf("expected examples for core event types, got %d", len(examples))
	}
	for _, example := range examples {
		result, err := ValidateCanonicalEvent(example.Event)
		if err != nil {
			t.Fatalf("%s example failed validation: %v", example.EventType, err)
		}
		if result.Status != "valid" {
			t.Fatalf("%s example should have full provenance: %#v", example.EventType, result)
		}
	}
	filtered := CanonicalEventExamples("model.call")
	if len(filtered) != 1 || filtered[0].EventType != "model.call" {
		t.Fatalf("model.call filter failed: %#v", filtered)
	}
}

func TestIngestCanonicalEventRejectsPromptContent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()

	_, err = db.IngestCanonicalEvent(CanonicalEvent{
		Source:    "codex",
		EventType: "workload.started",
		Payload:   rawJSON(t, map[string]interface{}{"goal": "bad", "prompt": "raw user prompt"}),
	})
	if err == nil {
		t.Fatal("expected prompt content rejection")
	}
}

func TestIngestCanonicalRunRedactsCommandSecrets(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	start, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:   "evt-secret-run-workload",
		Source:    "codex",
		EventType: "workload.started",
		Payload:   rawJSON(t, map[string]interface{}{"goal": "secret run", "project": "agent-ledger"}),
	})
	if err != nil {
		t.Fatalf("workload start: %v", err)
	}
	if _, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:    "evt-secret-run",
		Source:     "codex",
		EventType:  "agent.run.started",
		WorkloadID: start.WorkloadID,
		Payload: rawJSON(t, map[string]interface{}{
			"run_id":  "run-secret-command",
			"command": "ANTHROPIC_API_KEY=sk-ant-test codex --api-key=sk-openai --secret secret-value",
		}),
	}); err != nil {
		t.Fatalf("run start: %v", err)
	}
	detail, err := db.GetWorkloadDetail(start.WorkloadID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(detail.Runs) != 1 {
		t.Fatalf("runs=%#v", detail.Runs)
	}
	command := detail.Runs[0].Command
	for _, leaked := range []string{"sk-ant-test", "sk-openai", "secret-value"} {
		if strings.Contains(command, leaked) {
			t.Fatalf("command leaked %q: %s", leaked, command)
		}
	}
}

func TestAgentRunHeartbeatRejectsPromptMetrics(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	workloadID, err := db.CreateWorkload("async run", "codex", "agent-ledger", "", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "codex", "codex", "C:/work/agent-ledger")
	if err != nil {
		t.Fatalf("StartAgentRun: %v", err)
	}
	_, err = db.RecordAgentRunHeartbeat("", runID, "running", "planning", "working", 0.1, map[string]interface{}{"messages": []string{"raw prompt"}}, time.Time{}, 1)
	if err == nil {
		t.Fatal("expected heartbeat metrics prompt/content rejection")
	}
}

func TestCanonicalEventProvenancePersistsAndExports(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	schema := CanonicalEventSchema()
	envelope, ok := schema["envelope_fields"].(map[string]string)
	if !ok {
		t.Fatalf("schema envelope has unexpected shape: %#v", schema["envelope_fields"])
	}
	for _, field := range []string{"schema_version", "source_version", "parser_version", "raw_ref", "match_type"} {
		if envelope[field] == "" {
			t.Fatalf("schema missing provenance field %q", field)
		}
	}

	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	_, err = db.IngestCanonicalEvent(CanonicalEvent{
		EventID:       "evt-provenance",
		Source:        "codex",
		EventType:     "workload.started",
		SchemaVersion: "v1",
		SourceVersion: "codex-cli 1.2.3",
		ParserVersion: "agent-ledger-codex-adapter@v2",
		SourceEventID: "native-turn-42",
		RawRef:        "sha256:source-file#byte=1024",
		MatchType:     "source-reported",
		Timestamp:     ts,
		Payload: rawJSON(t, map[string]interface{}{
			"goal":    "track provenance",
			"project": "agent-ledger",
		}),
	})
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	var schemaVersion, sourceVersion, parserVersion, rawRef, matchType string
	if err := db.db.QueryRow(`SELECT schema_version,source_version,parser_version,raw_ref,match_type FROM canonical_events WHERE event_id=?`, "evt-provenance").
		Scan(&schemaVersion, &sourceVersion, &parserVersion, &rawRef, &matchType); err != nil {
		t.Fatalf("query provenance: %v", err)
	}
	if schemaVersion != "v1" || sourceVersion != "codex-cli 1.2.3" || parserVersion != "agent-ledger-codex-adapter@v2" || rawRef != "sha256:source-file#byte=1024" || matchType != "source_reported" {
		t.Fatalf("unexpected provenance: schema=%q source=%q parser=%q raw_ref=%q match=%q", schemaVersion, sourceVersion, parserVersion, rawRef, matchType)
	}

	events, err := db.GetCanonicalEvents(ts.Add(-time.Minute), ts.Add(time.Minute), "codex", "", "", 10)
	if err != nil {
		t.Fatalf("export canonical events: %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events=%#v", events)
	}
	exported := events[0]
	if exported.SchemaVersion != "v1" || exported.SourceVersion != "codex-cli 1.2.3" || exported.ParserVersion != "agent-ledger-codex-adapter@v2" || exported.RawRef != "sha256:source-file#byte=1024" || exported.MatchType != "source_reported" {
		t.Fatalf("export lost provenance: %#v", exported)
	}
}

func TestValidateCanonicalEventDryRun(t *testing.T) {
	result, err := ValidateCanonicalEvent(CanonicalEvent{
		Source:    "codex",
		EventType: "workload.started",
		Payload:   rawJSON(t, map[string]interface{}{"goal": "validate adapter"}),
	})
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if result.EventID == "" || result.PayloadHash == "" || result.EventType != "workload.started" || result.Source != "codex" {
		t.Fatalf("unexpected validation result: %#v", result)
	}
	if result.Status != "valid_with_warnings" || len(result.Warnings) == 0 {
		t.Fatalf("expected provenance warnings: %#v", result)
	}
	if _, err := ValidateCanonicalEvent(CanonicalEvent{
		Source:    "codex",
		EventType: "unknown.event",
		Payload:   rawJSON(t, map[string]interface{}{}),
	}); err == nil {
		t.Fatal("expected unsupported event type error")
	}
}

func rawJSON(t *testing.T, value map[string]interface{}) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
