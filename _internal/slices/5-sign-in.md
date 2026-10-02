# Slice 5: sign-in

## Goal

A visitor signs in to Rulemart with GitHub, sees who they're signed in as on every page, and signs out. Their account
page shows exactly what Rulemart keeps about them, and lets them sign out everywhere or delete the account. Nothing
else changes for visitors who don't sign in: browsing still needs no account, and the pages they see stay cached.

Later slices build on this: 6 lets anyone signed in list a library, and adds the unvetted area; 7 adds stars; 8 adds
the cart and its checkout prompt. "For later slices" below says how they plug in.

Decisions marked **Proposed** are new in this slice and wait for review. **Decided** ones are Josh's. **Existing**
ones are already in [decisions.md](../decisions.md) or an earlier slice.

## What a visitor can do

- **Sign in with GitHub.** Every page's header has a Sign in link, which leads to `/sign-in` and returns to the page.
  Continue with GitHub sends the visitor to GitHub to authorize Rulemart, and GitHub sends them back to
  `/account/github/callback`, which signs them in and returns them where they started. A visitor who cancels on
  GitHub, takes too long, or comes back in another browser gets the sign-in page again, saying what happened: a code
  GitHub refuses, such as one already used, says "That sign-in didn't complete", and GitHub failing to answer says to
  try again in a minute. A visitor already signed in who reaches a callback that can't complete, such as by going Back
  to it, simply goes on, still signed in.
- **See they're signed in.** The header shows their GitHub avatar, or their initial, which opens a menu with their
  login, Account, and Sign out.
- **Sign out.** Sign out ends the session in the database and clears the cookie, then returns to the page, or home
  from a page only a signed-in visitor can see, such as the account page, and says "You're signed out."
- **See what Rulemart keeps.** `/account` shows their GitHub user ID, login, avatar, and when the account was made,
  and says that's all.
- **Sign out everywhere, or delete the account.** Both end every session, return home, and say so; deleting also
  removes the account, behind a "What deleting does" disclosure, so it takes two deliberate clicks.

## Decisions

### Caching signed-in pages

**Proposed: a signed-in request bypasses every cache, and public pages stay identical for everyone.** CloudFront
caches whatever the function marks `public`, under a key without cookies, so a page that showed one visitor's avatar
and was marked public would be served to everyone. Three layers keep that from happening:

1. **The function.** Any response to a request that carries the session cookie, valid or not, and any response that
   sets a cookie, is `Cache-Control: private, no-store`, set in one middleware after the handler runs, so no page can
   forget it. Static files are the exception, only as the static route serves them: the same for everyone, and they set no
   cookie. A missing page under `/_static/` is a page like any other.
2. **CloudFront.** The cache policy adds the session cookie, and the one-time notice cookie, to the cache key. A signed-in request then never matches
   the shared signed-out copy, and CloudFront never collapses two visitors' requests into one origin request. Its
   responses are never stored anyway, so each signed-in visitor's key holds nothing. CloudFront also caches
   `Set-Cookie` with an object when cookies are in the key, which the first layer rules out.
3. **Browsers.** Every other response carries `Vary: Cookie`, which CloudFront passes through, so a browser that
   signs in doesn't reuse a page it kept from before.

The alternative was to keep every page identical and load the signed-in parts separately, with JavaScript, from an
uncached endpoint. It keeps signed-in visitors on the cache, but every later per-visitor element, a star's state or
the cart's count, would need a script and a second request, pages would flash signed-out first, and nothing would
work without JavaScript. Bypassing the cache costs a function invocation and a Neon read for every page a signed-in
visitor views, which is cheap at Rulemart's traffic. If signed-in traffic grows enough to matter, the separate
endpoint can come later without changing the session model.

### GitHub

- **Decided: GitHub is the only sign-in provider.** Every library is a GitHub repository.
- **Proposed: a GitHub OAuth app, asking for no scopes.** With no scopes, GitHub's consent screen says Rulemart reads
  only public information, and the token can read nothing private. Rulemart needs the user's ID, login, and avatar,
  nothing else. A GitHub App would ask for "act on your behalf" and issue expiring tokens to refresh, for no benefit
  while Rulemart never calls GitHub for a user. A later slice that needs private data, such as the prototype's
  project tracking, can add a GitHub App then; accounts keep working, since both report the same user ID.
