package handler

import (
	"net/http"
	"strconv"

	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/storage"
)

type LabelsHandler struct {
	servicesRepo storage.ServicesRepo
	metricsRepo  storage.MetricsRepo
	labelsRepo   storage.LabelsRepo
}

func NewLabelsHandler(servicesRepo storage.ServicesRepo, metricsRepo storage.MetricsRepo, labelsRepo storage.LabelsRepo) *LabelsHandler {
	return &LabelsHandler{
		servicesRepo: servicesRepo,
		metricsRepo:  metricsRepo,
		labelsRepo:   labelsRepo,
	}
}

func (h *LabelsHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	serviceName := r.PathValue("service")
	metricName := r.PathValue("metric")

	service, err := h.servicesRepo.GetByName(ctx, scanID, serviceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if service == nil {
		writeError(w, http.StatusNotFound, "service not found")
		return
	}

	metric, err := h.metricsRepo.GetByName(ctx, service.ID, metricName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if metric == nil {
		writeError(w, http.StatusNotFound, "metric not found")
		return
	}

	labels, err := h.labelsRepo.List(ctx, metric.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if labels == nil {
		labels = []models.LabelSnapshot{}
	}

	writeJSON(w, http.StatusOK, labels)
}
