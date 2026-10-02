# Slice R1: shell and navigation

## Goal

Give every page the prototype's frame and the prototype's way in: the header and footer, the home page, the browse
pages for technologies and practices, the "other groups" page, owner pages, the FAQ, the feedback page, and the
prototype's addresses for groups. No data changes. This is the first realignment slice;
[realignment.md](../realignment.md) holds the decisions behind it and the slices that follow.

The prototype is the spec. Run it with `python3 -m http.server 8766 --directory prototype` from a checkout of the
`prototype` branch, and compare each route below with the site side by side, at 1280 and 390 pixels, light and dark.
Where this document and the prototype differ, this document says why, and the difference is a **Proposed**
decision.

## What a visitor sees

- **The header**, on every page: Fabrica's mark and name, a slash, Rulemart; the search field; then Techs,
  Practices, Libraries, FAQ; the cart; and Sign in with GitHub, or the visitor's avatar and menu. Techs, Practices,
  Libraries, and FAQ hide on narrow screens, as the prototype's `hide-md` does, and a menu button opens them instead;
  the search field becomes a search icon, as today.
- **The home page** (`/`): the eyebrow "Fabrica / Rulemart", the heading "Agent coding best practices, off the shelf",
  the lede, the big search field with the placeholder "Try React effects, logging, testing", and "Popular" chips.
  Then **Technologies** and **Practices**, each a band with "Browse all →" and a four-column grid of tiles, one per
  canonical group that has current rules in a vetted library, sorted by rule count: icon, name, "N rules · M
  libraries". Then **Libraries**, the first four vetted libraries with "All libraries →". Then the band "Stock the
  shelves / Have a public Code Rules library? / List it on Rulemart in a minute. Rulemart updates with every
  library release." with "List your library →".
- **Browse** (`/browse/techs`, `/browse/practices`): the eyebrow "Browse", the title "Technologies" or "Practices",
  tabs between the two, and a row list of canonical groups with current rules in a vetted library: icon, name, and,
  for practices, its description; "N rules", "M libraries", a chevron. Below, when any exist, "View
  other technology groups (N) →".
- **Other groups** (`/browse/techs/other`, `/browse/practices/other`): crumbs "Technologies › Other groups", the title
  "Other technology groups", the prototype's two paragraphs explaining canonical groups, and a row per group a
  library declared that isn't canonical: its ID in monospace, the library's name, "N rules". Empty: "None right
  now."
- **A group's page** moves from `/groups/{kind}/{name}` to `/g/{kind}/{name}`, unchanged otherwise; slice R4
  redesigns it.
- **An owner's page** (`/{owner}`): the owner's avatar, their login as the heading, `github.com/{login}`, and
  "Libraries" with the owner's vetted libraries as the libraries page lists them.
- **The libraries page** (`/libraries`): the eyebrow "Libraries", the title "Libraries Rulemart has vetted", a lede
  that says anyone can list a public library and where unvetted ones are, and the rows as today.
- **FAQ** (`/faq`): the prototype's ten questions as disclosures, with the answers adjusted to what Rulemart does
  today, and "Still have a question? Ask us." leading to feedback.
- **Feedback** (`/feedback`): the prototype's topic rows under Rulemart, Code Rules, A specific rule, and Anything
  else. Each row opens a new GitHub issue, prefilled, in the repository that owns the topic, marked "↗ GitHub".
- **The footer**: "Fabrica / Rulemart", Fabrica linking fabricahq.com; About, Feedback, Privacy; GitHub's mark leading
  to Rulemart's source; the theme menu.

## Decisions

### Addresses

- **Proposed: the prototype's addresses.** `/browse/techs`, `/browse/practices`, `/browse/{kind}/other`,
  `/g/{kind}/{name}`, `/{owner}`, `/faq`, `/feedback`. `/groups` redirects permanently to `/browse/techs`, and
  `/groups/{kind}/{name}` to `/g/{kind}/{name}`, keeping the query. The sitemap, canonical links, and every internal
  link use the new addresses.
- **Proposed: `/browse` redirects to `/browse/techs`**, and a kind in another case, such as `/browse/Techs`, redirects
  to its one spelling, as a group's ID does. The prototype has no `/browse` of its own; a visitor who trims the
  address lands on the technologies.
