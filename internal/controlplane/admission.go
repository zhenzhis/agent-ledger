package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"
)

const AdmissionContract = "agent-ledger.admission-check"

// AdmissionInput describes a privacy-safe dry-run request for wrappers,
// routers, CI, and operators. It intentionally accepts operation metadata only;
// callers must not include prompt text, paths, payload bodies, or secrets.
type AdmissionInput struct {
	Surface        string `json:"surface"`
	Method         string `json:"method,omitempty"`
	Path           string `json:"path,omitempty"`
	Command        string `json:"command,omitempty"`
	Tool           string `json:"tool,omitempty"`
	Role           string `json:"role,omitempty"`
	RBACEnabled    bool   `json:"rbac_enabled"`
	AuthConfigured bool   `json:"auth_configured"`
	ReadOnly       bool   `json:"read_only"`
	DryRun         bool   `json:"dry_run,omitempty"`
	Record         bool   `json:"record,omitempty"`
	HasWorkloadID  bool   `json:"has_workload_id,omitempty"`
}

type AdmissionDecision struct {
	Product             string `json:"product"`
	Slug                string `json:"slug"`
	Contract            string `json:"contract"`
	Version             string `json:"version"`
	GeneratedAt         string `json:"generated_at"`
	Status              string `json:"status"`
	Allowed             bool   `json:"allowed"`
	Surface             string `json:"surface"`
	Operation           string `json:"operation"`
	Role                string `json:"role"`
	RequiredRole        string `json:"required_role"`
	RBACEnabled         bool   `json:"rbac_enabled"`
	AuthConfigured      bool   `json:"auth_configured"`
	ReadOnly            bool   `json:"read_only"`
	KnownOperation      bool   `json:"known_operation"`
	WritesLocalState    bool   `json:"writes_local_state"`
	WriteMode           string `json:"write_mode"`
	AvailableInReadOnly bool   `json:"available_in_read_only"`
	LocalOrAuthRequired bool   `json:"local_or_auth_required"`
	PromptContentStored bool   `json:"prompt_content_stored"`
	UsageDataUploaded   bool   `json:"usage_data_uploaded"`
	Reason              string `json:"reason"`
	Action              string `json:"action,omitempty"`
	PrivacyNote         string `json:"privacy_note"`
}

type OperationAccess struct {
	Known               bool
	WritesLocalState    bool
	WriteMode           string
	AvailableInReadOnly bool
	ReadOnlyBehavior    string
	RequiredRole        string
	LocalOrAuthRequired bool
	Reason              string
	Action              string
}

// EvaluateAdmission checks whether a REST, CLI, or MCP operation is allowed
// for the current runtime without reading request bodies or mutating state.
func EvaluateAdmission(input AdmissionInput, now time.Time) AdmissionDecision {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	input = normalizeAdmissionInput(input)
	access := accessForAdmission(input)
	role := effectiveAdmissionRole(input)
	roleOK := roleRank(role) >= roleRank(access.RequiredRole)
	allowed := access.Known && roleOK && (!input.ReadOnly || access.AvailableInReadOnly)
	reason := access.Reason
	action := access.Action
	if !access.Known {
		reason = "operation is unknown to the local admission catalog"
		action = "check the integration catalog or upgrade Agent Ledger before calling this operation"
	} else if !roleOK {
		reason = fmt.Sprintf("role %q is below required role %q", role, access.RequiredRole)
		action = "use a token with the required role or choose a read-only operation"
	} else if input.ReadOnly && !access.AvailableInReadOnly {
		reason = "read-only observer mode rejects this write-capable operation"
		action = "restart without rbac.read_only or use a validate/dry-run endpoint"
	}
	status := "denied"
	if allowed {
		status = "allowed"
	}
	return AdmissionDecision{
		Product:             "Agent Ledger",
		Slug:                "agent-ledger",
		Contract:            AdmissionContract,
		Version:             "v1",
		GeneratedAt:         now.UTC().Format(time.RFC3339Nano),
		Status:              status,
		Allowed:             allowed,
		Surface:             input.Surface,
		Operation:           operationLabel(input),
		Role:                role,
		RequiredRole:        access.RequiredRole,
		RBACEnabled:         input.RBACEnabled,
		AuthConfigured:      input.AuthConfigured,
		ReadOnly:            input.ReadOnly,
		KnownOperation:      access.Known,
		WritesLocalState:    access.WritesLocalState && !input.ReadOnly,
		WriteMode:           access.WriteMode,
		AvailableInReadOnly: access.AvailableInReadOnly,
		LocalOrAuthRequired: access.LocalOrAuthRequired,
		PromptContentStored: false,
		UsageDataUploaded:   false,
		Reason:              reason,
		Action:              action,
		PrivacyNote:         "Admission checks expose only operation metadata, role requirements, read-only behavior, and remediation hints; prompt content, request bodies, paths, secrets, sessions, projects, branches, machine names, and authors are excluded.",
	}
}

