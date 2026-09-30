package hello

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// openCounter opens pools that never connect, so tests run without Neon, and counts how often DB opens one.
func openCounter(t *testing.T) (*int, func(context.Context) (*pgxpool.Pool, error)) {
	opens := 0
	return &opens, func(ctx context.Context) (*pgxpool.Pool, error) {
		opens++
		pool, err := pgxpool.New(ctx, "postgres://test@127.0.0.1:1/test")
		if err != nil {
			t.Fatal(err)
		}
		return pool, nil
	}
}

func TestRunReopensAfterNeonRejectsThePassword(t *testing.T) {
	opens, open := openCounter(t)
	db := &DB{Open: open}
	var pools []*pgxpool.Pool
	err := db.Run(context.Background(), func(pool *pgxpool.Pool) error {
		pools = append(pools, pool)
		if len(pools) == 1 {
			return fmt.Errorf("connect: %w", &pgconn.PgError{Code: "28P01", Message: "password authentication failed"})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Run after reopening: %v", err)
	}
	if *opens != 2 || len(pools) != 2 || pools[0] == pools[1] {
		t.Fatalf("got %d opens and %d calls, want the second call on a new pool", *opens, len(pools))
	}
}

func TestRunKeepsThePoolForOtherErrors(t *testing.T) {
	opens, open := openCounter(t)
	db := &DB{Open: open}
	queryErr := &pgconn.PgError{Code: "23505", Message: "duplicate key"}
	for range 2 {
		if err := db.Run(context.Background(), func(*pgxpool.Pool) error { return queryErr }); !errors.Is(err, queryErr) {
			t.Fatalf("got %v, want the query's error", err)
		}
	}
	if *opens != 1 {
		t.Fatalf("opened %d pools, want one reused pool", *opens)
	}
}

func TestRunTriesOpeningAgainAfterAFailure(t *testing.T) {
	calls := 0
	db := &DB{Open: func(context.Context) (*pgxpool.Pool, error) {
		calls++
		return nil, errors.New("parameter doesn't hold a connection string yet")
	}}
	for range 2 {
		if err := db.Run(context.Background(), func(*pgxpool.Pool) error { return nil }); err == nil {
			t.Fatal("Run succeeded without a pool")
		}
	}
	if calls != 2 {
		t.Fatalf("tried opening %d times, want every call to try again", calls)
	}
}
