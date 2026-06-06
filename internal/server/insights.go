package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) handlePricingStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sources, err := s.db.GetPricingSources(s.options.Pricing.StaleAfter)
	if err != nil {
		serverError(w, err)
		return
	}
	unpriced, err := s.db.GetDataQuality(s.options.Pricing.StaleAfter)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{
		"sources":         sources,
		"unpriced_models": unpriced.UnpricedModels,
		"confidence_mix":  unpriced.ConfidenceMix,
		"mode":            s.options.Pricing.Mode,
		"stale_after":     s.options.Pricing.StaleAfter.String(),
	})
}

func (s *Server) handlePricingSync(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "admin") {
		return
	}
	if s.options.PricingSync == nil {
		http.Error(w, "pricing sync is not configured", http.StatusServiceUnavailable)
		return
	}
	if err := s.options.PricingSync(); err != nil {
		serverError(w, err)
		return
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "pricing.sync", "", nil)
	writeJSON(w, map[string]interface{}{"ok": true})
}

func (s *Server) handlePricingRecalculate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "admin") {
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "zero"
	}
	if mode != "zero" && mode != "all" {
		badRequest(w, fmt.Errorf("unsupported recalculate mode %q", mode))
		return
	}
	if s.options.RecalcMode != nil {
		if err := s.options.RecalcMode(mode); err != nil {
			serverError(w, err)
			return
		}
	} else if s.options.Recalc != nil {
		if err := s.options.Recalc(); err != nil {
			serverError(w, err)
			return
		}
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "pricing.recalculate", mode, map[string]string{"mode": mode})
	writeJSON(w, map[string]interface{}{"ok": true, "mode": mode})
}

func (s *Server) handlePricingAudit(w http.ResponseWriter, r *http.Request) {
	limit := parseLimit(r, 1000)
	rows, err := s.db.GetPricingAudit(limit)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleDataQuality(w http.ResponseWriter, r *http.Request) {
	report, err := s.db.GetDataQuality(s.options.Pricing.StaleAfter)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, report)
}

