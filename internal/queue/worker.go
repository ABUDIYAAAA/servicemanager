package queue

import (
	"log/slog"
)

// StartWorker initializes and starts the Asynq deployment worker in the background.
// It returns a shutdown function that gracefully stops the worker when called.
func StartWorker(redisURI string) func() {
	if redisURI == "" {
		slog.Warn("REDIS_URI not configured; deployment queue worker will not start")
		return func() {}
	}

	asynqSrv := NewAsynqServer(redisURI)
	mux := NewAsynqMux()

	slog.Info("Starting Asynq deployment queue worker...", slog.String("redis", redisURI))
	go func() {
		if err := asynqSrv.Start(mux); err != nil {
			slog.Error("Asynq worker failed", slog.Any("error", err))
		}
	}()

	return func() {
		slog.Info("Shutting down Asynq worker...")
		asynqSrv.Shutdown()
	}
}
