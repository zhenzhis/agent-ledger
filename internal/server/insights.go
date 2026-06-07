package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/reconciliation"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

type reconciliationImportPayload struct {
	Provider        string  `json:"provider"`
	Format          string  `json:"format"`
	ProviderCostUSD float64 `json:"provider_cost_usd"`
	LocalCostUSD    float64 `json:"local_cost_usd"`
	RowsSeen        int     `json:"rows_seen"`
	Notes           string  `json:"notes"`
	Raw             string  `json:"raw"`
}

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

func (s *Server) handleDoctor(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireRole(w, r, "viewer") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	report, err := s.db.GetDoctorReport(from, to, s.options.Pricing.StaleAfter,
		r.URL.Query().Get("source"), r.URL.Query().Get("model"), r.URL.Query().Get("project"))
	if err != nil {
		serverError(w, err)
		return
	}
	applyDoctorPrivacy(report, s.privacyFor(r))
	if strings.EqualFold(r.URL.Query().Get("format"), "markdown") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(storage.FormatDoctorMarkdown(report)))
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
		Name             string  `json:"name"`
		From             string  `json:"from"`
		To               string  `json:"to"`
		CostUSD          float64 `json:"cost_usd"`
		Tokens           int64   `json:"tokens"`
		Prompts          int     `json:"prompts"`
		CostLimit        float64 `json:"cost_limit"`
		TokenLimit       int64   `json:"token_limit"`
		RemainingCost    float64 `json:"remaining_cost"`
		RemainingTokens  int64   `json:"remaining_tokens"`
		BurnRatePerHour  float64 `json:"burn_rate_per_hour"`
		ProjectedCostUSD float64 `json:"projected_cost_usd"`
		ProjectedTokens  int64   `json:"projected_tokens"`
		ResetAt          string  `json:"reset_at"`
		TimeToLimitHours float64 `json:"time_to_limit_hours"`
	}
	makeWindow := func(name string, from, to time.Time) (window, error) {
		stats, err := s.db.GetDashboardStatsFiltered(from, to, "", "", "")
		if err != nil {
			return window{}, err
		}
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
		elapsedHours := mathMax(1, time.Since(from).Hours())
		windowHours := mathMax(1, to.Sub(from).Hours())
		burnRate := stats.TotalCost / elapsedHours
		tokenBurnRate := float64(stats.TotalTokens) / elapsedHours
		timeToLimit := -1.0
		if costLimit > 0 && burnRate > 0 {
			timeToLimit = (costLimit - stats.TotalCost) / burnRate
		} else if tokenLimit > 0 && tokenBurnRate > 0 {
			timeToLimit = float64(tokenLimit-stats.TotalTokens) / tokenBurnRate
		}
		resetAt := ""
		if name != "5h" {
			resetAt = to.Format(time.RFC3339)
		}
		return window{
			Name: name, From: from.Format(time.RFC3339), To: to.Format(time.RFC3339),
			CostUSD: stats.TotalCost, Tokens: stats.TotalTokens, Prompts: stats.TotalPrompts,
			CostLimit: costLimit, TokenLimit: tokenLimit,
			RemainingCost: costLimit - stats.TotalCost, RemainingTokens: tokenLimit - stats.TotalTokens,
			BurnRatePerHour:  burnRate,
			ProjectedCostUSD: burnRate * windowHours,
			ProjectedTokens:  int64(tokenBurnRate * windowHours),
			ResetAt:          resetAt,
			TimeToLimitHours: timeToLimit,
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

func (s *Server) handleRouterSimulation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireRole(w, r, "viewer") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	toModel := firstNonEmpty(r.URL.Query().Get("to_model"), r.URL.Query().Get("target_model"))
	if toModel == "" {
		badRequest(w, fmt.Errorf("to_model is required"))
		return
	}
	fromModel := firstNonEmpty(r.URL.Query().Get("from_model"), r.URL.Query().Get("model"))
	ratio := 1.0
	if raw := firstNonEmpty(r.URL.Query().Get("ratio"), r.URL.Query().Get("replacement_ratio")); raw != "" {
		ratio, err = strconv.ParseFloat(raw, 64)
		if err != nil {
			badRequest(w, fmt.Errorf("invalid replacement ratio %q", raw))
			return
		}
	}
	report, err := s.db.SimulateModelRouting(from, to, r.URL.Query().Get("source"), fromModel, toModel, r.URL.Query().Get("project"), ratio, parseLimit(r, 200))
	if err != nil {
		badRequest(w, err)
		return
	}
	writeJSON(w, report)
}

func (s *Server) handlePreflightEstimate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireRole(w, r, "viewer") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	task := firstNonEmpty(r.URL.Query().Get("task"), r.URL.Query().Get("type"), "custom")
	report, err := s.db.EstimatePreflightCost(from, to, task, r.URL.Query().Get("source"), r.URL.Query().Get("model"), r.URL.Query().Get("project"), parseLimit(r, 2000))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, report)
}

