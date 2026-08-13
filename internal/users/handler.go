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

	var payload ChangeRoleRequestPayload
	if err := decodeJSON(r, &payload); err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}

	err = h.service.ChangeUserRole(ctx, id, models.UserRole(payload.Role))
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
	var payload LoginRequestPayload
	if err := decodeJSON(r, &payload); err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}

	res, err := h.service.Login(ctx, payload.Email, payload.Password)
	if err != nil {
		if err == ErrInvalidCredentials {
			utils.ErrorResponse(w, http.StatusUnauthorized, err)
			return
		}
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	utils.JsonResponse(w, http.StatusOK, res)
}

func (h *UserHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// JWT is stateless; client should drop token. 
	// We return status OK.
	utils.JsonResponse(w, http.StatusOK, map[string]string{"message": "logged out successfully"})
}

func (h *UserHandler) CreateNewInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var payload InviteRequestPayload
	if err := decodeJSON(r, &payload); err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}

	invite, err := h.service.CreateInvite(ctx, payload.Email)
	if err != nil {
		utils.ErrorResponse(w, http.StatusInternalServerError, err)
		return
	}

	res := InviteResponsePayload{
		ID:        invite.ID,
		Email:     invite.Email,
		Token:     invite.Token,
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
			Token:     inv.Token,
			CreatedAt: inv.CreatedAt,
		})
	}

	utils.JsonResponse(w, http.StatusOK, res)
}

func (h *UserHandler) DeleteInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := r.PathValue("token")
	if token == "" {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("token is required"))
		return
	}

	err := h.service.DeleteInvite(ctx, token)
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
	var payload AcceptInviteRequestPayload
	if err := decodeJSON(r, &payload); err != nil {
		utils.ErrorResponse(w, http.StatusBadRequest, fmt.Errorf("invalid request body"))
		return
	}

	user, err := h.service.AcceptInvite(ctx, payload.Token, payload.Email, payload.Password)
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
	userCtx, ok := r.Context().Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		utils.ErrorResponse(w, http.StatusUnauthorized, fmt.Errorf("unauthorized"))
		return
	}

	utils.JsonResponse(w, http.StatusOK, UserResponsePayload{
		ID:    userCtx.UserID,
		Email: userCtx.Email,
		Role:  string(userCtx.Role),
	})
}
