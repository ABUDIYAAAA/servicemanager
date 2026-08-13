package database

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

func ConnectDB(connString string, ctx context.Context) (*pgxpool.Pool, error) {

	pool, err := pgxpool.New(ctx, connString)

	if err != nil {
		return nil, err
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, err
	}

	return pool, nil
}
