package pricing

import (
	"errors"
	"math"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestCalcCost_Basic(t *testing.T) {
	// prices: [input, output, cache_read, cache_creation]
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}

	cost := CalcCost(1000, 500, 0, 0, prices)
	// 1000 * 0.003 + 500 * 0.015 = 3.0 + 7.5 = 10.5
	if cost != 10.5 {
		t.Errorf("expected 10.5, got %f", cost)
	}
}

func TestCalcCost_WithCache(t *testing.T) {
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}

	// input=500 (non-cached), output=500, cacheCreation=200, cacheRead=300
	// cost = 500*0.003 + 200*0.004 + 300*0.001 + 500*0.015
	//      = 1.5 + 0.8 + 0.3 + 7.5 = 10.1
	cost := CalcCost(500, 500, 200, 300, prices)
	expected := 10.1
	if diff := cost - expected; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

func TestCalcCost_ZeroNonCachedInput(t *testing.T) {
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}

	// All input is cached, non-cached input = 0
	cost := CalcCost(0, 500, 200, 300, prices)
	// cost = 0 + 200*0.004 + 300*0.001 + 500*0.015 = 0.8 + 0.3 + 7.5 = 8.6
	expected := 8.6
	if diff := cost - expected; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("expected %f, got %f", expected, cost)
	}
}

func TestCalcCost_ZeroTokens(t *testing.T) {
	prices := [4]float64{0.003, 0.015, 0.001, 0.004}
	cost := CalcCost(0, 0, 0, 0, prices)
	if cost != 0 {
		t.Errorf("expected 0, got %f", cost)
	}
}

func TestCalcCost_ZeroPrices(t *testing.T) {
	prices := [4]float64{0, 0, 0, 0}
	cost := CalcCost(1000, 500, 200, 300, prices)
	if cost != 0 {
		t.Errorf("expected 0, got %f", cost)
	}
}

func TestOfficialSeedsCoverCurrentPrimaryModelsAndAliases(t *testing.T) {
	rows := officialPriceRows()
	byModel := map[string]storage.PricingAuditRow{}
	for _, row := range rows {
		if row.PricingSource == "" || row.MatchedModel != row.Model || row.MatchType != "official-seed" || row.Priority != 20 || row.Confidence != "official" {
			t.Fatalf("official seed row missing governance metadata: %+v", row)
		}
		if _, exists := byModel[row.Model]; exists {
			t.Fatalf("duplicate official seed row for %q", row.Model)
		}
		byModel[row.Model] = row
	}
	for _, tc := range []struct {
		model      string
		source     string
		inputPerM  float64
		outputPerM float64
		readPerM   float64
		writePerM  float64
	}{
		{model: "gpt-5.4", source: "openai-official", inputPerM: 2, outputPerM: 15, readPerM: 0.20},
		{model: "gpt-5-4", source: "openai-official", inputPerM: 2, outputPerM: 15, readPerM: 0.20},
		{model: "gpt-5.4-mini", source: "openai-official", inputPerM: 0.40, outputPerM: 2, readPerM: 0.04},
		{model: "gpt-5.5", source: "openai-official", inputPerM: 5, outputPerM: 30, readPerM: 0.50},
		{model: "claude-fable-5", source: "anthropic-official", inputPerM: 10, outputPerM: 50, readPerM: 1, writePerM: 12.5},
		{model: "claude-mythos-5", source: "anthropic-official", inputPerM: 10, outputPerM: 50, readPerM: 1, writePerM: 12.5},
		{model: "claude-opus-4.7", source: "anthropic-official", inputPerM: 5, outputPerM: 25, readPerM: 0.50, writePerM: 6.25},
		{model: "claude-opus-4-7", source: "anthropic-official", inputPerM: 5, outputPerM: 25, readPerM: 0.50, writePerM: 6.25},
		{model: "claude-sonnet-4-20250514", source: "anthropic-official", inputPerM: 3, outputPerM: 15, readPerM: 0.30, writePerM: 3.75},
		{model: "claude-haiku-3-5", source: "anthropic-official", inputPerM: 0.80, outputPerM: 4, readPerM: 0.08, writePerM: 1},
	} {
		row, ok := byModel[tc.model]
		if !ok {
			t.Fatalf("official seed missing %q", tc.model)
		}
		if row.PricingSource != tc.source {
			t.Fatalf("official seed %q source mismatch: %+v", tc.model, row)
		}
		assertPerMillionPrice(t, tc.model, "input", row.InputCostPerToken, tc.inputPerM)
		assertPerMillionPrice(t, tc.model, "output", row.OutputCostPerToken, tc.outputPerM)
		assertPerMillionPrice(t, tc.model, "cache read", row.CacheReadCostPerToken, tc.readPerM)
		assertPerMillionPrice(t, tc.model, "cache write", row.CacheWriteCostPerToken, tc.writePerM)
	}
}

