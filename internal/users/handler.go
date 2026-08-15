package users

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"servicemanager/internal/middleware"
	"servicemanager/internal/models"
	"servicemanager/internal/utils"
)

type UserHandler struct {
	service *UserService
}

func NewUserHandler(userService *UserService) *UserHandler {
	return &UserHandler{
		service: userService,
	}
}

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}

func (h *UserHandler) GetAllUsers(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	users, err := h.service.GetAllUsers(ctx)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	response := make([]UserResponsePayload, 0, len(users))
	for _, u := range users {
		response = append(response, UserResponsePayload{
			ID:        u.ID,
			Email:     u.Email,
			Role:      string(u.UserRole),
			CreatedAt: u.CreatedAt,
		})
	}

	utils.JsonResponse(w, http.StatusOK, response)
}

func (h *UserHandler) RemoveUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid user id"))
		return
	}

	err = h.service.RemoveUser(ctx, id)
	if err != nil {
		if err == ErrUserNotFound {
			utils.ErrorResponse(w, http.StatusNotFound, err)
			return
		}
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "user removed successfully"})
}

func (h *UserHandler) ChangeUserRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid user id"))
		return
	}

	var role string
	if r.Header.Get("Content-Type") == "application/json" {
		var payload ChangeRoleRequestPayload
		if err := decodeJSON(r, &payload); err != nil {
			utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
			return
		}
		role = payload.Role
	} else {
		_ = r.ParseForm()
		role = r.FormValue("role")
	}

	err = h.service.ChangeUserRole(ctx, id, models.UserRole(role))
	if err != nil {
		if err == ErrUserNotFound {
			utils.ErrorResponse(w, http.StatusNotFound, err)
			return
		}
		utils.ErrorResponse(w, http.StatusBadRequest, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "user role updated successfully"})
}

func (h *UserHandler) Login(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var email, password string
	if r.Header.Get("Content-Type") == "application/json" {
		var payload LoginRequestPayload
		if err := decodeJSON(r, &payload); err != nil {
			utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
			return
		}
		email = payload.Email
		password = payload.Password
	} else {
		_ = r.ParseForm()
		email = r.FormValue("email")
		password = r.FormValue("password")
	}

	res, err := h.service.Login(ctx, email, password)
	if err != nil {
		if err == ErrInvalidCredentials {
			utils.ErrorResponse(w, http.StatusUnauthorized, err)
			return
		}
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	// Set HttpOnly cookie for the token
	http.SetCookie(w, &http.Cookie{
		Name:  "token",
		Value: res.Token,
		Path:  "/",
		MaxAge:   365 * 24 * 3600, // 1 year expiry
		HttpOnly: true,
		Secure:   false, // Set to true if running over HTTPS
		SameSite: http.SameSiteLaxMode,
	})

	// Clear Token field from JSON response body
	res.Token = ""

	utils.JsonResponse(w, http.StatusOK, res)
}

func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Revoke the new global cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
	// Revoke the old incorrectly scoped cookie to unstuck existing users
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/api/v1/users",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
	utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "logged out successfully"})
}

func (h *UserHandler) CreateNewInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var email string
	if r.Header.Get("Content-Type") == "application/json" {
		var payload InviteRequestPayload
		if err := decodeJSON(r, &payload); err != nil {
			utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
			return
		}
		email = payload.Email
	} else {
		_ = r.ParseForm()
		email = r.FormValue("email")
	}

	invite, err := h.service.CreateInvite(ctx, email)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	res := InviteResponsePayload{
		ID:        invite.ID,
		Email:     invite.Email,
		CreatedAt: invite.CreatedAt,
	}

	utils.JsonResponse(w, http.StatusCreated, res)
}

func (h *UserHandler) GetAllInvites(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	invites, err := h.service.GetAllInvites(ctx)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	res := make([]InviteResponsePayload, 0, len(invites))
	for _, inv := range invites {
		res = append(res, InviteResponsePayload{
			ID:        inv.ID,
			Email:     inv.Email,
			CreatedAt: inv.CreatedAt,
		})
	}

	utils.JsonResponse(w, http.StatusOK, res)
}

