package storage

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOfflineBundleExportImportSigned(t *testing.T) {
	src, err := Open(filepath.Join(t.TempDir(), "src.db"))
	if err != nil {
		t.Fatalf("open src: %v", err)
	}
	defer src.Close()
	ts := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	start, err := src.IngestCanonicalEvent(CanonicalEvent{
		EventID:   "evt-bundle-workload",
		Source:    "codex",
		EventType: "workload.started",
		Timestamp: ts,
		Payload:   rawJSON(t, map[string]interface{}{"goal": "bundle transfer", "project": "agent-ledger", "git_branch": "main"}),
	})
	if err != nil {
		t.Fatalf("ingest workload: %v", err)
	}
	if _, err := src.IngestCanonicalEvent(CanonicalEvent{
		EventID:    "evt-bundle-call",
		Source:     "codex",
		EventType:  "model.call",
		WorkloadID: start.WorkloadID,
		SessionID:  "bundle-session",
		Model:      "gpt-5.5",
		Timestamp:  ts.Add(time.Minute),
		Payload:    rawJSON(t, map[string]interface{}{"input_tokens": 100, "output_tokens": 50, "cost_usd": 0.01}),
	}); err != nil {
		t.Fatalf("ingest call: %v", err)
	}

	bundle, raw, err := src.BuildOfflineBundle(ts.Add(-time.Hour), ts.Add(time.Hour), "", "", "", "metadata-only", "secret", "test-key", 100)
	if err != nil {
		t.Fatalf("build bundle: %v", err)
	}
	if bundle.BundleID == "" || bundle.Integrity.Signature == "" {
		t.Fatalf("missing bundle integrity: %#v", bundle.Integrity)
	}
	if ok, _, err := VerifyOfflineBundle(bundle, "secret", true); err != nil || !ok {
		t.Fatalf("verify signed bundle ok=%v err=%v", ok, err)
	}

	dst, err := Open(filepath.Join(t.TempDir(), "dst.db"))
	if err != nil {
		t.Fatalf("open dst: %v", err)
	}
	defer dst.Close()
	result, err := dst.ImportOfflineBundle(raw, "secret", true)
	if err != nil {
		t.Fatalf("import bundle: %v", err)
	}
	if result.EventsSeen != 2 || result.EventsInserted != 2 || !result.SignatureVerified {
		t.Fatalf("unexpected import result: %#v", result)
	}
	again, err := dst.ImportOfflineBundle(raw, "secret", true)
	if err != nil {
		t.Fatalf("reimport bundle: %v", err)
	}
	if again.EventsDuplicate != 2 {
		t.Fatalf("expected duplicate reimport, got %#v", again)
	}
	detail, err := dst.GetWorkloadDetail(start.WorkloadID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Summary.Goal != "bundle transfer" || len(detail.ModelCalls) != 1 {
		t.Fatalf("detail=%#v", detail)
	}
}

func TestOfflineBundleRejectsTamperingAndCanRedact(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "agent-ledger.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ts := time.Date(2026, 6, 7, 10, 0, 0, 0, time.UTC)
	if _, err := db.IngestCanonicalEvent(CanonicalEvent{
		EventID:   "evt-private-workload",
		Source:    "codex",
		EventType: "workload.started",
		Timestamp: ts,
		Payload:   rawJSON(t, map[string]interface{}{"goal": "secret goal", "project": "secret-project", "repo": "private/repo"}),
	}); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	bundle, raw, err := db.BuildOfflineBundle(ts.Add(-time.Hour), ts.Add(time.Hour), "", "", "", "redacted", "secret", "test-key", 100)
	if err != nil {
		t.Fatalf("build redacted bundle: %v", err)
	}
	if strings.Contains(string(raw), "secret-project") || strings.Contains(string(raw), "private/repo") || strings.Contains(string(raw), "secret goal") {
		t.Fatalf("redacted bundle leaked private metadata: %s", raw)
	}
	var tampered OfflineBundle
	if err := json.Unmarshal(raw, &tampered); err != nil {
		t.Fatalf("decode: %v", err)
	}
	tampered.Product = "Tampered"
	tamperedRaw, _ := json.Marshal(tampered)
	if _, _, err := VerifyOfflineBundle(&tampered, "secret", true); err == nil {
		t.Fatal("expected tampered bundle verification failure")
	}
	if _, err := db.ImportOfflineBundle(tamperedRaw, "secret", true); err == nil {
		t.Fatal("expected tampered import failure")
	}
	if bundle.Privacy != "redacted" {
		t.Fatalf("privacy=%s", bundle.Privacy)
	}
}
