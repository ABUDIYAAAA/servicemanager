package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"servicemanager/internal/middleware"
	"servicemanager/internal/models"
	"servicemanager/internal/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// HandleServiceLogsStream provides a Server-Sent Events (SSE) stream of build and runtime logs for a specific service.
func (h *ServiceHandler) HandleServiceLogsStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid service ID", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	// Load logs from the active deployment
	activeDeployment, err := h.service.GetActiveDeployment(ctx, id)
	if err == nil && activeDeployment != nil {
		if activeDeployment.BuildLogs != "" {
			msg := ServiceLogMessage{
				ServiceID:    id,
				DeploymentID: activeDeployment.ID,
				Type:         "build",
				Log:          activeDeployment.BuildLogs,
			}
			jsonData, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
			flusher.Flush()
		}
		
		// Fetch recent runtime logs from MongoDB if connected
		if utils.MongoClient != nil {
			col := utils.MongoClient.Database("servicesmanager").Collection("runtime_logs")
			filter := bson.M{"deployment_id": activeDeployment.ID}
			
			// Find up to 1000 latest logs
			findOptions := options.Find()
			findOptions.SetSort(bson.D{{Key: "timestamp", Value: 1}})
			findOptions.SetLimit(1000)

			cur, err := col.Find(ctx, filter, findOptions)
			if err == nil {
				for cur.Next(ctx) {
					var event models.LogEvent
					if err := cur.Decode(&event); err == nil {
						msg := ServiceLogMessage{
							ServiceID:    id,
							DeploymentID: activeDeployment.ID,
							Type:         "runtime",
							Log:          event.Message,
						}
						jsonData, _ := json.Marshal(msg)
						fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
					}
				}
				cur.Close(ctx)
				flusher.Flush()
			}
		}
	}

	// Register listener for live log updates
	ch := make(chan ServiceLogMessage, 100)
	LogBroadcaster.Register(id, ch)
	defer LogBroadcaster.Unregister(id, ch)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			jsonData, err := json.Marshal(msg)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
				flusher.Flush()
			}
		}
	}
}
