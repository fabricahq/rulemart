// Package database gives each Rulemart function one long-lived Postgres pool whose connection string lives in an
// SSM SecureString parameter.
//
// The pool opens on first use and is never closed while callers use it. Each new connection reads the current
// password, so a password changed in Neon and in the parameter takes effect without replacing the function: when
// Postgres rejects a password, Run reads the parameter again and retries once. Connections already open keep working,
// because changing a Postgres password doesn't end existing sessions.
package database

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// maxConns bounds each function instance's pool. An instance handles one invocation at a time, so it needs few
// connections; across the fleet, connections grow with concurrency, which Neon's pooler absorbs.
const maxConns = 2

// openTimeout bounds opening the pool, which reads the parameter and checks the schema.
const openTimeout = 10 * time.Second

// defaultParameterTimeout bounds each read of the parameter. pgx runs BeforeConnect with a context detached from the
// caller, so without it a stalled read would hold every new connection.
const defaultParameterTimeout = 5 * time.Second

// ParameterReader is the part of the SSM client that reads the connection string.
type ParameterReader interface {
	GetParameter(context.Context, *ssm.GetParameterInput, ...func(*ssm.Options)) (*ssm.GetParameterOutput, error)
}

// DB is a function's Postgres pool. Its methods are safe for concurrent use.
type DB struct {
	parameters    ParameterReader
	parameterName string
	// schemaVersion is the newest migration this release needs; the pool doesn't open until the database has it.
	schemaVersion int64
	// parameterTimeout bounds each read of the parameter, including the wait for another reader.
	parameterTimeout time.Duration

	pool atomic.Pointer[pgxpool.Pool]
	// opening admits one caller at a time to open the pool. Callers wait for it with their own context, so a canceled
	// caller stops waiting at once.
	opening chan struct{}
	// reading admits one caller at a time to read the parameter, waited for the same way.
	reading chan struct{}

	mu sync.Mutex
	// connString is the parameter's last value. It's empty until the first read, and empty again after Postgres
	// rejects its password, so the next connection reads the parameter again.
	connString string
}

// New returns a DB for the connection string in the SSM parameter parameterName. It reads nothing until first used.
func New(parameters ParameterReader, parameterName string, schemaVersion int64) *DB {
	return &DB{
		parameters:    parameters,
		parameterName: parameterName,
		schemaVersion: schemaVersion,
		// A test shortens it.
		parameterTimeout: defaultParameterTimeout,
		opening:          make(chan struct{}, 1),
		reading:          make(chan struct{}, 1),
	}
}

// Run calls fn with the pool, opening it first if needed. When Postgres rejects the password, Run reads the
// parameter again and makes one more attempt, so fn must be safe to repeat. Run returns fn's error unchanged.
func (d *DB) Run(ctx context.Context, fn func(*pgxpool.Pool) error) error {
	rejected, err := d.attempt(ctx, fn)
	if !rejected {
		return err
	}
	d.forgetConnString()
	_, err = d.attempt(ctx, fn)
	return err
}

// Close closes the pool. Call it only after every Run has returned, such as at the end of a test.
func (d *DB) Close() {
	if pool := d.pool.Swap(nil); pool != nil {
		pool.Close()
	}
}

// attempt runs fn once and reports whether its error, or opening the pool, failed because Postgres rejected the
// password.
func (d *DB) attempt(ctx context.Context, fn func(*pgxpool.Pool) error) (rejected bool, err error) {
	pool, err := d.get(ctx)
	if err != nil {
		return isAuthFailure(err), fmt.Errorf("open database parameter=%q: %v", d.parameterName, err)
	}
	err = fn(pool)
	return isAuthFailure(err), err
}

