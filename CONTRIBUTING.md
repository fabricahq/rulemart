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

`make db` also creates a `rulemart` database for local development, and the roles infrastructure creates in
production: two group roles that can't log in, `rulemart_catalog_reader` and `rulemart_catalog_writer`, and the
functions' login roles that are their members, `rulemart_web` and `rulemart_worker`. Migrate the database as its
owner, ingest a library as `rulemart_worker`, then serve the pages at <http://127.0.0.1:8080>:

```sh
make migrate
make ingest URL=https://github.com/fabricahq/code-rules-test-library
make web
```

The [Makefile](Makefile) names the local database's connections. `make migrate` connects as the database's owner,
with `LOCAL_DATABASE_URL`, and `make ingest` and `make worker` as `rulemart_worker`, with
`LOCAL_WORKER_DATABASE_URL`, unless you set `DATABASE_URL` or `DATABASE_URL_PARAMETER`; then they never fall back to
the local database. `make ingest` and `make worker` read either one. `make migrate` needs a direct connection: it
refuses a pooled `DATABASE_URL`, and from `DATABASE_URL_PARAMETER` it reads Neon's pooled connection string, as the
functions do, and derives the direct one. `make web` connects with `LOCAL_WEB_DATABASE_URL` as `rulemart_web`. Each
login may only do what its group role's grants allow, as the deployed functions do: `rulemart_web` reads the
catalog, and `rulemart_worker` writes it. Each starts from `LOCAL_DB_HOST` and `LOCAL_DB_PORT`, as does
`RULEMART_TEST_DATABASE_URL`, the server where tests create their databases.

Pages show only the libraries [catalog/vetted.yaml](catalog/vetted.yaml) lists, by code host and the host's
repository ID. To see another library locally, ingest it and add it there, as a vetting pull request would.

