package main

import (
	"context"
	"fmt"
	"os"

	"servicemanager/internal/config"
	"servicemanager/internal/database"
	"servicemanager/internal/models"
	"servicemanager/internal/utils"
)

func main() {
	env, err := config.LoadEnv()
	if err != nil {
		fmt.Println("Error loading config:", err)
		os.Exit(1)
	}

	adminEmail := os.Getenv("ADMIN_EMAIL")
	if adminEmail == "" {
		adminEmail = "admin@example.com"
	}

	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		adminPassword = "AdminPassword123!"
	}

	ctx := context.Background()
	pool, err := database.ConnectDB(env.DB_URL, ctx)
	if err != nil {
		fmt.Println("Error connecting to database:", err)
		os.Exit(1)
	}
	defer pool.Close()

	// Check if admin user already exists
	var count int
	err = pool.QueryRow(ctx, "SELECT COUNT(*) FROM users WHERE email = $1", adminEmail).Scan(&count)
	if err != nil {
		fmt.Println("Error checking for existing admin:", err)
		os.Exit(1)
	}

	if count > 0 {
		fmt.Printf("Admin user with email %s already exists. Skipping seeding.\n", adminEmail)
		return
	}

	// Hash password using Argon2
	hashedPassword, err := utils.HashPassword(adminPassword)
	if err != nil {
		fmt.Println("Error hashing password:", err)
		os.Exit(1)
	}

	// Insert admin user
	query := `INSERT INTO users (email, password_hash, user_role) VALUES ($1, $2, $3)`
	_, err = pool.Exec(ctx, query, adminEmail, hashedPassword, models.RoleAdmin)
	if err != nil {
		fmt.Println("Error creating admin user:", err)
		os.Exit(1)
	}

	fmt.Printf("Admin user created successfully!\n")
	fmt.Printf("Email: %s\n", adminEmail)
	fmt.Printf("Password: %s\n", adminPassword)
}
