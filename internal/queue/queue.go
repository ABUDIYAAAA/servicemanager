package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"servicemanager/internal/tasks"
)

// PingRedis attempts to connect to Redis to ensure it's available.
func PingRedis(ctx context.Context, redisURI string) error {
	if redisURI == "" {
		return fmt.Errorf("REDIS_URI is required and cannot be empty (will not default to localhost)")
	}
	opt, err := asynq.ParseRedisURI(redisURI)
	if err != nil {
		return err
	}
	r := opt.MakeRedisClient().(redis.UniversalClient)
	defer r.Close()
	return r.Ping(ctx).Err()
}

// DeployPayload is the data carried by a deployment.create task.
type DeployPayload struct {
	ServiceID    int `json:"service_id"`
	DeploymentID int `json:"deployment_id"`
	UserID       int `json:"user_id"`
}

// NewDeploymentCreateTask creates a new Asynq task for the deployment.create queue.
//
// Key behaviours:
//   - TaskID is unique per service: if a task for the same service is already *pending*
//     in the queue, Asynq will reject the duplicate enqueue (ErrTaskIDConflict).
//     We handle that at call-site by cancelling the older queued deployment record and
//     re-enqueuing with a fresh task ID that encodes the new deployment ID.
//   - MaxRetry(0): build failures are not retried automatically.
//   - Retention(24h): completed task records are kept for a day.
func NewDeploymentCreateTask(serviceID, deploymentID, userID int) (*asynq.Task, error) {
	payload, err := json.Marshal(DeployPayload{
		ServiceID:    serviceID,
		DeploymentID: deploymentID,
		UserID:       userID,
	})
	if err != nil {
		return nil, err
	}

	// Unique ID encodes both service and deployment so each deployment is distinct,
	// yet callers can inspect / cancel by service-scoped prefix.
	taskID := fmt.Sprintf("deploy:svc-%d:dep-%d", serviceID, deploymentID)

	return asynq.NewTask(
		tasks.TypeDeploymentCreate,
		payload,
		asynq.Queue(tasks.QueueDeployments),
		asynq.MaxRetry(0),
		asynq.TaskID(taskID),
		asynq.Retention(24*time.Hour),
	), nil
}

// DeployHandler is the function signature that the queue worker calls to execute a deployment.
type DeployHandler func(ctx context.Context, serviceID, deploymentID, userID int) error

var deployHandler DeployHandler

// SetDeployHandler registers the handler that processes deployment.create tasks.
func SetDeployHandler(h DeployHandler) {
	deployHandler = h
}

// HandleDeploymentCreateTask is the Asynq handler for deployment.create tasks.
func HandleDeploymentCreateTask(ctx context.Context, t *asynq.Task) error {
	var payload DeployPayload
	if err := json.Unmarshal(t.Payload(), &payload); err != nil {
		return fmt.Errorf("failed to unmarshal deploy payload: %w", err)
	}

	slog.Info("Processing deployment.create task",
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
		slog.Error("Failed to parse Redis URI for Asynq client", slog.Any("error", err))
		panic(fmt.Sprintf("Failed to parse Redis URI: %v", err))
	}
	return asynq.NewClient(opt)
}

// NewAsynqServer creates a new Asynq server on the deployments queue.
func NewAsynqServer(redisURI string) *asynq.Server {
	opt, err := asynq.ParseRedisURI(redisURI)
	if err != nil {
		slog.Error("Failed to parse Redis URI for Asynq server", slog.Any("error", err))
		panic(fmt.Sprintf("Failed to parse Redis URI: %v", err))
	}

	return asynq.NewServer(opt, asynq.Config{
		Concurrency: 5,
		Queues: map[string]int{
			tasks.QueueDeployments: 5,
		},
	})
}
