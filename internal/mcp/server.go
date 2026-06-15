package mcp

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/controlplane"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	ledgerpolicy "github.com/zhenzhis/agent-ledger/internal/policy"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// Server implements a small MCP-compatible stdio JSON-RPC tool surface.
type Server struct {
	db                   *storage.DB
	cfg                  *config.Config
	now                  func() time.Time
	subscriptionInterval time.Duration
}

// New creates a stdio MCP server.
func New(db *storage.DB, cfg *config.Config) *Server {
	return &Server{db: db, cfg: cfg, now: time.Now, subscriptionInterval: 5 * time.Second}
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

type subscriptionState struct {
	server      *Server
	enc         *json.Encoder
	mu          sync.Mutex
	done        chan struct{}
	subscribed  map[string]string
	pollStarted bool
}

func newSubscriptionState(server *Server, enc *json.Encoder) *subscriptionState {
	return &subscriptionState{
		server:     server,
		enc:        enc,
		done:       make(chan struct{}),
		subscribed: map[string]string{},
	}
}

func (s *subscriptionState) encode(v interface{}) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enc.Encode(v)
}

func (s *subscriptionState) subscribe(uri string) (map[string]interface{}, error) {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return nil, fmt.Errorf("uri is required")
	}
	cursor, err := s.server.resourceCursor(uri)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	s.subscribed[uri] = cursor
	if !s.pollStarted {
		s.pollStarted = true
		go s.poll()
	}
	s.mu.Unlock()
	return map[string]interface{}{"ok": true, "uri": uri, "cursor": cursor, "mode": "local-poll"}, nil
}

func (s *subscriptionState) unsubscribe(uri string) map[string]interface{} {
	uri = strings.TrimSpace(uri)
	s.mu.Lock()
	_, existed := s.subscribed[uri]
	delete(s.subscribed, uri)
	s.mu.Unlock()
	return map[string]interface{}{"ok": true, "uri": uri, "subscribed": existed}
}

func (s *subscriptionState) stop() {
	select {
	case <-s.done:
	default:
		close(s.done)
	}
}

func (s *subscriptionState) poll() {
	interval := s.server.subscriptionInterval
	if interval <= 0 {
		interval = 5 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.pollOnce()
		case <-s.done:
			return
		}
	}
}

func (s *subscriptionState) pollOnce() {
	s.mu.Lock()
	uris := make([]string, 0, len(s.subscribed))
	for uri := range s.subscribed {
		uris = append(uris, uri)
	}
	s.mu.Unlock()
	for _, uri := range uris {
		cursor, err := s.server.resourceCursor(uri)
		if err != nil {
			continue
		}
		s.mu.Lock()
		previous, ok := s.subscribed[uri]
		if !ok || previous == cursor {
			s.mu.Unlock()
			continue
		}
		s.subscribed[uri] = cursor
		s.mu.Unlock()
		_ = s.encode(map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "notifications/resources/updated",
			"params": map[string]string{
				"uri":    uri,
				"cursor": cursor,
			},
		})
	}
}

// Serve reads newline-delimited JSON-RPC requests from r and writes responses to w.
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(w)
	subscriptions := newSubscriptionState(s, enc)
	defer subscriptions.stop()
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			_ = subscriptions.encode(response{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: err.Error()}})
			continue
		}
		if len(req.ID) == 0 && strings.HasPrefix(req.Method, "notifications/") {
			continue
		}
		resp := response{JSONRPC: "2.0", ID: req.ID}
		result, err := s.handle(req, subscriptions)
		if err != nil {
			resp.Error = &rpcError{Code: -32000, Message: err.Error()}
		} else {
			resp.Result = result
		}
		if len(req.ID) != 0 {
			if err := subscriptions.encode(resp); err != nil {
				return err
			}
		}
	}
	return scanner.Err()
}

