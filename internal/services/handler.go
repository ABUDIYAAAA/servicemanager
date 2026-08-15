package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"servicemanager/internal/middleware"
	"servicemanager/internal/queue"
	"servicemanager/internal/utils"

	"github.com/hibiken/asynq"
)

type ServiceHandler struct {
	service          *ServiceService
	githubAppID      string
	githubPrivateKey string
	asynqClient      *asynq.Client
}

func NewServiceHandler(service *ServiceService, githubAppID, githubPrivateKey string, asynqClient *asynq.Client) *ServiceHandler {
	return &ServiceHandler{
		service:          service,
		githubAppID:      githubAppID,
		githubPrivateKey: githubPrivateKey,
		asynqClient:      asynqClient,
	}
}

func (h *ServiceHandler) GetRepositoryDirectories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	repoName := r.URL.Query().Get("repo")
	if repoName == "" {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("repo query parameter is required"))
		return
	}

	dirs, err := h.service.GetRepositoryDirectories(ctx, userCtx.UserID, repoName, h.githubAppID, h.githubPrivateKey)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, dirs)
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

func (h *ServiceHandler) GetServiceByID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid service id"))
		return
	}

	service, err := h.service.GetServiceByID(ctx, id)
	if err != nil {
		utils.ErrorResponse(w, http.StatusNotFound, fmt.Errorf("service not found"))
		return
	}
	utils.JsonResponse(w, http.StatusOK, service)
}

func (h *ServiceHandler) UpdateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid service id"))
		return
	}

	var payload UpdateServiceRequestPayload
	if err := decodeJSON(r, &payload); err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}

	svc, err := h.service.UpdateService(ctx, id, payload)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, svc)
}

func (h *ServiceHandler) CreateService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	var name, description, repoName, rootDirectory, buildCmd, runCmd, framework, domain string
	var port int
	var envVars map[string]string
	if r.Header.Get("Content-Type") == "application/json" {
		var payload CreateServiceRequestPayload
		if err := decodeJSON(r, &payload); err != nil {
			utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
			return
		}
		name = payload.Name
		description = payload.Description
		repoName = payload.GithubRepoName
		rootDirectory = payload.RootDirectory
		buildCmd = payload.BuildCommand
		runCmd = payload.RunCommand
		port = payload.Port
		framework = payload.Framework
		envVars = payload.EnvVars
		domain = payload.Domain
	} else {
		_ = r.ParseForm()
		name = r.FormValue("name")
		description = r.FormValue("description")
		repoName = r.FormValue("github_repo_name")
		rootDirectory = r.FormValue("root_directory")
		framework = r.FormValue("framework")
		domain = r.FormValue("domain")
	}

	if rootDirectory == "" {
		rootDirectory = "."
	}
	if port == 0 {
		port = 3000
	}
	if framework == "" {
		framework = "Other"
	}

	s, err := h.service.CreateService(ctx, name, description, repoName, rootDirectory, buildCmd, runCmd, framework, port, envVars, domain)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	// Auto-trigger first deployment
	if repoName != "" {
		deployment, depErr := h.service.CreateDeployment(ctx, s.ID, "manual", "")
		if depErr == nil {
			task, taskErr := queue.NewDeploymentCreateTask(s.ID, deployment.ID, userCtx.UserID)
			if taskErr == nil {
				_, _ = h.asynqClient.Enqueue(task)
			}
		}
	}

	utils.JsonResponse(w, http.StatusCreated, s)
}

// TriggerDeploy manually triggers a new deployment for a service.
func (h *ServiceHandler) TriggerDeploy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid service id"))
		return
	}

	svc, err := h.service.GetServiceByID(ctx, id)
	if err != nil || svc == nil {
		utils.ErrorResponse(w, http.StatusNotFound, fmt.Errorf("service not found"))
		return
	}

	deployment, err := h.service.CreateDeployment(ctx, id, "manual", "")
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, fmt.Errorf("failed to create deployment: %w", err))
		return
	}

	task, err := queue.NewDeploymentCreateTask(id, deployment.ID, userCtx.UserID)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, fmt.Errorf("failed to build task: %w", err))
		return
	}

	if _, err := h.asynqClient.Enqueue(task); err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, fmt.Errorf("failed to enqueue deployment: %w", err))
		return
	}

	utils.JsonResponse(w, http.StatusAccepted, deployment)
}

// GetLatestDeployment returns the most recent deployment for a service.
func (h *ServiceHandler) GetLatestDeployment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid service id"))
		return
	}

	dep, err := h.service.GetLatestDeployment(ctx, id)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}
	utils.JsonResponse(w, http.StatusOK, dep)
}

// GetDeployments returns all deployments for a service.
func (h *ServiceHandler) GetDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid service id"))
		return
	}

	deps, err := h.service.GetDeploymentsByServiceID(ctx, id)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}
	utils.JsonResponse(w, http.StatusOK, deps)
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
