package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/zhenzhis/agent-ledger/internal/collector"
	"github.com/zhenzhis/agent-ledger/internal/config"
	"github.com/zhenzhis/agent-ledger/internal/pricing"
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
	if err := db.RebuildUsageAggregates(); err != nil {
		log.Printf("aggregate rebuild: %v", err)
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
			return recalcCostsMode(db, "zero")
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
		return recalcCostsMode(db, "zero")
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
		if err := db.RebuildUsageAggregates(); err != nil {
			log.Printf("aggregate rebuild: %v", err)
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
				if err := db.RebuildUsageAggregates(); err != nil {
					log.Printf("aggregate rebuild: %v", err)
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
			if err := db.RebuildUsageAggregates(); err != nil {
				log.Printf("aggregate rebuild: %v", err)
			}
		}
	}()

	// Start web server
	addr := fmt.Sprintf("%s:%d", cfg.Server.BindAddress, cfg.Server.Port)
	srv := server.New(db, addr, server.Options{
		AuthToken:   cfg.Server.AuthToken,
		AdminToken:  cfg.Server.AdminToken,
		ViewerToken: cfg.Server.ViewerToken,
		RBAC:        cfg.RBAC,
		Privacy:     cfg.Privacy,
		Budgets:     cfg.Budgets,
		Quota:       cfg.Quota,
		Watchdog:    cfg.Watchdog,
		Policies:    cfg.Policies,
		Webhooks:    cfg.Webhooks,
		Teams:       cfg.Teams,
		Pricing:     cfg.Pricing,
		Sources:     sourceOptions,
		Scan:        scanSource,
		Recalc:      func() error { return recalcCostsMode(db, "zero") },
		RecalcMode:  func(mode string) error { return recalcCostsMode(db, mode) },
		PricingSync: func() error { return pricing.SyncWithConfig(db, cfg.Pricing) },
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
		return db.RebuildUsageAggregates()
	}
	if err := db.RecalcCostsMode(prices, pricing.CalcCost, mode); err != nil {
		return err
	}
	return db.RebuildUsageAggregates()
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
		report, err := db.GetDataQuality(cfg.Pricing.StaleAfter)
		if err != nil {
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
		monthFrom := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.Local)
		stats, err := db.GetDashboardStatsFiltered(monthFrom, dayTo, "", "", "")
		if err != nil {
			return err
		}
		models, _ := db.GetCostByModelFiltered(monthFrom, dayTo, "", "")
		fmt.Printf("# Agent Ledger Wrapped\n\n- Tokens: %d\n- Cost: $%.4f\n- Sessions: %d\n", stats.TotalTokens, stats.TotalCost, stats.TotalSessions)
		if len(models) > 0 {
			fmt.Printf("- Top model: %s ($%.4f)\n", models[0].Model, models[0].Cost)
		}
	default:
		return fmt.Errorf("unknown command %q", cmd)
	}
	return nil
}
