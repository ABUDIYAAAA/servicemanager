package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/hibiken/asynq"
)

const (
	TypeServiceDeploy = "service:deploy"
	QueueDeployments  = "deployments"
)

type DeployPayload struct {
	ServiceID    int `json:"service_id"`
	DeploymentID int `json:"deployment_id"`
	UserID       int `json:"user_id"`
}

// NewDeployTask creates a new Asynq task for deploying a service.
// Uses asynq.Queue to route to the deployments queue and asynq.MaxRetry(0) to prevent automatic retries.
func NewDeployTask(serviceID, deploymentID, userID int) (*asynq.Task, error) {
	payload, err := json.Marshal(DeployPayload{
		ServiceID:    serviceID,
		DeploymentID: deploymentID,
		UserID:       userID,
	})
	if err != nil {
		return nil, err
	}
	return asynq.NewTask(TypeServiceDeploy, payload, asynq.Queue(QueueDeployments), asynq.MaxRetry(0)), nil
}

// DeployHandler is the function signature that the queue worker calls to execute a deployment.
// It is set externally by the main package to avoid circular imports.
type DeployHandler func(ctx context.Context, serviceID, deploymentID, userID int) error

var deployHandler DeployHandler

// SetDeployHandler sets the handler function that processes deploy tasks.
func SetDeployHandler(h DeployHandler) {
	deployHandler = h
}

// HandleDeployTask is the Asynq handler that deserializes the task payload and delegates to the deploy handler.
func HandleDeployTask(ctx context.Context, t *asynq.Task) error {
	var payload DeployPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal deploy payload: %w", err)
	}

	slog.Info("Processing deploy task from queue",
		slog.Int("service_id", payload.ServiceID),
		slog.Int("deployment_id", payload.DeploymentID),
		slog.Int("user_id", payload.UserID),
	)

	if deployHandler == nil {
		return fmt.Errorf("deploy handler not configured")
	}

	return deployHandler(ctx, payload.ServiceID, payload.DeploymentID, payload.UserID)
}

// NewAsynqClient creates a new Asynq client connected to the given Redis URI.
func NewAsynqClient(redisURI string) *asynq.Client {
	opt, err := asynq.ParseRedisURI(redisURI)
	if err != nil {
		slog.Error("Failed to parse Redis URI for Asynq client, using default localhost", slog.Any("error", err))
		opt = asynq.RedisClientOpt{Addr: "localhost:6379"}
	}
	return asynq.NewClient(opt)
}

// NewAsynqServer creates a new Asynq server with concurrency=1 on the deployments queue.
// This ensures only one deployment runs at a time, preventing race conditions.
func NewAsynqServer(redisURI string) *asynq.Server {
	opt, err := asynq.ParseRedisURI(redisURI)
	if err != nil {
		slog.Error("Failed to parse Redis URI for Asynq server, using default localhost", slog.Any("error", err))
		opt = asynq.RedisClientOpt{Addr: "localhost:6379"}
	}

	return asynq.NewServer(opt, asynq.Config{
		Concurrency: 1,
		Queues: map[string]int{
			QueueDeployments: 1,
		},
		ErrorHandler: asynq.ErrorHandlerFunc(func(ctx context.Context, task *asynq.Task, err error) {
			slog.Error("Asynq task failed",
				slog.String("type", task.Type()),
				slog.Any("error", err),
			)
		}),
	})
}

// NewAsynqMux creates a new Asynq ServeMux and registers the deploy task handler.
func NewAsynqMux() *asynq.ServeMux {
	mux := asynq.NewServeMux()
	mux.HandleFunc(TypeServiceDeploy, HandleDeployTask)
	return mux
}
