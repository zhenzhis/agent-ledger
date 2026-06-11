package storage

import (
	"math"
	"sort"
	"strings"
	"time"
)

// PreflightEstimateValues are estimated workload metrics for a proposed task.
type PreflightEstimateValues struct {
	CostUSD         float64 `json:"cost_usd"`
	Tokens          int64   `json:"tokens"`
	Calls           int     `json:"calls"`
	Prompts         int     `json:"prompts"`
	DurationMinutes float64 `json:"duration_minutes"`
}

// PreflightEstimateReport estimates likely cost before starting an agent task.
type PreflightEstimateReport struct {
	GeneratedAt string                  `json:"generated_at"`
	From        string                  `json:"from"`
	To          string                  `json:"to"`
	Task        string                  `json:"task"`
	Source      string                  `json:"source,omitempty"`
	Model       string                  `json:"model,omitempty"`
	Project     string                  `json:"project,omitempty"`
	Method      string                  `json:"method"`
	Samples     int                     `json:"samples"`
	Confidence  string                  `json:"confidence"`
	Factor      float64                 `json:"factor"`
	Baseline    PreflightEstimateValues `json:"baseline"`
	Estimate    PreflightEstimateValues `json:"estimate"`
	P75         PreflightEstimateValues `json:"p75"`
	Issues      []string                `json:"issues,omitempty"`
}

type preflightSample struct {
	cost            float64
	tokens          int64
	calls           int
	prompts         int
	durationMinutes float64
}

// EstimatePreflightCost estimates task cost from local historical sessions.
// It uses metadata only: token/cost/call counts and timestamps, never prompt
// text or model output.
func (d *DB) EstimatePreflightCost(from, to time.Time, task, source, model, project string, limit int) (*PreflightEstimateReport, error) {
	from, to = utcRange(from, to)
	if limit <= 0 || limit > 5000 {
		limit = 2000
	}
	task = normalizePreflightTask(task)
	factor := preflightTaskFactor(task)
	samples, method, err := d.preflightUsageSamples(from, to, source, model, project, limit)
	if err != nil {
		return nil, err
	}
	if len(samples) == 0 {
		samples, method, err = d.preflightModelCallSamples(from, to, source, model, project, limit)
		if err != nil {
			return nil, err
		}
	}
	report := &PreflightEstimateReport{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		From:        from.UTC().Format(time.RFC3339),
		To:          to.UTC().Format(time.RFC3339),
		Task:        task,
		Source:      source,
		Model:       model,
		Project:     project,
		Method:      method,
		Samples:     len(samples),
		Confidence:  preflightConfidence(len(samples)),
		Factor:      factor,
	}
	if len(samples) == 0 {
		report.Issues = append(report.Issues, "no matching historical sessions; run more tasks or widen filters")
		return report, nil
	}
	report.Baseline = preflightPercentile(samples, 0.50)
	report.P75 = scalePreflightValues(preflightPercentile(samples, 0.75), factor)
	report.Estimate = scalePreflightValues(report.Baseline, factor)
	return report, nil
}

