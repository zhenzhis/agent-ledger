package integrations

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/config"
)

func TestGoalCoverageReportHasEvidenceAndNoActionableGaps(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	report := GoalCoverageReportFor(OptionsFromConfig(cfg), nil)
	if report.Contract != "agent-ledger.goal-coverage" || report.Version != "v1" || report.Slug != "agent-ledger" {
		t.Fatalf("unexpected identity: %#v", report)
	}
	if report.PromptContentStored || report.UsageDataUploaded || !report.LocalFirst || !report.ReadOnly {
		t.Fatalf("privacy/local-first flags wrong: %#v", report)
	}
	if report.CoverageHash == "" || !strings.HasPrefix(report.CoverageHash, "sha256:") ||
		report.CapabilityCatalogHash == "" || report.OpenAPIHash == "" || report.ContractBundleHash == "" ||
		report.CanonicalSchemaHash == "" || report.AdapterSpecHash == "" {
		t.Fatalf("missing hashes: %#v", report)
	}
	if GoalCoverageHasGap(report) || report.Summary.Gaps != 0 {
		t.Fatalf("coverage should not have actionable gaps: %#v", report.Summary)
	}
	if report.Summary.TotalSections != len(report.Sections) || report.Summary.TotalSections < 10 {
		t.Fatalf("unexpected section count: summary=%#v sections=%d", report.Summary, len(report.Sections))
	}
	if report.Summary.Experimental == 0 {
		t.Fatalf("expected experimental surfaces to be visible, summary=%#v", report.Summary)
	}
	if len(report.ExternalDependencies) == 0 {
		t.Fatal("expected external dependencies to be disclosed")
	}
	assertGoalSection(t, report, "canonical_event_workload_ledger")
	assertGoalSection(t, report, "ecosystem_adapters_and_gateway")
	assertGoalSection(t, report, "pricing_cost_accuracy")
	assertGoalSection(t, report, "team_finops_audit_policy_notifications")
	assertGoalSection(t, report, "ui_ux_static_dashboard")
}

func TestGoalCoverageDoesNotExposeRawRuntimePaths(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Collectors.Claude.Paths = []string{"C:/Users/example/.claude/projects"}
	cfg.Collectors.Codex.Paths = []string{"C:/Users/example/.codex/sessions"}
	report := GoalCoverageReportFor(OptionsFromConfig(cfg), nil)
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	text := string(raw)
	for _, forbidden := range []string{"C:/Users/example", ".claude/projects", ".codex/sessions", "secret-token", "raw response body"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("coverage leaked forbidden text %q: %s", forbidden, text)
		}
	}
}

func assertGoalSection(t *testing.T, report GoalCoverageReport, id string) {
	t.Helper()
	for _, section := range report.Sections {
		if section.ID != id {
			continue
		}
		if section.Status == "" || section.Status == "gap" {
			t.Fatalf("section %s has bad status: %#v", id, section)
		}
		if section.Objective == "" || section.Privacy == "" {
			t.Fatalf("section %s missing objective/privacy: %#v", id, section)
		}
		if len(section.Evidence.Endpoints)+len(section.Evidence.Commands)+len(section.Evidence.MCPTools)+len(section.Evidence.Tables)+len(section.Evidence.Tests)+len(section.Evidence.Docs) == 0 {
			t.Fatalf("section %s missing evidence: %#v", id, section)
		}
		return
	}
	t.Fatalf("section %s missing from coverage report", id)
}
