# Rulemart decisions

The product and technical decisions that shape Rulemart, and why. Update an entry when a decision changes, rather
than adding history.

## Catalog and trust

- **Anyone signed in can list a public library.** New libraries are unvetted until vetted. Listing is a GET form
  that checks the repository's name, then a POST whose action holds it; the worker looks the repository up and
  ingests it within seconds, and the lister follows that on `/account/listings`, where they can try a failed listing
  again or remove one. A failure the repository caused is the lister's to see there, not an alarm.
  [Slice 6](slices/6-listing-and-unvetted.md) explains the choices.
- **An account holds at most 5 unvetted listings, and Rulemart at most 500.** Removing a listing frees its place, and
  a vetted one takes none. The cap bounds the unvetted area, the worker's hourly checks, and what someone with many
  GitHub accounts can add. Ingestion's size and memory limits apply to every library, and nothing runs a library's
  files.
- **A listing records who listed it, and pages don't say.** The lister can remove it at any time, which hides the
  library again. Deleting an account removes its listings, so a deleted account's listings can't fill the cap.
- **An account lists or retries at most 20 times a day, and every account together at most 100 times an hour**,
  since each one makes the worker check a repository, and may cost a GitHub API call.
- **Vetting is a reviewed change to a `catalog/vetted.yaml` file**, which lists each library by its code host and
  the host's repository ID. `main`'s protection guards it, and it ships with each release, so the public
  history shows when and why each library was vetted. A listed library becomes vetted when the release that adds it
  deploys, without being listed again: pages decide vetted or not from the release's list as they read.
- **Vetting covers a library, including its future releases.** A major version is declared by the library's
  maintainer, so pausing vetting on one would add nothing. The FAQ says so.
- **Unvetted libraries are hidden from normal browsing.** They're reached only through "View unvetted libraries" at
  the bottom of the libraries page, and every unvetted page shows "This library has not been vetted. Tread
  carefully." Rules are instructions that coding agents follow, so an unvetted rule is untrusted input for an
  agent. A library's own pages find it when it's vetted or a listing names it; every page across libraries reads
  only the vetted ones. The warning is a band in the one amber the palette has, which means caution and nothing else.
- **Search covers vetted libraries only.**
- **Unvetted rules can go in the cart only after an explicit confirmation**, and the checkout prompt names them to
  the agent.
- **Unvetted pages carry `noindex`**, and links to them `nofollow`, so listing a repository can't borrow Rulemart's
  reputation in search engines. Their robots tag says `noindex, nofollow`, which covers every link on them, the
  repository's own included, and they name no canonical address.

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
  names. A canonical group without an icon shows its initial: Devicon has no logo for Goose, TanStack Query, or
  TanStack Router, so they show initials until it does. An icon drawn mostly in dark colors, such as Zustand's,
  keeps a light tile in dark themes, rather than being inverted as monochrome icons are. Pages show icons with `<img>`, and tests reject an
  SVG that holds scripts, event handlers, or references outside itself.

## Browsing and search

- **Search is Postgres full-text search, in the Neon database Rulemart already uses.** It needs no other service,
  and each search reads one state of the catalog, as pages do, so a result can't name a rule its library's page
  doesn't show. Each current rule version stores a generated `tsvector` of its title, its reading guidance and
  impact description, and its body, weighted in that order. Search adds each rule's group names as it reads: the
  canonical list's name, passed as a parameter, and the name part of the group's ID, never the name a library
  declares. A word joined with `-`, `/`, or `:`, such as `keep-tests-independent` or `techs/go`, also matches the IDs
  pages show. Rules that hold every word come first, then rules that hold some, each ranked by where the words match,
  title first, and a rule that lacks some words names them. [Slice 3](slices/3-browse-and-search.md) compares the
  alternatives and states the ranking.
- **Only a canonical group has a page across libraries,** at `/groups/{techs|practices}/{name}`. Any other group
  stands alone, so the groups page lists it once per library and leads to that library's section for it.
