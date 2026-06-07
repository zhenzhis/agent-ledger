package storage

import (
	"strings"
	"testing"
	"time"
)

func TestGetDoctorReportEmpty(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	report, err := db.GetDoctorReport(ts.Add(-time.Hour), ts.Add(time.Hour), time.Hour, "", "", "")
	if err != nil {
		t.Fatalf("GetDoctorReport: %v", err)
	}
	if report.Summary == "ok" || len(report.Checks) < 2 {
		t.Fatalf("expected empty usage and health checks: %+v", report)
	}
	if !hasDoctorCheck(report.Checks, "usage.empty") || !hasDoctorCheck(report.Checks, "ingestion.missing") {
		t.Fatalf("missing expected checks: %+v", report.Checks)
	}
	md := FormatDoctorMarkdown(report)
	if !strings.Contains(md, "Agent Ledger Doctor") || !strings.Contains(md, "usage.empty") {
		t.Fatalf("unexpected markdown: %s", md)
	}
}

func TestGetDoctorReportPathAndPricingIssues(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.UpsertIngestionHealth(IngestionHealth{
		Source: "codex", Enabled: true, Paths: []string{"C:/missing"},
		PathStatus: []PathStatus{{Path: "C:/missing", Exists: false}},
		LastScanAt: ts.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpsertIngestionHealth: %v", err)
	}
	if err := db.UpsertPricingSource(PricingSourceStatus{
		Name: "openai-official", Kind: "official", Status: "ok",
		LastFetchAt: ts.Add(-48 * time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpsertPricingSource: %v", err)
	}
	report, err := db.GetDoctorReport(ts.Add(-time.Hour), ts.Add(time.Hour), time.Hour, "codex", "", "")
	if err != nil {
		t.Fatalf("GetDoctorReport: %v", err)
	}
	if !hasDoctorCheck(report.Checks, "path.missing") || !hasDoctorCheck(report.Checks, "pricing.stale") {
		t.Fatalf("missing expected checks: %+v", report.Checks)
	}
	if !strings.Contains(report.Summary, "critical") {
		t.Fatalf("expected critical summary, got %q", report.Summary)
	}
}

func TestGetDoctorReportProjectionIssues(t *testing.T) {
	db := tempDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	for _, item := range []struct {
		eventID   string
		sessionID string
		cost      float64
	}{
		{"evt-doctor-missing", "doctor-missing", 1},
		{"evt-doctor-mismatch", "doctor-mismatch", 2},
	} {
		if _, err := db.IngestCanonicalEvent(CanonicalEvent{
			EventID:   item.eventID,
			Source:    "gateway",
			EventType: "model.call",
			SessionID: item.sessionID,
			Model:     "gpt-5",
			Project:   "agent-ledger",
			Timestamp: ts,
			Payload: rawJSON(t, map[string]interface{}{
				"goal":          "doctor projection quality",
				"call_id":       item.eventID + "-call",
				"input_tokens":  10,
				"output_tokens": 5,
				"cost_usd":      item.cost,
			}),
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.db.Exec(`DELETE FROM usage_records WHERE source='gateway' AND session_id='doctor-missing'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.db.Exec(`UPDATE usage_records SET cost_usd=9 WHERE source='gateway' AND session_id='doctor-mismatch'`); err != nil {
		t.Fatal(err)
	}
	report, err := db.GetDoctorReport(ts.Add(-time.Hour), ts.Add(time.Hour), time.Hour, "gateway", "", "agent-ledger")
	if err != nil {
		t.Fatalf("GetDoctorReport: %v", err)
	}
	if !hasDoctorCheck(report.Checks, "projection.missing_usage") || !hasDoctorCheck(report.Checks, "projection.cost_mismatch") {
		t.Fatalf("missing projection checks: %+v", report.Checks)
	}
	md := FormatDoctorMarkdown(report)
	if !strings.Contains(md, "Projection:") || !strings.Contains(md, "cost mismatch") {
		t.Fatalf("projection markdown missing: %s", md)
	}
}

func hasDoctorCheck(checks []DoctorCheck, name string) bool {
	for _, check := range checks {
		if check.Name == name {
			return true
		}
	}
	return false
}