func (s *Server) handle(req request, subscriptions *subscriptionState) (interface{}, error) {
	switch req.Method {
	case "initialize":
		return map[string]interface{}{
			"protocolVersion": "2024-11-05",
			"capabilities": map[string]interface{}{
				"tools":     map[string]interface{}{"listChanged": false},
				"resources": map[string]interface{}{"subscribe": true, "listChanged": false},
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
	case "resources/subscribe":
		var params resourceReadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, err
		}
		return subscriptions.subscribe(params.URI)
	case "resources/unsubscribe":
		var params resourceReadParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, err
		}
		return subscriptions.unsubscribe(params.URI), nil
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

// ToolNames returns the actual MCP tool names exposed by the server.
func ToolNames() []string {
	out := make([]string, 0, len(tools()))
	for _, tool := range tools() {
		if name, _ := tool["name"].(string); name != "" {
			out = append(out, name)
		}
	}
	return out
}

// ResourceURIs returns the actual MCP resource URIs exposed by the server.
func ResourceURIs() []string {
	out := make([]string, 0, len(resources()))
	for _, resource := range resources() {
		if uri, _ := resource["uri"].(string); uri != "" {
			out = append(out, uri)
		}
	}
	return out
}

// PromptNames returns the actual MCP prompt names exposed by the server.
func PromptNames() []string {
	out := make([]string, 0, len(prompts()))
	for _, prompt := range prompts() {
		if name, _ := prompt["name"].(string); name != "" {
			out = append(out, name)
		}
	}
	return out
}

func tools() []map[string]interface{} {
	return []map[string]interface{}{
		tool("ledger.current_budget", "Return local quota/budget consumption for 5h/day/week/month windows.", map[string]interface{}{
			"window":  enumSchema([]string{"5h", "day", "week", "month", "all"}),
			"source":  stringSchema(),
			"model":   stringSchema(),
			"project": stringSchema(),
		}),
		tool("ledger.discovery", "Return the local Agent Ledger discovery manifest with runtime, schema, adapter, and conformance entrypoints.", map[string]interface{}{}),
		tool("ledger.contracts", "Return the Agent Ledger contract bundle with URI, hash, ETag, CLI, and MCP entrypoint metadata.", map[string]interface{}{}),
		tool("ledger.contracts_verify", "Return the Agent Ledger control-plane contract self-check report for CI, wrappers, and routers.", map[string]interface{}{}),
		tool("ledger.goal_coverage", "Return the Agent Ledger product goal coverage report with requirement-level evidence and remaining external dependencies.", map[string]interface{}{}),
		tool("ledger.openapi", "Return the metadata-only OpenAPI 3.1 contract for stable Agent Ledger control-plane endpoints.", map[string]interface{}{}),
		tool("ledger.runtime_status", "Return process-level runtime mode, read-only state, background task state, and write-operation status.", map[string]interface{}{}),
		tool("ledger.config_status", "Return privacy-safe deployment configuration status without paths, secrets, webhook URLs, prompt content, or session ids.", map[string]interface{}{}),
		tool("ledger.readiness", "Return privacy-safe control-plane readiness for wrappers, routers, CI, and deployment checks.", map[string]interface{}{}),
		tool("ledger.admission_check", "Dry-run whether an HTTP, CLI, or MCP operation is allowed in the current runtime.", map[string]interface{}{
			"surface":         enumSchema([]string{"http", "cli", "mcp"}),
			"method":          stringSchema(),
			"path":            stringSchema(),
			"command":         stringSchema(),
			"tool":            stringSchema(),
			"role":            stringSchema(),
			"dry_run":         booleanSchema(),
			"record":          booleanSchema(),
			"has_workload_id": booleanSchema(),
		}),
		tool("ledger.start_workload", "Create a workload and optionally attach an initial agent run.", map[string]interface{}{
			"goal":            requiredStringSchema(),
			"source":          stringSchema(),
			"project":         stringSchema(),
			"repo":            stringSchema(),
			"git_branch":      stringSchema(),
			"owner":           stringSchema(),
			"team":            stringSchema(),
			"budget_usd":      numberSchema(),
			"agent_name":      stringSchema(),
			"command":         stringSchema(),
			"cwd":             stringSchema(),
			"idempotency_key": stringSchema(),
		}),
		tool("ledger.close_workload", "Close a workload with status and outcome.", map[string]interface{}{
			"workload_id": requiredStringSchema(),
			"status":      stringSchema(),
			"outcome":     stringSchema(),
		}),
		tool("ledger.link_workloads", "Create a metadata-only dependency or lineage edge between workloads.", map[string]interface{}{
			"source_workload_id": requiredStringSchema(),
			"target_workload_id": requiredStringSchema(),
			"relation":           stringSchema(),
			"reason":             stringSchema(),
			"created_by":         stringSchema(),
		}),
		tool("ledger.acquire_workload_lease", "Acquire a retry-safe local lease before an async router starts executing a workload.", map[string]interface{}{
			"workload_id": requiredStringSchema(),
			"holder":      requiredStringSchema(),
			"purpose":     stringSchema(),
			"ttl":         stringSchema(),
			"ttl_seconds": integerSchema(),
		}),
		tool("ledger.claim_next_workload", "Atomically claim the next queue-eligible workload and return a short-lived execution lease.", map[string]interface{}{
			"holder":      requiredStringSchema(),
			"purpose":     stringSchema(),
			"ttl":         stringSchema(),
			"ttl_seconds": integerSchema(),
			"source":      stringSchema(),
			"project":     stringSchema(),
			"repo":        stringSchema(),
			"team":        stringSchema(),
			"owner":       stringSchema(),
			"status":      stringSchema(),
			"q":           stringSchema(),
		}),
		tool("ledger.workload_queue", "Return read-only workload queue claimability and lease pressure stats.", map[string]interface{}{
			"source":  stringSchema(),
			"project": stringSchema(),
			"repo":    stringSchema(),
			"team":    stringSchema(),
			"owner":   stringSchema(),
			"status":  stringSchema(),
			"q":       stringSchema(),
		}),
		tool("ledger.renew_workload_lease", "Renew an active workload lease using its lease token.", map[string]interface{}{
			"lease_id":    requiredStringSchema(),
			"lease_token": requiredStringSchema(),
			"ttl":         stringSchema(),
			"ttl_seconds": integerSchema(),
		}),
		tool("ledger.release_workload_lease", "Release an active workload lease using its lease token.", map[string]interface{}{
			"lease_id":    requiredStringSchema(),
			"lease_token": requiredStringSchema(),
		}),
		tool("ledger.workload_leases", "List active workload leases without exposing lease tokens.", map[string]interface{}{
			"include_inactive": booleanSchema(),
			"limit":            integerSchema(),
		}),
		tool("ledger.start_run", "Start a new agent run attached to an existing workload.", map[string]interface{}{
			"workload_id":     requiredStringSchema(),
			"source":          stringSchema(),
			"agent_name":      stringSchema(),
			"command":         stringSchema(),
			"cwd":             stringSchema(),
			"idempotency_key": stringSchema(),
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
		tool("ledger.workload_timeline", "Return a chronological metadata-only workload audit timeline.", map[string]interface{}{
			"workload_id": requiredStringSchema(),
			"limit":       integerSchema(),
		}),
		tool("ledger.workload_state", "Return a derived terminal-state snapshot for one async agent workload.", map[string]interface{}{
			"workload_id": requiredStringSchema(),
			"max_age":     stringSchema(),
		}),
		tool("ledger.workload_feed", "Return a cursor-stable metadata-only workload state feed for local monitors and agent routers.", map[string]interface{}{
			"from":        stringSchema(),
			"to":          stringSchema(),
			"source":      stringSchema(),
			"model":       stringSchema(),
			"project":     stringSchema(),
			"phase":       stringSchema(),
			"severity":    stringSchema(),
			"limit":       integerSchema(),
			"stale_after": stringSchema(),
		}),
		tool("ledger.record_tool_call", "Record metadata-only tool execution such as shell, file, browser, MCP, or custom agent actions.", map[string]interface{}{
			"workload_id":  requiredStringSchema(),
			"run_id":       stringSchema(),
			"source":       stringSchema(),
			"tool_call_id": stringSchema(),
			"tool_name":    requiredStringSchema(),
			"tool_type":    stringSchema(),
			"status":       stringSchema(),
			"error_class":  stringSchema(),
			"duration_ms":  integerSchema(),
			"params_hash":  stringSchema(),
			"event_id":     stringSchema(),
			"timestamp":    stringSchema(),
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
		tool("ledger.record_evaluation", "Record a metadata-only quality, test, review, or acceptance signal for a workload.", map[string]interface{}{
			"workload_id":   requiredStringSchema(),
			"run_id":        stringSchema(),
			"evaluation_id": stringSchema(),
			"source":        stringSchema(),
			"evaluator":     stringSchema(),
			"status":        stringSchema(),
			"score":         numberSchema(),
			"signal":        stringSchema(),
			"notes":         stringSchema(),
			"event_id":      stringSchema(),
			"timestamp":     stringSchema(),
		}),
		tool("ledger.record_context", "Record a metadata-only context reference such as a repo, worktree, trace, task, or external memory handle.", map[string]interface{}{
			"workload_id":    requiredStringSchema(),
			"run_id":         stringSchema(),
			"source":         stringSchema(),
			"context_ref_id": stringSchema(),
			"ref_type":       stringSchema(),
			"ref_hash":       stringSchema(),
			"label":          stringSchema(),
			"repo":           stringSchema(),
			"git_branch":     stringSchema(),
			"commit_sha":     stringSchema(),
			"privacy_label":  stringSchema(),
			"event_id":       stringSchema(),
			"timestamp":      stringSchema(),
		}),
		tool("ledger.record_event", "Ingest one canonical metadata-only event into the workload ledger.", map[string]interface{}{
			"source":          requiredStringSchema(),
			"event_type":      requiredStringSchema(),
			"event_id":        stringSchema(),
			"schema_version":  stringSchema(),
			"source_version":  stringSchema(),
			"parser_version":  stringSchema(),
			"source_event_id": stringSchema(),
			"raw_ref":         stringSchema(),
			"match_type":      stringSchema(),
			"workload_id":     stringSchema(),
			"agent_run_id":    stringSchema(),
			"session_id":      stringSchema(),
			"model":           stringSchema(),
			"project":         stringSchema(),
			"git_branch":      stringSchema(),
			"timestamp":       stringSchema(),
			"payload":         objectSchema(),
		}),
		tool("ledger.validate_event", "Validate one canonical metadata-only event without writing local state.", map[string]interface{}{
			"source":          requiredStringSchema(),
			"event_type":      requiredStringSchema(),
			"event_id":        stringSchema(),
			"schema_version":  stringSchema(),
			"source_version":  stringSchema(),
			"parser_version":  stringSchema(),
			"source_event_id": stringSchema(),
			"raw_ref":         stringSchema(),
			"match_type":      stringSchema(),
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
		tool("ledger.schema_evolution_gate", "Return a privacy-safe pass/review/block compatibility gate for canonical event schema pins and adapter event families.", map[string]interface{}{
			"strict":                 booleanSchema(),
			"schema_version":         stringSchema(),
			"schema_hash":            stringSchema(),
			"required_event_types":   map[string]interface{}{"type": "array", "items": stringSchema()},
			"required_rejected_keys": map[string]interface{}{"type": "array", "items": stringSchema()},
			"event_type":             map[string]interface{}{"oneOf": []map[string]interface{}{stringSchema(), {"type": "array", "items": stringSchema()}}},
			"rejected_key":           map[string]interface{}{"oneOf": []map[string]interface{}{stringSchema(), {"type": "array", "items": stringSchema()}}},
		}),
		tool("ledger.event_examples", "Return privacy-safe canonical event examples for adapter authors.", map[string]interface{}{
			"event_type": stringSchema(),
		}),
		tool("ledger.adapter_conformance", "Validate a canonical/provider/provider-stream/OpenTelemetry/A2A fixture without writing local state.", map[string]interface{}{
			"kind":      stringSchema(),
			"strict":    booleanSchema(),
			"raw_json":  stringSchema(),
			"raw_input": stringSchema(),
		}),
		tool("ledger.adapter_contract", "Return the machine-readable adapter contract for privacy-safe ecosystem integrations.", map[string]interface{}{}),
		tool("ledger.conformance_matrix", "Return the adapter conformance matrix with supported input kinds, strict CI fixtures, expected event families, and validation entrypoints.", map[string]interface{}{}),
		tool("ledger.integrations", "Return the Agent Ledger integration capability catalog.", map[string]interface{}{}),
		tool("ledger.provider_profiles", "Return the privacy-safe provider and runtime profile catalog for adapters, routers, relays, local runtimes, and edge models.", map[string]interface{}{}),
		tool("ledger.agent_profiles", "Return the privacy-safe agent framework profile catalog for CLIs, wrappers, routers, protocol adapters, and observability bridges.", map[string]interface{}{}),
		tool("ledger.signal_taxonomy", "Return the static privacy-safe signal taxonomy for wrappers, routers, provider adapters, and observability bridges.", map[string]interface{}{}),
		tool("ledger.signal_coverage", "Return the static privacy-safe signal coverage report linking taxonomy signals to adapters, providers, and agent profiles.", map[string]interface{}{}),
		tool("ledger.integration_readiness", "Return the privacy-safe runtime activation readiness report for local integration capabilities.", map[string]interface{}{}),
		tool("ledger.integration_smoke", "Return the privacy-safe rollout smoke report for contract, conformance, signal, readiness, and recommendation checks.", map[string]interface{}{}),
		tool("ledger.integration_compatibility", "Return the privacy-safe agent/provider compatibility matrix for adapter CI and ecosystem rollout planning.", map[string]interface{}{
			"agent_profile_id":    stringSchema(),
			"agent":               stringSchema(),
			"profile":             stringSchema(),
			"framework":           stringSchema(),
			"provider_profile_id": stringSchema(),
			"provider":            stringSchema(),
			"runtime":             stringSchema(),
			"surface":             stringSchema(),
			"min_confidence":      stringSchema(),
		}),
		tool("ledger.integration_rollout_plan", "Return a privacy-safe adapter rollout checklist with fixture, admission, pricing, policy, smoke, and release gates.", map[string]interface{}{
			"agent_profile_id":    stringSchema(),
			"agent":               stringSchema(),
			"profile":             stringSchema(),
			"framework":           stringSchema(),
			"provider_profile_id": stringSchema(),
			"provider":            stringSchema(),
			"runtime":             stringSchema(),
			"surface":             stringSchema(),
			"min_confidence":      stringSchema(),
		}),
		tool("ledger.integration_evidence_kit", "Return a privacy-safe adapter release evidence kit for CI, review, pricing, policy, and rollout gates.", map[string]interface{}{
			"agent_profile_id":    stringSchema(),
			"agent":               stringSchema(),
			"profile":             stringSchema(),
			"framework":           stringSchema(),
			"provider_profile_id": stringSchema(),
			"provider":            stringSchema(),
			"runtime":             stringSchema(),
			"surface":             stringSchema(),
			"min_confidence":      stringSchema(),
		}),
		tool("ledger.integration_drift", "Return a privacy-safe current-vs-expected integration contract hash drift report.", map[string]interface{}{
			"strict":   booleanSchema(),
			"expected": map[string]interface{}{"type": "object", "additionalProperties": stringSchema()},
			"hash":     map[string]interface{}{"oneOf": []map[string]interface{}{stringSchema(), {"type": "array", "items": stringSchema()}}},
		}),
		tool("ledger.integration_lockfile", "Return a privacy-safe static integration contract hash lockfile baseline for adapter, wrapper, router, and CI release pins.", map[string]interface{}{}),
		tool("ledger.integration_upgrade_gate", "Return a privacy-safe pass/review/block integration upgrade gate for pinned lockfiles and current control-plane hashes.", map[string]interface{}{
			"strict":   booleanSchema(),
			"expected": map[string]interface{}{"type": "object", "additionalProperties": stringSchema()},
			"hash":     map[string]interface{}{"oneOf": []map[string]interface{}{stringSchema(), {"type": "array", "items": stringSchema()}}},
		}),
		tool("ledger.integration_recommendation", "Return a read-only integration recommendation for an agent/profile/provider/surface combination using static Agent Ledger catalogs.", map[string]interface{}{
			"agent_profile_id":    stringSchema(),
			"agent":               stringSchema(),
			"profile":             stringSchema(),
			"framework":           stringSchema(),
			"provider_profile_id": stringSchema(),
			"provider":            stringSchema(),
			"runtime":             stringSchema(),
			"surface":             stringSchema(),
			"signals":             map[string]interface{}{"oneOf": []map[string]interface{}{stringSchema(), {"type": "array", "items": stringSchema()}}},
			"runtime_mode":        stringSchema(),
			"read_only":           booleanSchema(),
		}),
		tool("ledger.get_policy", "Evaluate local advisory policy rules for a proposed agent action.", map[string]interface{}{
			"workload_id": stringSchema(),
			"run_id":      stringSchema(),
			"source":      stringSchema(),
			"model":       stringSchema(),
			"project":     stringSchema(),
			"repo":        stringSchema(),
			"git_branch":  stringSchema(),
			"team":        stringSchema(),
			"action":      stringSchema(),
			"target":      stringSchema(),
			"role":        stringSchema(),
		}),
		tool("ledger.policy_audit", "Audit historical usage, tool calls, and workloads against local policy rules.", map[string]interface{}{
			"from":    stringSchema(),
			"to":      stringSchema(),
			"source":  stringSchema(),
			"model":   stringSchema(),
			"project": stringSchema(),
			"limit":   integerSchema(),
		}),
		tool("ledger.approval_routes", "Return pending local policy approval route rollups for approval bots and local notification adapters.", map[string]interface{}{
			"due_within": stringSchema(),
			"limit":      integerSchema(),
			"privacy":    booleanSchema(),
		}),
		tool("ledger.approvals", "List local policy approval requests for approval bots and local control-plane agents.", map[string]interface{}{
			"status":  enumSchema([]string{"pending", "approved", "rejected", "all"}),
			"limit":   integerSchema(),
			"privacy": booleanSchema(),
		}),
		tool("ledger.resolve_approval", "Cast an approve or reject vote for a local policy approval request.", map[string]interface{}{
			"request_id":         requiredStringSchema(),
			"status":             enumSchema([]string{"approved", "rejected"}),
			"voter":              stringSchema(),
			"role":               stringSchema(),
			"note":               stringSchema(),
			"required_approvals": integerSchema(),
		}),
		tool("ledger.audit_log", "Return local operational audit log events with optional privacy redaction.", map[string]interface{}{
			"from":    stringSchema(),
			"to":      stringSchema(),
			"actor":   stringSchema(),
			"role":    stringSchema(),
			"action":  stringSchema(),
			"target":  stringSchema(),
			"limit":   integerSchema(),
			"privacy": map[string]interface{}{"type": "boolean"},
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
	access := controlplane.MCPToolAccessFor(name)
	out := map[string]interface{}{
		"name":        name,
		"description": description,
		"inputSchema": map[string]interface{}{
			"type":       "object",
			"properties": props,
			"required":   required,
		},
	}
	out["annotations"] = map[string]interface{}{
		"readOnlyHint":    access.WriteMode == "none",
		"destructiveHint": false,
		"idempotentHint":  access.WriteMode == "none",
		"openWorldHint":   false,
	}
	out["_meta"] = map[string]interface{}{
		"agent_ledger": map[string]interface{}{
			"writes_local_state":     access.WritesLocalState,
			"write_mode":             access.WriteMode,
			"available_in_read_only": access.AvailableInReadOnly,
			"read_only_behavior":     access.ReadOnlyBehavior,
		},
	}
	return out
}

func stringSchema() map[string]interface{} { return map[string]interface{}{"type": "string"} }
func requiredStringSchema() map[string]interface{} {
	return map[string]interface{}{"type": "string", "x-required": true}
}
func numberSchema() map[string]interface{}  { return map[string]interface{}{"type": "number"} }
func integerSchema() map[string]interface{} { return map[string]interface{}{"type": "integer"} }
func objectSchema() map[string]interface{}  { return map[string]interface{}{"type": "object"} }
func booleanSchema() map[string]interface{} { return map[string]interface{}{"type": "boolean"} }
func enumSchema(values []string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "enum": values}
}

func resources() []map[string]interface{} {
	return []map[string]interface{}{
		resource("agent-ledger://discovery/manifest", "Discovery Manifest", "Privacy-safe local discovery contract for wrappers, routers, and agent frameworks.", "application/json"),
		resource("agent-ledger://contracts/bundle", "Contract Bundle", "Privacy-safe index of stable Agent Ledger contracts, hashes, cache semantics, CLI commands, and MCP entrypoints.", "application/json"),
		resource("agent-ledger://contracts/verification", "Contract Verification", "Machine-readable self-check for stable Agent Ledger control-plane contracts.", "application/json"),
		resource("agent-ledger://goal/coverage", "Goal Coverage", "Privacy-safe Agent Ledger product goal coverage report with implementation evidence and external dependencies.", "application/json"),
		resource("agent-ledger://contracts/openapi", "Control Plane OpenAPI", "Metadata-only OpenAPI 3.1 document for stable Agent Ledger REST control-plane endpoints.", "application/json"),
		resource("agent-ledger://schema/canonical-events", "Canonical Event Schema", "Metadata-only event contract for workload, run, model-call, tool-call, artifact, evaluation, and policy events.", "application/json"),
		resource("agent-ledger://schema/evolution-gate", "Schema Evolution Gate", "Privacy-safe canonical event schema compatibility gate; supports strict=true, schema_hash, event_type, and rejected_key query parameters.", "application/json"),
		resource("agent-ledger://schema/canonical-event-examples", "Canonical Event Examples", "Privacy-safe templates for all supported canonical event types.", "application/json"),
		resource("agent-ledger://integrations/catalog", "Integration Capability Catalog", "Privacy-safe catalog of implemented, experimental, and planned integration surfaces.", "application/json"),
		resource("agent-ledger://integrations/provider-profiles", "Provider Profile Catalog", "Privacy-safe provider/runtime profile catalog for adapters, routers, relays, local runtimes, and edge models.", "application/json"),
		resource("agent-ledger://integrations/agent-profiles", "Agent Framework Profile Catalog", "Privacy-safe agent CLI, framework, wrapper, router, protocol, and observability profile catalog.", "application/json"),
		resource("agent-ledger://integrations/signal-taxonomy", "Signal Taxonomy Catalog", "Static privacy-safe signal dictionary for mapping adapter and router metadata to canonical event families.", "application/json"),
		resource("agent-ledger://integrations/signal-coverage", "Signal Coverage Report", "Static privacy-safe coverage report linking taxonomy signal ids to adapter, provider, and agent profile contracts.", "application/json"),
		resource("agent-ledger://integrations/readiness", "Integration Readiness Report", "Privacy-safe runtime activation readiness report for local integration capabilities.", "application/json"),
		resource("agent-ledger://integrations/smoke", "Integration Smoke Report", "Privacy-safe rollout smoke report for contract, conformance, signal, readiness, and recommendation checks.", "application/json"),
		resource("agent-ledger://integrations/compatibility", "Integration Compatibility Matrix", "Privacy-safe agent/provider compatibility matrix for adapter CI and ecosystem rollout planning; supports agent/provider/surface/min_confidence query parameters.", "application/json"),
		resource("agent-ledger://integrations/rollout-plan", "Integration Rollout Plan", "Privacy-safe adapter rollout checklist with fixture, admission, pricing, policy, smoke, and release gates; supports agent/provider/surface/min_confidence query parameters.", "application/json"),
		resource("agent-ledger://integrations/evidence-kit", "Integration Evidence Kit", "Privacy-safe adapter release evidence kit with CI commands, review lanes, hashes, fixture evidence, and rollout gates; supports agent/provider/surface/min_confidence query parameters.", "application/json"),
		resource("agent-ledger://integrations/drift", "Integration Drift Report", "Privacy-safe current-vs-expected integration contract hash drift report; supports strict=true and expected hash query parameters.", "application/json"),
		resource("agent-ledger://integrations/lockfile", "Integration Lockfile", "Privacy-safe static integration contract hash lockfile baseline for adapter, wrapper, router, and CI release pins.", "application/json"),
		resource("agent-ledger://integrations/upgrade-gate", "Integration Upgrade Gate", "Privacy-safe pass/review/block integration upgrade gate; supports strict=true and expected hash query parameters.", "application/json"),
		resource("agent-ledger://integrations/recommendation", "Integration Recommendation", "Read-only advisor for choosing Agent Ledger ingest, validation, privacy, and quality gates from static integration catalogs; supports agent/provider/surface/signals query parameters.", "application/json"),
		resource("agent-ledger://integrations/adapter-contract", "Adapter Contract", "Machine-readable contract for writing privacy-safe Agent Ledger adapters.", "application/json"),
		resource("agent-ledger://integrations/conformance-matrix", "Adapter Conformance Matrix", "Privacy-safe matrix of supported adapter input kinds, strict CI fixtures, expected event families, and validation entrypoints.", "application/json"),
		resource("agent-ledger://runtime/status", "Runtime Status", "Process-level observer/control-plane mode, read-only state, background task state, and write-operation status.", "application/json"),
		resource("agent-ledger://config/status", "Config Status", "Privacy-safe deployment configuration status with risk checks and remediation hints.", "application/json"),
		resource("agent-ledger://readiness", "Readiness", "Privacy-safe control-plane readiness report for local deployment and wrapper checks.", "application/json"),
		resource("agent-ledger://admission/check", "Admission Check", "Privacy-safe dry-run for HTTP, CLI, and MCP operations; supports surface/method/path/command/tool/role/dry_run/record/has_workload_id query parameters.", "application/json"),
		resource("agent-ledger://budget/current", "Current Budget Windows", "Local quota and budget estimate for 5h/day/week/month windows; supports window/source/model/project query parameters.", "application/json"),
		resource("agent-ledger://workloads/recent", "Recent Workloads", "Recent workload summaries and terminal-state snapshots from the local ledger; supports from/to/source/model/project/status/q/limit/offset/stale_after query parameters.", "application/json"),
		resource("agent-ledger://workloads/queue", "Workload Queue", "Read-only workload queue claimability and lease pressure stats; supports source/project/repo/team/owner/status/q query parameters.", "application/json"),
		resource("agent-ledger://workloads/leases", "Workload Leases", "Privacy-safe read-only workload lease rows for router context; supports include_inactive and limit query parameters.", "application/json"),
		resource("agent-ledger://workloads/feed", "Workload Event Feed", "Cursor-stable metadata-only workload state feed for local monitors and agent routers; supports from/to/source/model/project/phase/severity/limit/stale_after query parameters.", "application/json"),
		resource("agent-ledger://workload/state", "Workload State", "Privacy-safe terminal-state snapshot for one workload; requires workload_id and supports max_age.", "application/json"),
		resource("agent-ledger://workload/timeline", "Workload Timeline", "Privacy-safe timeline event kinds for one workload; requires workload_id and supports limit.", "application/json"),
		resource("agent-ledger://agent-runs/liveness", "Agent Run Liveness", "Privacy-safe active run liveness rows for async agent monitors; supports max_age, stale_only, source, project, and limit query parameters.", "application/json"),
		resource("agent-ledger://policies/status", "Policy Status", "Local policy configuration summary without prompt or secret content.", "application/json"),
		resource("agent-ledger://policy/approvals", "Policy Approvals", "Local policy approval queue; supports status, limit, and privacy query parameters.", "application/json"),
		resource("agent-ledger://policy/approval-routes", "Policy Approval Routes", "Pending local approval route rollups; supports due_within, limit, and privacy query parameters.", "application/json"),
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
	if s.cfg != nil && s.cfg.RBAC.ReadOnly && mcpToolRequiresWrite(name, args) {
		return nil, fmt.Errorf("read-only mode: MCP tool %q is disabled because it writes local state", name)
	}
	switch name {
	case "ledger.current_budget":
		return s.toolCurrentBudget(args)
	case "ledger.discovery":
		return integrations.Discovery(integrations.OptionsFromConfig(s.cfg)), nil
	case "ledger.contracts":
		return integrations.ContractBundleFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "ledger.contracts_verify":
		return integrations.ContractVerificationReportFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "ledger.goal_coverage":
		return integrations.GoalCoverageReportFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "ledger.openapi":
		return integrations.OpenAPISpecFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "ledger.runtime_status":
		return s.runtimeStatus(), nil
	case "ledger.config_status":
		return config.StatusReport(s.cfg), nil
	case "ledger.readiness":
		return s.readinessReport(), nil
	case "ledger.admission_check":
		return s.toolAdmissionCheck(args)
	case "ledger.start_workload":
		return s.toolStartWorkload(args)
	case "ledger.start_run":
		return s.toolStartRun(args)
	case "ledger.close_workload":
		return s.toolCloseWorkload(args)
	case "ledger.link_workloads":
		return s.toolLinkWorkloads(args)
	case "ledger.acquire_workload_lease":
		return s.toolAcquireWorkloadLease(args)
	case "ledger.claim_next_workload":
		return s.toolClaimNextWorkload(args)
	case "ledger.workload_queue":
		return s.toolWorkloadQueue(args)
	case "ledger.renew_workload_lease":
		return s.toolRenewWorkloadLease(args)
	case "ledger.release_workload_lease":
		return s.toolReleaseWorkloadLease(args)
	case "ledger.workload_leases":
		return s.toolWorkloadLeases(args)
	case "ledger.heartbeat_run":
		return s.toolHeartbeatRun(args)
	case "ledger.run_liveness":
		return s.toolRunLiveness(args)
	case "ledger.workload_timeline":
		return s.toolWorkloadTimeline(args)
	case "ledger.workload_state":
		return s.toolWorkloadState(args)
	case "ledger.workload_feed":
		return s.toolWorkloadFeed(args)
	case "ledger.record_tool_call":
		return s.toolRecordToolCall(args)
	case "ledger.record_artifact":
		return s.toolRecordArtifact(args)
	case "ledger.record_evaluation":
		return s.toolRecordEvaluation(args)
	case "ledger.record_context":
		return s.toolRecordContext(args)
	case "ledger.record_event":
		return s.toolRecordEvent(args)
	case "ledger.validate_event":
		return s.toolValidateEvent(args)
	case "ledger.event_schema":
		return storage.CanonicalEventSchema(), nil
	case "ledger.schema_evolution_gate":
		return toolSchemaEvolutionGate(args)
	case "ledger.event_examples":
		var req struct {
			EventType string `json:"event_type"`
		}
		_ = json.Unmarshal(args, &req)
		return map[string]interface{}{
			"contract": "agent-ledger.canonical-event-examples",
			"version":  "v1",
			"examples": storage.CanonicalEventExamples(req.EventType),
		}, nil
	case "ledger.adapter_conformance":
		return s.toolAdapterConformance(args)
	case "ledger.adapter_contract":
		return integrations.AdapterContractSpec(), nil
	case "ledger.conformance_matrix":
		return integrations.AdapterConformanceMatrixSpec(), nil
	case "ledger.integrations":
		return integrations.Registry(integrations.OptionsFromConfig(s.cfg)), nil
	case "ledger.provider_profiles":
		return integrations.ProviderProfiles(), nil
	case "ledger.agent_profiles":
		return integrations.AgentFrameworkProfiles(), nil
	case "ledger.signal_taxonomy":
		return integrations.SignalTaxonomy(), nil
	case "ledger.signal_coverage":
		return integrations.SignalCoverage(), nil
	case "ledger.integration_readiness":
		return integrations.IntegrationReadiness(integrations.OptionsFromConfig(s.cfg)), nil
	case "ledger.integration_smoke":
		opts := integrations.OptionsFromConfig(s.cfg)
		return integrations.IntegrationSmokeReportFor(opts, s.runtimeStatus()), nil
	case "ledger.integration_compatibility":
		return toolIntegrationCompatibility(args)
	case "ledger.integration_rollout_plan":
		return toolIntegrationRolloutPlan(args)
	case "ledger.integration_evidence_kit":
		return s.toolIntegrationEvidenceKit(args)
	case "ledger.integration_drift":
		return s.toolIntegrationDrift(args)
	case "ledger.integration_lockfile":
		return integrations.IntegrationLockfileFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "ledger.integration_upgrade_gate":
		return s.toolIntegrationUpgradeGate(args)
	case "ledger.integration_recommendation":
		return toolIntegrationRecommendation(args)
	case "ledger.get_policy":
		return s.toolGetPolicy(args)
	case "ledger.policy_audit":
		return s.toolPolicyAudit(args)
	case "ledger.approval_routes":
		return s.toolApprovalRoutes(args)
	case "ledger.approvals":
		return s.toolApprovals(args)
	case "ledger.resolve_approval":
		return s.toolResolveApproval(args)
	case "ledger.audit_log":
		return s.toolAuditLog(args)
	case "ledger.explain_cost":
		return s.toolExplainCost(args)
	case "ledger.find_similar_workloads":
		return s.toolFindSimilarWorkloads(args)
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func mcpToolRequiresWrite(name string, args json.RawMessage) bool {
	access := controlplane.MCPToolAccessFor(name)
	switch access.WriteMode {
	case "always":
		return true
	case "conditional":
		if name != "ledger.get_policy" {
			return true
		}
		var in struct {
			WorkloadID string `json:"workload_id"`
		}
		_ = json.Unmarshal(args, &in)
		return strings.TrimSpace(in.WorkloadID) != ""
	default:
		return false
	}
}

func (s *Server) runtimeStatus() *storage.RuntimeStatus {
	if s.cfg != nil && s.cfg.RBAC.ReadOnly {
		return integrations.EnrichRuntimeStatus(&storage.RuntimeStatus{
			Mode:             "observer",
			ReadOnly:         true,
			WriteOperations:  "disabled",
			BackgroundTasks:  "disabled",
			DisabledFeatures: []string{"background collectors", "pricing sync", "cost recalculation", "manual scans", "imports", "write APIs", "write MCP tools", "derived GET writebacks"},
			Message:          "read-only observer mode: local state is not mutated by this process",
		}, integrations.OptionsFromConfig(s.cfg))
	}
	return integrations.EnrichRuntimeStatus(&storage.RuntimeStatus{
		Mode:            "control-plane",
		ReadOnly:        false,
		WriteOperations: "enabled",
		BackgroundTasks: "enabled",
		Message:         "write operations and background collectors are enabled",
	}, integrations.OptionsFromConfig(s.cfg))
}

func (s *Server) readinessReport() *controlplane.ReadinessReport {
	runtime := s.runtimeStatus()
	return controlplane.BuildReadinessReport(
		s.db,
		s.cfg,
		runtime,
		integrations.ContractVerificationReportFor(integrations.OptionsFromConfig(s.cfg), runtime),
		s.now().UTC(),
	)
}

func (s *Server) toolAdmissionCheck(args json.RawMessage) (interface{}, error) {
	input := s.admissionDefaults()
	if len(args) > 0 {
		if err := json.Unmarshal(args, &input); err != nil {
			return nil, err
		}
		defaults := s.admissionDefaults()
		input.RBACEnabled = defaults.RBACEnabled
		input.AuthConfigured = defaults.AuthConfigured
		input.ReadOnly = defaults.ReadOnly
	}
	decision := controlplane.EvaluateAdmission(input, s.now().UTC())
	return decision, nil
}

func (s *Server) admissionDefaults() controlplane.AdmissionInput {
	if s.cfg == nil {
		return controlplane.AdmissionInput{}
	}
	return controlplane.AdmissionInput{
		RBACEnabled:    s.cfg.RBAC.Enabled,
		AuthConfigured: s.cfg.Server.AuthToken != "" || s.cfg.Server.AdminToken != "" || s.cfg.Server.ViewerToken != "",
		ReadOnly:       s.cfg.RBAC.ReadOnly,
	}
}

func (s *Server) readResource(uri string) (interface{}, error) {
	payload, err := s.resourcePayload(uri)
	if err != nil {
		return nil, err
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

func (s *Server) resourcePayload(uri string) (interface{}, error) {
	baseURI, values, err := parseResourceURI(uri)
	if err != nil {
		return nil, err
	}
	switch baseURI {
	case "agent-ledger://discovery/manifest":
		return integrations.Discovery(integrations.OptionsFromConfig(s.cfg)), nil
	case "agent-ledger://contracts/bundle":
		return integrations.ContractBundleFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "agent-ledger://contracts/verification":
		return integrations.ContractVerificationReportFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "agent-ledger://goal/coverage":
		return integrations.GoalCoverageReportFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "agent-ledger://contracts/openapi":
		return integrations.OpenAPISpecFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "agent-ledger://schema/canonical-events":
		return storage.CanonicalEventSchema(), nil
	case "agent-ledger://schema/evolution-gate":
		return integrations.SchemaEvolutionGateFor(integrations.SchemaEvolutionGateFromValues(values)), nil
	case "agent-ledger://schema/canonical-event-examples":
		return map[string]interface{}{
			"contract": "agent-ledger.canonical-event-examples",
			"version":  "v1",
			"examples": storage.CanonicalEventExamples(firstNonEmpty(values.Get("type"), values.Get("event_type"))),
		}, nil
	case "agent-ledger://integrations/catalog":
		return integrations.Registry(integrations.OptionsFromConfig(s.cfg)), nil
	case "agent-ledger://integrations/provider-profiles":
		return integrations.ProviderProfiles(), nil
	case "agent-ledger://integrations/agent-profiles":
		return integrations.AgentFrameworkProfiles(), nil
	case "agent-ledger://integrations/signal-taxonomy":
		return integrations.SignalTaxonomy(), nil
	case "agent-ledger://integrations/signal-coverage":
		return integrations.SignalCoverage(), nil
	case "agent-ledger://integrations/readiness":
		return integrations.IntegrationReadiness(integrations.OptionsFromConfig(s.cfg)), nil
	case "agent-ledger://integrations/smoke":
		opts := integrations.OptionsFromConfig(s.cfg)
		return integrations.IntegrationSmokeReportFor(opts, s.runtimeStatus()), nil
	case "agent-ledger://integrations/compatibility":
		return integrations.IntegrationCompatibilityReportFor(integrations.IntegrationCompatibilityFromValues(values)), nil
	case "agent-ledger://integrations/rollout-plan":
		return integrations.IntegrationRolloutPlanFor(integrations.IntegrationRolloutFromValues(values)), nil
	case "agent-ledger://integrations/evidence-kit":
		return integrations.IntegrationEvidenceKitFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus(), integrations.IntegrationEvidenceKitFromValues(values)), nil
	case "agent-ledger://integrations/drift":
		return integrations.IntegrationDriftReportFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus(), integrations.IntegrationDriftFromValues(values)), nil
	case "agent-ledger://integrations/lockfile":
		return integrations.IntegrationLockfileFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus()), nil
	case "agent-ledger://integrations/upgrade-gate":
		return integrations.IntegrationUpgradeGateFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus(), integrations.IntegrationUpgradeGateFromValues(values)), nil
	case "agent-ledger://integrations/recommendation":
		return integrations.IntegrationRecommendation(integrations.IntegrationRecommendationFromValues(values)), nil
	case "agent-ledger://integrations/adapter-contract":
		return integrations.AdapterContractSpec(), nil
	case "agent-ledger://integrations/conformance-matrix":
		return integrations.AdapterConformanceMatrixSpec(), nil
	case "agent-ledger://runtime/status":
		return s.runtimeStatus(), nil
	case "agent-ledger://config/status":
		return config.StatusReport(s.cfg), nil
	case "agent-ledger://readiness":
		return s.readinessReport(), nil
	case "agent-ledger://admission/check":
		return controlplane.EvaluateAdmission(controlplane.AdmissionInputFromValues(values, s.admissionDefaults()), s.now().UTC()), nil
	case "agent-ledger://budget/current":
		return s.resourceBudget(values)
	case "agent-ledger://workloads/recent":
		return s.resourceRecentWorkloads(values)
	case "agent-ledger://workloads/queue":
		return s.resourceWorkloadQueue(values)
	case "agent-ledger://workloads/leases":
		return s.resourceWorkloadLeases(values)
	case "agent-ledger://workloads/feed":
		return s.resourceWorkloadFeed(values)
	case "agent-ledger://workload/state":
		return s.resourceWorkloadState(values)
	case "agent-ledger://workload/timeline":
		return s.resourceWorkloadTimeline(values)
	case "agent-ledger://agent-runs/liveness":
		return s.resourceRunLiveness(values)
	case "agent-ledger://policies/status":
		return map[string]interface{}{
			"enabled":                s.cfg.Policies.Enabled,
			"require_privacy_export": s.cfg.Policies.RequirePrivacyExport,
			"rules":                  s.cfg.Policies.Rules,
		}, nil
	case "agent-ledger://policy/approvals":
		return s.resourceApprovals(values)
	case "agent-ledger://policy/approval-routes":
		return s.resourceApprovalRoutes(values)
	default:
		return nil, fmt.Errorf("unknown resource %q", uri)
	}
}

func parseResourceURI(raw string) (string, url.Values, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, fmt.Errorf("uri is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", nil, err
	}
	if parsed.Scheme != "agent-ledger" || parsed.Host == "" {
		return "", nil, fmt.Errorf("unknown resource %q", raw)
	}
	base := parsed.Scheme + "://" + parsed.Host + parsed.EscapedPath()
	return base, parsed.Query(), nil
}

func (s *Server) resourceBudget(values url.Values) (interface{}, error) {
	payload := map[string]string{
		"window":  firstNonEmpty(values.Get("window"), "all"),
		"source":  values.Get("source"),
		"model":   values.Get("model"),
		"project": values.Get("project"),
	}
	raw, _ := json.Marshal(payload)
	return s.toolCurrentBudget(raw)
}

func (s *Server) resourceRecentWorkloads(values url.Values) (interface{}, error) {
	now := s.now()
	from, to, err := parseDateRange(values.Get("from"), values.Get("to"), now)
	if err != nil {
		return nil, err
	}
	limit := boundedQueryInt(values, "limit", 20, 1, 100)
	offset := boundedQueryInt(values, "offset", 0, 0, 1000000)
	staleAfter, err := queryDuration(values, "stale_after", 10*time.Minute)
	if err != nil {
		return nil, err
	}
	page, err := s.db.GetWorkloadsPage(from, to, values.Get("source"), values.Get("model"), values.Get("project"), values.Get("status"), values.Get("q"), limit, offset)
	if err != nil {
		return nil, err
	}
	states, err := s.db.GetWorkloadStates(from, to, values.Get("source"), values.Get("model"), values.Get("project"), limit, staleAfter)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"rows":                page.Rows,
		"total":               page.Total,
		"limit":               page.Limit,
		"offset":              page.Offset,
		"next_cursor":         page.NextCursor,
		"states":              states,
		"from":                from.UTC().Format(time.RFC3339Nano),
		"to":                  to.UTC().Format(time.RFC3339Nano),
		"stale_after_seconds": int64(staleAfter / time.Second),
	}, nil
}

func (s *Server) resourceWorkloadFeed(values url.Values) (*storage.WorkloadEventFeed, error) {
	if len(values) == 0 {
		return s.defaultWorkloadFeed()
	}
	now := s.now()
	from, to, err := parseDateRange(values.Get("from"), values.Get("to"), now)
	if err != nil {
		return nil, err
	}
	staleAfter, err := queryDuration(values, "stale_after", 10*time.Minute)
	if err != nil {
		return nil, err
	}
	return s.db.GetWorkloadEventFeed(from, to,
		values.Get("source"),
		values.Get("model"),
		values.Get("project"),
		values.Get("phase"),
		values.Get("severity"),
		boundedQueryInt(values, "limit", 50, 1, 200),
		staleAfter)
}

func (s *Server) resourceWorkloadQueue(values url.Values) (*storage.WorkloadQueueStats, error) {
	return s.db.GetWorkloadQueueStats(storage.WorkloadClaimFilter{
		Source:  values.Get("source"),
		Project: values.Get("project"),
		Repo:    values.Get("repo"),
		Team:    values.Get("team"),
		Owner:   values.Get("owner"),
		Status:  values.Get("status"),
		Query:   values.Get("q"),
	})
}

func (s *Server) resourceWorkloadLeases(values url.Values) (map[string]interface{}, error) {
	includeInactive := queryBool(values, "include_inactive")
	limit := boundedQueryInt(values, "limit", 50, 1, 500)
	rows, err := s.db.ListWorkloadLeases(includeInactive, limit)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"include_inactive": includeInactive,
		"limit":            limit,
		"rows":             privacySafeLeaseRows(rows),
	}, nil
}

func privacySafeLeaseRows(rows []storage.WorkloadLease) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]interface{}{
			"lease_id":        row.LeaseID,
			"status":          row.Status,
			"acquired_at":     row.AcquiredAt,
			"expires_at":      row.ExpiresAt,
			"last_renewed_at": row.LastRenewedAt,
			"released_at":     row.ReleasedAt,
			"expired":         row.Expired,
			"ttl_seconds":     row.TTLSeconds,
		})
	}
	return out
}

func (s *Server) resourceWorkloadState(values url.Values) (map[string]interface{}, error) {
	workloadID := strings.TrimSpace(values.Get("workload_id"))
	if workloadID == "" {
		return nil, fmt.Errorf("workload_id is required")
	}
	maxAge, err := queryDuration(values, "max_age", 10*time.Minute)
	if err != nil {
		return nil, err
	}
	state, err := s.db.GetWorkloadState(workloadID, maxAge)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"max_age":           maxAge.String(),
		"state":             privacySafeWorkloadState(state),
		"privacy_redaction": "workload_id, goal, project, repo, git_branch, team, owner, and status messages omitted",
	}, nil
}

func privacySafeWorkloadState(state *storage.WorkloadState) map[string]interface{} {
	if state == nil {
		return map[string]interface{}{}
	}
	return map[string]interface{}{
		"status":                     state.Status,
		"source":                     state.Source,
		"phase":                      state.Phase,
		"terminal":                   state.Terminal,
		"stale":                      state.Stale,
		"readiness_score":            state.ReadinessScore,
		"progress":                   state.Progress,
		"next_action":                state.NextAction,
		"reasons":                    state.Reasons,
		"risks":                      state.Risks,
		"last_activity":              state.LastActivity,
		"stale_after_seconds":        state.StaleAfterSeconds,
		"runs":                       state.Runs,
		"active_runs":                state.ActiveRuns,
		"stale_runs":                 state.StaleRuns,
		"completed_runs":             state.CompletedRuns,
		"failed_runs":                state.FailedRuns,
		"model_calls":                state.ModelCalls,
		"tool_calls":                 state.ToolCalls,
		"context_refs":               state.ContextRefs,
		"artifacts":                  state.Artifacts,
		"evaluations":                state.Evaluations,
		"positive_evaluations":       state.PositiveEvaluations,
		"negative_evaluations":       state.NegativeEvaluations,
		"policy_blocks":              state.PolicyBlocks,
		"policy_approvals_required":  state.PolicyApprovalsRequired,
		"cost_usd":                   state.CostUSD,
		"tokens":                     state.Tokens,
		"estimated_remaining_budget": state.EstimatedRemainingBudget,
		"estimated_budget_exhausted": state.EstimatedBudgetExhausted,
		"budget_configured":          state.BudgetUSD > 0,
	}
}

func (s *Server) resourceWorkloadTimeline(values url.Values) (map[string]interface{}, error) {
	workloadID := strings.TrimSpace(values.Get("workload_id"))
	if workloadID == "" {
		return nil, fmt.Errorf("workload_id is required")
	}
	limit := boundedQueryInt(values, "limit", 50, 1, 500)
	rows, err := s.db.GetWorkloadTimeline(workloadID, limit)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"limit":             limit,
		"rows":              privacySafeWorkloadTimelineRows(rows),
		"privacy_redaction": "workload_id, event ids, run ids, labels, details, goals, project metadata, status messages, and raw identifiers omitted",
	}, nil
}

func privacySafeWorkloadTimelineRows(rows []storage.WorkloadTimelineRow) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]interface{}{
			"kind":        row.Kind,
			"source":      row.Source,
			"status":      row.Status,
			"tokens":      row.Tokens,
			"cost_usd":    row.CostUSD,
			"duration_ms": row.DurationMS,
			"timestamp":   row.Timestamp,
			"confidence":  row.Confidence,
		})
	}
	return out
}

