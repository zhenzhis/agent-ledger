package pricing

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

const liteLLMPricingURL = "https://raw.githubusercontent.com/BerriAI/litellm/main/model_prices_and_context_window.json"
const openAIPricingURL = "https://openai.com/api/pricing/"
const anthropicPricingURL = "https://platform.claude.com/docs/en/about-claude/pricing"
const maxPricingResponseBytes = 8 * 1024 * 1024

var fetchPricingBytes = fetchBytes

type modelPricing struct {
	InputCostPerToken           *float64 `json:"input_cost_per_token"`
	OutputCostPerToken          *float64 `json:"output_cost_per_token"`
	CacheReadInputTokenCost     *float64 `json:"cache_read_input_token_cost"`
	CacheCreationInputTokenCost *float64 `json:"cache_creation_input_token_cost"`
}

// Sync fetches pricing with default options.
func Sync(db *storage.DB) error {
	return SyncWithConfig(db, config.PricingConfig{})
}

// SyncWithConfig applies local overrides, official seed prices, and LiteLLM fallback.
func SyncWithConfig(db *storage.DB, cfg config.PricingConfig) error {
	if cfg.Mode == "" {
		cfg.Mode = "official-plus-litellm"
	}
	now := time.Now().UTC().Format(time.RFC3339)
	var warnings []string
	if err := syncLiteLLM(db); err != nil {
		_ = db.UpsertPricingSource(storage.PricingSourceStatus{
			Name: "litellm", Kind: "fallback", Priority: 100, URL: liteLLMPricingURL, LastFetchAt: now, Status: "error", LastError: err.Error(),
		})
		_ = db.InsertPricingAuditEvent("sync.warning", "litellm", "", err.Error())
		warnings = append(warnings, "LiteLLM fallback sync failed: "+err.Error())
	}
	if err := applyOfficialSeeds(db); err != nil {
		return err
	}
	for _, override := range cfg.Overrides {
		if strings.TrimSpace(override.Model) == "" {
			continue
		}
		source := override.Source
		if source == "" {
			source = "local-override"
		}
		if err := db.UpsertPricingDetailed(storage.PricingAuditRow{
			Model:                  override.Model,
			PricingSource:          source,
			MatchedModel:           override.Model,
			MatchType:              "override",
			Priority:               1,
			InputCostPerToken:      override.InputCostPerToken,
			OutputCostPerToken:     override.OutputCostPerToken,
			CacheReadCostPerToken:  override.CacheReadCostPerToken,
			CacheWriteCostPerToken: override.CacheWriteCostPerToken,
			EffectiveAt:            override.EffectiveAt,
			Confidence:             "override",
		}); err != nil {
			return err
		}
	}
	if err := recordOverrideSources(db, cfg.Overrides, now); err != nil {
		return err
	}
	message := "pricing sync completed with official overlays and LiteLLM fallback"
	if len(warnings) > 0 {
		message = "pricing sync completed with official/local rules; " + strings.Join(warnings, "; ")
	}
	_ = db.InsertPricingAuditEvent("sync", "pricing", "", message)
	return nil
}

