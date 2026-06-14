package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func (s *Server) handleIngestionHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	data, err := s.db.GetIngestionHealth()
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSONWithPayloadETag(w, r, data)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	if s.options.Scan == nil {
		http.Error(w, "scan is not configured", http.StatusServiceUnavailable)
		return
	}
	source := r.URL.Query().Get("source")
	reset := r.URL.Query().Get("reset") == "1" || r.URL.Query().Get("reset") == "true"
	if reset && source == "" {
		badRequest(w, fmt.Errorf("reset scan requires a source"))
		return
	}
	if err := s.options.Scan(source, reset); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true, "source": source, "reset": reset})
}

func (s *Server) handleRecalculateCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "admin") {
		return
	}
	if s.options.Recalc == nil && s.options.RecalcMode == nil {
		http.Error(w, "recalculate costs is not configured", http.StatusServiceUnavailable)
		return
	}
	mode := r.URL.Query().Get("mode")
	if mode == "" {
		mode = "zero"
	}
	if s.options.RecalcMode != nil {
		if err := s.options.RecalcMode(mode); err != nil {
			serverError(w, err)
			return
		}
	} else {
		if err := s.options.Recalc(); err != nil {
			serverError(w, err)
			return
		}
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "costs.recalculate", mode, map[string]string{"mode": mode})
	writeJSON(w, map[string]interface{}{"ok": true, "mode": mode})
}

func (s *Server) handleRepairProjections(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "admin") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	result, err := s.db.RepairUsageProjections(from, to, source, model, project)
	if err != nil {
		serverError(w, err)
		return
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "projections.repair", source, map[string]string{
		"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "source": source, "model": model, "project": project,
		"inserted": fmt.Sprint(result.Inserted), "updated": fmt.Sprint(result.Updated),
	})
	writeJSON(w, map[string]interface{}{"ok": true, "result": result})
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	from, to, tzOffset, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	exportType := strings.ToLower(r.URL.Query().Get("type"))
	if exportType == "" {
		exportType = "sessions"
	}
	format := strings.ToLower(r.URL.Query().Get("format"))
	if format == "" {
		format = "csv"
	}
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	if !s.evaluateOperationPolicy(w, r, "export", source, model, project, exportType) {
		return
	}
	privacy := s.privacyFor(r)
	if s.options.Policies.RequirePrivacyExport && !privacy.ScreenshotMode && r.URL.Query().Get("privacy") != "1" && r.URL.Query().Get("privacy") != "true" {
		badRequest(w, fmt.Errorf("policy requires privacy=1 for exports"))
		return
	}

	var payload interface{}
	switch exportType {
	case "workloads":
		page, err := s.db.GetWorkloadsPage(from, to, source, model, project, "", "", 10000, 0)
		if err != nil {
			serverError(w, err)
			return
		}
		applyWorkloadPagePrivacy(page, privacy)
		payload = page.Rows
	case "sessions":
		page, err := s.db.GetSessionsPage(from, to, source, model, project, 10000, 0)
		if err != nil {
			serverError(w, err)
			return
		}
		applySessionPagePrivacy(page, privacy)
		payload = page.Rows
	case "daily":
		payload, err = s.db.GetTokensOverTimeFiltered(from, to, "1d", source, model, project, tzOffset)
		if err != nil {
			serverError(w, err)
			return
		}
	case "models":
		payload, err = s.db.GetCostByModelFiltered(from, to, source, project)
		if err != nil {
			serverError(w, err)
			return
		}
	case "model-calls":
		payload, err = s.db.GetModelCalls(from, to, source, model, project, 1000)
		if err != nil {
			serverError(w, err)
			return
		}
	case "chargeback":
		rows, err := s.db.GetChargeback(from, to, source, model, project, s.options.Teams.Groups, s.options.Teams.MachineName, s.options.Teams.GitAuthor, 1000)
		if err != nil {
			serverError(w, err)
			return
		}
		applyChargebackPrivacy(rows, privacy)
		payload = rows
	case "audit":
		rows, auditErr := s.db.QueryAuditLog(storage.AuditLogFilter{
			From:   from,
			To:     to,
			Actor:  r.URL.Query().Get("actor"),
			Role:   r.URL.Query().Get("role"),
			Action: r.URL.Query().Get("action"),
			Target: r.URL.Query().Get("target"),
			Limit:  1000,
		})
		err = auditErr
		if err != nil {
			serverError(w, err)
			return
		}
		applyAuditEventPrivacy(rows, privacy)
		payload = rows
	case "quality":
		payload, err = s.db.GetDataQuality(s.options.Pricing.StaleAfter)
		if err != nil {
			serverError(w, err)
			return
		}
	default:
		badRequest(w, fmt.Errorf("unsupported export type %q", exportType))
		return
	}

	filename := fmt.Sprintf("agent-ledger-%s-%s.%s", exportType, time.Now().Format("20060102-150405"), format)
	switch format {
	case "json":
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		s.appendAuditLog("local", s.roleFor(r), "export", exportType, map[string]string{"format": format, "privacy": fmt.Sprint(r.URL.Query().Get("privacy"))})
		writeJSON(w, payload)
	case "csv":
		body, err := csvFor(exportType, payload)
		if err != nil {
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		s.appendAuditLog("local", s.roleFor(r), "export", exportType, map[string]string{"format": format, "privacy": fmt.Sprint(r.URL.Query().Get("privacy"))})
		_, _ = w.Write(body)
	default:
		badRequest(w, fmt.Errorf("unsupported export format %q", format))
	}
}