func (s *Server) resourceRunLiveness(values url.Values) (map[string]interface{}, error) {
	maxAge, err := queryDuration(values, "max_age", 10*time.Minute)
	if err != nil {
		return nil, err
	}
	staleOnly := queryBool(values, "stale_only")
	limit := boundedQueryInt(values, "limit", 50, 1, 500)
	rows, err := s.db.GetAgentRunLiveness(maxAge, staleOnly, limit, values.Get("source"), values.Get("project"))
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"max_age":           maxAge.String(),
		"stale_only":        staleOnly,
		"limit":             limit,
		"source_filter":     strings.TrimSpace(values.Get("source")) != "",
		"project_filter":    strings.TrimSpace(values.Get("project")) != "",
		"rows":              privacySafeRunLivenessRows(rows),
		"privacy_redaction": "run_id, workload_id, goal, project, repo, git_branch, and status_message omitted",
	}, nil
}

func privacySafeRunLivenessRows(rows []storage.AgentRunLivenessRow) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]interface{}{
			"source":            row.Source,
			"agent_name":        row.AgentName,
			"status":            row.Status,
			"phase":             row.Phase,
			"progress":          row.Progress,
			"started_at":        row.StartedAt,
			"last_heartbeat_at": row.LastHeartbeatAt,
			"last_activity":     row.LastActivity,
			"heartbeat_count":   row.HeartbeatCount,
			"age_seconds":       row.AgeSeconds,
			"stale":             row.Stale,
		})
	}
	return out
}

