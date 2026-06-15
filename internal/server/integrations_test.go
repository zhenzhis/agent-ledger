package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/controlplane"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	"github.com/zhenzhis/agent-ledger/internal/storage"
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
		manifest.ProviderProfilesURI != "/api/provider-profiles" ||
		manifest.AgentProfilesURI != "/api/agent-profiles" ||
		manifest.SignalTaxonomyURI != "/api/signal-taxonomy" ||
		manifest.SignalCoverageURI != "/api/integrations/signal-coverage" ||
		manifest.IntegrationReadinessURI != "/api/integrations/readiness" ||
		manifest.RecommendationURI != "/api/integrations/recommendation" ||
		manifest.CanonicalSchemaURI != "/api/event-schema" || manifest.EventExamplesURI != "/api/event-examples" ||
		manifest.AdapterSpecURI != "/api/integrations/adapter-spec" ||
		manifest.AdapterConformanceURI != "/api/integrations/conformance" ||
		manifest.ConformanceMatrixURI != "/api/integrations/conformance-matrix" ||
		manifest.RuntimeStatusURI != "/api/runtime/status" ||
		manifest.CanonicalSchemaHash == "" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if manifest.AdapterSpecHash == "" || manifest.AdapterSpecHash != integrations.AdapterContractFingerprint() {
		t.Fatalf("unexpected adapter contract hash: %+v", manifest)
	}
	if manifest.ProviderProfilesHash == "" || manifest.ProviderProfilesHash != integrations.ProviderProfilesFingerprint() {
		t.Fatalf("unexpected provider profile hash: %+v", manifest)
	}
	if manifest.AgentProfilesHash == "" || manifest.AgentProfilesHash != integrations.AgentFrameworkProfilesFingerprint() {
		t.Fatalf("unexpected agent profile hash: %+v", manifest)
	}
	if manifest.SignalTaxonomyHash == "" || manifest.SignalTaxonomyHash != integrations.SignalTaxonomyFingerprint() {
		t.Fatalf("unexpected signal taxonomy hash: %+v", manifest)
	}
	if manifest.SignalCoverageHash == "" || manifest.SignalCoverageHash != integrations.SignalCoverageFingerprint() {
		t.Fatalf("unexpected signal coverage hash: %+v", manifest)
	}
	if manifest.IntegrationReadinessHash == "" || manifest.IntegrationReadinessHash != integrations.IntegrationReadinessFingerprint(srv.integrationOptions()) {
		t.Fatalf("unexpected integration readiness hash: %+v", manifest)
	}
	if manifest.RecommendationHash == "" || manifest.RecommendationHash != integrations.IntegrationRecommendationContractFingerprint() {
		t.Fatalf("unexpected recommendation hash: %+v", manifest)
	}
	if manifest.ConformanceMatrixHash == "" || manifest.ConformanceMatrixHash != integrations.AdapterConformanceMatrixFingerprint() {
		t.Fatalf("unexpected conformance matrix hash: %+v", manifest)
	}
	if manifest.A2A.Endpoint != "/api/a2a/tasks" || manifest.A2A.ConformanceKind != "a2a" ||
		manifest.A2A.FullServer || manifest.A2A.MessageContentStored || manifest.A2A.PromptContentStored ||
		!manifest.A2A.SupportsDelegatedLineage || !manifest.A2A.SupportsEvidenceReferences {
		t.Fatalf("unexpected A2A discovery metadata: %+v", manifest.A2A)
	}
	if !discoveryHasProtocol(manifest, "protocol.runtime_status") || !discoveryHasProtocol(manifest, "protocol.config_status") || !discoveryHasProtocol(manifest, "protocol.readiness") || !discoveryHasProtocol(manifest, "protocol.admission_check") || !discoveryHasProtocol(manifest, "protocol.provider_profiles") || !discoveryHasProtocol(manifest, "protocol.agent_profiles") || !discoveryHasProtocol(manifest, "protocol.signal_taxonomy") || !discoveryHasProtocol(manifest, "protocol.signal_coverage") || !discoveryHasProtocol(manifest, "protocol.integration_readiness") || !discoveryHasProtocol(manifest, "protocol.integration_recommendation") || !discoveryHasProtocol(manifest, "protocol.workload_event_feed") {
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
	if !contractBundleHasDocument(bundle, "discovery") || !contractBundleHasDocument(bundle, "goal-coverage") || !contractBundleHasDocument(bundle, "openapi") || !contractBundleHasDocument(bundle, "provider-profiles") || !contractBundleHasDocument(bundle, "agent-profiles") || !contractBundleHasDocument(bundle, "signal-taxonomy") || !contractBundleHasDocument(bundle, "signal-coverage") || !contractBundleHasDocument(bundle, "integration-readiness") || !contractBundleHasDocument(bundle, "integration-recommendation") || !contractBundleHasDocument(bundle, "runtime-status") ||
		!contractBundleHasDocument(bundle, "admission-check") || !contractBundleHasDocument(bundle, "canonical-event-schema") || !contractBundleHasDocument(bundle, "adapter-contract") || !contractBundleHasDocument(bundle, "adapter-conformance-matrix") {
		t.Fatalf("contract bundle missing core documents: %+v", bundle.Documents)
	}
	if rr.Header().Get("ETag") != `"`+bundle.BundleHash+`"` {
		t.Fatalf("contracts ETag=%q want %q", rr.Header().Get("ETag"), `"`+bundle.BundleHash+`"`)
	}
	assertETagRevalidates(t, srv.handleContracts, "http://127.0.0.1/api/contracts", rr.Header().Get("ETag"))
}

func TestProviderProfilesEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/provider-profiles", nil)
	rr := httptest.NewRecorder()
	srv.handleProviderProfiles(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("provider profiles status=%d body=%s", rr.Code, rr.Body.String())
	}
	var catalog integrations.ProviderProfileCatalog
	if err := json.Unmarshal(rr.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("decode provider profiles: %v", err)
	}
	if catalog.Contract != "agent-ledger.provider-profile-catalog" || catalog.Summary.Profiles < 6 || catalog.Summary.LocalRuntimeProfiles < 2 {
		t.Fatalf("unexpected provider profiles: %+v", catalog)
	}
	if !strings.Contains(rr.Body.String(), "openrouter-relay") || !strings.Contains(rr.Body.String(), "ollama-local") {
		t.Fatalf("provider profiles should cover relay and local runtimes: %s", rr.Body.String())
	}
	assertETagRevalidates(t, srv.handleProviderProfiles, "http://127.0.0.1/api/provider-profiles", rr.Header().Get("ETag"))
}

func TestAgentProfilesEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/agent-profiles", nil)
	rr := httptest.NewRecorder()
	srv.handleAgentProfiles(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("agent profiles status=%d body=%s", rr.Code, rr.Body.String())
	}
	var catalog integrations.AgentFrameworkProfileCatalog
	if err := json.Unmarshal(rr.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("decode agent profiles: %v", err)
	}
	if catalog.Contract != "agent-ledger.agent-framework-profile-catalog" || catalog.Summary.Profiles < 10 || catalog.Summary.LocalCollectors < 6 {
		t.Fatalf("unexpected agent profiles: %+v", catalog)
	}
	if !strings.Contains(rr.Body.String(), "codex-cli") || !strings.Contains(rr.Body.String(), "mcp-wrapper") || !strings.Contains(rr.Body.String(), "a2a-task-runtime") {
		t.Fatalf("agent profiles should cover current and future agent surfaces: %s", rr.Body.String())
	}
	assertETagRevalidates(t, srv.handleAgentProfiles, "http://127.0.0.1/api/agent-profiles", rr.Header().Get("ETag"))
}

func TestSignalTaxonomyEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/signal-taxonomy", nil)
	rr := httptest.NewRecorder()
	srv.handleSignalTaxonomy(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("signal taxonomy status=%d body=%s", rr.Code, rr.Body.String())
	}
	var catalog integrations.SignalTaxonomyCatalog
	if err := json.Unmarshal(rr.Body.Bytes(), &catalog); err != nil {
		t.Fatalf("decode signal taxonomy: %v", err)
	}
	if catalog.Contract != "agent-ledger.signal-taxonomy" || !catalog.ReadOnlySafe || catalog.WritesLocalState || catalog.Summary.Signals < 11 {
		t.Fatalf("unexpected signal taxonomy: %+v", catalog)
	}
	if !strings.Contains(rr.Body.String(), "usage.tokens") || !strings.Contains(rr.Body.String(), "workload.identity") {
		t.Fatalf("signal taxonomy should cover usage and workload signals: %s", rr.Body.String())
	}
	assertETagRevalidates(t, srv.handleSignalTaxonomy, "http://127.0.0.1/api/signal-taxonomy", rr.Header().Get("ETag"))
}

func TestSignalCoverageEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/integrations/signal-coverage", nil)
	rr := httptest.NewRecorder()
	srv.handleSignalCoverage(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("signal coverage status=%d body=%s", rr.Code, rr.Body.String())
	}
	var report integrations.SignalCoverageReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode signal coverage: %v", err)
	}
	if report.Contract != "agent-ledger.signal-coverage" || !report.ReadOnlySafe || report.WritesLocalState ||
		report.TaxonomyHash != integrations.SignalTaxonomyFingerprint() || report.Summary.UnknownSignalReferences != 0 ||
		report.Summary.SignalsWithoutAdapterCoverage != 0 {
		t.Fatalf("unexpected signal coverage report: %+v", report)
	}
	if !strings.Contains(rr.Body.String(), "provider-stream") || !strings.Contains(rr.Body.String(), "usage.tokens") {
		t.Fatalf("signal coverage should link adapter kinds and taxonomy ids: %s", rr.Body.String())
	}
	assertETagRevalidates(t, srv.handleSignalCoverage, "http://127.0.0.1/api/integrations/signal-coverage", rr.Header().Get("ETag"))
}

func TestIntegrationReadinessEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{
		Integrations: config.IntegrationsConfig{OTLPReceiver: config.OTLPReceiverConfig{GRPCEnabled: true}},
	})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/integrations/readiness", nil)
	rr := httptest.NewRecorder()
	srv.handleIntegrationReadiness(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("integration readiness status=%d body=%s", rr.Code, rr.Body.String())
	}
	var report integrations.IntegrationReadinessReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode integration readiness: %v", err)
	}
	if report.Contract != "agent-ledger.integration-readiness" || !report.ReadOnlySafe || report.WritesLocalState ||
		report.SignalCoverageHash != integrations.SignalCoverageFingerprint() || !report.Runtime.OTLPReceiverGRPCEnabled ||
		report.Summary.Failures == 0 {
		t.Fatalf("unexpected integration readiness report: %+v", report)
	}
	if !strings.Contains(rr.Body.String(), "otlp.grpc_requires_receiver") || !strings.Contains(rr.Body.String(), "gateway.provider_live_proxy") {
		t.Fatalf("integration readiness should expose activation gates: %s", rr.Body.String())
	}
	assertETagRevalidates(t, srv.handleIntegrationReadiness, "http://127.0.0.1/api/integrations/readiness", rr.Header().Get("ETag"))
}

func TestIntegrationRecommendationEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/integrations/recommendation?agent=codex-cli&provider=openai-official&surface=provider-stream&signals=model,usage,cache", nil)
	rr := httptest.NewRecorder()
	srv.handleIntegrationRecommendation(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("recommendation status=%d body=%s", rr.Code, rr.Body.String())
	}
	var report integrations.IntegrationRecommendationReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode recommendation: %v", err)
	}
	if report.Contract != "agent-ledger.integration-recommendation" || report.RecommendedSurface != "provider-stream" || report.MatchedAgentProfile == nil || report.MatchedAgentProfile.ID != "codex-cli" {
		t.Fatalf("unexpected recommendation: %+v", report)
	}
	if report.Hashes["recommendation_contract"] != integrations.IntegrationRecommendationContractFingerprint() {
		t.Fatalf("missing recommendation contract hash: %+v", report.Hashes)
	}
	assertETagRevalidates(t, srv.handleIntegrationRecommendation, req.URL.String(), rr.Header().Get("ETag"))
}

func TestConformanceMatrixEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/integrations/conformance-matrix", nil)
	rr := httptest.NewRecorder()
	srv.handleConformanceMatrix(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("conformance matrix status=%d body=%s", rr.Code, rr.Body.String())
	}
	var matrix integrations.AdapterConformanceMatrix
	if err := json.Unmarshal(rr.Body.Bytes(), &matrix); err != nil {
		t.Fatalf("decode conformance matrix: %v", err)
	}
	if matrix.Contract != "agent-ledger.adapter-conformance-matrix" || matrix.Summary.InputKinds != len(integrations.SupportedAdapterConformanceKinds()) || matrix.Summary.Fixtures < 10 {
		t.Fatalf("unexpected conformance matrix: %+v", matrix)
	}
	if !strings.Contains(rr.Body.String(), "provider-openai-response.json") || !strings.Contains(rr.Body.String(), "a2a-delegated-task.json") {
		t.Fatalf("matrix missing fixture coverage: %s", rr.Body.String())
	}
	assertETagRevalidates(t, srv.handleConformanceMatrix, "http://127.0.0.1/api/integrations/conformance-matrix", rr.Header().Get("ETag"))
}

func TestGoalCoverageEndpoint(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{RBAC: config.RBACConfig{ReadOnly: true}, Sources: testSourceOptions()})
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/goal-coverage", nil)
	rr := httptest.NewRecorder()
	srv.handleGoalCoverage(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("goal coverage status=%d body=%s", rr.Code, rr.Body.String())
	}
	var report integrations.GoalCoverageReport
	if err := json.Unmarshal(rr.Body.Bytes(), &report); err != nil {
		t.Fatalf("decode goal coverage: %v", err)
	}
	if report.Contract != "agent-ledger.goal-coverage" || report.CoverageHash == "" || report.ContractBundleHash == "" || !report.ReadOnly {
		t.Fatalf("unexpected goal coverage report: %+v", report)
	}
	if report.ProviderProfilesHash != integrations.ProviderProfilesFingerprint() {
		t.Fatalf("unexpected provider profile hash in goal coverage: %+v", report)
	}
	if report.AgentProfilesHash != integrations.AgentFrameworkProfilesFingerprint() {
		t.Fatalf("unexpected agent profile hash in goal coverage: %+v", report)
	}
	if integrations.GoalCoverageHasGap(report) {
		t.Fatalf("goal coverage has gaps: %+v", report.Summary)
	}
	if len(report.Sections) == 0 || report.Summary.TotalSections != len(report.Sections) || len(report.ExternalDependencies) == 0 {
		t.Fatalf("coverage missing sections or external dependency disclosure: %+v", report)
	}
	if strings.Contains(rr.Body.String(), "C:/Users/") || strings.Contains(rr.Body.String(), "secret-viewer-token") {
		t.Fatalf("goal coverage leaked local-sensitive content: %s", rr.Body.String())
	}
	assertETagRevalidates(t, srv.handleGoalCoverage, "http://127.0.0.1/api/goal-coverage", rr.Header().Get("ETag"))
}

