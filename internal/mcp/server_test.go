package mcp

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/controlplane"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

func TestMCPSurfaceMatchesCapabilityCatalog(t *testing.T) {
	catalog := integrations.Registry(integrations.OptionsFromConfig(config.DefaultConfig()))
	var mcpCapability integrations.Capability
	found := false
	for _, capability := range catalog.Capabilities {
		if capability.ID == "protocol.mcp_stdio" {
			mcpCapability = capability
			found = true
			break
		}
	}
	if !found {
		t.Fatal("protocol.mcp_stdio capability missing")
	}
	compareStringSet(t, "mcp tools", mcpCapability.Tools, ToolNames())
	compareStringSet(t, "mcp resources", mcpCapability.Resources, ResourceURIs())
	compareStringSet(t, "mcp prompts", mcpCapability.Prompts, PromptNames())
}

func TestMCPToolMetadataMatchesAdmission(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range tools() {
		name, ok := tool["name"].(string)
		if !ok || name == "" {
			t.Fatalf("tool missing name: %#v", tool)
		}
		if seen[name] {
			t.Fatalf("duplicate MCP tool name %q", name)
		}
		seen[name] = true
		meta := agentLedgerToolMeta(t, tool)
		access := controlplane.MCPToolAccessFor(name)
		if meta["writes_local_state"] != access.WritesLocalState ||
			meta["write_mode"] != access.WriteMode ||
			meta["available_in_read_only"] != access.AvailableInReadOnly ||
			meta["read_only_behavior"] != access.ReadOnlyBehavior {
			t.Fatalf("tool %s metadata drifted from admission: meta=%#v access=%#v", name, meta, access)
		}
		annotations, ok := tool["annotations"].(map[string]interface{})
		if !ok {
			t.Fatalf("tool %s missing annotations: %#v", name, tool)
		}
		readOnly := access.WriteMode == "none"
		if annotations["readOnlyHint"] != readOnly ||
			annotations["idempotentHint"] != readOnly ||
			annotations["destructiveHint"] != false ||
			annotations["openWorldHint"] != false {
			t.Fatalf("tool %s annotations drifted from admission: annotations=%#v access=%#v", name, annotations, access)
		}
	}
}

func TestMCPToolsListAndBudget(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Quota.Enabled = true
	cfg.Quota.Plan = "custom"
	cfg.Quota.MonthlyBudget = 300
	cfg.Quota.TokenBudget = 3_000_000
	srv := New(db, cfg)
	srv.now = func() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ledger.current_budget","arguments":{"window":"day"}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ledger.discovery","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ledger.contracts","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"ledger.contracts_verify","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"ledger.openapi","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"ledger.config_status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"ledger.readiness","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"ledger.admission_check","arguments":{"surface":"mcp","tool":"ledger.start_workload","role":"operator"}}}`,
	)
	if len(out) != 9 {
		t.Fatalf("responses=%d want 9", len(out))
	}
	tools := out[0]["result"].(map[string]interface{})["tools"].([]interface{})
	if !hasTool(tools, "ledger.start_workload") || !hasTool(tools, "ledger.start_run") || !hasTool(tools, "ledger.link_workloads") || !hasTool(tools, "ledger.claim_next_workload") || !hasTool(tools, "ledger.workload_queue") || !hasTool(tools, "ledger.acquire_workload_lease") || !hasTool(tools, "ledger.renew_workload_lease") || !hasTool(tools, "ledger.release_workload_lease") || !hasTool(tools, "ledger.workload_leases") || !hasTool(tools, "ledger.get_policy") || !hasTool(tools, "ledger.policy_audit") || !hasTool(tools, "ledger.approval_routes") || !hasTool(tools, "ledger.approvals") || !hasTool(tools, "ledger.resolve_approval") || !hasTool(tools, "ledger.audit_log") || !hasTool(tools, "ledger.workload_timeline") || !hasTool(tools, "ledger.workload_state") || !hasTool(tools, "ledger.workload_feed") || !hasTool(tools, "ledger.record_tool_call") || !hasTool(tools, "ledger.record_context") || !hasTool(tools, "ledger.record_evaluation") || !hasTool(tools, "ledger.record_event") || !hasTool(tools, "ledger.validate_event") || !hasTool(tools, "ledger.event_schema") || !hasTool(tools, "ledger.event_examples") || !hasTool(tools, "ledger.adapter_contract") || !hasTool(tools, "ledger.conformance_matrix") || !hasTool(tools, "ledger.adapter_conformance") || !hasTool(tools, "ledger.integrations") || !hasTool(tools, "ledger.provider_profiles") || !hasTool(tools, "ledger.discovery") || !hasTool(tools, "ledger.contracts") || !hasTool(tools, "ledger.contracts_verify") || !hasTool(tools, "ledger.goal_coverage") || !hasTool(tools, "ledger.openapi") || !hasTool(tools, "ledger.runtime_status") || !hasTool(tools, "ledger.config_status") || !hasTool(tools, "ledger.readiness") || !hasTool(tools, "ledger.admission_check") {
		t.Fatalf("expected workload and policy tools, got %#v", tools)
	}
	if !hasTool(tools, "ledger.signal_taxonomy") {
		t.Fatalf("expected signal taxonomy tool, got %#v", tools)
	}
	budgetMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.current_budget"))
	if budgetMeta["write_mode"] != "none" || budgetMeta["writes_local_state"] != false || budgetMeta["available_in_read_only"] != true {
		t.Fatalf("budget tool metadata wrong: %#v", budgetMeta)
	}
	startMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.start_workload"))
	if startMeta["write_mode"] != "always" || startMeta["writes_local_state"] != true || startMeta["available_in_read_only"] != false {
		t.Fatalf("start workload metadata wrong: %#v", startMeta)
	}
	leaseMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.acquire_workload_lease"))
	if leaseMeta["write_mode"] != "always" || leaseMeta["writes_local_state"] != true || leaseMeta["available_in_read_only"] != false {
		t.Fatalf("lease acquire metadata wrong: %#v", leaseMeta)
	}
	claimMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.claim_next_workload"))
	if claimMeta["write_mode"] != "always" || claimMeta["writes_local_state"] != true || claimMeta["available_in_read_only"] != false {
		t.Fatalf("claim-next metadata wrong: %#v", claimMeta)
	}
	queueMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.workload_queue"))
	if queueMeta["write_mode"] != "none" || queueMeta["writes_local_state"] != false || queueMeta["available_in_read_only"] != true {
		t.Fatalf("workload queue metadata wrong: %#v", queueMeta)
	}
	leaseListMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.workload_leases"))
	if leaseListMeta["write_mode"] != "none" || leaseListMeta["writes_local_state"] != false || leaseListMeta["available_in_read_only"] != true {
		t.Fatalf("lease list metadata wrong: %#v", leaseListMeta)
	}
	policyMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.get_policy"))
	if policyMeta["write_mode"] != "conditional" || policyMeta["writes_local_state"] != true || policyMeta["available_in_read_only"] != true {
		t.Fatalf("policy tool metadata wrong: %#v", policyMeta)
	}
	runtimeMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.runtime_status"))
	if runtimeMeta["write_mode"] != "none" || runtimeMeta["writes_local_state"] != false || runtimeMeta["available_in_read_only"] != true {
		t.Fatalf("runtime tool metadata wrong: %#v", runtimeMeta)
	}
	discoveryMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.discovery"))
	if discoveryMeta["write_mode"] != "none" || discoveryMeta["writes_local_state"] != false || discoveryMeta["available_in_read_only"] != true {
		t.Fatalf("discovery tool metadata wrong: %#v", discoveryMeta)
	}
	contractsMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.contracts"))
	if contractsMeta["write_mode"] != "none" || contractsMeta["writes_local_state"] != false || contractsMeta["available_in_read_only"] != true {
		t.Fatalf("contracts tool metadata wrong: %#v", contractsMeta)
	}
	verificationMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.contracts_verify"))
	if verificationMeta["write_mode"] != "none" || verificationMeta["writes_local_state"] != false || verificationMeta["available_in_read_only"] != true {
		t.Fatalf("contract verification tool metadata wrong: %#v", verificationMeta)
	}
	goalCoverageMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.goal_coverage"))
	if goalCoverageMeta["write_mode"] != "none" || goalCoverageMeta["writes_local_state"] != false || goalCoverageMeta["available_in_read_only"] != true {
		t.Fatalf("goal coverage tool metadata wrong: %#v", goalCoverageMeta)
	}
	openAPIMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.openapi"))
	if openAPIMeta["write_mode"] != "none" || openAPIMeta["writes_local_state"] != false || openAPIMeta["available_in_read_only"] != true {
		t.Fatalf("openapi tool metadata wrong: %#v", openAPIMeta)
	}
	configMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.config_status"))
	if configMeta["write_mode"] != "none" || configMeta["writes_local_state"] != false || configMeta["available_in_read_only"] != true {
		t.Fatalf("config status tool metadata wrong: %#v", configMeta)
	}
	readinessMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.readiness"))
	if readinessMeta["write_mode"] != "none" || readinessMeta["writes_local_state"] != false || readinessMeta["available_in_read_only"] != true {
		t.Fatalf("readiness tool metadata wrong: %#v", readinessMeta)
	}
	admissionMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.admission_check"))
	if admissionMeta["write_mode"] != "none" || admissionMeta["writes_local_state"] != false || admissionMeta["available_in_read_only"] != true {
		t.Fatalf("admission tool metadata wrong: %#v", admissionMeta)
	}
	providerProfilesMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.provider_profiles"))
	if providerProfilesMeta["write_mode"] != "none" || providerProfilesMeta["writes_local_state"] != false || providerProfilesMeta["available_in_read_only"] != true {
		t.Fatalf("provider profiles tool metadata wrong: %#v", providerProfilesMeta)
	}
	agentProfilesMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.agent_profiles"))
	if agentProfilesMeta["write_mode"] != "none" || agentProfilesMeta["writes_local_state"] != false || agentProfilesMeta["available_in_read_only"] != true {
		t.Fatalf("agent profiles tool metadata wrong: %#v", agentProfilesMeta)
	}
	signalTaxonomyMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.signal_taxonomy"))
	if signalTaxonomyMeta["write_mode"] != "none" || signalTaxonomyMeta["writes_local_state"] != false || signalTaxonomyMeta["available_in_read_only"] != true {
		t.Fatalf("signal taxonomy tool metadata wrong: %#v", signalTaxonomyMeta)
	}
	conformanceMatrixMeta := agentLedgerToolMeta(t, toolByName(t, tools, "ledger.conformance_matrix"))
	if conformanceMatrixMeta["write_mode"] != "none" || conformanceMatrixMeta["writes_local_state"] != false || conformanceMatrixMeta["available_in_read_only"] != true {
		t.Fatalf("conformance matrix tool metadata wrong: %#v", conformanceMatrixMeta)
	}
	if annotations := toolByName(t, tools, "ledger.current_budget")["annotations"].(map[string]interface{}); annotations["readOnlyHint"] != true {
		t.Fatalf("budget annotations wrong: %#v", annotations)
	}
	if annotations := toolByName(t, tools, "ledger.start_workload")["annotations"].(map[string]interface{}); annotations["readOnlyHint"] != false {
		t.Fatalf("start workload annotations wrong: %#v", annotations)
	}
	payload := toolTextPayload(t, out[1])
	if payload["plan"] != "custom" || payload["method"] != "local-estimate" {
		t.Fatalf("unexpected budget payload: %#v", payload)
	}
	windows := payload["windows"].([]interface{})
	if len(windows) != 1 || windows[0].(map[string]interface{})["name"] != "day" {
		t.Fatalf("unexpected windows: %#v", windows)
	}
	discoveryPayload := toolTextPayload(t, out[2])
	if discoveryPayload["contract"] != "agent-ledger.discovery" || discoveryPayload["capability_catalog_hash"] == "" || discoveryPayload["adapter_spec_hash"] == "" || discoveryPayload["runtime_status_uri"] != "/api/runtime/status" {
		t.Fatalf("unexpected discovery payload: %#v", discoveryPayload)
	}
	contractsPayload := toolTextPayload(t, out[3])
	if contractsPayload["contract"] != "agent-ledger.contract-bundle" || contractsPayload["bundle_hash"] == "" {
		t.Fatalf("unexpected contracts payload: %#v", contractsPayload)
	}
	verificationPayload := toolTextPayload(t, out[4])
	if verificationPayload["contract"] != "agent-ledger.contract-verification" || verificationPayload["ok"] != true || verificationPayload["failed"] != float64(0) {
		t.Fatalf("unexpected contract verification payload: %#v", verificationPayload)
	}
	openAPIPayload := toolTextPayload(t, out[5])
	if openAPIPayload["openapi"] != "3.1.0" || openAPIPayload["x-agent-ledger"] == nil {
		t.Fatalf("unexpected openapi payload: %#v", openAPIPayload)
	}
	configPayload := toolTextPayload(t, out[6])
	if configPayload["contract"] != "agent-ledger.config-status" || configPayload["path_values_exposed"] != false || configPayload["secret_values_exposed"] != false {
		t.Fatalf("unexpected config status payload: %#v", configPayload)
	}
	readinessPayload := toolTextPayload(t, out[7])
	if readinessPayload["contract"] != "agent-ledger.readiness" || readinessPayload["prompt_content_stored"] != false || readinessPayload["usage_data_uploaded"] != false {
		t.Fatalf("unexpected readiness payload: %#v", readinessPayload)
	}
	admissionPayload := toolTextPayload(t, out[8])
	if admissionPayload["contract"] != "agent-ledger.admission-check" || admissionPayload["allowed"] != true || admissionPayload["writes_local_state"] != true {
		t.Fatalf("unexpected admission payload: %#v", admissionPayload)
	}
}

