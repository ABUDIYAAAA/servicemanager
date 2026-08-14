package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Env struct {
	PORT                   string
	BASE_URL               string
	DB_URL                 string
	JWT_SECRET             string
	JWT_EXPIRY             string
	ADMIN_EMAIL            string
	ADMIN_PASSWORD         string
	GITHUB_APP_ID          string
	GITHUB_APP_PRIVATE_KEY string // Direct PEM string or path to PEM file
	GITHUB_CLIENT_ID       string
	GITHUB_CLIENT_SECRET   string

	// SMTP configuration
	SMTP_HOST string
	SMTP_PORT string
	SMTP_USER string
	SMTP_PASS string
	SMTP_FROM string

	// Infisical configuration
	INFISICAL_URL           string
	INFISICAL_CLIENT_ID     string
	INFISICAL_CLIENT_SECRET string
	MONGO_DB_URL            string
	SERVICES_ROOT_DIR       string
	REDIS_URI               string
	GITHUB_WEBHOOK_SECRET   string
}

func LoadEnv() (*Env, error) {
	// Try loading .env, ignore error if it doesn't exist (e.g. in production envs)
	_ = godotenv.Load()

	// For private key, let's see if it's a file path or direct content
	privateKey := os.Getenv("GITHUB_APP_PRIVATE_KEY")
	if privateKey != "" {
		// If it looks like a path, try reading it
		if _, err := os.Stat(privateKey); err == nil {
			content, err := os.ReadFile(privateKey)
			if err == nil {
				privateKey = string(content)
			}
		}
	}

	return &Env{
		PORT:                   os.Getenv("PORT"),
		BASE_URL:               os.Getenv("BASE_URL"),
		DB_URL:                 os.Getenv("DB_URL"),
		JWT_SECRET:             os.Getenv("JWT_SECRET"),
		JWT_EXPIRY:             os.Getenv("JWT_EXPIRY"),
		ADMIN_EMAIL:            os.Getenv("ADMIN_EMAIL"),
		ADMIN_PASSWORD:         os.Getenv("ADMIN_PASSWORD"),
		GITHUB_APP_ID:          os.Getenv("GITHUB_APP_ID"),
		GITHUB_APP_PRIVATE_KEY: privateKey,
		GITHUB_CLIENT_ID:       os.Getenv("GITHUB_CLIENT_ID"),
		GITHUB_CLIENT_SECRET:   os.Getenv("GITHUB_CLIENT_SECRET"),
		SMTP_HOST:              os.Getenv("SMTP_HOST"),
		SMTP_PORT:              os.Getenv("SMTP_PORT"),
		SMTP_USER:              os.Getenv("SMTP_USER"),
		SMTP_PASS:              os.Getenv("SMTP_PASS"),
		SMTP_FROM:              os.Getenv("SMTP_FROM"),
		INFISICAL_URL:           os.Getenv("INFISICAL_URL"),
		INFISICAL_CLIENT_ID:     os.Getenv("INFISICAL_CLIENT_ID"),
		INFISICAL_CLIENT_SECRET: os.Getenv("INFISICAL_CLIENT_SECRET"),
		MONGO_DB_URL:            os.Getenv("MONGO_DB_URL"),
		SERVICES_ROOT_DIR:       os.Getenv("SERVICES_ROOT_DIR"),
		REDIS_URI:               os.Getenv("REDIS_URI"),
		GITHUB_WEBHOOK_SECRET:   os.Getenv("GITHUB_WEBHOOK_SECRET"),
	}, nil
}