- **Proposed: the token is discarded after reading the user, not stored or revoked.** It has no scopes, so it can read
  only what's public. Revoking it would add a request and a failure mode to every sign-in for no protection.
- **Proposed: state and PKCE together.** `POST /sign-in` keeps a random state and a PKCE verifier in a cookie and sends
  GitHub the state and the verifier's S256 challenge. The callback refuses a state that isn't the cookie's, compared
  in constant time, before asking GitHub anything, and exchanges the code with the verifier, so a leaked code is
  worthless without this browser's cookie.
- **Proposed: the flow lives in a `__Host-rulemart-sign-in` cookie for ten minutes**, holding the state, the verifier,
  and the return path. It needs no signing key: the `__Host-` prefix stops any other site, even under
  `fabricahq.com`, from setting it, and the return path it holds is checked again when it comes back. Starting a second
  sign-in in another tab replaces the first, whose callback then says to sign in again.
- **Proposed: only the sign-in page's content security policy lets a form lead to GitHub.** Browsers check a form's
  redirects against `form-action`, and the GitHub button posts to `/sign-in`, which redirects to
  `https://github.com/login/oauth/authorize`. The sign-in page alone, including the callback's errors, allows that one
  address; every other page keeps `form-action 'self'`.
- **Proposed: sign-in happens on the public origin.** Sign-in links are absolute on `RULEMART_BASE_URL`, and GitHub's
  callback is `RULEMART_BASE_URL/account/github/callback`, so a visitor on CloudFront's `cloudfront.net` domain moves to
  the public one, where the cookies and the OAuth app's callback are. Locally, without a base URL, the callback is on
  the request's own host over HTTP; GitHub accepts only the callback URLs its OAuth app registers.

### Sessions

- **Proposed: sessions in Postgres, by token hash.** The session cookie holds 32 random bytes, base64url-encoded; the
  `sessions` table stores only their SHA-256, so reading the table signs no one in. A malformed cookie is refused
  before any database read.
- **Proposed: a fixed 30-day lifetime, never extended.** Extending on use would write to Neon on page views. Signing
  in again takes one click once a visitor has authorized Rulemart, since GitHub skips its consent screen.
- **Proposed: at most 20 sessions per account.** Signing in on a 21st browser ends the oldest, so a script signing in
  over and over can't grow the table without bound. Every sign-in also deletes every expired session, so the table
  needs no scheduled cleanup.
- **Proposed: a new session at every sign-in, ending the one the browser held.** A token planted in a browser before
  sign-in signs no one in after it.
- **Proposed: the session cookie is `__Host-rulemart-session`: Secure, HttpOnly, SameSite=Lax, Path=/, no Domain**,
  and Max-Age the session's lifetime. Lax sends it on GitHub's top-level redirect back, and on links from other sites,
  so a visitor following a link arrives signed in, but not on another site's form posts or fetches. A cookie that no
  longer signs anyone in is cleared on the next page.
- **Proposed: signing out everywhere and deleting an account act only for a live session.** Each finds the account
  from the request's session token in the same statement that writes, so a request whose session another browser
  ended a moment earlier changes nothing, even after the visitor signs in again.
- **Proposed: a failure to read the session fails the page** with the usual 503, rather than showing a signed-in
  visitor a signed-out page that could mislead them, such as into signing in again.

### Writes and CSRF

- **Proposed: every state-changing request is a POST with an empty body, and the function refuses one another site
  started**, with Go's `http.CrossOriginProtection`: a browser's `Sec-Fetch-Site` must say `same-origin` or `none`,
  or, from a browser too old to send it, `Origin` must be this host or `RULEMART_BASE_URL`. A request with neither,
  such as from curl, carries no visitor's cookies against their will. `SameSite=Lax` is a second barrier.