- **Proposed: reserved first segments, and `/o/{login}` for an owner whose login is one.** The site's own one-segment
  pages are `browse`, `g`, `libraries`, `search`, `unvetted`, `list`, `about`, `privacy`, `faq`, `feedback`, `o`,
  and the account pages. GitHub has users named `g`, `faq`, `browse`, `list`, `o`, and `me`, so an owner page can't
  share their address: every owner is also at `/o/{login}`, which is the canonical address for an owner whose login
  is reserved, and every other owner's `/o/{login}` redirects to `/{login}`. Library and rule pages have two or more
  segments, so no site page hides them; a library owned by `faq` stays at `/faq/{repo}`. The lowercase redirect for
  site sections covers the new sections. One library is hidden: a library owned by the user `o` whose name is a
  login, since `/o/{name}` is that login's owner address; its rules' pages, with three or more segments, stay.
- **Proposed: an owner page exists for an owner with a vetted library**, and answers 404 otherwise, even for an owner
  with a listed, unvetted library, so listing a repository can't create a page under Rulemart's address. The page
  shows the login, the avatar the catalog stores, and the owner's vetted libraries. The owner's display name, kind
  (organization or person), and bio wait for a slice that stores them, since the catalog keeps only what ingestion
  reads from the repository.
- **Proposed: a library's About panel leads to its owner's page**, as the prototype's does, and its Repository row to
  GitHub. An unvetted library's owner has no page, so its Owner row leads to the owner on GitHub instead. The
  libraries page's lede says each library shows its owner and repository, rather than the prototype's "owner line",
  which no row has.

### Home

- **Proposed: "Popular" names the two technologies and two practices with the most rules**, since Rulemart has no
  traffic data yet; the prototype hard-codes four. The chips link to the group pages.
- **Proposed: tiles sort by rule count, then name**, as the prototype's, and say "N rules · M libraries"; the libraries band shows
  the first four vetted libraries in owner and name order (**Existing**), since nothing sorts libraries by anything
  else. On a narrow phone, under 384 pixels, the tiles stand in one column rather than the prototype's two, which cut
  off names such as Concurrency at 320 pixels.
- **Proposed: "List your library →" leads to `/list`** while that is the page that lists a library, signed in, and to
  sign-in with a return to it otherwise; slice R7 moves it to `/me/add`. Where listing isn't available, as in a build
  without sign-in, it leads to the about page's "Get a library vetted".

### Browse

- **Proposed: a practice's row shows its description, a technology's doesn't**, as the prototype does: a technology's
  name says what it is, and the row stays one line.
- **Proposed: "other groups" lists one row per library and group ID**, naming the library, since two libraries that
  choose the same ID that isn't canonical hold separate groups (**Existing**). The row leads to the group's section
  on the library's All rules tab until slice R4 gives every group a page. The prototype's "similar to" note needs a
  list of near-canonical names Rulemart doesn't keep, so there is none.
- **Proposed: the counts count current rules in vetted libraries**, as the groups page did.
- **Proposed: rows sort by rule count, then name**, as the home page's tiles do, so both pages rank groups alike; the
  prototype lists groups by size too. Tiles with as many rules also sort by name, rather than by ID.
- **Proposed: on a phone, a row's counts drop under its text**, in line with it, rather than stand beside it as the
  prototype's do, since beside it, "10 rules" and "2 libraries" squeezed a practice's description into a column about
  120 pixels wide at 390 pixels and broke names mid-word at 320. Names wrap only between words.
- **Decided (Josh): a browse row shows no group ID**, only the icon, the name, and a practice's description. On a
  screen 1280 pixels wide or more, each row's text keeps to one line: the description takes the room left of the
  counts and, as a safety, ends in an ellipsis rather than wrap. Where the text wraps, on a phone, the icon stands at
  the top of the text rather than beside its middle.

### Icons

- **Decided (Josh): Zustand's tile shows the one-color bear**, the vector cicero-mello contributed in
  pmndrs/zustand#1623, vendored as `community/zustand.svg` and drawn like the Lucide icons: ink on light themes,
  inverted on dark ones, on the usual tile. Devicon's Zustand logo, a photo-like vectorization of 235 paths, didn't
  read at a tile's size and needed a white tile in dark themes. The lightTile option stays for any later icon that
  needs it. Rulemart replaces the bear if the project publishes an official vector.

### Header, footer, FAQ, feedback

- **Proposed: the header's cart keeps today's behavior**, a link to the cart for signed-in visitors only, drawn as the
  prototype's icon with its count badge, until slice R5 moves the cart into the browser and shows it to everyone. It
  shows at every width, as the prototype's does, so the avatar no longer carries the count on a phone.
