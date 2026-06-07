package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func (s *Server) handleWorkloads(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleWorkloadsList(w, r)
	case http.MethodPost:
		s.handleWorkloadCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleWorkloadsList(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	limit, offset := parseLimitOffset(r)
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		parsed, err := strconv.Atoi(cursor)
		if err != nil || parsed < 0 {
			badRequest(w, fmt.Errorf("invalid cursor %q", cursor))
			return
		}
		offset = parsed
	}
	page, err := s.db.GetWorkloadsPage(from, to,
		r.URL.Query().Get("source"),
		r.URL.Query().Get("model"),
		r.URL.Query().Get("project"),
		r.URL.Query().Get("status"),
		r.URL.Query().Get("q"),
		limit, offset)
	if err != nil {
		serverError(w, err)
		return
	}
	applyWorkloadPagePrivacy(page, s.privacyFor(r))
	writeJSON(w, page)
}

func (s *Server) handleWorkloadCreate(w http.ResponseWriter, r *http.Request) {
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	var payload struct {
		Goal      string  `json:"goal"`
		Source    string  `json:"source"`
		Project   string  `json:"project"`
		Repo      string  `json:"repo"`
		GitBranch string  `json:"git_branch"`
		Owner     string  `json:"owner"`
		Team      string  `json:"team"`
		BudgetUSD float64 `json:"budget_usd"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		badRequest(w, err)
		return
	}
	id, err := s.db.CreateWorkload(payload.Goal, payload.Source, payload.Project, payload.Repo, payload.GitBranch, payload.Owner, payload.Team, payload.BudgetUSD)
	if err != nil {
		badRequest(w, err)
		return
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "workload.create", id, map[string]string{"source": payload.Source, "project": payload.Project})
	writeJSON(w, map[string]interface{}{"ok": true, "workload_id": id})
}

func (s *Server) handleWorkloadClose(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	var payload struct {
		WorkloadID string `json:"workload_id"`
		Status     string `json:"status"`
		Outcome    string `json:"outcome"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		badRequest(w, err)
		return
	}
	if payload.WorkloadID == "" {
		payload.WorkloadID = r.URL.Query().Get("workload_id")
	}
	if err := s.db.CloseWorkload(payload.WorkloadID, payload.Status, payload.Outcome); err != nil {
		badRequest(w, err)
		return
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "workload.close", payload.WorkloadID, map[string]string{"status": payload.Status})
	writeJSON(w, map[string]interface{}{"ok": true, "workload_id": payload.WorkloadID, "status": payload.Status})
}

func (s *Server) handleWorkloadLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	var payload struct {
		SourceWorkloadID string  `json:"source_workload_id"`
		TargetWorkloadID string  `json:"target_workload_id"`
		Relation         string  `json:"relation"`
		Reason           string  `json:"reason"`
		CreatedBy        string  `json:"created_by"`
		Confidence       float64 `json:"confidence"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		badRequest(w, err)
		return
	}
	if payload.SourceWorkloadID == "" {
		payload.SourceWorkloadID = r.URL.Query().Get("source_workload_id")
	}
	if payload.TargetWorkloadID == "" {
		payload.TargetWorkloadID = r.URL.Query().Get("target_workload_id")
	}
	linkID, err := s.db.LinkWorkloads(payload.SourceWorkloadID, payload.TargetWorkloadID, payload.Relation, payload.Reason, payload.CreatedBy, payload.Confidence)
	if err != nil {
		badRequest(w, err)
		return
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "workload.link", linkID, map[string]string{"relation": payload.Relation})
	writeJSON(w, map[string]interface{}{"ok": true, "link_id": linkID, "source_workload_id": payload.SourceWorkloadID, "target_workload_id": payload.TargetWorkloadID})
}

func (s *Server) handleAgentRuns(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	var payload struct {
		WorkloadID string `json:"workload_id"`
		Source     string `json:"source"`
		AgentName  string `json:"agent_name"`
		Command    string `json:"command"`
		CWD        string `json:"cwd"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		badRequest(w, err)
		return
	}
	if payload.WorkloadID == "" {
		payload.WorkloadID = r.URL.Query().Get("workload_id")
	}
	runID, err := s.db.StartAgentRun(payload.WorkloadID, payload.Source, firstNonEmpty(payload.AgentName, payload.Source, "agent"), payload.Command, payload.CWD)
	if err != nil {
		badRequest(w, err)
		return
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "agent_run.start", runID, map[string]string{"source": payload.Source, "agent_name": payload.AgentName, "workload_id": payload.WorkloadID})
	writeJSON(w, map[string]interface{}{"ok": true, "workload_id": payload.WorkloadID, "run_id": runID, "status": "running"})
}