func FormatAdmissionMarkdown(decision AdmissionDecision) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent Ledger Admission\n\n")
	fmt.Fprintf(&b, "- Status: `%s`\n", decision.Status)
	fmt.Fprintf(&b, "- Operation: `%s`\n", decision.Operation)
	fmt.Fprintf(&b, "- Surface: `%s`\n", decision.Surface)
	fmt.Fprintf(&b, "- Role: `%s` requires `%s`\n", decision.Role, decision.RequiredRole)
	fmt.Fprintf(&b, "- Read only: `%t`\n", decision.ReadOnly)
	fmt.Fprintf(&b, "- Writes local state: `%t`\n", decision.WritesLocalState)
	fmt.Fprintf(&b, "- Write mode: `%s`\n", decision.WriteMode)
	fmt.Fprintf(&b, "- Available in read-only: `%t`\n", decision.AvailableInReadOnly)
	fmt.Fprintf(&b, "- Reason: %s\n", decision.Reason)
	if decision.Action != "" {
		fmt.Fprintf(&b, "- Action: %s\n", decision.Action)
	}
	return b.String()
}

func AdmissionFingerprint(decision AdmissionDecision) string {
	decision.GeneratedAt = ""
	raw, err := json.Marshal(decision)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func AdmissionInputFromValues(values url.Values, defaults AdmissionInput) AdmissionInput {
	out := defaults
	out.Surface = firstNonEmpty(values.Get("surface"), out.Surface)
	out.Method = firstNonEmpty(values.Get("method"), out.Method)
	out.Path = firstNonEmpty(values.Get("path"), out.Path)
	out.Command = firstNonEmpty(values.Get("command"), out.Command)
	out.Tool = firstNonEmpty(values.Get("tool"), out.Tool)
	out.Role = firstNonEmpty(values.Get("role"), out.Role)
	out.DryRun = boolValue(values, "dry_run", out.DryRun)
	out.Record = boolValue(values, "record", out.Record)
	out.HasWorkloadID = boolValue(values, "has_workload_id", out.HasWorkloadID) || strings.TrimSpace(values.Get("workload_id")) != "" || strings.TrimSpace(values.Get("workload-id")) != ""
	return out
}

func IsHTTPSafeMethod(method string) bool {
	method = strings.ToUpper(strings.TrimSpace(method))
	return method == "GET" || method == "HEAD" || method == "OPTIONS"
}

func IsReadOnlyAllowedHTTP(method, path string, dryRun bool) bool {
	access := HTTPAccessFor(method, path, AdmissionInput{ReadOnly: true, DryRun: dryRun})
	return access.Known && access.AvailableInReadOnly
}

func HTTPAccessFor(method, path string, input AdmissionInput) OperationAccess {
	method = strings.ToUpper(strings.TrimSpace(method))
	path = canonicalPath(path)
	if IsHTTPSafeMethod(method) {
		return OperationAccess{
			Known:               true,
			WriteMode:           "none",
			AvailableInReadOnly: true,
			ReadOnlyBehavior:    "safe HTTP method; available in read-only mode",
			RequiredRole:        "viewer",
			Reason:              "safe HTTP query",
		}
	}
	if method != "POST" && method != "PUT" && method != "PATCH" && method != "DELETE" {
		return unknownAccess("unsupported HTTP method")
	}
	switch path {
	case "/api/events/validate", "/api/integrations/conformance":
		return readOnlyHTTPAccess("metadata validation does not write SQLite")
	case "/api/policy/evaluate":
		writes := input.Record || input.HasWorkloadID || !input.ReadOnly
		return OperationAccess{
			Known:               true,
			WritesLocalState:    writes,
			WriteMode:           "conditional",
			AvailableInReadOnly: !input.Record && !input.HasWorkloadID,
			ReadOnlyBehavior:    "advisory policy evaluation is allowed in read-only mode only when it does not record a decision",
			RequiredRole:        "operator",
			LocalOrAuthRequired: true,
			Reason:              "policy evaluation may write audit metadata or policy decisions depending on record/workload_id",
		}
	case "/api/notifications/webhook":
		return OperationAccess{
			Known:               true,
			WritesLocalState:    !input.ReadOnly,
			WriteMode:           "conditional",
			AvailableInReadOnly: input.DryRun,
			ReadOnlyBehavior:    "only dry_run webhook previews are allowed in read-only mode",
			RequiredRole:        "operator",
			LocalOrAuthRequired: true,
			Reason:              "webhook notification may write audit metadata and can send outbound traffic unless dry_run is set",
		}
	case "/api/scan", "/api/agent-runs", "/api/agent-runs/heartbeat", "/api/workloads", "/api/workloads/close", "/api/workloads/link", "/api/workloads/lease", "/api/workloads/lease/renew", "/api/workloads/lease/release", "/api/events", "/api/otel/genai", "/api/otlp/v1/traces", "/v1/traces", "/api/a2a/tasks", "/api/provider/calls", "/api/reconciliation/import", "/api/offline-bundle/import":
		return writeHTTPAccess("operator", "operation writes local ledger state")
	case "/api/pricing/sync", "/api/pricing/recalculate", "/api/recalculate-costs", "/api/projections/repair":
		return writeHTTPAccess("admin", "operation mutates pricing, derived costs, or projections")
	case "/api/offline-bundle/export", "/api/export", "/api/report":
		return OperationAccess{
			Known:               true,
			WritesLocalState:    !input.ReadOnly,
			WriteMode:           "conditional",
			AvailableInReadOnly: true,
			ReadOnlyBehavior:    "export/report reads are allowed in read-only mode; audit writebacks are suppressed",
			RequiredRole:        "viewer",
			LocalOrAuthRequired: true,
			Reason:              "export/report may append audit metadata when not in read-only mode",
		}
	case "/api/policy/approvals":
		return writeHTTPAccess("admin", "approval votes mutate local policy approval state")
	case "/gateway/openai/v1/chat/completions", "/gateway/openai/v1/responses", "/gateway/anthropic/v1/messages":
		return writeHTTPAccess("operator", "gateway calls proxy upstream traffic and write usage/audit metadata")
	default:
		return unknownAccess("unknown HTTP path")
	}
}

func MCPToolAccessFor(name string) OperationAccess {
	switch strings.TrimSpace(name) {
	case "ledger.start_workload",
		"ledger.start_run",
		"ledger.close_workload",
		"ledger.link_workloads",
		"ledger.acquire_workload_lease",
		"ledger.renew_workload_lease",
		"ledger.release_workload_lease",
		"ledger.heartbeat_run",
		"ledger.record_tool_call",
		"ledger.record_artifact",
		"ledger.record_evaluation",
		"ledger.record_context",
		"ledger.record_event",
		"ledger.resolve_approval":
		return OperationAccess{
			Known:               true,
			WritesLocalState:    true,
			WriteMode:           "always",
			AvailableInReadOnly: false,
			ReadOnlyBehavior:    "disabled in read-only mode",
			RequiredRole:        "operator",
			Reason:              "MCP tool writes local ledger state",
		}
	case "ledger.get_policy":
		return OperationAccess{
			Known:               true,
			WritesLocalState:    true,
			WriteMode:           "conditional",
			AvailableInReadOnly: true,
			ReadOnlyBehavior:    "advisory calls are available; calls with workload_id are rejected because they record policy decisions",
			RequiredRole:        "viewer",
			Reason:              "policy reads are advisory unless workload_id is provided",
		}
	default:
		if strings.HasPrefix(strings.TrimSpace(name), "ledger.") {
			return OperationAccess{
				Known:               true,
				WriteMode:           "none",
				AvailableInReadOnly: true,
				ReadOnlyBehavior:    "available in read-only mode",
				RequiredRole:        "viewer",
				Reason:              "MCP tool is read-only",
			}
		}
		return unknownAccess("unknown MCP tool")
	}
}

func CLICommandAccessFor(command string, input AdmissionInput) OperationAccess {
	command = strings.ToLower(strings.TrimSpace(command))
	if command == "" {
		return unknownAccess("missing CLI command")
	}
	parts := strings.Fields(command)
	if len(parts) > 0 && parts[0] == "agent-ledger" {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return unknownAccess("missing CLI command")
	}
	switch parts[0] {
	case "version", "today", "top", "doctor", "battery", "wrapped", "discovery", "contracts", "openapi", "integrations", "runtime", "config", "readiness", "admission", "adapter", "replay", "badge", "preflight", "chargeback", "fleet":
		return OperationAccess{Known: true, WriteMode: "none", AvailableInReadOnly: true, ReadOnlyBehavior: "available in read-only mode", RequiredRole: "viewer", Reason: "CLI command is read-only or dry-run validation"}
	case "event":
		if len(parts) > 1 && parts[1] == "ingest" {
			return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "disabled in read-only mode", RequiredRole: "operator", Reason: "event ingest writes canonical events"}
		}
		return OperationAccess{Known: true, WriteMode: "none", AvailableInReadOnly: true, ReadOnlyBehavior: "validation/schema commands are available in read-only mode", RequiredRole: "viewer", Reason: "event command does not write unless ingest is used"}
	case "workload":
		if len(parts) > 1 && (parts[1] == "lease" || parts[1] == "leases") {
			if len(parts) == 2 || readOnlySubcommand(parts[2], []string{"list"}) {
				return OperationAccess{Known: true, WriteMode: "none", AvailableInReadOnly: true, ReadOnlyBehavior: "workload lease list is available in read-only mode", RequiredRole: "viewer", Reason: "workload lease list reads local ledger state"}
			}
			return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "disabled in read-only mode", RequiredRole: "operator", Reason: "workload lease command writes local ledger state"}
		}
		if len(parts) == 1 || readOnlySubcommand(parts[1], []string{"list", "show", "timeline", "state", "status", "feed", "events", "liveness"}) {
			return OperationAccess{Known: true, WriteMode: "none", AvailableInReadOnly: true, ReadOnlyBehavior: "workload query commands are available in read-only mode", RequiredRole: "viewer", Reason: "workload command reads local ledger state"}
		}
		return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "disabled in read-only mode", RequiredRole: "operator", Reason: "workload command writes local ledger state"}
	case "provider", "otel", "a2a":
		if len(parts) > 1 && parts[1] == "convert" {
			return OperationAccess{Known: true, WriteMode: "none", AvailableInReadOnly: true, ReadOnlyBehavior: "convert commands are available in read-only mode", RequiredRole: "viewer", Reason: "convert validates and maps input without writing SQLite"}
		}
		return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "disabled in read-only mode", RequiredRole: "operator", Reason: "ingest command writes local ledger state"}
	case "reconcile":
		if len(parts) > 1 && (parts[1] == "status" || parts[1] == "parse") {
			return OperationAccess{Known: true, WriteMode: "none", AvailableInReadOnly: true, ReadOnlyBehavior: "reconcile status/parse are available in read-only mode", RequiredRole: "viewer", Reason: "reconcile command reads or parses local/provider summary data"}
		}
		return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "disabled in read-only mode", RequiredRole: "operator", Reason: "reconcile import writes local reconciliation records"}
	case "run":
		return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "disabled in read-only mode", RequiredRole: "operator", Reason: "CLI command writes local ledger state"}
	case "pricing", "projection":
		return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "disabled in read-only mode", RequiredRole: "admin", Reason: "CLI command mutates pricing or derived projections"}
	case "bundle":
		if len(parts) > 1 && parts[1] == "import" {
			return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "disabled in read-only mode", RequiredRole: "operator", Reason: "bundle import writes canonical events"}
		}
		return OperationAccess{Known: true, WritesLocalState: !input.ReadOnly, WriteMode: "conditional", AvailableInReadOnly: true, ReadOnlyBehavior: "bundle export is allowed in read-only mode; audit writebacks are suppressed", RequiredRole: "viewer", Reason: "bundle export may write audit metadata when not in read-only mode"}
	case "policy":
		if len(parts) > 1 && parts[1] == "resolve" {
			return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "approval resolution is disabled in read-only mode", RequiredRole: "admin", Reason: "policy resolve writes approval votes"}
		}
		if len(parts) > 1 && parts[1] == "evaluate" {
			return OperationAccess{Known: true, WritesLocalState: input.Record || input.HasWorkloadID || !input.ReadOnly, WriteMode: "conditional", AvailableInReadOnly: !input.Record && !input.HasWorkloadID, ReadOnlyBehavior: "advisory evaluation is allowed in read-only mode only when no decision is recorded", RequiredRole: "operator", Reason: "policy evaluate may write audit metadata or decisions"}
		}
		return OperationAccess{Known: true, WriteMode: "none", AvailableInReadOnly: true, ReadOnlyBehavior: "policy read commands are available in read-only mode", RequiredRole: "viewer", Reason: "policy command is read-only"}
	case "notify":
		return OperationAccess{Known: true, WritesLocalState: !input.ReadOnly, WriteMode: "conditional", AvailableInReadOnly: input.DryRun, ReadOnlyBehavior: "only dry-run notification previews are allowed in read-only mode", RequiredRole: "operator", Reason: "notify can send outbound traffic and write audit metadata"}
	default:
		return unknownAccess("unknown CLI command")
	}
}

