package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func hasString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestPricingSourceFreshnessDerivesSeededFetchedAndConfigured(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	old := time.Now().UTC().Add(-48 * time.Hour).Format(time.RFC3339)
	if err := db.UpsertPricingSource(PricingSourceStatus{
		Name: "openai-official", Kind: "official", Priority: 20, URL: "https://openai.com/api/pricing/",
		LastFetchAt: old, Status: "seeded", SHA256: "sha256:seed", ModelCount: 1,
	}); err != nil {
		t.Fatalf("UpsertPricingSource seeded: %v", err)
	}
	if err := db.UpsertPricingSource(PricingSourceStatus{
		Name: "litellm", Kind: "fallback", Priority: 100, URL: "https://example.test/prices.json",
		LastFetchAt: old, Status: "ok", SHA256: "sha256:fetched", ModelCount: 1,
	}); err != nil {
		t.Fatalf("UpsertPricingSource fetched: %v", err)
	}
	if err := db.UpsertPricingSource(PricingSourceStatus{
		Name: "contract", Kind: "override", Priority: 1, URL: "local-config",
		Status: "configured", SHA256: "sha256:contract", ModelCount: 1,
	}); err != nil {
		t.Fatalf("UpsertPricingSource override: %v", err)
	}
	if err := db.UpsertPricingSource(PricingSourceStatus{
		Name: "mystery", Kind: "custom", Priority: 200, Status: "ok",
	}); err != nil {
		t.Fatalf("UpsertPricingSource unknown: %v", err)
	}

	sources, err := db.GetPricingSources(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]PricingSourceStatus{}
	for _, source := range sources {
		byName[source.Name] = source
	}
	if byName["openai-official"].FreshnessKind != "seeded" || byName["openai-official"].Stale {
		t.Fatalf("seeded official source should not be time-stale: %+v", byName["openai-official"])
	}
	if !strings.Contains(byName["openai-official"].FreshnessNote, "not a live fetch") {
		t.Fatalf("seeded official source missing explicit note: %+v", byName["openai-official"])
	}
	if byName["litellm"].FreshnessKind != "fetched" || !byName["litellm"].Stale {
		t.Fatalf("old fetched remote source should be stale: %+v", byName["litellm"])
	}
	if byName["contract"].FreshnessKind != "configured" || byName["contract"].Stale {
		t.Fatalf("configured override should not be time-stale: %+v", byName["contract"])
	}
	if byName["mystery"].FreshnessKind != "fetched" || !byName["mystery"].Stale {
		t.Fatalf("status-ok source without fetch metadata should be stale: %+v", byName["mystery"])
	}
}

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
	if rows[0].InputTokens != 1000 || rows[0].OutputTokens != 500 || rows[0].OfficialPricedCalls != 1 {
		t.Fatalf("cost intelligence did not expose token/pricing breakdown: %+v", rows[0])
	}
	if !hasString(rows[0].PricingSources, "openai-official") || !hasString(rows[0].PricingConfidences, "official") {
		t.Fatalf("cost intelligence did not expose pricing provenance: %+v", rows[0])
	}
	quality, err := db.GetDataQuality(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if quality.ConfidenceMix["official"] != 1 {
		t.Fatalf("expected official confidence mix, got %+v", quality.ConfidenceMix)
	}
}

