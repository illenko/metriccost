package models

import "time"

type MetricSnapshot struct {
	ID                 int64          `json:"id"`
	CollectedAt        time.Time      `json:"collected_at"`
	MetricName         string         `json:"metric_name"`
	Cardinality        int            `json:"cardinality"`
	EstimatedSizeBytes int64          `json:"estimated_size_bytes"`
	SampleCount        int            `json:"sample_count,omitempty"`
	Team               string         `json:"team,omitempty"`
	Labels             map[string]int `json:"labels,omitempty"` // label name -> unique values count
}

type Recommendation struct {
	ID                      int64     `json:"id"`
	CreatedAt               time.Time `json:"created_at"`
	MetricName              string    `json:"metric_name"`
	Type                    string    `json:"type"`
	Priority                string    `json:"priority"`
	CurrentCardinality      int       `json:"current_cardinality,omitempty"`
	CurrentSizeBytes        int64     `json:"current_size_bytes,omitempty"`
	PotentialReductionBytes int64     `json:"potential_reduction_bytes,omitempty"`
	Description             string    `json:"description"`
	SuggestedAction         string    `json:"suggested_action"`
}

const (
	RecommendationHighCardinality = "high_cardinality"
	RecommendationUnused          = "unused"
	RecommendationRedundantLabels = "redundant_labels"
)

const (
	PriorityHigh   = "high"
	PriorityMedium = "medium"
	PriorityLow    = "low"
)

type DashboardStats struct {
	ID            int64     `json:"id"`
	CollectedAt   time.Time `json:"collected_at"`
	DashboardUID  string    `json:"dashboard_uid"`
	DashboardName string    `json:"dashboard_name"`
	FolderName    string    `json:"folder_name,omitempty"`
	LastViewedAt  time.Time `json:"last_viewed_at,omitempty"`
	QueryCount    int       `json:"query_count"`
	MetricsUsed   []string  `json:"metrics_used,omitempty"`
}

type Snapshot struct {
	ID               int64                  `json:"id"`
	CollectedAt      time.Time              `json:"collected_at"`
	TotalMetrics     int                    `json:"total_metrics"`
	TotalCardinality int64                  `json:"total_cardinality"`
	TotalSizeBytes   int64                  `json:"total_size_bytes"`
	TeamBreakdown    map[string]TeamMetrics `json:"team_breakdown,omitempty"`
}

type TeamMetrics struct {
	Cardinality int64 `json:"cardinality"`
	SizeBytes   int64 `json:"size_bytes"`
	MetricCount int   `json:"metric_count"`
}

type Overview struct {
	TotalMetrics     int                    `json:"total_metrics"`
	TotalCardinality int64                  `json:"total_cardinality"`
	TotalSizeBytes   int64                  `json:"total_size_bytes"`
	TrendPercentage  float64                `json:"trend_percentage"`
	LastScan         time.Time              `json:"last_scan"`
	TeamBreakdown    map[string]TeamMetrics `json:"team_breakdown"`
}

type TrendDataPoint struct {
	Date         time.Time `json:"date"`
	TotalMetrics int       `json:"total_metrics"`
	Cardinality  int64     `json:"cardinality"`
	SizeBytes    int64     `json:"size_bytes"`
}

type MetricListItem struct {
	Name               string  `json:"name"`
	Cardinality        int     `json:"cardinality"`
	EstimatedSizeBytes int64   `json:"estimated_size_bytes"`
	Team               string  `json:"team"`
	TrendPercentage    float64 `json:"trend_percentage"`
}

type UnusedDashboard struct {
	UID           string    `json:"uid"`
	Name          string    `json:"name"`
	FolderName    string    `json:"folder_name,omitempty"`
	LastViewed    time.Time `json:"last_viewed"`
	DaysSinceView int       `json:"days_since_view"`
	MetricsCount  int       `json:"metrics_count"`
	URL           string    `json:"url,omitempty"`
}

type HealthStatus struct {
	Status              string    `json:"status"`
	PrometheusConnected bool      `json:"prometheus_connected"`
	GrafanaConnected    bool      `json:"grafana_connected"`
	DatabaseOK          bool      `json:"database_ok"`
	LastScan            time.Time `json:"last_scan,omitempty"`
}