func TestMCPResourcesAndPrompts(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = true
	srv := New(db, cfg)
	srv.now = func() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"initialize"}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"agent-ledger://schema/canonical-events"}}`,
		`{"jsonrpc":"2.0","id":4,"method":"resources/read","params":{"uri":"agent-ledger://integrations/adapter-contract"}}`,
		`{"jsonrpc":"2.0","id":5,"method":"resources/read","params":{"uri":"agent-ledger://runtime/status"}}`,
		`{"jsonrpc":"2.0","id":6,"method":"resources/read","params":{"uri":"agent-ledger://discovery/manifest"}}`,
		`{"jsonrpc":"2.0","id":7,"method":"resources/read","params":{"uri":"agent-ledger://contracts/bundle"}}`,
		`{"jsonrpc":"2.0","id":8,"method":"resources/read","params":{"uri":"agent-ledger://contracts/verification"}}`,
		`{"jsonrpc":"2.0","id":9,"method":"resources/read","params":{"uri":"agent-ledger://contracts/openapi"}}`,
		`{"jsonrpc":"2.0","id":10,"method":"resources/read","params":{"uri":"agent-ledger://config/status"}}`,
		`{"jsonrpc":"2.0","id":11,"method":"resources/read","params":{"uri":"agent-ledger://readiness"}}`,
		`{"jsonrpc":"2.0","id":12,"method":"resources/read","params":{"uri":"agent-ledger://admission/check?surface=http&method=POST&path=/api/events&role=operator"}}`,
		`{"jsonrpc":"2.0","id":13,"method":"prompts/list"}`,
		`{"jsonrpc":"2.0","id":14,"method":"prompts/get","params":{"name":"agent-ledger/workload-brief","arguments":{"goal":"ship router","project":"quant","constraints":"privacy strict"}}}`,
	)
	caps := out[0]["result"].(map[string]interface{})["capabilities"].(map[string]interface{})
	if caps["resources"] == nil || caps["prompts"] == nil {
		t.Fatalf("missing resource/prompt capabilities: %#v", caps)
	}
	resourceCaps := caps["resources"].(map[string]interface{})
	if resourceCaps["subscribe"] != true {
		t.Fatalf("resource subscriptions should be advertised: %#v", resourceCaps)
	}
	resources := out[1]["result"].(map[string]interface{})["resources"].([]interface{})
	if !hasResource(resources, "agent-ledger://discovery/manifest") || !hasResource(resources, "agent-ledger://contracts/bundle") || !hasResource(resources, "agent-ledger://contracts/verification") || !hasResource(resources, "agent-ledger://goal/coverage") || !hasResource(resources, "agent-ledger://contracts/openapi") || !hasResource(resources, "agent-ledger://schema/canonical-events") || !hasResource(resources, "agent-ledger://schema/canonical-event-examples") || !hasResource(resources, "agent-ledger://integrations/provider-profiles") || !hasResource(resources, "agent-ledger://integrations/agent-profiles") || !hasResource(resources, "agent-ledger://integrations/adapter-contract") || !hasResource(resources, "agent-ledger://integrations/conformance-matrix") || !hasResource(resources, "agent-ledger://runtime/status") || !hasResource(resources, "agent-ledger://config/status") || !hasResource(resources, "agent-ledger://readiness") || !hasResource(resources, "agent-ledger://admission/check") || !hasResource(resources, "agent-ledger://budget/current") || !hasResource(resources, "agent-ledger://workloads/queue") || !hasResource(resources, "agent-ledger://workloads/leases") || !hasResource(resources, "agent-ledger://workloads/feed") || !hasResource(resources, "agent-ledger://workload/state") || !hasResource(resources, "agent-ledger://workload/timeline") || !hasResource(resources, "agent-ledger://agent-runs/liveness") || !hasResource(resources, "agent-ledger://policy/approvals") || !hasResource(resources, "agent-ledger://policy/approval-routes") {
		t.Fatalf("expected core resources, got %#v", resources)
	}
	if !hasResource(resources, "agent-ledger://integrations/signal-taxonomy") {
		t.Fatalf("expected signal taxonomy resource, got %#v", resources)
	}
	resourceText := resourceTextPayload(t, out[2])
	if !strings.Contains(resourceText, "workload.started") || !strings.Contains(resourceText, "rejected_payload_keys") {
		t.Fatalf("unexpected schema resource text: %s", resourceText)
	}
	runtimeText := resourceTextPayload(t, out[4])
	if !strings.Contains(runtimeText, `"contract": "agent-ledger.runtime-status"`) ||
		!strings.Contains(runtimeText, `"mode": "control-plane"`) ||
		!strings.Contains(runtimeText, `"write_operations": "enabled"`) ||
		!strings.Contains(runtimeText, `"capability_catalog_hash": "sha256:`) ||
		!strings.Contains(runtimeText, `"canonical_schema_hash": "sha256:`) ||
		!strings.Contains(runtimeText, `"adapter_spec_hash": "sha256:`) {
		t.Fatalf("unexpected runtime resource text: %s", runtimeText)
	}
	discoveryText := resourceTextPayload(t, out[5])
	if !strings.Contains(discoveryText, `"contract": "agent-ledger.discovery"`) || !strings.Contains(discoveryText, `"capability_catalog_hash": "sha256:`) || !strings.Contains(discoveryText, `"adapter_spec_hash": "sha256:`) {
		t.Fatalf("unexpected discovery resource text: %s", discoveryText)
	}
	contractsText := resourceTextPayload(t, out[6])
	if !strings.Contains(contractsText, `"contract": "agent-ledger.contract-bundle"`) ||
		!strings.Contains(contractsText, `"bundle_hash": "sha256:`) ||
		!strings.Contains(contractsText, `"primary_uri": "/api/goal-coverage"`) ||
		!strings.Contains(contractsText, `"primary_uri": "/api/admission/check"`) ||
		!strings.Contains(contractsText, `"primary_uri": "/api/event-schema"`) {
		t.Fatalf("unexpected contracts resource text: %s", contractsText)
	}
	verificationText := resourceTextPayload(t, out[7])
	if !strings.Contains(verificationText, `"contract": "agent-ledger.contract-verification"`) ||
		!strings.Contains(verificationText, `"ok": true`) ||
		!strings.Contains(verificationText, `"openapi.path./api/contracts/verify"`) {
		t.Fatalf("unexpected contract verification resource text: %s", verificationText)
	}
	openAPIText := resourceTextPayload(t, out[8])
	if !strings.Contains(openAPIText, `"openapi": "3.1.0"`) ||
		!strings.Contains(openAPIText, `"/api/events/validate"`) ||
		!strings.Contains(openAPIText, `"agent-ledger.control-plane-openapi"`) {
		t.Fatalf("unexpected openapi resource text: %s", openAPIText)
	}
	configText := resourceTextPayload(t, out[9])
	if !strings.Contains(configText, `"contract": "agent-ledger.config-status"`) ||
		!strings.Contains(configText, `"path_values_exposed": false`) ||
		!strings.Contains(configText, `"secret_values_exposed": false`) {
		t.Fatalf("unexpected config status resource text: %s", configText)
	}
	readinessText := resourceTextPayload(t, out[10])
	if !strings.Contains(readinessText, `"contract": "agent-ledger.readiness"`) ||
		!strings.Contains(readinessText, `"prompt_content_stored": false`) ||
		!strings.Contains(readinessText, `"usage_data_uploaded": false`) {
		t.Fatalf("unexpected readiness resource text: %s", readinessText)
	}
	admissionText := resourceTextPayload(t, out[11])
	if !strings.Contains(admissionText, `"contract": "agent-ledger.admission-check"`) ||
		!strings.Contains(admissionText, `"operation": "POST /api/events"`) ||
		!strings.Contains(admissionText, `"writes_local_state": true`) {
		t.Fatalf("unexpected admission resource text: %s", admissionText)
	}
	adapterText := resourceTextPayload(t, out[3])
	if !strings.Contains(adapterText, "agent-ledger.adapter-contract") || !strings.Contains(adapterText, "provider") || !strings.Contains(adapterText, "forbidden_payload_keys") {
		t.Fatalf("unexpected adapter contract resource text: %s", adapterText)
	}
	prompts := out[12]["result"].(map[string]interface{})["prompts"].([]interface{})
	if !hasPrompt(prompts, "agent-ledger/workload-brief") || !hasPrompt(prompts, "agent-ledger/cost-review") {
		t.Fatalf("expected prompts, got %#v", prompts)
	}
	promptText := promptTextPayload(t, out[13])
	if !strings.Contains(promptText, "ship router") || !strings.Contains(promptText, "privacy strict") {
		t.Fatalf("prompt did not interpolate arguments: %s", promptText)
	}
	if strings.Contains(strings.ToLower(promptText), "raw file contents") && !strings.Contains(promptText, "avoid") {
		t.Fatalf("prompt should warn against durable sensitive content: %s", promptText)
	}
}

