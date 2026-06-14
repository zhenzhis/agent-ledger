package integrations

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestConvertA2ATaskSnapshotToCanonicalEvents(t *testing.T) {
	raw := []byte(`{
		"id":"task-123",
		"contextId":"ctx-abc",
		"kind":"task",
		"status":{"state":"completed","timestamp":"2026-06-07T11:00:00Z","message":{"role":"agent","parts":[{"kind":"text","text":"must not persist"}]}},
		"history":[{"role":"user","parts":[{"kind":"text","text":"secret request"}]}],
		"artifacts":[{
			"artifactId":"artifact-1",
			"name":"summary",
			"description":"short report",
			"parts":[{"kind":"text","text":"artifact body must not persist"},{"kind":"data","data":{"x":1}}],
			"metadata":{"sha256":"abc123"}
		}],
		"metadata":{
			"agent_ledger.goal":"delegate research",
			"agent_ledger.project":"agent-ledger",
			"agent_ledger.agent_name":"remote-researcher"
		}
	}`)
	tasks, err := DecodeA2ATasks(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	events, err := ConvertA2ATasks(tasks)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	for _, want := range []string{"workload.started", "agent.run.started", "context.ref", "artifact.created", "agent.run.finished", "workload.closed", "evaluation.recorded"} {
		if findCanonical(events, want).EventType == "" {
			t.Fatalf("%s missing in %#v", want, events)
		}
	}
	workloadID := findCanonical(events, "workload.started").WorkloadID
	if workloadID == "" {
		t.Fatal("missing workload id")
	}
	for _, event := range events {
		if event.WorkloadID != workloadID {
			t.Fatalf("events must share workload: %s != %s for %s", event.WorkloadID, workloadID, event.EventType)
		}
		if string(event.Payload) == "" {
			t.Fatalf("missing payload for %s", event.EventType)
		}
		if event.SchemaVersion != "v1" || event.SourceVersion == "" || event.ParserVersion != "agent-ledger-a2a@v1" || event.RawRef == "" || event.MatchType == "" {
			t.Fatalf("A2A provenance missing for %s: %#v", event.EventType, event)
		}
		if containsAny(string(event.Payload), "must not persist", "secret request", "artifact body") {
			t.Fatalf("sensitive A2A content leaked in %s payload: %s", event.EventType, string(event.Payload))
		}
	}
	artifact := findCanonical(events, "artifact.created")
	var payload map[string]interface{}
	if err := json.Unmarshal(artifact.Payload, &payload); err != nil {
		t.Fatalf("artifact payload: %v", err)
	}
	metadata := payload["metadata"].(map[string]interface{})
	if metadata["part_count"].(float64) != 2 {
		t.Fatalf("part count missing: %#v", metadata)
	}
}

func TestConvertA2ADelegatedTaskLineageAndEvidence(t *testing.T) {
	raw := []byte(`{
		"id":"task-child",
		"contextId":"ctx-child",
		"parentTaskId":"task-parent",
		"status":{"state":"working","timestamp":"2026-06-07T11:10:00Z"},
		"metadata":{
			"agent_ledger.goal":"delegated execution",
			"agent_ledger.parent_goal":"parent strategy workload",
			"agent_ledger.project":"agent-ledger",
			"agent_ledger.agent_name":"remote-builder",
			"agent_ledger.evidence_refs":[
				{"id":"ev-fixture-1","ref_type":"evidence_bundle","ref_hash":"sha256:evidencebundle","label":"delegation evidence","repo":"zhenzhis/agent-ledger","git_branch":"main","commit_sha":"abc123","privacy_label":"team-share"},
				{"id":"ev-fixture-2","type":"issue","source_ref":"issue-42","label":"upstream issue"}
			]
		}
	}`)
	tasks, err := DecodeA2ATasks(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	events, err := ConvertA2ATasks(tasks)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	parent := findEventByID(events, "a2a:parent-workload:task-child:"+deterministicLedgerID("wl", "a2a-task:task-parent"))
	child := findEventByID(events, "a2a:workload:task-child")
	link := findEventByID(events, "a2a:workload-link:task-child:"+parent.WorkloadID)
	if parent.EventType != "workload.started" || parent.MatchType != "reconstructed" || parent.WorkloadID == "" {
		t.Fatalf("parent placeholder missing: %#v", parent)
	}
	if child.EventType != "workload.started" || child.WorkloadID == "" || child.WorkloadID == parent.WorkloadID {
		t.Fatalf("child workload missing: child=%#v parent=%#v", child, parent)
	}
	if link.EventType != "workload.linked" || link.WorkloadID != child.WorkloadID {
		t.Fatalf("lineage link missing: %#v", link)
	}
	var linkPayload map[string]interface{}
	if err := json.Unmarshal(link.Payload, &linkPayload); err != nil {
		t.Fatalf("link payload: %v", err)
	}
	if linkPayload["target_workload_id"] != parent.WorkloadID || linkPayload["relation"] != "spawned_by" {
		t.Fatalf("unexpected link payload: %#v", linkPayload)
	}
	var evidenceCount int
	for _, event := range events {
		if strings.HasPrefix(event.EventID, "a2a:evidence:") {
			evidenceCount++
			if event.EventType != "context.ref" || event.AgentRunID == "" {
				t.Fatalf("unexpected evidence context: %#v", event)
			}
			if containsAny(string(event.Payload), "source_ref", "issue-42", "must not persist", "secret") {
				t.Fatalf("evidence leaked raw source ref: %s", string(event.Payload))
			}
		}
	}
	if evidenceCount != 2 {
		t.Fatalf("evidence context count=%d events=%#v", evidenceCount, events)
	}

	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	for _, event := range events {
		if _, err := db.IngestCanonicalEvent(event); err != nil {
			t.Fatalf("ingest %s %s: %v", event.EventType, event.EventID, err)
		}
	}
	detail, err := db.GetWorkloadDetail(child.WorkloadID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(detail.Links) != 1 || detail.Links[0].TargetWorkloadID != parent.WorkloadID || detail.Links[0].Relation != "spawned_by" {
		t.Fatalf("ingested lineage missing: %#v", detail.Links)
	}
	if len(detail.ContextRefs) != 3 {
		t.Fatalf("expected base context plus two evidence refs, got %#v", detail.ContextRefs)
	}
}

func TestDecodeA2AJSONRPCResultAndEvent(t *testing.T) {
	raw := []byte(`[
		{"jsonrpc":"2.0","id":"1","result":{"task":{"id":"task-rpc","contextId":"ctx-rpc","status":{"state":"working"},"metadata":{"agent_ledger.goal":"rpc task"}}}},
		{"taskId":"task-event","contextId":"ctx-event","status":{"state":"failed","timestamp":"2026-06-07T11:05:00Z"},"metadata":{"agent_ledger.goal":"event task"}}
	]`)
	tasks, err := DecodeA2ATasks(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("tasks=%d", len(tasks))
	}
	events, err := ConvertA2ATasks(tasks)
	if err != nil {
		t.Fatalf("convert: %v", err)
	}
	if findEventByID(events, "a2a:workload:task-rpc").EventType != "workload.started" {
		t.Fatalf("rpc task missing")
	}
	if findEventByID(events, "a2a:workload-closed:task-event:failed").EventType != "workload.closed" {
		t.Fatalf("failed task close missing")
	}
}

func findCanonical(events []storage.CanonicalEvent, eventType string) storage.CanonicalEvent {
	for _, event := range events {
		if event.EventType == eventType {
			return event
		}
	}
	return storage.CanonicalEvent{}
}

func findEventByID(events []storage.CanonicalEvent, eventID string) storage.CanonicalEvent {
	for _, event := range events {
		if event.EventID == eventID {
			return event
		}
	}
	return storage.CanonicalEvent{}
}

func containsAny(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(haystack, needle) {
			return true
		}
	}
	return false
}
