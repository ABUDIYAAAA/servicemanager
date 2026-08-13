package main

import (
	"context"
	"fmt"
	"net/http"
	migrations "servicemanager/cmd/migrate"
	"servicemanager/internal/config"
	"servicemanager/internal/database"
	"servicemanager/internal/router"
)

func main() {

	env, err := config.LoadEnv()

	if err != nil {
		fmt.Println("Error loading .env", err)
		return
	}

	err = migrations.MigrateDB(env.DB_URL)

	if err != nil {
		fmt.Println("Error applying migrations", err)
		return
	}

	ctx := context.Background()
	pool, err := database.ConnectDB(env.DB_URL, ctx)

	if err != nil {
		fmt.Println("DB connection failed: ", err)
		return
	}

	defer pool.Close()

	router := router.Router(pool, env)

	err = http.ListenAndServe(fmt.Sprintf(":%s", env.PORT), router)

	if err != nil {
		fmt.Println("Error starting server: ", err)

	}
	fmt.Println("Running on port: ", env.PORT)

}
