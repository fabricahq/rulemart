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
