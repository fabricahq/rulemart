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

## Infrastructure

In infra-live, beside slice 5's units: the token key and the app's private key and webhook secret as SSM
SecureStrings, the app's IDs as variables, the worker token made required, and the webhook route on CloudFront
(POST with a body, so the function URL's signing needs the body hash; a Lambda function URL route of its own if
CloudFront can't forward it). Documented in the slice's PR and the runbook.

## Not in this slice

- Discussion (rulemart#27), library stars, a second code host.

## Verification

- `make check` and `make check-generated` pass, including a fake GitHub for the scan, the app's installation
  check, and the webhook signature.
- Store tests: the encrypted token round trip, the snapshot's writes and reads, the installation record, and
  deletion removing all of it.
- Page tests: every dashboard state (no orgs, no projects, private on and off, failures), the picker's three kinds
  of row, the run page's states, the private page both ways, the menu, the redirects, the privacy text.
- In a browser with the real OAuth app "Rulemart (local)" against the visitor's own GitHub account, beside the
  prototype's `/me`, `/me/add`, `/me/private`, and checkout picker, at 1280, 390, and 320 pixels, light and dark.
- Then the verification [realignment.md](../realignment.md) sets for every slice.
