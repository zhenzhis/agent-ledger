package server

import (
	"bytes"
	"net/http"
	"strconv"

	"github.com/zhenzhis/agent-ledger/internal/integrations"
)

func (s *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, integrations.Registry(s.integrationOptions()))
}

func (s *Server) handleDiscovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, integrations.Discovery(s.integrationOptions()))
}

func (s *Server) handleRuntimeStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, s.runtimeStatus())
}

func (s *Server) handleAdapterSpec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, integrations.AdapterContractSpec())
}

func (s *Server) handleAdapterConformance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.requireLocalOrAuth(w, r) {
		return
	}
	raw := bytes.Buffer{}
	if _, err := raw.ReadFrom(http.MaxBytesReader(w, r.Body, 4<<20)); err != nil {
		badRequest(w, err)
		return
	}
	strict, err := strconv.ParseBool(r.URL.Query().Get("strict"))
	if r.URL.Query().Get("strict") == "" {
		strict = false
		err = nil
	}
	if err != nil {
		badRequest(w, err)
		return
	}
	report, err := integrations.RunAdapterConformanceWithOptions(integrations.AdapterConformanceOptions{
		Kind:   r.URL.Query().Get("kind"),
		Strict: strict,
	}, raw.Bytes())
	if err != nil {
		badRequest(w, err)
		return
	}
	writeJSON(w, report)
}

func (s *Server) integrationOptions() integrations.Options {
	sources := make([]integrations.Source, 0, len(s.options.Sources))
	for _, source := range s.options.Sources {
		sources = append(sources, integrations.Source{
			Source:    source.Source,
			Enabled:   source.Enabled,
			PathCount: len(source.Paths),
		})
	}
	return integrations.Options{
		Sources:             sources,
		PricingMode:         s.options.Pricing.Mode,
		PoliciesEnabled:     s.options.Policies.Enabled,
		RBACEnabled:         s.options.RBAC.Enabled,
		ReadOnly:            s.options.RBAC.ReadOnly,
		QuotaEnabled:        s.options.Quota.Enabled,
		WebhooksEnabled:     s.options.Webhooks.Enabled,
		OTLPReceiverEnabled: s.options.Integrations.OTLPReceiver.Enabled,
		GatewayEnabled:      s.options.Gateway.Enabled,
	}
}