func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	if !s.evaluateOperationPolicy(w, r, "report", source, model, project, "markdown") {
		return
	}
	stats, err := s.db.GetDashboardStatsFiltered(from, to, source, model, project)
	if err != nil {
		serverError(w, err)
		return
	}
	models, err := s.db.GetCostByModelFiltered(from, to, source, project)
	if err != nil {
		serverError(w, err)
		return
	}
	budgets, err := s.evaluateBudgets(time.Now())
	if err != nil {
		serverError(w, err)
		return
	}
	var b strings.Builder
	b.WriteString("# Agent Ledger report\n\n")
	b.WriteString(fmt.Sprintf("- Window: `%s` to `%s`\n", from.Format("2006-01-02"), to.Add(-time.Nanosecond).Format("2006-01-02")))
	b.WriteString(fmt.Sprintf("- Tokens: `%d`\n", stats.TotalTokens))
	b.WriteString(fmt.Sprintf("- Cost: `$%.4f`\n", stats.TotalCost))
	b.WriteString(fmt.Sprintf("- Sessions: `%d`\n", stats.TotalSessions))
	b.WriteString(fmt.Sprintf("- Prompts: `%d`\n\n", stats.TotalPrompts))
	b.WriteString("## Models\n\n| Model | Cost |\n|---|---:|\n")
	for _, row := range models {
		b.WriteString(fmt.Sprintf("| %s | $%.4f |\n", row.Model, row.Cost))
	}
	if len(budgets) > 0 {
		b.WriteString("\n## Budgets\n\n| Rule | Severity | Usage |\n|---|---|---:|\n")
		for _, row := range budgets {
			b.WriteString(fmt.Sprintf("| %s | %s | %.2f / %.2f |\n", row.Name, row.Severity, row.Value, row.Limit))
		}
	}
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	s.appendAuditLog("local", s.roleFor(r), "report", "markdown", map[string]string{"source": source, "model": model, "project": project})
	_, _ = w.Write([]byte(b.String()))
}

