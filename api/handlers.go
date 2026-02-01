package api

import (
	"net/http"
	"strconv"

	"github.com/illenko/metriccost/models"
	"github.com/illenko/metriccost/scheduler"
	"github.com/illenko/metriccost/storage"
)

type Handlers struct {
	snapshots *storage.SnapshotsRepository
	services  *storage.ServicesRepository
	metrics   *storage.MetricsRepository
	labels    *storage.LabelsRepository
	scheduler *scheduler.Scheduler
	db        *storage.DB
}

type HandlersConfig struct {
	Snapshots *storage.SnapshotsRepository
	Services  *storage.ServicesRepository
	Metrics   *storage.MetricsRepository
	Labels    *storage.LabelsRepository
	Scheduler *scheduler.Scheduler
	DB        *storage.DB
}

func NewHandlers(cfg HandlersConfig) *Handlers {
	return &Handlers{
		snapshots: cfg.Snapshots,
		services:  cfg.Services,
		metrics:   cfg.Metrics,
		labels:    cfg.Labels,
		scheduler: cfg.Scheduler,
		db:        cfg.DB,
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

	latest, _ := h.snapshots.GetLatest(ctx)
	if latest != nil {
		status.LastScan = latest.CollectedAt
	}

	writeJSON(w, http.StatusOK, status)
}

func (h *Handlers) ListScans(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit := parseIntParam(r, "limit", 100)

	scans, err := h.snapshots.List(ctx, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if scans == nil {
		scans = []models.Snapshot{}
	}

	writeJSON(w, http.StatusOK, scans)
}

func (h *Handlers) GetLatestScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scan, err := h.snapshots.GetLatest(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if scan == nil {
		writeError(w, http.StatusNotFound, "no scans found")
		return
	}

	writeJSON(w, http.StatusOK, scan)
}

func (h *Handlers) GetScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	scan, err := h.snapshots.GetByID(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if scan == nil {
		writeError(w, http.StatusNotFound, "scan not found")
		return
	}

	writeJSON(w, http.StatusOK, scan)
}

func (h *Handlers) ListServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	opts := storage.ServiceListOptions{
		Sort:   r.URL.Query().Get("sort"),
		Order:  r.URL.Query().Get("order"),
		Search: r.URL.Query().Get("search"),
	}

	services, err := h.services.List(ctx, scanID, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if services == nil {
		services = []models.ServiceSnapshot{}
	}

	writeJSON(w, http.StatusOK, services)
}

func (h *Handlers) GetService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	serviceName := r.PathValue("service")

	service, err := h.services.GetByName(ctx, scanID, serviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if service == nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	writeJSON(w, http.StatusOK, service)
}

func (h *Handlers) ListMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	serviceName := r.PathValue("service")

	service, err := h.services.GetByName(ctx, scanID, serviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if service == nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	opts := storage.MetricListOptions{
		Sort:  r.URL.Query().Get("sort"),
		Order: r.URL.Query().Get("order"),
	}

	metrics, err := h.metrics.List(ctx, service.ID, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if metrics == nil {
		metrics = []models.MetricSnapshot{}
	}

	writeJSON(w, http.StatusOK, metrics)
}

func (h *Handlers) GetMetric(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	serviceName := r.PathValue("service")
	metricName := r.PathValue("metric")

	service, err := h.services.GetByName(ctx, scanID, serviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if service == nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	metric, err := h.metrics.GetByName(ctx, service.ID, metricName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if metric == nil {
		writeError(w, http.StatusNotFound, "metric not found")
		return
	}

	writeJSON(w, http.StatusOK, metric)
}

func (h *Handlers) ListLabels(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	serviceName := r.PathValue("service")
	metricName := r.PathValue("metric")

	service, err := h.services.GetByName(ctx, scanID, serviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if service == nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	metric, err := h.metrics.GetByName(ctx, service.ID, metricName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if metric == nil {
		writeError(w, http.StatusNotFound, "metric not found")
		return
	}

	labels, err := h.labels.List(ctx, metric.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if labels == nil {
		labels = []models.LabelSnapshot{}
	}

	writeJSON(w, http.StatusOK, labels)
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
