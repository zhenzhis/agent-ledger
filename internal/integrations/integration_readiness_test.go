package integrations

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/config"
)

func TestIntegrationReadinessDefaultIsPrivacySafeAndNonFailing(t *testing.T) {
	report := IntegrationReadiness(OptionsFromConfig(config.DefaultConfig()))
	if report.Contract != "agent-ledger.integration-readiness" || !report.ReadOnlySafe || report.WritesLocalState {
		t.Fatalf("unexpected integration readiness identity: %+v", report)
	}
	if report.CatalogHash != CatalogFingerprint(OptionsFromConfig(config.DefaultConfig())) ||
		report.SignalCoverageHash != SignalCoverageFingerprint() {
		t.Fatalf("unexpected readiness hash chain: %+v", report)
	}
	if report.Summary.TotalCapabilities == 0 || report.Summary.Failures != 0 ||
		report.Summary.Experimental != 0 || report.Summary.ReviewRequired == 0 ||
		report.Summary.DisabledByConfig == 0 {
		t.Fatalf("unexpected readiness summary: %+v", report.Summary)
	}
	gateway, ok := integrationReadinessByID(report, "gateway.provider_live_proxy")
	if !ok {
		t.Fatalf("gateway readiness missing")
	}
	if gateway.ActivationState != "disabled-by-config" || gateway.RiskLevel != "high" {
		t.Fatalf("unexpected default gateway readiness: %+v", gateway)
	}
	otlp, ok := integrationReadinessByID(report, "protocol.otlp_receiver")
	if !ok {
		t.Fatalf("OTLP readiness missing")
	}
	if otlp.ActivationState != "disabled-by-config" || !integrationReadinessGateHasStatus(otlp, "otlp.explicit_enablement", "info") {
		t.Fatalf("unexpected default OTLP readiness: %+v", otlp)
	}
	if IntegrationReadinessFingerprint(OptionsFromConfig(config.DefaultConfig())) == "" ||
		!strings.HasPrefix(IntegrationReadinessFingerprint(OptionsFromConfig(config.DefaultConfig())), "sha256:") {
		t.Fatalf("missing readiness fingerprint")
	}
}

func TestIntegrationReadinessFlagsEnabledPreviewSurfaces(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Gateway.Enabled = true
	cfg.Integrations.OTLPReceiver.Enabled = true
	cfg.Integrations.OTLPReceiver.GRPCEnabled = true
	cfg.Webhooks.Enabled = true
	opts := OptionsFromConfig(cfg)
	report := IntegrationReadiness(opts)
	if report.LocalFirst {
		t.Fatalf("gateway/webhook-enabled readiness should not claim local_first: %+v", report.Runtime)
	}
	if report.Runtime.GatewayEnabled != true || report.Runtime.OTLPReceiverGRPCEnabled != true || report.Runtime.WebhooksEnabled != true {
		t.Fatalf("runtime flags missing: %+v", report.Runtime)
	}
	for _, id := range []string{"gateway.provider_live_proxy", "protocol.otlp_receiver", "notification.redacted_webhook"} {
		capability, ok := integrationReadinessByID(report, id)
		if !ok {
			t.Fatalf("%s readiness missing", id)
		}
		if capability.ActivationState != "review-required" {
			t.Fatalf("%s should require review when enabled: %+v", id, capability)
		}
	}
	if report.Summary.Experimental != 0 || report.Summary.ReviewRequired == 0 {
		t.Fatalf("enabled guarded surfaces should require review without experimental status: %+v", report.Summary)
	}
}

func TestIntegrationReadinessBlocksInconsistentOTLPGRPC(t *testing.T) {
	opts := OptionsFromConfig(config.DefaultConfig())
	opts.OTLPReceiverEnabled = false
	opts.OTLPReceiverGRPCEnabled = true
	report := IntegrationReadiness(opts)
	otlp, ok := integrationReadinessByID(report, "protocol.otlp_receiver")
	if !ok {
		t.Fatalf("OTLP readiness missing")
	}
	if otlp.ActivationState != "blocked" || !integrationReadinessGateHasStatus(otlp, "otlp.grpc_requires_receiver", "blocked") ||
		report.Summary.Failures == 0 {
		t.Fatalf("inconsistent OTLP gRPC should be blocked: summary=%+v otlp=%+v", report.Summary, otlp)
	}
}

func TestIntegrationReadinessPrivacySafe(t *testing.T) {
	raw, err := json.Marshal(IntegrationReadiness(OptionsFromConfig(config.DefaultConfig())))
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(raw))
	for _, forbidden := range []string{"sk-proj-", "sk_live_", "sk_test_", "api_key", "bearer ", "c:/users", "\\users\\", "prompt text", "response text", "output text", "transcript text", "session_id", "webhook_url"} {
		if strings.Contains(lower, strings.ToLower(forbidden)) {
			t.Fatalf("integration readiness leaked forbidden marker %q: %s", forbidden, raw)
		}
	}
}

func integrationReadinessByID(report IntegrationReadinessReport, id string) (IntegrationReadinessCapability, bool) {
	for _, capability := range report.Capabilities {
		if capability.ID == id {
			return capability, true
		}
	}
	return IntegrationReadinessCapability{}, false
}

func integrationReadinessGateHasStatus(capability IntegrationReadinessCapability, id, status string) bool {
	for _, gate := range capability.Gates {
		if gate.ID == id && gate.Status == status {
			return true
		}
	}
	return false
}
