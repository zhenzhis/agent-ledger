package integrations

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

// SignalTaxonomyCatalog is a static, privacy-safe signal dictionary for
// wrappers, routers, provider adapters, and observability bridges.
type SignalTaxonomyCatalog struct {
	Product          string                 `json:"product"`
	Contract         string                 `json:"contract"`
	Version          string                 `json:"version"`
	GeneratedFrom    string                 `json:"generated_from"`
	LocalFirst       bool                   `json:"local_first"`
	ReadOnlySafe     bool                   `json:"read_only_safe"`
	WritesLocalState bool                   `json:"writes_local_state"`
	PrivacyPolicy    string                 `json:"privacy_policy"`
	Summary          SignalTaxonomySummary  `json:"summary"`
	Signals          []SignalTaxonomySignal `json:"signals"`
	QualityGates     []string               `json:"quality_gates"`
	RoutingGuidance  []string               `json:"routing_guidance"`
}

// SignalTaxonomySummary captures stable signal coverage counts.
type SignalTaxonomySummary struct {
	Signals          int `json:"signals"`
	EventSignals     int `json:"event_signals"`
	UsageSignals     int `json:"usage_signals"`
	WorkloadSignals  int `json:"workload_signals"`
	PolicySignals    int `json:"policy_signals"`
	ExactPreferred   int `json:"exact_preferred"`
	EstimatedAllowed int `json:"estimated_allowed"`
}

// SignalTaxonomySignal describes one metadata-only integration signal.
type SignalTaxonomySignal struct {
	ID                  string   `json:"id"`
	Label               string   `json:"label"`
	Category            string   `json:"category"`
	Description         string   `json:"description"`
	CanonicalEventTypes []string `json:"canonical_event_types"`
	RecommendedFields   []string `json:"recommended_fields"`
	AcceptedAliases     []string `json:"accepted_aliases,omitempty"`
	Precision           string   `json:"precision"`
	RequiredFor         []string `json:"required_for,omitempty"`
	PrivacyClass        string   `json:"privacy_class"`
	ContentSafe         bool     `json:"content_safe"`
	QualityGates        []string `json:"quality_gates"`
}

