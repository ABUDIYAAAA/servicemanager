package router

import (
	"net/http"
	"servicemanager/internal/config"
	"servicemanager/internal/users"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Router(pool *pgxpool.Pool, env *config.Env) *http.ServeMux {
	mux := http.NewServeMux()
	userRouter := users.UserRouter(pool, env)

	mux.Handle("/api/v1/users/", http.StripPrefix("/api/v1/users", userRouter))

	return mux
}
