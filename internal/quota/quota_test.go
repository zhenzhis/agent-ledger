package quota

import (
	"errors"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestBuildStatusUsesResetDayFiltersAndCustomWindows(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cfg := config.QuotaConfig{
		Enabled:       true,
		Plan:          "team",
		MonthlyBudget: 300,
		TokenBudget:   3_000_000,
		PromptBudget:  900,
		ResetDay:      10,
		Window5H:      true,
		CustomWindows: []config.QuotaWindow{{
			Name:        "release-train",
			Duration:    "48h",
			CostLimit:   50,
			TokenLimit:  500_000,
			PromptLimit: 120,
		}},
	}
	var seenSource, seenModel, seenProject string
	status, err := BuildStatus(now, cfg, Filter{
		Window:  "month",
		Source:  "codex",
		Model:   "gpt-5",
		Project: "alpha",
	}, func(from, to time.Time, source, model, project string) (*storage.DashboardStats, error) {
		seenSource, seenModel, seenProject = source, model, project
		if got := from.Format("2006-01-02"); got != "2026-06-10" {
			t.Fatalf("month starts at reset day: %s", got)
		}
		if got := to.Format("2006-01-02"); got != "2026-07-10" {
			t.Fatalf("month ends at next reset day: %s", got)
		}
		return &storage.DashboardStats{TotalCost: 25, TotalTokens: 250_000, TotalPrompts: 25, TotalCalls: 7}, nil
	})
	if err != nil {
		t.Fatalf("BuildStatus: %v", err)
	}
	if seenSource != "codex" || seenModel != "gpt-5" || seenProject != "alpha" {
		t.Fatalf("filters not forwarded: source=%q model=%q project=%q", seenSource, seenModel, seenProject)
	}
	if !status.Enabled || status.Plan != "team" || status.ResetDay != 10 || status.Method != MethodLocalEstimate {
		t.Fatalf("unexpected status metadata: %+v", status)
	}
	if len(status.Windows) != 1 || status.Windows[0].Name != "month" {
		t.Fatalf("unexpected windows: %+v", status.Windows)
	}
	row := status.Windows[0]
	if row.CostLimit != 300 || row.TokenLimit != 3_000_000 || row.PromptLimit != 900 {
		t.Fatalf("unexpected limits: %+v", row)
	}
	if row.RemainingCost != 275 || row.RemainingTokens != 2_750_000 || row.RemainingPrompts != 875 {
		t.Fatalf("unexpected remaining values: %+v", row)
	}
	if row.Calls != 7 || row.ResetAt == "" || row.TimeToLimitHours <= 0 {
		t.Fatalf("missing operational fields: %+v", row)
	}

	custom, err := WindowSpecs(now, cfg, "release-train")
	if err != nil {
		t.Fatalf("custom WindowSpecs: %v", err)
	}
	if len(custom) != 1 || custom[0].from != now.Add(-48*time.Hour) || custom[0].costLimit != 50 || custom[0].promptLimit != 120 {
		t.Fatalf("unexpected custom window: %+v", custom)
	}
}

func TestBuildStatusRejectsUnknownAndInvalidCustomWindows(t *testing.T) {
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	_, err := BuildStatus(now, config.QuotaConfig{Window5H: true}, Filter{Window: "quarter"}, func(time.Time, time.Time, string, string, string) (*storage.DashboardStats, error) {
		return &storage.DashboardStats{}, nil
	})
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("unknown window error=%v", err)
	}
	_, err = WindowSpecs(now, config.QuotaConfig{CustomWindows: []config.QuotaWindow{{Name: "bad", Duration: "soon"}}}, "")
	if !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("invalid custom window error=%v", err)
	}
}

func TestMonthWindowClampsResetDay(t *testing.T) {
	now := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	from, to := MonthWindow(now, 31)
	if got := from.Format("2006-01-02"); got != "2026-02-28" {
		t.Fatalf("from=%s", got)
	}
	if got := to.Format("2006-01-02"); got != "2026-03-28" {
		t.Fatalf("to=%s", got)
	}
}
