package testdb

import (
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
