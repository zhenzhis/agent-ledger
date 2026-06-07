package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestReadOnlyRejectsMutationWithoutRBACEnabled(t *testing.T) {
	db := testServerDB(t)
	called := false
	srv := New(db, "", Options{
		RBAC:        config.RBACConfig{ReadOnly: true},
		PricingSync: func() error { called = true; return nil },
	})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/pricing/sync", nil)
	rr := httptest.NewRecorder()
	srv.handlePricingSync(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if called {
		t.Fatalf("pricing sync callback ran in read-only mode")
	}
}

func TestReadOnlyAnomaliesEndpointDoesNotWriteDerivedEvents(t *testing.T) {
	db := testServerDB(t)
	from := time.Date(2026, 6, 7, 0, 0, 0, 0, time.UTC)
	var records []*storage.UsageRecord
	for i := 0; i < 7; i++ {
		records = append(records, &storage.UsageRecord{
			Source: "codex", SessionID: fmt.Sprintf("baseline-%d", i), Model: "gpt-5",
			InputTokens: 100, Timestamp: from.Add(time.Duration(i) * time.Minute), Project: "agent-ledger",
		})
	}
	records = append(records, &storage.UsageRecord{
		Source: "codex", SessionID: "spike", Model: "gpt-5",
		InputTokens: 10000, Timestamp: from.Add(8 * time.Minute), Project: "agent-ledger",
	})
	if err := db.InsertUsageBatch(records); err != nil {
		t.Fatalf("InsertUsageBatch: %v", err)
	}

	srv := New(db, "", Options{
		RBAC: config.RBACConfig{ReadOnly: true},
		Watchdog: config.WatchdogConfig{
			TokenSpikeMultiplier: 4,
			NightStartHour:       22,
			NightEndHour:         6,
		},
	})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/anomalies?from=2026-06-07&to=2026-06-07&source=codex&limit=100", nil)
	rr := httptest.NewRecorder()
	srv.handleAnomalies(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("read-only anomalies status=%d body=%s", rr.Code, rr.Body.String())
	}
	var rows []storage.InsightEvent
	if err := json.Unmarshal(rr.Body.Bytes(), &rows); err != nil {
		t.Fatalf("decode read-only rows: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("read-only endpoint returned derived rows that should not have been written: %+v", rows)
	}
	stored, err := db.GetInsightEventsFiltered(storage.InsightEventFilter{
		Kind: "anomaly", From: from, To: from.Add(24 * time.Hour), Limit: 100,
	})
	if err != nil {
		t.Fatalf("GetInsightEventsFiltered: %v", err)
	}
	if len(stored) != 0 {
		t.Fatalf("read-only endpoint wrote anomaly events: %+v", stored)
	}

	normal := New(db, "", Options{Watchdog: config.WatchdogConfig{TokenSpikeMultiplier: 4, NightStartHour: 22, NightEndHour: 6}})
	rr = httptest.NewRecorder()
	normal.handleAnomalies(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("normal anomalies status=%d body=%s", rr.Code, rr.Body.String())
	}
	stored, err = db.GetInsightEventsFiltered(storage.InsightEventFilter{
		Kind: "anomaly", From: from, To: from.Add(24 * time.Hour), Limit: 100,
	})
	if err != nil {
		t.Fatalf("GetInsightEventsFiltered after normal call: %v", err)
	}
	if len(stored) == 0 {
		t.Fatalf("test fixture did not produce anomaly events in writable mode")
	}
}

func TestReadOnlyRuntimeVisibleInDashboardAndDoctor(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{RBAC: config.RBACConfig{ReadOnly: true}})

	runtimeReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/runtime/status", nil)
	runtimeRR := httptest.NewRecorder()
	srv.handleRuntimeStatus(runtimeRR, runtimeReq)
	if runtimeRR.Code != http.StatusOK {
		t.Fatalf("runtime status=%d body=%s", runtimeRR.Code, runtimeRR.Body.String())
	}
	var runtimeStatus storage.RuntimeStatus
	if err := json.Unmarshal(runtimeRR.Body.Bytes(), &runtimeStatus); err != nil {
		t.Fatalf("decode runtime: %v", err)
	}
	if !runtimeStatus.ReadOnly || runtimeStatus.Mode != "observer" || runtimeStatus.BackgroundTasks != "disabled" {
		t.Fatalf("unexpected runtime endpoint status: %+v", runtimeStatus)
	}

	dashboardReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/dashboard?from=2026-06-07&to=2026-06-07", nil)
	dashboardRR := httptest.NewRecorder()
	srv.handleDashboard(dashboardRR, dashboardReq)
	if dashboardRR.Code != http.StatusOK {
		t.Fatalf("dashboard status=%d body=%s", dashboardRR.Code, dashboardRR.Body.String())
	}
	var dashboard struct {
		Runtime storage.RuntimeStatus `json:"runtime"`
	}
	if err := json.Unmarshal(dashboardRR.Body.Bytes(), &dashboard); err != nil {
		t.Fatalf("decode dashboard: %v", err)
	}
	if !dashboard.Runtime.ReadOnly || dashboard.Runtime.Mode != "observer" || dashboard.Runtime.WriteOperations != "disabled" {
		t.Fatalf("unexpected dashboard runtime: %+v", dashboard.Runtime)
	}

	doctorReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/doctor?from=2026-06-07&to=2026-06-07", nil)
	doctorRR := httptest.NewRecorder()
	srv.handleDoctor(doctorRR, doctorReq)
	if doctorRR.Code != http.StatusOK {
		t.Fatalf("doctor status=%d body=%s", doctorRR.Code, doctorRR.Body.String())
	}
	var report storage.DoctorReport
	if err := json.Unmarshal(doctorRR.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode doctor: %v", err)
	}
	if report.Runtime == nil || !report.Runtime.ReadOnly {
		t.Fatalf("doctor runtime missing: %+v", report.Runtime)
	}
	found := false
	for _, check := range report.Checks {
		if check.Name == "runtime.read_only" && check.Severity == "info" {
			found = true
		}
	}
	if !found {
		t.Fatalf("runtime.read_only doctor check missing: %+v", report.Checks)
	}
}

func TestReadOnlyAllowsCanonicalEventValidationWithoutWrites(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{RBAC: config.RBACConfig{ReadOnly: true}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/events/validate", strings.NewReader(`{
		"source":"codex",
		"event_type":"workload.started",
		"payload":{"goal":"validate only"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleCanonicalEventValidate(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("validate status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		Results []storage.CanonicalEventValidation `json:"results"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if len(body.Results) != 1 || body.Results[0].Status != "valid_with_warnings" {
		t.Fatalf("unexpected validate response: %+v", body.Results)
	}
	quality, err := db.GetDataQuality(time.Hour)
	if err != nil {
		t.Fatalf("GetDataQuality: %v", err)
	}
	if quality.Provenance == nil || quality.Provenance.Events != 0 {
		t.Fatalf("validate wrote canonical events: %#v", quality.Provenance)
	}
}

func TestReadOnlyAllowsAdapterConformanceWithoutWrites(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{RBAC: config.RBACConfig{ReadOnly: true}})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/integrations/conformance?kind=canonical", strings.NewReader(`{
		"source":"codex",
		"event_type":"workload.started",
		"payload":{"goal":"adapter conformance only"}
	}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.handleAdapterConformance(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("conformance status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body struct {
		OK            bool `json:"ok"`
		DecodedEvents int  `json:"decoded_events"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode conformance response: %v", err)
	}
	if !body.OK || body.DecodedEvents != 1 {
		t.Fatalf("unexpected conformance response: %+v", body)
	}
	quality, err := db.GetDataQuality(time.Hour)
	if err != nil {
		t.Fatalf("GetDataQuality: %v", err)
	}
	if quality.Provenance == nil || quality.Provenance.Events != 0 {
		t.Fatalf("conformance wrote canonical events: %#v", quality.Provenance)
	}
}
