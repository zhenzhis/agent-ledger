package notifications

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// WebhookPayload is the redacted summary sent to an explicitly configured webhook.
type WebhookPayload struct {
	Product     string                      `json:"product"`
	Kind        string                      `json:"kind"`
	GeneratedAt string                      `json:"generated_at"`
	Summary     WebhookSummary              `json:"summary"`
	Events      []storage.WorkloadFeedEvent `json:"events"`
	Approvals   []WebhookApproval           `json:"approvals,omitempty"`
	Routes      *WebhookApprovalRoutes      `json:"approval_routes,omitempty"`
}

// WebhookSummary contains aggregate counts without local paths or prompt content.
type WebhookSummary struct {
	Total            int            `json:"total"`
	PendingApprovals int            `json:"pending_approvals"`
	ApprovalRoutes   int            `json:"approval_routes"`
	ByPhase          map[string]int `json:"by_phase"`
	BySeverity       map[string]int `json:"by_severity"`
}

// WebhookApproval is a redacted local approval request summary.
type WebhookApproval struct {
	RequestID              string `json:"request_id"`
	PolicyDecisionID       string `json:"policy_decision_id,omitempty"`
	WorkloadID             string `json:"workload_id,omitempty"`
	RunID                  string `json:"run_id,omitempty"`
	Source                 string `json:"source,omitempty"`
	Model                  string `json:"model,omitempty"`
	Project                string `json:"project"`
	Action                 string `json:"action"`
	Target                 string `json:"target"`
	ActorRole              string `json:"actor_role,omitempty"`
	Status                 string `json:"status"`
	RequiredApprovals      int    `json:"required_approvals"`
	ApprovalVotes          int    `json:"approval_votes"`
	RejectionVotes         int    `json:"rejection_votes"`
	EscalationAfterSeconds int64  `json:"escalation_after_seconds,omitempty"`
	DueAt                  string `json:"due_at,omitempty"`
	Overdue                bool   `json:"overdue"`
	Reason                 string `json:"reason"`
	CreatedAt              string `json:"created_at,omitempty"`
	UpdatedAt              string `json:"updated_at,omitempty"`
}

// WebhookApprovalRoutes is a redacted route rollup for local approval queues.
type WebhookApprovalRoutes struct {
	GeneratedAt string                            `json:"generated_at"`
	DueWithin   string                            `json:"due_within"`
	Summary     storage.ApprovalRouteSummaryStats `json:"summary"`
	Routes      []WebhookApprovalRoute            `json:"routes"`
}

// WebhookApprovalRoute redacts route metadata while keeping dispatch counts useful.
type WebhookApprovalRoute struct {
	RouteKeyHash         string   `json:"route_key_hash"`
	ApproverHash         string   `json:"approver_hash,omitempty"`
	EscalationTargetHash string   `json:"escalation_target_hash,omitempty"`
	Pending              int      `json:"pending"`
	Overdue              int      `json:"overdue"`
	DueSoon              int      `json:"due_soon"`
	ApprovalVotes        int      `json:"approval_votes"`
	RejectionVotes       int      `json:"rejection_votes"`
	MaxRequiredApprovals int      `json:"max_required_approvals"`
	DueNext              string   `json:"due_next,omitempty"`
	Sources              []string `json:"sources,omitempty"`
	Models               []string `json:"models,omitempty"`
	Projects             []string `json:"projects,omitempty"`
	Actions              []string `json:"actions,omitempty"`
}

// DeliveryResult describes one attempted notification delivery.
type DeliveryResult struct {
	Enabled            bool   `json:"enabled"`
	DryRun             bool   `json:"dry_run"`
	EventCount         int    `json:"event_count"`
	ApprovalCount      int    `json:"approval_count"`
	ApprovalRouteCount int    `json:"approval_route_count"`
	StatusCode         int    `json:"status_code,omitempty"`
	Message            string `json:"message"`
}

// BuildWebhookPayload creates a privacy-safe notification payload from a workload event feed.
func BuildWebhookPayload(feed *storage.WorkloadEventFeed, maxEvents int) WebhookPayload {
	return BuildWebhookPayloadWithApprovals(feed, nil, maxEvents)
}

// BuildWebhookPayloadWithApprovals creates a privacy-safe notification payload
// from workload events plus pending approval requests.
func BuildWebhookPayloadWithApprovals(feed *storage.WorkloadEventFeed, approvals []storage.ApprovalRequest, maxEvents int) WebhookPayload {
	return BuildWebhookPayloadWithApprovalRoutes(feed, approvals, nil, maxEvents)
}