func TestRecalcCostsDetailedPreservesSourceReportedAndMarksUnpriced(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*UsageRecord{
		{
			Source: "gateway", SessionID: "source-cost", Model: "gpt-5",
			InputTokens: 1000, OutputTokens: 500, CostUSD: 12.34, Timestamp: ts,
			PricingSource: "gateway", PricingModel: "gpt-5", PricingConfidence: "source-reported",
		},
		{
			Source: "gateway", SessionID: "missing-price", Model: "unknown-frontier-model",
			InputTokens: 1000, OutputTokens: 500, CostUSD: 0, Timestamp: ts.Add(time.Minute),
		},
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
		return 999
	}, "all", false); err != nil {
		t.Fatal(err)
	}
	var sourceCost float64
	var sourcePricing, sourceConfidence string
	if err := db.db.QueryRow(`SELECT cost_usd,pricing_source,pricing_confidence FROM usage_records WHERE session_id='source-cost'`).
		Scan(&sourceCost, &sourcePricing, &sourceConfidence); err != nil {
		t.Fatal(err)
	}
	if sourceCost != 12.34 || sourcePricing != "gateway" || sourceConfidence != "source-reported" {
		t.Fatalf("source-reported cost was overwritten: cost=%f source=%q confidence=%q", sourceCost, sourcePricing, sourceConfidence)
	}
	var missingConfidence, missingNote string
	if err := db.db.QueryRow(`SELECT pricing_confidence,pricing_note FROM usage_records WHERE session_id='missing-price'`).
		Scan(&missingConfidence, &missingNote); err != nil {
		t.Fatal(err)
	}
	if missingConfidence != "unpriced" || !strings.Contains(missingNote, "no pricing rule matched") {
		t.Fatalf("unpriced row not annotated: confidence=%q note=%q", missingConfidence, missingNote)
	}
	quality, err := db.GetDataQuality(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	unpricedRecords := 0
	for _, source := range quality.SourceQuality {
		unpricedRecords += source.UnpricedRecords
	}
	if unpricedRecords != 1 || len(quality.UnpricedModels) != 1 || quality.UnpricedModels[0].Model != "unknown-frontier-model" {
		t.Fatalf("data quality did not expose unpriced model: %+v", quality)
	}
}

func TestRecalcCostsDetailedSkipsEstimatedAggregate(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source: "codex", SessionID: "sqlite-thread", Model: "gpt-5-codex",
		InputTokens: 900, Timestamp: ts, PricingConfidence: "estimated-aggregate",
		PricingNote: "codex sqlite thread tokens_used aggregate",
	}); err != nil {
		t.Fatal(err)
	}
	prices := map[string]PricingAuditRow{
		"gpt-5-codex": {
			Model: "gpt-5-codex", PricingSource: "openai-official", MatchedModel: "gpt-5-codex", MatchType: "official-seed", Priority: 20,
			InputCostPerToken: 1, OutputCostPerToken: 2, Confidence: "official",
		},
	}
	if err := db.RecalcCostsDetailed(prices, func(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64 {
		return 999
	}, "all", false); err != nil {
		t.Fatal(err)
	}
	var cost float64
	var confidence, note string
	if err := db.db.QueryRow(`SELECT cost_usd,pricing_confidence,pricing_note FROM usage_records WHERE session_id='sqlite-thread'`).
		Scan(&cost, &confidence, &note); err != nil {
		t.Fatal(err)
	}
	if cost != 0 || confidence != "estimated-aggregate" || !strings.Contains(note, "thread tokens_used aggregate") {
		t.Fatalf("estimated aggregate row was repriced: cost=%f confidence=%q note=%q", cost, confidence, note)
	}
}

