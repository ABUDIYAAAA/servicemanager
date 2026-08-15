package services

import (
	"context"
	"log/slog"
	"time"

	"servicemanager/internal/models"
	"servicemanager/internal/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var LogPipeline = make(chan models.LogEvent, 1000)

// StartLogPipeline starts a worker to consume and persist log events.
func StartLogPipeline(ctx context.Context, broadcaster *ServiceLogBroadcaster) {
	slog.Info("Starting Log Pipeline Worker...")
	go func() {
		for {
			select {
			case <-ctx.Done():
				slog.Info("Shutting down Log Pipeline Worker.")
				return
			case event := <-LogPipeline:
				// 1. Broadcast to SSE clients
				msg := ServiceLogMessage{
					ServiceID:    event.ServiceID,
					DeploymentID: event.DeploymentID,
					Type:         "runtime",
					Log:          event.Message,
				}
				
				// Let's modify the broadcaster logic in a moment or we can just find service ID from deployment ID.
				// Since finding serviceID per log line is expensive, maybe we include ServiceID in the container labels too.
				broadcaster.Broadcast(msg)

				// 2. Persist to MongoDB
				if utils.MongoClient != nil {
					col := utils.MongoClient.Database("servicesmanager").Collection("runtime_logs")
					// Fire and forget insert to keep pipeline fast. For production, batching is better.
					go func(e models.LogEvent) {
						insertCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
						defer cancel()
						_, err := col.InsertOne(insertCtx, e)
						if err != nil {
							slog.Error("Failed to persist log event to MongoDB", slog.Any("error", err), slog.Int("deployment_id", e.DeploymentID))
						}
					}(event)
				}
			}
		}
	}()
}

// InitMongoIndexes creates necessary indexes for the log pipeline.
func InitMongoIndexes(ctx context.Context, client *mongo.Client) {
	if client == nil {
		return
	}
	col := client.Database("servicesmanager").Collection("runtime_logs")

	// Create TTL index on timestamp (expire after 24 hours)
	indexModel := mongo.IndexModel{
		Keys: bson.D{{Key: "timestamp", Value: 1}},
		Options: options.Index().SetExpireAfterSeconds(int32(24 * 60 * 60)),
	}

	_, err := col.Indexes().CreateOne(ctx, indexModel)
	if err != nil {
		slog.Error("Failed to create TTL index for runtime_logs", slog.Any("error", err))
	} else {
		slog.Info("Successfully ensured TTL index for runtime_logs")
	}
}