func testSourceOptions() []SourceOption {
	return []SourceOption{
		{Source: "claude", Enabled: true, Paths: []string{"~/.claude/projects"}},
		{Source: "codex", Enabled: true, Paths: []string{"~/.codex/sessions"}},
		{Source: "openclaw", Enabled: true, Paths: []string{"~/.openclaw/sessions"}},
		{Source: "opencode", Enabled: true, Paths: []string{"~/.opencode"}},
		{Source: "kiro", Enabled: true, Paths: []string{"~/.local/share/kiro-cli/data.sqlite3"}},
		{Source: "pi", Enabled: true, Paths: []string{"~/.pi/agent/sessions"}},
	}
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
	if !contractVerificationHasCheck(report, "discovery.provider_profiles") || !contractVerificationHasCheck(report, "adapter.provider_profiles") || !contractVerificationHasCheck(report, "openapi.provider_profiles_hash") || !contractVerificationHasCheck(report, "bundle.document.provider-profiles") {
		t.Fatalf("verification report missing provider profile checks: %+v", report.Checks)
	}
	if !contractVerificationHasCheck(report, "discovery.agent_profiles") || !contractVerificationHasCheck(report, "openapi.agent_profiles_hash") || !contractVerificationHasCheck(report, "bundle.document.agent-profiles") {
		t.Fatalf("verification report missing agent profile checks: %+v", report.Checks)
	}
	if !contractVerificationHasCheck(report, "discovery.signal_taxonomy") || !contractVerificationHasCheck(report, "openapi.signal_taxonomy_hash") || !contractVerificationHasCheck(report, "bundle.document.signal-taxonomy") {
		t.Fatalf("verification report missing signal taxonomy checks: %+v", report.Checks)
	}
	if !contractVerificationHasCheck(report, "discovery.signal_coverage") || !contractVerificationHasCheck(report, "adapter.signal_coverage") || !contractVerificationHasCheck(report, "openapi.signal_coverage_hash") || !contractVerificationHasCheck(report, "bundle.document.signal-coverage") {
		t.Fatalf("verification report missing signal coverage checks: %+v", report.Checks)
	}
	if !contractVerificationHasCheck(report, "discovery.integration_readiness") || !contractVerificationHasCheck(report, "openapi.integration_readiness_hash") || !contractVerificationHasCheck(report, "bundle.document.integration-readiness") {
		t.Fatalf("verification report missing integration readiness checks: %+v", report.Checks)
	}
	if !contractVerificationHasCheck(report, "discovery.conformance_matrix") || !contractVerificationHasCheck(report, "adapter.conformance_matrix") || !contractVerificationHasCheck(report, "openapi.conformance_matrix_hash") || !contractVerificationHasCheck(report, "bundle.document.adapter-conformance-matrix") {
		t.Fatalf("verification report missing conformance matrix checks: %+v", report.Checks)
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
	if meta["signal_taxonomy_hash"] != integrations.SignalTaxonomyFingerprint() {
		t.Fatalf("unexpected signal taxonomy hash in openapi metadata: %+v", meta)
	}
	if meta["signal_coverage_hash"] != integrations.SignalCoverageFingerprint() {
		t.Fatalf("unexpected signal coverage hash in openapi metadata: %+v", meta)
	}
	if meta["integration_readiness_hash"] != integrations.IntegrationReadinessFingerprint(srv.integrationOptions()) {
		t.Fatalf("unexpected integration readiness hash in openapi metadata: %+v", meta)
	}
	paths := spec["paths"].(map[string]interface{})
	for _, path := range integrations.OpenAPIContractPaths() {
		if paths[path] == nil {
			t.Fatalf("openapi missing expected path %s: %+v", path, paths)
		}
	}
	assertETagRevalidates(t, srv.handleOpenAPI, "http://127.0.0.1/api/openapi.json", rr.Header().Get("ETag"))
}

func TestAuthenticatedAPIRequiresBearerTokenWhenConfigured(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{ViewerToken: "secret-viewer-token"})
	handler := srv.auth(http.HandlerFunc(srv.handleOpenAPI))

	unauthenticated := httptest.NewRecorder()
	handler.ServeHTTP(unauthenticated, httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/openapi.json", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status=%d body=%s", unauthenticated.Code, unauthenticated.Body.String())
	}
	if strings.Contains(unauthenticated.Body.String(), "secret-viewer-token") {
		t.Fatalf("unauthenticated response leaked token: %s", unauthenticated.Body.String())
	}

	authenticatedReq := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/api/openapi.json", nil)
	authenticatedReq.Header.Set("Authorization", "Bearer secret-viewer-token")
	authenticated := httptest.NewRecorder()
	handler.ServeHTTP(authenticated, authenticatedReq)
	if authenticated.Code != http.StatusOK {
		t.Fatalf("authenticated status=%d body=%s", authenticated.Code, authenticated.Body.String())
	}
}

