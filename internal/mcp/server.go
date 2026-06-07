package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// Server implements a small MCP-compatible stdio JSON-RPC tool surface.
type Server struct {
	db  *storage.DB
	cfg *config.Config
	now func() time.Time
}

// New creates a stdio MCP server.
func New(db *storage.DB, cfg *config.Config) *Server {
	return &Server{db: db, cfg: cfg, now: time.Now}
}

type request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type toolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// Serve reads newline-delimited JSON-RPC requests from r and writes responses to w.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(w)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = enc.Encode(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: err.Error()}})
			continue
		}
		if len(req.ID) == 0 && strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		resp := response{JSONRPC: "2.0", ID: req.ID}
		result, err := s.handle(req)
		if err != nil {
			resp.Error = &rpcError{Code: -32000, Message: err.Error()}
		} else {
			resp.Result = result
		}
		if len(req.ID) != 0 {
			if err := enc.Encode(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func (s *Server) handle(req request) (interface{}, error) {
	switch req.Method {
	case "initialize":
		return map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools": map[string]interface{}{"listChanged": false},
			},
			"serverInfo": map[string]string{"name": "agent-ledger", "version": "dev"},
		}, nil
	case "tools/list":
		return map[string]interface{}{"tools": tools()}, nil
	case "tools/call":
		var params toolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, err
		}
		payload, err := s.callTool(params.Name, params.Arguments)
		if err != nil {
			return nil, err
		}
		raw, _ := json.MarshalIndent(payload, "", "  ")
		return map[string]interface{}{
			"content": []map[string]string{{"type": "text", "text": string(raw)}},
		}, nil
	case "ping":
		return map[string]bool{"ok": true}, nil
	default:
		return nil, fmt.Errorf("unsupported method %q", req.Method)
	}
}

func tools() []map[string]interface{} {
	return []map[string]interface{}{
		tool("ledger.current_budget", "Return local quota/budget consumption for 5h/day/week/month windows.", map[string]interface{}{
			"window":  enumSchema([]string{"5h", "day", "week", "month", "all"}),
			"source":  stringSchema(),
			"model":   stringSchema(),
			"project": stringSchema(),
		}),
		tool("ledger.start_workload", "Create a workload and optionally attach an initial agent run.", map[string]interface{}{
			"goal":       requiredStringSchema(),
			"source":     stringSchema(),
			"project":    stringSchema(),
			"repo":       stringSchema(),
			"git_branch": stringSchema(),
			"owner":      stringSchema(),
			"team":       stringSchema(),
			"budget_usd": numberSchema(),
			"agent_name": stringSchema(),
			"command":    stringSchema(),
			"cwd":        stringSchema(),
		}),
		tool("ledger.close_workload", "Close a workload with status and outcome.", map[string]interface{}{
			"workload_id": requiredStringSchema(),
			"status":      stringSchema(),
			"outcome":     stringSchema(),
		}),
		tool("ledger.record_artifact", "Record a privacy-safe artifact reference by hash or label.", map[string]interface{}{
			"workload_id":   requiredStringSchema(),
			"run_id":        stringSchema(),
			"artifact_type": stringSchema(),
			"label":         stringSchema(),
			"path_hash":     stringSchema(),
			"sha256":        stringSchema(),
			"metadata":      objectSchema(),
		}),
		tool("ledger.record_event", "Ingest one canonical metadata-only event into the workload ledger.", map[string]interface{}{
			"source":          requiredStringSchema(),
			"event_type":      requiredStringSchema(),
			"event_id":        stringSchema(),
			"source_event_id": stringSchema(),
			"workload_id":     stringSchema(),
			"agent_run_id":    stringSchema(),
			"session_id":      stringSchema(),
			"model":           stringSchema(),
			"project":         stringSchema(),
			"git_branch":      stringSchema(),
			"timestamp":       stringSchema(),
			"payload":         objectSchema(),
		}),
		tool("ledger.event_schema", "Return the canonical metadata-only event schema and supported event types.", map[string]interface{}{}),
		tool("ledger.get_policy", "Evaluate local advisory policy rules for a proposed agent action.", map[string]interface{}{
			"workload_id": stringSchema(),
			"run_id":      stringSchema(),
			"source":      stringSchema(),
			"model":       stringSchema(),
			"project":     stringSchema(),
			"action":      stringSchema(),
			"role":        stringSchema(),
		}),
		tool("ledger.explain_cost", "Explain expensive sessions without reading prompt content.", map[string]interface{}{
			"from":    stringSchema(),
			"to":      stringSchema(),
			"source":  stringSchema(),
			"model":   stringSchema(),
			"project": stringSchema(),
			"limit":   integerSchema(),
		}),
		tool("ledger.find_similar_workloads", "Find recent workloads by goal, repo, project, branch, or id.", map[string]interface{}{
			"query":   stringSchema(),
			"source":  stringSchema(),
			"model":   stringSchema(),
			"project": stringSchema(),
			"limit":   integerSchema(),
		}),
	}
}

