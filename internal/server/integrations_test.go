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
		manifest.AdapterConformanceURI != "/api/integrations/conformance" || manifest.RuntimeStatusURI != "/api/runtime/status" {
		t.Fatalf("unexpected manifest: %+v", manifest)
	}
	if !discoveryHasProtocol(manifest, "protocol.workload_event_feed") {
		t.Fatalf("missing workload feed protocol: %+v", manifest.Protocols)
	}
	if manifest.PromptContentStored || manifest.UsageDataUploaded {
		t.Fatalf("privacy flags wrong: %+v", manifest)
	}
}

func discoveryHasProtocol(manifest integrations.DiscoveryManifest, id string) bool {
	for _, protocol := range manifest.Protocols {
		if protocol.ID == id {
			return true
		}
	}
	return false
}
