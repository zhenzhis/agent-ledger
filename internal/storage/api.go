package storage

import (
	"fmt"
	"strings"
	"time"
)

// sourceFilter returns a SQL clause and args for optional source filtering.
func sourceFilter(source string) (string, []interface{}) {
	if source == "" {
		return "", nil
	}
	return " AND source=?", []interface{}{source}
}

// modelFilter returns a SQL clause and args for optional model filtering.
func modelFilter(model string) (string, []interface{}) {
	if model == "" {
		return "", nil
	}
	return " AND model=?", []interface{}{model}
}

// projectFilter returns a SQL clause and args for optional project filtering.
func projectFilter(project string) (string, []interface{}) {
	if project == "" {
		return "", nil
	}
	return " AND project=?", []interface{}{project}
}

func buildUsageFilter(source, model, project string) (string, []interface{}) {
	sf, sa := sourceFilter(source)
	mf, ma := modelFilter(model)
	pf, pa := projectFilter(project)
	args := append([]interface{}{}, sa...)
	args = append(args, ma...)
	args = append(args, pa...)
	return sf + mf + pf, args
}

func escapeLike(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	value = strings.ReplaceAll(value, `%`, `\%`)
	value = strings.ReplaceAll(value, `_`, `\_`)
	return value
}

func sessionSearchFilter(query string) (string, []interface{}) {
	query = strings.TrimSpace(query)
	if query == "" {
		return "", nil
	}
	if runes := []rune(query); len(runes) > 200 {
		query = string(runes[:200])
	}
	pattern := "%" + escapeLike(query) + "%"
	return ` WHERE (s.project LIKE ? ESCAPE '\' OR s.cwd LIKE ? ESCAPE '\' OR s.git_branch LIKE ? ESCAPE '\')`,
		[]interface{}{pattern, pattern, pattern}
}

func sessionOrderBy(sortKey, direction string) string {
	dir := "DESC"
	if strings.EqualFold(direction, "asc") {
		dir = "ASC"
	}
	columns := map[string]string{
		"source":        "s.source",
		"project":       "s.project",
		"git_branch":    "s.git_branch",
		"start_time":    "s.start_time",
		"last_activity": "u.last_activity",
		"prompts":       "COALESCE(p.prompts,0)",
		"tokens":        "u.tokens",
		"total_cost":    "u.cost",
	}
	column, ok := columns[sortKey]
	if !ok {
		column = "u.last_activity"
	}
	return fmt.Sprintf(" ORDER BY %s %s, u.last_activity DESC, s.start_time DESC", column, dir)
}

// DashboardStats holds aggregate statistics for the dashboard summary cards.
type DashboardStats struct {
	TotalCost     float64 `json:"total_cost"`
	TotalTokens   int64   `json:"total_tokens"`
	TotalSessions int     `json:"total_sessions"`
	TotalPrompts  int     `json:"total_prompts"`
	TotalCalls    int     `json:"total_calls"`
	CacheHitRate  float64 `json:"cache_hit_rate"`
}

// DashboardConsistencyIssue explains a mismatch between dashboard modules.
type DashboardConsistencyIssue struct {
	Metric   string  `json:"metric"`
	Expected float64 `json:"expected"`
	Actual   float64 `json:"actual"`
	Delta    float64 `json:"delta"`
	Severity string  `json:"severity"`
	Message  string  `json:"message"`
}

// RuntimeStatus describes process-level operation mode for UI, doctor, and
// local agent discovery without exposing secrets or local paths.
type RuntimeStatus struct {
	Contract              string   `json:"contract,omitempty"`
	Version               string   `json:"version,omitempty"`
	Mode                  string   `json:"mode"`
	ReadOnly              bool     `json:"read_only"`
	WriteOperations       string   `json:"write_operations"`
	BackgroundTasks       string   `json:"background_tasks"`
	CapabilityCatalogHash string   `json:"capability_catalog_hash,omitempty"`
	CanonicalSchemaHash   string   `json:"canonical_schema_hash,omitempty"`
	AdapterSpecHash       string   `json:"adapter_spec_hash,omitempty"`
	DisabledFeatures      []string `json:"disabled_features,omitempty"`
	Message               string   `json:"message"`
}

