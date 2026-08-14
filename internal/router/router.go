package router

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"servicemanager/internal/config"
	"servicemanager/internal/dashboard"
	"servicemanager/internal/middleware"
	"servicemanager/internal/models"
	"servicemanager/internal/queue"
	"servicemanager/internal/services"
	"servicemanager/internal/users"
	"servicemanager/internal/utils"
	"servicemanager/internal/webhook"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func Router(pool *pgxpool.Pool, env *config.Env) http.Handler {
	mux := http.NewServeMux()

	// Initialize domain-driven instances
	userRepo := users.NewUserRepository(pool)
	userService := users.NewUserService(userRepo, env.JWT_SECRET, env.JWT_EXPIRY, env.GITHUB_APP_ID, env.GITHUB_APP_PRIVATE_KEY, env.BASE_URL)

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

	// Page Handlers
	usersPageHandler := users.NewUsersPageHandler(userService, env)
	servicesPageHandler := services.NewServicesPageHandler(serviceService, userService, env, asynqClient)
	dashboardHandler := dashboard.NewDashboardHandler(userService, serviceService, env)

	authMw := middleware.AuthMiddleware(env.JWT_SECRET)

	// Public Page Routes
	mux.HandleFunc("GET /login", usersPageHandler.GetLogin)
	mux.HandleFunc("POST /login", usersPageHandler.PostLogin)
	mux.HandleFunc("POST /logout", usersPageHandler.PostLogout)
	mux.HandleFunc("GET /accept-invite", usersPageHandler.GetAcceptInvite)
	mux.HandleFunc("POST /accept-invite", usersPageHandler.PostAcceptInvite)

	// GitHub Webhook (public — GitHub sends webhooks without auth)
	webhookHandler := webhook.NewWebhookHandler(pool, serviceService, asynqClient, env.GITHUB_WEBHOOK_SECRET)
	mux.HandleFunc("POST /api/v1/webhooks/github", webhookHandler.HandleGitHubWebhook)

	// Protected Page Actions
	mux.Handle("GET /{$}", authMw(http.HandlerFunc(dashboardHandler.GetDashboard)))
	mux.Handle("POST /users/invite", authMw(http.HandlerFunc(usersPageHandler.PostInviteUser)))
	mux.Handle("POST /services/add", authMw(http.HandlerFunc(servicesPageHandler.PostAddService)))
	mux.Handle("GET /services/{id}/deploy", authMw(http.HandlerFunc(servicesPageHandler.GetDeploy)))
	mux.Handle("POST /services/{id}/deploy", authMw(http.HandlerFunc(servicesPageHandler.PostDeploy)))
	mux.Handle("GET /services/{id}", authMw(http.HandlerFunc(servicesPageHandler.GetDetails)))
	mux.Handle("POST /services/{id}/envs", authMw(http.HandlerFunc(servicesPageHandler.PostUpdateEnvs)))
	mux.Handle("POST /services/{id}/redeploy", authMw(http.HandlerFunc(servicesPageHandler.PostRedeploy)))
	mux.Handle("GET /github/callback", authMw(http.HandlerFunc(dashboardHandler.GetGithubCallback)))

	// Live application logs stream for admins
	mux.Handle("GET /api/v1/admin/logs/stream", authMw(http.HandlerFunc(HandleLogsStream)))

	// Live logs stream for individual services (build & runtime logs from active deployment)
	mux.Handle("GET /api/v1/services/{id}/logs/stream", authMw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		HandleServiceLogsStream(w, r, serviceService)
	})))

	return middleware.LoggingMiddleware(mux)
}

func HandleLogsStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok || userCtx.Role != models.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Get historical logs if MongoDB is connected
	if utils.MongoClient != nil {
		col := utils.MongoClient.Database("servicesmanager").Collection("logs")
		findOptions := options.Find().SetLimit(100).SetSort(bson.D{{Key: "time", Value: -1}})
		cursor, err := col.Find(ctx, bson.D{}, findOptions)
		if err == nil {
			defer cursor.Close(ctx)
			var historicalLogs []bson.M
			if err := cursor.All(ctx, &historicalLogs); err == nil {
				for i := len(historicalLogs) - 1; i >= 0; i-- {
					jsonData, err := json.Marshal(historicalLogs[i])
					if err == nil {
						fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
						flusher.Flush()
					}
				}
			}
		}
	}

	ch := make(chan string, 100)
	utils.Broadcaster.Register(ch)
	defer utils.Broadcaster.Unregister(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}

func HandleServiceLogsStream(w http.ResponseWriter, r *http.Request, serviceService *services.ServiceService) {
	ctx := r.Context()
	_, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Load logs from the active deployment (not the service record)
	activeDeployment, err := serviceService.GetActiveDeployment(ctx, id)
	if err == nil && activeDeployment != nil {
		if activeDeployment.BuildLogs != "" {
			msg := services.ServiceLogMessage{
				ServiceID:    id,
				DeploymentID: activeDeployment.ID,
				Type:         "build",
				Log:          activeDeployment.BuildLogs,
			}
			jsonData, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
			flusher.Flush()
		}
		if activeDeployment.RuntimeLogs != "" {
			msg := services.ServiceLogMessage{
				ServiceID:    id,
				DeploymentID: activeDeployment.ID,
				Type:         "runtime",
				Log:          activeDeployment.RuntimeLogs,
			}
			jsonData, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
			flusher.Flush()
		}
	}

	// Register listener for live log updates
	ch := make(chan services.ServiceLogMessage, 100)
	services.LogBroadcaster.Register(id, ch)
	defer services.LogBroadcaster.Unregister(id, ch)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			jsonData, err := json.Marshal(msg)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
				flusher.Flush()
			}
		}
	}
}