func TestDataQualitySurfacesEstimatedAggregate(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsageBatch([]*UsageRecord{
		{
			Source: "codex", SessionID: "sqlite-thread", Model: "gpt-5-codex",
			InputTokens: 900, Timestamp: ts, PricingConfidence: "estimated-aggregate",
			PricingNote: "codex sqlite thread tokens_used aggregate",
		},
		{
			Source: "codex", SessionID: "exact-thread", Model: "gpt-5-codex",
			InputTokens: 100, OutputTokens: 50, Timestamp: ts.Add(time.Minute), PricingConfidence: "official",
		},
	}); err != nil {
		t.Fatal(err)
	}
	quality, err := db.GetDataQuality(time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if quality.ConfidenceMix["estimated-aggregate"] != 1 {
		t.Fatalf("expected estimated aggregate in confidence mix: %+v", quality.ConfidenceMix)
	}
	var codex QualitySource
	for _, source := range quality.SourceQuality {
		if source.Source == "codex" {
			codex = source
			break
		}
	}
	if codex.EstimatedAggregateRecords != 1 || !strings.Contains(codex.Message, "aggregate token estimates") {
		t.Fatalf("estimated aggregate not surfaced in source quality: %+v", codex)
	}
	if codex.Confidence >= 1 {
		t.Fatalf("expected confidence penalty for estimated aggregate records: %+v", codex)
	}
}

func TestCostIntelligenceExplainsPricingAndTokenDrivers(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.UpsertSession(&SessionRecord{
		Source: "codex", SessionID: "expensive", Project: "quant", GitBranch: "main", StartTime: ts, Prompts: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := db.InsertUsageBatch([]*UsageRecord{
		{
			Source: "codex", SessionID: "expensive", Model: "gpt-5",
			InputTokens: 60000, CacheReadInputTokens: 1000, CacheCreationInputTokens: 20000,
			OutputTokens: 70000, ReasoningOutputTokens: 10000, CostUSD: 15, Timestamp: ts,
			Project: "quant", GitBranch: "main", PricingSource: "litellm", PricingModel: "gpt-5", PricingConfidence: "fallback",
		},
		{
			Source: "codex", SessionID: "expensive", Model: "future-model",
			InputTokens: 1000, OutputTokens: 100, CostUSD: 0, Timestamp: ts.Add(time.Minute),
			Project: "quant", GitBranch: "main", PricingConfidence: "unpriced", PricingNote: "no pricing rule matched",
		},
	}); err != nil {
		t.Fatal(err)
	}
	rows, err := db.GetCostIntelligence(ts.Add(-time.Minute), ts.Add(time.Hour), "codex", "", "", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("expected one insight row, got %+v", rows)
	}
	row := rows[0]
	if row.InputTokens != 61000 || row.CacheReadTokens != 1000 || row.CacheWriteTokens != 20000 || row.OutputTokens != 70100 || row.ReasoningTokens != 10000 {
		t.Fatalf("unexpected token breakdown: %+v", row)
	}
	if row.FallbackPricedCalls != 1 || row.UnpricedCalls != 1 {
		t.Fatalf("unexpected pricing counters: %+v", row)
	}
	if !hasString(row.PricingSources, "litellm") || !hasString(row.PricingSources, "unknown") {
		t.Fatalf("pricing sources missing expected provenance: %+v", row.PricingSources)
	}
	if !hasString(row.PricingConfidences, "fallback") || !hasString(row.PricingConfidences, "unpriced") {
		t.Fatalf("pricing confidences missing expected states: %+v", row.PricingConfidences)
	}
	for _, reason := range []string{
		"unpriced records in session",
		"fallback pricing source used",
		"multiple models used in one session",
		"low cache hit rate with large context",
		"high tokens per prompt",
		"cache writes exceed cache reads",
		"high output/input ratio",
		"reasoning tokens present",
	} {
		if !hasString(row.Reasons, reason) {
			t.Fatalf("missing reason %q in %+v", reason, row.Reasons)
		}
	}
	if row.CostPerCall != 7.5 || row.CostPerPrompt != 15 || row.TokensPerPrompt != 152100 {
		t.Fatalf("unexpected derived cost metrics: %+v", row)
	}
	if row.QualityScore >= 1 {
		t.Fatalf("expected quality score penalty for low-confidence/high-cost row: %+v", row)
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

func TestTimeRangeQueriesNormalizeLocalBoundsToUTC(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 11, 17, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&UsageRecord{
		Source: "gateway", SessionID: "s-local-window", Model: "gpt-5.5", InputTokens: 100, OutputTokens: 25, CostUSD: 0.25, Timestamp: ts, Project: "agent-ledger",
	}); err != nil {
		t.Fatal(err)
	}

	local := time.FixedZone("UTC+2", 2*60*60)
	from := ts.Add(-time.Minute).In(local)
	to := ts.Add(time.Minute).In(local)
	calls, err := db.GetModelCalls(from, to, "gateway", "gpt-5.5", "agent-ledger", 10)
	if err != nil {
		t.Fatalf("GetModelCalls: %v", err)
	}
	if len(calls) != 1 || calls[0].Calls != 1 || calls[0].Tokens != 125 {
		t.Fatalf("local bounds failed to match UTC usage row: %+v", calls)
	}
	stats, err := db.GetDashboardStatsFiltered(from, to, "gateway", "gpt-5.5", "agent-ledger")
	if err != nil {
		t.Fatalf("GetDashboardStatsFiltered: %v", err)
	}
	if stats.TotalCalls != 1 || stats.TotalTokens != 125 || stats.TotalCost != 0.25 {
		t.Fatalf("local bounds failed to match dashboard stats: %+v", stats)
	}
}

func TestDataQualityIncludesCanonicalEventProvenance(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if _, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:       "evt-provenance-complete",
		Source:        "codex",
		EventType:     "workload.started",
		SchemaVersion: "v1",
		SourceVersion: "codex-cli 1.2.3",
		ParserVersion: "agent-ledger-codex-adapter@v2",
		RawRef:        "sha256:source#row=1",
		MatchType:     "exact",
		Timestamp:     ts,
		Payload:       rawJSON(t, map[string]interface{}{"goal": "complete provenance"}),
	}); err != nil {
		t.Fatalf("complete event: %v", err)
	}
	if _, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:   "evt-provenance-incomplete",
		Source:    "opencode",
		EventType: "workload.started",
		Timestamp: ts.Add(time.Minute),
		Payload:   rawJSON(t, map[string]interface{}{"goal": "missing provenance"}),
	}); err != nil {
		t.Fatalf("incomplete event: %v", err)
	}
	quality, err := db.GetDataQuality(time.Hour)
	if err != nil {
		t.Fatalf("GetDataQuality: %v", err)
	}
	if quality.Provenance == nil || quality.Provenance.Events != 2 {
		t.Fatalf("provenance missing: %#v", quality.Provenance)
	}
	if quality.Provenance.MissingParserVersion != 1 || quality.Provenance.MissingRawRef != 1 || quality.Provenance.MatchTypeMix["unknown"] != 1 || quality.Provenance.MatchTypeMix["exact"] != 1 {
		t.Fatalf("unexpected provenance quality: %#v", quality.Provenance)
	}
	if quality.Provenance.Confidence >= 1 {
		t.Fatalf("expected confidence penalty for incomplete provenance: %#v", quality.Provenance)
	}

	report, err := db.GetDoctorReport(ts.Add(-time.Hour), ts.Add(time.Hour), time.Hour, "", "", "")
	if err != nil {
		t.Fatalf("GetDoctorReport: %v", err)
	}
	found := false
	for _, check := range report.Checks {
		if check.Name == "provenance.incomplete" {
			found = true
		}
	}
	if !found {
		t.Fatalf("doctor provenance warning missing: %+v", report.Checks)
	}
	if !strings.Contains(FormatDoctorMarkdown(report), "Provenance") {
		t.Fatalf("doctor markdown omitted provenance")
	}
}

