package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/collector"
	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	"github.com/zhenzhis/agent-ledger/internal/mcp"
	ledgerpolicy "github.com/zhenzhis/agent-ledger/internal/policy"
	"github.com/zhenzhis/agent-ledger/internal/pricing"
	"github.com/zhenzhis/agent-ledger/internal/reconciliation"
	"github.com/zhenzhis/agent-ledger/internal/server"
	"github.com/zhenzhis/agent-ledger/internal/storage"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

type collectorEntry struct {
	source string
	name   string
	c      collector.Collector
	cfg    config.CollectorConfig
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf("Agent Ledger %s (agent-ledger binary, commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	configPath := flag.String("config", "", "path to config file")
	flag.Parse()

	cfg, err := config.Load(config.ResolveConfigPath(*configPath))
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	db, err := storage.Open(cfg.Storage.Path)
	if err != nil {
		log.Fatalf("storage: %v", err)
	}
	defer db.Close()
	db.SetProjectOptions(cfg.Projects.Aliases, cfg.Projects.Exclude)
	if flag.NArg() > 0 {
		if err := runCLI(flag.Args(), cfg, db); err != nil {
			log.Fatalf("cli: %v", err)
		}
		return
	}

	// Check if version changed — if so, reset scan state to force full re-scan
	// (needed when prompt counting logic or other parsing changes)
	lastVer, _ := db.GetMeta("version")
	if lastVer != "" && lastVer != version {
		log.Printf("version changed (%s -> %s), resetting scan state for full re-scan", lastVer, version)
		if err := db.ResetScanState(); err != nil {
			log.Printf("reset scan state: %v", err)
		}
	}
	db.SetMeta("version", version)

	// Sync pricing
	log.Println("syncing pricing data...")
	if err := pricing.SyncWithConfig(db, cfg.Pricing); err != nil {
		log.Printf("pricing sync failed: %v (continuing without pricing)", err)
	}

	// Calculate costs for existing records
	if err := recalcCostsMode(db, "zero"); err != nil {
		log.Printf("recalc costs: %v", err)
	}

	// Collector loop
	collectors := []collectorEntry{
		{"claude", "Claude Code", collector.NewClaudeCollector(db, cfg.Collectors.Claude.Paths), cfg.Collectors.Claude},
		{"codex", "Codex", collector.NewCodexCollector(db, cfg.Collectors.Codex.Paths), cfg.Collectors.Codex},
		{"openclaw", "OpenClaw", collector.NewOpenClawCollector(db, cfg.Collectors.OpenClaw.Paths), cfg.Collectors.OpenClaw},
		{"opencode", "OpenCode", collector.NewOpenCodeCollector(db, cfg.Collectors.OpenCode.Paths), cfg.Collectors.OpenCode},
		{"kiro", "kiro", collector.NewKiroCollector(db, cfg.Collectors.Kiro.Paths), cfg.Collectors.Kiro},
		{"pi", "Pi", collector.NewPiCollector(db, cfg.Collectors.Pi.Paths), cfg.Collectors.Pi},
	}
	collectorBySource := map[string]collectorEntry{}
	sourceOptions := make([]server.SourceOption, 0, len(collectors))
	for _, ce := range collectors {
		collectorBySource[ce.source] = ce
		sourceOptions = append(sourceOptions, server.SourceOption{Source: ce.source, Enabled: ce.cfg.Enabled, Paths: ce.cfg.Paths})
		if !ce.cfg.Enabled {
			recordDisabledHealth(db, ce)
		}
	}
	var scanMu sync.Mutex
	scanSource := func(source string, reset bool) error {
		scanMu.Lock()
		defer scanMu.Unlock()
		if source == "" {
			for _, ce := range collectors {
				if ce.cfg.Enabled {
					if err := scanCollector(db, ce, false); err != nil {
						return err
					}
				}
			}
			if err := recalcCostsMode(db, "zero"); err != nil {
				return err
			}
			return nil
		}
		ce, ok := collectorBySource[source]
		if !ok {
			return fmt.Errorf("unknown source %q", source)
		}
		if !ce.cfg.Enabled {
			return fmt.Errorf("source %q is disabled", source)
		}
		if reset {
			if err := db.ResetSource(source, ce.cfg.Paths); err != nil {
				return err
			}
		}
		if err := scanCollector(db, ce, reset); err != nil {
			return err
		}
		if err := recalcCostsMode(db, "zero"); err != nil {
			return err
		}
		return nil
	}
	for _, ce := range collectors {
		if !ce.cfg.Enabled {
			continue
		}
		log.Printf("scanning %s sessions...", ce.name)
		if err := scanCollector(db, ce, false); err != nil {
			log.Printf("%s scan: %v", ce.name, err)
		}
		if err := recalcCostsMode(db, "zero"); err != nil {
			log.Printf("recalc costs: %v", err)
		}

		go func(ce collectorEntry) {
			interval := ce.cfg.ScanInterval
			if interval <= 0 {
				interval = 60 * time.Second
			}
			ticker := time.NewTicker(interval)
			for range ticker.C {
				scanMu.Lock()
				err := scanCollector(db, ce, false)
				scanMu.Unlock()
				if err != nil {
					log.Printf("%s scan: %v", ce.name, err)
				}
				if err := recalcCostsMode(db, "zero"); err != nil {
					log.Printf("recalc costs: %v", err)
				}
			}
		}(ce)
	}

	// Periodic pricing sync
	go func() {
		ticker := time.NewTicker(cfg.Pricing.SyncInterval)
		for range ticker.C {
			if err := pricing.SyncWithConfig(db, cfg.Pricing); err != nil {
				log.Printf("pricing sync failed: %v", err)
			}
			if err := recalcCostsMode(db, "zero"); err != nil {
				log.Printf("recalc costs: %v", err)
			}
		}
	}()

	// Start web server
	addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.Port)
	srv := server.New(db, addr, server.Options{
		AuthToken:    cfg.Server.AuthToken,
		AdminToken:   cfg.Server.AdminToken,
		ViewerToken:  cfg.Server.ViewerToken,
		RBAC:         cfg.RBAC,
		Privacy:      cfg.Privacy,
		Budgets:      cfg.Budgets,
		Quota:        cfg.Quota,
		Watchdog:     cfg.Watchdog,
		Policies:     cfg.Policies,
		Webhooks:     cfg.Webhooks,
		Teams:        cfg.Teams,
		Integrations: cfg.Integrations,
		Gateway:      cfg.Gateway,
		Pricing:      cfg.Pricing,
		Sources:      sourceOptions,
		Scan:         scanSource,
		Recalc:       func() error { return recalcCostsMode(db, "zero") },
		RecalcMode:   func(mode string) error { return recalcCostsMode(db, mode) },
		PricingSync:  func() error { return pricing.SyncWithConfig(db, cfg.Pricing) },
	})
	log.Fatal(srv.Start())
}

