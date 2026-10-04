# Contributing to Rulemart

## Set up

You need the Go version in [go.mod](go.mod), Docker, Python 3.11 or later, and a C compiler for sqlc. Node is
optional and needs no packages: with it, `make check` also runs the site scripts' tests, which CI always runs. Bash
4 or later, such as Homebrew's, is optional too: with it, `make check` also checks that the conformance audit runs a
browser of its own.

```sh
make db     # start Postgres 18 in Docker on 127.0.0.1:55432
make check  # vet, run every test with the race detector, and the scripts' tests with Node when it's installed
```

Tests create their own databases on that server and drop them afterward. `make db-stop` removes the container.

## Run the site locally

`make db` also creates a `rulemart` database for local development, and the roles infrastructure creates in
production: three group roles that can't log in, `rulemart_catalog_reader`, `rulemart_accounts_writer`, and
`rulemart_catalog_writer`, and the functions' login roles that are their members: `rulemart_web` of the first two,
and `rulemart_worker` of the third. Migrate the database as its
owner, ingest a library as `rulemart_worker`, then serve the pages at <http://127.0.0.1:8080>:

```sh
make migrate
make ingest URL=https://github.com/fabricahq/code-rules-test-library
make ingest URL=https://github.com/fabricahq/public-rules
make web
```

