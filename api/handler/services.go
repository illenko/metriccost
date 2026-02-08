package handler

import (
	"net/http"
	"strconv"

	"github.com/illenko/whodidthis/models"
	"github.com/illenko/whodidthis/storage"
)

type ServicesHandler struct {
	servicesRepo storage.ServicesRepo
}

func NewServicesHandler(servicesRepo storage.ServicesRepo) *ServicesHandler {
	return &ServicesHandler{

		servicesRepo: servicesRepo,
	}
}

func (s *ServicesHandler) List(w http.ResponseWriter, r *http.Request) {
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

	services, err := s.servicesRepo.List(ctx, scanID, opts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	if services == nil {
		services = []models.ServiceSnapshot{}
	}

	writeJSON(w, http.StatusOK, services)
}

func (s *ServicesHandler) Get(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	scanID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid scan id")
		return
	}

	serviceName := r.PathValue("service")

	service, err := s.servicesRepo.GetByName(ctx, scanID, serviceName)
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
