package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// ProviderProfileCatalog is a machine-readable provider/model ecosystem matrix.
// It is metadata-only and intended for wrappers, routers, and CI checks that
// need to know how to map provider usage into Agent Ledger without prompt data.
type ProviderProfileCatalog struct {
	Product         string            `json:"product"`
	Contract        string            `json:"contract"`
	Version         string            `json:"version"`
	GeneratedFrom   string            `json:"generated_from"`
	LocalFirst      bool              `json:"local_first"`
	PrivacyPolicy   string            `json:"privacy_policy"`
	Summary         ProviderSummary   `json:"summary"`
	Profiles        []ProviderProfile `json:"profiles"`
	QualityGates    []string          `json:"quality_gates"`
	RoutingGuidance []string          `json:"routing_guidance"`
}

// ProviderSummary captures high-level ecosystem coverage counts.
type ProviderSummary struct {
	Profiles              int `json:"profiles"`
	GatewayProfiles       int `json:"gateway_profiles"`
	LocalRuntimeProfiles  int `json:"local_runtime_profiles"`
	EdgeRuntimeProfiles   int `json:"edge_runtime_profiles"`
	OpenAICompatible      int `json:"openai_compatible"`
	AnthropicCompatible   int `json:"anthropic_compatible"`
	UsageMetadataProfiles int `json:"usage_metadata_profiles"`
}

// ProviderProfile describes one provider/runtime family without any API credentials,
// endpoints containing secrets, local paths, prompts, or response content.
type ProviderProfile struct {
	ID                    string   `json:"id"`
	Label                 string   `json:"label"`
	Kind                  string   `json:"kind"`
	Families              []string `json:"families"`
	ModelNameExamples     []string `json:"model_name_examples"`
	UsageSchemas          []string `json:"usage_schemas"`
	AcceptedInputKinds    []string `json:"accepted_input_kinds"`
	GatewayRoutes         []string `json:"gateway_routes,omitempty"`
	RecommendedSource     string   `json:"recommended_source"`
	PricingStrategy       string   `json:"pricing_strategy"`
	ReconciliationSupport string   `json:"reconciliation_support"`
	PrivacyNotes          []string `json:"privacy_notes"`
	ConformanceFixtures   []string `json:"conformance_fixtures,omitempty"`
	Limitations           []string `json:"limitations,omitempty"`
}