func (s *Server) resourceApprovalRoutes(values url.Values) (*storage.ApprovalRouteSummary, error) {
	dueWithin, err := boundedDuration(firstNonEmpty(values.Get("due_within"), values.Get("approval_due_within")), "due_within", 24*time.Hour, 30*24*time.Hour)
	if err != nil {
		return nil, err
	}
	report, err := s.db.GetApprovalRouteSummary(boundedQueryInt(values, "limit", 200, 1, 1000), dueWithin)
	if err != nil {
		return nil, err
	}
	if queryBool(values, "privacy") {
		redactMCPApprovalRoutes(report)
	}
	return report, nil
}

func (s *Server) resourceApprovals(values url.Values) (map[string]interface{}, error) {
	status := firstNonEmpty(values.Get("status"), "pending")
	rows, err := s.db.ListApprovalRequests(status, boundedQueryInt(values, "limit", 200, 1, 1000))
	if err != nil {
		return nil, err
	}
	if queryBool(values, "privacy") {
		redactMCPApprovalRequests(rows)
	}
	return map[string]interface{}{"status": status, "rows": rows}, nil
}

func boundedQueryInt(values url.Values, key string, fallback, min, max int) int {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	if parsed < min {
		return min
	}
	if parsed > max {
		return max
	}
	return parsed
}

