package queue

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

// NewWorkerServer creates a new Asynq server configured for a specific queue.
func NewWorkerServer(redisURI string, queueName string, concurrency int) *asynq.Server {
	opt, err := asynq.ParseRedisURI(redisURI)
	if err != nil {
		slog.Error("Failed to parse Redis URI for Asynq server", slog.Any("error", err))
		panic(fmt.Sprintf("Failed to parse Redis URI: %v", err))
	}

	return asynq.NewServer(opt, asynq.Config{
		Concurrency: concurrency,
		Queues: map[string]int{
			queueName: 1,
		},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			slog.Error("Asynq task failed",
				slog.String("queue", queueName),
				slog.String("type", task.Type()),
				slog.Any("error", err),
			)
		}),
	})
}

// NewWorkerMux creates a new generic Asynq ServeMux.
func NewWorkerMux() *asynq.ServeMux {
	return asynq.NewServeMux()
}
