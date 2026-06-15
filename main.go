package main

import (
	"context"
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
	"github.com/zhenzhis/agent-ledger/internal/controlplane"
	"github.com/zhenzhis/agent-ledger/internal/integrations"
	"github.com/zhenzhis/agent-ledger/internal/mcp"
	"github.com/zhenzhis/agent-ledger/internal/notifications"
	ledgerpolicy "github.com/zhenzhis/agent-ledger/internal/policy"
	"github.com/zhenzhis/agent-ledger/internal/pricing"
	"github.com/zhenzhis/agent-ledger/internal/quota"
	"github.com/zhenzhis/agent-ledger/internal/reconciliation"
	"github.com/zhenzhis/agent-ledger/internal/server"
	"github.com/zhenzhis/agent-ledger/internal/storage"
	"github.com/zhenzhis/agent-ledger/internal/ui"
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

	if cfg.RBAC.ReadOnly {
		log.Println("read-only mode enabled: version writes, background scans, pricing sync, and cost recalculation are disabled")
	} else {
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
		if !ce.cfg.Enabled && !cfg.RBAC.ReadOnly {
			recordDisabledHealth(db, ce)
		}
	}
	var scanMu sync.Mutex
	scanSource := func(source string, reset bool) error {
		if cfg.RBAC.ReadOnly {
			return fmt.Errorf("read-only mode: scan is disabled")
		}
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
	if !cfg.RBAC.ReadOnly {
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
	}

	// Start web server
	addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.Port)
	srv := server.New(db, addr, server.Options{
		Config:       cfg,
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
		ConfigStatus: config.StatusReport(cfg),
		Sources:      sourceOptions,
		Scan:         scanSource,
		Recalc:       func() error { return recalcCostsMode(db, "zero") },
		RecalcMode:   func(mode string) error { return recalcCostsMode(db, mode) },
		PricingSync:  func() error { return pricing.SyncWithConfig(db, cfg.Pricing) },
	})
	if cfg.Integrations.OTLPReceiver.Enabled && cfg.Integrations.OTLPReceiver.GRPCEnabled {
		go func() {
			if err := server.StartOTLPGRPCReceiver(db, cfg); err != nil {
				log.Printf("OTLP gRPC receiver: %v", err)
			}
		}()
	}
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
	if cfg.RBAC.ReadOnly && cliCommandRequiresWrite(args) {
		return fmt.Errorf("read-only mode: command %q is disabled because it writes local state", strings.Join(args, " "))
	}
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
		return runBatteryCLI(args[1:], cfg, db)
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
	case "adapter":
		return runAdapterCLI(args[1:])
	case "bundle":
		return runBundleCLI(args[1:], db, !cfg.RBAC.ReadOnly)
	case "policy":
		return runPolicyCLI(args[1:], cfg, db)
	case "audit":
		return runAuditCLI(args[1:], db)
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
	case "notify":
		return runNotifyCLI(args[1:], cfg, db)
	case "discovery":
		return json.NewEncoder(os.Stdout).Encode(integrations.Discovery(integrations.OptionsFromConfig(cfg)))
	case "contracts":
		return runContractsCLI(args[1:], cfg)
	case "openapi":
		return json.NewEncoder(os.Stdout).Encode(integrations.OpenAPISpecFor(integrations.OptionsFromConfig(cfg), server.RuntimeStatusFromConfig(cfg)))
	case "integrations":
		if len(args) > 1 && args[1] == "readiness" {
			return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationReadiness(integrations.OptionsFromConfig(cfg)))
		}
		if len(args) > 1 && args[1] == "smoke" {
			opts := integrations.OptionsFromConfig(cfg)
			return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationSmokeReportFor(opts, server.RuntimeStatusFromConfig(cfg)))
		}
		if len(args) > 1 && args[1] == "compatibility" {
			return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationCompatibilityReportFor(integrationCompatibilityRequestFromCLI(args[2:])))
		}
		if len(args) > 1 && (args[1] == "rollout-plan" || args[1] == "rollout") {
			return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationRolloutPlanFor(integrationRolloutRequestFromCLI(args[2:])))
		}
		if len(args) > 1 && (args[1] == "evidence-kit" || args[1] == "evidence") {
			opts := integrations.OptionsFromConfig(cfg)
			return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationEvidenceKitFor(opts, server.RuntimeStatusFromConfig(cfg), integrationEvidenceKitRequestFromCLI(args[2:])))
		}
		if len(args) > 1 && args[1] == "drift" {
			opts := integrations.OptionsFromConfig(cfg)
			return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationDriftReportFor(opts, server.RuntimeStatusFromConfig(cfg), integrationDriftRequestFromCLI(args[2:])))
		}
		if len(args) > 1 && (args[1] == "lockfile" || args[1] == "lock") {
			opts := integrations.OptionsFromConfig(cfg)
			return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationLockfileFor(opts, server.RuntimeStatusFromConfig(cfg)))
		}
		if len(args) > 1 && (args[1] == "upgrade-gate" || args[1] == "gate") {
			opts := integrations.OptionsFromConfig(cfg)
			return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationUpgradeGateFor(opts, server.RuntimeStatusFromConfig(cfg), integrationUpgradeGateRequestFromCLI(args[2:])))
		}
		if len(args) > 1 && (args[1] == "production-gate" || args[1] == "production") {
			opts := integrations.OptionsFromConfig(cfg)
			return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationProductionGateFor(opts, server.RuntimeStatusFromConfig(cfg), integrationProductionGateRequestFromCLI(args[2:])))
		}
		if len(args) > 1 && (args[1] == "schema-gate" || args[1] == "schema") {
			return json.NewEncoder(os.Stdout).Encode(integrations.SchemaEvolutionGateFor(schemaEvolutionGateRequestFromCLI(args[2:])))
		}
		return json.NewEncoder(os.Stdout).Encode(integrations.Registry(integrations.OptionsFromConfig(cfg)))
	case "integration-readiness":
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationReadiness(integrations.OptionsFromConfig(cfg)))
	case "integration-smoke":
		opts := integrations.OptionsFromConfig(cfg)
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationSmokeReportFor(opts, server.RuntimeStatusFromConfig(cfg)))
	case "integration-compatibility":
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationCompatibilityReportFor(integrationCompatibilityRequestFromCLI(args[1:])))
	case "integration-rollout", "integration-rollout-plan":
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationRolloutPlanFor(integrationRolloutRequestFromCLI(args[1:])))
	case "integration-evidence", "integration-evidence-kit":
		opts := integrations.OptionsFromConfig(cfg)
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationEvidenceKitFor(opts, server.RuntimeStatusFromConfig(cfg), integrationEvidenceKitRequestFromCLI(args[1:])))
	case "integration-drift":
		opts := integrations.OptionsFromConfig(cfg)
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationDriftReportFor(opts, server.RuntimeStatusFromConfig(cfg), integrationDriftRequestFromCLI(args[1:])))
	case "integration-lockfile":
		opts := integrations.OptionsFromConfig(cfg)
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationLockfileFor(opts, server.RuntimeStatusFromConfig(cfg)))
	case "integration-upgrade-gate":
		opts := integrations.OptionsFromConfig(cfg)
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationUpgradeGateFor(opts, server.RuntimeStatusFromConfig(cfg), integrationUpgradeGateRequestFromCLI(args[1:])))
	case "integration-production-gate":
		opts := integrations.OptionsFromConfig(cfg)
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationProductionGateFor(opts, server.RuntimeStatusFromConfig(cfg), integrationProductionGateRequestFromCLI(args[1:])))
	case "schema-gate", "schema-evolution-gate":
		return json.NewEncoder(os.Stdout).Encode(integrations.SchemaEvolutionGateFor(schemaEvolutionGateRequestFromCLI(args[1:])))
	case "signals", "signal-taxonomy":
		if len(args) > 1 && args[1] == "coverage" {
			return json.NewEncoder(os.Stdout).Encode(integrations.SignalCoverage())
		}
		return json.NewEncoder(os.Stdout).Encode(integrations.SignalTaxonomy())
	case "signal-coverage":
		return json.NewEncoder(os.Stdout).Encode(integrations.SignalCoverage())
	case "goal":
		return runGoalCLI(args[1:], cfg)
	case "ui":
		return runUICLI(args[1:])
	case "runtime":
		return json.NewEncoder(os.Stdout).Encode(server.RuntimeStatusFromConfig(cfg))
	case "config":
		return runConfigCLI(args[1:], cfg)
	case "readiness":
		return runReadinessCLI(args[1:], cfg, db)
	case "admission":
		return runAdmissionCLI(args[1:], cfg)
	case "otel":
		return runOTelCLI(args[1:], db)
	case "a2a":
		return runA2ACLI(args[1:], db)
	case "provider":
		return runProviderCLI(args[1:], db)
	case "agent":
		return runAgentCLI(args[1:])
	case "projection":
		return runProjectionCLI(args[1:], db)
	case "mcp":
		return mcp.New(db, cfg).Serve(os.Stdin, os.Stdout)
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	return nil
}

