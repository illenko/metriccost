package collector

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/illenko/whodidthis/config"
	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/prometheus"
	"github.com/illenko/whodidthis/storage"
)

type Collector struct {
	client       prometheus.MetricsClient
	snapshots    storage.SnapshotsRepo
	services     storage.ServicesRepo
	metrics      storage.MetricsRepo
	labels       storage.LabelsRepo
	serviceLabel string
	sampleLimit  int
	concurrency  int
	logger       *slog.Logger
}

func NewCollector(
	client prometheus.MetricsClient,
	snapshots storage.SnapshotsRepo,
	services storage.ServicesRepo,
	metrics storage.MetricsRepo,
	labels storage.LabelsRepo,
	cfg *config.Config,
) *Collector {
	return &Collector{
		client:       client,
		snapshots:    snapshots,
		services:     services,
		metrics:      metrics,
		labels:       labels,
		serviceLabel: cfg.Discovery.ServiceLabel,
		sampleLimit:  cfg.Scan.SampleValuesLimit,
		concurrency:  cfg.Scan.Concurrency,
		logger:       slog.Default(),
	}
}

type CollectResult struct {
	SnapshotID    int64
	TotalServices int
	TotalSeries   int64
	Duration      time.Duration
}

type ProgressCallback func(phase string, current, total int, detail string)

func (c *Collector) Collect(ctx context.Context, scanID int64, progress ProgressCallback) (*CollectResult, error) {
	logger := c.logger.With("scan_id", scanID)
	start := time.Now()
	collectedAt := start.Truncate(time.Second)

	if progress == nil {
		progress = func(string, int, int, string) {}
	}

	logger.Info("starting service discovery", "label", c.serviceLabel)
	progress("discovering", 0, 0, "Discovering services...")

	snapshot := &models.Snapshot{
		CollectedAt: collectedAt,
	}
	snapshotID, err := c.snapshots.Create(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	snapshot.ID = snapshotID

	serviceInfos, err := c.client.DiscoverServices(ctx, c.serviceLabel)
	if err != nil {
		return nil, err
	}

	logger.Info("discovered services", "count", len(serviceInfos))

	var totalSeries atomic.Int64

	sem := make(chan struct{}, c.concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	completed := 0

	for _, svc := range serviceInfos {
		if ctx.Err() != nil {
			break
		}

		wg.Add(1)
		go func(svc prometheus.ServiceInfo) {
			defer wg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}

			logger.Debug("scanning service", "name", svc.Name)

			mu.Lock()
			progress("processing_service", completed, len(serviceInfos), svc.Name)
			mu.Unlock()

			// collectService always releases the sem slot internally
			serviceSnapshot, err := c.collectService(ctx, snapshotID, svc, sem, progress)

			mu.Lock()
			completed++
			progress("service_complete", completed, len(serviceInfos), svc.Name)
			mu.Unlock()

			if err != nil {
				logger.Error("failed to collect service", "name", svc.Name, "error", err)
				return
			}

			totalSeries.Add(int64(serviceSnapshot.TotalSeries))
		}(svc)
	}

	wg.Wait()

	finalTotalSeries := totalSeries.Load()
	snapshot.TotalServices = len(serviceInfos)
	snapshot.TotalSeries = finalTotalSeries
	snapshot.ScanDurationMs = int(time.Since(start).Milliseconds())

	if err := c.snapshots.Update(ctx, snapshot); err != nil {
		return nil, err
	}

	duration := time.Since(start)
	logger.Info("collection complete",
		"services", len(serviceInfos),
		"total_series", finalTotalSeries,
		"duration", duration,
	)

	return &CollectResult{
		SnapshotID:    snapshotID,
		TotalServices: len(serviceInfos),
		TotalSeries:   finalTotalSeries,
		Duration:      duration,
	}, nil
}

func (c *Collector) collectService(ctx context.Context, snapshotID int64, svc prometheus.ServiceInfo, sem chan struct{}, progress ProgressCallback) (*models.ServiceSnapshot, error) {
	metricInfos, err := c.client.GetMetricsForService(ctx, c.serviceLabel, svc.Name)
	if err != nil {
		<-sem
		return nil, err
	}

	c.logger.Debug("found metrics for service",
		"service", svc.Name,
		"metrics", len(metricInfos),
		"series", svc.SeriesCount,
	)

	progress("collecting_labels", len(metricInfos), int(svc.SeriesCount), svc.Name)

	serviceSnapshot := &models.ServiceSnapshot{
		SnapshotID:  snapshotID,
		ServiceName: svc.Name,
		TotalSeries: svc.SeriesCount,
		MetricCount: len(metricInfos),
	}

	serviceSnapshotID, err := c.services.Create(ctx, serviceSnapshot)
	if err != nil {
		<-sem
		return nil, err
	}
	serviceSnapshot.ID = serviceSnapshotID

	<-sem

	var metricWg sync.WaitGroup
	for _, metric := range metricInfos {
		if ctx.Err() != nil {
			break
		}

		metricWg.Add(1)
		go func(metric prometheus.MetricInfo) {
			defer metricWg.Done()

			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-sem }()

			c.logger.Debug("collecting metric",
				"service", svc.Name,
				"metric", metric.Name,
				"series", metric.SeriesCount,
			)

			if err := c.collectMetric(ctx, serviceSnapshotID, svc.Name, metric); err != nil {
				c.logger.Debug("failed to collect metric", "service", svc.Name, "metric", metric.Name, "error", err)
			}
		}(metric)
	}

	metricWg.Wait()

	return serviceSnapshot, nil
}

func (c *Collector) collectMetric(ctx context.Context, serviceSnapshotID int64, serviceName string, metric prometheus.MetricInfo) error {
	labelInfos, err := c.client.GetLabelsForMetric(ctx, c.serviceLabel, serviceName, metric.Name, c.sampleLimit)
	if err != nil {
		c.logger.Debug("failed to get labels", "metric", metric.Name, "error", err)
		labelInfos = nil
	} else {
		c.logger.Debug("collected labels",
			"metric", metric.Name,
			"labels", len(labelInfos),
		)
	}

	metricSnapshot := &models.MetricSnapshot{
		ServiceSnapshotID: serviceSnapshotID,
		MetricName:        metric.Name,
		SeriesCount:       metric.SeriesCount,
		LabelCount:        len(labelInfos),
	}

	metricSnapshotID, err := c.metrics.Create(ctx, metricSnapshot)
	if err != nil {
		return err
	}

	if len(labelInfos) > 0 {
		labelSnapshots := make([]*models.LabelSnapshot, 0, len(labelInfos))
		for _, label := range labelInfos {
			labelSnapshots = append(labelSnapshots, &models.LabelSnapshot{
				MetricSnapshotID:  metricSnapshotID,
				LabelName:         label.Name,
				UniqueValuesCount: label.UniqueValues,
				SampleValues:      label.SampleValues,
			})
		}

		if err := c.labels.CreateBatch(ctx, labelSnapshots); err != nil {
			c.logger.Debug("failed to batch store labels", "metric", metric.Name, "error", err)
		}
	}

	return nil
}
