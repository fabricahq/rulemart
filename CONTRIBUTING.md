# Contributing to Rulemart

## Set up

You need the Go version in [go.mod](go.mod), Docker, Python 3.11 or later, and a C compiler for sqlc. There's no
Node.

```sh
make db     # start Postgres 18 in Docker on 127.0.0.1:55432
make check  # vet, and run every test with the race detector
```

Tests create their own databases on that server and drop them afterward. `make db-stop` removes the container.

## Run the site locally

`make db` also creates a `rulemart` database for local development, and the `rulemart_web` role the web function
connects as. Migrate the database and ingest a library as its owner, then serve the pages at
<http://127.0.0.1:8080>; `make web` connects as `rulemart_web`, as the deployed function does:

```sh
export DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/rulemart?sslmode=disable'
make migrate
go run ./cmd/ingest https://github.com/fabricahq/code-rules-test-library
make web
```

Pages show only the libraries [catalog/vetted.yaml](catalog/vetted.yaml) lists, by GitHub repository ID. To see
another library locally, ingest it and add its ID there, as a vetting pull request would.

`cmd/ingest` reads a library's `release/<number>` tags from GitHub and replaces what the catalog stores about it, in
one transaction; running it again on unchanged tags changes nothing. It fetches only the tagged commits, into
memory, and refuses a library whose tags or objects pass the limits in `internal/ingest/bounded.go`. Set
`GITHUB_TOKEN` if GitHub's rate limit for anonymous requests gets in the way. Against Neon, set
`DATABASE_URL_PARAMETER` to the SSM parameter holding the connection string instead of `DATABASE_URL`, as the
functions do. Ingestion parses records with the copy of Code Rules' parser in
[third_party/coderules](third_party/coderules), so it reads only libraries released with a Code Rules version that
writes the same record format.

## Generated files

sqlc writes the database queries' Go, templ the pages' Go, and Tailwind the stylesheet. Their output is committed,
so building needs none of them. After changing a `query.sql`, a migration, a `.templ` file, or
`internal/site/styles/app.css`, run:

```sh
make generate
```

It runs sqlc and templ as Go tools, and Tailwind as its standalone binary, which it downloads into `bin/` and checks
against the SHA-256 pinned in the [Makefile](Makefile). CI runs `make check-generated`, which fails when a committed
file differs from what its sources generate.

## Layout

- `cmd/web` serves the pages, through CloudFront on Lambda or as a local HTTP server, and acknowledges the schedule's
  event. `cmd/worker` consumes the job queue, which has no jobs yet. Both run on Lambda.
- `cmd/migrate` applies schema migrations, and `cmd/ingest` ingests a library. They run on an operator's machine,
  never on Lambda.
- `internal/ingest` reads a library's release tags and writes the catalog, and `internal/site` serves the pages from
  it. Each keeps its queries in `query.sql`.
- `internal/database` owns the connection to Neon, and `internal/migrate` the migrations.
- `db/migrations` holds the schema as numbered SQL files, and `catalog/vetted.yaml` the vetted libraries.

## Schema and migrations

[goose](https://github.com/pressly/goose) manages the schema, and every change is a new migration:

```sh
go tool goose -dir db/migrations -s create add_libraries sql
```

- Keep `db/migrations` for numbered `.sql` files only.
- Migrations are forward-only. Leave the `Down` section as `-- Up-only migration; no rollback defined.` and fix a
  mistake with a new migration. To undo a local experiment, recreate the database.
- Migrations change the schema, not data for testing.
- Grant `rulemart_web` what the web function needs from each new table, usually `SELECT` on what the pages read,
  and nothing on tables the pages don't read. The site's tests read as that role, so a missing grant fails them.
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