func TestOpenAPIContractPathsMatchRegisteredRoutes(t *testing.T) {
	raw, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatalf("read server.go: %v", err)
	}
	registered := map[string]bool{}
	matches := regexp.MustCompile(`mux\.HandleFunc\("([^"]+)"`).FindAllStringSubmatch(string(raw), -1)
	for _, match := range matches {
		registered[match[1]] = true
	}
	if len(registered) == 0 {
		t.Fatal("no registered routes found in server.go")
	}
	contracted := map[string]bool{}
	for _, path := range integrations.OpenAPIContractPaths() {
		contracted[path] = true
	}
	var missing []string
	for path := range registered {
		if !contracted[path] {
			missing = append(missing, path)
		}
	}
	var stale []string
	for path := range contracted {
		if !registered[path] {
			stale = append(stale, path)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("OpenAPI route drift: missing=%v stale=%v", missing, stale)
	}
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
	if spec.ProviderProfilesURI != "/api/provider-profiles" || spec.ProviderProfilesHash != integrations.ProviderProfilesFingerprint() {
		t.Fatalf("adapter spec missing provider profile catalog link: %+v", spec)
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

func TestOversizedRequestBodyReturns413JSON(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/integrations/conformance", strings.NewReader(strings.Repeat("x", (4<<20)+1)))
	rr := httptest.NewRecorder()
	srv.handleAdapterConformance(rr, req)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	if got := rr.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("expected JSON error content type, got %q", got)
	}
	if !strings.Contains(rr.Body.String(), "request body too large") {
		t.Fatalf("expected body size error, got %s", rr.Body.String())
	}
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
		{name: "provider-profiles", url: "http://127.0.0.1/api/provider-profiles", handler: srv.handleProviderProfiles},
		{name: "agent-profiles", url: "http://127.0.0.1/api/agent-profiles", handler: srv.handleAgentProfiles},
		{name: "signal-taxonomy", url: "http://127.0.0.1/api/signal-taxonomy", handler: srv.handleSignalTaxonomy},
		{name: "signal-coverage", url: "http://127.0.0.1/api/integrations/signal-coverage", handler: srv.handleSignalCoverage},
		{name: "integration-readiness", url: "http://127.0.0.1/api/integrations/readiness", handler: srv.handleIntegrationReadiness},
		{name: "integration-recommendation", url: "http://127.0.0.1/api/integrations/recommendation?agent=codex-cli&provider=openai-official&surface=provider-stream", handler: srv.handleIntegrationRecommendation},
		{name: "conformance-matrix", url: "http://127.0.0.1/api/integrations/conformance-matrix", handler: srv.handleConformanceMatrix},
		{name: "goal-coverage", url: "http://127.0.0.1/api/goal-coverage", handler: srv.handleGoalCoverage},
		{name: "contracts", url: "http://127.0.0.1/api/contracts", handler: srv.handleContracts},
		{name: "contract-verification", url: "http://127.0.0.1/api/contracts/verify", handler: srv.handleContractVerification},
		{name: "openapi", url: "http://127.0.0.1/api/openapi.json", handler: srv.handleOpenAPI},
		{name: "runtime-status", url: "http://127.0.0.1/api/runtime/status", handler: srv.handleRuntimeStatus},
		{name: "config-status", url: "http://127.0.0.1/api/config/status", handler: srv.handleConfigStatus},
		{name: "readiness", url: "http://127.0.0.1/api/readiness", handler: srv.handleReadiness},
		{name: "admission", url: "http://127.0.0.1/api/admission/check?surface=mcp&tool=ledger.discovery&role=viewer", handler: srv.handleAdmissionCheck},
		{name: "event-schema", url: "http://127.0.0.1/api/event-schema", handler: srv.handleCanonicalEventSchema},
		{name: "event-examples", url: "http://127.0.0.1/api/event-examples?type=model.call", handler: srv.handleCanonicalEventExamples},
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

func TestReadJSONEndpointsRejectNonGET(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	cases := []struct {
		name    string
		url     string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "stats", url: "http://127.0.0.1/api/stats?from=2026-06-07&to=2026-06-08", handler: srv.handleStats},
		{name: "dashboard", url: "http://127.0.0.1/api/dashboard?from=2026-06-07&to=2026-06-08", handler: srv.handleDashboard},
		{name: "cost-by-model", url: "http://127.0.0.1/api/cost-by-model?from=2026-06-07&to=2026-06-08", handler: srv.handleCostByModel},
		{name: "cost-over-time", url: "http://127.0.0.1/api/cost-over-time?from=2026-06-07&to=2026-06-08", handler: srv.handleCostOverTime},
		{name: "tokens-over-time", url: "http://127.0.0.1/api/tokens-over-time?from=2026-06-07&to=2026-06-08", handler: srv.handleTokensOverTime},
		{name: "sessions", url: "http://127.0.0.1/api/sessions?from=2026-06-07&to=2026-06-08", handler: srv.handleSessions},
		{name: "session-detail", url: "http://127.0.0.1/api/session-detail?source=codex&session_id=test", handler: srv.handleSessionDetail},
		{name: "session-replay", url: "http://127.0.0.1/api/session-replay?source=codex&session_id=test", handler: srv.handleSessionReplay},
		{name: "pricing-audit", url: "http://127.0.0.1/api/pricing/audit", handler: srv.handlePricingAudit},
		{name: "data-quality", url: "http://127.0.0.1/api/data-quality", handler: srv.handleDataQuality},
		{name: "model-calls", url: "http://127.0.0.1/api/model-calls?from=2026-06-07&to=2026-06-08", handler: srv.handleModelCalls},
		{name: "cost-intelligence", url: "http://127.0.0.1/api/cost-intelligence?from=2026-06-07&to=2026-06-08", handler: srv.handleCostIntelligence},
		{name: "cache-doctor", url: "http://127.0.0.1/api/cache/doctor?from=2026-06-07&to=2026-06-08", handler: srv.handleCacheDoctor},
		{name: "quota-status", url: "http://127.0.0.1/api/quota/status", handler: srv.handleQuotaStatus},
		{name: "anomalies", url: "http://127.0.0.1/api/anomalies?from=2026-06-07&to=2026-06-08", handler: srv.handleAnomalies},
		{name: "watchdog", url: "http://127.0.0.1/api/watchdog/events?from=2026-06-07&to=2026-06-08", handler: srv.handleWatchdogEvents},
		{name: "audit-log", url: "http://127.0.0.1/api/audit-log", handler: srv.handleAuditLog},
		{name: "reconciliation-status", url: "http://127.0.0.1/api/reconciliation/status", handler: srv.handleReconciliationStatus},
		{name: "policy-status", url: "http://127.0.0.1/api/policies/status", handler: srv.handlePolicyStatus},
		{name: "policy-audit", url: "http://127.0.0.1/api/policy/audit?from=2026-06-07&to=2026-06-08", handler: srv.handlePolicyAudit},
		{name: "policy-enforcement", url: "http://127.0.0.1/api/policy/enforcement", handler: srv.handlePolicyEnforcement},
		{name: "policy-approval-routes", url: "http://127.0.0.1/api/policy/approval-routes", handler: srv.handlePolicyApprovalRoutes},
		{name: "workload-events", url: "http://127.0.0.1/api/workload-events?from=2026-06-07&to=2026-06-08", handler: srv.handleWorkloadEvents},
		{name: "fleet-attribution", url: "http://127.0.0.1/api/fleet-attribution?from=2026-06-07&to=2026-06-08", handler: srv.handleFleetAttribution},
		{name: "model-registry", url: "http://127.0.0.1/api/model-registry", handler: srv.handleModelRegistry},
		{name: "policy-decisions", url: "http://127.0.0.1/api/policy/decisions", handler: srv.handlePolicyDecisions},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			tc.handler(rr, httptest.NewRequest(http.MethodPost, tc.url, nil))
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("POST status=%d body=%s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("Allow header=%q", rr.Header().Get("Allow"))
			}
		})
	}
}

