# Rulemart decisions

The product and technical decisions that shape Rulemart, and why. Update an entry when a decision changes, rather
than adding history.

## Catalog and trust

- **Anyone signed in can list a public library.** New libraries are unvetted until vetted.
- **Vetting is a reviewed change to a `catalog/vetted.yaml` file**, which lists each library by its code host and
  the host's repository ID. `main`'s protection guards it, and it ships with each release, so the public
  history shows when and why each library was vetted.
- **Vetting covers a library, including its future releases.** A major version is declared by the library's
  maintainer, so pausing vetting on one would add nothing. The FAQ says so.
- **Unvetted libraries are hidden from normal browsing.** They're reached only through "View unvetted libraries" at
  the bottom of the libraries page, and every unvetted page shows "This library has not been vetted. Tread
  carefully." Rules are instructions that coding agents follow, so an unvetted rule is untrusted input for an
  agent.
- **Search covers vetted libraries only.**
- **Unvetted rules can go in the cart only after an explicit confirmation**, and the checkout prompt names them to
  the agent.
- **Unvetted pages carry `noindex`**, and links to them `nofollow`, so listing a repository can't borrow Rulemart's
  reputation in search engines.

## Groups

- **Code Rules owns the canonical group list; Rulemart pins and reads it.** `catalog/canonical-groups.yaml` is
  Code Rules' file at a pinned release, copied unchanged, and the vendored parser in `internal/lib/coderules` reads
  it, so Rulemart and Code Rules can't disagree about which IDs are canonical. Rulemart updates the pin deliberately,
  in its own pull request.
- **A group is canonical only when its ID is on the list exactly. The list has no aliases**: a library's
  `techs/golang` is a group of its own, never `techs/go`.
- **Pages name a canonical group by the list's display name, and any other group by its ID, flagged "not
  canonical".** They never show the name a library declares for a group, so a library can't rename a group every
  library shares, or pass off its own group as one by naming it like one.
- **Canonical status is decided when pages read, not stored at ingestion.** The list ships with each release, as
  `vetted.yaml` does, and holds under a hundred IDs, so applying it to a page's groups costs a map lookup each.
  Storing it would need a migration and a reingestion of every library whenever the pin moves, and a stored flag
  could disagree with the list the running release ships. A later query across libraries, such as browsing one
  canonical group, can pass the list's IDs as a parameter, as the page reads pass the vetted libraries.
- **Rulemart owns the groups' icons**, in `catalog/group-icons.yaml`: Devicon logos (MIT) for technologies and
  Lucide icons (ISC) for practices, vendored with their licenses, only for canonical groups, and only the files it
  names. A canonical group without an icon shows its initial. Pages show icons with `<img>`, and tests reject an
  SVG that holds scripts, event handlers, or references outside itself.

## Application

- **Go, templ, Tailwind, sqlc, and goose, with Postgres on Neon.** No Node: Tailwind runs as its standalone
  binary, and HTMX is vendored.
- **Static assets are embedded in the web binary** and cached by CloudFront for a year under hashed names.
- **Ingestion reads library repositories with go-git over HTTPS**, the way Code Rules reads them, and parses
  release records with Code Rules' own parser: a copy in `internal/lib/coderules` until Code Rules publishes a
  public parsing package.
- **The web function connects as `rulemart_web`, a login that can only read what the pages show, through its
  membership in `rulemart_catalog_reader`.** Infrastructure owns the roles: it creates `rulemart_catalog_reader`
  with SQL, as a NOLOGIN group role, creates the login, and makes the login a member, because a role made through
  Neon's API or console joins `neon_superuser`, which can read and write every table and create roles and
  databases. Migrations own the grants: they grant the group role what each table needs, never grant to a login,
  and never create roles, so a release can't migrate before infrastructure has, and a login can be replaced or
  rotated without a migration. Migrations connect as the database's owner.
- **Ingestion connects as `rulemart_worker`, a login that can only write the catalog, through its membership in
  `rulemart_catalog_writer`.** The split is the web function's: infrastructure creates the NOLOGIN group role, the
  login, and the membership with SQL, and migrations grant the group exactly what ingestion writes, which never
  includes deleting a library or changing the schema. The worker function and the operator's `cmd/ingest` both use
  it, so production ingestion never needs the owner.
- **The worker keeps vetted libraries current.** The EventBridge schedule invokes the worker function every hour,
  and it queues one job per vetted library on the SQS jobs queue, which invokes it again for each job. A
  job lists the library's `release/<number>` tags with go-git, without fetching objects, and ingests only when their
  numbers and tag object IDs differ from what the catalog stored, so an unchanged library costs one request and no
  GitHub API call. A failed job writes nothing; SQS retries it, then moves it to the dead-letter queue, whose alarm
  reports it. Hourly, not every 10 minutes, because each job reads the stored tags from Postgres and wakes Neon's
  compute: about $3 a month instead of about $12. Faster updates would keep a tag fingerprint outside Postgres.
