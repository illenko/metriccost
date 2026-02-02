package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/illenko/whodidthis/collector"
)

type Scheduler struct {
	collector *collector.Collector
	interval  time.Duration
	stopCh    chan struct{}
	status    *ScanStatus
	mu        sync.RWMutex
}

type ScanStatus struct {
	Running       bool      `json:"running"`
	Progress      string    `json:"progress,omitempty"`
	LastScanAt    time.Time `json:"last_scan_at,omitempty"`
	LastDuration  string    `json:"last_duration,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
	NextScanAt    time.Time `json:"next_scan_at,omitempty"`
	TotalServices int       `json:"total_services,omitempty"`
	TotalSeries   int64     `json:"total_series,omitempty"`
}

type Config struct {
	Interval time.Duration
}

func New(collector *collector.Collector, cfg Config) *Scheduler {
	if cfg.Interval == 0 {
		cfg.Interval = 24 * time.Hour
	}

	return &Scheduler{
		collector: collector,
		interval:  cfg.Interval,
		stopCh:    make(chan struct{}),
		status:    &ScanStatus{},
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

func (s *Scheduler) TriggerScan(_ context.Context) error {
	s.mu.RLock()
	if s.status.Running {
		s.mu.RUnlock()
		return ErrScanAlreadyRunning
	}
	s.mu.RUnlock()

	go s.runScan(context.Background())
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
	s.status.Progress = "Starting..."
	s.mu.Unlock()

	start := time.Now()
	slog.Info("starting scan")

	var scanErr error
	defer func() {
		s.mu.Lock()
		s.status.Running = false
		s.status.Progress = ""
		s.status.LastScanAt = start
		s.status.LastDuration = time.Since(start).String()
		if scanErr != nil {
			s.status.LastError = scanErr.Error()
		}
		s.mu.Unlock()
	}()

	// Progress callback
	progress := func(phase string, current, total int, detail string) {
		s.mu.Lock()
		if total > 0 {
			s.status.Progress = detail
		} else {
			s.status.Progress = phase
		}
		s.mu.Unlock()
	}

	// Run collection
	result, err := s.collector.Collect(ctx, progress)
	if err != nil {
		slog.Error("collection failed", "error", err)
		scanErr = err
		return
	}

	s.mu.Lock()
	s.status.TotalServices = result.TotalServices
	s.status.TotalSeries = result.TotalSeries
	s.mu.Unlock()

	slog.Info("scan complete",
		"services", result.TotalServices,
		"series", result.TotalSeries,
		"duration", time.Since(start),
	)
}

type scanError string

func (e scanError) Error() string { return string(e) }

const ErrScanAlreadyRunning = scanError("scan already running")
