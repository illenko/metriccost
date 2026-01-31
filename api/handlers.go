package api

import (
	"net/http"
	"strconv"

	"github.com/illenko/metriccost/analyzer"
	"github.com/illenko/metriccost/models"
	"github.com/illenko/metriccost/scheduler"
	"github.com/illenko/metriccost/storage"
)

type Handlers struct {
	metricsRepo    *storage.MetricsRepository
	recsRepo       *storage.RecommendationsRepository
	dashboardsRepo *storage.DashboardsRepository
	snapshotsRepo  *storage.SnapshotsRepository
	trends         *analyzer.TrendsCalculator
	scheduler      *scheduler.Scheduler
	db             *storage.DB
}

type HandlersConfig struct {
	MetricsRepo    *storage.MetricsRepository
	RecsRepo       *storage.RecommendationsRepository
	DashboardsRepo *storage.DashboardsRepository
	SnapshotsRepo  *storage.SnapshotsRepository
	Trends         *analyzer.TrendsCalculator
	Scheduler      *scheduler.Scheduler
	DB             *storage.DB
}

func NewHandlers(cfg HandlersConfig) *Handlers {
	return &Handlers{
		metricsRepo:    cfg.MetricsRepo,
		recsRepo:       cfg.RecsRepo,
		dashboardsRepo: cfg.DashboardsRepo,
		snapshotsRepo:  cfg.SnapshotsRepo,
		trends:         cfg.Trends,
		scheduler:      cfg.Scheduler,
		db:             cfg.DB,
	}
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	status := models.HealthStatus{
		Status:     "healthy",
		DatabaseOK: true,
	}

	if _, err := h.db.Stats(ctx); err != nil {
		status.Status = "unhealthy"
		status.DatabaseOK = false
	}

	lastScan, _ := h.metricsRepo.GetLatestCollectionTime(ctx)
	if !lastScan.IsZero() {
		status.LastScan = lastScan
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *Handlers) GetOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	overview, err := h.trends.GetOverview(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, overview)
}

func (h *Handlers) ListMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	latestTime, err := h.metricsRepo.GetLatestCollectionTime(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if latestTime.IsZero() {
		writeJSON(w, http.StatusOK, []models.MetricListItem{})
		return
	}

	opts := storage.ListOptions{
		Limit:  parseIntParam(r, "limit", 20),
		Offset: parseIntParam(r, "offset", 0),
		SortBy: r.URL.Query().Get("sort"),
		Team:   r.URL.Query().Get("team"),
		Search: r.URL.Query().Get("search"),
	}

	metrics, err := h.metricsRepo.List(ctx, latestTime, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var items []models.MetricListItem
	for _, m := range metrics {
		trend, _ := h.trends.GetMetricTrend(ctx, m.MetricName)
		items = append(items, models.MetricListItem{
			Name:               m.MetricName,
			Cardinality:        m.Cardinality,
			EstimatedSizeBytes: m.EstimatedSizeBytes,
			Team:               m.Team,
			TrendPercentage:    trend,
		})
	}

	writeJSON(w, http.StatusOK, items)
}

func (h *Handlers) GetMetric(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	name := r.PathValue("name")

	metric, err := h.metricsRepo.GetByName(ctx, name)
	if err != nil {
		writeError(w, http.StatusNotFound, "metric not found")
		return
	}

	trend, _ := h.trends.GetMetricTrend(ctx, name)

	response := struct {
		*models.MetricSnapshot
		TrendPercentage float64                  `json:"trend_percentage"`
		Recommendations []*models.Recommendation `json:"recommendations,omitempty"`
	}{
		MetricSnapshot:  metric,
		TrendPercentage: trend,
	}

	recs, _ := h.recsRepo.GetByMetricName(ctx, name)
	if len(recs) > 0 {
		response.Recommendations = recs
	}

	writeJSON(w, http.StatusOK, response)
}

func (h *Handlers) ListRecommendations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	priority := r.URL.Query().Get("priority")

	recs, err := h.recsRepo.List(ctx, priority)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if recs == nil {
		recs = []*models.Recommendation{}
	}

	writeJSON(w, http.StatusOK, recs)
}

func (h *Handlers) GetTrends(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	periodStr := r.URL.Query().Get("period")
	var period analyzer.TrendPeriod
	switch periodStr {
	case "7d":
		period = analyzer.TrendPeriod7Days
	case "90d":
		period = analyzer.TrendPeriod90Days
	default:
		period = analyzer.TrendPeriod30Days
	}

	trends, err := h.trends.GetOverallTrends(ctx, period)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if trends == nil {
		trends = []*models.TrendDataPoint{}
	}

	writeJSON(w, http.StatusOK, trends)
}

func (h *Handlers) GetUnusedDashboards(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	days := parseIntParam(r, "days", 90)

	dashboards, err := h.dashboardsRepo.GetUnusedDashboards(ctx, days)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if dashboards == nil {
		dashboards = []*models.UnusedDashboard{}
	}

	writeJSON(w, http.StatusOK, dashboards)
}

func (h *Handlers) TriggerScan(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	err := h.scheduler.TriggerScan(r.Context())
	if err != nil {
		if err == scheduler.ErrScanAlreadyRunning {
			writeError(w, http.StatusConflict, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scan started"})
}

func (h *Handlers) GetScanStatus(w http.ResponseWriter, r *http.Request) {
	if h.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	status := h.scheduler.GetStatus()
	writeJSON(w, http.StatusOK, status)
}

func parseIntParam(r *http.Request, name string, defaultVal int) int {
	val := r.URL.Query().Get(name)
	if val == "" {
		return defaultVal
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		return defaultVal
	}
	return n
}