func TestDetectWatchdogEventsUsesObservedTimeAndUpserts(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	var records []*UsageRecord
	for i := 0; i < 8; i++ {
		records = append(records, &UsageRecord{
			Source: "codex", SessionID: "baseline-" + string(rune('a'+i)), Model: "gpt-5",
			InputTokens: 1000, OutputTokens: 500, CostUSD: 0.01, Timestamp: ts.Add(time.Duration(-i-1) * time.Minute), Project: "private-project",
		})
	}
	for i := 0; i < 12; i++ {
		records = append(records, &UsageRecord{
			Source: "codex", SessionID: "runaway-session", Model: "gpt-5",
			InputTokens: 20000, OutputTokens: 100, CostUSD: 0.5, Timestamp: ts.Add(time.Duration(i) * time.Minute), Project: "private-project",
		})
	}
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	if err := db.InsertPromptBatch([]*PromptEvent{{Source: "codex", SessionID: "runaway-session", Model: "gpt-5", Project: "private-project", Timestamp: ts}}); err != nil {
		t.Fatalf("InsertPromptBatch: %v", err)
	}
	from, to := ts.Add(-time.Hour), ts.Add(time.Hour)
	if err := db.DetectWatchdogEvents(from, to, "codex", "", "private-project", 4, 8, 22, 6); err != nil {
		t.Fatalf("DetectWatchdogEvents: %v", err)
	}
	rows, err := db.GetInsightEventsFiltered(InsightEventFilter{Kind: "watchdog", Source: "codex", Project: "private-project", From: from, To: to, Limit: 100})
	if err != nil {
		t.Fatalf("GetInsightEventsFiltered: %v", err)
	}
	metrics := map[string]bool{}
	for _, row := range rows {
		if row.SessionID == "runaway-session" {
			metrics[row.Metric] = true
			if !strings.HasPrefix(row.CreatedAt, "2026-06-07") {
				t.Fatalf("watchdog event did not use observed time: %+v", row)
			}
		}
	}
	for _, metric := range []string{"call_density", "calls_per_prompt", "low_output_ratio", "cache_miss_risk"} {
		if !metrics[metric] {
			t.Fatalf("missing watchdog metric %s in %+v", metric, rows)
		}
	}
	if err := db.DetectWatchdogEvents(from, to, "codex", "", "private-project", 4, 8, 22, 6); err != nil {
		t.Fatalf("DetectWatchdogEvents duplicate: %v", err)
	}
	rowsAgain, err := db.GetInsightEventsFiltered(InsightEventFilter{Kind: "watchdog", Source: "codex", Project: "private-project", From: from, To: to, Limit: 100})
	if err != nil {
		t.Fatalf("GetInsightEventsFiltered duplicate: %v", err)
	}
	if len(rowsAgain) != len(rows) {
		t.Fatalf("watchdog upsert grew duplicate rows: before=%d after=%d rows=%+v", len(rows), len(rowsAgain), rowsAgain)
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

func TestRepairUsageProjectionsBackfillsAndRealigns(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 13, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		eventID   string
		sessionID string
		callID    string
		cost      float64
		cacheRead int64
	}{
		{"evt-repair-missing", "sess-repair-missing", "call-repair-missing", 2.5, 11},
		{"evt-repair-loose", "sess-repair-loose", "call-repair-loose", 3.5, 17},
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
				"goal":                    "repair projection",
				"call_id":                 item.callID,
				"input_tokens":            100,
				"cache_read_input_tokens": item.cacheRead,
				"output_tokens":           20,
				"cost_usd":                item.cost,
				"pricing_source":          "openai-official",
				"pricing_confidence":      "official",
				"model_alias":             "gpt-5",
			}),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.db.Exec(`DELETE FROM usage_records WHERE source='gateway' AND session_id='sess-repair-missing'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE usage_records SET cache_read_input_tokens=0, cost_usd=0, pricing_source='', pricing_confidence='stale'
		WHERE source='gateway' AND session_id='sess-repair-loose'`); err != nil {
		t.Fatal(err)
	}
	before, err := db.GetProjectionQuality(ts.Add(-time.Hour), ts.Add(time.Hour), "gateway", "gpt-5", "agent-ledger")
	if err != nil {
		t.Fatal(err)
	}
	if before.MissingUsageProjection != 2 {
		t.Fatalf("expected two projection issues before repair, got %+v", before)
	}
	result, err := db.RepairUsageProjections(ts.Add(-time.Hour), ts.Add(time.Hour), "gateway", "gpt-5", "agent-ledger")
	if err != nil {
		t.Fatal(err)
	}
	if result.Inserted != 1 || result.Updated != 1 {
		t.Fatalf("unexpected repair result: %+v", result)
	}
	if result.After.MissingUsageProjection != 0 || result.After.CostMismatchRecords != 0 || result.After.Confidence != 1 {
		t.Fatalf("projection still degraded after repair: %+v", result.After)
	}
	var cacheRead int64
	var cost float64
	var note string
	if err := db.db.QueryRow(`SELECT cache_read_input_tokens,cost_usd,pricing_note FROM usage_records
		WHERE source='gateway' AND session_id='sess-repair-loose'`).Scan(&cacheRead, &cost, &note); err != nil {
		t.Fatal(err)
	}
	if cacheRead != 17 || cost != 3.5 || note != "canonical model.call projection repaired" {
		t.Fatalf("loose projection was not realigned: cache=%d cost=%f note=%q", cacheRead, cost, note)
	}
	rows, err := db.GetModelCalls(ts.Add(-time.Hour), ts.Add(time.Hour), "gateway", "gpt-5", "agent-ledger", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Calls != 2 || rows[0].CostUSD != 6 {
		t.Fatalf("aggregates/model analytics not refreshed: %+v", rows)
	}
	again, err := db.RepairUsageProjections(ts.Add(-time.Hour), ts.Add(time.Hour), "gateway", "gpt-5", "agent-ledger")
	if err != nil {
		t.Fatal(err)
	}
	if again.Inserted != 0 || again.Updated != 0 {
		t.Fatalf("repair should be idempotent, got %+v", again)
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

func TestMultiActorApprovalVotesRequireQuorum(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id, err := db.CreateApprovalRequest(ApprovalRequest{
		Action: "model.call", Target: "gpt-5.5", Source: "gateway", Model: "gpt-5.5", Project: "agent-ledger", ActorRole: "operator", RequiredApprovals: 2, Reason: "high cost model",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := db.CastApprovalVote(id, "approved", "alice", "admin", "looks ok", 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "pending" || first.Decided || first.ApprovalVotes != 1 || first.RequiredApprovals != 2 {
		t.Fatalf("first vote should not decide quorum: %+v", first)
	}
	allowed, err := db.ApprovalAllows(id, "model.call", "gpt-5.5")
	if err != nil {
		t.Fatal(err)
	}
	if allowed {
		t.Fatal("single vote should not authorize a two-actor approval")
	}
	updated, err := db.CastApprovalVote(id, "approved", "alice", "admin", "still ok", 2)
	if err != nil {
		t.Fatal(err)
	}
	if updated.ApprovalVotes != 1 || updated.Status != "pending" {
		t.Fatalf("same voter should update rather than duplicate: %+v", updated)
	}
	second, err := db.CastApprovalVote(id, "approved", "bob", "admin", "approved", 2)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "approved" || !second.Decided || second.ApprovalVotes != 2 {
		t.Fatalf("second vote should approve request: %+v", second)
	}
	allowed, err = db.ApprovalAllows(id, "model.call", "gpt-5.5")
	if err != nil {
		t.Fatal(err)
	}
	if !allowed {
		t.Fatal("approved quorum should authorize matching operation")
	}
	strictAllowed, err := db.ApprovalAllowsOperation(ApprovalOperation{RequestID: id, Action: "model.call", Target: "gpt-5.5", Source: "gateway", Model: "gpt-5.5", Project: "agent-ledger"})
	if err != nil {
		t.Fatal(err)
	}
	if !strictAllowed {
		t.Fatal("strict context should authorize matching operation")
	}
	wrongModel, err := db.ApprovalAllowsOperation(ApprovalOperation{RequestID: id, Action: "model.call", Target: "gpt-5.5", Source: "gateway", Model: "gpt-4.1", Project: "agent-ledger"})
	if err != nil {
		t.Fatal(err)
	}
	if wrongModel {
		t.Fatal("strict context should not allow a different model")
	}
	wrongSource, err := db.ApprovalAllowsOperation(ApprovalOperation{RequestID: id, Action: "model.call", Target: "gpt-5.5", Source: "codex", Model: "gpt-5.5", Project: "agent-ledger"})
	if err != nil {
		t.Fatal(err)
	}
	if wrongSource {
		t.Fatal("strict context should not allow a different source")
	}
	wrongProject, err := db.ApprovalAllowsOperation(ApprovalOperation{RequestID: id, Action: "model.call", Target: "gpt-5.5", Source: "gateway", Model: "gpt-5.5", Project: "other-project"})
	if err != nil {
		t.Fatal(err)
	}
	if wrongProject {
		t.Fatal("strict context should not allow a different project")
	}
	rows, err := db.ListApprovalRequests("approved", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].RequiredApprovals != 2 || rows[0].ApprovalVotes != 2 || !strings.Contains(rows[0].DecidedBy, "alice") || !strings.Contains(rows[0].DecidedBy, "bob") {
		t.Fatalf("approved request missing vote evidence: %+v", rows)
	}
}

func TestAuditLogFiltering(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	base := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.AppendAuditLog("local", "operator", "pricing.sync", "openai", map[string]string{"mode": "manual"}); err != nil {
		t.Fatal(err)
	}
	if err := db.AppendAuditLog("local", "viewer", "export", "sessions", map[string]string{"format": "csv"}); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE audit_log SET created_at=? WHERE action='pricing.sync'`, base); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE audit_log SET created_at=? WHERE action='export'`, base.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	rows, err := db.QueryAuditLog(AuditLogFilter{
		From:   base.Add(-time.Minute),
		To:     base.Add(time.Hour),
		Role:   "operator",
		Action: "pricing",
		Target: "open",
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Action != "pricing.sync" || !strings.Contains(rows[0].Params, "manual") {
		t.Fatalf("unexpected filtered audit rows: %+v", rows)
	}
	recent, err := db.GetAuditLog(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 2 {
		t.Fatalf("legacy GetAuditLog wrapper returned %+v", recent)
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
