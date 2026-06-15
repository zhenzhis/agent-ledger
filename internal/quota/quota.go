package quota

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

const MethodLocalEstimate = "local-estimate"

var ErrInvalidRequest = errors.New("invalid quota request")

// Filter narrows quota consumption reads without changing the configured
// quota limits. Limits remain local estimates, not provider billing facts.
type Filter struct {
	Window  string
	Source  string
	Model   string
	Project string
}

// StatsProvider supplies usage totals for one quota window.
type StatsProvider func(from, to time.Time, source, model, project string) (*storage.DashboardStats, error)

type Status struct {
	Enabled  bool     `json:"enabled"`
	Plan     string   `json:"plan"`
	ResetDay int      `json:"reset_day"`
	Windows  []Window `json:"windows"`
	Method   string   `json:"method"`
}

type Window struct {
	Name                  string  `json:"name"`
	From                  string  `json:"from"`
	To                    string  `json:"to"`
	CostUSD               float64 `json:"cost_usd"`
	Tokens                int64   `json:"tokens"`
	Prompts               int     `json:"prompts"`
	Calls                 int     `json:"calls,omitempty"`
	CostLimit             float64 `json:"cost_limit"`
	TokenLimit            int64   `json:"token_limit"`
	PromptLimit           int64   `json:"prompt_limit,omitempty"`
	RemainingCost         float64 `json:"remaining_cost"`
	RemainingTokens       int64   `json:"remaining_tokens"`
	RemainingPrompts      int64   `json:"remaining_prompts,omitempty"`
	BurnRatePerHour       float64 `json:"burn_rate_per_hour"`
	TokenBurnRatePerHour  float64 `json:"token_burn_rate_per_hour,omitempty"`
	PromptBurnRatePerHour float64 `json:"prompt_burn_rate_per_hour,omitempty"`
	ProjectedCostUSD      float64 `json:"projected_cost_usd"`
	ProjectedTokens       int64   `json:"projected_tokens"`
	ProjectedPrompts      int64   `json:"projected_prompts,omitempty"`
	ResetAt               string  `json:"reset_at"`
	TimeToLimitHours      float64 `json:"time_to_limit_hours"`
}

type windowSpec struct {
	name        string
	from        time.Time
	to          time.Time
	costLimit   float64
	tokenLimit  int64
	promptLimit int64
	resetAt     string
}

// BuildStatus calculates the local Agent Battery quota report used by REST,
// CLI, and MCP surfaces. It never calls provider APIs and never inspects prompt
// or response content.
func BuildStatus(now time.Time, cfg config.QuotaConfig, filter Filter, stats StatsProvider) (*Status, error) {
	if stats == nil {
		return nil, fmt.Errorf("%w: stats provider is required", ErrInvalidRequest)
	}
	resetDay := NormalizeResetDay(cfg.ResetDay)
	specs, err := WindowSpecs(now, cfg, filter.Window)
	if err != nil {
		return nil, err
	}
	out := Status{
		Enabled:  cfg.Enabled,
		Plan:     firstNonEmpty(cfg.Plan, "custom"),
		ResetDay: resetDay,
		Method:   MethodLocalEstimate,
	}
	for _, spec := range specs {
		row, err := stats(spec.from, spec.to, filter.Source, filter.Model, filter.Project)
		if err != nil {
			return nil, err
		}
		out.Windows = append(out.Windows, BuildWindow(now, spec, row))
	}
	return &out, nil
}

