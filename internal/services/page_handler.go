package services

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"

	"servicemanager/internal/config"
	"servicemanager/internal/middleware"
	"servicemanager/internal/models"
	"servicemanager/internal/queue"
	"servicemanager/internal/users"
	"servicemanager/internal/utils"
	viewsServices "servicemanager/internal/views/services"

	"github.com/hibiken/asynq"
)

type ServicesPageHandler struct {
	service     *ServiceService
	userService *users.UserService
	env         *config.Env
	asynqClient *asynq.Client
}

func NewServicesPageHandler(service *ServiceService, userService *users.UserService, env *config.Env, asynqClient *asynq.Client) *ServicesPageHandler {
	return &ServicesPageHandler{
		service:     service,
		userService: userService,
		env:         env,
		asynqClient: asynqClient,
	}
}

func (h *ServicesPageHandler) PostAddService(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok || userCtx.Role != models.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	installed, _ := h.userService.IsGithubInstalled(ctx, userCtx.UserID)
	if !installed {
		http.Redirect(w, r, "/?service_error=GitHub connection required to add services#services", http.StatusSeeOther)
		return
	}

	_ = r.ParseForm()
	name := r.FormValue("name")
	description := r.FormValue("description")
	repoName := r.FormValue("github_repo_name")

	_, err := h.service.CreateService(ctx, name, description, repoName)
	if err != nil {
		http.Redirect(w, r, "/?service_error="+err.Error()+"#services", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/#services", http.StatusSeeOther)
}

func (h *ServicesPageHandler) GetDeploy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok || userCtx.Role != models.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	service, err := h.service.GetServiceByID(ctx, id)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	dirs, err := h.service.GetRepositoryDirectories(ctx, userCtx.UserID, service.GithubRepoName, h.env.GITHUB_APP_ID, h.env.GITHUB_APP_PRIVATE_KEY)
	if err != nil || len(dirs) == 0 {
		dirs = []string{"."}
	}

	viewsServices.Deploy(*service, dirs, "").Render(ctx, w)
}

func (h *ServicesPageHandler) PostDeploy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok || userCtx.Role != models.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	service, err := h.service.GetServiceByID(ctx, id)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	dirs, err := h.service.GetRepositoryDirectories(ctx, userCtx.UserID, service.GithubRepoName, h.env.GITHUB_APP_ID, h.env.GITHUB_APP_PRIVATE_KEY)
	if err != nil || len(dirs) == 0 {
		dirs = []string{"."}
	}

	_ = r.ParseForm()
	buildCmd := r.FormValue("build_command")
	runCmd := r.FormValue("run_command")
	portStr := r.FormValue("port")
	rootDirectory := r.FormValue("root_directory")
	if rootDirectory == "" {
		rootDirectory = "."
	}

	port, err := strconv.Atoi(portStr)
	if err != nil {
		viewsServices.Deploy(*service, dirs, "Invalid port number").Render(ctx, w)
		return
	}

	allServices, err := h.service.GetAllServices(ctx)
	if err == nil {
		for _, s := range allServices {
			if s.ID != id && s.Port == port && s.Port != 0 {
				viewsServices.Deploy(*service, dirs, fmt.Sprintf("Port %d is already in use by service '%s'", port, s.Name)).Render(ctx, w)
				return
			}
		}
	}

	envKeys := r.Form["env_keys[]"]
	envVals := r.Form["env_vals[]"]
	envVars := make(map[string]string)
	for i := 0; i < len(envKeys); i++ {
		if envKeys[i] != "" {
			envVars[envKeys[i]] = envVals[i]
		}
	}

	// Default Infisical values
	workspaceID := fmt.Sprintf("proj-%d", id)
	infEnv := "dev"

	// Sync to Infisical if enabled
	if h.env.INFISICAL_URL != "" && h.env.INFISICAL_CLIENT_ID != "" && h.env.INFISICAL_CLIENT_SECRET != "" {
		infisicalClient := utils.NewInfisicalClient(h.env.INFISICAL_URL, h.env.INFISICAL_CLIENT_ID, h.env.INFISICAL_CLIENT_SECRET)

		projectSlug := fmt.Sprintf("proj-%d", id)
		realProjectID, syncErr := infisicalClient.UpsertProject(ctx, service.Name, projectSlug)
		if syncErr != nil {
			slog.Warn("Failed to upsert Infisical project", slog.String("service", service.Name), slog.Any("error", syncErr))
		} else {
			workspaceID = realProjectID
			syncErr = infisicalClient.SyncSecrets(ctx, workspaceID, infEnv, envVars)
			if syncErr != nil {
				slog.Warn("Failed to sync secrets to Infisical", slog.String("service", service.Name), slog.Any("error", syncErr))
			}
		}
	}

	if runCmd == "" {
		viewsServices.Deploy(*service, dirs, "Run Command is required").Render(ctx, w)
		return
	}

	err = h.service.UpdateDeployConfig(ctx, id, "deploying", buildCmd, runCmd, port, envVars, workspaceID, infEnv, rootDirectory)
	if err != nil {
		viewsServices.Deploy(*service, dirs, err.Error()).Render(ctx, w)
		return
	}

	// Create deployment record and enqueue via Asynq
	deployment, err := h.service.CreateDeployment(ctx, id, "manual", "")
	if err != nil {
		slog.Error("Failed to create deployment record", slog.Any("error", err))
		http.Redirect(w, r, "/?service_error=Failed to create deployment#services", http.StatusSeeOther)
		return
	}

	task, err := queue.NewDeployTask(id, deployment.ID, userCtx.UserID)
	if err == nil {
		_, err = h.asynqClient.Enqueue(task)
		if err != nil {
			slog.Error("Failed to enqueue deployment task", slog.Any("error", err))
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/services/%d#logs", id), http.StatusSeeOther)
}

func (h *ServicesPageHandler) GetDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	service, err := h.service.GetServiceByID(ctx, id)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	// Fetch active deployment for display
	activeDeployment, _ := h.service.GetActiveDeployment(ctx, id)

	currentUser := models.User{
		ID:       userCtx.UserID,
		Email:    userCtx.Email,
		UserRole: userCtx.Role,
	}

	viewsServices.Details(*service, currentUser, activeDeployment).Render(ctx, w)
}

func (h *ServicesPageHandler) PostUpdateEnvs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok || userCtx.Role != models.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	service, err := h.service.GetServiceByID(ctx, id)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	_ = r.ParseForm()
	envKeys := r.Form["env_keys[]"]
	envVals := r.Form["env_vals[]"]
	envVars := make(map[string]string)
	for i := 0; i < len(envKeys); i++ {
		if envKeys[i] != "" {
			envVars[envKeys[i]] = envVals[i]
		}
	}

	workspaceID := service.InfisicalWorkspaceID
	if workspaceID == "" {
		workspaceID = fmt.Sprintf("proj-%d", id)
	}
	infEnv := service.InfisicalEnv
	if infEnv == "" {
		infEnv = "dev"
	}

	// Sync to Infisical if configured
	if h.env.INFISICAL_URL != "" && h.env.INFISICAL_CLIENT_ID != "" && h.env.INFISICAL_CLIENT_SECRET != "" {
		infisicalClient := utils.NewInfisicalClient(h.env.INFISICAL_URL, h.env.INFISICAL_CLIENT_ID, h.env.INFISICAL_CLIENT_SECRET)

		projectSlug := fmt.Sprintf("proj-%d", id)
		realProjectID, syncErr := infisicalClient.UpsertProject(ctx, service.Name, projectSlug)
		if syncErr == nil {
			workspaceID = realProjectID
		}
		_ = infisicalClient.SyncSecrets(ctx, workspaceID, infEnv, envVars)
	}

	// Update config and trigger redeployment
	err = h.service.UpdateDeployConfig(ctx, id, "deploying", service.BuildCommand, service.RunCommand, service.Port, envVars, workspaceID, infEnv, service.RootDirectory)
	if err != nil {
		http.Error(w, "Failed to update configuration: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Create deployment record and enqueue via Asynq
	deployment, err := h.service.CreateDeployment(ctx, id, "manual", "")
	if err == nil {
		task, err := queue.NewDeployTask(id, deployment.ID, userCtx.UserID)
		if err == nil {
			_, _ = h.asynqClient.Enqueue(task)
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/services/%d#envs", id), http.StatusSeeOther)
}

func (h *ServicesPageHandler) PostRedeploy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok || userCtx.Role != models.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	_, err = h.service.GetServiceByID(ctx, id)
	if err != nil {
		http.Error(w, "Service not found", http.StatusNotFound)
		return
	}

	err = h.service.UpdateStatus(ctx, id, "deploying")
	if err != nil {
		http.Error(w, "Failed to update service status: "+err.Error(), http.StatusInternalServerError)
		return
	}

	deployment, err := h.service.CreateDeployment(ctx, id, "manual", "")
	if err != nil {
		slog.Error("Failed to create deployment record for redeploy", slog.Any("error", err))
		http.Redirect(w, r, fmt.Sprintf("/services/%d?error=Failed+to+redeploy", id), http.StatusSeeOther)
		return
	}

	task, err := queue.NewDeployTask(id, deployment.ID, userCtx.UserID)
	if err == nil {
		_, err = h.asynqClient.Enqueue(task)
		if err != nil {
			slog.Error("Failed to enqueue deployment task for redeploy", slog.Any("error", err))
		}
	}

	http.Redirect(w, r, fmt.Sprintf("/services/%d#logs", id), http.StatusSeeOther)
}
