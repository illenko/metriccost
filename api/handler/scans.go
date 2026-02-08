package handler

import (
	"net/http"
	"strconv"

	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/scheduler"
	"github.com/illenko/whodidthis/storage"
)

type ScansHandler struct {
	repo      storage.SnapshotsRepo
	scheduler *scheduler.Scheduler
}

func NewScansHandler(repo storage.SnapshotsRepo, scheduler *scheduler.Scheduler) *ScansHandler {
	return &ScansHandler{
		repo:      repo,
		scheduler: scheduler,
	}
}

func (s *ScansHandler) List(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	limit := parseIntParam(r, "limit", 100)

	scans, err := s.repo.List(ctx, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if scans == nil {
		scans = []models.Snapshot{}
	}

	writeJSON(w, http.StatusOK, scans)
}

func (s *ScansHandler) GetLatest(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scan, err := s.repo.GetLatest(ctx)
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

func (s *ScansHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	scan, err := s.repo.GetByID(ctx, id)
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

func (s *ScansHandler) Trigger(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	err := s.scheduler.TriggerScan(r.Context())
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

func (s *ScansHandler) GetStatus(w http.ResponseWriter, r *http.Request) {
	if s.scheduler == nil {
		writeError(w, http.StatusServiceUnavailable, "scheduler not configured")
		return
	}

	status := s.scheduler.GetStatus()
	writeJSON(w, http.StatusOK, status)
}