func recalcCosts(db *storage.DB) error {
	return recalcCostsMode(db, "zero")
}

func recalcCostsMode(db *storage.DB, mode string) error {
	prices, err := db.GetAllPricing()
	if err != nil {
		return err
	}
	if detailed, err := db.GetAllPricingDetailed(); err == nil && len(detailed) > 0 {
		if err := db.RecalcCostsDetailed(detailed, pricing.CalcCost, mode, false); err != nil {
			return err
		}
		return refreshDerivedLedgers(db)
	}
	if err := db.RecalcCostsMode(prices, pricing.CalcCost, mode); err != nil {
		return err
	}
	return refreshDerivedLedgers(db)
}

func refreshDerivedLedgers(db *storage.DB) error {
	if err := db.RebuildUsageAggregates(); err != nil {
		return err
	}
	return db.BackfillWorkloadsFromUsage(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC), time.Now().UTC().AddDate(10, 0, 0))
}

func scanCollector(db *storage.DB, ce collectorEntry, reset bool) error {
	beforeRecords, _ := db.CountUsageRecords(ce.source)
	beforePrompts, _ := db.CountPromptEvents(ce.source)
	start := time.Now()
	err := ce.c.Scan()
	afterRecords, _ := db.CountUsageRecords(ce.source)
	afterPrompts, _ := db.CountPromptEvents(ce.source)
	filesSeen, watermark, _ := db.FileStateStats(ce.cfg.Paths)
	lastError := ""
	if err != nil {
		lastError = err.Error()
	}
	health := storage.IngestionHealth{
		Source:          ce.source,
		Enabled:         ce.cfg.Enabled,
		Paths:           ce.cfg.Paths,
		PathStatus:      inspectPaths(ce.cfg.Paths),
		LastScanAt:      time.Now().UTC().Format(time.RFC3339),
		DurationMS:      time.Since(start).Milliseconds(),
		Watermark:       watermark,
		FilesSeen:       filesSeen,
		RecordsInserted: maxInt(0, afterRecords-beforeRecords),
		PromptsInserted: maxInt(0, afterPrompts-beforePrompts),
		SkippedRows:     0,
		LastError:       lastError,
	}
	if reset && health.LastError == "" {
		health.LastError = "scan state reset before scan"
	}
	if hErr := db.UpsertIngestionHealth(health); hErr != nil {
		log.Printf("%s health update: %v", ce.name, hErr)
	}
	_ = db.AppendAuditLog("local", "operator", "collector.scan", ce.source, map[string]string{"reset": fmt.Sprint(reset), "error": lastError})
	return err
}

func recordDisabledHealth(db *storage.DB, ce collectorEntry) {
	if err := db.UpsertIngestionHealth(storage.IngestionHealth{
		Source:     ce.source,
		Enabled:    false,
		Paths:      ce.cfg.Paths,
		PathStatus: inspectPaths(ce.cfg.Paths),
		LastError:  "collector disabled",
	}); err != nil {
		log.Printf("%s health update: %v", ce.name, err)
	}
}