func (s *Server) handleChargeback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireRole(w, r, "viewer") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	rows, err := s.db.GetChargeback(from, to,
		r.URL.Query().Get("source"),
		r.URL.Query().Get("model"),
		r.URL.Query().Get("project"),
		s.options.Teams.Groups,
		s.options.Teams.MachineName,
		s.options.Teams.GitAuthor,
		parseLimit(r, 200),
	)
	if err != nil {
		serverError(w, err)
		return
	}
	applyChargebackPrivacy(rows, s.privacyFor(r))
	writeJSON(w, rows)
}

func (s *Server) handleWrapped(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireRole(w, r, "viewer") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	report, err := s.db.GetAgentWrapped(from, to,
		firstNonEmpty(r.URL.Query().Get("period"), "custom"),
		r.URL.Query().Get("source"),
		r.URL.Query().Get("model"),
		r.URL.Query().Get("project"),
	)
	if err != nil {
		serverError(w, err)
		return
	}
	applyWrappedPrivacy(report, s.privacyFor(r))
	if strings.EqualFold(r.URL.Query().Get("format"), "markdown") {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		_, _ = w.Write([]byte(storage.FormatWrappedMarkdown(report)))
		return
	}
	writeJSON(w, report)
}

func (s *Server) handleReconciliationImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	var payload reconciliationImportPayload
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4<<20))
	if err != nil {
		badRequest(w, err)
		return
	}
	if len(raw) == 0 {
		badRequest(w, fmt.Errorf("reconciliation import body is required"))
		return
	}
	isJSON := strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "json") || len(raw) > 0 && raw[0] == '{'
	hasSummaryFields := false
	if isJSON {
		_ = json.Unmarshal(raw, &payload)
		var probe map[string]interface{}
		if err := json.Unmarshal(raw, &probe); err == nil {
			for _, key := range []string{"provider_cost_usd", "local_cost_usd", "rows_seen", "notes", "raw"} {
				if _, ok := probe[key]; ok {
					hasSummaryFields = true
					break
				}
			}
		}
	}
	row, err := s.reconciliationRowFromRequest(r, raw, payload, hasSummaryFields)
	if err != nil {
		badRequest(w, err)
		return
	}
	storage.PrepareReconciliationImport(row)
	if err := s.db.InsertReconciliationImportDetailed(*row); err != nil {
		serverError(w, err)
		return
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "reconciliation.import", row.Provider, map[string]string{"format": row.Format, "sha256": row.PayloadSHA256})
	writeJSON(w, map[string]interface{}{"ok": true, "import": row})
}

