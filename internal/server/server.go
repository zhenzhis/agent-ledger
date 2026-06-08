package server

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

//go:embed static
var staticFS embed.FS

const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 30 * time.Second
	writeTimeout      = 30 * time.Second
	idleTimeout       = 120 * time.Second
	maxHeaderBytes    = 1 << 20
)

// Server serves the web dashboard and REST API.
type Server struct {
	db      *storage.DB
	addr    string
	options Options
}

// SourceOption describes one collector source for health and manual scans.
type SourceOption struct {
	Source  string
	Enabled bool
	Paths   []string
}

// Options provides optional operational capabilities for the HTTP server.
type Options struct {
	AuthToken    string
	AdminToken   string
	ViewerToken  string
	RBAC         config.RBACConfig
	Privacy      config.PrivacyConfig
	Budgets      config.BudgetConfig
	Quota        config.QuotaConfig
	Watchdog     config.WatchdogConfig
	Policies     config.PolicyConfig
	Webhooks     config.WebhookConfig
	Teams        config.TeamsConfig
	Integrations config.IntegrationsConfig
	Gateway      config.GatewayConfig
	Pricing      config.PricingConfig
	Sources      []SourceOption
	Scan         func(source string, reset bool) error
	Recalc       func() error
	RecalcMode   func(mode string) error
	PricingSync  func() error
}

// New creates a Server that will listen on the given address (host:port).
func New(db *storage.DB, addr string, options Options) *Server {
	return &Server{db: db, addr: addr, options: options}
}

