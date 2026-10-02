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
  View on GitHub. Signed in, it stars the library and returns to the same page, saying "You starred this library.",
  where it reads Starred, filled, and unstars it. Signed out, it leads to the sign-in page, which says "Sign in to star
  libraries. You'll come back to this one.", and returns to the page, which says "You're signed in. Star owner/name?"
  with the button focused and highlighted, one click from starring.
- **See stars.** The libraries list, on the home page and `/libraries`, shows each vetted library's stars when it has
  any, filled when the visitor starred it.
- **See their stars.** "Your stars" in the account menu leads to `/account/stars`: the libraries they starred, most
  recent first, each with when, and Unstar. A library unstarred there stays named at the top, with Star again.

## Decisions

### What can be starred

- **Proposed: libraries, not rules.** A library is stored by its code host and repository ID, which survive renames and
  transfers, so a star always means the same library. A rule can be retired, renamed, or replaced, which a star would
  have to follow or lose; and slice 8's cart already collects rules to adopt. Starring rules can come later as a second
  table without changing this one.
- **Proposed: a library's own pages star it, not its rules' pages.** A star beside a rule's title would read as starring
  the rule; the rule's page links its library, one click from Star. Devin's review asked for it on rule pages too.
- **Proposed: only vetted libraries.** An unvetted library's pages show no stars, and starring one is missing. A star
  count is a trust signal, which an unvetted library shouldn't borrow, as its pages' `nofollow` keeps it from borrowing
  Rulemart's reputation in search engines. It's also where throwaway accounts would inflate counts. A library that
  loses its vetting keeps its stars, unshown; its starrers see it on their stars page as Unvetted, when a listing
  names it, or No longer on Rulemart, and can unstar it. Vetting it again shows them again.

### Counts and caching

- **Proposed: counts are public**, on a vetted library's pages, whose button always shows its count, 0 included, and its
  row in the libraries list, which shows none at 0, so lists stay quiet until a library has stars.
- **Proposed: pages count stars as they read**, with `count(*)` on an index, rather than keeping a count column.
  Nothing can drift, and at Rulemart's size each count is an index lookup.
- **Proposed: a count may be a minute old for visitors who aren't signed in.** CloudFront keeps their pages for a
  minute (**Existing**), so a new star shows within a minute. A signed-in visitor's pages are never cached
  (**Existing**), so whoever stars sees their own star and the new count at once.
- **Proposed: browsers ask again for every public page: `public, max-age=0, s-maxage=60`.** With `max-age=60`, a
  browser that signed out showed the page it kept from before signing in, with a stale count. CloudFront's cache
  policy honors `s-maxage` between its minimum TTL of 0 and maximum of a year, so it keeps pages a minute as before,
  and answers browsers' requests from its copy; nothing in infrastructure changes. Each page view now reaches
  CloudFront, which it did for any page not seen in the last minute anyway.

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
  and unstarring what isn't starred does nothing, so a double click or a resent form lands where one does. Unstarring
  a library the catalog doesn't have is missing, as starring one is, and the page names it.
- **Proposed: each returns to its `return` parameter, or the library's page.** The button carries the page it's on,
  so starring from the All rules tab or a comparison returns there; a return path is checked as signing in checks one
  (**Existing**), and one that fails the check returns to the library's page. The stars page's Unstar returns to it.
- **Proposed: the page that follows says what happened, by the notice cookie** (**Existing**): "You starred this
  library. It's on Your stars." or "You unstarred this library.", and it focuses the button with `autofocus`, so
  keyboard and screen reader users land where they were and hear its new state, without a script.
- **Proposed: a visitor who isn't signed in gets a link, not a form.** The page is the same for everyone and cached,
  so its Star is a link to `/sign-in?return=<page>?star=1&to=star`, named "Sign in to star owner/name, N stars". The
  sign-in page says why, and promises to come back only when the return path passed its check. A POST without a
  session, such as from a tab whose session ended, does the same and changes nothing.
- **Proposed: signing in to star prompts once, and never stars by itself.** The library's page takes `star=1` off its
  address with a redirect that sets a notice, so the prompt shows once, and reloading or sharing the page doesn't
  repeat it. The prompt names the library, and the button is focused and filled in the primary color, one click from
  starring. A library the visitor starred already gets only "You're signed in." Starring on the way back would need
  the sign-in flow to carry an action, or a GET that writes.
- **Proposed: no JavaScript.** Each star is a form post and a redirect back, which reloads the page with the button at
  its top. A script could save the reload, but the page would then show a count it didn't read.
- **Proposed: a failure to read whether the visitor starred a library fails the page**, with the usual 503, as a
  failed session read does (**Existing**), rather than offering a star they have.

### The stars page

- **Proposed: `/account/stars`, newest first.** It's the visitor's own list, so recency fits better than the owner
  and name order public lists use. Signing in may return to it, as to the listings page; signing out from it returns
  home (**Existing**, extended). Each star says when: minutes or hours ago within a day, so stars made the same day
  show their order, and the date after that, with the exact time in UTC on hover.
- **Proposed: unstarring there keeps the library named, with Star again.** Unstar returns to
  `/account/stars?unstarred=owner/name`, which names the library at the top while the visitor hasn't starred it again;
  the parameter must name a library as GitHub spells names, or the page ignores it.
- **Proposed: the account page says Rulemart keeps which libraries you star, and when, and shows others only the
  counts; deleting the account says it removes your stars.**

### Accessibility

- **Proposed: the button is a toggle, with `aria-pressed`, named "Star, 3 stars" or "Starred, 3 stars"**, while the
  eye reads a star, Star or Starred, and the count. It's as wide either way, and starred, it's shaded and edged in
  ink, with its star filled in ink, not a color: the palette's one amber means caution and nothing else
  (**Existing**). Its hover text says what a star is for, as do the stars page and the account page.
- **Proposed: tabs name their counts apart**, "Groups, 14" rather than "Groups14", with a visually hidden comma, and fit
  a 320-pixel phone, wrapping rather than scrolling if they ever don't, so no tab or focus ring is hidden.

### Text Postgres can't hold

- **Proposed: a path with a NUL byte or bytes that aren't UTF-8 is missing, before any read**, since every name a path
  holds is text the catalog stores, and Postgres would refuse the query. A library to star or unstar named with such
  text is missing too. Other parameters are each page's to read, as search already cleans its query (**Existing**). An
  end-to-end test sends such text in every part of every address, signed in and out, and wants no failure. Browser QA
  found the 503s.

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
  stars page and its sign-in; the menu and account page; a failed read; no sign-in; no stars; the notices, focus, and
  `aria-pressed` after starring; the prompt after signing in to star; tampered returns and names; the visitor's own
  stars in lists; the stars page's times and Star again.
- **End to end against Postgres:** two visitors star, the count shows for everyone, deleting one account takes its star
  away, and unstarring empties the stars page.
- **In a browser, locally:** both libraries ingested, `make web-dev`, then starring signed out through sign-in, the
  starred page, the libraries list, the stars page and its Unstar, the menu, and the account page, at 1280, 390, and
  320 pixels, light and dark, with no console errors.
- **After deployment:** `curl -sI https://rulemart.fabricahq.com/fabricahq/public-rules` shows
  `cache-control: public, max-age=0, s-maxage=60`; signed in, starring returns to the page with Starred and the new count, and
  signed out, the count catches up within a minute.

## Not in this slice

Starring rules. Sorting or ranking by stars. Showing who starred a library. Counting only established accounts. A
WAF rate rule. The cart and checkout.
