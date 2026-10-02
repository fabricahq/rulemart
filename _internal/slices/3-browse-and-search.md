# Slice 3: browse and search

## Goal

A visitor finds rules across every vetted library: by technology or practice, or by searching for words. Pages that
show rules from more than one library say which library each rule comes from, and list libraries in an order no
library can buy.

Production vets two libraries, `fabricahq/public-rules` and `fabricahq/code-rules-test-library`, so every page
already has more than one library to handle.

Decisions marked **Proposed** are new in this slice and wait for review. **Decided** ones are new and Josh has
accepted them. **Existing** ones are already in [decisions.md](../decisions.md) or an earlier slice.

## What a visitor can do

- **Search from any page.** Every page's header has a search field. On a phone it's a search link. The home page
  leads with a larger search field.
- **Browse groups.** `/groups` lists every group that holds a current rule in a vetted library: technologies first,
  then practices. The home page shows the canonical ones as a grid of tiles, with a link to `/groups`.
- **See one group across libraries.** `/groups/techs/go` shows every vetted library's current Go rules, under each
  library's name, avatar, and a link to its page.
- **Search.** `/search?q=retry limits` lists the best-matching current rules of vetted libraries, best first. Each
  result names its rule, its library, and its group.

## Decisions

### Search engine

**Decided: Postgres full-text search, in the Neon database Rulemart already uses.** The comparison that led to it:

| Option | For | Against |
| --- | --- | --- |
| **Postgres full-text search** | No new service, account, or bill. Each search reads one consistent state of the catalog, the way every page does, so a result can't name a rule its library page doesn't show. English stemming, weights, and ranking are built in. `websearch_to_tsquery` accepts any input, including quotes, `or`, and `-word`, without a syntax error. | Weaker typo tolerance and relevance tuning than a dedicated engine. It ranks within one query and has no learning from clicks. |
| Trigram similarity (`pg_trgm`) | Tolerates typos and finds partial words. | No stemming or word weights, and on long rule bodies similarity scores reward short documents, so it ranks poorly as the main engine. It also needs an extension and its own indexes. |
| An external search service (Algolia, Typesense, Meilisearch, OpenSearch) | Best typo tolerance, facets, and relevance tools. | A second store to keep in step with ingestion, which can disagree with the pages, a new secret and network dependency for the worker, a monthly cost, and a new failure mode, for 133 rules. |
| An index the browser downloads | No server work per search. | Downloads every rule's text to each visitor, needs JavaScript, and grows with every vetted library. |

Postgres fits now: the catalog is small, the database is already there, and search stays consistent with the pages
by construction. Trigram matching can be added beside it later, for typos, without changing the URL or the page.

### Search

- **What it matches. Proposed.** Migration 00006 adds `rule_versions.search_document`, a `tsvector` Postgres
  generates from the row: the title weighted A, the reading guidance and impact description B, and the Markdown body
  D, all stemmed with the `english` configuration. Each part is cut to a fixed length first (1,000, 10,000, and
  100,000 characters), because a vector holds at most 1 MB and a rule file may hold 1 MiB: an uncut rule could fail
  its library's ingestion.
- **Group names. Proposed.** A rule matches its text and its group's name together, so `retry go` finds a Go rule
  whose text says retry. A group's name is the canonical list's name for a canonical group, and the name part of any
  group's ID, such as `go` in `techs/go`. Never the name a library declares for its group (**Existing**: pages never
  show it). Search reads the list when it runs, as a parameter, as other cross-library reads do (**Existing**:
  canonical status is decided when pages read), so the names aren't stored and a release that moves the pin changes
  search at once.
