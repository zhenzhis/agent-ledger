package storage

import (
	"math"
	"testing"
	"time"
)

func TestSimulateModelRouting(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	if err := db.UpsertPricingDetailed(PricingAuditRow{
		Model: "expensive", PricingSource: "test", MatchedModel: "expensive", MatchType: "direct", Priority: 1,
		InputCostPerToken: 0.01, OutputCostPerToken: 0.02, CacheReadCostPerToken: 0.001, CacheWriteCostPerToken: 0.005, Confidence: "override",
	}); err != nil {
		t.Fatalf("UpsertPricingDetailed expensive: %v", err)
	}
	if err := db.UpsertPricingDetailed(PricingAuditRow{
		Model: "cheap", PricingSource: "test", MatchedModel: "cheap", MatchType: "direct", Priority: 1,
		InputCostPerToken: 0.001, OutputCostPerToken: 0.002, CacheReadCostPerToken: 0.0001, CacheWriteCostPerToken: 0.0005, Confidence: "override",
	}); err != nil {
		t.Fatalf("UpsertPricingDetailed cheap: %v", err)
	}
	if err := db.InsertUsageBatch([]*UsageRecord{
		{
			Source: "codex", SessionID: "s1", Model: "expensive", Project: "alpha", Timestamp: ts,
			InputTokens: 1000, OutputTokens: 500, CacheCreationInputTokens: 100, CacheReadInputTokens: 200,
			CostUSD: 20.7, PricingConfidence: "override",
		},
		{
			Source: "codex", SessionID: "s2", Model: "other", Project: "alpha", Timestamp: ts,
			InputTokens: 1000, OutputTokens: 500, CostUSD: 99, PricingConfidence: "source-reported",
		},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	report, err := db.SimulateModelRouting(ts.Add(-time.Hour), ts.Add(time.Hour), "codex", "expensive", "cheap", "alpha", 1, 20)
	if err != nil {
		t.Fatalf("SimulateModelRouting: %v", err)
	}
	if report.Status != "ok" {
		t.Fatalf("status=%s issues=%v", report.Status, report.Issues)
	}
	if report.Summary.Groups != 1 || len(report.Rows) != 1 {
		t.Fatalf("expected one simulated group, got summary=%+v rows=%d", report.Summary, len(report.Rows))
	}
	row := report.Rows[0]
	if row.TargetPricingSource != "test" || row.TargetConfidence != "override" {
		t.Fatalf("target pricing metadata not preserved: %+v", row)
	}
	if !near(row.CurrentCostUSD, 20.7, 0.000001) {
		t.Fatalf("current cost=%f", row.CurrentCostUSD)
	}
	if !near(row.SimulatedCostUSD, 2.07, 0.000001) {
		t.Fatalf("simulated cost=%f", row.SimulatedCostUSD)
	}
	if !near(row.DeltaUSD, -18.63, 0.000001) {
		t.Fatalf("delta=%f", row.DeltaUSD)
	}

	partial, err := db.SimulateModelRouting(ts.Add(-time.Hour), ts.Add(time.Hour), "codex", "expensive", "cheap", "alpha", 0.5, 20)
	if err != nil {
		t.Fatalf("partial SimulateModelRouting: %v", err)
	}
	if !near(partial.Summary.SimulatedCostUSD, 11.385, 0.000001) {
		t.Fatalf("partial simulated=%f", partial.Summary.SimulatedCostUSD)
	}
	if !near(partial.Summary.DeltaUSD, -9.315, 0.000001) {
		t.Fatalf("partial delta=%f", partial.Summary.DeltaUSD)
	}
}

func TestSimulateModelRoutingRequiresTargetPricing(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{Source: "codex", SessionID: "s1", Model: "expensive", Timestamp: ts, CostUSD: 1}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	_, err := db.SimulateModelRouting(ts.Add(-time.Hour), ts.Add(time.Hour), "", "", "missing", "", 1, 20)
	if err == nil {
		t.Fatalf("expected missing target pricing error")
	}
}

func near(got, want, tolerance float64) bool {
	return math.Abs(got-want) <= tolerance
}