// Start registers HTTP handlers and begins listening. It blocks until the server stops.
func (s *Server) Start() error {
	mux := http.NewServeMux()

	sub, _ := fs.Sub(staticFS, "static")
	mux.Handle("/", http.FileServer(http.FS(sub)))

	mux.HandleFunc("/.well-known/agent-ledger.json", s.handleDiscovery)
	mux.HandleFunc("/api/discovery", s.handleDiscovery)
	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/dashboard", s.handleDashboard)
	mux.HandleFunc("/api/cost-by-model", s.handleCostByModel)
	mux.HandleFunc("/api/cost-over-time", s.handleCostOverTime)
	mux.HandleFunc("/api/tokens-over-time", s.handleTokensOverTime)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/session-detail", s.handleSessionDetail)
	mux.HandleFunc("/api/session-replay", s.handleSessionReplay)
	mux.HandleFunc("/api/workloads", s.handleWorkloads)
	mux.HandleFunc("/api/workloads/close", s.handleWorkloadClose)
	mux.HandleFunc("/api/workloads/link", s.handleWorkloadLink)
	mux.HandleFunc("/api/agent-runs", s.handleAgentRuns)
	mux.HandleFunc("/api/agent-runs/heartbeat", s.handleAgentRunHeartbeat)
	mux.HandleFunc("/api/agent-runs/liveness", s.handleAgentRunLiveness)
	mux.HandleFunc("/api/workload-detail", s.handleWorkloadDetail)
	mux.HandleFunc("/api/workload-graph", s.handleWorkloadGraph)
	mux.HandleFunc("/api/workload-timeline", s.handleWorkloadTimeline)
	mux.HandleFunc("/api/workload-state", s.handleWorkloadState)
	mux.HandleFunc("/api/workload-events", s.handleWorkloadEvents)
	mux.HandleFunc("/api/workload-events/stream", s.handleWorkloadEventsStream)
	mux.HandleFunc("/api/fleet-attribution", s.handleFleetAttribution)
	mux.HandleFunc("/api/integrations", s.handleIntegrations)
	mux.HandleFunc("/api/integrations/adapter-spec", s.handleAdapterSpec)
	mux.HandleFunc("/api/integrations/conformance", s.handleAdapterConformance)
	mux.HandleFunc("/api/runtime/status", s.handleRuntimeStatus)
	mux.HandleFunc("/api/event-schema", s.handleCanonicalEventSchema)
	mux.HandleFunc("/api/event-examples", s.handleCanonicalEventExamples)
	mux.HandleFunc("/api/events/validate", s.handleCanonicalEventValidate)
	mux.HandleFunc("/api/events", s.handleCanonicalEvents)
	mux.HandleFunc("/api/otel/genai", s.handleOTelGenAI)
	mux.HandleFunc("/api/otlp/v1/traces", s.handleOTLPTraces)
	mux.HandleFunc("/v1/traces", s.handleOTLPTraces)
	mux.HandleFunc("/api/a2a/tasks", s.handleA2ATasks)
	mux.HandleFunc("/api/provider/calls", s.handleProviderCalls)
	mux.HandleFunc("/gateway/openai/v1/chat/completions", s.handleOpenAIChatGateway)
	mux.HandleFunc("/gateway/openai/v1/responses", s.handleOpenAIResponsesGateway)
	mux.HandleFunc("/gateway/anthropic/v1/messages", s.handleAnthropicMessagesGateway)
	mux.HandleFunc("/api/health/ingestion", s.handleIngestionHealth)
	mux.HandleFunc("/api/scan", s.handleScan)
	mux.HandleFunc("/api/recalculate-costs", s.handleRecalculateCosts)
	mux.HandleFunc("/api/projections/repair", s.handleRepairProjections)
	mux.HandleFunc("/api/pricing/status", s.handlePricingStatus)
	mux.HandleFunc("/api/pricing/sync", s.handlePricingSync)
	mux.HandleFunc("/api/pricing/recalculate", s.handlePricingRecalculate)
	mux.HandleFunc("/api/pricing/audit", s.handlePricingAudit)
	mux.HandleFunc("/api/budgets/status", s.handleBudgetStatus)
	mux.HandleFunc("/api/quota/status", s.handleQuotaStatus)
	mux.HandleFunc("/api/data-quality", s.handleDataQuality)
	mux.HandleFunc("/api/doctor", s.handleDoctor)
	mux.HandleFunc("/api/model-calls", s.handleModelCalls)
	mux.HandleFunc("/api/model-registry", s.handleModelRegistry)
	mux.HandleFunc("/api/cost-intelligence", s.handleCostIntelligence)
	mux.HandleFunc("/api/cache/doctor", s.handleCacheDoctor)
	mux.HandleFunc("/api/anomalies", s.handleAnomalies)
	mux.HandleFunc("/api/watchdog/events", s.handleWatchdogEvents)
	mux.HandleFunc("/api/notifications/webhook", s.handleWebhookNotification)
	mux.HandleFunc("/api/audit-log", s.handleAuditLog)
	mux.HandleFunc("/api/reconciliation/status", s.handleReconciliationStatus)
	mux.HandleFunc("/api/reconciliation/import", s.handleReconciliationImport)
	mux.HandleFunc("/api/router/simulate", s.handleRouterSimulation)
	mux.HandleFunc("/api/preflight/estimate", s.handlePreflightEstimate)
	mux.HandleFunc("/api/chargeback", s.handleChargeback)
	mux.HandleFunc("/api/wrapped", s.handleWrapped)
	mux.HandleFunc("/api/badge/repo.svg", s.handleRepoBadge)
	mux.HandleFunc("/api/evidence-bundle", s.handleEvidenceBundle)
	mux.HandleFunc("/api/offline-bundle/export", s.handleOfflineBundleExport)
	mux.HandleFunc("/api/offline-bundle/import", s.handleOfflineBundleImport)
	mux.HandleFunc("/api/policies/status", s.handlePolicyStatus)
	mux.HandleFunc("/api/policy/evaluate", s.handlePolicyEvaluate)
	mux.HandleFunc("/api/policy/audit", s.handlePolicyAudit)
	mux.HandleFunc("/api/policy/enforcement", s.handlePolicyEnforcement)
	mux.HandleFunc("/api/policy/decisions", s.handlePolicyDecisions)
	mux.HandleFunc("/api/policy/approvals", s.handlePolicyApprovals)
	mux.HandleFunc("/api/policy/approval-routes", s.handlePolicyApprovalRoutes)
	mux.HandleFunc("/api/export", s.handleExport)
	mux.HandleFunc("/api/report", s.handleReport)

	log.Printf("server: listening on %s", s.addr)
	srv := &http.Server{
		Addr:              s.addr,
		Handler:           securityHeaders(s.auth(mux)),
		ReadHeaderTimeout: readHeaderTimeout,
		ReadTimeout:       readTimeout,
		WriteTimeout:      writeTimeout,
		IdleTimeout:       idleTimeout,
		MaxHeaderBytes:    maxHeaderBytes,
	}
	return srv.ListenAndServe()
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) parseTimeRange(r *http.Request) (time.Time, time.Time, int, error) {
	from := r.URL.Query().Get("from")
	to := r.URL.Query().Get("to")

	// Parse tz_offset (minutes, JS getTimezoneOffset convention: UTC+8 = -480)
	tzOffset := 0
	if tzStr := r.URL.Query().Get("tz_offset"); tzStr != "" {
		parsed, err := strconv.Atoi(tzStr)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid 'tz_offset' %q: expected minutes", tzStr)
		}
		if parsed < -14*60 || parsed > 14*60 {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid 'tz_offset' %q: outside supported timezone range", tzStr)
		}
		tzOffset = parsed
	}

	var fromTime, toTime time.Time
	var err error
	if from != "" {
		fromTime, err = time.Parse("2006-01-02", from)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid 'from' date %q: expected YYYY-MM-DD", from)
		}
	}
	if to != "" {
		toTime, err = time.Parse("2006-01-02", to)
		if err != nil {
			return time.Time{}, time.Time{}, 0, fmt.Errorf("invalid 'to' date %q: expected YYYY-MM-DD", to)
		}
		toTime = toTime.Add(24 * time.Hour)
	}
	if fromTime.IsZero() {
		fromTime = time.Now().AddDate(0, -1, 0)
	}
	if toTime.IsZero() {
		toTime = time.Now().Add(24 * time.Hour)
	}

	// Apply timezone offset: convert local day boundaries to UTC
	if tzOffset != 0 {
		offset := time.Duration(tzOffset) * time.Minute
		fromTime = fromTime.Add(offset)
		toTime = toTime.Add(offset)
	}

	if !fromTime.Before(toTime) {
		return time.Time{}, time.Time{}, 0, fmt.Errorf("'from' date (%s) is after 'to' date (%s): swap them or correct the range", from, to)
	}
	return fromTime, toTime, tzOffset, nil
}

