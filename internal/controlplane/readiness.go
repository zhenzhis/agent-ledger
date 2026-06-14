package controlplane

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

// ReadinessReport is a privacy-safe control-plane probe for wrappers, routers,
// CI, and deployment scripts. It exposes readiness facts and counts only; it
// never includes raw collector paths, pricing URLs, secrets, prompts, sessions,
// projects, branches, machine names, or authors.
type ReadinessReport struct {
	Product             string           `json:"product"`
	Slug                string           `json:"slug"`
	Contract            string           `json:"contract"`
	Version             string           `json:"version"`
	GeneratedAt         string           `json:"generated_at"`
	Status              string           `json:"status"`
	Mode                string           `json:"mode"`
	ReadOnly            bool             `json:"read_only"`
	AcceptsWrites       bool             `json:"accepts_writes"`
	LocalFirst          bool             `json:"local_first"`
	PromptContentStored bool             `json:"prompt_content_stored"`
	UsageDataUploaded   bool             `json:"usage_data_uploaded"`
	Summary             ReadinessSummary `json:"summary"`
	Checks              []ReadinessCheck `json:"checks"`
	PrivacyNote         string           `json:"privacy_note"`
}

type ReadinessSummary struct {
	TotalChecks        int    `json:"total_checks"`
	PassingChecks      int    `json:"passing_checks"`
	CriticalFailures   int    `json:"critical_failures"`
	Warnings           int    `json:"warnings"`
	Info               int    `json:"info"`
	UsageRecords       int    `json:"usage_records"`
	PromptEvents       int    `json:"prompt_events"`
	IdempotencyKeys    int    `json:"idempotency_keys"`
	IdempotencyReplays int    `json:"idempotency_replays"`
	QueueClaimable     int    `json:"queue_claimable"`
	QueueNonTerminal   int    `json:"queue_non_terminal"`
	QueueOldestAge     string `json:"queue_oldest_claimable_age"`
	QueueNextLease     string `json:"queue_next_lease_expiry"`
	ActiveLeases       int    `json:"active_leases"`
	ExpiredLeases      int    `json:"expired_leases"`
	ReleasedLeases     int    `json:"released_leases"`
	ActiveRuns         int    `json:"active_runs"`
	StaleRuns          int    `json:"stale_runs"`
	OldestRunAge       string `json:"oldest_run_age"`
	HealthSources      int    `json:"health_sources"`
	HealthErrors       int    `json:"health_errors"`
	PricingSources     int    `json:"pricing_sources"`
	PricingStale       int    `json:"pricing_stale"`
	PricingErrors      int    `json:"pricing_errors"`
	ConfigIssues       int    `json:"config_issues"`
	ContractChecks     int    `json:"contract_checks"`
	ContractFailures   int    `json:"contract_failures"`
	Recommendation     string `json:"recommendation"`
}

