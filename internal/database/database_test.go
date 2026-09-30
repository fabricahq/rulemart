package database

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/fabricahq/rulemart/internal/migrate"
	"github.com/fabricahq/rulemart/internal/testdb"
)

// parameter stands in for the SSM parameter that holds the connection string.
type parameter struct {
	mu    sync.Mutex
	value string
	reads int
	// gate, when set, holds each read until it's closed or the reader's context ends.
	gate chan struct{}
}

func (p *parameter) GetParameter(ctx context.Context, _ *ssm.GetParameterInput, _ ...func(*ssm.Options)) (*ssm.GetParameterOutput, error) {
	p.mu.Lock()
	p.reads++
	value, gate := p.value, p.gate
	p.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return &ssm.GetParameterOutput{Parameter: &types.Parameter{Value: aws.String(value)}}, nil
}

func (p *parameter) set(value string) {
	p.mu.Lock()
	p.value = value
	p.mu.Unlock()
}

func (p *parameter) readCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.reads
}

// appRole creates a login role that can read the migrated database at connString, and returns a function that
// sets its password and returns a connection string for it.
func appRole(t *testing.T, connString string) func(password string) string {
	t.Helper()
	role := "rulemart_app_" + testdb.RandomHex(t, 6)
	ident := pgx.Identifier{role}.Sanitize()
	testdb.Exec(t, connString, "CREATE ROLE "+ident+" LOGIN")
	testdb.Exec(t, connString, "GRANT SELECT ON ALL TABLES IN SCHEMA public TO "+ident)
	t.Cleanup(func() {
		// A test may have dropped the database, and the role's privileges with it.
		var exists bool
		testdb.QueryRow(t, testdb.Server(t), `SELECT exists(SELECT 1 FROM pg_database WHERE datname = '`+databaseName(t, connString)+`')`, &exists)
		if exists {
			testdb.Exec(t, connString, "DROP OWNED BY "+ident)
		}
		testdb.Exec(t, testdb.Server(t), "DROP ROLE "+ident)
	})
	return func(password string) string {
		testdb.Exec(t, connString, "ALTER ROLE "+ident+" PASSWORD "+quote(password))
		return testdb.WithUser(t, connString, role, password)
	}
}

func quote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

func migrated(t *testing.T) string {
	t.Helper()
	connString := testdb.New(t)
	if err := migrate.Up(context.Background(), connString); err != nil {
		t.Fatalf("migrate test database: %v", err)
	}
	return connString
}

func requiredVersion(t *testing.T) int64 {
	t.Helper()
	version, err := migrate.RequiredVersion()
	if err != nil {
		t.Fatal(err)
	}
	return version
}

func newDB(t *testing.T, param *parameter, schemaVersion int64) *DB {
	t.Helper()
	db := New(param, "/test/database-url", schemaVersion)
	t.Cleanup(db.Close)
	return db
}

func selectOne(ctx context.Context) func(*pgxpool.Pool) error {
	return func(pool *pgxpool.Pool) error {
		var one int
		return pool.QueryRow(ctx, "SELECT 1").Scan(&one)
	}
}

// forceNewConnections closes the pool's idle connections, so the next query needs a new one and its password.
func forceNewConnections(db *DB) { db.pool.Load().Reset() }

func TestRunSucceedsWithTheNewPasswordAfterItChanges(t *testing.T) {
	ctx := context.Background()
	withPassword := appRole(t, migrated(t))
	param := &parameter{value: withPassword("first")}
	db := newDB(t, param, requiredVersion(t))
	if err := db.Run(ctx, selectOne(ctx)); err != nil {
		t.Fatalf("query with the first password: %v", err)
	}

	param.set(withPassword("second"))
	forceNewConnections(db)

	if err := db.Run(ctx, selectOne(ctx)); err != nil {
		t.Fatalf("query after the password changed: %v", err)
	}
	if reads := param.readCount(); reads != 2 {
		t.Fatalf("read the parameter %d times, want once at first and once after the rejected password", reads)
	}
}

func TestRunRetriesOnceWhenThePasswordStaysWrong(t *testing.T) {
	ctx := context.Background()
	param := &parameter{value: testdb.WithUser(t, migrated(t), "nobody_"+testdb.RandomHex(t, 4), "wrong")}
	db := newDB(t, param, requiredVersion(t))

	calls := 0
	err := db.Run(ctx, func(*pgxpool.Pool) error { calls++; return nil })
	if err == nil {
		t.Fatal("Run succeeded with a password Postgres rejects")
	}
	if reads := param.readCount(); reads != 2 || calls != 0 {
		t.Fatalf("read the parameter %d times and called fn %d times, want 2 reads and no calls", reads, calls)
	}
}