func runGoalCLI(args []string, cfg *config.Config) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent-ledger goal coverage|audit")
	}
	report := integrations.GoalCoverageReportFor(integrations.OptionsFromConfig(cfg), server.RuntimeStatusFromConfig(cfg))
	switch args[0] {
	case "coverage":
		return json.NewEncoder(os.Stdout).Encode(report)
	case "audit", "completion-audit", "completion":
		return json.NewEncoder(os.Stdout).Encode(report.CompletionAudit)
	default:
		return fmt.Errorf("usage: agent-ledger goal coverage|audit")
	}
}

func integrationCompatibilityRequestFromCLI(args []string) integrations.IntegrationCompatibilityRequest {
	return integrations.NormalizeIntegrationCompatibilityRequest(integrations.IntegrationCompatibilityRequest{
		AgentProfileID:    firstNonEmptyCLI(cliValue(args, "--agent-profile-id"), cliValue(args, "--agent"), cliValue(args, "--profile"), cliValue(args, "--framework")),
		ProviderProfileID: firstNonEmptyCLI(cliValue(args, "--provider-profile-id"), cliValue(args, "--provider"), cliValue(args, "--runtime")),
		Surface:           firstNonEmptyCLI(cliValue(args, "--surface"), cliValue(args, "--ingest"), cliValue(args, "--kind")),
		MinConfidence:     firstNonEmptyCLI(cliValue(args, "--min-confidence"), cliValue(args, "--min_confidence")),
	})
}

func integrationRolloutRequestFromCLI(args []string) integrations.IntegrationRolloutRequest {
	return integrations.NormalizeIntegrationRolloutRequest(integrations.IntegrationRolloutRequest{
		AgentProfileID:    firstNonEmptyCLI(cliValue(args, "--agent-profile-id"), cliValue(args, "--agent"), cliValue(args, "--profile"), cliValue(args, "--framework")),
		ProviderProfileID: firstNonEmptyCLI(cliValue(args, "--provider-profile-id"), cliValue(args, "--provider"), cliValue(args, "--runtime")),
		Surface:           firstNonEmptyCLI(cliValue(args, "--surface"), cliValue(args, "--ingest"), cliValue(args, "--kind")),
		MinConfidence:     firstNonEmptyCLI(cliValue(args, "--min-confidence"), cliValue(args, "--min_confidence")),
	})
}

func integrationEvidenceKitRequestFromCLI(args []string) integrations.IntegrationEvidenceKitRequest {
	return integrations.NormalizeIntegrationEvidenceKitRequest(integrations.IntegrationEvidenceKitRequest{
		AgentProfileID:    firstNonEmptyCLI(cliValue(args, "--agent-profile-id"), cliValue(args, "--agent"), cliValue(args, "--profile"), cliValue(args, "--framework")),
		ProviderProfileID: firstNonEmptyCLI(cliValue(args, "--provider-profile-id"), cliValue(args, "--provider"), cliValue(args, "--runtime")),
		Surface:           firstNonEmptyCLI(cliValue(args, "--surface"), cliValue(args, "--ingest"), cliValue(args, "--kind")),
		MinConfidence:     firstNonEmptyCLI(cliValue(args, "--min-confidence"), cliValue(args, "--min_confidence")),
	})
}

