# Slice 7: stars

## Goal

Anyone signed in can star a vetted library, to keep it on a list of their own and to tell others it's worth a look.
Every vetted library's pages, and the libraries list, show how many accounts starred it. Starring works without
JavaScript, and pages for visitors who aren't signed in stay the same for everyone and cached.

Decisions marked **Proposed** are new in this slice and wait for review. **Decided** ones are Josh's. **Existing**
ones are already in [decisions.md](../decisions.md) or an earlier slice. decisions.md said nothing about stars beyond
slice 5's note that deleting an account should remove them, so every choice here is Proposed.

## What a visitor can do

- **Star a library.** A vetted library's pages, every tab and comparison, show a Star button with its count beside
  View on GitHub. Signed in, it stars the library and returns to the same page, where it reads Starred, filled, and
  unstars it. Signed out, it leads to the sign-in page, which says "Sign in to star libraries. You'll come back to this
  one.", and returns to the page, where one more click stars it.
- **See stars.** The libraries list, on the home page and `/libraries`, shows each vetted library's stars when it has
  any.
- **See their stars.** "Your stars" in the account menu leads to `/account/stars`: the libraries they starred, most
  recent first, each with Unstar.

## Decisions

### What can be starred

- **Proposed: libraries, not rules.** A library is stored by its code host and repository ID, which survive renames and
  transfers, so a star always means the same library. A rule can be retired, renamed, or replaced, which a star would
  have to follow or lose; and slice 8's cart already collects rules to adopt. Starring rules can come later as a second
  table without changing this one.
- **Proposed: only vetted libraries.** An unvetted library's pages show no stars, and starring one is missing. A star
  count is a trust signal, which an unvetted library shouldn't borrow, as its pages' `nofollow` keeps it from borrowing
  Rulemart's reputation in search engines. It's also where throwaway accounts would inflate counts. A library that
  loses its vetting keeps its stars, unshown; its starrers see it on their stars page as Unvetted, when a listing
  names it, or No longer on Rulemart, and can unstar it. Vetting it again shows them again.

### Counts and caching

- **Proposed: counts are public**, on a vetted library's pages and its row in the libraries list, where a row shows
  none at zero.
- **Proposed: pages count stars as they read**, with `count(*)` on an index, rather than keeping a count column.
  Nothing can drift, and at Rulemart's size each count is an index lookup.
- **Proposed: a count may be a minute old for visitors who aren't signed in.** Their pages are cached for a minute
  (**Existing**), so a new star shows within a minute. A signed-in visitor's pages are never cached (**Existing**), so
  whoever stars sees their own star and the new count at once.

### Ordering and abuse

- **Proposed: nothing sorts by stars, and there's no "most starred" view.** Libraries are listed by owner and name so
  no library can buy its place (**Existing**); a ranking by stars would reward inflating them. With counts that only
  inform, an account stars a library at most once, and inflating a count takes a GitHub account per star.
- **Proposed: no rate limit on starring.** A star is one row per account and library, so the table is bounded by
  accounts times vetted libraries, and starring and unstarring over and over writes one row each time, within the web
  function's concurrency, as slice 5 reasoned for sign-in. A CloudFront WAF rate rule on `POST` covers it if abuse
  appears (**Existing**). An operator removes an account's stars with SQL, as the database's owner:
  `DELETE FROM stars WHERE account_id = <id>`. If counts start to matter, counting only accounts of a certain age is
  the next step.

### Starring

- **Proposed: `POST /account/stars?library=owner/name` stars, and `POST /account/stars/remove?library=owner/name`
  unstars.** Writes are POSTs with an empty body and their input in the action's query string, refused when another
  site starts them (**Existing**). Paths under `/account` can't hide a library's `/{owner}/{repo}` (**Existing**). The
  library is named as its pages' addresses name it, matched without regard to case.
- **Proposed: two actions, not a toggle, so repeating one is harmless.** Starring a starred library keeps one star,
  and unstarring what isn't starred does nothing, so a double click or a resent form lands where one does.
- **Proposed: each returns to its `return` parameter, or the library's page.** The button carries the page it's on,
  so starring from the All rules tab or a comparison returns there; a return path is checked as signing in checks one
  (**Existing**). The stars page's Unstar returns to it.
