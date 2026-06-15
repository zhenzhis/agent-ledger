package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// SignalCoverageReport links the canonical signal taxonomy to adapter,
// provider, and agent-framework contracts so wrapper CI can detect drift.
type SignalCoverageReport struct {
	Product               string                  `json:"product"`
	Contract              string                  `json:"contract"`
	Version               string                  `json:"version"`
	GeneratedFrom         string                  `json:"generated_from"`
	LocalFirst            bool                    `json:"local_first"`
	ReadOnlySafe          bool                    `json:"read_only_safe"`
	WritesLocalState      bool                    `json:"writes_local_state"`
	TaxonomyHash          string                  `json:"taxonomy_hash"`
	AdapterSpecHash       string                  `json:"adapter_spec_hash"`
	ConformanceMatrixHash string                  `json:"conformance_matrix_hash"`
	ProviderProfilesHash  string                  `json:"provider_profiles_hash"`
	AgentProfilesHash     string                  `json:"agent_profiles_hash"`
	PrivacyPolicy         string                  `json:"privacy_policy"`
	Summary               SignalCoverageSummary   `json:"summary"`
	Signals               []SignalCoverageSignal  `json:"signals"`
	AdapterKinds          []SignalCoverageAdapter `json:"adapter_kinds"`
	Gaps                  []SignalCoverageGap     `json:"gaps"`
	QualityGates          []string                `json:"quality_gates"`
	RoutingGuidance       []string                `json:"routing_guidance"`
}

// SignalCoverageSummary captures stable signal coverage counts.
type SignalCoverageSummary struct {
	TaxonomySignals               int `json:"taxonomy_signals"`
	AdapterKinds                  int `json:"adapter_kinds"`
	ProviderProfiles              int `json:"provider_profiles"`
	AgentProfiles                 int `json:"agent_profiles"`
	CoveredSignals                int `json:"covered_signals"`
	RequiredSignalReferences      int `json:"required_signal_references"`
	UnknownSignalReferences       int `json:"unknown_signal_references"`
	SignalsWithoutAdapterCoverage int `json:"signals_without_adapter_coverage"`
	Gaps                          int `json:"gaps"`
}

// SignalCoverageSignal describes how one taxonomy signal is covered.
type SignalCoverageSignal struct {
	ID                     string   `json:"id"`
	Label                  string   `json:"label"`
	Category               string   `json:"category"`
	CanonicalEventTypes    []string `json:"canonical_event_types"`
	RequiredByAdapterKinds []string `json:"required_by_adapter_kinds,omitempty"`
	ProviderProfileIDs     []string `json:"provider_profile_ids,omitempty"`
	AgentProfileIDs        []string `json:"agent_profile_ids,omitempty"`
	Status                 string   `json:"status"`
}

// SignalCoverageAdapter describes taxonomy signal coverage for one adapter kind.
type SignalCoverageAdapter struct {
	Kind               string   `json:"kind"`
	ConformanceKind    string   `json:"conformance_kind"`
	RequiredSignals    []string `json:"required_signals"`
	UnknownSignals     []string `json:"unknown_signals,omitempty"`
	ExpectedEventTypes []string `json:"expected_event_types"`
	Fixtures           int      `json:"fixtures"`
	Status             string   `json:"status"`
}

// SignalCoverageGap is a privacy-safe coverage issue for adapter authors.
type SignalCoverageGap struct {
	Severity    string `json:"severity"`
	Surface     string `json:"surface"`
	Reference   string `json:"reference"`
	Message     string `json:"message"`
	Remediation string `json:"remediation"`
}

