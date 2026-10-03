# Realignment with the prototype

The `prototype` branch's user experience is the spec for every page of Rulemart. This file says how to run the
prototype, the decisions that shape the site where it departs from the prototype, and the matrix of accepted
differences, route by route. Its last part records how the realignment with the prototype was done.

## Why

The `prototype` branch holds a click-through mock, `prototype/`, that is the user experience Rulemart should have.
Slices 3 to 9 were built from the plans in `slices/`, which name the prototype three times, so the stack took the
prototype's design tokens and little else. The backend is sound and stays: ingestion, release records and version
diffs, sign-in, sessions, the listing worker, vetting, caching, SEO, and security headers. The gap is in
information architecture, interactions, and two data decisions, stars and the cart.

**The prototype is the spec.** A page is done when it behaves as the prototype's does, judged side by side in a
browser, except where a decision below or the conformance matrix says otherwise.

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

Each prototype route, the site's, and how they compare, as slice R8's audit found them on 2026-10-03, side by side at
1280 and 390 pixels, light and dark. [audit/conformance.sh](audit/conformance.sh) takes those screenshots again; its
table lists each case it compares: every public page signed out and signed in, the dashboard's pages signed out, where
the site redirects to sign-in, and signed in, a comparison of two library releases, and, open, the "Sign in to star
rules" dialog, the Add to cart dialog, and checkout's project picker with its box for a new project. "Match" means
the same layout, type, spacing, and behavior, where the pages differ only in their data: the prototype's libraries
are invented, and the site shows the two real ones. Every other difference is named, with the decision that chose it,
so the next person knows it was chosen, not missed, and the ones still open say so.

Differences on every page:

- **Library names.** A Code Rules library declares no display name, so pages name it by its repository, such as
  `public-rules`, or `owner/name` beside rules, where the prototype shows invented names such as "Fabrica Public
  Rules".
- **The header and footer on a phone.** Below 960 pixels, a menu button stands in for the header's links, as slice
  R1's decision says, where the prototype hides them; the footer keeps one line, with About, Feedback, Privacy, and the
  source and theme icons, as R1 decided, and its links and icons are 44-pixel tap targets.
- **Page padding on a phone.** Below 720 pixels, the prototype's `.wrap { padding: 0 16px }` also zeroes `.page`'s 36
  pixels above and 80 below, a slip of the shorthand that puts the content against the header. The site keeps them.
- **Sign-in's GitHub mark.** "Sign in with GitHub" and Continue with GitHub show only when GitHub sign-in is
  configured, so a local `make web-dev` build shows "Sign in" and its test users' box instead.
- **Proposed: dates are absolute**, such as "1 Oct 2026", where the prototype writes "3 days ago". Pages are cached and
  the same for everyone, so a date never goes stale and needs no script.
- **Toasts** show at the viewport's bottom right, and the first star adds an info toast that links to Starred rules,
  as slice R3 decided.