func queryDuration(values url.Values, key string, fallback time.Duration) (time.Duration, error) {
	raw := strings.TrimSpace(values.Get(key))
	if raw == "" {
		raw = strings.TrimSpace(values.Get("max_age"))
	}
	if raw == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return parsed, nil
}

func boundedDuration(raw, key string, fallback, max time.Duration) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if parsed <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	if max > 0 && parsed > max {
		return 0, fmt.Errorf("%s must be <= %s", key, max)
	}
	return parsed, nil
}

func queryBool(values url.Values, key string) bool {
	raw := strings.ToLower(strings.TrimSpace(values.Get(key)))
	return raw == "1" || raw == "true" || raw == "yes" || raw == "on"
}

func (s *Server) resourceCursor(uri string) (string, error) {
	payload, err := s.resourcePayload(uri)
	if err != nil {
		return "", err
	}
	if feed, ok := payload.(*storage.WorkloadEventFeed); ok {
		return feed.Cursor, nil
	}
	stablePayload, err := stableResourceCursorPayload(payload)
	if err != nil {
		return "", err
	}
	raw, err := json.Marshal(stablePayload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func stableResourceCursorPayload(payload interface{}) (interface{}, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var stable interface{}
	if err := json.Unmarshal(raw, &stable); err != nil {
		return nil, err
	}
	stripVolatileResourceFields(stable)
	return stable, nil
}

func stripVolatileResourceFields(value interface{}) {
	switch v := value.(type) {
	case map[string]interface{}:
		delete(v, "generated_at")
		delete(v, "age_seconds")
		for _, child := range v {
			stripVolatileResourceFields(child)
		}
	case []interface{}:
		for _, child := range v {
			stripVolatileResourceFields(child)
		}
	}
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
		Goal           string  `json:"goal"`
		Source         string  `json:"source"`
		Project        string  `json:"project"`
		Repo           string  `json:"repo"`
		GitBranch      string  `json:"git_branch"`
		Owner          string  `json:"owner"`
		Team           string  `json:"team"`
		BudgetUSD      float64 `json:"budget_usd"`
		AgentName      string  `json:"agent_name"`
		Command        string  `json:"command"`
		CWD            string  `json:"cwd"`
		IdempotencyKey string  `json:"idempotency_key"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	id, replayed, err := s.db.CreateWorkloadIdempotent(in.IdempotencyKey, in.Goal, in.Source, in.Project, in.Repo, in.GitBranch, in.Owner, in.Team, in.BudgetUSD)
	if err != nil {
		return nil, err
	}
	runID := ""
	runReplayed := false
	if in.AgentName != "" || in.Command != "" || in.CWD != "" {
		runKey := ""
		if strings.TrimSpace(in.IdempotencyKey) != "" {
			runKey = strings.TrimSpace(in.IdempotencyKey) + ":initial-run"
		}
		runID, runReplayed, err = s.db.StartAgentRunIdempotent(runKey, id, in.Source, firstNonEmpty(in.AgentName, in.Source, "agent"), in.Command, in.CWD)
		if err != nil {
			return nil, err
		}
	}
	return map[string]interface{}{"workload_id": id, "run_id": runID, "status": "active", "idempotent_replay": replayed, "run_idempotent_replay": runReplayed}, nil
}

func (s *Server) toolStartRun(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID     string `json:"workload_id"`
		Source         string `json:"source"`
		AgentName      string `json:"agent_name"`
		Command        string `json:"command"`
		CWD            string `json:"cwd"`
		IdempotencyKey string `json:"idempotency_key"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	runID, replayed, err := s.db.StartAgentRunIdempotent(in.IdempotencyKey, in.WorkloadID, in.Source, firstNonEmpty(in.AgentName, in.Source, "agent"), in.Command, in.CWD)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"workload_id": in.WorkloadID, "run_id": runID, "status": "running", "idempotent_replay": replayed}, nil
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

func (s *Server) toolLinkWorkloads(args json.RawMessage) (interface{}, error) {
	var in struct {
		SourceWorkloadID string `json:"source_workload_id"`
		TargetWorkloadID string `json:"target_workload_id"`
		Relation         string `json:"relation"`
		Reason           string `json:"reason"`
		CreatedBy        string `json:"created_by"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	linkID, err := s.db.LinkWorkloads(in.SourceWorkloadID, in.TargetWorkloadID, in.Relation, in.Reason, in.CreatedBy, 1)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"link_id":            linkID,
		"source_workload_id": in.SourceWorkloadID,
		"target_workload_id": in.TargetWorkloadID,
		"relation":           firstNonEmpty(in.Relation, "relates_to"),
	}, nil
}

func (s *Server) toolAcquireWorkloadLease(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID string `json:"workload_id"`
		Holder     string `json:"holder"`
		Purpose    string `json:"purpose"`
		TTL        string `json:"ttl"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	ttl, err := mcpLeaseTTL(in.TTL, in.TTLSeconds)
	if err != nil {
		return nil, err
	}
	lease, err := s.db.AcquireWorkloadLease(in.WorkloadID, in.Holder, in.Purpose, ttl)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"ok": true, "lease": lease}, nil
}

func (s *Server) toolClaimNextWorkload(args json.RawMessage) (interface{}, error) {
	var in struct {
		Holder     string `json:"holder"`
		Purpose    string `json:"purpose"`
		TTL        string `json:"ttl"`
		TTLSeconds int64  `json:"ttl_seconds"`
		Source     string `json:"source"`
		Project    string `json:"project"`
		Repo       string `json:"repo"`
		Team       string `json:"team"`
		Owner      string `json:"owner"`
		Status     string `json:"status"`
		Query      string `json:"q"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	ttl, err := mcpLeaseTTL(in.TTL, in.TTLSeconds)
	if err != nil {
		return nil, err
	}
	result, err := s.db.ClaimNextWorkload(in.Holder, in.Purpose, ttl, storage.WorkloadClaimFilter{
		Source:  in.Source,
		Project: in.Project,
		Repo:    in.Repo,
		Team:    in.Team,
		Owner:   in.Owner,
		Status:  in.Status,
		Query:   in.Query,
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Server) toolWorkloadQueue(args json.RawMessage) (interface{}, error) {
	var in struct {
		Source  string `json:"source"`
		Project string `json:"project"`
		Repo    string `json:"repo"`
		Team    string `json:"team"`
		Owner   string `json:"owner"`
		Status  string `json:"status"`
		Query   string `json:"q"`
	}
	_ = json.Unmarshal(args, &in)
	return s.db.GetWorkloadQueueStats(storage.WorkloadClaimFilter{
		Source:  in.Source,
		Project: in.Project,
		Repo:    in.Repo,
		Team:    in.Team,
		Owner:   in.Owner,
		Status:  in.Status,
		Query:   in.Query,
	})
}

func (s *Server) toolRenewWorkloadLease(args json.RawMessage) (interface{}, error) {
	var in struct {
		LeaseID    string `json:"lease_id"`
		LeaseToken string `json:"lease_token"`
		TTL        string `json:"ttl"`
		TTLSeconds int64  `json:"ttl_seconds"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	ttl, err := mcpLeaseTTL(in.TTL, in.TTLSeconds)
	if err != nil {
		return nil, err
	}
	lease, err := s.db.RenewWorkloadLease(in.LeaseID, in.LeaseToken, ttl)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"ok": true, "lease": lease}, nil
}

func (s *Server) toolReleaseWorkloadLease(args json.RawMessage) (interface{}, error) {
	var in struct {
		LeaseID    string `json:"lease_id"`
		LeaseToken string `json:"lease_token"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	lease, err := s.db.ReleaseWorkloadLease(in.LeaseID, in.LeaseToken)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"ok": true, "lease": lease}, nil
}

func (s *Server) toolWorkloadLeases(args json.RawMessage) (interface{}, error) {
	var in struct {
		IncludeInactive bool `json:"include_inactive"`
		Limit           int  `json:"limit"`
	}
	_ = json.Unmarshal(args, &in)
	rows, err := s.db.ListWorkloadLeases(in.IncludeInactive, in.Limit)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"rows": rows, "include_inactive": in.IncludeInactive}, nil
}

func mcpLeaseTTL(raw string, seconds int64) (time.Duration, error) {
	if strings.TrimSpace(raw) != "" {
		ttl, err := time.ParseDuration(strings.TrimSpace(raw))
		if err != nil {
			return 0, err
		}
		if ttl <= 0 {
			return 0, fmt.Errorf("ttl must be positive")
		}
		return ttl, nil
	}
	if seconds > 0 {
		return time.Duration(seconds) * time.Second, nil
	}
	return 0, nil
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

func (s *Server) toolWorkloadTimeline(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID string `json:"workload_id"`
		Limit      int    `json:"limit"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	rows, err := s.db.GetWorkloadTimeline(in.WorkloadID, in.Limit)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"workload_id": in.WorkloadID, "rows": rows}, nil
}

func (s *Server) toolWorkloadState(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID string `json:"workload_id"`
		MaxAge     string `json:"max_age"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if in.WorkloadID == "" {
		return nil, fmt.Errorf("workload_id is required")
	}
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
	return s.db.GetWorkloadState(in.WorkloadID, maxAge)
}

func (s *Server) toolWorkloadFeed(args json.RawMessage) (interface{}, error) {
	var in struct {
		From       string `json:"from"`
		To         string `json:"to"`
		Source     string `json:"source"`
		Model      string `json:"model"`
		Project    string `json:"project"`
		Phase      string `json:"phase"`
		Severity   string `json:"severity"`
		Limit      int    `json:"limit"`
		StaleAfter string `json:"stale_after"`
	}
	_ = json.Unmarshal(args, &in)
	now := s.now()
	from, to, err := parseDateRange(in.From, in.To, now)
	if err != nil {
		return nil, err
	}
	staleAfter := 10 * time.Minute
	if in.StaleAfter != "" {
		parsed, err := time.ParseDuration(in.StaleAfter)
		if err != nil {
			return nil, err
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("stale_after must be positive")
		}
		staleAfter = parsed
	}
	return s.db.GetWorkloadEventFeed(from, to, in.Source, in.Model, in.Project, in.Phase, in.Severity, in.Limit, staleAfter)
}

func (s *Server) defaultWorkloadFeed() (*storage.WorkloadEventFeed, error) {
	now := s.now()
	return s.db.GetWorkloadEventFeed(now.AddDate(0, 0, -30), now.AddDate(0, 0, 1), "", "", "", "", "", 50, 10*time.Minute)
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

func (s *Server) toolRecordEvaluation(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID   string  `json:"workload_id"`
		RunID        string  `json:"run_id"`
		EvaluationID string  `json:"evaluation_id"`
		Source       string  `json:"source"`
		Evaluator    string  `json:"evaluator"`
		Status       string  `json:"status"`
		Score        float64 `json:"score"`
		Signal       string  `json:"signal"`
		Notes        string  `json:"notes"`
		EventID      string  `json:"event_id"`
		Timestamp    string  `json:"timestamp"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if in.WorkloadID == "" {
		return nil, fmt.Errorf("workload_id is required")
	}
	ts := time.Now().UTC()
	if in.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339Nano, in.Timestamp)
		if err != nil {
			return nil, err
		}
		ts = parsed
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"evaluation_id": in.EvaluationID,
		"evaluator":     firstNonEmpty(in.Evaluator, "mcp"),
		"status":        firstNonEmpty(in.Status, "unknown"),
		"score":         in.Score,
		"signal":        firstNonEmpty(in.Signal, "manual"),
		"notes":         in.Notes,
	})
	return s.db.IngestCanonicalEvent(storage.CanonicalEvent{
		EventID:       in.EventID,
		Source:        firstNonEmpty(in.Source, "mcp"),
		EventType:     "evaluation.recorded",
		SourceEventID: in.EvaluationID,
		WorkloadID:    in.WorkloadID,
		AgentRunID:    in.RunID,
		Timestamp:     ts,
		Payload:       payload,
		Confidence:    1,
	})
}

