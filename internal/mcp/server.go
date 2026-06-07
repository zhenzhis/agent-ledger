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
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	ledgerpolicy "github.com/zhenzhis/agent-ledger/internal/policy"
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

type resourceReadParams struct {
	URI string `json:"uri"`
}

type promptGetParams struct {
	Name      string            `json:"name"`
	Arguments map[string]string `json:"arguments"`
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
				"tools":     map[string]interface{}{"listChanged": false},
				"resources": map[string]interface{}{"subscribe": false, "listChanged": false},
				"prompts":   map[string]interface{}{"listChanged": false},
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
	case "resources/list":
		return map[string]interface{}{"resources": resources()}, nil
	case "resources/read":
		var params resourceReadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, err
		}
		return s.readResource(params.URI)
	case "prompts/list":
		return map[string]interface{}{"prompts": prompts()}, nil
	case "prompts/get":
		var params promptGetParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, err
		}
		return getPrompt(params.Name, params.Arguments)
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
		tool("ledger.start_run", "Start a new agent run attached to an existing workload.", map[string]interface{}{
			"workload_id": requiredStringSchema(),
			"source":      stringSchema(),
			"agent_name":  stringSchema(),
			"command":     stringSchema(),
			"cwd":         stringSchema(),
		}),
		tool("ledger.heartbeat_run", "Append a metadata-only liveness/progress heartbeat for an async agent run.", map[string]interface{}{
			"run_id":     requiredStringSchema(),
			"status":     stringSchema(),
			"phase":      stringSchema(),
			"progress":   numberSchema(),
			"message":    stringSchema(),
			"metrics":    objectSchema(),
			"event_id":   stringSchema(),
			"timestamp":  stringSchema(),
			"confidence": numberSchema(),
		}),
		tool("ledger.run_liveness", "Return active agent runs and whether their heartbeat is stale.", map[string]interface{}{
			"max_age":    stringSchema(),
			"stale_only": map[string]interface{}{"type": "boolean"},
			"limit":      integerSchema(),
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
		tool("ledger.integrations", "Return the Agent Ledger integration capability catalog.", map[string]interface{}{}),
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

func resources() []map[string]interface{} {
	return []map[string]interface{}{
		resource("agent-ledger://schema/canonical-events", "Canonical Event Schema", "Metadata-only event contract for workload, run, model-call, tool-call, artifact, evaluation, and policy events.", "application/json"),
		resource("agent-ledger://integrations/catalog", "Integration Capability Catalog", "Privacy-safe catalog of implemented, experimental, and planned integration surfaces.", "application/json"),
		resource("agent-ledger://budget/current", "Current Budget Windows", "Local quota and budget estimate for 5h/day/week/month windows.", "application/json"),
		resource("agent-ledger://workloads/recent", "Recent Workloads", "Recent workload summaries from the local ledger.", "application/json"),
		resource("agent-ledger://policies/status", "Policy Status", "Local policy configuration summary without prompt or secret content.", "application/json"),
	}
}

func resource(uri, name, description, mimeType string) map[string]interface{} {
	return map[string]interface{}{"uri": uri, "name": name, "description": description, "mimeType": mimeType}
}

func prompts() []map[string]interface{} {
	return []map[string]interface{}{
		prompt("agent-ledger/workload-brief", "Plan an agent workload with explicit goal, context boundaries, budget awareness, and ledger instrumentation.", []map[string]string{
			{"name": "goal", "description": "The workload goal to execute or review.", "required": "true"},
			{"name": "project", "description": "Project, repo, or workspace label.", "required": "false"},
			{"name": "constraints", "description": "Operational, privacy, policy, or budget constraints.", "required": "false"},
		}),
		prompt("agent-ledger/cost-review", "Review local token/cost usage with privacy-safe Agent Ledger resources and tools.", []map[string]string{
			{"name": "period", "description": "Time window such as today, week, month, or YYYY-MM-DD..YYYY-MM-DD.", "required": "false"},
			{"name": "project", "description": "Optional project or repo filter.", "required": "false"},
		}),
		prompt("agent-ledger/incident-evidence", "Prepare a privacy-safe data quality, pricing, or usage anomaly evidence bundle.", []map[string]string{
			{"name": "issue", "description": "Observed discrepancy or failure mode.", "required": "true"},
			{"name": "period", "description": "Relevant time window.", "required": "false"},
		}),
	}
}

func prompt(name, description string, args []map[string]string) map[string]interface{} {
	outArgs := make([]map[string]interface{}, 0, len(args))
	for _, arg := range args {
		outArgs = append(outArgs, map[string]interface{}{
			"name":        arg["name"],
			"description": arg["description"],
			"required":    arg["required"] == "true",
		})
	}
	return map[string]interface{}{"name": name, "description": description, "arguments": outArgs}
}

func (s *Server) callTool(name string, args json.RawMessage) (interface{}, error) {
	switch name {
	case "ledger.current_budget":
		return s.toolCurrentBudget(args)
	case "ledger.start_workload":
		return s.toolStartWorkload(args)
	case "ledger.start_run":
		return s.toolStartRun(args)
	case "ledger.close_workload":
		return s.toolCloseWorkload(args)
	case "ledger.heartbeat_run":
		return s.toolHeartbeatRun(args)
	case "ledger.run_liveness":
		return s.toolRunLiveness(args)
	case "ledger.record_artifact":
		return s.toolRecordArtifact(args)
	case "ledger.record_event":
		return s.toolRecordEvent(args)
	case "ledger.event_schema":
		return storage.CanonicalEventSchema(), nil
	case "ledger.integrations":
		return integrations.Registry(integrations.OptionsFromConfig(s.cfg)), nil
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

func (s *Server) readResource(uri string) (interface{}, error) {
	var payload interface{}
	switch uri {
	case "agent-ledger://schema/canonical-events":
		payload = storage.CanonicalEventSchema()
	case "agent-ledger://integrations/catalog":
		payload = integrations.Registry(integrations.OptionsFromConfig(s.cfg))
	case "agent-ledger://budget/current":
		budget, err := s.toolCurrentBudget([]byte(`{"window":"all"}`))
		if err != nil {
			return nil, err
		}
		payload = budget
	case "agent-ledger://workloads/recent":
		now := s.now()
		page, err := s.db.GetWorkloadsPage(now.AddDate(0, 0, -30), now.AddDate(0, 0, 1), "", "", "", "", "", 20, 0)
		if err != nil {
			return nil, err
		}
		payload = page
	case "agent-ledger://policies/status":
		payload = map[string]interface{}{
			"enabled":                s.cfg.Policies.Enabled,
			"require_privacy_export": s.cfg.Policies.RequirePrivacyExport,
			"rules":                  s.cfg.Policies.Rules,
		}
	default:
		return nil, fmt.Errorf("unknown resource %q", uri)
	}
	raw, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"contents": []map[string]string{{
			"uri":      uri,
			"mimeType": "application/json",
			"text":     string(raw),
		}},
	}, nil
}

func getPrompt(name string, args map[string]string) (interface{}, error) {
	switch name {
	case "agent-ledger/workload-brief":
		return promptResponse(name, fmt.Sprintf(`Use Agent Ledger as the local workload control plane.

Goal: %s
Project: %s
Constraints: %s

Start by checking ledger resources for schema, policy, and budget. If execution begins, create or attach a workload, record metadata-only events, and avoid sending prompt content, secrets, raw file contents, or private paths into durable ledger fields. Keep cost, cache, model choice, and policy decisions explainable.`, promptArg(args, "goal", "<unset>"), promptArg(args, "project", "<unset>"), promptArg(args, "constraints", "<unset>"))), nil
	case "agent-ledger/cost-review":
		return promptResponse(name, fmt.Sprintf(`Review Agent Ledger usage for period %s and project %s.

Use budget, cost-intelligence, integrations, and recent workload resources/tools. Explain cost drivers by model, cache behavior, project attribution, and anomalies. Do not inspect or request prompt content. Produce concise findings with concrete next actions.`, promptArg(args, "period", "recent"), promptArg(args, "project", "all"))), nil
	case "agent-ledger/incident-evidence":
		return promptResponse(name, fmt.Sprintf(`Prepare a privacy-safe Agent Ledger incident evidence summary.

Issue: %s
Period: %s

Use diagnostics, integration catalog, pricing/data-quality resources, audit metadata, and evidence/export tools where available. Redact paths, session IDs, project names, machine names, and authors unless explicitly authorized. Separate verified facts from hypotheses and list exact remediation steps.`, promptArg(args, "issue", "<unset>"), promptArg(args, "period", "recent"))), nil
	default:
		return nil, fmt.Errorf("unknown prompt %q", name)
	}
}

func promptResponse(description, text string) map[string]interface{} {
	return map[string]interface{}{
		"description": description,
		"messages": []map[string]interface{}{{
			"role": "user",
			"content": map[string]string{
				"type": "text",
				"text": text,
			},
		}},
	}
}

func promptArg(args map[string]string, key, fallback string) string {
	if args == nil {
		return fallback
	}
	if strings.TrimSpace(args[key]) == "" {
		return fallback
	}
	return args[key]
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

func (s *Server) toolStartRun(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID string `json:"workload_id"`
		Source     string `json:"source"`
		AgentName  string `json:"agent_name"`
		Command    string `json:"command"`
		CWD        string `json:"cwd"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	runID, err := s.db.StartAgentRun(in.WorkloadID, in.Source, firstNonEmpty(in.AgentName, in.Source, "agent"), in.Command, in.CWD)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"workload_id": in.WorkloadID, "run_id": runID, "status": "running"}, nil
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

func (s *Server) toolHeartbeatRun(args json.RawMessage) (interface{}, error) {
	var in struct {
		RunID      string                 `json:"run_id"`
		Status     string                 `json:"status"`
		Phase      string                 `json:"phase"`
		Progress   float64                `json:"progress"`
		Message    string                 `json:"message"`
		Metrics    map[string]interface{} `json:"metrics"`
		EventID    string                 `json:"event_id"`
		Timestamp  string                 `json:"timestamp"`
		Confidence float64                `json:"confidence"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	var ts time.Time
	if in.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339Nano, in.Timestamp)
		if err != nil {
			return nil, err
		}
		ts = parsed
	}
	if in.Confidence <= 0 {
		in.Confidence = 1
	}
	return s.db.RecordAgentRunHeartbeat(in.EventID, in.RunID, in.Status, in.Phase, in.Message, in.Progress, in.Metrics, ts, in.Confidence)
}

func (s *Server) toolRunLiveness(args json.RawMessage) (interface{}, error) {
	var in struct {
		MaxAge    string `json:"max_age"`
		StaleOnly bool   `json:"stale_only"`
		Limit     int    `json:"limit"`
	}
	_ = json.Unmarshal(args, &in)
	maxAge := 10 * time.Minute
	if in.MaxAge != "" {
		parsed, err := time.ParseDuration(in.MaxAge)
		if err != nil {
			return nil, err
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("max_age must be positive")
		}
		maxAge = parsed
	}
	rows, err := s.db.GetAgentRunLiveness(maxAge, in.StaleOnly, in.Limit)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"rows": rows, "max_age": maxAge.String(), "stale_only": in.StaleOnly}, nil
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
	result := ledgerpolicy.Evaluate(s.cfg.Policies, ledgerpolicy.Request{
		WorkloadID: in.WorkloadID,
		RunID:      in.RunID,
		Source:     in.Source,
		Model:      in.Model,
		Project:    in.Project,
		Action:     in.Action,
		Role:       in.Role,
	})
	for i := range result.Decisions {
		if in.WorkloadID != "" {
			id, err := s.db.RecordPolicyDecision(in.WorkloadID, in.RunID, result.Decisions[i].Rule, result.Decisions[i].Action, result.Decisions[i].Message, firstNonEmpty(in.Role, "agent"))
			if err != nil {
				return nil, err
			}
			result.Decisions[i].DecisionID = id
		}
	}
	return result, nil
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

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