- **Proposed: a visitor who isn't signed in gets a link, not a form.** The page is the same for everyone and cached,
  so its Star is a link to `/sign-in?return=<page>&to=star`. The sign-in page says why, and returns to the page. A POST
  without a session, such as from a tab whose session ended, does the same and changes nothing.
- **Known: starring after signing in takes a second click.** Starring on the way back would need the sign-in flow to
  carry an action, or a GET that writes.
- **Proposed: no JavaScript.** Each star is a form post and a redirect back, which reloads the page with the button at
  its top. A script could save the reload, but the page would then show a count it didn't read.
- **Proposed: a failure to read whether the visitor starred a library fails the page**, with the usual 503, as a
  failed session read does (**Existing**), rather than offering a star they have.

### The stars page

- **Proposed: `/account/stars`, newest first.** It's the visitor's own list, so recency fits better than the owner
  and name order public lists use. Signing in may return to it, as to the listings page; signing out from it returns
  home (**Existing**, extended).
- **Proposed: the account page says Rulemart keeps which libraries you star, and when, and shows others only the
  counts; deleting the account says it removes your stars.**

### Accessibility

- **Proposed: the button's name says what it does and the count in words**: "Star, 3 stars", or "Starred, 3 stars.
  Unstar", while the eye reads a star, Star or Starred, and the count. The starred star is filled in ink, not a color:
  the palette's one amber means caution and nothing else (**Existing**).

### Data and roles

- **Proposed: migration 00011 adds `stars`**: an account, a library, and when, keyed by the account and library.
  Deleting the account removes its stars, which slice 5 asked for, so a deleted account's stars stop counting, and
  "Rulemart deletes everything it keeps about you" stays true. A library's row keeps its id across ingestions and
  renames (**Existing**), so a star follows the library; neither function deletes a library, and if an operator does,
  its stars go with it. It only adds a table, which the release still running doesn't read.
- **Proposed: no new role.** A star is an account's, as a listing is, so `rulemart_accounts_writer` gets `SELECT`,
  `INSERT`, and `DELETE`, never `UPDATE`, so no one can backdate one; `rulemart_catalog_reader` gets `SELECT`, to
  count. The worker gets nothing.
- **Proposed: stars live in the catalog context**, beside listings: `app.Stars`, `store.Stars`, and
  `queries/stars.sql`, since counts are page reads of the catalog and a star names a library.
- **Proposed: the web function logs nothing new.** Its access log already records each star's route and status, by
  pattern (**Existing**).

## Infrastructure

None. Migration 00011 grants roles that exist, and nothing in AWS changes: CloudFront already forwards every method,
and the session cookie is already in its cache key.

### Order

1. Release Rulemart with this slice. Its pre-publish workflow runs 00011.
2. Pin the release and apply `web_lambda`. Before then, the running release ignores the table.

## Verification

- **Migration tests:** 00011's grants, and what each role may and may not do with stars.
- **Store and app tests**, as the web function's role: starring once by any spelling, counts on the library's page and
  the libraries list, unstarring only one's own star, starring only vetted libraries, the stars list's order and
  states, and deleting an account removing its stars.
- **Page tests:** the Star link for visitors who aren't signed in, on every tab, and the cached public page; the
  signed-in button and its private page; starring and unstarring returning where they started; repeating either;
  signing in first; return paths; cross-site posts refused; unvetted and unknown libraries; counts in the list; the
  stars page and its sign-in; the menu and account page; a failed read; no sign-in; no stars.
- **End to end against Postgres:** two visitors star, the count shows for everyone, deleting one account takes its star
  away, and unstarring empties the stars page.
- **In a browser, locally:** both libraries ingested, `make web-dev`, then starring signed out through sign-in, the
  starred page, the libraries list, the stars page and its Unstar, the menu, and the account page, at 1280, 390, and
  320 pixels, light and dark, with no console errors.
- **After deployment:** `curl -sI https://rulemart.fabricahq.com/fabricahq/public-rules` shows
  `cache-control: public, max-age=60`; signed in, starring returns to the page with Starred and the new count, and
  signed out, the count catches up within a minute.

## Not in this slice

Starring rules. Sorting or ranking by stars. Showing who starred a library. Counting only established accounts. A
WAF rate rule. The cart and checkout.
