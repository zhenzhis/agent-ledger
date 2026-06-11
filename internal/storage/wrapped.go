package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// WrappedProject is a project-level highlight for Agent Wrapped.
type WrappedProject struct {
	Project  string  `json:"project"`
	Sessions int     `json:"sessions"`
	Calls    int     `json:"calls"`
	Tokens   int64   `json:"tokens"`
	CostUSD  float64 `json:"cost_usd"`
}

// WrappedDay is a day-level highlight for activity and cache behavior.
type WrappedDay struct {
	Date         string  `json:"date"`
	Calls        int     `json:"calls"`
	Tokens       int64   `json:"tokens"`
	CostUSD      float64 `json:"cost_usd"`
	CacheHitRate float64 `json:"cache_hit_rate"`
}

// WrappedHighlight is a concise narrative-safe highlight.
type WrappedHighlight struct {
	Label  string `json:"label"`
	Value  string `json:"value"`
	Detail string `json:"detail"`
}

// WrappedReport summarizes a personal/team period without reading prompt text.
type WrappedReport struct {
	GeneratedAt          string             `json:"generated_at"`
	Period               string             `json:"period"`
	From                 string             `json:"from"`
	To                   string             `json:"to"`
	Stats                DashboardStats     `json:"stats"`
	TopModel             CostByModel        `json:"top_model"`
	TopProject           WrappedProject     `json:"top_project"`
	MostActiveDay        WrappedDay         `json:"most_active_day"`
	BestCacheDay         WrappedDay         `json:"best_cache_day"`
	MostExpensiveSession CostInsightRow     `json:"most_expensive_session"`
	Highlights           []WrappedHighlight `json:"highlights"`
	Issues               []string           `json:"issues"`
}

// GetAgentWrapped returns a compact period summary for human review and export.
func (d *DB) GetAgentWrapped(from, to time.Time, period, source, model, project string) (*WrappedReport, error) {
	from, to = utcRange(from, to)
	stats, err := d.GetDashboardStatsFiltered(from, to, source, model, project)
	if err != nil {
		return nil, err
	}
	report := &WrappedReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		Period:      normalizeWrappedPeriod(period),
		From:        from.Format(time.RFC3339),
		To:          to.Format(time.RFC3339),
		Stats:       *stats,
	}
	models, err := d.GetCostByModelFiltered(from, to, source, project)
	if err != nil {
		return nil, err
	}
	if len(models) > 0 {
		report.TopModel = models[0]
	}
	if row, ok, err := d.topWrappedProject(from, to, source, model, project); err != nil {
		return nil, err
	} else if ok {
		report.TopProject = row
	}
	if row, ok, err := d.topWrappedDay(from, to, source, model, project, "tokens"); err != nil {
		return nil, err
	} else if ok {
		report.MostActiveDay = row
	}
	if row, ok, err := d.topWrappedDay(from, to, source, model, project, "cache"); err != nil {
		return nil, err
	} else if ok {
		report.BestCacheDay = row
	}
	insights, err := d.GetCostIntelligence(from, to, source, model, project, 1)
	if err != nil {
		return nil, err
	}
	if len(insights) > 0 {
		report.MostExpensiveSession = insights[0]
	}
	report.Highlights = wrappedHighlights(report)
	if stats.TotalCalls == 0 {
		report.Issues = append(report.Issues, "no usage records in the selected period")
	}
	return report, nil
}

func (d *DB) topWrappedProject(from, to time.Time, source, model, project string) (WrappedProject, bool, error) {
	from, to = utcRange(from, to)
	filter, fa := buildUsageFilter(source, model, project)
	args := append([]interface{}{from, to}, fa...)
	row := d.db.QueryRow(`SELECT COALESCE(NULLIF(project,''),'unknown'),
		COUNT(DISTINCT source || char(0) || session_id), COUNT(*),
		COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens),0),
		COALESCE(SUM(cost_usd),0)
		FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter+`
		GROUP BY COALESCE(NULLIF(project,''),'unknown')
		ORDER BY COALESCE(SUM(cost_usd),0) DESC, COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens),0) DESC
		LIMIT 1`, args...)
	var out WrappedProject
	if err := row.Scan(&out.Project, &out.Sessions, &out.Calls, &out.Tokens, &out.CostUSD); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WrappedProject{}, false, nil
		}
		return WrappedProject{}, false, err
	}
	return out, true, nil
}

func (d *DB) topWrappedDay(from, to time.Time, source, model, project, mode string) (WrappedDay, bool, error) {
	from, to = utcRange(from, to)
	filter, fa := buildUsageFilter(source, model, project)
	args := append([]interface{}{from, to}, fa...)
	order := `tokens DESC, cost_usd DESC`
	if mode == "cache" {
		order = `cache_hit_rate DESC, tokens DESC`
	}
	row := d.db.QueryRow(`SELECT day,calls,tokens,cost_usd,
		CASE WHEN total_input > 0 THEN CAST(cache_read AS REAL) / CAST(total_input AS REAL) ELSE 0 END AS cache_hit_rate
		FROM (
			SELECT SUBSTR(timestamp,1,10) AS day, COUNT(*) AS calls,
				COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens),0) AS tokens,
				COALESCE(SUM(cost_usd),0) AS cost_usd,
				COALESCE(SUM(cache_read_input_tokens),0) AS cache_read,
				COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens),0) AS total_input
			FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter+`
			GROUP BY day
		) ORDER BY `+order+` LIMIT 1`, args...)
	var out WrappedDay
	if err := row.Scan(&out.Date, &out.Calls, &out.Tokens, &out.CostUSD, &out.CacheHitRate); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WrappedDay{}, false, nil
		}
		return WrappedDay{}, false, err
	}
	return out, true, nil
}

