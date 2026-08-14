package utils

import (
	"log/slog"
	"os"
)

// InitLogger sets up a structured JSON logger as the default logger.
func InitLogger() {
	opts := &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}

	// Use text formatting in development and JSON in production if needed,
	// but default to structured JSON for consistent logging patterns.
	var handler slog.Handler
	if os.Getenv("ENV") == "development" || os.Getenv("ENV") == "" {
		handler = slog.NewTextHandler(os.Stdout, opts)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	}

	logger := slog.New(handler)
	slog.SetDefault(logger)
}
