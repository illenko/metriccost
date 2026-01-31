package collector

import (
	"context"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/illenko/metriccost/internal/grafana"
	"github.com/illenko/metriccost/internal/storage"
	"github.com/illenko/metriccost/pkg/models"
)

type GrafanaCollector struct {
	client        *grafana.Client
	dashboardRepo *storage.DashboardsRepository
	grafanaURL    string
}

func NewGrafanaCollector(
	client *grafana.Client,
	dashboardRepo *storage.DashboardsRepository,
	grafanaURL string,
) *GrafanaCollector {
	return &GrafanaCollector{
		client:        client,
		dashboardRepo: dashboardRepo,
		grafanaURL:    grafanaURL,
	}
}

type GrafanaCollectResult struct {
	TotalDashboards int
	TotalQueries    int
	UniqueMetrics   int
	Duration        time.Duration
	Errors          []error
}

func (c *GrafanaCollector) Collect(ctx context.Context) (*GrafanaCollectResult, error) {
	start := time.Now()
	collectedAt := start.Truncate(time.Second)

	slog.Info("starting grafana dashboard collection")

	dashboards, err := c.client.ListDashboards(ctx)
	if err != nil {
		return nil, err
	}

	slog.Info("found dashboards", "count", len(dashboards))

	result := &GrafanaCollectResult{}
	allMetrics := make(map[string]struct{})
	var stats []*models.DashboardStats

	for i, d := range dashboards {
		if ctx.Err() != nil {
			break
		}

		if (i+1)%10 == 0 || i+1 == len(dashboards) {
			slog.Info("processing dashboards", "progress", i+1, "total", len(dashboards))
		}

		detail, err := c.client.GetDashboard(ctx, d.UID)
		if err != nil {
			slog.Debug("failed to get dashboard", "uid", d.UID, "error", err)
			result.Errors = append(result.Errors, err)
			continue
		}

		metrics := c.extractMetrics(detail)
		for m := range metrics {
			allMetrics[m] = struct{}{}
		}

		queryCount := c.countQueries(detail)
		result.TotalQueries += queryCount

		metricsSlice := make([]string, 0, len(metrics))
		for m := range metrics {
			metricsSlice = append(metricsSlice, m)
		}

		stat := &models.DashboardStats{
			CollectedAt:   collectedAt,
			DashboardUID:  d.UID,
			DashboardName: d.Title,
			FolderName:    d.FolderTitle,
			LastViewedAt:  detail.Meta.UpdatedAt, // Using UpdatedAt as proxy for activity
			QueryCount:    queryCount,
			MetricsUsed:   metricsSlice,
		}

		stats = append(stats, stat)
		result.TotalDashboards++
	}

	if len(stats) > 0 {
		if err := c.dashboardRepo.SaveBatch(ctx, stats); err != nil {
			return nil, err
		}
	}

	result.UniqueMetrics = len(allMetrics)
	result.Duration = time.Since(start)

	slog.Info("grafana collection complete",
		"dashboards", result.TotalDashboards,
		"queries", result.TotalQueries,
		"unique_metrics", result.UniqueMetrics,
		"duration", result.Duration,
		"errors", len(result.Errors),
	)

	return result, nil
}

func (c *GrafanaCollector) extractMetrics(detail *grafana.DashboardDetail) map[string]struct{} {
	metrics := make(map[string]struct{})

	var extractFromPanels func(panels []grafana.Panel)
	extractFromPanels = func(panels []grafana.Panel) {
		for _, panel := range panels {
			for _, target := range panel.Targets {
				expr := target.Expr
				if expr == "" {
					expr = target.Expression
				}
				if expr == "" {
					continue
				}

				extracted := extractMetricNames(expr)
				for _, m := range extracted {
					metrics[m] = struct{}{}
				}
			}

			if len(panel.Panels) > 0 {
				extractFromPanels(panel.Panels)
			}
		}
	}

	extractFromPanels(detail.Dashboard.Panels)
	return metrics
}

func (c *GrafanaCollector) countQueries(detail *grafana.DashboardDetail) int {
	count := 0

	var countFromPanels func(panels []grafana.Panel)
	countFromPanels = func(panels []grafana.Panel) {
		for _, panel := range panels {
			count += len(panel.Targets)
			if len(panel.Panels) > 0 {
				countFromPanels(panel.Panels)
			}
		}
	}

	countFromPanels(detail.Dashboard.Panels)
	return count
}

var metricNameRegex = regexp.MustCompile(`([a-zA-Z_:][a-zA-Z0-9_:]*)\s*(\{|$|\()`)

var promqlFuncs = map[string]struct{}{
	"sum": {}, "avg": {}, "min": {}, "max": {}, "count": {},
	"rate": {}, "irate": {}, "increase": {}, "delta": {},
	"histogram_quantile": {}, "label_replace": {}, "label_join": {},
	"abs": {}, "absent": {}, "ceil": {}, "floor": {}, "round": {},
	"clamp": {}, "clamp_max": {}, "clamp_min": {},
	"day_of_month": {}, "day_of_week": {}, "days_in_month": {},
	"hour": {}, "minute": {}, "month": {}, "year": {},
	"deriv": {}, "predict_linear": {}, "holt_winters": {},
	"exp": {}, "ln": {}, "log2": {}, "log10": {}, "sqrt": {},
	"sort": {}, "sort_desc": {}, "time": {}, "timestamp": {},
	"vector": {}, "scalar": {}, "sgn": {}, "deg": {}, "rad": {},
	"acos": {}, "acosh": {}, "asin": {}, "asinh": {}, "atan": {}, "atanh": {},
	"cos": {}, "cosh": {}, "sin": {}, "sinh": {}, "tan": {}, "tanh": {},
	"topk": {}, "bottomk": {}, "quantile": {},
	"stddev": {}, "stdvar": {},
	"count_values": {}, "group": {},
	"changes": {}, "resets": {},
	"avg_over_time": {}, "min_over_time": {}, "max_over_time": {},
	"sum_over_time": {}, "count_over_time": {}, "quantile_over_time": {},
	"stddev_over_time": {}, "stdvar_over_time": {},
	"last_over_time": {}, "present_over_time": {},
	"by": {}, "without": {}, "on": {}, "ignoring": {}, "group_left": {}, "group_right": {},
	"bool": {}, "offset": {},
}

func extractMetricNames(expr string) []string {
	expr = removePromQLStrings(expr)

	matches := metricNameRegex.FindAllStringSubmatch(expr, -1)

	seen := make(map[string]struct{})
	var result []string

	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		name := match[1]

		if _, isFunc := promqlFuncs[strings.ToLower(name)]; isFunc {
			continue
		}

		if strings.HasPrefix(name, "$") {
			continue
		}

		if _, ok := seen[name]; !ok {
			seen[name] = struct{}{}
			result = append(result, name)
		}
	}

	return result
}

func removePromQLStrings(expr string) string {
	var result strings.Builder
	inString := false
	stringChar := byte(0)

	for i := 0; i < len(expr); i++ {
		c := expr[i]
		if !inString && (c == '"' || c == '\'' || c == '`') {
			inString = true
			stringChar = c
		} else if inString && c == stringChar {
			inString = false
		} else if !inString {
			result.WriteByte(c)
		}
	}

	return result.String()
}
