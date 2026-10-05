# Slice R5: cart and checkout

## Goal

The prototype's cart: it lives in the visitor's browser, needs no account, and checks out into one prompt or a set
of commands for any project. Slice 8 built a cart in Postgres behind sign-in; its checkout safeguards stay, its
storage goes. [realignment.md](../realignment.md) records the decision.

## What a visitor sees

- **The header's cart icon**, for everyone, with a count badge once the cart holds anything.
- **Add to cart** on a rule page opens the prototype's modal: "What would you like to add?" with "Just this rule"
  ("Adds only 'title'. It stays in sync with the library, and nothing else from the group is added.") and "The whole
  <Group> group" ("Adds this rule and the N other … New rules the library adds to this group arrive when you
  update."), and the footnote about choosing a project and forking at checkout. For a rule in an unvetted library,
  the modal first shows the amber warning and asks the visitor to confirm. Afterward the head shows "✓ In cart",
  Checkout, and "Remove from cart", or "✓ <Group> group in cart" when the group covers it.
- **Add <Group> group to cart** on a library group page and **Add N groups to cart** from a library page's Groups
  tab (both pages come with R6; this slice gives them the control and the cart logic).
- **A toast** says "Added to cart", "Added N groups to cart", or "Removed from cart".
- **`/cart`**, empty: the cart icon, "Your cart is empty", the prototype's sentence, and "Browse techs" and "Browse
  practices". With items, the title "Checkout" and three numbered cards:
  1. **What you're adding**: items grouped by library (avatar, name, ID). A rule row has a document icon, the title,
     "Rule in <Group> · version", a segmented "Stay in sync / Fork" control, and ×. A group row has the group's icon,
     name, a "Whole group" tag, "N rules · id", the rules' titles as a list, "Stays in sync", and ×. Per library,
     the checkbox "Also add the other <Group> rules (N more)" when picked rules leave others of their group out.
     "Clear cart".
  2. **Where it goes**: signed out, "Pick from your projects / Sign in to choose a project and see when its rules
     have updates" with a Sign in with GitHub button, an "or" divider, and "Enter your project" with the repository
     field ("GitHub repository (optional, so the prompt names it)"). Signed in, the visitor's projects as radios
     (from R7; until then the signed-in view is the field with "We didn't find any of your projects using Code
     Rules"), and "+ Or use a project that doesn't use Code Rules yet". "New to Code Rules? The prompt sets it up for
     you."
  3. **Finish checkout**, a sticky aside: "Give your agent the prompt, or run the commands yourself. Both update as
     you change your cart.", the **Prompt** and **Commands** tabs, the live text with changed lines briefly
     highlighted, "Copy prompt for agent" or "Copy commands", and "Rules move to newer versions only when your
     project runs `code-rules project update`."

## Decisions

- **Decided (Josh), 2026-10-05: the cart is a `localStorage` entry**, `rulemart-cart`, holding the prototype's state: `cart` (ordered
  keys `owner/repo::group/slug` and `group::owner/repo::group`), `fork` (per rule key), `full` (per library),
  `project`, `repo`, and `confirmed` (per unvetted library). At most 100 items, and each choice lasts only while the
  cart holds its rule or an item of its library, so the checkout's maps of libraries stay within the 100 the server
  allows. A script, `cart.js`, owns it: it
  paints the badge and each page's In cart state from data attributes the page renders (the page's library, group,
  and rule keys), handles the modal, and renders `/cart`.
- **Decided (Josh), 2026-10-05: `/cart` is one server-rendered page whose content the script fills**, by posting the cart's keys to
  `POST /cart/checkout.json` and rendering the response. The endpoint, same-origin only, resolves each key against
  the catalog and answers with each library (owner, name, avatar, vetted or not, latest release), each item (title,
  group name and ID, version, state: ready, retired, missing, gone, unvetted), the rules each whole group brings,
  and the prompt and commands texts for the choices sent (forks, full, project, repository). The texts are built
  in the catalog domain, as slice 8's were, with golden tests. Without JavaScript, `/cart` explains that the cart
  needs it.
- **Decided (Josh), 2026-10-05: the prompt and commands follow the prototype's text**: the intro by mode (known project, new project,
  unknown), "From <Library> (<id>):" with one line per whole group, synced rule, or forked rule, "Run:" with the
  commands, and the closing line about AGENTS.md and `code-rules project update`. Commands: `# From the root of
  …`, the install and `code-rules project init` when the project is new or unknown, `code-rules project add
  library <alias> --repository … --groups … --rules …` per library, `code-rules project add rule <id> --from
  <alias or URL>@<version>` per fork, with `--reason` when the group is also imported whole, then
  `code-rules project sync` or `project build` for forks only. The alias is the owner with a trailing `hq` removed
  and non-alphanumerics dropped. Nothing is pinned by default; the Commands tab's footnote says how to add
  `ref: release/<n>`. Each command is verified against the real CLI as slice 8's prompt was.
