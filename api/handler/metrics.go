package handler

import (
	"net/http"
	"strconv"

	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/storage"
)

type MetricsHandler struct {
	servicesRepo storage.ServicesRepo
	metricsRepo  storage.MetricsRepo
}

func NewMetricsHandler(servicesRepo storage.ServicesRepo, metricsRepo storage.MetricsRepo) *MetricsHandler {
	return &MetricsHandler{
		servicesRepo: servicesRepo,
		metricsRepo:  metricsRepo,
	}
}

func (m *MetricsHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	serviceName := r.PathValue("service")

	service, err := m.servicesRepo.GetByName(ctx, scanID, serviceName)
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

	metrics, err := m.metricsRepo.List(ctx, service.ID, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if metrics == nil {
		metrics = []models.MetricSnapshot{}
	}

	writeJSON(w, http.StatusOK, metrics)
}

func (m *MetricsHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	serviceName := r.PathValue("service")
	metricName := r.PathValue("metric")

	service, err := m.servicesRepo.GetByName(ctx, scanID, serviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if service == nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	metric, err := m.metricsRepo.GetByName(ctx, service.ID, metricName)
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
