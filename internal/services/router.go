package services

import (
	"net/http"
	"servicemanager/internal/config"
	"servicemanager/internal/middleware"
	"servicemanager/internal/models"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ServiceRouter(pool *pgxpool.Pool, env *config.Env) *http.ServeMux {
	mux := http.NewServeMux()
	repo := NewServiceRepository(pool)
	service := NewServiceService(repo)
	handler := NewServiceHandler(service)

	authMw := middleware.AuthMiddleware(env.JWT_SECRET)
	adminOnly := func(next http.Handler) http.Handler {
		return authMw(middleware.RequireRole(models.RoleAdmin)(next))
	}

	// Read routes (authenticated users can read)
	mux.Handle("GET /", authMw(http.HandlerFunc(handler.GetAllServices)))

	// Write routes (admins only)
	mux.Handle("POST /", adminOnly(http.HandlerFunc(handler.CreateService)))
	mux.Handle("DELETE /{id}", adminOnly(http.HandlerFunc(handler.DeleteService)))

	return mux
}
