package server

import (
	"net/http"

	"github.com/zhenzhis/agent-ledger/internal/integrations"
)

func (s *Server) handleIntegrations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, integrations.Registry(s.integrationOptions()))
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
		QuotaEnabled:        s.options.Quota.Enabled,
		WebhooksEnabled:     s.options.Webhooks.Enabled,
		OTLPReceiverEnabled: s.options.Integrations.OTLPReceiver.Enabled,
		GatewayEnabled:      s.options.Gateway.Enabled,
	}
}
