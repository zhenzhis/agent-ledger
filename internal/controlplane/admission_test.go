package controlplane

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/integrations"
)

func TestAdmissionAllowsReadOnlyValidation(t *testing.T) {
	decision := EvaluateAdmission(AdmissionInput{
		Surface:     "http",
		Method:      "POST",
		Path:        "/api/events/validate",
		Role:        "viewer",
		RBACEnabled: true,
		ReadOnly:    true,
	}, fixedAdmissionTime())
	if !decision.Allowed || decision.WritesLocalState || decision.RequiredRole != "viewer" || !decision.AvailableInReadOnly {
		t.Fatalf("unexpected validation admission: %+v", decision)
	}
}

func TestAdmissionRejectsWriteInReadOnly(t *testing.T) {
	decision := EvaluateAdmission(AdmissionInput{
		Surface:     "http",
		Method:      "POST",
		Path:        "/api/events",
		Role:        "operator",
		RBACEnabled: true,
		ReadOnly:    true,
	}, fixedAdmissionTime())
	if decision.Allowed || decision.Status != "denied" || !strings.Contains(decision.Reason, "read-only") {
		t.Fatalf("expected read-only denial: %+v", decision)
	}
}

func TestAdmissionDetectsConditionalPolicyWrite(t *testing.T) {
	readOnlyDecision := EvaluateAdmission(AdmissionInput{
		Surface:       "mcp",
		Tool:          "ledger.get_policy",
		Role:          "viewer",
		RBACEnabled:   true,
		ReadOnly:      true,
		HasWorkloadID: true,
	}, fixedAdmissionTime())
	if readOnlyDecision.Allowed || readOnlyDecision.AvailableInReadOnly {
		t.Fatalf("policy with workload_id must be rejected in read-only mode: %+v", readOnlyDecision)
	}
	controlDecision := EvaluateAdmission(AdmissionInput{
		Surface:       "mcp",
		Tool:          "ledger.get_policy",
		Role:          "viewer",
		RBACEnabled:   true,
		HasWorkloadID: true,
	}, fixedAdmissionTime())
	if !controlDecision.Allowed || !controlDecision.WritesLocalState || controlDecision.WriteMode != "conditional" {
		t.Fatalf("policy with workload_id should be conditional write in control-plane mode: %+v", controlDecision)
	}
}

func TestAdmissionRoleAndUnknownOperation(t *testing.T) {
	roleDenied := EvaluateAdmission(AdmissionInput{
		Surface:     "http",
		Method:      "POST",
		Path:        "/api/recalculate-costs",
		Role:        "operator",
		RBACEnabled: true,
	}, fixedAdmissionTime())
	if roleDenied.Allowed || !strings.Contains(roleDenied.Reason, "below required role") || roleDenied.RequiredRole != "admin" {
		t.Fatalf("expected role denial: %+v", roleDenied)
	}
	unknown := EvaluateAdmission(AdmissionInput{Surface: "mcp", Tool: "private.tool", Role: "admin"}, fixedAdmissionTime())
	if unknown.Allowed || unknown.KnownOperation {
		t.Fatalf("unknown operation should be denied: %+v", unknown)
	}
}

func TestAdmissionRejectsUnknownHTTPPathsAndMethodMismatches(t *testing.T) {
	cases := []struct {
		name   string
		method string
		path   string
	}{
		{name: "unknown-get", method: "GET", path: "/private/path"},
		{name: "get-post-only", method: "GET", path: "/api/events"},
		{name: "post-get-only", method: "POST", path: "/api/dashboard"},
		{name: "head-get-only", method: "HEAD", path: "/api/dashboard"},
		{name: "options-get-only", method: "OPTIONS", path: "/api/dashboard"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := EvaluateAdmission(AdmissionInput{
				Surface:     "http",
				Method:      tc.method,
				Path:        tc.path,
				Role:        "admin",
				RBACEnabled: true,
				ReadOnly:    true,
			}, fixedAdmissionTime())
			if decision.Allowed || decision.KnownOperation {
				t.Fatalf("expected unknown HTTP operation to be denied: %+v", decision)
			}
		})
	}
}