func (s *Server) handleModelCalls(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	rows, err := s.db.GetModelCalls(from, to, r.URL.Query().Get("source"), r.URL.Query().Get("model"), r.URL.Query().Get("project"), parseLimit(r, 200))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleCostIntelligence(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	rows, err := s.db.GetCostIntelligence(from, to, r.URL.Query().Get("source"), r.URL.Query().Get("model"), r.URL.Query().Get("project"), parseLimit(r, 20))
	if err != nil {
		serverError(w, err)
		return
	}
	privacy := s.privacyFor(r)
	if privacy.HashSessionIDs || privacy.ScreenshotMode {
		for i := range rows {
			rows[i].SessionID = hashValue(rows[i].SessionID)
		}
	}
	if privacy.HideProjectNames || privacy.ScreenshotMode {
		for i := range rows {
			rows[i].Project = "project"
			rows[i].GitBranch = "branch"
		}
	}
	writeJSON(w, rows)
}

func (s *Server) handleCacheDoctor(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	rows, err := s.db.GetCacheDoctor(from, to, r.URL.Query().Get("source"), r.URL.Query().Get("model"), r.URL.Query().Get("project"), parseLimit(r, 100))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleQuotaStatus(w http.ResponseWriter, r *http.Request) {
	now := time.Now()
	dayFrom, dayTo, _ := budgetWindow(now, "day")
	weekFrom, weekTo, _ := budgetWindow(now, "week")
	monthFrom, monthTo, _ := budgetWindow(now, "month")
	type window struct {
		Name            string  `json:"name"`
		From            string  `json:"from"`
		To              string  `json:"to"`
		CostUSD         float64 `json:"cost_usd"`
		Tokens          int64   `json:"tokens"`
		Prompts         int     `json:"prompts"`
		CostLimit       float64 `json:"cost_limit"`
		TokenLimit      int64   `json:"token_limit"`
		RemainingCost   float64 `json:"remaining_cost"`
		RemainingTokens int64   `json:"remaining_tokens"`
		BurnRatePerHour float64 `json:"burn_rate_per_hour"`
	}
	makeWindow := func(name string, from, to time.Time) (window, error) {
		stats, err := s.db.GetDashboardStatsFiltered(from, to, "", "", "")
		if err != nil {
			return window{}, err
		}
		hours := mathMax(1, time.Since(from).Hours())
		costLimit := s.options.Quota.MonthlyBudget
		tokenLimit := s.options.Quota.TokenBudget
		if name == "5h" {
			costLimit = s.options.Quota.MonthlyBudget / 30 / 24 * 5
			tokenLimit = s.options.Quota.TokenBudget / 30 / 24 * 5
		} else if name == "day" {
			costLimit = s.options.Quota.MonthlyBudget / 30
			tokenLimit = s.options.Quota.TokenBudget / 30
		} else if name == "week" {
			costLimit = s.options.Quota.MonthlyBudget / 4.35
			tokenLimit = s.options.Quota.TokenBudget / 4
		}
		return window{
			Name: name, From: from.Format(time.RFC3339), To: to.Format(time.RFC3339),
			CostUSD: stats.TotalCost, Tokens: stats.TotalTokens, Prompts: stats.TotalPrompts,
			CostLimit: costLimit, TokenLimit: tokenLimit,
			RemainingCost: costLimit - stats.TotalCost, RemainingTokens: tokenLimit - stats.TotalTokens,
			BurnRatePerHour: stats.TotalCost / hours,
		}, nil
	}
	var windows []window
	for _, spec := range []struct {
		name string
		from time.Time
		to   time.Time
	}{{"5h", now.Add(-5 * time.Hour), now}, {"day", dayFrom, dayTo}, {"week", weekFrom, weekTo}, {"month", monthFrom, monthTo}} {
		wnd, err := makeWindow(spec.name, spec.from, spec.to)
		if err != nil {
			serverError(w, err)
			return
		}
		windows = append(windows, wnd)
	}
	writeJSON(w, map[string]interface{}{
		"enabled":   s.options.Quota.Enabled,
		"plan":      s.options.Quota.Plan,
		"reset_day": s.options.Quota.ResetDay,
		"windows":   windows,
		"method":    "local-estimate",
	})
}

func (s *Server) handleAnomalies(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	if err := s.db.DetectAnomalies(from, to, s.options.Watchdog.TokenSpikeMultiplier, s.options.Watchdog.NightStartHour, s.options.Watchdog.NightEndHour); err != nil {
		serverError(w, err)
		return
	}
	rows, err := s.db.GetInsightEvents("anomaly", parseLimit(r, 100))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleWatchdogEvents(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.GetInsightEvents("watchdog", parseLimit(r, 100))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "viewer") {
		return
	}
	rows, err := s.db.GetAuditLog(parseLimit(r, 200))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleReconciliationStatus(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.GetReconciliationImports(parseLimit(r, 50))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleReconciliationImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	var payload struct {
		Provider        string  `json:"provider"`
		Format          string  `json:"format"`
		ProviderCostUSD float64 `json:"provider_cost_usd"`
		LocalCostUSD    float64 `json:"local_cost_usd"`
		RowsSeen        int     `json:"rows_seen"`
		Notes           string  `json:"notes"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		badRequest(w, err)
		return
	}
	if payload.Provider == "" {
		payload.Provider = "manual"
	}
	if payload.Format == "" {
		payload.Format = "json"
	}
	if payload.LocalCostUSD == 0 {
		from, to, _, err := s.parseTimeRange(r)
		if err == nil {
			stats, _ := s.db.GetDashboardStatsFiltered(from, to, "", "", "")
			if stats != nil {
				payload.LocalCostUSD = stats.TotalCost
			}
		}
	}
	if err := s.db.InsertReconciliationImport(payload.Provider, payload.Format, payload.LocalCostUSD, payload.ProviderCostUSD, payload.RowsSeen, payload.Notes); err != nil {
		serverError(w, err)
		return
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "reconciliation.import", payload.Provider, map[string]string{"format": payload.Format})
	writeJSON(w, map[string]interface{}{"ok": true})
}

func (s *Server) handleEvidenceBundle(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	quality, err := s.db.GetDataQuality(s.options.Pricing.StaleAfter)
	if err != nil {
		serverError(w, err)
		return
	}
	health, _ := s.db.GetIngestionHealth()
	pricingRows, _ := s.db.GetPricingAudit(500)
	insights, _ := s.db.GetCostIntelligence(from, to, r.URL.Query().Get("source"), r.URL.Query().Get("model"), r.URL.Query().Get("project"), 20)
	bundle := map[string]interface{}{
		"product":           "Agent Ledger",
		"generated_at":      time.Now().UTC().Format(time.RFC3339),
		"window":            map[string]string{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339)},
		"privacy":           "redacted",
		"quality":           quality,
		"ingestion_health":  health,
		"pricing_audit":     pricingRows,
		"cost_intelligence": insights,
	}
	raw, _ := json.Marshal(bundle)
	_ = s.db.RecordOfflineBundle(fmt.Sprintf("evidence-%d", time.Now().Unix()), raw, "json")
	w.Header().Set("Content-Disposition", "attachment; filename=agent-ledger-evidence.json")
	writeJSON(w, bundle)
}

func (s *Server) handlePolicyStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]interface{}{
		"enabled":                s.options.Policies.Enabled,
		"require_privacy_export": s.options.Policies.RequirePrivacyExport,
		"rules":                  s.options.Policies.Rules,
		"webhooks_enabled":       s.options.Webhooks.Enabled,
	})
}

func parseLimit(r *http.Request, fallback int) int {
	limit, err := strconv.Atoi(r.URL.Query().Get("limit"))
	if err != nil || limit <= 0 {
		return fallback
	}
	if limit > 5000 {
		return 5000
	}
	return limit
}

func mathMax(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
