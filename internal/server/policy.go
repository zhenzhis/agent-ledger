package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	ledgerpolicy "github.com/zhenzhis/agent-ledger/internal/policy"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func (s *Server) handlePolicyEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	var payload struct {
		ledgerpolicy.Request
		Record *bool `json:"record"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		badRequest(w, err)
		return
	}
	result := ledgerpolicy.Evaluate(s.options.Policies, payload.Request)
	shouldRecord := payload.WorkloadID != "" && result.Enabled && len(result.Decisions) > 0
	if payload.Record != nil {
		shouldRecord = *payload.Record && result.Enabled && len(result.Decisions) > 0
	}
	if shouldRecord && payload.WorkloadID == "" {
		badRequest(w, fmt.Errorf("record=true requires workload_id"))
		return
	}
	if shouldRecord && !s.canWriteDerivedData() {
		http.Error(w, "read-only mode: policy decision recording is disabled", http.StatusForbidden)
		return
	}
	if shouldRecord {
		for i := range result.Decisions {
			id, err := s.db.RecordPolicyDecision(payload.WorkloadID, payload.RunID, result.Decisions[i].Rule, result.Decisions[i].Action, result.Decisions[i].Message, s.roleFor(r))
			if err != nil {
				serverError(w, err)
				return
			}
			result.Decisions[i].DecisionID = id
		}
	}
	s.appendAuditLog("local", s.roleFor(r), "policy.evaluate", result.Action, map[string]string{
		"source": payload.Source,
		"model":  payload.Model,
		"action": payload.Action,
		"target": payload.Target,
		"record": fmt.Sprint(shouldRecord),
	})
	writeJSON(w, result)
}

func (s *Server) handlePolicyAudit(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "viewer") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	limit := parseLimit(r, 200)
	candidates, err := s.db.GetPolicyAuditCandidates(from, to, r.URL.Query().Get("source"), r.URL.Query().Get("model"), r.URL.Query().Get("project"), limit*5)
	if err != nil {
		serverError(w, err)
		return
	}
	report := ledgerpolicy.Audit(s.options.Policies, candidates, limit)
	report.WindowFrom = from.Format(time.RFC3339)
	report.WindowTo = to.Format(time.RFC3339)
	report.Scope = "usage_records,tool_calls,workloads"
	applyPolicyAuditPrivacy(&report, s.privacyFor(r))
	writeJSONWithPayloadETag(w, r, report)
}

func (s *Server) handlePolicyEnforcement(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "viewer") {
		return
	}
	report, err := s.db.GetPolicyEnforcementReport(parseLimit(r, 200))
	if err != nil {
		serverError(w, err)
		return
	}
	applyPolicyEnforcementPrivacy(report, s.privacyFor(r))
	writeJSONWithPayloadETag(w, r, report, "generated_at")
}

func (s *Server) handlePolicyApprovalRoutes(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "viewer") {
		return
	}
	dueWithin := 24 * time.Hour
	if raw := strings.TrimSpace(r.URL.Query().Get("due_within")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 || parsed > 30*24*time.Hour {
			badRequest(w, fmt.Errorf("invalid due_within %q: expected duration from 1ns to 720h", raw))
			return
		}
		dueWithin = parsed
	}
	report, err := s.db.GetApprovalRouteSummary(parseLimit(r, 200), dueWithin)
	if err != nil {
		serverError(w, err)
		return
	}
	applyApprovalRoutePrivacy(report, s.privacyFor(r))
	writeJSONWithPayloadETag(w, r, report, "generated_at")
}

func (s *Server) evaluateOperationPolicy(w http.ResponseWriter, r *http.Request, action, source, model, project, target string) bool {
	result := ledgerpolicy.Evaluate(s.options.Policies, ledgerpolicy.Request{
		Source:  source,
		Model:   model,
		Project: project,
		Action:  action,
		Target:  target,
		Role:    s.roleFor(r),
	})
	if result.Enabled && len(result.Decisions) > 0 && s.canWriteDerivedData() {
		raw, _ := json.Marshal(result.Decisions)
		s.appendAuditLog("local", s.roleFor(r), "policy.evaluate", target, map[string]string{
			"action":           action,
			"target":           target,
			"effective_action": result.Action,
			"source":           source,
			"model":            model,
			"project":          project,
			"decisions":        string(raw),
		})
	}
	switch ledgerpolicy.NormalizeAction(result.Action) {
	case "block":
		http.Error(w, "blocked by policy", http.StatusForbidden)
		return false
	case "require_approval":
		approvalID := r.URL.Query().Get("approval_id")
		if approvalID == "" {
			approvalID = r.Header.Get("X-Agent-Ledger-Approval")
		}
		allowed, err := s.db.ApprovalAllowsOperation(storage.ApprovalOperation{
			RequestID: approvalID,
			Action:    action,
			Target:    target,
			Source:    source,
			Model:     model,
			Project:   project,
		})
		if err != nil {
			serverError(w, err)
			return false
		}
		if allowed {
			s.appendAuditLog("local", s.roleFor(r), "policy.approval.used", approvalID, map[string]string{"action": action, "target": target})
			return true
		}
		if !s.canWriteDerivedData() {
			http.Error(w, "operation requires approval by policy; approval request creation is disabled in read-only mode", http.StatusForbidden)
			return false
		}
		requestID, err := s.createPolicyApprovalRequest(r, result, action, source, model, project, target)
		if err != nil {
			serverError(w, err)
			return false
		}
		http.Error(w, fmt.Sprintf("operation requires approval by policy; approval_request_id=%s", requestID), http.StatusForbidden)
		return false
	default:
		return true
	}
}

func applyPolicyAuditPrivacy(report *ledgerpolicy.AuditReport, privacy config.PrivacyConfig) {
	if report == nil {
		return
	}
	for i := range report.Rows {
		if privacy.HashSessionIDs || privacy.ScreenshotMode {
			report.Rows[i].SessionID = hashValue(report.Rows[i].SessionID)
		}
		if privacy.HideProjectNames || privacy.RedactPaths || privacy.ScreenshotMode {
			report.Rows[i].Project = "<redacted>"
			report.Rows[i].Repo = "<redacted>"
			report.Rows[i].GitBranch = "<redacted>"
			report.Rows[i].Team = "<redacted>"
		}
		if privacy.ScreenshotMode {
			report.Rows[i].WorkloadID = hashValue(report.Rows[i].WorkloadID)
			report.Rows[i].RunID = hashValue(report.Rows[i].RunID)
			report.Rows[i].Target = "<redacted>"
			report.Rows[i].Evidence = "<redacted>"
		}
	}
}

func applyPolicyEnforcementPrivacy(report *storage.PolicyEnforcementReport, privacy config.PrivacyConfig) {
	if report == nil {
		return
	}
	if privacy.HashSessionIDs || privacy.ScreenshotMode {
		for i := range report.Decisions {
			report.Decisions[i].DecisionID = hashValue(report.Decisions[i].DecisionID)
			report.Decisions[i].WorkloadID = hashValue(report.Decisions[i].WorkloadID)
			report.Decisions[i].RunID = hashValue(report.Decisions[i].RunID)
		}
	}
	applyApprovalRequestsPrivacy(report.ApprovalRequests, privacy)
	if privacy.ScreenshotMode {
		for i := range report.Decisions {
			report.Decisions[i].Reason = "<redacted>"
		}
	}
	applyAuditEventPrivacy(report.AuditEvents, privacy)
}

func applyApprovalRequestsPrivacy(rows []storage.ApprovalRequest, privacy config.PrivacyConfig) {
	if !(privacy.RedactPaths || privacy.HideProjectNames || privacy.HashSessionIDs || privacy.ScreenshotMode) {
		return
	}
	for i := range rows {
		if privacy.HashSessionIDs || privacy.ScreenshotMode {
			rows[i].RequestID = hashValue(rows[i].RequestID)
			rows[i].PolicyDecisionID = hashValue(rows[i].PolicyDecisionID)
			rows[i].WorkloadID = hashValue(rows[i].WorkloadID)
			rows[i].RunID = hashValue(rows[i].RunID)
		}
		if privacy.HideProjectNames || privacy.RedactPaths || privacy.ScreenshotMode {
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
}

func applyApprovalRoutePrivacy(report *storage.ApprovalRouteSummary, privacy config.PrivacyConfig) {
	if report == nil || !(privacy.RedactPaths || privacy.HideProjectNames || privacy.HashSessionIDs || privacy.ScreenshotMode) {
		return
	}
	for i := range report.Routes {
		if privacy.ScreenshotMode || privacy.HashSessionIDs || privacy.HideProjectNames {
			report.Routes[i].RouteKey = hashValue(report.Routes[i].RouteKey)
		}
		if privacy.ScreenshotMode || privacy.HashSessionIDs || privacy.HideProjectNames {
			report.Routes[i].Approver = "<redacted>"
			report.Routes[i].EscalationTarget = "<redacted>"
		}
		if privacy.HideProjectNames || privacy.RedactPaths || privacy.ScreenshotMode {
			for j := range report.Routes[i].Projects {
				report.Routes[i].Projects[j] = "<redacted>"
			}
		}
	}
}

func (s *Server) createPolicyApprovalRequest(r *http.Request, result ledgerpolicy.Result, action, source, model, project, target string) (string, error) {
	reason := result.Action
	if len(result.Decisions) > 0 {
		reason = result.Decisions[0].Message
		if reason == "" {
			reason = result.Decisions[0].Rule
		}
	}
	routing := approvalRoutingFromDecisions(result.Decisions)
	raw, _ := json.Marshal(map[string]interface{}{
		"effective_action":       result.Action,
		"decisions":              result.Decisions,
		"required_approvals":     routing.requiredApprovals,
		"approvers":              routing.approvers,
		"escalate_after_seconds": routing.escalateAfterSeconds,
		"escalate_to":            routing.escalateTo,
	})
	requestID, err := s.db.CreateApprovalRequest(storage.ApprovalRequest{
		Source:                 source,
		Model:                  model,
		Project:                project,
		Action:                 action,
		Target:                 target,
		ActorRole:              s.roleFor(r),
		Status:                 "pending",
		RequiredApprovals:      routing.requiredApprovals,
		ApproverHint:           strings.Join(routing.approvers, ","),
		EscalationTarget:       strings.Join(routing.escalateTo, ","),
		EscalationAfterSeconds: routing.escalateAfterSeconds,
		Reason:                 reason,
		RequestPayload:         string(raw),
	})
	if err != nil {
		return "", err
	}
	s.appendAuditLog("local", s.roleFor(r), "policy.approval.requested", requestID, map[string]string{"action": action, "target": target, "source": source, "model": model, "project": project, "required_approvals": fmt.Sprint(routing.requiredApprovals), "approvers": strings.Join(routing.approvers, ","), "escalate_to": strings.Join(routing.escalateTo, ","), "escalate_after_seconds": fmt.Sprint(routing.escalateAfterSeconds)})
	return requestID, nil
}

type approvalRouting struct {
	requiredApprovals    int
	approvers            []string
	escalateAfterSeconds int64
	escalateTo           []string
}

func approvalRoutingFromDecisions(decisions []ledgerpolicy.Decision) approvalRouting {
	routing := approvalRouting{requiredApprovals: 1}
	approvers := map[string]bool{}
	escalateTo := map[string]bool{}
	for _, decision := range decisions {
		if ledgerpolicy.NormalizeAction(decision.Action) != "require_approval" {
			continue
		}
		if decision.RequiredApprovals > routing.requiredApprovals {
			routing.requiredApprovals = decision.RequiredApprovals
		}
		for _, approver := range decision.Approvers {
			addUniqueRoutingValue(approver, approvers, &routing.approvers)
		}
		for _, target := range decision.EscalateTo {
			addUniqueRoutingValue(target, escalateTo, &routing.escalateTo)
		}
		if decision.EscalateAfterSeconds > 0 && (routing.escalateAfterSeconds == 0 || decision.EscalateAfterSeconds < routing.escalateAfterSeconds) {
			routing.escalateAfterSeconds = decision.EscalateAfterSeconds
		}
	}
	if routing.requiredApprovals <= 0 {
		routing.requiredApprovals = 1
	}
	if routing.requiredApprovals > 20 {
		routing.requiredApprovals = 20
	}
	return routing
}

func addUniqueRoutingValue(value string, seen map[string]bool, out *[]string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	key := strings.ToLower(value)
	if seen[key] {
		return
	}
	seen[key] = true
	*out = append(*out, value)
}

func (s *Server) handlePolicyApprovals(w http.ResponseWriter, r *http.Request) {
	if !s.requireLocalOrAuth(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		if !s.requireRole(w, r, "operator") {
			return
		}
		status := r.URL.Query().Get("status")
		if status == "" {
			status = "pending"
		}
		rows, err := s.db.ListApprovalRequests(status, parseLimit(r, 200))
		if err != nil {
			serverError(w, err)
			return
		}
		applyApprovalRequestsPrivacy(rows, s.privacyFor(r))
		writeJSONWithPayloadETag(w, r, map[string]interface{}{"rows": rows, "status": status})
	case http.MethodPost:
		if !s.requireRole(w, r, "admin") {
			return
		}
		var payload struct {
			RequestID         string `json:"request_id"`
			Status            string `json:"status"`
			Note              string `json:"note"`
			Voter             string `json:"voter"`
			RequiredApprovals int    `json:"required_approvals"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
			badRequest(w, err)
			return
		}
		voter := payload.Voter
		if voter == "" {
			voter = s.roleFor(r)
		}
		result, err := s.db.CastApprovalVote(payload.RequestID, payload.Status, voter, s.roleFor(r), payload.Note, payload.RequiredApprovals)
		if err != nil {
			badRequest(w, err)
			return
		}
		s.appendAuditLog("local", s.roleFor(r), "policy.approval."+result.Status, payload.RequestID, map[string]string{
			"approval_votes":     fmt.Sprint(result.ApprovalVotes),
			"decided":            fmt.Sprint(result.Decided),
			"note_present":       fmt.Sprint(strings.TrimSpace(payload.Note) != ""),
			"rejection_votes":    fmt.Sprint(result.RejectionVotes),
			"required_approvals": fmt.Sprint(result.RequiredApprovals),
			"voter":              voter,
		})
		writeJSON(w, map[string]interface{}{"ok": true, "result": result})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
