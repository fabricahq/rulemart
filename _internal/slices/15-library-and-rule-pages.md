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

### Decided while building

The spec's Proposed decisions are built as written, except where an entry here says otherwise and why.

- **Proposed: a rule's own assets are the files in its asset directory, `assets/<rule name>/` beside its file**, such
  as `practices/testing/assets/test-changed-behavior/`, as Code Rules' parser defines the directory a rule's version
  covers, read at the release that published the current version; shared assets are the library-root `assets/` files
  the rule's text, its reading guidance, or its own Markdown files link to, and the files those shared Markdown files
  link to in turn, as the prototype's `linkedShared` follows them, read at the latest release. A link to a shared file
  the latest release doesn't hold stays a link to GitHub. Only current rules have assets; a retired rule's page shows
  none, as the prototype's doesn't.
- **Proposed: bytes are kept only for images and UTF-8 text**, which pages show; any other file, such as a binary
  within the caps, is listed with its size and linked on GitHub, since a page can't show it and Rulemart serves no
  file but an image. A file's type comes from its name for images (PNG, JPEG, GIF, WebP, AVIF, SVG) and Markdown, and
  from its bytes for other text.
- **Proposed: the caps keep bytes in path order while they fit**: a file of 256 KiB at most, while the rule's own
  files kept come to 2 MiB at most, so a later, smaller file can still be kept after a larger one wasn't. The library's
  shared files count as one more rule, 2 MiB together, in the order rules find them. Kept bytes and assets' HTML come
  out of the existing content budget, and a library may list at most 10,000 assets, Code Rules' own limit on a
  library's files, past which ingestion refuses it, as it refuses a library past its other limits. Finding the shared
  files a rule links to holds each link's destination once, however many references name it, and spends the content
  budget as rendering does, since it comes before rendering.
- **Proposed: text assets are stored as highlighted code**, rendered at ingestion with the same highlighter as rules'
  fenced code, by the file's name, as the prototype highlights them. So rendering is now an interface, `Renderer`,
  with Markdown, Code, and Links, rather than one function, and resolving a link moves to the domain, which finds the
  shared files before the rule is rendered.
- **Proposed: links to assets are written at ingestion, and a shared asset's link names its rule as pages read it.** A
  rule's or a Markdown asset's link to a known asset leads to its page; its image loads from Rulemart when Rulemart
  keeps the bytes, and from GitHub otherwise. A shared Markdown file is rendered once for every rule that links to it,
  so pages add `?rule=<rule ID>` to links to shared assets' pages as they're read. A link to an asset keeps its
  fragment and drops any query its author wrote.
- **Proposed: other relative links lead to GitHub at the release that holds the file**: the rule's release tag for its
  own file and asset directory, and the latest release for library-wide files, as the prototype's `ghFileUrl` and the
  renderer already did. The spec says "at the rule's release tag", but a library-wide file isn't part of the rule's
  version, and the newest release holds the copy a project gets.
- **Proposed: stored text names no library.** Assembly renders links to a library's pages and its files on GitHub with
  a placeholder in place of its owner and name, which pages fill in with the library they show, as they add `?rule=`.
  Copied to another library's rows, as a local fixture of an unvetted copy was, or read after a repository is
  renamed, a rule's text still leads only within the library whose page shows it. Text stored before this release
  names its library, as before, until the worker ingests it again.
- **Proposed: an image's bytes are served at its page's address with `?raw=1`**, from the asset table, as the type
  ingestion recorded, with `nosniff`, `Cache-Control: public, max-age=86400`, and a content security policy of its own,
  `default-src 'none'; style-src 'unsafe-inline'; sandbox`, so an SVG a library wrote can't run or load anything even
  opened on its own. A response to a signed-in visitor stays `private, no-store`, as every response to one is. The
  asset page's Raw button leads to GitHub's raw file, as the prototype's does.
- **Proposed: a shared asset's page without `?rule=`, or with a rule that doesn't list it, shows it with the first rule
  in path order that does**, as the prototype's does, and every spelling names its address without `rule` as
  canonical, so search engines index the file once. After browser QA, a shared asset's page says how many other rules
  use it, "used by this rule and N other rules", so the rule it shows with doesn't read as the file's only one.
- **Proposed: the worker ingests again a library stored without tags.** Ingestion writes each version's tags, an empty
  array when it lists none, and its assets in one transaction, so a version with content but no tags marks a library a
  release before this one stored, whose assets are missing too. Tags are read from the frontmatter, text between
  commas or an array of strings, as Code Rules' template writes them, and kept on every version's content.
