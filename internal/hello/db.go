package hello

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// DB is a function's Neon pool. It opens on first use, so a function starts before Neon is reachable, and it reopens
// when Neon rejects the password, so a changed password takes effect without replacing the function.
type DB struct {
	// Open connects to Neon, reading the connection string again each time. It defaults to OpenDB.
	Open func(context.Context) (*pgxpool.Pool, error)

	mu   sync.Mutex
	pool *pgxpool.Pool
}

// Run calls fn with the pool. When Neon rejects the pool's password, Run reopens the pool and calls fn once more.
func (d *DB) Run(ctx context.Context, fn func(*pgxpool.Pool) error) error {
	pool, err := d.get(ctx)
	if err != nil {
		return err
	}
	err = fn(pool)
	if !isAuthFailure(err) {
		return err
	}
	d.discard(pool)
	if pool, err = d.get(ctx); err != nil {
		return err
	}
	return fn(pool)
}

// get returns the open pool, opening it within ten seconds if no earlier attempt succeeded.
func (d *DB) get(ctx context.Context) (*pgxpool.Pool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pool != nil {
		return d.pool, nil
	}
	open := d.Open
	if open == nil {
		open = OpenDB
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	pool, err := open(ctx)
	if err != nil {
		return nil, err
	}
	d.pool = pool
	return pool, nil
}

// discard closes pool unless another caller already replaced it.
func (d *DB) discard(pool *pgxpool.Pool) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.pool == pool {
		d.pool.Close()
		d.pool = nil
	}
}

// isAuthFailure reports whether Postgres rejected the connection's credentials.
func isAuthFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "28P01" || pgErr.Code == "28000")
}