func BuildWindow(now time.Time, spec windowSpec, stats *storage.DashboardStats) Window {
	if stats == nil {
		stats = &storage.DashboardStats{}
	}
	elapsedHours := math.Max(1, minTime(now, spec.to).Sub(spec.from).Hours())
	windowHours := math.Max(1, spec.to.Sub(spec.from).Hours())
	costBurn := stats.TotalCost / elapsedHours
	tokenBurn := float64(stats.TotalTokens) / elapsedHours
	promptBurn := float64(stats.TotalPrompts) / elapsedHours
	return Window{
		Name:                  spec.name,
		From:                  spec.from.Format(time.RFC3339),
		To:                    spec.to.Format(time.RFC3339),
		CostUSD:               stats.TotalCost,
		Tokens:                stats.TotalTokens,
		Prompts:               stats.TotalPrompts,
		Calls:                 stats.TotalCalls,
		CostLimit:             spec.costLimit,
		TokenLimit:            spec.tokenLimit,
		PromptLimit:           spec.promptLimit,
		RemainingCost:         spec.costLimit - stats.TotalCost,
		RemainingTokens:       spec.tokenLimit - stats.TotalTokens,
		RemainingPrompts:      spec.promptLimit - int64(stats.TotalPrompts),
		BurnRatePerHour:       costBurn,
		TokenBurnRatePerHour:  tokenBurn,
		PromptBurnRatePerHour: promptBurn,
		ProjectedCostUSD:      costBurn * windowHours,
		ProjectedTokens:       int64(tokenBurn * windowHours),
		ProjectedPrompts:      int64(promptBurn * windowHours),
		ResetAt:               spec.resetAt,
		TimeToLimitHours:      timeToLimitHours(stats, spec, costBurn, tokenBurn, promptBurn),
	}
}

func WindowSpecs(now time.Time, cfg config.QuotaConfig, window string) ([]windowSpec, error) {
	resetDay := NormalizeResetDay(cfg.ResetDay)
	monthFrom, monthTo := MonthWindow(now, resetDay)
	base := []windowSpec{}
	if cfg.Window5H {
		base = append(base, windowSpec{
			name:        "5h",
			from:        now.Add(-5 * time.Hour),
			to:          now,
			costLimit:   cfg.MonthlyBudget / 30 / 24 * 5,
			tokenLimit:  scaleInt64(cfg.TokenBudget, 5, 30*24),
			promptLimit: scaleInt64(cfg.PromptBudget, 5, 30*24),
		})
	}
	dayFrom, dayTo := DayWindow(now)
	weekFrom, weekTo := WeekWindow(now)
	base = append(base,
		windowSpec{name: "day", from: dayFrom, to: dayTo, costLimit: cfg.MonthlyBudget / 30, tokenLimit: scaleInt64(cfg.TokenBudget, 1, 30), promptLimit: scaleInt64(cfg.PromptBudget, 1, 30), resetAt: dayTo.Format(time.RFC3339)},
		windowSpec{name: "week", from: weekFrom, to: weekTo, costLimit: cfg.MonthlyBudget / 4.35, tokenLimit: scaleInt64(cfg.TokenBudget, 1, 4), promptLimit: scaleInt64(cfg.PromptBudget, 1, 4), resetAt: weekTo.Format(time.RFC3339)},
		windowSpec{name: "month", from: monthFrom, to: monthTo, costLimit: cfg.MonthlyBudget, tokenLimit: cfg.TokenBudget, promptLimit: cfg.PromptBudget, resetAt: monthTo.Format(time.RFC3339)},
	)
	for i, custom := range cfg.CustomWindows {
		name := strings.TrimSpace(custom.Name)
		if name == "" {
			name = fmt.Sprintf("custom-%d", i+1)
		}
		duration, err := time.ParseDuration(custom.Duration)
		if err != nil || duration <= 0 {
			return nil, fmt.Errorf("%w: custom quota window %q has invalid duration %q", ErrInvalidRequest, name, custom.Duration)
		}
		base = append(base, windowSpec{
			name:        name,
			from:        now.Add(-duration),
			to:          now,
			costLimit:   custom.CostLimit,
			tokenLimit:  custom.TokenLimit,
			promptLimit: custom.PromptLimit,
		})
	}
	if strings.TrimSpace(window) == "" || strings.EqualFold(window, "all") {
		return base, nil
	}
	want := normalizeWindowName(window)
	for _, spec := range base {
		if normalizeWindowName(spec.name) == want {
			return []windowSpec{spec}, nil
		}
	}
	return nil, fmt.Errorf("%w: unknown quota window %q", ErrInvalidRequest, window)
}

func DayWindow(now time.Time) (time.Time, time.Time) {
	y, m, d := now.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	return start, start.AddDate(0, 0, 1)
}

func WeekWindow(now time.Time) (time.Time, time.Time) {
	y, m, d := now.Date()
	start := time.Date(y, m, d, 0, 0, 0, 0, now.Location())
	start = start.AddDate(0, 0, -int((start.Weekday()+6)%7))
	return start, start.AddDate(0, 0, 7)
}

