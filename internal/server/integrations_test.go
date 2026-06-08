package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/zhenzhis/agent-ledger/internal/config"
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
		manifest.CanonicalSchemaURI != "/api/event-schema" || manifest.EventExamplesURI != "/api/event-examples" ||
		manifest.AdapterSpecURI != "/api/integrations/adapter-spec" ||
		manifest.AdapterConformanceURI != "/api/integrations/conformance" || manifest.RuntimeStatusURI != "/api/runtime/status" ||
		manifest.CanonicalSchemaHash == "" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if manifest.AdapterSpecHash == "" || manifest.AdapterSpecHash != integrations.AdapterContractFingerprint() {
		t.Fatalf("unexpected adapter contract hash: %+v", manifest)
	}
	if !discoveryHasProtocol(manifest, "protocol.runtime_status") || !discoveryHasProtocol(manifest, "protocol.workload_event_feed") {
		t.Fatalf("missing control-plane protocols: %+v", manifest.Protocols)
	}
	if manifest.PromptContentStored || manifest.UsageDataUploaded {
		t.Fatalf("privacy flags wrong: %+v", manifest)
	}
	assertETagRevalidates(t, srv.handleDiscovery, "http://127.0.0.1/.well-known/agent-ledger.json", rr.Header().Get("ETag"))
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
		{name: "runtime-status", url: "http://127.0.0.1/api/runtime/status", handler: srv.handleRuntimeStatus},
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
