package ui

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCheckCurrentStaticDashboard(t *testing.T) {
	report, err := Check(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !report.OK {
		t.Fatalf("UI contract failed: %+v", report)
	}
	assertCheckpoint(t, report, "charts.accessible")
	assertCheckpoint(t, report, "palette.monochrome")
	assertCheckpoint(t, report, "responsive.breakpoints")
	assertCheckpoint(t, report, "privacy.entrypoints")
	assertCheckpoint(t, report, "integration_readiness.panel")
	if report.ContractHash == "" || report.Checked == 0 {
		t.Fatalf("missing contract hash/check count: %+v", report)
	}
}

func TestCheckRejectsColoredUnlabelledFixture(t *testing.T) {
	root := t.TempDir()
	static := filepath.Join(root, "internal", "server", "static")
	if err := os.MkdirAll(static, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeFile(t, filepath.Join(static, "index.html"), `<!doctype html>
<html><head><meta name="viewport" content="width=device-width, initial-scale=1.0"></head>
<body><a class="skip-link" href="#main-content">Skip</a><main id="main-content">
<button></button>
<button id="btn-privacy">Privacy</button>
<article class="kpi-card accent-cyan"></article>
<div class="chart" id="chart-cost"></div>
</main></body></html>`)
	writeFile(t, filepath.Join(static, "styles.css"), `:root { --accent: #ff0000; }
.brand-mark { background: linear-gradient(90deg, red, blue); }
@media (max-width: 520px) { button { min-height: 44px; } }`)
	writeFile(t, filepath.Join(static, "app.js"), `const state = { privacy: false }; function privacyLabel(value) { return value; }`)

	report, err := Check(root)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if report.OK || report.Failed == 0 {
		t.Fatalf("expected fixture to fail UI contract: %+v", report)
	}
	for _, name := range []string{"buttons.labelled", "charts.accessible", "palette.monochrome", "effects.no_decorative_gradients", "legacy.no_colored_kpi_accents"} {
		if checkpointStatus(report, name) != "fail" {
			t.Fatalf("expected %s to fail: %+v", name, report.Checkpoints)
		}
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("WriteFile %s: %v", path, err)
	}
}

func assertCheckpoint(t *testing.T, report ContractReport, name string) {
	t.Helper()
	if got := checkpointStatus(report, name); got != "pass" {
		t.Fatalf("checkpoint %s status=%q report=%+v", name, got, report)
	}
}

func checkpointStatus(report ContractReport, name string) string {
	for _, check := range report.Checkpoints {
		if check.Name == name {
			return check.Status
		}
	}
	return ""
}
