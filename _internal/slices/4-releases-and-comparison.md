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
  the rule's versions, or to the comparison of its two versions, and each release to its comparison with the one
  before. A retired rule has a page: when and why it was retired, what replaced it, its last text, and its versions.
  A rule's Rule and Versions tabs name the retired rules it replaced or renamed. The All rules tab lists retired rules
  after the current ones, renamed apart from replaced. The library's "Latest library release" fact leads to the
  Library releases tab.

## Decisions

### Data

- **Every rule version keeps its content. Proposed.** Until now only the current version of a current rule stored
  its file, so nothing could compare it with an older one. Each version now stores the rule as its release published
  it: title, impact, impact description, reading guidance, and the whole Markdown file. Only the current version of a
  current rule stores the HTML its page shows, which still marks it as current. A retired rule's last version stores
  the HTML of its body in a column of its own, `retired_html`, so its page can show it while every read that finds
  current rules by their HTML still finds only those. Other old versions are read for comparison and titles, never
  rendered, so ingestion doesn't spend rendering on them.
- **Ingestion parses every version's file with Code Rules' parser, at the release that published it. Proposed.** A
  release that can't produce a version's file, or one the parser refuses, fails the library's ingestion, naming the
  release and file, as a current version's file already does (**Existing**). Code Rules checks every rule file it
  publishes, so a library it released passes. The content budget now covers every version's file, so ingestion's
  memory stays bounded as before (**Existing**: one ingestion holds at most `ContentLimits.ContentBytes`).
- **The worker ingests again a library stored without its versions' content. Proposed.** The update check already
  ingests a library whose clone URL or tag IDs a release before them didn't store (**Existing**); a stored version
  without content, or a retired rule's last version without its body's HTML, now counts the same way. Production fills in older versions on the first hourly poll after the
  release deploys, and needs no operator backfill or infrastructure change. Until then, a comparison says it doesn't
  have the text yet.
- **Migration 00008 only relaxes a check and adds a column. Existing rule, Proposed content.** It replaces 00002's
  check that a version has all of its content or none, together with its HTML, by two checks: a version has all of its
  content or none of it, and HTML only beside Markdown. It adds `retired_html`, nullable, with a check that it's only
  on a version without `html` and with Markdown. The release still running writes rows that pass both, and rows it stored keep
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
  be 100,000 rows. A page counts each release's changes and versions, and ten rows more for its card, shows releases
  while they fit, and always at least one, the first page starting at the latest release and each next one where the
  one before ends. `?tab=releases&release=<n>` shows the page that holds release n, with the releases newer than it on
  that page, so a link to a release leads to its card among its neighbors; the first page has one address, so a
  release on it redirects there, and the browser keeps the link's fragment. A release of more than 1,000 rules leaves
  out its table of every rule's version and links its GitHub Release page, which lists them, so even a release of
  10,000 rules, the most Code Rules allows, stays within one response.
- **A release names each rule by the title it published then. Proposed.** A rule renamed later keeps its old title in
  the releases before the rename, and a retired rule shows its last title.
- **A retired rule's replacement is followed to a current rule. Proposed.** When the rule that replaced a retired one
  was retired too, every page that names the replacement names the chain to the rule current now: "Replaced by Verify
  retries, itself replaced by Verify retry limits". A retired rule's page, its row on the All rules tab, a release's
  card, and a comparison all follow it the same way, so the reader always reaches the rule that holds the guidance
  today. A release's card and a comparison name the first replacement by its title then. Following stops at a rule it
  already named, so a cycle in a library's records can't loop, and after 20 rules; a chain of more than three names its
  first two rules and its last, and counts the ones between, so a library that retires a rule every release can't make a
  page's work grow with the square of its rules.
- **A rename shows once, as a rename. Proposed.** Code Rules records a rename as a retired rule replaced by a new one.
  When the release that retired a rule added its replacement under the retired rule's last title, Rulemart shows one
  change, under Renamed rules: the new ID, "Renamed from" the old one, and the old rule's last text compared with the
  new rule's. The change names only the new rule's version, since the old one's belongs to another rule, and links
  "Compare the text"; when the text didn't change, the diff says so rather than calling the two files the same. Pages
  say "renamed to" and "renamed from" rather than "replaced by". A replacement under another title stays a retirement
  and a new rule.
- **A linked release or diff stands out. Proposed.** Following a link to a release's card or a rule's diff outlines
  that card, and a page leaves room below its last cards, so the target scrolls to the top even at the page's end.
- **Long lists of changes fold. Proposed.** A section of more than 20 rules shows 10 and folds the rest behind a
  disclosure that counts them, such as public-rules' first release, which adds 127; open, it reads "Show fewer".
- **Each release links its comparison with the release before it, and its tag links its card. Proposed.**
- **"Latest library release" links to the Library releases tab. Proposed.** Each card still links to the release's
  GitHub Release page, which is only its announcement.

### Comparing