func inspectPaths(paths []string) []storage.PathStatus {
	result := make([]storage.PathStatus, 0, len(paths))
	for _, p := range paths {
		status := storage.PathStatus{Path: p}
		info, err := os.Stat(p)
		if err != nil {
			status.Error = err.Error()
			result = append(result, status)
			continue
		}
		status.Exists = true
		if info.IsDir() {
			_, err = os.ReadDir(p)
		} else {
			var f *os.File
			f, err = os.Open(p)
			if f != nil {
				_ = f.Close()
			}
		}
		if err != nil {
			status.Error = err.Error()
		} else {
			status.Readable = true
		}
		result = append(result, status)
	}
	return result
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func runCLI(args []string, cfg *config.Config, db *storage.DB) error {
	cmd := args[0]
	now := time.Now()
	dayFrom := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
	dayTo := dayFrom.Add(24 * time.Hour)
	switch cmd {
	case "today":
		stats, err := db.GetDashboardStatsFiltered(dayFrom, dayTo, "", "", "")
		if err != nil {
			return err
		}
		fmt.Printf("Agent Ledger today: tokens=%d cost=$%.4f sessions=%d prompts=%d calls=%d cache=%.1f%%\n",
			stats.TotalTokens, stats.TotalCost, stats.TotalSessions, stats.TotalPrompts, stats.TotalCalls, stats.CacheHitRate*100)
	case "top":
		rows, err := db.GetCostIntelligence(dayFrom.AddDate(0, 0, -30), dayTo, "", "", "", 10)
		if err != nil {
			return err
		}
		for _, row := range rows {
			fmt.Printf("%s\t%s\t%s\t%s\t$%.4f\t%d tokens\t%s\n", row.Source, row.Project, row.GitBranch, row.SessionID, row.CostUSD, row.Tokens, row.LastActivity)
		}
	case "doctor":
		from, to, err := cliDateRange(args[1:], now)
		if err != nil {
			return err
		}
		report, err := db.GetDoctorReport(from, to, cfg.Pricing.StaleAfter, cliValue(args[1:], "--source"), cliValue(args[1:], "--model"), cliValue(args[1:], "--project"))
		if err != nil {
			return err
		}
		if cliBool(args[1:], "--privacy") {
			redactDoctorReport(report)
		}
		if strings.EqualFold(cliValue(args[1:], "--format"), "markdown") {
			_, err = os.Stdout.Write([]byte(storage.FormatDoctorMarkdown(report)))
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	case "battery":
		stats, err := db.GetDashboardStatsFiltered(dayFrom, dayTo, "", "", "")
		if err != nil {
			return err
		}
		remaining := cfg.Quota.MonthlyBudget/30 - stats.TotalCost
		fmt.Printf("Agent Ledger battery: plan=%s today=$%.4f remaining_estimate=$%.4f tokens=%d method=local-estimate\n",
			cfg.Quota.Plan, stats.TotalCost, remaining, stats.TotalTokens)
	case "export":
		page, err := db.GetSessionsPage(dayFrom.AddDate(0, 0, -30), dayTo, "", "", "", 500, 0)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(page.Rows)
	case "pricing":
		if len(args) > 1 && args[1] == "sync" {
			if err := pricing.SyncWithConfig(db, cfg.Pricing); err != nil {
				return err
			}
			return recalcCostsMode(db, "zero")
		}
		rows, err := db.GetPricingSources(cfg.Pricing.StaleAfter)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(rows)
	case "wrapped":
		return runAgentWrappedCLI(args[1:], db)
	case "workload":
		return runWorkloadCLI(args[1:], db)
	case "run":
		return runWrappedCLI(args[1:], db)
	case "event":
		return runEventCLI(args[1:], db)
	case "bundle":
		return runBundleCLI(args[1:], db)
	case "policy":
		return runPolicyCLI(args[1:], cfg, db)
	case "reconcile":
		return runReconcileCLI(args[1:], db)
	case "router":
		return runRouterCLI(args[1:], db)
	case "replay":
		return runReplayCLI(args[1:], db)
	case "badge":
		return runBadgeCLI(args[1:], db)
	case "preflight":
		return runPreflightCLI(args[1:], db)
	case "chargeback":
		return runChargebackCLI(args[1:], cfg, db)
	case "fleet":
		return runFleetCLI(args[1:], db)
	case "integrations":
		return json.NewEncoder(os.Stdout).Encode(integrations.Registry(integrations.OptionsFromConfig(cfg)))
	case "otel":
		return runOTelCLI(args[1:], db)
	case "a2a":
		return runA2ACLI(args[1:], db)
	case "provider":
		return runProviderCLI(args[1:], db)
	case "projection":
		return runProjectionCLI(args[1:], db)
	case "mcp":
		return mcp.New(db, cfg).Serve(os.Stdin, os.Stdout)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	return nil
}

func runChargebackCLI(args []string, cfg *config.Config, db *storage.DB) error {
	now := time.Now()
	from, to, err := cliDateRange(args, now)
	if err != nil {
		return err
	}
	limit := 200
	if raw := cliValue(args, "--limit"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err != nil {
			return fmt.Errorf("invalid --limit %q: %w", raw, err)
		}
		limit = parsed
	}
	rows, err := db.GetChargeback(from, to, cliValue(args, "--source"), cliValue(args, "--model"), cliValue(args, "--project"),
		cfg.Teams.Groups, cfg.Teams.MachineName, cfg.Teams.GitAuthor, limit)
	if err != nil {
		return err
	}
	if strings.EqualFold(cliValue(args, "--format"), "csv") {
		w := csv.NewWriter(os.Stdout)
		if err := w.Write([]string{"team", "project", "source", "model", "calls", "sessions", "tokens", "cost_usd", "mapping_source", "data_source", "confidence"}); err != nil {
			return err
		}
		for _, row := range rows {
			if err := w.Write([]string{
				row.Team, row.Project, row.Source, row.Model,
				fmt.Sprint(row.Calls), fmt.Sprint(row.Sessions), fmt.Sprint(row.Tokens), fmt.Sprintf("%.6f", row.CostUSD),
				row.MappingSource, row.DataSource, fmt.Sprintf("%.2f", row.Confidence),
			}); err != nil {
				return err
			}
		}
		w.Flush()
		return w.Error()
	}
	return json.NewEncoder(os.Stdout).Encode(rows)
}

func runFleetCLI(args []string, db *storage.DB) error {
	now := time.Now()
	from, to, err := cliDateRange(args, now)
	if err != nil {
		return err
	}
	limit := 100
	if raw := cliValue(args, "--limit"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err != nil {
			return fmt.Errorf("invalid --limit %q: %w", raw, err)
		}
		limit = parsed
	}
	report, err := db.GetFleetAttribution(from, to, cliValue(args, "--source"), cliValue(args, "--model"), cliValue(args, "--project"), limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func runAgentWrappedCLI(args []string, db *storage.DB) error {
	period, from, to, err := wrappedCLIWindow(args, time.Now())
	if err != nil {
		return err
	}
	report, err := db.GetAgentWrapped(from, to, period, cliValue(args, "--source"), cliValue(args, "--model"), cliValue(args, "--project"))
	if err != nil {
		return err
	}
	if cliBool(args, "--privacy") {
		redactWrappedReport(report)
	}
	if strings.EqualFold(cliValue(args, "--format"), "json") {
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	_, err = os.Stdout.Write([]byte(storage.FormatWrappedMarkdown(report)))
	return err
}

func wrappedCLIWindow(args []string, now time.Time) (string, time.Time, time.Time, error) {
	if cliValue(args, "--from") != "" || cliValue(args, "--to") != "" {
		from, to, err := cliDateRange(args, now)
		return firstNonEmptyCLI(cliValue(args, "--period"), "custom"), from, to, err
	}
	period := strings.ToLower(firstNonEmptyCLI(cliValue(args, "--period"), "month"))
	switch period {
	case "week", "weekly":
		d := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
		d = d.AddDate(0, 0, -int((d.Weekday()+6)%7))
		return "weekly", d, now, nil
	case "year", "yearly", "annual":
		return "yearly", time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.Local), now, nil
	case "month", "monthly":
		return "monthly", time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local), now, nil
	default:
		return period, now.AddDate(0, -1, 0), now, nil
	}
}

func redactWrappedReport(report *storage.WrappedReport) {
	if report == nil {
		return
	}
	report.TopProject.Project = "<redacted>"
	report.MostExpensiveSession.SessionID = "<redacted>"
	report.MostExpensiveSession.Project = "<redacted>"
	report.MostExpensiveSession.GitBranch = "<redacted>"
	for i := range report.Highlights {
		if report.Highlights[i].Label == "top project" {
			report.Highlights[i].Value = "<redacted>"
		}
	}
}

func redactDoctorReport(report *storage.DoctorReport) {
	if report == nil {
		return
	}
	for i := range report.Ingestion {
		for j := range report.Ingestion[i].Paths {
			report.Ingestion[i].Paths[j] = "<redacted>"
		}
		for j := range report.Ingestion[i].PathStatus {
			report.Ingestion[i].PathStatus[j].Path = "<redacted>"
		}
	}
	for i := range report.Checks {
		if report.Checks[i].Name == "path.missing" || report.Checks[i].Name == "path.unreadable" {
			report.Checks[i].Message = "<redacted path>"
		}
	}
}

func runReconcileCLI(args []string, db *storage.DB) error {
	if len(args) == 0 || (args[0] != "status" && args[0] != "parse" && args[0] != "import") {
		return fmt.Errorf("usage: agent-ledger reconcile status|parse|import [--file bill.csv|bill.json] [--format csv|json|auto] [--provider name] [--from YYYY-MM-DD --to YYYY-MM-DD]")
	}
	if args[0] == "status" {
		rows, err := db.GetReconciliationImports(50)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(rows)
	}
	raw, err := readCLIInput(args[1:], "--file", 4<<20)
	if err != nil {
		return err
	}
	summary, err := reconciliation.ParseProviderStatement(raw, cliValue(args[1:], "--format"), cliValue(args[1:], "--provider"))
	if err != nil {
		return err
	}
	if args[0] == "parse" {
		return json.NewEncoder(os.Stdout).Encode(summary)
	}
	from, to, err := cliReconciliationWindow(args[1:], summary)
	if err != nil {
		return err
	}
	localCost, err := parseFloat(cliValue(args[1:], "--local-cost-usd"))
	if err != nil {
		return err
	}
	if localCost == 0 {
		stats, err := db.GetDashboardStatsFiltered(from, to, cliValue(args[1:], "--source"), cliValue(args[1:], "--model"), cliValue(args[1:], "--project"))
		if err != nil {
			return err
		}
		localCost = stats.TotalCost
	}
	row := storage.ReconciliationImport{
		Provider: summary.Provider, Format: summary.Format, Currency: summary.Currency,
		LocalCostUSD: localCost, ProviderCostUSD: summary.ProviderCostUSD, RowsSeen: summary.RowsSeen,
		PayloadSHA256: summary.PayloadSHA256, Warnings: reconciliation.WarningsJSON(summary.Warnings),
	}
	if !summary.WindowStart.IsZero() {
		row.WindowStart = summary.WindowStart.Format(time.RFC3339)
	}
	if !summary.WindowEnd.IsZero() {
		row.WindowEnd = summary.WindowEnd.Format(time.RFC3339)
	}
	storage.PrepareReconciliationImport(&row)
	if err := db.InsertReconciliationImportDetailed(row); err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(row)
}

func cliReconciliationWindow(args []string, summary *reconciliation.ImportSummary) (time.Time, time.Time, error) {
	now := time.Now()
	if cliValue(args, "--from") != "" || cliValue(args, "--to") != "" {
		return cliDateRange(args, now)
	}
	if summary != nil && !summary.WindowStart.IsZero() && !summary.WindowEnd.IsZero() {
		return summary.WindowStart, statementWindowEnd(summary.WindowEnd), nil
	}
	return now.AddDate(0, -1, 0), now, nil
}

func statementWindowEnd(end time.Time) time.Time {
	if end.Hour() == 0 && end.Minute() == 0 && end.Second() == 0 && end.Nanosecond() == 0 {
		return end.AddDate(0, 0, 1)
	}
	return end.Add(time.Nanosecond)
}

func runRouterCLI(args []string, db *storage.DB) error {
	if len(args) == 0 || args[0] != "simulate" {
		return fmt.Errorf("usage: agent-ledger router simulate --to-model model [--from-model model] [--ratio 0.3] [--from YYYY-MM-DD --to YYYY-MM-DD] [--source s] [--project p]")
	}
	now := time.Now()
	from, to, err := cliDateRange(args[1:], now)
	if err != nil {
		return err
	}
	toModel := firstNonEmptyCLI(cliValue(args[1:], "--to-model"), cliValue(args[1:], "--target-model"))
	if toModel == "" {
		return fmt.Errorf("--to-model is required")
	}
	ratio := 1.0
	if raw := firstNonEmptyCLI(cliValue(args[1:], "--ratio"), cliValue(args[1:], "--replacement-ratio")); raw != "" {
		ratio, err = parseFloat(raw)
		if err != nil {
			return fmt.Errorf("invalid --ratio %q: %w", raw, err)
		}
	}
	report, err := db.SimulateModelRouting(from, to, cliValue(args[1:], "--source"),
		firstNonEmptyCLI(cliValue(args[1:], "--from-model"), cliValue(args[1:], "--model")),
		toModel, cliValue(args[1:], "--project"), ratio, 200)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func runReplayCLI(args []string, db *storage.DB) error {
	sessionID := firstNonEmptyCLI(cliValue(args, "--session-id"), cliValue(args, "--session_id"), cliValue(args, "--id"))
	if sessionID == "" {
		return fmt.Errorf("usage: agent-ledger replay --session-id id [--source s] [--limit n]")
	}
	limit := 1000
	if raw := cliValue(args, "--limit"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err != nil {
			return fmt.Errorf("invalid --limit %q: %w", raw, err)
		}
		limit = parsed
	}
	report, err := db.GetSessionReplay(cliValue(args, "--source"), sessionID, limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func runBadgeCLI(args []string, db *storage.DB) error {
	now := time.Now()
	from, to, err := cliDateRange(args, now)
	if err != nil {
		return err
	}
	project := cliValue(args, "--project")
	stats, err := db.GetDashboardStatsFiltered(from, to, cliValue(args, "--source"), cliValue(args, "--model"), project)
	if err != nil {
		return err
	}
	metric := firstNonEmptyCLI(cliValue(args, "--metric"), "cost")
	value, err := server.BadgeValue(metric, stats)
	if err != nil {
		return err
	}
	label := firstNonEmptyCLI(cliValue(args, "--label"), project, "Agent Ledger")
	svg := server.RenderBadgeSVG(label, value)
	if out := cliValue(args, "--out"); out != "" {
		return os.WriteFile(out, []byte(svg), 0o644)
	}
	_, err = os.Stdout.Write([]byte(svg + "\n"))
	return err
}

func runPreflightCLI(args []string, db *storage.DB) error {
	now := time.Now()
	from, to, err := cliDateRange(args, now)
	if err != nil {
		return err
	}
	limit := 2000
	if raw := cliValue(args, "--limit"); raw != "" {
		var parsed int
		if _, err := fmt.Sscanf(raw, "%d", &parsed); err != nil {
			return fmt.Errorf("invalid --limit %q: %w", raw, err)
		}
		limit = parsed
	}
	report, err := db.EstimatePreflightCost(from, to, firstNonEmptyCLI(cliValue(args, "--task"), cliValue(args, "--type"), "custom"),
		cliValue(args, "--source"), cliValue(args, "--model"), cliValue(args, "--project"), limit)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func runProviderCLI(args []string, db *storage.DB) error {
	if len(args) == 0 || (args[0] != "convert" && args[0] != "ingest") {
		return fmt.Errorf("usage: agent-ledger provider convert|ingest [--file response.json]")
	}
	raw, err := readCLIInput(args[1:], "--file", 4<<20)
	if err != nil {
		return err
	}
	calls, err := integrations.DecodeProviderCalls(raw)
	if err != nil {
		return err
	}
	events, err := integrations.ConvertProviderCalls(calls)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("no provider usage calls found")
	}
	if args[0] == "convert" {
		return json.NewEncoder(os.Stdout).Encode(events)
	}
	results := make([]*storage.CanonicalEventResult, 0, len(events))
	for _, event := range events {
		result, err := db.IngestCanonicalEvent(event)
		if err != nil {
			return err
		}
		results = append(results, result)
	}
	return json.NewEncoder(os.Stdout).Encode(results)
}

func runProjectionCLI(args []string, db *storage.DB) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent-ledger projection quality|repair [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--source s] [--model m] [--project p]")
	}
	from, to, err := cliDateRange(args[1:], time.Now())
	if err != nil {
		return err
	}
	source := cliValue(args[1:], "--source")
	model := cliValue(args[1:], "--model")
	project := cliValue(args[1:], "--project")
	switch args[0] {
	case "quality":
		report, err := db.GetProjectionQuality(from, to, source, model, project)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	case "repair":
		result, err := db.RepairUsageProjections(from, to, source, model, project)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	default:
		return fmt.Errorf("unknown projection command %q", args[0])
	}
}

func runA2ACLI(args []string, db *storage.DB) error {
	if len(args) == 0 || (args[0] != "convert" && args[0] != "ingest") {
		return fmt.Errorf("usage: agent-ledger a2a convert|ingest [--file task.json]")
	}
	raw, err := readCLIInput(args[1:], "--file", 4<<20)
	if err != nil {
		return err
	}
	tasks, err := integrations.DecodeA2ATasks(raw)
	if err != nil {
		return err
	}
	events, err := integrations.ConvertA2ATasks(tasks)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("no A2A task events found")
	}
	if args[0] == "convert" {
		return json.NewEncoder(os.Stdout).Encode(events)
	}
	results := make([]*storage.CanonicalEventResult, 0, len(events))
	for _, event := range events {
		result, err := db.IngestCanonicalEvent(event)
		if err != nil {
			return err
		}
		results = append(results, result)
	}
	return json.NewEncoder(os.Stdout).Encode(results)
}

func runOTelCLI(args []string, db *storage.DB) error {
	if len(args) == 0 || (args[0] != "convert" && args[0] != "ingest") {
		return fmt.Errorf("usage: agent-ledger otel convert|ingest [--file spans.json]")
	}
	raw, err := readCLIInput(args[1:], "--file", 4<<20)
	if err != nil {
		return err
	}
	spans, err := integrations.DecodeOTelGenAISpans(raw)
	if err != nil {
		return err
	}
	events, err := integrations.ConvertOTelGenAISpans(spans)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("no GenAI spans found")
	}
	if args[0] == "convert" {
		return json.NewEncoder(os.Stdout).Encode(events)
	}
	results := make([]*storage.CanonicalEventResult, 0, len(events))
	for _, event := range events {
		result, err := db.IngestCanonicalEvent(event)
		if err != nil {
			return err
		}
		results = append(results, result)
	}
	return json.NewEncoder(os.Stdout).Encode(results)
}

func runPolicyCLI(args []string, cfg *config.Config, db *storage.DB) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent-ledger policy evaluate|approvals|resolve")
	}
	switch args[0] {
	case "audit":
		from, to, err := cliDateRange(args[1:], time.Now())
		if err != nil {
			return err
		}
		limit := cliInt(args[1:], "--limit", 200)
		candidates, err := db.GetPolicyAuditCandidates(from, to, cliValue(args[1:], "--source"), cliValue(args[1:], "--model"), cliValue(args[1:], "--project"), limit*5)
		if err != nil {
			return err
		}
		report := ledgerpolicy.Audit(cfg.Policies, candidates, limit)
		report.WindowFrom = from.Format(time.RFC3339)
		report.WindowTo = to.Format(time.RFC3339)
		report.Scope = "usage_records,tool_calls,workloads"
		if cliBool(args[1:], "--privacy") {
			redactPolicyAuditReport(&report)
		}
		if strings.EqualFold(cliValue(args[1:], "--format"), "markdown") {
			printPolicyAuditMarkdown(report)
			return nil
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	case "approvals":
		status := cliValue(args[1:], "--status")
		if status == "" {
			status = "pending"
		}
		rows, err := db.ListApprovalRequests(status, cliInt(args[1:], "--limit", 200))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"status": status, "rows": rows})
	case "resolve":
		requestID := firstNonEmptyCLI(cliValue(args[1:], "--id"), cliValue(args[1:], "--request-id"), cliValue(args[1:], "--request_id"))
		status := cliValue(args[1:], "--status")
		if status == "" {
			status = "approved"
		}
		if err := db.ResolveApprovalRequest(requestID, status, "cli", cliValue(args[1:], "--note")); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"ok": true, "request_id": requestID, "status": status})
	case "evaluate":
	default:
		return fmt.Errorf("usage: agent-ledger policy audit [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--format markdown|json]; agent-ledger policy evaluate [--source s] [--model m] [--project p] [--action a] [--workload-id id] [--run-id id] [--role role] [--record]; agent-ledger policy approvals [--status pending|approved|rejected|all]; agent-ledger policy resolve --id id --status approved|rejected [--note text]")
	}
	req := ledgerpolicy.Request{
		WorkloadID: firstNonEmptyCLI(cliValue(args[1:], "--workload-id"), cliValue(args[1:], "--workload_id")),
		RunID:      firstNonEmptyCLI(cliValue(args[1:], "--run-id"), cliValue(args[1:], "--run_id")),
		Source:     cliValue(args[1:], "--source"),
		Model:      cliValue(args[1:], "--model"),
		Project:    cliValue(args[1:], "--project"),
		Action:     cliValue(args[1:], "--action"),
		Role:       firstNonEmptyCLI(cliValue(args[1:], "--role"), "operator"),
	}
	result := ledgerpolicy.Evaluate(cfg.Policies, req)
	if cliBool(args[1:], "--record") && len(result.Decisions) > 0 {
		if req.WorkloadID == "" {
			return fmt.Errorf("--record requires --workload-id")
		}
		for i := range result.Decisions {
			id, err := db.RecordPolicyDecision(req.WorkloadID, req.RunID, result.Decisions[i].Rule, result.Decisions[i].Action, result.Decisions[i].Message, req.Role)
			if err != nil {
				return err
			}
			result.Decisions[i].DecisionID = id
		}
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func printPolicyAuditMarkdown(report ledgerpolicy.AuditReport) {
	fmt.Printf("# Agent Ledger Policy Audit\n\n")
	fmt.Printf("- enabled: %t\n", report.Enabled)
	fmt.Printf("- window: %s -> %s\n", report.WindowFrom, report.WindowTo)
	fmt.Printf("- checked: %d\n", report.Checked)
	fmt.Printf("- matches: %d\n", report.Matches)
	fmt.Printf("- blocks: %d\n", report.Blocks)
	fmt.Printf("- approvals: %d\n", report.Approvals)
	fmt.Printf("- warnings: %d\n\n", report.Warnings)
	if len(report.Rows) == 0 {
		fmt.Println("No policy matches.")
		return
	}
	fmt.Println("| action | kind | source | model | project | evidence |")
	fmt.Println("|---|---|---|---|---|---|")
	for _, row := range report.Rows {
		fmt.Printf("| %s | %s | %s | %s | %s | %s |\n", row.EffectiveAction, row.Kind, row.Source, row.Model, row.Project, strings.ReplaceAll(row.Evidence, "|", "/"))
	}
}

func redactPolicyAuditReport(report *ledgerpolicy.AuditReport) {
	for i := range report.Rows {
		report.Rows[i].Project = "<redacted>"
		report.Rows[i].SessionID = "<redacted>"
		report.Rows[i].WorkloadID = "<redacted>"
		report.Rows[i].RunID = "<redacted>"
		report.Rows[i].Evidence = "<redacted>"
	}
}

func runBundleCLI(args []string, db *storage.DB) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent-ledger bundle export|import")
	}
	now := time.Now()
	from, to, err := cliDateRange(args[1:], now)
	if err != nil {
		return err
	}
	key := os.Getenv("AGENT_LEDGER_BUNDLE_KEY")
	switch args[0] {
	case "export":
		signed := cliBool(args[1:], "--signed")
		if signed && key == "" {
			return fmt.Errorf("AGENT_LEDGER_BUNDLE_KEY is required for signed bundle export")
		}
		signingKey := ""
		if signed {
			signingKey = key
		}
		privacyLabel := "metadata-only"
		if cliBool(args[1:], "--privacy") {
			privacyLabel = "redacted"
		}
		_, raw, err := db.BuildOfflineBundle(from, to, cliValue(args[1:], "--source"), cliValue(args[1:], "--model"), cliValue(args[1:], "--project"),
			privacyLabel, signingKey, cliValue(args[1:], "--key-id"), 10000)
		if err != nil {
			return err
		}
		if out := cliValue(args[1:], "--out"); out != "" {
			return os.WriteFile(out, raw, 0o600)
		}
		_, err = os.Stdout.Write(raw)
		if err == nil {
			_, err = os.Stdout.Write([]byte("\n"))
		}
		return err
	case "import":
		var raw []byte
		if file := cliValue(args[1:], "--file"); file != "" {
			raw, err = os.ReadFile(file)
		} else {
			raw, err = io.ReadAll(io.LimitReader(os.Stdin, 32<<20))
		}
		if err != nil {
			return err
		}
		result, err := db.ImportOfflineBundle(raw, key, cliBool(args[1:], "--verify"))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	default:
		return fmt.Errorf("unknown bundle command %q", args[0])
	}
}