func TestMCPProviderProfilesToolAndResource(t *testing.T) {
	db := openTestDB(t)
	srv := New(db, config.DefaultConfig())

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.provider_profiles","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"agent-ledger://integrations/provider-profiles"}}`,
	)
	toolPayload := toolTextPayload(t, out[0])
	if toolPayload["contract"] != "agent-ledger.provider-profile-catalog" || toolPayload["local_first"] != true {
		t.Fatalf("unexpected provider profile tool payload: %#v", toolPayload)
	}
	summary := toolPayload["summary"].(map[string]interface{})
	if summary["profiles"] != float64(7) || summary["openai_compatible"] == float64(0) {
		t.Fatalf("unexpected provider profile summary: %#v", summary)
	}
	rawTool, _ := json.Marshal(toolPayload)
	for _, forbidden := range []string{"api_key", "sk-", "C:/Users", "session_id"} {
		if strings.Contains(strings.ToLower(string(rawTool)), strings.ToLower(forbidden)) {
			t.Fatalf("provider profile tool leaked %q: %s", forbidden, rawTool)
		}
	}

	resourceText := resourceTextPayload(t, out[1])
	if !strings.Contains(resourceText, `"contract": "agent-ledger.provider-profile-catalog"`) ||
		!strings.Contains(resourceText, `"id": "openai-official"`) ||
		!strings.Contains(resourceText, `"id": "vllm-local"`) {
		t.Fatalf("unexpected provider profile resource: %s", resourceText)
	}
	for _, forbidden := range []string{"api_key", "sk-", "C:/Users", "session_id"} {
		if strings.Contains(strings.ToLower(resourceText), strings.ToLower(forbidden)) {
			t.Fatalf("provider profile resource leaked %q: %s", forbidden, resourceText)
		}
	}
}

func TestMCPAgentProfilesToolAndResource(t *testing.T) {
	db := openTestDB(t)
	srv := New(db, config.DefaultConfig())

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.agent_profiles","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"agent-ledger://integrations/agent-profiles"}}`,
	)
	toolPayload := toolTextPayload(t, out[0])
	if toolPayload["contract"] != "agent-ledger.agent-framework-profile-catalog" || toolPayload["local_first"] != true ||
		toolPayload["read_only_safe"] != true || toolPayload["writes_local_state"] != false {
		t.Fatalf("unexpected agent profile tool payload: %#v", toolPayload)
	}
	summary := toolPayload["summary"].(map[string]interface{})
	if summary["profiles"].(float64) < 10 || summary["local_collectors"].(float64) < 6 || summary["mcp_tooling"].(float64) == 0 {
		t.Fatalf("unexpected agent profile summary: %#v", summary)
	}
	rawTool, _ := json.Marshal(toolPayload)
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "bearer ", "C:/Users", "session_id", "webhook"} {
		if strings.Contains(strings.ToLower(string(rawTool)), strings.ToLower(forbidden)) {
			t.Fatalf("agent profile tool leaked %q: %s", forbidden, rawTool)
		}
	}

	resourceText := resourceTextPayload(t, out[1])
	if !strings.Contains(resourceText, `"contract": "agent-ledger.agent-framework-profile-catalog"`) ||
		!strings.Contains(resourceText, `"id": "codex-cli"`) ||
		!strings.Contains(resourceText, `"id": "mcp-wrapper"`) ||
		!strings.Contains(resourceText, `"id": "a2a-task-runtime"`) {
		t.Fatalf("unexpected agent profile resource: %s", resourceText)
	}
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "bearer ", "C:/Users", "session_id", "webhook"} {
		if strings.Contains(strings.ToLower(resourceText), strings.ToLower(forbidden)) {
			t.Fatalf("agent profile resource leaked %q: %s", forbidden, resourceText)
		}
	}
}

func TestMCPSignalTaxonomyToolAndResource(t *testing.T) {
	db := openTestDB(t)
	srv := New(db, config.DefaultConfig())

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.signal_taxonomy","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"agent-ledger://integrations/signal-taxonomy"}}`,
	)
	toolPayload := toolTextPayload(t, out[0])
	if toolPayload["contract"] != "agent-ledger.signal-taxonomy" || toolPayload["local_first"] != true ||
		toolPayload["read_only_safe"] != true || toolPayload["writes_local_state"] != false {
		t.Fatalf("unexpected signal taxonomy tool payload: %#v", toolPayload)
	}
	summary := toolPayload["summary"].(map[string]interface{})
	if summary["signals"].(float64) < 11 || summary["event_signals"].(float64) < 11 || summary["usage_signals"].(float64) < 3 {
		t.Fatalf("unexpected signal taxonomy summary: %#v", summary)
	}
	rawTool, _ := json.Marshal(toolPayload)
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "bearer ", "C:/Users", "session_id", "prompt text", "response text"} {
		if strings.Contains(strings.ToLower(string(rawTool)), strings.ToLower(forbidden)) {
			t.Fatalf("signal taxonomy tool leaked %q: %s", forbidden, rawTool)
		}
	}

	resourceText := resourceTextPayload(t, out[1])
	if !strings.Contains(resourceText, `"contract": "agent-ledger.signal-taxonomy"`) ||
		!strings.Contains(resourceText, `"id": "usage.tokens"`) ||
		!strings.Contains(resourceText, `"id": "workload.identity"`) ||
		!strings.Contains(resourceText, `"id": "pricing.provenance"`) {
		t.Fatalf("unexpected signal taxonomy resource: %s", resourceText)
	}
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "bearer ", "C:/Users", "session_id", "prompt text", "response text"} {
		if strings.Contains(strings.ToLower(resourceText), strings.ToLower(forbidden)) {
			t.Fatalf("signal taxonomy resource leaked %q: %s", forbidden, resourceText)
		}
	}
}

func TestMCPIntegrationRecommendationToolAndResource(t *testing.T) {
	db := openTestDB(t)
	srv := New(db, config.DefaultConfig())

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.integration_recommendation","arguments":{"agent":"codex-cli","provider":"openai-official","surface":"provider-stream","signals":["model","usage","cache"]}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"agent-ledger://integrations/recommendation?agent=codex-cli&provider=openai-official&surface=provider-stream&signals=model,usage,cache"}}`,
	)
	toolPayload := toolTextPayload(t, out[0])
	if toolPayload["contract"] != "agent-ledger.integration-recommendation" || toolPayload["read_only_safe"] != true || toolPayload["writes_local_state"] != false {
		t.Fatalf("unexpected recommendation tool payload: %#v", toolPayload)
	}
	if toolPayload["recommended_surface"] != "provider-stream" || toolPayload["confidence"].(float64) < 0.8 {
		t.Fatalf("unexpected recommendation match: %#v", toolPayload)
	}
	rawTool, _ := json.Marshal(toolPayload)
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "bearer ", "C:/Users", "session_id", "prompt text", "response text"} {
		if strings.Contains(strings.ToLower(string(rawTool)), strings.ToLower(forbidden)) {
			t.Fatalf("recommendation tool leaked %q: %s", forbidden, rawTool)
		}
	}

	resourceText := resourceTextPayload(t, out[1])
	if !strings.Contains(resourceText, `"contract": "agent-ledger.integration-recommendation"`) ||
		!strings.Contains(resourceText, `"recommended_surface": "provider-stream"`) ||
		!strings.Contains(resourceText, `"recommendation_contract"`) {
		t.Fatalf("unexpected recommendation resource: %s", resourceText)
	}
	for _, forbidden := range []string{"api_key", "sk-proj-", "sk_live_", "bearer ", "C:/Users", "session_id", "prompt text", "response text"} {
		if strings.Contains(strings.ToLower(resourceText), strings.ToLower(forbidden)) {
			t.Fatalf("recommendation resource leaked %q: %s", forbidden, resourceText)
		}
	}
}