// ProviderProfiles returns the stable provider profile catalog for current and
// future provider, relay, open-source, and edge model integrations.
func ProviderProfiles() ProviderProfileCatalog {
	profiles := []ProviderProfile{
		{
			ID:                    "openai-official",
			Label:                 "OpenAI official API",
			Kind:                  "commercial-provider",
			Families:              []string{"openai", "openai-compatible"},
			ModelNameExamples:     []string{"gpt-5", "gpt-5-mini", "o-series"},
			UsageSchemas:          []string{"openai-responses-or-anthropic", "openai-chat-completions"},
			AcceptedInputKinds:    []string{"provider", "provider-stream", "otel"},
			GatewayRoutes:         []string{"/gateway/openai/v1/chat/completions", "/gateway/openai/v1/responses"},
			RecommendedSource:     "gateway or provider",
			PricingStrategy:       "official OpenAI seed first, LiteLLM fallback, local override for contract pricing",
			ReconciliationSupport: "provider CSV/JSON summary import with statement hashes",
			PrivacyNotes:          []string{"request and response bodies are ignored by provider ingest", "gateway forwards prompts in memory only when explicitly enabled", "API credentials are read from environment variables and not persisted"},
			ConformanceFixtures:   []string{"examples/adapter-fixtures/provider-openai-response.json", "examples/adapter-fixtures/provider-openai-chat-completion.json", "examples/adapter-fixtures/provider-openai-chat-stream.sse", "examples/adapter-fixtures/provider-openai-responses-stream.sse"},
		},
		{
			ID:                    "anthropic-official",
			Label:                 "Anthropic official API",
			Kind:                  "commercial-provider",
			Families:              []string{"anthropic"},
			ModelNameExamples:     []string{"claude-opus-4-7", "claude-sonnet-4-5"},
			UsageSchemas:          []string{"anthropic", "openai-responses-or-anthropic"},
			AcceptedInputKinds:    []string{"provider", "provider-stream", "otel"},
			GatewayRoutes:         []string{"/gateway/anthropic/v1/messages"},
			RecommendedSource:     "gateway or provider",
			PricingStrategy:       "official Anthropic seed first, LiteLLM fallback, local override for contract pricing",
			ReconciliationSupport: "provider CSV/JSON summary import with statement hashes",
			PrivacyNotes:          []string{"message content is ignored by provider ingest", "gateway forwards prompts in memory only when explicitly enabled", "cache read/write tokens are preserved as non-overlapping input components"},
			ConformanceFixtures:   []string{"examples/adapter-fixtures/provider-anthropic-message.json", "examples/adapter-fixtures/provider-anthropic-message-stream.sse"},
		},
		{
			ID:                    "openrouter-relay",
			Label:                 "OpenRouter or OpenAI-compatible relay",
			Kind:                  "relay-provider",
			Families:              []string{"openai-compatible", "relay", "openrouter"},
			ModelNameExamples:     []string{"openai/gpt-5", "anthropic/claude-sonnet-4-5", "qwen/qwen3-coder"},
			UsageSchemas:          []string{"openai-chat-completions", "usage-metadata", "generic"},
			AcceptedInputKinds:    []string{"provider", "provider-stream"},
			GatewayRoutes:         []string{"/gateway/openai/v1/chat/completions"},
			RecommendedSource:     "provider",
			PricingStrategy:       "local override for relay contract price, then LiteLLM fallback; source-reported cost is retained as evidence but not authoritative for recalculation",
			ReconciliationSupport: "manual or relay statement import with hashed invoice/bill references",
			PrivacyNotes:          []string{"relay-specific headers are not persisted", "provider account and organization ids must be hashed before persistence", "model aliases should preserve the relay prefix for attribution"},
			ConformanceFixtures:   []string{"examples/adapter-fixtures/provider-openai-chat-completion.json", "examples/adapter-fixtures/provider-openai-chat-stream.sse", "examples/adapter-fixtures/provider-generic-usage-metadata-stream.sse"},
			Limitations:           []string{"relay model pricing may differ from official provider list; configure local overrides for exact billing"},
		},
		{
			ID:                    "litellm-proxy",
			Label:                 "LiteLLM proxy or compatible gateway",
			Kind:                  "relay-provider",
			Families:              []string{"litellm", "openai-compatible", "usage-metadata"},
			ModelNameExamples:     []string{"gpt-5-mini", "claude-sonnet-4-5", "gemini-2.5-pro"},
			UsageSchemas:          []string{"openai-chat-completions", "usage-metadata", "generic"},
			AcceptedInputKinds:    []string{"provider", "provider-stream", "otel"},
			GatewayRoutes:         []string{"/gateway/openai/v1/chat/completions"},
			RecommendedSource:     "provider",
			PricingStrategy:       "LiteLLM model price table fallback plus local override for proxy-specific pricing",
			ReconciliationSupport: "proxy statement summary import when available",
			PrivacyNotes:          []string{"request/response wrappers are whitelisted", "raw proxy headers and secrets are ignored", "usageMetadata relay events are mapped to non-overlapping token fields"},
			ConformanceFixtures:   []string{"examples/adapter-fixtures/provider-openai-chat-completion.json", "examples/adapter-fixtures/provider-openai-chat-stream.sse", "examples/adapter-fixtures/provider-generic-usage-metadata-stream.sse"},
		},
		{
			ID:                    "google-gemini",
			Label:                 "Google Gemini or Vertex AI model API",
			Kind:                  "commercial-provider",
			Families:              []string{"google", "gemini", "vertex-ai", "usage-metadata"},
			ModelNameExamples:     []string{"gemini-2.5-pro", "gemini-2.5-flash"},
			UsageSchemas:          []string{"usage-metadata"},
			AcceptedInputKinds:    []string{"provider", "provider-stream", "otel"},
			RecommendedSource:     "provider",
			PricingStrategy:       "LiteLLM fallback or local override until official adapter seed is added",
			ReconciliationSupport: "manual statement summary import",
			PrivacyNotes:          []string{"usageMetadata token fields are supported", "prompt and candidate text are ignored", "cachedContentTokenCount is mapped to cache read input tokens"},
			ConformanceFixtures:   []string{"examples/adapter-fixtures/provider-generic-usage-metadata-stream.sse"},
		},
		{
			ID:                    "ollama-local",
			Label:                 "Ollama local runtime",
			Kind:                  "local-runtime",
			Families:              []string{"ollama", "openai-compatible", "local-model"},
			ModelNameExamples:     []string{"llama3.3", "qwen2.5-coder", "deepseek-coder"},
			UsageSchemas:          []string{"openai-chat-completions", "generic"},
			AcceptedInputKinds:    []string{"provider", "provider-stream", "otel"},
			GatewayRoutes:         []string{"/gateway/openai/v1/chat/completions"},
			RecommendedSource:     "provider",
			PricingStrategy:       "local override; default cost may be zero while tokens and calls remain authoritative",
			ReconciliationSupport: "not applicable unless a local chargeback profile is configured",
			PrivacyNotes:          []string{"local model prompts never need to leave the machine for ledger ingest", "usage must be supplied by the runtime or wrapper", "hardware energy/accounting can be modeled as a local override profile"},
			Limitations:           []string{"native token usage varies by runtime; wrappers should emit explicit usage when possible"},
		},
		{
			ID:                    "vllm-local",
			Label:                 "vLLM, LM Studio, llama.cpp, or edge OpenAI-compatible runtime",
			Kind:                  "local-or-edge-runtime",
			Families:              []string{"vllm", "lm-studio", "llama.cpp", "openai-compatible", "edge-model"},
			ModelNameExamples:     []string{"qwen3-coder", "llama-4-local", "mistral-local"},
			UsageSchemas:          []string{"openai-chat-completions", "generic"},
			AcceptedInputKinds:    []string{"provider", "provider-stream", "otel"},
			GatewayRoutes:         []string{"/gateway/openai/v1/chat/completions"},
			RecommendedSource:     "provider",
			PricingStrategy:       "local override or zero-cost token ledger; separate infrastructure chargeback can be imported through reconciliation",
			ReconciliationSupport: "local chargeback CSV/JSON import",
			PrivacyNotes:          []string{"edge deployments should emit metadata-only usage envelopes", "device ids, hostnames, and authors should be hashed before team aggregation", "do not include local file paths in provider metadata"},
			Limitations:           []string{"some runtimes omit usage in streaming mode; missing usage is explicit and never fabricated"},
		},
	}
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].ID < profiles[j].ID })
	catalog := ProviderProfileCatalog{
		Product:       "Agent Ledger",
		Contract:      "agent-ledger.provider-profile-catalog",
		Version:       "v1",
		GeneratedFrom: "static privacy-safe provider/runtime capability profiles",
		LocalFirst:    true,
		PrivacyPolicy: "Provider profiles are static metadata. They contain no API credentials, raw endpoints with secrets, prompts, responses, local paths, session ids, machine names, authors, or usage rows.",
		Profiles:      profiles,
		QualityGates: []string{
			"new provider adapters must pass adapter conformance before ingest is enabled",
			"provider usage must expose model name and non-overlapping token fields or confidence must be below 1",
			"source-reported cost is evidence; local pricing governance remains authoritative for recalculation",
			"local or edge runtimes may use zero cost, but token and call counts must remain explicit",
			"request/response bodies, headers, prompts, transcripts, and secrets must not be persisted",
		},
		RoutingGuidance: []string{
			"prefer OpenAI-compatible provider envelopes for relays and local runtimes when usage fields are available",
			"use usageMetadata mapping for Gemini-style providers and relays",
			"configure local overrides for enterprise contract, relay, local hardware, or edge chargeback pricing",
			"keep gateway and fallback routing disabled by default; enable only after provider profile and policy smoke tests pass",
		},
	}
	catalog.Summary = summarizeProviderProfiles(catalog.Profiles)
	return catalog
}

func ProviderProfilesFingerprint() string {
	raw, err := json.Marshal(ProviderProfiles())
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func summarizeProviderProfiles(profiles []ProviderProfile) ProviderSummary {
	var out ProviderSummary
	out.Profiles = len(profiles)
	for _, profile := range profiles {
		if len(profile.GatewayRoutes) > 0 {
			out.GatewayProfiles++
		}
		if profile.Kind == "local-runtime" || profile.Kind == "local-or-edge-runtime" {
			out.LocalRuntimeProfiles++
		}
		if profile.Kind == "local-or-edge-runtime" || containsProviderFamily(profile, "edge-model") {
			out.EdgeRuntimeProfiles++
		}
		if containsProviderFamily(profile, "openai-compatible") {
			out.OpenAICompatible++
		}
		if containsProviderFamily(profile, "anthropic") {
			out.AnthropicCompatible++
		}
		if containsString(profile.UsageSchemas, "usage-metadata") {
			out.UsageMetadataProfiles++
		}
	}
	return out
}

func containsProviderFamily(profile ProviderProfile, family string) bool {
	return containsString(profile.Families, family)
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
