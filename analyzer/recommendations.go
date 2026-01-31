package analyzer

import (
	"context"
	"fmt"
	"time"

	"github.com/illenko/metriccost/models"
	"github.com/illenko/metriccost/storage"
)

type RecommendationsEngine struct {
	metricsRepo    *storage.MetricsRepository
	dashboardsRepo *storage.DashboardsRepository
	recsRepo       *storage.RecommendationsRepository
	sizeCalc       *SizeCalculator

	highCardinalityThreshold int
	highLabelValueThreshold  int
	minSizeImpactBytes       int64
}

type RecommendationsConfig struct {
	HighCardinalityThreshold int
	HighLabelValueThreshold  int
	MinSizeImpactMB          int
}

func NewRecommendationsEngine(
	metricsRepo *storage.MetricsRepository,
	dashboardsRepo *storage.DashboardsRepository,
	recsRepo *storage.RecommendationsRepository,
	sizeCalc *SizeCalculator,
	cfg RecommendationsConfig,
) *RecommendationsEngine {
	if cfg.HighCardinalityThreshold == 0 {
		cfg.HighCardinalityThreshold = 10000
	}
	if cfg.HighLabelValueThreshold == 0 {
		cfg.HighLabelValueThreshold = 100
	}
	if cfg.MinSizeImpactMB == 0 {
		cfg.MinSizeImpactMB = 100
	}

	return &RecommendationsEngine{
		metricsRepo:              metricsRepo,
		dashboardsRepo:           dashboardsRepo,
		recsRepo:                 recsRepo,
		sizeCalc:                 sizeCalc,
		highCardinalityThreshold: cfg.HighCardinalityThreshold,
		highLabelValueThreshold:  cfg.HighLabelValueThreshold,
		minSizeImpactBytes:       int64(cfg.MinSizeImpactMB) * 1024 * 1024,
	}
}

type AnalyzeResult struct {
	TotalRecommendations int
	HighPriority         int
	MediumPriority       int
	LowPriority          int
	PotentialSavings     int64
}

func (e *RecommendationsEngine) Analyze(ctx context.Context) (*AnalyzeResult, error) {
	if err := e.recsRepo.Clear(ctx); err != nil {
		return nil, fmt.Errorf("failed to clear old recommendations: %w", err)
	}

	latestTime, err := e.metricsRepo.GetLatestCollectionTime(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get latest collection time: %w", err)
	}
	if latestTime.IsZero() {
		return &AnalyzeResult{}, nil
	}

	metrics, err := e.metricsRepo.List(ctx, latestTime, storage.ListOptions{Limit: 100000})
	if err != nil {
		return nil, fmt.Errorf("failed to list metrics: %w", err)
	}

	usedMetrics, err := e.dashboardsRepo.GetAllMetricsUsed(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get used metrics: %w", err)
	}

	var recommendations []*models.Recommendation
	now := time.Now()

	for _, m := range metrics {
		recs := e.analyzeMetric(m, usedMetrics, now)
		recommendations = append(recommendations, recs...)
	}

	if len(recommendations) > 0 {
		if err := e.recsRepo.SaveBatch(ctx, recommendations); err != nil {
			return nil, fmt.Errorf("failed to save recommendations: %w", err)
		}
	}

	result := &AnalyzeResult{
		TotalRecommendations: len(recommendations),
	}

	for _, r := range recommendations {
		switch r.Priority {
		case models.PriorityHigh:
			result.HighPriority++
		case models.PriorityMedium:
			result.MediumPriority++
		case models.PriorityLow:
			result.LowPriority++
		}
		result.PotentialSavings += r.PotentialReductionBytes
	}

	return result, nil
}

func (e *RecommendationsEngine) analyzeMetric(m *models.MetricSnapshot, usedMetrics map[string]struct{}, now time.Time) []*models.Recommendation {
	var recs []*models.Recommendation

	_, isUsed := usedMetrics[m.MetricName]

	if rec := e.checkUnused(m, isUsed, now); rec != nil {
		recs = append(recs, rec)
	}

	if rec := e.checkHighCardinality(m, isUsed, now); rec != nil {
		recs = append(recs, rec)
	}

	if labelRecs := e.checkHighCardinalityLabels(m, now); len(labelRecs) > 0 {
		recs = append(recs, labelRecs...)
	}

	return recs
}

