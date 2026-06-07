package policy

import (
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/config"
)

func TestEvaluateDisabledAllowsWithoutDecisions(t *testing.T) {
	result := Evaluate(config.PolicyConfig{
		Enabled: false,
		Rules: []config.PolicyRule{{
			Name: "disabled-block", Scope: "model", Match: "gpt-5.5", Action: "block",
		}},
	}, Request{Model: "gpt-5.5"})
	if result.Enabled || result.Action != "allow" || len(result.Decisions) != 0 {
		t.Fatalf("unexpected disabled result: %#v", result)
	}
}

func TestEvaluateChoosesHighestSeverity(t *testing.T) {
	result := Evaluate(config.PolicyConfig{
		Enabled: true,
		Rules: []config.PolicyRule{
			{Name: "warn-codex", Scope: "source", Match: "codex", Action: "warn"},
			{Name: "approval-model", Scope: "model", Match: "gpt-5.5", Action: "approval"},
			{Name: "block-project", Scope: "project", Match: "secret", Action: "deny"},
		},
	}, Request{Source: "codex", Model: "gpt-5.5", Project: "secret-alpha"})
	if result.Action != "block" || len(result.Decisions) != 3 {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestEvaluateRoleAndActionScopes(t *testing.T) {
	result := Evaluate(config.PolicyConfig{
		Enabled: true,
		Rules: []config.PolicyRule{
			{Name: "viewer-export", Scope: "role", Match: "viewer", Action: "block"},
			{Name: "scan-warning", Scope: "action", Match: "scan", Action: "warn"},
		},
	}, Request{Role: "viewer", Action: "scan"})
	if result.Action != "block" || len(result.Decisions) != 2 {
		t.Fatalf("unexpected scoped result: %#v", result)
	}
}

func TestUnknownScopeDoesNotMatch(t *testing.T) {
	result := Evaluate(config.PolicyConfig{
		Enabled: true,
		Rules: []config.PolicyRule{{
			Name: "bad-scope", Scope: "unknown", Match: "codex", Action: "block",
		}},
	}, Request{Source: "codex"})
	if result.Action != "allow" || len(result.Decisions) != 0 {
		t.Fatalf("unknown scope matched: %#v", result)
	}
}

func TestAuditReportsHistoricalMatches(t *testing.T) {
	cfg := config.PolicyConfig{Enabled: true, Rules: []config.PolicyRule{
		{Name: "warn-gpt", Scope: "model", Match: "gpt-5.5", Action: "warn", Message: "review model spend"},
		{Name: "block-tool", Scope: "action", Match: "tool.call", Action: "block", Message: "tools require review"},
	}}
	report := Audit(cfg, []AuditCandidate{
		{Kind: "usage_session", Source: "codex", Model: "gpt-5.5", Project: "agent-ledger", Action: "model.call", Tokens: 100, Evidence: "usage_records"},
		{Kind: "tool_call", Source: "codex", Project: "agent-ledger", Action: "tool.call", Evidence: "tool_calls"},
		{Kind: "usage_session", Source: "codex", Model: "gpt-4o-mini", Project: "agent-ledger", Action: "model.call"},
	}, 10)
	if report.Checked != 3 || report.Matches != 2 || report.Warnings != 1 || report.Blocks != 1 {
		t.Fatalf("unexpected audit report: %+v", report)
	}
	if len(report.Rows) != 2 || report.Rows[0].EffectiveAction != "warn" || report.Rows[1].EffectiveAction != "block" {
		t.Fatalf("unexpected rows: %+v", report.Rows)
	}
}