func tool(name, description string, props map[string]interface{}) map[string]interface{} {
	required := []string{}
	for k, v := range props {
		if m, ok := v.(map[string]interface{}); ok {
			if req, _ := m["x-required"].(bool); req {
				required = append(required, k)
				delete(m, "x-required")
			}
		}
	}
	return map[string]interface{}{
		"name":        name,
		"description": description,
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": props,
			"required":   required,
		},
	}
}

func stringSchema() map[string]interface{} { return map[string]interface{}{"type": "string"} }
func requiredStringSchema() map[string]interface{} {
	return map[string]interface{}{"type": "string", "x-required": true}
}
func numberSchema() map[string]interface{}  { return map[string]interface{}{"type": "number"} }
func integerSchema() map[string]interface{} { return map[string]interface{}{"type": "integer"} }
func objectSchema() map[string]interface{}  { return map[string]interface{}{"type": "object"} }
func enumSchema(values []string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "enum": values}
}

func (s *Server) callTool(name string, args json.RawMessage) (interface{}, error) {
	switch name {
	case "ledger.current_budget":
		return s.toolCurrentBudget(args)
	case "ledger.start_workload":
		return s.toolStartWorkload(args)
	case "ledger.close_workload":
		return s.toolCloseWorkload(args)
	case "ledger.record_artifact":
		return s.toolRecordArtifact(args)
	case "ledger.record_event":
		return s.toolRecordEvent(args)
	case "ledger.event_schema":
		return storage.CanonicalEventSchema(), nil
	case "ledger.get_policy":
		return s.toolGetPolicy(args)
	case "ledger.explain_cost":
		return s.toolExplainCost(args)
	case "ledger.find_similar_workloads":
		return s.toolFindSimilarWorkloads(args)
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func (s *Server) toolCurrentBudget(args json.RawMessage) (interface{}, error) {
	var in struct {
		Window  string `json:"window"`
		Source  string `json:"source"`
		Model   string `json:"model"`
		Project string `json:"project"`
	}
	_ = json.Unmarshal(args, &in)
	now := s.now()
	windows := []string{"5h", "day", "week", "month"}
	if in.Window != "" && in.Window != "all" {
		windows = []string{in.Window}
	}
	type budgetWindowResult struct {
		Name             string  `json:"name"`
		From             string  `json:"from"`
		To               string  `json:"to"`
		CostUSD          float64 `json:"cost_usd"`
		Tokens           int64   `json:"tokens"`
		Prompts          int     `json:"prompts"`
		Calls            int     `json:"calls"`
		CostLimit        float64 `json:"cost_limit"`
		TokenLimit       int64   `json:"token_limit"`
		RemainingCostUSD float64 `json:"remaining_cost_usd"`
		RemainingTokens  int64   `json:"remaining_tokens"`
		BurnRatePerHour  float64 `json:"burn_rate_per_hour"`
	}
	out := []budgetWindowResult{}
	for _, name := range windows {
		from, to := quotaWindow(now, name)
		stats, err := s.db.GetDashboardStatsFiltered(from, to, in.Source, in.Model, in.Project)
		if err != nil {
			return nil, err
		}
		costLimit, tokenLimit := s.quotaLimits(name)
		hours := math.Max(1, now.Sub(from).Hours())
		out = append(out, budgetWindowResult{
			Name: name, From: from.Format(time.RFC3339), To: to.Format(time.RFC3339),
			CostUSD: stats.TotalCost, Tokens: stats.TotalTokens, Prompts: stats.TotalPrompts, Calls: stats.TotalCalls,
			CostLimit: costLimit, TokenLimit: tokenLimit,
			RemainingCostUSD: costLimit - stats.TotalCost,
			RemainingTokens:  tokenLimit - stats.TotalTokens,
			BurnRatePerHour:  stats.TotalCost / hours,
		})
	}
	return map[string]interface{}{
		"enabled": s.cfg.Quota.Enabled,
		"plan":    s.cfg.Quota.Plan,
		"method":  "local-estimate",
		"windows": out,
	}, nil
}

func (s *Server) toolStartWorkload(args json.RawMessage) (interface{}, error) {
	var in struct {
		Goal      string  `json:"goal"`
		Source    string  `json:"source"`
		Project   string  `json:"project"`
		Repo      string  `json:"repo"`
		GitBranch string  `json:"git_branch"`
		Owner     string  `json:"owner"`
		Team      string  `json:"team"`
		BudgetUSD float64 `json:"budget_usd"`
		AgentName string  `json:"agent_name"`
		Command   string  `json:"command"`
		CWD       string  `json:"cwd"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	id, err := s.db.CreateWorkload(in.Goal, in.Source, in.Project, in.Repo, in.GitBranch, in.Owner, in.Team, in.BudgetUSD)
	if err != nil {
		return nil, err
	}
	runID := ""
	if in.AgentName != "" || in.Command != "" || in.CWD != "" {
		runID, err = s.db.StartAgentRun(id, in.Source, firstNonEmpty(in.AgentName, in.Source, "agent"), in.Command, in.CWD)
		if err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"workload_id": id, "run_id": runID, "status": "active"}, nil
}

func (s *Server) toolCloseWorkload(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID string `json:"workload_id"`
		Status     string `json:"status"`
		Outcome    string `json:"outcome"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if err := s.db.CloseWorkload(in.WorkloadID, in.Status, in.Outcome); err != nil {
		return nil, err
	}
	return map[string]interface{}{"workload_id": in.WorkloadID, "status": firstNonEmpty(in.Status, "completed")}, nil
}

func (s *Server) toolRecordArtifact(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID   string                 `json:"workload_id"`
		RunID        string                 `json:"run_id"`
		ArtifactType string                 `json:"artifact_type"`
		Label        string                 `json:"label"`
		PathHash     string                 `json:"path_hash"`
		SHA256       string                 `json:"sha256"`
		Metadata     map[string]interface{} `json:"metadata"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	meta, _ := json.Marshal(in.Metadata)
	id, err := s.db.RecordArtifact(in.WorkloadID, in.RunID, in.ArtifactType, in.Label, in.PathHash, in.SHA256, string(meta), 1)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"artifact_id": id, "workload_id": in.WorkloadID}, nil
}

func (s *Server) toolRecordEvent(args json.RawMessage) (interface{}, error) {
	var event storage.CanonicalEvent
	if err := json.Unmarshal(args, &event); err != nil {
		return nil, err
	}
	return s.db.IngestCanonicalEvent(event)
}

func (s *Server) toolGetPolicy(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID string `json:"workload_id"`
		RunID      string `json:"run_id"`
		Source     string `json:"source"`
		Model      string `json:"model"`
		Project    string `json:"project"`
		Action     string `json:"action"`
		Role       string `json:"role"`
	}
	_ = json.Unmarshal(args, &in)
	if !s.cfg.Policies.Enabled {
		return map[string]interface{}{
			"enabled":        false,
			"action":         "allow",
			"decisions":      []map[string]string{},
			"webhooks":       "disabled-by-default",
			"privacy_export": s.cfg.Policies.RequirePrivacyExport,
		}, nil
	}
	decisions := []map[string]string{}
	finalAction := "allow"
	for _, rule := range s.cfg.Policies.Rules {
		if !policyMatches(rule, in.Source, in.Model, in.Project, in.Action) {
			continue
		}
		action := normalizedPolicyAction(rule.Action)
		if policyRank(action) > policyRank(finalAction) {
			finalAction = action
		}
		decisionID := ""
		if in.WorkloadID != "" {
			id, err := s.db.RecordPolicyDecision(in.WorkloadID, in.RunID, rule.Name, action, rule.Message, firstNonEmpty(in.Role, "agent"))
			if err != nil {
				return nil, err
			}
			decisionID = id
		}
		decisions = append(decisions, map[string]string{
			"decision_id": decisionID,
			"rule":        rule.Name,
			"scope":       rule.Scope,
			"match":       rule.Match,
			"action":      action,
			"message":     rule.Message,
		})
	}
	return map[string]interface{}{
		"enabled":        s.cfg.Policies.Enabled,
		"action":         finalAction,
		"decisions":      decisions,
		"webhooks":       "disabled-by-default",
		"privacy_export": s.cfg.Policies.RequirePrivacyExport,
	}, nil
}

func (s *Server) toolExplainCost(args json.RawMessage) (interface{}, error) {
	var in struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Source  string `json:"source"`
		Model   string `json:"model"`
		Project string `json:"project"`
		Limit   int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &in)
	from, to, err := parseDateRange(in.From, in.To, s.now())
	if err != nil {
		return nil, err
	}
	rows, err := s.db.GetCostIntelligence(from, to, in.Source, in.Model, in.Project, in.Limit)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339), "rows": rows}, nil
}

func (s *Server) toolFindSimilarWorkloads(args json.RawMessage) (interface{}, error) {
	var in struct {
		Query   string `json:"query"`
		Source  string `json:"source"`
		Model   string `json:"model"`
		Project string `json:"project"`
		Limit   int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &in)
	if in.Limit <= 0 || in.Limit > 50 {
		in.Limit = 10
	}
	now := s.now()
	page, err := s.db.GetWorkloadsPage(now.AddDate(-1, 0, 0), now.AddDate(0, 0, 1), in.Source, in.Model, in.Project, "", in.Query, in.Limit, 0)
	if err != nil {
		return nil, err
	}
	return page, nil
}

func (s *Server) quotaLimits(name string) (float64, int64) {
	cost := s.cfg.Quota.MonthlyBudget
	tokens := s.cfg.Quota.TokenBudget
	switch name {
	case "5h":
		return cost / 30 / 24 * 5, tokens / 30 / 24 * 5
	case "day":
		return cost / 30, tokens / 30
	case "week":
		return cost / 4.35, tokens / 4
	default:
		return cost, tokens
	}
}

func quotaWindow(now time.Time, name string) (time.Time, time.Time) {
	y, m, d := now.Date()
	loc := now.Location()
	switch name {
	case "5h":
		return now.Add(-5 * time.Hour), now
	case "week":
		start := time.Date(y, m, d, 0, 0, 0, 0, loc)
		start = start.AddDate(0, 0, -int((start.Weekday()+6)%7))
		return start, start.AddDate(0, 0, 7)
	case "month":
		start := time.Date(y, m, 1, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 1, 0)
	default:
		start := time.Date(y, m, d, 0, 0, 0, 0, loc)
		return start, start.AddDate(0, 0, 1)
	}
}

func parseDateRange(fromRaw, toRaw string, now time.Time) (time.Time, time.Time, error) {
	if fromRaw == "" {
		return now.AddDate(0, 0, -30), now, nil
	}
	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if toRaw == "" {
		return from, from.AddDate(0, 0, 1), nil
	}
	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to.AddDate(0, 0, 1), nil
}

func policyMatches(rule config.PolicyRule, source, model, project, action string) bool {
	scope := strings.ToLower(rule.Scope)
	match := strings.ToLower(strings.TrimSpace(rule.Match))
	if match == "" || match == "*" {
		return true
	}
	switch scope {
	case "source":
		return strings.EqualFold(source, rule.Match)
	case "model":
		return strings.EqualFold(model, rule.Match)
	case "project":
		return strings.Contains(strings.ToLower(project), match)
	case "action":
		return strings.EqualFold(action, rule.Match)
	default:
		return true
	}
}

func normalizedPolicyAction(action string) string {
	switch strings.ToLower(action) {
	case "block", "deny":
		return "block"
	case "approval", "require_approval", "approve":
		return "require_approval"
	case "warn", "warning":
		return "warn"
	default:
		return "allow"
	}
}

func policyRank(action string) int {
	switch action {
	case "block":
		return 3
	case "require_approval":
		return 2
	case "warn":
		return 1
	default:
		return 0
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
