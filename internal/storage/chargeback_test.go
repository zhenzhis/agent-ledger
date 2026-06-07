package storage

import (
	"testing"
	"time"
)

func TestGetChargebackUsesRawUsageAndTeamMapping(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "codex", SessionID: "s1", Model: "gpt-5", Project: "alpha", Timestamp: ts, InputTokens: 100, OutputTokens: 50, CostUSD: 1.5},
		{Source: "codex", SessionID: "s2", Model: "gpt-5", Project: "alpha", Timestamp: ts.Add(time.Minute), InputTokens: 200, OutputTokens: 50, CostUSD: 2.5},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	if _, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:    "evt-should-not-double-count",
		Source:     "gateway",
		EventType:  "model.call",
		WorkloadID: "wl-no-double-count",
		SessionID:  "canonical",
		Model:      "gpt-5",
		Project:    "alpha",
		Timestamp:  ts,
		Payload:    rawJSON(t, map[string]interface{}{"input_tokens": 10000, "cost_usd": 99}),
	}); err != nil {
		t.Fatalf("IngestCanonicalEvent: %v", err)
	}

	rows, err := db.GetChargeback(ts.Add(-time.Hour), ts.Add(time.Hour), "", "", "", map[string]string{"project:alpha": "research"}, "", "", 10)
	if err != nil {
		t.Fatalf("GetChargeback: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d %+v", len(rows), rows)
	}
	row := rows[0]
	if row.Team != "research" || row.MappingSource != "project:alpha" || row.DataSource != "usage_records" {
		t.Fatalf("unexpected mapping: %+v", row)
	}
	if row.Calls != 2 || row.Sessions != 2 || row.Tokens != 400 || !near(row.CostUSD, 4.0, 0.000001) {
		t.Fatalf("unexpected raw aggregation: %+v", row)
	}
}

func TestGetChargebackFallsBackToCanonicalWorkloadTeam(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	start, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:   "evt-chargeback-workload",
		Source:    "gateway",
		EventType: "workload.started",
		Timestamp: ts,
		Payload: rawJSON(t, map[string]interface{}{
			"goal":    "canonical team showback",
			"project": "agent-ledger",
			"repo":    "zhenzhis/agent-ledger",
			"team":    "platform",
		}),
	})
	if err != nil {
		t.Fatalf("workload.started: %v", err)
	}
	if _, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:    "evt-chargeback-call",
		Source:     "gateway",
		EventType:  "model.call",
		WorkloadID: start.WorkloadID,
		SessionID:  "s-canon",
		Model:      "gpt-5",
		Timestamp:  ts.Add(time.Minute),
		Payload:    rawJSON(t, map[string]interface{}{"input_tokens": 100, "output_tokens": 25, "cost_usd": 1.25}),
	}); err != nil {
		t.Fatalf("model.call: %v", err)
	}

	rows, err := db.GetChargeback(ts.Add(-time.Hour), ts.Add(time.Hour), "gateway", "", "", nil, "", "", 10)
	if err != nil {
		t.Fatalf("GetChargeback: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d %+v", len(rows), rows)
	}
	row := rows[0]
	if row.Team != "platform" || row.MappingSource != "workload.team" || row.DataSource != "model_calls" {
		t.Fatalf("unexpected canonical mapping: %+v", row)
	}
	if row.Calls != 1 || row.Sessions != 1 || row.Tokens != 125 || !near(row.CostUSD, 1.25, 0.000001) {
		t.Fatalf("unexpected canonical aggregation: %+v", row)
	}
}
