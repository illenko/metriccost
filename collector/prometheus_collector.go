package collector

import (
	"context"
	"log/slog"
	"time"

	"github.com/illenko/whodidthis/config"
	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/prometheus"
	"github.com/illenko/whodidthis/storage"
)

type Collector struct {
	client       *prometheus.Client
	snapshots    *storage.SnapshotsRepository
	services     *storage.ServicesRepository
	metrics      *storage.MetricsRepository
	labels       *storage.LabelsRepository
	serviceLabel string
	sampleLimit  int
}

func NewCollector(
	client *prometheus.Client,
	snapshots *storage.SnapshotsRepository,
	services *storage.ServicesRepository,
	metrics *storage.MetricsRepository,
	labels *storage.LabelsRepository,
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
	}
}

type CollectResult struct {
	SnapshotID    int64
	TotalServices int
	TotalSeries   int64
	Duration      time.Duration
}

// ProgressCallback is called to report scan progress
type ProgressCallback func(phase string, current, total int, detail string)

func (c *Collector) Collect(ctx context.Context, progress ProgressCallback) (*CollectResult, error) {
	start := time.Now()
	collectedAt := start.Truncate(time.Second)

	if progress == nil {
		progress = func(string, int, int, string) {}
	}

	slog.Info("starting service discovery", "label", c.serviceLabel)
	progress("discovering", 0, 0, "Discovering services...")

	// Step 1: Create snapshot
	snapshot := &models.Snapshot{
		CollectedAt: collectedAt,
	}
	snapshotID, err := c.snapshots.Create(ctx, snapshot)
	if err != nil {
		return nil, err
	}
	snapshot.ID = snapshotID

	// Step 2: Discover services
	serviceInfos, err := c.client.DiscoverServices(ctx, c.serviceLabel)
	if err != nil {
		return nil, err
	}

	slog.Info("discovered services", "count", len(serviceInfos))

	var totalSeries int64

	// Step 3: For each service, collect metrics and labels
	for i, svc := range serviceInfos {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		progress("scanning", i+1, len(serviceInfos), svc.Name)
		slog.Info("scanning service", "name", svc.Name, "progress", i+1, "total", len(serviceInfos))

		serviceSnapshot, err := c.collectService(ctx, snapshotID, svc)
		if err != nil {
			slog.Error("failed to collect service", "name", svc.Name, "error", err)
			continue
		}

		totalSeries += int64(serviceSnapshot.TotalSeries)
	}

	// Step 4: Update snapshot with totals
	snapshot.TotalServices = len(serviceInfos)
	snapshot.TotalSeries = totalSeries
	snapshot.ScanDurationMs = int(time.Since(start).Milliseconds())

	if err := c.snapshots.Update(ctx, snapshot); err != nil {
		return nil, err
	}

	duration := time.Since(start)
	slog.Info("collection complete",
		"services", len(serviceInfos),
		"total_series", totalSeries,
		"duration", duration,
	)

	return &CollectResult{
		SnapshotID:    snapshotID,
		TotalServices: len(serviceInfos),
		TotalSeries:   totalSeries,
		Duration:      duration,
	}, nil
}

func (c *Collector) collectService(ctx context.Context, snapshotID int64, svc prometheus.ServiceInfo) (*models.ServiceSnapshot, error) {
	// Get metrics for this service
	metricInfos, err := c.client.GetMetricsForService(ctx, c.serviceLabel, svc.Name)
	if err != nil {
		return nil, err
	}

	// Create service snapshot
	serviceSnapshot := &models.ServiceSnapshot{
		SnapshotID:  snapshotID,
		ServiceName: svc.Name,
		TotalSeries: svc.SeriesCount,
		MetricCount: len(metricInfos),
	}

	serviceSnapshotID, err := c.services.Create(ctx, serviceSnapshot)
	if err != nil {
		return nil, err
	}
	serviceSnapshot.ID = serviceSnapshotID

	// Collect each metric
	for _, metric := range metricInfos {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if err := c.collectMetric(ctx, serviceSnapshotID, svc.Name, metric); err != nil {
			slog.Debug("failed to collect metric", "service", svc.Name, "metric", metric.Name, "error", err)
			continue
		}
	}

	return serviceSnapshot, nil
}

func (c *Collector) collectMetric(ctx context.Context, serviceSnapshotID int64, serviceName string, metric prometheus.MetricInfo) error {
	// Get labels for this metric
	labelInfos, err := c.client.GetLabelsForMetric(ctx, c.serviceLabel, serviceName, metric.Name, c.sampleLimit)
	if err != nil {
		// Log but don't fail - we can still store the metric without labels
		slog.Debug("failed to get labels", "metric", metric.Name, "error", err)
		labelInfos = nil
	}

	// Create metric snapshot
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

	// Store labels
	for _, label := range labelInfos {
		labelSnapshot := &models.LabelSnapshot{
			MetricSnapshotID:  metricSnapshotID,
			LabelName:         label.Name,
			UniqueValuesCount: label.UniqueValues,
			SampleValues:      label.SampleValues,
		}

		if _, err := c.labels.Create(ctx, labelSnapshot); err != nil {
			slog.Debug("failed to store label", "label", label.Name, "error", err)
			continue
		}
	}

	return nil
}