- **Proposed: "On Rulemart since" for a library stored before this release is the migration's date**, since nothing
  recorded its first ingestion; production's libraries came days before. "Added by" links the lister's GitHub profile,
  with `nofollow`, by the login they last signed in with, and reads "Fabrica", unlinked, for a library vetted without
  a listing. The privacy page says a listing shows its lister's username.
- **Proposed: the Groups tab's controls need JavaScript, as R5 decided for every cart control**, rather than a form
  with a fallback page. The cart lives in the browser, so a page that only lists what to add, without JavaScript,
  adds nothing. The server reads `?sel=` to render the ticked boxes, the box's count, and its button, and `cart.js`
  keeps the address and each row's link to its group page in step as boxes change, so the selection survives a visit
  to a group page and back, whose links carry it too. Adding the groups leaves them out of `sel`, as the prototype
  clears it.
- **Proposed: a library's group rows lead only to the group's page in the library**, as the prototype's, not across
  libraries, partly reversing R4: the library group page offers "See <Group> rules from every library" for a canonical
  group, and a rule's crumbs lead to any group across libraries. Only practices show a blurb, as the prototype.
- **Proposed: the library's title is its repository's name**, beside its owner's avatar with the vetted check mark,
  since Rulemart keeps no display name; its facts are the prototype's plain list, with Report this library under it.
- **Proposed: a retired row on the All rules tab reads "Retired in release/N, replaced by <title> <ID>"** (or
  "renamed to <ID>") in the shared row's gray, after its group's current rules, and a group whose rules are all retired
  shows only with the option on; every list's retired rows name a replacement by title and ID alike, as the prototype's
  name it by ID. The option is a one-checkbox form, as the unvetted opt-in is, which `filters.js` submits, with a
  checkbox the size of the group rows'.
- **Proposed: the rule page's crumbs lead its group to the group across libraries**, as the prototype's, and drop
  R4's separate "rules in every library" link. The About panel's Impact fact and note are gone with the prototype's
  layout, so the head's impact label leads to Code Rules' explanation of the levels, named by what its level means,
  where a touch or a keyboard reaches it.
- **Proposed: "Questions or suggestions?" leads to the library's issues on GitHub** until Discuss arrives with
  rulemart#27, rather than ending in nothing. The Discussion tab, between Rule and Versions, renders nothing yet.
- **Proposed: the engage row shows only when it holds the Star control**, after browser QA, reversing its kept height:
  on an unvetted library's rule page it held nothing, a 46-pixel gap under the title. Discuss adds it back to every
  current rule's page with rulemart#27.
- **Proposed: "Published by" names Fabrica for a library of Fabrica's, and otherwise the owner's login**, since
  Rulemart keeps no display name, on a rule's About panel and every row's library mark alike. "Added by" stays the
  lister's `@login`, or "Fabrica" for a library vetted without a listing.
- **Proposed: a library group page flags a group that isn't canonical** beside its ID in the title, as its rows do.
- **Proposed: sizes read in bytes below 1 KB, then KB and MB of 1,024**, to one decimal place below 100.
- **Proposed: an asset's page takes its Markdown's headings a level down**, after browser QA, so the file's name stays
  the page's one top heading, as a retired rule's page does with its last text. Rendered Markdown still styles a
  level-one heading, which a rule's body can hold when it doesn't repeat the title.

## Not in this slice

- Discussion (rulemart#27), the dashboard's library totals (R7), the brand mark (R2).
- Left as they are after browser QA, deliberately: "On Rulemart since" and every other date reads in UTC, as across
  the site; the owner page's display name waits for R7; and "Select all groups" stays active with every group ticked,
  as the prototype's does.

## Verification

- `make check` and `make check-generated` pass.
- Ingestion tests: assets within and past the caps, shared assets found through links, a Markdown asset's links,
  a missing asset.
- Page tests: every panel's facts, the Groups tab's selection in the address, the Add groups box, the library
  group page and its back link, the asset pages and content types, link rewriting, Added by for listed and vetted
  libraries, the reserved Discuss places.
- In a browser beside the prototype's library, library group, rule, and asset pages at 1280, 390, and 320 pixels,
  light and dark. Done with both real libraries ingested, an unvetted copy of the test library listed, and an SVG and
  a file too large to keep added to a rule: every page without sideways scroll or console errors, and the Groups tab's
  flow with a script: ticking rows, the address and links following, a group page and back with the ticks kept, and
  adding the groups.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