func (h *UserHandler) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	email := r.PathValue("email")
	if email == "" {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("email is required"))
		return
	}

	err := h.service.DeleteInviteByEmail(ctx, email)
	if err != nil {
		if err == ErrInviteNotFound {
			utils.ErrorResponse(w, http.StatusNotFound, err)
			return
		}
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "invite revoked successfully"})
}

func (h *UserHandler) AcceptInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var token, email, password string
	if r.Header.Get("Content-Type") == "application/json" {
		var payload AcceptInviteRequestPayload
		if err := decodeJSON(r, &payload); err != nil {
			utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
			return
		}
		token = payload.Token
		email = payload.Email
		password = payload.Password
	} else {
		_ = r.ParseForm()
		token = r.FormValue("token")
		email = r.FormValue("email")
		password = r.FormValue("password")
	}

	user, err := h.service.AcceptInvite(ctx, token, email, password)
	if err != nil {
		if err == ErrInviteNotFound || err == ErrEmailMismatch {
			utils.ErrorResponse(w, http.StatusBadRequest, err)
			return
		}
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	res := UserResponsePayload{
		ID:        user.ID,
		Email:     user.Email,
		Role:      string(user.UserRole),
		CreatedAt: user.CreatedAt,
	}

	utils.JsonResponse(w, http.StatusCreated, res)
}

func (h *UserHandler) ForgotPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var payload ForgotPasswordRequestPayload
	if err := decodeJSON(r, &payload); err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}

	token, err := h.service.ForgotPassword(ctx, payload.Email)
	if err != nil {
		if err == ErrUserNotFound {
			// For security, can return StatusOK, but we'll return StatusOK with token here for testing
			utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "password reset email sent"})
			return
		}
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, map[string]string{
		"message": "password reset email sent",
		"token":   token, // Return for testing & verification as we do not run an email SMTP server
	})
}

func (h *UserHandler) ResetPassword(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var payload ResetPasswordRequestPayload
	if err := decodeJSON(r, &payload); err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}

	err := h.service.ResetPassword(ctx, payload.Token, payload.Password)
	if err != nil {
		if err == ErrUserNotFound || err == ErrTokenExpired {
			utils.ErrorResponse(w, http.StatusBadRequest, err)
			return
		}
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "password reset successfully"})
}

func (h *UserHandler) GetMe(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	installed, err := h.service.IsGithubInstalled(ctx, userCtx.UserID)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, UserResponsePayload{
		ID:              userCtx.UserID,
		Email:           userCtx.Email,
		Role:            string(userCtx.Role),
		GithubInstalled: installed,
	})
}

func (h *UserHandler) InstallGithub(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	var payload GithubInstallRequestPayload
	if err := decodeJSON(r, &payload); err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}

	err := h.service.InstallGithub(ctx, userCtx.UserID, payload.InstallationID)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "github app installed successfully"})
}

func (h *UserHandler) GetGithubRepositories(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	repos, err := h.service.GetGithubRepositories(ctx, userCtx.UserID)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, repos)
}

func (h *UserHandler) GetGithubURL(w http.ResponseWriter, r *http.Request) {
	url, err := h.service.GetGithubInstallURL()
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, map[string]string{"url": url})
}

func (h *UserHandler) DisconnectGithub(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	err := h.service.DisconnectGithub(ctx, userCtx.UserID)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "github disconnected successfully"})
}

func (h *UserHandler) GetGithubCallback(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		http.Redirect(w, r, "http://localhost:3001/login", http.StatusTemporaryRedirect)
		return
	}

	installationIDStr := r.URL.Query().Get("installation_id")
	if installationIDStr != "" {
		installationID, err := strconv.ParseInt(installationIDStr, 10, 64)
		if err == nil && installationID != 0 {
			_ = h.service.InstallGithub(ctx, userCtx.UserID, installationID)
		}
	}

	http.Redirect(w, r, "http://localhost:3001/dashboard", http.StatusTemporaryRedirect)
}