func TestControlPlaneEndpointsRejectNonGET(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	cases := []struct {
		name    string
		url     string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "integrations", url: "http://127.0.0.1/api/integrations", handler: srv.handleIntegrations},
		{name: "provider-profiles", url: "http://127.0.0.1/api/provider-profiles", handler: srv.handleProviderProfiles},
		{name: "agent-profiles", url: "http://127.0.0.1/api/agent-profiles", handler: srv.handleAgentProfiles},
		{name: "signal-taxonomy", url: "http://127.0.0.1/api/signal-taxonomy", handler: srv.handleSignalTaxonomy},
		{name: "signal-coverage", url: "http://127.0.0.1/api/integrations/signal-coverage", handler: srv.handleSignalCoverage},
		{name: "integration-readiness", url: "http://127.0.0.1/api/integrations/readiness", handler: srv.handleIntegrationReadiness},
		{name: "integration-recommendation", url: "http://127.0.0.1/api/integrations/recommendation?agent=codex-cli&provider=openai-official&surface=provider-stream", handler: srv.handleIntegrationRecommendation},
		{name: "conformance-matrix", url: "http://127.0.0.1/api/integrations/conformance-matrix", handler: srv.handleConformanceMatrix},
		{name: "discovery", url: "http://127.0.0.1/api/discovery", handler: srv.handleDiscovery},
		{name: "contracts", url: "http://127.0.0.1/api/contracts", handler: srv.handleContracts},
		{name: "contract-verification", url: "http://127.0.0.1/api/contracts/verify", handler: srv.handleContractVerification},
		{name: "openapi", url: "http://127.0.0.1/api/openapi.json", handler: srv.handleOpenAPI},
		{name: "runtime-status", url: "http://127.0.0.1/api/runtime/status", handler: srv.handleRuntimeStatus},
		{name: "config-status", url: "http://127.0.0.1/api/config/status", handler: srv.handleConfigStatus},
		{name: "readiness", url: "http://127.0.0.1/api/readiness", handler: srv.handleReadiness},
		{name: "admission", url: "http://127.0.0.1/api/admission/check?surface=http&method=GET&path=/api/stats&role=viewer", handler: srv.handleAdmissionCheck},
		{name: "adapter-spec", url: "http://127.0.0.1/api/integrations/adapter-spec", handler: srv.handleAdapterSpec},
		{name: "event-schema", url: "http://127.0.0.1/api/event-schema", handler: srv.handleCanonicalEventSchema},
		{name: "event-examples", url: "http://127.0.0.1/api/event-examples?type=model.call", handler: srv.handleCanonicalEventExamples},
		{name: "desktop-notification", url: "http://127.0.0.1/api/notifications/desktop", handler: srv.handleDesktopNotificationPayload},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			tc.handler(rr, httptest.NewRequest(http.MethodPost, tc.url, nil))
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("POST status=%d body=%s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("Allow header=%q", rr.Header().Get("Allow"))
			}
		})
	}
}

func TestDownloadReportEndpointsRejectNonGET(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	cases := []struct {
		name    string
		url     string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "export", url: "http://127.0.0.1/api/export?from=2026-06-07&to=2026-06-08&type=sessions&format=json", handler: srv.handleExport},
		{name: "report", url: "http://127.0.0.1/api/report?from=2026-06-07&to=2026-06-08", handler: srv.handleReport},
		{name: "evidence-bundle", url: "http://127.0.0.1/api/evidence-bundle?from=2026-06-07&to=2026-06-08", handler: srv.handleEvidenceBundle},
		{name: "offline-bundle-export", url: "http://127.0.0.1/api/offline-bundle/export?from=2026-06-07&to=2026-06-08", handler: srv.handleOfflineBundleExport},
		{name: "repo-badge", url: "http://127.0.0.1/api/badge/repo.svg?from=2026-06-07&to=2026-06-08", handler: srv.handleRepoBadge},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			tc.handler(rr, httptest.NewRequest(http.MethodPost, tc.url, nil))
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("POST status=%d body=%s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Allow") != http.MethodGet {
				t.Fatalf("Allow header=%q", rr.Header().Get("Allow"))
			}
		})
	}
}

