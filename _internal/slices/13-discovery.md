# Slice R4: discovery

## Goal

Make finding rules work as the prototype's does: group pages as one ranked list with a filter sidebar and sort
tabs, every group with a page, search grouped by group with the same sidebar, retired rules found and labeled, and
unvetted libraries an opt-in wherever rules or libraries are listed. [realignment.md](../realignment.md) records
the decisions.

## What a visitor sees

- **A group's page** (`/g/{kind}/{name}`): crumbs "Technologies › techs/go" (an uncanonical group adds "› Other
  groups"), the icon and name as the title, a "Not canonical" chip and a note for an uncanonical group, then "N
  rules from M libraries · description". Below, two columns: the **filter sidebar** and the **results**. The results
  head with "N rules in M libraries" and a segmented control, **Most starred** (default) and **Newest**. Each result
  is the prototype's `ruleResult` row: title with impact badge, then the library's mark and name, and "★ N" when
  starred. Empty: "No rules match these filters. Clear filters".
- **The filter sidebar**: **Libraries** (one checkbox per library in the list, with its avatar, name, and count;
  Fabrica's first; "My libraries" when signed in, meaning libraries the visitor's dashboard lists, once R7 exists,
  hidden until then), **Impact** ("Critical and high", "Medium and lower"), **Stars** (Any, 10+, 50+, 100+), an
  **Unvetted** section with "Include unvetted libraries", and "Clear filters" when any is on. Search adds **Kind**
  (Technologies, Practices). Every control is a form field; changing one submits the form, so the page works
  without JavaScript, and a small script submits on change so it feels like the prototype.
- **Search** (`/search?q=`): the eyebrow "Search", the title "Rules matching “q”" or "All rules", the sidebar, and
  results grouped by group in the order of each group's best rule: a group header row (icon, name, ID, count)
  linking to the group's page, then its rules. Sort: **Best match** (default), **Most starred**, **Newest**. Twenty
  results a page, as today. A retired rule that matches shows a "Retired" chip and "replaced by <title>" under its
  title, ranked below current rules with the same score. Empty: "No rules match. Try a broader word, or tell us
  what you were looking for" (feedback).
- **Browse pages** and **the libraries page** gain the same opt-in: "Include unvetted libraries" as a control that
  adds `unvetted=1` to the address; when on, unvetted rows carry an "Unvetted" chip and the browse counts include
  them. `/libraries` keeps its "View unvetted libraries" link for the page that lists only them.
- **Vetted libraries** show the check mark, titled "Vetted by Rulemart", beside their name on the libraries page,
  the home band, owner pages, and library pages (the library page's mark comes with R6; here the lists).
- **Unvetted pages** read "This library has not been vetted. Be sure to review these rules carefully."

## Decisions

- **Proposed: filters and sort are query parameters**: `libs` (comma-separated `owner/name`), `impact` (`high` or
  `medium`), `stars` (10, 50, 100), `kind` (`techs` or `practices`, search only), `sort` (`stars`, `new`, `best`),
  `unvetted=1`. The default value of each is left out of the address, so the canonical address of a group page
  stays `/g/{kind}/{name}` and a filtered page is `noindex`.
- **Proposed: "Newest" orders by the release that first published the rule**, newest first, then stars, since
  Rulemart has no "fresh library" flag; the prototype's `new` sort orders demo-added libraries first.
- **Proposed: ties in the stars sort fall to Fabrica's libraries, then owner and name**, as the prototype's
  `fabricaFirst`, while counts are small. Fabrica's libraries are those owned by `fabricahq`.
- **Proposed: every group has a page.** A canonical group's page reads across vetted libraries (plus unvetted when
  opted in). Any other group ID's page reads the rules of every library that declared exactly that ID, titled by the
  ID, with the chip and the note "techs/golang isn't a canonical group, so it only includes rules from libraries
  that chose this exact name." Rulemart keeps no "similar to" list, so that sentence is left out. The browse pages'
  other-groups rows and the library pages' group rows link to it. Reverses slice 3's "only canonical groups get a
  page" and replaces the per-library sections.
