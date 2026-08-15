package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"

	"servicemanager/internal/middleware"
	"servicemanager/internal/models"
	"servicemanager/internal/utils"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// HandleAdminLogsStream provides a Server-Sent Events (SSE) stream of system-wide logs for administrators.
func HandleAdminLogsStream(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userCtx, ok := ctx.Value(middleware.UserContextKey).(middleware.UserContext)
	if !ok || userCtx.Role != models.RoleAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
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

	// Get historical logs if MongoDB is connected
	if utils.MongoClient != nil {
		col := utils.MongoClient.Database("servicesmanager").Collection("logs")
		findOptions := options.Find().SetLimit(100).SetSort(bson.D{{Key: "time", Value: -1}})
		cursor, err := col.Find(ctx, bson.D{}, findOptions)
		if err == nil {
			defer cursor.Close(ctx)
			var historicalLogs []bson.M
			if err := cursor.All(ctx, &historicalLogs); err == nil {
				for i := len(historicalLogs) - 1; i >= 0; i-- {
					jsonData, err := json.Marshal(historicalLogs[i])
					if err == nil {
						fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
						flusher.Flush()
					}
				}
			}
		}
	}

	ch := make(chan string, 100)
	utils.Broadcaster.Register(ch)
	defer utils.Broadcaster.Unregister(ch)

	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}
	}
}