// DashboardBundle returns the core dashboard modules from one consistent read window.
type DashboardBundle struct {
	GeneratedAt    string                      `json:"generated_at"`
	From           string                      `json:"from"`
	To             string                      `json:"to"`
	Granularity    string                      `json:"granularity"`
	Source         string                      `json:"source,omitempty"`
	Model          string                      `json:"model,omitempty"`
	Project        string                      `json:"project,omitempty"`
	Stats          *DashboardStats             `json:"stats"`
	CostByModel    []CostByModel               `json:"cost_by_model"`
	CostOverTime   []TimeSeriesPoint           `json:"cost_over_time"`
	TokensOverTime []TokenTimeSeriesPoint      `json:"tokens_over_time"`
	Consistency    []DashboardConsistencyIssue `json:"consistency"`
	Runtime        *RuntimeStatus              `json:"runtime,omitempty"`
}

// CostByModel represents total cost for a single model.
type CostByModel struct {
	Model string  `json:"model"`
	Cost  float64 `json:"cost"`
}

// TimeSeriesPoint represents a single data point in a daily cost time series.
type TimeSeriesPoint struct {
	Date  string  `json:"date"`
	Value float64 `json:"value"`
	Model string  `json:"model,omitempty"`
}

// TokenTimeSeriesPoint represents daily token usage broken down by category.
type TokenTimeSeriesPoint struct {
	Date         string `json:"date"`
	InputTokens  int64  `json:"input_tokens"`
	OutputTokens int64  `json:"output_tokens"`
	CacheRead    int64  `json:"cache_read"`
	CacheCreate  int64  `json:"cache_create"`
}

// SessionInfo represents a session with aggregated cost and token totals.
type SessionInfo struct {
	SessionID    string  `json:"session_id"`
	Source       string  `json:"source"`
	Project      string  `json:"project"`
	CWD          string  `json:"cwd"`
	GitBranch    string  `json:"git_branch"`
	StartTime    string  `json:"start_time"`
	LastActivity string  `json:"last_activity"`
	Prompts      int     `json:"prompts"`
	TotalCost    float64 `json:"total_cost"`
	Tokens       int64   `json:"tokens"`
}

// SessionPage represents a paginated session ledger response.
type SessionPage struct {
	Rows       []SessionInfo `json:"rows"`
	Total      int           `json:"total"`
	Limit      int           `json:"limit"`
	Offset     int           `json:"offset"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// GetDashboardStats returns aggregate cost, token, session, and prompt counts
// for usage records within the given time range.
func (d *DB) GetDashboardStats(from, to time.Time, source, model string) (*DashboardStats, error) {
	return d.GetDashboardStatsFiltered(from, to, source, model, "")
}

// GetDashboardStatsFiltered returns aggregate stats with optional project filtering.
func (d *DB) GetDashboardStatsFiltered(from, to time.Time, source, model, project string) (*DashboardStats, error) {
	return d.getDashboardStatsFiltered(from, to, source, model, project, true)
}

func (d *DB) getDashboardStatsFiltered(from, to time.Time, source, model, project string, useAggregate bool) (*DashboardStats, error) {
	from, to = utcRange(from, to)
	s := &DashboardStats{}
	filter, fa := buildUsageFilter(source, model, project)
	args := append([]interface{}{from, to}, fa...)
	var cacheRead, totalInput int64
	usedAggregate := false
	if useAggregate && to.Sub(from) >= 24*time.Hour && isUTCDayAligned(from) && isUTCDayAligned(to) {
		aggFilter, aggArgs := buildUsageFilter(source, model, project)
		aggArgs = append([]interface{}{from.Format("2006-01-02"), to.Format("2006-01-02")}, aggArgs...)
		var rowsSeen int
		err := d.db.QueryRow(`SELECT COALESCE(SUM(cost_usd),0),
			COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens),0),
			COALESCE(SUM(cache_read_input_tokens),0),
			COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens),0),
			COALESCE(SUM(calls),0)
			FROM daily_usage_aggregate WHERE bucket >= ? AND bucket < ?`+aggFilter, aggArgs...).Scan(&s.TotalCost, &s.TotalTokens, &cacheRead, &totalInput, &s.TotalCalls)
		if err == nil {
			_ = d.db.QueryRow(`SELECT COUNT(*) FROM daily_usage_aggregate WHERE bucket >= ? AND bucket < ?`+aggFilter, aggArgs...).Scan(&rowsSeen)
			usedAggregate = rowsSeen > 0
		}
	}
	if !usedAggregate {
		err := d.db.QueryRow(`SELECT COALESCE(SUM(cost_usd),0),
			COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens),0),
			COALESCE(SUM(cache_read_input_tokens),0),
			COALESCE(SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens),0)
			FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter, args...).Scan(&s.TotalCost, &s.TotalTokens, &cacheRead, &totalInput)
		if err != nil {
			return nil, err
		}
		d.db.QueryRow(`SELECT COUNT(*) FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter, args...).Scan(&s.TotalCalls)
	}
	if totalInput > 0 {
		s.CacheHitRate = float64(cacheRead) / float64(totalInput)
	}
	d.db.QueryRow(`SELECT COUNT(*) FROM (SELECT source,session_id FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter+` GROUP BY source,session_id)`, args...).Scan(&s.TotalSessions)
	pf, pa := buildUsageFilter(source, "", project)
	d.db.QueryRow(`SELECT COUNT(*) FROM prompt_events WHERE timestamp >= ? AND timestamp < ?`+pf, append([]interface{}{from, to}, pa...)...).Scan(&s.TotalPrompts)
	return s, nil
}