- **Discuss** and the Discussion tabs wait for [rulemart#27](https://github.com/fabricahq/rulemart/issues/27).
- **Search relevance.** The site ranks real rules with Postgres full-text search, as slice R4 built it, so the order of
  equally good matches can differ from the prototype's hand-tuned mock.

| Prototype route | Site | Status |
| --- | --- | --- |
| `#/` home | `/` | Match, except the hero opens with Rulemart's logo, as R2 decided, and lists show vetted libraries |
| `#/browse/techs`, `#/browse/practices` | `/browse/techs`, `/browse/practices` | Match, except rows show no group ID, as R1 decided, and the Include unvetted libraries control, as the vetting decision says. "View other technology groups (N) →" shows, as the prototype's does, only while a library declares a group that isn't canonical, which neither real library does |
| `#/browse/{kind}/other` | `/browse/{kind}/other` | Match, with the opt-in control |
| `#/libraries` | `/libraries` | Match, except it lists vetted libraries, titled "Libraries Rulemart has vetted", with the opt-in control |
| `#/{owner}` | `/{owner}`, `/o/{login}` | Match, except it shows the login and avatar only: the display name, kind, verified domain, and bio wait for a slice that stores them, as R1's doc proposes |
| `#/faq` | `/faq` | Match, except answers that mention Discuss point to the library's repository until rulemart#27 |
| `#/feedback` | `/feedback` | Match, plus "Something broken on Rulemart", as R1 decided; a specific rule's feedback goes to the library's repository until rulemart#27 |
| Header and footer | Every page | Match, except as listed above |
| Brand mark, favicons | Every page | Different, as R2 decided |
| `#/g/{kind}/{group}` | `/g/{kind}/{group}` | Match, except the sidebar's Retired and Unvetted filters, which R4 decided, a library row in stronger type while it's ticked rather than for Fabrica's, as slice R4's doc records, and on a phone the sidebar folds into a Filters disclosure, as R4 decided. The count under the title counts current rules only, and the libraries they come from, even while retired ones show. **Open:** signed in, the prototype's Libraries filter starts with My libraries, which slice R4's doc planned for once the dashboard existed; the site doesn't offer it yet |
| `#/search` | `/search` | Match, plus retired rules in their own section and the sidebar's Retired and Unvetted filters, as R4 decided, with the phone's Filters disclosure, as on group pages, and the ranking named above. **Open:** signed in, it lacks the prototype's My libraries filter, as group pages do |
| `#/cart` | `/cart` | Match, with the vetted check on each vetted library's avatar. The list of items isn't a live region: screen readers hear a change through its toast or the control that made it |
| Checkout's project picker | `/cart`, signed in | Match, plus a line saying when the projects were read, with a link to include private projects. "+ Or use a project that doesn't use Code Rules yet" opens the same box for a new project, and moves focus to its repository field, since the button it replaces goes |
| Library page | `/{owner}/{repo}` | Match, except the Discussion tab waits for rulemart#27, the Report link stays, as R6 decided, and, proposed, a group already in the cart shows its checkbox ticked and disabled, so it can't be added twice. The All rules tab lists every rule on one page, as the prototype's does |
| Library releases tab | `/{owner}/{repo}?tab=releases` | Match, with the compare form, which compares as soon as a release is chosen, and each release's Compare link from slice 4; each card links its "GitHub Release page" |
| None | `/{owner}/{repo}?tab=releases&from=&to=` | Site only, from slice 4: the prototype compares a rule's versions but not two library releases. The page takes the rule comparison's layout: what changed, then each changed rule's text |
| `#/{owner}/{repo}/{kind}/{group}` | The same | Match; the Whole group box keeps the library's `owner/name` on one line |
| Rule page | `/{owner}/{repo}/{kind}/{group}/{rule}` | Match, except Discuss and the Discussion tab wait for rulemart#27, and the About panel says "Questions or suggestions? Ask on GitHub" meanwhile. Star, signed out, opens the "Sign in to star rules" dialog, which matches the prototype's, with "Sign in" for Continue with GitHub on a local build; without a script it goes to sign-in. Add to cart opens a dialog that matches the prototype's. The Assets panel puts "These files come with the rule" under the rule's own files and the release note under the shared ones, so neither speaks for the other's |
| Versions tab and compare view | `?tab=versions`, `&from=&to=` | Match: a choice compares at once, and the line reads "N files changed between release/x and release/y, limited to this rule's file", since the site compares the rule's file, not its assets. The Compare button shows only without a script, and each version row's comparison buttons are as wide as each other |
| Retired rule page | The same address | Match, plus the last version's text, from slice 4 |
| Asset pages | `/{owner}/{repo}/.../assets/{file}` | Match |
| Discussion tabs, Discuss modal | None | Deferred, rulemart#27 |
| `#/signin` | `/signin` | Match |
| `#/me`, `#/me?tab=stars`, `#/me/add`, `#/me/private`, signed out | The same, which redirect to `/signin?return=` | Match: the prototype draws its sign-in page at the dashboard's address, while the site redirects to its own, which returns there after signing in and adds a line saying what for, such as "Sign in to add a library." |
| `#/me`, `#/me?tab=stars` | `/me`, `/me?tab=stars` | Match, plus when GitHub was read, with Refresh, a New tag on a library that came to Rulemart in the last day, and the Account section, as slice R7's doc records |
| `#/me/add`, `#/me/add/run` | `/me/add`, `/me/add/run` | Match, plus when GitHub was read, with Refresh |
| `#/me/private`, `#/gh/install` | `/me/private`, GitHub's own install page | Match; the prototype's mock of GitHub's page has no counterpart |
| About, privacy, robots, sitemap, headers, analytics | `/about`, `/privacy`, `/robots.txt`, `/sitemap.xml` | Site only, updated in R8. About and Privacy take FAQ's layout, under an About label. `make web-dev` names its loopback address as the base URL, so the sitemap answers locally too |

## Record: how the realignment was done

### Slices

Each slice was one pull request from `main`, with a doc in `slices/` numbered from 10, and went through the
verification below before it merged. Each built on the last.

#### R1. Shell and navigation

The header, footer, home, browse pages, owner pages, FAQ, feedback, and the prototype's URLs. No data change.
Routes: `/`, `/browse/techs`, `/browse/practices`, `/browse/{kind}/other`, `/g/{kind}/{name}` (the existing group
page, moved), `/{owner}`, `/faq`, `/feedback`; redirects from `/groups` and `/groups/{kind}/{name}`.

#### R2. Brand

The header mark, favicons, apple-touch-icon, social image, and the README's mark from `brand/`, including Josh's
local header and favicon edits. One pull request with before-and-after screenshots at 1280 and 390 pixels, light
and dark.

#### R3. Stars on rules

Migration: drop `stars`, add `rule_stars (account_id, rule_id, created_at)` with the same grants. The Star control in
the rule page head and the count on every rule row and card; `/account/stars` as a minimal Starred rules list until
R7 builds the dashboard. The star routes move to `/stars` and `/stars/remove`, named by rule.

#### R4. Discovery

Group pages as flat lists with the filter sidebar (libraries, impact, stars, and the unvetted opt-in) and sort tabs
(Most starred, Newest); non-canonical group pages with the Not canonical note. Search grouped by group with the
sidebar (plus Kind) and sort tabs (Best match, Most starred, Newest), retired rules labeled. The rule row used
everywhere: library mark, title, impact, stars. The opt-in control on `/libraries` and the browse pages.

#### R5. Cart and checkout

The browser cart: keys as the prototype's (`owner/repo::group/slug`, `group::owner/repo::group`), the header badge,
In cart states, the add-to-cart modal on rule pages, and the unvetted confirmation inside it. The `/cart` page with
the three cards, sync or fork per rule, the "also add the rest of the group" upsell, the project picker (signed out:
a repository field; signed in: the visitor's projects once R7 lands), and the Prompt and Commands tabs with Copy.
A JSON endpoint under `/cart/` that resolves keys and returns the texts. Migration: drop `cart_items`. Verified
against the real `code-rules` CLI as slice 8 was.

#### R6. Library and rule pages

Library page: the verified check, Added by and On Rulemart since, the Groups tab with checkboxes and the Add groups
panel, All rules with the Retired section, and the Report link kept. The library group page. Rule page: tags, the
Star and Discuss places, the About panel with Published by and Updated, the assets panel, asset pages, and relative
link rewriting as the prototype does. Code highlighting already happens at ingestion.

#### R7. Dashboard and add a library

Sign-in with `read:org`, the token kept encrypted per session, `/signin` with the perks. `/me` with My libraries
(published by you and your orgs, with totals; used in your projects from `provenance.json`, with update counts) and
Starred rules. `/me/add` with the picker over the visitor's repositories, the URL form, and the `/me/add/run`
checklist over the existing listing worker. `/me/private` and the GitHub App install, with a webhook or
installation callback that records which repositories the app can see. The project picker at checkout reads the
same projects. Infrastructure: an SSM parameter for the GitHub App's private key, the worker token made required,
and the OAuth secret already planned in infra-live#23.

#### R8. Launch gate

Analytics on every page. About, privacy, robots, and the sitemap updated for the routes and data above. The
side-by-side screenshot audit of every prototype route at 1280 and 390 pixels, light and dark, with the prototype
served next to the site; fix the differences. Then [launch.md](launch.md), updated for the merged stack.

### Verification, for every slice

In this order, once the slice was built and its own tests passed:

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

Each slice's doc recorded the Proposed decisions it added, as the earlier slices' did.