func MonthWindow(now time.Time, resetDay int) (time.Time, time.Time) {
	resetDay = NormalizeResetDay(resetDay)
	y, m, _ := now.Date()
	loc := now.Location()
	start := time.Date(y, m, resetDay, 0, 0, 0, 0, loc)
	if now.Before(start) {
		start = start.AddDate(0, -1, 0)
	}
	return start, start.AddDate(0, 1, 0)
}

func NormalizeResetDay(day int) int {
	if day < 1 {
		return 1
	}
	if day > 28 {
		return 28
	}
	return day
}

func timeToLimitHours(stats *storage.DashboardStats, spec windowSpec, costBurn, tokenBurn, promptBurn float64) float64 {
	candidates := []float64{}
	if spec.costLimit > 0 && costBurn > 0 {
		candidates = append(candidates, (spec.costLimit-stats.TotalCost)/costBurn)
	}
	if spec.tokenLimit > 0 && tokenBurn > 0 {
		candidates = append(candidates, float64(spec.tokenLimit-stats.TotalTokens)/tokenBurn)
	}
	if spec.promptLimit > 0 && promptBurn > 0 {
		candidates = append(candidates, float64(spec.promptLimit-int64(stats.TotalPrompts))/promptBurn)
	}
	if len(candidates) == 0 {
		return -1
	}
	best := candidates[0]
	for _, value := range candidates[1:] {
		if value < best {
			best = value
		}
	}
	return best
}

func FormatText(status *Status) string {
	if status == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Agent Ledger battery: plan=%s enabled=%t method=%s reset_day=%d\n", status.Plan, status.Enabled, status.Method, status.ResetDay)
	for _, row := range status.Windows {
		eta := "n/a"
		if row.TimeToLimitHours >= 0 {
			eta = fmt.Sprintf("%.1fh", row.TimeToLimitHours)
		}
		reset := ""
		if row.ResetAt != "" {
			reset = " reset=" + row.ResetAt
		}
		fmt.Fprintf(&b, "%s\tcost=$%.4f/$%.4f\tremaining=$%.4f\ttokens=%d/%d\tprompts=%d/%d\tburn=$%.4f/h\teta=%s%s\n",
			row.Name, row.CostUSD, row.CostLimit, row.RemainingCost, row.Tokens, row.TokenLimit, row.Prompts, row.PromptLimit, row.BurnRatePerHour, eta, reset)
	}
	return b.String()
}

func FormatMarkdown(status *Status) string {
	if status == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent Ledger Battery\n\n")
	fmt.Fprintf(&b, "- Plan: `%s`\n", status.Plan)
	fmt.Fprintf(&b, "- Enabled: `%t`\n", status.Enabled)
	fmt.Fprintf(&b, "- Method: `%s`\n", status.Method)
	fmt.Fprintf(&b, "- Reset day: `%d`\n\n", status.ResetDay)
	fmt.Fprintf(&b, "| Window | Cost | Remaining | Tokens | Prompts | Burn | ETA | Reset |\n")
	fmt.Fprintf(&b, "|---|---:|---:|---:|---:|---:|---:|---|\n")
	for _, row := range status.Windows {
		eta := "n/a"
		if row.TimeToLimitHours >= 0 {
			eta = fmt.Sprintf("%.1fh", row.TimeToLimitHours)
		}
		fmt.Fprintf(&b, "| %s | $%.4f / $%.4f | $%.4f | %d / %d | %d / %d | $%.4f/h | %s | %s |\n",
			row.Name, row.CostUSD, row.CostLimit, row.RemainingCost, row.Tokens, row.TokenLimit, row.Prompts, row.PromptLimit, row.BurnRatePerHour, eta, row.ResetAt)
	}
	return b.String()
}

func minTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

func scaleInt64(value int64, numerator, denominator int64) int64 {
	if value == 0 || denominator == 0 {
		return 0
	}
	return value * numerator / denominator
}

func normalizeWindowName(value string) string {
	return strings.ToLower(strings.TrimSpace(strings.ReplaceAll(value, "_", "-")))
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
