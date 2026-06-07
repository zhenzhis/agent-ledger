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

func TestRecalcCostsDetailedUpdatesCanonicalModelCalls(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	start, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:   "evt-priced-workload",
		Source:    "gateway",
		EventType: "workload.started",
		Timestamp: ts,
		Payload:   rawJSON(t, map[string]interface{}{"goal": "price canonical call", "project": "agent-ledger"}),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:    "evt-priced-call",
		Source:     "gateway",
		EventType:  "model.call",
		WorkloadID: start.WorkloadID,
		SessionID:  "sess-priced-call",
		Model:      "gpt-5",
		Timestamp:  ts.Add(time.Minute),
		Payload: rawJSON(t, map[string]interface{}{
			"call_id":                     "call-priced",
			"input_tokens":                100,
			"cache_read_input_tokens":     50,
			"cache_creation_input_tokens": 20,
			"output_tokens":               10,
			"pricing_confidence":          "needs-local-pricing",
		}),
	}); err != nil {
		t.Fatal(err)
	}
	prices := map[string]PricingAuditRow{
		"gpt-5": {
			Model: "gpt-5", PricingSource: "openai-official", MatchedModel: "gpt-5", MatchType: "official-seed", Priority: 20,
			InputCostPerToken: 1, OutputCostPerToken: 2, CacheReadCostPerToken: 0.5, CacheWriteCostPerToken: 0.25, Confidence: "official",
		},
	}
	if err := db.RecalcCostsDetailed(prices, func(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64 {
		return float64(inputTokens)*prices[0] + float64(outputTokens)*prices[1] + float64(cacheCreation)*prices[3] + float64(cacheRead)*prices[2]
	}, "zero", false); err != nil {
		t.Fatal(err)
	}
	wantCost := 150.0
	usage, err := db.GetModelCalls(ts.Add(-time.Hour), ts.Add(time.Hour), "gateway", "gpt-5", "agent-ledger", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(usage) != 1 || usage[0].CostUSD != wantCost {
		t.Fatalf("usage projection cost not recalculated: %+v", usage)
	}
	detail, err := db.GetWorkloadDetail(start.WorkloadID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.ModelCalls) != 1 || detail.ModelCalls[0].CostUSD != wantCost || detail.ModelCalls[0].PricingConfidence != "official" {
		t.Fatalf("model call cost not recalculated: %+v", detail.ModelCalls)
	}
}

func TestProjectionQualityDetectsMissingAndCostMismatch(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		eventID   string
		sessionID string
		cost      float64
	}{
		{"evt-projection-ok", "sess-ok", 1},
		{"evt-projection-missing", "sess-missing", 2},
		{"evt-projection-mismatch", "sess-mismatch", 3},
	} {
		if _, err := db.IngestCanonicalEvent(CanonicalEvent{
			EventID:   item.eventID,
			Source:    "gateway",
			EventType: "model.call",
			SessionID: item.sessionID,
			Model:     "gpt-5",
			Project:   "agent-ledger",
			Timestamp: ts,
			Payload: rawJSON(t, map[string]interface{}{
				"goal":          "projection quality",
				"call_id":       item.eventID + "-call",
				"input_tokens":  10,
				"output_tokens": 5,
				"cost_usd":      item.cost,
			}),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.db.Exec(`DELETE FROM usage_records WHERE source='gateway' AND session_id='sess-missing'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE usage_records SET cost_usd=9 WHERE source='gateway' AND session_id='sess-mismatch'`); err != nil {
		t.Fatal(err)
	}
	q, err := db.GetProjectionQuality(ts.Add(-time.Hour), ts.Add(time.Hour), "gateway", "", "agent-ledger")
	if err != nil {
		t.Fatal(err)
	}
	if q.ModelCalls != 3 || q.ProjectedUsageRecords != 2 || q.MissingUsageProjection != 1 || q.CostMismatchRecords != 1 {
		t.Fatalf("unexpected projection quality: %+v", q)
	}
	if q.Confidence >= 1 || q.Message == "" {
		t.Fatalf("expected degraded projection quality: %+v", q)
	}
}

func TestApprovalRequestLifecycle(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := db.CreateApprovalRequest(ApprovalRequest{
		Action: "export", Target: "sessions", Source: "codex", Model: "gpt-5", Project: "quant", ActorRole: "operator", Reason: "requires review",
	})
	if err != nil {
		t.Fatal(err)
	}
	rows, err := db.ListApprovalRequests("pending", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RequestID != id || rows[0].Status != "pending" {
		t.Fatalf("unexpected pending approvals: %+v", rows)
	}
	allowed, err := db.ApprovalAllows(id, "export", "sessions")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("pending approval should not allow operation")
	}
	if err := db.ResolveApprovalRequest(id, "approved", "admin", "ok"); err != nil {
		t.Fatal(err)
	}
	allowed, err = db.ApprovalAllows(id, "export", "sessions")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("approved request should allow matching operation")
	}
	wrongTarget, err := db.ApprovalAllows(id, "export", "audit")
	if err != nil {
		t.Fatal(err)
	}
	if wrongTarget {
		t.Fatal("approved request should not allow a different target")
	}
}

func TestDashboardStatsUseRawForNonUTCDayAlignedRange(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := db.InsertUsage(&UsageRecord{
		Source:       "codex",
		SessionID:    "previous-day",
		Model:        "gpt-5",
		InputTokens:  10,
		OutputTokens: 5,
		CostUSD:      1,
		Timestamp:    time.Date(2026, 6, 5, 21, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source:       "codex",
		SessionID:    "local-day",
		Model:        "gpt-5",
		InputTokens:  100,
		OutputTokens: 50,
		CostUSD:      2,
		Timestamp:    time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RebuildUsageAggregates(); err != nil {
		t.Fatal(err)
	}

	stats, err := db.GetDashboardStatsFiltered(
		time.Date(2026, 6, 5, 22, 0, 0, 0, time.UTC),
		time.Date(2026, 6, 6, 22, 0, 0, 0, time.UTC),
		"", "", "",
	)
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalTokens != 150 || stats.TotalCost != 2 || stats.TotalCalls != 1 {
		t.Fatalf("expected raw local-day stats, got %+v", stats)
	}
}

func TestDashboardBundleCoreModulesAreConsistent(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 6, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*UsageRecord{
		{Source: "codex", SessionID: "s1", Model: "gpt-5", InputTokens: 100, OutputTokens: 50, CacheReadInputTokens: 10, CacheCreationInputTokens: 20, CostUSD: 1.25, Timestamp: ts, Project: "p"},
		{Source: "codex", SessionID: "s2", Model: "gpt-5-mini", InputTokens: 200, OutputTokens: 60, CostUSD: 2.75, Timestamp: ts.Add(30 * time.Minute), Project: "p"},
	}); err != nil {
		t.Fatal(err)
	}
	bundle, err := db.GetDashboardBundleFiltered(ts.Add(-time.Hour), ts.Add(time.Hour), "1h", "codex", "", "p", 0)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Stats.TotalTokens != 440 || !near(bundle.Stats.TotalCost, 4, 0.000001) {
		t.Fatalf("unexpected bundle stats: %+v", bundle.Stats)
	}
	if len(bundle.Consistency) != 0 {
		t.Fatalf("expected consistent dashboard bundle, got %+v", bundle.Consistency)
	}
}

func TestDashboardBundleIgnoresStaleDailyAggregate(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 6, 10, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source: "codex", SessionID: "s1", Model: "gpt-5", InputTokens: 100, CostUSD: 1, Timestamp: ts,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.RebuildUsageAggregates(); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertUsage(&UsageRecord{
		Source: "codex", SessionID: "s2", Model: "gpt-5", InputTokens: 200, CostUSD: 2, Timestamp: ts.Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
	stats, err := db.GetDashboardStatsFiltered(time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC), "", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if stats.TotalTokens != 100 {
		t.Fatalf("expected compatibility stats endpoint to use stale aggregate in this setup, got %+v", stats)
	}
	bundle, err := db.GetDashboardBundleFiltered(time.Date(2026, 6, 6, 0, 0, 0, 0, time.UTC), time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC), "1h", "", "", "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Stats.TotalTokens != 300 || !near(bundle.Stats.TotalCost, 3, 0.000001) {
		t.Fatalf("expected raw authoritative bundle stats, got %+v", bundle.Stats)
	}
	if len(bundle.Consistency) != 0 {
		t.Fatalf("expected bundle modules to stay internally consistent, got %+v", bundle.Consistency)
	}
}

func TestInsertReconciliationImportDetailed(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	row := ReconciliationImport{
		Provider: "openai", Format: "csv", Currency: "USD", LocalCostUSD: 5, ProviderCostUSD: 8,
		RowsSeen: 2, PayloadSHA256: "abc123", WindowStart: "2026-06-06T00:00:00Z",
		WindowEnd: "2026-06-07T00:00:00Z", Warnings: `["sample warning"]`,
	}
	if err := db.InsertReconciliationImportDetailed(row); err != nil {
		t.Fatal(err)
	}
	rows, err := db.GetReconciliationImports(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d", len(rows))
	}
	got := rows[0]
	if got.Status != "mismatch" || got.PayloadSHA256 != "abc123" || got.WindowStart == "" || got.Warnings == "" {
		t.Fatalf("unexpected reconciliation row: %+v", got)
	}
}
