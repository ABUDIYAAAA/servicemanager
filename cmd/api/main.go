package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	migrations "servicemanager/cmd/migrate"
	"servicemanager/internal/config"
	"servicemanager/internal/database"
	"servicemanager/internal/mail"
	"servicemanager/internal/queue"
	"servicemanager/internal/router"
	"servicemanager/internal/services"
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
		} else {
			// Initialize TTL index for runtime_logs
			services.InitMongoIndexes(context.Background(), utils.MongoClient)
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

	// Verify Redis connection and fail startup if unavailable
	redisCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	err = queue.PingRedis(redisCtx, env.REDIS_URI)
	cancel()
	if err != nil {
		slog.Error("Failed to connect to Redis", slog.Any("error", err), slog.String("uri", env.REDIS_URI))
		os.Exit(1)
	}

	// Start SMTP Email Queue worker
	mailService := mail.NewMailService(env.SMTP_HOST, env.SMTP_PORT, env.SMTP_USER, env.SMTP_PASS, env.SMTP_FROM)
	shutdownEmailWorker := mail.StartWorker(env.REDIS_URI, mailService)

	// Connect to PostgreSQL database pool
	pool, err := database.ConnectDB(env.DB_URL, ctx)
	if err != nil {
		slog.Error("DB connection failed", slog.Any("error", err))
		return
	}
	defer pool.Close()

	shutdownAsynq := queue.StartWorker(env.REDIS_URI)

	router := router.Router(pool, env)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%s", env.PORT),
		Handler: router,
	}

	go func() {
		slog.Info("Starting server...", slog.String("port", env.PORT))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Error starting HTTP server", slog.Any("error", err))
		}
	}()

	// Start Log Pipeline & Collector
	services.StartLogPipeline(ctx, services.LogBroadcaster)
	if collector, err := services.NewLogCollector(); err == nil {
		collector.Start()
		defer collector.Stop()
	} else {
		slog.Error("Failed to initialize Log Collector", slog.Any("error", err))
	}

	// Wait for interrupt signal to gracefully shut down the servers
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit
	slog.Info("Graceful shutdown initiated...")

	shutdownAsynq()
	shutdownEmailWorker()

	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctxShutdown); err != nil {
		slog.Error("HTTP Server forced to shutdown", slog.Any("error", err))
	}

	slog.Info("Server exiting cleanly")
}