- **A comparison is the Library releases or Versions tab with `from` and `to` parameters. Proposed.** A form with two
  selects submits them with GET, so choosing needs no script (**Existing**: the policy's `form-action 'self'`). The
  older one comes first whichever way round they're given, and comparing a release or version with itself says to
  choose two.
- **A release or version a page can't find answers 404 within its library or rule. Proposed.** One that isn't a
  release number or version, or that the library or rule doesn't have, shows the library's or rule's header and tabs,
  the form to choose again, and a link back to the list, with status 404 and `noindex`. A library or rule that doesn't
  exist answers the site's missing page (**Existing**).
- **A comparison says when shared files changed, and offers words or lines only for a diff. Proposed.** Code Rules
  records that a release changed library-wide files, without which ones, so a comparison names the releases in its
  range that did, "release/5 updates shared files", or counts them when there are more than three.
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
  renders at most 10,000 diff rows, blocks, and marks, each diff's panel counting 25 of them, and a diff past that says
  it's too large to show. A comparison of releases that changed more rules than fit says how many it leaves to their
  own comparisons. Diffs take short markup the stylesheet styles.
- **Every page fits one response. Proposed.** A change summary shows at most 1,000 characters, since it's one line of
  any length a library writes. And as a last guard, a page past 5 MiB, which only a library far past any Rulemart knows
  could make, says it's too large instead, and the web function logs `page too large` with its route.
- **Changed words are marked on their own. Proposed.** In prose, a mark covers the words that changed, never the
  space beside them; an insertion is underlined, as a deletion is struck through, so neither relies on color; and a
  deletion and the insertion that replaces it stand apart. In code, a change of indentation is marked after the line
  break, which stays unmarked.
- **The lines view folds the unchanged lines between hunks. Proposed.** A reader can show them, as the words view
  shows unchanged blocks; once shown, the next hunk drops its "@@" header, which would otherwise sit mid-run.
- **Rule text in a diff is text. Existing rule.** A diff shows a rule's Markdown escaped, as segments the template
  escapes, never as HTML Rulemart assembles, so markup in a rule can't run or load.
- **Diff colors are the first colors in the palette. Proposed.** Green and red, after GitHub's diff colors, as tokens
  for light and dark themes, beside the grays (**Existing**: only the tokens are colors), darkened in light themes and
  lightened in dark ones where text needs 4.5:1 contrast. Line numbers on a changed line use the muted gray.
- **Latest and Major look alike, and Major says what it means. Proposed.** Both are small pills. Major links to how
  versions work, and its accessible name says what a major change means, so no one needs to hover. Every link to a
  release's notes on GitHub is named "Release notes", and every link to a release on Rulemart looks the same.

### Retired rules

- **A retired rule has a page. Proposed.** Releases and comparisons name retired rules, so they need somewhere to
  lead. The page shows the rule's last title and version, the release that retired it and why, what replaced or
  renamed it, its last version's text with a link to that file on GitHub, and its versions, which compare as a current
  rule's do. The text's headings sit a level below the section that holds it, so "Rule" and "Evidence" nest under
  "Text of version 1.0.0". The All rules tab lists retired rules after the current ones, in the same order:
  technologies first, by group, then by title.
- **Retired rules stay out of search. Proposed.** Search finds rules to adopt (**Existing**: it reads current rules
  only); a retired rule is reached from its library's pages and from the releases and rules that name it.
- **A retirement's reason names what the library wrote.** In the test library, release/5 retired verify-timeouts
  "Covered by verify-retry-limits." without a `replacedBy` in its record, so the page names no replacement. Code
  Rules' record has that field, and Rulemart shows it whenever a library fills it in.

## Verification

- **Diff tests:** a longest common subsequence on small random inputs, rebuilding both sides on large ones within
  seconds, hunks and line numbers as git writes them, word marks, whitespace-only changes, folding, frontmatter, fenced
  code, and one huge block.
- **Ingestion tests:** every version's content read at the release that published it, a release missing an older
  version's file, the budget spent on older versions, and an update that ingests a library stored without its
  versions' content once.
- **Migration tests:** rows the previous release stores still fit, before and after 00008, while partial content or
  HTML without Markdown doesn't.
- **Store tests**, as `rulemart_web`: retired rules and their replacements, every version's text, texts within the
  limit in path order, a version without text, and unvetted libraries not found.
- **App tests:** each release's changes and every rule's version after it, by the titles they had then, pages of
  releases at the row limit, a comparison across several releases, the older release or version first, and releases
  the library doesn't have.
- **App tests** also cover replacement chains, renames, and a cycle in a library's records.
- **Page tests:** each page's text and links, headings, the forms, both views, noindex, retired rules, renames, the
  404 within a library, shared files, folded lists, the Major mark, hidden lines in the lines view, pages of releases, the
  states a comparison can't show, odd parameters, a rule's raw HTML shown as text in a diff of an ingested library,
  and page sizes: a release of 10,000 rules, thousands of releases that list no rules, a comparison of 4,000 changed
  rules, diffs of every line changed or of thousands of changed paragraphs, a very long summary, and a page past 5
  MiB.
- **Real data, locally:** both production libraries ingested as `rulemart_worker`, a migration of a database slice 3
  stored, and `make worker` ingesting the test library again for its versions' content, then every new page in a
  browser at desktop and phone widths, in light and dark themes, including a fixture library whose rule changes code
  examples and long unbroken lines, which a page test also ingests.
- **After deployment:** the worker's logs show one ingestion of each library with older versions, then none; an HTTP
  smoke check of `/fabricahq/code-rules-test-library?tab=releases` and a comparison.

## Not in this slice

Comparing a rule's assets, and showing a rule's assets at all. A rendered preview of changes. Storing tags' Markdown
release notes. Comparing rules across libraries. Paging through a comparison
of releases that changes thousands of rules, whose list of changes, unlike its diffs, grows with the library. Reading
only one page's releases from the database, rather than a library's whole history. Unvetted libraries, sign-in, listing libraries, stars, discussion, and the cart. Dropping
`hello_messages`, which belongs in its own change.