func assertPerMillionPrice(t *testing.T, model, field string, gotPerToken, wantPerMillion float64) {
	t.Helper()
	want := wantPerMillion / 1_000_000
	if math.Abs(gotPerToken-want) > 1e-12 {
		t.Fatalf("%s %s price mismatch: got %.12f want %.12f", model, field, gotPerToken, want)
	}
}

func TestSyncWithConfigAppliesOfficialAndOverrideWhenLiteLLMFails(t *testing.T) {
	oldFetch := fetchPricingBytes
	fetchPricingBytes = func(string) ([]byte, string, error) {
		return nil, "", errors.New("offline fallback")
	}
	defer func() { fetchPricingBytes = oldFetch }()

	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := SyncWithConfig(db, config.PricingConfig{Overrides: []config.PriceRule{{
		Model: "gpt-5", Source: "contract", InputCostPerToken: 0.000001, OutputCostPerToken: 0.000002,
	}}}); err != nil {
		t.Fatalf("SyncWithConfig should preserve official/local rules on fallback failure: %v", err)
	}
	rows, err := db.GetPricingAudit(100)
	if err != nil {
		t.Fatal(err)
	}
	var gpt5 storage.PricingAuditRow
	for _, row := range rows {
		if row.Model == "gpt-5" {
			gpt5 = row
			break
		}
	}
	if gpt5.PricingSource != "contract" || gpt5.Priority != 1 || gpt5.Confidence != "override" {
		t.Fatalf("local override did not win over official/fallback rows: %+v", gpt5)
	}
	summary, err := db.GetPricingRuleSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.OverrideRules < 1 || summary.OfficialRules < 1 || summary.TotalRules < summary.OverrideRules+summary.OfficialRules {
		t.Fatalf("unexpected pricing summary: %+v", summary)
	}
	sources, err := db.GetPricingSources(24 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	status := map[string]string{}
	byName := map[string]storage.PricingSourceStatus{}
	for _, source := range sources {
		status[source.Name] = source.Status
		byName[source.Name] = source
	}
	if status["litellm"] != "error" || status["openai-official"] != "seeded" || status["anthropic-official"] != "seeded" {
		t.Fatalf("unexpected source statuses: %+v", sources)
	}
	for _, name := range []string{"openai-official", "anthropic-official"} {
		source := byName[name]
		if source.FreshnessKind != "seeded" || source.Stale || source.LastFetchAt != "" {
			t.Fatalf("official seed source has misleading freshness metadata: %+v", source)
		}
		if !strings.Contains(source.FreshnessNote, "not a live fetch") {
			t.Fatalf("official seed source missing provenance note: %+v", source)
		}
	}
	if status["contract"] != "configured" || byName["contract"].Kind != "override" || byName["contract"].Priority != 1 || byName["contract"].ModelCount != 1 {
		t.Fatalf("local override source was not recorded: %+v", sources)
	}
	if byName["contract"].FreshnessKind != "configured" || byName["contract"].Stale {
		t.Fatalf("local override source has misleading freshness metadata: %+v", byName["contract"])
	}
	for _, name := range []string{"openai-official", "anthropic-official", "contract"} {
		if !strings.HasPrefix(byName[name].SHA256, "sha256:") {
			t.Fatalf("pricing source %s missing stable sha256 hash: %+v", name, byName[name])
		}
	}
}
