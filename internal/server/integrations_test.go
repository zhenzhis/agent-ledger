package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/controlplane"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
)

func TestDiscoveryEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{RBAC: config.RBACConfig{Enabled: true}})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/.well-known/agent-ledger.json", nil)
	rr := httptest.NewRecorder()
	srv.handleDiscovery(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("discovery status=%d body=%s", rr.Code, rr.Body.String())
	}
	var manifest integrations.DiscoveryManifest
	if err := json.Unmarshal(rr.Body.Bytes(), &manifest); err != nil {
		t.Fatalf("decode discovery: %v", err)
	}
	if manifest.Contract != "agent-ledger.discovery" || manifest.WellKnownURI != "/.well-known/agent-ledger.json" ||
		manifest.ContractBundleURI != "/api/contracts" ||
		manifest.OpenAPIURI != "/api/openapi.json" ||
		manifest.CanonicalSchemaURI != "/api/event-schema" || manifest.EventExamplesURI != "/api/event-examples" ||
		manifest.AdapterSpecURI != "/api/integrations/adapter-spec" ||
		manifest.AdapterConformanceURI != "/api/integrations/conformance" || manifest.RuntimeStatusURI != "/api/runtime/status" ||
		manifest.CanonicalSchemaHash == "" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if manifest.AdapterSpecHash == "" || manifest.AdapterSpecHash != integrations.AdapterContractFingerprint() {
		t.Fatalf("unexpected adapter contract hash: %+v", manifest)
	}
	if !discoveryHasProtocol(manifest, "protocol.runtime_status") || !discoveryHasProtocol(manifest, "protocol.config_status") || !discoveryHasProtocol(manifest, "protocol.readiness") || !discoveryHasProtocol(manifest, "protocol.admission_check") || !discoveryHasProtocol(manifest, "protocol.workload_event_feed") {
		t.Fatalf("missing control-plane protocols: %+v", manifest.Protocols)
	}
	if manifest.PromptContentStored || manifest.UsageDataUploaded {
		t.Fatalf("privacy flags wrong: %+v", manifest)
	}
	assertETagRevalidates(t, srv.handleDiscovery, "http://127.0.0.1/.well-known/agent-ledger.json", rr.Header().Get("ETag"))
}

func TestContractsEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{RBAC: config.RBACConfig{Enabled: true}})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/contracts", nil)
	rr := httptest.NewRecorder()
	srv.handleContracts(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("contracts status=%d body=%s", rr.Code, rr.Body.String())
	}
	var bundle integrations.ContractBundle
	if err := json.Unmarshal(rr.Body.Bytes(), &bundle); err != nil {
		t.Fatalf("decode contracts: %v", err)
	}
	if bundle.Contract != "agent-ledger.contract-bundle" || bundle.BundleHash == "" || !strings.HasPrefix(bundle.BundleHash, "sha256:") {
		t.Fatalf("unexpected contract bundle: %+v", bundle)
	}
	if !contractBundleHasDocument(bundle, "discovery") || !contractBundleHasDocument(bundle, "openapi") || !contractBundleHasDocument(bundle, "runtime-status") ||
		!contractBundleHasDocument(bundle, "admission-check") || !contractBundleHasDocument(bundle, "canonical-event-schema") || !contractBundleHasDocument(bundle, "adapter-contract") {
		t.Fatalf("contract bundle missing core documents: %+v", bundle.Documents)
	}
	if rr.Header().Get("ETag") != `"`+bundle.BundleHash+`"` {
		t.Fatalf("contracts ETag=%q want %q", rr.Header().Get("ETag"), `"`+bundle.BundleHash+`"`)
	}
	assertETagRevalidates(t, srv.handleContracts, "http://127.0.0.1/api/contracts", rr.Header().Get("ETag"))
}

func TestContractVerificationEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{RBAC: config.RBACConfig{ReadOnly: true}})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/contracts/verify", nil)
	rr := httptest.NewRecorder()
	srv.handleContractVerification(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("contract verification status=%d body=%s", rr.Code, rr.Body.String())
	}
	var report integrations.ContractVerificationReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode contract verification: %v", err)
	}
	if report.Contract != "agent-ledger.contract-verification" || !report.OK || report.Failed != 0 ||
		report.Checked == 0 || report.BundleHash == "" || report.OpenAPIHash == "" || !report.ReadOnly {
		t.Fatalf("unexpected contract verification report: %+v", report)
	}
	if !contractVerificationHasCheck(report, "openapi.path./api/contracts/verify") {
		t.Fatalf("verification report missing OpenAPI path check: %+v", report.Checks)
	}
	assertETagRevalidates(t, srv.handleContractVerification, "http://127.0.0.1/api/contracts/verify", rr.Header().Get("ETag"))
}

func TestOpenAPIEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{RBAC: config.RBACConfig{Enabled: true}})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/openapi.json", nil)
	rr := httptest.NewRecorder()
	srv.handleOpenAPI(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("openapi status=%d body=%s", rr.Code, rr.Body.String())
	}
	var spec map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode openapi: %v", err)
	}
	if spec["openapi"] != "3.1.0" {
		t.Fatalf("unexpected openapi identity: %+v", spec)
	}
	meta := spec["x-agent-ledger"].(map[string]interface{})
	if meta["contract"] != "agent-ledger.control-plane-openapi" || meta["prompt_content_stored"] != false || meta["usage_data_uploaded"] != false {
		t.Fatalf("unexpected openapi metadata: %+v", meta)
	}
	paths := spec["paths"].(map[string]interface{})
	if paths["/api/contracts"] == nil || paths["/api/contracts/verify"] == nil || paths["/api/openapi.json"] == nil || paths["/api/config/status"] == nil || paths["/api/readiness"] == nil || paths["/api/admission/check"] == nil || paths["/api/events/validate"] == nil || paths["/api/workloads"] == nil || paths["/api/workloads/claim-next"] == nil || paths["/api/workloads/queue"] == nil || paths["/api/workloads/lease"] == nil || paths["/api/workloads/lease/renew"] == nil || paths["/api/workloads/lease/release"] == nil || paths["/api/workloads/leases"] == nil || paths["/api/agent-runs"] == nil || paths["/api/agent-runs/liveness"] == nil || paths["/api/workload-events"] == nil {
		t.Fatalf("openapi missing expected paths: %+v", paths)
	}
	assertETagRevalidates(t, srv.handleOpenAPI, "http://127.0.0.1/api/openapi.json", rr.Header().Get("ETag"))
}

func TestConfigStatusEndpointIsPrivacySafe(t *testing.T) {
	db := testServerDB(t)
	cfg := config.DefaultConfig()
	cfg.Server.AuthToken = "secret-auth-token"
	cfg.Server.AdminToken = "secret-admin-token"
	cfg.Collectors.Codex.Paths = []string{"C:/Users/zhang/private/.codex/sessions"}
	cfg.Storage.Path = "C:/Users/zhang/private/agent-ledger.db"
	cfg.Webhooks.Enabled = true
	cfg.Webhooks.URL = "https://hooks.example.test/secret-webhook"
	cfg.Teams.MachineName = "private-machine"
	cfg.Teams.GitAuthor = "private-author"
	srv := New(db, "", Options{ConfigStatus: config.StatusReport(cfg)})

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/config/status", nil)
	rr := httptest.NewRecorder()
	srv.handleConfigStatus(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("config status status=%d body=%s", rr.Code, rr.Body.String())
	}
	var report config.ConfigStatusReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode config status: %v", err)
	}
	if report.Contract != "agent-ledger.config-status" || report.PathValuesExposed || report.SecretValuesExposed ||
		!report.Auth.AnyTokenConfigured || !report.Outbound.WebhookURLConfigured {
		t.Fatalf("unexpected config status: %+v", report)
	}
	body := rr.Body.String()
	for _, forbidden := range []string{"secret-auth-token", "secret-admin-token", "secret-webhook", "C:/Users/zhang/private", "private-machine", "private-author"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("config status leaked %q: %s", forbidden, body)
		}
	}
	assertETagRevalidates(t, srv.handleConfigStatus, "http://127.0.0.1/api/config/status", rr.Header().Get("ETag"))
}