func (s *Server) handleOfflineBundleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "viewer") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	if !s.evaluateOperationPolicy(w, r, "export", source, model, project, "offline-bundle") {
		return
	}
	privacyLabel := "metadata-only"
	if r.URL.Query().Get("privacy") == "1" || r.URL.Query().Get("privacy") == "true" {
		privacyLabel = "redacted"
	}
	signed := r.URL.Query().Get("signed") == "1" || r.URL.Query().Get("signed") == "true"
	key := os.Getenv("AGENT_LEDGER_BUNDLE_KEY")
	signingKey := ""
	if signed && key == "" {
		badRequest(w, fmt.Errorf("AGENT_LEDGER_BUNDLE_KEY is required for signed bundle export"))
		return
	}
	if signed {
		signingKey = key
	}
	bundle, raw, err := s.db.BuildOfflineBundleWithOptions(
		from, to,
		source,
		model,
		project,
		privacyLabel,
		signingKey,
		r.URL.Query().Get("key_id"),
		parseLimit(r, 10000),
		s.canWriteDerivedData(),
	)
	if err != nil {
		serverError(w, err)
		return
	}
	s.appendAuditLog("local", s.roleFor(r), "offline_bundle.export", bundle.BundleID, map[string]string{"privacy": privacyLabel, "signed": fmt.Sprint(bundle.Integrity.Signature != "")})
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", "attachment; filename=agent-ledger-"+bundle.BundleID+".json")
	_, _ = w.Write(raw)
}

func (s *Server) handleOfflineBundleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
	if err != nil {
		badRequest(w, err)
		return
	}
	requireSignature := r.URL.Query().Get("verify") == "1" || r.URL.Query().Get("verify") == "true"
	key := os.Getenv("AGENT_LEDGER_BUNDLE_KEY")
	if requireSignature && key == "" {
		badRequest(w, fmt.Errorf("AGENT_LEDGER_BUNDLE_KEY is required for signature verification"))
		return
	}
	result, err := s.db.ImportOfflineBundle(raw, key, requireSignature)
	if err != nil {
		badRequest(w, err)
		return
	}
	s.appendAuditLog("local", s.roleFor(r), "offline_bundle.import", result.BundleID, map[string]string{"events_inserted": fmt.Sprint(result.EventsInserted), "signature_verified": fmt.Sprint(result.SignatureVerified)})
	writeJSON(w, map[string]interface{}{"ok": true, "result": result})
}

