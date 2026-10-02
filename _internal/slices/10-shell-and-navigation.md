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
  Libraries, and FAQ hide on narrow screens, as the prototype's `hide-md` does; the search field becomes a search
  icon, as today.
- **The home page** (`/`): the eyebrow "Fabrica / Rulemart", the heading "Agent coding best practices, off the shelf",
  the lede, the big search field with the placeholder "Try React effects, logging, testing", and "Popular" chips.
  Then **Technologies** and **Practices**, each a band with "Browse all →" and a four-column grid of tiles, one per
  canonical group that has current rules in a vetted library, sorted by rule count: icon, name, "N rules · M
  libraries". Then **Libraries**, the first four vetted libraries with "All libraries →". Then the band "Stock the
  shelves / Have a public Code Rules library? / List it on Rulemart in a minute. Rulemart updates with every
  library release." with "List your library →".
- **Browse** (`/browse/techs`, `/browse/practices`): the eyebrow "Browse", the title "Technologies" or "Practices",
  tabs between the two, and a row list of canonical groups with current rules in a vetted library: icon, name, the
  group's ID and, for practices, its description; "N rules", "M libraries", a chevron. Below, when any exist, "View
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
- **The footer**: "Fabrica / Rulemart"; About Rulemart, Privacy, About Code Rules, Source on GitHub, Give us
  feedback; the theme menu.

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

### Home

- **Proposed: "Popular" names the two technologies and two practices with the most rules**, since Rulemart has no
  traffic data yet; the prototype hard-codes four. The chips link to the group pages.
- **Proposed: tiles sort by rule count**, as the prototype's, and say "N rules · M libraries"; the libraries band shows
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

### Header, footer, FAQ, feedback

- **Proposed: the header's cart keeps today's behavior**, a link to the cart for signed-in visitors only, drawn as the
  prototype's icon with its count badge, until slice R5 moves the cart into the browser and shows it to everyone. It
  shows at every width, as the prototype's does, so the avatar no longer carries the count on a phone.
- **Proposed: the account menu lists Dashboard, Add a library, Starred rules, Your cart, Your listings, Sign out**,
  leading to today's `/account`, `/list`, `/account/stars`, `/account/cart`, and `/account/listings`, until slices R3
  and R7 fold them into the dashboard.
- **Proposed: `/` focuses the header's search field**, as the prototype, when nothing else has focus; the small script
  that closes menus gains that.
- **Proposed: the FAQ's answers describe Rulemart as it is.** "Who can publish a library?" says anyone can list a
  public library and that Rulemart vets the ones it shows by default; "Do I need a Rulemart account?" names the
  perks that exist, with project tracking and the picker added by slice R7; "How do I give feedback?" points a rule's
  feedback at the library's repository, since Discuss comes later (rulemart#27).
- **Proposed: feedback topics open prefilled GitHub issues**: Rulemart topics in fabricahq/rulemart, Code Rules
  topics in fabricahq/code-rules, and "Anything else" in fabricahq/rulemart, the page's owner, with the title prefix
  `[<topic label>] `, the prototype's body, and the labels `feedback` and `topic:<key>`, which both repositories
  have. GitHub drops labels for people who can't set them, so the title prefix carries the topic too. "Something
  broken on Rulemart", a row under Rulemart, leads to the existing Report a problem issue form, so the footer's link
  moves to the feedback page.
- **Proposed: the footer keeps About Rulemart, Privacy, About Code Rules, and Source on GitHub**, and replaces Report
  a problem with Give us feedback, which leads to the page that offers it.

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
  pages' links and labels; the sitemap's and canonical links' new addresses; the footer's links; and the header's
  current link on each section.
- In a browser, every route above at 1280, 390, and 320 pixels, light and dark, beside the prototype, with no
  horizontal scroll and no console error, signed in and out.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
