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

// The roles are shared fixtures tests don't change, so a leftover role shaped differently from production's must
// stop the tests rather than make them pass with access rulemart_web won't have.
func TestCheckRoleAcceptsOnlyRolesShapedLikeProductions(t *testing.T) {
	server := Server(t)
	group := "rulemart_check_" + RandomHex(t, 6)
	member := "rulemart_check_" + RandomHex(t, 6)
	powerful := "rulemart_check_" + RandomHex(t, 6)
	loner := "rulemart_check_" + RandomHex(t, 6)
	loginGroup := "rulemart_check_" + RandomHex(t, 6)
	exec(t, server, "CREATE ROLE "+group+" NOLOGIN")
	exec(t, server, "CREATE ROLE "+member+" LOGIN IN ROLE "+group)
	exec(t, server, "CREATE ROLE "+powerful+" LOGIN CREATEDB CREATEROLE IN ROLE "+group)
	exec(t, server, "GRANT pg_read_all_data TO "+powerful)
	exec(t, server, "CREATE ROLE "+loner+" LOGIN")
	exec(t, server, "CREATE ROLE "+loginGroup+" LOGIN")
	t.Cleanup(func() { exec(t, server, "DROP ROLE "+member+", "+powerful+", "+loner+", "+loginGroup+", "+group) })
	conn := connect(t, server)
	ctx := context.Background()

	if err := checkRole(ctx, conn, group, false); err != nil {
		t.Errorf("rejected a plain NOLOGIN group role: %v", err)
	}
	if err := checkRole(ctx, conn, member, true, group); err != nil {
		t.Errorf("rejected a plain LOGIN role that is a member of only its group: %v", err)
	}
	for name, tc := range map[string]struct {
		err  error
		want []string
	}{
		"a login role with attributes and another membership": {
			checkRole(ctx, conn, powerful, true, group), []string{"CREATEDB", "CREATEROLE", "pg_read_all_data", "make db-stop"},
		},
		"a login role missing its group": {checkRole(ctx, conn, loner, true, group), []string{"no role", group}},
		"a group role that can log in":   {checkRole(ctx, conn, loginGroup, false), []string{"can log in"}},
		"a group role that is a member":  {checkRole(ctx, conn, member, false), []string{"can log in", group}},
	} {
		t.Run(name, func(t *testing.T) {
			if tc.err == nil {
				t.Fatal("accepted it")
			}
			for _, want := range tc.want {
				if !strings.Contains(tc.err.Error(), want) {
					t.Errorf("the error %q doesn't mention %s", tc.err, want)
				}
			}
		})
	}
}
