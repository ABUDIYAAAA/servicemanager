package services

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"servicemanager/internal/middleware"
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
		if activeDeployment.RuntimeLogs != "" {
			msg := ServiceLogMessage{
				ServiceID:    id,
				DeploymentID: activeDeployment.ID,
				Type:         "runtime",
				Log:          activeDeployment.RuntimeLogs,
			}
			jsonData, _ := json.Marshal(msg)
			fmt.Fprintf(w, "data: %s\n\n", string(jsonData))
			flusher.Flush()
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
