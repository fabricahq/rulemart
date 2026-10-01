package testdb

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// pgx applies user and password from the query after the userinfo, so a rewrite that kept them would still
// connect as the original user.
func TestWithUserReplacesTheCredentialsPgxUses(t *testing.T) {
	for name, connString := range map[string]string{
		"credentials in the userinfo": "postgres://owner:secret@127.0.0.1:55432/db?sslmode=disable",
		"credentials in the query":    "postgres://127.0.0.1:55432/db?user=owner&password=secret&sslmode=disable",
		"credentials in both":         "postgres://owner:secret@127.0.0.1:55432/db?user=owner&password=secret&sslmode=disable",
	} {
		t.Run(name, func(t *testing.T) {
			config, err := pgx.ParseConfig(WithUser(t, connString, "rulemart_web", "web-password"))
			if err != nil {
				t.Fatal(err)
			}
			if config.User != "rulemart_web" || config.Password != "web-password" {
				t.Fatalf("connects as %q with password %q", config.User, config.Password)
			}
			if config.Database != "db" || config.TLSConfig != nil {
				t.Fatalf("lost the database or sslmode: %+v", config.Config)
			}
		})
	}
}

// The web role is a shared fixture tests don't change, so a leftover role with more power than production's must
// stop the tests rather than make them pass with access rulemart_web won't have.
func TestCheckRoleRejectsARoleWithMoreThanLogin(t *testing.T) {
	server := Server(t)
	plain := "rulemart_check_" + RandomHex(t, 6)
	powerful := "rulemart_check_" + RandomHex(t, 6)
	exec(t, server, "CREATE ROLE "+plain+" LOGIN")
	exec(t, server, "CREATE ROLE "+powerful+" LOGIN CREATEDB CREATEROLE")
	exec(t, server, "GRANT pg_read_all_data TO "+powerful)
	t.Cleanup(func() { exec(t, server, "DROP ROLE "+plain+", "+powerful) })
	conn := connect(t, server)

	if err := checkRole(context.Background(), conn, plain); err != nil {
		t.Fatalf("rejected a plain LOGIN role: %v", err)
	}
	err := checkRole(context.Background(), conn, powerful)
	if err == nil {
		t.Fatal("accepted a role with CREATEDB, CREATEROLE, and a membership")
	}
	for _, want := range []string{"CREATEDB", "CREATEROLE", "pg_read_all_data", "make db-stop"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the error %q doesn't mention %s", err, want)
		}
	}
}
