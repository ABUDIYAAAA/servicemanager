package main

import (
	"context"
	"log/slog"
	"os"

	"servicemanager/internal/config"
	"servicemanager/internal/database"
	"servicemanager/internal/models"
	"servicemanager/internal/utils"
)

func main() {
	// Initialize structured logger
	utils.InitLogger()

	env, err := config.LoadEnv()
	if err != nil {
		slog.Error("Error loading config", slog.Any("error", err))
		os.Exit(1)
	}

	adminEmail := env.ADMIN_EMAIL
	adminPassword := env.ADMIN_PASSWORD

	ctx := context.Background()
	pool, err := database.ConnectDB(env.DB_URL, ctx)
	if err != nil {
		slog.Error("Error connecting to database", slog.Any("error", err))
		os.Exit(1)
	}
	defer pool.Close()

	// Check if admin user already exists
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE email = $1", adminEmail).Scan(&count)
	if err != nil {
		slog.Error("Error checking for existing admin", slog.Any("error", err))
		os.Exit(1)
	}

	if count > 0 {
		slog.Info("Admin user already exists. Skipping seeding.", slog.String("email", adminEmail))
		return
	}

	// Hash password using Argon2
	hashedPassword, err := utils.HashPassword(adminPassword)
	if err != nil {
		slog.Error("Error hashing password", slog.Any("error", err))
		os.Exit(1)
	}

	// Insert admin user
	query := `INSERT INTO users (email, password_hash, user_role) VALUES ($1, $2, $3)`
	_, err = pool.Exec(ctx, query, adminEmail, hashedPassword, models.RoleAdmin)
	if err != nil {
		slog.Error("Error creating admin user", slog.Any("error", err))
		os.Exit(1)
	}

	slog.Info("Admin user created successfully!",
		slog.String("email", adminEmail),
		slog.String("password", adminPassword),
	)
}
