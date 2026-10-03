# Slice R8: launch gate

## Goal

Prove the site is the prototype, then put it live. Everything launch-related that the realignment changed is
brought up to date, every prototype route is compared with the site side by side, and the runbook is run.
[realignment.md](../realignment.md) records the decisions.

## What changes

- **The conformance audit covers the signed-in pages too**: `/signin`, `/me`, `/me?tab=stars`, `/me/add`,
  `/me/add/run`, `/me/private`, and the checkout picker, using the `rulemartdev` fake GitHub and test users.
- **Analytics on every page.** The Cloudflare beacon loads on signed-in pages too, since the paths it reports (`/me`,
  `/cart`, `/me/add`) carry nothing about the visitor, and the cart's contents never appear in an address. The
  privacy page says analytics count page views on every page.
- **About, privacy, robots, sitemap.** `/about` describes vetting as the opt-in it now is; `/privacy` covers the
  token, snapshot, and organizations (R7), the browser cart (R5), and rule stars (R3); `robots.txt` disallows
  `/me`, `/cart`, `/signin`, `/o/` is allowed; the sitemap lists `/faq`, the browse pages, owner pages, every
  group page, and asset pages are left out.
- **The conformance audit.** A script under `_internal/audit/` serves the prototype and the site side by side,
  visits every route in the conformance matrix at 1280 and 390 pixels, light and dark, signed in and out, and
  writes paired screenshots to a directory; the differences are reviewed and fixed, and the final set is attached
  to the pull request. The matrix in `realignment.md` is updated to "Match" or to a named, accepted difference.
- **The README.** Remove the note that the live site runs v0.1.0, and reword "Checkout pins each library to the release you saw" once R5 lands, so the README describes the released site.
- **The runbook.** `launch.md` is rewritten for the merged state: the infra-live changes from R7 (infra-live#27,
  stacked on infra-live#25), the release that carries migrations 00013 to 00016, the SSM parameters to set (OAuth
  secret, token key, app key, webhook secret, worker token, analytics token), the GitHub App's settings to confirm
  (setup URL `/me/github/installed`, webhook URL `/account/github/webhook`, the Installation and Installation
  repositories events, which GitHub sends every app), the post-deploy checks, and the announcement. Two facts the
  runbook must state plainly: the webhook route is not wired at launch (the site's Function URL requires signed
  requests, so GitHub's deliveries get 403 until infra-catalog gains a public alias on the Lambda and an unsigned
  CloudFront path; installs and repository changes still reach Rulemart through the setup return, and a suspended
  or uninstalled installation is noticed at the visitor's next read), and rotating the token key needs every warm
  instance to pick up the new parameter (a redeploy), with sessions sealed under the old key asking to sign in
  again.

## Decisions

- **Proposed: the audit script is kept**, as the way to check the site against the prototype after any later
  change, and runs on demand, not in CI, since it needs both servers and a browser.
- **Proposed: every accepted difference from the prototype is named in the matrix**, so the next person knows it was
  chosen, not missed.

## Verification

- `make check` and `make check-generated` pass.
- The audit's screenshot pairs show no unexplained difference.
- Then the verification [realignment.md](../realignment.md) sets for every slice, and the runbook.