// BuildWebhookPayloadWithApprovalRoutes creates a privacy-safe notification
// payload from workload events, pending approval requests, and route rollups.
func BuildWebhookPayloadWithApprovalRoutes(feed *storage.WorkloadEventFeed, approvals []storage.ApprovalRequest, routes *storage.ApprovalRouteSummary, maxEvents int) WebhookPayload {
	redacted := RedactWorkloadEventFeed(feed, maxEvents)
	redactedApprovals := RedactApprovalRequests(approvals, maxEvents)
	redactedRoutes := RedactApprovalRouteSummary(routes, maxEvents)
	routeCount := 0
	if redactedRoutes != nil {
		routeCount = len(redactedRoutes.Routes)
	}
	summary := WebhookSummary{
		Total:            len(redacted) + len(redactedApprovals),
		PendingApprovals: len(redactedApprovals),
		ApprovalRoutes:   routeCount,
		ByPhase:          map[string]int{},
		BySeverity:       map[string]int{},
	}
	for _, event := range redacted {
		summary.ByPhase[event.Phase]++
		summary.BySeverity[event.Severity]++
	}
	return WebhookPayload{
		Product:     "Agent Ledger",
		Kind:        "workload_event_summary",
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Summary:     summary,
		Events:      redacted,
		Approvals:   redactedApprovals,
		Routes:      redactedRoutes,
	}
}

// RedactWorkloadEventFeed returns a bounded copy suitable for external summaries.
func RedactWorkloadEventFeed(feed *storage.WorkloadEventFeed, maxEvents int) []storage.WorkloadFeedEvent {
	if feed == nil || len(feed.Rows) == 0 {
		return []storage.WorkloadFeedEvent{}
	}
	if maxEvents <= 0 || maxEvents > 100 {
		maxEvents = 20
	}
	limit := maxEvents
	if len(feed.Rows) < limit {
		limit = len(feed.Rows)
	}
	out := make([]storage.WorkloadFeedEvent, 0, limit)
	for i := 0; i < limit; i++ {
		event := feed.Rows[i]
		event.EventID = shortHash(event.EventID)
		event.WorkloadID = shortHash(event.WorkloadID)
		event.Goal = "<redacted>"
		event.Project = "<redacted>"
		event.Repo = "<redacted>"
		event.GitBranch = "<redacted>"
		event.Team = "<redacted>"
		out = append(out, event)
	}
	return out
}

// RedactApprovalRequests returns bounded approval summaries suitable for
// external notifications without local paths, targets, reasons, or payloads.
func RedactApprovalRequests(approvals []storage.ApprovalRequest, maxApprovals int) []WebhookApproval {
	if len(approvals) == 0 {
		return []WebhookApproval{}
	}
	if maxApprovals <= 0 || maxApprovals > 100 {
		maxApprovals = 20
	}
	limit := maxApprovals
	if len(approvals) < limit {
		limit = len(approvals)
	}
	out := make([]WebhookApproval, 0, limit)
	for i := 0; i < limit; i++ {
		approval := approvals[i]
		out = append(out, WebhookApproval{
			RequestID:              shortHash(approval.RequestID),
			PolicyDecisionID:       shortHash(approval.PolicyDecisionID),
			WorkloadID:             shortHash(approval.WorkloadID),
			RunID:                  shortHash(approval.RunID),
			Source:                 approval.Source,
			Model:                  approval.Model,
			Project:                "<redacted>",
			Action:                 approval.Action,
			Target:                 "<redacted>",
			ActorRole:              approval.ActorRole,
			Status:                 approval.Status,
			RequiredApprovals:      approval.RequiredApprovals,
			ApprovalVotes:          approval.ApprovalVotes,
			RejectionVotes:         approval.RejectionVotes,
			EscalationAfterSeconds: approval.EscalationAfterSeconds,
			DueAt:                  approval.DueAt,
			Overdue:                approval.Overdue,
			Reason:                 "<redacted>",
			CreatedAt:              approval.CreatedAt,
			UpdatedAt:              approval.UpdatedAt,
		})
	}
	return out
}

