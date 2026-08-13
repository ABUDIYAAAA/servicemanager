package config

import (
	"os"

	"github.com/joho/godotenv"
)

type Env struct {
	PORT   string
	DB_URL string

	JWT_SECRET string
	JWT_EXPIRY string
}

func LoadEnv() (*Env, error) {
	err := godotenv.Load()

	if err != nil {

		return nil, err
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "super-secret-key-change-me"
	}
	jwtExpiry := os.Getenv("JWT_EXPIRY")
	if jwtExpiry == "" {
		jwtExpiry = "24h"
	}

	return &Env{
		PORT:       os.Getenv("PORT"),
		DB_URL:     os.Getenv("DB_URL"),
		JWT_SECRET: jwtSecret,
		JWT_EXPIRY: jwtExpiry,
	}, nil

}
