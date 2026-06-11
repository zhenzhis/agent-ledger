package config

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestLoadPolicyApprovalRouting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte(`
policies:
  enabled: true
  rules:
    - name: expensive-model-review
      scope: model
      match: gpt-5.5
      action: require_approval
      message: Review expensive model usage
      required_approvals: 2
      approvers: ["desk-lead", "risk"]
      escalate_after: 30m
      escalate_to: ["research-head"]
`)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if !cfg.Policies.Enabled || len(cfg.Policies.Rules) != 1 {
		t.Fatalf("policy config not loaded: %+v", cfg.Policies)
	}
	rule := cfg.Policies.Rules[0]
	if rule.RequiredApprovals != 2 || rule.EscalateAfter != 30*time.Minute {
		t.Fatalf("approval routing numeric fields not parsed: %+v", rule)
	}
	if len(rule.Approvers) != 2 || rule.Approvers[0] != "desk-lead" || rule.Approvers[1] != "risk" {
		t.Fatalf("approvers not parsed: %+v", rule.Approvers)
	}
	if len(rule.EscalateTo) != 1 || rule.EscalateTo[0] != "research-head" {
		t.Fatalf("escalation targets not parsed: %+v", rule.EscalateTo)
	}
}

func TestExampleConfigLoadsAndStaysLocalFirst(t *testing.T) {
	path := filepath.Join("..", "..", "config.example.yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read example config: %v", err)
	}
	for _, pattern := range []string{`sk-[A-Za-z0-9_-]{12,}`, `xoxb-[A-Za-z0-9_-]{12,}`, `ghp_[A-Za-z0-9_]{12,}`, `BEGIN PRIVATE KEY`} {
		if regexp.MustCompile(pattern).Match(raw) {
			t.Fatalf("example config appears to contain secret pattern %q", pattern)
		}
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load example config: %v", err)
	}
	if cfg.Server.BindAddress != "127.0.0.1" || cfg.Storage.Path == "" {
		t.Fatalf("example config is not local by default: server=%+v storage=%+v", cfg.Server, cfg.Storage)
	}
	if cfg.Webhooks.Enabled || cfg.Webhooks.URL != "" {
		t.Fatalf("example config must not enable outbound webhooks by default: %+v", cfg.Webhooks)
	}
	if cfg.Gateway.Enabled || cfg.Gateway.APIKeyEnv == "" || cfg.Gateway.AnthropicAPIKeyEnv == "" {
		t.Fatalf("example gateway config should be disabled and reference env var names only: %+v", cfg.Gateway)
	}
	if cfg.RBAC.ReadOnly {
		t.Fatalf("example config should document read_only but not force it on local dev: %+v", cfg.RBAC)
	}
	if len(cfg.Collectors.Codex.Paths) == 0 || !strings.Contains(cfg.Collectors.Codex.Paths[0], ".codex") {
		t.Fatalf("example config missing codex collector path: %+v", cfg.Collectors.Codex.Paths)
	}
	if !cfg.Watchdog.Enabled || cfg.Watchdog.MinCalls <= 0 {
		t.Fatalf("example config should keep local watchdog enabled: %+v", cfg.Watchdog)
	}
}