func TestHTTPAdmissionMatchesOpenAPIContractMethods(t *testing.T) {
	spec := integrations.OpenAPISpecFor(integrations.Options{}, nil)
	paths := spec["paths"].(map[string]interface{})
	for path, rawPathItem := range paths {
		pathItem, ok := rawPathItem.(map[string]interface{})
		if !ok {
			t.Fatalf("OpenAPI path %s has invalid item: %#v", path, rawPathItem)
		}
		for _, method := range []string{"get", "post"} {
			rawOperation, ok := pathItem[method]
			if !ok {
				continue
			}
			operation, ok := rawOperation.(map[string]interface{})
			if !ok {
				t.Fatalf("OpenAPI %s %s has invalid operation: %#v", method, path, rawOperation)
			}
			access := HTTPAccessFor(strings.ToUpper(method), path, AdmissionInput{ReadOnly: true})
			if !access.Known {
				t.Fatalf("admission does not know OpenAPI operation %s %s: %+v", method, path, access)
			}
			meta := openAPIOperationMeta(operation)
			if meta["required_role"] != access.RequiredRole || meta["write_mode"] != access.WriteMode || meta["available_in_read_only"] != access.AvailableInReadOnly {
				t.Fatalf("OpenAPI/admission metadata mismatch for %s %s: openapi=%#v admission=%+v", method, path, meta, access)
			}
			if method == "get" {
				if !access.AvailableInReadOnly || access.WritesLocalState {
					t.Fatalf("GET %s should be read-only in admission: %+v", path, access)
				}
				continue
			}
			expectedReadOnly := openAPIOperationReadOnlySafe(operation)
			if access.AvailableInReadOnly != expectedReadOnly {
				t.Fatalf("OpenAPI/admission read-only mismatch for %s %s: openapi=%t admission=%+v", method, path, expectedReadOnly, access)
			}
		}
	}
}

func TestAdmissionClassifiesReadOnlyCLISubcommands(t *testing.T) {
	for _, command := range []string{"agent-ledger workload feed --limit 10", "agent-ledger workload liveness", "agent-ledger workload queue --source codex", "agent-ledger reconcile status", "agent-ledger provider convert --file provider.json"} {
		decision := EvaluateAdmission(AdmissionInput{Surface: "cli", Command: command, Role: "viewer", RBACEnabled: true, ReadOnly: true}, fixedAdmissionTime())
		if !decision.Allowed || decision.WritesLocalState {
			t.Fatalf("expected read-only CLI command to be allowed: command=%q decision=%+v", command, decision)
		}
	}
	writeDecision := EvaluateAdmission(AdmissionInput{Surface: "cli", Command: "agent-ledger workload heartbeat --run-id run_1", Role: "operator", RBACEnabled: true, ReadOnly: true}, fixedAdmissionTime())
	if writeDecision.Allowed || writeDecision.AvailableInReadOnly {
		t.Fatalf("expected workload heartbeat to be rejected in read-only mode: %+v", writeDecision)
	}
}

