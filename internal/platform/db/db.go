package db

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB wraps pgxpool with helper methods.
type DB struct {
	pool *pgxpool.Pool
}

// New initializes a pgx connection pool.
func New(ctx context.Context, databaseURL string) (*DB, error) {
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("unable to create connection pool: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		return nil, fmt.Errorf("unable to ping database: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Pool returns the underlying pgxpool.
func (db *DB) Pool() *pgxpool.Pool {
	return db.pool
}

// Close shuts down the connection pool.
func (db *DB) Close() {
	db.pool.Close()
}

// WithTx executes a function within a database transaction.
func (db *DB) WithTx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, err := db.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// HealthCheck verifies database connectivity.
func (db *DB) HealthCheck(ctx context.Context) error {
	return db.pool.Ping(ctx)
}