func TestMCPConformanceMatrixToolAndResource(t *testing.T) {
	db := openTestDB(t)
	srv := New(db, config.DefaultConfig())

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.conformance_matrix","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"agent-ledger://integrations/conformance-matrix"}}`,
	)
	toolPayload := toolTextPayload(t, out[0])
	if toolPayload["contract"] != "agent-ledger.adapter-conformance-matrix" || toolPayload["read_only_safe"] != true || toolPayload["writes_local_state"] != false {
		t.Fatalf("unexpected conformance matrix tool payload: %#v", toolPayload)
	}
	summary := toolPayload["summary"].(map[string]interface{})
	if summary["fixtures"] != float64(13) || summary["input_kinds"] == float64(0) {
		t.Fatalf("unexpected conformance matrix summary: %#v", summary)
	}
	rawTool, _ := json.Marshal(toolPayload)
	for _, forbidden := range []string{"api_key", "sk-", "C:/Users", "session_id"} {
		if strings.Contains(strings.ToLower(string(rawTool)), strings.ToLower(forbidden)) {
			t.Fatalf("conformance matrix tool leaked %q: %s", forbidden, rawTool)
		}
	}

	resourceText := resourceTextPayload(t, out[1])
	if !strings.Contains(resourceText, `"contract": "agent-ledger.adapter-conformance-matrix"`) ||
		!strings.Contains(resourceText, `"path": "examples/adapter-fixtures/provider-openai-response.json"`) ||
		!strings.Contains(resourceText, `"path": "examples/adapter-fixtures/a2a-delegated-task.json"`) {
		t.Fatalf("unexpected conformance matrix resource: %s", resourceText)
	}
	for _, forbidden := range []string{"api_key", "sk-", "C:/Users", "session_id"} {
		if strings.Contains(strings.ToLower(resourceText), strings.ToLower(forbidden)) {
			t.Fatalf("conformance matrix resource leaked %q: %s", forbidden, resourceText)
		}
	}
}

func TestMCPRecentWorkloadsResourceIncludesState(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	workloadID, err := db.CreateWorkload("resource state workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}

	out := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"agent-ledger://workloads/recent"}}`)
	var payload map[string]interface{}
	text := resourceTextPayload(t, out[0])
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode workload resource: %v\n%s", err, text)
	}
	rows, ok := payload["rows"].([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("resource missing workload rows: %#v", payload)
	}
	states, ok := payload["states"].([]interface{})
	if !ok || len(states) == 0 {
		t.Fatalf("resource missing workload states: %#v", payload)
	}
	state := states[0].(map[string]interface{})
	if state["workload_id"] != workloadID || state["phase"] != "planned" || state["terminal"] != false {
		t.Fatalf("unexpected resource state: %#v", state)
	}
	if payload["stale_after_seconds"] == nil {
		t.Fatalf("resource missing stale threshold: %#v", payload)
	}
}

func TestMCPWorkloadQueueResource(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	if _, err := db.CreateWorkload("queue resource workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0); err != nil {
		t.Fatalf("create workload: %v", err)
	}

	out := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"agent-ledger://workloads/queue?source=codex"}}`)
	var payload map[string]interface{}
	text := resourceTextPayload(t, out[0])
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode queue resource: %v\n%s", err, text)
	}
	if payload["ok"] != true || payload["claimable"].(float64) != 1 || payload["active_leases"].(float64) != 0 {
		t.Fatalf("unexpected queue resource payload: %#v", payload)
	}
}

func TestMCPWorkloadLeasesResourceIsPrivacySafe(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	workloadID, err := db.CreateWorkload("lease resource workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}
	lease, err := db.AcquireWorkloadLease(workloadID, "private-router", "private purpose note", 2*time.Minute)
	if err != nil {
		t.Fatalf("acquire lease: %v", err)
	}

	out := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"agent-ledger://workloads/leases?include_inactive=1&limit=10"}}`)
	text := resourceTextPayload(t, out[0])
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode leases resource: %v\n%s", err, text)
	}
	rows := payload["rows"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("unexpected lease resource rows: %#v", payload)
	}
	raw := []byte(text)
	for _, forbidden := range []string{workloadID, "private-router", "private purpose note", lease.LeaseToken} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("leases resource leaked %q: %s", forbidden, text)
		}
	}
	row := rows[0].(map[string]interface{})
	if row["lease_id"] != lease.LeaseID || row["status"] == "" || row["ttl_seconds"] == nil {
		t.Fatalf("unexpected lease resource row: %#v", row)
	}
}

func TestMCPRunLivenessResourceIsPrivacySafeAndCursorStable(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	workloadID, err := db.CreateWorkload("private liveness goal", "codex", "private-project", "private/repo", "private-branch", "", "infra", 0)
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "private-agent", "codex --token secret", "C:/Users/zhang/private")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-liveness-resource", runID, "working", "testing", "private status message", 0.5, nil, time.Now().UTC().Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	uri := "agent-ledger://agent-runs/liveness?max_age=10m&stale_only=1&source=codex&project=private-project&limit=10"
	out := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"`+uri+`"}}`)
	text := resourceTextPayload(t, out[0])
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode liveness resource: %v\n%s", err, text)
	}
	rows := payload["rows"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("unexpected liveness rows: %#v", payload)
	}
	raw := []byte(text)
	for _, forbidden := range []string{workloadID, runID, "private liveness goal", "private-project", "private/repo", "private-branch", "private status message", "C:/Users/zhang/private", "secret"} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("liveness resource leaked %q: %s", forbidden, text)
		}
	}
	row := rows[0].(map[string]interface{})
	if row["source"] != "codex" || row["status"] == "" || row["stale"] != true {
		t.Fatalf("unexpected liveness row: %#v", row)
	}
	firstCursor, err := srv.resourceCursor(uri)
	if err != nil {
		t.Fatalf("first liveness cursor: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	secondCursor, err := srv.resourceCursor(uri)
	if err != nil {
		t.Fatalf("second liveness cursor: %v", err)
	}
	if firstCursor != secondCursor {
		t.Fatalf("liveness cursor changed without semantic change: first=%s second=%s", firstCursor, secondCursor)
	}
}

func TestMCPWorkloadStateAndTimelineResourcesArePrivacySafe(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	workloadID, err := db.CreateWorkload("private state goal", "codex", "private-project", "private/repo", "private-branch", "", "private-team", 100)
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "private-agent", "codex --token secret", "C:/Users/zhang/private")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-state-resource", runID, "working", "testing", "private status message", 0.5, nil, time.Now().UTC().Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	stateURI := "agent-ledger://workload/state?workload_id=" + workloadID + "&max_age=10m"
	timelineURI := "agent-ledger://workload/timeline?workload_id=" + workloadID + "&limit=20"
	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"`+stateURI+`"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"`+timelineURI+`"}}`,
	)
	stateText := resourceTextPayload(t, out[0])
	timelineText := resourceTextPayload(t, out[1])
	combined := stateText + "\n" + timelineText
	for _, forbidden := range []string{workloadID, runID, "private state goal", "private-project", "private/repo", "private-branch", "private-team", "private status message", "C:/Users/zhang/private", "secret", "private-agent"} {
		if strings.Contains(combined, forbidden) {
			t.Fatalf("workload state/timeline resource leaked %q:\n%s", forbidden, combined)
		}
	}
	var statePayload map[string]interface{}
	if err := json.Unmarshal([]byte(stateText), &statePayload); err != nil {
		t.Fatalf("decode state resource: %v\n%s", err, stateText)
	}
	state := statePayload["state"].(map[string]interface{})
	if state["source"] != "codex" || state["phase"] == "" || state["stale"] != true {
		t.Fatalf("unexpected privacy-safe state: %#v", state)
	}
	var timelinePayload map[string]interface{}
	if err := json.Unmarshal([]byte(timelineText), &timelinePayload); err != nil {
		t.Fatalf("decode timeline resource: %v\n%s", err, timelineText)
	}
	rows := timelinePayload["rows"].([]interface{})
	if len(rows) == 0 {
		t.Fatalf("expected privacy-safe timeline rows: %#v", timelinePayload)
	}
	firstCursor, err := srv.resourceCursor(stateURI)
	if err != nil {
		t.Fatalf("first state cursor: %v", err)
	}
	secondCursor, err := srv.resourceCursor(stateURI)
	if err != nil {
		t.Fatalf("second state cursor: %v", err)
	}
	if firstCursor != secondCursor {
		t.Fatalf("state cursor changed without semantic change: first=%s second=%s", firstCursor, secondCursor)
	}
}

func TestMCPWorkloadFeedToolAndResource(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	if _, err := db.CreateWorkload("feed workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0); err != nil {
		t.Fatalf("create workload: %v", err)
	}

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.workload_feed","arguments":{"limit":10,"severity":"info"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"agent-ledger://workloads/feed"}}`,
	)
	payload := toolTextPayload(t, out[0])
	rows, ok := payload["rows"].([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("tool feed missing rows: %#v", payload)
	}
	if !strings.HasPrefix(payload["cursor"].(string), "sha256:") || payload["generated_at"] == "" {
		t.Fatalf("tool feed missing cursor metadata: %#v", payload)
	}
	row := rows[0].(map[string]interface{})
	if row["event_type"] != "workload.state.planned" || row["source"] != "codex" {
		t.Fatalf("unexpected feed row: %#v", row)
	}

	var resourcePayload map[string]interface{}
	text := resourceTextPayload(t, out[1])
	if err := json.Unmarshal([]byte(text), &resourcePayload); err != nil {
		t.Fatalf("decode feed resource: %v\n%s", err, text)
	}
	resourceRows, ok := resourcePayload["rows"].([]interface{})
	if !ok || len(resourceRows) == 0 || !strings.HasPrefix(resourcePayload["cursor"].(string), "sha256:") {
		t.Fatalf("resource feed missing rows/cursor: %#v", resourcePayload)
	}
}

