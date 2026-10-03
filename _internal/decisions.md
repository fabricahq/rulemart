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
- **A listing records who listed it, and the library's facts say "Added by" and "On Rulemart since"**: the login the
  lister last signed in with, linked to their GitHub profile, and when they listed it; a library vetted without a
  listing reads "Fabrica" and the date it was first ingested. The lister can remove it at any time, after a page that
  says what removing does, which hides the library again. A listing that failed before its library ever ingested
  doesn't reserve the repository: another account listing it replaces it. Deleting an account removes its listings, so a deleted account's listings can't fill the cap.
- **An account lists or retries at most 20 times a day, and every account together at most 100 times an hour**,
  since each one makes the worker check a repository, and may cost a GitHub API call.
- **Vetting is a reviewed change to a `catalog/vetted.yaml` file**, which lists each library by its code host and
  the host's repository ID. `main`'s protection guards it, and it ships with each release, so the public
  history shows when and why each library was vetted. A listed library becomes vetted when the release that adds it
  deploys, without being listed again: pages decide vetted or not from the release's list as they read.
- **Vetting covers a library, including its future releases.** A major version is declared by the library's
  maintainer, so pausing vetting on one would add nothing. The FAQ says so.
- **Unvetted libraries are an opt-in at every listing.** Home, the libraries page, browse, group pages, and search
  show vetted libraries only, and each carries a control that includes unvetted ones, as a query parameter, so the
  default view never changes and unvetted rows in an opted-in list carry an Unvetted tag. A vetted library shows a
  check mark, "Vetted by Rulemart". Every unvetted page shows "This library has not been vetted. Be sure to review
  these rules carefully." Rules are instructions that coding agents follow, so an unvetted rule is untrusted input
  for an agent. A library's own pages find it when it's vetted or a listing names it. The warning is a band in the
  one amber the palette has, which means caution and nothing else. [Realignment](realignment.md) explains the choice.
- **Search covers vetted libraries by default**, and unvetted ones when the visitor opts in.
- **Unvetted rules can go in the cart only after an explicit confirmation**, which the cart records, and the checkout
  prompt names their library to the agent, and asks it to review the rules before following them. An item whose
  library loses its vetting needs confirming again.
- **Unvetted pages carry `noindex`**, and links to them `nofollow`, so listing a repository can't borrow Rulemart's
  reputation in search engines. Their robots tag says `noindex, nofollow`, which covers every link on them, the
  repository's own included, and they name no canonical address.

## Search engines, sharing, and reports

- **The sitemap lists what search engines may index, and `robots.txt` keeps them out of the rest.** `/sitemap.xml`
  lists the site's own pages, the page of each group a vetted library holds, canonical or not, and each vetted library
  and its current rules, by their canonical addresses on `RULEMART_BASE_URL`, in one file of at most 45,000 groups and
  rules; without a base URL there's none.
  `/robots.txt` disallows account pages, sign-in, listing, search, the unvetted area, and comparisons, which also say
  `noindex`. [Slice 9](slices/9-launch-readiness.md) explains the choices.
- **A page with a canonical address describes itself to social sites**, with Open Graph tags and one image of
  Rulemart's; a page without one, such as an unvetted library's, shows no card.
- **Reports and requests to vet a library are GitHub issues** in Rulemart's public repository, through issue forms,
  so Rulemart stores nothing new. Each library's page links a report with the library filled in.
- **`/about` and `/privacy` say what Rulemart is and what it keeps.** The privacy notice states only what the code
  and infrastructure do, and changes with them: a change to what Rulemart keeps, logs, or shares updates it in the
  same pull request.

## Stars

- **Anyone signed in can star a rule in a vetted library.** A star is an account's mark on a rule, as the prototype
  has it; a rule that is renamed keeps its stars through the rename record. Rules in unvetted libraries show no
  Star button, so listing a repository can't borrow a count. Libraries have no stars of their own: the dashboard
  shows a library's total as the sum of its rules'. [Realignment](realignment.md) explains the change from
  [slice 7](slices/7-stars.md), whose other choices stand.
- **Counts are public and counted as pages read.** Every rule row and rule page shows its count, up to a minute old
  for visitors who aren't signed in, as every cached page is. Group and search pages offer a Most starred sort and a
  stars filter.
- **Starring is a POST to `/stars`, and unstarring to `/stars/remove`**, each naming the rule in its query string and
  returning to the page, which says what it did and focuses the button; repeating either changes nothing. A visitor
  who isn't signed in gets a link that signs them in and returns them, prompted once to star the rule. The
  dashboard's Starred rules tab lists a visitor's stars, newest first.
- **Deleting an account removes its stars**, so they stop counting.

## Cart and checkout