func cliDateRange(args []string, now time.Time) (time.Time, time.Time, error) {
	fromRaw := cliValue(args, "--from")
	toRaw := cliValue(args, "--to")
	if fromRaw == "" {
		from := now.AddDate(0, 0, -30)
		return from, now, nil
	}
	from, err := time.Parse("2006-01-02", fromRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	if toRaw == "" {
		return from, from.AddDate(0, 0, 1), nil
	}
	to, err := time.Parse("2006-01-02", toRaw)
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	return from, to.AddDate(0, 0, 1), nil
}

func runEventCLI(args []string, db *storage.DB) error {
	if len(args) > 0 && args[0] == "schema" {
		return json.NewEncoder(os.Stdout).Encode(storage.CanonicalEventSchema())
	}
	if len(args) == 0 || args[0] != "ingest" {
		return fmt.Errorf("usage: agent-ledger event schema | agent-ledger event ingest [--file event.json]")
	}
	raw, err := readCLIInput(args[1:], "--file", 4<<20)
	if err != nil {
		return err
	}
	events, err := decodeCLIEvents(raw)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("at least one event is required")
	}
	if len(events) > 500 {
		return fmt.Errorf("too many events: max 500")
	}
	results := make([]*storage.CanonicalEventResult, 0, len(events))
	for _, event := range events {
		result, err := db.IngestCanonicalEvent(event)
		if err != nil {
			return err
		}
		results = append(results, result)
	}
	return json.NewEncoder(os.Stdout).Encode(results)
}

