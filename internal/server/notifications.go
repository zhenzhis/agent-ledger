package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/notifications"
)

func (s *Server) handleWebhookNotification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	maxAge := 10 * time.Minute
	if raw := r.URL.Query().Get("max_age"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			badRequest(w, fmt.Errorf("invalid max_age: %w", err))
			return
		}
		if parsed <= 0 {
			badRequest(w, fmt.Errorf("invalid max_age: must be positive"))
			return
		}
		maxAge = parsed
	}
	limit := parseLimit(r, s.options.Webhooks.MaxEvents)
	feed, err := s.db.GetWorkloadEventFeed(from, to,
		r.URL.Query().Get("source"),
		r.URL.Query().Get("model"),
		r.URL.Query().Get("project"),
		r.URL.Query().Get("phase"),
		r.URL.Query().Get("severity"),
		limit,
		maxAge)
	if err != nil {
		serverError(w, err)
		return
	}
	approvals, err := s.db.ListApprovalRequests("pending", limit)
	if err != nil {
		serverError(w, err)
		return
	}
	dryRun := r.URL.Query().Get("dry_run") == "1" || r.URL.Query().Get("dry_run") == "true"
	result, err := notifications.SendWebhookWithApprovals(r.Context(), s.options.Webhooks, feed, approvals, dryRun)
	if err != nil {
		s.appendAuditLog("local", s.roleFor(r), "notification.webhook.failed", "webhook", map[string]string{"error": err.Error(), "dry_run": fmt.Sprint(dryRun)})
		badRequest(w, err)
		return
	}
	s.appendAuditLog("local", s.roleFor(r), "notification.webhook", "webhook", map[string]string{"dry_run": fmt.Sprint(dryRun), "events": fmt.Sprint(result.EventCount), "approvals": fmt.Sprint(result.ApprovalCount)})
	if dryRun {
		writeJSON(w, map[string]interface{}{
			"result":  result,
			"payload": notifications.BuildWebhookPayloadWithApprovals(feed, approvals, s.options.Webhooks.MaxEvents),
		})
		return
	}
	writeJSON(w, result)
}
