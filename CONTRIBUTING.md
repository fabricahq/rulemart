# Contributing to Rulemart

## Set up

You need the Go version in [go.mod](go.mod), Docker, and Python 3.11 or later.

```sh
make db     # start Postgres 18 in Docker on 127.0.0.1:55432
make check  # vet, and run every test with the race detector
```

Tests create their own databases on that server and drop them afterward. `make db-stop` removes the container.

## Layout

- `cmd/web` answers through CloudFront and on the schedule, and `cmd/worker` stores queued messages. Both run on
  Lambda.
- `cmd/migrate` applies schema migrations. It runs on an operator's machine, never on Lambda.
- `internal/database` owns the connection to Neon, `internal/hello` the skeleton's messages and their store, and
  `internal/migrate` the migrations.
- `db/migrations` holds the schema as numbered SQL files.

## Schema and migrations

[goose](https://github.com/pressly/goose) manages the schema, and every change is a new migration:

```sh
go tool goose -dir db/migrations -s create add_libraries sql
```

- Keep `db/migrations` for numbered `.sql` files only.
- Migrations are forward-only. Leave the `Down` section as `-- Up-only migration; no rollback defined.` and fix a
  mistake with a new migration. To undo a local experiment, recreate the database.
- Migrations change the schema, not data for testing.
- A migration must work with the release that's still running, because the schema changes before the functions
  do. Make a breaking change in two releases: add the new shape first, and remove the old one after nothing uses
  it.

The functions never change the schema. When they first connect, they check that goose has applied the newest
migration in their release, and refuse to use an older database. So apply migrations before deploying a release
that needs them:

```sh
DATABASE_URL='<Neon direct connection string>' make migrate
```

Use Neon's direct connection string, not the pooled one: the migration lock needs a session that the pooler
doesn't keep, so `migrate` refuses a pooled host. To try a migration on real data first, run it against a Neon
branch.

## Release

`make dist` builds each Lambda function for `provided.al2023` on arm64 and writes `dist/<function>.zip`,
`SHA256SUMS`, and `manifest.json`. The ZIPs are reproducible.

CI runs `make check` and builds the same assets on every push. Pushing a `v*` tag on a commit merged into `main`
publishes them as a GitHub release. A deployment pins each release's SHA-256 values in Fabrica's infrastructure
repository, so publishing a release doesn't deploy it.

## Engineering rules

Agents and people follow the rules in [.code-rules](.code-rules), as [AGENTS.md](AGENTS.md) explains.