func (s *Server) reconciliationRowFromRequest(r *http.Request, raw []byte, payload reconciliationImportPayload, hasSummaryFields bool) (*storage.ReconciliationImport, error) {
	format := firstNonEmpty(r.URL.Query().Get("format"), payload.Format)
	provider := firstNonEmpty(r.URL.Query().Get("provider"), payload.Provider)
	row := &storage.ReconciliationImport{}
	if !hasSummaryFields || payload.Raw != "" || strings.Contains(strings.ToLower(r.Header.Get("Content-Type")), "text/csv") {
		statementRaw := raw
		if payload.Raw != "" {
			statementRaw = []byte(payload.Raw)
		}
		summary, err := reconciliation.ParseProviderStatement(statementRaw, format, provider)
		if err != nil {
			return nil, err
		}
		row.Provider = summary.Provider
		row.Format = summary.Format
		row.Currency = summary.Currency
		row.ProviderCostUSD = summary.ProviderCostUSD
		row.RowsSeen = summary.RowsSeen
		row.PayloadSHA256 = summary.PayloadSHA256
		if !summary.WindowStart.IsZero() {
			row.WindowStart = summary.WindowStart.Format(time.RFC3339)
		}
		if !summary.WindowEnd.IsZero() {
			row.WindowEnd = summary.WindowEnd.Format(time.RFC3339)
		}
		row.Warnings = reconciliation.WarningsJSON(summary.Warnings)
		row.Notes = payload.Notes
	} else {
		row.Provider = firstNonEmpty(provider, "manual")
		row.Format = firstNonEmpty(format, "json")
		row.Currency = "USD"
		row.ProviderCostUSD = payload.ProviderCostUSD
		row.LocalCostUSD = payload.LocalCostUSD
		row.RowsSeen = payload.RowsSeen
		row.Notes = payload.Notes
	}
	if row.LocalCostUSD == 0 {
		from, to, err := reconciliationLocalWindow(r, row)
		if err != nil {
			return nil, err
		}
		stats, err := s.db.GetDashboardStatsFiltered(from, to, r.URL.Query().Get("source"), r.URL.Query().Get("model"), r.URL.Query().Get("project"))
		if err != nil {
			return nil, err
		}
		row.LocalCostUSD = stats.TotalCost
	}
	return row, nil
}

func reconciliationLocalWindow(r *http.Request, row *storage.ReconciliationImport) (time.Time, time.Time, error) {
	if r.URL.Query().Get("from") != "" || r.URL.Query().Get("to") != "" {
		from := r.URL.Query().Get("from")
		to := r.URL.Query().Get("to")
		if from == "" || to == "" {
			return time.Time{}, time.Time{}, fmt.Errorf("both from and to are required when overriding reconciliation window")
		}
		fromTime, err := time.Parse("2006-01-02", from)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		toTime, err := time.Parse("2006-01-02", to)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return fromTime, toTime.AddDate(0, 0, 1), nil
	}
	if row.WindowStart != "" && row.WindowEnd != "" {
		from, err := time.Parse(time.RFC3339, row.WindowStart)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		to, err := time.Parse(time.RFC3339, row.WindowEnd)
		if err != nil {
			return time.Time{}, time.Time{}, err
		}
		return from, reconciliationStatementWindowEnd(to), nil
	}
	now := time.Now()
	return now.AddDate(0, -1, 0), now, nil
}

func reconciliationStatementWindowEnd(end time.Time) time.Time {
	if end.Hour() == 0 && end.Minute() == 0 && end.Second() == 0 && end.Nanosecond() == 0 {
		return end.AddDate(0, 0, 1)
	}
	return end.Add(time.Nanosecond)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func (s *Server) handleEvidenceBundle(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	if !s.evaluateOperationPolicy(w, r, "export", source, model, project, "evidence-bundle") {
		return
	}
	quality, err := s.db.GetDataQuality(s.options.Pricing.StaleAfter)
	if err != nil {
		serverError(w, err)
		return
	}
	health, _ := s.db.GetIngestionHealth()
	pricingRows, _ := s.db.GetPricingAudit(500)
	insights, _ := s.db.GetCostIntelligence(from, to, source, model, project, 20)
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
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "export", "evidence-bundle", map[string]string{"privacy": "redacted", "source": source, "model": model, "project": project})
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
