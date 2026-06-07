package storage

import (
	"encoding/json"
	"path/filepath"
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
	for _, eventType := range []string{"workload.started", "agent.run.started", "model.call", "tool.call", "policy.decision"} {
		if !seen[eventType] {
			t.Fatalf("schema missing %s", eventType)
		}
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

func rawJSON(t *testing.T, value map[string]interface{}) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}