- **A page that lists rules from more than one library names each rule's library**, by its owner's avatar and
  `owner/name`, and search results show each rule's source-qualified ID, `owner/name:rule-ID`, Code Rules'
  `source:rule` form with the repository as the source.
- **Libraries are listed by owner and name wherever several appear**, so no library can buy its place; search
  orders by relevance.
- **Search result pages carry `noindex` and name no canonical address.** Each query would otherwise be a page of
  its own to a search engine.

## Releases and comparison

- **Every rule version keeps the file its release published**, with its title, impact, and reading guidance, so pages
  can compare any two versions and name a retired rule. Only a current rule's current version keeps the HTML its page
  shows. The worker ingests again a library stored before this, so production fills in older versions by itself.
- **A library's releases are built from their stored release records**, laid out as Code Rules' generated release
  notes are, rather than from the tags' Markdown notes, which a library could fill with text Rulemart can't check.
  They're paged, newest first, at most 2,000 rows a page, since each release lists every rule's version again.
- **Comparisons are the Library releases and Versions tabs with `from` and `to` parameters**, chosen with a GET form,
  and carry `noindex` without a canonical address, as search does, since every pair would be a page of its own.
- **Diffs are computed when a page is read**, by `internal/lib/textdiff`, a bounded diff that marks changed words in
  Markdown blocks or shows a unified diff of lines, within 512 KiB of rule text and 10,000 rendered rows and marks
  per page. A rule's text in a diff is
  always escaped. [Slice 4](slices/4-releases-and-comparison.md) explains the choices.
- **A retired rule has a page**: its retirement, its chain of replacements to a current rule, its last text, and its
  versions. A rename, which Code Rules records as a retirement and a new rule under the same title, shows as one.
  Retired rules stay out of search.

## Accounts and sign-in

- **GitHub is the only sign-in provider, through an OAuth app that asks for no scopes.** Rulemart reads the user's
  ID, login, and avatar once, discards the token, and keeps nothing else: an account is keyed by GitHub's numeric user
  ID, since logins change. The flow uses state and PKCE, kept in a ten-minute `__Host-` cookie.
  [Slice 5](slices/5-sign-in.md) explains the choices.
- **Sessions live in Postgres, by the SHA-256 of a random token** the `__Host-rulemart-session` cookie holds: Secure,
  HttpOnly, SameSite=Lax. A session lasts 30 days and is never extended, each sign-in replaces the browser's session,
  and an account keeps at most 20.
- **A page for a signed-in visitor is never cached.** Any response to a request with the session cookie, or that sets
  a cookie, is `private, no-store`; CloudFront keys its cache on the session cookie too; and other pages vary with
  `Cookie` in browsers. Pages for everyone stay public and identical, so signed-out traffic keeps the cache. A notice
  after signing out comes from a one-time cookie, which CloudFront also keys on, so it never needs a query string.
- **Writes are POSTs with empty bodies, refused when another site starts them**, by `Sec-Fetch-Site` or `Origin`.
  CloudFront can't forward a body a browser didn't hash for origin access control, so forms carry their input in the
  action's query string, and there's no CSRF token.
- **Without `GITHUB_CLIENT_ID`, pages offer no sign-in.** Local builds with the `rulemartdev` tag sign in test users
  instead; release builds can't include that, and a test checks the release binary.

## Application

- **Go, templ, Tailwind, sqlc, and goose, with Postgres on Neon.** No Node: Tailwind runs as its standalone
  binary, and HTMX is vendored.
- **Static assets are embedded in the web binary** and cached by CloudFront for a year under hashed names.
- **Ingestion reads library repositories with go-git over HTTPS**, the way Code Rules reads them, and parses
  release records with Code Rules' own parser: a copy in `internal/lib/coderules` until Code Rules publishes a
  public parsing package.