func readCLIInput(args []string, fileFlag string, limit int64) ([]byte, error) {
	if path := cliValue(args, fileFlag); path != "" {
		return os.ReadFile(path)
	}
	return io.ReadAll(io.LimitReader(os.Stdin, limit))
}

func decodeCLIEvents(raw []byte) ([]storage.CanonicalEvent, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return nil, fmt.Errorf("empty event input")
	}
	if strings.HasPrefix(trimmed, "[") {
		var events []storage.CanonicalEvent
		if err := json.Unmarshal([]byte(trimmed), &events); err != nil {
			return nil, err
		}
		return events, nil
	}
	var envelope struct {
		Events []storage.CanonicalEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(trimmed), &envelope); err == nil && len(envelope.Events) > 0 {
		return envelope.Events, nil
	}
	var event storage.CanonicalEvent
	if err := json.Unmarshal([]byte(trimmed), &event); err != nil {
		return nil, err
	}
	return []storage.CanonicalEvent{event}, nil
}

func runWorkloadCLI(args []string, db *storage.DB) error {
	if len(args) == 0 || args[0] == "list" {
		now := time.Now()
		page, err := db.GetWorkloadsPage(now.AddDate(0, 0, -30), now.Add(24*time.Hour), "", "", "", "", "", 50, 0)
		if err != nil {
			return err
		}
		for _, row := range page.Rows {
			fmt.Printf("%s\t%s\t$%.4f\t%d tokens\t%s\t%s\n", row.WorkloadID, row.Status, row.CostUSD, row.Tokens, row.Source, row.Goal)
		}
		return nil
	}
	switch args[0] {
	case "create":
		goal := cliValue(args[1:], "--goal")
		budget, _ := parseFloat(cliValue(args[1:], "--budget-usd"))
		id, err := db.CreateWorkload(goal, cliValue(args[1:], "--source"), cliValue(args[1:], "--project"), cliValue(args[1:], "--repo"),
			cliValue(args[1:], "--branch"), cliValue(args[1:], "--owner"), cliValue(args[1:], "--team"), budget)
		if err != nil {
			return err
		}
		fmt.Println(id)
	case "show":
		id := firstNonEmptyCLI(cliValue(args[1:], "--id"), cliValue(args[1:], "--workload-id"))
		detail, err := db.GetWorkloadDetail(id)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(detail)
	case "timeline":
		id := firstNonEmptyCLI(cliValue(args[1:], "--id"), cliValue(args[1:], "--workload-id"))
		rows, err := db.GetWorkloadTimeline(id, cliInt(args[1:], "--limit", 500))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"workload_id": id, "rows": rows})
	case "close":
		id := firstNonEmptyCLI(cliValue(args[1:], "--id"), cliValue(args[1:], "--workload-id"))
		status := firstNonEmptyCLI(cliValue(args[1:], "--status"), "completed")
		if err := db.CloseWorkload(id, status, cliValue(args[1:], "--outcome")); err != nil {
			return err
		}
		fmt.Printf("%s\t%s\n", id, status)
	case "start-run":
		workloadID := firstNonEmptyCLI(cliValue(args[1:], "--workload-id"), cliValue(args[1:], "--id"))
		source := cliValue(args[1:], "--source")
		agentName := firstNonEmptyCLI(cliValue(args[1:], "--agent-name"), cliValue(args[1:], "--agent"), source, "agent")
		runID, err := db.StartAgentRun(workloadID, source, agentName, cliValue(args[1:], "--command"), cliValue(args[1:], "--cwd"))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"workload_id": workloadID, "run_id": runID, "status": "running"})
	case "heartbeat":
		runID := firstNonEmptyCLI(cliValue(args[1:], "--run-id"), cliValue(args[1:], "--run_id"), cliValue(args[1:], "--id"))
		progress, err := parseFloat(cliValue(args[1:], "--progress"))
		if err != nil {
			return err
		}
		metrics := map[string]interface{}{}
		if rawMetrics := cliValue(args[1:], "--metrics-json"); rawMetrics != "" {
			if err := json.Unmarshal([]byte(rawMetrics), &metrics); err != nil {
				return fmt.Errorf("--metrics-json must be a JSON object: %w", err)
			}
		}
		row, err := db.RecordAgentRunHeartbeat(cliValue(args[1:], "--event-id"), runID, cliValue(args[1:], "--status"), cliValue(args[1:], "--phase"),
			cliValue(args[1:], "--message"), progress, metrics, time.Time{}, 1)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(row)
	case "liveness":
		maxAge := 10 * time.Minute
		if raw := cliValue(args[1:], "--max-age"); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil {
				return err
			}
			if parsed <= 0 {
				return fmt.Errorf("--max-age must be positive")
			}
			maxAge = parsed
		}
		staleOnly := cliBool(args[1:], "--stale-only")
		rows, err := db.GetAgentRunLiveness(maxAge, staleOnly, cliInt(args[1:], "--limit", 200))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"rows": rows, "max_age": maxAge.String(), "stale_only": staleOnly})
	case "context", "record-context":
		return runWorkloadContextCLI(args[1:], db)
	case "tool", "record-tool":
		return runWorkloadToolCLI(args[1:], db)
	default:
		return fmt.Errorf("unknown workload command %q", args[0])
	}
	return nil
}