func isUTCDayAligned(t time.Time) bool {
	t = t.UTC()
	return t.Hour() == 0 && t.Minute() == 0 && t.Second() == 0 && t.Nanosecond() == 0
}

// GetDashboardBundleFiltered reads KPI and core chart data while holding the
// storage mutex, so concurrent scans or cost rebuilds cannot make the visible
// dashboard modules disagree with each other.
func (d *DB) GetDashboardBundleFiltered(from, to time.Time, granularity, source, model, project string, tzOffset int) (*DashboardBundle, error) {
	from, to = utcRange(from, to)
	d.mu.Lock()
	defer d.mu.Unlock()

	stats, err := d.getDashboardStatsFiltered(from, to, source, model, project, false)
	if err != nil {
		return nil, err
	}
	costByModel, err := d.GetCostByModelFiltered(from, to, source, project)
	if err != nil {
		return nil, err
	}
	costOverTime, err := d.GetCostOverTimeFiltered(from, to, granularity, source, model, project, tzOffset)
	if err != nil {
		return nil, err
	}
	tokensOverTime, err := d.GetTokensOverTimeFiltered(from, to, granularity, source, model, project, tzOffset)
	if err != nil {
		return nil, err
	}
	bundle := &DashboardBundle{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		From:           from.Format(time.RFC3339),
		To:             to.Format(time.RFC3339),
		Granularity:    granularity,
		Source:         source,
		Model:          model,
		Project:        project,
		Stats:          stats,
		CostByModel:    costByModel,
		CostOverTime:   costOverTime,
		TokensOverTime: tokensOverTime,
	}
	bundle.Consistency = dashboardConsistency(stats, costByModel, costOverTime, tokensOverTime)
	return bundle, nil
}

