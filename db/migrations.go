// Package db holds Rulemart's Postgres schema as goose migrations, embedded so the release binaries can apply and
// check it.
package db

import "embed"

// Migrations holds the numbered SQL migrations under migrations/, which goose applies in order.
//
//go:embed migrations/*.sql
var Migrations embed.FS
