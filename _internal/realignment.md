# Realignment with the prototype

How Rulemart gets from the merged slice 3 to 9 stack to the user experience the `prototype` branch defines, and
what launches at the end.

## Why

The `prototype` branch holds a click-through mock, `prototype/`, that is the user experience Rulemart should have.
Slices 3 to 9 were built from the plans in `slices/`, which name the prototype three times, so the stack took the
prototype's design tokens and little else. The backend is sound and stays: ingestion, release records and version
diffs, sign-in, sessions, the listing worker, vetting, caching, SEO, and security headers. The gap is in
information architecture, interactions, and two data decisions, stars and the cart.

From here on, **the prototype is the spec.** A slice is done when its routes behave as the prototype's do, judged
side by side in a browser, except where a decision below says otherwise.

To run the prototype:

```sh
git worktree add ../rulemart-prototype prototype
python3 -m http.server 8766 --directory ../rulemart-prototype/prototype
```

Then open <http://localhost:8766>. Its routes are hash routes, such as `#/g/techs/go`.

## Decisions

Decided by Josh on 2026-10-02. Each one that changes an entry in [decisions.md](decisions.md) is also changed there.

- **Stars apply to rules, not libraries.** Every rule row and rule page shows its count; the Star button sits in the
  rule page head beside Discuss; a library's total is the sum of its rules' stars, shown on the dashboard. Only rules
  in vetted libraries can be starred. Group and search pages sort by stars. The unreleased `stars` table is replaced
  by `rule_stars` before the next release.
- **The cart lives in the browser**, in `localStorage`, as the prototype's does. Nothing asks for sign-in to add to
  it. The header badge, the In cart states, and the cart page are painted by a script that reads it. Checkout sends
  the cart's keys to a JSON endpoint, which returns the resolved items, each library's latest release, and the prompt
  and commands text, so the text always matches the real catalog and CLI. Sign-in is needed only to pick a project
  at checkout. The unreleased `cart_items` table is dropped.
- **Checkout has two tabs, Prompt and Commands**, both built from `code-rules project add library`,
  `code-rules project add rule --from`, and `code-rules project sync`, which the CLI has today. Each rule can stay in
  sync or be forked; a whole group always stays in sync. The default does not pin a library to a release: rules move
  when the project runs `code-rules project update`. The Commands tab explains how to pin with `ref` instead.
- **Vetting stays, as an opt-in at every listing.** Home, browse, group pages, and search show vetted libraries only,
  each with a control that includes unvetted libraries, carried as a query parameter so the default view never
  changes. Unvetted rows in an opted-in list carry an Unvetted tag. Vetted libraries show the prototype's check
  mark, titled "Vetted by Rulemart". Unvetted pages keep the amber band, now reading "This library has not been
  vetted. Be sure to review these rules carefully.", with `noindex`, `nofollow` links, and the cart confirmation.
  The dashboard and the cart show whatever the visitor chose, tagged.
- **Retired rules appear in search**, labeled Retired with a "replaced by" line, ranked below current rules that
  match equally well. Reverses slice 4.
- **Listings show who listed them**: the About panel has "Added by @login" and "On Rulemart since", as the prototype.
  Reverses slice 6.
- **The dashboard and the Add a library picker launch.** `/me` shows the libraries the visitor and their
  organizations publish, the projects that use Rulemart libraries, read from each repository's
  `.code-rules/generated/provenance.json`, with update counts, and the visitor's starred rules. `/me/add` lists the
  visitor's and their organizations' repositories that hold a `rule-library.yaml` and a release, beside the URL
  form. Sign-in asks GitHub for `read:org`, and Rulemart keeps the OAuth token, encrypted, in the session row so the
  dashboard can refresh, and deletes it at sign-out. Private repositories need the GitHub App "Rulemart by Fabrica",
  which the visitor installs from `/me/private`.
- **Two GitHub apps**, both created on 2026-10-02 in the fabricahq organization: the OAuth app "Rulemart" for
  sign-in, and the GitHub App "Rulemart by Fabrica" (Contents read, Metadata read, installable on any account) for
  private repositories. A GitHub App's user token sees only organizations it is installed on, so it cannot replace
  the OAuth app for the dashboard's first view.
