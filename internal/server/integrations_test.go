package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
	if !discoveryHasProtocol(manifest, "protocol.workload_event_feed") {
		t.Fatalf("missing workload feed protocol: %+v", manifest.Protocols)
	}
	if manifest.PromptContentStored || manifest.UsageDataUploaded {
		t.Fatalf("privacy flags wrong: %+v", manifest)
	}
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
	if spec.Contract != "agent-ledger.adapter-contract" || spec.SchemaHash == "" || len(spec.SupportedInputKinds) < 4 {
		t.Fatalf("unexpected adapter spec: %+v", spec)
	}
	if !adapterSpecForbids(spec, "prompt") || !adapterSpecForbids(spec, "messages") {
		t.Fatalf("adapter spec missing privacy forbidden keys: %+v", spec.ForbiddenPayloadKeys)
	}
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