func integrationDriftRequestFromCLI(args []string) integrations.IntegrationDriftRequest {
	expected := map[string]string{}
	for _, id := range integrations.IntegrationDriftHashIDs() {
		if value := firstNonEmptyCLI(cliValue(args, "--"+id), cliValue(args, "--"+strings.ReplaceAll(id, "_", "-"))); value != "" {
			expected[id] = value
		}
	}
	for _, raw := range cliValues(args, "--hash") {
		key, value, ok := strings.Cut(raw, "=")
		if ok && strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			expected[key] = value
		}
	}
	for _, raw := range cliValues(args, "--expected") {
		key, value, ok := strings.Cut(raw, "=")
		if ok && strings.TrimSpace(key) != "" && strings.TrimSpace(value) != "" {
			expected[key] = value
		}
	}
	return integrations.NormalizeIntegrationDriftRequest(integrations.IntegrationDriftRequest{
		Strict:   cliBool(args, "--strict"),
		Expected: expected,
	})
}

func integrationUpgradeGateRequestFromCLI(args []string) integrations.IntegrationUpgradeGateRequest {
	drift := integrationDriftRequestFromCLI(args)
	return integrations.NormalizeIntegrationUpgradeGateRequest(integrations.IntegrationUpgradeGateRequest{
		Strict:   drift.Strict,
		Expected: drift.Expected,
	})
}

func integrationProductionGateRequestFromCLI(args []string) integrations.IntegrationProductionGateRequest {
	return integrations.NormalizeIntegrationProductionGateRequest(integrations.IntegrationProductionGateRequest{
		Strict:        cliBool(args, "--strict"),
		AllowPreview:  firstCLIBool(args, "--allow-preview", "--allow_preview", "--preview-ok", "--preview_ok"),
		AllowOutbound: firstCLIBool(args, "--allow-outbound", "--allow_outbound", "--outbound-ok", "--outbound_ok"),
	})
}

func schemaEvolutionGateRequestFromCLI(args []string) integrations.SchemaEvolutionGateRequest {
	return integrations.NormalizeSchemaEvolutionGateRequest(integrations.SchemaEvolutionGateRequest{
		Strict:               cliBool(args, "--strict"),
		ExpectedVersion:      firstNonEmptyCLI(cliValue(args, "--schema-version"), cliValue(args, "--schema_version"), cliValue(args, "--version")),
		ExpectedSchemaHash:   firstNonEmptyCLI(cliValue(args, "--schema-hash"), cliValue(args, "--schema_hash"), cliValue(args, "--canonical-schema-hash"), cliValue(args, "--canonical_schema_hash"), cliValue(args, "--hash")),
		RequiredEventTypes:   appendCLIValues(cliValues(args, "--event-type"), cliValues(args, "--event_type"), cliValues(args, "--event-types"), cliValues(args, "--event_types"), cliValues(args, "--required-event-type"), cliValues(args, "--required_event_type")),
		RequiredRejectedKeys: appendCLIValues(cliValues(args, "--rejected-key"), cliValues(args, "--rejected_key"), cliValues(args, "--rejected-keys"), cliValues(args, "--rejected_keys"), cliValues(args, "--required-rejected-key"), cliValues(args, "--required_rejected_key")),
	})
}

