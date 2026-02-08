package analyzer

import (
	"context"
	"fmt"

	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/storage"
)

type ToolExecutor struct {
	services storage.ServicesRepo
	metrics  storage.MetricsRepo
	labels   storage.LabelsRepo
}

func NewToolExecutor(services storage.ServicesRepo, metrics storage.MetricsRepo, labels storage.LabelsRepo) *ToolExecutor {
	return &ToolExecutor{
		services: services,
		metrics:  metrics,
		labels:   labels,
	}
}

func (e *ToolExecutor) Execute(ctx context.Context, toolName string, args map[string]any) (any, error) {
	switch toolName {
	case "get_service_metrics":
		return e.getServiceMetrics(ctx, args)
	case "get_metric_labels":
		return e.getMetricLabels(ctx, args)
	case "compare_services":
		return e.compareServices(ctx, args)
	default:
		return nil, fmt.Errorf("unknown tool: %s", toolName)
	}
}

type ServiceMetricsResult struct {
	ServiceName string                  `json:"service_name"`
	SnapshotID  int64                   `json:"snapshot_id"`
	TotalSeries int                     `json:"total_series"`
	MetricCount int                     `json:"metric_count"`
	Metrics     []models.MetricSnapshot `json:"metrics"`
}

func (e *ToolExecutor) getServiceMetrics(ctx context.Context, args map[string]any) (*ServiceMetricsResult, error) {
	snapshotID, err := getInt64Arg(args, "snapshot_id")
	if err != nil {
		return nil, err
	}
	serviceName, err := getStringArg(args, "service_name")
	if err != nil {
		return nil, err
	}

	service, err := e.services.GetByName(ctx, snapshotID, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("service %q not found in snapshot %d", serviceName, snapshotID)
	}

	metrics, err := e.metrics.List(ctx, service.ID, storage.MetricListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list metrics: %w", err)
	}

	return &ServiceMetricsResult{
		ServiceName: serviceName,
		SnapshotID:  snapshotID,
		TotalSeries: service.TotalSeries,
		MetricCount: service.MetricCount,
		Metrics:     metrics,
	}, nil
}

type MetricLabelsResult struct {
	ServiceName string                 `json:"service_name"`
	MetricName  string                 `json:"metric_name"`
	SnapshotID  int64                  `json:"snapshot_id"`
	SeriesCount int                    `json:"series_count"`
	Labels      []models.LabelSnapshot `json:"labels"`
}

func (e *ToolExecutor) getMetricLabels(ctx context.Context, args map[string]any) (*MetricLabelsResult, error) {
	snapshotID, err := getInt64Arg(args, "snapshot_id")
	if err != nil {
		return nil, err
	}
	serviceName, err := getStringArg(args, "service_name")
	if err != nil {
		return nil, err
	}
	metricName, err := getStringArg(args, "metric_name")
	if err != nil {
		return nil, err
	}

	service, err := e.services.GetByName(ctx, snapshotID, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get service: %w", err)
	}
	if service == nil {
		return nil, fmt.Errorf("service %q not found in snapshot %d", serviceName, snapshotID)
	}

	metric, err := e.metrics.GetByName(ctx, service.ID, metricName)
	if err != nil {
		return nil, fmt.Errorf("failed to get metric: %w", err)
	}
	if metric == nil {
		return nil, fmt.Errorf("metric %q not found in service %q", metricName, serviceName)
	}

	labels, err := e.labels.List(ctx, metric.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to list labels: %w", err)
	}

	return &MetricLabelsResult{
		ServiceName: serviceName,
		MetricName:  metricName,
		SnapshotID:  snapshotID,
		SeriesCount: metric.SeriesCount,
		Labels:      labels,
	}, nil
}

type CompareServicesResult struct {
	ServiceName      string             `json:"service_name"`
	CurrentSnapshot  *ServiceComparison `json:"current_snapshot"`
	PreviousSnapshot *ServiceComparison `json:"previous_snapshot"`
	MetricChanges    []MetricChange     `json:"metric_changes"`
	UnchangedCount   int                `json:"unchanged_count"`
}

