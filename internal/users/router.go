package users

import (
	"net/http"

	"servicemanager/internal/config"
	"servicemanager/internal/middleware"
	"servicemanager/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

func UserRouter(pool *pgxpool.Pool, env *config.Env) *http.ServeMux {
	mux := http.NewServeMux()
	userRepo := NewUserRepository(pool)
	userService := NewUserService(userRepo, env.JWT_SECRET, env.JWT_EXPIRY, env.GITHUB_APP_ID, env.GITHUB_APP_PRIVATE_KEY, env.BASE_URL)
	userHandler := NewUserHandler(userService)

	// Public Routes
	mux.HandleFunc("POST /login", userHandler.Login)
	mux.HandleFunc("POST /logout", userHandler.Logout)
	mux.HandleFunc("POST /accept-invite", userHandler.AcceptInvite)
	mux.HandleFunc("POST /forgot-password", userHandler.ForgotPassword)
	mux.HandleFunc("POST /reset-password", userHandler.ResetPassword)

	// Auth Middleware
	authMw := middleware.AuthMiddleware(env.JWT_SECRET)
	adminOnly := func(next http.Handler) http.Handler {
		return authMw(middleware.RequireRole(models.RoleAdmin)(next))
	}

	// Authenticated Routes
	mux.Handle("GET /me", authMw(http.HandlerFunc(userHandler.GetMe)))
	mux.Handle("POST /github/installations", authMw(http.HandlerFunc(userHandler.InstallGithub)))
	mux.Handle("GET /github/repos", authMw(http.HandlerFunc(userHandler.GetGithubRepositories)))
	mux.Handle("GET /github/url", authMw(http.HandlerFunc(userHandler.GetGithubURL)))
	mux.Handle("GET /", authMw(http.HandlerFunc(userHandler.GetAllUsers)))
	mux.Handle("GET /invites", authMw(http.HandlerFunc(userHandler.GetAllInvites)))

	// Admin Only Routes
	mux.Handle("DELETE /{id}", adminOnly(http.HandlerFunc(userHandler.RemoveUser)))
	mux.Handle("PATCH /{id}/role", adminOnly(http.HandlerFunc(userHandler.ChangeUserRole)))
	mux.Handle("POST /invite", adminOnly(http.HandlerFunc(userHandler.CreateNewInvite)))
	mux.Handle("DELETE /invite/{email}", adminOnly(http.HandlerFunc(userHandler.DeleteInvite)))

	return mux
}
