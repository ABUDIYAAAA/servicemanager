package router

import (
	"net/http"
	"servicemanager/internal/config"
	"servicemanager/internal/dashboard"
	"servicemanager/internal/middleware"
	"servicemanager/internal/services"
	"servicemanager/internal/users"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Router(pool *pgxpool.Pool, env *config.Env) http.Handler {
	mux := http.NewServeMux()

	// Initialize domain-driven instances
	userRepo := users.NewUserRepository(pool)
	userService := users.NewUserService(userRepo, env.JWT_SECRET, env.JWT_EXPIRY, env.GITHUB_APP_ID, env.GITHUB_APP_PRIVATE_KEY, env.BASE_URL)

	serviceRepo := services.NewServiceRepository(pool)
	serviceService := services.NewServiceService(serviceRepo)

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
	servicesPageHandler := services.NewServicesPageHandler(serviceService, userService)
	dashboardHandler := dashboard.NewDashboardHandler(userService, serviceService, env)

	authMw := middleware.AuthMiddleware(env.JWT_SECRET)

	// Public Page Routes
	mux.HandleFunc("GET /login", usersPageHandler.GetLogin)
	mux.HandleFunc("POST /login", usersPageHandler.PostLogin)
	mux.HandleFunc("POST /logout", usersPageHandler.PostLogout)
	mux.HandleFunc("GET /accept-invite", usersPageHandler.GetAcceptInvite)
	mux.HandleFunc("POST /accept-invite", usersPageHandler.PostAcceptInvite)

	// Protected Page Actions
	// Note: We use "GET /{$}" instead of "GET /" to prevent conflicts with other routing paths (e.g. "/api/v1/users/")
	mux.Handle("GET /{$}", authMw(http.HandlerFunc(dashboardHandler.GetDashboard)))
	mux.Handle("POST /users/invite", authMw(http.HandlerFunc(usersPageHandler.PostInviteUser)))
	mux.Handle("POST /services/add", authMw(http.HandlerFunc(servicesPageHandler.PostAddService)))
	mux.Handle("GET /services/{id}/deploy", authMw(http.HandlerFunc(servicesPageHandler.GetDeploy)))
	mux.Handle("POST /services/{id}/deploy", authMw(http.HandlerFunc(servicesPageHandler.PostDeploy)))
	mux.Handle("GET /services/{id}", authMw(http.HandlerFunc(servicesPageHandler.GetDetails)))
	mux.Handle("GET /github/callback", authMw(http.HandlerFunc(dashboardHandler.GetGithubCallback)))

	return middleware.LoggingMiddleware(mux)
}
