package storage

import (
	"sort"
	"strings"
	"time"
)

// ChargebackRow is a team/project showback row. It intentionally contains only
// metadata and usage numbers, never prompt text or command content.
type ChargebackRow struct {
	Team          string  `json:"team"`
	Project       string  `json:"project"`
	Source        string  `json:"source"`
	Model         string  `json:"model"`
	Calls         int     `json:"calls"`
	Sessions      int     `json:"sessions"`
	Tokens        int64   `json:"tokens"`
	CostUSD       float64 `json:"cost_usd"`
	AvgTokensCall float64 `json:"avg_tokens_per_call"`
	CostPerCall   float64 `json:"cost_per_call"`
	UnpricedCalls int     `json:"unpriced_calls"`
	MappingSource string  `json:"mapping_source"`
	DataSource    string  `json:"data_source"`
	Confidence    float64 `json:"confidence"`
}

// GetChargeback returns team/project/model showback data. Raw usage_records are
// authoritative. Canonical model_calls are used only when no raw usage records
// exist in the requested window, avoiding double counting after backfill.
func (d *DB) GetChargeback(from, to time.Time, source, model, project string, groups map[string]string, machineName, gitAuthor string, limit int) ([]ChargebackRow, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	if d.hasUsageRows(from, to, source, model, project) {
		return d.getRawChargeback(from, to, source, model, project, groups, machineName, gitAuthor, limit)
	}
	return d.getCanonicalChargeback(from, to, source, model, project, groups, machineName, gitAuthor, limit)
}

func (d *DB) hasUsageRows(from, to time.Time, source, model, project string) bool {
	filter, fa := buildUsageFilterAlias("u", source, model, project)
	args := append([]interface{}{from, to}, fa...)
	var rows int
	if err := d.db.QueryRow(`SELECT COUNT(*) FROM usage_records u WHERE u.timestamp >= ? AND u.timestamp < ?`+filter, args...).Scan(&rows); err != nil {
		return false
	}
	return rows > 0
}