func TestMCPParameterizedWorkloadResources(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	now := time.Now().UTC()
	srv.now = func() time.Time { return now }
	if _, err := db.CreateWorkload("planned feed workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0); err != nil {
		t.Fatalf("create planned workload: %v", err)
	}
	staleWorkloadID, err := db.CreateWorkload("stale feed workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("create stale workload: %v", err)
	}
	runID, err := db.StartAgentRun(staleWorkloadID, "codex", "codex", "codex run", "C:/work")
	if err != nil {
		t.Fatalf("start stale run: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-stale-mcp", runID, "working", "testing", "stale heartbeat", 0.4, nil, now.Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("record stale heartbeat: %v", err)
	}

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"agent-ledger://workloads/feed?severity=warning&source=codex&project=agent-ledger&limit=5&stale_after=1m"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"agent-ledger://workloads/recent?source=codex&project=agent-ledger&limit=1"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/read","params":{"uri":"agent-ledger://budget/current?window=day&source=codex&project=agent-ledger"}}`,
	)
	var feed map[string]interface{}
	if err := json.Unmarshal([]byte(resourceTextPayload(t, out[0])), &feed); err != nil {
		t.Fatalf("decode feed resource: %v", err)
	}
	rows := feed["rows"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("expected one warning row, got %#v", feed)
	}
	row := rows[0].(map[string]interface{})
	if row["severity"] != "warning" || row["phase"] != "stale" || row["source"] != "codex" {
		t.Fatalf("unexpected parameterized feed row: %#v", row)
	}
	if feed["stale_after_seconds"] != float64(60) {
		t.Fatalf("parameterized stale_after not applied: %#v", feed)
	}

	var recent map[string]interface{}
	if err := json.Unmarshal([]byte(resourceTextPayload(t, out[1])), &recent); err != nil {
		t.Fatalf("decode recent resource: %v", err)
	}
	if recent["limit"] != float64(1) || recent["from"] == "" || recent["to"] == "" {
		t.Fatalf("recent resource query parameters not applied: %#v", recent)
	}
	var budget map[string]interface{}
	if err := json.Unmarshal([]byte(resourceTextPayload(t, out[2])), &budget); err != nil {
		t.Fatalf("decode budget resource: %v", err)
	}
	windows := budget["windows"].([]interface{})
	if budget["method"] != "local-estimate" || len(windows) != 1 || windows[0].(map[string]interface{})["name"] != "day" {
		t.Fatalf("budget resource query parameters not applied: %#v", budget)
	}
}

func TestMCPResourceSubscribeUnsubscribe(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	if _, err := db.CreateWorkload("subscribe workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0); err != nil {
		t.Fatalf("create workload: %v", err)
	}

	out := serveRawLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"resources/subscribe","params":{"uri":"agent-ledger://workloads/feed"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/unsubscribe","params":{"uri":"agent-ledger://workloads/feed"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"resources/subscribe","params":{"uri":"agent-ledger://unknown"}}`,
	)
	subscribe := out[0]["result"].(map[string]interface{})
	if subscribe["ok"] != true || subscribe["uri"] != "agent-ledger://workloads/feed" || !strings.HasPrefix(subscribe["cursor"].(string), "sha256:") {
		t.Fatalf("unexpected subscribe result: %#v", subscribe)
	}
	unsubscribe := out[1]["result"].(map[string]interface{})
	if unsubscribe["ok"] != true || unsubscribe["subscribed"] != true {
		t.Fatalf("unexpected unsubscribe result: %#v", unsubscribe)
	}
	if out[2]["error"] == nil {
		t.Fatalf("unknown resource subscribe should fail: %#v", out[2])
	}
}

func TestMCPParameterizedResourceSubscriptionNotification(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	srv.subscriptionInterval = time.Hour
	now := time.Now().UTC()
	srv.now = func() time.Time { return now }
	var output bytes.Buffer
	subscriptions := newSubscriptionState(srv, json.NewEncoder(&output))
	defer subscriptions.stop()
	uri := "agent-ledger://workloads/feed?severity=warning&source=codex&project=agent-ledger&limit=10&stale_after=1m"
	result, err := subscriptions.subscribe(uri)
	if err != nil {
		t.Fatalf("subscribe parameterized resource: %v", err)
	}
	if result["uri"] != uri || result["mode"] != "local-poll" {
		t.Fatalf("unexpected subscribe result: %#v", result)
	}
	workloadID, err := db.CreateWorkload("subscription warning workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("create workload: %v", err)
	}
	runID, err := db.StartAgentRun(workloadID, "codex", "codex", "codex run", "C:/work")
	if err != nil {
		t.Fatalf("start run: %v", err)
	}
	if _, err := db.RecordAgentRunHeartbeat("evt-subscribe-mcp", runID, "working", "testing", "stale heartbeat", 0.4, nil, now.Add(-20*time.Minute), 1); err != nil {
		t.Fatalf("record heartbeat: %v", err)
	}
	subscriptions.pollOnce()

	var notification map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &notification); err != nil {
		t.Fatalf("decode notification: %v\n%s", err, output.String())
	}
	if notification["method"] != "notifications/resources/updated" {
		t.Fatalf("unexpected notification: %#v", notification)
	}
	params := notification["params"].(map[string]interface{})
	if params["uri"] != uri || !strings.HasPrefix(params["cursor"].(string), "sha256:") {
		t.Fatalf("unexpected notification params: %#v", params)
	}
}

func TestMCPQueueResourceSubscriptionIgnoresGeneratedAtNoise(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	srv.subscriptionInterval = time.Hour
	uri := "agent-ledger://workloads/queue?source=codex"
	first, err := srv.resourceCursor(uri)
	if err != nil {
		t.Fatalf("initial queue cursor: %v", err)
	}
	time.Sleep(2 * time.Millisecond)
	second, err := srv.resourceCursor(uri)
	if err != nil {
		t.Fatalf("second queue cursor: %v", err)
	}
	if first != second {
		t.Fatalf("queue cursor changed without queue data change: first=%s second=%s", first, second)
	}

	var output bytes.Buffer
	subscriptions := newSubscriptionState(srv, json.NewEncoder(&output))
	defer subscriptions.stop()
	if _, err := subscriptions.subscribe(uri); err != nil {
		t.Fatalf("subscribe queue resource: %v", err)
	}
	subscriptions.pollOnce()
	if strings.TrimSpace(output.String()) != "" {
		t.Fatalf("queue subscription emitted without data change: %s", output.String())
	}
	if _, err := db.CreateWorkload("queue subscription workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0); err != nil {
		t.Fatalf("create workload: %v", err)
	}
	subscriptions.pollOnce()
	var notification map[string]interface{}
	if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &notification); err != nil {
		t.Fatalf("decode queue notification: %v\n%s", err, output.String())
	}
	if notification["method"] != "notifications/resources/updated" {
		t.Fatalf("unexpected queue notification: %#v", notification)
	}
	params := notification["params"].(map[string]interface{})
	if params["uri"] != uri || !strings.HasPrefix(params["cursor"].(string), "sha256:") {
		t.Fatalf("unexpected queue notification params: %#v", params)
	}
}

func TestMCPUnknownResourceReturnsError(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	responses := serveRawLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"agent-ledger://unknown"}}`)
	if responses[0]["error"] == nil {
		t.Fatalf("expected unknown resource error: %#v", responses[0])
	}
}

func TestMCPReadOnlyAllowsReadToolsAndRejectsWriteTools(t *testing.T) {
	db := openTestDB(t)
	workloadID, err := db.CreateWorkload("read-only policy guard", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	cfg.Policies.Enabled = true
	cfg.Policies.Rules = []config.PolicyRule{{
		Name: "warn-model", Scope: "model", Match: "gpt-5.5", Action: "warn", Message: "review model",
	}}
	srv := New(db, cfg)
	srv.now = func() time.Time { return time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC) }

	readResponses := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.current_budget","arguments":{"window":"day"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ledger.approvals","arguments":{"status":"pending","privacy":true}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ledger.get_policy","arguments":{"model":"gpt-5.5","action":"model.call"}}}`,
		`{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ledger.runtime_status","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"ledger.discovery","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"ledger.contracts","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"ledger.contracts_verify","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"ledger.openapi","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"ledger.workload_leases","arguments":{"limit":10}}}`,
		`{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"ledger.workload_queue","arguments":{"source":"codex"}}}`,
	)
	if toolTextPayload(t, readResponses[0])["method"] != "local-estimate" {
		t.Fatalf("read-only read tool returned unexpected payload: %#v", readResponses[0])
	}
	if toolTextPayload(t, readResponses[1])["status"] != "pending" {
		t.Fatalf("read-only approvals tool returned unexpected payload: %#v", readResponses[1])
	}
	if toolTextPayload(t, readResponses[2])["action"] != "warn" {
		t.Fatalf("read-only policy advisory returned unexpected payload: %#v", readResponses[2])
	}
	runtimePayload := toolTextPayload(t, readResponses[3])
	if runtimePayload["contract"] != "agent-ledger.runtime-status" || runtimePayload["mode"] != "observer" ||
		runtimePayload["read_only"] != true || runtimePayload["write_operations"] != "disabled" ||
		runtimePayload["capability_catalog_hash"] == "" || runtimePayload["canonical_schema_hash"] == "" ||
		runtimePayload["adapter_spec_hash"] == "" {
		t.Fatalf("read-only runtime tool returned unexpected payload: %#v", runtimePayload)
	}
	discoveryPayload := toolTextPayload(t, readResponses[4])
	if discoveryPayload["contract"] != "agent-ledger.discovery" || discoveryPayload["read_only"] != true || discoveryPayload["capability_catalog_hash"] == "" || discoveryPayload["adapter_spec_hash"] == "" {
		t.Fatalf("read-only discovery tool returned unexpected payload: %#v", discoveryPayload)
	}
	contractsPayload := toolTextPayload(t, readResponses[5])
	if contractsPayload["contract"] != "agent-ledger.contract-bundle" || contractsPayload["read_only"] != true || contractsPayload["bundle_hash"] == "" {
		t.Fatalf("read-only contracts tool returned unexpected payload: %#v", contractsPayload)
	}
	verificationPayload := toolTextPayload(t, readResponses[6])
	if verificationPayload["contract"] != "agent-ledger.contract-verification" || verificationPayload["ok"] != true || verificationPayload["read_only"] != true {
		t.Fatalf("read-only contract verification tool returned unexpected payload: %#v", verificationPayload)
	}
	openAPIPayload := toolTextPayload(t, readResponses[7])
	if openAPIPayload["openapi"] != "3.1.0" || openAPIPayload["x-agent-ledger"] == nil {
		t.Fatalf("read-only openapi tool returned unexpected payload: %#v", openAPIPayload)
	}
	leaseListPayload := toolTextPayload(t, readResponses[8])
	if _, ok := leaseListPayload["rows"].([]interface{}); !ok {
		t.Fatalf("read-only lease list returned unexpected payload: %#v", leaseListPayload)
	}
	queuePayload := toolTextPayload(t, readResponses[9])
	if queuePayload["ok"] != true {
		t.Fatalf("read-only workload queue returned unexpected payload: %#v", queuePayload)
	}

	writeResponses := serveRawLines(t, srv,
		`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"ledger.start_workload","arguments":{"goal":"blocked","source":"codex"}}}`,
		`{"jsonrpc":"2.0","id":12,"method":"tools/call","params":{"name":"ledger.resolve_approval","arguments":{"request_id":"apr_x","status":"approved"}}}`,
		`{"jsonrpc":"2.0","id":13,"method":"tools/call","params":{"name":"ledger.get_policy","arguments":{"workload_id":"`+workloadID+`","model":"gpt-5.5","action":"model.call"}}}`,
		`{"jsonrpc":"2.0","id":14,"method":"tools/call","params":{"name":"ledger.acquire_workload_lease","arguments":{"workload_id":"`+workloadID+`","holder":"router-a"}}}`,
		`{"jsonrpc":"2.0","id":15,"method":"tools/call","params":{"name":"ledger.claim_next_workload","arguments":{"holder":"router-a"}}}`,
	)
	if writeResponses[0]["error"] == nil {
		t.Fatalf("expected read-only write tool error: %#v", writeResponses[0])
	}
	if writeResponses[1]["error"] == nil {
		t.Fatalf("expected read-only approval resolve error: %#v", writeResponses[1])
	}
	if writeResponses[2]["error"] == nil {
		t.Fatalf("expected read-only policy record error: %#v", writeResponses[2])
	}
	if writeResponses[3]["error"] == nil {
		t.Fatalf("expected read-only lease acquire error: %#v", writeResponses[3])
	}
	if writeResponses[4]["error"] == nil {
		t.Fatalf("expected read-only claim-next error: %#v", writeResponses[4])
	}
	report, err := db.GetPolicyEnforcementReport(10)
	if err != nil {
		t.Fatalf("GetPolicyEnforcementReport: %v", err)
	}
	if len(report.Decisions) != 0 {
		t.Fatalf("read-only MCP get_policy wrote policy decisions: %+v", report.Decisions)
	}
}

