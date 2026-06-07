package storage

import (
	"fmt"
	"strings"
	"time"
)

// WorkloadFeedEvent is a metadata-only derived event for monitoring agent workloads.
type WorkloadFeedEvent struct {
	EventID        string   `json:"event_id"`
	EventType      string   `json:"event_type"`
	WorkloadID     string   `json:"workload_id"`
	Goal           string   `json:"goal"`
	Source         string   `json:"source"`
	Project        string   `json:"project"`
	Repo           string   `json:"repo"`
	GitBranch      string   `json:"git_branch"`
	Team           string   `json:"team"`
	Phase          string   `json:"phase"`
	Severity       string   `json:"severity"`
	Message        string   `json:"message"`
	NextAction     string   `json:"next_action"`
	Timestamp      string   `json:"timestamp"`
	Terminal       bool     `json:"terminal"`
	Stale          bool     `json:"stale"`
	ReadinessScore float64  `json:"readiness_score"`
	Progress       float64  `json:"progress"`
	Tokens         int64    `json:"tokens"`
	CostUSD        float64  `json:"cost_usd"`
	Reasons        []string `json:"reasons"`
	Risks          []string `json:"risks"`
}

// WorkloadEventFeed is a bounded, derived feed for local monitors and agent routers.
type WorkloadEventFeed struct {
	Rows              []WorkloadFeedEvent `json:"rows"`
	Total             int                 `json:"total"`
	Limit             int                 `json:"limit"`
	From              string              `json:"from"`
	To                string              `json:"to"`
	StaleAfterSeconds int64               `json:"stale_after_seconds"`
}

// GetWorkloadEventFeed returns recent workload state events without reading prompt content.
func (d *DB) GetWorkloadEventFeed(from, to time.Time, source, model, project, phase, severity string, limit int, staleAfter time.Duration) (*WorkloadEventFeed, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	if staleAfter <= 0 {
		staleAfter = 10 * time.Minute
	}
	states, err := d.GetWorkloadStates(from, to, source, model, project, limit, staleAfter)
	if err != nil {
		return nil, err
	}
	phase = strings.ToLower(strings.TrimSpace(phase))
	severity = strings.ToLower(strings.TrimSpace(severity))
	rows := make([]WorkloadFeedEvent, 0, len(states))
	for _, state := range states {
		eventSeverity := workloadFeedSeverity(state)
		if phase != "" && strings.ToLower(state.Phase) != phase {
			continue
		}
		if severity != "" && eventSeverity != severity {
			continue
		}
		rows = append(rows, workloadFeedEventFromState(state, eventSeverity))
	}
	return &WorkloadEventFeed{
		Rows:              rows,
		Total:             len(rows),
		Limit:             limit,
		From:              from.UTC().Format(time.RFC3339Nano),
		To:                to.UTC().Format(time.RFC3339Nano),
		StaleAfterSeconds: int64(staleAfter / time.Second),
	}, nil
}

func workloadFeedEventFromState(state WorkloadState, severity string) WorkloadFeedEvent {
	timestamp := state.LastActivity
	if timestamp == "" {
		timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}
	return WorkloadFeedEvent{
		EventID:        workloadFeedEventID(state.WorkloadID, state.Phase, timestamp),
		EventType:      "workload.state." + firstNonEmpty(state.Phase, "unknown"),
		WorkloadID:     state.WorkloadID,
		Goal:           state.Goal,
		Source:         state.Source,
		Project:        state.Project,
		Repo:           state.Repo,
		GitBranch:      state.GitBranch,
		Team:           state.Team,
		Phase:          state.Phase,
		Severity:       severity,
		Message:        workloadFeedMessage(state),
		NextAction:     state.NextAction,
		Timestamp:      timestamp,
		Terminal:       state.Terminal,
		Stale:          state.Stale,
		ReadinessScore: state.ReadinessScore,
		Progress:       state.Progress,
		Tokens:         state.Tokens,
		CostUSD:        state.CostUSD,
		Reasons:        state.Reasons,
		Risks:          state.Risks,
	}
}

func workloadFeedSeverity(state WorkloadState) string {
	switch state.Phase {
	case "blocked":
		return "critical"
	case "needs_approval", "stale", "needs_revision", "rejected":
		return "warning"
	case "needs_evaluation":
		return "notice"
	case "accepted":
		return "success"
	}
	if state.EstimatedBudgetExhausted {
		return "critical"
	}
	if state.Stale || state.FailedRuns > 0 || state.NegativeEvaluations > 0 {
		return "warning"
	}
	if state.Terminal {
		return "success"
	}
	return "info"
}

func workloadFeedMessage(state WorkloadState) string {
	switch state.Phase {
	case "blocked":
		return "workload is blocked by local policy"
	case "needs_approval":
		return "workload is waiting for local approval"
	case "stale":
		return "workload has stale active agent runs"
	case "needs_revision":
		return "workload has a failing evaluation signal"
	case "needs_evaluation":
		return "workload has artifacts but no evaluation signal"
	case "accepted":
		return "workload is terminal with a positive evaluation"
	case "rejected":
		return "workload is terminal with a negative evaluation"
	case "terminal":
		return "workload is terminal"
	case "running":
		return "workload has active agent runs"
	case "planned":
		return "workload is planned but has no agent run"
	default:
		if state.Phase != "" {
			return fmt.Sprintf("workload phase is %s", state.Phase)
		}
		return "workload state is available"
	}
}

func workloadFeedEventID(workloadID, phase, timestamp string) string {
	raw := workloadID + ":" + phase + ":" + timestamp
	raw = strings.ReplaceAll(raw, " ", "_")
	raw = strings.ReplaceAll(raw, ":", "_")
	raw = strings.ReplaceAll(raw, ".", "_")
	return raw
}