// SignalTaxonomy returns the static signal dictionary for current and future
// Agent Ledger integrations.
func SignalTaxonomy() SignalTaxonomyCatalog {
	signals := []SignalTaxonomySignal{
		{
			ID:                  "agent.run.lifecycle",
			Label:               "Agent run lifecycle",
			Category:            "workload",
			Description:         "Start, heartbeat, progress, phase, and finish metadata for asynchronous agent execution.",
			CanonicalEventTypes: []string{"agent.run.started", "agent.run.heartbeat", "agent.run.finished"},
			RecommendedFields:   []string{"workload_id", "agent_run_id", "source", "agent_name", "status", "phase", "progress", "timestamp"},
			AcceptedAliases:     []string{"run lifecycle", "heartbeat", "liveness", "agent status"},
			Precision:           "exact-preferred",
			RequiredFor:         []string{"workload-ledger", "run-liveness", "agentops-control-plane"},
			PrivacyClass:        "metadata-only lifecycle state",
			ContentSafe:         true,
			QualityGates:        []string{"run identifiers must be source-scoped or ledger-generated", "status values should be finite and explicit", "heartbeat timestamps must be monotonic when emitted by one runner"},
		},
		{
			ID:                  "artifact.reference",
			Label:               "Artifact reference",
			Category:            "artifact",
			Description:         "Artifact metadata, content hashes, mime labels, and local opaque references without artifact bodies.",
			CanonicalEventTypes: []string{"artifact.created"},
			RecommendedFields:   []string{"workload_id", "agent_run_id", "artifact_id", "artifact_type", "mime_type", "content_hash", "ref_hash", "timestamp"},
			AcceptedAliases:     []string{"artifact hash", "file hash", "evidence reference"},
			Precision:           "hash-or-label",
			RequiredFor:         []string{"incident-evidence", "offline-bundle", "compliance-export"},
			PrivacyClass:        "hashes and short labels only",
			ContentSafe:         true,
			QualityGates:        []string{"artifact bodies must stay outside the ledger", "external references should be hashed before persistence", "mime labels must not contain raw local paths"},
		},
		{
			ID:                  "context.reference",
			Label:               "Context reference",
			Category:            "context",
			Description:         "Context source metadata, byte/token estimates, and stable reference hashes without storing context bodies.",
			CanonicalEventTypes: []string{"context.ref"},
			RecommendedFields:   []string{"workload_id", "agent_run_id", "context_id", "context_type", "ref_hash", "token_estimate", "timestamp", "confidence"},
			AcceptedAliases:     []string{"context ref", "evidence ref", "retrieval ref"},
			Precision:           "exact-or-estimated",
			RequiredFor:         []string{"cache-doctor", "cost-intelligence", "data-quality"},
			PrivacyClass:        "reference hash plus bounded metadata",
			ContentSafe:         true,
			QualityGates:        []string{"context bodies must not be persisted", "token estimates require confidence below exact counters", "local paths should be replaced by aliases or hashes"},
		},
		{
			ID:                  "evaluation.signal",
			Label:               "Evaluation signal",
			Category:            "quality",
			Description:         "Local quality, test, review, or acceptance metadata used to connect cost with outcomes.",
			CanonicalEventTypes: []string{"evaluation.recorded"},
			RecommendedFields:   []string{"workload_id", "agent_run_id", "evaluation_id", "evaluator", "status", "score", "signal", "timestamp"},
			AcceptedAliases:     []string{"quality signal", "test result", "acceptance signal", "review signal"},
			Precision:           "exact-preferred",
			RequiredFor:         []string{"efficiency-score", "session-quality", "wrapped"},
			PrivacyClass:        "bounded labels and numeric scores",
			ContentSafe:         true,
			QualityGates:        []string{"notes must remain short metadata", "scores should declare evaluator and status", "negative outcomes should not include raw failure logs with secrets"},
		},
		{
			ID:                  "model.identity",
			Label:               "Model identity",
			Category:            "model",
			Description:         "Provider spelling, model alias, and model family metadata for pricing, routing, and attribution.",
			CanonicalEventTypes: []string{"model.call"},
			RecommendedFields:   []string{"source", "provider", "model", "model_alias", "pricing_model", "timestamp"},
			AcceptedAliases:     []string{"model", "model name", "deployment", "engine"},
			Precision:           "exact-preferred",
			RequiredFor:         []string{"pricing-governance", "router-simulator", "model-call-analytics"},
			PrivacyClass:        "provider and model metadata",
			ContentSafe:         true,
			QualityGates:        []string{"preserve provider spelling", "use model_alias for normalization rather than overwriting raw model", "unrecognized models must surface as unpriced or fuzzy"},
		},
		{
			ID:                  "policy.decision",
			Label:               "Policy decision",
			Category:            "policy",
			Description:         "Advisory or enforcement decision metadata for local budget, model, export, approval, and routing policies.",
			CanonicalEventTypes: []string{"policy.decision"},
			RecommendedFields:   []string{"workload_id", "agent_run_id", "rule_id", "action", "target", "effective_action", "role", "timestamp"},
			AcceptedAliases:     []string{"policy", "approval", "enforcement", "guardrail"},
			Precision:           "exact-preferred",
			RequiredFor:         []string{"policy-audit", "approval-routing", "rbac"},
			PrivacyClass:        "decision metadata and redacted evidence",
			ContentSafe:         true,
			QualityGates:        []string{"evidence must be redacted before persistence", "approval records must not include prompt content", "policy decisions should be replayable from rule metadata"},
		},
		{
			ID:                  "pricing.provenance",
			Label:               "Pricing provenance",
			Category:            "pricing",
			Description:         "Pricing source, matched model, match type, freshness, and confidence metadata for cost recalculation.",
			CanonicalEventTypes: []string{"model.call"},
			RecommendedFields:   []string{"pricing_source", "pricing_model", "pricing_confidence", "match_type", "stale", "unpriced", "timestamp"},
			AcceptedAliases:     []string{"pricing source", "price freshness", "matched model", "unpriced model"},
			Precision:           "exact-or-fuzzy",
			RequiredFor:         []string{"pricing-governance", "provider-reconciliation", "data-quality"},
			PrivacyClass:        "public price metadata plus local override labels",
			ContentSafe:         true,
			QualityGates:        []string{"source-reported cost is evidence, not authoritative recalculation", "stale and fuzzy matches must stay visible", "local override labels must not expose contract secrets"},
		},
		{
			ID:                  "project.attribution",
			Label:               "Project attribution",
			Category:            "attribution",
			Description:         "Project, repo, branch, team, owner, machine, and workspace aliases used for showback and filtering.",
			CanonicalEventTypes: []string{"workload.started", "agent.run.started", "model.call", "tool.call"},
			RecommendedFields:   []string{"project", "repo", "git_branch", "team", "owner", "machine_hash", "workspace_alias"},
			AcceptedAliases:     []string{"project", "repo", "repository", "branch", "team", "workspace"},
			Precision:           "alias-or-hash",
			RequiredFor:         []string{"chargeback", "showback", "repo-cost", "branch-cost"},
			PrivacyClass:        "aliases or hashes for local identifiers",
			ContentSafe:         true,
			QualityGates:        []string{"absolute paths must be aliased or hashed before sharing", "detached branches should remain explicit", "machine and author labels should use configured privacy presets"},
		},
		{
			ID:                  "tool.call.metadata",
			Label:               "Tool call metadata",
			Category:            "tool",
			Description:         "Tool category, duration, status, and bounded error class metadata without raw arguments or outputs.",
			CanonicalEventTypes: []string{"tool.call"},
			RecommendedFields:   []string{"workload_id", "agent_run_id", "tool_name", "tool_category", "status", "duration_ms", "error_class", "timestamp"},
			AcceptedAliases:     []string{"tool", "command", "function call", "mcp tool"},
			Precision:           "exact-preferred",
			RequiredFor:         []string{"runaway-watchdog", "audit-log", "session-quality"},
			PrivacyClass:        "bounded operation metadata",
			ContentSafe:         true,
			QualityGates:        []string{"raw tool arguments must not be persisted", "secret-like command fragments must be redacted", "error_class should summarize failure without log bodies"},
		},
		{
			ID:                  "usage.tokens",
			Label:               "Token usage",
			Category:            "usage",
			Description:         "Non-overlapping input, cache read/write, output, reasoning, call count, and timestamp counters.",
			CanonicalEventTypes: []string{"model.call"},
			RecommendedFields:   []string{"input_tokens", "cache_read_input_tokens", "cache_creation_input_tokens", "output_tokens", "reasoning_output_tokens", "calls", "timestamp"},
			AcceptedAliases:     []string{"usage", "token usage", "usage counters", "prompt tokens", "completion tokens", "cache tokens"},
			Precision:           "exact-preferred",
			RequiredFor:         []string{"cost-intelligence", "budgets", "quota", "model-call-analytics"},
			PrivacyClass:        "numeric counters only",
			ContentSafe:         true,
			QualityGates:        []string{"token components must be non-overlapping", "estimated aggregate rows must carry lower confidence", "missing usage must remain explicit and never be fabricated"},
		},
		{
			ID:                  "workload.identity",
			Label:               "Workload identity",
			Category:            "workload",
			Description:         "Goal, source, parent, idempotency, and DAG metadata for async goal/context execution without storing prompt bodies.",
			CanonicalEventTypes: []string{"workload.started", "workload.linked", "workload.closed"},
			RecommendedFields:   []string{"workload_id", "parent_workload_id", "source", "goal_hash", "goal_label", "idempotency_key", "relation", "timestamp"},
			AcceptedAliases:     []string{"goal", "task", "workload", "run group", "dag node"},
			Precision:           "exact-preferred",
			RequiredFor:         []string{"async-goal-ledger", "workload-graph", "offline-bundle"},
			PrivacyClass:        "bounded labels, hashes, and lineage ids",
			ContentSafe:         true,
			QualityGates:        []string{"goal labels should be short and share-safe", "idempotency keys should be stable and non-secret", "parent links must not imply remote execution ownership"},
		},
	}
	sort.Slice(signals, func(i, j int) bool { return signals[i].ID < signals[j].ID })
	catalog := SignalTaxonomyCatalog{
		Product:          "Agent Ledger",
		Contract:         "agent-ledger.signal-taxonomy",
		Version:          "v1",
		GeneratedFrom:    "static privacy-safe canonical signal dictionary",
		LocalFirst:       true,
		ReadOnlySafe:     true,
		WritesLocalState: false,
		PrivacyPolicy:    "Signal taxonomy is static metadata. It contains no local usage rows, prompt content, response content, raw paths, secrets, machine names, authors, account ids, or session ids.",
		Signals:          signals,
		QualityGates: []string{
			"adapter fixtures should map source fields to taxonomy signal ids before ingest",
			"signals carrying estimates must expose confidence or precision below exact-preferred",
			"content-bearing source fields must be represented by hashes, aliases, counts, or bounded labels",
			"wrappers should pin the taxonomy fingerprint in CI when building stable adapters",
		},
		RoutingGuidance: []string{
			"use usage.tokens and model.identity for provider envelope adapters",
			"use workload.identity and agent.run.lifecycle for async goal/context runners",
			"use project.attribution for chargeback and team showback",
			"use pricing.provenance when reconciling official, fallback, override, and source-reported prices",
			"use tool.call.metadata and evaluation.signal to connect spend with operational quality without reading content",
		},
	}
	catalog.Summary = summarizeSignalTaxonomy(signals)
	return catalog
}

func SignalTaxonomyFingerprint() string {
	raw, err := json.Marshal(SignalTaxonomy())
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func summarizeSignalTaxonomy(signals []SignalTaxonomySignal) SignalTaxonomySummary {
	out := SignalTaxonomySummary{Signals: len(signals)}
	for _, signal := range signals {
		switch signal.Category {
		case "usage", "model", "pricing":
			out.UsageSignals++
		case "workload", "attribution":
			out.WorkloadSignals++
		case "policy":
			out.PolicySignals++
		}
		if len(signal.CanonicalEventTypes) > 0 {
			out.EventSignals++
		}
		switch signal.Precision {
		case "exact-preferred":
			out.ExactPreferred++
		case "exact-or-estimated", "exact-or-fuzzy", "alias-or-hash", "hash-or-label":
			out.EstimatedAllowed++
		}
	}
	return out
}
