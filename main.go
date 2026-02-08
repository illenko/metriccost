package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/illenko/whodidthis/analyzer"
	"github.com/illenko/whodidthis/api"
	"github.com/illenko/whodidthis/api/handler"
	"github.com/illenko/whodidthis/collector"
	"github.com/illenko/whodidthis/config"
	"github.com/illenko/whodidthis/prometheus"
	"github.com/illenko/whodidthis/scheduler"
	"github.com/illenko/whodidthis/storage"
)

var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()})))
	slog.Info("starting whodidthis", "version", version, "commit", commit, "built", buildTime)

	db, err := storage.New(cfg.Storage.Path)
	if err != nil {
		return fmt.Errorf("init database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			slog.Error("failed to close database", "error", err)
		}
	}()

	snapshotsRepo := storage.NewSnapshotsRepository(db)
	servicesRepo := storage.NewServicesRepository(db)
	metricsRepo := storage.NewMetricsRepository(db)
	labelsRepo := storage.NewLabelsRepository(db)

	promClient, err := prometheus.NewClient(prometheus.Config{
		URL:      cfg.Prometheus.URL,
		Username: cfg.Prometheus.Username,
		Password: cfg.Prometheus.Password,
		Timeout:  cfg.Prometheus.Timeout,
	})
	if err != nil {
		return fmt.Errorf("create prometheus client: %w", err)
	}

	coll := collector.NewCollector(
		promClient,
		snapshotsRepo,
		servicesRepo,
		metricsRepo,
		labelsRepo,
		cfg,
	)

	sched := scheduler.New(coll, scheduler.Config{
		Interval:  cfg.Scan.Interval,
		Retention: cfg.RetentionDuration(),
		DB:        db,
	})

	analysisRepo := storage.NewAnalysisRepository(db)

	var snapshotAnalyzer *analyzer.Analyzer
	if cfg.Gemini.APIKey != "" {
		toolExecutor := analyzer.NewToolExecutor(servicesRepo, metricsRepo, labelsRepo)
		snapshotAnalyzer, err = analyzer.New(context.Background(), analyzer.Config{
			Gemini:       cfg.Gemini,
			ToolExecutor: toolExecutor,
			AnalysisRepo: analysisRepo,
			Snapshots:    snapshotsRepo,
			Services:     servicesRepo,
		})
		if err != nil {
			return fmt.Errorf("create analyzer: %w", err)
		}
		slog.Info("AI analysis enabled", "model", cfg.Gemini.Model)
	} else {
		slog.Warn("AI analysis disabled: WDT_GEMINI_API_KEY not set")
	}

	healthHandler := handler.NewHealthHandler(snapshotsRepo, db, promClient)
	scansHandler := handler.NewScansHandler(snapshotsRepo, sched)
	analysisHandler := handler.NewAnalysisHandler(snapshotAnalyzer)
	servicesHandler := handler.NewServicesHandler(servicesRepo)
	metricsHandler := handler.NewMetricsHandler(servicesRepo, metricsRepo)
	labelsHandler := handler.NewLabelsHandler(servicesRepo, metricsRepo, labelsRepo)

	server := api.NewServer(
		healthHandler,
		scansHandler,
		analysisHandler,
		servicesHandler,
		metricsHandler,
		labelsHandler,
		api.ServerConfig{
			Host: cfg.Server.Host,
			Port: cfg.Server.Port,
		})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go sched.Start(ctx)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		slog.Info("shutting down...")
		cancel()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()

		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
	}()

	return server.Start()
}