- **A job names a library only by its code host and the host's repository ID, and only a vetted one.** It carries no
  URL: the worker fetches from the clone URL the catalog stored, or the one the host's API returns for that ID, so a
  queued message can't point it at another repository.
- **Code is organized by bounded context first, and by layer only within a context**, following fabricahq/greenfield's
  ADR 0002 (backend bounded contexts). `internal/contexts/catalog` owns the catalog: `domain` for its values and
  rules, with no I/O; `render` for rules' Markdown, which assembly takes as a function so the web function doesn't
  link a Markdown renderer; `app` for ingestion and page reads; `source/git` and `source/github` for the adapters
  that fetch libraries; `store` for the persistence contract, with `store/postgres` as its only implementation and the
  catalog's only SQL; and `views` for what pages read. `internal/platform` holds runtime that contexts share, such
  as the database, migrations, and the web server, which stays in platform as greenfield's transports do.
  `internal/lib` holds narrow libraries that own no product concept, such as the parser copy. Contexts added later,
  such as accounts or the cart, get the same layout.
- **Build in thin vertical slices**, each deployed and checked end to end.
- **Page URLs, such as `/{owner}/{repo}`, assume one code host, GitHub.** The routing decision for a second host is
  host-qualified URLs, such as `/gitlab/{group}/{repo}`, with GitHub keeping the short form. Libraries are stored by
  host and the host's repository ID, but the schema's host check, the vetting parser, and ingestion allow only
  github, so a second host also needs changes there.

## Infrastructure and delivery

- **One environment until launch**, at `rulemart.fabricahq.com`, with Neon branches for trying migrations on real
  data. `rulemart.ai` redirects there through a Cloudflare rule set up by hand.
- **Pages name their address on `RULEMART_BASE_URL` as canonical.** CloudFront's own `cloudfront.net` domain serves
  the same pages, so each page links its address on the public origin, without a tab's query string, and search
  engines index that one. Infrastructure sets the variable; unset, as in local development, pages name none, and a
  value that isn't a bare https origin stops the web function at start.
- **Releases are published by [Release Planner](https://release-planner.fabricahq.com)**, and merging a release
  pull request approves one. Nothing else tags or publishes a release.
- **Migrations are the application's concern, and run after a release is approved and before it's published**, as
  Release Planner's pre-publish workflow, "Migrate the database". If they fail, nothing is published. They never
  run at deploy time or from the infrastructure repository. The job runs in a `production` environment that only
  `main` can use, assumes an AWS role that trusts only that environment through GitHub OIDC, reads the pooled
  connection string from SSM, and derives the direct one. The environment's variables name the role and parameter,
  so this public repository names no account details.
- **Migrations are safe to run again, late, and twice at once**: goose skips applied migrations and takes a Postgres
  session lock. They work with the release that's still running: expand in one release, contract in a later one.
- **GitHub repositories are created by hand**, and their rulesets, environments, and Pages are managed in code.
- **Lambda packaging uses [lambda-build](https://github.com/fabricahq/lambda-build)**, a public, shared tool, as
  Release Planner's release-assets workflow. A release is pinned for deployment only after a matching rebuild or a
  verified build attestation.
- **Local development and tests use Postgres 18 in Docker.**

## What Rulemart logs

- **Two sources record traffic.** CloudFront's standard access logs hold one row for every request it serves, cache
  hits included, with the visitor's IP address, country, user agent, referrer, path, and query string. The web
  function logs one line for each request that reaches it. Analysis of who visits and what they read uses
  CloudFront's logs, through Athena; the function's lines explain how the origin behaved.
- **Only CloudFront's logs hold visitors' IP addresses.** The web function's logs never include an IP address, a
  raw path, a query string, or headers, because those can identify a visitor or carry a secret.
  - In place of the path, each line records the route pattern, such as `/{owner}/{repo}`. That also groups requests
    by page type.
  - When an error message names a library or rule, the name is replaced with the route's placeholder, such as
    `{owner}`.
  - The rest of an error message is kept, even when it names an internal address such as the database host, because
    diagnosing the failure needs it.
- **CloudFront's logs are kept for 6 months, and the functions' for 30 days.** S3 deletes each access log file 180
  days after delivery, and CloudWatch Logs deletes function lines after 30 days. IP addresses are personal data, so
  the privacy notice says what CloudFront logs, why, and for how long, and lowering the retention is how Rulemart
  keeps less.
- **Every command logs JSON lines through `internal/platform/logging`**, at the level `LOG_LEVEL` names, info by
  default, and each line names the release that wrote it, from `RULEMART_RELEASE`. An unknown level stops the command
  at start, rather than logging at a level nobody chose.
- **Log volume stays small**, because CloudWatch Logs bills by the byte. The function logs one compact line per
  request it serves, which CloudFront's cache keeps to a fraction of traffic; nothing at debug level by default; no
  line per query or per rule; and a stack trace only for a panic. A long-lived function logs `ready`, with its schema
  version and how long starting took, or logs `startup failed`, with the error, and exits. `migrate-database` logs
  each migration it applies, because CI keeps its output.