- **Discussion tabs and the Discuss button launch later**, tracked in
  [rulemart#27](https://github.com/fabricahq/rulemart/issues/27). The rule page head reserves their place.
- **Analytics load on every page, signed in or not**, as long as nothing sent to Cloudflare identifies a visitor or
  exposes one. Its beacon reports the page's path and referrer; dashboard and cart paths carry no visitor data, and
  the cart's items never leave the browser except to the checkout endpoint.
- **The brand package** in `brand/` replaces the header mark, favicons, the social image, and the GitHub App icon, in
  a slice of its own so the change is reviewed in isolation.
- **The prototype's URLs** replace the stack's where they differ: `/browse/techs`, `/browse/practices`,
  `/browse/{kind}/other`, `/g/{kind}/{name}`, `/{owner}`, `/{owner}/{repo}/{kind}/{group}`, `/cart`, `/signin`,
  `/me`, `/me/add`, `/me/private`, `/faq`, `/feedback`. Old addresses redirect permanently.

Everything else the slice docs proposed is accepted as they state it.

## Conformance matrix

Each prototype route, what the stack has today, and the slice that closes the gap. "Different" means a decision
above chose otherwise.

| Prototype route | Today | Status | Slice |
| --- | --- | --- | --- |
| `#/` home: hero, popular chips, tech and practice tiles, four libraries, List your library | Hero, libraries, group tiles | Partial | R1 |
| `#/browse/techs`, `#/browse/practices`: row per canonical group | `/groups`, one page | Partial | R1 |
| `#/browse/{kind}/other`: non-canonical groups | Listed per library on `/groups` | Missing | R1 |
| `#/libraries` | `/libraries`, vetted only, unvetted behind a link | Match, plus the opt-in control | R4 |
| `#/{owner}`: owner page with libraries | None | Missing | R1 |
| `#/faq`, `#/feedback` | `/about`, issue forms | Missing | R1 |
| Header: Techs, Practices, Libraries, FAQ, search, cart, sign-in | Libraries, Groups, search, sign-in | Partial | R1 |
| Footer: feedback, theme | Links, theme menu | Partial | R1 |
| Brand mark, favicons | Cube mark | Different | R2 |
| Star on rules, counts on rows, Starred rules tab | Stars on libraries | Different | R3 |
| `#/g/{kind}/{group}`: flat ranked list, filter sidebar, sort tabs, non-canonical with note | Canonical only, sectioned by library, no filters | Partial | R4 |
| `#/search`: grouped by group, filter sidebar, sort tabs | Flat, paged, no filters | Partial | R4 |
| Retired rules in search | Excluded | Different | R4 |
| Unvetted opt-in on browse, group, search | Hidden except `/unvetted` | Different | R4 |
| Cart in the browser, header badge, add-to-cart modal | Postgres cart, sign-in required | Different | R5 |
| `#/cart`: three-step checkout, sync or fork, upsell, project picker, Prompt and Commands tabs | Cart page and a prompt page | Partial | R5 |
| Library page: verified check, Groups tab checkboxes and Add groups panel, Added by, On Rulemart since | Add library button, per-row Add, Report link | Partial | R6 |
| `#/{owner}/{repo}/{kind}/{group}`: library group page with Add group | None | Missing | R6 |
| Rule page: tags, Star, Discuss, add-to-cart modal, About panel with Discuss, assets panel | Rule and Versions tabs, About panel | Partial | R6 |
| Asset pages | None | Missing | R6 |
| Versions tab and compare view | Present | Match | none |
| Library releases tab | Present | Match | none |
| Retired rule page | Present | Match | none |
| Discussion tabs, Discuss modal | None | Deferred, #27 | later |
| `#/signin` with perks | `/sign-in` | Partial | R7 |
| `#/me`: My libraries, used in your projects, Starred rules | `/account`, `/account/listings`, `/account/stars` | Partial | R7 |
| `#/me/add`: repo picker, URL form, `#/me/add/run` checklist | `/list` form, `/account/listings` | Partial | R7 |
| `#/me/private`, `#/gh/install` | None | Missing | R7 |
| About, privacy, robots, sitemap, headers, analytics | Present | Match, updated for the above | R8 |

## Slices

Each slice is one pull request from `main`, with a doc in `slices/` numbered from 10, and goes through the
verification below before it merges. Order matters: each builds on the last.

### R1. Shell and navigation

The header, footer, home, browse pages, owner pages, FAQ, feedback, and the prototype's URLs. No data change.
Routes: `/`, `/browse/techs`, `/browse/practices`, `/browse/{kind}/other`, `/g/{kind}/{name}` (the existing group
page, moved), `/{owner}`, `/faq`, `/feedback`; redirects from `/groups` and `/groups/{kind}/{name}`.

### R2. Brand

The header mark, favicons, apple-touch-icon, social image, and the README's mark from `brand/`, including Josh's
local header and favicon edits. One pull request with before-and-after screenshots at 1280 and 390 pixels, light
and dark.

### R3. Stars on rules

Migration: drop `stars`, add `rule_stars (account_id, rule_id, created_at)` with the same grants. The Star control in
the rule page head and the count on every rule row and card; `/account/stars` as a minimal Starred rules list until
R7 builds the dashboard. The star routes move to `/stars` and `/stars/remove`, named by rule.

### R4. Discovery

Group pages as flat lists with the filter sidebar (libraries, impact, stars, and the unvetted opt-in) and sort tabs
(Most starred, Newest); non-canonical group pages with the Not canonical note. Search grouped by group with the
sidebar (plus Kind) and sort tabs (Best match, Most starred, Newest), retired rules labeled. The rule row used
everywhere: library mark, title, impact, stars. The opt-in control on `/libraries` and the browse pages.

### R5. Cart and checkout

The browser cart: keys as the prototype's (`owner/repo::group/slug`, `group::owner/repo::group`), the header badge,
In cart states, the add-to-cart modal on rule pages, and the unvetted confirmation inside it. The `/cart` page with
the three cards, sync or fork per rule, the "also add the rest of the group" upsell, the project picker (signed out:
a repository field; signed in: the visitor's projects once R7 lands), and the Prompt and Commands tabs with Copy.
A JSON endpoint under `/cart/` that resolves keys and returns the texts. Migration: drop `cart_items`. Verified
against the real `code-rules` CLI as slice 8 was.

### R6. Library and rule pages

Library page: the verified check, Added by and On Rulemart since, the Groups tab with checkboxes and the Add groups
panel, All rules with the Retired section, and the Report link kept. The library group page. Rule page: tags, the
Star and Discuss places, the About panel with Published by and Updated, the assets panel, asset pages, and relative
link rewriting as the prototype does. Code highlighting already happens at ingestion.

### R7. Dashboard and add a library

Sign-in with `read:org`, the token kept encrypted per session, `/signin` with the perks. `/me` with My libraries
(published by you and your orgs, with totals; used in your projects from `provenance.json`, with update counts) and
Starred rules. `/me/add` with the picker over the visitor's repositories, the URL form, and the `/me/add/run`
checklist over the existing listing worker. `/me/private` and the GitHub App install, with a webhook or
installation callback that records which repositories the app can see. The project picker at checkout reads the
same projects. Infrastructure: an SSM parameter for the GitHub App's private key, the worker token made required,
and the OAuth secret already planned in infra-live#23.

### R8. Launch gate

Analytics on every page. About, privacy, robots, and the sitemap updated for the routes and data above. The
side-by-side screenshot audit of every prototype route at 1280 and 390 pixels, light and dark, with the prototype
served next to the site; fix the differences. Then [launch.md](launch.md), updated for the merged stack.

## Verification, for every slice

In this order, once the slice is built and its own tests pass:

1. **A fresh browser agent**, with no knowledge of the code, uses `chrome-devtools-axi` against a local Rulemart with
   both real libraries ingested to carry out the slice's use cases the way the prototype's click paths go. It
   reports what was confusing or could be better. Implement those changes.
2. **An architecture pass** with the `improve-code-organization` skill. Act on the findings that merit it, which
   may be none.
3. **Astra** (`codex exec -m gpt-6-astra -c model_reasoning_effort="high"`), at most two rounds, producing findings
   only, focused on how the code could be simpler, how well it follows this repository's Code Rules, correctness,
   and security. Implement at the maintainer agent's discretion.
4. **Devin's review** of the pull request. Resolve its feedback.
5. **Merge.**

Each slice's doc records the Proposed decisions it adds, as the earlier slices' do.
