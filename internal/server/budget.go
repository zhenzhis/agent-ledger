package server

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// BudgetStatus describes current consumption against one budget rule.
type BudgetStatus struct {
	Name      string  `json:"name"`
	Period    string  `json:"period"`
	Scope     string  `json:"scope"`
	Match     string  `json:"match"`
	Metric    string  `json:"metric"`
	Value     float64 `json:"value"`
	Limit     float64 `json:"limit"`
	Ratio     float64 `json:"ratio"`
	Severity  string  `json:"severity"`
	Message   string  `json:"message"`
	PeriodKey string  `json:"period_key"`
}

func (s *Server) handleBudgetStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	status, err := s.evaluateBudgets(time.Now())
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSONWithPayloadETag(w, r, map[string]interface{}{
		"enabled": s.options.Budgets.Enabled,
		"rules":   status,
	})
}

func (s *Server) evaluateBudgets(now time.Time) ([]BudgetStatus, error) {
	if !s.options.Budgets.Enabled {
		return []BudgetStatus{}, nil
	}
	var result []BudgetStatus
	for _, rule := range s.options.Budgets.Rules {
		if strings.TrimSpace(rule.Name) == "" || rule.Limit <= 0 {
			continue
		}
		from, to, periodKey := budgetWindow(now, rule.Period)
		source, model, project := "", "", ""
		switch strings.ToLower(rule.Scope) {
		case "", "global":
		case "source":
			source = rule.Match
		case "model":
			model = rule.Match
		case "project":
			project = rule.Match
		default:
			continue
		}
		stats, err := s.db.GetDashboardStatsFiltered(from, to, source, model, project)
		if err != nil {
			return nil, err
		}
		value := budgetMetricValue(stats, rule.Metric)
		ratio := value / rule.Limit
		warnRatio := rule.WarnRatio
		if warnRatio <= 0 || warnRatio >= 1 {
			warnRatio = 0.8
		}
		severity := "ok"
		if ratio >= 1 {
			severity = "critical"
		} else if ratio >= warnRatio {
			severity = "warning"
		}
		status := BudgetStatus{
			Name:      rule.Name,
			Period:    normalizedPeriod(rule.Period),
			Scope:     normalizedScope(rule.Scope),
			Match:     rule.Match,
			Metric:    normalizedMetric(rule.Metric),
			Value:     value,
			Limit:     rule.Limit,
			Ratio:     ratio,
			Severity:  severity,
			Message:   fmt.Sprintf("%s %.2f / %.2f", rule.Name, value, rule.Limit),
			PeriodKey: periodKey,
		}
		result = append(result, status)
		if severity != "ok" && s.canWriteDerivedData() {
			_ = s.db.UpsertBudgetEvent(storage.BudgetEvent{
				EventKey:  strings.Join([]string{status.Name, status.PeriodKey, status.Scope, status.Match, status.Metric}, "|"),
				RuleName:  status.Name,
				Period:    status.Period,
				Scope:     status.Scope,
				Match:     status.Match,
				Metric:    status.Metric,
				Value:     status.Value,
				Limit:     status.Limit,
				Severity:  status.Severity,
				Message:   status.Message,
				CreatedAt: now,
			})
		}
	}
	return result, nil
}

func budgetWindow(now time.Time, period string) (time.Time, time.Time, string) {
	y, m, d := now.Date()
	loc := now.Location()
	switch normalizedPeriod(period) {
	case "week":
		start := time.Date(y, m, d, 0, 0, 0, 0, loc)
		start = start.AddDate(0, 0, -int((start.Weekday()+6)%7))
		return start, start.AddDate(0, 0, 7), start.Format("2006-01-02")
	case "month":
		start := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 1, 0), start.Format("2006-01")
	default:
		start := time.Date(y, m, d, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 0, 1), start.Format("2006-01-02")
	}
}

func budgetMetricValue(stats *storage.DashboardStats, metric string) float64 {
	if stats == nil {
		return 0
	}
	switch normalizedMetric(metric) {
	case "tokens":
		return float64(stats.TotalTokens)
	case "prompts":
		return float64(stats.TotalPrompts)
	default:
		return stats.TotalCost
	}
}

func normalizedMetric(metric string) string {
	switch strings.ToLower(metric) {
	case "tokens", "token":
		return "tokens"
	case "prompts", "prompt":
		return "prompts"
	default:
		return "cost_usd"
	}
}

func normalizedPeriod(period string) string {
	switch strings.ToLower(period) {
	case "week", "weekly":
		return "week"
	case "month", "monthly":
		return "month"
	default:
		return "day"
	}
}

func normalizedScope(scope string) string {
	switch strings.ToLower(scope) {
	case "source", "model", "project":
		return strings.ToLower(scope)
	default:
		return "global"
	}
}

func defaultBudgetRule(name string) config.BudgetRule {
	return config.BudgetRule{Name: name, Period: "day", Scope: "global", Metric: "cost_usd", Limit: 0, WarnRatio: 0.8}
}
