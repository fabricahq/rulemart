# Slice R7: dashboard and add a library

## Goal

Give a signed-in visitor the prototype's dashboard: the libraries they and their organizations publish, the
projects of theirs that use Rulemart libraries and whether updates wait, their starred rules, a picker to add a
library from their repositories, and the optional GitHub App install for private repositories. Sign-in grows to
ask for organization membership. [realignment.md](../realignment.md) records the decisions, and the two GitHub
apps exist.

## What a visitor sees

- **Sign in** (`/signin`): Fabrica's mark, "Sign in to Rulemart", the three perks ("Star the rules you find useful",
  "Track the libraries your projects use", "Publish your libraries and see who uses them"), Continue with GitHub,
  and "Browsing needs no account. Rulemart reads your public profile and public repos, and never writes to
  GitHub." GitHub's authorize page then asks for identity and organization membership.
- **The dashboard** (`/me`): avatar, the visitor's name, "@login · member of <orgs>", and two tabs, **My libraries**
  and **Starred rules (N)**.
  - **My libraries**: the note "Showing public repos only. Include private projects" or "Including private
    projects from the repos you selected. Private projects are only visible to you. Manage". Then **Published by
    you and your orgs (N)**: each library with a "New" chip when added today, its ID and rule count, and its total
    stars; "+ Add a library". Then **Used in your projects (N)**: per library, each repository that imports it, with
    "(private)", and "N rule updates" or "up to date", and the project count. The footnote "Read from each project's
    `.code-rules/generated/provenance.json`."
  - **Starred rules**: the rows from R3, or the empty state. Below them, under "No longer counted", any star of the
    visitor's that counts toward no current rule of a vetted library (the rule was retired without a replacement, its
    replacement chain passed the bound, or its library lost its vetting), each with Unstar, so the visitor can let go of
    a star R3 can neither count nor list (R3's open item).
- **Add a library** (`/me/add`): the eyebrow "Publish", "Add a library", the intro, the panel **Your libraries on
  GitHub** listing each repository of the visitor's or their organizations' that holds a `rule-library.yaml` and a
  `release/<n>` tag: public ones with "Add this library", ones already on Rulemart with "✓ On Rulemart" (and "via
  the <org> organization"), private ones dimmed with "Private libraries can't be published on Rulemart". The note
  about public libraries and "Include private repos". Then "Or add any public library by URL" with the form, whose
  errors read as the prototype's.
- **Adding** (`/me/add/run?repo=`): "Adding owner/repo" and the checklist that ticks as the listing's check runs:
  "Found rule-library.yaml in …", "Read library release release/N · N rules at their published versions", "N
  groups, N rules indexed · license X", "Watching for new library releases"; then "owner/repo is live on Rulemart."
  with "View library page" and "Back to Dashboard". A failed check shows the failure where the tick would be, with
  "Try again" and "Remove", as the listings page did.
- **Private projects** (`/me/private`): the prototype's page: the Optional chip, the permissions table, "We never"
  and "Worth knowing", and "Continue to GitHub" (the app's install page) with "Skip, public repos only", or "Done"
  and "Remove access to private repos".
- **Checkout's project picker** lists the same projects.
- **The account menu** reads: name, @login, Dashboard, Add a library, Starred rules, Sign out. Account settings
  (sign out everywhere, delete) live at the bottom of the dashboard under "Account".

## Decisions

- **Proposed: sign-in asks for `read:org`**, so Rulemart can list the visitor's organizations and their
  repositories, including memberships they keep private. The authorize page says so. Everything else about the
  flow stays (state, PKCE, the flow cookie, return paths).
- **Proposed: the OAuth token is kept, encrypted, in the session row**, with a key from SSM (`/rulemart/prod/
  token-key`, 32 random bytes, AES-GCM), and deleted with the session. The dashboard reads GitHub with it at
  sign-in and when the visitor presses Refresh, never on every page view; results are cached per account in a
  `github_snapshots` table (organizations, publishable repositories, projects and their sources) with the time
  read, shown with "as of" and refreshed at most once a minute. Migration 00016 adds the column and the table,
  with grants to `rulemart_accounts_writer`. Reverses slice 5's "discard the token".
- **Proposed: what the scan reads.** With the visitor's token: `/user/orgs`, then the visitor's and each
  organization's repositories (public ones, plus private ones where the GitHub App is installed, read with the
  app's installation token). For each repository: does its default branch hold `rule-library.yaml` and a
  `release/<n>` tag (publishable); does it hold `.code-rules/generated/provenance.json` (a project), parsed by
  the Code Rules parser for its sources and pinned versions. Update counts compare each source's rule versions
  with the library's current versions in the catalog. Repositories are listed with the GitHub search API where it
  cuts the work (`filename:rule-library.yaml user:<login>`), with the REST fallback. Budget: at most 200
  repositories a scan; past that the dashboard says so.
- **Proposed: the GitHub App "Rulemart by Fabrica"** handles private repositories: `/me/private` sends the visitor to
  `https://github.com/apps/rulemart-by-fabrica/installations/new`; GitHub returns to `/me/github/installed` with
  the installation ID, which Rulemart records on the account after checking with the app's token that the
  installation belongs to the visitor; a webhook at `/github/webhook` (secret in SSM) records installation
  changes and removals. The app's private key lives in SSM (`/rulemart/prod/github-app-key`), with its App ID and
  client ID as variables. "Remove access" links to the installation's settings on GitHub and forgets the
  installation.
- **Proposed: `/me/add` lists publishable repositories from the snapshot**, and "Add this library" posts the existing
  listing flow for that repository, then shows `/me/add/run`, which polls the listing's state every two seconds
  with a small script, or reloads without one. The URL form keeps the existing input checks. Private publishable
  repositories are shown but can't be added.
- **Proposed: "Published by you and your orgs" are the vetted or listed libraries whose owner is the visitor or one of
  their organizations**, with totals as the sum of rule stars; "New" marks ones listed within the day.
- **Proposed: the prototype's URLs**: `/signin` (from `/sign-in`, redirect), `/me`, `/me/add`, `/me/add/run`,
  `/me/private`, `/me/github/installed`, `/signout` (POST), and the account actions under `/me/account/...`. The
  old `/account/...` addresses redirect. `me` is a GitHub user, so an owner named `me` is at `/o/me` (R1).
- **Proposed: the privacy page grows** to name the token, the snapshot, the organizations, repository names, and
  provenance data Rulemart keeps, for how long (until sign-out or deletion), and that private repository data is
  shown only to the visitor.
- **Existing:** accounts keyed by GitHub ID, sessions, private responses for signed-in pages, the listing worker and
  its limits, the listing failure states.

### Decided while building

The spec's Proposed decisions are built as written, except where an entry here says otherwise and why.

- **Proposed: GitHub is read on the first page after sign-in that shows it, not inside the callback.** Each sign-in
  discards the account's snapshot, in the transaction that adds the session, and the dashboard, `/me/add`, or
  checkout reads GitHub when it finds none. A read of 200 repositories takes a few seconds, which would hold every
  sign-in's redirect, whatever page the visitor returns to; most never open the dashboard. Refresh reads again at
  most once a minute.
- **Proposed: a read lists repositories with GitHub's REST API, not its search.** Code search finds a repository only
  once GitHub has indexed it, so a library pushed a minute ago would be missing; it allows 10 requests a minute, which a
  visitor in five organizations exceeds; and it can't see what an installation reads. The REST read costs one request
  per repository, a listing of its root, plus its release tags or provenance file only where the root holds
  `rule-library.yaml` or `.code-rules`, eight at a time, within `domain.MaxRepositories`, the 200 most recently pushed
  across the visitor and their organizations (at most `domain.MaxOrganizations`, 100), and a 25-second deadline.
- **Proposed: provenance is parsed by Rulemart, in `accounts/domain`, from the fields Code Rules writes.** Code Rules
  has no provenance parser to copy, only the writer in `internal/build/output.go`; the reader takes each source's
  name, repository, release, and groups, and each rule's ID, origin, and version, with the vendored
  `coderules.ParseRuleVersion` and `ValidateRuleID`, and skips a local or forked rule, which holds no library's
  version. A file it can't parse makes no project rather than failing the read. Its test reads Rulemart's own
  `provenance.json`. Replace it with Code Rules' parser once one ships.
- **Proposed: an update is a rule the library has since published a newer version of, or retired.** Rules a library
  added to a group the project imports aren't counted: the project's configuration may exclude them, which provenance
  doesn't record, so counting them would show updates that never come. Counts are computed as the dashboard reads,
  from the catalog's current versions, so a new library release shows at once without another read of GitHub.
- **Proposed: a failed read keeps the last snapshot and says so.** GitHub failing, rate limiting, or timing out leaves
  the snapshot that was, with "Rulemart couldn't read your repositories on GitHub just now", when that one was read,
  and Try again, and the next read waits a minute, so a broken GitHub isn't asked on every page. A token GitHub
  refuses, a session without one, or one sealed under a key since rotated asks the visitor to sign in again, at
  `/signin?again=1`, since only the sign-in page's content security policy lets a form lead to GitHub.
- **Proposed: the webhook is at `/account/github/webhook`.** `/github/webhook` has two segments under `github`, a GitHub
  account, so the route takes the address of a library `github/webhook`, which the routes' tests refuse; `account` is
  reserved and GitHub has no account by that name. The OAuth callback stays at `/account/github/callback`, the OAuth
  app's registered URL, and the dev sign-in at `/account/dev-sign-in`, since neither is a page. The visitor's pages are
  under `/me`, which takes the pages of an owner named `me`'s libraries, as `libraryPageTaken` records.
- **Proposed: an organization's installation is the visitor's only when GitHub says they own the organization.**
  Rulemart checks the installation's account with the app's JWT, and for an organization, the visitor's membership
  role with their own token: only an owner can install an app on an organization, and only an owner can see every
  repository it may read, so a member who reached `/me/github/installed` with an owner's installation ID can't list
  repositories GitHub hides from them. Installations are recorded per account, so each owner who connects an
  organization's installation reads through it.
- **Proposed: the webhook acts on removals and changes, not new installations.** `installation.deleted` and `suspend`
  forget the installation for every account and discard their snapshots; `installation_repositories`, `unsuspend`,
  and `new_permissions_accepted` discard the snapshots; anything else, including another app's deliveries, answers
  204. A new installation is recorded when GitHub returns its installer to Rulemart, the one moment Rulemart knows
  which account it's for. A read that finds an installation GitHub no longer knows forgets it too, for a missed
  delivery.
- **Proposed: Remove access forgets the installations, and the page says how to uninstall the app on GitHub.**
  Redirecting a POST to GitHub's settings breaks `form-action 'self'`, so the button forgets them and returns to `/me`
  with the prototype's toast "Private repo access removed"; the Manage view beside the button links each
  installation's settings on GitHub, where the visitor uninstalls it.
- **Proposed: an account keeps its GitHub profile's name**, refreshed at each sign-in, for the menu and the dashboard's
  head, as the prototype shows "Josh Padnick" above "@josh-padnick". Control and formatting characters are dropped.
  Without a name, both show the login.
- **Proposed: the listings page stays, at `/me/listings`**, linked from the Account section, since the dashboard lists
  only libraries the visitor and their organizations own, and a visitor can add anyone's public library: without it,
  a listing of another owner's repository could be neither seen nor removed once its check page was left.
- **Proposed: the run page's checklist ticks every step at once, when Rulemart has the library.** The worker's check
  isn't observable step by step, so while it runs the first step reads "Looking for rule-library.yaml in …" with a
  spinner; once the listing is listed or vetted, each step says what Rulemart found, from the library's page; a
  failed check shows the reason under the first step, with Try again, which stays on the page, Remove, and Back to
  Dashboard. `poll.js` fetches the page every two seconds and swaps the checklist; without it, `<noscript>` reloads the
  page as often. A check queued more than three minutes ago says "This is taking longer than usual", as the listings
  page does, with Refresh status, and the page stops following it, since the next check is the worker's hourly poll. Following a
  library already on Rulemart, which the visitor didn't list, shows it done. The prototype's "within minutes" reads
  "within the hour", the worker's poll.
- **Proposed: the picker shows a repository's latest release, not its group count**, which would need reading its
  manifest or release record: "Public · release/3". It orders rows as the prototype: what the visitor can add, what
  Rulemart is adding (linked to its check), what Rulemart has, then private ones; a library the visitor's organizations
  publish on Rulemart that the read didn't reach is listed as on Rulemart too. The URL form keeps the listing's checks,
  in the prototype's words, and shows an address it accepts as a row to add, since a GET form can't post.
- **Proposed: Starred rules' No longer counted names why**: the library is no longer on Rulemart, isn't vetted now,
  retired the rule without a replacement, or its replacements end at a rule that isn't current. Unstar posts to
  `/stars/remove`, which, for a rule it can't star, removes the visitor's own star on it.
- **Proposed: GitHub sign-in refuses to start without `TOKEN_KEY` or `TOKEN_KEY_PARAMETER`**, since every session keeps
  a token, and a key given directly is checked at start. The GitHub App's five variables are all or nothing, and need
  GitHub sign-in. A local build without GitHub sign-in serves `githubtest.DevFake` in memory, with a GitHub App whose
  install page returns at once, and a random token key.
- **Proposed: `robots.txt` keeps crawlers off `/me`, `/me/`, `/signin`, and the old `/sign-in` and `/list`**, as it did
  the account pages, alone and with a query, so it doesn't keep them off an owner such as `meta`.

## Infrastructure

The web function reads, as `cmd/web` documents:

| Variable | Value | Secret |
| --- | --- | --- |
| `GITHUB_CLIENT_ID` | the OAuth app "Rulemart"'s client ID (slice 5) | no |
| `GITHUB_CLIENT_SECRET_PARAMETER` | `/rulemart/prod/github-client-secret` (slice 5) | SSM SecureString |
| `TOKEN_KEY_PARAMETER` | `/rulemart/prod/token-key`: 32 random bytes in standard base64, `openssl rand -base64 32` | SSM SecureString |
| `GITHUB_APP_ID` | the GitHub App "Rulemart by Fabrica"'s numeric App ID | no |
| `GITHUB_APP_CLIENT_ID` | the app's client ID, which its JWTs name as their issuer | no |
| `GITHUB_APP_SLUG` | `rulemart-by-fabrica`, the app's name in its install page's address | no |
| `GITHUB_APP_PRIVATE_KEY_PARAMETER` | `/rulemart/prod/github-app-key`: the app's private key, the PEM GitHub gives | SSM SecureString |
| `GITHUB_APP_WEBHOOK_SECRET_PARAMETER` | `/rulemart/prod/github-app-webhook-secret`: the app's webhook secret | SSM SecureString |

Each secret may instead be given directly, without `_PARAMETER`, as locally. The function needs `ssm:GetParameter` on
the three new parameters. The worker needs nothing new; its token stays optional in the code, and infra-live makes it
required, as the realignment planned. The app's settings on GitHub: setup URL
`https://rulemart.fabricahq.com/me/github/installed`, "Redirect on update" on, no OAuth during installation; webhook URL
`https://rulemart.fabricahq.com/account/github/webhook`, its secret the parameter's, events Installation and Installation
repositories; permissions Contents read and Metadata read. The webhook's POST carries a body, which CloudFront's origin
access control signs only when the sender hashes it, so `/account/github/webhook` needs a CloudFront behavior that
forwards it to the Function URL without OAC signing, or a Function URL of its own with `AuthType NONE`; the delivery's
HMAC signature is what authenticates it. The runbook's step 3 lists the parameters, and the pull request the
infra-live change, as a checklist.

## Not in this slice

- Discussion (rulemart#27), library stars, a second code host.

## Verification

- `make check` and `make check-generated` pass, including a fake GitHub for the scan, the app's installation
  check, and the webhook signature.
- Store tests: the encrypted token round trip, the snapshot's writes and reads, the installation record, and
  deletion removing all of it.
- Page tests: every dashboard state (no orgs, no projects, private on and off, failures), the picker's three kinds
  of row, the run page's states, the private page both ways, the menu, the redirects, the privacy text.
- In a browser beside the prototype's `/me`, `/me/add`, `/me/private`, and checkout picker, at 1280, 390, and 320
  pixels, light and dark. Done with `make web-dev`'s fake GitHub and the dev sign-in, since the real OAuth app's and
  GitHub App's secrets weren't available to the slice's agent: a real account's read waits for the infra-live change.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
