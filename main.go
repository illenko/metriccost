package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/illenko/whodidthis/api"
	"github.com/illenko/whodidthis/collector"
	"github.com/illenko/whodidthis/config"
	"github.com/illenko/whodidthis/prometheus"
	"github.com/illenko/whodidthis/scheduler"
	"github.com/illenko/whodidthis/storage"
)

func main() {
	logLevel := slog.LevelInfo
	if os.Getenv("DEBUG") == "true" {
		logLevel = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})))

	configPath := os.Getenv("CONFIG_PATH")
	if configPath == "" {
		configPath = "config.yaml"
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	db, err := storage.New(cfg.Storage.Path)
	if err != nil {
		slog.Error("failed to initialize database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	snapshotsRepo := storage.NewSnapshotsRepository(db)
	servicesRepo := storage.NewServicesRepository(db)
	metricsRepo := storage.NewMetricsRepository(db)
	labelsRepo := storage.NewLabelsRepository(db)

	promClient, err := prometheus.NewClient(prometheus.Config{
		URL:      cfg.Prometheus.URL,
		Username: cfg.Prometheus.Username,
		Password: cfg.Prometheus.Password,
	})
	if err != nil {
		slog.Error("failed to create prometheus client", "error", err)
		os.Exit(1)
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
		Interval: cfg.Scan.Interval,
	})

	handlers := api.NewHandlers(api.HandlersConfig{
		Snapshots: snapshotsRepo,
		Services:  servicesRepo,
		Metrics:   metricsRepo,
		Labels:    labelsRepo,
		Scheduler: sched,
		DB:        db,
	})

	server := api.NewServer(handlers, api.ServerConfig{
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
		server.Shutdown(context.Background())
	}()

	if err := server.Start(); err != nil {
		slog.Error("server error", "error", err)
		os.Exit(1)
	}
}
