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
	if err := syncLiteLLM(db); err != nil {
		_ = db.UpsertPricingSource(storage.PricingSourceStatus{
			Name: "litellm", Kind: "fallback", Priority: 100, URL: liteLLMPricingURL, LastFetchAt: now, Status: "error", LastError: err.Error(),
		})
		return err
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
	_ = db.InsertPricingAuditEvent("sync", "pricing", "", "pricing sync completed with official overlays and LiteLLM fallback")
	return nil
}

func syncLiteLLM(db *storage.DB) error {
	body, etag, err := fetchBytes(liteLLMPricingURL)
	now := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(body)
	sha := hex.EncodeToString(sum[:])
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
	for _, row := range official {
		if err := db.UpsertPricingDetailed(row); err != nil {
			return err
		}
		sourceCounts[row.PricingSource]++
	}
	for _, s := range []storage.PricingSourceStatus{
		{Name: "openai-official", Kind: "official", Priority: 20, URL: openAIPricingURL, LastFetchAt: now, ModelCount: sourceCounts["openai-official"], Status: "seeded"},
		{Name: "anthropic-official", Kind: "official", Priority: 20, URL: anthropicPricingURL, LastFetchAt: now, ModelCount: sourceCounts["anthropic-official"], Status: "seeded"},
	} {
		if err := db.UpsertPricingSource(s); err != nil {
			return err
		}
		_ = db.InsertPricingSnapshot(s.Name, "seed-"+s.Name, s.ModelCount, map[string]string{"url": s.URL, "mode": "official-seed"})
	}
	return nil
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
	for _, model := range []string{"claude-opus-4.8", "claude-opus-4.7", "claude-opus-4.6", "claude-opus-4.5"} {
		rows = append(rows, row(model, "anthropic-official", 5, 25, 0.50, 6.25))
	}
	for _, model := range []string{"claude-sonnet-4.6", "claude-sonnet-4.5", "claude-sonnet-4"} {
		rows = append(rows, row(model, "anthropic-official", 3, 15, 0.30, 3.75))
	}
	for _, model := range []string{"claude-haiku-4.5"} {
		rows = append(rows, row(model, "anthropic-official", 1, 5, 0.10, 1.25))
	}
	for _, model := range []string{"gpt-5", "gpt-5-codex"} {
		rows = append(rows, row(model, "openai-official", 1.25, 10, 0.125, 0))
	}
	for _, model := range []string{"gpt-5-mini"} {
		rows = append(rows, row(model, "openai-official", 0.25, 2, 0.025, 0))
	}
	for _, model := range []string{"gpt-5-nano"} {
		rows = append(rows, row(model, "openai-official", 0.05, 0.40, 0.005, 0))
	}
	for _, model := range []string{"gpt-5.5"} {
		rows = append(rows, row(model, "openai-official", 5, 30, 0.50, 0))
	}
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