- **Why no CSRF token:** CloudFront signs requests to the Function URL with origin access control, which signs the
  body only when the viewer sends its SHA-256 in `x-amz-content-sha256`. Browsers' form posts don't, so a POST with a
  body fails at Lambda, while an empty one passes (see the infra-catalog module's README). A token in a hidden field
  would need a body, and in the query string it would land in CloudFront's access logs. `Sec-Fetch-Site` needs neither.
  Forms carry what they need in their action's query string, which holds nothing secret, such as where to return.
- **Proposed: return paths are paths on this site only.** A return parameter must start with one `/`, have no
  backslash or control character, parse with no scheme or host, fit in 2,000 bytes, and not be a sign-in page;
  anything else returns to `/`. `//evil.example`, `/\evil.example`, and `https://evil.example` all go home. Nothing
  under `/account/` is a return target either: it holds the callback and actions that take POST. Signing out doesn't
  return to the account page, which a signed-out visitor can't see, but home. A URL's `#fragment` never reaches the
  server, so a visitor returns to the page without it; keeping it would need a script.
- **Proposed: after signing out, signing out everywhere, or deleting an account, the next page says so, once.** The
  action's redirect sets `__Host-rulemart-notice` for a minute, naming one of Rulemart's notices, never text to show.
  The page that renders it clears it, so that one response sets a cookie and is private, and the next is cached as
  usual. CloudFront keys its cache on this cookie too, so a cached page never hides the notice. A query parameter would
  have kept the response cacheable, but would stay in the address bar, history, and shared links. Deleting an account
  redirects home with its notice rather than answering the POST with a page, so reloading or going back doesn't post
  again.
- **Known: Back right after signing in does nothing visible.** The history holds the sign-in page, which sends a
  signed-in visitor on to where they were going, as GitHub's authorization page does once they've authorized Rulemart.
  Skipping it would need a script to replace the history entry.

### Accounts

- **Decided: accounts are keyed by GitHub's numeric user ID.** Logins change, and a freed login can belong to someone
  else. `accounts.github_user_id` is unique; `github_login` isn't. A login may have an underscore, as an Enterprise
  Managed User's does, such as `octocat_acme`.
- **Proposed: an account keeps only the GitHub user ID, the login, and the avatar's address**, the last two refreshed
  at each sign-in, plus when it was made and last signed in. No name, email, or token. An avatar that isn't on
  `avatars.githubusercontent.com`, the only image host the pages' content security policy allows, isn't kept.
- **Proposed: a visitor can delete their account.** It deletes the row and, by cascade, its sessions. Later slices
  decide what deleting does to what they add: stars should go with it, and a listed library should stay listed.
- **Proposed: a new bounded context, `internal/contexts/accounts`**, laid out as the catalog is: `domain` for
  identities, accounts, and session tokens; `store` and `store/postgres`, with sqlc's output in
  `generated/accountsdb`; `app` for signing in and out; and `github` for the OAuth app.

### Database roles

- **Proposed: a new group role, `rulemart_accounts_writer`**, NOLOGIN, which infrastructure creates with SQL like the
  catalog's, and which `rulemart_web` joins beside `rulemart_catalog_reader`. Migration 00009 grants it `USAGE` on
  `public`, `SELECT`, `INSERT`, `UPDATE`, and `DELETE` on `accounts`, and `SELECT`, `INSERT`, and `DELETE` on
  `sessions`: a session is never changed, so no one can extend one. It refuses a missing or privileged role, as 00003
  and 00005 do. Neither catalog role gets anything on these tables, so the worker can't read who has signed in.
- **00009 only adds tables**, which the release still running doesn't read.

### Routes

- **Proposed: `/sign-in` and `/sign-out` at one segment, and everything else under `/account`.** A one-segment path
  can't hide a library's `/{owner}/{repo}`, and `account` is a GitHub route, so no GitHub user can take it. Two
  candidates are real GitHub users and were avoided: `sign-in` (so not `/sign-in/github`) and `dev`.

### Local development

- **Decided: a dev sign-in that production builds can't include.** Built with the `rulemartdev` tag, as `make web-dev`
  does, the sign-in page offers two test users, `test_user` and `test_user_2`, which `POST
  /account/dev-sign-in` signs in through the same code as GitHub's callback. Their IDs, 9,000,000,001 and
  9,000,000,002, are far past GitHub's, no personal GitHub account can have an underscore in its login, and they have
  no avatar. Their account page calls them local test users and links no GitHub profile. **Proposed** guards:
  - The code lives in `internal/platform/web/dev_sign_in.go`, which only the tag compiles;
    `dev_sign_in_off.go` stands in otherwise.
  - `cmd/web`'s tests build the web function with `lambda-build.toml`'s own tags and check that its binary lacks the
    dev sign-in's route, while a `rulemartdev` build has it, so the check would see a regression.
  - A `rulemartdev` build refuses to start on Lambda.
  - `make check` vets and tests both builds.
- **Proposed: without `GITHUB_CLIENT_ID`, pages offer no sign-in**, outside a dev build: no header link, and
  `/sign-in` answers 404, saying sign-in isn't available yet. A release with this slice can deploy before the OAuth app
  exists, and sign-in appears once infrastructure sets the variables. The header's Sign in shows GitHub's mark only
  when it leads to GitHub.

### Header

- **Proposed: the account slot is as wide as its widest content at each width**, the Sign in button, a phone's Sign in
  link, or a narrow phone's icon, and stays, empty, on the sign-in page, so signing in or out never moves Libraries,
  Groups, or search. Its content sits at the page's edge: the avatar's button shades past it, into the margin, and its
  focus ring is drawn just inside the avatar.
- **Proposed: below 384 pixels, the header drops Fabrica's mark and shows Sign in as a labeled person icon**, so it
  fits down to 320 pixels; between 384 and 720 it keeps the mark, without Fabrica's name, and a Sign in link.

### Logging and abuse

- **Proposed: the web function logs `signed in` and `deleted account` with the account's internal ID**, and
  `sign-in refused`, `sign-in failed`, and `cross-origin request refused` with a reason, never a code, state, token,
  verifier, login, or cookie. The access log still records only the route, so `/account/github/callback?code=...`
  logs as `/account/github/callback`. An internal account ID is pseudonymous; the privacy notice should mention it.
- **Proposed: no rate limiting in the function for now.** Each sign-in needs a real GitHub authorization, and
  GitHub limits code exchanges; each account keeps at most 20 sessions; and the account's 10 Lambda slots bound the
  load anyone can put on Neon. A CloudFront WAF rate rule on `POST` and `/account/github/callback` is the next step if
  abuse appears, and it would also cover the listing form of slice 6.

## For later slices

- **Who's asking:** every page handler can read `visitorOf(r.Context())`; `s.signedIn(w, r)` returns the account or
  sends the visitor to sign in and come back. Signed-in pages can show anything per visitor, since they're never
  cached.
- **Writes:** a POST with an empty body and its input in the action's query string, such as
  `POST /stars?rule=owner/repo:rule-id`, needs nothing more: `withSameOriginWrites` already protects it. Listing a
  library can take its URL in the query string the same way, from a GET form that shows a confirmation first, which
  also suits slice 6's "this library isn't vetted" warning. A form that must send a body needs a script that computes
  `x-amz-content-sha256`.
- **Data:** reference `accounts(id)`, not the GitHub ID, and choose `ON DELETE CASCADE` or `SET NULL` deliberately.
  Grant new tables to `rulemart_accounts_writer`, or to a new group role if the web function shouldn't write them.
- **The cart** can stay in the browser for visitors who aren't signed in, since checkout needs no account.

## Infrastructure

Nothing is applied. Two pull requests carry the changes:

- **fabricahq/infra-catalog:** `cloudfront-function-url` gains `cache_key_cookies`, the cookies whose values join the
  cache key. The module had no way to add one.
- **fabricahq/infra-live:** the `accounts_writer_role` unit, `rulemart_web`'s membership in it, the
  `/rulemart/prod/github-client-secret` parameter, the web function's `GITHUB_CLIENT_ID` and
  `GITHUB_CLIENT_SECRET_PARAMETER` with permission to read it, and CloudFront accepting every method, with the session
  and notice cookies in its cache key.

### What Josh creates

A GitHub OAuth app, in the fabricahq organization's settings, under Developer settings, OAuth Apps, New OAuth App:

| Field | Value |
| --- | --- |
| Application name | Rulemart |
| Homepage URL | `https://rulemart.fabricahq.com` |
| Application description | Sign in to Rulemart, the Code Rules library catalog. |
| Authorization callback URL | `https://rulemart.fabricahq.com/account/github/callback` |
| Enable Device Flow | Off |

An OAuth app has no permissions to set: it asks for scopes at authorization, and Rulemart asks for none, so GitHub
shows visitors that it reads only public information. Then generate a client secret, put the client ID in the stack's
`github_oauth.client_id`, and the secret in the `/rulemart/prod/github-client-secret` parameter, as the infra-live
pull request says. The client ID isn't secret.

For signing in with GitHub locally, optionally, a second OAuth app, "Rulemart (local)", with the homepage
`http://127.0.0.1:8080` and the callback `http://127.0.0.1/account/github/callback`: GitHub accepts any port on a
loopback callback. Run `make web` with its `GITHUB_CLIENT_ID` and `GITHUB_CLIENT_SECRET`.

### Order

1. Merge and release the infra-catalog pull request, as v0.8.0.
2. Apply `accounts_writer_role`, then `web_database_role`, which adds the membership. Migration 00009 refuses to run
   until the role exists, so this comes before Rulemart's release with this slice.
3. Create the OAuth app, apply `github_client_secret`, and set the parameter's value.
4. Release Rulemart. Its pre-publish workflow runs 00009.
5. Pin the release, and apply `cdn` and `web_lambda`. Either order works: the release before this slice answers a
   POST with its 404 page, and ignores the new variables.

## Verification

- **Domain tests:** identities GitHub would and wouldn't report, avatars off GitHub's host, and session tokens'
  shape and strict parsing.
- **Store tests**, as `rulemart_web`: sign-in creates and renames an account by GitHub ID, replaces the browser's
  session, keeps 20 per account, deletes expired sessions, and sign-out, sign-out everywhere, and deletion end
  exactly the right sessions.
- **GitHub tests**, against a fake GitHub: the exchange's parameters, PKCE's RFC 7636 example, no scopes, every
  failure without leaking the code, verifier, token, or response text, and forgetting a refused secret.
- **Page tests:** the whole flow; every callback failure; return paths; the flow cookie edited; cookie attributes;
  cache headers signed in, signed out, with a stale or broken cookie, and on static files; cross-origin writes
  refused by `Sec-Fetch-Site` and `Origin`; the header, sign-in, and account pages; a failed session read; and the
  sign-in flow end to end against Postgres.
- **Build tests:** the release build has no dev sign-in, and a dev build won't start on Lambda; the Function URL
  passes cookies both ways.
- **Migration tests:** 00009's grants, refusing a missing or privileged `rulemart_accounts_writer`, and what
  `rulemart_web` may and may not do.
- **In a browser, locally:** both libraries ingested, `make web-dev`, then signing in as a test user, the menu,
  the account page, signing out, and the GitHub button and an expired callback with a placeholder OAuth app, at
  1280 and 390 pixels, light and dark, with no console errors.
- **After deployment:** `curl -sI https://rulemart.fabricahq.com/` shows `cache-control: public, max-age=0, s-maxage=60` and
  `vary: Cookie`; after signing in, the same page shows `private, no-store` and `x-cache: Miss from cloudfront` on
  every reload; `curl -si -X POST https://rulemart.fabricahq.com/sign-out` answers 303, proving an empty POST reaches
  the function; and the web function's logs show `signed in` without a code or token.

## Not in this slice

Listing a library, the unvetted area, stars, the cart, and checkout. A GitHub App or any GitHub scope. Showing a
visitor their other sessions, or signing one out from another. Rate limiting with a WAF. Email or any other provider.
A privacy notice, which should now mention accounts and their retention.
