# Slice 1: see a library

## Goal

A visitor opens Rulemart and sees one real library, built from its real release
tags: the library's page, and each rule's page with its full version history.
Nothing is written except by ingestion, which an operator runs.

The library is `fabricahq/code-rules-test-library`, which publishes Code Rules'
`release/<n>` tags. `fabricahq/public-rules` hasn't published one yet.

## Scope

**Data.** Migrations add libraries, their releases, rules, and rule versions. A
library is matched by its code host and the host's repository ID, GitHub's for now,
so renames don't break it. The
current version of each rule stores its rendered content. Every version stores
its release, change level, and summaries.

**Ingestion.** The catalog context fetches a library's repository with go-git over
HTTPS, in memory, and reads every annotated `release/<n>` tag. For each release
it parses the record: which rules changed, their versions, their change levels
and summaries, and retirements. It reads each current rule's file, and each
group's `_group.yaml`, at the tag that published that rule's current version. It
writes everything in one transaction, so running it again changes nothing.
Parsing uses a temporary copy of Code Rules' parser in `internal/lib/coderules`,
marked for removal once Code Rules publishes a public package.

**Operator command.** `cmd/ingest <repository URL>` runs ingestion against
`DATABASE_URL`, or against `DATABASE_URL_PARAMETER` like `cmd/migrate-database`. The
scheduled poller that runs ingestion automatically is slice 2.

**Vetting.** `catalog/vetted.yaml` lists vetted libraries by GitHub repository
ID. This slice lists the test library. Pages show only vetted libraries; the
unvetted area comes later.

**Web.** The web function becomes a standard `net/http` handler, served on
Lambda through a Function URL adapter and locally with `go run ./cmd/web`.
Pages use templ components and Tailwind, with the prototype's design tokens and
layout, including dark mode:

- `/`: the vetted libraries, one for now.
- `/{owner}/{repo}`: the library page, with its groups and rules, and its latest
  release.
- `/{owner}/{repo}/{rule ID}`: a rule page with the rendered rule, and a
  Versions tab listing each version with its release, date, change marker, and
  summaries.

Rule Markdown is rendered with raw HTML escaped, since rule content comes from
repositories we don't control. Pages are cacheable by CloudFront for a minute.
The skeleton's `/enqueue`, `/messages`, and `/cached` routes are removed, along
with `internal/hello`.

**Tooling.** templ runs as `go tool templ`. Tailwind runs as its standalone
binary, which the Makefile downloads and verifies by pinned checksum. There's
no Node. CI checks that generated files are current.

## Verification

- **Ingestion tests:** integration tests against Postgres and a Git repository
  built in the test, with annotated release tags. They cover:
  - a first release
  - later releases with each change level
  - a retired rule
  - a rule whose current version comes from an older release
  - running ingestion twice
  - a malformed release record, which stops ingestion without partial writes
- **Page tests:** each page against ingested fixture data, asserting what a
  visitor sees: titles, versions, summaries, change markers, and a missing
  library or rule returning 404.
- **Real data, locally:** `make db`, then ingesting `fabricahq/code-rules-test-library`
  and browsing the pages locally, with screenshots compared against the
  prototype.
- **After deployment:** an HTTP smoke check against the deployed site.

## Not in this slice

Automatic updates (slice 2). Browsing by technology or practice, search, and more
than one library (slice 3). The Library releases tab and version comparison
(slice 4). Sign-in, stars, adding libraries, the unvetted area, and the cart.
