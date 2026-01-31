package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/illenko/metriccost/internal/analyzer"
	"github.com/illenko/metriccost/internal/api"
	"github.com/illenko/metriccost/internal/collector"
	"github.com/illenko/metriccost/internal/config"
	"github.com/illenko/metriccost/internal/grafana"
	"github.com/illenko/metriccost/internal/prometheus"
	"github.com/illenko/metriccost/internal/scheduler"
	"github.com/illenko/metriccost/internal/storage"
)

func main() {
	// Setup logging
	logLevel := slog.LevelInfo
	if os.Getenv("DEBUG") == "true" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	// Load config
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize database
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "metriccost.db"
	}

	db, err := storage.New(dbPath)
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	// Initialize repositories
	metricsRepo := storage.NewMetricsRepository(db)
	snapshotsRepo := storage.NewSnapshotsRepository(db)
	recsRepo := storage.NewRecommendationsRepository(db)
	dashboardsRepo := storage.NewDashboardsRepository(db)

	// Initialize Prometheus client
	promClient, err := prometheus.NewClient(prometheus.Config{
		URL:      cfg.Prometheus.URL,
		Username: cfg.Prometheus.Username,
		Password: cfg.Prometheus.Password,
	})
	if err != nil {
		slog.Error("failed to create prometheus client", "error", err)
		os.Exit(1)
	}

	// Initialize Grafana client (optional)
	var grafanaClient *grafana.Client
	if cfg.Grafana.URL != "" {
		grafanaClient, err = grafana.NewClient(grafana.Config{
			URL:      cfg.Grafana.URL,
			APIToken: cfg.Grafana.APIToken,
			Username: cfg.Grafana.Username,
			Password: cfg.Grafana.Password,
		})
		if err != nil {
			slog.Warn("failed to create grafana client", "error", err)
		}
	}

	// Initialize team matcher
	teamPatterns := make(map[string][]string)
	for team, tc := range cfg.Teams {
		teamPatterns[team] = tc.MetricsPatterns
	}
	teamMatcher, _ := analyzer.NewTeamMatcher(teamPatterns)

	// Initialize size calculator
	sizeCalc := analyzer.NewSizeCalculator(analyzer.SizeConfig{
		BytesPerSample: cfg.SizeModel.BytesPerSample,
		RetentionDays:  cfg.SizeModel.DefaultRetentionDays,
		ScrapeInterval: cfg.SizeModel.ScrapeInterval,
	})

	// Initialize collectors
	promCollector := collector.NewPrometheusCollector(
		promClient, metricsRepo, snapshotsRepo, teamMatcher, sizeCalc,
		collector.CollectorConfig{},
	)

	var grafanaCollector *collector.GrafanaCollector
	if grafanaClient != nil {
		grafanaCollector = collector.NewGrafanaCollector(grafanaClient, dashboardsRepo, cfg.Grafana.URL)
	}

	// Initialize recommendations engine
	recsEngine := analyzer.NewRecommendationsEngine(
		metricsRepo, dashboardsRepo, recsRepo, sizeCalc,
		analyzer.RecommendationsConfig{
			HighCardinalityThreshold: cfg.Recommendations.HighCardinalityThreshold,
			MinSizeImpactMB:          cfg.Recommendations.MinSizeImpactMB,
		},
	)

	// Initialize trends calculator
	trends := analyzer.NewTrendsCalculator(snapshotsRepo, metricsRepo)

	// Initialize scheduler
	sched := scheduler.New(promCollector, grafanaCollector, recsEngine, scheduler.Config{
		Interval: cfg.Collection.Interval,
	})

	// Initialize API handlers
	handlers := api.NewHandlers(api.HandlersConfig{
		MetricsRepo:    metricsRepo,
		RecsRepo:       recsRepo,
		DashboardsRepo: dashboardsRepo,
		SnapshotsRepo:  snapshotsRepo,
		Trends:         trends,
		Scheduler:      sched,
		DB:             db,
	})

	// Initialize server
	server := api.NewServer(handlers, api.ServerConfig{
		Host: cfg.Server.Host,
		Port: cfg.Server.Port,
	})

	// Start scheduler in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		slog.Info("shutting down...")
		cancel()
		server.Shutdown(context.Background())
	}()

	// Start server
	if err := server.Start(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
