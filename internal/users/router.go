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
	userService := NewUserService(userRepo, env.JWT_SECRET, env.JWT_EXPIRY)
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

	// Admin Only Routes
	mux.Handle("GET /", adminOnly(http.HandlerFunc(userHandler.GetAllUsers)))
	mux.Handle("DELETE /{id}", adminOnly(http.HandlerFunc(userHandler.RemoveUser)))
	mux.Handle("PATCH /{id}/role", adminOnly(http.HandlerFunc(userHandler.ChangeUserRole)))
	mux.Handle("POST /invite", adminOnly(http.HandlerFunc(userHandler.CreateNewInvite)))
	mux.Handle("GET /invites", adminOnly(http.HandlerFunc(userHandler.GetAllInvites)))
	mux.Handle("DELETE /invite/{token}", adminOnly(http.HandlerFunc(userHandler.DeleteInvite)))

	return mux
}
