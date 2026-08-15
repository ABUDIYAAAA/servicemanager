package services

import (
	"net/http"
	"servicemanager/internal/config"
	"servicemanager/internal/middleware"
	"servicemanager/internal/models"
	"servicemanager/internal/queue"
	"servicemanager/internal/utils"

	"github.com/hibiken/asynq"
	"github.com/jackc/pgx/v5/pgxpool"
)

func ServiceRouter(pool *pgxpool.Pool, env *config.Env, asynqClient *asynq.Client) *http.ServeMux {
	mux := http.NewServeMux()
	repo := NewServiceRepository(pool)
	deployRepo := NewDeploymentRepository(pool)
	infisicalClient := utils.NewInfisicalClient(env.INFISICAL_URL, env.INFISICAL_CLIENT_ID, env.INFISICAL_CLIENT_SECRET)
	service := NewServiceService(repo, deployRepo, infisicalClient)

	// Pass the asynq client into the handler for manual deploys
	if asynqClient == nil {
		asynqClient = queue.NewAsynqClient(env.REDIS_URI)
	}
	handler := NewServiceHandler(service, env.GITHUB_APP_ID, env.GITHUB_APP_PRIVATE_KEY, asynqClient)

	authMw := middleware.AuthMiddleware(env.JWT_SECRET)
	adminOnly := func(next http.Handler) http.Handler {
		return authMw(middleware.RequireRole(models.RoleAdmin)(next))
	}

	// Read routes (authenticated users)
	mux.Handle("GET /", authMw(http.HandlerFunc(handler.GetAllServices)))
	mux.Handle("GET /directories", authMw(http.HandlerFunc(handler.GetRepositoryDirectories)))
	mux.Handle("GET /{id}/logs/stream", authMw(http.HandlerFunc(handler.HandleServiceLogsStream)))
	mux.Handle("GET /{id}/deployment/latest", authMw(http.HandlerFunc(handler.GetLatestDeployment)))

	// Write routes (admins only)
	mux.Handle("POST /", adminOnly(http.HandlerFunc(handler.CreateService)))
	mux.Handle("POST /{id}/deploy", adminOnly(http.HandlerFunc(handler.TriggerDeploy)))
	mux.Handle("DELETE /{id}", adminOnly(http.HandlerFunc(handler.DeleteService)))

	return mux
}