func TestReadinessEndpointIsPrivacySafe(t *testing.T) {
	db := testServerDB(t)
	cfg := config.DefaultConfig()
	cfg.Server.AuthToken = "secret-auth-token"
	cfg.Collectors.Codex.Paths = []string{"C:/Users/zhang/private/.codex/sessions"}
	cfg.Storage.Path = "C:/Users/zhang/private/agent-ledger.db"
	cfg.Webhooks.Enabled = true
	cfg.Webhooks.URL = "https://hooks.example.test/secret-webhook"
	cfg.Teams.MachineName = "private-machine"
	srv := New(db, "", Options{Config: cfg})

	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/readiness", nil)
	rr := httptest.NewRecorder()
	srv.handleReadiness(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("readiness status=%d body=%s", rr.Code, rr.Body.String())
	}
	var report controlplane.ReadinessReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode readiness: %v", err)
	}
	if report.Contract != "agent-ledger.readiness" || report.PromptContentStored || report.UsageDataUploaded {
		t.Fatalf("unexpected readiness: %+v", report)
	}
	body := rr.Body.String()
	for _, forbidden := range []string{"secret-auth-token", "secret-webhook", "C:/Users/zhang/private", "private-machine"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("readiness leaked %q: %s", forbidden, body)
		}
	}
	assertETagRevalidates(t, srv.handleReadiness, "http://127.0.0.1/api/readiness", rr.Header().Get("ETag"))
}

func TestAdmissionEndpointChecksReadOnlyAccess(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{RBAC: config.RBACConfig{Enabled: true, ReadOnly: true}, ViewerToken: "secret-viewer-token"})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/admission/check?surface=http&method=POST&path=/api/events&role=operator", nil)
	rr := httptest.NewRecorder()
	srv.handleAdmissionCheck(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("admission status=%d body=%s", rr.Code, rr.Body.String())
	}
	var decision map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &decision); err != nil {
		t.Fatalf("decode admission: %v", err)
	}
	if decision["contract"] != "agent-ledger.admission-check" || decision["allowed"] != false || decision["status"] != "denied" || decision["read_only"] != true {
		t.Fatalf("unexpected admission decision: %+v", decision)
	}
	if strings.Contains(rr.Body.String(), "secret-viewer-token") {
		t.Fatalf("admission leaked token: %s", rr.Body.String())
	}
	assertETagRevalidates(t, srv.handleAdmissionCheck, "http://127.0.0.1/api/admission/check?surface=http&method=POST&path=/api/events&role=operator", rr.Header().Get("ETag"))
}

func TestAdapterSpecEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/integrations/adapter-spec", nil)
	rr := httptest.NewRecorder()
	srv.handleAdapterSpec(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("adapter spec status=%d body=%s", rr.Code, rr.Body.String())
	}
	var spec integrations.AdapterContract
	if err := json.Unmarshal(rr.Body.Bytes(), &spec); err != nil {
		t.Fatalf("decode adapter spec: %v", err)
	}
	if spec.Contract != "agent-ledger.adapter-contract" || spec.SchemaHash == "" || len(spec.SupportedInputKinds) < 5 {
		t.Fatalf("unexpected adapter spec: %+v", spec)
	}
	if !adapterSpecHasKind(spec, "provider-stream") {
		t.Fatalf("adapter spec missing provider-stream kind: %+v", spec.SupportedInputKinds)
	}
	if !adapterSpecForbids(spec, "prompt") || !adapterSpecForbids(spec, "messages") {
		t.Fatalf("adapter spec missing privacy forbidden keys: %+v", spec.ForbiddenPayloadKeys)
	}
	expectedETag := `"` + integrations.AdapterContractFingerprint() + `"`
	if rr.Header().Get("ETag") != expectedETag {
		t.Fatalf("adapter spec ETag=%q want %q", rr.Header().Get("ETag"), expectedETag)
	}
	assertETagRevalidates(t, srv.handleAdapterSpec, "http://127.0.0.1/api/integrations/adapter-spec", rr.Header().Get("ETag"))
}

