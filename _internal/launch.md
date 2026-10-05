# Launch runbook

How to put Rulemart's `main` live, from the open infrastructure pull requests to the announcement, in order. `main`
holds slices 3 to 9 and the [realignment](realignment.md)'s slices R1 to R8. Production runs v0.1.0, slices 1, 1b,
and 2, at <https://rulemart.fabricahq.com>, and `rulemart.ai` redirects there.

Every step names who does it. **Status, 2026-10-03:** every Rulemart change is merged to `main`, and both GitHub apps
exist. Nothing in infra-live or infra-catalog is merged or applied. The infra-live stack is
`aws/rulemart/us-west-2/rulemart-prod`; run its commands from that folder with the `rulemart` AWS profile and the Neon
credentials its README's [Configure Neon](https://github.com/fabricahq/infra-live/blob/main/aws/rulemart/us-west-2/rulemart-prod/README.md#configure-neon)
describes. The `domain_redirect` unit doesn't change, so no step needs `CLOUDFLARE_API_TOKEN`; apply units one at a
time rather than with `run --all`, which plans that unit too.

## Two facts to know first

1. **The GitHub App's webhook isn't wired at launch.** The site's Function URL requires requests signed by
   CloudFront's origin access control, which signs a body only when the sender sends `x-amz-content-sha256`. GitHub
   doesn't, so every delivery to `/account/github/webhook` gets 403 from Lambda until infra-catalog gains a public
   alias on the web function and an unsigned CloudFront path to it, as step 9 describes. Rulemart works without it,
   since it reads a visitor's GitHub account at three moments: on the first page that shows it after they sign in,
   which discards the account's snapshot; when they press Refresh, at most once a minute; and when GitHub returns them
   to the setup URL, `/me/github/installed`, after they install the app or change which repositories it reads, which
   reads at once. Opening the dashboard otherwise shows the snapshot kept, without reading. Each read asks GitHub about
   every installation the account reads through: it forgets one GitHub no longer knows, as after an uninstall, for
   every account, and one on an organization the visitor no longer owns, for theirs, discarding the snapshot and
   reading again without it, while a suspended one makes the read fail. A failed read keeps the snapshot the account
   had, marked failed and dated, and the next read waits a minute; a suspended installation keeps failing reads until
   it's unsuspended or the visitor chooses Remove access to private repos at `/me/private`, which forgets every
   installation and discards the snapshot. So until then, a change to an installation on GitHub reaches Rulemart only
   at one of those reads; once the webhook is wired, each delivery about an installation discards the snapshots of the
   accounts that read through it, so their next page reads again, and the privacy page can say so. GitHub's
   **Advanced** tab lists the failed deliveries, which can be redelivered for three days.
2. **Rotating the token key needs a redeploy.** Each session keeps the visitor's GitHub token sealed with the key in
   `/rulemart/prod/token-key`. Warm web instances keep the key they read, and seal new sessions' tokens with it,
   until they start again: an instance that can't open a token reads the parameter once more, but one that only seals
   never does. So after replacing the parameter, make every instance start fresh, with any update to the web
   function's configuration, such as `aws lambda update-function-configuration` changing its description. Sessions
   sealed under the old key ask the visitor to sign in again before the dashboard reads GitHub. Nothing else breaks.

## The pull requests

