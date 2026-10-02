# Slice 4: releases and comparison

## Goal

A visitor sees what each release of a library published, and what changed between any two releases of a library or
any two versions of a rule, with the changed rule text marked word by word or line by line. Before adopting a rule or
updating a library, they can read exactly what they're agreeing to.

Decisions marked **Proposed** are new in this slice and wait for review. **Existing** ones are already in
[decisions.md](../decisions.md) or an earlier slice.

## What a visitor can do

- **Read a library's releases.** `/{owner}/{repo}?tab=releases`, the Library releases tab, lists each release newest
  first, as Code Rules' generated release notes describe it: its tag, date, and a link to its GitHub Release page; a
  sentence counting its changes; the rules it added, changed (major, minor, patch), and retired, each with its
  versions and change summaries; and every rule's version after it.
- **Compare two releases of a library.** `/{owner}/{repo}?tab=releases&from=2&to=6` lists every rule added, changed,
  and retired between them, then each changed rule's text with its changes marked. The Library releases tab chooses
  the two releases with a form.
- **Compare two versions of a rule.** `/{owner}/{repo}/{rule ID}?tab=versions&from=1.0.0&to=3.0.0` lists what each
  version between them changed, then the rule file's changes. The Versions tab compares each version with the one
  before it, and the first with the latest.
- **Switch how changes show.** `view=lines` shows a unified diff with numbered lines; otherwise changed words are
  marked in the Markdown, and unchanged blocks away from a change fold away.
- **Follow history across pages.** A version row leads to the release that published it. A release's change leads to
  the rule's versions, or to the comparison of its two versions. A retired rule has a page: when and why it was
  retired, what replaced it, and its versions. A rule's Versions tab names the retired rules it replaced. The All
  rules tab lists retired rules after the current ones. The library's "Latest library release" fact leads to the
  Library releases tab.

## Decisions

### Data

- **Every rule version keeps its content. Proposed.** Until now only the current version of a current rule stored
  its file, so nothing could compare it with an older one. Each version now stores the rule as its release published
  it: title, impact, impact description, reading guidance, and the whole Markdown file. Only the current version of a
  current rule stores the HTML its page shows, which still marks it as current. Old versions are read for comparison
  and for a retired rule's title, never rendered, so ingestion doesn't spend rendering on them.
- **Ingestion parses every version's file with Code Rules' parser, at the release that published it. Proposed.** A
  release that can't produce a version's file, or one the parser refuses, fails the library's ingestion, naming the
  release and file, as a current version's file already does (**Existing**). Code Rules checks every rule file it
  publishes, so a library it released passes. The content budget now covers every version's file, so ingestion's
  memory stays bounded as before (**Existing**: one ingestion holds at most `ContentLimits.ContentBytes`).
- **The worker ingests again a library stored without its versions' content. Proposed.** The update check already
  ingests a library whose clone URL or tag IDs a release before them didn't store (**Existing**); a stored version
  without content now counts the same way. Production fills in older versions on the first hourly poll after the
  release deploys, and needs no operator backfill or infrastructure change. Until then, a comparison says it doesn't
  have the text yet.
- **Migration 00007 only relaxes a check. Existing rule, Proposed content.** It replaces 00002's check that a version
  has all of its content or none, together with its HTML, by two checks: a version has all of its content or none of
  it, and HTML only beside Markdown. The release still running writes rows that pass both, and rows it stored keep
  passing. Generated search documents now cover older versions too; search still reads only current ones
  (**Existing**).
- **No new grants. Proposed.** `rulemart_catalog_reader` reads the content through its `SELECT` on `rule_versions`
  (00003), and `rulemart_catalog_writer` writes it through its `INSERT` and `UPDATE` (00005). Tests read as
  `rulemart_web` and ingest as `rulemart_worker`, so a missing grant would fail them.

### Releases

- **The Library releases tab is built from stored release records, not the tags' Markdown notes. Proposed.** Code
  Rules generates each tag's notes from its record, which ingestion already stores (**Existing**), so Rulemart lays
  the record out the way the notes do: the same opening sentence, the sections in the same order (new, major, minor,
  patch, retired), the major-change warning, the shared-files note, and the table of every rule's version. Storing
  the notes would need a migration and would let a library show text Rulemart can't check. A first release's notes
  don't repeat "Add the rule." under each rule, and neither does its card.
- **A release that includes a major change is marked Major. Proposed.** The mark matches a major version's on the
  Versions tab, so a reader scanning releases sees which ones to review before updating.
- **Rules are listed in code point order of their IDs. Proposed.** Code Rules' notes do the same, and the database
  orders them with the `C` collation, whatever the server's default.
- **A page of releases holds whole releases, newest first, up to 2,000 rows. Proposed.** Each release's notes list
  every rule's version again, so a library's notes grow with rules times releases: 1,000 rules over 100 releases would
  be 100,000 rows. A page counts each release's changes and versions, shows releases while they fit, and always at
  least one, then links the older ones, from `?tab=releases&until=<n>`, and back to the newest. A link to a release
  elsewhere leads to the page that starts with it. A release of more than 1,000 rules, as large as its library, leaves
  out its table of every rule's version and links its GitHub Release page, which lists them, so even a release of
  10,000 rules, the most Code Rules allows, stays within one response.
- **A release names each rule by the title it published then. Proposed.** A rule renamed later keeps its old title in
  the releases before the rename, and a retired rule shows its last title.
