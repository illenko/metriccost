package handler

import (
	"net/http"

	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/prometheus"
	"github.com/illenko/whodidthis/storage"
)

type HealthHandler struct {
	snapshots  storage.SnapshotsRepo
	db         *storage.DB
	promClient prometheus.MetricsClient
}

func NewHealthHandler(snapshots storage.SnapshotsRepo,
	db *storage.DB,
	promClient prometheus.MetricsClient) *HealthHandler {
	return &HealthHandler{
		snapshots:  snapshots,
		db:         db,
		promClient: promClient,
	}
}

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	status := models.HealthStatus{
		Status:              "healthy",
		DatabaseOK:          true,
		PrometheusConnected: true,
	}

	if _, err := h.db.Stats(ctx); err != nil {
		status.Status = "unhealthy"
		status.DatabaseOK = false
	}

	if h.promClient != nil {
		if err := h.promClient.HealthCheck(ctx); err != nil {
			status.PrometheusConnected = false
			if status.Status == "healthy" {
				status.Status = "degraded"
			}
		}
	}

	latest, _ := h.snapshots.GetLatest(ctx)
	if latest != nil {
		status.LastScan = latest.CollectedAt
	}

	writeJSON(w, http.StatusOK, status)
}
