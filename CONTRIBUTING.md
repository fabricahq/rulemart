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
<http://127.0.0.1:8080>:

```sh
make migrate
make ingest URL=https://github.com/fabricahq/code-rules-test-library
make web
```

The [Makefile](Makefile) names the local database's connections. `make migrate` and `make ingest` connect as the
database's owner, with `LOCAL_DATABASE_URL`, unless you set `DATABASE_URL` or `DATABASE_URL_PARAMETER`; then they
never fall back to the local database. `make ingest` reads either one. `make migrate` needs a direct connection
string in `DATABASE_URL`, so with only `DATABASE_URL_PARAMETER` set it stops and says so. `make web` connects with
`LOCAL_WEB_DATABASE_URL` as `rulemart_web`, which may only read the catalog, as the deployed function does. Each
starts from `LOCAL_DB_HOST` and `LOCAL_DB_PORT`, as does `RULEMART_TEST_DATABASE_URL`, the server where tests create
their databases.

Pages show only the libraries [catalog/vetted.yaml](catalog/vetted.yaml) lists, by code host and the host's
repository ID. To see another library locally, ingest it and add it there, as a vetting pull request would.

`cmd/ingest` reads a library's `release/<number>` tags from GitHub and replaces what the catalog stores about it, in
one transaction; running it again on unchanged tags changes nothing. It fetches only the tagged commits, into
memory, and refuses a library that passes any of the limits that
[internal/contexts/catalog/domain/limits.go](internal/contexts/catalog/domain/limits.go) documents: release tags,
the size of a tag, of the fetched packfile, of each object and of all of them, the number of objects, the size of
each file it reads, and the content it holds until the library is stored.
Set `GITHUB_TOKEN` if GitHub's rate limit for anonymous requests gets in the way. Against Neon, set
`DATABASE_URL_PARAMETER` to the SSM parameter holding the connection string instead of `DATABASE_URL`, as the
functions do. Ingestion parses records with the copy of Code Rules' parser in
[internal/lib/coderules](internal/lib/coderules), so it reads only libraries released with a Code Rules version that
writes the same record format.

## Generated files

sqlc writes the database queries' Go, templ the pages' Go, and Tailwind the stylesheet. Their output is committed,
so building needs none of them, and every generated file says so where it lives:

- sqlc writes the catalog's queries into `internal/contexts/catalog/store/postgres/generated/catalogdb`, with files
  named `*.generated.go`.
- Tailwind writes `internal/platform/web/static/generated/app.css`.
- templ output must stay beside its `.templ` source, because Go needs it in the same package and so the same
  directory. `make generate` renames templ's `x_templ.go` to `x_templ.generated.go`.

[.gitattributes](.gitattributes) marks `generated/` directories and `*.generated.*` files as generated, so GitHub
collapses them in diffs. After changing a query in `internal/contexts/catalog/store/postgres/queries`, a migration,
a `.templ` file, or `internal/platform/web/styles/app.css`, run:

```sh
make generate
```

It deletes the generated files, then runs sqlc and templ as Go tools, and Tailwind as its standalone binary, which it
downloads into `bin/` and checks against the SHA-256 pinned in the [Makefile](Makefile). CI runs
`make check-generated`, which fails when a committed generated file differs from what its sources generate, or is
one they no longer generate, such as a file under an old name.

## Layout

- `cmd/web` serves the pages, through CloudFront on Lambda or as a local HTTP server, and acknowledges the schedule's
  event. `cmd/worker` consumes the job queue, which has no jobs yet. Both run on Lambda.
- `cmd/migrate` applies schema migrations, and `cmd/ingest` ingests a library. They run on an operator's machine,
  never on Lambda.
- `internal/contexts/catalog` owns the catalog, organized by layer within the context, as
  [docs/decisions.md](docs/decisions.md) explains:
  - `domain` holds the catalog's values and rules, with no I/O: release history, assembling a library from release
    snapshots within the content budget, rendering rules' Markdown, addresses such as tags and GitHub URLs, and
    every ingestion limit.
  - `app` holds the operations: `Ingester` ingests a library, and `Pages` reads what the pages show.
  - `source/git` fetches release snapshots with go-git, which nothing else uses outside its test fixture
    `source/git/gittest`, and `source/github` looks repositories up in GitHub's API.
  - `store` is the persistence contract, and `store/postgres` implements it, with every catalog query in `queries`
    and sqlc's output in `generated/catalogdb`.
  - `views` holds the plain values pages read.
- `internal/platform` holds shared runtime: `database` owns the connection to Neon, `migrate` the migrations, `web`
  the HTTP server, templates, and static files, and `postgrestest` and `database/databasetest` the test databases.
- `internal/lib/coderules` is the vendored copy of Code Rules' parser.
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
- Give each catalog table an `id` primary key and keep its natural key, such as a library and a rule's path, as
  a unique constraint. Ingestion upserts on the natural keys, so a row keeps its id for as long as it exists.
- Grant `rulemart_web` what the web function needs from each new table, usually `SELECT` on what the pages read,
  and nothing on tables the pages don't read. The store's page-read tests and the site's end-to-end tests read as
  that role, so a missing grant fails them.
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
