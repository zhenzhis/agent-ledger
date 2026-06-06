package server

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
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
	writeJSON(w, data)
}

func (s *Server) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) {
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
	if !s.requireLocalOrAuth(w, r) {
		return
	}
	if s.options.Recalc == nil {
		http.Error(w, "recalculate costs is not configured", http.StatusServiceUnavailable)
		return
	}
	if err := s.options.Recalc(); err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, map[string]interface{}{"ok": true})
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
	privacy := s.privacyFor(r)

	var payload interface{}
	switch exportType {
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
	default:
		badRequest(w, fmt.Errorf("unsupported export type %q", exportType))
		return
	}

	filename := fmt.Sprintf("agent-ledger-%s-%s.%s", exportType, time.Now().Format("20060102-150405"), format)
	switch format {
	case "json":
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		writeJSON(w, payload)
	case "csv":
		body, err := csvFor(exportType, payload)
		if err != nil {
			serverError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
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
	_, _ = w.Write([]byte(b.String()))
}

func csvFor(exportType string, payload interface{}) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	switch exportType {
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
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}