func (d *DB) getRawChargeback(from, to time.Time, source, model, project string, groups map[string]string, machineName, gitAuthor string, limit int) ([]ChargebackRow, error) {
	filter, fa := buildUsageFilterAlias("u", source, model, project)
	args := append([]interface{}{from, to}, fa...)
	rows, err := d.db.Query(`SELECT u.source,u.model,COALESCE(NULLIF(w.project,''),u.project,''),COALESCE(w.repo,''),COALESCE(w.owner,''),COALESCE(w.team,''),COUNT(*),
		COUNT(DISTINCT u.source || char(0) || u.session_id),
		COALESCE(SUM(u.input_tokens+u.cache_read_input_tokens+u.cache_creation_input_tokens+u.output_tokens),0),
		COALESCE(SUM(u.cost_usd),0),
		SUM(CASE WHEN u.cost_usd=0 THEN 1 ELSE 0 END)
		FROM usage_records u
		LEFT JOIN (
			SELECT ws.source,ws.session_id,
				COALESCE(MAX(CASE WHEN COALESCE(w.outcome,'')<>'legacy-session-derived' THEN ws.workload_id END),MAX(ws.workload_id)) AS workload_id
			FROM workload_sessions ws JOIN workloads w ON w.workload_id=ws.workload_id
			GROUP BY ws.source,ws.session_id
		) owner ON owner.source=u.source AND owner.session_id=u.session_id
		LEFT JOIN workloads w ON w.workload_id=owner.workload_id
		WHERE u.timestamp >= ? AND u.timestamp < ?`+filter+`
		GROUP BY u.source,u.model,COALESCE(NULLIF(w.project,''),u.project,''),w.repo,w.owner,w.team`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agg := map[string]*ChargebackRow{}
	for rows.Next() {
		var r ChargebackRow
		var repo, owner, explicitTeam string
		if err := rows.Scan(&r.Source, &r.Model, &r.Project, &repo, &owner, &explicitTeam, &r.Calls, &r.Sessions, &r.Tokens, &r.CostUSD, &r.UnpricedCalls); err != nil {
			return nil, err
		}
		r.Team, r.MappingSource, r.Confidence = resolveShowbackTeam(groups, r.Project, repo, owner, explicitTeam, machineName, gitAuthor)
		r.DataSource = "usage_records"
		mergeChargeback(agg, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sortedChargeback(agg, limit), nil
}

func (d *DB) getCanonicalChargeback(from, to time.Time, source, model, project string, groups map[string]string, machineName, gitAuthor string, limit int) ([]ChargebackRow, error) {
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
		where = append(where, "(w.project=? OR w.repo=?)")
		args = append(args, project, project)
	}
	rows, err := d.db.Query(`SELECT mc.source,mc.model,COALESCE(w.project,''),COALESCE(w.repo,''),COALESCE(w.owner,''),COALESCE(w.team,''),
		COUNT(*), COUNT(DISTINCT mc.source || char(0) || mc.session_id),
		COALESCE(SUM(mc.input_tokens+mc.cache_read_input_tokens+mc.cache_creation_input_tokens+mc.output_tokens),0),
		COALESCE(SUM(mc.cost_usd),0),
		SUM(CASE WHEN mc.cost_usd=0 THEN 1 ELSE 0 END),
		COALESCE(AVG(mc.confidence),1)
		FROM model_calls mc
		LEFT JOIN workloads w ON mc.workload_id=w.workload_id
		WHERE `+strings.Join(where, " AND ")+`
		GROUP BY mc.source,mc.model,w.project,w.repo,w.owner,w.team`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	agg := map[string]*ChargebackRow{}
	for rows.Next() {
		var r ChargebackRow
		var repo, owner, explicitTeam string
		if err := rows.Scan(&r.Source, &r.Model, &r.Project, &repo, &owner, &explicitTeam, &r.Calls, &r.Sessions, &r.Tokens, &r.CostUSD, &r.UnpricedCalls, &r.Confidence); err != nil {
			return nil, err
		}
		r.Team, r.MappingSource, r.Confidence = resolveShowbackTeam(groups, r.Project, repo, owner, explicitTeam, machineName, gitAuthor)
		r.DataSource = "model_calls"
		mergeChargeback(agg, r)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return sortedChargeback(agg, limit), nil
}

func mergeChargeback(agg map[string]*ChargebackRow, row ChargebackRow) {
	if row.Team == "" {
		row.Team = "unassigned"
	}
	if row.Confidence <= 0 {
		row.Confidence = 0.3
	}
	key := strings.Join([]string{row.Team, row.Project, row.Source, row.Model}, "\x00")
	dst, ok := agg[key]
	if !ok {
		agg[key] = &row
		return
	}
	dst.Calls += row.Calls
	dst.Sessions += row.Sessions
	dst.Tokens += row.Tokens
	dst.CostUSD += row.CostUSD
	dst.UnpricedCalls += row.UnpricedCalls
	if dst.MappingSource != row.MappingSource {
		dst.MappingSource = "mixed"
	}
	if dst.DataSource != row.DataSource {
		dst.DataSource = "mixed"
	}
	if row.Confidence < dst.Confidence {
		dst.Confidence = row.Confidence
	}
}

func sortedChargeback(agg map[string]*ChargebackRow, limit int) []ChargebackRow {
	out := make([]ChargebackRow, 0, len(agg))
	for _, row := range agg {
		if row.Calls > 0 {
			row.AvgTokensCall = float64(row.Tokens) / float64(row.Calls)
			row.CostPerCall = row.CostUSD / float64(row.Calls)
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CostUSD != out[j].CostUSD {
			return out[i].CostUSD > out[j].CostUSD
		}
		if out[i].Tokens != out[j].Tokens {
			return out[i].Tokens > out[j].Tokens
		}
		if out[i].Calls != out[j].Calls {
			return out[i].Calls > out[j].Calls
		}
		return out[i].Team < out[j].Team
	})
	if len(out) > limit {
		return out[:limit]
	}
	if out == nil {
		return []ChargebackRow{}
	}
	return out
}

func resolveShowbackTeam(groups map[string]string, project, repo, owner, explicitTeam, machineName, gitAuthor string) (string, string, float64) {
	if strings.TrimSpace(explicitTeam) != "" {
		return strings.TrimSpace(explicitTeam), "workload.team", 1
	}
	match := bestTeamMapping(groups, map[string]string{
		"project": project,
		"repo":    repo,
		"path":    project,
		"owner":   owner,
		"author":  gitAuthor,
		"machine": machineName,
	})
	if match.team != "" {
		return match.team, match.source, match.confidence
	}
	return "unassigned", "unmapped", 0.3
}

type teamMatch struct {
	team       string
	source     string
	confidence float64
	score      int
}

func bestTeamMapping(groups map[string]string, values map[string]string) teamMatch {
	var best teamMatch
	for rawKey, rawTeam := range groups {
		team := strings.TrimSpace(rawTeam)
		if team == "" {
			continue
		}
		scope, key := splitMappingKey(rawKey)
		if key == "" {
			continue
		}
		scopes := []string{scope}
		if scope == "" {
			scopes = []string{"project", "repo", "path", "owner", "author", "machine"}
		}
		for _, candidateScope := range scopes {
			value := values[candidateScope]
			if value == "" {
				continue
			}
			if score := mappingScore(candidateScope, key, value); score > best.score {
				best = teamMatch{team: team, source: mappingSource(candidateScope, rawKey), confidence: mappingConfidence(candidateScope), score: score}
			}
		}
	}
	return best
}

func splitMappingKey(raw string) (string, string) {
	key := strings.TrimSpace(raw)
	if key == "" {
		return "", ""
	}
	if idx := strings.Index(key, ":"); idx > 0 {
		return strings.ToLower(strings.TrimSpace(key[:idx])), normalizeMappingValue(key[idx+1:])
	}
	return "", normalizeMappingValue(key)
}

func mappingScore(scope, key, value string) int {
	value = normalizeMappingValue(value)
	if key == "" || value == "" {
		return 0
	}
	if scope == "path" || strings.Contains(key, "/") || strings.Contains(key, "\\") {
		if value == key {
			return 2000 + len(key)
		}
		if strings.HasPrefix(value, strings.TrimRight(key, "/")+"/") {
			return 1000 + len(key)
		}
		return 0
	}
	if value == key {
		return 900 + len(key)
	}
	if scope == "" && strings.Contains(value, key) {
		return 100 + len(key)
	}
	return 0
}

func mappingSource(scope, rawKey string) string {
	rawKey = strings.TrimSpace(rawKey)
	if strings.Contains(rawKey, ":") {
		return rawKey
	}
	if scope == "" {
		return "groups:" + rawKey
	}
	return scope + ":" + rawKey
}

func mappingConfidence(scope string) float64 {
	switch scope {
	case "project", "repo", "owner", "author", "machine":
		return 0.9
	case "path":
		return 0.85
	default:
		return 0.8
	}
}

func normalizeMappingValue(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.ReplaceAll(value, "\\", "/")
	for strings.Contains(value, "//") {
		value = strings.ReplaceAll(value, "//", "/")
	}
	return strings.TrimRight(value, "/")
}
