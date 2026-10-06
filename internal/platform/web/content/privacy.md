---
title: Privacy · Rulemart
heading: Privacy
description: What Rulemart keeps about you, why, for how long, and how to delete it.
eyebrow: About
lede: What Rulemart keeps about you, why, for how long, and how to delete it. Rulemart is run by <a href="https://fabricahq.com">Fabrica</a>.
---

## Browsing {#browsing}

Browsing Rulemart needs no account, and sets no cookie. If you choose a color theme in the footer, your browser keeps
the choice, and never sends it to Rulemart.

## Your cart {#cart}

Your cart needs no account either: your browser keeps it, with your choices for it and the repository you enter at
checkout, until you remove them. Rulemart keeps none of it. Your cart's page sends them to Rulemart each time it
changes, to write the prompt and commands that check it out, and Rulemart answers without storing them; its log records
only that the page asked, as for any page.

## Your account {#account}

You sign in with GitHub. Rulemart asks GitHub to read your public profile and the organizations you belong to, and
never to write anything. It keeps your GitHub user ID, your username, the name your profile shows, the address of your
avatar, when you made your account, and when you last signed in, and updates your username, name, and avatar each time
you sign in. Your avatar loads from GitHub's servers.

When you sign in, GitHub gives Rulemart a token for your account. Rulemart stores that token with your session,
encrypted with a key that only its server holds, and uses it to read your GitHub account as the next section describes.
Signing out deletes the token along with the session, whether you sign out of this browser or everywhere. A session you
never sign out of ends on its own after 30 days.

## What Rulemart reads from GitHub {#github}

**What it reads**, using your GitHub token:

- the names of your organizations;
- the names of the public repositories you and your organizations own, at most 200 of them, most recently pushed first;
- whether you have write access to each repository, which decides whether you can add it to Rulemart;
- once you install the GitHub App "Rulemart by Fabrica", the private repositories you chose, read the same way.

**What it looks for in each repository:**

- A `rule-library.yaml` file and `release/<number>` tags. Together they make a library you could add to Rulemart.
- A `.code-rules/generated/provenance.json` file, which marks the repository as a project. From it, Rulemart keeps
  which libraries the project imports and under what names, which groups it imports, and which rule versions it holds,
  so it can count the updates waiting for the project.

Rulemart keeps nothing else about your repositories, and none of their code.

**Private repositories.** With the app installed, Rulemart collects the same things about the private repositories you
chose as about public ones: their names, whether they hold a rule library, and what their `provenance.json` says the
project imports. It never reads their code. Rulemart uses this only to show those projects on your dashboard and at
checkout, and to count the updates waiting for them. No other visitor ever sees any information about your private
repositories, and they are never included in anything other visitors see.

**The app's installations.** Rulemart remembers which installations of the app it reads your private repositories
through, and checks them with GitHub on every read. If you uninstall the app, Rulemart forgets the installation and
everything it read through it. If you lose ownership of an organization, it forgets that organization's installation
too. If GitHub suspends the app, reads fail until the suspension is lifted.

**How long, and how to delete.** Rulemart reads your GitHub account only when you act: when you sign in, press
Refresh, or return from installing the app. It never reads in the background. {{if .GitHubWebhook}}GitHub also tells
Rulemart when an installation you read through is uninstalled, suspended, or unsuspended, or changes which
repositories it reads. Rulemart then asks GitHub about the installation, forgets it if it's uninstalled, and discards
what it read through it, so the next page that shows your account reads it again. Otherwise, what it did
read{{else}}What it did read{{end}} stays until a later read replaces it, until you choose Remove access to private
repos, which discards it and forgets the installations, or until you delete your account.

## What you add while signed in {#adds}

While you are signed in, Rulemart also keeps the things you add on the site. Each stays until you remove it yourself:

- **Stars:** which rules you starred, and when. Others see only how many stars a rule has.
- **Listings:** which repositories you listed, and when you listed or retried them. A library's page shows the username
  you last signed in with as who added it, while your listing stands. To check that you may add a repository, Rulemart
  reads it from GitHub with your token when you confirm its address and when you add it, and keeps nothing of that read
  beyond the listing. To limit how often anyone lists, Rulemart also notes when each account lists or retries, and
  deletes each note once it's a day old.

## Cookies {#cookies}

Rulemart sets only cookies that signing in needs, never for advertising or tracking:

- `__Host-rulemart-sign-in`, for 10 minutes: the state of a sign-in in progress.
- `__Host-rulemart-session`, for 30 days, or until you sign out: a random token that keeps you signed in. Rulemart
  stores only its SHA-256 hash.
- `__Host-rulemart-notice`, for 1 minute: which notice to show once, such as after you sign out.

## Logs {#logs}

Rulemart is served through Amazon CloudFront, whose access logs record each request: your IP address and its country,
your browser's user agent, the page you came from, the address you asked for, including a search's words, and when.
Rulemart uses them to understand its traffic and to stop abuse, and deletes them after 180 days.

The server that builds pages logs each request it serves by the kind of page, such as a library's, never your IP
address, the page's address, or your cookies. When you sign in or delete your account, it logs your account's number in
Rulemart, not your GitHub details. It deletes these logs after 30 days.

## Analytics {#analytics}

{{if .Analytics}}

Rulemart counts page views on every page, signed in or not, with
[Cloudflare Web Analytics](https://www.cloudflare.com/web-analytics/), which sets no cookie and doesn't record a page
address's query, such as a search's words. Your browser sends it each page's address, the page you came from, your
browser's user agent, and how quickly the page loaded. No address names you or holds what's in your cart, so Cloudflare
can't tell who you are or what you chose.

{{else}}

Rulemart uses no analytics service.

{{end}}

## Who else handles it {#others}

- **Amazon Web Services** hosts Rulemart, in the United States, and keeps its logs.
- **Neon** hosts its database, in the United States.
- **GitHub** signs you in, answers what Rulemart reads of your account, and serves avatars and the images in rules.
- **Cloudflare** {{if .Analytics}}counts page views, and redirects{{else}}redirects{{end}} rulemart.ai here.

Rulemart sells nothing about you, and shows no ads.

## Deleting your data {#delete}

Delete your account at the bottom of your [dashboard]({{.DashboardHref}}), under Delete your account. Rulemart deletes
your account, sessions and their tokens, stars, listings, what it read of your GitHub account, and its record of the
GitHub App's installations at once, and unlinks its notes of when you listed. Uninstall the app in your GitHub settings
to remove its access there too. The database's history, which lets Fabrica restore it after a failure, keeps them for
up to 6 hours more. Logs age out as above.

For any question about your data, write to [legal@fabricahq.com](mailto:legal@fabricahq.com).
