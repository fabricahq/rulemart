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
  rotated without a migration. Migrations and ingestion connect as the database's owner.
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