// get returns the open pool, opening it if no earlier attempt succeeded. A failed open leaves nothing behind, so
// the next call tries again.
func (d *DB) get(ctx context.Context) (*pgxpool.Pool, error) {
	if pool := d.pool.Load(); pool != nil {
		return pool, nil
	}
	if err := acquire(ctx, d.opening); err != nil {
		return nil, err
	}
	defer release(d.opening)
	if pool := d.pool.Load(); pool != nil {
		return pool, nil
	}
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	pool, err := d.open(ctx)
	if err != nil {
		// Whatever the parameter held didn't open a pool, so read it again next time: it may have been corrected.
		d.forgetConnString()
		return nil, err
	}
	d.pool.Store(pool)
	return pool, nil
}

// open connects a new pool and returns it once the database has the schema this release needs. It closes the pool
// on failure, before any caller has seen it.
func (d *DB) open(ctx context.Context) (*pgxpool.Pool, error) {
	connString, err := d.readConnString(ctx)
	if err != nil {
		return nil, err
	}
	config, err := pgxpool.ParseConfig(connString)
	if err != nil {
		// The parse error would repeat the connection string, so leave it out.
		return nil, errors.New("the parameter doesn't hold a valid Postgres connection string")
	}
	config.MaxConns = maxConns
	config.BeforeConnect = d.useCurrentPassword
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}
	if err := checkSchema(ctx, pool, d.schemaVersion); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// useCurrentPassword gives a new connection the password in the parameter's latest value.
func (d *DB) useCurrentPassword(ctx context.Context, config *pgx.ConnConfig) error {
	connString, err := d.readConnString(ctx)
	if err != nil {
		return err
	}
	current, err := pgx.ParseConfig(connString)
	if err != nil {
		return errors.New("the parameter doesn't hold a valid Postgres connection string")
	}
	config.User, config.Password = current.User, current.Password
	return nil
}

// readConnString returns the parameter's value, reading it from SSM unless an earlier read is still trusted. It only
// caches a value that parses as a Postgres connection string.
func (d *DB) readConnString(ctx context.Context) (string, error) {
	if connString := d.cachedConnString(); connString != "" {
		return connString, nil
	}
	ctx, cancel := context.WithTimeout(ctx, d.parameterTimeout)
	defer cancel()
	if err := acquire(ctx, d.reading); err != nil {
		return "", err
	}
	defer release(d.reading)
	if connString := d.cachedConnString(); connString != "" {
		return connString, nil
	}
	out, err := d.parameters.GetParameter(ctx, &ssm.GetParameterInput{Name: aws.String(d.parameterName), WithDecryption: aws.Bool(true)})
	if err != nil {
		return "", fmt.Errorf("read parameter: %v", err)
	}
	connString := aws.ToString(out.Parameter.Value)
	if !strings.HasPrefix(connString, "postgres") {
		return "", errors.New("the parameter doesn't hold a Postgres connection string yet")
	}
	if _, err := pgx.ParseConfig(connString); err != nil {
		// The parse error would repeat the connection string, so leave it out.
		return "", errors.New("the parameter doesn't hold a valid Postgres connection string")
	}
	d.mu.Lock()
	d.connString = connString
	d.mu.Unlock()
	return connString, nil
}

func (d *DB) cachedConnString() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.connString
}

func (d *DB) forgetConnString() {
	d.mu.Lock()
	d.connString = ""
	d.mu.Unlock()
}

// checkSchema returns an error unless goose has applied migration version required or later. It only reads.
func checkSchema(ctx context.Context, pool *pgxpool.Pool, required int64) error {
	var applied int64
	err := pool.QueryRow(ctx, `SELECT coalesce(max(version_id), 0) FROM goose_db_version WHERE is_applied`).Scan(&applied)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "42P01" { // undefined_table: no migration has run.
		err = nil
	}
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if applied < required {
		return fmt.Errorf("the database schema is at version %d, and this release needs %d: run the migrate command", applied, required)
	}
	return nil
}

// isAuthFailure reports whether Postgres rejected a connection's credentials.
func isAuthFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && (pgErr.Code == "28P01" || pgErr.Code == "28000")
}

// acquire takes the one slot in gate, or gives up when ctx ends.
func acquire(ctx context.Context, gate chan struct{}) error {
	select {
	case gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func release(gate chan struct{}) { <-gate }