func (d *DB) preflightUsageSamples(from, to time.Time, source, model, project string, limit int) ([]preflightSample, string, error) {
	from, to = utcRange(from, to)
	filter, fa := buildUsageFilterAlias("u", source, model, project)
	args := append([]interface{}{from, to}, fa...)
	args = append(args, limit)
	rows, err := d.db.Query(`SELECT u.source,u.session_id,COUNT(*),
		COALESCE(SUM(u.input_tokens+u.cache_read_input_tokens+u.cache_creation_input_tokens+u.output_tokens),0),
		COALESCE(SUM(u.cost_usd),0), MIN(u.timestamp), MAX(u.timestamp), COALESCE(p.prompts,0)
		FROM usage_records u
		LEFT JOIN (SELECT source,session_id,COUNT(*) prompts FROM prompt_events GROUP BY source,session_id) p
			ON u.source=p.source AND u.session_id=p.session_id
		WHERE u.timestamp >= ? AND u.timestamp < ?`+filter+`
		GROUP BY u.source,u.session_id
		ORDER BY MAX(u.timestamp) DESC LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	samples, err := scanPreflightSamples(rows)
	return samples, "local-usage-session-median-with-task-multiplier", err
}

func (d *DB) preflightModelCallSamples(from, to time.Time, source, model, project string, limit int) ([]preflightSample, string, error) {
	from, to = utcRange(from, to)
	filter, fa := buildModelCallPreflightFilter(source, model, project)
	args := append([]interface{}{from, to}, fa...)
	args = append(args, limit)
	rows, err := d.db.Query(`SELECT mc.source,
		COALESCE(NULLIF(mc.session_id,''), NULLIF(mc.workload_id,''), NULLIF(mc.run_id,''), mc.call_id) AS group_key,
		COUNT(*),
		COALESCE(SUM(mc.input_tokens+mc.cache_read_input_tokens+mc.cache_creation_input_tokens+mc.output_tokens),0),
		COALESCE(SUM(mc.cost_usd),0), MIN(mc.timestamp), MAX(mc.timestamp), 0
		FROM model_calls mc
		LEFT JOIN workloads w ON mc.workload_id=w.workload_id
		WHERE mc.timestamp >= ? AND mc.timestamp < ?`+filter+`
		GROUP BY mc.source, group_key
		ORDER BY MAX(mc.timestamp) DESC LIMIT ?`, args...)
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()
	samples, err := scanPreflightSamples(rows)
	return samples, "canonical-model-call-median-with-task-multiplier", err
}

type preflightScanner interface {
	Next() bool
	Scan(dest ...interface{}) error
	Err() error
}

func scanPreflightSamples(rows preflightScanner) ([]preflightSample, error) {
	var samples []preflightSample
	for rows.Next() {
		var sample preflightSample
		var ignoredSource, ignoredSession, first, last string
		if err := rows.Scan(&ignoredSource, &ignoredSession, &sample.calls, &sample.tokens, &sample.cost, &first, &last, &sample.prompts); err != nil {
			return nil, err
		}
		if start, ok1 := parseDBTime(first); ok1 {
			if end, ok2 := parseDBTime(last); ok2 && end.After(start) {
				sample.durationMinutes = end.Sub(start).Minutes()
			}
		}
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return samples, nil
}

func buildModelCallPreflightFilter(source, model, project string) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	if source != "" {
		clauses = append(clauses, "mc.source=?")
		args = append(args, source)
	}
	if model != "" {
		clauses = append(clauses, "mc.model=?")
		args = append(args, model)
	}
	if project != "" {
		clauses = append(clauses, "w.project=?")
		args = append(args, project)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func normalizePreflightTask(task string) string {
	task = strings.ToLower(strings.TrimSpace(task))
	task = strings.ReplaceAll(task, "_", "-")
	if task == "" {
		return "custom"
	}
	switch task {
	case "small", "small-change", "change", "edit":
		return "small-change"
	case "doc", "docs", "documentation":
		return "documentation"
	case "review", "pr", "pr-review":
		return "pr-review"
	case "repo", "full-repo", "fullrepo", "repository":
		return "full-repo"
	default:
		return task
	}
}

func preflightTaskFactor(task string) float64 {
	switch task {
	case "small-change":
		return 0.35
	case "documentation":
		return 0.55
	case "debug":
		return 0.75
	case "pr-review":
		return 0.80
	case "refactor":
		return 1.25
	case "full-repo", "rewrite", "research":
		return 2.00
	default:
		return 1.00
	}
}

func preflightConfidence(samples int) string {
	switch {
	case samples >= 30:
		return "high"
	case samples >= 8:
		return "medium"
	case samples > 0:
		return "low"
	default:
		return "none"
	}
}

func preflightPercentile(samples []preflightSample, percentile float64) PreflightEstimateValues {
	if len(samples) == 0 {
		return PreflightEstimateValues{}
	}
	costs := make([]float64, len(samples))
	tokens := make([]float64, len(samples))
	calls := make([]float64, len(samples))
	prompts := make([]float64, len(samples))
	durations := make([]float64, len(samples))
	for i, sample := range samples {
		costs[i] = sample.cost
		tokens[i] = float64(sample.tokens)
		calls[i] = float64(sample.calls)
		prompts[i] = float64(sample.prompts)
		durations[i] = sample.durationMinutes
	}
	return PreflightEstimateValues{
		CostUSD:         roundPreflightCost(percentileFloat(costs, percentile)),
		Tokens:          int64(math.Round(percentileFloat(tokens, percentile))),
		Calls:           int(math.Round(percentileFloat(calls, percentile))),
		Prompts:         int(math.Round(percentileFloat(prompts, percentile))),
		DurationMinutes: roundPreflightMinutes(percentileFloat(durations, percentile)),
	}
}

func percentileFloat(values []float64, percentile float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sort.Float64s(values)
	if len(values) == 1 {
		return values[0]
	}
	pos := percentile * float64(len(values)-1)
	lower := int(math.Floor(pos))
	upper := int(math.Ceil(pos))
	if lower == upper {
		return values[lower]
	}
	weight := pos - float64(lower)
	return values[lower]*(1-weight) + values[upper]*weight
}

func scalePreflightValues(v PreflightEstimateValues, factor float64) PreflightEstimateValues {
	return PreflightEstimateValues{
		CostUSD:         roundPreflightCost(v.CostUSD * factor),
		Tokens:          int64(math.Round(float64(v.Tokens) * factor)),
		Calls:           int(math.Round(float64(v.Calls) * factor)),
		Prompts:         int(math.Round(float64(v.Prompts) * factor)),
		DurationMinutes: roundPreflightMinutes(v.DurationMinutes * factor),
	}
}

func roundPreflightCost(v float64) float64 {
	return math.Round(v*1_000_000) / 1_000_000
}

func roundPreflightMinutes(v float64) float64 {
	return math.Round(v*100) / 100
}
