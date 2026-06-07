package storage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestEstimatePreflightCost(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "codex", SessionID: "s1", Model: "m", Project: "alpha", Timestamp: ts, InputTokens: 100, OutputTokens: 100, CostUSD: 2},
		{Source: "codex", SessionID: "s1", Model: "m", Project: "alpha", Timestamp: ts.Add(10 * time.Minute), InputTokens: 100, CostUSD: 1},
		{Source: "codex", SessionID: "s2", Model: "m", Project: "alpha", Timestamp: ts.Add(time.Hour), InputTokens: 500, OutputTokens: 100, CostUSD: 6},
		{Source: "codex", SessionID: "s3", Model: "m", Project: "beta", Timestamp: ts.Add(2 * time.Hour), InputTokens: 10000, CostUSD: 100},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	if err := db.InsertPromptBatch([]*PromptEvent{
		{Source: "codex", SessionID: "s1", Model: "m", Project: "alpha", Timestamp: ts},
		{Source: "codex", SessionID: "s2", Model: "m", Project: "alpha", Timestamp: ts.Add(time.Hour)},
		{Source: "codex", SessionID: "s2", Model: "m", Project: "alpha", Timestamp: ts.Add(time.Hour + time.Minute)},
	}); err != nil {
		t.Fatalf("InsertPromptBatch: %v", err)
	}
	report, err := db.EstimatePreflightCost(ts.Add(-time.Hour), ts.Add(24*time.Hour), "debug", "codex", "m", "alpha", 100)
	if err != nil {
		t.Fatalf("EstimatePreflightCost: %v", err)
	}
	if report.Task != "debug" || report.Factor != 0.75 || report.Confidence != "low" || report.Samples != 2 {
		t.Fatalf("unexpected report metadata: %+v", report)
	}
	if !near(report.Baseline.CostUSD, 4.5, 0.000001) {
		t.Fatalf("baseline cost=%f", report.Baseline.CostUSD)
	}
	if !near(report.Estimate.CostUSD, 3.375, 0.000001) {
		t.Fatalf("estimate cost=%f", report.Estimate.CostUSD)
	}
	if report.Baseline.Tokens != 450 || report.Estimate.Tokens != 338 {
		t.Fatalf("tokens baseline=%d estimate=%d", report.Baseline.Tokens, report.Estimate.Tokens)
	}
	if report.Baseline.Prompts != 2 {
		t.Fatalf("prompts baseline=%d", report.Baseline.Prompts)
	}
}

func TestEstimatePreflightCostEmpty(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	report, err := db.EstimatePreflightCost(ts.Add(-time.Hour), ts.Add(time.Hour), "small", "", "", "", 100)
	if err != nil {
		t.Fatalf("EstimatePreflightCost empty: %v", err)
	}
	if report.Confidence != "none" || report.Samples != 0 || len(report.Issues) == 0 {
		t.Fatalf("expected explicit empty estimate issue: %+v", report)
	}
	if report.Task != "small-change" || report.Factor != 0.35 {
		t.Fatalf("task normalization failed: %+v", report)
	}
}

func TestEstimatePreflightCostUsesCanonicalUsageProjection(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	payload, _ := json.Marshal(map[string]interface{}{
		"goal":          "canonical preflight",
		"call_id":       "call-preflight",
		"input_tokens":  100,
		"output_tokens": 50,
		"cost_usd":      1.5,
		"project":       "alpha",
	})
	_, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:   "evt-preflight-call",
		Source:    "gateway",
		EventType: "model.call",
		SessionID: "canon-preflight",
		Model:     "gpt-test",
		Project:   "alpha",
		Timestamp: ts,
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("IngestCanonicalEvent: %v", err)
	}
	report, err := db.EstimatePreflightCost(ts.Add(-time.Hour), ts.Add(time.Hour), "custom", "gateway", "gpt-test", "alpha", 100)
	if err != nil {
		t.Fatalf("EstimatePreflightCost canonical: %v", err)
	}
	if report.Method != "local-usage-session-median-with-task-multiplier" || report.Samples != 1 {
		t.Fatalf("unexpected projection method: %+v", report)
	}
	if report.Estimate.Tokens != 150 || !near(report.Estimate.CostUSD, 1.5, 0.000001) {
		t.Fatalf("unexpected projection estimate: %+v", report)
	}
}
