# Slice R3: stars on rules

## Goal

Move stars from libraries to rules, as the prototype has them: a Star button in every rule page's head, a count on
every rule row, and a Starred rules list for the visitor. Slice 7 built stars on libraries; its mechanics stay
(idempotent POSTs, return paths, the sign-in prompt, notices, no JavaScript) and its subject changes.
[realignment.md](../realignment.md) records the decision.

## What a visitor sees

- **A rule page's head** shows, under the title's impact and version line, the "engage" row: a small ghost button
  reading "Star" with the count, or "Starred" with a filled star when the visitor starred it. Signed out, the button
  is a link to sign in that returns to the rule, with the one-time prompt slice 7 gives after signing in. (The
  Discuss button beside it comes with rulemart#27; leave its place.)
- **Every rule row and card** (group pages, search results, a library's All rules tab, the group sections) shows
  "★ N" at the right when N is above zero, as the prototype's `ruleResult` does.
- **Starred rules** at `/account/stars`: the visitor's starred rules, newest first, as rule rows, or "You haven't
  starred any rules yet. Star a rule from its page." The account menu's entry reads "Starred rules". Slice R7 moves
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
- Then the verification [realignment.md](../realignment.md) sets for every slice.
