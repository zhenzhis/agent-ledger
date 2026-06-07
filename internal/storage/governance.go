package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

// UnpricedModel summarizes records that cannot be priced yet.
type UnpricedModel struct {
	Source  string `json:"source"`
	Model   string `json:"model"`
	Records int    `json:"records"`
}

// QualitySource summarizes source-level trust signals.
type QualitySource struct {
	Source            string  `json:"source"`
	Records           int     `json:"records"`
	Sessions          int     `json:"sessions"`
	UnpricedRecords   int     `json:"unpriced_records"`
	CacheAwareRecords int     `json:"cache_aware_records"`
	Confidence        float64 `json:"confidence"`
	Message           string  `json:"message"`
}

// DataQualityReport explains why current data is trustworthy or incomplete.
type DataQualityReport struct {
	GeneratedAt    string                `json:"generated_at"`
	PricingSources []PricingSourceStatus `json:"pricing_sources"`
	SourceQuality  []QualitySource       `json:"source_quality"`
	UnpricedModels []UnpricedModel       `json:"unpriced_models"`
	ConfidenceMix  map[string]int        `json:"confidence_mix"`
	Projection     *ProjectionQuality    `json:"projection"`
	Issues         []InsightEvent        `json:"issues"`
}

// ProjectionQuality explains whether canonical model calls and usage records stay aligned.
type ProjectionQuality struct {
	ModelCalls             int     `json:"model_calls"`
	ProjectedUsageRecords  int     `json:"projected_usage_records"`
	MissingUsageProjection int     `json:"missing_usage_projection"`
	CostMismatchRecords    int     `json:"cost_mismatch_records"`
	CostDeltaUSD           float64 `json:"cost_delta_usd"`
	DuplicateSessionOwners int     `json:"duplicate_session_owners"`
	Confidence             float64 `json:"confidence"`
	Message                string  `json:"message"`
}

// ModelCallRow summarizes model call counts and cost.
type ModelCallRow struct {
	Source        string  `json:"source"`
	Model         string  `json:"model"`
	Project       string  `json:"project"`
	Calls         int     `json:"calls"`
	Tokens        int64   `json:"tokens"`
	CostUSD       float64 `json:"cost_usd"`
	AvgTokensCall float64 `json:"avg_tokens_per_call"`
	CostPerCall   float64 `json:"cost_per_call"`
	UnpricedCalls int     `json:"unpriced_calls"`
}

// CostInsightRow explains one high-impact session without reading prompt text.
type CostInsightRow struct {
	Source       string   `json:"source"`
	SessionID    string   `json:"session_id"`
	Project      string   `json:"project"`
	GitBranch    string   `json:"git_branch"`
	Models       int      `json:"models"`
	Calls        int      `json:"calls"`
	Prompts      int      `json:"prompts"`
	Tokens       int64    `json:"tokens"`
	CostUSD      float64  `json:"cost_usd"`
	CacheHitRate float64  `json:"cache_hit_rate"`
	OutputRatio  float64  `json:"output_ratio"`
	QualityScore float64  `json:"quality_score"`
	Reasons      []string `json:"reasons"`
	Advice       []string `json:"advice"`
	LastActivity string   `json:"last_activity"`
}