func runUICLI(args []string) error {
	if len(args) == 0 || args[0] != "check" {
		return fmt.Errorf("usage: agent-ledger ui check [--root path] [--format json|markdown]")
	}
	root := cliValue(args[1:], "--root")
	report, err := ui.Check(root)
	if err != nil {
		return err
	}
	if strings.EqualFold(cliValue(args[1:], "--format"), "markdown") {
		_, err := os.Stdout.Write([]byte(ui.FormatMarkdown(report)))
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func runConfigCLI(args []string, cfg *config.Config) error {
	if len(args) == 0 || args[0] != "status" {
		return fmt.Errorf("usage: agent-ledger config status [--format json|markdown]")
	}
	report := config.StatusReport(cfg)
	if strings.EqualFold(cliValue(args[1:], "--format"), "markdown") {
		_, err := os.Stdout.Write([]byte(config.FormatStatusMarkdown(report)))
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func runReadinessCLI(args []string, cfg *config.Config, db *storage.DB) error {
	runtime := server.RuntimeStatusFromConfig(cfg)
	report := controlplane.BuildReadinessReport(
		db,
		cfg,
		runtime,
		integrations.ContractVerificationReportFor(integrations.OptionsFromConfig(cfg), runtime),
		time.Now().UTC(),
	)
	if strings.EqualFold(cliValue(args, "--format"), "markdown") {
		_, err := os.Stdout.Write([]byte(controlplane.FormatReadinessMarkdown(report)))
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
}

func runAdmissionCLI(args []string, cfg *config.Config) error {
	if len(args) == 0 || args[0] != "check" {
		return fmt.Errorf("usage: agent-ledger admission check [--surface http|cli|mcp] [--method GET|POST] [--path /api/...] [--command \"agent-ledger ...\"] [--tool ledger.name] [--role viewer|operator|admin] [--dry-run] [--record] [--has-workload-id] [--format json|markdown]")
	}
	input := controlplane.AdmissionInput{
		Surface:        cliValue(args[1:], "--surface"),
		Method:         cliValue(args[1:], "--method"),
		Path:           cliValue(args[1:], "--path"),
		Command:        cliValue(args[1:], "--command"),
		Tool:           cliValue(args[1:], "--tool"),
		Role:           cliValue(args[1:], "--role"),
		RBACEnabled:    cfg != nil && cfg.RBAC.Enabled,
		AuthConfigured: cfg != nil && (cfg.Server.AuthToken != "" || cfg.Server.AdminToken != "" || cfg.Server.ViewerToken != ""),
		ReadOnly:       cfg != nil && cfg.RBAC.ReadOnly,
		DryRun:         cliBool(args[1:], "--dry-run") || cliBool(args[1:], "--dry_run"),
		Record:         cliBool(args[1:], "--record"),
		HasWorkloadID:  cliBool(args[1:], "--has-workload-id") || cliBool(args[1:], "--has_workload_id") || cliValue(args[1:], "--workload-id") != "" || cliValue(args[1:], "--workload_id") != "",
	}
	decision := controlplane.EvaluateAdmission(input, time.Now().UTC())
	if strings.EqualFold(cliValue(args[1:], "--format"), "markdown") {
		_, err := os.Stdout.Write([]byte(controlplane.FormatAdmissionMarkdown(decision)))
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(decision)
}

func runContractsCLI(args []string, cfg *config.Config) error {
	opts := integrations.OptionsFromConfig(cfg)
	runtime := server.RuntimeStatusFromConfig(cfg)
	if len(args) == 0 {
		return json.NewEncoder(os.Stdout).Encode(integrations.ContractBundleFor(opts, runtime))
	}
	switch args[0] {
	case "verify":
		return json.NewEncoder(os.Stdout).Encode(integrations.ContractVerificationReportFor(opts, runtime))
	default:
		return fmt.Errorf("usage: agent-ledger contracts [verify]")
	}
}

func cliCommandRequiresWrite(args []string) bool {
	if len(args) == 0 {
		return false
	}
	cmd := args[0]
	sub := ""
	if len(args) > 1 {
		sub = args[1]
	}
	switch cmd {
	case "pricing":
		return sub == "sync"
	case "event":
		return sub == "ingest"
	case "bundle":
		return sub == "import"
	case "reconcile":
		return sub == "import"
	case "provider", "otel", "a2a":
		return sub == "ingest"
	case "projection":
		return sub == "repair"
	case "notify":
		notifyArgs := []string{}
		if len(args) > 2 {
			notifyArgs = args[2:]
		}
		if sub == "desktop" {
			return false
		}
		return !(sub == "webhook" && cliBool(notifyArgs, "--dry-run"))
	case "run":
		return true
	case "policy":
		if sub == "resolve" {
			return true
		}
		if sub != "evaluate" || len(args) <= 2 {
			return false
		}
		return cliBool(args[2:], "--record")
	case "workload":
		if sub == "lease" || sub == "leases" {
			return len(args) > 2 && args[2] != "list" && !strings.HasPrefix(args[2], "--")
		}
		switch sub {
		case "", "list", "show", "timeline", "state", "status", "feed", "events", "liveness":
			return false
		default:
			return true
		}
	default:
		return false
	}
}

func runNotifyCLI(args []string, cfg *config.Config, db *storage.DB) error {
	if len(args) == 0 {
		return fmt.Errorf("notify command required: webhook or desktop")
	}
	switch args[0] {
	case "webhook":
		ctx, err := notificationCLIData(args[1:], cfg, db)
		if err != nil {
			return err
		}
		dryRun := cliBool(args[1:], "--dry-run")
		if dryRun {
			return json.NewEncoder(os.Stdout).Encode(notifications.BuildWebhookPayloadWithApprovalRoutes(ctx.feed, ctx.approvals, ctx.routes, cfg.Webhooks.MaxEvents))
		}
		result, err := notifications.SendWebhookWithApprovalRoutes(context.Background(), cfg.Webhooks, ctx.feed, ctx.approvals, ctx.routes, false)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(result)
	case "desktop":
		ctx, err := notificationCLIData(args[1:], cfg, db)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(notifications.BuildDesktopPayloadWithApprovalRoutes(ctx.feed, ctx.approvals, ctx.routes, cfg.Webhooks.MaxEvents))
	default:
		return fmt.Errorf("unknown notify command %q", args[0])
	}
}

type notificationCLIContext struct {
	feed      *storage.WorkloadEventFeed
	approvals []storage.ApprovalRequest
	routes    *storage.ApprovalRouteSummary
}

func notificationCLIData(args []string, cfg *config.Config, db *storage.DB) (*notificationCLIContext, error) {
	now := time.Now()
	from, to, err := cliDateRange(args, now)
	if err != nil {
		return nil, err
	}
	maxAge := 10 * time.Minute
	if raw := cliValue(args, "--max-age"); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil {
			return nil, err
		}
		if parsed <= 0 {
			return nil, fmt.Errorf("--max-age must be positive")
		}
		maxAge = parsed
	}
	limit := cliInt(args, "--limit", cfg.Webhooks.MaxEvents)
	feed, err := db.GetWorkloadEventFeed(from, to,
		cliValue(args, "--source"),
		cliValue(args, "--model"),
		cliValue(args, "--project"),
		cliValue(args, "--phase"),
		cliValue(args, "--severity"),
		limit,
		maxAge)
	if err != nil {
		return nil, err
	}
	approvals, err := db.ListApprovalRequests("pending", limit)
	if err != nil {
		return nil, err
	}
	approvalDueWithin := 24 * time.Hour
	if raw := firstNonEmptyCLI(cliValue(args, "--approval-due-within"), cliValue(args, "--due-within")); raw != "" {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 || parsed > 30*24*time.Hour {
			return nil, fmt.Errorf("invalid --approval-due-within %q: expected duration from 1ns to 720h", raw)
		}
		approvalDueWithin = parsed
	}
	routes, err := db.GetApprovalRouteSummary(limit, approvalDueWithin)
	if err != nil {
		return nil, err
	}
	return &notificationCLIContext{feed: feed, approvals: approvals, routes: routes}, nil
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

func runBatteryCLI(args []string, cfg *config.Config, db *storage.DB) error {
	if db == nil {
		return fmt.Errorf("battery requires a database")
	}
	status, err := quota.BuildStatus(time.Now(), cfg.Quota, quota.Filter{
		Window:  cliValue(args, "--window"),
		Source:  cliValue(args, "--source"),
		Model:   cliValue(args, "--model"),
		Project: cliValue(args, "--project"),
	}, db.GetDashboardStatsFiltered)
	if err != nil {
		return err
	}
	switch strings.ToLower(cliValue(args, "--format")) {
	case "json":
		return json.NewEncoder(os.Stdout).Encode(status)
	case "markdown", "md":
		_, err := os.Stdout.Write([]byte(quota.FormatMarkdown(status)))
		return err
	default:
		_, err := os.Stdout.Write([]byte(quota.FormatText(status)))
		return err
	}
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
	if len(args) > 0 && args[0] == "profiles" {
		return json.NewEncoder(os.Stdout).Encode(integrations.ProviderProfiles())
	}
	if len(args) == 0 || (args[0] != "convert" && args[0] != "ingest") {
		return fmt.Errorf("usage: agent-ledger provider profiles|convert|ingest [--file response.json]")
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

func runAgentCLI(args []string) error {
	if len(args) > 0 && args[0] == "profiles" {
		return json.NewEncoder(os.Stdout).Encode(integrations.AgentFrameworkProfiles())
	}
	if len(args) > 0 && args[0] == "recommend" {
		req := integrations.NormalizeIntegrationRecommendationRequest(integrations.IntegrationRecommendationRequest{
			AgentProfileID:    firstNonEmptyCLI(cliValue(args[1:], "--profile"), cliValue(args[1:], "--agent"), cliValue(args[1:], "--framework")),
			ProviderProfileID: firstNonEmptyCLI(cliValue(args[1:], "--provider"), cliValue(args[1:], "--runtime")),
			Surface:           cliValue(args[1:], "--surface"),
			Signals:           strings.Split(cliValue(args[1:], "--signals"), ","),
			RuntimeMode:       firstNonEmptyCLI(cliValue(args[1:], "--runtime-mode"), cliValue(args[1:], "--mode")),
			ReadOnly:          cliBool(args[1:], "--read-only") || cliBool(args[1:], "--readonly"),
		})
		return json.NewEncoder(os.Stdout).Encode(integrations.IntegrationRecommendation(req))
	}
	return fmt.Errorf("usage: agent-ledger agent profiles|recommend [--profile codex-cli] [--provider openai-official] [--surface provider-stream] [--signals model,usage,cache]")
}

func runAdapterCLI(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: agent-ledger adapter spec|matrix|conformance [--kind auto|canonical|provider|provider-stream|otel|a2a] [--strict] [--file fixture.json]")
	}
	if args[0] == "spec" {
		return json.NewEncoder(os.Stdout).Encode(integrations.AdapterContractSpec())
	}
	if args[0] == "matrix" || args[0] == "conformance-matrix" {
		return json.NewEncoder(os.Stdout).Encode(integrations.AdapterConformanceMatrixSpec())
	}
	if args[0] != "conformance" {
		return fmt.Errorf("usage: agent-ledger adapter spec|matrix|conformance [--kind auto|canonical|provider|provider-stream|otel|a2a] [--strict] [--file fixture.json]")
	}
	raw, err := readCLIInput(args[1:], "--file", 4<<20)
	if err != nil {
		return err
	}
	report, err := integrations.RunAdapterConformanceWithOptions(integrations.AdapterConformanceOptions{
		Kind:   cliValue(args[1:], "--kind"),
		Strict: cliBool(args[1:], "--strict"),
	}, raw)
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(report)
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
		return fmt.Errorf("usage: agent-ledger policy audit|enforcement|routes|evaluate|approvals|resolve")
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
	case "enforcement":
		report, err := db.GetPolicyEnforcementReport(cliInt(args[1:], "--limit", 200))
		if err != nil {
			return err
		}
		if cliBool(args[1:], "--privacy") {
			redactPolicyEnforcementReport(report)
		}
		return json.NewEncoder(os.Stdout).Encode(report)
	case "routes":
		dueWithin := 24 * time.Hour
		if raw := strings.TrimSpace(cliValue(args[1:], "--due-within")); raw != "" {
			parsed, err := time.ParseDuration(raw)
			if err != nil || parsed <= 0 || parsed > 30*24*time.Hour {
				return fmt.Errorf("invalid --due-within %q: expected duration from 1ns to 720h", raw)
			}
			dueWithin = parsed
		}
		report, err := db.GetApprovalRouteSummary(cliInt(args[1:], "--limit", 200), dueWithin)
		if err != nil {
			return err
		}
		if cliBool(args[1:], "--privacy") {
			redactApprovalRouteSummary(report)
		}
		if strings.EqualFold(cliValue(args[1:], "--format"), "markdown") {
			printApprovalRouteSummaryMarkdown(report)
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
		if cliBool(args[1:], "--privacy") {
			redactApprovalRequests(rows)
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"status": status, "rows": rows})
	case "resolve":
		requestID := firstNonEmptyCLI(cliValue(args[1:], "--id"), cliValue(args[1:], "--request-id"), cliValue(args[1:], "--request_id"))
		status := cliValue(args[1:], "--status")
		if status == "" {
			status = "approved"
		}
		voter := firstNonEmptyCLI(cliValue(args[1:], "--voter"), cliValue(args[1:], "--actor"), "cli")
		requiredApprovals := firstNonEmptyInt(cliInt(args[1:], "--required-approvals", 0), cliInt(args[1:], "--required_approvals", 0))
		note := cliValue(args[1:], "--note")
		result, err := db.CastApprovalVote(requestID, status, voter, "cli", note, requiredApprovals)
		if err != nil {
			return err
		}
		if err := db.AppendAuditLog("local", "cli", "policy.approval."+result.Status, requestID, map[string]string{
			"approval_votes":     fmt.Sprint(result.ApprovalVotes),
			"decided":            fmt.Sprint(result.Decided),
			"note_present":       fmt.Sprint(strings.TrimSpace(note) != ""),
			"rejection_votes":    fmt.Sprint(result.RejectionVotes),
			"required_approvals": fmt.Sprint(result.RequiredApprovals),
			"voter":              voter,
		}); err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"ok": true, "result": result})
	case "evaluate":
	default:
		return fmt.Errorf("usage: agent-ledger policy audit [--from YYYY-MM-DD] [--to YYYY-MM-DD] [--format markdown|json]; agent-ledger policy enforcement [--limit n] [--privacy]; agent-ledger policy routes [--due-within 24h] [--limit n] [--privacy] [--format markdown|json]; agent-ledger policy evaluate [--source s] [--model m] [--project p] [--repo r] [--branch b] [--team t] [--action a] [--target x] [--workload-id id] [--run-id id] [--role role] [--record]; agent-ledger policy approvals [--status pending|approved|rejected|all] [--limit n] [--privacy]; agent-ledger policy resolve --id id --status approved|rejected [--voter name] [--required-approvals n] [--note text]")
	}
	req := ledgerpolicy.Request{
		WorkloadID: firstNonEmptyCLI(cliValue(args[1:], "--workload-id"), cliValue(args[1:], "--workload_id")),
		RunID:      firstNonEmptyCLI(cliValue(args[1:], "--run-id"), cliValue(args[1:], "--run_id")),
		Source:     cliValue(args[1:], "--source"),
		Model:      cliValue(args[1:], "--model"),
		Project:    cliValue(args[1:], "--project"),
		Repo:       cliValue(args[1:], "--repo"),
		GitBranch:  firstNonEmptyCLI(cliValue(args[1:], "--branch"), cliValue(args[1:], "--git-branch"), cliValue(args[1:], "--git_branch")),
		Team:       cliValue(args[1:], "--team"),
		Action:     cliValue(args[1:], "--action"),
		Target:     cliValue(args[1:], "--target"),
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
	fmt.Println("| action | kind | source | model | project | target | evidence |")
	fmt.Println("|---|---|---|---|---|---|---|")
	for _, row := range report.Rows {
		fmt.Printf("| %s | %s | %s | %s | %s | %s | %s |\n", row.EffectiveAction, row.Kind, row.Source, row.Model, row.Project, row.Target, strings.ReplaceAll(row.Evidence, "|", "/"))
	}
}

func redactPolicyAuditReport(report *ledgerpolicy.AuditReport) {
	for i := range report.Rows {
		report.Rows[i].Project = "<redacted>"
		report.Rows[i].Repo = "<redacted>"
		report.Rows[i].GitBranch = "<redacted>"
		report.Rows[i].Team = "<redacted>"
		report.Rows[i].Target = "<redacted>"
		report.Rows[i].SessionID = "<redacted>"
		report.Rows[i].WorkloadID = "<redacted>"
		report.Rows[i].RunID = "<redacted>"
		report.Rows[i].Evidence = "<redacted>"
	}
}

func redactPolicyEnforcementReport(report *storage.PolicyEnforcementReport) {
	if report == nil {
		return
	}
	for i := range report.Decisions {
		report.Decisions[i].DecisionID = "<redacted>"
		report.Decisions[i].WorkloadID = "<redacted>"
		report.Decisions[i].RunID = "<redacted>"
		report.Decisions[i].Reason = "<redacted>"
	}
	for i := range report.ApprovalRequests {
		report.ApprovalRequests[i].RequestID = "<redacted>"
		report.ApprovalRequests[i].PolicyDecisionID = "<redacted>"
		report.ApprovalRequests[i].WorkloadID = "<redacted>"
		report.ApprovalRequests[i].RunID = "<redacted>"
		report.ApprovalRequests[i].Project = "<redacted>"
		report.ApprovalRequests[i].Target = "<redacted>"
		report.ApprovalRequests[i].Reason = "<redacted>"
		report.ApprovalRequests[i].RequestPayload = "<redacted>"
		report.ApprovalRequests[i].DecisionNote = "<redacted>"
	}
	redactAuditRows(report.AuditEvents)
}

func redactApprovalRequests(rows []storage.ApprovalRequest) {
	for i := range rows {
		rows[i].RequestID = "<redacted>"
		rows[i].PolicyDecisionID = "<redacted>"
		rows[i].WorkloadID = "<redacted>"
		rows[i].RunID = "<redacted>"
		rows[i].Project = "<redacted>"
		rows[i].Target = "<redacted>"
		rows[i].ApproverHint = "<redacted>"
		rows[i].EscalationTarget = "<redacted>"
		rows[i].Reason = "<redacted>"
		rows[i].RequestPayload = "<redacted>"
		rows[i].DecidedBy = "<redacted>"
		rows[i].DecisionNote = "<redacted>"
	}
}

func redactApprovalRouteSummary(report *storage.ApprovalRouteSummary) {
	if report == nil {
		return
	}
	for i := range report.Routes {
		report.Routes[i].RouteKey = "<redacted>"
		report.Routes[i].Approver = "<redacted>"
		report.Routes[i].EscalationTarget = "<redacted>"
		for j := range report.Routes[i].Projects {
			report.Routes[i].Projects[j] = "<redacted>"
		}
	}
}

func printApprovalRouteSummaryMarkdown(report *storage.ApprovalRouteSummary) {
	fmt.Println("# Agent Ledger Approval Routes")
	fmt.Println()
	if report == nil {
		fmt.Println("No approval route summary.")
		return
	}
	fmt.Printf("- pending: %d\n", report.Summary.Pending)
	fmt.Printf("- routes: %d\n", report.Summary.Routes)
	fmt.Printf("- overdue: %d\n", report.Summary.Overdue)
	fmt.Printf("- due_soon: %d\n", report.Summary.DueSoon)
	fmt.Printf("- unassigned: %d\n", report.Summary.Unassigned)
	fmt.Printf("- due_within: %s\n\n", report.DueWithin)
	if len(report.Routes) == 0 {
		fmt.Println("No pending approval routes.")
		return
	}
	fmt.Println("| route | pending | overdue | due soon | approver | escalation | actions | models |")
	fmt.Println("|---|---:|---:|---:|---|---|---|---|")
	for _, row := range report.Routes {
		fmt.Printf("| %s | %d | %d | %d | %s | %s | %s | %s |\n",
			markdownCell(row.RouteKey),
			row.Pending,
			row.Overdue,
			row.DueSoon,
			markdownCell(row.Approver),
			markdownCell(row.EscalationTarget),
			markdownCell(strings.Join(row.Actions, ", ")),
			markdownCell(strings.Join(row.Models, ", ")),
		)
	}
}

func markdownCell(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "-"
	}
	return strings.ReplaceAll(value, "|", "\\|")
}

func runAuditCLI(args []string, db *storage.DB) error {
	filter := storage.AuditLogFilter{
		Actor:  cliValue(args, "--actor"),
		Role:   cliValue(args, "--role"),
		Action: cliValue(args, "--action"),
		Target: cliValue(args, "--target"),
		Limit:  cliInt(args, "--limit", 200),
	}
	if cliValue(args, "--from") != "" || cliValue(args, "--to") != "" {
		from, to, err := cliDateRange(args, time.Now())
		if err != nil {
			return err
		}
		filter.From = from
		filter.To = to
	}
	rows, err := db.QueryAuditLog(filter)
	if err != nil {
		return err
	}
	if cliBool(args, "--privacy") {
		redactAuditRows(rows)
	}
	switch strings.ToLower(cliValue(args, "--format")) {
	case "markdown", "md":
		printAuditLogMarkdown(rows)
	case "csv":
		return writeAuditLogCSV(rows)
	default:
		return json.NewEncoder(os.Stdout).Encode(rows)
	}
	return nil
}

func printAuditLogMarkdown(rows []storage.AuditEvent) {
	fmt.Println("# Agent Ledger Audit Log")
	fmt.Println()
	if len(rows) == 0 {
		fmt.Println("No audit events.")
		return
	}
	fmt.Println("| time | role | action | target | actor |")
	fmt.Println("|---|---|---|---|---|")
	for _, row := range rows {
		fmt.Printf("| %s | %s | %s | %s | %s |\n", row.CreatedAt, row.Role, row.Action, row.Target, row.Actor)
	}
}

func writeAuditLogCSV(rows []storage.AuditEvent) error {
	w := csv.NewWriter(os.Stdout)
	if err := w.Write([]string{"id", "actor", "role", "action", "target", "params", "created_at"}); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.Write([]string{fmt.Sprint(row.ID), row.Actor, row.Role, row.Action, row.Target, row.Params, row.CreatedAt}); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func redactAuditRows(rows []storage.AuditEvent) {
	for i := range rows {
		rows[i].Target = "<redacted>"
		rows[i].Params = "<redacted>"
	}
}

func runBundleCLI(args []string, db *storage.DB, recordExport bool) error {
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
		_, raw, err := db.BuildOfflineBundleWithOptions(from, to, cliValue(args[1:], "--source"), cliValue(args[1:], "--model"), cliValue(args[1:], "--project"),
			privacyLabel, signingKey, cliValue(args[1:], "--key-id"), 10000, recordExport)
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
	if len(args) > 0 && (args[0] == "schema-gate" || args[0] == "gate") {
		return json.NewEncoder(os.Stdout).Encode(integrations.SchemaEvolutionGateFor(schemaEvolutionGateRequestFromCLI(args[1:])))
	}
	if len(args) > 0 && (args[0] == "examples" || args[0] == "example") {
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{
			"contract": "agent-ledger.canonical-event-examples",
			"version":  "v1",
			"examples": storage.CanonicalEventExamples(firstNonEmptyCLI(cliValue(args[1:], "--type"), cliValue(args[1:], "--event-type"))),
		})
	}
	if len(args) == 0 || (args[0] != "ingest" && args[0] != "validate") {
		return fmt.Errorf("usage: agent-ledger event schema | agent-ledger event schema-gate [--strict --schema-version v1 --schema-hash sha256:...] | agent-ledger event examples [--type model.call] | agent-ledger event validate [--file event.json] | agent-ledger event ingest [--file event.json]")
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
	if args[0] == "validate" {
		results := make([]*storage.CanonicalEventValidation, 0, len(events))
		for _, event := range events {
			result, err := storage.ValidateCanonicalEvent(event)
			if err != nil {
				return err
			}
			results = append(results, result)
		}
		return json.NewEncoder(os.Stdout).Encode(results)
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
		idempotencyKey := firstNonEmptyCLI(cliValue(args[1:], "--idempotency-key"), cliValue(args[1:], "--idempotency_key"))
		id, _, err := db.CreateWorkloadIdempotent(idempotencyKey, goal, cliValue(args[1:], "--source"), cliValue(args[1:], "--project"), cliValue(args[1:], "--repo"),
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
	case "state", "status":
		id := firstNonEmptyCLI(cliValue(args[1:], "--id"), cliValue(args[1:], "--workload-id"))
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
		state, err := db.GetWorkloadState(id, maxAge)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(state)
	case "feed", "events":
		from, to, err := cliDateRange(args[1:], time.Now())
		if err != nil {
			return err
		}
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
		feed, err := db.GetWorkloadEventFeed(from, to,
			cliValue(args[1:], "--source"),
			cliValue(args[1:], "--model"),
			cliValue(args[1:], "--project"),
			cliValue(args[1:], "--phase"),
			cliValue(args[1:], "--severity"),
			cliInt(args[1:], "--limit", 100),
			maxAge)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(feed)
	case "close":
		id := firstNonEmptyCLI(cliValue(args[1:], "--id"), cliValue(args[1:], "--workload-id"))
		status := firstNonEmptyCLI(cliValue(args[1:], "--status"), "completed")
		if err := db.CloseWorkload(id, status, cliValue(args[1:], "--outcome")); err != nil {
			return err
		}
		fmt.Printf("%s\t%s\n", id, status)
	case "link":
		sourceID := firstNonEmptyCLI(cliValue(args[1:], "--source-workload-id"), cliValue(args[1:], "--source-id"), cliValue(args[1:], "--from"), cliValue(args[1:], "--id"))
		targetID := firstNonEmptyCLI(cliValue(args[1:], "--target-workload-id"), cliValue(args[1:], "--target-id"), cliValue(args[1:], "--to"))
		linkID, err := db.LinkWorkloads(sourceID, targetID, cliValue(args[1:], "--relation"), cliValue(args[1:], "--reason"), cliValue(args[1:], "--created-by"), 1)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"link_id": linkID, "source_workload_id": sourceID, "target_workload_id": targetID})
	case "start-run":
		workloadID := firstNonEmptyCLI(cliValue(args[1:], "--workload-id"), cliValue(args[1:], "--id"))
		source := cliValue(args[1:], "--source")
		agentName := firstNonEmptyCLI(cliValue(args[1:], "--agent-name"), cliValue(args[1:], "--agent"), source, "agent")
		idempotencyKey := firstNonEmptyCLI(cliValue(args[1:], "--idempotency-key"), cliValue(args[1:], "--idempotency_key"))
		runID, replayed, err := db.StartAgentRunIdempotent(idempotencyKey, workloadID, source, agentName, cliValue(args[1:], "--command"), cliValue(args[1:], "--cwd"))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"workload_id": workloadID, "run_id": runID, "status": "running", "idempotent_replay": replayed})
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
	case "claim-next", "claim":
		return runWorkloadClaimNextCLI(args[1:], db)
	case "queue", "queue-status":
		return runWorkloadQueueCLI(args[1:], db)
	case "lease", "leases":
		return runWorkloadLeaseCLI(args[1:], db)
	case "context", "record-context":
		return runWorkloadContextCLI(args[1:], db)
	case "tool", "record-tool":
		return runWorkloadToolCLI(args[1:], db)
	case "evaluation", "record-evaluation", "eval":
		return runWorkloadEvaluationCLI(args[1:], db)
	default:
		return fmt.Errorf("unknown workload command %q", args[0])
	}
	return nil
}

func runWorkloadClaimNextCLI(args []string, db *storage.DB) error {
	ttl, err := cliLeaseTTL(args)
	if err != nil {
		return err
	}
	result, err := db.ClaimNextWorkload(cliValue(args, "--holder"), cliValue(args, "--purpose"), ttl, storage.WorkloadClaimFilter{
		Source:  cliValue(args, "--source"),
		Project: cliValue(args, "--project"),
		Repo:    cliValue(args, "--repo"),
		Team:    cliValue(args, "--team"),
		Owner:   cliValue(args, "--owner"),
		Status:  cliValue(args, "--status"),
		Query:   firstNonEmptyCLI(cliValue(args, "--q"), cliValue(args, "--query")),
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(result)
}

func runWorkloadQueueCLI(args []string, db *storage.DB) error {
	stats, err := db.GetWorkloadQueueStats(storage.WorkloadClaimFilter{
		Source:  cliValue(args, "--source"),
		Project: cliValue(args, "--project"),
		Repo:    cliValue(args, "--repo"),
		Team:    cliValue(args, "--team"),
		Owner:   cliValue(args, "--owner"),
		Status:  cliValue(args, "--status"),
		Query:   firstNonEmptyCLI(cliValue(args, "--q"), cliValue(args, "--query")),
	})
	if err != nil {
		return err
	}
	return json.NewEncoder(os.Stdout).Encode(stats)
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

func runWorkloadLeaseCLI(args []string, db *storage.DB) error {
	if len(args) == 0 || args[0] == "list" || strings.HasPrefix(args[0], "--") {
		rows, err := db.ListWorkloadLeases(cliBool(args, "--include-inactive"), cliInt(args, "--limit", 200))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"rows": rows})
	}
	switch args[0] {
	case "claim-next":
		return runWorkloadClaimNextCLI(args[1:], db)
	case "acquire", "claim":
		workloadID := firstNonEmptyCLI(cliValue(args[1:], "--workload-id"), cliValue(args[1:], "--id"))
		ttl, err := cliLeaseTTL(args[1:])
		if err != nil {
			return err
		}
		lease, err := db.AcquireWorkloadLease(workloadID, cliValue(args[1:], "--holder"), cliValue(args[1:], "--purpose"), ttl)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"ok": true, "lease": lease})
	case "renew":
		ttl, err := cliLeaseTTL(args[1:])
		if err != nil {
			return err
		}
		lease, err := db.RenewWorkloadLease(cliValue(args[1:], "--lease-id"), cliValue(args[1:], "--lease-token"), ttl)
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"ok": true, "lease": lease})
	case "release":
		lease, err := db.ReleaseWorkloadLease(cliValue(args[1:], "--lease-id"), cliValue(args[1:], "--lease-token"))
		if err != nil {
			return err
		}
		return json.NewEncoder(os.Stdout).Encode(map[string]interface{}{"ok": true, "lease": lease})
	default:
		return fmt.Errorf("usage: agent-ledger workload lease [list|claim-next|acquire|renew|release] [--workload-id id] [--holder name] [--purpose text] [--ttl 30m] [--lease-id id] [--lease-token token]")
	}
}

