package server

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/briqt/agent-usage/internal/config"
	"github.com/briqt/agent-usage/internal/storage"
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
	AuthToken string
	Privacy   config.PrivacyConfig
	Budgets   config.BudgetConfig
	Sources   []SourceOption
	Scan      func(source string, reset bool) error
	Recalc    func() error
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

	mux.HandleFunc("/api/stats", s.handleStats)
	mux.HandleFunc("/api/cost-by-model", s.handleCostByModel)
	mux.HandleFunc("/api/cost-over-time", s.handleCostOverTime)
	mux.HandleFunc("/api/tokens-over-time", s.handleTokensOverTime)
	mux.HandleFunc("/api/sessions", s.handleSessions)
	mux.HandleFunc("/api/session-detail", s.handleSessionDetail)
	mux.HandleFunc("/api/health/ingestion", s.handleIngestionHealth)
	mux.HandleFunc("/api/scan", s.handleScan)
	mux.HandleFunc("/api/recalculate-costs", s.handleRecalculateCosts)
	mux.HandleFunc("/api/budgets/status", s.handleBudgetStatus)
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
	if s.options.AuthToken == "" {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/styles.css" || r.URL.Path == "/app.js" || r.URL.Path == "/vendor/echarts/echarts.min.js" {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+s.options.AuthToken {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
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
	json.NewEncoder(w).Encode(v)
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
