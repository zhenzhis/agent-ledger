package server

import (
	"encoding/json"
	"fmt"
	"net/http"
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
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "policy.evaluate", result.Action, map[string]string{
		"source": payload.Source,
		"model":  payload.Model,
		"action": payload.Action,
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
	writeJSON(w, report)
}

func (s *Server) evaluateOperationPolicy(w http.ResponseWriter, r *http.Request, action, source, model, project, target string) bool {
	result := ledgerpolicy.Evaluate(s.options.Policies, ledgerpolicy.Request{
		Source:  source,
		Model:   model,
		Project: project,
		Action:  action,
		Role:    s.roleFor(r),
	})
	if result.Enabled && len(result.Decisions) > 0 {
		raw, _ := json.Marshal(result.Decisions)
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "policy.evaluate", target, map[string]string{
			"action":           action,
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
		allowed, err := s.db.ApprovalAllows(approvalID, action, target)
		if err != nil {
			serverError(w, err)
			return false
		}
		if allowed {
			_ = s.db.AppendAuditLog("local", s.roleFor(r), "policy.approval.used", approvalID, map[string]string{"action": action, "target": target})
			return true
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
		}
		if privacy.ScreenshotMode {
			report.Rows[i].WorkloadID = hashValue(report.Rows[i].WorkloadID)
			report.Rows[i].RunID = hashValue(report.Rows[i].RunID)
			report.Rows[i].Evidence = "<redacted>"
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
	raw, _ := json.Marshal(map[string]interface{}{
		"effective_action": result.Action,
		"decisions":        result.Decisions,
	})
	requestID, err := s.db.CreateApprovalRequest(storage.ApprovalRequest{
		Source:         source,
		Model:          model,
		Project:        project,
		Action:         action,
		Target:         target,
		ActorRole:      s.roleFor(r),
		Status:         "pending",
		Reason:         reason,
		RequestPayload: string(raw),
	})
	if err != nil {
		return "", err
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "policy.approval.requested", requestID, map[string]string{"action": action, "target": target, "source": source, "model": model, "project": project})
	return requestID, nil
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
		writeJSON(w, map[string]interface{}{"rows": rows, "status": status})
	case http.MethodPost:
		if !s.requireRole(w, r, "admin") {
			return
		}
		var payload struct {
			RequestID string `json:"request_id"`
			Status    string `json:"status"`
			Note      string `json:"note"`
		}
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
			badRequest(w, err)
			return
		}
		if err := s.db.ResolveApprovalRequest(payload.RequestID, payload.Status, s.roleFor(r), payload.Note); err != nil {
			badRequest(w, err)
			return
		}
		_ = s.db.AppendAuditLog("local", s.roleFor(r), "policy.approval."+payload.Status, payload.RequestID, map[string]string{"note": payload.Note})
		writeJSON(w, map[string]interface{}{"ok": true, "request_id": payload.RequestID, "status": payload.Status})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
