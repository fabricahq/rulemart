package migrate

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/fabricahq/rulemart/internal/platform/postgrestest"
)

// No published release created the cart_items 00014 drops, so a database that holds a cart item came from an
// unreleased build. Migrating it stops before losing the item, and goes on once someone has removed it, leaving no
// table behind.
func TestDroppingCartItemsRefusesADatabaseWhoseCartsHoldRows(t *testing.T) {
	ctx := context.Background()
	connString := postgrestest.New(t)
	if _, err := up(ctx, connString, migrationsThrough(t, 13)); err != nil {
		t.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, connString)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(ctx)
	insertCurrentRule(t, ctx, conn, "Return errors", "When handling failures.", "---\ntitle: Return errors\n---\nWrap errors.")
	exec := func(sql string) {
		t.Helper()
		if _, err := conn.Exec(ctx, sql); err != nil {
			t.Fatalf("run %q: %v", sql, err)
		}
	}
	exec(`INSERT INTO accounts (github_user_id, github_login, avatar_url) VALUES (1, 'octocat', '')`)
	exec(`INSERT INTO cart_items (account_id, library_id, kind, path) SELECT a.id, l.id, 'rule', 'techs/go/x' FROM accounts a, libraries l`)

	result, err := Up(ctx, connString)

	if err == nil || !strings.Contains(err.Error(), "cart_items holds rows") {
		t.Fatalf("got %v, want a refusal saying cart_items holds rows", err)
	}
	if len(result.Applied) != 0 {
		t.Errorf("reported %v applied, want none", appliedVersions(result))
	}
	var version, items int64
	postgrestest.QueryRow(t, connString, "SELECT max(version_id) FROM goose_db_version", &version)
	postgrestest.QueryRow(t, connString, "SELECT count(*) FROM cart_items", &items)
	if version != 13 || items != 1 {
		t.Fatalf("the database is at version %d with %d cart items, want 13 with its item", version, items)
	}

	exec(`DELETE FROM cart_items`)
	if _, err := Up(ctx, connString); err != nil {
		t.Fatalf("migrating once the carts are empty: %v", err)
	}
	var table *string
	postgrestest.QueryRow(t, connString, "SELECT to_regclass('cart_items')::text", &table)
	if table != nil {
		t.Errorf("cart_items is still there after migrating")
	}
}