// SignalCoverage returns a read-only, deterministic mapping between taxonomy
// signals and the public ecosystem integration contracts.
func SignalCoverage() SignalCoverageReport {
	taxonomy := SignalTaxonomy()
	matrix := AdapterConformanceMatrixSpec()
	providers := ProviderProfiles()
	agents := AgentFrameworkProfiles()

	knownSignals := make(map[string]SignalTaxonomySignal, len(taxonomy.Signals))
	eventSignals := map[string][]string{}
	coverage := make(map[string]*SignalCoverageSignal, len(taxonomy.Signals))
	for _, signal := range taxonomy.Signals {
		knownSignals[signal.ID] = signal
		entry := SignalCoverageSignal{
			ID:                  signal.ID,
			Label:               signal.Label,
			Category:            signal.Category,
			CanonicalEventTypes: append([]string(nil), signal.CanonicalEventTypes...),
			Status:              "taxonomy-only",
		}
		coverage[signal.ID] = &entry
		for _, eventType := range signal.CanonicalEventTypes {
			eventSignals[eventType] = appendUnique(eventSignals[eventType], signal.ID)
		}
	}

	gaps := []SignalCoverageGap{}
	adapterSignals := map[string][]string{}
	adapters := make([]SignalCoverageAdapter, 0, len(matrix.Kinds))
	requiredRefs := 0
	unknownRefs := 0
	for _, kind := range matrix.Kinds {
		adapter := SignalCoverageAdapter{
			Kind:               kind.Kind,
			ConformanceKind:    kind.ConformanceKind,
			RequiredSignals:    append([]string(nil), kind.RequiredSignals...),
			ExpectedEventTypes: append([]string(nil), kind.ExpectedEventTypes...),
			Fixtures:           len(kind.Fixtures),
			Status:             "covered",
		}
		for _, signalID := range kind.RequiredSignals {
			requiredRefs++
			if _, ok := knownSignals[signalID]; !ok {
				unknownRefs++
				adapter.UnknownSignals = appendUnique(adapter.UnknownSignals, signalID)
				gaps = append(gaps, SignalCoverageGap{
					Severity:    "critical",
					Surface:     "adapter-conformance",
					Reference:   kind.ConformanceKind + ":" + signalID,
					Message:     "adapter kind references a signal id that is not present in the taxonomy",
					Remediation: "add the signal to the taxonomy or replace the adapter required_signals entry with an existing taxonomy id",
				})
				continue
			}
			adapterSignals[kind.ConformanceKind] = appendUnique(adapterSignals[kind.ConformanceKind], signalID)
			entry := coverage[signalID]
			entry.RequiredByAdapterKinds = appendUnique(entry.RequiredByAdapterKinds, kind.ConformanceKind)
			entry.Status = "covered"
		}
		if len(adapter.UnknownSignals) > 0 {
			sort.Strings(adapter.UnknownSignals)
			adapter.Status = "gap"
		}
		sort.Strings(adapter.RequiredSignals)
		sort.Strings(adapter.ExpectedEventTypes)
		adapters = append(adapters, adapter)
	}

	for _, profile := range providers.Profiles {
		signals := signalsForAdapterKinds(profile.AcceptedInputKinds, adapterSignals)
		if profile.PricingStrategy != "" || profile.ReconciliationSupport != "" {
			signals = appendUnique(signals, "pricing.provenance")
		}
		for _, signalID := range signals {
			if entry := coverage[signalID]; entry != nil {
				entry.ProviderProfileIDs = appendUnique(entry.ProviderProfileIDs, profile.ID)
			}
		}
		for _, kind := range profile.AcceptedInputKinds {
			if _, ok := adapterSignals[kind]; !ok {
				gaps = append(gaps, SignalCoverageGap{
					Severity:    "warning",
					Surface:     "provider-profile",
					Reference:   profile.ID + ":" + kind,
					Message:     "provider profile references an adapter kind without signal coverage",
					Remediation: "add the adapter kind to the conformance matrix or remove the unsupported accepted_input_kinds value",
				})
			}
		}
	}

	for _, profile := range agents.Profiles {
		signals := signalsForAdapterKinds(profile.AdapterInputKinds, adapterSignals)
		for _, eventType := range profile.CanonicalEventTypes {
			for _, signalID := range eventSignals[eventType] {
				signals = appendUnique(signals, signalID)
			}
		}
		for _, signalID := range signals {
			if entry := coverage[signalID]; entry != nil {
				entry.AgentProfileIDs = appendUnique(entry.AgentProfileIDs, profile.ID)
			}
		}
		for _, kind := range profile.AdapterInputKinds {
			if _, ok := adapterSignals[kind]; !ok {
				gaps = append(gaps, SignalCoverageGap{
					Severity:    "warning",
					Surface:     "agent-profile",
					Reference:   profile.ID + ":" + kind,
					Message:     "agent profile references an adapter kind without signal coverage",
					Remediation: "add the adapter kind to the conformance matrix or update the agent profile adapter_input_kinds",
				})
			}
		}
	}

	signals := make([]SignalCoverageSignal, 0, len(coverage))
	withoutAdapter := 0
	covered := 0
	for _, entry := range coverage {
		sort.Strings(entry.CanonicalEventTypes)
		sort.Strings(entry.RequiredByAdapterKinds)
		sort.Strings(entry.ProviderProfileIDs)
		sort.Strings(entry.AgentProfileIDs)
		if len(entry.RequiredByAdapterKinds) == 0 {
			withoutAdapter++
			gaps = append(gaps, SignalCoverageGap{
				Severity:    "warning",
				Surface:     "signal-taxonomy",
				Reference:   entry.ID,
				Message:     "taxonomy signal has no adapter conformance coverage",
				Remediation: "cover the signal in at least one adapter required_signals list or mark it planned in a future taxonomy revision",
			})
		}
		if entry.Status == "covered" || len(entry.ProviderProfileIDs) > 0 || len(entry.AgentProfileIDs) > 0 {
			covered++
		}
		signals = append(signals, *entry)
	}
	sort.Slice(signals, func(i, j int) bool { return signals[i].ID < signals[j].ID })
	sort.Slice(adapters, func(i, j int) bool { return adapters[i].ConformanceKind < adapters[j].ConformanceKind })
	sort.Slice(gaps, func(i, j int) bool {
		if gaps[i].Severity == gaps[j].Severity {
			return gaps[i].Reference < gaps[j].Reference
		}
		return gaps[i].Severity < gaps[j].Severity
	})

	report := SignalCoverageReport{
		Product:               "Agent Ledger",
		Contract:              "agent-ledger.signal-coverage",
		Version:               "v1",
		GeneratedFrom:         "taxonomy, adapter contract, conformance matrix, provider profiles, and agent framework profiles",
		LocalFirst:            true,
		ReadOnlySafe:          true,
		WritesLocalState:      false,
		TaxonomyHash:          SignalTaxonomyFingerprint(),
		AdapterSpecHash:       AdapterContractFingerprint(),
		ConformanceMatrixHash: AdapterConformanceMatrixFingerprint(),
		ProviderProfilesHash:  ProviderProfilesFingerprint(),
		AgentProfilesHash:     AgentFrameworkProfilesFingerprint(),
		PrivacyPolicy:         "Signal coverage is static metadata only. It contains no usage rows, prompt content, response content, raw paths, secrets, machine names, authors, account ids, or session ids.",
		Signals:               signals,
		AdapterKinds:          adapters,
		Gaps:                  gaps,
		QualityGates: []string{
			"adapter required_signals must be canonical taxonomy signal ids",
			"provider and agent profiles should reference adapter kinds with signal coverage",
			"coverage gaps must be visible in contract verification before new adapters are enabled",
			"coverage reports must remain static metadata and must not read local usage rows",
		},
		RoutingGuidance: []string{
			"adapter authors should validate fixtures and this coverage report in CI",
			"provider wrappers should select provider or provider-stream coverage unless they emit canonical events directly",
			"multi-agent runtimes should prefer workload.identity and agent.run.lifecycle for goal/context execution",
			"observability bridges should map model.identity, usage.tokens, context.reference, and tool.call.metadata before ingest",
		},
	}
	report.Summary = SignalCoverageSummary{
		TaxonomySignals:               len(taxonomy.Signals),
		AdapterKinds:                  len(matrix.Kinds),
		ProviderProfiles:              len(providers.Profiles),
		AgentProfiles:                 len(agents.Profiles),
		CoveredSignals:                covered,
		RequiredSignalReferences:      requiredRefs,
		UnknownSignalReferences:       unknownRefs,
		SignalsWithoutAdapterCoverage: withoutAdapter,
		Gaps:                          len(gaps),
	}
	return report
}

// SignalCoverageFingerprint returns a stable hash for discovery and CI cache
// validators.
func SignalCoverageFingerprint() string {
	raw, err := json.Marshal(SignalCoverage())
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func signalsForAdapterKinds(kinds []string, adapterSignals map[string][]string) []string {
	out := []string{}
	for _, kind := range kinds {
		for _, signalID := range adapterSignals[kind] {
			out = appendUnique(out, signalID)
		}
	}
	sort.Strings(out)
	return out
}

func appendUnique(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