func TestRunConnectsOnceTheParameterHoldsAConnectionString(t *testing.T) {
	ctx := context.Background()
	param := &parameter{value: "REPLACE_IN_AWS_CONSOLE"}
	db := newDB(t, param, requiredVersion(t))
	if err := db.Run(ctx, selectOne(ctx)); err == nil || !strings.Contains(err.Error(), "doesn't hold a Postgres connection string yet") {
		t.Fatalf("got %v, want an error saying the parameter holds no connection string yet", err)
	}

	param.set(migrated(t))

	if err := db.Run(ctx, selectOne(ctx)); err != nil {
		t.Fatalf("query after the parameter was set: %v", err)
	}
}

// Function instances used to create the schema when they connected, and concurrent cold starts raced. They now
// check the schema and refuse to change it.
func TestRunRefusesADatabaseWithoutTheSchemaTheReleaseNeeds(t *testing.T) {
	ctx := context.Background()
	for name, tc := range map[string]struct {
		migrate       bool
		schemaVersion func(required int64) int64
	}{
		"no migrations applied":           {migrate: false, schemaVersion: func(required int64) int64 { return required }},
		"release needs a newer migration": {migrate: true, schemaVersion: func(required int64) int64 { return required + 1 }},
	} {
		t.Run(name, func(t *testing.T) {
			connString := testdb.New(t)
			if tc.migrate {
				if err := migrate.Up(ctx, connString); err != nil {
					t.Fatal(err)
				}
			}
			db := newDB(t, &parameter{value: connString}, tc.schemaVersion(requiredVersion(t)))

			err := db.Run(ctx, selectOne(ctx))

			if err == nil || !strings.Contains(err.Error(), "run the migrate command") {
				t.Fatalf("got %v, want an error asking to run the migrate command", err)
			}
			var tables int
			testdb.QueryRow(t, connString, `SELECT count(*) FROM pg_tables WHERE tablename = 'hello_messages'`, &tables)
			if want := map[bool]int{false: 0, true: 1}[tc.migrate]; tables != want {
				t.Fatalf("found %d hello_messages tables, want %d: opening must not change the schema", tables, want)
			}
		})
	}
}