func cliLeaseTTL(args []string) (time.Duration, error) {
	if raw := cliValue(args, "--ttl"); raw != "" {
		ttl, err := time.ParseDuration(raw)
		if err != nil {
			return 0, err
		}
		if ttl <= 0 {
			return 0, fmt.Errorf("--ttl must be positive")
		}
		return ttl, nil
	}
	seconds := cliInt(args, "--ttl-seconds", 0)
	if seconds > 0 {
		return time.Duration(seconds) * time.Second, nil
	}
	return 0, nil
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

func runWorkloadEvaluationCLI(args []string, db *storage.DB) error {
	workloadID := firstNonEmptyCLI(cliValue(args, "--workload-id"), cliValue(args, "--id"))
	if workloadID == "" {
		return fmt.Errorf("--workload-id is required")
	}
	status := firstNonEmptyCLI(cliValue(args, "--status"), "unknown")
	signal := firstNonEmptyCLI(cliValue(args, "--signal"), cliValue(args, "--type"), "manual")
	score, err := parseFloat(firstNonEmptyCLI(cliValue(args, "--score"), "0"))
	if err != nil {
		return fmt.Errorf("invalid --score: %w", err)
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
		"evaluation_id": firstNonEmptyCLI(cliValue(args, "--evaluation-id"), cliValue(args, "--eval-id")),
		"evaluator":     firstNonEmptyCLI(cliValue(args, "--evaluator"), "local"),
		"status":        status,
		"score":         score,
		"signal":        signal,
		"notes":         cliValue(args, "--notes"),
	}
	rawPayload, _ := json.Marshal(payload)
	result, err := db.IngestCanonicalEvent(storage.CanonicalEvent{
		EventID:       cliValue(args, "--event-id"),
		Source:        firstNonEmptyCLI(cliValue(args, "--source"), "local"),
		EventType:     "evaluation.recorded",
		SourceEventID: firstNonEmptyCLI(cliValue(args, "--evaluation-id"), cliValue(args, "--eval-id"), cliValue(args, "--source-event-id")),
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

func cliValues(args []string, key string) []string {
	values := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == key && i+1 < len(args) {
			values = append(values, args[i+1])
			i++
			continue
		}
		if strings.HasPrefix(args[i], key+"=") {
			values = append(values, strings.TrimPrefix(args[i], key+"="))
		}
	}
	return values
}

func appendCLIValues(groups ...[]string) []string {
	values := []string{}
	for _, group := range groups {
		for _, raw := range group {
			for _, part := range strings.Split(raw, ",") {
				if trimmed := strings.TrimSpace(part); trimmed != "" {
					values = append(values, trimmed)
				}
			}
		}
	}
	return values
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

func firstCLIBool(args []string, keys ...string) bool {
	for _, key := range keys {
		if cliBool(args, key) {
			return true
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

func firstNonEmptyInt(values ...int) int {
	for _, v := range values {
		if v > 0 {
			return v
		}
	}
	return 0
}

func firstNonEmptyCLI(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
