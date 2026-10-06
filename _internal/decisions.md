# Rulemart decisions

The product and technical decisions that shape Rulemart, and why. Update an entry when a decision changes, rather
than adding history. "Decided (Josh)" with a date marks a decision made in the week of the launch, where the date
matters.

## Vetting and listings

- **A public library's maintainers can list it.** New libraries are unvetted until vetted. Adding one at `/me/add` picks
  it from the visitor's and their organizations' repositories that publish one, or checks the address a GET form names,
  then POSTs with the repository in its action, for a repository the visitor may push to, as decided below; the worker
  looks the repository up and ingests it within seconds, which `/me/add/run` follows, and the lister can try a failed
  listing again or remove one from My libraries on the dashboard.
- **The web function checks the input and the visitor's write access, and the worker checks the rest.** The web function
  checks that the input names a GitHub repository, as `owner/name` or an address, including a page inside the
  repository, with one trailing `.git` dropped and owner names GitHub refuses, or keeps for its own pages, refused; that
  it's within the limits below; that no listing or visible library has that name; and, by asking GitHub with the
  visitor's own token, that they may push to it. That is the one call it makes to GitHub, never as Rulemart itself,
  whose unauthenticated limit of 60 API requests an hour Lambda functions share. The worker looks the repository up,
  which catches a missing or private one, and ingests it, which catches one that isn't a Code Rules library. The web
  function queues the listing's job at once; if that fails, the failure is logged and alarmed, and the hourly poll
  queues it again.
- **A listed library's failure is the lister's to see, not an alarm.** A job for a listing records why its check
  failed on the listing and succeeds, so SQS doesn't retry it or move it to the dead-letter queue, whose alarm is for
  Rulemart's own failures. A failure Rulemart caused, such as the database being unreachable or GitHub's API
  refusing a request, still fails the job, as a vetted library's failure does.
- **A listing is keyed by the repository's ID once the worker resolves it**, as vetting is, and is unique by the name
  the lister gave, without regard to case, so a renamed repository can't be listed twice under two names.
- **A failed listing doesn't reserve its repository.** When another account lists a repository whose only listing
  failed before its library ever ingested, listing it replaces the failed one, so one bad first check, or one account
  listing a name early, can't keep everyone else out. A refusal says whose listing stands in the way: the visitor's
  own, another's being checked, or another's that's listed, linked with `nofollow`.
- **A lister sees where each listing stands**: Checking while the worker hasn't finished, saying when it was asked for
  and, after three minutes, that it's taking longer than usual and Rulemart checks again within the hour; Listed once
  ingested; Vetted once the release's `vetted.yaml` names it; Failed, with the reason, when it never ingested; and
  Listed with the last check's failure when a later check failed, while its pages keep the last release ingested. A
  reason says what the repository got wrong, as a sentence, never where it happened or a database error, which only
  the worker's log has, and never a raw repository ID. A check asked for after another started records its own
  result, and the older one records nothing.
- **A listing that never ingested is checked again by itself only for a day, and only once GitHub has confirmed its
  repository.** A failure to fetch it may be GitHub's for a moment, and can't be reliably told apart from the
  repository's, so the hourly poll tries it for a day after it was listed or retried; one GitHub has no public
  repository for, or that another listing names, isn't, since checking it costs an API call and the answer won't
  change. Its lister can try again at any time, within the request limits. A listing that ingested is checked every
  hour, failing or not, as a vetted library is, and its pages keep the last good release.
- **An account holds at most 5 unvetted listings, and Rulemart at most 500.** Removing a listing frees its place, and
  a vetted one takes none. Both are checked in the transaction that lists, under an advisory lock, so two requests at
  once can't pass either. The cap bounds the unvetted area, the worker's hourly checks, and what someone with many
  GitHub accounts can add; at it, the add page says Rulemart isn't taking new listings. Ingestion's size and memory
  limits apply to every library, and nothing runs a library's files.
- **A listing records who listed it, and the library's facts say "Added by" and "On Rulemart since"**: the login the
  lister last signed in with, linked to their GitHub profile with `nofollow`, and when they listed it; a library
  vetted without a listing reads "Fabrica", unlinked, and the date it was first ingested. For a library stored before
  listings existed, the date is the migration's, since nothing recorded its first ingestion. The lister can remove
  the listing at any time, after a page that says what removing does, by its state: a listed library leaves Rulemart;
  a vetted one stays, and only the listing goes; a failed or unchecked one is forgotten. Removing it hides the library
  again, its catalog rows stay since neither function can delete a library, and a later listing reuses them without
  ingesting again. Deleting an account removes its listings, so a deleted account's listings can't
  fill the cap, one GitHub user can't list, delete, and repeat until nobody can list, and "Rulemart deletes
  everything it keeps about you" stays true.
- **An account lists or retries at most 20 times a day, and every account together at most 100 times an hour**,
  since each one makes the worker check a repository, and may cost a GitHub API call. `listing_requests` keeps each
  request for a day, and outlives a deleted account, unlinked, so the hourly cap holds however many accounts ask.
  Past either, the add page and Try again say when to come back.
- **An operator removes an abusive listing with SQL**, `DELETE FROM listings WHERE id = ...`, as the database's
  owner, and reads the repository ID to vet from the listing's `host_repository_id`, which a lister never sees.
- **Vetting is a reviewed change to a `catalog/vetted.yaml` file**, which lists each library by its code host and
  the host's repository ID. `main`'s protection guards it, and it ships with each release, so the public
  history shows when and why each library was vetted. A listed library becomes vetted when the release that adds it
  deploys, without being listed again: pages decide vetted or not from the release's list as they read. Its listing
  stays, showing Vetted to its lister, and stops counting toward their limit, and the worker checks it as a vetted
  library. Removing it from `vetted.yaml` returns it to the unvetted area if it's listed, or hides it.
- **Vetting covers a library, including its future releases, and means Fabrica chose to show it, not that Fabrica
  checked every rule.** A major version is declared by the library's maintainer, so pausing vetting on one would add
  nothing. The FAQ and the vetting page say so.
- **Unvetted libraries are an opt-in at every listing.** Home, the libraries page, browse, group pages, and search
  show vetted libraries only, and each carries a control that includes unvetted ones, as a query parameter, so the
  default view never changes and unvetted rows in an opted-in list carry an Unvetted tag. The dashboard and the cart
  show whatever the visitor chose, tagged. `/unvetted`, linked from the libraries page, lists only unvetted libraries
  under the warning. A vetted library shows a check mark, "Vetted by Rulemart". Every unvetted page shows "This
  library has not been vetted. Be sure to review these rules carefully." Rules are instructions that coding agents
  follow, so an unvetted rule is untrusted input for an agent. The warning is a band under the library's name, in
  the one amber the palette has, which means caution and nothing else.
- **A page finds a library that's vetted or listed, and nothing else.** Home, the libraries page, browse, group pages,
  and search read only vetted libraries unless the visitor opts in, and a library's own pages also find one a listing
  names. A library the catalog stores that nobody lists or vets, such as one whose listing was removed, is missing.
  Unvetted pages are cached for a minute, as vetted ones are, so a removed listing's pages can show a minute more.
- **Unvetted rules can go in the cart only after an explicit confirmation**, which the cart records, and the checkout
  prompt names their library to the agent, and asks it to review the rules before following them. An item whose
  library loses its vetting needs confirming again.
- **Unvetted pages carry `noindex`**, and links to them `nofollow`, so listing a repository can't borrow Rulemart's
  reputation in search engines. Their robots tag says `noindex, nofollow`, which covers every link on them, the
  repository's own included, and they name no canonical address. Links an unvetted library's author wrote in its
  rules also carry `rel="nofollow ugc"`.

## Browsing and search

- **Search is Postgres full-text search, in the Neon database Rulemart already uses.** It needs no other service,
  and each search reads one state of the catalog, as pages do, so a result can't name a rule its library's page
  doesn't show. Each current rule version stores a generated `tsvector` of its title, its reading guidance and
  impact description, and its body, weighted in that order, each part cut to a fixed length so a huge rule can't fail
  its library's ingestion. Search adds each rule's group names as it reads: the canonical list's name, passed as a
  parameter, and the name part of the group's ID, never the name a library declares. A word joined with `-`, `/`, or
  `:`, such as `keep-tests-independent` or `techs/go`, also matches the IDs pages show, and `techs/go` never finds
  `techs/goose`. There is no prefix matching. Typo tolerance, through trigram matching, can be added beside it later.