- **The web function connects as `rulemart_web`, a login that can only read what the pages show, through its
  membership in `rulemart_catalog_reader`, and sign visitors in and out, through its membership in
  `rulemart_accounts_writer`, which writes only accounts, sessions, and listings.** Infrastructure owns the roles: it creates
  each group role with SQL, as a NOLOGIN role, creates the login, and makes the login a member, because a role made
  through Neon's API or console joins `neon_superuser`, which can read and write every table and create roles and
  databases. Migrations own the grants: they grant each group role what each table needs, never grant to a login,
  and never create roles, so a release can't migrate before infrastructure has, and a login can be replaced or
  rotated without a migration. Migrations connect as the database's owner.
- **Ingestion connects as `rulemart_worker`, a login that can only write the catalog, through its membership in
  `rulemart_catalog_writer`.** The split is the web function's: infrastructure creates the NOLOGIN group role, the
  login, and the membership with SQL, and migrations grant the group exactly what ingestion writes, which never
  includes deleting a library or changing the schema. Of a listing, it may change only what a check finds: the
  repository's ID, when it last checked, and why that failed. The worker function and the operator's `cmd/ingest` both use
  it, so production ingestion never needs the owner.
- **The worker keeps vetted and listed libraries current.** The EventBridge schedule invokes the worker function
  every hour, and it queues one job per vetted library, and one per listing to check, on the SQS jobs queue, which
  invokes it again for each job. The web function queues a new listing's job at once. A
  job lists the library's `release/<number>` tags with go-git, without fetching objects, and ingests only when their
  numbers and tag object IDs differ from what the catalog stored, so an unchanged library costs one request and no
  GitHub API call. A failed job writes nothing; SQS retries it, then moves it to the dead-letter queue, whose alarm
  reports it. Hourly, not every 10 minutes, because each job reads the stored tags from Postgres and wakes Neon's
  compute: about $3 a month instead of about $12. Faster updates would keep a tag fingerprint outside Postgres.
- **A job names a library only by its code host and the host's repository ID, and only a vetted one, or a listing by
  its ID.** It carries no URL: the worker fetches from the clone URL the catalog stored, or the one the host's API
  returns for that ID or the listing's name, so a queued message can't point it at another repository.
- **The worker may authenticate to GitHub's API with a token from SSM**, which raises its limit from 60 requests an
  hour, shared with other Lambda functions on the same address. It calls the API once per new listing, and once per
  library whose tags changed; listing tags uses Git.
- **Code is organized by bounded context first, and by layer only within a context**, following fabricahq/greenfield's
  ADR 0002 (backend bounded contexts). `internal/contexts/catalog` owns the catalog: `domain` for its values and
  rules, with no I/O; `render` for rules' Markdown, which assembly takes as a function so the web function doesn't
  link a Markdown renderer; `app` for ingestion and page reads; `source/git` and `source/github` for the adapters
  that fetch libraries; `store` for the persistence contract, with `store/postgres` as its only implementation and the
  catalog's only SQL; and `views` for what pages read. `internal/platform` holds runtime that contexts share, such
  as the database, migrations, and the web server, which stays in platform as greenfield's transports do.
  `internal/lib` holds narrow libraries that own no product concept, such as the parser copy.
  `internal/contexts/accounts` owns accounts and sessions with the same layout, plus `github` for the OAuth app.
  Contexts added later, such as the cart, get it too.
- **Build in thin vertical slices**, each deployed and checked end to end.
- **Page URLs, such as `/{owner}/{repo}`, assume one code host, GitHub.** The routing decision for a second host is
  host-qualified URLs, such as `/gitlab/{group}/{repo}`, with GitHub keeping the short form. Libraries are stored by
  host and the host's repository ID, but the schema's host check, the vetting parser, and ingestion allow only
  github, so a second host also needs changes there.

## Infrastructure and delivery

- **One environment until launch**, at `rulemart.fabricahq.com`, with Neon branches for trying migrations on real
  data. `rulemart.ai` and `www.rulemart.ai` redirect there temporarily, with a 302, keeping the path, through a
  Cloudflare Single Redirect that infra-live manages in code with its DNS records, so the redirect is reviewed and
  can't drift from the domain it points at. It stays temporary while the main domain may still change, because
  browsers cache a permanent redirect indefinitely.
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