func (s *Server) toolRecordToolCall(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID string `json:"workload_id"`
		RunID      string `json:"run_id"`
		Source     string `json:"source"`
		ToolCallID string `json:"tool_call_id"`
		ToolName   string `json:"tool_name"`
		ToolType   string `json:"tool_type"`
		Status     string `json:"status"`
		ErrorClass string `json:"error_class"`
		DurationMS int64  `json:"duration_ms"`
		ParamsHash string `json:"params_hash"`
		EventID    string `json:"event_id"`
		Timestamp  string `json:"timestamp"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if in.WorkloadID == "" {
		return nil, fmt.Errorf("workload_id is required")
	}
	if in.ToolName == "" {
		return nil, fmt.Errorf("tool_name is required")
	}
	ts := time.Now().UTC()
	if in.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339Nano, in.Timestamp)
		if err != nil {
			return nil, err
		}
		ts = parsed
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"tool_name":   in.ToolName,
		"tool_type":   in.ToolType,
		"status":      firstNonEmpty(in.Status, "ok"),
		"error_class": in.ErrorClass,
		"duration_ms": in.DurationMS,
		"params_hash": in.ParamsHash,
	})
	return s.db.IngestCanonicalEvent(storage.CanonicalEvent{
		EventID:       in.EventID,
		Source:        firstNonEmpty(in.Source, "mcp"),
		EventType:     "tool.call",
		SourceEventID: in.ToolCallID,
		WorkloadID:    in.WorkloadID,
		AgentRunID:    in.RunID,
		Timestamp:     ts,
		Payload:       payload,
		Confidence:    1,
	})
}

func (s *Server) toolRecordContext(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID   string `json:"workload_id"`
		RunID        string `json:"run_id"`
		Source       string `json:"source"`
		ContextRefID string `json:"context_ref_id"`
		RefType      string `json:"ref_type"`
		RefHash      string `json:"ref_hash"`
		Label        string `json:"label"`
		Repo         string `json:"repo"`
		GitBranch    string `json:"git_branch"`
		CommitSHA    string `json:"commit_sha"`
		PrivacyLabel string `json:"privacy_label"`
		EventID      string `json:"event_id"`
		Timestamp    string `json:"timestamp"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	if in.WorkloadID == "" {
		return nil, fmt.Errorf("workload_id is required")
	}
	if in.RefHash == "" && in.Label == "" {
		return nil, fmt.Errorf("at least one of ref_hash or label is required")
	}
	ts := time.Now().UTC()
	if in.Timestamp != "" {
		parsed, err := time.Parse(time.RFC3339Nano, in.Timestamp)
		if err != nil {
			return nil, err
		}
		ts = parsed
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"ref_type":      firstNonEmpty(in.RefType, "context"),
		"ref_hash":      in.RefHash,
		"label":         in.Label,
		"repo":          in.Repo,
		"git_branch":    in.GitBranch,
		"commit_sha":    in.CommitSHA,
		"privacy_label": firstNonEmpty(in.PrivacyLabel, "local"),
	})
	return s.db.IngestCanonicalEvent(storage.CanonicalEvent{
		EventID:       in.EventID,
		Source:        firstNonEmpty(in.Source, "mcp"),
		EventType:     "context.ref",
		SourceEventID: in.ContextRefID,
		WorkloadID:    in.WorkloadID,
		AgentRunID:    in.RunID,
		GitBranch:     in.GitBranch,
		Timestamp:     ts,
		Payload:       payload,
		Confidence:    1,
	})
}