func TestControlPlaneEndpointETags(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{
		Sources: []SourceOption{
			{Source: "codex", Enabled: true, Paths: []string{"C:/Users/zhang/.codex"}},
			{Source: "opencode", Enabled: false, Paths: []string{"C:/Users/zhang/.opencode"}},
		},
	})
	cases := []struct {
		name    string
		url     string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "integrations", url: "http://127.0.0.1/api/integrations", handler: srv.handleIntegrations},
		{name: "contracts", url: "http://127.0.0.1/api/contracts", handler: srv.handleContracts},
		{name: "contract-verification", url: "http://127.0.0.1/api/contracts/verify", handler: srv.handleContractVerification},
		{name: "openapi", url: "http://127.0.0.1/api/openapi.json", handler: srv.handleOpenAPI},
		{name: "runtime-status", url: "http://127.0.0.1/api/runtime/status", handler: srv.handleRuntimeStatus},
		{name: "config-status", url: "http://127.0.0.1/api/config/status", handler: srv.handleConfigStatus},
		{name: "readiness", url: "http://127.0.0.1/api/readiness", handler: srv.handleReadiness},
		{name: "admission", url: "http://127.0.0.1/api/admission/check?surface=mcp&tool=ledger.discovery&role=viewer", handler: srv.handleAdmissionCheck},
		{name: "event-schema", url: "http://127.0.0.1/api/event-schema", handler: srv.handleCanonicalEventSchema},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.url, nil)
			rr := httptest.NewRecorder()
			tc.handler(rr, req)
			if rr.Code != http.StatusOK {
				t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
			}
			assertETagRevalidates(t, tc.handler, tc.url, rr.Header().Get("ETag"))
		})
	}
}

func assertETagRevalidates(t *testing.T, handler func(http.ResponseWriter, *http.Request), url, etag string) {
	t.Helper()
	if etag == "" {
		t.Fatalf("missing ETag for %s", url)
	}
	trimmed := strings.Trim(etag, `"`)
	if !strings.HasPrefix(trimmed, "sha256:") {
		t.Fatalf("unexpected ETag for %s: %q", url, etag)
	}
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("If-None-Match", etag)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusNotModified {
		t.Fatalf("revalidation status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr.Body.Len() != 0 {
		t.Fatalf("304 response should not include a body: %q", rr.Body.String())
	}
}

func contractVerificationHasCheck(report integrations.ContractVerificationReport, name string) bool {
	for _, check := range report.Checks {
		if check.Name == name {
			return true
		}
	}
	return false
}

func contractBundleHasDocument(bundle integrations.ContractBundle, id string) bool {
	for _, doc := range bundle.Documents {
		if doc.ID == id {
			return true
		}
	}
	return false
}

func adapterSpecHasKind(spec integrations.AdapterContract, kind string) bool {
	for _, item := range spec.SupportedInputKinds {
		if item.Kind == kind {
			return true
		}
	}
	return false
}

func adapterSpecForbids(spec integrations.AdapterContract, key string) bool {
	for _, value := range spec.ForbiddenPayloadKeys {
		if value == key {
			return true
		}
	}
	return false
}

func discoveryHasProtocol(manifest integrations.DiscoveryManifest, id string) bool {
	for _, protocol := range manifest.Protocols {
		if protocol.ID == id {
			return true
		}
	}
	return false
}
