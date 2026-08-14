package dashboard

import (
	"fmt"
	"net/http"
	"strconv"

	"servicemanager/internal/config"
	"servicemanager/internal/middleware"
	"servicemanager/internal/models"
	"servicemanager/internal/services"
	"servicemanager/internal/users"
	"servicemanager/internal/utils"
	viewDashboard "servicemanager/internal/views/dashboard"
)

type DashboardHandler struct {
	userService    *users.UserService
	serviceService *services.ServiceService
	env            *config.Env
}

func NewDashboardHandler(us *users.UserService, ss *services.ServiceService, env *config.Env) *DashboardHandler {
	return &DashboardHandler{
		userService:    us,
		serviceService: ss,
		env:            env,
	}
}

func (h *DashboardHandler) GetDashboard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	currentUser, err := h.userService.GetUserByID(ctx, userCtx.UserID)
	if err != nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	var allUsers []models.User
	var invites []models.Invite
	var allServices []models.Service
	var githubRepos []models.GithubRepo

	allServices, _ = h.serviceService.GetAllServices(ctx)
	allUsers, _ = h.userService.GetAllUsers(ctx)

	if currentUser.UserRole == models.RoleAdmin {
		invites, _ = h.userService.GetAllInvites(ctx)
	}

	installations, _ := h.userService.GetAllGithubInstallations(ctx, currentUser.ID)
	if len(installations) > 0 {
		githubRepos, _ = h.userService.GetGithubRepositories(ctx, currentUser.ID)
	}

	githubAuthURL, _ := utils.GetAppURL(h.env.GITHUB_APP_ID, h.env.GITHUB_APP_PRIVATE_KEY)
	if githubAuthURL == "" {
		githubAuthURL = fmt.Sprintf("https://github.com/apps/YOUR_APP_NAME/installations/new")
	}

	inviteErr := r.URL.Query().Get("invite_error")
	serviceErr := r.URL.Query().Get("service_error")

	viewDashboard.Dashboard(
		*currentUser,
		allUsers,
		invites,
		allServices,
		githubRepos,
		installations,
		githubAuthURL,
		inviteErr,
		serviceErr,
	).Render(ctx, w)
}

func (h *DashboardHandler) GetGithubCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	setupAction := r.URL.Query().Get("setup_action")
	instIDStr := r.URL.Query().Get("installation_id")

	// Handle cases where GitHub redirects without an installation_id
	// This happens on org installation requests (setup_action=request) or other flows
	if instIDStr == "" {
		if setupAction == "request" {
			// Org admin needs to approve the installation request
			http.Redirect(w, r, "/?service_error=Installation request sent to your organization admin for approval#services", http.StatusSeeOther)
			return
		}
		// No installation_id and no known setup_action — redirect gracefully
		http.Redirect(w, r, "/#services", http.StatusSeeOther)
		return
	}

	instID, err := strconv.ParseInt(instIDStr, 10, 64)
	if err != nil {
		http.Redirect(w, r, "/?service_error=Invalid installation ID in GitHub callback#services", http.StatusSeeOther)
		return
	}

	err = h.userService.InstallGithub(ctx, userCtx.UserID, instID)
	if err != nil {
		http.Redirect(w, r, "/?service_error=Failed to link GitHub installation: "+err.Error()+"#services", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/#services", http.StatusSeeOther)
}
