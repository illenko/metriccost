package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/illenko/whodidthis/analyzer"
	"github.com/illenko/whodidthis/models"
)

type AnalysisHandler struct {
	analyzer *analyzer.Analyzer
}

func NewAnalysisHandler(analyzer *analyzer.Analyzer) *AnalysisHandler {
	return &AnalysisHandler{
		analyzer: analyzer,
	}
}

func (a *AnalysisHandler) Start(w http.ResponseWriter, r *http.Request) {
	if a.analyzer == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis not configured (missing Gemini API key)")
		return
	}

	var req struct {
		CurrentSnapshotID  int64 `json:"current_snapshot_id"`
		PreviousSnapshotID int64 `json:"previous_snapshot_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.CurrentSnapshotID == 0 || req.PreviousSnapshotID == 0 {
		writeError(w, http.StatusBadRequest, "current_snapshot_id and previous_snapshot_id are required")
		return
	}

	analysis, err := a.analyzer.StartAnalysis(r.Context(), req.CurrentSnapshotID, req.PreviousSnapshotID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusAccepted, analysis)
}

func (a *AnalysisHandler) Get(w http.ResponseWriter, r *http.Request) {
	if a.analyzer == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis not configured (missing Gemini API key)")
		return
	}

	currentID, err := strconv.ParseInt(r.URL.Query().Get("current"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid current parameter")
		return
	}
	previousID, err := strconv.ParseInt(r.URL.Query().Get("previous"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid previous parameter")
		return
	}

	analysis, err := a.analyzer.GetAnalysis(r.Context(), currentID, previousID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if analysis == nil {
		writeError(w, http.StatusNotFound, "analysis not found")
		return
	}

	writeJSON(w, http.StatusOK, analysis)
}

func (a *AnalysisHandler) ListBySnapshot(w http.ResponseWriter, r *http.Request) {
	if a.analyzer == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis not configured (missing Gemini API key)")
		return
	}

	snapshotID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	analyses, err := a.analyzer.ListAnalyses(r.Context(), snapshotID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if analyses == nil {
		analyses = []models.SnapshotAnalysis{}
	}

	writeJSON(w, http.StatusOK, analyses)
}

func (a *AnalysisHandler) Delete(w http.ResponseWriter, r *http.Request) {
	if a.analyzer == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis not configured (missing Gemini API key)")
		return
	}

	currentID, err := strconv.ParseInt(r.URL.Query().Get("current"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid current parameter")
		return
	}
	previousID, err := strconv.ParseInt(r.URL.Query().Get("previous"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid previous parameter")
		return
	}

	if err := a.analyzer.DeleteAnalysis(r.Context(), currentID, previousID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (a *AnalysisHandler) GetStatus(w http.ResponseWriter, _ *http.Request) {
	if a.analyzer == nil {
		writeError(w, http.StatusServiceUnavailable, "analysis not configured (missing Gemini API key)")
		return
	}

	status := a.analyzer.GetGlobalStatus()
	writeJSON(w, http.StatusOK, status)
}