func syncLiteLLM(db *storage.DB) error {
	body, etag, err := fetchPricingBytes(liteLLMPricingURL)
	now := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	sha := "sha256:" + hex.EncodeToString(sum[:])
	var data map[string]json.RawMessage
	if err := json.Unmarshal(body, &data); err != nil {
		return err
	}
	count := 0
	for model, raw := range data {
		var p modelPricing
		if err := json.Unmarshal(raw, &p); err != nil {
			continue
		}
		if p.InputCostPerToken == nil || p.OutputCostPerToken == nil {
			continue
		}
		var cacheRead, cacheCreate float64
		if p.CacheReadInputTokenCost != nil {
			cacheRead = *p.CacheReadInputTokenCost
		}
		if p.CacheCreationInputTokenCost != nil {
			cacheCreate = *p.CacheCreationInputTokenCost
		}
		if err := db.UpsertPricingDetailed(storage.PricingAuditRow{
			Model:                  model,
			PricingSource:          "litellm",
			MatchedModel:           model,
			MatchType:              "direct",
			Priority:               100,
			InputCostPerToken:      *p.InputCostPerToken,
			OutputCostPerToken:     *p.OutputCostPerToken,
			CacheReadCostPerToken:  cacheRead,
			CacheWriteCostPerToken: cacheCreate,
			Confidence:             "fallback",
		}); err != nil {
			log.Printf("pricing: error upserting %s: %v", model, err)
		}
		count++
	}
	if err := db.UpsertPricingSource(storage.PricingSourceStatus{
		Name: "litellm", Kind: "fallback", Priority: 100, URL: liteLLMPricingURL, LastFetchAt: now, ETag: etag, SHA256: sha, ModelCount: count, Status: "ok",
	}); err != nil {
		return err
	}
	_ = db.InsertPricingSnapshot("litellm", sha, count, map[string]string{"url": liteLLMPricingURL})
	log.Printf("pricing: synced %d LiteLLM fallback models", count)
	return nil
}

func applyOfficialSeeds(db *storage.DB) error {
	now := time.Now().UTC().Format(time.RFC3339)
	official := officialPriceRows()
	sourceCounts := map[string]int{}
	sourceRows := map[string][]storage.PricingAuditRow{}
	for _, row := range official {
		if err := db.UpsertPricingDetailed(row); err != nil {
			return err
		}
		sourceCounts[row.PricingSource]++
		sourceRows[row.PricingSource] = append(sourceRows[row.PricingSource], row)
	}
	for _, s := range []storage.PricingSourceStatus{
		{Name: "openai-official", Kind: "official", Priority: 20, URL: openAIPricingURL, SHA256: hashPricingRows(sourceRows["openai-official"]), ModelCount: sourceCounts["openai-official"], Status: "seeded"},
		{Name: "anthropic-official", Kind: "official", Priority: 20, URL: anthropicPricingURL, SHA256: hashPricingRows(sourceRows["anthropic-official"]), ModelCount: sourceCounts["anthropic-official"], Status: "seeded"},
	} {
		if err := db.UpsertPricingSource(s); err != nil {
			return err
		}
		_ = db.InsertPricingSnapshot(s.Name, s.SHA256, s.ModelCount, map[string]string{"url": s.URL, "mode": "official-seed", "applied_at": now})
	}
	return nil
}

func recordOverrideSources(db *storage.DB, overrides []config.PriceRule, now string) error {
	sourceRows := map[string][]storage.PricingAuditRow{}
	for _, override := range overrides {
		if strings.TrimSpace(override.Model) == "" {
			continue
		}
		source := override.Source
		if source == "" {
			source = "local-override"
		}
		sourceRows[source] = append(sourceRows[source], storage.PricingAuditRow{
			Model:                  override.Model,
			PricingSource:          source,
			MatchedModel:           override.Model,
			MatchType:              "override",
			Priority:               1,
			InputCostPerToken:      override.InputCostPerToken,
			OutputCostPerToken:     override.OutputCostPerToken,
			CacheReadCostPerToken:  override.CacheReadCostPerToken,
			CacheWriteCostPerToken: override.CacheWriteCostPerToken,
			EffectiveAt:            override.EffectiveAt,
			Confidence:             "override",
		})
	}
	for source, rows := range sourceRows {
		sha := hashPricingRows(rows)
		if err := db.UpsertPricingSource(storage.PricingSourceStatus{
			Name: source, Kind: "override", Priority: 1, URL: "local-config", LastFetchAt: now, SHA256: sha, ModelCount: len(rows), Status: "configured",
		}); err != nil {
			return err
		}
		_ = db.InsertPricingSnapshot(source, sha, len(rows), map[string]string{"source": source, "mode": "local-override"})
	}
	return nil
}

