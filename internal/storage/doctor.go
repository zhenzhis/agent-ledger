package storage

import (
	"fmt"
	"strings"
	"time"
)

// DoctorCheck is one actionable diagnostic result.
type DoctorCheck struct {
	Name     string `json:"name"`
	Source   string `json:"source,omitempty"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Action   string `json:"action"`
}

// DoctorReport combines ingestion, pricing, quality, and usage checks.
type DoctorReport struct {
	GeneratedAt    string                `json:"generated_at"`
	From           string                `json:"from"`
	To             string                `json:"to"`
	Stats          DashboardStats        `json:"stats"`
	Ingestion      []IngestionHealth     `json:"ingestion"`
	Quality        *DataQualityReport    `json:"quality"`
	Projection     *ProjectionQuality    `json:"projection"`
	WorkloadStates []WorkloadState       `json:"workload_states,omitempty"`
	PricingSources []PricingSourceStatus `json:"pricing_sources"`
	Checks         []DoctorCheck         `json:"checks"`
	Summary        string                `json:"summary"`
	Runtime        *RuntimeStatus        `json:"runtime,omitempty"`
}

// GetDoctorReport returns local diagnostics for "why is my data wrong or empty?"
func (d *DB) GetDoctorReport(from, to time.Time, staleAfter time.Duration, source, model, project string) (*DoctorReport, error) {
	from, to = utcRange(from, to)
	stats, err := d.GetDashboardStatsFiltered(from, to, source, model, project)
	if err != nil {
		return nil, err
	}
	health, err := d.GetIngestionHealth()
	if err != nil {
		return nil, err
	}
	quality, err := d.GetDataQuality(staleAfter)
	if err != nil {
		return nil, err
	}
	pricingSources, err := d.GetPricingSources(staleAfter)
	if err != nil {
		return nil, err
	}
	projection, err := d.GetProjectionQuality(from, to, source, model, project)
	if err != nil {
		return nil, err
	}
	workloadStates, err := d.GetWorkloadStates(from, to, source, model, project, 20, 10*time.Minute)
	if err != nil {
		return nil, err
	}
	report := &DoctorReport{
		GeneratedAt:    time.Now().UTC().Format(time.RFC3339),
		From:           from.Format(time.RFC3339),
		To:             to.Format(time.RFC3339),
		Stats:          *stats,
		Ingestion:      health,
		Quality:        quality,
		Projection:     projection,
		WorkloadStates: workloadStates,
		PricingSources: pricingSources,
	}
	report.Checks = append(report.Checks, usageDoctorChecks(*stats, source, model, project)...)
	report.Checks = append(report.Checks, ingestionDoctorChecks(health, source)...)
	report.Checks = append(report.Checks, pricingDoctorChecks(pricingSources, quality)...)
	report.Checks = append(report.Checks, provenanceDoctorChecks(quality)...)
	report.Checks = append(report.Checks, projectionDoctorChecks(projection)...)
	report.Checks = append(report.Checks, workloadStateDoctorChecks(workloadStates)...)
	report.Summary = doctorSummary(report.Checks)
	if len(report.Checks) == 0 {
		report.Checks = append(report.Checks, DoctorCheck{
			Name: "doctor.ok", Status: "ok", Severity: "ok",
			Message: "no local usage, ingestion, pricing, or quality issue detected",
			Action:  "continue monitoring the selected window",
		})
		report.Summary = "ok"
	}
	return report, nil
}

func provenanceDoctorChecks(quality *DataQualityReport) []DoctorCheck {
	if quality == nil || quality.Provenance == nil {
		return []DoctorCheck{{
			Name: "provenance.unavailable", Status: "warning", Severity: "warning",
			Message: "canonical event provenance quality could not be calculated",
			Action:  "run doctor again and inspect database access errors if this persists",
		}}
	}
	provenance := quality.Provenance
	if provenance.Events == 0 {
		return nil
	}
	if provenance.Confidence >= 0.85 {
		return nil
	}
	return []DoctorCheck{{
		Name:     "provenance.incomplete",
		Status:   "warning",
		Severity: "warning",
		Message:  provenance.Message,
		Action:   "update source adapters to set schema_version, source_version, parser_version, raw_ref, and match_type without storing prompt content",
	}}
}

func workloadStateDoctorChecks(states []WorkloadState) []DoctorCheck {
	var checks []DoctorCheck
	for _, state := range states {
		switch state.Phase {
		case "stale":
			checks = append(checks, DoctorCheck{
				Name: "workload.stale", Status: "warning", Severity: "warning",
				Message: fmt.Sprintf("%s has %d stale active runs", state.WorkloadID, state.StaleRuns),
				Action:  "inspect workload state, run liveness, and recent heartbeat events",
			})
		case "blocked":
			checks = append(checks, DoctorCheck{
				Name: "workload.blocked", Status: "critical", Severity: "critical",
				Message: fmt.Sprintf("%s has %d blocking policy decisions", state.WorkloadID, state.PolicyBlocks),
				Action:  "resolve the local policy decision before continuing the workload",
			})
		case "needs_approval":
			checks = append(checks, DoctorCheck{
				Name: "workload.needs_approval", Status: "warning", Severity: "warning",
				Message: fmt.Sprintf("%s has %d approval-required policy decisions", state.WorkloadID, state.PolicyApprovalsRequired),
				Action:  "approve, reject, or revise the guarded action",
			})
		case "needs_evaluation":
			checks = append(checks, DoctorCheck{
				Name: "workload.needs_evaluation", Status: "info", Severity: "info",
				Message: fmt.Sprintf("%s has artifacts but no evaluation signal", state.WorkloadID),
				Action:  "record a test, review, quality, or acceptance signal",
			})
		}
		if state.EstimatedBudgetExhausted {
			checks = append(checks, DoctorCheck{
				Name: "workload.budget_exhausted", Status: "warning", Severity: "warning",
				Message: fmt.Sprintf("%s estimated cost $%.4f reached budget $%.4f", state.WorkloadID, state.CostUSD, state.BudgetUSD),
				Action:  "review budget policy before starting additional runs",
			})
		}
	}
	return checks
}

func projectionDoctorChecks(projection *ProjectionQuality) []DoctorCheck {
	if projection == nil {
		return []DoctorCheck{{
			Name: "projection.unavailable", Status: "warning", Severity: "warning",
			Message: "ledger projection quality could not be calculated",
			Action:  "run doctor again and inspect database access errors if this persists",
		}}
	}
	var checks []DoctorCheck
	if projection.MissingUsageProjection > 0 {
		checks = append(checks, DoctorCheck{
			Name: "projection.missing_usage", Status: "warning", Severity: "warning",
			Message: fmt.Sprintf("%d canonical model calls do not have matching usage_records rows", projection.MissingUsageProjection),
			Action:  "run pricing recalculation or re-ingest affected canonical events after backing up the database",
		})
	}
	if projection.CostMismatchRecords > 0 {
		checks = append(checks, DoctorCheck{
			Name: "projection.cost_mismatch", Status: "warning", Severity: "warning",
			Message: fmt.Sprintf("%d canonical model calls disagree with projected usage cost by $%.4f total", projection.CostMismatchRecords, projection.CostDeltaUSD),
			Action:  "run POST /api/pricing/recalculate?mode=all or agent-ledger pricing sync followed by recalculation",
		})
	}
	if projection.DuplicateSessionOwners > 0 {
		checks = append(checks, DoctorCheck{
			Name: "projection.duplicate_owners", Status: "warning", Severity: "warning",
			Message: fmt.Sprintf("%d sessions have both legacy and canonical workload owners", projection.DuplicateSessionOwners),
			Action:  "inspect workload detail for the sessions; legacy backfill no longer creates new duplicates",
		})
	}
	return checks
}

func usageDoctorChecks(stats DashboardStats, source, model, project string) []DoctorCheck {
	if stats.TotalCalls > 0 {
		return nil
	}
	scope := strings.TrimSpace(strings.Join([]string{source, model, project}, " "))
	if scope == "" {
		scope = "selected window"
	}
	return []DoctorCheck{{
		Name: "usage.empty", Status: "warning", Severity: "warning",
		Message: fmt.Sprintf("no usage records matched %s", scope),
		Action:  "check the time range and filters, then run scan for the expected source",
	}}
}

func ingestionDoctorChecks(health []IngestionHealth, source string) []DoctorCheck {
	var checks []DoctorCheck
	if len(health) == 0 {
		return []DoctorCheck{{
			Name: "ingestion.missing", Status: "warning", Severity: "warning",
			Message: "no ingestion health has been recorded yet",
			Action:  "start the server or run a manual scan once",
		}}
	}
	for _, h := range health {
		if source != "" && h.Source != source {
			continue
		}
		if !h.Enabled {
			checks = append(checks, DoctorCheck{Name: "ingestion.disabled", Source: h.Source, Status: "info", Severity: "info", Message: "collector is disabled", Action: "enable the collector if this source should be tracked"})
			continue
		}
		if len(h.Paths) == 0 {
			checks = append(checks, DoctorCheck{Name: "ingestion.no_paths", Source: h.Source, Status: "warning", Severity: "warning", Message: "collector has no configured paths", Action: "add the source session path to config.yaml"})
		}
		if h.LastScanAt == "" {
			checks = append(checks, DoctorCheck{Name: "ingestion.never_scanned", Source: h.Source, Status: "warning", Severity: "warning", Message: "collector has not scanned yet", Action: "run manual scan or wait for scan_interval"})
		}
		if strings.TrimSpace(h.LastError) != "" {
			checks = append(checks, DoctorCheck{Name: "ingestion.error", Source: h.Source, Status: "critical", Severity: "critical", Message: h.LastError, Action: "fix the collector error, then rescan this source"})
		}
		for _, path := range h.PathStatus {
			if !path.Exists {
				checks = append(checks, DoctorCheck{Name: "path.missing", Source: h.Source, Status: "critical", Severity: "critical", Message: path.Path + " does not exist", Action: "verify the agent is installed or remove the mount/path from config"})
			} else if !path.Readable {
				msg := path.Path + " is not readable"
				if path.Error != "" {
					msg += ": " + path.Error
				}
				checks = append(checks, DoctorCheck{Name: "path.unreadable", Source: h.Source, Status: "critical", Severity: "critical", Message: msg, Action: "fix local permissions or Docker mount user mapping"})
			}
		}
	}
	return checks
}

func pricingDoctorChecks(sources []PricingSourceStatus, quality *DataQualityReport) []DoctorCheck {
	var checks []DoctorCheck
	if len(sources) == 0 {
		checks = append(checks, DoctorCheck{Name: "pricing.missing", Status: "warning", Severity: "warning", Message: "no pricing source status is available", Action: "run pricing sync"})
	}
	for _, src := range sources {
		if src.Status == "error" {
			checks = append(checks, DoctorCheck{Name: "pricing.error", Source: src.Name, Status: "critical", Severity: "critical", Message: src.LastError, Action: "check network access or pricing override configuration"})
		} else if src.Stale {
			checks = append(checks, DoctorCheck{Name: "pricing.stale", Source: src.Name, Status: "warning", Severity: "warning", Message: "pricing source is stale", Action: "run pricing sync or verify offline pricing policy"})
		}
	}
	if quality != nil && len(quality.UnpricedModels) > 0 {
		checks = append(checks, DoctorCheck{Name: "pricing.unpriced_models", Status: "warning", Severity: "warning", Message: fmt.Sprintf("%d model groups are unpriced", len(quality.UnpricedModels)), Action: "add local overrides or sync pricing sources"})
	}
	return checks
}

func doctorSummary(checks []DoctorCheck) string {
	critical, warning := 0, 0
	for _, check := range checks {
		switch check.Severity {
		case "critical":
			critical++
		case "warning":
			warning++
		}
	}
	switch {
	case critical > 0:
		return fmt.Sprintf("critical: %d critical, %d warning", critical, warning)
	case warning > 0:
		return fmt.Sprintf("warning: %d warning", warning)
	default:
		return "ok"
	}
}

// FormatDoctorMarkdown renders a local diagnostic report.
func FormatDoctorMarkdown(report *DoctorReport) string {
	if report == nil {
		return "# Agent Ledger Doctor\n\nNo report.\n"
	}
	var b strings.Builder
	b.WriteString("# Agent Ledger Doctor\n\n")
	b.WriteString(fmt.Sprintf("- Summary: `%s`\n", report.Summary))
	b.WriteString(fmt.Sprintf("- Window: `%s` to `%s`\n", report.From, report.To))
	b.WriteString(fmt.Sprintf("- Calls: `%d`\n- Tokens: `%d`\n- Cost: `$%.4f`\n\n", report.Stats.TotalCalls, report.Stats.TotalTokens, report.Stats.TotalCost))
	if report.Runtime != nil {
		b.WriteString(fmt.Sprintf("- Runtime: `%s` (%s)\n\n", sanitizeMarkdownCell(report.Runtime.Mode), sanitizeMarkdownCell(report.Runtime.Message)))
	}
	if report.Projection != nil {
		b.WriteString(fmt.Sprintf("- Projection: `%s` (`%.2f` confidence)\n\n", sanitizeMarkdownCell(report.Projection.Message), report.Projection.Confidence))
	}
	if report.Quality != nil && report.Quality.Provenance != nil {
		b.WriteString(fmt.Sprintf("- Provenance: `%s` (`%.2f` confidence)\n\n", sanitizeMarkdownCell(report.Quality.Provenance.Message), report.Quality.Provenance.Confidence))
	}
	if len(report.WorkloadStates) > 0 {
		b.WriteString("## Workload States\n\n| Workload | Phase | Readiness | Progress | Next Action |\n|---|---|---:|---:|---|\n")
		for _, state := range report.WorkloadStates {
			b.WriteString(fmt.Sprintf("| %s | %s | %.0f%% | %.0f%% | %s |\n",
				shortID(state.WorkloadID), sanitizeMarkdownCell(state.Phase), state.ReadinessScore*100, state.Progress*100, sanitizeMarkdownCell(state.NextAction)))
		}
		b.WriteString("\n")
	}
	b.WriteString("## Checks\n\n| Severity | Check | Source | Message | Action |\n|---|---|---|---|---|\n")
	for _, check := range report.Checks {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s |\n", check.Severity, check.Name, firstDoctorValue(check.Source, "-"), sanitizeMarkdownCell(check.Message), sanitizeMarkdownCell(check.Action)))
	}
	return b.String()
}

func firstDoctorValue(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func sanitizeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}
