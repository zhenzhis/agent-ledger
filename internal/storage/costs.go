package storage

import "strings"

// CostCalcFunc is a function that calculates USD cost from token counts and per-token prices.
type CostCalcFunc func(inputTokens, outputTokens, cacheCreation, cacheRead int64, prices [4]float64) float64

// RecalcCosts recalculates costs for all usage records where cost_usd is zero,
// using fuzzy model name matching against the provided pricing map.
func (d *DB) RecalcCosts(allPrices map[string][4]float64, calcFn CostCalcFunc) error {
	return d.RecalcCostsMode(allPrices, calcFn, "zero")
}

// RecalcCostsMode recalculates usage costs. mode=zero preserves non-zero rows;
// mode=all recalculates every row except source-reported rows marked as such.
func (d *DB) RecalcCostsMode(allPrices map[string][4]float64, calcFn CostCalcFunc, mode string) error {
	detailed := make(map[string]PricingAuditRow, len(allPrices))
	for model, prices := range allPrices {
		detailed[model] = PricingAuditRow{
			Model:                  model,
			PricingSource:          "legacy",
			MatchedModel:           model,
			MatchType:              "direct",
			Priority:               999,
			InputCostPerToken:      prices[0],
			OutputCostPerToken:     prices[1],
			CacheReadCostPerToken:  prices[2],
			CacheWriteCostPerToken: prices[3],
			Confidence:             "fallback",
		}
	}
	return d.RecalcCostsDetailed(detailed, calcFn, mode, false)
}

// RecalcCostsDetailed recalculates costs and stores pricing governance metadata.
func (d *DB) RecalcCostsDetailed(allPrices map[string]PricingAuditRow, calcFn CostCalcFunc, mode string, forceSourceReported bool) error {
	d.mu.Lock()
	defer d.mu.Unlock()

	where := "WHERE cost_usd = 0"
	if mode == "all" {
		where = "WHERE 1=1"
	}
	if !forceSourceReported {
		where += " AND COALESCE(pricing_confidence,'') != 'source-reported'"
	}
	rows, err := d.db.Query(`SELECT id, model, input_tokens, output_tokens, cache_creation_input_tokens, cache_read_input_tokens, cost_usd FROM usage_records ` + where)
	if err != nil {
		return err
	}
	defer rows.Close()

	type rec struct {
		id                    int64
		model                 string
		input, output, cc, cr int64
		cost                  float64
	}
	var recs []rec
	for rows.Next() {
		var r rec
		if err := rows.Scan(&r.id, &r.model, &r.input, &r.output, &r.cc, &r.cr, &r.cost); err != nil {
			return err
		}
		recs = append(recs, r)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()

	if len(recs) == 0 {
		return nil
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare("UPDATE usage_records SET cost_usd=?, pricing_source=?, pricing_model=?, pricing_confidence=?, pricing_note=? WHERE id=?")
	if err != nil {
		return err
	}
	defer stmt.Close()

	updated := 0
	touched := 0
	for _, r := range recs {
		match, ok := matchPricingDetailed(r.model, allPrices)
		if !ok {
			if r.cost > 0 {
				if _, err := tx.Exec(`UPDATE usage_records
					SET pricing_source=?, pricing_model=?, pricing_confidence=?, pricing_note=?
					WHERE id=?`, "source-reported", r.model, "source-reported", "source reported cost preserved; no pricing rule matched", r.id); err != nil {
					return err
				}
				touched++
				continue
			}
			if _, err := tx.Exec("UPDATE usage_records SET pricing_confidence=?, pricing_note=? WHERE id=?", "unpriced", "no pricing rule matched", r.id); err != nil {
				return err
			}
			touched++
			continue
		}
		prices := [4]float64{match.InputCostPerToken, match.OutputCostPerToken, match.CacheReadCostPerToken, match.CacheWriteCostPerToken}
		cost := calcFn(r.input, r.output, r.cc, r.cr, prices)
		if _, err := stmt.Exec(cost, match.PricingSource, match.MatchedModel, match.Confidence, match.MatchType, r.id); err != nil {
			return err
		}
		if cost > 0 {
			updated++
		}
		touched++
	}

	if touched > 0 || updated > 0 {
		return tx.Commit()
	}
	return nil
}

func matchPricing(model string, allPrices map[string][4]float64) ([4]float64, bool) {
	// Direct match
	if p, ok := allPrices[model]; ok {
		return p, true
	}
	// Try with provider prefix
	for _, prefix := range []string{"anthropic/", "openai/", "deepseek/", "gemini/", "google/", "mistral/", "cohere/", "azure_ai/"} {
		if p, ok := allPrices[prefix+model]; ok {
			return p, true
		}
	}

	// Normalize: replace / with . and version dots with dashes for matching
	norm := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "/", ".")
		return s
	}

	modelNorm := norm(model)
	// Also try normalizing version numbers: 4.6 -> 4-6
	modelNormDash := strings.NewReplacer("4.6", "4-6", "4.5", "4-5", "3.5", "3-5", "5.4", "5-4").Replace(modelNorm)

	var bestKey string
	var bestScore int
	for k := range allPrices {
		kNorm := norm(k)
		for _, mn := range []string{modelNorm, modelNormDash} {
			if strings.Contains(kNorm, mn) || strings.Contains(mn, kNorm) {
				// Shortest key wins — avoids matching reseller paths over original provider
				score := 10000 - len(k)
				if kNorm == mn {
					score += 100000 // exact normalized match bonus
				}
				if score > bestScore {
					bestKey = k
					bestScore = score
				}
			}
		}
	}
	if bestKey != "" {
		p := allPrices[bestKey]
		return p, true
	}
	return [4]float64{}, false
}