// RedactApprovalRouteSummary returns bounded approval route rollups without
// exposing approver names, escalation targets, or project names.
func RedactApprovalRouteSummary(summary *storage.ApprovalRouteSummary, maxRoutes int) *WebhookApprovalRoutes {
	if summary == nil || len(summary.Routes) == 0 {
		return nil
	}
	if maxRoutes <= 0 || maxRoutes > 100 {
		maxRoutes = 20
	}
	limit := maxRoutes
	if len(summary.Routes) < limit {
		limit = len(summary.Routes)
	}
	out := &WebhookApprovalRoutes{
		GeneratedAt: summary.GeneratedAt,
		DueWithin:   summary.DueWithin,
		Summary:     summary.Summary,
		Routes:      make([]WebhookApprovalRoute, 0, limit),
	}
	for i := 0; i < limit; i++ {
		row := summary.Routes[i]
		projects := make([]string, 0, len(row.Projects))
		for range row.Projects {
			projects = append(projects, "<redacted>")
		}
		out.Routes = append(out.Routes, WebhookApprovalRoute{
			RouteKeyHash:         shortHash(row.RouteKey),
			ApproverHash:         shortHash(row.Approver),
			EscalationTargetHash: shortHash(row.EscalationTarget),
			Pending:              row.Pending,
			Overdue:              row.Overdue,
			DueSoon:              row.DueSoon,
			ApprovalVotes:        row.ApprovalVotes,
			RejectionVotes:       row.RejectionVotes,
			MaxRequiredApprovals: row.MaxRequiredApprovals,
			DueNext:              row.DueNext,
			Sources:              append([]string(nil), row.Sources...),
			Models:               append([]string(nil), row.Models...),
			Projects:             projects,
			Actions:              append([]string(nil), row.Actions...),
		})
	}
	return out
}

// SendWebhook sends a redacted notification payload, or returns an explicit disabled/dry-run result.
func SendWebhook(ctx context.Context, cfg config.WebhookConfig, feed *storage.WorkloadEventFeed, dryRun bool) (*DeliveryResult, error) {
	return SendWebhookWithApprovals(ctx, cfg, feed, nil, dryRun)
}

// SendWebhookWithApprovals sends a redacted notification payload that may include
// local pending approval request summaries.
func SendWebhookWithApprovals(ctx context.Context, cfg config.WebhookConfig, feed *storage.WorkloadEventFeed, approvals []storage.ApprovalRequest, dryRun bool) (*DeliveryResult, error) {
	return SendWebhookWithApprovalRoutes(ctx, cfg, feed, approvals, nil, dryRun)
}

// SendWebhookWithApprovalRoutes sends a redacted notification payload that may
// include local pending approval requests plus approval route rollups.
func SendWebhookWithApprovalRoutes(ctx context.Context, cfg config.WebhookConfig, feed *storage.WorkloadEventFeed, approvals []storage.ApprovalRequest, routes *storage.ApprovalRouteSummary, dryRun bool) (*DeliveryResult, error) {
	if cfg.MaxEvents <= 0 || cfg.MaxEvents > 100 {
		cfg.MaxEvents = 20
	}
	payload := BuildWebhookPayloadWithApprovalRoutes(feed, approvals, routes, cfg.MaxEvents)
	result := &DeliveryResult{
		Enabled:            cfg.Enabled,
		DryRun:             dryRun,
		EventCount:         len(payload.Events),
		ApprovalCount:      len(payload.Approvals),
		ApprovalRouteCount: payload.Summary.ApprovalRoutes,
	}
	if dryRun {
		result.Message = "dry run payload built; webhook not sent"
		return result, nil
	}
	if !cfg.Enabled {
		result.Message = "webhook disabled"
		return result, fmt.Errorf("webhook disabled")
	}
	if strings.TrimSpace(cfg.URL) == "" {
		result.Message = "webhook url is required when webhooks are enabled"
		return result, fmt.Errorf("webhook url is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.URL, bytes.NewReader(body))
	if err != nil {
		result.Message = "invalid webhook url"
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "agent-ledger-webhook")
	client := &http.Client{Timeout: cfg.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		result.Message = err.Error()
		return result, err
	}
	defer resp.Body.Close()
	result.StatusCode = resp.StatusCode
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		result.Message = fmt.Sprintf("webhook returned status %d", resp.StatusCode)
		return result, errors.New(result.Message)
	}
	result.Message = "webhook sent"
	return result, nil
}

func shortHash(value string) string {
	if value == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:12]
}
