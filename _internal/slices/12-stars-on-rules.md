# Slice R3: stars on rules

## Goal

Move stars from libraries to rules, as the prototype has them: a Star button in every rule page's head, a count on
every rule row, and a Starred rules list for the visitor. Slice 7 built stars on libraries; its mechanics stay
(idempotent POSTs, return paths, the sign-in prompt, notices, no JavaScript) and its subject changes.
[realignment.md](../realignment.md) records the decision.

Decisions marked **Proposed** are new in this slice and wait for review. **Existing** ones are in
[decisions.md](../decisions.md) or an earlier slice. The spec's Proposed decisions are built as written, except where
a decision under "Decided while building" says otherwise and why.

## What a visitor sees

- **A rule page's head** shows, under the title's impact and version line, the "engage" row: a small ghost button
  reading "Star" with the count, or "Starred" with a filled star when the visitor starred it. As in the prototype,
  the count is left out at 0, and the row shows on the Versions tab and comparisons too, as slice 7's button did. Signed out, the button
  is a link to sign in that returns to the rule, with the one-time prompt slice 7 gives after signing in. (The
  Discuss button beside it comes with rulemart#27; leave its place.)
- **Every rule row and card** (group pages, search results, a library's All rules tab, the group sections) shows
  "★ N" at the right when N is above zero, as the prototype's `ruleResult` does.
- **Starred rules** at `/account/stars`: the visitor's starred rules, newest first, as rule rows (search's result
  rows, now the shared `ruleResult` part), or "You haven't starred any rules yet. Star a rule from its page." The account menu's entry reads "Starred rules". Slice R7 moves
  this list into the dashboard's tab.
- **Library pages** no longer show a Star button or count. The dashboard (R7) will show a library's total.
- **The account page** says Rulemart keeps which rules the visitor starred.

## Decisions

- **Proposed: migration 00013 drops `stars` and creates `rule_stars (account_id, rule_id, created_at)`**, primary
  key on the pair, `ON DELETE CASCADE` from accounts and from rules, an index on `rule_id`, and the same grants as
  00011: `rulemart_accounts_writer` SELECT, INSERT, DELETE; `rulemart_catalog_reader` SELECT; the worker nothing.
  Dropping is safe because 00011 was never released: production is at schema 5. The migration refuses to run if
  `stars` holds any row, so a database that somehow has stars is looked at before they are lost.
- **Proposed: only a current rule of a vetted library can be starred.** A retired rule's page shows no button, and an
  unvetted library's rule pages show none, so listing a repository can't collect a count.
- **Proposed: a rule's count includes stars on the retired rules it replaced**, following `replaced_by` within the
  library as the catalog already does for replacement chains, so a rename keeps its stars. The button acts on the
  current rule; a star on a retired rule stays on it, counted toward its replacement, and listed on the visitor's
  Starred rules as the replacement with "renamed from" when the visitor starred the old one.
- **Proposed: a rule is named in the star routes by library and path**: `POST /stars?library=owner/name&rule=path`
  and `POST /stars/remove?...`, plus `return`; a rule that doesn't exist, is retired, or belongs to an unvetted
  library answers 404 and changes nothing. `/stars` has one segment and GitHub has no account named `stars`.
- **Proposed: counts are read with the rules**, in the same reads that list them: a `count(*)` join grouped by rule
  on the index, or a subquery, in `GetLibrary`, `GroupPage`, `Search`, and the rule page. Measured once on the two
  real libraries; if a page's read grows past a few milliseconds, a counter column is the next step.
- **Proposed: the star button and the row count share one `starBadge` part**, so the filled star, the count's
  formatting (1,234), and the hidden-at-zero rule live in one place.
- **Existing (slice 7):** two idempotent actions, not a toggle; validated return paths; the sign-in link for signed-out
  visitors with a one-time prompt after sign-in; `aria-pressed` on the button; a notice and autofocus after the
  action; signed-in pages private, public pages cached a minute; deleting an account removes its stars.

### Decided while building

- **Proposed: counts are read by one query in each read's snapshot, rather than a join inside each read's query.**
  `CountRuleStars` takes the rule IDs a read found and follows each rule's line, the rule and every retired rule whose
  chain of replacements reaches it, with a recursive query, so counting has one statement instead of a copy in
  `GetLibrary`, `ListGroupRules`, `SearchRules`, and the rule's read. The chain walk itself has no single owner:
  `UnstarRule`, `IsRuleStarred`, and `CountRuleStars` walk a line backward, `ListAccountRuleStars` walks forward to the
  current rule, and `app/links.go` walks forward in Go for the rule pages' replacement chains. `domain.MaxReplacements`
  bounds all five, and tests at both layers hold them to it: the store's
  `TestAStarCountsOnlyWithinTheReplacementsPagesFollow` counts, lists, reads, and unstars a star on each side of the
  bound, and the app's `TestReplacementsFollowABoundedChain` stops a page's chain at it. It runs in the same read-only
  snapshot as the read, one more round trip. Measured locally on both real libraries with 3,914 stars spread over
  public-rules' 127 rules: 1.2 ms for every current rule at once. An index on `rules (library_id, replaced_by)` keeps
  the walk backwards an index lookup.
- **Proposed: an account counts once toward a rule**, however many rules of its line it starred: a count is the
  accounts whose stars count toward the rule, so starring a rule before and after its rename doesn't count twice.
- **Proposed: a rule reads Starred when any of the visitor's stars counts toward it, and unstarring removes them
  all**, on the rule and the retired rules it replaced. Otherwise a visitor who starred the old rule would see the
  new one, listed on their Starred rules, offering to star it again, and couldn't take their star back.
- **Proposed: the starred list says "You starred `old-id`, which this rule replaced."** in place of "renamed from",
  since a star follows a replacement as well as a rename, and the line holds for both, however many rules the chain
  passed.
- **Proposed: a star that counts toward no current rule of a vetted library stays stored, uncounted and unlisted**:
  one on a rule retired without a replacement, or past `domain.MaxReplacements` replacements, or in a library that
  lost its vetting, which lists and counts again if the library is vetted again, as slice 7 kept a library's stars.
  Starring and unstarring refuse such a rule, as the spec says, so only deleting the account removes such a star.
  **Open:** whether the list should show these with a way to remove them, which R7's dashboard tab could do.
- **Proposed: the chain is followed as far as pages follow it.** `MaxReplacements` moves from `app` to `domain`, so
  the store's queries and the rule pages' replacement chains share one bound.
- **Proposed: `stars` is reserved like the account pages.** A one-segment route takes an owner's page, so an owner
  named stars would have theirs under `/o/`, and the sitemap leaves out `stars/remove` as a library's page, as the
  routes' tests require of every route.
- **Proposed: two parts draw every star, rather than one `starBadge` part**: `starFace`, the button's face, Star or
  Starred with the count, and `starCount`, a row's "★ N" with the count in words for screen readers, and nothing at 0.
  The star (`starIcon`), the count's formatting (`formatCount`), and its words (`starCountText`) each have one owner the
  two parts share, and both leave the count out at 0, so the filled star, the formatting, and the hidden-at-zero rule
  look the same everywhere without one part that a label and a class switch between the two. The button is the
  prototype's small ghost button: a quiet outline, and once starred, ink on the surface color, rather than slice 7's
  filled primary button, and it no longer keeps the width of "Starred", which left a gap after "Star".
- **Proposed: without sign-in, or without stars, a rule's page shows its count alone**, with nothing to click, and
  only when it has stars; rows always show the counts the catalog reads.
- **Proposed: Starred rules is as wide as search's results**, which list rules the same way, so a row's library,
  group, version, and stars fit one line at 1280 pixels.

## Not in this slice

- Sorting and filtering by stars (R4), the dashboard tab (R7), a library's total (R7), stars on unvetted rules.

## Verification

- `make check` and `make check-generated` pass.
- Migration tests: 00013's grants, the refusal when `stars` holds rows, what each role may do with `rule_stars`.
- Store and app tests as `rulemart_web`: starring once by any spelling of the library and rule, counts, counts
  through a replacement chain, cross-account isolation, retired, unvetted, and unknown rules refused, the starred
  list's order, and deleting an account.
- Page tests: the button signed out (link) and in (form), on vetted current rules only, counts on every kind of rule
  row, the return paths, repeats, cross-site POSTs refused, the starred list and its empty state, the menu entry,
  the account page's text, a failed read.
- In a browser, beside the prototype's rule page and starred tab, at 1280, 390, and 320 pixels, light and dark.
  Done with both libraries ingested and `test_user` signed in through the dev sign-in: the Star link signed out,
  the prompt after signing in, starring and unstarring, a count through the test library's rename, group, search,
  and All rules rows, and Starred rules, with no sideways scroll and no console error. The prototype's starred tab
  couldn't be shown signed in after a reload, which resets its state, so the list was matched to its `ruleResult`
  rows in `app.js`.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
