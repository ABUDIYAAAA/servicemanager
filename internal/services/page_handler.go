package services

import (
	"fmt"
	"net/http"
	"strconv"

	"servicemanager/internal/middleware"
	"servicemanager/internal/models"
	"servicemanager/internal/users"
	viewsServices "servicemanager/internal/views/services"
)

type ServicesPageHandler struct {
	service     *ServiceService
	userService *users.UserService
}

func NewServicesPageHandler(service *ServiceService, userService *users.UserService) *ServicesPageHandler {
	return &ServicesPageHandler{
		service:     service,
		userService: userService,
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

	viewsServices.Deploy(*service, "").Render(ctx, w)
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

	_ = r.ParseForm()
	buildCmd := r.FormValue("build_command")
	runCmd := r.FormValue("run_command")
	portStr := r.FormValue("port")

	port, err := strconv.Atoi(portStr)
	if err != nil {
		viewsServices.Deploy(*service, "Invalid port number").Render(ctx, w)
		return
	}

	envKeys := r.Form["env_keys[]"]
	envVals := r.Form["env_vals[]"]
	envVars := make(map[string]string)
	for i := 0; i < len(envKeys); i++ {
		if envKeys[i] != "" {
			envVars[envKeys[i]] = envVals[i]
		}
	}

	// Automatically assign or sync Workspace ID based on service ID
	workspaceID := fmt.Sprintf("proj-%d", id)
	infEnv := "dev"

	if runCmd == "" {
		viewsServices.Deploy(*service, "Run Command is required").Render(ctx, w)
		return
	}

	err = h.service.UpdateDeployConfig(ctx, id, "deploying", buildCmd, runCmd, port, envVars, workspaceID, infEnv)
	if err != nil {
		viewsServices.Deploy(*service, err.Error()).Render(ctx, w)
		return
	}

	http.Redirect(w, r, "/#services", http.StatusSeeOther)
}

func (h *ServicesPageHandler) GetDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
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

	viewsServices.Details(*service).Render(ctx, w)
}