- **Decided (Josh), 2026-10-05: an unvetted library's items are allowed only after the modal's confirmation**, recorded per library in
  the cart, and checkout names each unvetted library in the prompt and asks the agent to review its rules first,
  as slice 8 did. A library that loses its vetting after being added needs confirming again at checkout.
- **Decided (Josh), 2026-10-05: items whose rule is retired or gone, or whose library left Rulemart, stay in the cart and say so**, and
  checkout leaves them out, as slice 8 did; the script shows the state the endpoint reports.
- **Decided (Josh), 2026-10-05: migration 00014 drops `cart_items`**, which was never released, with the same guard 00013 uses: it
  refuses if the table holds rows. The cart store, app, and web routes under `/account/cart` are removed, with
  their tests; the checkout text builders move to the new endpoint. The old pages' addresses, `/account/cart` and
  `/account/cart/checkout`, redirect permanently to `/cart`, as the realignment's old addresses do.
- **Decided (Josh), 2026-10-05: the header's badge counts items**, so a whole group counts as one, as the prototype.
- **Decided (Josh), 2026-10-05: `cart.js` is the site's one feature script**, in `static/`, plain and framework-free, loaded on every
  page with `defer`, under the existing content security policy (no inline script). The modal uses `<dialog>`.
- **Existing:** the content security policy, static file hashing and caching, the `/` shortcut and theme scripts.

### Decided while building

The spec's decisions are built as written, except where an entry here says otherwise and why.

- **Decided (Josh), 2026-10-05: the prompt names rules by ID, not by title**, such as "The rule techs/go/return-errors, without the
  rest of its group", where the prototype writes the rule's title. A title is text a library wrote, and slice 8's
  safeguard, which decisions.md keeps, is that no library writes into the prompt. A whole group is named by its
  canonical name, which Code Rules' list owns, or its ID. "From <Library>" names the repository, `owner/name`, once,
  since the catalog keeps no library display name.
- **Decided (Josh), 2026-10-05: a sync runs before forks, and a build after them.** Checked against Code Rules 0.3.0,
  `code-rules project add rule --from <alias>@<version>` refuses until the source is synced ("missing source record"),
  so the prototype's order, forks then one sync at the end, fails. The commands add the libraries, sync, fork, then
  `code-rules project build`. Forks alone, from libraries the project doesn't import, copy from the repository's
  address and need only the build.
- **Decided (Josh), 2026-10-05: aliases are lowercased and made unique.** The prototype's alias, the owner without a trailing `hq`,
  keeps the owner's case, which Code Rules' `[a-z][a-z0-9-]*` refuses, and gives both of Fabrica's libraries
  `fabrica`, which the second `add library` refuses. Libraries that would share an alias add their repository's name
  (`fabrica-public-rules`), a name Code Rules can't take starts with `library-`, and a known project's own source
  names win and are never reused.
- **Decided (Josh), 2026-10-05: an unvetted library stays pinned to the commit Rulemart saw**, as slice 8 pinned it: its
  `add library` command carries `--ref <commit>`, and both texts give the command that fetches that commit outside the
  project for review first. Nothing else is pinned; the Commands tab's footnote says how to add `--ref release/<n>`,
  which is what writes `ref: release/<n>` to the configuration.
- **Decided (Josh), 2026-10-05: the commands of a cart holding an unvetted library come in two steps**, which the Commands tab shows
  as two blocks, each under its heading with its own Copy: "1. Fetch and review", the command that fetches each
  unvetted library at the reviewed commit outside the project, ending with a comment to read the rules picked from it
  there and stop if any is unsafe, and "2. After you've reviewed, import", the rest. One block wrote the review as
  comments right before the `add library` command, so pasting it imported rules nobody had read, where the prompt makes
  the agent wait for approval. The prompt's commands are the import step, after its own review paragraph. A cart of
  vetted libraries keeps one block, with the Copy below it. The prototype shows one block for every cart.
- **Decided (Josh), 2026-10-05: the intro says which Code Rules it needs** ("with the Code Rules CLI (0.2.0 or later)"), the oldest
  with `--rules` and `--from`, which slice 8's prompt checked for, since an older install, such as 0.1.0 from
  Homebrew, fails the commands.
