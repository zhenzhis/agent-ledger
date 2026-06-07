package server

import (
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func (s *Server) handleRepoBadge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireRole(w, r, "viewer") {
		return
	}
	from, to, _, err := s.parseTimeRange(r)
	if err != nil {
		badRequest(w, err)
		return
	}
	project := r.URL.Query().Get("project")
	stats, err := s.db.GetDashboardStatsFiltered(from, to, r.URL.Query().Get("source"), r.URL.Query().Get("model"), project)
	if err != nil {
		serverError(w, err)
		return
	}
	label := firstNonEmpty(r.URL.Query().Get("label"), project, "Agent Ledger")
	privacy := s.privacyFor(r)
	if privacy.HideProjectNames || privacy.ScreenshotMode {
		label = "project"
	}
	metric := strings.ToLower(firstNonEmpty(r.URL.Query().Get("metric"), "cost"))
	value, err := BadgeValue(metric, stats)
	if err != nil {
		badRequest(w, err)
		return
	}
	w.Header().Set("Content-Type", "image/svg+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(RenderBadgeSVG(label, value)))
}

// BadgeValue formats a dashboard metric for a compact SVG badge.
func BadgeValue(metric string, stats *storage.DashboardStats) (string, error) {
	switch metric {
	case "cost", "usd":
		return fmt.Sprintf("$%.2f", stats.TotalCost), nil
	case "tokens":
		return compactInt(stats.TotalTokens), nil
	case "sessions":
		return fmt.Sprintf("%d sessions", stats.TotalSessions), nil
	case "cache":
		return fmt.Sprintf("%.0f%% cache", stats.CacheHitRate*100), nil
	default:
		return "", fmt.Errorf("unsupported badge metric %q", metric)
	}
}

// RenderBadgeSVG renders a local black/white SVG badge. The label and value are
// escaped before interpolation.
func RenderBadgeSVG(label, value string) string {
	label = html.EscapeString(label)
	value = html.EscapeString(value)
	labelWidth := badgeTextWidth(label)
	valueWidth := badgeTextWidth(value)
	totalWidth := labelWidth + valueWidth
	valueX := labelWidth + valueWidth/2
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">
  <title>%s: %s</title>
  <rect width="%d" height="20" fill="#111"/>
  <rect x="%d" width="%d" height="20" fill="#e8e8e8"/>
  <text x="%d" y="14" fill="#fff" font-family="Inter,Segoe UI,Arial,sans-serif" font-size="11" text-anchor="middle">%s</text>
  <text x="%d" y="14" fill="#111" font-family="Inter,Segoe UI,Arial,sans-serif" font-size="11" font-weight="700" text-anchor="middle">%s</text>
</svg>`, totalWidth, label, value, label, value, totalWidth, labelWidth, valueWidth, labelWidth/2, label, valueX, value)
}

func badgeTextWidth(text string) int {
	width := 10 + len([]rune(text))*7
	if width < 46 {
		return 46
	}
	if width > 280 {
		return 280
	}
	return width
}

func compactInt(value int64) string {
	abs := value
	if abs < 0 {
		abs = -abs
	}
	switch {
	case abs >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(value)/1_000_000_000)
	case abs >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(value)/1_000_000)
	case abs >= 1_000:
		return fmt.Sprintf("%.1fK", float64(value)/1_000)
	default:
		return fmt.Sprintf("%d", value)
	}
}
