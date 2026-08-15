package queue

import (
	"log/slog"
	"servicemanager/internal/tasks"
)

// StartWorker initializes and starts the Asynq deployment worker in the background.
// Returns a shutdown function that gracefully stops the worker when called.
func StartWorker(redisURI string) func() {
	if redisURI == "" {
		slog.Warn("REDIS_URI not configured; deployment queue worker will not start")
		return func() {}
	}

	asynqSrv := NewWorkerServer(redisURI, tasks.QueueDeployments, 5)
	mux := NewWorkerMux()
	mux.HandleFunc(tasks.TypeDeploymentCreate, HandleDeploymentCreateTask)

	slog.Info("Starting Asynq deployment queue worker...", slog.String("redis", redisURI))
	go func() {
		if err := asynqSrv.Start(mux); err != nil {
			slog.Error("Asynq deployment worker failed", slog.Any("error", err))
		}
	}()

	return func() {
		slog.Info("Shutting down Asynq deployment worker...")
		asynqSrv.Shutdown()
	}
}