func wrappedHighlights(report *WrappedReport) []WrappedHighlight {
	if report == nil {
		return nil
	}
	highlights := []WrappedHighlight{
		{Label: "tokens", Value: fmt.Sprintf("%d", report.Stats.TotalTokens), Detail: fmt.Sprintf("%d calls, %.1f%% cache hit", report.Stats.TotalCalls, report.Stats.CacheHitRate*100)},
		{Label: "cost", Value: fmt.Sprintf("$%.4f", report.Stats.TotalCost), Detail: fmt.Sprintf("%d sessions, %d prompts", report.Stats.TotalSessions, report.Stats.TotalPrompts)},
	}
	if report.TopModel.Model != "" {
		highlights = append(highlights, WrappedHighlight{Label: "top model", Value: report.TopModel.Model, Detail: fmt.Sprintf("$%.4f", report.TopModel.Cost)})
	}
	if report.TopProject.Project != "" {
		highlights = append(highlights, WrappedHighlight{Label: "top project", Value: report.TopProject.Project, Detail: fmt.Sprintf("$%.4f, %d tokens", report.TopProject.CostUSD, report.TopProject.Tokens)})
	}
	if report.MostActiveDay.Date != "" {
		highlights = append(highlights, WrappedHighlight{Label: "most active day", Value: report.MostActiveDay.Date, Detail: fmt.Sprintf("%d tokens, $%.4f", report.MostActiveDay.Tokens, report.MostActiveDay.CostUSD)})
	}
	if report.BestCacheDay.Date != "" {
		highlights = append(highlights, WrappedHighlight{Label: "best cache day", Value: report.BestCacheDay.Date, Detail: fmt.Sprintf("%.1f%% cache hit", report.BestCacheDay.CacheHitRate*100)})
	}
	return highlights
}

// FormatWrappedMarkdown renders a privacy-safe Markdown report from a WrappedReport.
func FormatWrappedMarkdown(report *WrappedReport) string {
	if report == nil {
		return "# Agent Ledger Wrapped\n\nNo report.\n"
	}
	var b strings.Builder
	b.WriteString("# Agent Ledger Wrapped\n\n")
	b.WriteString(fmt.Sprintf("- Period: `%s`\n", report.Period))
	b.WriteString(fmt.Sprintf("- Window: `%s` to `%s`\n", report.From, report.To))
	b.WriteString(fmt.Sprintf("- Tokens: `%d`\n", report.Stats.TotalTokens))
	b.WriteString(fmt.Sprintf("- Cost: `$%.4f`\n", report.Stats.TotalCost))
	b.WriteString(fmt.Sprintf("- Sessions: `%d`\n", report.Stats.TotalSessions))
	b.WriteString(fmt.Sprintf("- Prompts: `%d`\n", report.Stats.TotalPrompts))
	b.WriteString(fmt.Sprintf("- Calls: `%d`\n", report.Stats.TotalCalls))
	b.WriteString(fmt.Sprintf("- Cache hit: `%.1f%%`\n\n", report.Stats.CacheHitRate*100))
	if len(report.Highlights) > 0 {
		b.WriteString("## Highlights\n\n| Signal | Value | Detail |\n|---|---:|---|\n")
		for _, h := range report.Highlights {
			b.WriteString(fmt.Sprintf("| %s | %s | %s |\n", h.Label, h.Value, h.Detail))
		}
		b.WriteString("\n")
	}
	if report.MostExpensiveSession.SessionID != "" {
		s := report.MostExpensiveSession
		b.WriteString("## Most Expensive Session\n\n")
		b.WriteString(fmt.Sprintf("- Source: `%s`\n- Session: `%s`\n- Project: `%s`\n- Cost: `$%.4f`\n- Tokens: `%d`\n- Main reason: `%s`\n\n",
			s.Source, s.SessionID, s.Project, s.CostUSD, s.Tokens, firstWrappedString(s.Reasons, "normal cost profile")))
	}
	if len(report.Issues) > 0 {
		b.WriteString("## Notes\n\n")
		for _, issue := range report.Issues {
			b.WriteString(fmt.Sprintf("- %s\n", issue))
		}
	}
	return b.String()
}

func normalizeWrappedPeriod(period string) string {
	period = strings.ToLower(strings.TrimSpace(period))
	switch period {
	case "week", "weekly":
		return "weekly"
	case "year", "yearly", "annual":
		return "yearly"
	case "month", "monthly", "":
		return "monthly"
	default:
		return period
	}
}

func firstWrappedString(values []string, fallback string) string {
	if len(values) == 0 || strings.TrimSpace(values[0]) == "" {
		return fallback
	}
	return values[0]
}