func normalizeAdmissionInput(input AdmissionInput) AdmissionInput {
	input.Surface = strings.ToLower(strings.TrimSpace(input.Surface))
	input.Method = strings.ToUpper(strings.TrimSpace(input.Method))
	input.Path = canonicalPath(input.Path)
	input.Command = strings.TrimSpace(input.Command)
	input.Tool = strings.TrimSpace(input.Tool)
	input.Role = strings.ToLower(strings.TrimSpace(input.Role))
	if input.Surface == "" {
		switch {
		case input.Path != "" || input.Method != "":
			input.Surface = "http"
		case input.Tool != "":
			input.Surface = "mcp"
		case input.Command != "":
			input.Surface = "cli"
		default:
			input.Surface = "unknown"
		}
	}
	return input
}

func accessForAdmission(input AdmissionInput) OperationAccess {
	switch input.Surface {
	case "http", "rest":
		return HTTPAccessFor(input.Method, input.Path, input)
	case "mcp":
		access := MCPToolAccessFor(input.Tool)
		if access.WriteMode == "conditional" && input.Tool == "ledger.get_policy" && input.HasWorkloadID {
			access.AvailableInReadOnly = false
			access.WritesLocalState = true
		}
		return access
	case "cli":
		return CLICommandAccessFor(input.Command, input)
	default:
		return unknownAccess("unknown surface")
	}
}