func (e *RecommendationsEngine) checkUnused(m *models.MetricSnapshot, isUsed bool, now time.Time) *models.Recommendation {
	if isUsed {
		return nil
	}

	if m.EstimatedSizeBytes < e.minSizeImpactBytes {
		return nil
	}

	return &models.Recommendation{
		CreatedAt:               now,
		MetricName:              m.MetricName,
		Type:                    models.RecommendationUnused,
		Priority:                models.PriorityHigh,
		CurrentCardinality:      m.Cardinality,
		CurrentSizeBytes:        m.EstimatedSizeBytes,
		PotentialReductionBytes: m.EstimatedSizeBytes,
		Description:             fmt.Sprintf("Metric '%s' is not used in any Grafana dashboard", m.MetricName),
		SuggestedAction:         "Consider dropping this metric if it's not needed for alerting or other purposes",
	}
}

func (e *RecommendationsEngine) checkHighCardinality(m *models.MetricSnapshot, isUsed bool, now time.Time) *models.Recommendation {
	if m.Cardinality < e.highCardinalityThreshold {
		return nil
	}

	if m.EstimatedSizeBytes < e.minSizeImpactBytes {
		return nil
	}

	priority := models.PriorityMedium
	if !isUsed || m.Cardinality > e.highCardinalityThreshold*10 {
		priority = models.PriorityHigh
	}

	// Potential reduction = full size if dropped, or based on reducing to threshold
	var potentialReduction int64
	if !isUsed {
		potentialReduction = m.EstimatedSizeBytes
	} else {
		// Estimate reduction if cardinality reduced to threshold
		reducedSize := e.sizeCalc.EstimateSize(e.highCardinalityThreshold)
		potentialReduction = m.EstimatedSizeBytes - reducedSize
	}

	return &models.Recommendation{
		CreatedAt:               now,
		MetricName:              m.MetricName,
		Type:                    models.RecommendationHighCardinality,
		Priority:                priority,
		CurrentCardinality:      m.Cardinality,
		CurrentSizeBytes:        m.EstimatedSizeBytes,
		PotentialReductionBytes: potentialReduction,
		Description:             fmt.Sprintf("Metric '%s' has high cardinality (%d time series)", m.MetricName, m.Cardinality),
		SuggestedAction:         "Review labels for high-cardinality values (user IDs, request IDs, etc.) and consider using recording rules for aggregation",
	}
}

func (e *RecommendationsEngine) checkHighCardinalityLabels(m *models.MetricSnapshot, now time.Time) []*models.Recommendation {
	if len(m.Labels) == 0 {
		return nil
	}

	var recs []*models.Recommendation

	for labelName, uniqueCount := range m.Labels {
		if uniqueCount < e.highLabelValueThreshold {
			continue
		}

		// If this label has N unique values, removing it could reduce cardinality by factor of N
		// New cardinality ≈ current / uniqueCount, so reduction = current - (current / uniqueCount)
		reducedCardinality := m.Cardinality / uniqueCount
		if reducedCardinality < 1 {
			reducedCardinality = 1
		}
		reducedSize := e.sizeCalc.EstimateSize(reducedCardinality)
		estimatedReduction := m.EstimatedSizeBytes - reducedSize

		if estimatedReduction < e.minSizeImpactBytes {
			continue
		}

		recs = append(recs, &models.Recommendation{
			CreatedAt:               now,
			MetricName:              m.MetricName,
			Type:                    models.RecommendationRedundantLabels,
			Priority:                models.PriorityMedium,
			CurrentCardinality:      m.Cardinality,
			CurrentSizeBytes:        m.EstimatedSizeBytes,
			PotentialReductionBytes: estimatedReduction,
			Description:             fmt.Sprintf("Label '%s' on metric '%s' has %d unique values", labelName, m.MetricName, uniqueCount),
			SuggestedAction:         fmt.Sprintf("Consider removing or aggregating the '%s' label if it contains high-cardinality values like IDs", labelName),
		})
	}

	return recs
}