- **Search ranks by where words match, in tiers.** Current rules that hold every word come first in every order, then
  the other current rules under "Rules that match some of your words", then retired rules, those that hold every
  word first, each tier grouped by group. A word scores by its best place: the title, then the group's name or an
  identifier, then the reading guidance or impact description, then the body or the library's name. Equal scores fall
  to `ts_rank`, then stars, vetted libraries before unvetted ones, Fabrica's libraries, and title, owner, and name,
  so the order is stable. A rule that lacks some words names them, and the results head says how many match every
  word.
- **A search query is bounded and cleaned.** The query is `q`, trimmed, with control characters and invalid UTF-8
  turned into spaces; one over 200 characters isn't run; one with no word to find says why. Results are 20 a page,
  and a page past 200, or a page number spelled any other way than its own, redirects or is a missing page that says
  so. Search never logs what was searched. The header's search field holds the query on the search page, and a
  phone's header, which has none, shows a field in the page.
- **Search result pages carry `noindex` and name no canonical address.** Each query would otherwise be a page of
  its own to a search engine.
- **Search covers vetted libraries by default**, and unvetted ones when the visitor opts in, through the same reads
  with a flag; nothing from an unvetted library is read unless the flag is on.
- **Retired rules are found, and always after the current ones.** Search for words finds them after every current
  rule, labeled Retired with the rule that replaced it. A group's page, "All rules" (search without a query), and a
  library's All rules tab offer "Show retired rules", off by default and `retired=1` in the address when on, which
  puts them in place after the current rules. A retired rule looks deprioritized, never like another group: the
  shared rule row grays it, with the Retired chip and a line naming the last of its replacements, "Renamed to
  `new-id`" when every step was a rename.
- **Every group has a page across libraries,** at `/g/{techs|practices}/{name}`. A canonical group's page combines
  every library; any other group's page holds the rules of the libraries that chose that exact ID, says it isn't
  canonical, and points at the canonical group it resembles when the catalog names one. A group's page lists at most
  500 rules, which bounds a group many listed libraries share, and says so past them. Browse pages list canonical
  groups and lead to the others under "Other groups", which lists each ID once with the libraries that chose it.
- **Lists are filtered and sorted by query parameters, and each page has one spelling of its address.** `libs`
  (comma-separated `owner/name`, in the order given, lowercase, each once, at most 50), `impact`, `stars`, `kind`
  (search only), `sort`, `retired`, and `unvetted=1`, with each default left out, so a group page's canonical address
  stays `/g/{kind}/{name}` and a filtered page is `noindex`. An address that spells its choices any other way
  redirects permanently to the one spelling. Sort tabs are links, and the filter sidebar's form submits on change
  with a script that replaces the history entry, and shows an Apply button without one. Clear filters clears the
  filters only, keeping the order and the unvetted and retired choices.
- **Group pages and search sort by stars or newest, and ties fall to a fixed order.** Group pages default to Most
  starred and search to Best match; both offer Most starred and Newest. "Newest" orders by the release that first
  published the rule, then stars. Ties in the stars sort fall to Fabrica's libraries, which `fabricahq` owns, then
  owner and name, while counts are small.
- **The filter sidebar** offers Libraries, Impact, Stars, retired rules, and the unvetted opt-in. A library is named
  by its repository beside its owner's avatar, in stronger type while ticked, with `owner/name` on hover and to
  screen readers, and the counts are counted within the page's unfiltered set, so narrowing a list never raises a
  count. Signed in, the Libraries filter starts with My libraries, the libraries the dashboard lists: those the
  visitor and their organizations publish, and those their projects use; signed out, `mine=1` is ignored. On a phone
  the sidebar folds into a "Filters" disclosure, closed while no filter is on, whose summary counts the choices.
- **A page that lists rules from more than one library names each rule's library**, by its mark, Fabrica's logo or
  its owner's avatar, and `owner/name`, in the one rule row every list shows: title, impact, library, and stars, and
  the starred list adds the group. A group page's count under its title counts current rules only, and the libraries
  they come from, even while retired ones show.
- **Libraries are listed by owner and name wherever several appear**, so no library can buy its place. A Code Rules
  library declares no display name, so pages name it by its repository, such as `public-rules`, or `owner/name`
  beside rules. The libraries page is titled "Libraries Rulemart has vetted", and "Every library on Rulemart" while
  it includes unvetted ones.
- **Pages live at the addresses a visitor would guess**: `/browse/techs`, `/browse/practices`, `/browse/{kind}/other`,
  `/g/{kind}/{name}`, `/{owner}`, `/{owner}/{repo}`, `/{owner}/{repo}/{kind}/{group}`, a rule at
  `/{owner}/{repo}/{kind}/{group}/{rule}`, `/cart`, `/signin`, `/me`, `/me/add`, `/me/private`, `/faq`, and `/feedback`.
  Older addresses, such as `/groups`, `/sign-in`, `/list`, and `/account/...`, redirect permanently, keeping the
  query, and `/browse` redirects to `/browse/techs`.
- **No page has a trailing slash or a second spelling.** A path with a trailing slash redirects to the path without
  it. A group's or rule's ID in another case redirects to its own spelling, and so do the site's sections, except
  where an owner named like one has a library page. A path starting with two slashes never redirects off the site,
  and a path with a NUL byte or bytes that aren't UTF-8 is missing before any read.
- **The site's one-segment pages are reserved from owners, and an owner whose login is one is at `/o/{login}`.**
  `browse`, `g`, `o`, `groups`, `libraries`, `search`, `unvetted`, `about`, `privacy`, `faq`, `feedback`, `cart`, and
  the account pages, such as `me`, `signin`, `signout`, `account`, `stars`, and `list`, are reserved; GitHub has users
  named `g`, `faq`, `browse`, `list`, `o`, and `me`. `/o/{login}` is canonical for those, and every other owner's
  `/o/{login}` redirects to `/{login}`. A library's page has two segments, a library group's four, and a rule's at
  least five, so under a section only its own pages are the site's, and every other path reaches the library and rule
  pages: a library owned by `faq` stays at `/faq/{repo}`. The sitemap leaves out a library whose page one of the
  site's own takes, and a route test checks that every route under a segment a login could spell takes only these.
- **An owner page exists for an owner with a vetted library**, and answers 404 otherwise, even for an owner with a
  listed, unvetted library, so listing a repository can't create a page under Rulemart's address. It shows the login,
  the avatar the catalog stores, and the owner's vetted libraries; the display name, kind, verified domain, and bio
  wait for a change that stores them.
- **The home page leads with a search field and the groups people look for.** "Popular" names the two technologies
  and two practices with the most rules, since Rulemart has no traffic data yet. Technology and practice tiles sort
  by rule count, then name, and say "N rules · M libraries"; below 384 pixels they stand in one column. The libraries
  band shows the first four vetted libraries in owner and name order, and "List your library" leads to `/me/add`, or
  to sign-in with a return to it.
- **Browse pages list canonical groups that hold current rules in a vetted library**, by rule count, then name, with
  "N rules" and "M libraries" and no group ID. A practice's row shows its description and a technology's doesn't, so
  a row stays one line; on a phone its counts drop under its text, and names wrap only between words. "View other
  technology groups (N)" shows only while a library declares a group that isn't canonical.

## Groups

- **Code Rules owns the canonical group list; Rulemart pins and reads it.** `catalog/canonical-groups.yaml` is
  Code Rules' file at a pinned release, copied unchanged, and the vendored parser in `internal/lib/coderules` reads
  it, so Rulemart and Code Rules can't disagree about which IDs are canonical. Rulemart updates the pin deliberately,
  in its own pull request.
- **A group is canonical only when its ID is on the list exactly. The list has no aliases**: a library's
  `techs/golang` is a group of its own, never `techs/go`.
