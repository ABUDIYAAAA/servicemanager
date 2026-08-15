package router

import (
	"net/http"

	"servicemanager/internal/config"
	"servicemanager/internal/dashboard"
	"servicemanager/internal/middleware"
	"servicemanager/internal/queue"
	"servicemanager/internal/services"
	"servicemanager/internal/users"
	"servicemanager/internal/webhook"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Router(pool *pgxpool.Pool, env *config.Env) http.Handler {
	mux := http.NewServeMux()


	serviceRepo := services.NewServiceRepository(pool)
	deployRepo := services.NewDeploymentRepository(pool)
	serviceService := services.NewServiceService(serviceRepo, deployRepo)

	// Initialize ServiceBuilder
	builder := services.NewServiceBuilder(pool, deployRepo, env.GITHUB_APP_ID, env.GITHUB_APP_PRIVATE_KEY, env.SERVICES_ROOT_DIR)

	// Initialize Asynq client
	asynqClient := queue.NewAsynqClient(env.REDIS_URI)

	// Register deploy handler for the queue worker
	queue.SetDeployHandler(builder.ExecuteDeployment)

	// Routers for APIs
	userRouter := users.UserRouter(pool, env)
	serviceRouter := services.ServiceRouter(pool, env)

	mux.Handle("/api/v1/users/", http.StripPrefix("/api/v1/users", userRouter))
	mux.Handle("/api/v1/services/", http.StripPrefix("/api/v1/services", serviceRouter))

	// Swagger documentation endpoints
	mux.HandleFunc("GET /swagger.json", ServeSwaggerJSON)
	mux.HandleFunc("GET /swagger/", ServeSwaggerUI)

	authMw := middleware.AuthMiddleware(env.JWT_SECRET)

	// GitHub Webhook (public — GitHub sends webhooks without auth)
	webhookHandler := webhook.NewWebhookHandler(pool, serviceService, asynqClient, env.GITHUB_WEBHOOK_SECRET)
	mux.HandleFunc("POST /api/v1/webhooks/github", webhookHandler.HandleGitHubWebhook)

	// Live application logs stream for admins
	mux.Handle("GET /api/v1/admin/logs/stream", authMw(http.HandlerFunc(dashboard.HandleAdminLogsStream)))

	return middleware.LoggingMiddleware(middleware.CorsMiddleware(mux))
}