func (s *Server) toolRecordEvent(args json.RawMessage) (interface{}, error) {
	var event storage.CanonicalEvent
	if err := json.Unmarshal(args, &event); err != nil {
		return nil, err
	}
	return s.db.IngestCanonicalEvent(event)
}

func (s *Server) toolValidateEvent(args json.RawMessage) (interface{}, error) {
	var event storage.CanonicalEvent
	if err := json.Unmarshal(args, &event); err != nil {
		return nil, err
	}
	return storage.ValidateCanonicalEvent(event)
}

func (s *Server) toolAdapterConformance(args json.RawMessage) (interface{}, error) {
	var in struct {
		Kind     string `json:"kind"`
		Strict   bool   `json:"strict"`
		RawJSON  string `json:"raw_json"`
		RawInput string `json:"raw_input"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	raw := firstNonEmpty(in.RawInput, in.RawJSON)
	if strings.TrimSpace(raw) == "" {
		return nil, fmt.Errorf("raw_input or raw_json is required")
	}
	return integrations.RunAdapterConformanceWithOptions(integrations.AdapterConformanceOptions{
		Kind:   in.Kind,
		Strict: in.Strict,
	}, []byte(raw))
}

func toolIntegrationRecommendation(args json.RawMessage) (interface{}, error) {
	var in struct {
		AgentProfileID    string      `json:"agent_profile_id"`
		Agent             string      `json:"agent"`
		Profile           string      `json:"profile"`
		Framework         string      `json:"framework"`
		ProviderProfileID string      `json:"provider_profile_id"`
		Provider          string      `json:"provider"`
		Runtime           string      `json:"runtime"`
		Surface           string      `json:"surface"`
		Signals           interface{} `json:"signals"`
		RuntimeMode       string      `json:"runtime_mode"`
		ReadOnly          bool        `json:"read_only"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	req := integrations.IntegrationRecommendationRequest{
		AgentProfileID:    firstNonEmpty(in.AgentProfileID, in.Agent, in.Profile, in.Framework),
		ProviderProfileID: firstNonEmpty(in.ProviderProfileID, in.Provider, in.Runtime),
		Surface:           in.Surface,
		Signals:           mcpStringList(in.Signals),
		RuntimeMode:       in.RuntimeMode,
		ReadOnly:          in.ReadOnly,
	}
	return integrations.IntegrationRecommendation(req), nil
}

func toolIntegrationCompatibility(args json.RawMessage) (interface{}, error) {
	var in struct {
		AgentProfileID    string `json:"agent_profile_id"`
		Agent             string `json:"agent"`
		Profile           string `json:"profile"`
		Framework         string `json:"framework"`
		ProviderProfileID string `json:"provider_profile_id"`
		Provider          string `json:"provider"`
		Runtime           string `json:"runtime"`
		Surface           string `json:"surface"`
		MinConfidence     string `json:"min_confidence"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	req := integrations.IntegrationCompatibilityRequest{
		AgentProfileID:    firstNonEmpty(in.AgentProfileID, in.Agent, in.Profile, in.Framework),
		ProviderProfileID: firstNonEmpty(in.ProviderProfileID, in.Provider, in.Runtime),
		Surface:           in.Surface,
		MinConfidence:     in.MinConfidence,
	}
	return integrations.IntegrationCompatibilityReportFor(req), nil
}

func toolIntegrationRolloutPlan(args json.RawMessage) (interface{}, error) {
	var in struct {
		AgentProfileID    string `json:"agent_profile_id"`
		Agent             string `json:"agent"`
		Profile           string `json:"profile"`
		Framework         string `json:"framework"`
		ProviderProfileID string `json:"provider_profile_id"`
		Provider          string `json:"provider"`
		Runtime           string `json:"runtime"`
		Surface           string `json:"surface"`
		MinConfidence     string `json:"min_confidence"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	req := integrations.IntegrationRolloutRequest{
		AgentProfileID:    firstNonEmpty(in.AgentProfileID, in.Agent, in.Profile, in.Framework),
		ProviderProfileID: firstNonEmpty(in.ProviderProfileID, in.Provider, in.Runtime),
		Surface:           in.Surface,
		MinConfidence:     in.MinConfidence,
	}
	return integrations.IntegrationRolloutPlanFor(req), nil
}

func (s *Server) toolIntegrationEvidenceKit(args json.RawMessage) (interface{}, error) {
	var in struct {
		AgentProfileID    string `json:"agent_profile_id"`
		Agent             string `json:"agent"`
		Profile           string `json:"profile"`
		Framework         string `json:"framework"`
		ProviderProfileID string `json:"provider_profile_id"`
		Provider          string `json:"provider"`
		Runtime           string `json:"runtime"`
		Surface           string `json:"surface"`
		MinConfidence     string `json:"min_confidence"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	req := integrations.IntegrationEvidenceKitRequest{
		AgentProfileID:    firstNonEmpty(in.AgentProfileID, in.Agent, in.Profile, in.Framework),
		ProviderProfileID: firstNonEmpty(in.ProviderProfileID, in.Provider, in.Runtime),
		Surface:           in.Surface,
		MinConfidence:     in.MinConfidence,
	}
	return integrations.IntegrationEvidenceKitFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus(), req), nil
}

func (s *Server) toolIntegrationDrift(args json.RawMessage) (interface{}, error) {
	var in struct {
		Strict   bool              `json:"strict"`
		Expected map[string]string `json:"expected"`
		Hash     interface{}       `json:"hash"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	expected := map[string]string{}
	for key, value := range in.Expected {
		expected[key] = value
	}
	for _, raw := range mcpStringList(in.Hash) {
		key, value, ok := strings.Cut(raw, "=")
		if ok {
			expected[key] = value
		}
	}
	req := integrations.IntegrationDriftRequest{Strict: in.Strict, Expected: expected}
	return integrations.IntegrationDriftReportFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus(), req), nil
}

func (s *Server) toolIntegrationUpgradeGate(args json.RawMessage) (interface{}, error) {
	var in struct {
		Strict   bool              `json:"strict"`
		Expected map[string]string `json:"expected"`
		Hash     interface{}       `json:"hash"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	expected := map[string]string{}
	for key, value := range in.Expected {
		expected[key] = value
	}
	for _, raw := range mcpStringList(in.Hash) {
		key, value, ok := strings.Cut(raw, "=")
		if ok {
			expected[key] = value
		}
	}
	req := integrations.IntegrationUpgradeGateRequest{Strict: in.Strict, Expected: expected}
	return integrations.IntegrationUpgradeGateFor(integrations.OptionsFromConfig(s.cfg), s.runtimeStatus(), req), nil
}

func toolSchemaEvolutionGate(args json.RawMessage) (interface{}, error) {
	var in struct {
		Strict               bool        `json:"strict"`
		SchemaVersion        string      `json:"schema_version"`
		SchemaHash           string      `json:"schema_hash"`
		ExpectedVersion      string      `json:"expected_version"`
		ExpectedSchemaHash   string      `json:"expected_schema_hash"`
		RequiredEventTypes   []string    `json:"required_event_types"`
		RequiredRejectedKeys []string    `json:"required_rejected_keys"`
		EventType            interface{} `json:"event_type"`
		RejectedKey          interface{} `json:"rejected_key"`
	}
	if len(args) > 0 {
		if err := json.Unmarshal(args, &in); err != nil {
			return nil, err
		}
	}
	req := integrations.SchemaEvolutionGateRequest{
		Strict:               in.Strict,
		ExpectedVersion:      firstNonEmpty(in.ExpectedVersion, in.SchemaVersion),
		ExpectedSchemaHash:   firstNonEmpty(in.ExpectedSchemaHash, in.SchemaHash),
		RequiredEventTypes:   append(append([]string{}, in.RequiredEventTypes...), mcpStringList(in.EventType)...),
		RequiredRejectedKeys: append(append([]string{}, in.RequiredRejectedKeys...), mcpStringList(in.RejectedKey)...),
	}
	return integrations.SchemaEvolutionGateFor(req), nil
}

func mcpStringList(value interface{}) []string {
	switch v := value.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return strings.Split(v, ",")
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case []string:
		return v
	default:
		return nil
	}
}

func (s *Server) toolGetPolicy(args json.RawMessage) (interface{}, error) {
	var in struct {
		WorkloadID string `json:"workload_id"`
		RunID      string `json:"run_id"`
		Source     string `json:"source"`
		Model      string `json:"model"`
		Project    string `json:"project"`
		Repo       string `json:"repo"`
		GitBranch  string `json:"git_branch"`
		Team       string `json:"team"`
		Action     string `json:"action"`
		Target     string `json:"target"`
		Role       string `json:"role"`
	}
	_ = json.Unmarshal(args, &in)
	result := ledgerpolicy.Evaluate(s.cfg.Policies, ledgerpolicy.Request{
		WorkloadID: in.WorkloadID,
		RunID:      in.RunID,
		Source:     in.Source,
		Model:      in.Model,
		Project:    in.Project,
		Repo:       in.Repo,
		GitBranch:  in.GitBranch,
		Team:       in.Team,
		Action:     in.Action,
		Target:     in.Target,
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

func (s *Server) toolPolicyAudit(args json.RawMessage) (interface{}, error) {
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
	limit := in.Limit
	if limit <= 0 {
		limit = 200
	}
	candidates, err := s.db.GetPolicyAuditCandidates(from, to, in.Source, in.Model, in.Project, limit*5)
	if err != nil {
		return nil, err
	}
	report := ledgerpolicy.Audit(s.cfg.Policies, candidates, limit)
	report.WindowFrom = from.Format(time.RFC3339)
	report.WindowTo = to.Format(time.RFC3339)
	report.Scope = "usage_records,tool_calls,workloads"
	return report, nil
}

func (s *Server) toolApprovalRoutes(args json.RawMessage) (interface{}, error) {
	var in struct {
		DueWithin string `json:"due_within"`
		Limit     int    `json:"limit"`
		Privacy   bool   `json:"privacy"`
	}
	_ = json.Unmarshal(args, &in)
	dueWithin, err := boundedDuration(in.DueWithin, "due_within", 24*time.Hour, 30*24*time.Hour)
	if err != nil {
		return nil, err
	}
	report, err := s.db.GetApprovalRouteSummary(in.Limit, dueWithin)
	if err != nil {
		return nil, err
	}
	if in.Privacy {
		redactMCPApprovalRoutes(report)
	}
	return report, nil
}

func (s *Server) toolApprovals(args json.RawMessage) (interface{}, error) {
	var in struct {
		Status  string `json:"status"`
		Limit   int    `json:"limit"`
		Privacy bool   `json:"privacy"`
	}
	_ = json.Unmarshal(args, &in)
	status := firstNonEmpty(in.Status, "pending")
	rows, err := s.db.ListApprovalRequests(status, in.Limit)
	if err != nil {
		return nil, err
	}
	if in.Privacy {
		redactMCPApprovalRequests(rows)
	}
	return map[string]interface{}{"status": status, "rows": rows}, nil
}

func (s *Server) toolResolveApproval(args json.RawMessage) (interface{}, error) {
	var in struct {
		RequestID         string `json:"request_id"`
		Status            string `json:"status"`
		Voter             string `json:"voter"`
		Role              string `json:"role"`
		Note              string `json:"note"`
		RequiredApprovals int    `json:"required_approvals"`
	}
	if err := json.Unmarshal(args, &in); err != nil {
		return nil, err
	}
	status := firstNonEmpty(in.Status, "approved")
	role := firstNonEmpty(in.Role, "mcp")
	voter := firstNonEmpty(in.Voter, role)
	result, err := s.db.CastApprovalVote(in.RequestID, status, voter, role, in.Note, in.RequiredApprovals)
	if err != nil {
		return nil, err
	}
	_ = s.db.AppendAuditLog("local", role, "policy.approval."+result.Status, in.RequestID, map[string]string{
		"voter":              voter,
		"required_approvals": fmt.Sprint(result.RequiredApprovals),
		"approval_votes":     fmt.Sprint(result.ApprovalVotes),
		"rejection_votes":    fmt.Sprint(result.RejectionVotes),
		"decided":            fmt.Sprint(result.Decided),
		"note_present":       fmt.Sprint(strings.TrimSpace(in.Note) != ""),
	})
	return map[string]interface{}{"ok": true, "result": result}, nil
}

func (s *Server) toolAuditLog(args json.RawMessage) (interface{}, error) {
	var in struct {
		From    string `json:"from"`
		To      string `json:"to"`
		Actor   string `json:"actor"`
		Role    string `json:"role"`
		Action  string `json:"action"`
		Target  string `json:"target"`
		Limit   int    `json:"limit"`
		Privacy bool   `json:"privacy"`
	}
	_ = json.Unmarshal(args, &in)
	filter := storage.AuditLogFilter{
		Actor:  in.Actor,
		Role:   in.Role,
		Action: in.Action,
		Target: in.Target,
		Limit:  in.Limit,
	}
	if in.From != "" || in.To != "" {
		from, to, err := parseDateRange(in.From, in.To, s.now())
		if err != nil {
			return nil, err
		}
		filter.From = from
		filter.To = to
	}
	rows, err := s.db.QueryAuditLog(filter)
	if err != nil {
		return nil, err
	}
	if in.Privacy {
		redactMCPAuditRows(rows)
	}
	return map[string]interface{}{"count": len(rows), "rows": rows}, nil
}

func redactMCPAuditRows(rows []storage.AuditEvent) {
	for i := range rows {
		rows[i].Target = "<redacted>"
		rows[i].Params = "<redacted>"
	}
}

func redactMCPApprovalRoutes(report *storage.ApprovalRouteSummary) {
	if report == nil {
		return
	}
	for i := range report.Routes {
		report.Routes[i].RouteKey = "<redacted>"
		report.Routes[i].Approver = "<redacted>"
		report.Routes[i].EscalationTarget = "<redacted>"
		for j := range report.Routes[i].Projects {
			report.Routes[i].Projects[j] = "<redacted>"
		}
	}
}

func redactMCPApprovalRequests(rows []storage.ApprovalRequest) {
	for i := range rows {
		rows[i].RequestID = mcpHashValue(rows[i].RequestID)
		rows[i].PolicyDecisionID = mcpHashValue(rows[i].PolicyDecisionID)
		rows[i].WorkloadID = mcpHashValue(rows[i].WorkloadID)
		rows[i].RunID = mcpHashValue(rows[i].RunID)
		rows[i].Project = "<redacted>"
		rows[i].Target = "<redacted>"
		rows[i].ApproverHint = "<redacted>"
		rows[i].EscalationTarget = "<redacted>"
		rows[i].Reason = "<redacted>"
		rows[i].RequestPayload = "<redacted>"
		rows[i].DecidedBy = "<redacted>"
		rows[i].DecisionNote = "<redacted>"
	}
}

func mcpHashValue(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])[:16]
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
	now = now.UTC()
	if fromRaw == "" {
		return now.AddDate(0, 0, -30), now.AddDate(0, 0, 1), nil
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
