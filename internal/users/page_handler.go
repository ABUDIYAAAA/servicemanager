package users

import (
	"net/http"
	"time"

	"servicemanager/internal/config"
	"servicemanager/internal/middleware"
	"servicemanager/internal/models"
	"servicemanager/internal/utils"
	"servicemanager/internal/views/auth"
)

type UsersPageHandler struct {
	service *UserService
	env     *config.Env
}

func NewUsersPageHandler(service *UserService, env *config.Env) *UsersPageHandler {
	return &UsersPageHandler{
		service: service,
		env:     env,
	}
}

func (h *UsersPageHandler) GetLogin(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie("token"); err == nil && cookie.Value != "" {
		_, err := utils.VerifyToken(cookie.Value, h.env.JWT_SECRET)
		if err == nil {
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
	}
	auth.Login("").Render(r.Context(), w)
}

func (h *UsersPageHandler) PostLogin(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	email := r.FormValue("email")
	password := r.FormValue("password")

	res, err := h.service.Login(r.Context(), email, password)
	if err != nil {
		auth.Login("Invalid email or password").Render(r.Context(), w)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    res.Token,
		Path:     "/",
		Expires:  time.Now().Add(30 * 24 * time.Hour),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})

	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *UsersPageHandler) PostLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		Expires:  time.Unix(0, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *UsersPageHandler) GetAcceptInvite(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	email := r.URL.Query().Get("email")
	if token == "" {
		auth.AcceptInvite("", "", "Missing invitation token").Render(r.Context(), w)
		return
	}
	auth.AcceptInvite(token, email, "").Render(r.Context(), w)
}

func (h *UsersPageHandler) PostAcceptInvite(w http.ResponseWriter, r *http.Request) {
	_ = r.ParseForm()
	token := r.FormValue("token")
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm_password")

	if password != confirm {
		auth.AcceptInvite(token, email, "Passwords do not match").Render(r.Context(), w)
		return
	}

	_, err := h.service.AcceptInvite(r.Context(), token, email, password)
	if err != nil {
		auth.AcceptInvite(token, email, err.Error()).Render(r.Context(), w)
		return
	}

	http.Redirect(w, r, "/login?success=1", http.StatusSeeOther)
}

func (h *UsersPageHandler) PostInviteUser(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok || userCtx.Role != models.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	_ = r.ParseForm()
	email := r.FormValue("email")

	_, err := h.service.CreateInvite(ctx, email)
	if err != nil {
		http.Redirect(w, r, "/?invite_error="+err.Error()+"#users", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/#users", http.StatusSeeOther)
}