| Repository | PR | Branch | Base | What |
| --- | --- | --- | --- | --- |
| infra-catalog | [#25](https://github.com/fabricahq/infra-catalog/pull/25) | `claude/environment-reviewers` | `main` | `github-repository-config` environment reviewers, for Release Planner. Not on Rulemart's path. |
| infra-catalog | [#26](https://github.com/fabricahq/infra-catalog/pull/26) | `claude/cloudfront-cache-key-cookies` | `main` | `cloudfront-function-url`: `cache_key_cookies`. |
| infra-catalog | [#27](https://github.com/fabricahq/infra-catalog/pull/27) | `claude/rulemart-launch-cloudfront` | #26 | `cloudfront-function-url`: `static_paths`, `write_rate_limit`; `http-lambda`: `log_error_patterns`. |
| infra-live | [#23](https://github.com/fabricahq/infra-live/pull/23) | `claude/rulemart-slice-5-sign-in` | `main` | Sign-in's role, OAuth secret, CloudFront methods and cookies; pins infra-catalog v0.8.0. |
| infra-live | [#24](https://github.com/fabricahq/infra-live/pull/24) | `claude/rulemart-slice-6-listing` | #23 | The web function queues listings; the worker's GitHub token parameter. |
| infra-live | [#25](https://github.com/fabricahq/infra-live/pull/25) | `claude/rulemart-slice-9-launch` | #24 | Analytics token, static paths, optional WAF rule, log alarms. |
| infra-live | [#27](https://github.com/fabricahq/infra-live/pull/27) | `claude/rulemart-slice-r7-dashboard` | #25 | Slice R7: the token key, the GitHub App's two parameters and five variables, and the worker's token made required. |

A stacked pull request is merged after the one under it, by retargeting it to `main`. Use merge commits, not squash,
so each branch above still shares its history with `main` and merges without conflicts. `delete_branch_on_merge` is
off in both repositories, so GitHub doesn't retarget for you.

For each step that merges, wait for the pull request's checks, failing closed:

```sh
for _ in $(seq 1 90); do
  s=$(gh pr checks "$PR" -R "$REPO" --json name,bucket 2>/dev/null); true
  jq -e 'length > 0 and all(.bucket != "pending")' <<<"${s:-[]}" >/dev/null && break
  sleep 20
done
jq -e 'length > 0 and all(.bucket == "pass" or .bucket == "skipping")' <<<"${s:-[]}" >/dev/null \
  || { echo "checks not green: ${s:-<empty>}"; exit 1; }
```

## 1. Decide and prepare (Josh)

Before anything merges:

1. **Confirm the privacy notice** at `/privacy` (`internal/platform/web/about.templ`), which states only what the code
   and infrastructure do: the sealed GitHub token, what Rulemart reads of a GitHub account and its organizations, the
   cart the browser keeps, rule stars, and page views counted on every page once the analytics token is set. Decide:
   - the contact for privacy questions is `legal@fabricahq.com` (decided 2026-10-05), which the privacy page names;
   - whether it needs anything legal the code can't tell, such as a lawful basis, a controller's name and address,
     or a jurisdiction. It deliberately claims none;
   - whether "Fabrica" should name the legal entity that runs Rulemart.
2. **Confirm the vetting page's wording** at `/about/vetting`: lists show vetted libraries until the visitor
   includes unvetted ones, and vetting "means Fabrica chose to show the library, not that it checked every rule".
   Say what Fabrica checks if it should say more.
3. **The proposed decisions were decided** on 2026-10-05 and recorded in the slice documents.
4. **Collect the OAuth app's values.** The OAuth app "Rulemart" exists in the fabricahq organization. Confirm its
   authorization callback URL is `https://rulemart.fabricahq.com/account/github/callback` and Device Flow is off.
   Rulemart asks for `read:org` when a visitor signs in, so the app needs no change for it. Generate a client secret
   and keep it for step 3.4, and note the client ID, which isn't secret.
5. **Confirm the GitHub App "Rulemart by Fabrica"**, in the fabricahq organization's **Developer settings**,
   **GitHub Apps**:

   | Setting | Value |
   | --- | --- |
   | Setup URL | `https://rulemart.fabricahq.com/me/github/installed`, with **Redirect on update** on, and no user authorization during installation |
   | Webhook | Active, URL `https://rulemart.fabricahq.com/account/github/webhook`, with a new secret (`openssl rand -hex 32`), kept for step 3.4 |
   | Events | None under "Subscribe to events": GitHub sends every app the **Installation** and **Installation repositories** events |
   | Repository permissions | **Contents** read-only and **Metadata** read-only, nothing else |
   | Where it can be installed | Any account |

   Generate a private key, which downloads a `.pem` file, and note the App ID, the client ID, and the slug,
   `rulemart-by-fabrica`. None of those three is secret.
6. **Create the worker's GitHub token**, which infra-live#27 makes required: GitHub **Settings**, **Developer
   settings**, **Personal access tokens**, **Fine-grained tokens**: resource owner fabricahq or yourself, **Public
   repositories** (read-only), no permissions, the longest expiry GitHub allows, and a reminder to rotate it. While its
   parameter holds the placeholder, every lookup of a new listing fails.
7. **Optionally, add Cloudflare Web Analytics**: in the Cloudflare dashboard for the fabricahq account, open **Web
   Analytics** (under **Analytics & Logs**), choose **Add a site**, enter `rulemart.fabricahq.com`, and choose
   **Done**, without automatic setup, since Cloudflare doesn't proxy this hostname. Under **Manage site**, copy the
   `"token"` value from the snippet's `data-cf-beacon`. It isn't secret, since every page shows it, so it's a stack
   value in step 5, not a parameter. Without it, Rulemart loads no analytics, and `/privacy` says so.
8. **Decide on the WAF rate rule**, which stays off (`write_rate_limit = null`, about $6 a month if on).

## 2. Release infra-catalog v0.8.0 (Josh)

1. Merge [infra-catalog#25](https://github.com/fabricahq/infra-catalog/pull/25) whenever it's ready; it changes no
   Rulemart unit, and merged first it ships in v0.8.0 too.
2. Merge [infra-catalog#26](https://github.com/fabricahq/infra-catalog/pull/26).
3. Retarget and merge [infra-catalog#27](https://github.com/fabricahq/infra-catalog/pull/27):
   `gh pr edit 27 -R fabricahq/infra-catalog --base main`, wait for its checks, then merge.
4. Ask an agent to make an infra-catalog release with Release Planner, as **v0.8.0**: three new optional inputs and
   one module input, none breaking. Merge its release pull request.

infra-live#23 already pins `v0.8.0`. If #26 went out alone as v0.8.0, release #27 as v0.9.0 and change
`catalog_ref` to it in infra-live#25 before merging it.

## 3. Infrastructure before the Rulemart release (Josh)

Production still runs v0.1.0 throughout this step, which ignores every new variable, answers a POST with its 404
page, and keeps working on the new cache settings.

1. **Merge infra-live**, in order: [#23](https://github.com/fabricahq/infra-live/pull/23); then
   `gh pr edit 24 -R fabricahq/infra-live --base main` and [#24](https://github.com/fabricahq/infra-live/pull/24);
   then `gh pr edit 25 -R fabricahq/infra-live --base main` and [#25](https://github.com/fabricahq/infra-live/pull/25);
   then `gh pr edit 27 -R fabricahq/infra-live --base main` and [#27](https://github.com/fabricahq/infra-live/pull/27).
   `rulemart_release` stays `v0.1.0`, and `github_oauth.client_id`, `github_app.app_id`, `github_app.client_id`, and
   `web_analytics.site_token` stay unset until step 5.
2. From the stack's folder on `main`:

   ```sh
   terragrunt stack clean && terragrunt stack generate
   ```

3. **Roles**, which migration 00009 refuses to run without; 00010 and 00016 grant to the same role:

   ```sh
   terragrunt --working-dir .terragrunt-stack/accounts_writer_role run apply   # creates rulemart_accounts_writer, NOLOGIN
   terragrunt --working-dir .terragrunt-stack/web_database_role run apply      # adds it to rulemart_web's memberships, in place
   ```

   Optionally plan `catalog_reader_role`, `catalog_writer_role`, and `worker_database_role`: no changes expected, only
   the catalog ref and the login unit's wiring moved.
4. **Parameters.** Each unit creates an SSM SecureString holding infra-catalog's placeholder, which the functions
   treat as unset:

   ```sh
   for unit in github_client_secret token_key github_app_key github_app_webhook_secret worker_github_token; do
     terragrunt --working-dir .terragrunt-stack/$unit run apply
   done
   ```

   | Parameter | Holds | The variable that names it | Read by |
   | --- | --- | --- | --- |
   | `/rulemart/prod/github-client-secret` | The OAuth app's client secret | `GITHUB_CLIENT_SECRET_PARAMETER` | web |
   | `/rulemart/prod/token-key` | 32 random bytes in base64, the key that seals each session's GitHub token | `TOKEN_KEY_PARAMETER` | web |
   | `/rulemart/prod/github-app-key` | The GitHub App's private key, in PEM | `GITHUB_APP_PRIVATE_KEY_PARAMETER` | web |
   | `/rulemart/prod/github-app-webhook-secret` | The GitHub App's webhook secret | `GITHUB_APP_WEBHOOK_SECRET_PARAMETER` | web |
   | `/rulemart/prod/worker-github-token` | The worker's fine-grained token | `GITHUB_TOKEN_PARAMETER` | worker |

   Set each value so none passes through OpenTofu or a shell's history:

   ```sh
   put() { aws ssm put-parameter --profile rulemart --region us-west-2 --overwrite --type SecureString --name "$1" --value "$2" >/dev/null; }
   put /rulemart/prod/github-client-secret "$(pbpaste)"          # the OAuth app's client secret, on the clipboard
   put /rulemart/prod/token-key "$(openssl rand -base64 32)"     # never shown
   put /rulemart/prod/github-app-key "file://$HOME/Downloads/<the downloaded>.private-key.pem"
   put /rulemart/prod/github-app-webhook-secret "$(pbpaste)"     # the secret entered in the app's settings
   put /rulemart/prod/worker-github-token "$(pbpaste)"           # the fine-grained token
   ```

   The functions' connection strings, which `DATABASE_URL_PARAMETER` names, are already set in production, such as
   the worker's `/rulemart/prod/worker-database-url`.
5. **CloudFront**: `terragrunt --working-dir .terragrunt-stack/cdn run plan`, then `apply`. Expect the default
   behavior's allowed methods to become all seven, its cache policy's cookies a whitelist of
   `__Host-rulemart-session` and `__Host-rulemart-notice`, and two new ordered behaviors, `/_static/*` and
   `/favicon.ico`, with a new `rulemart-web-static` cache policy and origin request policy. No web ACL. It takes
   several minutes to deploy. Then check v0.1.0 still serves:

   ```sh
   curl -sI https://rulemart.fabricahq.com/ | grep -iE '^(HTTP|cache-control|x-cache)'
   curl -si -X POST https://rulemart.fabricahq.com/signout | head -1   # v0.1.0 answers 404, from the function
   ```

## 4. Release Rulemart v0.2.0 (Josh, with an agent)

1. **Ask an agent to make a Rulemart release** with Release Planner. Expect **v0.2.0**: new features, no breaking
   change for infrastructure, since every new variable can stay unset and the new role exists. Its notes must say it
   adds migrations **00006 to 00016**, all expand-only but 00013 and 00014, which drop tables no published release
   created:

   | Migration | Slice | Changes | Read by v0.1.0? |
   | --- | --- | --- | --- |
   | 00006 | 3 | Adds the generated `rule_versions.search_document`, which rewrites `rule_versions` once | No |
   | 00007 | 3 | Adds nullable `rule_versions.when_to_read_html` and `rendered_when_to_read` | No |
   | 00008 | 4 | Relaxes `rule_versions`' content check; adds nullable `rules.retired_html` | No |
   | 00009 | 5 | Adds `accounts` and `sessions`; grants `rulemart_accounts_writer` | No |
   | 00010 | 6 | Adds `listings` and `listing_requests` | No |
   | 00011 | 7 | Adds `stars` | No |
   | 00012 | 8 | Adds `cart_items` | No |
   | 00013 | R3 | Drops 00011's `stars`, refusing if it holds rows; adds `rule_stars` | No |
   | 00014 | R5 | Drops 00012's `cart_items`, refusing if it holds rows: the cart moved to the browser | No |
   | 00015 | R6 | Adds rules' assets and tags, and when each library came to Rulemart | No |
   | 00016 | R7 | Adds each session's sealed GitHub token, the GitHub snapshots, and the GitHub App's installations | No |

   A fresh database migrates through all sixteen, as slice R8 checked.
2. **Merge the release pull request.** Release Planner's pre-publish workflow, "Migrate the database", applies
   00006 to 00016 to production, then tags and publishes `web.zip` and `worker.zip`. If it fails, nothing is
   published: fix the cause, such as a missing role, and re-run the failed job. v0.1.0 keeps serving on the migrated
   schema either way.

## 5. Deploy v0.2.0 (Josh)

1. Verify the release's ZIPs were attested from `main`:

   ```sh
   gh release download v0.2.0 --repo fabricahq/rulemart --pattern '*.zip' --pattern SHA256SUMS --dir /tmp/rulemart-v0.2.0
   for zip in /tmp/rulemart-v0.2.0/*.zip; do
     gh attestation verify "$zip" --repo fabricahq/rulemart \
       --signer-workflow fabricahq/rulemart/.github/workflows/release-planner.yml --source-ref refs/heads/main
   done
   cat /tmp/rulemart-v0.2.0/SHA256SUMS
   ```

2. In an infra-live pull request, in `terragrunt.stack.hcl`, once every parameter in step 3.4 holds its value:
   - `rulemart_release`: `tag = "v0.2.0"`, and `web_sha256` and `worker_sha256` from `SHA256SUMS`;
   - `github_oauth.client_id`: the OAuth app's client ID;
   - `github_app.app_id` and `github_app.client_id`: the GitHub App's; `slug` already is `rulemart-by-fabrica`;
   - optionally `web_analytics.site_token`: the Cloudflare token.

   Merge it, then from `main`:

   ```sh
   terragrunt stack clean && terragrunt stack generate
   terragrunt --working-dir .terragrunt-stack/web_lambda run plan
   terragrunt --working-dir .terragrunt-stack/web_lambda run apply
   terragrunt --working-dir .terragrunt-stack/worker_lambda run plan
   terragrunt --working-dir .terragrunt-stack/worker_lambda run apply
   ```

   Either order works: the schema is already migrated. What each function reads, as `cmd/web` and `cmd/worker`
   document:

   | Function | Variables |
   | --- | --- |
   | web | `DATABASE_URL_PARAMETER`, `RULEMART_BASE_URL`, `LOG_LEVEL`, `RULEMART_RELEASE`; `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET_PARAMETER`, `TOKEN_KEY_PARAMETER`; `GITHUB_APP_ID`, `GITHUB_APP_CLIENT_ID`, `GITHUB_APP_SLUG`, `GITHUB_APP_PRIVATE_KEY_PARAMETER`, `GITHUB_APP_WEBHOOK_SECRET_PARAMETER`; `QUEUE_URL`; `CLOUDFLARE_WEB_ANALYTICS_TOKEN` |
   | worker | `DATABASE_URL_PARAMETER`, `QUEUE_URL`, `GITHUB_TOKEN_PARAMETER`, `LOG_LEVEL`, `RULEMART_RELEASE` |

   The web function refuses to start with `GITHUB_CLIENT_ID` but no token key, with the GitHub App's variables but no
   `GITHUB_CLIENT_ID`, or with only some of the App's five, so `web_lambda`'s plan shows them together: the new code,
   the three sign-in variables, the five App variables, `QUEUE_URL`, the analytics token if set, `ssm:GetParameter`
   on each parameter, an `sqs:SendMessage` statement, and three metric filters and alarms:
   `rulemart-web-sign-in-failed`, `-listing-not-queued`, and `-sitemap-truncated`. `worker_lambda`'s shows the new code
   and `GITHUB_TOKEN_PARAMETER`.
3. The first hourly poll after the deploy re-ingests each library whose older versions lack their content, or whose
   assets and tags weren't stored, so comparisons and asset pages fill in within the hour; nothing to run by hand.

## 6. Check production (Josh)

Confirm the SNS subscription email first, if it's pending, so the new alarms reach you.

```sh
B=https://rulemart.fabricahq.com
# Pages: cached for a minute at CloudFront, the security headers, analytics only if its token is set.
curl -sI $B/ | grep -iE '^(HTTP|cache-control|vary|strict-transport-security|permissions-policy|cross-origin-opener-policy|content-security-policy|x-cache)'
curl -sI $B/ | grep -i x-cache                      # Hit from cloudfront, the second time
# Crawlers: robots keeps them off /me, /cart, and /signin, and names the sitemap, which lists only vetted libraries'
# pages, their groups' among them, and no asset page.
curl -s $B/robots.txt
curl -s $B/sitemap.xml | grep -c '<loc>'
curl -s $B/sitemap.xml | grep -c '/assets/'          # 0
curl -s $B/sitemap.xml | grep -c code-rules-test-library   # its pages, until step 7 unvets it
# Social card and canonical address on a library page.
curl -s $B/fabricahq/public-rules | grep -oE '<(link rel="canonical"|meta property="og:[a-z:]+")[^>]*>'
# Static files: one copy for everyone, signed in or not, kept a year.
css=$(curl -s $B/ | grep -oE '/_static/[0-9a-f]+/generated/app\.css')
curl -sI $B$css | grep -iE '^(cache-control|x-cache)'
curl -sI -H 'Cookie: __Host-rulemart-session=anything' $B$css | grep -i x-cache   # Hit, not Miss
curl -sI $B/favicon.ico | grep -iE '^(HTTP|content-type)'
# Writes: an empty POST passes CloudFront's signing and reaches the function.
curl -si -X POST $B/signout | head -1               # 303
# The pages, and the old addresses' permanent redirects.
for p in /browse/techs /browse/practices /g/techs/go /fabricahq /faq /feedback /about /about/vetting /privacy /cart /signin '/search?q=retry' /unvetted; do
  curl -s -o /dev/null -w "%{http_code} $p\n" "$B$p"
done
for p in /groups /groups/techs/go /sign-in /list /account; do curl -s -o /dev/null -w "%{http_code} $p -> %{redirect_url}\n" $B$p; done
# The webhook is not wired yet: a delivery with a body and no x-amz-content-sha256, as GitHub sends one, fails
# CloudFront's signing and gets 403 at the Function URL, as "Two facts" says.
curl -si -X POST -H 'Content-Type: application/json' -d '{"action":"deleted"}' $B/account/github/webhook | head -1  # 403
# An empty POST passes the signing and reaches the function, which refuses it for want of GitHub's signature.
curl -si -X POST $B/account/github/webhook | head -1  # 401
```

In a browser, signed in with GitHub:

1. **Sign in** from any page: GitHub asks to read your organizations, and returns to the page, with your avatar in
   the header. Reload: `x-cache: Miss from cloudfront` and `cache-control: private, no-store` every time. `/me` shows
   the libraries you and fabricahq publish and the projects that use Rulemart libraries, read from GitHub.
2. **Star** a rule of fabricahq/public-rules: the count rises, and `/me?tab=stars` lists it. Unstar it.
3. **Add a library**: at `/me/add`, add `https://github.com/fabricahq/release-planner`, which has no release tags.
   Within seconds, `/me/add/run` says the check failed, and why, which proves the web function queued its check and
   the worker ran it. Remove it from `/me/listings`.
4. **Private projects**: `/me/private`, Continue to GitHub, and install the app on your account with Only select
   repositories. GitHub returns to `/me/github/installed`, then `/me` includes the private projects you picked, marked
   Private. Uninstall it on GitHub, then press Refresh on `/me`: the private projects are gone, read without the
   webhook.
5. **Cart and checkout**, which need no sign-in: on a rule page of fabricahq/public-rules, Add to cart, Just this
   rule; on the library's Groups tab, tick Go and Add 1 group to cart. The header's badge says 2. `/cart` lists both,
   with the Prompt and Commands tabs filled in; Fork on the rule adds a `project add rule` line, and the Commands
   tab's footnote suggests `--ref release/<n>` without pinning anything. Signed in, Where it goes lists your projects.
   Copy copies the tab's text. Clear cart empties it at once and shows the empty state.
6. **Sign out**: the page says you're signed out, once.
7. Report this library on a library page opens a GitHub issue form with the library filled in.
8. With analytics on, Cloudflare's dashboard shows page views within a few minutes, for `/me` and `/cart` too, and the
   browser console shows no content security policy error.

Then the logs and alarms:

```sh
aws logs tail /aws/lambda/rulemart-web --profile rulemart --region us-west-2 --since 30m \
  | grep -E '"msg":"(signed in|sign-in failed|sign-in refused|listing not queued|request failed)"'
aws logs tail /aws/lambda/rulemart-web --profile rulemart --region us-west-2 --since 30m \
  | grep -ciE 'code=|state=|token|__Host-'            # 0: no code, state, token, or cookie in any line
aws logs tail /aws/lambda/rulemart-worker --profile rulemart --region us-west-2 --since 30m | grep -E 'listing|ingested'
aws cloudwatch describe-alarms --profile rulemart --region us-west-2 --state-value ALARM --query 'MetricAlarms[].AlarmName'
```

The last prints `[]`.

## 7. Unvet the test library, before announcing (Josh decides when)

The test library, fabricahq/code-rules-test-library, exists to test Rulemart, and is vetted so every page has two
libraries. Before announcing, remove it from `catalog/vetted.yaml` in a pull request of its own, which this runbook
prepares but doesn't make. The change deletes these three lines:

```diff
 libraries:
-  - host: github
-    repositoryID: 1398540739
-    repository: fabricahq/code-rules-test-library
   - host: github
     repositoryID: 1382078543
     repository: fabricahq/public-rules
```

with the reason in the pull request, since the file's history is the public record of vetting. The same pull request
updates CONTRIBUTING.md, which says both libraries are vetted for local development: locally, add the test library at
`/me/add` and run `make worker`, or keep it vetted in a local, uncommitted change. Since an unvetted library's pages
answer 404 until it's listed, it also updates the routes in `_internal/audit/conformance.sh` that open the test
library's rules, or has the script's header say to keep the test library vetted locally that way. Then release it
with Release Planner, which has no migration, pin it, and apply `web_lambda` and `worker_lambda` as in step 5.

Afterwards its pages answer 404 unless someone lists it, when they show under the unvetted warning; lists, groups,
search, and the sitemap leave it out unless a visitor includes unvetted libraries; carts holding its rules say it's no
longer on Rulemart, or, if it's listed, that they need confirming as unvetted, and checkout leaves them out until
then; its rules' stars stop showing. The worker stops checking it unless it's listed. Nothing is deleted.

## 8. Announce (Josh)

Once step 6 passes and step 7 is live, share <https://rulemart.fabricahq.com> where Code Rules' users are, saying what
Rulemart does for them in the README's words, with its call to list a library, which leads to `/me/add`. For the
first day, watch the alarms and the new listings, and vet libraries as requests arrive.

## 9. Later

- **Wire the webhook.** In infra-catalog: an optional `public_alias` input on `http-lambda`, which adds an alias with
  a Function URL whose authorization type is `NONE`, and an optional `unsigned_paths` input on
  `cloudfront-function-url`, which adds that URL as a second origin without origin access control and one cache
  behavior per path. In Rulemart: answer 404 to everything through the alias's URL except
  `POST /account/github/webhook`, whose signature check already refuses unsigned deliveries. In infra-live: set
  `public_alias` on `web_lambda` and `unsigned_paths = { function_url = <its endpoint>, path_patterns =
  ["/account/github/webhook"] }` on `cdn`, then apply `web_lambda`, then `cdn`. infra-live#27's description has the
  details. Once deliveries arrive, the privacy page's account of when Rulemart notices a change to an installation can
  add that a delivery discards what was read through it.
- **Drop `hello_messages`**, the walking skeleton's table, in a release after v0.2.0 is live. No release since v0.1.0
  reads or writes it, so it's a contract step that's safe while any of them runs: a new migration,
  `DROP TABLE IF EXISTS hello_messages;`, in its own pull request, with `internal/platform/database/migrate`'s tests
  that read it updated, and the release notes saying it removes a table only v0.0.x used, which rolls back no
  further than v0.1.0.
- **Turn the WAF rule on** if abuse appears: `write_rate_limit = { requests = 100, window_seconds = 300 }` in the stack,
  then apply `cdn`.
- **Switch `rulemart.ai`'s redirect to 301** once the domain is settled: `redirect_domain.status_code`, then apply
  `domain_redirect`, which needs `CLOUDFLARE_API_TOKEN`.
- **Restore reserved concurrency** when AWS raises the account's Lambda limit, as the stack's README describes.

## Rollback

- **Code**: pin the previous release, its tag and SHA-256 values, and apply `web_lambda` and `worker_lambda`. v0.1.0
  runs on the migrated schema, since v0.2.0's migrations only add tables, nullable or defaulted columns, and grants,
  or drop tables no published release created, and pages then show no sign-in, stars, listing, cart, or dashboard;
  the data stays. A function refuses only a schema older than its own.
- **Migrations** are forward-only: fix a bad one with a new one. Neon keeps 6 hours of history, so data damaged within
  that window can be restored from a branch at an earlier time, in Neon's console.
- **Sign-in**: empty `github_oauth.client_id`, and `github_app.app_id` with it, since the App needs sign-in, and apply
  `web_lambda`; pages offer no new sign-in. Visitors already signed in stay signed in until their session ends or they
  sign out.
- **Private repositories**: empty `github_app.app_id` and apply `web_lambda`; the dashboard reads public repositories
  only.
- **Analytics**: empty `web_analytics.site_token` and apply `web_lambda`.
- **CloudFront**: remove `static_paths` or set `write_rate_limit = null`, and apply `cdn`. Going back to only GET and
  HEAD, or dropping the cookies from the key, needs v0.1.0 deployed first: v0.2.0 depends on both.
- **The whole site**: set `web_reserved_concurrency = 0`, regenerate, and apply `web_lambda`, as the stack's README
  describes; set it back to `null` to reopen.
- **infra-catalog**: fix forward with a new release; the units use v0.8.0's inputs, so pinning v0.7.0 back would fail
  to plan.
