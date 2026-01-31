package analyzer

import (
	"context"
	"time"

	"github.com/illenko/metriccost/models"
	"github.com/illenko/metriccost/storage"
)

type TrendsCalculator struct {
	snapshotsRepo *storage.SnapshotsRepository
	metricsRepo   *storage.MetricsRepository
}

func NewTrendsCalculator(
	snapshotsRepo *storage.SnapshotsRepository,
	metricsRepo *storage.MetricsRepository,
) *TrendsCalculator {
	return &TrendsCalculator{
		snapshotsRepo: snapshotsRepo,
		metricsRepo:   metricsRepo,
	}
}

type TrendPeriod string

const (
	TrendPeriod7Days  TrendPeriod = "7d"
	TrendPeriod30Days TrendPeriod = "30d"
	TrendPeriod90Days TrendPeriod = "90d"
)

func (p TrendPeriod) Duration() time.Duration {
	switch p {
	case TrendPeriod7Days:
		return 7 * 24 * time.Hour
	case TrendPeriod30Days:
		return 30 * 24 * time.Hour
	case TrendPeriod90Days:
		return 90 * 24 * time.Hour
	default:
		return 30 * 24 * time.Hour
	}
}

func (t *TrendsCalculator) GetOverallTrends(ctx context.Context, period TrendPeriod) ([]*models.TrendDataPoint, error) {
	since := time.Now().Add(-period.Duration())

	snapshots, err := t.snapshotsRepo.GetTrends(ctx, since)
	if err != nil {
		return nil, err
	}

	var points []*models.TrendDataPoint
	for _, s := range snapshots {
		points = append(points, &models.TrendDataPoint{
			Date:         s.CollectedAt,
			TotalMetrics: s.TotalMetrics,
			Cardinality:  s.TotalCardinality,
			SizeBytes:    s.TotalSizeBytes,
		})
	}

	return points, nil
}

func (t *TrendsCalculator) GetOverview(ctx context.Context) (*models.Overview, error) {
	latest, err := t.snapshotsRepo.GetLatest(ctx)
	if err != nil {
		return nil, err
	}
	if latest == nil {
		return &models.Overview{
			TeamBreakdown: make(map[string]models.TeamMetrics),
		}, nil
	}

	trend, err := t.snapshotsRepo.CalculateTrend(ctx, latest)
	if err != nil {
		trend = 0
	}

	return &models.Overview{
		TotalMetrics:     latest.TotalMetrics,
		TotalCardinality: latest.TotalCardinality,
		TotalSizeBytes:   latest.TotalSizeBytes,
		TrendPercentage:  trend,
		LastScan:         latest.CollectedAt,
		TeamBreakdown:    latest.TeamBreakdown,
	}, nil
}

func (t *TrendsCalculator) GetMetricTrend(ctx context.Context, metricName string) (float64, error) {
	latest, err := t.snapshotsRepo.GetLatest(ctx)
	if err != nil || latest == nil {
		return 0, err
	}

	previous, err := t.snapshotsRepo.GetPrevious(ctx, latest.CollectedAt)
	if err != nil || previous == nil {
		return 0, nil
	}

	return t.metricsRepo.GetTrend(ctx, metricName, latest.CollectedAt, previous.CollectedAt)
}