- **Anyone collects rules in a cart that lives in their browser**, in `localStorage`, as the prototype's does: one
  rule, or a whole group of one library, at most 100 items. Nothing asks for sign-in to add. A script paints the
  header's badge and each page's In cart state from the stored keys, so public pages stay identical and cached for
  everyone. [Realignment](realignment.md) explains the change from [slice 8](slices/8-cart-and-checkout.md), whose
  checkout safeguards stand.
- **Adding opens a modal on the rule page** that offers just the rule or its whole group, and asks for a confirmation
  when the library is unvetted, which the cart records for the library. Items are named by library, group, and rule
  ID, so checkout resolves them against the catalog and says when one is retired, gone, or in a library that lost
  its vetting, and leaves it out.
- **Checkout is a page that asks a JSON endpoint for the cart's items, each library's latest release, and the
  texts.** It shows a Prompt tab and a Commands tab, both built from `code-rules project add library`,
  `code-rules project add rule --from`, and `code-rules project sync`; each rule can stay in sync or be forked, and
  a group always stays in sync. By default nothing is pinned, so rules move when the project runs
  `code-rules project update`; the Commands tab says how to pin with `ref`. The prompt holds no text a library
  wrote, so no library can write instructions into it, and names each unvetted library so the agent reviews its
  rules first, pinned to the commit reviewed. [Slice R5](slices/14-cart-and-checkout.md) records how the commands
  were checked against the real CLI.
- **Signed in, checkout offers the visitor's projects**, read from their repositories' provenance files, so the
  prompt names the repository and says which libraries it already imports.

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
  names. Where Devicon's logo doesn't read at a tile's size, a project's own one-color logo, vendored under
  `community/` with its source, takes its place, as Zustand's bear does. A canonical group without an icon shows its
  initial: Devicon has no logo for Goose, TanStack Query, or TanStack Router, so they show initials until it does. An
  icon drawn mostly in dark colors keeps a light tile in dark themes, rather than being inverted as monochrome icons
  are. Pages show icons with `<img>`, and tests reject an
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
- **Every group has a page across libraries,** at `/g/{techs|practices}/{name}`. A canonical group's page combines
  every library; any other group's page holds the rules of the libraries that chose that exact ID, says it isn't
  canonical, and points at the canonical group it resembles when the catalog names one. Browse pages list canonical
  groups and lead to the others under "Other groups".
- **A page that lists rules from more than one library names each rule's library**, by its mark, Fabrica's logo or
  its owner's avatar, and `owner/name`, in the one rule row every list shows, as the prototype's does.
  [Slice R4](slices/13-discovery.md) explains the change from slice 3's source-qualified IDs in search results.
- **Libraries are listed by owner and name wherever several appear**, so no library can buy its place. Search orders
  by relevance by default, and group pages by stars; both offer Most starred and Newest.
- **Search result pages carry `noindex` and name no canonical address.** Each query would otherwise be a page of
  its own to a search engine.

## Releases and comparison

- **Every rule version keeps the file its release published**, with its title, impact, reading guidance, and tags, so
  pages can compare any two versions, name a retired rule, and link a rule's tags to search. Only a current rule's current version keeps the HTML its page
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
  Retired rules appear in search, labeled Retired with their replacement, below current rules that match as well.

## Rule pages and assets

- **A current rule's assets are stored at ingestion**: the files in its asset directory, `assets/<rule name>/` beside
  its file, at the release that published its current version, and the library-root `assets/` files its text or
  Markdown files link to, at the latest release. Ingestion keeps the bytes of images and text within 256 KiB a file
  and 2 MiB a rule, the shared files counting as one more rule, out of the content budget, renders Markdown with raw
  HTML escaped and text as highlighted code, and lists any other file with its size, linked on GitHub. A rule's links
  to its assets lead to their pages, and its images load from Rulemart. [Slice R6](slices/15-library-and-rule-pages.md)
  explains the choices.
- **Rulemart serves only images from a library, from its own origin**, at an asset's page address with `?raw=1`, as
  the type ingestion recorded, with `nosniff`, cached a day, under a content security policy that loads and runs
  nothing in a sandbox, so no library can serve a page, or a script, from Rulemart.
- **A library's pages and a rule's follow the prototype's**: the Groups tab's ticked groups live in `?sel=`, so they
  survive a visit to a group's page and back; All rules shows retired rules in place, grayed, when asked; and
  Discussion's tab and Discuss keep their places, rendering nothing, until rulemart#27.

## Accounts and sign-in