func dashboardConsistency(stats *DashboardStats, costByModel []CostByModel, costOverTime []TimeSeriesPoint, tokensOverTime []TokenTimeSeriesPoint) []DashboardConsistencyIssue {
	if stats == nil {
		return []DashboardConsistencyIssue{{
			Metric: "stats", Severity: "critical",
			Message: "dashboard stats are missing",
		}}
	}
	var tokenTotal int64
	for _, row := range tokensOverTime {
		tokenTotal += row.InputTokens + row.OutputTokens + row.CacheRead + row.CacheCreate
	}
	var costTimeTotal float64
	for _, row := range costOverTime {
		costTimeTotal += row.Value
	}
	var costModelTotal float64
	for _, row := range costByModel {
		costModelTotal += row.Cost
	}
	var issues []DashboardConsistencyIssue
	if delta := float64(tokenTotal - stats.TotalTokens); delta != 0 {
		issues = append(issues, DashboardConsistencyIssue{
			Metric: "tokens", Expected: float64(stats.TotalTokens), Actual: float64(tokenTotal), Delta: delta, Severity: "warning",
			Message: "token chart total does not match KPI total; rebuild aggregates or rerun scan if this persists",
		})
	}
	if issue, ok := costConsistencyIssue("cost_over_time", stats.TotalCost, costTimeTotal); ok {
		issues = append(issues, issue)
	}
	if issue, ok := costConsistencyIssue("cost_by_model", stats.TotalCost, costModelTotal); ok {
		issues = append(issues, issue)
	}
	return issues
}

func costConsistencyIssue(metric string, expected, actual float64) (DashboardConsistencyIssue, bool) {
	delta := actual - expected
	if delta < 0 {
		delta = -delta
	}
	tolerance := 0.000001
	if expected > 1 {
		tolerance = expected * 0.000001
	}
	if delta <= tolerance {
		return DashboardConsistencyIssue{}, false
	}
	return DashboardConsistencyIssue{
		Metric: metric, Expected: expected, Actual: actual, Delta: actual - expected, Severity: "warning",
		Message: "cost chart total does not match KPI total; rebuild costs or aggregates if this persists",
	}, true
}

// GetCostByModel returns total cost grouped by model within the given time range.
func (d *DB) GetCostByModel(from, to time.Time, source string) ([]CostByModel, error) {
	return d.GetCostByModelFiltered(from, to, source, "")
}

