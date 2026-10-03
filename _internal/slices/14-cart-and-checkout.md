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

- **Proposed: the cart is a `localStorage` entry**, `rulemart-cart`, holding the prototype's state: `cart` (ordered
  keys `owner/repo::group/slug` and `group::owner/repo::group`), `fork` (per rule key), `full` (per library),
  `project`, `repo`, and `confirmed` (per unvetted library). At most 100 items. A script, `cart.js`, owns it: it
  paints the badge and each page's In cart state from data attributes the page renders (the page's library, group,
  and rule keys), handles the modal, and renders `/cart`.
- **Proposed: `/cart` is one server-rendered page whose content the script fills**, by posting the cart's keys to
  `POST /cart/checkout.json` and rendering the response. The endpoint, same-origin only, resolves each key against
  the catalog and answers with each library (owner, name, avatar, vetted or not, latest release), each item (title,
  group name and ID, version, state: ready, retired, missing, gone, unvetted), the rules each whole group brings,
  and the prompt and commands texts for the choices sent (forks, full, project, repository). The texts are built
  in the catalog domain, as slice 8's were, with golden tests. Without JavaScript, `/cart` explains that the cart
  needs it.
- **Proposed: the prompt and commands follow the prototype's text**: the intro by mode (known project, new project,
  unknown), "From <Library> (<id>):" with one line per whole group, synced rule, or forked rule, "Run:" with the
  commands, and the closing line about AGENTS.md and `code-rules project update`. Commands: `# From the root of
  …`, the install and `code-rules project init` when the project is new or unknown, `code-rules project add
  library <alias> --repository … --groups … --rules …` per library, `code-rules project add rule <id> --from
  <alias or URL>@<version>` per fork, with `--reason` when the group is also imported whole, then
  `code-rules project sync` or `project build` for forks only. The alias is the owner with a trailing `hq` removed
  and non-alphanumerics dropped. Nothing is pinned by default; the Commands tab's footnote says how to add
  `ref: release/<n>`. Each command is verified against the real CLI as slice 8's prompt was.
- **Proposed: an unvetted library's items are allowed only after the modal's confirmation**, recorded per library in
  the cart, and checkout names each unvetted library in the prompt and asks the agent to review its rules first,
  as slice 8 did. A library that loses its vetting after being added needs confirming again at checkout.
- **Proposed: items whose rule is retired or gone, or whose library left Rulemart, stay in the cart and say so**, and
  checkout leaves them out, as slice 8 did; the script shows the state the endpoint reports.
- **Proposed: migration 00014 drops `cart_items`**, which was never released, with the same guard 00013 uses: it
  refuses if the table holds rows. The cart store, app, and web routes under `/account/cart` are removed, with
  their tests; the checkout text builders move to the new endpoint.
- **Proposed: the header's badge counts items**, so a whole group counts as one, as the prototype.
- **Proposed: `cart.js` is the site's one feature script**, in `static/`, plain and framework-free, loaded on every
  page with `defer`, under the existing content security policy (no inline script). The modal uses `<dialog>`.
- **Existing:** the content security policy, static file hashing and caching, the `/` shortcut and theme scripts.

## Not in this slice

- The library page's Groups tab and group page controls' final layout (R6), the project picker's projects (R7),
  sharing a cart.

## Verification

- `make check` and `make check-generated` pass. Golden tests for the prompt and commands in every mode, with
  forks, full groups, several libraries, and an unvetted library.
- Endpoint tests: resolution of every item state, unknown keys, the 100-item cap, cross-origin refused, unvetted
  without confirmation, a library gone, the texts.
- Browser tests (Playwright or chrome-devtools-axi driven from a test script, as the repository allows) of the
  script: add a rule and a group, badge count, In cart states, the modal's two choices and the unvetted
  confirmation, remove, clear, fork, full, project field, tab switch, copy, the empty state, and the no-JavaScript
  message. Each flow's commands run against the real `code-rules` CLI in a scratch repository, as slice 8 did.
- In a browser beside the prototype's cart at 1280, 390, and 320 pixels, light and dark.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