func TestWriteAndMixedEndpointsRejectUnsupportedMethods(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	postOnly := []struct {
		name    string
		url     string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "adapter-conformance", url: "http://127.0.0.1/api/integrations/conformance", handler: srv.handleAdapterConformance},
		{name: "pricing-sync", url: "http://127.0.0.1/api/pricing/sync", handler: srv.handlePricingSync},
		{name: "pricing-recalculate", url: "http://127.0.0.1/api/pricing/recalculate", handler: srv.handlePricingRecalculate},
		{name: "reconciliation-import", url: "http://127.0.0.1/api/reconciliation/import", handler: srv.handleReconciliationImport},
		{name: "scan", url: "http://127.0.0.1/api/scan", handler: srv.handleScan},
		{name: "recalculate-costs", url: "http://127.0.0.1/api/recalculate-costs", handler: srv.handleRecalculateCosts},
		{name: "repair-projections", url: "http://127.0.0.1/api/projections/repair", handler: srv.handleRepairProjections},
		{name: "offline-bundle-import", url: "http://127.0.0.1/api/offline-bundle/import", handler: srv.handleOfflineBundleImport},
		{name: "webhook-notification", url: "http://127.0.0.1/api/notifications/webhook", handler: srv.handleWebhookNotification},
		{name: "policy-evaluate", url: "http://127.0.0.1/api/policy/evaluate", handler: srv.handlePolicyEvaluate},
		{name: "canonical-events", url: "http://127.0.0.1/api/events", handler: srv.handleCanonicalEvents},
		{name: "canonical-event-validate", url: "http://127.0.0.1/api/events/validate", handler: srv.handleCanonicalEventValidate},
		{name: "otel-genai", url: "http://127.0.0.1/api/otel/genai", handler: srv.handleOTelGenAI},
		{name: "otlp-traces", url: "http://127.0.0.1/api/otlp/v1/traces", handler: srv.handleOTLPTraces},
		{name: "provider-calls", url: "http://127.0.0.1/api/provider/calls", handler: srv.handleProviderCalls},
		{name: "a2a-tasks", url: "http://127.0.0.1/api/a2a/tasks", handler: srv.handleA2ATasks},
		{name: "openai-chat-gateway", url: "http://127.0.0.1/gateway/openai/v1/chat/completions", handler: srv.handleOpenAIChatGateway},
		{name: "openai-responses-gateway", url: "http://127.0.0.1/gateway/openai/v1/responses", handler: srv.handleOpenAIResponsesGateway},
		{name: "anthropic-gateway", url: "http://127.0.0.1/gateway/anthropic/v1/messages", handler: srv.handleAnthropicMessagesGateway},
		{name: "workload-close", url: "http://127.0.0.1/api/workloads/close", handler: srv.handleWorkloadClose},
		{name: "workload-link", url: "http://127.0.0.1/api/workloads/link", handler: srv.handleWorkloadLink},
		{name: "workload-claim-next", url: "http://127.0.0.1/api/workloads/claim-next", handler: srv.handleWorkloadClaimNext},
		{name: "workload-lease", url: "http://127.0.0.1/api/workloads/lease", handler: srv.handleWorkloadLeaseAcquire},
		{name: "workload-lease-renew", url: "http://127.0.0.1/api/workloads/lease/renew", handler: srv.handleWorkloadLeaseRenew},
		{name: "workload-lease-release", url: "http://127.0.0.1/api/workloads/lease/release", handler: srv.handleWorkloadLeaseRelease},
		{name: "agent-runs", url: "http://127.0.0.1/api/agent-runs", handler: srv.handleAgentRuns},
		{name: "agent-run-heartbeat", url: "http://127.0.0.1/api/agent-runs/heartbeat", handler: srv.handleAgentRunHeartbeat},
	}
	for _, tc := range postOnly {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			tc.handler(rr, httptest.NewRequest(http.MethodGet, tc.url, nil))
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("GET status=%d body=%s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Allow") != http.MethodPost {
				t.Fatalf("Allow header=%q", rr.Header().Get("Allow"))
			}
		})
	}

	mixed := []struct {
		name    string
		url     string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "workloads", url: "http://127.0.0.1/api/workloads", handler: srv.handleWorkloads},
		{name: "policy-approvals", url: "http://127.0.0.1/api/policy/approvals", handler: srv.handlePolicyApprovals},
	}
	for _, tc := range mixed {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			tc.handler(rr, httptest.NewRequest(http.MethodPut, tc.url, nil))
			if rr.Code != http.StatusMethodNotAllowed {
				t.Fatalf("PUT status=%d body=%s", rr.Code, rr.Body.String())
			}
			if rr.Header().Get("Allow") != "GET, POST" {
				t.Fatalf("Allow header=%q", rr.Header().Get("Allow"))
			}
		})
	}
}