func TestRunStopsWaitingForAnotherCallersOpenWhenItsContextEnds(t *testing.T) {
	param := &parameter{value: "postgres://unused", gate: make(chan struct{})}
	db := newDB(t, param, 1)
	opening := make(chan struct{})
	go func() {
		close(opening)
		_ = db.Run(context.Background(), func(*pgxpool.Pool) error { return nil })
	}()
	<-opening
	for param.readCount() == 0 {
		time.Sleep(time.Millisecond)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	err := db.Run(ctx, func(*pgxpool.Pool) error { return nil })
	waited := time.Since(start)
	close(param.gate)

	if err == nil || !strings.Contains(err.Error(), "context deadline exceeded") {
		t.Fatalf("got %v, want the caller's deadline", err)
	}
	if waited > time.Second {
		t.Fatalf("waited %v behind another caller's open, want about the 50ms deadline", waited)
	}
}

// The previous pool was replaced after a rejected password, which closed it under callers that were still using it.
func TestRunKeepsAPausedCallersPoolUsableWhileAnotherRecoversFromAChangedPassword(t *testing.T) {
	ctx := context.Background()
	withPassword := appRole(t, migrated(t))
	param := &parameter{value: withPassword("first")}
	db := newDB(t, param, requiredVersion(t))
	if err := db.Run(ctx, selectOne(ctx)); err != nil {
		t.Fatal(err)
	}

	paused, resume := make(chan struct{}), make(chan struct{})
	pausedResult := make(chan error, 1)
	go func() {
		pausedResult <- db.Run(ctx, func(pool *pgxpool.Pool) error {
			close(paused)
			<-resume
			return selectOne(ctx)(pool)
		})
	}()
	<-paused

	param.set(withPassword("second"))
	forceNewConnections(db)
	if err := db.Run(ctx, selectOne(ctx)); err != nil {
		t.Fatalf("recover from the changed password: %v", err)
	}
	close(resume)

	if err := <-pausedResult; err != nil {
		t.Fatalf("the paused caller failed after another caller recovered: %v", err)
	}
}

// A connection string that starts with postgres but can't be used stayed cached, so correcting the parameter didn't
// help until the function was replaced.
func TestRunRereadsTheParameterAfterAConnectionStringThatDoesNotWork(t *testing.T) {
	ctx := context.Background()
	for name, broken := range map[string]string{
		"malformed":       "postgres://%zz",
		"host refuses it": "postgres://postgres:postgres@127.0.0.1:1/postgres?sslmode=disable&connect_timeout=2",
	} {
		t.Run(name, func(t *testing.T) {
			param := &parameter{value: broken}
			db := newDB(t, param, requiredVersion(t))
			if err := db.Run(ctx, selectOne(ctx)); err == nil {
				t.Fatal("Run succeeded with a connection string that can't work")
			}

			param.set(migrated(t))

			if err := db.Run(ctx, selectOne(ctx)); err != nil {
				t.Fatalf("query after the parameter was corrected: %v", err)
			}
		})
	}
}

// pgx runs BeforeConnect with a context detached from the caller, so a refresh that never answered held the caller,
// and the next ones, indefinitely.
func TestRunGivesUpOnAPasswordRefreshThatStalls(t *testing.T) {
	ctx := context.Background()
	withPassword := appRole(t, migrated(t))
	param := &parameter{value: withPassword("first")}
	db := newDB(t, param, requiredVersion(t))
	db.parameterTimeout = 100 * time.Millisecond
	if err := db.Run(ctx, selectOne(ctx)); err != nil {
		t.Fatal(err)
	}

	second := withPassword("second")
	param.mu.Lock()
	param.value, param.gate = second, make(chan struct{})
	param.mu.Unlock()
	forceNewConnections(db)

	result := make(chan error, 1)
	start := time.Now()
	go func() { result <- db.Run(ctx, selectOne(ctx)) }()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("Run succeeded while the parameter couldn't be read")
		}
	case <-time.After(5 * time.Second):
		close(param.gate)
		t.Fatalf("Run was still waiting on the stalled refresh after %v", time.Since(start))
	}

	param.mu.Lock()
	param.gate = nil
	param.mu.Unlock()
	if err := db.Run(ctx, selectOne(ctx)); err != nil {
		t.Fatalf("query after the parameter could be read again: %v", err)
	}
}

func currentDatabase(ctx context.Context, name *string) func(*pgxpool.Pool) error {
	return func(pool *pgxpool.Pool) error { return pool.QueryRow(ctx, "SELECT current_database()").Scan(name) }
}

// databaseName returns the database a connection string names.
func databaseName(t *testing.T, connString string) string {
	t.Helper()
	config, err := pgx.ParseConfig(connString)
	if err != nil {
		t.Fatal(err)
	}
	return config.Database
}

// New connections used to keep the host, port, and database the pool first opened with, and take only the user and
// password from the parameter, so a warm function couldn't follow the connection string to another database.
func TestRunFollowsTheParameterToAnotherDatabase(t *testing.T) {
	ctx := context.Background()
	for name, cutover := range map[string]func(t *testing.T, oldDB string){
		"the old database rejects the password": func(*testing.T, string) {},
		"the old database no longer exists": func(t *testing.T, oldDB string) {
			testdb.Exec(t, testdb.Server(t), "DROP DATABASE "+pgx.Identifier{databaseName(t, oldDB)}.Sanitize()+" WITH (FORCE)")
		},
	} {
		t.Run(name, func(t *testing.T) {
			oldDB, newDB := migrated(t), migrated(t)
			oldPassword, newPassword := appRole(t, oldDB), appRole(t, newDB)
			param := &parameter{value: oldPassword("first")}
			db := New(param, "/test/database-url", requiredVersion(t))
			t.Cleanup(db.Close)
			var got string
			if err := db.Run(ctx, currentDatabase(ctx, &got)); err != nil || got != databaseName(t, oldDB) {
				t.Fatalf("first query reached %q, err %v; want %q", got, err, databaseName(t, oldDB))
			}

			oldPassword("retired")
			param.set(newPassword("second"))
			forceNewConnections(db)
			cutover(t, oldDB)

			if err := db.Run(ctx, currentDatabase(ctx, &got)); err != nil {
				t.Fatalf("query after the parameter moved: %v", err)
			}
			if want := databaseName(t, newDB); got != want {
				t.Fatalf("reached %q after the parameter moved, want %q", got, want)
			}
		})
	}
}