func runWorkloadContextCLI(args []string, db *storage.DB) error {
	workloadID := firstNonEmptyCLI(cliValue(args, "--workload-id"), cliValue(args, "--id"))
	if workloadID == "" {
		return fmt.Errorf("--workload-id is required")
	}
	refHash := firstNonEmptyCLI(cliValue(args, "--hash"), cliValue(args, "--ref-hash"))
	label := cliValue(args, "--label")
	if refHash == "" && label == "" {
		return fmt.Errorf("at least one of --hash or --label is required")
	}
	ts := time.Now().UTC()
	if raw := cliValue(args, "--timestamp"); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return fmt.Errorf("invalid --timestamp: %w", err)
		}
		ts = parsed
	}
	payload := map[string]interface{}{
		"ref_type":      firstNonEmptyCLI(cliValue(args, "--type"), cliValue(args, "--ref-type"), "context"),
		"ref_hash":      refHash,
		"label":         label,
		"repo":          cliValue(args, "--repo"),
		"git_branch":    firstNonEmptyCLI(cliValue(args, "--branch"), cliValue(args, "--git-branch")),
		"commit_sha":    firstNonEmptyCLI(cliValue(args, "--commit"), cliValue(args, "--commit-sha")),
		"privacy_label": firstNonEmptyCLI(cliValue(args, "--privacy-label"), "local"),
	}
	rawPayload, _ := json.Marshal(payload)
	result, err := db.IngestCanonicalEvent(storage.CanonicalEvent{
		EventID:       cliValue(args, "--event-id"),
		Source:        firstNonEmptyCLI(cliValue(args, "--source"), "local"),
		EventType:     "context.ref",
		SourceEventID: firstNonEmptyCLI(cliValue(args, "--context-ref-id"), cliValue(args, "--source-event-id")),
		WorkloadID:    workloadID,
		AgentRunID:    firstNonEmptyCLI(cliValue(args, "--run-id"), cliValue(args, "--agent-run-id")),
		Project:       cliValue(args, "--project"),
		GitBranch:     firstNonEmptyCLI(cliValue(args, "--branch"), cliValue(args, "--git-branch")),
		Timestamp:     ts,
		Payload:       rawPayload,
		Confidence:    1,
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func runWorkloadToolCLI(args []string, db *storage.DB) error {
	workloadID := firstNonEmptyCLI(cliValue(args, "--workload-id"), cliValue(args, "--id"))
	if workloadID == "" {
		return fmt.Errorf("--workload-id is required")
	}
	toolName := firstNonEmptyCLI(cliValue(args, "--tool-name"), cliValue(args, "--name"))
	if toolName == "" {
		return fmt.Errorf("--tool-name is required")
	}
	ts := time.Now().UTC()
	if raw := cliValue(args, "--timestamp"); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			return fmt.Errorf("invalid --timestamp: %w", err)
		}
		ts = parsed
	}
	payload := map[string]interface{}{
		"tool_name":   toolName,
		"tool_type":   firstNonEmptyCLI(cliValue(args, "--tool-type"), cliValue(args, "--type")),
		"status":      firstNonEmptyCLI(cliValue(args, "--status"), "ok"),
		"error_class": cliValue(args, "--error-class"),
		"duration_ms": cliInt(args, "--duration-ms", 0),
		"params_hash": firstNonEmptyCLI(cliValue(args, "--params-hash"), cliValue(args, "--hash")),
	}
	rawPayload, _ := json.Marshal(payload)
	result, err := db.IngestCanonicalEvent(storage.CanonicalEvent{
		EventID:       cliValue(args, "--event-id"),
		Source:        firstNonEmptyCLI(cliValue(args, "--source"), "local"),
		EventType:     "tool.call",
		SourceEventID: firstNonEmptyCLI(cliValue(args, "--tool-call-id"), cliValue(args, "--source-event-id")),
		WorkloadID:    workloadID,
		AgentRunID:    firstNonEmptyCLI(cliValue(args, "--run-id"), cliValue(args, "--agent-run-id")),
		Project:       cliValue(args, "--project"),
		GitBranch:     firstNonEmptyCLI(cliValue(args, "--branch"), cliValue(args, "--git-branch")),
		Timestamp:     ts,
		Payload:       rawPayload,
		Confidence:    1,
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func runWrappedCLI(args []string, db *storage.DB) error {
	sep := -1
	for i, arg := range args {
		if arg == "--" {
			sep = i
			break
		}
	}
	if sep < 0 || sep == len(args)-1 {
		return fmt.Errorf("usage: agent-ledger run --goal <goal> [--agent codex] -- <command>")
	}
	meta := args[:sep]
	commandArgs := args[sep+1:]
	goal := cliValue(meta, "--goal")
	if goal == "" {
		return fmt.Errorf("--goal is required")
	}
	budget, _ := parseFloat(cliValue(meta, "--budget-usd"))
	agent := firstNonEmptyCLI(cliValue(meta, "--agent"), cliValue(meta, "--source"), "local")
	source := firstNonEmptyCLI(cliValue(meta, "--source"), agent)
	cwd, _ := os.Getwd()
	workloadID, err := db.CreateWorkload(goal, source, cliValue(meta, "--project"), cliValue(meta, "--repo"), cliValue(meta, "--branch"), "", "", budget)
	if err != nil {
		return err
	}
	runID, err := db.StartAgentRun(workloadID, source, agent, strings.Join(commandArgs, " "), cwd)
	if err != nil {
		return err
	}
	start := time.Now()
	cmd := exec.Command(commandArgs[0], commandArgs[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Dir = cwd
	err = cmd.Run()
	duration := time.Since(start).Milliseconds()
	status := "completed"
	exitCode := 0
	errText := ""
	if err != nil {
		status = "failed"
		errText = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1
		}
	}
	_ = db.FinishAgentRun(runID, status, exitCode, errText, duration)
	_ = db.CloseWorkload(workloadID, status, status)
	fmt.Printf("workload=%s run=%s status=%s exit=%d duration_ms=%d\n", workloadID, runID, status, exitCode, duration)
	return err
}

func cliValue(args []string, key string) string {
	for i := 0; i < len(args); i++ {
		if args[i] == key && i+1 < len(args) {
			return args[i+1]
		}
		if strings.HasPrefix(args[i], key+"=") {
			return strings.TrimPrefix(args[i], key+"=")
		}
	}
	return ""
}

func cliBool(args []string, key string) bool {
	for i := 0; i < len(args); i++ {
		if args[i] == key {
			return true
		}
		if strings.HasPrefix(args[i], key+"=") {
			value := strings.ToLower(strings.TrimPrefix(args[i], key+"="))
			return value == "1" || value == "true" || value == "yes"
		}
	}
	return false
}

func parseFloat(raw string) (float64, error) {
	if raw == "" {
		return 0, nil
	}
	var v float64
	_, err := fmt.Sscanf(raw, "%f", &v)
	return v, err
}

func cliInt(args []string, key string, fallback int) int {
	raw := cliValue(args, key)
	if raw == "" {
		return fallback
	}
	var v int
	if _, err := fmt.Sscanf(raw, "%d", &v); err != nil || v <= 0 {
		return fallback
	}
	return v
}

func firstNonEmptyCLI(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