- **Pages name a canonical group by the list's display name, and any other group by its ID, flagged "not
  canonical".** They never show the name a library declares for a group, so a library can't rename a group every
  library shares, or pass off its own group as one by naming it like one. A practice's description is the list's,
  never a library's, for the same reason.
- **Canonical status is decided when pages read, not stored at ingestion.** The list ships with each release, as
  `vetted.yaml` does, and holds under a hundred IDs, so applying it to a page's groups costs a map lookup each.
  Storing it would need a migration and a reingestion of every library whenever the pin moves, and a stored flag
  could disagree with the list the running release ships. A later query across libraries, such as browsing one
  canonical group, can pass the list's IDs as a parameter, as the page reads pass the vetted libraries.
- **Rulemart owns the groups' icons**, in `catalog/group-icons.yaml`: Devicon logos (MIT) for technologies and
  Lucide icons (ISC) for practices, vendored with their licenses, only for canonical groups, and only the files it
  names. Where Devicon's logo doesn't read at a tile's size, a project's own one-color logo, vendored under
  `community/` with its source, takes its place. Zustand's tile shows the bear cicero-mello contributed in
  pmndrs/zustand#1623 in two tones, a colored logo on the usual tile in both themes, until the project publishes an
  official vector. A canonical group without an icon shows its initial: Devicon has no logo for Goose, TanStack
  Query, or TanStack Router, so they show initials until it does. An icon drawn mostly in dark colors keeps a light
  tile in dark themes, rather than being inverted as monochrome icons are. Pages show icons with `<img>`, and tests
  reject an SVG that holds scripts, event handlers, or references outside itself.

## Library and rule pages

### Library pages

- **A library's page is titled by its repository's name**, beside its owner's avatar with the vetted check mark, since
  Rulemart keeps no display name, with its description and "View on GitHub". Its tabs are Groups (N), All rules (N),
  and Library releases (N). The About panel lists Owner, Repository, License, Latest library release, Updated, On
  Rulemart since, Added by, and Report this library. Owner leads to the owner's page, or, for an unvetted library,
  which has none, to the owner on GitHub; "Latest library release" leads to the Library releases tab.
- **"Published by" names Fabrica for a library of Fabrica's, and otherwise the owner's login**, on a rule's About
  panel and every row's library mark alike, since Rulemart keeps no display name. "Added by" stays the lister's
  `@login`, or "Fabrica" for a library vetted without a listing.
- **The Groups tab selects whole groups to add to the cart.** Its ticked groups live in `?sel=`, so they survive a
  visit to a group's page and back, and adding the groups clears them. Its controls need JavaScript, as every cart
  control does, since the cart lives in the browser. A group the cart holds shows ticked and disabled, so Select all
  groups, the count, and the button cover only what the box would add. On a phone, a bar at the viewport's bottom
  adds the ticked groups. A group's row leads only to the group's page in that library, and only practices show a
  blurb.
- **All rules lists a section per group**, and shows retired rules in place, grayed, when asked, each reading
  "Retired in release/N, replaced by <title> <ID>", or "renamed to <ID>".
- **A library's group page exists at `/{owner}/{repo}/{kind}/{group}`**, with crumbs, the group's rules, a Whole group
  panel that adds the group to the cart, and, for a canonical group, "See <Group> rules from every library". It flags
  a group that isn't canonical beside its ID.

### Rule pages

- **A rule's page** has crumbs that lead its group to the group across libraries, the title, the impact badge, the
  version, and `#tag` links to search; the engage row; Rule and Versions tabs; and About, Assets, and facts panels.
  The impact label leads to Code Rules' explanation of the levels, named by what its level means, so touch and
  keyboard visitors reach it too. A rule's reading guidance can hold inline Markdown, so ingestion renders it with the
  body's renderer, and pages that show text, such as results and descriptions, show its text without the syntax.
