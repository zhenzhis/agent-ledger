package controlplane

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestReadinessReportReadyAndPrivacySafe(t *testing.T) {
	db := openReadinessDB(t)
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&storage.UsageRecord{
		Source:      "codex",
		SessionID:   "private-session",
		Model:       "gpt-5",
		InputTokens: 10,
		CostUSD:     0.01,
		Timestamp:   now,
		Project:     "private-project",
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	if err := db.InsertPromptBatch([]*storage.PromptEvent{{
		Source: "codex", SessionID: "private-session", Model: "gpt-5", Project: "private-project", Timestamp: now,
	}}); err != nil {
		t.Fatalf("InsertPromptBatch: %v", err)
	}
	if err := db.UpsertIngestionHealth(storage.IngestionHealth{
		Source: "codex", Enabled: true, Paths: []string{"C:/Users/zhang/private/.codex"}, LastScanAt: now.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpsertIngestionHealth: %v", err)
	}
	if err := db.UpsertPricingSource(storage.PricingSourceStatus{
		Name: "openai-official", Kind: "official", Priority: 10, URL: "https://private.example/pricing", LastFetchAt: time.Now().UTC().Format(time.RFC3339), ModelCount: 5, Status: "ok",
	}); err != nil {
		t.Fatalf("UpsertPricingSource: %v", err)
	}
	if _, _, err := db.CreateWorkloadIdempotent("private-readiness-key", "readiness idempotency", "codex", "agent-ledger", "agent-ledger", "main", "", "", 0); err != nil {
		t.Fatalf("CreateWorkloadIdempotent first: %v", err)
	}
	if _, replayed, err := db.CreateWorkloadIdempotent("private-readiness-key", "readiness idempotency", "codex", "agent-ledger", "agent-ledger", "main", "", "", 0); err != nil || !replayed {
		t.Fatalf("CreateWorkloadIdempotent replay replayed=%v err=%v", replayed, err)
	}
	workloadID, err := db.CreateWorkload("private lease readiness", "codex", "agent-ledger", "agent-ledger", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	lease, err := db.AcquireWorkloadLease(workloadID, "private-router", "private readiness purpose", time.Minute)
	if err != nil {
		t.Fatalf("AcquireWorkloadLease: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Storage.Path = "C:/Users/zhang/private/agent-ledger.db"
	cfg.Collectors.Codex.Paths = []string{"C:/Users/zhang/private/.codex"}
	cfg.Pricing.StaleAfter = 24 * time.Hour
	runtime := integrations.EnrichRuntimeStatus(&storage.RuntimeStatus{Mode: "control-plane", ReadOnly: false, WriteOperations: "enabled", BackgroundTasks: "enabled"}, integrations.OptionsFromConfig(cfg))

	report := BuildReadinessReport(db, cfg, runtime, integrations.ContractVerificationReportFor(integrations.OptionsFromConfig(cfg), runtime), now)
	if report.Status != "ready" || !report.AcceptsWrites || report.Summary.CriticalFailures != 0 || report.Summary.Warnings != 0 {
		t.Fatalf("unexpected readiness: %+v", report)
	}
	if report.Summary.IdempotencyKeys != 1 || report.Summary.IdempotencyReplays != 1 {
		t.Fatalf("missing idempotency summary: %+v", report.Summary)
	}
	if report.Summary.ActiveLeases != 1 {
		t.Fatalf("missing lease summary: %+v", report.Summary)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal readiness: %v", err)
	}
	for _, forbidden := range []string{"private-session", "private-project", "C:/Users/zhang/private", "private.example", "private-readiness-key", lease.LeaseToken, "private-router", "private readiness purpose"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("readiness leaked %q: %s", forbidden, raw)
		}
	}
	md := FormatReadinessMarkdown(report)
	if !strings.Contains(md, "Workload leases") || strings.Contains(md, lease.LeaseToken) {
		t.Fatalf("unexpected readiness markdown: %s", md)
	}
}

func TestReadinessReportNotReadyForCriticalConfigIssue(t *testing.T) {
	db := openReadinessDB(t)
	cfg := config.DefaultConfig()
	cfg.Server.BindAddress = "0.0.0.0"
	cfg.Server.AuthToken = ""
	cfg.RBAC.Enabled = false
	runtime := integrations.EnrichRuntimeStatus(&storage.RuntimeStatus{Mode: "control-plane", ReadOnly: false, WriteOperations: "enabled", BackgroundTasks: "enabled"}, integrations.OptionsFromConfig(cfg))

	report := BuildReadinessReport(db, cfg, runtime, integrations.ContractVerificationReportFor(integrations.OptionsFromConfig(cfg), runtime), time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC))
	if report.Status != "not_ready" || report.Summary.CriticalFailures == 0 {
		t.Fatalf("expected not_ready due to public bind without auth: %+v", report)
	}
	if !hasReadinessCheck(report, "config.critical_issues", false, "critical") {
		t.Fatalf("expected critical config readiness check: %+v", report.Checks)
	}
}

func TestReadinessReportDegradedForMissingOperationalEvidence(t *testing.T) {
	db := openReadinessDB(t)
	cfg := config.DefaultConfig()
	runtime := integrations.EnrichRuntimeStatus(&storage.RuntimeStatus{Mode: "control-plane", ReadOnly: false, WriteOperations: "enabled", BackgroundTasks: "enabled"}, integrations.OptionsFromConfig(cfg))

	report := BuildReadinessReport(db, cfg, runtime, integrations.ContractVerificationReportFor(integrations.OptionsFromConfig(cfg), runtime), time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC))
	if report.Status != "degraded" || report.Summary.Warnings == 0 {
		t.Fatalf("expected degraded readiness for missing health/pricing evidence: %+v", report)
	}
	if !hasReadinessCheck(report, "ingestion.health_present", false, "warning") ||
		!hasReadinessCheck(report, "pricing.sources_present", false, "warning") {
		t.Fatalf("expected health and pricing warnings: %+v", report.Checks)
	}
}

func openReadinessDB(t *testing.T) *storage.DB {
	t.Helper()
	db, err := storage.Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func hasReadinessCheck(report *ReadinessReport, name string, ok bool, severity string) bool {
	for _, check := range report.Checks {
		if check.Name == name && check.OK == ok && check.Severity == severity {
			return true
		}
	}
	return false
}
