# Rulemart decisions

The product and technical decisions that shape Rulemart, and why. Update an entry when a decision changes, rather
than adding history.

## Catalog and trust

- **Anyone signed in can list a public library.** New libraries are unvetted until vetted.
- **Vetting is a reviewed change to a `catalog/vetted.yaml` file**, which lists each library by GitHub
  repository ID. `main`'s protection guards it, and it ships with each release, so the public
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
  release records with Code Rules' own parser: a copy in `third_party/coderules` until Code Rules publishes a public
  parsing package.
- **The web function connects as `rulemart_web`, a role that can only read what the pages show.** Infrastructure
  creates it with SQL, as a plain LOGIN role, because a role made through Neon's API or console joins
  `neon_superuser`, which can read and write every table and create roles and databases. Migrations grant it what
  each table needs and never create it, so a release can't migrate before infrastructure has. Migrations and
  ingestion connect as the database's owner.
- **Build in thin vertical slices**, each deployed and checked end to end.

## Infrastructure and delivery

- **One environment until launch**, at `rulemart.fabricahq.com`, with Neon branches for trying migrations on real
  data. `rulemart.ai` redirects there through a Cloudflare rule set up by hand.
- **Migrations run in CI on each `v*` release tag**, before assets are published (planned). The job assumes an AWS
  role through GitHub OIDC, reads the pooled connection string from SSM, and derives the direct one. It runs in a
  `production` environment limited to `v*` tags, which only admins can create.
- **GitHub repositories are created by hand**, and their rulesets, environments, and Pages are managed in code.
- **Lambda packaging moves to a public, shared tool** (planned), and a release is pinned for deployment only after
  a matching rebuild or a verified build attestation.
- **Local development and tests use Postgres 18 in Docker.**
