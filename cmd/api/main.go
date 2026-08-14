package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	migrations "servicemanager/cmd/migrate"
	"servicemanager/internal/config"
	"servicemanager/internal/database"
	"servicemanager/internal/queue"
	"servicemanager/internal/router"
	"servicemanager/internal/utils"
)

func main() {
	ctx := context.Background()

	// Load env configuration
	env, err := config.LoadEnv()
	if err != nil {
		fmt.Println("Error loading config:", err)
		return
	}

	// Connect to MongoDB if MONGO_DB_URL is configured
	if env.MONGO_DB_URL != "" {
		mongoCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		_, mongoErr := utils.ConnectMongo(mongoCtx, env.MONGO_DB_URL)
		cancel()
		if mongoErr != nil {
			fmt.Println("MongoDB connection failed, logging to stdout only:", mongoErr)
		}
	}

	// Initialize structured logger (which wraps MongoHandler if connected)
	utils.InitLogger()

	// Run PostgreSQL database migrations
	err = migrations.MigrateDB(env.DB_URL)
	if err != nil {
		slog.Error("Error applying migrations", slog.Any("error", err))
		return
	}

	// Start SMTP Email Queue worker
	utils.StartEmailWorker(ctx, env.SMTP_HOST, env.SMTP_PORT, env.SMTP_USER, env.SMTP_PASS, env.SMTP_FROM)

	// Connect to PostgreSQL database pool
	pool, err := database.ConnectDB(env.DB_URL, ctx)
	if err != nil {
		slog.Error("DB connection failed", slog.Any("error", err))
		return
	}
	defer pool.Close()

	// Start Asynq worker in a goroutine for processing deployment tasks
	if env.REDIS_URI != "" {
		go func() {
			srv := queue.NewAsynqServer(env.REDIS_URI)
			mux := queue.NewAsynqMux()
			slog.Info("Starting Asynq deployment queue worker...", slog.String("redis", env.REDIS_URI))
			if err := srv.Run(mux); err != nil {
				slog.Error("Asynq worker failed", slog.Any("error", err))
			}
		}()
	} else {
		slog.Warn("REDIS_URI not configured; deployment queue worker will not start")
	}

	router := router.Router(pool, env)

	slog.Info("Starting server...", slog.String("port", env.PORT))
	err = http.ListenAndServe(fmt.Sprintf(":%s", env.PORT), router)
	if err != nil {
		slog.Error("Error starting server", slog.Any("error", err))
	}
}