- **Identifiers. Proposed.** Visitors copy the IDs pages show, so a word that joins words with `-`, `/`, or `:`, such
  as `keep-tests-independent`, `retry-limits`, `techs/go`, `public-rules`, or
  `fabricahq/public-rules:practices/testing/keep-tests-independent`, is a phrase of its words. It matches that phrase
  in the rule's text, and in the words of the rule's source-qualified ID, `owner/name:rule-ID`, whose rule ID starts
  with its group's ID. So `techs/go` finds the Go rules but not `techs/goose`'s, and `retry-limits` finds
  `verify-retry-limits`. A plain word doesn't match IDs, so `techs` alone finds nothing, but any word matches the
  library's owner and name, as body text, so `fabricahq` finds its libraries' rules. A hyphen leaves a word out only
  at the start of a word.
- **Every word, then some. Proposed.** A rule matches when it holds at least one of the words to find and none of the
  words to leave out. A rule that lacks some of the words names them, as "Missing: handling", and the results say how
  many hold every word. `or` joins the words on either side into one term that either satisfies.
- **Ranking. Proposed.** Each word scores by the best place it matches: the title 1, the group's name or the
  identifiers 0.8, the reading guidance or impact description 0.5, and the body or the library's name 0.1. A rule's
  score is its words' average times the square of the share of words it holds, so a rule that holds every word
  usually comes first, but one whose title names the subject can pass one whose body mentions every word in passing:
  `error handling` lists the Go error rules before rules that only mention both words. A title made mostly of matched
  words adds up to 0.25, so "Verify retry limits" outranks "Reset query errors when an error boundary retries" for
  `retry`. Words to leave out only filter, so `testing -react` keeps this order. Equal scores fall back to `ts_rank`,
  then title, library owner and name, and rule ID, so the order is stable. There's no prefix matching: `go` never
  matches `goose`, and finds a goose rule only when its text names Go.
- **Input. Proposed.** The query is the `q` parameter. Rulemart trims it, turns control characters and invalid
  UTF-8 into spaces, and collapses runs of spaces. An empty query shows the search page without searching. A query
  longer than 200 characters isn't run: the page says to shorten it. A query with no word to find, such as only stop words like `the`,
  only punctuation, or only words left out, matches nothing, and the page says why.
- **Results. Proposed.** A search shows at most the 50 best matches, and says how many matched in all. Paging
  through more waits until the catalog needs it. Each result shows its title, impact, reading guidance, library,
  group, and version.
- **Cost. Proposed.** Search reads the stored documents of every vetted library's current rules and scores each word
  against each, since an index on the document alone can't find a rule whose words are split between its text, its
  group, and its IDs. On production's two libraries, about 150 rules, that takes a few milliseconds in Postgres. An
  index waits until measurement shows search is slow.
- **Vetted libraries only. Existing.** Search reads only the libraries in the release's `catalog/vetted.yaml`.

### Browsing

- **URLs. Proposed.** `/groups` is the index, `/groups/{techs|practices}/{name}` a group, and `/search` search.
  `groups` and `search` aren't GitHub accounts, and `/search` has one segment, so it can't shadow a library. A
  vetting review would notice a library owned by a future `groups` account, which would then need host-qualified
  URLs (**Existing**: the routing decision for a second host).
- **Only canonical groups have a page across libraries. Proposed.** The canonical list names the groups libraries
  share; a group whose ID isn't on it stands alone (**Existing**). So `/groups/techs/golang` is a missing page, and
  the index lists a group that isn't canonical once per library, flagged "not canonical", linking to that library's
  section for it. A canonical group with no current rule in a vetted library shows its page with an empty state, so
  its URL works before and after a library adopts it.
- **What the index shows. Proposed.** Canonical groups by the list's name and icon (**Existing**), with how many
  current rules they hold, and in how many libraries. A practice also shows the list's one-line description, as a
  library's page shows a practice's reading guidance and a technology's name explains itself. The description is
  Code Rules', not a library's, so no library can describe a group every library shares. A group that isn't
  canonical shows the library that holds it.
- **Order. Proposed.** Technologies, then practices. Within each, canonical groups by name, then groups that aren't
  canonical by ID and library. On a group's page, libraries are in owner and name order, as on the home page, and
  each library's rules in title order.

