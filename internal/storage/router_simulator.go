package storage

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// RouterSimulationRow estimates the cost impact of routing a usage group to a
// different target model. It never mutates ledger records.
type RouterSimulationRow struct {
	Source                   string  `json:"source"`
	FromModel                string  `json:"from_model"`
	ToModel                  string  `json:"to_model"`
	Project                  string  `json:"project"`
	Calls                    int     `json:"calls"`
	InputTokens              int64   `json:"input_tokens"`
	OutputTokens             int64   `json:"output_tokens"`
	CacheCreationInputTokens int64   `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int64   `json:"cache_read_input_tokens"`
	Tokens                   int64   `json:"tokens"`
	CurrentCostUSD           float64 `json:"current_cost_usd"`
	SimulatedCostUSD         float64 `json:"simulated_cost_usd"`
	DeltaUSD                 float64 `json:"delta_usd"`
	SavingsPct               float64 `json:"savings_pct"`
	ReplacementRatio         float64 `json:"replacement_ratio"`
	UnpricedCurrentCalls     int     `json:"unpriced_current_calls"`
	TargetPricingSource      string  `json:"target_pricing_source"`
	TargetPricingModel       string  `json:"target_pricing_model"`
	TargetMatchType          string  `json:"target_match_type"`
	TargetConfidence         string  `json:"target_confidence"`
	Note                     string  `json:"note,omitempty"`
}

// RouterSimulationSummary aggregates the modeled impact across all rows.
type RouterSimulationSummary struct {
	Calls                int     `json:"calls"`
	Tokens               int64   `json:"tokens"`
	CurrentCostUSD       float64 `json:"current_cost_usd"`
	SimulatedCostUSD     float64 `json:"simulated_cost_usd"`
	DeltaUSD             float64 `json:"delta_usd"`
	SavingsPct           float64 `json:"savings_pct"`
	Groups               int     `json:"groups"`
	UnpricedCurrentCalls int     `json:"unpriced_current_calls"`
}

// RouterSimulationReport describes a what-if model routing scenario.
type RouterSimulationReport struct {
	GeneratedAt      string                  `json:"generated_at"`
	From             string                  `json:"from"`
	To               string                  `json:"to"`
	Source           string                  `json:"source,omitempty"`
	FromModel        string                  `json:"from_model,omitempty"`
	ToModel          string                  `json:"to_model"`
	Project          string                  `json:"project,omitempty"`
	ReplacementRatio float64                 `json:"replacement_ratio"`
	TargetPricing    PricingAuditRow         `json:"target_pricing"`
	Status           string                  `json:"status"`
	Issues           []string                `json:"issues,omitempty"`
	Summary          RouterSimulationSummary `json:"summary"`
	Rows             []RouterSimulationRow   `json:"rows"`
}

// SimulateModelRouting estimates cost if matching usage were routed to
// targetModel. replacementRatio is clamped by callers and must be in (0,1].
func (d *DB) SimulateModelRouting(from, to time.Time, source, fromModel, targetModel, project string, replacementRatio float64, limit int) (*RouterSimulationReport, error) {
	targetModel = strings.TrimSpace(targetModel)
	if targetModel == "" {
		return nil, fmt.Errorf("target model is required")
	}
	if replacementRatio <= 0 || replacementRatio > 1 {
		return nil, fmt.Errorf("replacement ratio must be > 0 and <= 1")
	}
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	prices, err := d.GetAllPricingDetailed()
	if err != nil {
		return nil, err
	}
	target, ok := matchPricingDetailed(targetModel, prices)
	if !ok {
		return nil, fmt.Errorf("target model %q is unpriced; sync pricing or add a local override", targetModel)
	}
	targetPrices := [4]float64{target.InputCostPerToken, target.OutputCostPerToken, target.CacheReadCostPerToken, target.CacheWriteCostPerToken}
	filter, filterArgs := buildUsageFilterAlias("u", source, fromModel, project)
	args := append([]interface{}{from, to}, filterArgs...)
	args = append(args, limit)
	rows, err := d.db.Query(`SELECT u.source,u.model,u.project,COUNT(*),
		COALESCE(SUM(u.input_tokens),0), COALESCE(SUM(u.output_tokens),0),
		COALESCE(SUM(u.cache_creation_input_tokens),0), COALESCE(SUM(u.cache_read_input_tokens),0),
		COALESCE(SUM(u.cost_usd),0),
		SUM(CASE WHEN COALESCE(u.pricing_confidence,'')='unpriced' OR u.cost_usd=0 THEN 1 ELSE 0 END)
		FROM usage_records u WHERE u.timestamp >= ? AND u.timestamp < ?`+filter+`
		GROUP BY u.source,u.model,u.project ORDER BY SUM(u.cost_usd) DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	report := &RouterSimulationReport{
		GeneratedAt:      time.Now().UTC().Format(time.RFC3339),
		From:             from.UTC().Format(time.RFC3339),
		To:               to.UTC().Format(time.RFC3339),
		Source:           source,
		FromModel:        fromModel,
		ToModel:          targetModel,
		Project:          project,
		ReplacementRatio: replacementRatio,
		TargetPricing:    target,
		Status:           "ok",
		Rows:             []RouterSimulationRow{},
	}
	for rows.Next() {
		var r RouterSimulationRow
		var currentCost float64
		if err := rows.Scan(&r.Source, &r.FromModel, &r.Project, &r.Calls, &r.InputTokens, &r.OutputTokens,
			&r.CacheCreationInputTokens, &r.CacheReadInputTokens, &currentCost, &r.UnpricedCurrentCalls); err != nil {
			return nil, err
		}
		if fromModel == "" && strings.EqualFold(r.FromModel, targetModel) {
			continue
		}
		targetCostAll := calcTokensCost(r.InputTokens, r.OutputTokens, r.CacheCreationInputTokens, r.CacheReadInputTokens, targetPrices)
		r.ToModel = targetModel
		r.Tokens = r.InputTokens + r.OutputTokens + r.CacheCreationInputTokens + r.CacheReadInputTokens
		r.CurrentCostUSD = currentCost
		r.SimulatedCostUSD = currentCost*(1-replacementRatio) + targetCostAll*replacementRatio
		r.DeltaUSD = r.SimulatedCostUSD - r.CurrentCostUSD
		if r.CurrentCostUSD > 0 {
			r.SavingsPct = -r.DeltaUSD / r.CurrentCostUSD
		}
		r.ReplacementRatio = replacementRatio
		r.TargetPricingSource = target.PricingSource
		r.TargetPricingModel = target.MatchedModel
		r.TargetMatchType = target.MatchType
		r.TargetConfidence = target.Confidence
		if r.UnpricedCurrentCalls > 0 {
			r.Note = "current ledger contains unpriced or zero-cost calls; savings are conservative"
		}
		report.Rows = append(report.Rows, r)
		report.Summary.Calls += r.Calls
		report.Summary.Tokens += r.Tokens
		report.Summary.CurrentCostUSD += r.CurrentCostUSD
		report.Summary.SimulatedCostUSD += r.SimulatedCostUSD
		report.Summary.UnpricedCurrentCalls += r.UnpricedCurrentCalls
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	report.Summary.Groups = len(report.Rows)
	report.Summary.DeltaUSD = report.Summary.SimulatedCostUSD - report.Summary.CurrentCostUSD
	if report.Summary.CurrentCostUSD > 0 {
		report.Summary.SavingsPct = -report.Summary.DeltaUSD / report.Summary.CurrentCostUSD
	}
	if report.Summary.Groups == 0 {
		report.Status = "empty"
		report.Issues = append(report.Issues, "no matching usage rows for this routing scenario")
	}
	if report.Summary.UnpricedCurrentCalls > 0 {
		report.Status = "warning"
		report.Issues = append(report.Issues, fmt.Sprintf("%d current calls are unpriced or zero-cost", report.Summary.UnpricedCurrentCalls))
	}
	roundRouterReport(report)
	return report, nil
}

func calcTokensCost(input, output, cacheCreation, cacheRead int64, prices [4]float64) float64 {
	return float64(input)*prices[0] +
		float64(output)*prices[1] +
		float64(cacheRead)*prices[2] +
		float64(cacheCreation)*prices[3]
}

func roundRouterReport(report *RouterSimulationReport) {
	roundCost := func(v float64) float64 { return math.Round(v*1_000_000) / 1_000_000 }
	roundPct := func(v float64) float64 { return math.Round(v*10_000) / 10_000 }
	report.Summary.CurrentCostUSD = roundCost(report.Summary.CurrentCostUSD)
	report.Summary.SimulatedCostUSD = roundCost(report.Summary.SimulatedCostUSD)
	report.Summary.DeltaUSD = roundCost(report.Summary.DeltaUSD)
	report.Summary.SavingsPct = roundPct(report.Summary.SavingsPct)
	for i := range report.Rows {
		report.Rows[i].CurrentCostUSD = roundCost(report.Rows[i].CurrentCostUSD)
		report.Rows[i].SimulatedCostUSD = roundCost(report.Rows[i].SimulatedCostUSD)
		report.Rows[i].DeltaUSD = roundCost(report.Rows[i].DeltaUSD)
		report.Rows[i].SavingsPct = roundPct(report.Rows[i].SavingsPct)
	}
}
