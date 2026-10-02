# Slice 8: the cart and checkout

## Goal

A visitor collects rules from one or more libraries, as single rules, whole groups, or whole libraries, then checks
out: Rulemart writes a prompt they give their coding agent, which sets their project up to import exactly those rules
with [Code Rules](https://code-rules.fabricahq.com), each library pinned to the release the visitor saw. Adding works
without JavaScript, and pages for visitors who aren't signed in stay the same for everyone and cached.

Decisions marked **Proposed** are new in this slice and wait for review. **Decided** ones are Josh's. **Existing**
ones are already in [decisions.md](../decisions.md) or an earlier slice. decisions.md said only that unvetted rules
can go in the cart after an explicit confirmation, and that the checkout prompt names them.

## What a visitor can do

- **Add to the cart.** A library's pages show "Add library to cart", which adds every group; each row of its Groups
  tab shows Add, which adds the group; and a current rule's page shows "Add to cart". Signed in, each adds the item and
  returns to the page, which says "Added to your cart." and shows the control as In cart, focused, leading to the
  cart. A rule whose group, or whole library, is in the cart shows In cart too. Signed out, each control leads to the
  sign-in page, which says "Sign in to collect rules in your cart. You'll come back to this page.", and returns there.
- **Add from an unvetted library, deliberately.** On an unvetted library's pages, each control leads to a page under
  the unvetted warning that names the item, says checkout will name the library to the agent, and offers "Add to cart
  anyway" or Cancel.
- **See the cart.** The header's cart, with its count, leads to `/account/cart`, as "Your cart" in the account menu
  does: each library's items, the whole library first, then groups, then rules, with Remove, Empty cart, and Check out.
  An item checkout leaves out says why, and one another item imports already says which.
- **Check out.** `/account/cart/checkout` lists the libraries checkout imports, each pinned to its latest release, and
  shows the prompt, with Copy prompt, and, under "Or set it up yourself", the configuration and the commands.

## Decisions

### Who has a cart

- **Proposed: the cart needs sign-in, and lives in Postgres.** A cart in a cookie would make every page that shows
  it, every library and rule page, depend on that cookie: CloudFront would need it in its cache key, which is an
  infrastructure change, and every visitor with a cart would then miss the shared cache on every page, as signed-in
  visitors do. A cookie also holds about 4 KB, some 40 rule IDs, and the server would trust IDs the browser can edit.
  Signed-in pages are already private and uncached (**Existing**), so they can show the cart's state for free, and a
  cart follows the visitor to another browser. Signing in is one GitHub click for the developers Rulemart is for.
  Reversible: an anonymous cart can come later by adding its cookie to the cache key.
- **Proposed: signed-out controls are links to sign in, as stars' are** (**Existing** pattern), so public pages stay
  identical for everyone. A POST without a session, such as from a tab whose session ended, does the same and changes
  nothing.
- **Known: adding after signing in takes a second click.** Stars prompt once after sign-in; the cart doesn't yet,
  since the page shows several controls and the prompt would need to name one.

### What goes in it

- **Proposed: three kinds of item: a whole library, a group of one library, or one rule.** They map one to one onto
  Code Rules' configuration: `groups: "*"`, an entry of `groups`, and an entry of `rules`. A canonical group across
  libraries isn't an item: Code Rules imports per library, and adding Testing from every library would import rules
  the visitor never saw.
- **Proposed: items are named by the library's row and the ID in the library**, not by a rule's row, so an item
  stays when its library releases again, and the cart can say what became of it.
- **Proposed: overlaps are allowed, and checkout drops what another item imports.** A rule whose group is in the
  cart, or anything of a library that's in the cart whole, shows as included with it, and the configuration leaves it
  out, as Code Rules asks (it warns about a listed rule whose group is selected). Removing the group leaves the rule.
- **Proposed: at most 100 items a cart.** It bounds each account's rows and the prompt, and leaves room for every
  group of several libraries. A full cart returns to the page saying so, and suggests a whole group instead of its
  rules. Two items added at once can't both take the last place: adding locks the account's row.
- **Proposed: only what a library's pages show can be added**: a vetted or listed library, a group that holds
  current rules, and a current rule. Anything else, including text no GitHub name or Code Rules ID can hold, such as a
  NUL, is missing, a 404, as slice 7 answers unrepresentable input.

### When the catalog changes

- **Proposed: items stay, and the cart says what happened; checkout leaves them out.** A rule a release retired says
  so, with a link to its page, which names what replaced it; a rule the library no longer has, or a group without
  current rules, says so; a library that's neither vetted nor listed says it's no longer on Rulemart. The visitor
  removes them. Removing them silently would lose what the visitor chose without telling them.
- **Proposed: an item of a library Rulemart doesn't vet needs a recorded confirmation.** The cart keeps when the
  visitor confirmed adding it as unvetted. A library that loses its vetting leaves its items unconfirmed, so checkout
  leaves them out until the visitor confirms again, from the cart, through the same page. A library that gains its
  vetting needs nothing.

### Checkout

- **Proposed: checkout writes nothing.** It's a GET page that reads the cart and the catalog, so reloading it is
  harmless, and the cart stays until the visitor empties it.
- **Proposed: each library is pinned to its latest release with Code Rules' `ref: release/<n>`**, the release the
  checkout page names and links. Code Rules imports every selected rule at the version that release published, and
  `code-rules project update` doesn't move a source with `ref`, so the project gets exactly what the visitor saw. The
  prompt says how to upgrade: change `ref` to a newer release's tag and run `code-rules project sync`, or delete `ref`
  and run `code-rules project sync`, after which `code-rules project update` previews newer versions. Pinning each
  rule with `pins` instead would need a reason per rule, and wouldn't hold the rules a selected group gains.
- **Proposed: the prompt tells the agent what to put in `.code-rules/config.yaml`, and which commands to run**: check
  `code-rules --version` is 0.2.0 or later, the first with `rules` and `ref`, and ask before installing; run
  `code-rules project init` unless the project has a configuration; add the sources, merging with any it has; run
  `code-rules project sync`, then `code-rules project check`; check the generated rules by their source-qualified IDs;
  connect the agent's instruction file as `.code-rules/README.md` says; and report, committing only when asked. It
  gives YAML rather than `code-rules project add library` commands, because a project may already import the library,
  which that command refuses, and an agent merges YAML with what's there.
- **Proposed: each library becomes a source named for its repository**, lowercased and hyphenated as Code Rules
  requires; an organization's `.code-rules` repository takes the owner's name; a name that would start with a digit,
  or be `local`, which Code Rules reserves, or that two libraries share, adds the owner's; a number settles the rest.
  The prompt says to use the project's own source name when it imports the repository already.
- **Proposed: the prompt holds no text a library wrote**: only IDs, which Code Rules' format restricts, GitHub
  addresses, and Rulemart's own words. A rule's title could otherwise write instructions into the prompt.
- **Proposed: the prompt names each unvetted library, its source, and where its files land, and asks the agent to
  read its rules, and say which ask for anything unsafe or unexpected, before following any.** The checkout page says
  so beside the libraries, and badges each unvetted one.
- **Proposed: the page also offers the plain configuration and commands**, under "Or set it up yourself", for a
  visitor who'd rather edit the file.
- **Proposed: the prompt is selectable without JavaScript, and Copy is a small enhancement.** Its block has
  `user-select: all`, so one click selects all of it. `copy.js` shows the Copy buttons, which stay hidden without a
  script, copies with the Clipboard API, and selects the text instead when the browser refuses.

### Pages and feedback

- **Proposed: the header shows the cart with its count, for signed-in visitors, from a phone's width up**, inside the
  account slot, which already reserves that width for the Sign in button, so signing in never moves the links. On a
  phone, the avatar shows the count, and the menu's "Your cart" says it.
- **Proposed: an addition's page says "Added to your cart." and focuses the new In cart control**, as starring does
  since slice 7: a rule's or a whole library's control by `autofocus`, since a page has one of each, and a group's,
  one of several, by the return address's fragment, which the server can't see when it renders. Removing says
  "Removed from your cart."
- **Proposed: a failure to read the cart fails the page** with the usual 503 (**Existing** pattern), rather than show
  a wrong count or offer to add what the cart holds.

### Writes

- **Proposed: `POST /account/cart?library=owner/name[&group=ID|&rule=ID][&return=…][&unvetted=confirmed]` adds,
  `POST /account/cart/remove?…` removes, and `POST /account/cart/empty` empties.** Writes are POSTs with an empty body
  and their input in the action's query string, refused when another site starts them (**Existing**). Adding and
  removing are separate, so repeating either lands where once does. Each returns to its checked `return` parameter,
  or the cart. The confirmation, `/account/cart/confirm`, and checkout, `/account/cart/checkout`, are GET pages.

### Data and roles

- **Proposed: migration 00012 adds `cart_items`**: an account, a library, the item's kind and ID, when the visitor
  confirmed it as unvetted, and when it was added, keyed by the account, library, kind, and ID. Deleting the account
  empties its cart, so "Rulemart deletes everything it keeps about you" stays true. It only adds a table, which the
  release still running doesn't read.
- **Proposed: no new role.** A cart is an account's, as stars are, so `rulemart_accounts_writer` gets `SELECT`,
  `INSERT`, and `DELETE`, and `UPDATE` only of the confirmation. Reading a cart joins the catalog, which
  `rulemart_web` reads through `rulemart_catalog_reader` (**Existing**). The worker gets nothing.
- **Proposed: the cart lives in the catalog context**, as stars and listings do: `domain` holds items and checkout,
  pure; `app.Cart`, `store.Cart`, and `queries/cart.sql` the rest. Every item names a library, group, or rule, and the
  cart reads them as pages do, from one snapshot. A context of its own would need the catalog's tables in its SQL, or
  a second read per library through the catalog's app. decisions.md expected a context of its own; this reverses that.
- **Proposed: the web function logs nothing new.** Its access log records each change's route and status, by pattern
  (**Existing**).

## Infrastructure

None. Migration 00012 grants roles that exist; CloudFront already forwards every method and keys on the session cookie.

### Order

1. Release Rulemart with this slice. Its pre-publish workflow runs 00012.
2. Pin the release and apply `web_lambda`. Before then, the running release ignores the table.

## Verification

- **Domain tests:** parsing what forms name, refusing what names nothing; which item covers another; and golden files
  of the configuration and prompt for a one-library cart and a three-library cart with a whole library, a group, a
  covered rule, and an unvetted library; source names for colliding, odd, and reserved names.
- **Store tests**, as the web function's role: adding by any spelling once, only current items of vetted or listed
  libraries, confirmation, the limit with two adds at once, removing and emptying only one's own, deleting an
  account, and reading items as the catalog has them after a release retires and drops rules.
- **App tests:** each item's state and what checkout imports.
- **Page tests:** signed-out links and the cached public page; signed-in forms and private pages; In cart, also for
  covered rules; repeating; the unvetted confirmation; missing and unrepresentable items; a full cart; feedback and
  focus; signing in first; return paths; cross-site posts refused; the cart and checkout pages, empty and full; the
  header, menu, and account page; failed reads; no cart.
- **End to end against Postgres:** a visitor fills a cart from a library's pages, checkout shows the configuration,
  another visitor's cart is empty, and deleting the account empties it.
- **With the real Code Rules 0.3.0:** the configuration two carts checked out locally, one library with a group, a
  covered rule, and a rule; and two libraries with groups and rules, synced into new projects with
  `code-rules project sync`, passed `code-rules project check`, and imported exactly the rules the prompt lists, and a
  whole library imported its 127 rules. Changing `ref` from `release/5` to `release/6` and syncing moved the rules; deleting
  `ref` and syncing kept them, and `code-rules project update` then previewed.
- **In a browser, locally:** both libraries ingested, a listed copy as an unvetted one, `make web-dev`, then adding by
  clicking, the confirmation, the cart with every state, and checkout, at 1280, 390, and 320 pixels, light and dark,
  with no console errors or sideways scrolling.

## Not in this slice

An anonymous cart. Adding a canonical group from every library at once. Excluding or pinning single rules. Sharing a
cart, or a link to a checkout. Prompting to add after signing in.