### More than one library

- **Attribution. Proposed.** A page listing rules from more than one library names each rule's library with its
  owner's avatar and `owner/name`. On a group's page the library heads its section. In search results each rule
  shows its source-qualified ID, `owner/name:rule-ID`: Code Rules' `source:rule` form, with the library's repository
  as the source, since Rulemart doesn't know the name a project gives a source.
- **Library pages name their owner. Proposed.** A library's page heads with `owner / name`, so two libraries with one
  name read apart.
- **The home page reads its libraries and groups from one state of the catalog (Existing rule).** The store's
  `HomePage` read replaces `Libraries`, so the counts on one page always agree.

### Pages and caching

- **Search pages carry `noindex`, and name no canonical address. Proposed.** They're results, not content, and
  each query would be its own page to a search engine. The index and group pages name their canonical address
  (**Existing**).
- **Every page, search included, is cacheable for a minute (Existing).** CloudFront's cache policy keys on every
  query string, so each query is cached on its own, and no infrastructure change is needed.
- **The search form submits with GET to the site itself. Proposed.** The Content-Security-Policy's `form-action`
  goes from `'none'` to `'self'`. No script is needed. The field's text is 16 px, so phones don't zoom into it, and
  it holds at most 200 characters.
- **Logs keep the route, never the query (Existing).** The access log records `/search`, not what was searched, and
  a failed search's error names no query.

### Database

- **Migration 00006 only adds. Existing rule, Proposed content.** It adds a generated column, which the
  running v0.1.0 never names: its queries list their columns, and Postgres fills the generated column on every
  insert and update, so the release that's still running keeps ingesting while this one migrates. Adding the column
  rewrites `rule_versions` under a brief exclusive lock, milliseconds at this size.
- **No new grants. Proposed.** `rulemart_catalog_reader` reads the new column through its `SELECT` on
  `rule_versions` (**Existing**, migration 00003), and `rulemart_catalog_writer` never writes it. Tests read as `rulemart_web` and ingest as `rulemart_worker`, so a missing grant would fail them.

## Verification

- **Migration tests:** a rule stored before 00006 is searchable after it, and a rule too long to search in full is
  still stored.
- **Store tests**, against Postgres as `rulemart_web`:
  - a title match ranks above a match only in the reading guidance, which ranks above a match only in the body
  - a group's canonical name finds its rules, and a name a library declares doesn't
  - search, the group index, and a group's page skip unvetted libraries and retired rules
  - a group's rules come from every vetted library, attributed to each, in owner and name order
  - stemming, quoted phrases, `-word`, stop words only, and the result cap with its total
  - IDs: a rule's, part of one, a group's, a library's owner and name, and a source-qualified ID; `techs/go` never
    finds `techs/golang`
  - ranking on fixtures shaped like `fabricahq/public-rules`: `retry`, `error handling`, `go`, and a copied rule ID
- **App tests:** the index combines a canonical group across libraries and keeps a group that isn't canonical apart,
  per library, in the decided order; a group's page refuses an ID that isn't canonical.
- **Page tests:** each page's text, links, and attribution; odd input (control characters, invalid UTF-8, quotes,
  markup, only punctuation, very long); the empty and no-results states; `noindex`; the form's policy.
- **Real data, locally:** both production libraries ingested into a local database, then every new page checked in a
  browser at desktop and phone widths, in light and dark themes, with screenshots in the pull request.
- **After deployment:** an HTTP smoke check of `/groups`, `/groups/techs/react`, and `/search?q=effects`.

## Not in this slice

Paging through more than 50 results. Typo tolerance and partial words, through trigram matching. Filtering search
results by group or library, and highlighting the matched words. Search suggestions as you type. Unvetted libraries
and the unvetted area, sign-in, listing libraries, stars, and the cart. The library releases tab and version
comparison (slice 4). Dropping `hello_messages`, which slice 2 left for the release after v0.1.0, belongs in its own
change.