func TestFinOpsDiagnosticsEndpointETags(t *testing.T) {
	db := testServerDB(t)
	srv := New(db, "", Options{})
	cases := []struct {
		name    string
		url     string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "ingestion-health", url: "http://127.0.0.1/api/health/ingestion", handler: srv.handleIngestionHealth},
		{name: "pricing-status", url: "http://127.0.0.1/api/pricing/status", handler: srv.handlePricingStatus},
		{name: "pricing-audit", url: "http://127.0.0.1/api/pricing/audit", handler: srv.handlePricingAudit},
		{name: "budget-status", url: "http://127.0.0.1/api/budgets/status", handler: srv.handleBudgetStatus},
		{name: "data-quality", url: "http://127.0.0.1/api/data-quality", handler: srv.handleDataQuality},
		{name: "doctor", url: "http://127.0.0.1/api/doctor?from=2026-06-07&to=2026-06-08", handler: srv.handleDoctor},
		{name: "model-calls", url: "http://127.0.0.1/api/model-calls?from=2026-06-07&to=2026-06-08", handler: srv.handleModelCalls},
		{name: "model-registry", url: "http://127.0.0.1/api/model-registry", handler: srv.handleModelRegistry},
		{name: "cost-intelligence", url: "http://127.0.0.1/api/cost-intelligence?from=2026-06-07&to=2026-06-08", handler: srv.handleCostIntelligence},
		{name: "cache-doctor", url: "http://127.0.0.1/api/cache/doctor?from=2026-06-07&to=2026-06-08", handler: srv.handleCacheDoctor},
		{name: "anomalies", url: "http://127.0.0.1/api/anomalies?from=2026-06-07&to=2026-06-08", handler: srv.handleAnomalies},
		{name: "watchdog", url: "http://127.0.0.1/api/watchdog/events?from=2026-06-07&to=2026-06-08", handler: srv.handleWatchdogEvents},
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

func TestDashboardSessionEndpointETags(t *testing.T) {
	db := testServerDB(t)
	ts := time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)
	if err := db.UpsertSession(&storage.SessionRecord{
		Source:    "codex",
		SessionID: "dash-session",
		Project:   "agent-ledger",
		CWD:       "/workspace/agent-ledger",
		StartTime: ts,
	}); err != nil {
		t.Fatalf("UpsertSession: %v", err)
	}
	if err := db.InsertUsage(&storage.UsageRecord{
		Source:       "codex",
		SessionID:    "dash-session",
		Model:        "gpt-5",
		InputTokens:  100,
		OutputTokens: 50,
		Project:      "agent-ledger",
		Timestamp:    ts,
		CostUSD:      0.01,
	}); err != nil {
		t.Fatalf("InsertUsage: %v", err)
	}
	srv := New(db, "", Options{})
	cases := []struct {
		name    string
		url     string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "stats", url: "http://127.0.0.1/api/stats?from=2026-06-07&to=2026-06-08", handler: srv.handleStats},
		{name: "dashboard", url: "http://127.0.0.1/api/dashboard?from=2026-06-07&to=2026-06-08", handler: srv.handleDashboard},
		{name: "cost-by-model", url: "http://127.0.0.1/api/cost-by-model?from=2026-06-07&to=2026-06-08", handler: srv.handleCostByModel},
		{name: "cost-over-time", url: "http://127.0.0.1/api/cost-over-time?from=2026-06-07&to=2026-06-08", handler: srv.handleCostOverTime},
		{name: "tokens-over-time", url: "http://127.0.0.1/api/tokens-over-time?from=2026-06-07&to=2026-06-08", handler: srv.handleTokensOverTime},
		{name: "sessions", url: "http://127.0.0.1/api/sessions?from=2026-06-07&to=2026-06-08&limit=100", handler: srv.handleSessions},
		{name: "session-detail", url: "http://127.0.0.1/api/session-detail?source=codex&session_id=dash-session", handler: srv.handleSessionDetail},
		{name: "session-replay", url: "http://127.0.0.1/api/session-replay?source=codex&session_id=dash-session", handler: srv.handleSessionReplay},
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

func TestGovernanceTeamEndpointETags(t *testing.T) {
	db := testServerDB(t)
	if err := db.UpsertPricing("gpt-5-mini", 0.0000001, 0.0000002, 0.00000001, 0.00000005); err != nil {
		t.Fatalf("UpsertPricing: %v", err)
	}
	srv := New(db, "", Options{})
	cases := []struct {
		name    string
		url     string
		handler func(http.ResponseWriter, *http.Request)
	}{
		{name: "fleet-attribution", url: "http://127.0.0.1/api/fleet-attribution?from=2026-06-07&to=2026-06-08", handler: srv.handleFleetAttribution},
		{name: "audit-log", url: "http://127.0.0.1/api/audit-log?from=2026-06-07&to=2026-06-08", handler: srv.handleAuditLog},
		{name: "reconciliation-status", url: "http://127.0.0.1/api/reconciliation/status", handler: srv.handleReconciliationStatus},
		{name: "router-simulate", url: "http://127.0.0.1/api/router/simulate?from=2026-06-07&to=2026-06-08&to_model=gpt-5-mini", handler: srv.handleRouterSimulation},
		{name: "preflight-estimate", url: "http://127.0.0.1/api/preflight/estimate?from=2026-06-07&to=2026-06-08&task=debug", handler: srv.handlePreflightEstimate},
		{name: "chargeback", url: "http://127.0.0.1/api/chargeback?from=2026-06-07&to=2026-06-08", handler: srv.handleChargeback},
		{name: "wrapped-json", url: "http://127.0.0.1/api/wrapped?from=2026-06-07&to=2026-06-08&period=custom", handler: srv.handleWrapped},
		{name: "policy-status", url: "http://127.0.0.1/api/policies/status", handler: srv.handlePolicyStatus},
		{name: "policy-audit", url: "http://127.0.0.1/api/policy/audit?from=2026-06-07&to=2026-06-08", handler: srv.handlePolicyAudit},
		{name: "policy-enforcement", url: "http://127.0.0.1/api/policy/enforcement", handler: srv.handlePolicyEnforcement},
		{name: "policy-decisions", url: "http://127.0.0.1/api/policy/decisions", handler: srv.handlePolicyDecisions},
		{name: "policy-approvals", url: "http://127.0.0.1/api/policy/approvals", handler: srv.handlePolicyApprovals},
		{name: "policy-approval-routes", url: "http://127.0.0.1/api/policy/approval-routes", handler: srv.handlePolicyApprovalRoutes},
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