func TestMCPWorkloadLifecycleArtifactAndPolicy(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = true
	cfg.Policies.Rules = []config.PolicyRule{{
		Name:    "warn-high-model",
		Scope:   "model",
		Match:   "gpt-5.5",
		Action:  "warn",
		Message: "use with budget awareness",
	}}
	srv := New(db, cfg)

	create := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.start_workload","arguments":{"goal":"ship allocator audit","source":"codex","project":"agent-ledger","repo":"zhenzhis/agent-ledger","git_branch":"main","agent_name":"codex","command":"codex run","cwd":"C:/work"}}}`)[0]
	created := toolTextPayload(t, create)
	workloadID, _ := created["workload_id"].(string)
	runID, _ := created["run_id"].(string)
	if workloadID == "" || runID == "" {
		t.Fatalf("missing ids: %#v", created)
	}
	parentID, err := db.CreateWorkload("parent workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("CreateWorkload parent: %v", err)
	}

	startRunLine := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ledger.start_run","arguments":{"workload_id":"` + workloadID + `","source":"codex","agent_name":"codex-worker","command":"codex worker","cwd":"C:/work"}}}`
	linkLine := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ledger.link_workloads","arguments":{"source_workload_id":"` + workloadID + `","target_workload_id":"` + parentID + `","relation":"depends-on","reason":"privacy-safe parent dependency","created_by":"mcp-test"}}}`
	policyLine := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ledger.get_policy","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","model":"gpt-5.5","role":"operator"}}}`
	toolLine := `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"ledger.record_tool_call","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","source":"codex","tool_call_id":"tool-mcp","tool_name":"shell","tool_type":"command","status":"ok","duration_ms":120,"params_hash":"sha256:params"}}}`
	contextLine := `{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"ledger.record_context","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","source":"codex","context_ref_id":"ctx-mcp","ref_type":"repo","ref_hash":"sha256:context","label":"privacy-safe-context","repo":"zhenzhis/agent-ledger","git_branch":"main","privacy_label":"synthetic"}}}`
	artifactLine := `{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"ledger.record_artifact","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","artifact_type":"report","label":"privacy-safe-summary","path_hash":"sha256:abc","sha256":"def","metadata":{"format":"markdown"}}}}`
	evaluationLine := `{"jsonrpc":"2.0","id":8,"method":"tools/call","params":{"name":"ledger.record_evaluation","arguments":{"workload_id":"` + workloadID + `","run_id":"` + runID + `","source":"codex","evaluation_id":"eval-mcp","evaluator":"ci","status":"pass","score":0.97,"signal":"unit-tests","notes":"privacy-safe acceptance signal"}}}`
	closeLine := `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"ledger.close_workload","arguments":{"workload_id":"` + workloadID + `","status":"completed","outcome":"accepted"}}}`
	timelineLine := `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"ledger.workload_timeline","arguments":{"workload_id":"` + workloadID + `","limit":20}}}`
	stateLine := `{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"ledger.workload_state","arguments":{"workload_id":"` + workloadID + `","max_age":"10m"}}}`
	responses := serveLines(t, srv, startRunLine, linkLine, policyLine, toolLine, contextLine, artifactLine, evaluationLine, closeLine, timelineLine, stateLine)
	startedRun := toolTextPayload(t, responses[0])
	if startedRun["run_id"] == "" || startedRun["workload_id"] != workloadID {
		t.Fatalf("start run payload=%#v", startedRun)
	}
	link := toolTextPayload(t, responses[1])
	if link["link_id"] == "" || link["target_workload_id"] != parentID {
		t.Fatalf("missing link result: %#v", link)
	}
	policy := toolTextPayload(t, responses[2])
	if policy["action"] != "warn" {
		t.Fatalf("policy action=%#v", policy["action"])
	}
	tool := toolTextPayload(t, responses[3])
	if tool["status"] != "inserted" {
		t.Fatalf("missing tool result: %#v", tool)
	}
	context := toolTextPayload(t, responses[4])
	if context["status"] != "inserted" {
		t.Fatalf("missing context result: %#v", context)
	}
	artifact := toolTextPayload(t, responses[5])
	if artifact["artifact_id"] == "" {
		t.Fatalf("missing artifact id: %#v", artifact)
	}
	evaluation := toolTextPayload(t, responses[6])
	if evaluation["status"] != "inserted" {
		t.Fatalf("missing evaluation result: %#v", evaluation)
	}
	closed := toolTextPayload(t, responses[7])
	if closed["status"] != "completed" {
		t.Fatalf("close payload=%#v", closed)
	}
	timeline := toolTextPayload(t, responses[8])
	rows, ok := timeline["rows"].([]interface{})
	if !ok || len(rows) == 0 {
		t.Fatalf("timeline payload=%#v", timeline)
	}
	if !timelineHasKind(rows, "evaluation") {
		t.Fatalf("timeline missing evaluation: %#v", timeline)
	}
	if !timelineHasKind(rows, "workload_link") {
		t.Fatalf("timeline missing workload link: %#v", timeline)
	}
	state := toolTextPayload(t, responses[9])
	if state["phase"] != "accepted" || state["terminal"] != true {
		t.Fatalf("unexpected workload state: %#v", state)
	}

	detail, err := db.GetWorkloadDetail(workloadID)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Summary.Status != "completed" {
		t.Fatalf("status=%s", detail.Summary.Status)
	}
	if len(detail.Runs) != 2 {
		t.Fatalf("runs=%#v", detail.Runs)
	}
	if len(detail.ToolCalls) != 1 || detail.ToolCalls[0].ToolCallID != "tool-mcp" || detail.ToolCalls[0].ToolName != "shell" {
		t.Fatalf("tool_calls=%#v", detail.ToolCalls)
	}
	if len(detail.ContextRefs) != 1 || detail.ContextRefs[0].ContextRefID != "ctx-mcp" || detail.ContextRefs[0].RefHash != "sha256:context" {
		t.Fatalf("context_refs=%#v", detail.ContextRefs)
	}
	if len(detail.Policies) != 1 || detail.Policies[0].Action != "warn" {
		t.Fatalf("policy decisions=%#v", detail.Policies)
	}
	if len(detail.Links) != 1 || detail.Links[0].TargetWorkloadID != parentID || detail.Links[0].Relation != "depends_on" {
		t.Fatalf("links=%#v", detail.Links)
	}
	if len(detail.Artifacts) != 1 || detail.Artifacts[0].Label != "privacy-safe-summary" {
		t.Fatalf("artifacts=%#v", detail.Artifacts)
	}
	if len(detail.Evaluations) != 1 || detail.Evaluations[0].EvaluationID != "eval-mcp" || detail.Evaluations[0].Status != "pass" || detail.Evaluations[0].Signal != "unit-tests" {
		t.Fatalf("evaluations=%#v", detail.Evaluations)
	}
}

func timelineHasKind(rows []interface{}, kind string) bool {
	for _, row := range rows {
		m, ok := row.(map[string]interface{})
		if ok && m["kind"] == kind {
			return true
		}
	}
	return false
}