- **Proposed: retired rules are indexed**, using the search document their last version has, and ranked below current
  rules by adding a retired penalty after the score, so a retired rule never outranks a current one that matches as
  well. Reverses slice 4.
- **Proposed: the unvetted opt-in reads listed libraries too**, through the same reads with a flag, and search over
  unvetted rules uses the same document; nothing from an unvetted library is read unless the flag is on. The
  sitemap and the home page never include them.
- **Proposed: one `ruleRow` part** renders every rule listing on the site (search, groups, All rules, starred), with
  the mark, impact, stars, and the retired chip, replacing today's `ruleCard`, so the lists stay identical.
- **Proposed: the sidebar's library counts are counted within the page's unfiltered set**, as the prototype counts
  `base`, not the filtered rows.
- **Existing:** Postgres full-text search, its ranking, paging, `noindex` on search pages, and the search syntax.
- **Decided (Josh):** retired rules look deprioritized, never like another group. The shared `ruleRow` draws a
  retired rule grayed out, the same wherever it appears: muted title and metadata, the Retired chip, and the line
  naming its replacement; `data-retired` marks the row. A group's page has a "Show retired rules" checkbox, off by
  default and `retired=1` in the address when on, which puts the group's retired rules in place in the ranked list,
  grayed, rather than in a section of their own. Search keeps retired rules, ranked below the current rules they tie
  with, in the same style. The library's All rules tab keeps its Retired section until slice R6 replaces it with the
  same option and rows.

### Decided while building

The spec's Proposed decisions are built as written, except where an entry here says otherwise and why.

- **Proposed: one read lists every kind of list.** The store's `Rules` reads a group's rules, every rule, or a
  search's matches, as `domain.RuleList` describes them, from one query, `ListRules`, so the filters, the orders, the
  unvetted opt-in, and the retired rules have one owner. A group's page is a list with a group and no query. It
  filters, orders, and pages in SQL, so a search never moves every match to Go. Each row also carries what the list
  holds before its filters, the sidebar's counts, and when no row passes the filters, a second read of the first
  rule without them supplies the counts. It replaces `GroupRules` and `Search`.
- **Proposed: stars reach the list as parameters.** `CountRuleStars` stays the one statement that counts, as slice R3
  decided: the read counts the current rules of the vetted libraries in scope first, then passes the counts to
  `ListRules`, which filters and orders by them in the same snapshot.