- **GitHub is the only sign-in provider, through the OAuth app "Rulemart", which asks for `read:org`.** Rulemart
  reads the user's ID, login, avatar, and organizations, and keeps the token encrypted in the session row so the
  dashboard can read the visitor's repositories again, until sign-out deletes it. An account is keyed by GitHub's
  numeric user ID, since logins change. The flow uses state and PKCE, kept in a ten-minute `__Host-` cookie.
  Private repositories need the GitHub App "Rulemart by Fabrica" (Contents read, Metadata read), which the visitor
  installs from the dashboard; a GitHub App's user token sees only organizations it's installed on, so it can't
  replace the OAuth app for the first view. [Slice 5](slices/5-sign-in.md) and [realignment](realignment.md)
  explain the choices.
- **Sessions live in Postgres, by the SHA-256 of a random token** the `__Host-rulemart-session` cookie holds: Secure,
  HttpOnly, SameSite=Lax. A session lasts 30 days and is never extended, each sign-in replaces the browser's session,
  and an account keeps at most 20.
- **Public pages are `public, max-age=0, s-maxage=60`**: CloudFront keeps them a minute, and browsers ask it again
  each time, so a browser that signs in or out never shows a page it kept from before.
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
  binary. Pages are server-rendered; small scripts paint what only the browser knows, such as the cart.
- **Static assets are embedded in the web binary** and cached by CloudFront for a year under hashed names.
- **Ingestion reads library repositories with go-git over HTTPS**, the way Code Rules reads them, and parses
  release records with Code Rules' own parser: a copy in `internal/lib/coderules` until Code Rules publishes a
  public parsing package.
- **The web function connects as `rulemart_web`, a login that can only read what the pages show, through its
  membership in `rulemart_catalog_reader`, and sign visitors in and out, through its membership in
  `rulemart_accounts_writer`, which writes only accounts, sessions, listings, and stars.** Carts live in browsers, so
  checkout writes nothing. Infrastructure owns the roles: it creates each group role with SQL, as a NOLOGIN role,
  creates the login, and makes the login a member, because a role made through Neon's API or console joins
  `neon_superuser`, which can read and write every table and create roles and databases. Migrations own the grants:
  they grant each group role what each table needs, never grant to a login, and never create roles, so a release can't
  migrate before infrastructure has, and a login can be replaced or rotated without a migration. Migrations connect as
  the database's owner.
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
  Stars and listings live in the catalog context, since each names a library or rule, and pages read them with the
  catalog from one snapshot; so does checkout, which resolves the keys a browser's cart sends against one snapshot
  of the catalog. A context added later gets the same layout.
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
- **The web function sends every response's security headers**: the content security policy, HSTS for a year without
  `preload`, a permissions policy that denies features Rulemart never uses, `Cross-Origin-Opener-Policy`,
  `nosniff`, and the referrer policy. Sending them from the function, rather than a CloudFront response headers
  policy, keeps them in one place that tests cover.
- **Cloudflare Web Analytics counts page views on every page, only once its site token is set**, as
  `CLOUDFLARE_WEB_ANALYTICS_TOKEN`. It sets no cookie, and the paths it reports name no visitor. Without the token,
  pages load no other site's script, and the content security policy allows none; with it, the policy adds
  Cloudflare's beacon and its reports, and the privacy notice says so.
- **Static files share one CloudFront copy**: `/_static/*` and `/favicon.ico` have a cache behavior whose key holds no
  cookie or query string, so a signed-in visitor doesn't cache them per session.
- **An AWS WAF rate rule on POSTs is ready but off**, at about $6 a month, since the function bounds each kind of
  write itself. Infrastructure turns it on if abuse appears.
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
- **Alarms cover what the function answers anyway.** Besides Lambda's errors and throttles, the URL's 5xx responses,
  and the jobs queue, the web function alarms on the lines it logs for a failed GitHub sign-in, a listing it couldn't
  queue, and a sitemap that left out rules.
- **CloudFront's logs are kept for 6 months, and the functions' for 30 days.** S3 deletes each access log file 180
  days after delivery, and CloudWatch Logs deletes function lines after 30 days. IP addresses are personal data, so
  `/privacy` says what CloudFront logs, why, and for how long, and lowering the retention is how Rulemart keeps less.
- **Every command logs JSON lines through `internal/platform/logging`**, at the level `LOG_LEVEL` names, info by
  default, and each line names the release that wrote it, from `RULEMART_RELEASE`. An unknown level stops the command
  at start, rather than logging at a level nobody chose.
- **Log volume stays small**, because CloudWatch Logs bills by the byte. The function logs one compact line per
  request it serves, which CloudFront's cache keeps to a fraction of traffic; nothing at debug level by default; no
  line per query or per rule; and a stack trace only for a panic. A long-lived function logs `ready`, with its schema
  version and how long starting took, or logs `startup failed`, with the error, and exits. `migrate-database` logs
  each migration it applies, because CI keeps its output.
