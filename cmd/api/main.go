package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	migrations "servicemanager/cmd/migrate"
	"servicemanager/internal/config"
	"servicemanager/internal/database"
	"servicemanager/internal/router"
	"servicemanager/internal/utils"
)

func main() {
	// Initialize structured logger
	utils.InitLogger()

	env, err := config.LoadEnv()
	if err != nil {
		slog.Error("Error loading .env", slog.Any("error", err))
		return
	}

	err = migrations.MigrateDB(env.DB_URL)
	if err != nil {
		slog.Error("Error applying migrations", slog.Any("error", err))
		return
	}

	ctx := context.Background()

	// Start SMTP Email Queue worker
	utils.StartEmailWorker(ctx, env.SMTP_HOST, env.SMTP_PORT, env.SMTP_USER, env.SMTP_PASS, env.SMTP_FROM)

	pool, err := database.ConnectDB(env.DB_URL, ctx)
	if err != nil {
		slog.Error("DB connection failed", slog.Any("error", err))
		return
	}
	defer pool.Close()

	router := router.Router(pool, env)

	slog.Info("Starting server...", slog.String("port", env.PORT))
	err = http.ListenAndServe(fmt.Sprintf(":%s", env.PORT), router)
	if err != nil {
		slog.Error("Error starting server", slog.Any("error", err))
	}
}