// CacheDoctorRow summarizes cache behavior by source/model/project.
type CacheDoctorRow struct {
	Source              string  `json:"source"`
	Model               string  `json:"model"`
	Project             string  `json:"project"`
	Calls               int     `json:"calls"`
	InputTokens         int64   `json:"input_tokens"`
	CacheReadTokens     int64   `json:"cache_read_tokens"`
	CacheWriteTokens    int64   `json:"cache_write_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	CostUSD             float64 `json:"cost_usd"`
	CacheHitRate        float64 `json:"cache_hit_rate"`
	EstimatedLostSaving float64 `json:"estimated_lost_saving"`
	Message             string  `json:"message"`
}

// ReconciliationImport records an imported provider bill comparison.
type ReconciliationImport struct {
	ID              int64   `json:"id"`
	Provider        string  `json:"provider"`
	Format          string  `json:"format"`
	Currency        string  `json:"currency"`
	LocalCostUSD    float64 `json:"local_cost_usd"`
	ProviderCostUSD float64 `json:"provider_cost_usd"`
	DiffUSD         float64 `json:"diff_usd"`
	RowsSeen        int     `json:"rows_seen"`
	PayloadSHA256   string  `json:"payload_sha256"`
	WindowStart     string  `json:"window_start"`
	WindowEnd       string  `json:"window_end"`
	Status          string  `json:"status"`
	Notes           string  `json:"notes"`
	Warnings        string  `json:"warnings"`
	ImportedAt      string  `json:"imported_at"`
}

// UpsertPricingSource records pricing source health.
func (d *DB) UpsertPricingSource(s PricingSourceStatus) error {
	_, err := d.db.Exec(`INSERT INTO pricing_sources(name,kind,priority,url,last_fetch_at,etag,sha256,model_count,status,last_error)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(name) DO UPDATE SET
			kind=excluded.kind,
			priority=excluded.priority,
			url=excluded.url,
			last_fetch_at=excluded.last_fetch_at,
			etag=excluded.etag,
			sha256=excluded.sha256,
			model_count=excluded.model_count,
			status=excluded.status,
			last_error=excluded.last_error`,
		s.Name, s.Kind, s.Priority, s.URL, s.LastFetchAt, s.ETag, s.SHA256, s.ModelCount, s.Status, s.LastError)
	return err
}

// InsertPricingSnapshot stores immutable pricing sync metadata.
func (d *DB) InsertPricingSnapshot(source, sha string, modelCount int, meta map[string]string) error {
	raw, _ := json.Marshal(meta)
	_, err := d.db.Exec(`INSERT INTO pricing_snapshots(source,sha256,model_count,raw_metadata,fetched_at) VALUES(?,?,?,?,?)`,
		source, sha, modelCount, string(raw), time.Now().UTC())
	return err
}

// InsertPricingAuditEvent stores a pricing audit event.
func (d *DB) InsertPricingAuditEvent(eventType, source, model, message string) error {
	_, err := d.db.Exec(`INSERT INTO pricing_audit_events(event_type,source,model,message,created_at) VALUES(?,?,?,?,?)`,
		eventType, source, model, message, time.Now().UTC())
	return err
}

// GetPricingSources returns pricing source health with stale calculation.
func (d *DB) GetPricingSources(staleAfter time.Duration) ([]PricingSourceStatus, error) {
	rows, err := d.db.Query(`SELECT name,kind,priority,url,COALESCE(last_fetch_at,''),etag,sha256,model_count,status,last_error
		FROM pricing_sources ORDER BY priority,name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PricingSourceStatus
	now := time.Now().UTC()
	for rows.Next() {
		var s PricingSourceStatus
		if err := rows.Scan(&s.Name, &s.Kind, &s.Priority, &s.URL, &s.LastFetchAt, &s.ETag, &s.SHA256, &s.ModelCount, &s.Status, &s.LastError); err != nil {
			return nil, err
		}
		if t, ok := parseDBTime(s.LastFetchAt); !ok || now.Sub(t) > staleAfter {
			s.Stale = true
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// GetPricingAudit returns effective pricing rows.
func (d *DB) GetPricingAudit(limit int) ([]PricingAuditRow, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := d.db.Query(`SELECT model,input_cost_per_token,output_cost_per_token,cache_read_input_token_cost,
		cache_creation_input_token_cost,COALESCE(pricing_source,''),COALESCE(matched_model,''),COALESCE(match_type,''),
		COALESCE(priority,999),COALESCE(confidence,''),COALESCE(updated_at,'')
		FROM pricing ORDER BY priority,model LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []PricingAuditRow
	for rows.Next() {
		var r PricingAuditRow
		if err := rows.Scan(&r.Model, &r.InputCostPerToken, &r.OutputCostPerToken, &r.CacheReadCostPerToken, &r.CacheWriteCostPerToken,
			&r.PricingSource, &r.MatchedModel, &r.MatchType, &r.Priority, &r.Confidence, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AppendAuditLog stores a local operation audit event.
func (d *DB) AppendAuditLog(actor, role, action, target string, params map[string]string) error {
	raw, _ := json.Marshal(params)
	_, err := d.db.Exec(`INSERT INTO audit_log(actor,role,action,target,params,created_at) VALUES(?,?,?,?,?,?)`,
		actor, role, action, target, string(raw), time.Now().UTC())
	return err
}

// GetAuditLog returns recent local audit events.
func (d *DB) GetAuditLog(limit int) ([]AuditEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := d.db.Query(`SELECT id,actor,role,action,target,params,created_at FROM audit_log ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEvent
	for rows.Next() {
		var e AuditEvent
		if err := rows.Scan(&e.ID, &e.Actor, &e.Role, &e.Action, &e.Target, &e.Params, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// RebuildUsageAggregates rebuilds hourly and daily rollups from raw usage.
func (d *DB) RebuildUsageAggregates() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, q := range []string{"DELETE FROM hourly_usage_aggregate", "DELETE FROM daily_usage_aggregate"} {
		if _, err := tx.Exec(q); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`INSERT INTO hourly_usage_aggregate(bucket,source,model,project,git_branch,calls,input_tokens,output_tokens,
		cache_read_input_tokens,cache_creation_input_tokens,reasoning_output_tokens,cost_usd)
		SELECT SUBSTR(timestamp,1,13) || ':00:00', source, model, project, git_branch, COUNT(*),
			COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
			COALESCE(SUM(cache_read_input_tokens),0), COALESCE(SUM(cache_creation_input_tokens),0),
			COALESCE(SUM(reasoning_output_tokens),0), COALESCE(SUM(cost_usd),0)
		FROM usage_records GROUP BY 1,2,3,4,5`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO daily_usage_aggregate(bucket,source,model,project,git_branch,calls,input_tokens,output_tokens,
		cache_read_input_tokens,cache_creation_input_tokens,reasoning_output_tokens,cost_usd)
		SELECT SUBSTR(timestamp,1,10), source, model, project, git_branch, COUNT(*),
			COALESCE(SUM(input_tokens),0), COALESCE(SUM(output_tokens),0),
			COALESCE(SUM(cache_read_input_tokens),0), COALESCE(SUM(cache_creation_input_tokens),0),
			COALESCE(SUM(reasoning_output_tokens),0), COALESCE(SUM(cost_usd),0)
		FROM usage_records GROUP BY 1,2,3,4,5`); err != nil {
		return err
	}
	return tx.Commit()
}

// GetDataQuality returns source and pricing quality diagnostics.
func (d *DB) GetDataQuality(staleAfter time.Duration) (*DataQualityReport, error) {
	report := &DataQualityReport{GeneratedAt: time.Now().UTC().Format(time.RFC3339), ConfidenceMix: map[string]int{}}
	sources, err := d.GetPricingSources(staleAfter)
	if err != nil {
		return nil, err
	}
	report.PricingSources = sources
	projection, err := d.GetProjectionQuality(time.Time{}, time.Now().UTC().AddDate(10, 0, 0), "", "", "")
	if err != nil {
		return nil, err
	}
	report.Projection = projection

	rows, err := d.db.Query(`SELECT source, model, COUNT(*) FROM usage_records
		WHERE COALESCE(pricing_confidence,'')='unpriced'
		GROUP BY source,model ORDER BY COUNT(*) DESC LIMIT 100`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var r UnpricedModel
		if err := rows.Scan(&r.Source, &r.Model, &r.Records); err != nil {
			rows.Close()
			return nil, err
		}
		report.UnpricedModels = append(report.UnpricedModels, r)
	}
	rows.Close()

	mixRows, err := d.db.Query(`SELECT COALESCE(NULLIF(pricing_confidence,''),'unknown'), COUNT(*) FROM usage_records GROUP BY 1`)
	if err != nil {
		return nil, err
	}
	for mixRows.Next() {
		var k string
		var v int
		if err := mixRows.Scan(&k, &v); err != nil {
			mixRows.Close()
			return nil, err
		}
		report.ConfidenceMix[k] = v
	}
	mixRows.Close()

	qRows, err := d.db.Query(`SELECT source, COUNT(*), COUNT(DISTINCT session_id),
		SUM(CASE WHEN COALESCE(pricing_confidence,'')='unpriced' THEN 1 ELSE 0 END),
		SUM(CASE WHEN cache_read_input_tokens>0 OR cache_creation_input_tokens>0 THEN 1 ELSE 0 END)
		FROM usage_records GROUP BY source ORDER BY source`)
	if err != nil {
		return nil, err
	}
	for qRows.Next() {
		var q QualitySource
		if err := qRows.Scan(&q.Source, &q.Records, &q.Sessions, &q.UnpricedRecords, &q.CacheAwareRecords); err != nil {
			qRows.Close()
			return nil, err
		}
		score := 1.0
		if q.Records > 0 {
			score -= float64(q.UnpricedRecords) / float64(q.Records) * 0.55
			if q.CacheAwareRecords == 0 {
				score -= 0.1
			}
		}
		if score < 0 {
			score = 0
		}
		q.Confidence = math.Round(score*100) / 100
		if q.UnpricedRecords > 0 {
			q.Message = fmt.Sprintf("%d records are unpriced", q.UnpricedRecords)
		} else {
			q.Message = "priced and queryable"
		}
		report.SourceQuality = append(report.SourceQuality, q)
	}
	qRows.Close()

	issues, err := d.GetInsightEvents("quality", 50)
	if err != nil {
		return nil, err
	}
	report.Issues = issues
	return report, nil
}

// GetProjectionQuality checks canonical model-call projection consistency for one scope.
func (d *DB) GetProjectionQuality(from, to time.Time, source, model, project string) (*ProjectionQuality, error) {
	if to.IsZero() {
		to = time.Now().UTC().AddDate(10, 0, 0)
	}
	where := []string{"mc.timestamp >= ?", "mc.timestamp < ?"}
	args := []interface{}{from, to}
	if source != "" {
		where = append(where, "mc.source=?")
		args = append(args, source)
	}
	if model != "" {
		where = append(where, "mc.model=?")
		args = append(args, model)
	}
	if project != "" {
		where = append(where, "COALESCE(w.project,'')=?")
		args = append(args, project)
	}
	sql := `WITH matched AS (
		SELECT mc.call_id, mc.cost_usd AS model_cost, u.id AS usage_id, u.cost_usd AS usage_cost
		FROM model_calls mc
		LEFT JOIN workloads w ON w.workload_id=mc.workload_id
		LEFT JOIN usage_records u ON u.source=mc.source
			AND u.session_id=mc.session_id
			AND u.model=mc.model
			AND u.timestamp=mc.timestamp
			AND u.input_tokens=mc.input_tokens
			AND u.output_tokens=mc.output_tokens
			AND u.cache_creation_input_tokens=mc.cache_creation_input_tokens
			AND u.cache_read_input_tokens=mc.cache_read_input_tokens
			AND u.reasoning_output_tokens=mc.reasoning_output_tokens
		WHERE ` + strings.Join(where, " AND ") + `
	)
	SELECT COUNT(*),
		COALESCE(SUM(CASE WHEN usage_id IS NOT NULL THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN usage_id IS NULL THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN usage_id IS NOT NULL AND ABS(COALESCE(model_cost,0)-COALESCE(usage_cost,0)) > 0.000001 THEN 1 ELSE 0 END),0),
		COALESCE(SUM(CASE WHEN usage_id IS NOT NULL THEN ABS(COALESCE(model_cost,0)-COALESCE(usage_cost,0)) ELSE 0 END),0)
	FROM matched`
	q := &ProjectionQuality{}
	if err := d.db.QueryRow(sql, args...).Scan(&q.ModelCalls, &q.ProjectedUsageRecords, &q.MissingUsageProjection, &q.CostMismatchRecords, &q.CostDeltaUSD); err != nil {
		return nil, err
	}
	usageFilter, usageArgs := buildUsageFilterAlias("u", source, model, project)
	dupArgs := append([]interface{}{from, to}, usageArgs...)
	dupSQL := `WITH scoped_sessions AS (
		SELECT DISTINCT u.source,u.session_id FROM usage_records u
		WHERE u.timestamp >= ? AND u.timestamp < ?` + usageFilter + `
	)
	SELECT COUNT(*) FROM (
		SELECT ws.source,ws.session_id
		FROM workload_sessions ws
		JOIN scoped_sessions ss ON ss.source=ws.source AND ss.session_id=ws.session_id
		JOIN workloads w ON w.workload_id=ws.workload_id
		GROUP BY ws.source,ws.session_id
		HAVING COUNT(DISTINCT ws.workload_id)>1
			AND SUM(CASE WHEN COALESCE(w.outcome,'')='legacy-session-derived' THEN 1 ELSE 0 END)>0
			AND SUM(CASE WHEN COALESCE(w.outcome,'')<>'legacy-session-derived' THEN 1 ELSE 0 END)>0
	)`
	if err := d.db.QueryRow(dupSQL, dupArgs...).Scan(&q.DuplicateSessionOwners); err != nil {
		return nil, err
	}
	q.CostDeltaUSD = math.Round(q.CostDeltaUSD*10000) / 10000
	q.Confidence = projectionConfidence(q)
	q.Message = projectionMessage(q)
	return q, nil
}

func projectionConfidence(q *ProjectionQuality) float64 {
	if q == nil {
		return 0
	}
	if q.ModelCalls == 0 {
		if q.DuplicateSessionOwners > 0 {
			return 0.7
		}
		return 1
	}
	score := 1.0
	score -= float64(q.MissingUsageProjection) / float64(q.ModelCalls) * 0.55
	score -= float64(q.CostMismatchRecords) / float64(q.ModelCalls) * 0.35
	if q.DuplicateSessionOwners > 0 {
		score -= 0.1
	}
	if score < 0 {
		score = 0
	}
	return math.Round(score*100) / 100
}

func projectionMessage(q *ProjectionQuality) string {
	if q == nil {
		return "projection quality unavailable"
	}
	if q.ModelCalls == 0 && q.DuplicateSessionOwners == 0 {
		return "no canonical model calls in scope"
	}
	var parts []string
	if q.MissingUsageProjection > 0 {
		parts = append(parts, fmt.Sprintf("%d canonical model calls are missing usage projection", q.MissingUsageProjection))
	}
	if q.CostMismatchRecords > 0 {
		parts = append(parts, fmt.Sprintf("%d projected usage records have cost mismatch", q.CostMismatchRecords))
	}
	if q.DuplicateSessionOwners > 0 {
		parts = append(parts, fmt.Sprintf("%d sessions have both legacy and canonical workload owners", q.DuplicateSessionOwners))
	}
	if len(parts) == 0 {
		return "canonical model calls and usage projections are consistent"
	}
	return strings.Join(parts, "; ")
}

// GetModelCalls returns call analytics grouped by model/source/project.
func (d *DB) GetModelCalls(from, to time.Time, source, model, project string, limit int) ([]ModelCallRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	filter, fa := buildUsageFilterAlias("u", source, model, project)
	args := append([]interface{}{from, to}, fa...)
	args = append(args, limit)
	rows, err := d.db.Query(`SELECT u.source,u.model,u.project,COUNT(*),
		COALESCE(SUM(u.input_tokens+u.cache_read_input_tokens+u.cache_creation_input_tokens+u.output_tokens),0),
		COALESCE(SUM(u.cost_usd),0), SUM(CASE WHEN u.cost_usd=0 THEN 1 ELSE 0 END)
		FROM usage_records u WHERE u.timestamp >= ? AND u.timestamp < ?`+filter+`
		GROUP BY u.source,u.model,u.project ORDER BY COUNT(*) DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelCallRow
	for rows.Next() {
		var r ModelCallRow
		if err := rows.Scan(&r.Source, &r.Model, &r.Project, &r.Calls, &r.Tokens, &r.CostUSD, &r.UnpricedCalls); err != nil {
			return nil, err
		}
		if r.Calls > 0 {
			r.AvgTokensCall = float64(r.Tokens) / float64(r.Calls)
			r.CostPerCall = r.CostUSD / float64(r.Calls)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetCostIntelligence returns high-cost session explanations and advice.
func (d *DB) GetCostIntelligence(from, to time.Time, source, model, project string, limit int) ([]CostInsightRow, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	filter, fa := buildUsageFilterAlias("u", source, model, project)
	args := append([]interface{}{from, to}, fa...)
	args = append(args, limit)
	rows, err := d.db.Query(`SELECT u.source,u.session_id,COALESCE(s.project,u.project),COALESCE(s.git_branch,''),
		COUNT(DISTINCT u.model), COUNT(*), COALESCE(s.prompts,0),
		COALESCE(SUM(u.input_tokens+u.cache_read_input_tokens+u.cache_creation_input_tokens+u.output_tokens),0),
		COALESCE(SUM(u.cost_usd),0), COALESCE(SUM(u.cache_read_input_tokens),0),
		COALESCE(SUM(u.input_tokens+u.cache_read_input_tokens+u.cache_creation_input_tokens),0),
		COALESCE(SUM(u.output_tokens),0), MAX(u.timestamp)
		FROM usage_records u LEFT JOIN sessions s ON u.source=s.source AND u.session_id=s.session_id
		WHERE u.timestamp >= ? AND u.timestamp < ?`+filter+`
		GROUP BY u.source,u.session_id ORDER BY COALESCE(SUM(u.cost_usd),0) DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CostInsightRow
	for rows.Next() {
		var r CostInsightRow
		var cacheRead, totalInput, output int64
		if err := rows.Scan(&r.Source, &r.SessionID, &r.Project, &r.GitBranch, &r.Models, &r.Calls, &r.Prompts, &r.Tokens, &r.CostUSD, &cacheRead, &totalInput, &output, &r.LastActivity); err != nil {
			return nil, err
		}
		if totalInput > 0 {
			r.CacheHitRate = float64(cacheRead) / float64(totalInput)
			r.OutputRatio = float64(output) / float64(totalInput)
		}
		r.Reasons, r.Advice = explainCost(r, totalInput, output)
		r.QualityScore = sessionQualityScore(r)
		out = append(out, r)
	}
	return out, rows.Err()
}

// GetCacheDoctor returns cache diagnostics.
func (d *DB) GetCacheDoctor(from, to time.Time, source, model, project string, limit int) ([]CacheDoctorRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	filter, fa := buildUsageFilter(source, model, project)
	args := append([]interface{}{from, to}, fa...)
	args = append(args, limit)
	rows, err := d.db.Query(`SELECT source,model,project,COUNT(*),
		COALESCE(SUM(input_tokens),0),COALESCE(SUM(cache_read_input_tokens),0),
		COALESCE(SUM(cache_creation_input_tokens),0),COALESCE(SUM(output_tokens),0),COALESCE(SUM(cost_usd),0)
		FROM usage_records WHERE timestamp >= ? AND timestamp < ?`+filter+`
		GROUP BY source,model,project ORDER BY SUM(input_tokens+cache_creation_input_tokens) DESC LIMIT ?`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []CacheDoctorRow
	for rows.Next() {
		var r CacheDoctorRow
		if err := rows.Scan(&r.Source, &r.Model, &r.Project, &r.Calls, &r.InputTokens, &r.CacheReadTokens, &r.CacheWriteTokens, &r.OutputTokens, &r.CostUSD); err != nil {
			return nil, err
		}
		totalInput := r.InputTokens + r.CacheReadTokens + r.CacheWriteTokens
		if totalInput > 0 {
			r.CacheHitRate = float64(r.CacheReadTokens) / float64(totalInput)
		}
		if r.CacheHitRate < 0.25 && totalInput > 10000 {
			r.EstimatedLostSaving = r.CostUSD * 0.20
			r.Message = "low cache hit rate; check cwd, resume/compact, model switches, or tool context churn"
		} else if r.CacheWriteTokens > r.CacheReadTokens*2 && r.CacheWriteTokens > 0 {
			r.EstimatedLostSaving = r.CostUSD * 0.10
			r.Message = "cache writes exceed cache reads; context may be changing too often"
		} else {
			r.Message = "cache behavior looks normal for available local fields"
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// UpsertInsightEvent inserts an insight event.
func (d *DB) UpsertInsightEvent(e InsightEvent) error {
	_, err := d.db.Exec(`INSERT INTO insight_events(kind,severity,source,model,project,session_id,metric,value,baseline,message,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`,
		e.Kind, e.Severity, e.Source, e.Model, e.Project, e.SessionID, e.Metric, e.Value, e.Baseline, e.Message, time.Now().UTC())
	return err
}

// GetInsightEvents returns recent insight events.
func (d *DB) GetInsightEvents(kind string, limit int) ([]InsightEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT id,kind,severity,source,model,project,session_id,metric,value,baseline,message,created_at FROM insight_events`
	args := []interface{}{}
	if kind != "" {
		q += ` WHERE kind=?`
		args = append(args, kind)
	}
	q += ` ORDER BY created_at DESC LIMIT ?`
	args = append(args, limit)
	rows, err := d.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []InsightEvent
	for rows.Next() {
		var e InsightEvent
		if err := rows.Scan(&e.ID, &e.Kind, &e.Severity, &e.Source, &e.Model, &e.Project, &e.SessionID, &e.Metric, &e.Value, &e.Baseline, &e.Message, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DetectAnomalies stores simple robust-statistics anomaly events for a window.
func (d *DB) DetectAnomalies(from, to time.Time, multiplier float64, nightStart, nightEnd int) error {
	rows, err := d.db.Query(`SELECT source,model,project,session_id,
		SUM(input_tokens+cache_read_input_tokens+cache_creation_input_tokens+output_tokens) as tokens,
		SUM(cost_usd), MAX(timestamp)
		FROM usage_records WHERE timestamp >= ? AND timestamp < ?
		GROUP BY source,model,project,session_id`, from, to)
	if err != nil {
		return err
	}
	type sample struct {
		source, model, project, session, ts string
		tokens                              float64
		cost                                float64
	}
	var samples []sample
	for rows.Next() {
		var s sample
		if err := rows.Scan(&s.source, &s.model, &s.project, &s.session, &s.tokens, &s.cost, &s.ts); err != nil {
			rows.Close()
			return err
		}
		samples = append(samples, s)
	}
	rows.Close()
	if len(samples) < 8 {
		return nil
	}
	values := make([]float64, len(samples))
	for i, s := range samples {
		values[i] = s.tokens
	}
	med := median(values)
	mad := medianAbsDeviation(values, med)
	if mad <= 0 {
		mad = 1
	}
	threshold := med + multiplier*mad*1.4826
	for _, s := range samples {
		if s.tokens > threshold {
			_ = d.UpsertInsightEvent(InsightEvent{
				Kind: "anomaly", Severity: "warning", Source: s.source, Model: s.model, Project: s.project, SessionID: s.session,
				Metric: "tokens", Value: s.tokens, Baseline: threshold,
				Message: "session token volume is above rolling median/MAD threshold",
			})
		}
		if t, ok := parseDBTime(s.ts); ok && isNightHour(t.Hour(), nightStart, nightEnd) {
			_ = d.UpsertInsightEvent(InsightEvent{
				Kind: "watchdog", Severity: "info", Source: s.source, Model: s.model, Project: s.project, SessionID: s.session,
				Metric: "night_usage", Value: s.tokens, Baseline: 0,
				Message: "usage occurred during configured non-working hours",
			})
		}
	}
	return nil
}

// InsertReconciliationImport stores one provider reconciliation summary.
func (d *DB) InsertReconciliationImport(provider, format string, localCost, providerCost float64, rowsSeen int, notes string) error {
	return d.InsertReconciliationImportDetailed(ReconciliationImport{
		Provider: provider, Format: format, Currency: "USD", LocalCostUSD: localCost,
		ProviderCostUSD: providerCost, RowsSeen: rowsSeen, Notes: notes,
	})
}

// InsertReconciliationImportDetailed stores one provider reconciliation summary
// with statement integrity and provider window metadata.
func (d *DB) InsertReconciliationImportDetailed(row ReconciliationImport) error {
	PrepareReconciliationImport(&row)
	_, err := d.db.Exec(`INSERT INTO reconciliation_imports(provider,format,currency,local_cost_usd,provider_cost_usd,diff_usd,rows_seen,payload_sha256,window_start,window_end,status,notes,warnings,imported_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, row.Provider, row.Format, row.Currency, row.LocalCostUSD, row.ProviderCostUSD, row.DiffUSD,
		row.RowsSeen, row.PayloadSHA256, row.WindowStart, row.WindowEnd, row.Status, row.Notes, row.Warnings, time.Now().UTC())
	return err
}

// PrepareReconciliationImport normalizes derived fields before storage or API
// responses.
func PrepareReconciliationImport(row *ReconciliationImport) {
	if row == nil {
		return
	}
	row.Provider = strings.TrimSpace(row.Provider)
	if row.Provider == "" {
		row.Provider = "provider"
	}
	row.Format = strings.TrimSpace(row.Format)
	if row.Format == "" {
		row.Format = "manual"
	}
	row.Currency = strings.ToUpper(strings.TrimSpace(row.Currency))
	if row.Currency == "" {
		row.Currency = "USD"
	}
	row.DiffUSD = row.ProviderCostUSD - row.LocalCostUSD
	row.Status = reconciliationStatus(row.LocalCostUSD, row.ProviderCostUSD, row.Warnings)
}

func reconciliationStatus(localCost, providerCost float64, warnings string) string {
	if providerCost == 0 {
		return "empty"
	}
	diff := providerCost - localCost
	if math.Abs(diff) > math.Max(1, localCost*0.05) {
		return "mismatch"
	}
	if strings.TrimSpace(warnings) != "" {
		return "warning"
	}
	return "ok"
}

// GetReconciliationImports returns recent reconciliation imports.
func (d *DB) GetReconciliationImports(limit int) ([]ReconciliationImport, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := d.db.Query(`SELECT id,provider,format,COALESCE(currency,'USD'),local_cost_usd,provider_cost_usd,diff_usd,rows_seen,
			COALESCE(payload_sha256,''),COALESCE(window_start,''),COALESCE(window_end,''),status,notes,COALESCE(warnings,''),imported_at
		FROM reconciliation_imports ORDER BY imported_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ReconciliationImport
	for rows.Next() {
		var r ReconciliationImport
		if err := rows.Scan(&r.ID, &r.Provider, &r.Format, &r.Currency, &r.LocalCostUSD, &r.ProviderCostUSD, &r.DiffUSD,
			&r.RowsSeen, &r.PayloadSHA256, &r.WindowStart, &r.WindowEnd, &r.Status, &r.Notes, &r.Warnings, &r.ImportedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// RecordOfflineBundle stores an offline bundle digest.
func (d *DB) RecordOfflineBundle(bundleID string, payload []byte, format string) error {
	sum := sha256.Sum256(payload)
	_, err := d.db.Exec(`INSERT INTO offline_bundles(bundle_id,sha256,format,created_at) VALUES(?,?,?,?)`,
		bundleID, hex.EncodeToString(sum[:]), format, time.Now().UTC())
	return err
}

func explainCost(r CostInsightRow, totalInput, output int64) ([]string, []string) {
	var reasons, advice []string
	if r.Models > 1 {
		reasons = append(reasons, "multiple models used in one session")
		advice = append(advice, "review model switches; keep low-risk turns on smaller models when possible")
	}
	if r.CacheHitRate < 0.25 && totalInput > 10000 {
		reasons = append(reasons, "low cache hit rate with large context")
		advice = append(advice, "stabilize cwd/context, avoid unnecessary resume/compact churn, and keep reusable context stable")
	}
	if r.Prompts > 0 && r.Tokens/int64(maxInt(1, r.Prompts)) > 50000 {
		reasons = append(reasons, "high tokens per prompt")
		advice = append(advice, "split broad tasks or reduce repository/context scope before starting the agent")
	}
	if output > 0 && totalInput > 0 && float64(output)/float64(totalInput) > 0.8 {
		reasons = append(reasons, "high output/input ratio")
		advice = append(advice, "check if generated output is overly verbose or repeated")
	}
	if len(reasons) == 0 {
		reasons = append(reasons, "normal cost profile for available local fields")
		advice = append(advice, "no obvious waste pattern detected")
	}
	return reasons, advice
}

func sessionQualityScore(r CostInsightRow) float64 {
	score := 1.0
	if r.CacheHitRate < 0.25 && r.Tokens > 10000 {
		score -= 0.25
	}
	if r.Prompts > 0 && r.Tokens/int64(maxInt(1, r.Prompts)) > 50000 {
		score -= 0.25
	}
	if r.Models > 1 {
		score -= 0.1
	}
	if r.CostUSD > 10 {
		score -= 0.1
	}
	if score < 0 {
		score = 0
	}
	return math.Round(score*100) / 100
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	cp := append([]float64(nil), values...)
	sort.Float64s(cp)
	mid := len(cp) / 2
	if len(cp)%2 == 1 {
		return cp[mid]
	}
	return (cp[mid-1] + cp[mid]) / 2
}

func medianAbsDeviation(values []float64, med float64) float64 {
	dev := make([]float64, len(values))
	for i, v := range values {
		dev[i] = math.Abs(v - med)
	}
	return median(dev)
}

func isNightHour(hour, start, end int) bool {
	if start == end {
		return false
	}
	if start > end {
		return hour >= start || hour < end
	}
	return hour >= start && hour < end
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func buildUsageFilterAlias(alias, source, model, project string) (string, []interface{}) {
	var clauses []string
	var args []interface{}
	prefix := ""
	if alias != "" {
		prefix = alias + "."
	}
	if source != "" {
		clauses = append(clauses, prefix+"source=?")
		args = append(args, source)
	}
	if model != "" {
		clauses = append(clauses, prefix+"model=?")
		args = append(args, model)
	}
	if project != "" {
		clauses = append(clauses, prefix+"project=?")
		args = append(args, project)
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func parseDBTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	formats := []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999 -0700 MST", "2006-01-02 15:04:05 -0700 MST", "2006-01-02 15:04:05"}
	for _, f := range formats {
		if t, err := time.Parse(f, raw); err == nil {
			return t, true
		}
	}
	if len(raw) >= 19 {
		if t, err := time.Parse("2006-01-02 15:04:05", strings.Replace(raw[:19], "T", " ", 1)); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func nullString(v sql.NullString) string {
	if v.Valid {
		return v.String
	}
	return ""
}