func operationLabel(input AdmissionInput) string {
	switch input.Surface {
	case "http", "rest":
		return strings.TrimSpace(input.Method + " " + input.Path)
	case "mcp":
		return input.Tool
	case "cli":
		return cliOperationLabel(input.Command)
	default:
		return ""
	}
}

func cliOperationLabel(command string) string {
	parts := strings.Fields(strings.TrimSpace(command))
	if len(parts) == 0 {
		return ""
	}
	if parts[0] == "agent-ledger" {
		parts = parts[1:]
	}
	if len(parts) == 0 {
		return "agent-ledger"
	}
	limit := 1
	if len(parts) > 1 && !strings.HasPrefix(parts[1], "-") {
		limit = 2
	}
	return "agent-ledger " + strings.Join(parts[:limit], " ")
}

func readOnlySubcommand(subcommand string, allowed []string) bool {
	for _, value := range allowed {
		if subcommand == value {
			return true
		}
	}
	return false
}

func effectiveAdmissionRole(input AdmissionInput) string {
	if input.Role != "" {
		return input.Role
	}
	if !input.RBACEnabled && !input.AuthConfigured {
		return "admin"
	}
	return "anonymous"
}

func roleRank(role string) int {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "viewer":
		return 1
	case "operator":
		return 2
	case "admin":
		return 3
	default:
		return 0
	}
}

func readOnlyHTTPAccess(reason string) OperationAccess {
	return OperationAccess{Known: true, WriteMode: "none", AvailableInReadOnly: true, ReadOnlyBehavior: "available in read-only mode", RequiredRole: "viewer", LocalOrAuthRequired: true, Reason: reason}
}

func writeHTTPAccess(requiredRole, reason string) OperationAccess {
	return OperationAccess{Known: true, WritesLocalState: true, WriteMode: "always", AvailableInReadOnly: false, ReadOnlyBehavior: "disabled in read-only mode", RequiredRole: requiredRole, LocalOrAuthRequired: true, Reason: reason}
}

func unknownAccess(reason string) OperationAccess {
	return OperationAccess{Known: false, WriteMode: "unknown", AvailableInReadOnly: false, RequiredRole: "viewer", Reason: reason, Action: "verify the operation name and refresh the local integration catalog"}
}

func canonicalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if parsed, err := url.Parse(path); err == nil && parsed.Path != "" {
		path = parsed.Path
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func boolValue(values url.Values, key string, fallback bool) bool {
	raw := strings.ToLower(strings.TrimSpace(values.Get(key)))
	if raw == "" {
		return fallback
	}
	return raw == "1" || raw == "true" || raw == "yes"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