// GetCostByModelFiltered returns total cost grouped by model with optional project filtering.
func (d *DB) GetCostByModelFiltered(from, to time.Time, source, project string) ([]CostByModel, error) {
	from, to = utcRange(from, to)
	sf, sa := buildUsageFilter(source, "", project)
	args := append([]interface{}{from, to}, sa...)
	rows, err := d.db.Query(`SELECT model, SUM(cost_usd) as cost FROM usage_records
		WHERE timestamp >= ? AND timestamp < ?`+sf+` GROUP BY model ORDER BY cost DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []CostByModel
	for rows.Next() {
		var r CostByModel
		if err := rows.Scan(&r.Model, &r.Cost); err != nil {
			return nil, err
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// granularityExpr returns a SQL expression that truncates a timestamp column
// to the bucket boundary for the given granularity code.
// Timestamps are stored as Go time strings like "2026-04-03 09:51:45.996 +0000 UTC",
// so we use SUBSTR-based extraction since SQLite's STRFTIME cannot parse this format.
// Supported codes: 1m, 30m, 1h, 6h, 12h, 1d, 1w, 1M.
func granularityExpr(g string, tzOffset int) string {
	// Base timestamp expression: either raw or shifted to user's local time.
	// tzOffset uses JS getTimezoneOffset() convention (UTC-local in minutes).
	// To convert UTC→local we apply -tzOffset minutes.
	ts := "timestamp"
	if tzOffset != 0 {
		ts = fmt.Sprintf("DATETIME(SUBSTR(timestamp,1,19), '%+d minutes')", -tzOffset)
	}

	switch g {
	case "1m":
		return `SUBSTR(` + ts + `,1,16)`
	case "30m":
		return `SUBSTR(` + ts + `,1,14) || PRINTF('%02d', (CAST(SUBSTR(` + ts + `,15,2) AS INTEGER)/30)*30)`
	case "1h":
		return `SUBSTR(` + ts + `,1,13)`
	case "6h":
		return `SUBSTR(` + ts + `,1,11) || PRINTF('%02d', (CAST(SUBSTR(` + ts + `,12,2) AS INTEGER)/6)*6)`
	case "12h":
		return `SUBSTR(` + ts + `,1,11) || PRINTF('%02d', (CAST(SUBSTR(` + ts + `,12,2) AS INTEGER)/12)*12)`
	case "1w":
		return `DATE(SUBSTR(` + ts + `,1,10), 'weekday 0', '-6 days')`
	case "1M":
		return `SUBSTR(` + ts + `,1,7)`
	default: // "1d" or unknown
		return `SUBSTR(` + ts + `,1,10)`
	}
}

// GetCostOverTime returns cost per model grouped by the given granularity within the time range.
func (d *DB) GetCostOverTime(from, to time.Time, granularity, source, model string, tzOffset int) ([]TimeSeriesPoint, error) {
	return d.GetCostOverTimeFiltered(from, to, granularity, source, model, "", tzOffset)
}

// GetCostOverTimeFiltered returns cost grouped by time with optional project filtering.
func (d *DB) GetCostOverTimeFiltered(from, to time.Time, granularity, source, model, project string, tzOffset int) ([]TimeSeriesPoint, error) {
	from, to = utcRange(from, to)
	expr := granularityExpr(granularity, tzOffset)
	filter, fa := buildUsageFilter(source, model, project)
	args := append([]interface{}{from, to}, fa...)
	rows, err := d.db.Query(`SELECT `+expr+` as d, model, SUM(cost_usd) as cost
		FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter+`
		GROUP BY d, model ORDER BY d`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TimeSeriesPoint
	for rows.Next() {
		var p TimeSeriesPoint
		if err := rows.Scan(&p.Date, &p.Model, &p.Value); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// GetTokensOverTime returns token usage breakdown grouped by the given granularity within the time range.
func (d *DB) GetTokensOverTime(from, to time.Time, granularity, source, model string, tzOffset int) ([]TokenTimeSeriesPoint, error) {
	return d.GetTokensOverTimeFiltered(from, to, granularity, source, model, "", tzOffset)
}

// GetTokensOverTimeFiltered returns token usage grouped by time with optional project filtering.
func (d *DB) GetTokensOverTimeFiltered(from, to time.Time, granularity, source, model, project string, tzOffset int) ([]TokenTimeSeriesPoint, error) {
	from, to = utcRange(from, to)
	expr := granularityExpr(granularity, tzOffset)
	filter, fa := buildUsageFilter(source, model, project)
	args := append([]interface{}{from, to}, fa...)
	rows, err := d.db.Query(`SELECT `+expr+` as d,
		SUM(input_tokens) as inp, SUM(output_tokens) as outp,
		SUM(cache_read_input_tokens) as cr, SUM(cache_creation_input_tokens) as cc
		FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter+`
		GROUP BY d ORDER BY d`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []TokenTimeSeriesPoint
	for rows.Next() {
		var p TokenTimeSeriesPoint
		if err := rows.Scan(&p.Date, &p.InputTokens, &p.OutputTokens, &p.CacheRead, &p.CacheCreate); err != nil {
			return nil, err
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

// SessionDetail represents per-model breakdown for a single session.
type SessionDetail struct {
	Model        string  `json:"model"`
	Calls        int     `json:"calls"`
	InputTokens  int64   `json:"input_tokens"`
	OutputTokens int64   `json:"output_tokens"`
	CacheRead    int64   `json:"cache_read"`
	CacheCreate  int64   `json:"cache_create"`
	CostUSD      float64 `json:"cost_usd"`
}

// GetSessionDetail returns per-model usage breakdown for a specific session.
func (d *DB) GetSessionDetail(sessionID string) ([]SessionDetail, error) {
	return d.GetSessionDetailScoped("", sessionID)
}

// GetSessionDetailScoped returns per-model usage breakdown for a specific source/session pair.
func (d *DB) GetSessionDetailScoped(source, sessionID string) ([]SessionDetail, error) {
	if source == "" {
		var sources int
		if err := d.db.QueryRow("SELECT COUNT(DISTINCT source) FROM usage_records WHERE session_id=?", sessionID).Scan(&sources); err != nil {
			return nil, err
		}
		if sources > 1 {
			return nil, fmt.Errorf("session_id %q exists in multiple sources; include source", sessionID)
		}
	}
	sf, sa := sourceFilter(source)
	args := append([]interface{}{sessionID}, sa...)
	rows, err := d.db.Query(`SELECT model, COUNT(*) as calls,
		SUM(input_tokens) as inp, SUM(output_tokens) as outp,
		SUM(cache_read_input_tokens) as cr, SUM(cache_creation_input_tokens) as cc,
		SUM(cost_usd) as cost
		FROM usage_records WHERE session_id=?`+sf+`
		GROUP BY model ORDER BY cost DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SessionDetail
	for rows.Next() {
		var d SessionDetail
		if err := rows.Scan(&d.Model, &d.Calls, &d.InputTokens, &d.OutputTokens, &d.CacheRead, &d.CacheCreate, &d.CostUSD); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}

// GetSessions returns sessions with aggregated cost and token totals within the given time range.
func (d *DB) GetSessions(from, to time.Time, source, model string) ([]SessionInfo, error) {
	page, err := d.GetSessionsPage(from, to, source, model, "", 10000, 0)
	if err != nil {
		return nil, err
	}
	return page.Rows, nil
}

// GetSessionsPage returns sessions with aggregated totals and pagination.
func (d *DB) GetSessionsPage(from, to time.Time, source, model, project string, limit, offset int) (*SessionPage, error) {
	return d.GetSessionsPageSorted(from, to, source, model, project, "", limit, offset, "last_activity", "desc")
}

// GetSessionsPageSorted returns sessions with server-side search, sorting, and pagination.
func (d *DB) GetSessionsPageSorted(from, to time.Time, source, model, project, query string, limit, offset int, sortKey, direction string) (*SessionPage, error) {
	from, to = utcRange(from, to)
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	filter, fa := buildUsageFilter(source, model, project)
	baseArgs := append([]interface{}{from, to}, fa...)
	promptFilter, promptFilterArgs := buildUsageFilter(source, "", project)
	promptArgs := append([]interface{}{from, to}, promptFilterArgs...)
	sessionFilter, sessionArgs := sessionSearchFilter(query)
	countRows, err := d.db.Query(`SELECT COUNT(*) FROM (
		SELECT s.source, s.session_id
		FROM sessions s
		JOIN (SELECT source, session_id
			FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter+` GROUP BY source, session_id) u
		ON s.source = u.source AND s.session_id = u.session_id
		`+sessionFilter+`
	)`, append(baseArgs, sessionArgs...)...)
	if err != nil {
		return nil, err
	}
	total := 0
	if countRows.Next() {
		if err := countRows.Scan(&total); err != nil {
			countRows.Close()
			return nil, err
		}
	}
	countRows.Close()
	args := append([]interface{}{}, baseArgs...)
	args = append(args, promptArgs...)
	args = append(args, sessionArgs...)
	args = append(args, limit, offset)
	rows, err := d.db.Query(`SELECT s.session_id, s.source, s.project, s.cwd, s.git_branch,
		COALESCE(s.start_time,''), COALESCE(u.last_activity,''), COALESCE(p.prompts,0),
		COALESCE(u.cost,0), COALESCE(u.tokens,0)
		FROM sessions s
		JOIN (SELECT source, session_id, SUM(cost_usd) as cost,
				SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens) as tokens,
				MAX(timestamp) as last_activity
			FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter+` GROUP BY source, session_id) u
		ON s.source = u.source AND s.session_id = u.session_id
		LEFT JOIN (SELECT source, session_id, COUNT(*) as prompts
			FROM prompt_events WHERE timestamp >= ? AND timestamp < ?`+promptFilter+` GROUP BY source, session_id) p
		ON s.source = p.source AND s.session_id = p.session_id
		`+sessionFilter+sessionOrderBy(sortKey, direction)+` LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []SessionInfo
	for rows.Next() {
		var s SessionInfo
		if err := rows.Scan(&s.SessionID, &s.Source, &s.Project, &s.CWD, &s.GitBranch, &s.StartTime, &s.LastActivity, &s.Prompts, &s.TotalCost, &s.Tokens); err != nil {
			return nil, err
		}
		result = append(result, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	next := ""
	if offset+limit < total {
		next = fmt.Sprintf("%d", offset+limit)
	}
	return &SessionPage{Rows: result, Total: total, Limit: limit, Offset: offset, NextCursor: next}, nil
}