func TestMCPPolicyDisabledDoesNotEvaluateRules(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = false
	cfg.Policies.Rules = []config.PolicyRule{{
		Name:    "disabled-block",
		Scope:   "model",
		Match:   "gpt-5.5",
		Action:  "block",
		Message: "should not run",
	}}
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.get_policy","arguments":{"model":"gpt-5.5"}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["enabled"] != false || payload["action"] != "allow" {
		t.Fatalf("unexpected disabled policy payload: %#v", payload)
	}
	decisions := payload["decisions"].([]interface{})
	if len(decisions) != 0 {
		t.Fatalf("disabled policy returned decisions: %#v", decisions)
	}
}

func TestMCPPolicyAudit(t *testing.T) {
	db := openTestDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.InsertUsage(&storage.UsageRecord{
		Source:       "codex",
		SessionID:    "sess-mcp-policy",
		Model:        "gpt-5.5",
		InputTokens:  100,
		OutputTokens: 25,
		CostUSD:      0.5,
		Timestamp:    ts,
		Project:      "agent-ledger",
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = true
	cfg.Policies.Rules = []config.PolicyRule{{Name: "warn-gpt", Scope: "model", Match: "gpt-5.5", Action: "warn"}}
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.policy_audit","arguments":{"from":"2026-06-07","to":"2026-06-08","limit":10}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["matches"] != float64(1) {
		t.Fatalf("unexpected policy audit payload: %#v", payload)
	}
}

func TestMCPApprovalRoutesToolAndResource(t *testing.T) {
	db := openTestDB(t)
	now := time.Now().UTC()
	if _, err := db.CreateApprovalRequest(storage.ApprovalRequest{
		Source:            "gateway",
		Model:             "gpt-5.5",
		Project:           "private-project",
		Action:            "model.call",
		Target:            "openai-chat-completions",
		Status:            "pending",
		RequiredApprovals: 2,
		ApproverHint:      "desk-lead",
		EscalationTarget:  "research-head",
		DueAt:             now.Add(30 * time.Minute).Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}
	cfg := config.DefaultConfig()
	srv := New(db, cfg)

	out := serveLines(t, srv,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.approval_routes","arguments":{"due_within":"1h","limit":10,"privacy":true}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"agent-ledger://policy/approval-routes?due_within=1h&limit=10&privacy=1"}}`,
	)
	toolPayload := toolTextPayload(t, out[0])
	summary := toolPayload["summary"].(map[string]interface{})
	if summary["pending"] != float64(1) || summary["due_soon"] != float64(1) {
		t.Fatalf("unexpected approval route tool summary: %#v", toolPayload)
	}
	rawTool, _ := json.Marshal(toolPayload)
	for _, forbidden := range []string{"private-project", "desk-lead", "research-head"} {
		if strings.Contains(string(rawTool), forbidden) {
			t.Fatalf("tool approval routes leaked %q: %s", forbidden, string(rawTool))
		}
	}

	resourceText := resourceTextPayload(t, out[1])
	if !strings.Contains(resourceText, `"pending": 1`) || !strings.Contains(resourceText, "redacted") {
		t.Fatalf("unexpected approval route resource: %s", resourceText)
	}
	for _, forbidden := range []string{"private-project", "desk-lead", "research-head"} {
		if strings.Contains(resourceText, forbidden) {
			t.Fatalf("resource approval routes leaked %q: %s", forbidden, resourceText)
		}
	}
}

func TestMCPApprovalsResolveQuorumAndPrivacy(t *testing.T) {
	db := openTestDB(t)
	requestID, err := db.CreateApprovalRequest(storage.ApprovalRequest{
		RequestID:         "apr-private-mcp",
		WorkloadID:        "wl-private-mcp",
		RunID:             "run-private-mcp",
		Source:            "gateway",
		Model:             "gpt-5.5",
		Project:           "private-project",
		Action:            "model.call",
		Target:            "openai-chat-completions",
		Status:            "pending",
		RequiredApprovals: 2,
		ApproverHint:      "desk-lead",
		EscalationTarget:  "research-head",
		Reason:            "private approval reason",
		RequestPayload:    `{"prompt":"do-not-send"}`,
	})
	if err != nil {
		t.Fatalf("CreateApprovalRequest: %v", err)
	}
	cfg := config.DefaultConfig()
	srv := New(db, cfg)

	listToolLine := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.approvals","arguments":{"status":"pending","limit":10,"privacy":true}}}`
	listResourceLine := `{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"agent-ledger://policy/approvals?status=pending&limit=10&privacy=1"}}`
	list := serveLines(t, srv,
		listToolLine,
		listResourceLine,
	)
	listPayload := toolTextPayload(t, list[0])
	rows := listPayload["rows"].([]interface{})
	if len(rows) != 1 {
		t.Fatalf("expected one approval row: %#v", listPayload)
	}
	rawList, _ := json.Marshal(listPayload)
	resourceText := resourceTextPayload(t, list[1])
	for _, forbidden := range []string{"apr-private-mcp", "wl-private-mcp", "run-private-mcp", "private-project", "desk-lead", "research-head", "do-not-send"} {
		if strings.Contains(string(rawList), forbidden) || strings.Contains(resourceText, forbidden) {
			t.Fatalf("approval listing leaked %q: tool=%s resource=%s", forbidden, string(rawList), resourceText)
		}
	}

	firstVoteLine := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"ledger.resolve_approval","arguments":{"request_id":"` + requestID + `","status":"approved","voter":"alice","role":"admin","required_approvals":2,"note":"first approval"}}}`
	firstVote := serveLines(t, srv, firstVoteLine)[0]
	first := toolTextPayload(t, firstVote)["result"].(map[string]interface{})
	if first["status"] != "pending" || first["decided"] != false || first["approval_votes"] != float64(1) {
		t.Fatalf("first vote should not decide quorum: %#v", first)
	}
	secondVoteLine := `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"ledger.resolve_approval","arguments":{"request_id":"` + requestID + `","status":"approved","voter":"bob","role":"admin","required_approvals":2,"note":"second approval"}}}`
	secondVote := serveLines(t, srv, secondVoteLine)[0]
	second := toolTextPayload(t, secondVote)["result"].(map[string]interface{})
	if second["status"] != "approved" || second["decided"] != true || second["approval_votes"] != float64(2) {
		t.Fatalf("second vote should approve quorum: %#v", second)
	}
	allowed, err := db.ApprovalAllowsOperation(storage.ApprovalOperation{RequestID: requestID, Action: "model.call", Target: "openai-chat-completions", Source: "gateway", Model: "gpt-5.5", Project: "private-project"})
	if err != nil {
		t.Fatalf("ApprovalAllowsOperation: %v", err)
	}
	if !allowed {
		t.Fatal("approved MCP quorum should authorize matching operation")
	}
	audit, err := db.QueryAuditLog(storage.AuditLogFilter{Action: "policy.approval", Limit: 10})
	if err != nil {
		t.Fatalf("QueryAuditLog: %v", err)
	}
	rawAudit, _ := json.Marshal(audit)
	if !strings.Contains(string(rawAudit), "policy.approval.approved") || strings.Contains(string(rawAudit), "first approval") || strings.Contains(string(rawAudit), "second approval") {
		t.Fatalf("approval audit missing action or leaked note text: %s", string(rawAudit))
	}
}

func TestMCPPolicyAgentOpsScopes(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.Policies.Enabled = true
	cfg.Policies.Rules = []config.PolicyRule{{Name: "branch-block", Scope: "git_branch", Match: "release", Action: "block"}}
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.get_policy","arguments":{"git_branch":"release/2026w23","action":"model.call","target":"gpt-5.5"}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["action"] != "block" {
		t.Fatalf("unexpected policy payload: %#v", payload)
	}
}

func TestMCPAuditLog(t *testing.T) {
	db := openTestDB(t)
	if err := db.AppendAuditLog("local", "operator", "pricing.sync", "openai", map[string]string{"project": "private"}); err != nil {
		t.Fatal(err)
	}
	cfg := config.DefaultConfig()
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.audit_log","arguments":{"action":"pricing","privacy":true,"limit":10}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["count"] != float64(1) {
		t.Fatalf("unexpected audit payload: %#v", payload)
	}
	rows := payload["rows"].([]interface{})
	row := rows[0].(map[string]interface{})
	if row["target"] != "<redacted>" || row["params"] != "<redacted>" {
		t.Fatalf("audit privacy redaction failed: %#v", row)
	}
}

func TestMCPStartWorkloadIdempotency(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)

	line1 := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.start_workload","arguments":{"goal":"mcp idempotent workload","source":"codex","project":"agent-ledger","agent_name":"codex","command":"codex exec","idempotency_key":"mcp-start-key"}}}`
	line2 := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ledger.start_workload","arguments":{"goal":"mcp idempotent workload","source":"codex","project":"agent-ledger","agent_name":"codex","command":"codex exec","idempotency_key":"mcp-start-key"}}}`
	responses := serveLines(t, srv, line1, line2)
	first := toolTextPayload(t, responses[0])
	second := toolTextPayload(t, responses[1])
	if first["workload_id"] == "" || second["workload_id"] != first["workload_id"] || second["run_id"] != first["run_id"] {
		t.Fatalf("unexpected MCP idempotency payloads: first=%#v second=%#v", first, second)
	}
	if second["idempotent_replay"] != true || second["run_idempotent_replay"] != true {
		t.Fatalf("expected workload and run replay flags: %#v", second)
	}
	now := time.Now().UTC()
	page, err := db.GetWorkloadsPage(now.Add(-time.Hour), now.Add(time.Hour), "", "", "", "", "", 10, 0)
	if err != nil {
		t.Fatalf("GetWorkloadsPage: %v", err)
	}
	if page.Total != 1 {
		t.Fatalf("expected one workload, got %d", page.Total)
	}
	detail, err := db.GetWorkloadDetail(first["workload_id"].(string))
	if err != nil {
		t.Fatalf("GetWorkloadDetail: %v", err)
	}
	if len(detail.Runs) != 1 {
		t.Fatalf("expected one run, got %+v", detail.Runs)
	}
}

