package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/illenko/metriccost/analyzer"
	"github.com/illenko/metriccost/collector"
)

type Scheduler struct {
	promCollector    *collector.PrometheusCollector
	grafanaCollector *collector.GrafanaCollector
	recsEngine       *analyzer.RecommendationsEngine

	interval time.Duration
	stopCh   chan struct{}
	status   *ScanStatus
	mu       sync.RWMutex
}

type ScanStatus struct {
	Running      bool      `json:"running"`
	LastScanAt   time.Time `json:"last_scan_at,omitempty"`
	LastDuration string    `json:"last_duration,omitempty"`
	LastError    string    `json:"last_error,omitempty"`
	NextScanAt   time.Time `json:"next_scan_at,omitempty"`

	PrometheusMetrics int   `json:"prometheus_metrics,omitempty"`
	GrafanaDashboards int   `json:"grafana_dashboards,omitempty"`
	Recommendations   int   `json:"recommendations,omitempty"`
	TotalSizeBytes    int64 `json:"total_size_bytes,omitempty"`
}

type Config struct {
	Interval time.Duration
}

func New(
	promCollector *collector.PrometheusCollector,
	grafanaCollector *collector.GrafanaCollector,
	recsEngine *analyzer.RecommendationsEngine,
	cfg Config,
) *Scheduler {
	if cfg.Interval == 0 {
		cfg.Interval = 24 * time.Hour
	}

	return &Scheduler{
		promCollector:    promCollector,
		grafanaCollector: grafanaCollector,
		recsEngine:       recsEngine,
		interval:         cfg.Interval,
		stopCh:           make(chan struct{}),
		status:           &ScanStatus{},
	}
}

func (s *Scheduler) Start(ctx context.Context) {
	slog.Info("starting scheduler", "interval", s.interval)

	// Run initial scan
	s.runScan(ctx)

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		s.status.NextScanAt = time.Now().Add(s.interval)
		s.mu.Unlock()

		select {
		case <-ctx.Done():
			slog.Info("scheduler stopped")
			return
		case <-s.stopCh:
			slog.Info("scheduler stopped")
			return
		case <-ticker.C:
			s.runScan(ctx)
		}
	}
}

func (s *Scheduler) Stop() {
	close(s.stopCh)
}

func (s *Scheduler) TriggerScan(ctx context.Context) error {
	s.mu.RLock()
	if s.status.Running {
		s.mu.RUnlock()
		return ErrScanAlreadyRunning
	}
	s.mu.RUnlock()

	go s.runScan(ctx)
	return nil
}

func (s *Scheduler) GetStatus() ScanStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return *s.status
}

func (s *Scheduler) runScan(ctx context.Context) {
	s.mu.Lock()
	if s.status.Running {
		s.mu.Unlock()
		return
	}
	s.status.Running = true
	s.status.LastError = ""
	s.mu.Unlock()

	start := time.Now()
	slog.Info("starting scan")

	var scanErr error
	defer func() {
		s.mu.Lock()
		s.status.Running = false
		s.status.LastScanAt = start
		s.status.LastDuration = time.Since(start).String()
		if scanErr != nil {
			s.status.LastError = scanErr.Error()
		}
		s.mu.Unlock()
	}()

	// Collect Prometheus metrics
	if s.promCollector != nil {
		promResult, err := s.promCollector.Collect(ctx)
		if err != nil {
			slog.Error("prometheus collection failed", "error", err)
			scanErr = err
		} else {
			s.mu.Lock()
			s.status.PrometheusMetrics = promResult.TotalMetrics
			s.status.TotalSizeBytes = promResult.TotalSizeBytes
			s.mu.Unlock()
		}
	}

	// Collect Grafana dashboards
	if s.grafanaCollector != nil {
		grafanaResult, err := s.grafanaCollector.Collect(ctx)
		if err != nil {
			slog.Error("grafana collection failed", "error", err)
			if scanErr == nil {
				scanErr = err
			}
		} else {
			s.mu.Lock()
			s.status.GrafanaDashboards = grafanaResult.TotalDashboards
			s.mu.Unlock()
		}
	}

	// Generate recommendations
	if s.recsEngine != nil {
		recsResult, err := s.recsEngine.Analyze(ctx)
		if err != nil {
			slog.Error("recommendations analysis failed", "error", err)
			if scanErr == nil {
				scanErr = err
			}
		} else {
			s.mu.Lock()
			s.status.Recommendations = recsResult.TotalRecommendations
			s.mu.Unlock()
		}
	}

	slog.Info("scan complete", "duration", time.Since(start))
}

type scanError string

func (e scanError) Error() string { return string(e) }

const ErrScanAlreadyRunning = scanError("scan already running")
