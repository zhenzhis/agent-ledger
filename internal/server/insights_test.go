package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestWatchdogEventsAPIUsesFiltersAndPrivacy(t *testing.T) {
	db := testServerDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	var records []*storage.UsageRecord
	for i := 0; i < 10; i++ {
		records = append(records, &storage.UsageRecord{
			Source: "codex", SessionID: "private-session", Model: "gpt-5",
			InputTokens: 20000, OutputTokens: 100, CostUSD: 0.5, Timestamp: ts.Add(time.Duration(i) * time.Minute), Project: "private-project",
		})
	}
	records = append(records, &storage.UsageRecord{
		Source: "opencode", SessionID: "other-session", Model: "gpt-5",
		InputTokens: 20000, OutputTokens: 100, CostUSD: 0.5, Timestamp: ts, Project: "other-project",
	})
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	if err := db.InsertPromptBatch([]*storage.PromptEvent{{Source: "codex", SessionID: "private-session", Model: "gpt-5", Project: "private-project", Timestamp: ts}}); err != nil {
		t.Fatalf("InsertPromptBatch: %v", err)
	}
	srv := New(db, "", Options{
		Privacy: config.PrivacyConfig{ScreenshotMode: true},
		Watchdog: config.WatchdogConfig{
			Enabled:              true,
			TokenSpikeMultiplier: 4,
			MinCalls:             8,
			NightStartHour:       22,
			NightEndHour:         6,
		},
	})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/watchdog/events?from=2026-06-07&to=2026-06-08&source=codex&project=private-project&privacy=1&limit=100", nil)
	rr := httptest.NewRecorder()
	srv.handleWatchdogEvents(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("watchdog status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rows []storage.InsightEvent
	if err := json.Unmarshal(rr.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode watchdog rows: %v", err)
	}
	if len(rows) == 0 {
		t.Fatalf("expected watchdog rows")
	}
	body := rr.Body.String()
	if strings.Contains(body, "private-session") || strings.Contains(body, "private-project") || strings.Contains(body, "other-session") || strings.Contains(body, "other-project") {
		t.Fatalf("watchdog privacy/filtering failed: %s", body)
	}
	for _, row := range rows {
		if row.Source != "codex" || row.Project != "<redacted>" || row.SessionID == "" || row.SessionID == "private-session" {
			t.Fatalf("unexpected watchdog row: %+v", row)
		}
	}
}

func TestQuotaStatusAPIUsesFiltersAndWindow(t *testing.T) {
	db := testServerDB(t)
	ts := time.Now().Add(-time.Hour).UTC()
	if err := db.InsertUsageBatch([]*storage.UsageRecord{
		{Source: "codex", SessionID: "codex-session", Model: "gpt-5", InputTokens: 100, OutputTokens: 25, CostUSD: 1, Timestamp: ts, Project: "alpha"},
		{Source: "opencode", SessionID: "other-session", Model: "gpt-5", InputTokens: 900, OutputTokens: 50, CostUSD: 9, Timestamp: ts, Project: "beta"},
	}); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}
	srv := New(db, "", Options{
		Quota: config.QuotaConfig{
			Enabled:       true,
			Plan:          "team",
			MonthlyBudget: 300,
			TokenBudget:   3_000_000,
			PromptBudget:  900,
			ResetDay:      10,
			Window5H:      true,
		},
	})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/quota/status?window=5h&source=codex&project=alpha", nil)
	rr := httptest.NewRecorder()
	srv.handleQuotaStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("quota status=%d body=%s", rr.Code, rr.Body.String())
	}
	var payload struct {
		Enabled bool `json:"enabled"`
		Windows []struct {
			Name        string  `json:"name"`
			CostUSD     float64 `json:"cost_usd"`
			Tokens      int64   `json:"tokens"`
			Calls       int     `json:"calls"`
			PromptLimit int64   `json:"prompt_limit"`
		} `json:"windows"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode quota: %v\n%s", err, rr.Body.String())
	}
	if !payload.Enabled || len(payload.Windows) != 1 {
		t.Fatalf("unexpected quota payload: %+v", payload)
	}
	row := payload.Windows[0]
	if row.Name != "5h" || row.CostUSD != 1 || row.Tokens != 125 || row.Calls != 1 || row.PromptLimit != 6 {
		t.Fatalf("quota filters/window not applied: %+v body=%s", row, rr.Body.String())
	}
}