func TestAdmissionClassifiesWorkloadLeases(t *testing.T) {
	httpDecision := EvaluateAdmission(AdmissionInput{
		Surface:     "http",
		Method:      "POST",
		Path:        "/api/workloads/lease",
		Role:        "operator",
		RBACEnabled: true,
		ReadOnly:    true,
	}, fixedAdmissionTime())
	if httpDecision.Allowed || httpDecision.WriteMode != "always" || httpDecision.RequiredRole != "operator" {
		t.Fatalf("expected HTTP lease acquire to be rejected in read-only mode: %+v", httpDecision)
	}
	claimHTTPDecision := EvaluateAdmission(AdmissionInput{
		Surface:     "http",
		Method:      "POST",
		Path:        "/api/workloads/claim-next",
		Role:        "operator",
		RBACEnabled: true,
		ReadOnly:    true,
	}, fixedAdmissionTime())
	if claimHTTPDecision.Allowed || claimHTTPDecision.WriteMode != "always" || claimHTTPDecision.RequiredRole != "operator" {
		t.Fatalf("expected HTTP claim-next to be rejected in read-only mode: %+v", claimHTTPDecision)
	}
	mcpDecision := EvaluateAdmission(AdmissionInput{
		Surface:     "mcp",
		Tool:        "ledger.acquire_workload_lease",
		Role:        "operator",
		RBACEnabled: true,
	}, fixedAdmissionTime())
	if !mcpDecision.Allowed || !mcpDecision.WritesLocalState || mcpDecision.AvailableInReadOnly {
		t.Fatalf("expected MCP lease acquire to be a write tool: %+v", mcpDecision)
	}
	claimMCPDecision := EvaluateAdmission(AdmissionInput{
		Surface:     "mcp",
		Tool:        "ledger.claim_next_workload",
		Role:        "operator",
		RBACEnabled: true,
	}, fixedAdmissionTime())
	if !claimMCPDecision.Allowed || !claimMCPDecision.WritesLocalState || claimMCPDecision.AvailableInReadOnly {
		t.Fatalf("expected MCP claim-next to be a write tool: %+v", claimMCPDecision)
	}
	listDecision := EvaluateAdmission(AdmissionInput{
		Surface:     "cli",
		Command:     "agent-ledger workload lease list --limit 10",
		Role:        "viewer",
		RBACEnabled: true,
		ReadOnly:    true,
	}, fixedAdmissionTime())
	if !listDecision.Allowed || listDecision.WritesLocalState {
		t.Fatalf("expected CLI lease list to be read-only: %+v", listDecision)
	}
	renewDecision := EvaluateAdmission(AdmissionInput{
		Surface:     "cli",
		Command:     "agent-ledger workload lease renew --lease-id lease_1",
		Role:        "operator",
		RBACEnabled: true,
		ReadOnly:    true,
	}, fixedAdmissionTime())
	if renewDecision.Allowed || renewDecision.AvailableInReadOnly {
		t.Fatalf("expected CLI lease renew to be rejected in read-only mode: %+v", renewDecision)
	}
	claimCLI := EvaluateAdmission(AdmissionInput{
		Surface:     "cli",
		Command:     "agent-ledger workload claim-next --holder router-a",
		Role:        "operator",
		RBACEnabled: true,
		ReadOnly:    true,
	}, fixedAdmissionTime())
	if claimCLI.Allowed || claimCLI.AvailableInReadOnly {
		t.Fatalf("expected CLI claim-next to be rejected in read-only mode: %+v", claimCLI)
	}
}

func TestAdmissionPrivacyAndFingerprint(t *testing.T) {
	decision := EvaluateAdmission(AdmissionInput{
		Surface: "cli",
		Command: "agent-ledger event validate --file C:/Users/zhang/private/fixture.json --token secret",
		Role:    "admin",
	}, fixedAdmissionTime())
	raw, err := json.Marshal(decision)
	if err != nil {
		t.Fatalf("marshal admission: %v", err)
	}
	for _, forbidden := range []string{"C:/Users/zhang/private", "fixture.json", "--token secret"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("admission leaked %q: %s", forbidden, raw)
		}
	}
	again := decision
	again.GeneratedAt = fixedAdmissionTime().Add(time.Minute).Format(time.RFC3339Nano)
	if AdmissionFingerprint(decision) != AdmissionFingerprint(again) {
		t.Fatalf("fingerprint should ignore generated_at")
	}
	md := FormatAdmissionMarkdown(decision)
	if !strings.Contains(md, "Agent Ledger Admission") || strings.Contains(md, "C:/Users/zhang/private") {
		t.Fatalf("unexpected markdown: %s", md)
	}
}

func openAPIOperationMeta(operation map[string]interface{}) map[string]interface{} {
	meta, ok := operation["x-agent-ledger"].(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	return meta
}

func openAPIOperationReadOnlySafe(operation map[string]interface{}) bool {
	meta, ok := operation["x-agent-ledger"].(map[string]interface{})
	if !ok {
		return false
	}
	switch value := meta["read_only_safe"].(type) {
	case bool:
		return value
	case string:
		lower := strings.ToLower(value)
		return strings.Contains(lower, "true") || strings.Contains(lower, "available")
	default:
		return false
	}
}

func fixedAdmissionTime() time.Time {
	return time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
}