- **Superseded by [slice R7](16-dashboard-and-add-a-library.md): only a known project's mode reads projects, and R5 knows none**, so every checkout is for a project
  Rulemart doesn't know: the endpoint takes the repository the visitor writes and parses it with the listing form's
  parser, which now also reads `git@github.com:owner/repo.git`. Known and new projects are built and tested in the
  domain, for R7.
- **Decided (Josh), 2026-10-05: the content security policy allows `connect-src 'self'`**, which `fetch` to the checkout needs; it
  allowed no connection before but Cloudflare's.
- **Decided (Josh), 2026-10-05: the cart's In cart green and the dialog's scrim are tokens**, `--ok`, `--ok-surface`, and `--scrim`,
  the prototype's values, since only tokens are colors.
- **Decided (Josh), 2026-10-05: the library group page exists now, minimally**, at `/{owner}/{repo}/{kind}/{group}` (two parts of a
  rule's address never name a rule, whose ID has three), with the prototype's crumbs, title, rules, and Whole group
  box, so its Add <Group> group to cart has a home. The Groups tab gains a checkbox per group, beside the row's
  existing link, an In cart badge, a link to the group's page, and the Add to cart box above the facts. R6 decides
  the rows' final interaction and finishes both layouts.
- **Decided (Josh), 2026-10-05: the cart's page renders from the checkout's answer**: a second script, `cart-page.js`, loaded only on
  `/cart`, builds item rows with the site's classes, which Tailwind now reads from it too, and the server renders
  everything else, the cards, the project section, and the dialogs, so only what depends on the cart is built in the
  browser. Each change shows at once from the last answer and asks again, at most every 200 ms while typing, keeping
  only the answer for the cart as it is now.
- **Decided (Josh), 2026-10-05: `cart.js` owns the cart, and the page script changes it only through the store's API.** `cart.js`,
  on every page, keeps the cart and paints the header and the controls that add; it exposes `window.rulemartCart`,
  as `toast.js` exposes `window.rulemartToast`, with named operations (add, remove, fork, add the rest of the
  groups, confirm, the repository, clear, drop unknown keys) and a copy of the state. `cart-page.js` never writes
  `localStorage`, and redraws on the `rulemart:cart` event every change sends, from this tab or another.
- **Decided (Josh), 2026-10-05: the "also add the rest of the groups" choice is `restOfGroups` throughout**, in the cart's state, the
  checkout's request and answer, and the code, rather than the prototype's `full`, which read as a full cart.
- **Decided (Josh), 2026-10-05: an unvetted library's items say they're pinned, not in sync.** Checkout pins such a library to the
  commit reviewed, so its rule's card reads "Pinned to the reviewed commit", its whole group's row says the same, the
  dialog and the group page say the rule or group is pinned and that later rules don't arrive, and the Commands
  footnote leaves its rules out of those `code-rules project update` moves. The prototype shows Stay in sync for
  every library.
- **Decided (Josh), 2026-10-05: an unvetted library's rules can't be forked yet.** `code-rules project add rule --from
  <alias>@<version>` finds the release in the repository's tags, not at the source's `--ref` commit, so a publisher
  could move a tag after the review and the fork would copy text nobody reviewed. The card offers no Fork and says
  "Forking waits until the rules are vetted, so you get exactly the reviewed text", the dialog says the same, and
  checkout treats a fork the browser still asks for as a rule kept at the reviewed commit. This waits for Code Rules
  to fork from a commit; the prototype offers Fork for every library.
- **Decided (Josh), 2026-10-05: a group the cart holds shows ticked and disabled on the Groups tab**, so Select all groups, the count,
  the button, and the toast cover only the groups the box would add. The prototype lets the visitor tick it again
  and counts it.
- **Decided (Josh), 2026-10-05: a rule whose whole group the cart holds is included in it.** Its card reads "Included in the <Group>
  group" with only Fork, which still copies it; the badge and the summary count it with the group; and its rule page
  says "<Group> group in cart" with "Remove <Group> group from cart". The prototype lists it as a second item that
  stays in sync, though the prompt leaves it out.
- **Decided (Josh), 2026-10-05: on a phone, a bar at the viewport's bottom adds the ticked groups.** Below the narrow breakpoint the
  Groups tab's Add to cart box follows every group, so once any is ticked a bar says "N selected" beside Add to cart.
  The prototype has none.
- **Decided (Josh), 2026-10-05: the commands end with `# Then make sure AGENTS.md tells agents to read .code-rules/generated/RULES.md`**,
  the instruction the prompt closes with, from one constant, so the Commands tab reminds a visitor running them by
  hand. The prototype's commands end at the sync.
- **Superseded by [slice R7](16-dashboard-and-add-a-library.md): signed in, Where it goes says what counts as a project and keeps "Enter your project"**: "We didn't
  find any of your projects using Code Rules: repositories with `.code-rules/generated/provenance.json`", then the
  heading and the field, which names the project. R7 replaces the sentence with the picker.
- **Decided (Josh), 2026-10-05: a whole group's row lists its first 5 rules and "+N more"**, which links to the group's page, since a
  group such as React brings 39. The checkout sends only those 5 titles and the group's count, and cuts every title it
  sends to 200 characters, since a library may write a title of any length, and a checkout larger than a page may be
  fails as one Rulemart can't give, so the answer always fits a Lambda function's response. The prototype lists them
  all.
- **Decided (Josh), 2026-10-05: a key the browser holds that names nothing is dropped**, on load when it isn't well formed, and when
  the checkout lists it as unknown; an item whose rule is retired or missing, or whose library left, stays and says
  so, with Remove.
- **Decided (Josh), 2026-10-05: toasts a script shows reuse the notice's toast**, from a `<template>` of the notice banner, through
  `window.rulemartToast`, so `toast.js` loads on every page; and the toasts follow the prototype's text, plus
  "Prompt copied" and "Commands copied" on Copy, and a full cart's notice.
- **Decided (Josh), 2026-10-05: below 27rem the header shows Rulemart's name without Fabrica's.** The cart icon joined the menu,
  search, and Sign in on a phone's header, which overflowed at 390 pixels with both names.
- **Decided (Josh), 2026-10-05: the cart's page never offers text the cart has outgrown.** Every change advances the cart's revision
  before the 200 ms wait, so the prompt and commands dim at once, "Updating…" shows, and Copy is off until the answer
  for that revision arrives; an answer or a failure for an earlier revision is ignored. A failed update says
  "Couldn't update the prompt." (or the commands) with Try again, even while an earlier answer shows. A small script,
  `cart-checkout.js`, loaded on `/cart` before `cart-page.js`, keeps that state as plain values, so it's tested
  without a browser.
- **Decided (Josh), 2026-10-05: the scripts' pure logic is tested with Node's own test runner**, in `*.test.mjs` files beside the web
  package's Go tests, outside `static/`, which the site embeds and serves. `make check` runs them through
  `make check-js` when `node` is installed and says it skipped them when it isn't; CI's runners have Node. The
  repository still has no Node packages.
- **Kept as they are after browser QA, deliberately**: the prompt names rules by ID, not title, so no library writes
  into it (above); toasts stay at the viewport's bottom right, Josh's decision for every toast; Clear cart empties
  the cart at once, without asking, as the prototype's does; and the cart outlasts signing out, since it's the
  browser's, not the account's, as the prototype's is.
- **Verified with the real CLI** (Code Rules 0.3.0, built from its v0.3.0 source, since this machine's Homebrew
  install is 0.1.0): the commands of six carts, run in new Git repositories without the installer line, each exited
  0, passed `code-rules project check`, and generated exactly the rules picked; the pull request lists them.

## Not in this slice

- The library page's Groups tab and group page controls' final layout (R6), the project picker's projects (R7),
  sharing a cart.

## Verification

- `make check`, with its JavaScript tests, and `make check-generated` pass. Golden tests for the prompt and commands in every mode, with
  forks, full groups, several libraries, and an unvetted library.
- Endpoint tests: resolution of every item state, unknown keys, the 100-item cap, cross-origin refused, unvetted
  without confirmation, a library gone, the texts.
- The repository has no browser test harness, so the scripts' flows were checked by hand in Chrome with
  chrome-devtools-axi, with no console errors: adding a rule, and its whole group, from the rule page's dialog;
  the unvetted library's confirmation before the choices; adding a group from its library group page; ticking groups
  on the Groups tab and adding them; the badge count and the In cart states; and on `/cart`, Stay in sync or Fork,
  Also add the other rules, Remove, Clear cart, the repository field, the Prompt and Commands tabs, Copy, and the
  empty state; and, after browser QA, an unvetted rule's card pinned to the reviewed commit, a rule its whole group
  in the cart includes, the Groups tab's disabled in-cart groups and selected rows, its bar on a phone, a whole
  group's "+N more", and the dialog at 320 pixels. The pages' markup, such as the no-JavaScript message, is tested in Go. Each flow's commands ran
  against the real `code-rules` CLI in a scratch repository, as slice 8 did.
- In a browser beside the prototype's cart at 1280, 390, and 320 pixels, light and dark.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