Pages name a group whose ID is on Code Rules' canonical group list,
[catalog/canonical-groups.yaml](catalog/canonical-groups.yaml), by the list's name and its icon from
[catalog/group-icons.yaml](catalog/group-icons.yaml), and flag any other group as not canonical. To update the
list, copy the file from a newer Code Rules commit and update the commit and the SHA-256 its test pins. To add an
icon, follow [the icons' README](internal/platform/web/static/icons/README.md).

`cmd/ingest` reads a library's `release/<number>` tags from GitHub and replaces what the catalog stores about it, in
one transaction; running it again on unchanged tags changes nothing. It fetches only the tagged commits, into
memory, and refuses a library that passes any of the limits that
[internal/contexts/catalog/domain/limits.go](internal/contexts/catalog/domain/limits.go) documents: the
references the repository advertises, release tags, the size of a tag, of the fetched packfile, of each object
and of all of them, the number of objects, the size of each file it reads, and the content it holds until the
library is stored. It ingests any public library, vetted or not, so use it to backfill one.
Set `GITHUB_TOKEN` if GitHub's rate limit for anonymous requests gets in the way. Against Neon, set
`DATABASE_URL_PARAMETER` to the SSM parameter holding the worker's connection string,
`/rulemart/prod/worker-database-url`, instead of `DATABASE_URL`, as the worker does. Ingestion parses records with
the copy of Code Rules' parser in [internal/lib/coderules](internal/lib/coderules), so it reads only libraries
released with a Code Rules version that writes the same record format.

`make worker` runs what the deployed worker does every hour, once: for each library `catalog/vetted.yaml`
lists, it lists the release tags without fetching them, and ingests the library when they aren't the ones the
catalog stored. A queue in memory stands in for SQS. Run it twice: the second run finds nothing to ingest.

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

- `cmd/web` serves the pages, through CloudFront on Lambda or as a local HTTP server. `cmd/worker` keeps the vetted
  libraries current: the schedule invokes it to queue one job per vetted library, and the job queue invokes it to
  run each job. Both run on Lambda.
- `cmd/migrate-database` applies schema migrations, in each release or on an operator's machine, and `cmd/ingest`
  ingests a library, on an operator's machine. Neither runs on Lambda.
- `internal/contexts/catalog` owns the catalog, organized by layer within the context, as
  [_internal/decisions.md](_internal/decisions.md) explains:
  - `domain` holds the catalog's values and rules, with no I/O: release history, assembling a library from release
    snapshots within the content budget, addresses such as tags and GitHub URLs, and every ingestion limit.
  - `render` renders rules' Markdown with Rulemart's link rules, within a byte allowance. Assembly takes it as a
    function, so only ingestion links goldmark and chroma, and the web function doesn't.
  - `app` holds the operations: `Ingester` ingests a library, or updates one whose release tags changed, and `Pages`
    reads what the pages show.
  - `source/git` fetches release snapshots, or lists release tags, with go-git, which nothing else uses outside its
    test fixture `source/git/gittest`, and `source/github` looks repositories up in GitHub's API.
  - `store` is the persistence contract, and `store/postgres` implements it, with every catalog query in `queries`
    and sqlc's output in `generated/catalogdb`.
  - `views` holds the plain values pages read.
- `internal/platform` holds shared runtime: `database` owns the connection to Neon, `database/migrate` the
  migrations, `web` the HTTP server, templates, and static files, `logging` the JSON logger every command builds from
  `LOG_LEVEL` and `RULEMART_RELEASE`, and `postgrestest` and `database/databasetest` the test databases.
- `internal/lib/coderules` is the vendored copy of Code Rules' parser.
- `db/migrations` holds the schema as numbered SQL files.
- `catalog` holds the data each release ships: `vetted.yaml`, the vetted libraries; `canonical-groups.yaml`, Code
  Rules' canonical group list, copied unchanged at a pinned commit; and `group-icons.yaml`, the icon for each
  canonical group, which `internal/platform/web/static/icons` vendors with its license.

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
- Grant `rulemart_catalog_reader` what the web function needs from each new table, usually `SELECT` on what the
  pages read, and nothing on tables the pages don't read. Grant `rulemart_catalog_writer` what ingestion writes,
  and nothing more. Never grant to `rulemart_web`, `rulemart_worker`, or another login role: infrastructure owns
  the logins and their memberships, and migrations own the grants, so a login can be replaced or rotated without a
  migration. The site's tests read as `rulemart_web` and ingest as `rulemart_worker`, through their memberships, so
  a missing grant fails them.
- A migration must work with the release that's still running, because the schema changes before the functions
  do. Make a breaking change in two releases: add the new shape first, and remove the old one after nothing uses
  it.

The functions never change the schema. When they first connect, they check that goose has applied the newest
migration in their release, and refuse to use an older database. Each release applies its migrations to production
before it's published, as [Release](#release) describes, so a published release always has its schema.

To apply migrations yourself, to a local database or a Neon branch, run:

```sh
DATABASE_URL='<direct connection string>' make migrate
```

Use Neon's direct connection string, not the pooled one: the migration lock needs a session that the pooler
doesn't keep, so `migrate-database` refuses a pooled host. To try a migration on real data first, run it against a
Neon branch.

## Release

[Release Planner](https://release-planner.fabricahq.com) publishes releases: ask an agent to make one, and it opens
a pull request that adds the release's notes under `_releases/`. Merging the pull request approves the release.
[.release-planner/policy.md](.release-planner/policy.md) says how to choose a version. Nothing else tags or
publishes a release.

Each release is the pair of Lambda functions, built from the release commit:

1. **On the release pull request,** [build-release.yml](.github/workflows/build-release.yml) builds `web.zip` and
   `worker.zip` for `provided.al2023` on arm64 with [lambda-build](https://github.com/fabricahq/lambda-build), as
   [lambda-build.toml](lambda-build.toml) says, plus `SHA256SUMS` and `manifest.json`. It builds twice in a pinned
   container and requires identical ZIPs. Release Planner attests the files.
2. **After the merge,** [migrate-database.yml](.github/workflows/migrate-database.yml), "Migrate the database", applies
   the release commit's migrations to production. Its job runs in the `production` GitHub environment, which only
   `main` can use, and assumes an AWS role that trusts only that environment, through GitHub's OIDC token, so no
   AWS key is stored in GitHub. It reads Neon's pooled connection string from SSM and derives the direct one.
3. **If the migrations succeed,** Release Planner tags the release commit and publishes the files as a GitHub
   release. If they fail, nothing is published: fix the cause, then re-run the failed jobs, or withdraw the release
   as Release Planner's [pre-publish docs](https://release-planner.fabricahq.com/customize/pre-publish/#if-it-fails)
   describe.

Publishing doesn't deploy. A deployment pins a release's tag and its ZIPs' SHA-256 values in Fabrica's
infrastructure repository, so production may still run an older release when the next one migrates.

CI runs `make check`, and builds the release files the same way, on every pull request and every push to main. The
build doesn't depend on the release's version, so CI's files for a commit are byte for byte the ones a release of it
publishes.
`make dist` builds them locally into `dist/`, from `HEAD`'s committed tree, with Docker.

## Engineering rules

Agents and people follow the rules in [.code-rules](.code-rules), as [AGENTS.md](AGENTS.md) explains.