- **Proposed: choices are parsed in one place**, `domain.ParseListChoices`, which reads what each kind of page offers
  (a group's, search, or a list of libraries or groups) and writes it back with every default left out. An address
  that spells its choices any other way, such as a form submitted without a script, a default named, both impact
  bands, a parameter the page doesn't take, or another order of parameters, redirects permanently to the one
  spelling. Libraries keep the order the address gives, lowercase, each once, at most 50. Both of a filter's two
  checkboxes on keep every rule.
- **Proposed: the sort tabs are links**, not form fields: each leads to the page in its order with the same filters,
  which works without a script and can't send two orders. The form keeps the current order in a hidden field.
- **Proposed: without a script the sidebar shows an Apply button**, in `noscript`; `filters.js` goes straight to the
  address the server would redirect the form to, and turns off a filter's other checkbox when one turns on. It
  replaces the page in the history, as the prototype replaces its address, so Back leaves the list rather than undoing
  each choice, and the next page focuses the control that changed, with the quiet ring of a control a page focuses
  after a click.
- **Proposed: search keeps slice 3's tiers, and adds retired rules as a third.** Current rules that hold every word
  come first in every order, then the other current rules under "Rules that match some of your words", then the
  retired rules under "Retired rules", those that hold every word first. Each tier groups its rules by group, in the
  order of each group's first rule, so a group's heading can appear in each. The results head adds "N match every
  word" when some don't. The group heading counts its rules that pass the filters, in its tier, on every page.
- **Decided (Josh): retired rules follow every current rule, in every list.** Search for words keeps finding retired
  rules, but after every current rule, whatever their match, so a retired rule never ranks first for a word a current
  rule also holds; a group's page and every rule put them after the current rules too when shown. This replaces
  ranking a retired rule below only the current rules it ties with.
- **Proposed: every rule offers retired rules as a group's page does.** Search without a query, "All rules", lists
  every current rule, with "Show retired rules" in its sidebar, off by default and `retired=1` in the address, so the
  two pages behave alike. A search for words always finds retired rules, so its sidebar doesn't offer them and its
  address drops `retired`. The sidebar's library counts include retired rules exactly when the list shows them, so
  narrowing a list never raises a count.
- **Proposed: ties fall to vetted libraries before unvetted ones**, so an opted-in list never puts an unvetted rule
  above a vetted one that ties with it. Best match falls to `ts_rank`, then stars, Fabrica's
  libraries, title, owner, and name; the other orders to stars, Fabrica's, owner, name, and title.
- **Proposed: a retired row names the last of its replacements**, following the chain as the rule's page does, so
  "Replaced by" names the rule current now rather than a rule retired since. A rule renamed at every step reads
  "Renamed to `new-id`" in place of "replaced by <title>", since its replacement has its title.
- **Proposed: a group's page lists at most 500 rules** and says so past them, which bounds a group many listed
  libraries share. It isn't paged.
- **Proposed: the other-groups page lists each ID once**, with the libraries that chose it, leading to its page, and
  a library's group rows and a rule's crumbs link every group's page, canonical or not.
- **Proposed: rows show the prototype's `ruleResult`**: title, impact, the library's mark (Fabrica's logo for
  `fabricahq`, else the owner's avatar) and `owner/name`, and stars; the starred list adds the group. The reading
  guidance, version, and source-qualified ID that search results showed are gone, so every list is the same.
- **Proposed: the check mark is the prototype's blue**, a `--vetted` token, the same in both themes, on the avatar of
  a vetted library in lists of libraries and the sidebar; "Vetted by Rulemart" is its hover text, and screen readers
  hear it after the library's name in a row.
- **Proposed: the header's search field holds the query on the search page**; a phone's header has none, so the page
  holds its own field below the narrow breakpoint. The syntax hints are gone, as in the prototype.
- **Proposed: the sidebar names a library by its repository**, beside its owner's avatar, with `owner/name` on hover
  and to screen readers, since the full slug was cut off at the sidebar's width, as the prototype shows a library's
  avatar and name. A ticked library's row is in stronger type; the prototype sets Fabrica's library in stronger type
  instead, which marks nothing while every library on Rulemart is Fabrica's.
- **Proposed: on a phone the sidebar folds into a "Filters" disclosure**, below the narrow breakpoint, closed while no
  filter is on, so the first result sits near the top of the screen rather than under the whole sidebar, and open
  while any is. Its summary counts the choices that are on ("Filters · 2"): each library, the impact, the stars, the
  kind, and retired rules and unvetted libraries, since those change the list too, though they don't open it, as Clear
  filters keeps them, so following a sort tab doesn't push the results down for them. A departure from the prototype,
  whose phone layout stacks the whole sidebar above the results. The desktop and tablet layouts don't change.
- **Proposed: Clear filters clears the filters only**, keeping the order and the unvetted and retired choices, and
  shows only when a filter is on.
- **Proposed: the libraries page reads "Every library on Rulemart"** while it includes unvetted ones.
- **Measured** locally on both real libraries, 133 current rules: a group's page reads in about 4 ms, every rule in
  about 8 ms, and a search in 12 to 17 ms, page included; `EXPLAIN ANALYZE` puts most of a search in building the
  documents of every rule, as before, so a list without a query builds none.

## Not in this slice

- The group page's cart button (R5 and R6), the library page's own mark and panel (R6), "My libraries" until R7.

## Verification

- `make check` and `make check-generated` pass.
- Store and app tests as `rulemart_web`: each filter alone and combined, each sort, the uncanonical group read, the
  retired rule's rank and label, the unvetted flag on and off for every read, the sidebar counts.
- Page tests: every control's address, the current state of each control, Clear filters, the grouped search, the
  retired chip, the empty states, the check mark on vetted rows, the Unvetted chip, `noindex` on filtered pages, the
  canonical address unchanged.
- In a browser beside the prototype's group and search pages, with and without filters, signed in and out, at 1280,
  390, and 320 pixels, light and dark, with and without JavaScript.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
