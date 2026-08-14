package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"servicemanager/internal/utils"
)

type ServiceHandler struct {
	service *ServiceService
}

func NewServiceHandler(service *ServiceService) *ServiceHandler {
	return &ServiceHandler{
		service: service,
	}
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func (h *ServiceHandler) GetAllServices(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	services, err := h.service.GetAllServices(ctx)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}
	utils.JsonResponse(w, http.StatusOK, services)
}

func (h *ServiceHandler) CreateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var name, description, repoName string
	if r.Header.Get("Content-Type") == "application/json" {
		var payload CreateServiceRequestPayload
		if err := decodeJSON(r, &payload); err != nil {
			utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
			return
		}
		name = payload.Name
		description = payload.Description
		repoName = payload.GithubRepoName
	} else {
		_ = r.ParseForm()
		name = r.FormValue("name")
		description = r.FormValue("description")
		repoName = r.FormValue("github_repo_name")
	}

	s, err := h.service.CreateService(ctx, name, description, repoName)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}
	utils.JsonResponse(w, http.StatusCreated, s)
}

func (h *ServiceHandler) DeleteService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid service id"))
		return
	}

	err = h.service.DeleteService(ctx, id)
	if err != nil {
		if err == ErrServiceNotFound {
			utils.ErrorResponse(w, http.StatusNotFound, err)
			return
		}
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "service deleted successfully"})
}
