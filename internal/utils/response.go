package utils

import (
	"encoding/json"
	"net/http"
)

type APIResponse struct {
	Success bool `json:"success"`
	Data    any  `json:"data,omitempty"`
	Error   any  `json:"error,omitempty"`
}

func JsonResponse(w http.ResponseWriter, status int, data any) {
	response := APIResponse{
		Success: true,
		Data:    data,
		Error:   nil,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(&response)
}

func ErrorResponse(w http.ResponseWriter, status int, err error) {

	response := APIResponse{
		Success: false,
		Data:    nil,
		Error:   err.Error(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(&response)
}