func csvFor(exportType string, payload interface{}) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	switch exportType {
	case "workloads":
		data, _ := json.Marshal(payload)
		var rows []map[string]interface{}
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		if err := w.Write([]string{"workload_id", "goal", "status", "source", "project", "repo", "git_branch", "model_calls", "tokens", "cost_usd", "outcome", "confidence", "updated_at"}); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err := w.Write([]string{
				fmt.Sprint(row["workload_id"]),
				fmt.Sprint(row["goal"]),
				fmt.Sprint(row["status"]),
				fmt.Sprint(row["source"]),
				fmt.Sprint(row["project"]),
				fmt.Sprint(row["repo"]),
				fmt.Sprint(row["git_branch"]),
				fmt.Sprint(row["model_calls"]),
				fmt.Sprint(row["tokens"]),
				fmt.Sprint(row["cost_usd"]),
				fmt.Sprint(row["outcome"]),
				fmt.Sprint(row["confidence"]),
				fmt.Sprint(row["updated_at"]),
			}); err != nil {
				return nil, err
			}
		}
	case "sessions":
		data, _ := json.Marshal(payload)
		var sessions []map[string]interface{}
		if err := json.Unmarshal(data, &sessions); err != nil {
			return nil, err
		}
		if err := w.Write([]string{"source", "session_id", "project", "cwd", "git_branch", "start_time", "prompts", "tokens", "total_cost"}); err != nil {
			return nil, err
		}
		for _, row := range sessions {
			if err := w.Write([]string{
				fmt.Sprint(row["source"]),
				fmt.Sprint(row["session_id"]),
				fmt.Sprint(row["project"]),
				fmt.Sprint(row["cwd"]),
				fmt.Sprint(row["git_branch"]),
				fmt.Sprint(row["start_time"]),
				fmt.Sprint(row["prompts"]),
				fmt.Sprint(row["tokens"]),
				fmt.Sprint(row["total_cost"]),
			}); err != nil {
				return nil, err
			}
		}
	case "daily":
		data, _ := json.Marshal(payload)
		var rows []map[string]interface{}
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		if err := w.Write([]string{"date", "input_tokens", "output_tokens", "cache_read", "cache_create"}); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err := w.Write([]string{
				fmt.Sprint(row["date"]),
				fmt.Sprint(row["input_tokens"]),
				fmt.Sprint(row["output_tokens"]),
				fmt.Sprint(row["cache_read"]),
				fmt.Sprint(row["cache_create"]),
			}); err != nil {
				return nil, err
			}
		}
	case "models":
		data, _ := json.Marshal(payload)
		var rows []map[string]interface{}
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		if err := w.Write([]string{"model", "cost"}); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err := w.Write([]string{fmt.Sprint(row["model"]), fmt.Sprint(row["cost"])}); err != nil {
				return nil, err
			}
		}
	case "model-calls":
		data, _ := json.Marshal(payload)
		var rows []map[string]interface{}
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		if err := w.Write([]string{"source", "model", "project", "calls", "tokens", "cost_usd", "avg_tokens_per_call", "cost_per_call", "unpriced_calls"}); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err := w.Write([]string{
				fmt.Sprint(row["source"]), fmt.Sprint(row["model"]), fmt.Sprint(row["project"]), fmt.Sprint(row["calls"]),
				fmt.Sprint(row["tokens"]), fmt.Sprint(row["cost_usd"]), fmt.Sprint(row["avg_tokens_per_call"]),
				fmt.Sprint(row["cost_per_call"]), fmt.Sprint(row["unpriced_calls"]),
			}); err != nil {
				return nil, err
			}
		}
	case "chargeback":
		data, _ := json.Marshal(payload)
		var rows []map[string]interface{}
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		if err := w.Write([]string{"team", "project", "source", "model", "calls", "sessions", "tokens", "cost_usd", "avg_tokens_per_call", "cost_per_call", "unpriced_calls", "mapping_source", "data_source", "confidence"}); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err := w.Write([]string{
				fmt.Sprint(row["team"]), fmt.Sprint(row["project"]), fmt.Sprint(row["source"]), fmt.Sprint(row["model"]),
				fmt.Sprint(row["calls"]), fmt.Sprint(row["sessions"]), fmt.Sprint(row["tokens"]), fmt.Sprint(row["cost_usd"]),
				fmt.Sprint(row["avg_tokens_per_call"]), fmt.Sprint(row["cost_per_call"]), fmt.Sprint(row["unpriced_calls"]),
				fmt.Sprint(row["mapping_source"]), fmt.Sprint(row["data_source"]), fmt.Sprint(row["confidence"]),
			}); err != nil {
				return nil, err
			}
		}
	case "audit":
		data, _ := json.Marshal(payload)
		var rows []map[string]interface{}
		if err := json.Unmarshal(data, &rows); err != nil {
			return nil, err
		}
		if err := w.Write([]string{"id", "actor", "role", "action", "target", "params", "created_at"}); err != nil {
			return nil, err
		}
		for _, row := range rows {
			if err := w.Write([]string{fmt.Sprint(row["id"]), fmt.Sprint(row["actor"]), fmt.Sprint(row["role"]), fmt.Sprint(row["action"]), fmt.Sprint(row["target"]), fmt.Sprint(row["params"]), fmt.Sprint(row["created_at"])}); err != nil {
				return nil, err
			}
		}
	case "quality":
		data, _ := json.Marshal(payload)
		var row map[string]interface{}
		if err := json.Unmarshal(data, &row); err != nil {
			return nil, err
		}
		if err := w.Write([]string{"generated_at", "pricing_sources", "unpriced_models"}); err != nil {
			return nil, err
		}
		if err := w.Write([]string{fmt.Sprint(row["generated_at"]), fmt.Sprint(row["pricing_sources"]), fmt.Sprint(row["unpriced_models"])}); err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