func (s *Server) auth(next http.Handler) http.Handler {
	if s.options.AuthToken == "" && s.options.AdminToken == "" && s.options.ViewerToken == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/styles.css" || r.URL.Path == "/app.js" || r.URL.Path == "/vendor/echarts/echarts.min.js" {
			next.ServeHTTP(w, r)
			return
		}
		role := s.roleFor(r)
		if role == "" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) roleFor(r *http.Request) string {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		if s.options.AuthToken == "" && s.options.AdminToken == "" && s.options.ViewerToken == "" {
			if s.options.RBAC.Enabled {
				return ""
			}
			return "admin"
		}
		return ""
	}
	if s.options.AdminToken != "" && token == s.options.AdminToken {
		return "admin"
	}
	if s.options.AuthToken != "" && token == s.options.AuthToken {
		return "operator"
	}
	if s.options.ViewerToken != "" && token == s.options.ViewerToken {
		return "viewer"
	}
	if !s.options.RBAC.Enabled && s.options.AuthToken != "" && token == s.options.AuthToken {
		return "admin"
	}
	return ""
}

func (s *Server) requireRole(w http.ResponseWriter, r *http.Request, minRole string) bool {
	if s.options.RBAC.ReadOnly && !isSafeMethod(r.Method) && !isReadOnlyAllowedRequest(r) {
		http.Error(w, "read-only mode: write operations are disabled", http.StatusForbidden)
		return false
	}
	if !s.options.RBAC.Enabled {
		return true
	}
	role := s.roleFor(r)
	rank := map[string]int{"viewer": 1, "operator": 2, "admin": 3}
	if rank[role] < rank[minRole] {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) canWriteDerivedData() bool {
	return !s.options.RBAC.ReadOnly
}

func (s *Server) appendAuditLog(actor, role, action, target string, params map[string]string) {
	if !s.canWriteDerivedData() {
		return
	}
	_ = s.db.AppendAuditLog(actor, role, action, target, params)
}

func (s *Server) runtimeStatus() *storage.RuntimeStatus {
	return integrations.EnrichRuntimeStatus(RuntimeStatusFromRBAC(s.options.RBAC), s.integrationOptions())
}

// RuntimeStatusFromRBAC returns the process-level runtime mode before
// config-derived compatibility hashes are attached.
func RuntimeStatusFromRBAC(rbac config.RBACConfig) *storage.RuntimeStatus {
	if rbac.ReadOnly {
		return &storage.RuntimeStatus{
			Mode:             "observer",
			ReadOnly:         true,
			WriteOperations:  "disabled",
			BackgroundTasks:  "disabled",
			DisabledFeatures: []string{"background collectors", "pricing sync", "cost recalculation", "manual scans", "imports", "write APIs", "write MCP tools", "derived GET writebacks"},
			Message:          "read-only observer mode: local state is not mutated by this process",
		}
	}
	return &storage.RuntimeStatus{
		Mode:            "control-plane",
		ReadOnly:        false,
		WriteOperations: "enabled",
		BackgroundTasks: "enabled",
		Message:         "write operations and background collectors are enabled",
	}
}

// RuntimeStatusFromConfig returns runtime status with compatibility hashes
// derived from the same config used by discovery and integration catalog.
func RuntimeStatusFromConfig(cfg *config.Config) *storage.RuntimeStatus {
	if cfg == nil {
		return integrations.EnrichRuntimeStatus(RuntimeStatusFromRBAC(config.RBACConfig{}), integrations.Options{})
	}
	return integrations.EnrichRuntimeStatus(RuntimeStatusFromRBAC(cfg.RBAC), integrations.OptionsFromConfig(cfg))
}

func isSafeMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func isReadOnlyAllowedRequest(r *http.Request) bool {
	if r.Method != http.MethodPost {
		return false
	}
	switch r.URL.Path {
	case "/api/events/validate", "/api/integrations/conformance", "/api/policy/evaluate":
		return true
	case "/api/notifications/webhook":
		return r.URL.Query().Get("dry_run") == "1" || r.URL.Query().Get("dry_run") == "true"
	default:
		return false
	}
}

func (s *Server) requireLocalOrAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.options.AuthToken != "" {
		return true
	}
	hostHeader, _, _ := net.SplitHostPort(r.Host)
	if hostHeader == "" {
		hostHeader = r.Host
	}
	if hostHeader == "localhost" {
		return true
	}
	if ip := net.ParseIP(hostHeader); ip != nil && ip.IsLoopback() {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		http.Error(w, "manual operations require localhost or auth_token", http.StatusForbidden)
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if v != nil {
		rv := reflect.ValueOf(v)
		if rv.Kind() == reflect.Slice && rv.IsNil() {
			w.Write([]byte("[]\n"))
			return
		}
	}
	json.NewEncoder(w).Encode(v)
}

func writeJSONWithETag(w http.ResponseWriter, r *http.Request, v interface{}, etag string) {
	quotedETag := quoteHTTPETag(etag)
	if quotedETag != "" {
		w.Header().Set("ETag", quotedETag)
		w.Header().Set("Cache-Control", "no-cache")
		if requestETagMatches(r.Header.Get("If-None-Match"), quotedETag) {
			w.WriteHeader(http.StatusNotModified)
			return
		}
	}
	writeJSON(w, v)
}

func jsonPayloadETag(v interface{}) (string, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func quoteHTTPETag(etag string) string {
	etag = strings.TrimSpace(etag)
	if etag == "" {
		return ""
	}
	etag = strings.TrimPrefix(etag, "W/")
	etag = strings.Trim(etag, `"`)
	etag = strings.ReplaceAll(etag, `"`, "")
	if etag == "" {
		return ""
	}
	return `"` + etag + `"`
}

func requestETagMatches(raw, etag string) bool {
	if raw == "" || etag == "" {
		return false
	}
	expected := strings.Trim(quoteHTTPETag(etag), `"`)
	if expected == "" {
		return false
	}
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "*" {
			return true
		}
		part = strings.TrimPrefix(part, "W/")
		part = strings.Trim(part, `"`)
		if part == expected {
			return true
		}
	}
	return false
}

func serverError(w http.ResponseWriter, err error) {
	log.Printf("api error: %v", err)
	http.Error(w, "internal server error", 500)
}

func badRequest(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(400)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	stats, err := s.db.GetDashboardStatsFiltered(from, to, source, model, project)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	from, to, tzOffset, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	granularity := r.URL.Query().Get("granularity")
	data, err := s.db.GetDashboardBundleFiltered(from, to, granularity, source, model, project, tzOffset)
	if err != nil {
		serverError(w, err)
		return
	}
	data.Runtime = s.runtimeStatus()
	writeJSON(w, data)
}

func (s *Server) handleCostByModel(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	project := r.URL.Query().Get("project")
	data, err := s.db.GetCostByModelFiltered(from, to, source, project)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleCostOverTime(w http.ResponseWriter, r *http.Request) {
	from, to, tzOffset, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	granularity := r.URL.Query().Get("granularity")
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	data, err := s.db.GetCostOverTimeFiltered(from, to, granularity, source, model, project, tzOffset)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleTokensOverTime(w http.ResponseWriter, r *http.Request) {
	from, to, tzOffset, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	granularity := r.URL.Query().Get("granularity")
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	data, err := s.db.GetTokensOverTimeFiltered(from, to, granularity, source, model, project, tzOffset)
	if err != nil {
		serverError(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	source := r.URL.Query().Get("source")
	model := r.URL.Query().Get("model")
	project := r.URL.Query().Get("project")
	query := r.URL.Query().Get("q")
	sortKey := r.URL.Query().Get("sort")
	direction := r.URL.Query().Get("dir")
	limit, offset := parseLimitOffset(r)
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		parsed, err := strconv.Atoi(cursor)
		if err != nil || parsed < 0 {
			badRequest(w, fmt.Errorf("invalid cursor %q", cursor))
			return
		}
		offset = parsed
	}
	data, err := s.db.GetSessionsPageSorted(from, to, source, model, project, query, limit, offset, sortKey, direction)
	if err != nil {
		serverError(w, err)
		return
	}
	applySessionPagePrivacy(data, s.privacyFor(r))
	writeJSON(w, data)
}

func (s *Server) handleSessionDetail(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session_id")
	if sid == "" {
		http.Error(w, "session_id required", 400)
		return
	}
	source := r.URL.Query().Get("source")
	data, err := s.db.GetSessionDetailScoped(source, sid)
	if err != nil {
		badRequest(w, err)
		return
	}
	writeJSON(w, data)
}

func (s *Server) handleSessionReplay(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session_id")
	if sid == "" {
		http.Error(w, "session_id required", 400)
		return
	}
	source := r.URL.Query().Get("source")
	data, err := s.db.GetSessionReplay(source, sid, parseLimit(r, 1000))
	if err != nil {
		badRequest(w, err)
		return
	}
	privacy := s.privacyFor(r)
	if privacy.HashSessionIDs || privacy.ScreenshotMode {
		data.SessionID = hashValue(data.SessionID)
		for i := range data.Points {
			data.Points[i].SessionID = hashValue(data.Points[i].SessionID)
		}
	}
	writeJSON(w, data)
}

func parseLimitOffset(r *http.Request) (int, int) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