To try signed-in pages, run `make web-dev` instead of `make web`: it builds the site with the `rulemartdev` tag, whose
sign-in page offers two test users, `test_user` and `test_user_2`, so you can sign in without GitHub. Without GitHub
sign-in, that build also serves a fake GitHub in memory,
[githubtest's `DevFake`](internal/contexts/accounts/github/githubtest/dev.go), so the dashboard at
<http://127.0.0.1:8080/me> has something to show: `test_user` belongs to the fabricahq organization, which publishes
the two libraries below, so `/me/add` lists them, and two projects import them at older rule versions, so they have
updates waiting. The fake lists only public libraries real GitHub has, since the worker reads real GitHub. Continue to GitHub on `/me/private` installs the fake's GitHub App at once, which adds a private library
and a private project. Nothing runs the worker locally, so a library added at `/me/add` stays in progress on
`/me/add/run` until you run `make worker`, in another terminal, which checks it once, against the same database when
`make web-dev` uses the local `rulemart` one; for another, give `make worker` that database too, such as
`LOCAL_WORKER_DATABASE_URL='postgres://rulemart_worker:rulemart-worker-local@127.0.0.1:55432/rulemart_dev?sslmode=disable' make worker`.
After three minutes the page says
the check is taking longer than usual, as it does when a deployed check's job is late. Each session seals its GitHub token with a key the build makes at start, so
after a restart the dashboard asks you to sign in again before it reads the fake again.
Release builds never have that tag, and a test checks that the web function's release binary has no dev sign-in. To
sign in with GitHub itself, create an OAuth app whose callback URL is `http://127.0.0.1/account/github/callback`, and
run `GITHUB_CLIENT_ID=<its client ID> GITHUB_CLIENT_SECRET=<its secret> TOKEN_KEY=$(openssl rand -base64 32) make web`:
each session keeps the visitor's GitHub token sealed with `TOKEN_KEY`, so a new key signs every browser's GitHub
token out of reach until it signs in again. The dashboard then reads your own GitHub account. To read private
repositories too, through a GitHub App of your own whose setup URL is `http://127.0.0.1:8080/me/github/installed`, add
`GITHUB_APP_ID`, `GITHUB_APP_CLIENT_ID`, `GITHUB_APP_SLUG`, `GITHUB_APP_PRIVATE_KEY="$(cat <its .pem>)"`, and
`GITHUB_APP_WEBHOOK_SECRET`; set all five or none. Its webhook, at `/account/github/webhook`, needs a public address,
such as a tunnel, so locally an uninstalled app is forgotten at the next read instead.

Deployed, each secret comes from an SSM SecureString that the variable of the same name with `_PARAMETER` names, as
[cmd/web](cmd/web/main.go) documents: `GITHUB_CLIENT_SECRET_PARAMETER` names `/rulemart/prod/github-client-secret`,
`TOKEN_KEY_PARAMETER` `/rulemart/prod/token-key`, 32 random bytes in base64, `GITHUB_APP_PRIVATE_KEY_PARAMETER`
`/rulemart/prod/github-app-key`, and `GITHUB_APP_WEBHOOK_SECRET_PARAMETER` `/rulemart/prod/github-app-webhook-secret`.
`GITHUB_CLIENT_ID`, `GITHUB_APP_ID`, `GITHUB_APP_CLIENT_ID`, and `GITHUB_APP_SLUG` aren't secret. Rulemart's cookies are `Secure`, which Chrome accepts from
`http://127.0.0.1`, as it would from no other plain-HTTP host but `localhost`.

Those are the two libraries [catalog/vetted.yaml](catalog/vetted.yaml) vets, so every page has more than one library
to show: `/browse/techs` and a group such as `/g/techs/go` across both, and `/search?q=retry`. The test library has
six releases to compare: its Library releases tab, `/fabricahq/code-rules-test-library?tab=releases`, compares two of
them, and a rule's Versions tab compares two of its versions. Its `techs/go/use-contexts` rule has assets of its own
and shared ones, such as `/fabricahq/code-rules-test-library/techs/go/use-contexts/assets/example.go`. Run `make ingest` again
to bring a library up to date, or `make worker` to update every vetted one. To keep the local `rulemart` database for
other work, point the commands at another database on the same server, such as
`LOCAL_DATABASE_URL='postgres://postgres:postgres@127.0.0.1:55432/rulemart_dev?sslmode=disable' make migrate`, after
creating it with `docker exec rulemart-postgres createdb -U postgres rulemart_dev`; `make ingest` and `make web` take
`LOCAL_WORKER_DATABASE_URL` and `LOCAL_WEB_DATABASE_URL` the same way.

The [Makefile](Makefile) names the local database's connections. `make migrate` connects as the database's owner,
with `LOCAL_DATABASE_URL`, and `make ingest` and `make worker` as `rulemart_worker`, with
`LOCAL_WORKER_DATABASE_URL`, unless you set `DATABASE_URL` or `DATABASE_URL_PARAMETER`; then they never fall back to
the local database. `make ingest` and `make worker` read either one. `make migrate` needs a direct connection: it
refuses a pooled `DATABASE_URL`, and from `DATABASE_URL_PARAMETER` it reads Neon's pooled connection string, as the
functions do, and derives the direct one. `make web` connects with `LOCAL_WEB_DATABASE_URL` as `rulemart_web`. Each
login may only do what its group roles' grants allow, as the deployed functions do: `rulemart_web` reads the
catalog and writes accounts, sessions, GitHub snapshots and installations, listings, and stars, and `rulemart_worker` writes the catalog and records listings' checks. Each starts from `LOCAL_DB_HOST` and `LOCAL_DB_PORT`, as does
`RULEMART_TEST_DATABASE_URL`, the server where tests create their databases.

`make web-dev` names <http://127.0.0.1:8080>, the loopback address it serves at, as the pages' canonical origin, so
robots.txt names <http://127.0.0.1:8080/sitemap.xml> and the social card points there too; `make web` names none, so
it has no sitemap, unless `RULEMART_BASE_URL` names one, such as `https://rulemart.example`. To try Cloudflare
Web Analytics, set `CLOUDFLARE_WEB_ANALYTICS_TOKEN` to a site's token: pages then load its beacon, and the content
security policy allows it. `/about` and `/privacy` describe Rulemart and what it keeps; when a change alters what
Rulemart keeps, logs, or shares, update `/privacy` in `internal/platform/web/about.templ` with it.

The `prototype` branch's click-through mock is the spec for every page, as
[_internal/realignment.md](_internal/realignment.md) says. After changing a page, compare it with the prototype. With
[chrome-devtools-axi](https://github.com/kunchenguid/chrome-devtools-axi) installed, the prototype served as that file
says, and the site served with `make web-dev`, run:

```sh
_internal/audit/conformance.sh http://127.0.0.1:8766 http://127.0.0.1:8080 <directory>
```

The script's header says what it shoots and how to limit it.

Pages show the libraries [catalog/vetted.yaml](catalog/vetted.yaml) lists, by code host and the host's
repository ID, and on their own pages, under a warning, the ones a listing names. To see another library locally,
list it: sign in with `make web-dev`, add its repository at <http://127.0.0.1:8080/me/add>, and run `make worker`,
which checks the listing, since locally nothing sends its job at once. Or ingest it and add it to `vetted.yaml`, as
a vetting pull request would. To remove an abusive listing, delete its row as the database's owner:
`DELETE FROM listings WHERE id = <id>`.

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
and of all of them, the number of objects, the size of each file it reads, the content it holds until the
library is stored, and the rules' assets it lists and the bytes it keeps of them. It ingests any public library, vetted or not, so use it to backfill one.
Set `GITHUB_TOKEN` if GitHub's rate limit for anonymous requests gets in the way. Against Neon, set
`DATABASE_URL_PARAMETER` to the SSM parameter holding the worker's connection string,
`/rulemart/prod/worker-database-url`, instead of `DATABASE_URL`, as the worker does. Ingestion parses records with
the copy of Code Rules' parser in [internal/lib/coderules](internal/lib/coderules), so it reads only libraries
released with a Code Rules version that writes the same record format.

`make worker` runs what the deployed worker does every hour, once: for each library `catalog/vetted.yaml`
lists, and each listing to check, it lists the release tags without fetching them, and ingests the library when they
aren't the ones the catalog stored, or when a release before every rule version kept its content, or before rules'
assets and tags were stored, stored it. A new
listing is looked up on GitHub first, and a listing whose repository fails records why, for its lister. A queue in
memory stands in for SQS. Run it twice: the second run finds nothing to ingest.

## Generated files

sqlc writes the database queries' Go, templ the pages' Go, and Tailwind the stylesheet. Their output is committed,
so building needs none of them, and every generated file says so where it lives:

- sqlc writes each context's queries beside its store: the catalog's into
  `internal/contexts/catalog/store/postgres/generated/catalogdb`, and accounts' into
  `internal/contexts/accounts/store/postgres/generated/accountsdb`, with files named `*.generated.go`.
- Tailwind writes `internal/platform/web/static/generated/app.css`.
- templ output must stay beside its `.templ` source, because Go needs it in the same package and so the same
  directory. `make generate` renames templ's `x_templ.go` to `x_templ.generated.go`.

[.gitattributes](.gitattributes) marks `generated/` directories and `*.generated.*` files as generated, so GitHub
collapses them in diffs. After changing a query in a context's `store/postgres/queries`, a migration,
a `.templ` file, or `internal/platform/web/styles/app.css`, run:

```sh
make generate
```

It deletes the generated files, then runs sqlc and templ as Go tools, and Tailwind as its standalone binary, which it
downloads into `bin/` and checks against the SHA-256 pinned in the [Makefile](Makefile). CI runs
`make check-generated`, which fails when a committed generated file differs from what its sources generate, or is
one they no longer generate, such as a file under an old name.

## Layout

Rulemart runs on AWS and Neon. Fabrica's private infrastructure repositories define and deploy the AWS resources.

```text
CloudFront -> web Lambda (Function URL) -> Neon Postgres
EventBridge schedule -> worker Lambda -> SQS, one job per vetted library or listing -> worker Lambda -> Neon Postgres
web Lambda -> SQS, one job per new listing
```

- `cmd/web` serves the pages, through CloudFront on Lambda or as a local HTTP server, and queues each new listing's
  check. `cmd/worker` keeps the vetted and listed libraries current: the schedule invokes it to queue one job per
  vetted library and per listing to check, and the job queue invokes it to run each job. Both run on Lambda.
- `cmd/migrate-database` applies schema migrations, in each release or on an operator's machine, and `cmd/ingest`
  ingests a library, on an operator's machine. Neither runs on Lambda.
- `internal/contexts/catalog` owns the catalog, organized by layer within the context, as
  [_internal/decisions.md](_internal/decisions.md) explains:
  - `domain` holds the catalog's values and rules, with no I/O: release history, assembling a library from release
    snapshots within the content budget, addresses such as tags and GitHub URLs, where a rule's links and images
    lead, every ingestion limit, and the
    cart's keys and the Code Rules commands and prompt its checkout writes, whose golden files are in
    `domain/testdata`; run `go test ./internal/contexts/catalog/domain -update` after changing them, and review the
    diff.
  - `render` renders rules' and their assets' Markdown, with links where the domain says, and their text files as
    highlighted code, within a byte allowance. Assembly takes it as an interface, so only ingestion links goldmark
    and chroma, and the web function doesn't.
  - `app` holds the operations: `Ingester` ingests a library, updates one whose release tags changed, or checks a
    listing; `Listings` lists libraries for accounts; `Stars` stars rules; `Carts` checks out the carts browsers keep;
    and `Pages` reads what the pages show.
  - `jobs` encodes and decodes the jobs queue's messages.
  - `source/git` fetches release snapshots, or lists release tags, with go-git, which nothing else uses outside its
    test fixture `source/git/gittest`, and `source/github` looks repositories up in GitHub's API.
  - `store` is the persistence contract, and `store/postgres` implements it, with every catalog query in `queries`
    and sqlc's output in `generated/catalogdb`.
  - `views` holds the plain values pages read.
- `internal/contexts/accounts` owns accounts and sessions, and what Rulemart reads of each visitor's GitHub account,
  in the same layout: `domain` holds identities, accounts, session tokens, the key that seals each session's GitHub
  token, the GitHub snapshot, and the parser of a project's provenance file; `app` signs visitors in and out, reads
  their GitHub accounts into snapshots, and records the GitHub App's installations; `github` signs them in with
  GitHub's OAuth app, reads GitHub's REST API, acts as the GitHub App, and checks its webhook's deliveries, and
  `github/githubtest` fakes that API for tests and the local build; and `store` and `store/postgres` keep all of it,
  with sqlc's output in `generated/accountsdb`.
- `internal/platform` holds shared runtime: `database` owns the connection to Neon, `database/migrate` the
  migrations, `web` the HTTP server, templates, and static files, with the canonical address each page names from
  `RULEMART_BASE_URL`, robots.txt and the sitemap, the about and privacy pages, the security headers and optional
  analytics, sign-in, sign-out, the dashboard, adding a library and following its check, the GitHub App's install
  return and webhook, listings, stars, rules' assets' pages and the images Rulemart serves,
  and the cart's page, its script, and its checkout, `queue` sends to the jobs queue, `secret`
  reads a secret from the environment or SSM, `logging` the JSON logger every command builds from
  `LOG_LEVEL` and `RULEMART_RELEASE`, and `postgrestest` and `database/databasetest` the test databases.
- `internal/lib/coderules` is the vendored copy of Code Rules' parser, and `internal/lib/textdiff` compares two
  versions' text for the comparison pages, within bounded work.
- `db/migrations` holds the schema as numbered SQL files.
- `.github/ISSUE_TEMPLATE` holds the issue forms that pages link to report a library, ask to vet one, or report a
  problem, by file name and field ID, so renaming either breaks those links.
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
  and `rulemart_accounts_writer` what signing in and out, reading visitors' GitHub accounts, listing, and starring
  write, and nothing more. Never grant to `rulemart_web`, `rulemart_worker`, or another login role: infrastructure owns
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
