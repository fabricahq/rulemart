# Slice R6: library and rule pages

## Goal

Lay out a library's pages and a rule's pages as the prototype does: the library head and About panel, the Groups
tab with its checkboxes and Add to cart panel, All rules with the Retired section, a page for one library's group,
the rule head with its tags and engage row, the About and Assets panels, and pages for a rule's supporting files.
The Versions tab, the comparison views, the releases tab, and the retired rule page already match and stay.
[realignment.md](../realignment.md) records the decisions.

## What a visitor sees

- **A library's page** (`/{owner}/{repo}`): a large avatar, the library's name as the title, with the vetted check
  mark, the description, and "View on GitHub". Tabs: Groups (N), All rules (N), Library releases (N); Discussion's
  place is kept for rulemart#27. Two columns: the tab's body and the side panels.
  - **Groups**: "Technologies · N" and "Practices · N" lists of rows, each a checkbox label with the group's icon,
    name, ID, the "not canonical" flag when it is one, the practice's blurb, "✓ In cart" when its whole group is
    in the cart, and "N rules ›" to the library group page. The side panel **Add to cart**: "Select whole groups
    to add. You can also add single rules from their pages." or "N groups selected. Whole groups stay in sync with
    <library>.", the button "Add N groups to cart" (disabled at zero), and the chips "Select all groups" and
    "Clear". The selection is carried in `?sel=` so it survives a visit to a group page and back.
  - **All rules**: a section head per group (name and ID) with the group's rule rows. No separate Retired section:
    a "Show retired rules" control, off by default (`retired=1` in the address when on), adds each retired rule back
    in place within its group, grayed out with the Retired chip, "retired in release/N", and "replaced by <title>",
    using the shared `ruleRow` style R4 gives retired rules (**Decided (Josh)**, 2026-10-02: retired rules must read
    as history, not as another group, and the group order must hold).
  - **The About panel**: Owner (link to the owner page), Repository, License, Latest library release, Updated, On
    Rulemart since, Added by (the lister's `@login`, for a listed library; "Fabrica" for a vetted-by-file one), and
    Report this library.
- **A library's group page** (`/{owner}/{repo}/{kind}/{group}`): crumbs (avatar, library, ID), the icon and name, "N
  rules in <library> · blurb", the group's rule rows, "← Back to all groups in <library>", and the side panel
  **Whole group**: "Adds all N <Group> rules from <library>. New rules the library adds to this group arrive when
  you update.", "Add <Group> group to cart" (or ✓ In cart, Checkout, Remove from cart), and for a canonical group
  "See <Group> rules from every library →".
- **A rule's page** (`/{owner}/{repo}/{kind}/{group}/{slug}`): crumbs (avatar, library › group icon and name, ID),
  the title, then the impact badge, the version in monospace, and `#tag` links to search; the engage row with Star
  (R3) and Discuss's place; Add to cart (R5). Tabs: Rule, Versions (N); Discussion's place kept. The Rule tab's
  side panels: **About** (publisher avatar and library, "Published by <owner>", Updated, "Questions or
  suggestions?" with the Discuss place), **Assets** when the rule has any (own files, then "Shared across the
  library" with the note "Not part of this rule's version. Projects get the copy from the newest library
  release.", and "These files come with the rule when you add it."), then Owner, Repository, License, File.
- **An asset's page** (`/{owner}/{repo}/{kind}/{group}/{slug}/assets/{path}` for a rule's own file,
  `/{owner}/{repo}/assets/{path}?rule=` for a shared one): the file name, "Supporting file for this rule, part of
  version X · size" or "Shared file in <library>, used by this rule. Not part of the rule's version; this copy is
  from release/N · size", View on GitHub and Raw, the content (an image, rendered Markdown, or code), "← Back to
  <rule>", and the Assets panel with the current file marked.
- **Links in a rule's text** that point at a known asset lead to its page; images are served from Rulemart; any
  other relative link leads to the file on GitHub at the rule's release tag.

## Decisions

- **Proposed: assets are stored at ingestion**, within the existing content budget: each rule's `assets/<slug>/`
  files and the library-root `assets/` files a rule's text or Markdown assets link to, with their size and type.
  Images and files up to a size cap (256 KiB a file, 2 MiB a rule) are kept as bytes in a new `assets` table;
  larger ones are listed with a GitHub link only. Markdown assets are rendered as rule text is, with raw HTML
  escaped. Migration 00015; grants as rule content's.
- **Proposed: a rule's image is served from Rulemart's own origin** at its asset address, with the content type the
  ingestion recorded and `nosniff`, cached a day. Rulemart has no separate cookieless domain yet; CloudFront's
  static behavior can take these paths later.
- **Proposed: "Added by" names the lister** for a listed library, from the listing row, and "Fabrica" for a library
  `vetted.yaml` named before any listing; "On Rulemart since" is the listing's or the first ingestion's date.
- **Proposed: `?sel=` carries the Groups tab's selection**, as the prototype, and the Add groups button is a form
  that posts the selected group keys to the cart script's handler without JavaScript falling back to a page that
  lists what to add, so the control works either way.
- **Proposed: the engage row and the tab bar reserve Discuss's place** with nothing rendered until rulemart#27, so
  the layout doesn't shift later.
- **Existing:** the Versions tab, comparisons, the releases tab, retired rule pages, impact levels, rendering at
  ingestion, and the unvetted band and robots rules.

## Not in this slice

- Discussion (rulemart#27), the dashboard's library totals (R7), the brand mark (R2).

## Verification

- `make check` and `make check-generated` pass.
- Ingestion tests: assets within and past the caps, shared assets found through links, a Markdown asset's links,
  a missing asset.
- Page tests: every panel's facts, the Groups tab's selection in the address, the Add groups form, the library
  group page and its back link, the asset pages and content types, link rewriting, Added by for listed and vetted
  libraries, the reserved Discuss places.
- In a browser beside the prototype's library, library group, rule, and asset pages at 1280, 390, and 320 pixels,
  light and dark.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