- **Discussion and Discuss launch later**, tracked in [rulemart#27](https://github.com/fabricahq/rulemart/issues/27).
  The tab bar and the engage row reserve their place and render nothing, the engage row showing only when it holds the
  Star control, so the layout doesn't shift later. Until then "Questions or suggestions?" leads to the library's issues
  on GitHub, a specific rule's feedback goes to the library's repository, and the FAQ's answers that mention Discuss
  say so.
- **A retired rule has a page**: its retirement, its chain of replacements to a current rule, its last text, and its
  versions. A rename, which Code Rules records as a retirement and a new rule under the same title, shows as one.
  The text's headings sit a level below the section that holds it.

### Assets

- **A current rule's assets are stored at ingestion**: the files in its asset directory, `assets/<rule name>/` beside
  its file, at the release that published its current version, and the library-root `assets/` files its text or
  Markdown files link to, and the files those link to in turn, at the latest release. Ingestion keeps the bytes of
  images and UTF-8 text within 256 KiB a file and 2 MiB a rule, in path order while they fit, the shared files
  counting as one more rule, out of the content budget, renders Markdown with raw HTML escaped and text as
  highlighted code, and lists any other file with its size, linked on GitHub. A retired rule shows no assets. A
  library may list at most 10,000 assets and its rules 50,000 links to them, past which ingestion refuses it.
- **A rule's links to its assets lead to their pages, its images load from Rulemart, and any other relative link
  leads to GitHub** at the release that holds the file: the rule's release tag for its own file and asset directory,
  and the latest release for library-wide files. Links are written at ingestion with a placeholder for the library, so
  stored text names no library and a rule's text leads only within the library whose page shows it. A shared asset's
  page without a rule, or with one that doesn't list it, shows it with the first rule in path order that does, names
  its address without `rule` as canonical, and says how many other rules use it.
- **Rulemart serves only images from a library, from its own origin**, at an asset's page address with `?raw=1`, as
  the type ingestion recorded, with `nosniff`, cached a day, under a content security policy that loads and runs
  nothing in a sandbox, so no library can serve a page, or a script, from Rulemart. An asset's page takes its
  Markdown's headings a level down, and the Raw button leads to GitHub's raw file. Sizes read in bytes below 1 KB,
  then KB and MB of 1,024. The Assets panel puts "These files come with the rule" under the rule's own files and the
  release note under the shared ones, so neither speaks for the other's.
- **Ingestion holds a hostile library within bounds.** It refuses more than 16 MiB of advertised references, gives a
  rule one second to highlight its code, shows code past that escaped, checks the job's deadline between rules and
  release records, and visits at most 100,000 tree entries and 64 directory levels listing asset directories. The
  worker's 2,048 MB of memory and 120-second timeout, with each job stopping 10 seconds before it, mean a library at
  the limits fails its job rather than the function.

### Releases and comparison

- **Every rule version keeps the file its release published**, with its title, impact, reading guidance, and tags, so
  pages can compare any two versions, name a retired rule, and link a rule's tags to search. Only a current rule's
  current version keeps the HTML its page shows, and a retired rule's last version keeps its body's HTML in
  `retired_html`. The worker ingests again a library stored before this, so production fills in older versions by
  itself, and a comparison says it doesn't have the text yet until then.
- **A library's releases are built from their stored release records**, laid out as Code Rules' generated release
  notes are, rather than from the tags' Markdown notes, which a library could fill with text Rulemart can't check.
  They're paged, newest first, at most 2,000 rows a page, since each release lists every rule's version again, and a
  release of more than 1,000 rules leaves out its table of every rule's version and links its GitHub Release page.
  A release is marked Major when it includes a major change, names each rule by the title it published then, lists
  rules in code point order of their IDs, folds a section of more than 20 rules behind a count, and links its
  comparison with the release before it.
- **A rename shows once, as a rename, and a retirement's replacement is followed to a current rule.** When the release
  that retired a rule added its replacement under the retired rule's last title, Rulemart shows one change, under
  Renamed rules, comparing the old text with the new. A chain of replacements is followed to the rule current now,
  stopping at a rule already named and after 20, and naming only its first two and last when it's longer than three.
- **Comparisons are the Library releases and Versions tabs with `from` and `to` parameters**, chosen with a GET form
  that compares as soon as a choice is made with a script, and carry `noindex` without a canonical address, as
  search does, since every pair would be a page of its own. The older release comes first whichever way round they're
  given; one the library or rule doesn't have is a 404 inside the library's or rule's own page. A release comparison
  spans every release between the two and names the ones that updated shared files; the Versions tab says "N files
  changed between release/x and release/y, limited to this rule's file", since only the rule's file is compared.
- **Diffs are computed when a page is read**, by `internal/lib/textdiff`, a bounded diff that marks changed words in
  Markdown blocks or shows a unified diff of lines, within 512 KiB of rule text and 10,000 rendered rows and marks
  per page, and a page past 5 MiB says it's too large. Words is the default, since rule text is prose; lines suit
  code. A rule's text in a diff is always escaped. In prose a change of whitespace alone isn't a change, and in code
  it is. Insertions are underlined and deletions struck through, so neither relies on color, and the lines view folds
  unchanged lines between hunks.

## Stars

- **Anyone signed in can star a current rule in a vetted library.** A star is an account's mark on a rule. Rules in
  unvetted libraries and retired rules show no Star button, so listing a repository can't borrow a count. Libraries
  have no stars of their own: the dashboard shows a library's total as the sum of its rules'.
- **A star follows a rule through its renames.** A rule's count includes stars on the retired rules it replaced,
  following the chain as far as pages follow it, `domain.MaxReplacements`; an account counts once toward a rule; a
  rule reads Starred when any of the visitor's stars counts toward it, and unstarring removes them all. The starred
  list says "You starred `old-id`, which this rule replaced." for a star on an older rule.
- **Counts are public and counted as pages read.** One query, `CountRuleStars`, counts for every read in the read's
  own snapshot. Every rule row shows "★ N" when N is above zero, and a rule page shows its count, up to a minute old
  for visitors who aren't signed in, as every cached page is; without sign-in, or stars, the count has nothing to
  click. Group and search pages offer a Most starred sort and a stars filter. Starring has no rate limit, since a star
  is one row per account and rule.
- **Starring is a POST to `/stars`, and unstarring to `/stars/remove`**, each naming the rule by library and path in
  its query string and returning to the page, which focuses the button; repeating either changes nothing, and a rule
  that doesn't exist, is retired, or is in an unvetted library answers 404. A visitor who isn't signed in gets a link
  that signs them in and returns them, prompted once to star the rule, and sign-in never stars by itself; with a
  script, the link opens a "Sign in to star rules" dialog first.
- **Only a visitor's first star says anything.** Starring and unstarring set no notice, since the button changing is
  the feedback; a first star shows an info toast, "You starred your first rule!", that links Starred rules.
- **The dashboard's Starred rules tab lists a visitor's stars**, newest first, as the same rule rows, and apart, under
  No longer counted, any star that counts toward no current rule of a vetted library, saying why: the library is no
  longer on Rulemart or isn't vetted, or the rule was retired without a replacement or its replacements end at a rule
  that isn't current. Unstar removes it, and such a star stays stored, uncounted, until then.
- **Deleting an account removes its stars**, so they stop counting.

## Cart and checkout

- **Anyone collects rules in a cart that lives in their browser**, in `localStorage` under `rulemart-cart`: one rule,
  or a whole group of one library, at most 100 items, the header's badge counting a whole group as one. Nothing asks
  for sign-in to add, and the cart outlasts signing out, since it's the browser's, not the account's. A script paints
  the header's badge and each page's In cart state from the stored keys, so public pages stay identical and cached for
  everyone. `cart.js` owns the cart and the page script changes it only through its API. A key the browser holds
  that names nothing is dropped, and an item whose rule is retired or gone, or whose library left Rulemart, stays
  and says so, and checkout leaves it out. Clear cart empties it at once, without asking.
- **Adding opens a modal on the rule page** that offers just the rule or its whole group, and asks for a confirmation
  when the library is unvetted, which the cart records for the library. Items are named by library, group, and rule
  ID, so checkout resolves them against the catalog and says when one is retired, gone, or in a library that lost
  its vetting, and leaves it out. A rule whose whole group the cart holds is included in it, and its page says so.
- **`/cart` is one server-rendered page that a script fills from the checkout's answer.** The cart's keys and
  choices go to a JSON endpoint; without JavaScript, the page explains the cart needs it. Each change dims the prompt
  and commands at once and turns Copy off until the answer for that revision arrives, and a failed update says so with
  Try again, so the page never offers text the cart has outgrown. A whole group's row lists its first 5 rules and
  "+N more", and every title the checkout sends is cut to 200 characters, so its answer always fits a Lambda
  function's response.
- **Checkout is a page that asks a JSON endpoint for the cart's items, each library's latest release, and the
  texts.** It shows a Prompt tab and a Commands tab, both built from `code-rules project add library`,
  `code-rules project add rule --from`, and `code-rules project sync`; each rule can stay in sync or be forked, and
  a group always stays in sync. By default nothing is pinned, so rules move when the project runs
  `code-rules project update`; the Commands tab says how to pin with `ref`. The prompt holds no text a library
  wrote, so no library can write instructions into it: it names rules by ID, not title, a group by its canonical
  name or ID, and a library by `owner/name`. It says it needs Code Rules 0.2.0 or later, and it names each unvetted
  library so the agent reviews its rules first, pinned to the commit reviewed.
- **The commands run in the order the CLI needs.** The commands add the libraries, sync, fork, then run
  `code-rules project build`, since `add rule --from` refuses until its source is synced, and end with a reminder
  that AGENTS.md tells agents to read `.code-rules/generated/RULES.md`. A library's alias is its owner lowercased
  without a trailing `hq`, made unique by adding the repository's name, and a known project's own source names win.
  Every command is checked against the real CLI.
- **An unvetted library is reviewed outside the project, and its items are pinned, not synced.** Its `add library`
  command carries `--ref <commit>`, and the Commands tab shows two blocks, "1. Fetch and review", which fetches each
  such library at the reviewed commit outside the project, and "2. After you've reviewed, import", so pasting one
  block never imports rules nobody read. Its items say "Pinned to the reviewed commit", and its rules can't be forked
  yet, since `add rule --from` finds the release in the repository's tags, which a publisher could move after the
  review. A cart of vetted libraries keeps one block.
- **The checkout request is the site's one request with a body, so it sends the body's SHA-256 in
  `x-amz-content-sha256`.** CloudFront's origin access control signs a request's body only when the viewer sends
  that header, and the function URL refuses an unsigned body with 403; every other write is a POST with an empty
  body. `cart-checkout.js` hashes the exact body with Web Crypto, and sends the request without the header in a
  browser that has none. Without the header, the cart's page can't load in production, though nothing local notices.
  The content security policy allows `connect-src 'self'` for it.
- **Signed in, checkout offers the visitor's projects**, the same as the dashboard's Projects tab, so the prompt names
  the repository and says which libraries it already imports. A line says when the projects were read, with a link to
  include private ones, and "+ Or use a project that doesn't use Code Rules yet" opens the same box for a new project.
  Signed out, checkout asks for the repository the visitor writes, so the prompt names it.

## Accounts and dashboard

### Sign-in and sessions

- **GitHub is the only sign-in provider, through the OAuth app "Rulemart", which asks for `read:org`.** Rulemart
  reads the user's ID, login, name, avatar, and organizations, and keeps the token encrypted in the session row, with
  AES-GCM under a key from SSM, so the dashboard can read the visitor's repositories again, until sign-out deletes
  it. An account is keyed by GitHub's numeric user ID, since logins change, and keeps that, the login, the profile's
  name, and the avatar's address, refreshed at each sign-in, plus when it was made and last signed in. Private
  repositories need the GitHub App "Rulemart by Fabrica" (Contents read, Metadata read, installable on any account),
  which the visitor installs from the dashboard; a GitHub App's user token sees only organizations it's installed on,
  so it can't replace the OAuth app for the first view. The flow uses state and PKCE, kept in a ten-minute `__Host-`
  cookie, happens on the public origin, and only the sign-in page's content security policy lets a form lead to GitHub.
- **Sessions live in Postgres, by the SHA-256 of a random token** the `__Host-rulemart-session` cookie holds: Secure,
  HttpOnly, SameSite=Lax. A session lasts 30 days and is never extended, so no page view writes to Neon; each sign-in
  replaces the browser's session and deletes every expired one, and an account keeps at most 20. Sessions and stars
  are never updated, so no one can extend a session or backdate a star. A failure to read the session fails the page
  with a 503, rather than showing a signed-in visitor a signed-out page. Signing out everywhere and deleting an
  account act only for a live session.
- **Return paths are paths on this site only**: one `/`, no backslash or control character, no scheme or host, within
  2,000 bytes, and not a sign-in page or under `/account/`; anything else returns to `/`. After signing out, signing
  out everywhere, or deleting an account, the next page says so once, from a one-time notice cookie that names one of
  Rulemart's own notices, never text to show.
- **Public pages are `public, max-age=0, s-maxage=60`**: CloudFront keeps them a minute, and browsers ask it again
  each time, so a browser that signs in or out never shows a page it kept from before.
- **A page for a signed-in visitor is never cached.** Any response to a request with the session cookie, or that sets
  a cookie, is `private, no-store`; CloudFront keys its cache on the session cookie too; and other pages vary with
  `Cookie` in browsers. Pages for everyone stay public and identical, so signed-out traffic keeps the cache. A notice
  after signing out comes from a one-time cookie, which CloudFront also keys on, so it never needs a query string.
- **Writes are POSTs with empty bodies, refused when another site starts them**, by `Sec-Fetch-Site` or `Origin`.
  CloudFront can't forward a body a browser didn't hash for origin access control, so forms carry their input in the
  action's query string, and there's no CSRF token. The function applies no rate limit to sign-in: each needs a real
  GitHub authorization, an account keeps 20 sessions, and Lambda's concurrency bounds the load on Neon.
- **Without `GITHUB_CLIENT_ID`, pages offer no sign-in.** GitHub sign-in refuses to start without a token key, and the
  GitHub App's five variables are all or nothing. Local builds with the `rulemartdev` tag sign in test users instead,
  and serve a fake GitHub; release builds can't include that, a test checks the release binary, and a dev build
  refuses to start on Lambda.
- **Signed out, the dashboard's pages redirect to `/signin?return=`**, which says what for, such as "Sign in to add a
  library." "Sign in with GitHub" and Continue with GitHub show only when GitHub sign-in is configured. Deleting an
  account removes its sessions, stars, listings, and GitHub snapshot.

### The dashboard

- **Decided (Josh), 2026-10-05: the dashboard has one tab per concern**: My libraries (N), Projects (N), Starred rules
  (N), and Account, at `/me`, `/me?tab=projects`, `/me?tab=stars`, and `/me?tab=account`, each showing only its own
  content, each count its rows, with no lede, since the tab's name says what it shows. A tab the page doesn't know, or
  doesn't offer, shows My libraries. On a phone, the tab bar scrolls sideways rather than wrapping.
- **Decided (Josh), 2026-10-05: the account menu has three entries**: the visitor's name and @login, then Dashboard,
  Add a library, and Sign out. Starred rules and Account are tabs of the dashboard, so the menu doesn't repeat them,
  and Dashboard is marked current on every tab.
- **My libraries is where a visitor's libraries and listings live.** It lists the libraries whose owner is the visitor
  or one of their organizations, vetted or listed, each with its state, Vetted, Unvetted, Checking, or Failed with why,
  its rule count, and its star total, and a "New" chip when listed today. A listing of the visitor's offers Remove, and
  Try again when its check failed, after the status, as small text actions; a listing of someone else's library goes
  under "Listed by you", shown only when there are any. + Add a library ends the list. `/me/listings` redirects to
  `/me`, since there is one place for a visitor's libraries.
- **Projects lists the visitor's repositories that use Rulemart libraries**, read from each repository's
  `.code-rules/generated/provenance.json`: per library, each repository that imports it, with "(private)", and "N rule
  updates" or "up to date". An update is a rule the library has since published a newer version of, or retired. Rules
  a library added to a group the project imports aren't counted, since the project's configuration may exclude them,
  which provenance doesn't record. Counts are computed as the dashboard reads, so a new release shows at once.
- **My libraries and Projects lead with their list and end with one muted line on the read of GitHub**: "Public repos
  only · read from GitHub 7 minutes ago · Refresh · Include private projects", or, with the GitHub App installed,
  "Including private projects from the repos you selected · ... · Manage". The lists are cards, with a 1-pixel
  border, the card radius, and rows divided inside, as the site's other lists of libraries are. Each row's figure, the
  star total or project count, sits beside its name, in the muted small style. The prompts to add more follow the
  list, never lead it.
- **The dashboard's head and headings are quiet.** The eyebrow stands on its own line above the avatar, centered beside
  the name and the login line; section headings are sentence case, never tracked uppercase, since the tab names already
  say what a list is, and a second group, No longer counted, is a plain heading with its count inline.
- **The Account tab shows the account's facts and its actions**: the facts in a two-column card capped at 36rem, a line
  pointing to the privacy page for what Rulemart keeps, Sign out, with Sign out everywhere beside it as a secondary text action, and Delete
  your account, a disclosure rather than a link. Deleting asks for the account's login, typed exactly, which the server checks and
  refuses with 400 otherwise, deleting nothing; with a script, a dialog says what goes, that nothing on GitHub changes,
  and enables Delete my account only once the field holds the login. The login travels in the POST's query, never its
  body, which CloudFront refuses from a browser's form: the dialog's script puts it in the action, and without a script
  the disclosure's GET form leads to a page that says what deleting does and posts the login on.
- **The dashboard reads the visitor's GitHub account at sign-in's first page and on Refresh, never on every page.**
  A read lists their organizations, the public repositories of theirs and their organizations', and the private ones
  the GitHub App's installations read, the 200 most recently pushed, and finds the ones that publish a library
  (`rule-library.yaml` and a `release/<n>` tag) and the projects (`.code-rules/generated/provenance.json`, of which it
  keeps each source and its rules' versions). It's kept per account in `github_snapshots`, refreshed at most once a
  minute, shown with when it was read, and update counts compare it with the catalog as pages read. Only its account
  sees it. GitHub returns a visitor who installed the app to `/me/github/installed`, which records the installation
  once GitHub says it's on their account or an organization they own, and the app's webhook, signed with its secret,
  forgets an uninstalled one, once infrastructure gives the webhook a path to the function, which rulemart#41 and
  infra-catalog#29 do; until then the visitor's next read notices.
- **A read uses GitHub's REST API, not its search, and is bounded.** Code search finds a repository only once GitHub
  has indexed it, allows 10 requests a minute, and can't see what an installation reads. The read costs one request
  per repository, plus its tags or provenance file only where the root holds `rule-library.yaml` or `.code-rules`,
  eight at a time, within 200 repositories, 100 organizations, and a 25-second deadline; GitHub isn't read inside the
  sign-in callback, which would hold every sign-in's redirect. Each sign-in discards the account's snapshot, and the
  first page that needs one reads GitHub.
- **A failed read keeps the last snapshot and says so.** GitHub failing, rate limiting, or timing out leaves the
  snapshot that was, with when it was read and Try again, and the next read waits a minute. A token GitHub refuses, a
  session without one, or one sealed under a key since rotated asks the visitor to sign in again, at `/signin?again=1`.
- **Provenance is parsed by Rulemart, in `accounts/domain`, from the fields Code Rules writes**, taking each source's
  name, repository, release, and groups, and each rule's ID, origin, and version, and skipping a local or forked rule.
  A file it can't parse makes no project rather than failing the read. Replace it with Code Rules' parser once one
  ships.
- **An installation is the visitor's only when GitHub says they own the account or organization.** Rulemart checks the
  installation's account with the app's JWT and, for an organization, the visitor's membership role: only an owner can
  install an app there and see every repository it may read. The webhook, at `/account/github/webhook`, acts on
  removals and changes, not new installations, which are recorded when GitHub returns the installer; `account` is
  reserved and the OAuth callback stays at `/account/github/callback`. Remove access forgets the installations and
  says how to uninstall the app on GitHub, since redirecting a POST there breaks `form-action 'self'`; Skip, public
  repos only, is a POST that returns to the dashboard with the toast "Okay. Rulemart will only look at your public
  repos.", changing nothing.
- **`/me/add` lists publishable repositories from the snapshot**, and "Add this library" posts the same listing flow. A
  row shows the repository's latest release, "Public · release/3", not its group count, since counting groups costs more
  requests than the number is worth. Rows order as what the visitor can add, what Rulemart is adding, what Rulemart has,
  then the ones that show but can't be added: a public library the visitor may only read, then private ones. The URL
  form keeps the listing's checks and the write-access check, and shows an address it accepts as a row to add, since a
  GET form can't post.
- **Decided (Josh), 2026-10-05: `/me/add` reads as the dashboard's lists do.** An intro in whole sentences says what the
  page lists, repositories the visitor and their organizations own that publish a library, and that a public repository
  can be added by its URL. The repositories are a bordered card of rows, with no header: each name links to it on
  GitHub, in the same tab as the site's other links to GitHub, with a small arrow and the accessible name "… on GitHub",
  above "Public · release/6 · via the X organization", with the action at the row's right, Add this library, "✓ On
  Rulemart", or, for a private repository or a public one the visitor can't write to, the dimmed row saying why it can't
  be added. One status line under the card, in the dashboard's pattern and with its parts ("Public repos only · read
  from GitHub … · Refresh · Include private repos", or "Including private repos from the ones you selected · … ·
  Manage"), replaces the dashed note and the separate line on the read. "Add a library by URL" is a sentence-case
  heading, with one sentence under it, above the field.
- **Decided (Josh), 2026-10-05: only a repository's maintainers can add it.** An account lists a repository only when
  the visitor's GitHub token has write access to it, checked by reading the repository from GitHub when the visitor
  confirms the address and again when the listing is created, so the picker and "My libraries" name the same set and
  the people who can fix or remove a listing are the people who own the code. A public library in the picker that the
  visitor can't write to shows dimmed, saying only someone with write access can add it. Discovery of libraries nobody
  listed is a later worker job, not a URL form.
- **`/me/add/run` follows a listing's check.** Its checklist ticks every step at once when Rulemart has the library,
  since the worker's check isn't observable step by step, and a failed check shows the reason with Try again, Remove,
  and Back to Dashboard. `poll.js` swaps the checklist every two seconds, and without it a `<noscript>` reload does the
  same. A check queued more than three minutes ago says it's taking longer than usual, with Refresh status, and the page
  stops following it, since the next check is the worker's hourly poll.

## Content pages

- **`/about`, `/about/vetting`, and `/privacy` say what Rulemart is, what vetting means, and what it keeps.**
  Decided (Josh), 2026-10-05: `/about` stays one screen about Rulemart and Code Rules, and what vetting means, unvetted
  libraries, getting a library vetted, and reporting a problem live on the vetting page. Getting a library vetted is:
  release it with Code Rules, list it, then ask on GitHub. About, vetting, privacy, and FAQ share one layout under an
  About label. The privacy notice states only what the code and infrastructure do, makes no legal claim, names
  `legal@fabricahq.com` for questions, and changes with them: a change to what Rulemart keeps, logs, or shares updates
  it in the same pull request.
- **The FAQ's answers describe Rulemart as it is**, naming real rules from fabricahq/public-rules and groups that hold
  rules today. "Still have a question? Ask us." leads to feedback.
- **Feedback topics open prefilled GitHub issues**: Rulemart topics, and "Anything else", in fabricahq/rulemart, and
  Code Rules topics in fabricahq/code-rules, with a title prefix naming the topic and the labels `feedback` and
  `topic:<key>`. "Something broken on Rulemart" leads to the Report a problem issue form.
- **The sitemap lists what search engines may index, and `robots.txt` keeps them out of the rest.** `/sitemap.xml`
  lists the site's own pages, the page of each group a vetted library holds, canonical or not, each owner's page, and
  each vetted library with its groups and current rules, by their canonical addresses on `RULEMART_BASE_URL`, in one
  file of at most 45,000 groups, library groups, and rules, and at most 5 MiB; past either it lists what fits and logs
  `sitemap truncated`, which alarms, and the next step is a sitemap index. Without a base URL there's none. It leaves
  out rules' assets' pages, which belong to their rule's, and retired rules' pages.
  `/robots.txt` disallows the visitor's own pages under `/me` and `/account`, the cart, sign-in, listing, search, the
  unvetted area, and comparisons, which also say `noindex`, and leaves owners' `/o/` addresses open. Each one-segment
  page is disallowed alone and with a query, so `/list` doesn't keep crawlers off a library such as `listr/rules`.
- **A page with a canonical address describes itself to social sites**, with Open Graph tags and one image of
  Rulemart's; a page without one, such as an unvetted library's, a search, or a comparison, shows no card. A page's
  description falls back to what it holds and is cut at a word to 200 characters. There is no structured data.
- **Reports and requests to vet a library are GitHub issues** in Rulemart's public repository, through issue forms
  (Report a library, Ask to vet a library, Report a problem), so Rulemart stores nothing new. Each library's page links
  a report with the library filled in; the forms ask reporters to leave out anything private, and to send security
  vulnerabilities to `hello@fabricahq.com`.
- **The README is primary tier**: Rulemart is a product people adopt for its own sake, so its README presents it as
  one. Its logo is its title, and it has no "What Rulemart doesn't do" section; the limits stay in `/about` and the FAQ.

## Design system

- **The click-through mock on the `prototype` branch was the UX spec through launch and is no longer the reference.**
  The pages, this file, and the code are the reference now.
- **Only tokens are colors.** The palette is grays, the diff's green and red, one amber that means caution and nothing
  else, the vetted check mark's blue, a green for In cart, and a soft light blue for info toasts. Diff colors are
  darkened in light themes and lightened in dark ones where text needs 4.5:1 contrast.
- **Pages are accessible by default.** Every page starts with a skip link; text meets WCAG AA contrast, 4.5:1, in both
  themes; sections are headings and lists are lists; long IDs and file names break between parts, and no page scrolls
  sideways at 320 pixels. A control the page focuses after a click wears a quiet ring that the visitor's first key
  turns into the keyboard's ring.
- **Notices that only report are toasts**: a pill at the viewport's bottom right, 24 pixels in or 16 on a phone, that
  holds about as long as reading it takes, at least 2.2 seconds, pausing while hovered or focused. An info toast, such
  as the first star's, is a soft light blue note with dark text and a close button, held at least 6 seconds. A notice
  that asks the visitor to act stays a banner in the page's flow. Without JavaScript every notice stays a banner.
- **The header is brand text, search, links, the cart, and the account.** It reads Fabrica's cube and name, a slash,
  and Rulemart in text, followed by the search field, Techs, Practices, Libraries, FAQ, the cart, and Sign in or the
  visitor's avatar. It's opaque and stays put, marks a link current only on its own pages, and `/` focuses its search
  field. Below 960 pixels, a menu button stands in for the links; below 27rem it shows Rulemart's name without
  Fabrica's, and below 384 pixels Sign in becomes a labeled person icon, down to 320 pixels. The account slot keeps its
  width, so signing in or out never moves the links.
- **The footer is one line at every width**: "Fabrica / Rulemart", the running release's tag linked to its GitHub
  release, or "dev" for a local build, About, Feedback, and Privacy, then icon links to the source on GitHub and the
  theme menu, as 44-pixel tap targets, 32 on the narrowest phones. It doesn't repeat the header's sections. Below 720 pixels the release and
  Fabrica's name drop out so the line fits.
- **The brand is text in the header and a logo in the home page's hero.** The hero shows two `<img>` elements, the
  dark and white horizontal logos, switched by the same dark rule as the color tokens, so the visitor's footer choice
  wins, not a `<picture>`'s system preference. The browser tab shows the package's adaptive favicon, with an ICO built
  from its dedicated 16-, 32-, and 48-pixel artwork, and a phone's home screen the apple-touch-icon. The social image
  is the dark logo over the tagline "Agent coding best practices, off the shelf", and `brand/assets.py` makes it and
  the ICO. A static SVG may hold a `<style>` that loads nothing.
- **Dates are absolute, in UTC**, such as "1 Oct 2026", rather than "3 days ago". Decided (Josh), 2026-10-05: pages
  are cached and the same for everyone, so a date never goes stale and needs no script.
- **Pages keep their vertical padding on a phone**, 36 pixels above and 80 below, so content never sits against the
  header.
- **Lists use shared parts**: one rule row for every list of rules, one library row with its owner's avatar and check
  mark, and bordered cards for lists of libraries and projects, so lists stay identical across pages.

## Application

- **Go, templ, Tailwind, sqlc, and goose, with Postgres on Neon.** No Node: Tailwind runs as its standalone
  binary. Pages are server-rendered; small scripts paint what only the browser knows, such as the cart. The scripts'
  pure logic is tested with Node's own test runner, in `*.test.mjs` files beside the web package, which `make check`
  runs when `node` is installed, and CI always runs; the repository has no Node packages.
- **Static assets are embedded in the web binary** and cached by CloudFront for a year under hashed names.
- **Prose pages are Markdown with a small template layer.** About, vetting, and privacy live in
  `internal/platform/web/content/*.md`, so Josh edits their words directly instead of through templ's syntax. Go
  template actions name the shared links and strings and switch on what the server offers; `make generate` renders
  the Markdown with goldmark, so the web function still links no Markdown renderer, and the server runs the templates
  at start, refusing to start on a missing value. The FAQ and feedback pages stay templ: their structure is markup.
- **Ingestion reads library repositories with go-git over HTTPS**, the way Code Rules reads them, and parses
  release records with Code Rules' own parser: a copy in `internal/lib/coderules` until Code Rules publishes a
  public parsing package. It replaces a library's rows in one transaction, so running it again changes nothing, and
  it parses every version's file at the release that published it, failing the library, naming the release and file,
  when it can't. The operator's `cmd/ingest` stays for local development and backfills, including libraries not yet
  vetted, and `make worker` runs one poll locally with an in-memory queue through the same handler as on Lambda.
- **The web function connects as `rulemart_web`, a login that can only read what the pages show, through its
  membership in `rulemart_catalog_reader`, and sign visitors in and out, through its membership in
  `rulemart_accounts_writer`, which writes only accounts, sessions, what Rulemart read of visitors' GitHub accounts,
  the GitHub App's installations, listings, and stars.** Carts live in browsers, so
  checkout writes nothing. Infrastructure owns the roles: it creates each group role with SQL, as a NOLOGIN role,
  creates the login, and makes the login a member, because a role made through Neon's API or console joins
  `neon_superuser`, which can read and write every table and create roles and databases. Migrations own the grants:
  they grant each group role what each table needs, never grant to a login, and never create roles, so a release can't
  migrate before infrastructure has, and a login can be replaced or rotated without a migration. Migrations connect as
  the database's owner.
- **Ingestion connects as `rulemart_worker`, a login that can only write the catalog, through its membership in
  `rulemart_catalog_writer`.** The split is the web function's: infrastructure creates the NOLOGIN group role, the
  login, and the membership with SQL, and migrations grant the group exactly what ingestion writes, which never
  includes deleting a library or changing the schema. Of a listing, it may change only what a check finds: the
  repository's ID, when it last checked, and why that failed. The worker function and the operator's `cmd/ingest` both use
  it, so production ingestion never needs the owner.
- **The worker keeps vetted and listed libraries current.** The EventBridge schedule invokes the worker function
  every hour, and it queues one job per vetted library, and one per listing to check, on the SQS jobs queue, which
  invokes it again for each job. The web function queues a new listing's job at once. A
  job lists the library's `release/<number>` tags with go-git, without fetching objects, and ingests only when their
  numbers and tag object IDs differ from what the catalog stored, so an unchanged library costs one request and no
  GitHub API call. A tag rewritten on the same commit is still a change, since a release's record is in its tag's
  message. A failed job writes nothing; SQS retries it after the visibility timeout, then moves it to the dead-letter
  queue, whose alarm reports it. Hourly, not every 10 minutes, because each job reads the stored tags from Postgres
  and wakes Neon's compute: about $3 a month instead of about $12. Faster updates would keep a tag fingerprint outside
  Postgres.
- **A job names a library only by its code host and the host's repository ID, and only a vetted one, or a listing by
  its ID.** It carries no URL: the worker fetches from the clone URL the catalog stored, or the one the host's API
  returns for that ID or the listing's name, so a queued message can't point it at another repository.
- **The worker may authenticate to GitHub's API with a token from SSM**, which raises its limit from 60 requests an
  hour, shared with other Lambda functions on the same address. It calls the API once per new listing, and once per
  library whose tags changed; listing tags uses Git.
- **After launch, the worker reads GitHub as the GitHub App "Rulemart by Fabrica" instead of with a personal token**,
  decided by Josh on 2026-10-05. A fine-grained personal token expires within a year and can't be rotated through
  GitHub's API, while the app's installation tokens are minted hourly from its private key, which SSM already holds
  for the web function. The worker uses the app's installation on fabricahq, since an installation token reads any
  public repository, not only the installation's, as `internal/lib/githubapp`'s live test checks. `GITHUB_APP_ID` and
  `GITHUB_APP_PRIVATE_KEY_PARAMETER` switch it, and replace `GITHUB_TOKEN_PARAMETER` once set, so the launch runs with
  the personal token, and the switch, and its rollback, are configuration changes in infra-live, whose
  `OPERATIONS.md` describes the switch once it is made.
- **Code is organized by bounded context first, and by layer only within a context**, following fabricahq/greenfield's
  ADR 0002 (backend bounded contexts). `internal/contexts/catalog` owns the catalog: `domain` for its values and
  rules, with no I/O; `render` for rules' Markdown and assets, which assembly takes as an interface,
  `domain.Renderer`, so the web function doesn't link a Markdown renderer; `app` for ingestion and page reads;
  `source/git` and `source/github` for the adapters that fetch libraries; `store` for the persistence contract, with
  `store/postgres` as its only implementation and the catalog's only SQL; and `views` for what pages read.
  `internal/platform` holds runtime that contexts share, such as the database, migrations, and the web server, which
  stays in platform as greenfield's transports do.
  `internal/lib` holds narrow libraries that own no product concept, such as the parser copy, and `githubapp`, which
  acts as a GitHub App: it signs the app's JWT and mints and keeps installation tokens, for accounts' adapter, which
  reads private repositories, and for the worker, which `cmd/worker` composes, since the catalog can't import
  accounts.
  `internal/contexts/accounts` owns accounts, sessions, and what Rulemart read of visitors' GitHub accounts, with the
  same layout, plus `github`, its adapter for GitHub: the OAuth app that signs visitors in, the REST API reads of a
  visitor's organizations and repositories, and the GitHub App that reads private repositories, with its webhook.
  `github/githubtest` is a fake GitHub for tests and the local build's dev sign-in; release builds never link it.
  Neither context imports the other: a page that combines them, such as the dashboard, which matches a visitor's
  GitHub snapshot against the catalog's libraries for their projects' updates, reads each and combines them in the
  web server.
  Stars and listings live in the catalog context, since each names a library or rule, and pages read them with the
  catalog from one snapshot; so does checkout, which resolves the keys a browser's cart sends against one snapshot
  of the catalog. A context added later gets the same layout.
- **The catalog's `GetLibrary` reads `accounts (id, github_login)` for a listed library's "Added by"**, a read across
  bounded contexts that 00015 grants `rulemart_catalog_reader`, column by column, so the page reads it in one query.
- **Build in thin vertical slices**, each deployed and checked end to end.
- **Page URLs, such as `/{owner}/{repo}`, assume one code host, GitHub.** The routing decision for a second host is
  host-qualified URLs, such as `/gitlab/{group}/{repo}`, with GitHub keeping the short form. Libraries are stored by
  host and the host's repository ID, but the schema's host check, the vetting parser, and ingestion allow only
  github, so a second host also needs changes there.

## Infrastructure and delivery

- **One environment until launch**, at `rulemart.fabricahq.com`, with Neon branches for trying migrations on real
  data. `rulemart.ai` and `www.rulemart.ai` redirect there temporarily, with a 302, keeping the path, through a
  Cloudflare Single Redirect that infra-live manages in code with its DNS records, so the redirect is reviewed and
  can't drift from the domain it points at. It stays temporary while the main domain may still change, because
  browsers cache a permanent redirect indefinitely.
- **The web function sends every response's security headers**: the content security policy, HSTS for a year without
  `preload`, a permissions policy that denies features Rulemart never uses, `Cross-Origin-Opener-Policy`,
  `nosniff`, and the referrer policy. Sending them from the function, rather than a CloudFront response headers
  policy, keeps them in one place that tests cover.
- **Cloudflare Web Analytics counts page views on every page, signed in or not, only once its site token is set**, as
  `CLOUDFLARE_WEB_ANALYTICS_TOKEN`. It sets no cookie, and the paths it reports, such as `/me` and `/cart`, name no
  visitor, and the cart's items never leave the browser except to the checkout endpoint. Without the token,
  pages load no other site's script, and the content security policy allows none; with it, the policy adds
  Cloudflare's beacon and its reports, and the privacy notice says so. A token that isn't letters, digits, hyphens, and
  underscores stops the web function at start.
- **Static files share one CloudFront copy**: `/_static/*` and `/favicon.ico` have a cache behavior whose key holds no
  cookie or query string, so a signed-in visitor doesn't cache them per session.
- **GitHub's deliveries reach the webhook through a public Lambda alias that answers nothing else.** CloudFront's
  origin access control can't sign a body GitHub didn't hash, so the web function gets a second Function URL, on an
  alias, with no authorization, and CloudFront sends only `/account/github/webhook` there. The function fails closed:
  only an invocation Lambda identifies as the site's, by the qualifier of the invoked function ARN, which Lambda sets
  from the URL, none, `$LATEST`, or a version, reaches the pages. Any alias, and a missing Lambda context, gets only
  the webhook, whatever `GITHUB_APP_WEBHOOK_ALIAS` says, so a wrong value can't expose the site; the variable only
  turns on the privacy page's paragraph and silences a start-up warning. The webhook answers a POST to its exact path
  before the visitor and cross-origin checks, which are for browsers, and a plain, cookie-free, uncacheable 404 to
  anything else there, a wrong method included, since through the alias it's the only resource.
- **A delivery says which installation changed; GitHub says how.** HMAC proves GitHub signed a body, not that it's
  fresh, so Rulemart doesn't apply what a delivery says happened: it asks GitHub for the installation's state, gone,
  suspended, or active, applies that, and discards the snapshots of the accounts that read through it. Only GitHub's
  404 means gone; a 403 refuses to say, so the delivery fails and GitHub sends it again. A captured suspension sent
  again after a newer unsuspension, even once its record has expired, then changes nothing. The webhook also records
  each delivery it acts on, by its ID and its body's SHA-256, since GitHub signs the body but not the ID, and ignores
  a repeat for a week. One transaction takes a Postgres advisory lock on the installation, records the delivery, then
  asks GitHub and applies the answer, so deliveries for one installation, across Lambda instances, act one at a time,
  each reading GitHub after the last committed and none undoing a newer one; a repeat returns before asking GitHub,
  which a replayed body can't make Rulemart spend requests on; and a failed read or application leaves no record.
  The transaction holds a database connection while GitHub answers, within the GitHub client's five-second timeout.
- **An AWS WAF rate rule on POSTs is ready but off**, at about $6 a month, since the function bounds each kind of
  write itself. Infrastructure turns it on if abuse appears.
- **There is no synthetic check yet.** The 5xx alarm already sees any failure a visitor meets; a check would add only
  failures before the function, such as DNS or the certificate, which the checks after each deploy cover, and its
  alarm would need a second SNS topic in us-east-1.
- **Pages name their address on `RULEMART_BASE_URL` as canonical.** CloudFront's own `cloudfront.net` domain serves
  the same pages, so each page links its address on the public origin, without a tab's query string, and search
  engines index that one. Infrastructure sets the variable. Unset, the production build names none, so it has no
  sitemap, while the dev build, which `make web-dev` runs, names the address it serves at when that's a loopback one,
  such as `http://127.0.0.1:8080`, so the sitemap and the social card work locally, as CONTRIBUTING.md says. A value that
  isn't a bare origin stops the web function at start: https, or http only on a loopback host, such as localhost or
  127.0.0.1, with no path, query, or fragment.
- **Releases are published by [Release Planner](https://release-planner.fabricahq.com)**, and merging a release
  pull request approves one. Nothing else tags or publishes a release.
- **Migrations are the application's concern, and run after a release is approved and before it's published**, as
  Release Planner's pre-publish workflow, "Migrate the database". If they fail, nothing is published. They never
  run at deploy time or from the infrastructure repository. The job runs in a `production` environment that only
  `main` can use, assumes an AWS role that trusts only that environment through GitHub OIDC, reads the pooled
  connection string from SSM, and derives the direct one. The environment's variables name the role and parameter,
  so this public repository names no account details.
- **Migrations are safe to run again, late, and twice at once**: goose skips applied migrations and takes a Postgres
  session lock. They work with the release that's still running: expand in one release, contract in a later one.
- **GitHub repositories are created by hand**, and their rulesets, environments, and Pages are managed in code.
- **Lambda packaging uses [lambda-build](https://github.com/fabricahq/lambda-build)**, a public, shared tool, as
  Release Planner's release-assets workflow. A release is pinned for deployment only after a matching rebuild or a
  verified build attestation.
- **Local development and tests use Postgres 18 in Docker.**
- **How production is operated, and the launch record, live in fabricahq/infra-live**, in
  `aws/rulemart/us-west-2/rulemart-prod/OPERATIONS.md`, beside the stack they describe: its secrets and their
  rotation, the GitHub apps' settings, deploying and rolling back a release, and the checks after a deploy. They
  describe private infrastructure, so this public repository keeps only what the code needs from it.

## What Rulemart logs

- **Two sources record traffic.** CloudFront's standard access logs hold one row for every request it serves, cache
  hits included, with the visitor's IP address, country, user agent, referrer, path, and query string. The web
  function logs one line for each request that reaches it. Analysis of who visits and what they read uses
  CloudFront's logs, through Athena; the function's lines explain how the origin behaved.
- **Only CloudFront's logs hold visitors' IP addresses.** The web function's logs never include an IP address, a
  raw path, a query string, or headers, because those can identify a visitor or carry a secret.
  - In place of the path, each line records the route pattern, such as `/{owner}/{repo}`. That also groups requests
    by page type.
  - When an error message names a library or rule, the name is replaced with the route's placeholder, such as
    `{owner}`.
  - The rest of an error message is kept, even when it names an internal address such as the database host, because
    diagnosing the failure needs it.
  - Sign-in logs `signed in` and `deleted account` with the account's internal ID, and `sign-in refused`,
    `sign-in failed`, and `cross-origin request refused` with a reason, never a code, state, token, verifier, login, or
    cookie.
- **Alarms cover what the function answers anyway.** Besides Lambda's errors and throttles, the URL's 5xx responses,
  and the jobs queue, the web function alarms on the lines it logs for a failed GitHub sign-in, a listing it couldn't
  queue, and a sitemap that left out rules.
- **CloudFront's logs are kept for 6 months, and the functions' for 30 days.** S3 deletes each access log file 180
  days after delivery, and CloudWatch Logs deletes function lines after 30 days. IP addresses are personal data, so
  `/privacy` says what CloudFront logs, why, and for how long, and lowering the retention is how Rulemart keeps less.
- **Every command logs JSON lines through `internal/platform/logging`**, at the level `LOG_LEVEL` names, info by
  default, and each line names the release that wrote it, from `RULEMART_RELEASE`. An unknown level stops the command
  at start, rather than logging at a level nobody chose. The worker logs one line per job with its `outcome`, one per
  SQS batch, and one per scheduled poll, under snake_case keys that mean the same on every line, and never a
  connection string or a token.
- **Log volume stays small**, because CloudWatch Logs bills by the byte. The function logs one compact line per
  request it serves, which CloudFront's cache keeps to a fraction of traffic; nothing at debug level by default; no
  line per query or per rule; and a stack trace only for a panic. A long-lived function logs `ready`, with its schema
  version and how long starting took, or logs `startup failed`, with the error, and exits. `migrate-database` logs
  each migration it applies, because CI keeps its output.