func hashPricingRows(rows []storage.PricingAuditRow) string {
	raw, err := json.Marshal(rows)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func officialPriceRows() []storage.PricingAuditRow {
	perM := func(input, output, cacheRead, cacheWrite float64) (float64, float64, float64, float64) {
		return input / 1_000_000, output / 1_000_000, cacheRead / 1_000_000, cacheWrite / 1_000_000
	}
	row := func(model, source string, input, output, cacheRead, cacheWrite float64) storage.PricingAuditRow {
		i, o, cr, cw := perM(input, output, cacheRead, cacheWrite)
		return storage.PricingAuditRow{
			Model:                  model,
			PricingSource:          source,
			MatchedModel:           model,
			MatchType:              "official-seed",
			Priority:               20,
			InputCostPerToken:      i,
			OutputCostPerToken:     o,
			CacheReadCostPerToken:  cr,
			CacheWriteCostPerToken: cw,
			Confidence:             "official",
		}
	}
	var rows []storage.PricingAuditRow
	add := func(models []string, source string, input, output, cacheRead, cacheWrite float64) {
		for _, model := range models {
			rows = append(rows, row(model, source, input, output, cacheRead, cacheWrite))
		}
	}
	add([]string{"claude-fable-5", "claude-mythos-5"}, "anthropic-official", 10, 50, 1, 12.5)
	add([]string{
		"claude-opus-4.8", "claude-opus-4-8",
		"claude-opus-4.7", "claude-opus-4-7",
		"claude-opus-4.6", "claude-opus-4-6",
		"claude-opus-4.5", "claude-opus-4-5",
	}, "anthropic-official", 5, 25, 0.50, 6.25)
	add([]string{"claude-opus-4.1", "claude-opus-4-1", "claude-opus-4"}, "anthropic-official", 15, 75, 1.50, 18.75)
	add([]string{
		"claude-sonnet-4.6", "claude-sonnet-4-6",
		"claude-sonnet-4.5", "claude-sonnet-4-5",
		"claude-sonnet-4", "claude-sonnet-4-20250514",
	}, "anthropic-official", 3, 15, 0.30, 3.75)
	add([]string{"claude-haiku-4.5", "claude-haiku-4-5"}, "anthropic-official", 1, 5, 0.10, 1.25)
	add([]string{"claude-haiku-3.5", "claude-haiku-3-5"}, "anthropic-official", 0.80, 4, 0.08, 1)
	add([]string{"gpt-5", "gpt-5-codex"}, "openai-official", 1.25, 10, 0.125, 0)
	add([]string{"gpt-5-mini"}, "openai-official", 0.25, 2, 0.025, 0)
	add([]string{"gpt-5-nano"}, "openai-official", 0.05, 0.40, 0.005, 0)
	add([]string{"gpt-5.5", "gpt-5-5"}, "openai-official", 5, 30, 0.50, 0)
	add([]string{"gpt-5.4", "gpt-5-4"}, "openai-official", 2, 15, 0.20, 0)
	add([]string{"gpt-5.4-mini", "gpt-5-4-mini"}, "openai-official", 0.40, 2, 0.04, 0)
	return rows
}

func fetchBytes(url string) ([]byte, string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Agent-Ledger/1.0 (agent-ledger; local FinOps)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("pricing: fetch failed with HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxPricingResponseBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(body)) > maxPricingResponseBytes {
		return nil, "", fmt.Errorf("pricing: response exceeds %d bytes", maxPricingResponseBytes)
	}
	return body, resp.Header.Get("ETag"), nil
}

// CalcCost computes the USD cost for a single API call given token counts and
// per-token prices. The prices array is [input, output, cache_read, cache_creation].
// input_tokens is the non-cached input only (cache tokens are separate fields).
func CalcCost(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64 {
	inputPrice := prices[0]
	outputPrice := prices[1]
	cacheReadPrice := prices[2]
	cacheCreatePrice := prices[3]

	cost := float64(inputTokens)*inputPrice +
		float64(cacheCreation)*cacheCreatePrice +
		float64(cacheRead)*cacheReadPrice +
		float64(outputTokens)*outputPrice
	return cost
}