func TestMCPWorkloadLeaseLifecycle(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)
	workloadID, err := db.CreateWorkload("mcp lease workload", "codex", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "", 0)
	if err != nil {
		t.Fatalf("CreateWorkload: %v", err)
	}
	claimWorkloadID, err := db.CreateWorkload("mcp claim workload", "opencode", "agent-ledger", "zhenzhis/agent-ledger", "main", "", "infra", 0)
	if err != nil {
		t.Fatalf("CreateWorkload claim: %v", err)
	}
	claimLine, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      0,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "ledger.claim_next_workload",
			"arguments": map[string]interface{}{"holder": "router-claim", "purpose": "private claim note", "source": "opencode", "ttl_seconds": 120},
		},
	})
	claimResponse := serveLines(t, srv, string(claimLine))[0]
	claimed := toolTextPayload(t, claimResponse)
	claimLease := claimed["lease"].(map[string]interface{})
	claimToken, _ := claimLease["lease_token"].(string)
	if claimed["workload_id"] != claimWorkloadID || claimToken == "" {
		t.Fatalf("unexpected claim-next payload: %#v", claimed)
	}

	acquireLine, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "ledger.acquire_workload_lease",
			"arguments": map[string]interface{}{"workload_id": workloadID, "holder": "router-a", "purpose": "private execution note", "ttl_seconds": 120},
		},
	})
	listLine := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"ledger.workload_leases","arguments":{"limit":10}}}`
	responses := serveLines(t, srv, string(acquireLine), listLine)
	acquired := toolTextPayload(t, responses[0])
	lease := acquired["lease"].(map[string]interface{})
	leaseID, _ := lease["lease_id"].(string)
	leaseToken, _ := lease["lease_token"].(string)
	if leaseID == "" || leaseToken == "" || lease["workload_id"] != workloadID {
		t.Fatalf("unexpected acquire payload: %#v", acquired)
	}
	listed := toolTextPayload(t, responses[1])
	rawList, _ := json.Marshal(listed)
	if strings.Contains(string(rawList), leaseToken) || strings.Contains(string(rawList), claimToken) {
		t.Fatalf("lease list leaked token: %s", rawList)
	}
	rows := listed["rows"].([]interface{})
	if len(rows) != 2 {
		t.Fatalf("unexpected lease rows: %#v", listed)
	}

	renewLine, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "ledger.renew_workload_lease",
			"arguments": map[string]interface{}{"lease_id": leaseID, "lease_token": leaseToken, "ttl_seconds": 240},
		},
	})
	releaseLine, _ := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name":      "ledger.release_workload_lease",
			"arguments": map[string]interface{}{"lease_id": leaseID, "lease_token": leaseToken},
		},
	})
	responses = serveLines(t, srv, string(renewLine), string(releaseLine))
	renewed := toolTextPayload(t, responses[0])
	if renewed["lease"].(map[string]interface{})["lease_id"] != leaseID {
		t.Fatalf("unexpected renew payload: %#v", renewed)
	}
	released := toolTextPayload(t, responses[1])
	if released["lease"].(map[string]interface{})["status"] != "released" {
		t.Fatalf("unexpected release payload: %#v", released)
	}
}

func TestMCPRecordEvent(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	srv := New(db, cfg)

	resp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.record_event","arguments":{"event_id":"evt-mcp-record","source":"codex","event_type":"workload.started","payload":{"goal":"mcp event bridge","project":"agent-ledger"}}}}`)[0]
	payload := toolTextPayload(t, resp)
	if payload["status"] != "inserted" || payload["workload_id"] == "" {
		t.Fatalf("unexpected record event payload: %#v", payload)
	}
	detail, err := db.GetWorkloadDetail(payload["workload_id"].(string))
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Summary.Goal != "mcp event bridge" {
		t.Fatalf("summary=%#v", detail.Summary)
	}
}

func TestMCPValidateAndConformanceAreReadOnly(t *testing.T) {
	db := openTestDB(t)
	cfg := config.DefaultConfig()
	cfg.RBAC.ReadOnly = true
	srv := New(db, cfg)

	validateResp := serveLines(t, srv, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"ledger.validate_event","arguments":{"source":"codex","event_type":"workload.started","payload":{"goal":"mcp validate only"}}}}`)[0]
	validatePayload := toolTextPayload(t, validateResp)
	if validatePayload["status"] != "valid_with_warnings" || validatePayload["event_id"] == "" {
		t.Fatalf("unexpected validate payload: %#v", validatePayload)
	}

	request, err := json.Marshal(map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]interface{}{
			"name": "ledger.adapter_conformance",
			"arguments": map[string]interface{}{
				"kind":     "canonical",
				"strict":   true,
				"raw_json": `{"source":"codex","event_type":"workload.started","payload":{"goal":"mcp conformance only"}}`,
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal request: %v", err)
	}
	conformanceResp := serveLines(t, srv, string(request))[0]
	conformancePayload := toolTextPayload(t, conformanceResp)
	if conformancePayload["ok"] != false || conformancePayload["status"] != "fail" {
		t.Fatalf("unexpected conformance payload: %#v", conformancePayload)
	}
	quality, err := db.GetDataQuality(time.Hour)
	if err != nil {
		t.Fatalf("GetDataQuality: %v", err)
	}
	if quality.Provenance == nil || quality.Provenance.Events != 0 {
		t.Fatalf("read-only validation tools wrote events: %#v", quality.Provenance)
	}
}

func openTestDB(t *testing.T) *storage.DB {
	t.Helper()
	dir, err := os.MkdirTemp("", strings.ReplaceAll(t.Name(), "/", "_")+"-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	db, err := storage.Open(filepath.Join(dir, "agent-ledger.db"))
	if err != nil {
		_ = os.RemoveAll(dir)
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("close db: %v", err)
		}
		for i := 0; i < 10; i++ {
			if err := os.RemoveAll(dir); err == nil {
				return
			}
			time.Sleep(time.Duration(i+1) * 25 * time.Millisecond)
		}
		if err := os.RemoveAll(dir); err != nil {
			t.Errorf("RemoveAll %s: %v", dir, err)
		}
	})
	return db
}

func serveLines(t *testing.T, srv *Server, lines ...string) []map[string]interface{} {
	t.Helper()
	responses := serveRawLines(t, srv, lines...)
	for _, resp := range responses {
		if errObj, ok := resp["error"]; ok {
			t.Fatalf("rpc error: %#v", errObj)
		}
	}
	return responses
}

func serveRawLines(t *testing.T, srv *Server, lines ...string) []map[string]interface{} {
	t.Helper()
	var input strings.Builder
	for _, line := range lines {
		input.WriteString(line)
		input.WriteByte('\n')
	}
	var output bytes.Buffer
	if err := srv.Serve(strings.NewReader(input.String()), &output); err != nil {
		t.Fatalf("serve: %v", err)
	}
	scanner := bufio.NewScanner(&output)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var responses []map[string]interface{}
	for scanner.Scan() {
		var resp map[string]interface{}
		if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
			t.Fatalf("decode response %q: %v", scanner.Text(), err)
		}
		responses = append(responses, resp)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan output: %v", err)
	}
	return responses
}

func toolTextPayload(t *testing.T, resp map[string]interface{}) map[string]interface{} {
	t.Helper()
	result := resp["result"].(map[string]interface{})
	content := result["content"].([]interface{})
	text := content[0].(map[string]interface{})["text"].(string)
	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode tool text: %v\n%s", err, text)
	}
	return payload
}

func hasTool(tools []interface{}, name string) bool {
	for _, tool := range tools {
		if tool.(map[string]interface{})["name"] == name {
			return true
		}
	}
	return false
}

func toolByName(t *testing.T, tools []interface{}, name string) map[string]interface{} {
	t.Helper()
	for _, raw := range tools {
		tool := raw.(map[string]interface{})
		if tool["name"] == name {
			return tool
		}
	}
	t.Fatalf("tool %q not found in %#v", name, tools)
	return nil
}

func agentLedgerToolMeta(t *testing.T, tool map[string]interface{}) map[string]interface{} {
	t.Helper()
	meta, ok := tool["_meta"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool missing _meta: %#v", tool)
	}
	ledger, ok := meta["agent_ledger"].(map[string]interface{})
	if !ok {
		t.Fatalf("tool missing agent_ledger metadata: %#v", tool)
	}
	return ledger
}

func resourceTextPayload(t *testing.T, resp map[string]interface{}) string {
	t.Helper()
	result := resp["result"].(map[string]interface{})
	contents := result["contents"].([]interface{})
	return contents[0].(map[string]interface{})["text"].(string)
}

func promptTextPayload(t *testing.T, resp map[string]interface{}) string {
	t.Helper()
	result := resp["result"].(map[string]interface{})
	messages := result["messages"].([]interface{})
	content := messages[0].(map[string]interface{})["content"].(map[string]interface{})
	return content["text"].(string)
}

func hasResource(resources []interface{}, uri string) bool {
	for _, resource := range resources {
		if resource.(map[string]interface{})["uri"] == uri {
			return true
		}
	}
	return false
}

func hasPrompt(prompts []interface{}, name string) bool {
	for _, prompt := range prompts {
		if prompt.(map[string]interface{})["name"] == name {
			return true
		}
	}
	return false
}

func compareStringSet(t *testing.T, label string, want, got []string) {
	t.Helper()
	wantSet := map[string]bool{}
	for _, value := range want {
		if value == "" {
			t.Fatalf("%s contains empty value in want list: %#v", label, want)
		}
		if wantSet[value] {
			t.Fatalf("%s contains duplicate want value %q: %#v", label, value, want)
		}
		wantSet[value] = true
	}
	gotSet := map[string]bool{}
	for _, value := range got {
		if value == "" {
			t.Fatalf("%s contains empty value in got list: %#v", label, got)
		}
		if gotSet[value] {
			t.Fatalf("%s contains duplicate got value %q: %#v", label, value, got)
		}
		gotSet[value] = true
	}
	var missing []string
	for value := range wantSet {
		if !gotSet[value] {
			missing = append(missing, value)
		}
	}
	var extra []string
	for value := range gotSet {
		if !wantSet[value] {
			extra = append(extra, value)
		}
	}
	if len(missing) != 0 || len(extra) != 0 {
		t.Fatalf("%s mismatch: missing=%#v extra=%#v want=%#v got=%#v", label, missing, extra, want, got)
	}
}