type ServiceComparison struct {
	SnapshotID  int64 `json:"snapshot_id"`
	TotalSeries int   `json:"total_series"`
	MetricCount int   `json:"metric_count"`
}

type MetricChange struct {
	MetricName          string  `json:"metric_name"`
	CurrentSeriesCount  int     `json:"current_series_count"`
	PreviousSeriesCount int     `json:"previous_series_count"`
	Change              int     `json:"change"`
	ChangePercent       float64 `json:"change_percent"`
}

func (e *ToolExecutor) compareServices(ctx context.Context, args map[string]any) (*CompareServicesResult, error) {
	currentSnapshotID, err := getInt64Arg(args, "current_snapshot_id")
	if err != nil {
		return nil, err
	}
	previousSnapshotID, err := getInt64Arg(args, "previous_snapshot_id")
	if err != nil {
		return nil, err
	}
	serviceName, err := getStringArg(args, "service_name")
	if err != nil {
		return nil, err
	}

	currentService, err := e.services.GetByName(ctx, currentSnapshotID, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get current service: %w", err)
	}

	previousService, err := e.services.GetByName(ctx, previousSnapshotID, serviceName)
	if err != nil {
		return nil, fmt.Errorf("failed to get previous service: %w", err)
	}

	result := &CompareServicesResult{
		ServiceName: serviceName,
	}

	if currentService != nil {
		result.CurrentSnapshot = &ServiceComparison{
			SnapshotID:  currentSnapshotID,
			TotalSeries: currentService.TotalSeries,
			MetricCount: currentService.MetricCount,
		}
	}

	if previousService != nil {
		result.PreviousSnapshot = &ServiceComparison{
			SnapshotID:  previousSnapshotID,
			TotalSeries: previousService.TotalSeries,
			MetricCount: previousService.MetricCount,
		}
	}

	var currentMetrics, previousMetrics []models.MetricSnapshot
	if currentService != nil {
		currentMetrics, err = e.metrics.List(ctx, currentService.ID, storage.MetricListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list current metrics: %w", err)
		}
	}
	if previousService != nil {
		previousMetrics, err = e.metrics.List(ctx, previousService.ID, storage.MetricListOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to list previous metrics: %w", err)
		}
	}

	currentMap := make(map[string]int)
	for _, m := range currentMetrics {
		currentMap[m.MetricName] = m.SeriesCount
	}
	previousMap := make(map[string]int)
	for _, m := range previousMetrics {
		previousMap[m.MetricName] = m.SeriesCount
	}

	allMetrics := make(map[string]bool)
	for name := range currentMap {
		allMetrics[name] = true
	}
	for name := range previousMap {
		allMetrics[name] = true
	}

	for name := range allMetrics {
		current := currentMap[name]
		previous := previousMap[name]
		change := current - previous

		if change == 0 {
			result.UnchangedCount++
			continue
		}

		var changePercent float64
		if previous > 0 {
			changePercent = float64(change) / float64(previous) * 100
		} else if current > 0 {
			changePercent = 100 // New metric
		}

		result.MetricChanges = append(result.MetricChanges, MetricChange{
			MetricName:          name,
			CurrentSeriesCount:  current,
			PreviousSeriesCount: previous,
			Change:              change,
			ChangePercent:       changePercent,
		})
	}

	return result, nil
}

func getInt64Arg(args map[string]any, key string) (int64, error) {
	val, ok := args[key]
	if !ok {
		return 0, fmt.Errorf("missing required argument: %s", key)
	}
	switch v := val.(type) {
	case float64:
		return int64(v), nil
	case int64:
		return v, nil
	case int:
		return int64(v), nil
	default:
		return 0, fmt.Errorf("invalid type for %s: %T", key, val)
	}
}

func getStringArg(args map[string]any, key string) (string, error) {
	val, ok := args[key]
	if !ok {
		return "", fmt.Errorf("missing required argument: %s", key)
	}
	str, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("invalid type for %s: expected string, got %T", key, val)
	}
	return str, nil
}