func (s *Server) handleAgentRunHeartbeat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	var payload struct {
		EventID   string                 `json:"event_id"`
		RunID     string                 `json:"run_id"`
		Status    string                 `json:"status"`
		Phase     string                 `json:"phase"`
		Message   string                 `json:"message"`
		Progress  float64                `json:"progress"`
		Metrics   map[string]interface{} `json:"metrics"`
		Timestamp string                 `json:"timestamp"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		badRequest(w, err)
		return
	}
	if payload.RunID == "" {
		payload.RunID = r.URL.Query().Get("run_id")
	}
	var ts time.Time
	var err error
	if payload.Timestamp != "" {
		ts, err = time.Parse(time.RFC3339Nano, payload.Timestamp)
		if err != nil {
			badRequest(w, fmt.Errorf("invalid timestamp: %w", err))
			return
		}
	}
	row, err := s.db.RecordAgentRunHeartbeat(payload.EventID, payload.RunID, payload.Status, payload.Phase, payload.Message, payload.Progress, payload.Metrics, ts, 1)
	if err != nil {
		badRequest(w, err)
		return
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "agent_run.heartbeat", payload.RunID, map[string]string{"status": row.Status, "phase": row.Phase, "workload_id": row.WorkloadID})
	writeJSON(w, map[string]interface{}{"ok": true, "heartbeat": row})
}

func (s *Server) handleAgentRunLiveness(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireRole(w, r, "viewer") {
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
	staleOnly := r.URL.Query().Get("stale_only") == "1" || r.URL.Query().Get("stale_only") == "true"
	rows, err := s.db.GetAgentRunLiveness(maxAge, staleOnly, parseLimit(r, 200), r.URL.Query().Get("source"), r.URL.Query().Get("project"))
	if err != nil {
		serverError(w, err)
		return
	}
	applyRunLivenessPrivacy(rows, s.privacyFor(r))
	writeJSON(w, map[string]interface{}{"rows": rows, "max_age": maxAge.String(), "stale_only": staleOnly})
}

func (s *Server) handleWorkloadDetail(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("workload_id")
	if id == "" {
		badRequest(w, fmt.Errorf("workload_id required"))
		return
	}
	detail, err := s.db.GetWorkloadDetail(id)
	if err != nil {
		badRequest(w, err)
		return
	}
	applyWorkloadDetailPrivacy(detail, s.privacyFor(r))
	writeJSON(w, detail)
}

func (s *Server) handleWorkloadGraph(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("workload_id")
	if id == "" {
		badRequest(w, fmt.Errorf("workload_id required"))
		return
	}
	graph, err := s.db.GetWorkloadGraph(id)
	if err != nil {
		badRequest(w, err)
		return
	}
	writeJSON(w, graph)
}

func (s *Server) handleWorkloadTimeline(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("workload_id")
	if id == "" {
		badRequest(w, fmt.Errorf("workload_id required"))
		return
	}
	rows, err := s.db.GetWorkloadTimeline(id, parseLimit(r, 500))
	if err != nil {
		badRequest(w, err)
		return
	}
	applyWorkloadTimelinePrivacy(rows, s.privacyFor(r))
	writeJSON(w, map[string]interface{}{"workload_id": id, "rows": rows})
}

func (s *Server) handleWorkloadState(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("workload_id")
	if id == "" {
		badRequest(w, fmt.Errorf("workload_id required"))
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
	state, err := s.db.GetWorkloadState(id, maxAge)
	if err != nil {
		badRequest(w, err)
		return
	}
	applyWorkloadStatePrivacy(state, s.privacyFor(r))
	writeJSON(w, state)
}

func (s *Server) handleWorkloadEvents(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "viewer") {
		return
	}
	feed, clientErr, err := s.workloadEventFeedFromRequest(r, 100)
	if err != nil {
		if clientErr {
			badRequest(w, err)
		} else {
			serverError(w, err)
		}
		return
	}
	applyWorkloadEventFeedPrivacy(feed, s.privacyFor(r))
	writeJSON(w, feed)
}

func (s *Server) handleWorkloadEventsStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireRole(w, r, "viewer") {
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	interval := 10 * time.Second
	if raw := r.URL.Query().Get("interval"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			badRequest(w, fmt.Errorf("invalid interval: %w", err))
			return
		}
		if parsed < time.Second || parsed > 5*time.Minute {
			badRequest(w, fmt.Errorf("invalid interval: must be between 1s and 5m"))
			return
		}
		interval = parsed
	}
	feed, clientErr, err := s.workloadEventFeedFromRequest(r, 100)
	if err != nil {
		if clientErr {
			badRequest(w, err)
		} else {
			serverError(w, err)
		}
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	writeWorkloadEventsSSE(w, flusher, feed, s.privacyFor(r))
	if r.URL.Query().Get("once") == "1" || r.URL.Query().Get("once") == "true" {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			feed, _, err := s.workloadEventFeedFromRequest(r, 100)
			if err != nil {
				writeSSE(w, flusher, "agent_ledger_error", map[string]string{"error": err.Error()})
				return
			}
			writeWorkloadEventsSSE(w, flusher, feed, s.privacyFor(r))
		}
	}
}

func (s *Server) workloadEventFeedFromRequest(r *http.Request, fallbackLimit int) (*storage.WorkloadEventFeed, bool, error) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		return nil, true, err
	}
	maxAge := 10 * time.Minute
	if raw := r.URL.Query().Get("max_age"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return nil, true, fmt.Errorf("invalid max_age: %w", err)
		}
		if parsed <= 0 {
			return nil, true, fmt.Errorf("invalid max_age: must be positive")
		}
		maxAge = parsed
	}
	feed, err := s.db.GetWorkloadEventFeed(from, to,
		r.URL.Query().Get("source"),
		r.URL.Query().Get("model"),
		r.URL.Query().Get("project"),
		r.URL.Query().Get("phase"),
		r.URL.Query().Get("severity"),
		parseLimit(r, fallbackLimit),
		maxAge)
	return feed, false, err
}

func writeWorkloadEventsSSE(w http.ResponseWriter, flusher http.Flusher, feed *storage.WorkloadEventFeed, privacy config.PrivacyConfig) {
	applyWorkloadEventFeedPrivacy(feed, privacy)
	writeSSE(w, flusher, "workload_events", feed)
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, event string, payload interface{}) {
	raw, err := json.Marshal(payload)
	if err != nil {
		raw = []byte(`{"error":"failed to encode event"}`)
	}
	fmt.Fprintf(w, "event: %s\n", event)
	fmt.Fprintf(w, "data: %s\n\n", raw)
	flusher.Flush()
}

func (s *Server) handleFleetAttribution(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "viewer") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	report, err := s.db.GetFleetAttribution(from, to,
		r.URL.Query().Get("source"),
		r.URL.Query().Get("model"),
		r.URL.Query().Get("project"),
		parseLimit(r, 100))
	if err != nil {
		serverError(w, err)
		return
	}
	applyFleetPrivacy(report, s.privacyFor(r))
	writeJSON(w, report)
}

func (s *Server) handleModelRegistry(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.GetModelRegistry(s.options.Pricing.StaleAfter, parseLimit(r, 1000))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, rows)
}

func (s *Server) handleCanonicalEventSchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, storage.CanonicalEventSchema())
}

func (s *Server) handleCanonicalEventExamples(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	eventType := r.URL.Query().Get("type")
	if eventType == "" {
		eventType = r.URL.Query().Get("event_type")
	}
	writeJSON(w, map[string]interface{}{
		"contract": "agent-ledger.canonical-event-examples",
		"version":  "v1",
		"examples": storage.CanonicalEventExamples(eventType),
	})
}

func (s *Server) handleCanonicalEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) || !s.requireRole(w, r, "operator") {
		return
	}
	raw := bytes.Buffer{}
	if _, err := raw.ReadFrom(http.MaxBytesReader(w, r.Body, 4<<20)); err != nil {
		badRequest(w, err)
		return
	}
	events, err := decodeCanonicalEventRequest(raw.Bytes())
	if err != nil {
		badRequest(w, err)
		return
	}
	if len(events) == 0 {
		badRequest(w, fmt.Errorf("at least one event is required"))
		return
	}
	if len(events) > 500 {
		badRequest(w, fmt.Errorf("too many events: max 500"))
		return
	}
	results := make([]*storage.CanonicalEventResult, 0, len(events))
	for _, event := range events {
		result, err := s.db.IngestCanonicalEvent(event)
		if err != nil {
			badRequest(w, err)
			return
		}
		results = append(results, result)
	}
	_ = s.db.AppendAuditLog("local", s.roleFor(r), "canonical_event.ingest", fmt.Sprintf("%d", len(results)), map[string]string{"events": fmt.Sprintf("%d", len(results))})
	writeJSON(w, map[string]interface{}{"ok": true, "results": results})
}

func (s *Server) handleCanonicalEventValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) {
		return
	}
	raw := bytes.Buffer{}
	if _, err := raw.ReadFrom(http.MaxBytesReader(w, r.Body, 4<<20)); err != nil {
		badRequest(w, err)
		return
	}
	events, err := decodeCanonicalEventRequest(raw.Bytes())
	if err != nil {
		badRequest(w, err)
		return
	}
	if len(events) == 0 {
		badRequest(w, fmt.Errorf("at least one event is required"))
		return
	}
	if len(events) > 500 {
		badRequest(w, fmt.Errorf("too many events: max 500"))
		return
	}
	results := make([]*storage.CanonicalEventValidation, 0, len(events))
	for _, event := range events {
		result, err := storage.ValidateCanonicalEvent(event)
		if err != nil {
			badRequest(w, err)
			return
		}
		results = append(results, result)
	}
	writeJSON(w, map[string]interface{}{"ok": true, "results": results})
}

func decodeCanonicalEventRequest(raw []byte) ([]storage.CanonicalEvent, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("empty request body")
	}
	if trimmed[0] == '[' {
		var events []storage.CanonicalEvent
		if err := json.Unmarshal(trimmed, &events); err != nil {
			return nil, err
		}
		return events, nil
	}
	var envelope struct {
		Events []storage.CanonicalEvent `json:"events"`
	}
	if err := json.Unmarshal(trimmed, &envelope); err == nil && len(envelope.Events) > 0 {
		return envelope.Events, nil
	}
	var event storage.CanonicalEvent
	if err := json.Unmarshal(trimmed, &event); err != nil {
		return nil, err
	}
	return []storage.CanonicalEvent{event}, nil
}

func (s *Server) handlePolicyDecisions(w http.ResponseWriter, r *http.Request) {
	if !s.requireRole(w, r, "viewer") {
		return
	}
	rows, err := s.db.GetPolicyDecisions(r.URL.Query().Get("workload_id"), parseLimit(r, 200))
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, rows)
}

func applyWorkloadPagePrivacy(page *storage.WorkloadPage, privacy config.PrivacyConfig) {
	if page == nil {
		return
	}
	for i := range page.Rows {
		applyWorkloadPrivacy(&page.Rows[i], privacy)
	}
}

func applyWorkloadDetailPrivacy(detail *storage.WorkloadDetail, privacy config.PrivacyConfig) {
	if detail == nil {
		return
	}
	applyWorkloadPrivacy(&detail.Summary, privacy)
	for i := range detail.Runs {
		if privacy.RedactPaths || privacy.ScreenshotMode {
			detail.Runs[i].CWD = "<redacted>"
			detail.Runs[i].Command = "<redacted>"
			detail.Runs[i].StatusMessage = "<redacted>"
		}
	}
	for i := range detail.RunEvents {
		if privacy.RedactPaths || privacy.ScreenshotMode {
			detail.RunEvents[i].Message = "<redacted>"
			detail.RunEvents[i].Metrics = "{}"
		}
	}
	for i := range detail.ModelCalls {
		if privacy.HashSessionIDs || privacy.ScreenshotMode {
			detail.ModelCalls[i].SessionID = hashValue(detail.ModelCalls[i].SessionID)
		}
	}
	for i := range detail.ContextRefs {
		if privacy.RedactPaths || privacy.HideProjectNames || privacy.ScreenshotMode {
			detail.ContextRefs[i].Label = "<redacted>"
			detail.ContextRefs[i].Repo = "<redacted>"
			detail.ContextRefs[i].GitBranch = "<redacted>"
			detail.ContextRefs[i].CommitSHA = "<redacted>"
		}
	}
	for i := range detail.Links {
		if privacy.HashSessionIDs || privacy.ScreenshotMode {
			detail.Links[i].LinkID = hashValue(detail.Links[i].LinkID)
			detail.Links[i].SourceWorkloadID = hashValue(detail.Links[i].SourceWorkloadID)
			detail.Links[i].TargetWorkloadID = hashValue(detail.Links[i].TargetWorkloadID)
		}
		if privacy.HideProjectNames || privacy.ScreenshotMode {
			detail.Links[i].Reason = "<redacted>"
			detail.Links[i].CreatedBy = "<redacted>"
		}
	}
	for i := range detail.Sessions {
		applySessionPrivacy(&detail.Sessions[i], privacy)
	}
}

func applyWorkloadTimelinePrivacy(rows []storage.WorkloadTimelineRow, privacy config.PrivacyConfig) {
	for i := range rows {
		if privacy.ScreenshotMode {
			rows[i].Label = "<redacted>"
			rows[i].Detail = "<redacted>"
			continue
		}
		if privacy.RedactPaths || privacy.HideProjectNames {
			switch rows[i].Kind {
			case "workload", "context_ref", "artifact", "run_event", "workload_link":
				rows[i].Detail = "<redacted>"
			}
		}
	}
}

func applyWorkloadStatePrivacy(state *storage.WorkloadState, privacy config.PrivacyConfig) {
	if state == nil {
		return
	}
	if privacy.HashSessionIDs || privacy.ScreenshotMode {
		state.WorkloadID = hashValue(state.WorkloadID)
	}
	if privacy.ScreenshotMode {
		state.Goal = "<redacted>"
	}
	if privacy.HideProjectNames || privacy.ScreenshotMode {
		state.Project = "<redacted>"
		state.Repo = "<redacted>"
		state.GitBranch = "<redacted>"
		state.Team = "<redacted>"
	}
}

func applyWorkloadEventFeedPrivacy(feed *storage.WorkloadEventFeed, privacy config.PrivacyConfig) {
	if feed == nil {
		return
	}
	for i := range feed.Rows {
		if privacy.HashSessionIDs || privacy.ScreenshotMode {
			feed.Rows[i].EventID = hashValue(feed.Rows[i].EventID)
			feed.Rows[i].WorkloadID = hashValue(feed.Rows[i].WorkloadID)
		}
		if privacy.ScreenshotMode {
			feed.Rows[i].Goal = "<redacted>"
		}
		if privacy.HideProjectNames || privacy.ScreenshotMode {
			feed.Rows[i].Project = "<redacted>"
			feed.Rows[i].Repo = "<redacted>"
			feed.Rows[i].GitBranch = "<redacted>"
			feed.Rows[i].Team = "<redacted>"
		}
	}
}

func applyWorkloadPrivacy(row *storage.WorkloadSummary, privacy config.PrivacyConfig) {
	if privacy.HideProjectNames || privacy.ScreenshotMode {
		row.Project = "<redacted>"
		row.Repo = "<redacted>"
		row.GitBranch = "<redacted>"
		row.Owner = "<redacted>"
		row.Team = "<redacted>"
	}
}

func applyFleetPrivacy(report *storage.FleetAttributionReport, privacy config.PrivacyConfig) {
	if report == nil {
		return
	}
	for i := range report.Rows {
		if privacy.ScreenshotMode {
			report.Rows[i].Goal = "<redacted>"
		}
		if privacy.HideProjectNames || privacy.ScreenshotMode {
			report.Rows[i].Project = "<redacted>"
			report.Rows[i].Repo = "<redacted>"
			report.Rows[i].GitBranch = "<redacted>"
			report.Rows[i].Team = "<redacted>"
		}
	}
}

func applyRunLivenessPrivacy(rows []storage.AgentRunLivenessRow, privacy config.PrivacyConfig) {
	for i := range rows {
		if privacy.ScreenshotMode {
			rows[i].Goal = "<redacted>"
			rows[i].StatusMessage = "<redacted>"
		}
		if privacy.HideProjectNames || privacy.ScreenshotMode {
			rows[i].Project = "<redacted>"
			rows[i].Repo = "<redacted>"
			rows[i].GitBranch = "<redacted>"
		}
	}
}
