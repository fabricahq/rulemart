# Slice 9: launch readiness

## Goal

Close the gaps between slices 1 to 8 and a public launch: what search engines and social sites read, the headers
that keep pages safe, optional privacy-friendly analytics, a privacy notice and an about page, a way to report a
library, alarms for the failures the web function catches, and a pass over every page for consistency.
The launch runbook put it all live; it now lives in infra-live, as
[decisions](../decisions.md#infrastructure-and-delivery) records.

Decisions marked **Decided** are Josh's: those dated 2026-10-05 were proposed in this slice and stand as built.
**Superseded** ones were proposed here and replaced by the slice named. **Existing** ones are already in
[decisions.md](../decisions.md) or an earlier slice.

## What a visitor sees

- **Search engines** find every vetted library, rule, and canonical group in `/sitemap.xml`, and `/robots.txt` keeps
  them out of account pages, sign-in, listing, search, the unvetted area, and comparisons.
- **A link shared** on a social site or in chat shows the page's title and description, and Rulemart's image.
- **Every page's footer** leads to About Rulemart, Privacy, About Code Rules, the source on GitHub, and Report a
  problem. The sign-in and account pages link Privacy too.
- **`/about`** says what Rulemart is, what vetting means and doesn't, what an unvetted library is, and how to get a
  library vetted.
- **`/privacy`** says what Rulemart keeps, for how long, who else handles it, and how to delete it.
- **Each library's About panel** has Report this library, which opens a GitHub issue with the library filled in.

## Decisions

### Search engines

- **Decided (Josh), 2026-10-05: `robots.txt` disallows `/account/`, `/sign-in`, `/list`, `/search`, `/unvetted`, and `/*from=`**, the
  query parameter only comparisons use, and names the sitemap on `RULEMART_BASE_URL`. Each one-segment page is
  disallowed alone and with a query, as `/list$` and `/list?`, and so is `/account`, since a rule matches every path it starts: `/list`
  alone would also keep crawlers off `/listr/rules`, a library's page. Each of those pages already
  says `noindex` (**Existing**), for crawlers that ignore `robots.txt`; disallowing them saves crawling search's and
  comparisons' endless addresses. An unvetted library's own pages can't be disallowed by path, since they share
  `/{owner}/{repo}` with vetted ones; they keep `noindex, nofollow` (**Existing**).
- **Superseded by [slice R4](13-discovery.md): one sitemap file, listing the home, libraries, groups, about, and privacy pages, each canonical group's
  page, and each vetted library and its current rules**, by their canonical addresses, with `lastmod` from the release
  that last changed each library or rule. Never an unvetted library, a search, a comparison, or a group that isn't
  canonical, which has no page of its own. One snapshot read, at most 45,000 rules, which leaves room in the
  protocol's 50,000 addresses, and at most 5 MiB, under the 6 MB a Lambda function's response holds, which rules with
  long IDs can reach first. Past either it lists what fits and logs `sitemap truncated`, which alarms,
  and the next step is a sitemap index. Retired rules' pages stay out: they're for people following an old link.
- **Superseded by [slice R8](17-launch-gate.md): without `RULEMART_BASE_URL` there's no sitemap**, since its addresses must be absolute, and pages name no
  canonical address either (**Existing**).
- **Decided (Josh), 2026-10-05: Open Graph tags, which X reads too, on exactly the pages that name a canonical address**: title,
  description, address, and one 1200x630 image of Rulemart's, `static/social.png`. An unvetted library's page,
  a search, a comparison, and a missing page show no card, so sharing one borrows nothing of Rulemart's.
- **Decided (Josh), 2026-10-05: a page's description falls back to what it holds** when a library has no description or a rule no
  reading guidance, and is cut at a word to 200 characters.
- **Decided (Josh), 2026-10-05: no structured data.** Google retired the sitelinks search box, and nothing else cheap applies.
- **Decided (Josh), 2026-10-05: `/favicon.ico` and an `apple-touch-icon`**, beside the SVG icon (**Existing**), so browsers' default
  request gets an icon instead of a missing page.

### Headers

- **Decided (Josh), 2026-10-05: the function sends `Strict-Transport-Security: max-age=31536000; includeSubDomains`** on every response.
  Neither CloudFront nor the function sent one before. CloudFront redirects HTTP to HTTPS (**Existing**); this spares
  the redirect and its unencrypted request. No `preload`, which would commit the domain for good while it may still
  change. The function, not a CloudFront response headers policy, sends it, as it sends the other headers, so a test
  covers them all.
- **Decided (Josh), 2026-10-05: `Permissions-Policy` denies the browser features Rulemart never uses**, and opts out of the Topics API,
  and **`Cross-Origin-Opener-Policy: same-origin`**, since no page opens another window. `X-Content-Type-Options`,
  `Referrer-Policy`, and the content security policy's `frame-ancestors 'none'` are **Existing**.
- **Existing, reviewed: the content security policy** allows only Rulemart's own scripts, styles, and fonts, images
  from GitHub's avatar and raw hosts, and forms to Rulemart, and the sign-in page's alone adds GitHub's authorization
  page to `form-action`. Every page in the QA pass loaded with no violation.

### Analytics

- **Decided: Cloudflare Web Analytics**, over Plausible, for counting page views without cookies.
- **Decided (Josh), 2026-10-05: it's off unless `CLOUDFLARE_WEB_ANALYTICS_TOKEN` is set**, which infrastructure sets from the stack's
  `web_analytics.site_token`. Off, pages load no other site's script, and the content security policy allows none and
  no connections. On, every page loads Cloudflare's beacon, deferred, before the body's end, and the policy adds
  `https://static.cloudflareinsights.com` to `script-src` and `connect-src https://cloudflareinsights.com`, the two
  Cloudflare's manual setup needs. The token isn't secret: every page sends it. A token other than letters, digits,
  hyphens, and underscores stops the web function at start, rather than being written into every page.
- **Decided (Josh), 2026-10-05: the beacon loads on every page, signed-in ones too.** It sets no cookie, and Cloudflare says it records
  no query string. The privacy page says whether analytics are on, from the same setting.

### Privacy and about

- **Decided (Josh), 2026-10-05: `/privacy` says only what the code and infrastructure do**: the account's GitHub user ID, login, and
  avatar address and its two dates; the three cookies and their lifetimes; stars, cart, and listings, and the one-day
  notes of listing requests that rate-limit listing and outlive a deleted account, unlinked; CloudFront's logs, with
  IP addresses, kept 180 days; the function's logs, without them, kept 30 days, which name an account's internal ID
  when it signs in or is deleted; Neon's 6-hour history; Cloudflare Web Analytics only when on; and who else handles
  data: AWS, Neon, GitHub, and Cloudflare. It names `legal@fabricahq.com` for questions, as Josh decided on
  2026-10-05. It makes no legal claim, such as a lawful basis or a jurisdiction.
- **Decided (Josh), 2026-10-05: `/about` says vetting means Fabrica chose to show a library, not that it checked every rule**, and that
  vetting covers future releases (**Existing**). Getting vetted is: release with Code Rules, list it, then ask on
  GitHub.
- **Decided (Josh), 2026-10-05: both are one-segment paths**, which can't hide a library's `/{owner}/{repo}` (**Existing** pattern), and
  are in the sitemap.

### Reports

- **Decided (Josh), 2026-10-05: reports are GitHub issues in Rulemart's public repository, through issue forms**: Report a library,
  Ask to vet a library, and Report a problem, in `.github/ISSUE_TEMPLATE`. Each library's About panel links Report
  this library with the library and title filled in by query parameters; the footer links the form chooser. Rulemart
  stores nothing new, and a report's discussion is public, as vetting's is (**Existing**). Forms ask reporters to leave
  out anything private, and to send security vulnerabilities to `hello@fabricahq.com`. GitHub reads issue forms only
  from the default branch, so the links work once this merges.
- **Existing: an operator removes an abusive listing with SQL**, and vetting stays a change to `vetted.yaml`.

### Infrastructure

- **Decided (Josh), 2026-10-05: static files get a CloudFront cache behavior with no cookie or query string in its key**, through
  infra-catalog's new `static_paths`, for `/_static/*` and `/favicon.ico`. With the session cookie in the default key
  (**Existing**), a signed-in visitor cached the stylesheet once per session. Static files are already cached for a
  year under hashed names (**Existing**), and the static route reads no cookie.
- **Decided (Josh), 2026-10-05: an AWS WAF rate rule on POSTs is available but off.** infra-catalog's `write_rate_limit` puts a web ACL
  on the distribution that answers an address's POSTs past a limit with 429, and the stack's `write_rate_limit` stays
  `null`. It costs about $6 a month, $5 for the web ACL and $1 for the rule, plus $0.60 per million requests, and the
  function already bounds every kind of write (**Existing**). Turn it on with `{ requests = 100, window_seconds = 300 }`
  if abuse appears.
- **Decided (Josh), 2026-10-05: the web function alarms on three lines it logs while answering anyway**: `sign-in failed`, which a
  wrong OAuth secret or GitHub being down causes, `listing not queued`, and `sitemap truncated`, through
  infra-catalog's `http-lambda` passing `log_error_patterns` on. Lambda errors and throttles, the URL's 5xx, and the
  jobs queue's dead letters and age already alarm (**Existing**), which covers every route's failures, since a failed
  page is a 503.
- **Decided (Josh), 2026-10-05: no synthetic check yet.** A Route 53 health check costs about $2 to $3 a month with HTTPS and string
  matching, and CloudWatch Synthetics about $1 a month hourly, but either alarms only in us-east-1, which needs a
  second SNS topic and email subscription there. The 5xx alarm already sees any failure a visitor meets; a check would
  add only failures before the function, such as DNS or the certificate, which the checks after each deploy cover,
  in infra-live's operations document.

### Logging

- **Existing, checked: every route added since slice 5 logs only its route pattern**, and failures replace a library's
  or item's names with the route's placeholders. The new routes log nothing of their own but `sitemap truncated`. No
  line holds a token, a code, a cookie, or a query string.

### QA

A Playwright pass over every page type, signed in and out, at 1280, 390, and 320 pixels, light and dark, found no
sideways scrolling and no console error but a missing page's own 404. It found:

- The listings and remove-listing pages named a repository in bold code type, where the cart, checkout, and stars name
  a library in the text type. They now match.
- The footer, with five links, kept the theme menu on a line of its own on phones; it now sits beside Rulemart's name.
- A sign-in page without GitHub, such as a local build's, linked no privacy notice; every sign-in page now does.

## Data and roles

No migration and no new role. The sitemap reads tables `rulemart_catalog_reader` already reads (**Existing**).

## Infrastructure

- **fabricahq/infra-catalog:** `cloudfront-function-url` gains `static_paths` and `write_rate_limit`, and
  `http-lambda` passes `log_error_patterns` through. Released with #26 as v0.8.0, infra-live's pin doesn't move.
- **fabricahq/infra-live:** the stack's `web_analytics.site_token`, empty, and `write_rate_limit`, null; `web_lambda`'s
  `CLOUDFLARE_WEB_ANALYTICS_TOKEN` once the token is set, and its three log alarms; `cdn`'s static paths.

The launch runbook ordered every merge, release, and apply; infra-live's operations document records it, as
[decisions](../decisions.md#infrastructure-and-delivery) says.

## Verification

- **Page tests:** every response's security headers; no analytics without a token, and the beacon, its data, and
  the policy's two sources with one, on the sign-in page too; a token that can't be one refused at start; robots.txt's
  rules and sitemap line; the sitemap's addresses, `lastmod`, order, cache, failure, truncation log, and absence
  without a base URL; social cards only on pages with an address of their own, and their image served; description
  fallbacks and cuts; icons; the footer's links on every kind of page; the about and privacy pages, with and without
  analytics; the report link on vetted and unvetted libraries; privacy from the sign-in page.
- **Store test**, as `rulemart_web`: the sitemap reads only vetted libraries' current rules and groups, and at most
  its limit. **App test:** only canonical groups.
- **Start-up test:** a bad analytics token stops `cmd/web`.
- **infra-catalog tests:** static paths share one cookie-less copy while pages stay keyed on the session; no web ACL by
  default; the rate rule's limit, window, scope, and 429; refusals of bad inputs; `log_error_patterns` reaching the
  function.
- **infra-live:** the `cdn` and `web_lambda` units validate against the new modules, with the rate limit and analytics
  off and on.
- **In a browser, locally:** both libraries ingested, one listed as unvetted and one failed listing, `make web-dev`,
  every page type at 1280, 390, and 320 pixels, light and dark, signed in and out; with a token, the beacon loads under
  the policy, with no violation.

## Not in this slice

A sitemap index, for more than 45,000 rules. A synthetic check. Turning the WAF rule on. Showing listers who reported
their library. Dropping `hello_messages`, in a later release's own change.