type ReadinessCheck struct {
	Name     string `json:"name"`
	OK       bool   `json:"ok"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Action   string `json:"action,omitempty"`
}

// BuildReadinessReport returns a deterministic, metadata-only readiness probe.
func BuildReadinessReport(db *storage.DB, cfg *config.Config, runtime *storage.RuntimeStatus, contract integrations.ContractVerificationReport, now time.Time) *ReadinessReport {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	cfgStatus := config.StatusReport(cfg)
	if runtime == nil {
		runtime = defaultRuntimeStatus()
	}
	report := &ReadinessReport{
		Product:             "Agent Ledger",
		Slug:                "agent-ledger",
		Contract:            "agent-ledger.readiness",
		Version:             "v1",
		GeneratedAt:         now.UTC().Format(time.RFC3339Nano),
		Mode:                runtime.Mode,
		ReadOnly:            runtime.ReadOnly,
		AcceptsWrites:       !runtime.ReadOnly && runtime.WriteOperations == "enabled",
		LocalFirst:          cfgStatus.LocalFirst,
		PromptContentStored: false,
		UsageDataUploaded:   false,
		Summary:             ReadinessSummary{QueueOldestAge: "none", QueueNextLease: "none", OldestRunAge: "none"},
		PrivacyNote:         "Readiness exposes status, counts, hashes, and remediation hints only; raw paths, URLs, secrets, prompts, responses, sessions, projects, branches, machine names, and authors are excluded.",
	}

	addCoreChecks(report, db, now.UTC())
	addConfigChecks(report, cfgStatus)
	addRuntimeCheck(report, runtime)
	addContractCheck(report, contract)
	addIngestionChecks(report, db)
	addPricingChecks(report, db, cfg)
	report.finalize()
	return report
}

// FormatReadinessMarkdown renders the readiness report for operator terminals.
func FormatReadinessMarkdown(report *ReadinessReport) string {
	if report == nil {
		report = BuildReadinessReport(nil, nil, nil, integrations.ContractVerificationReport{}, time.Now().UTC())
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# Agent Ledger Readiness\n\n")
	fmt.Fprintf(&b, "- Status: `%s`\n", report.Status)
	fmt.Fprintf(&b, "- Mode: `%s`\n", report.Mode)
	fmt.Fprintf(&b, "- Read only: `%t`\n", report.ReadOnly)
	fmt.Fprintf(&b, "- Accepts writes: `%t`\n", report.AcceptsWrites)
	fmt.Fprintf(&b, "- Local first: `%t`\n", report.LocalFirst)
	fmt.Fprintf(&b, "- Checks: `%d` passing, `%d` critical, `%d` warnings\n", report.Summary.PassingChecks, report.Summary.CriticalFailures, report.Summary.Warnings)
	fmt.Fprintf(&b, "- Data: `%d` usage records, `%d` prompt events, `%d` health sources\n", report.Summary.UsageRecords, report.Summary.PromptEvents, report.Summary.HealthSources)
	fmt.Fprintf(&b, "- Control idempotency: `%d` keys, `%d` replays\n", report.Summary.IdempotencyKeys, report.Summary.IdempotencyReplays)
	fmt.Fprintf(&b, "- Workload queue: `%d` claimable, `%d` non-terminal, oldest age `%s`, next lease expiry `%s`\n", report.Summary.QueueClaimable, report.Summary.QueueNonTerminal, report.Summary.QueueOldestAge, report.Summary.QueueNextLease)
	fmt.Fprintf(&b, "- Workload leases: `%d` active, `%d` expired, `%d` released\n", report.Summary.ActiveLeases, report.Summary.ExpiredLeases, report.Summary.ReleasedLeases)
	fmt.Fprintf(&b, "- Agent runs: `%d` active, `%d` stale, oldest age `%s`\n", report.Summary.ActiveRuns, report.Summary.StaleRuns, report.Summary.OldestRunAge)
	fmt.Fprintf(&b, "- Pricing: `%d` sources, `%d` stale, `%d` errors\n", report.Summary.PricingSources, report.Summary.PricingStale, report.Summary.PricingErrors)
	fmt.Fprintf(&b, "- Recommendation: %s\n\n", report.Summary.Recommendation)
	b.WriteString("## Checks\n\n")
	for _, check := range report.Checks {
		state := "pass"
		if !check.OK {
			state = "fail"
		}
		fmt.Fprintf(&b, "- `%s` `%s` %s: %s", check.Severity, state, check.Name, check.Message)
		if check.Action != "" {
			fmt.Fprintf(&b, " Action: %s", check.Action)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// ReadinessFingerprint hashes readiness semantics while ignoring GeneratedAt,
// so REST clients can revalidate unchanged readiness state with If-None-Match.
func ReadinessFingerprint(report *ReadinessReport) string {
	if report == nil {
		return ""
	}
	copyReport := *report
	copyReport.GeneratedAt = ""
	raw, err := json.Marshal(copyReport)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func addCoreChecks(report *ReadinessReport, db *storage.DB, now time.Time) {
	if db == nil {
		report.addCheck("database.open", false, "critical", "SQLite database handle is not available", "open storage before serving control-plane traffic")
		return
	}
	usageRecords, err := db.CountUsageRecords("")
	if err != nil {
		report.addCheck("database.usage_records_query", false, "critical", "usage_records query failed", "inspect SQLite file permissions and schema migrations")
	} else {
		report.Summary.UsageRecords = usageRecords
		report.addCheck("database.usage_records_query", true, "critical", "usage_records query succeeded", "")
	}
	promptEvents, err := db.CountPromptEvents("")
	if err != nil {
		report.addCheck("database.prompt_events_query", false, "critical", "prompt_events query failed", "inspect SQLite file permissions and schema migrations")
	} else {
		report.Summary.PromptEvents = promptEvents
		report.addCheck("database.prompt_events_query", true, "critical", "prompt_events query succeeded", "")
	}
	idempotency, err := db.GetControlIdempotencyStats()
	if err != nil {
		report.addCheck("database.control_idempotency_query", false, "critical", "control_idempotency query failed", "inspect SQLite schema migrations")
	} else {
		report.Summary.IdempotencyKeys = idempotency.TotalKeys
		report.Summary.IdempotencyReplays = idempotency.ReplayCount
		report.addCheck("database.control_idempotency_query", true, "critical", "control_idempotency query succeeded", "")
	}
	leases, err := db.GetWorkloadLeaseStats()
	if err != nil {
		report.addCheck("database.workload_leases_query", false, "critical", "workload_leases query failed", "inspect SQLite schema migrations")
	} else {
		report.Summary.ActiveLeases = leases.Active
		report.Summary.ExpiredLeases = leases.Expired
		report.Summary.ReleasedLeases = leases.Released
		report.addCheck("database.workload_leases_query", true, "critical", "workload_leases query succeeded", "")
	}
	queue, err := db.GetWorkloadQueueStats(storage.WorkloadClaimFilter{})
	if err != nil {
		report.addCheck("database.workload_queue_query", false, "critical", "workload queue query failed", "inspect SQLite schema migrations and workload queue indexes")
	} else {
		report.Summary.QueueClaimable = queue.Claimable
		report.Summary.QueueNonTerminal = queue.NonTerminal
		report.Summary.QueueOldestAge = elapsedBucket(queue.OldestClaimableAt, now)
		report.Summary.QueueNextLease = remainingBucket(queue.NextLeaseExpiryAt, now)
		report.addCheck("database.workload_queue_query", true, "info", "workload queue probe succeeded", "")
	}
	runStats, err := db.GetAgentRunLivenessStatsAt(10*time.Minute, now)
	if err != nil {
		report.addCheck("database.agent_run_liveness_query", false, "critical", "agent run liveness query failed", "inspect SQLite schema migrations and agent run indexes")
	} else {
		report.Summary.ActiveRuns = runStats.Active
		report.Summary.StaleRuns = runStats.Stale
		if runStats.Active > 0 {
			report.Summary.OldestRunAge = durationBucket(time.Duration(runStats.OldestAgeSeconds) * time.Second)
		}
		if runStats.Stale > 0 {
			report.addCheck("database.agent_run_liveness", false, "warning", "one or more active agent runs have stale heartbeats", "inspect agent-ledger workload liveness --stale-only")
		} else {
			report.addCheck("database.agent_run_liveness", true, "info", "agent run liveness probe succeeded", "")
		}
	}
}

func addConfigChecks(report *ReadinessReport, cfgStatus *config.ConfigStatusReport) {
	if cfgStatus == nil {
		report.addCheck("config.status", false, "critical", "configuration status is unavailable", "load and validate configuration before serving control-plane traffic")
		return
	}
	report.Summary.ConfigIssues = len(cfgStatus.Issues)
	if cfgStatus.Summary.CriticalIssues > 0 {
		report.addCheck("config.critical_issues", false, "critical", "configuration has critical deployment issues", "run agent-ledger config status --format markdown and fix critical issues")
		return
	}
	if cfgStatus.Summary.WarningIssues > 0 {
		report.addCheck("config.warnings", false, "warning", "configuration has warnings", "review agent-ledger config status --format markdown")
		return
	}
	report.addCheck("config.status", true, "critical", "configuration status has no critical issues", "")
}

func addRuntimeCheck(report *ReadinessReport, runtime *storage.RuntimeStatus) {
	if runtime == nil {
		report.addCheck("runtime.status", false, "critical", "runtime status is unavailable", "inspect process startup and RBAC configuration")
		return
	}
	if runtime.ReadOnly {
		report.addCheck("runtime.observer_mode", true, "info", "runtime is ready in observer mode; write operations are disabled by design", "")
		return
	}
	report.addCheck("runtime.control_plane_mode", true, "critical", "runtime is ready in control-plane mode", "")
}

func addContractCheck(report *ReadinessReport, contract integrations.ContractVerificationReport) {
	report.Summary.ContractChecks = contract.Checked
	report.Summary.ContractFailures = contract.Failed
	if contract.Contract == "" {
		report.addCheck("contracts.verification", false, "critical", "contract verification report is missing", "regenerate control-plane contract verification")
		return
	}
	if !contract.OK || contract.Failed > 0 {
		report.addCheck("contracts.verification", false, "critical", "control-plane contract verification failed", "run agent-ledger contracts verify and inspect failed checks")
		return
	}
	report.addCheck("contracts.verification", true, "critical", "control-plane contracts verify successfully", "")
}

func addIngestionChecks(report *ReadinessReport, db *storage.DB) {
	if db == nil {
		return
	}
	health, err := db.GetIngestionHealth()
	if err != nil {
		report.addCheck("ingestion.health_query", false, "warning", "ingestion health query failed", "inspect ingestion_health table and SQLite schema")
		return
	}
	report.Summary.HealthSources = len(health)
	if len(health) == 0 {
		report.addCheck("ingestion.health_present", false, "warning", "no ingestion health has been recorded yet", "run a scan or wait for the first background collector interval")
		return
	}
	for _, row := range health {
		if strings.TrimSpace(row.LastError) != "" {
			report.Summary.HealthErrors++
		}
	}
	if report.Summary.HealthErrors > 0 {
		report.addCheck("ingestion.source_errors", false, "warning", "one or more sources report recent ingestion errors", "open the ingestion health panel or GET /api/health/ingestion")
		return
	}
	report.addCheck("ingestion.health_present", true, "info", "ingestion health is recorded without recent source errors", "")
}

func addPricingChecks(report *ReadinessReport, db *storage.DB, cfg *config.Config) {
	if db == nil {
		return
	}
	staleAfter := 24 * time.Hour
	if cfg != nil && cfg.Pricing.StaleAfter > 0 {
		staleAfter = cfg.Pricing.StaleAfter
	}
	sources, err := db.GetPricingSources(staleAfter)
	if err != nil {
		report.addCheck("pricing.sources_query", false, "warning", "pricing source query failed", "run agent-ledger pricing sync and inspect pricing migrations")
		return
	}
	report.Summary.PricingSources = len(sources)
	if len(sources) == 0 {
		report.addCheck("pricing.sources_present", false, "warning", "no pricing sources have been recorded yet", "run agent-ledger pricing sync")
		return
	}
	for _, source := range sources {
		if source.Stale {
			report.Summary.PricingStale++
		}
		if strings.TrimSpace(source.LastError) != "" || strings.EqualFold(source.Status, "error") {
			report.Summary.PricingErrors++
		}
	}
	if report.Summary.PricingErrors > 0 {
		report.addCheck("pricing.source_errors", false, "warning", "one or more pricing sources report errors", "run agent-ledger pricing sync and inspect pricing status")
		return
	}
	if report.Summary.PricingStale > 0 {
		report.addCheck("pricing.source_freshness", false, "warning", "one or more pricing sources are stale", "run agent-ledger pricing sync")
		return
	}
	report.addCheck("pricing.sources_present", true, "info", "pricing sources are present and fresh", "")
}

func (r *ReadinessReport) addCheck(name string, ok bool, severity, message, action string) {
	r.Checks = append(r.Checks, ReadinessCheck{
		Name:     name,
		OK:       ok,
		Severity: severity,
		Message:  message,
		Action:   action,
	})
}

func (r *ReadinessReport) finalize() {
	r.Summary.TotalChecks = len(r.Checks)
	r.Summary.PassingChecks = 0
	r.Summary.CriticalFailures = 0
	r.Summary.Warnings = 0
	r.Summary.Info = 0
	for _, check := range r.Checks {
		if check.OK {
			r.Summary.PassingChecks++
			continue
		}
		switch strings.ToLower(check.Severity) {
		case "critical":
			r.Summary.CriticalFailures++
		case "warning":
			r.Summary.Warnings++
		default:
			r.Summary.Info++
		}
	}
	switch {
	case r.Summary.CriticalFailures > 0:
		r.Status = "not_ready"
		r.Summary.Recommendation = "fix critical checks before connecting agent routers or write-capable wrappers"
	case r.Summary.Warnings > 0:
		r.Status = "degraded"
		r.Summary.Recommendation = "ready for local use with warnings; review warning checks before team rollout"
	default:
		r.Status = "ready"
		r.Summary.Recommendation = "ready for local control-plane use"
	}
}

func defaultRuntimeStatus() *storage.RuntimeStatus {
	return integrations.EnrichRuntimeStatus(&storage.RuntimeStatus{
		Mode:            "control-plane",
		ReadOnly:        false,
		WriteOperations: "enabled",
		BackgroundTasks: "enabled",
		Message:         "write operations and background collectors are enabled",
	}, integrations.Options{})
}

func elapsedBucket(raw string, now time.Time) string {
	t, ok := parseReadinessTime(raw)
	if !ok || now.Before(t) {
		return "none"
	}
	return durationBucket(now.Sub(t))
}

func remainingBucket(raw string, now time.Time) string {
	t, ok := parseReadinessTime(raw)
	if !ok || !t.After(now) {
		return "none"
	}
	return durationBucket(t.Sub(now))
}

func durationBucket(d time.Duration) string {
	switch {
	case d < 5*time.Minute:
		return "under_5m"
	case d < time.Hour:
		return "under_1h"
	case d < 6*time.Hour:
		return "under_6h"
	case d < 24*time.Hour:
		return "under_24h"
	default:
		return "over_24h"
	}
}

func parseReadinessTime(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
		"2006-01-02 15:04:05",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, raw); err == nil {
			return t.UTC(), true
		}
	}
	if len(raw) >= 19 {
		if t, err := time.Parse("2006-01-02 15:04:05", strings.Replace(raw[:19], "T", " ", 1)); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}