func matchPricingDetailed(model string, allPrices map[string]PricingAuditRow) (PricingAuditRow, bool) {
	if p, ok := allPrices[model]; ok {
		p.MatchedModel = model
		p.MatchType = "direct"
		if p.Confidence == "" {
			p.Confidence = confidenceFromPriority(p.Priority)
		}
		return p, true
	}
	for _, prefix := range []string{"anthropic/", "openai/", "deepseek/", "gemini/", "google/", "mistral/", "cohere/", "azure_ai/"} {
		key := prefix + model
		if p, ok := allPrices[key]; ok {
			p.MatchedModel = key
			p.MatchType = "provider-prefix"
			if p.Confidence == "" {
				p.Confidence = confidenceFromPriority(p.Priority)
			}
			return p, true
		}
	}
	norm := func(s string) string {
		s = strings.ToLower(s)
		s = strings.ReplaceAll(s, "/", ".")
		return s
	}
	modelNorm := norm(model)
	modelNormDash := strings.NewReplacer("4.6", "4-6", "4.5", "4-5", "3.5", "3-5", "5.4", "5-4", "5.5", "5-5").Replace(modelNorm)
	var best PricingAuditRow
	var bestScore int
	for k, p := range allPrices {
		kNorm := norm(k)
		for _, mn := range []string{modelNorm, modelNormDash} {
			if strings.Contains(kNorm, mn) || strings.Contains(mn, kNorm) {
				score := 10000 - len(k)
				if kNorm == mn {
					score += 100000
				}
				score -= p.Priority * 10
				if score > bestScore {
					best = p
					best.MatchedModel = k
					best.MatchType = "fuzzy"
					best.Confidence = "fuzzy"
					bestScore = score
				}
			}
		}
	}
	if bestScore > 0 {
		return best, true
	}
	return PricingAuditRow{}, false
}

func confidenceFromPriority(priority int) string {
	switch {
	case priority <= 10:
		return "override"
	case priority <= 50:
		return "official"
	case priority <= 200:
		return "fallback"
	default:
		return "unknown"
	}
}
