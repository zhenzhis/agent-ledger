package server

import (
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestBadgeValue(t *testing.T) {
	stats := &storage.DashboardStats{TotalCost: 12.345, TotalTokens: 1_500_000, TotalSessions: 7, CacheHitRate: 0.42}
	cases := map[string]string{
		"cost":     "$12.35",
		"tokens":   "1.5M",
		"sessions": "7 sessions",
		"cache":    "42% cache",
	}
	for metric, want := range cases {
		got, err := BadgeValue(metric, stats)
		if err != nil {
			t.Fatalf("badgeValue(%s): %v", metric, err)
		}
		if got != want {
			t.Fatalf("badgeValue(%s)=%q want %q", metric, got, want)
		}
	}
	if _, err := BadgeValue("bad", stats); err == nil {
		t.Fatalf("expected unsupported metric error")
	}
}

func TestRenderBadgeSVGEscapesText(t *testing.T) {
	svg := RenderBadgeSVG(`repo<&"`, `$1<2`)
	if strings.Contains(svg, `repo<&"`) || strings.Contains(svg, `$1<2`) {
		t.Fatalf("unescaped text leaked into svg: %s", svg)
	}
	if !strings.Contains(svg, "repo&lt;&amp;&#34;") || !strings.Contains(svg, "$1&lt;2") {
		t.Fatalf("escaped text missing from svg: %s", svg)
	}
}