- **Proposed: the account menu lists Account, Add a library, Your stars, Your cart, Your listings, Sign out**,
  leading to today's `/account`, `/list`, `/account/stars`, `/account/cart`, and `/account/listings`, until slices R3
  and R7 fold them into the dashboard. The prototype's Dashboard and Starred rules would open pages titled "Account"
  and "Your stars", which lists starred libraries, so the menu names the pages as they are; slices R3 and R7 restore
  the prototype's names when the pages become those.
- **Proposed: the header marks a link current only on its own pages**: a kind's browse page and its other groups, the
  libraries page, and the FAQ, as the prototype does, and the cart marks its icon. A library's, a rule's, a group's,
  or an owner's page marks none, as in the prototype, and so do the unvetted libraries page and the page that lists a
  library, which the prototype doesn't have.
- **Decided (Josh): a menu button stands in for the header's links where they hide**, below the wide breakpoint (960
  pixels), as the prototype's `hide-md`: a round 44-pixel button with a three-line icon, named "Menu", just left of
  the search icon, so the name, the search icon, the cart, and the account control keep their places. It opens a small
  menu of Techs, Practices, Libraries, and FAQ, under the button with its right edge on the content edge, as the
  account menu's, marking the current one as the header does. It is a `<details data-menu>`, as the account and theme
  menus are, so it works without JavaScript and menus.js closes it. The prototype's phone header offers no way to
  these pages.
- **Proposed: `/` focuses the header's search field**, as the prototype, when nothing else has focus; the small script
  that closes menus gains that.
- **Proposed: the FAQ's answers describe Rulemart as it is.** "Who can publish a library?" says anyone can list a
  public library and that Rulemart vets the ones it shows by default; "Do I need a Rulemart account?" names the
  perks that exist, with project tracking and the picker added by slice R7; "How do I give feedback?" points a rule's
  feedback at the library's repository, since Discuss comes later (rulemart#27); "Should I stay in sync with a rule
  or fork it?" says the checkout prompt pins each library to the release the visitor saw, until slice R5's checkout
  stops pinning; and "How are rules versioned?" says an update applies every change once confirmed, as
  `code-rules project update` does. "What are rules, groups, and libraries?" names real rules from
  fabricahq/public-rules, "Add operation and identifier context to errors at boundaries" in Go and "Keep tests
  independent" in Testing, and groups that hold rules today, rather than the prototype's invented ones.
- **Proposed: feedback topics open prefilled GitHub issues**: Rulemart topics in fabricahq/rulemart, Code Rules
  topics in fabricahq/code-rules, and "Anything else" in fabricahq/rulemart, the page's owner, with the title prefix
  `[<topic label>] `, the prototype's body, and the labels `feedback` and `topic:<key>`, which both repositories
  have. GitHub drops labels for people who can't set them, so the title prefix carries the topic too. "Something
  broken on Rulemart", a row under Rulemart, leads to the existing Report a problem issue form, so the footer's link
  moves to the feedback page.
- **Decided (Josh): the footer's links are About, Feedback, and Privacy, in that order**, Feedback replacing Report a
  problem and leading to the page that offers it; "Fabrica" in "Fabrica / Rulemart" links fabricahq.com; and Source on
  GitHub is GitHub's mark, an icon link named "Source on GitHub", just left of the theme menu and the same size. On a
  phone, the footer's links stand on a line of their own under Rulemart's name and the two icons. The footer has no
  About Code Rules: the about page's text leads to Code Rules.
- **Decided (Josh): the footer doesn't repeat the header's sections.** Techs, Practices, Libraries, and FAQ stay in
  the header; where it hides them, its menu button opens them.

## Not in this slice

- The redesign of group pages, search, and the unvetted opt-in (R4), stars on rules (R3), the browser cart (R5),
  the dashboard and `/me/add` (R7), and the brand mark (R2).
- Owner display names, kinds, and bios.

## Verification

- `make check` and `make check-generated` pass.
- Page tests cover: each new route's title, heading, and links; the redirects from `/groups` and
  `/groups/{kind}/{name}`; `/o/{login}` for a reserved and an unreserved login; the owner page for an owner with
  vetted, only unvetted, and no libraries; the lowercase redirect for the new sections; the tiles' counts and
  sort; the Popular chips; the browse rows and the other-groups link and page, empty and not; the FAQ and feedback
  pages' links and labels; the sitemap's and canonical links' new addresses; the footer's links; the header's
  current link on each section; and the header's menu's links, current link, and place in the focus order.
- In a browser, every route above at 1280, 390, and 320 pixels, light and dark, beside the prototype, with no
  horizontal scroll and no console error, signed in and out.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
