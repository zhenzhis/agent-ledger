package storage

import (
	"path/filepath"
	"testing"
	"time"
)

func TestRecalcCostsDetailedAnnotatesPricing(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Now().UTC()
	if err := db.InsertUsage(&UsageRecord{
		Source: "codex", SessionID: "s1", Model: "gpt-5", InputTokens: 1000, OutputTokens: 500, Timestamp: ts,
	}); err != nil {
		t.Fatal(err)
	}
	prices := map[string]PricingAuditRow{
		"gpt-5": {
			Model: "gpt-5", PricingSource: "openai-official", MatchedModel: "gpt-5", MatchType: "official-seed", Priority: 20,
			InputCostPerToken: 1, OutputCostPerToken: 2, Confidence: "official",
		},
	}
	if err := db.RecalcCostsDetailed(prices, func(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64 {
		return float64(inputTokens)*prices[0] + float64(outputTokens)*prices[1]
	}, "zero", false); err != nil {
		t.Fatal(err)
	}
	rows, err := db.GetCostIntelligence(ts.Add(-time.Second), ts.Add(time.Second), "codex", "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].CostUSD != 2000 {
		t.Fatalf("unexpected cost intelligence: %+v", rows)
	}
	quality, err := db.GetDataQuality(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if quality.ConfidenceMix["official"] != 1 {
		t.Fatalf("expected official confidence mix, got %+v", quality.ConfidenceMix)
	}
}

func TestRebuildUsageAggregates(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 6, 12, 30, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "claude", SessionID: "s1", Model: "claude-sonnet-4.6", InputTokens: 100, OutputTokens: 50, CostUSD: 1.5, Timestamp: ts, Project: "p"},
		{Source: "claude", SessionID: "s1", Model: "claude-sonnet-4.6", InputTokens: 200, OutputTokens: 60, CostUSD: 2.5, Timestamp: ts.Add(time.Minute), Project: "p"},
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RebuildUsageAggregates(); err != nil {
		t.Fatal(err)
	}
	rows, err := db.GetModelCalls(ts.Add(-time.Hour), ts.Add(time.Hour), "claude", "", "p", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Calls != 2 || rows[0].Tokens != 410 {
		t.Fatalf("unexpected model calls: %+v", rows)
	}
}