- **"Latest library release" links to the Library releases tab. Proposed.** Each card still links to the release's
  GitHub Release page, which is only its announcement.

### Comparing

- **A comparison is the Library releases or Versions tab with `from` and `to` parameters. Proposed.** A form with two
  selects submits them with GET, so choosing needs no script (**Existing**: the policy's `form-action 'self'`). The
  older one comes first whichever way round they're given, comparing a release or version with itself says to choose
  two, and one the library or rule doesn't have, or that isn't a release number or version, is a missing page.
- **Comparisons carry `noindex` and name no canonical address. Proposed.** As with search (**Existing**), every pair
  would otherwise be a page of its own to a search engine. The tabs themselves name their page's address
  (**Existing**).
- **A comparison of releases spans every release between them. Proposed.** A rule changed more than once shows its
  largest change and each version's summaries; a rule added and retired between them doesn't show, since neither
  release had it.
- **Changes show as the Markdown source with changed words marked, or as a unified diff of lines. Proposed.** The
  prototype's two views. Words is the default, since rule text is prose; lines suit code examples and exact review.
  Rendering the Markdown instead would hide changes to code, links, and frontmatter. Only the rule's own file is
  compared: Rulemart doesn't store a rule's assets.
- **Rulemart computes diffs when a page is read, in the web function, with a bounded diff of its own. Proposed.**
  `internal/lib/textdiff` finds a longest common subsequence exactly for small stretches and anchors larger ones on
  lines or words that occur once on each side, as patience diff does, with a cap on the work of any comparison, past
  which the rest shows as replaced. Precomputing diffs at ingestion would cover only adjacent versions. The Go diff
  libraries bound their work with a timeout, which makes a page's diff depend on the machine, and none splits
  Markdown into blocks, so a dependency would save little of the code.
- **Whitespace in code is compared as it is. Proposed.** In prose, a change of whitespace alone, such as a rewrapped
  paragraph, isn't a change. In a fenced or indented code block, where indentation can change what code does, it is,
  and the words view marks it; the lines view marks changed whitespace in any replaced line.
- **A page compares at most 512 KiB of rule text. Proposed.** Each changed rule's two versions count, in path order;
  a pair past the limit says it's too large to show, and links both files on GitHub, as does a pair whose text the
  catalog doesn't have yet. The store reads only the texts that fit. Ordinary rules are a few KiB, so a comparison
  shows dozens in full. Short text can still make a long diff, such as every line of a file changed, so a page also
  renders at most 10,000 diff rows, blocks, and marks, and a diff past that says it's too large to show. Diffs take
  short markup the stylesheet styles. Together these keep a page well within a Lambda response, however large a
  library's rules are.
- **Rule text in a diff is text. Existing rule.** A diff shows a rule's Markdown escaped, as segments the template
  escapes, never as HTML Rulemart assembles, so markup in a rule can't run or load.
- **Diff colors are the first colors in the palette. Proposed.** Green and red, GitHub's diff colors, as tokens for
  light and dark themes, beside the grays (**Existing**: only the tokens are colors).

### Retired rules

- **A retired rule has a page. Proposed.** Releases and comparisons name retired rules, so they need somewhere to
  lead. The page shows the rule's last title and version, the release that retired it and why, what replaced it, and
  its versions, which compare as a current rule's do. It shows no body: the rule no longer applies, and its last
  version's text is a comparison away. The All rules tab lists retired rules after the current ones.

## Verification

- **Diff tests:** a longest common subsequence on small random inputs, rebuilding both sides on large ones within
  seconds, hunks and line numbers as git writes them, word marks, whitespace-only changes, folding, frontmatter, fenced
  code, and one huge block.
- **Ingestion tests:** every version's content read at the release that published it, a release missing an older
  version's file, the budget spent on older versions, and an update that ingests a library stored without its
  versions' content once.
- **Migration tests:** rows the previous release stores still fit, before and after 00007, while partial content or
  HTML without Markdown doesn't.
- **Store tests**, as `rulemart_web`: retired rules and their replacements, every version's text, texts within the
  limit in path order, a version without text, and unvetted libraries not found.
- **App tests:** each release's changes and every rule's version after it, by the titles they had then, pages of
  releases at the row limit, a comparison across several releases, the older release or version first, and releases
  the library doesn't have.
- **Page tests:** each page's text and links, the forms, both views, noindex, retired rules, pages of releases, the
  states a comparison can't show, odd parameters, a rule's raw HTML shown as text in a diff of an ingested library,
  and page sizes: a release of 10,000 rules, and diffs of every line changed or of thousands of changed paragraphs.
- **Real data, locally:** both production libraries ingested as `rulemart_worker`, a migration of a database slice 3
  stored, and `make worker` ingesting the test library again for its versions' content, then every new page in a
  browser at desktop and phone widths, in light and dark themes.
- **After deployment:** the worker's logs show one ingestion of each library with older versions, then none; an HTTP
  smoke check of `/fabricahq/code-rules-test-library?tab=releases` and a comparison.

## Not in this slice

Comparing a rule's assets, and showing a rule's assets at all. A rendered preview of changes. Storing tags' Markdown
release notes. Comparing rules across libraries. A retired rule's last text on its page. Paging through a comparison
of releases that changes thousands of rules, whose list of changes, unlike its diffs, isn't bounded below the
library's size. Unvetted libraries, sign-in, listing libraries, stars, discussion, and the cart. Dropping
`hello_messages`, which belongs in its own change.
