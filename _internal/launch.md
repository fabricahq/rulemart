# Launch runbook

How to put Rulemart's slices 3 to 9 live, from the open pull requests to the announcement, in order. Production runs
v0.1.0, slices 1, 1b, and 2, at <https://rulemart.fabricahq.com>, and `rulemart.ai` redirects there.

Every step names who does it. **Status, 2026-10-02:** the seven Rulemart pull requests are merged to `main` (#21 as
#28, since GitHub closed it when slice 3's branch was deleted); both GitHub apps exist; and the
[realignment](realignment.md) with the prototype comes before the release, so steps 2 onward wait for its slices.
Nothing in infra-live or infra-catalog is merged or applied. The infra-live stack is
`aws/rulemart/us-west-2/rulemart-prod`; run its commands from that folder with the `rulemart` AWS profile and the Neon
credentials its README's [Configure Neon](https://github.com/fabricahq/infra-live/blob/main/aws/rulemart/us-west-2/rulemart-prod/README.md#configure-neon)
describes. The `domain_redirect` unit doesn't change, so no step needs `CLOUDFLARE_API_TOKEN`; apply units one at a
time rather than with `run --all`, which plans that unit too.

## The pull requests

| Repository | PR | Branch | Base | What |
| --- | --- | --- | --- | --- |
| rulemart | [#20](https://github.com/fabricahq/rulemart/pull/20) | `claude/slice-3-browse-search` | `main` | Slice 3: browse and search. Migrations 00006, 00007. |
| rulemart | [#21](https://github.com/fabricahq/rulemart/pull/21) | `claude/slice-4-releases` | #20 | Slice 4: releases and comparison. Migration 00008. |
| rulemart | [#22](https://github.com/fabricahq/rulemart/pull/22) | `claude/slice-5-sign-in` | #21 | Slice 5: sign-in. Migration 00009, which needs `rulemart_accounts_writer`. |
| rulemart | [#23](https://github.com/fabricahq/rulemart/pull/23) | `claude/slice-6-listing` | #22 | Slice 6: listing and the unvetted area. Migration 00010. |
| rulemart | [#24](https://github.com/fabricahq/rulemart/pull/24) | `claude/slice-7-stars` | #23 | Slice 7: stars. Migration 00011. |
| rulemart | [#25](https://github.com/fabricahq/rulemart/pull/25) | `claude/slice-8-cart` | #24 | Slice 8: cart and checkout. Migration 00012. |
| rulemart | [#26](https://github.com/fabricahq/rulemart/pull/26) | `claude/slice-9-launch` | #25 | Slice 9: launch readiness and this runbook. No migration. |
| infra-catalog | [#25](https://github.com/fabricahq/infra-catalog/pull/25) | `claude/environment-reviewers` | `main` | `github-repository-config` environment reviewers, for Release Planner. Not on Rulemart's path. |
| infra-catalog | [#26](https://github.com/fabricahq/infra-catalog/pull/26) | `claude/cloudfront-cache-key-cookies` | `main` | `cloudfront-function-url`: `cache_key_cookies`. |
| infra-catalog | [#27](https://github.com/fabricahq/infra-catalog/pull/27) | `claude/rulemart-launch-cloudfront` | #26 | `cloudfront-function-url`: `static_paths`, `write_rate_limit`; `http-lambda`: `log_error_patterns`. |
| infra-live | [#23](https://github.com/fabricahq/infra-live/pull/23) | `claude/rulemart-slice-5-sign-in` | `main` | Sign-in's role, OAuth secret, CloudFront methods and cookies; pins infra-catalog v0.8.0. |
| infra-live | [#24](https://github.com/fabricahq/infra-live/pull/24) | `claude/rulemart-slice-6-listing` | #23 | The web function queues listings; optional worker GitHub token. |
| infra-live | [#25](https://github.com/fabricahq/infra-live/pull/25) | `claude/rulemart-slice-9-launch` | #24 | Analytics token, static paths, optional WAF rule, log alarms. |

Slices 7 and 8 need no infra-live change. A stacked pull request is merged after the one under it, by retargeting it
to `main`. Use merge commits, not squash, so each branch above still shares its history with `main` and merges
without conflicts. `delete_branch_on_merge` is off in all three repositories, so GitHub doesn't retarget for you.

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
   and infrastructure do. Decide:
   - whether `hello@fabricahq.com` is the right contact for privacy questions and security reports, which the
     privacy page and the Report a problem form name;
   - whether it needs anything legal the code can't tell, such as a lawful basis, a controller's name and address,
     or a jurisdiction. It deliberately claims none;
   - whether "Fabrica" should name the legal entity that runs Rulemart.
2. **Confirm the about page's vetting wording**: vetting "means Fabrica chose to show the library, not that it checked
   every rule". Say what Fabrica checks if it should say more.
3. **Decide the slice 9 Proposed decisions** in [slices/9-launch-readiness.md](slices/9-launch-readiness.md), and the
   earlier slices' that are still Proposed. Changing one is a commit on its slice's branch.
4. **Create the GitHub OAuth app**, in the fabricahq organization's settings, **Developer settings**, **OAuth Apps**,
   **New OAuth App**:

   | Field | Value |
   | --- | --- |
   | Application name | Rulemart |
   | Homepage URL | `https://rulemart.fabricahq.com` |
   | Application description | Sign in to Rulemart, the Code Rules library catalog. |
   | Authorization callback URL | `https://rulemart.fabricahq.com/account/github/callback` |
   | Enable Device Flow | Off |

   It has no permissions to set: Rulemart asks for no scopes. Generate a client secret, and keep it on the clipboard
   for step 3.4. Note the client ID, which isn't secret.
5. **Optionally, create the worker's GitHub token**: GitHub **Settings**, **Developer settings**, **Personal access
   tokens**, **Fine-grained tokens**: resource owner fabricahq or yourself, **Public repositories** (read-only), no
   permissions, the longest expiry GitHub allows, and a reminder to rotate it. Without it, the worker shares GitHub's
   60 anonymous API requests an hour.
6. **Optionally, add Cloudflare Web Analytics**: in the Cloudflare dashboard for the fabricahq account, open
   **Web Analytics** (under **Analytics & Logs**), choose **Add a site**, enter `rulemart.fabricahq.com`, and choose
   **Done**, without automatic setup, since Cloudflare doesn't proxy this hostname. Under **Manage site**, copy the
   `"token"` value from the snippet's `data-cf-beacon`. Without it, Rulemart loads no analytics, and `/privacy` says
   so.
7. **Decide on the WAF rate rule**, which stays off (`write_rate_limit = null`, about $6 a month if on).

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
   then `gh pr edit 25 -R fabricahq/infra-live --base main` and [#25](https://github.com/fabricahq/infra-live/pull/25).
   `rulemart_release` stays `v0.1.0`, and `github_oauth.client_id`, `web_analytics.site_token`, and
   `worker_github_token.token_set` stay unset until step 5.
2. From the stack's folder on `main`:

   ```sh
   terragrunt stack clean && terragrunt stack generate
   ```

3. **Roles**, which migration 00009 refuses to run without:

   ```sh
   terragrunt --working-dir .terragrunt-stack/accounts_writer_role run apply   # creates rulemart_accounts_writer, NOLOGIN
   terragrunt --working-dir .terragrunt-stack/web_database_role run apply      # adds it to rulemart_web's memberships, in place
   ```

   Optionally plan `catalog_reader_role`, `catalog_writer_role`, and `worker_database_role`: no changes expected, only
   the catalog ref and the login unit's wiring moved.
4. **Secrets**:
   1. `terragrunt --working-dir .terragrunt-stack/github_client_secret run apply` creates
      `/rulemart/prod/github-client-secret` with a placeholder, which the function treats as unset.
   2. `terragrunt --working-dir .terragrunt-stack/worker_github_token run apply` creates
      `/rulemart/prod/worker-github-token` with a placeholder.
   3. From slice R7, the units the dashboard needs create, each with a placeholder: `/rulemart/prod/token-key`, the
      key that seals each session's GitHub token, which GitHub sign-in refuses to start without;
      `/rulemart/prod/github-app-key`, the GitHub App "Rulemart by Fabrica"'s private key; and
      `/rulemart/prod/github-app-webhook-secret`, its webhook's secret.
   4. None of the values passes through OpenTofu or a shell's history:

      ```sh
      # the OAuth app's client secret, on the clipboard
      aws ssm put-parameter --profile rulemart --region us-west-2 --overwrite --type SecureString \
        --name /rulemart/prod/github-client-secret --value "$(pbpaste)"
      # optionally, the worker's token, on the clipboard
      aws ssm put-parameter --profile rulemart --region us-west-2 --overwrite --type SecureString \
        --name /rulemart/prod/worker-github-token --value "$(pbpaste)"
      # the token key: 32 random bytes in base64, never shown
      aws ssm put-parameter --profile rulemart --region us-west-2 --overwrite --type SecureString \
        --name /rulemart/prod/token-key --value "$(openssl rand -base64 32)"
      # the GitHub App's private key, the .pem file GitHub gave
      aws ssm put-parameter --profile rulemart --region us-west-2 --overwrite --type SecureString \
        --name /rulemart/prod/github-app-key --value "file://$HOME/Downloads/rulemart-by-fabrica.private-key.pem"
      # the app's webhook secret, the value set in the app's settings, on the clipboard
      aws ssm put-parameter --profile rulemart --region us-west-2 --overwrite --type SecureString \
        --name /rulemart/prod/github-app-webhook-secret --value "$(pbpaste)"
      ```

      Replacing `/rulemart/prod/token-key` later leaves every session's token sealed under the old key: each visitor
      is asked to sign in again before the dashboard reads GitHub. Nothing else breaks.

5. **CloudFront**: `terragrunt --working-dir .terragrunt-stack/cdn run plan`, then `apply`. Expect the default
   behavior's allowed methods to become all seven, its cache policy's cookies a whitelist of
   `__Host-rulemart-session` and `__Host-rulemart-notice`, and two new ordered behaviors, `/_static/*` and
   `/favicon.ico`, with a new `rulemart-web-static` cache policy and origin request policy. No web ACL. It takes
   several minutes to deploy. Then check v0.1.0 still serves:

   ```sh
   curl -sI https://rulemart.fabricahq.com/ | grep -iE '^(HTTP|cache-control|x-cache)'
   curl -si -X POST https://rulemart.fabricahq.com/sign-out | head -1   # v0.1.0 answers 404, from the function
   ```

## 4. Release Rulemart v0.2.0 (Josh, with an agent)

1. **Merge the rulemart stack, in order**, each after its checks pass, with a merge commit:
   [#20](https://github.com/fabricahq/rulemart/pull/20) into `main`; then for each of 21, 22, 23, 24, 25, 26:
   `gh pr edit <n> -R fabricahq/rulemart --base main`, wait for its checks, merge. `main` then holds slices 3 to 9.
   The issue forms that Report a library and Ask to vet a library open only work from `main`, so they start working
   now.
2. **Ask an agent to make a Rulemart release** with Release Planner. Expect **v0.2.0**: new features, no breaking
   change for infrastructure, since the new variables are optional and the new role exists. Its notes must say it
   adds migrations **00006 to 00014**, all expand-only but 00013 and 00014, which drop tables that no published
   release created:

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

3. **Merge the release pull request.** Release Planner's pre-publish workflow, "Migrate the database", applies
   00006 to 00014 to production, then tags and publishes `web.zip` and `worker.zip`. If it fails, nothing is
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

2. In an infra-live pull request, in `terragrunt.stack.hcl`:
   - `rulemart_release`: `tag = "v0.2.0"`, and `web_sha256` and `worker_sha256` from `SHA256SUMS`;
   - `github_oauth.client_id`: the OAuth app's client ID;
   - optionally `web_analytics.site_token`: the Cloudflare token;
   - optionally `worker_github_token.token_set = true`, once its parameter holds the token;
   - from slice R7, the GitHub App's `app_id`, `client_id`, and `slug` (`rulemart-by-fabrica`), once its two
     parameters hold their values; `/rulemart/prod/token-key` must hold its key before `github_oauth.client_id` is set.

   Merge it, then from `main`:

   ```sh
   terragrunt stack clean && terragrunt stack generate
   terragrunt --working-dir .terragrunt-stack/web_lambda run plan
   terragrunt --working-dir .terragrunt-stack/web_lambda run apply
   terragrunt --working-dir .terragrunt-stack/worker_lambda run plan
   terragrunt --working-dir .terragrunt-stack/worker_lambda run apply
   ```

   `web_lambda`'s plan shows the new code, `GITHUB_CLIENT_ID`, `GITHUB_CLIENT_SECRET_PARAMETER`, `TOKEN_KEY_PARAMETER`,
   `QUEUE_URL`, `CLOUDFLARE_WEB_ANALYTICS_TOKEN` if set, and from slice R7 `GITHUB_APP_ID`, `GITHUB_APP_CLIENT_ID`,
   `GITHUB_APP_SLUG`, `GITHUB_APP_PRIVATE_KEY_PARAMETER`, and `GITHUB_APP_WEBHOOK_SECRET_PARAMETER`, with
   `ssm:GetParameter` on each parameter, a second `ssm:GetParameter` and an `sqs:SendMessage` statement, and three
   metric filters and alarms: `rulemart-web-sign-in-failed`, `-listing-not-queued`, and `-sitemap-truncated`.
   `worker_lambda`'s shows the new code, and `GITHUB_TOKEN_PARAMETER` if `token_set`. Either order works: the schema
   is already migrated.
3. The first hourly poll after the deploy re-ingests each library whose older versions lack their content (slice 4),
   so comparisons fill in within the hour; nothing to run by hand.

## 6. Check production (Josh)

Confirm the SNS subscription email first, if it's pending, so the new alarms reach you.

```sh
B=https://rulemart.fabricahq.com
# Pages: cached for a minute at CloudFront, the security headers, analytics only if its token is set.
curl -sI $B/ | grep -iE '^(HTTP|cache-control|vary|strict-transport-security|permissions-policy|cross-origin-opener-policy|content-security-policy|x-cache)'
curl -sI $B/ | grep -i x-cache                      # Hit from cloudfront, the second time
# Crawlers: robots names the sitemap, the sitemap lists only vetted libraries' pages.
curl -s $B/robots.txt
curl -s $B/sitemap.xml | grep -c '<loc>'            # 5 site pages + groups + 2 libraries + their rules: about 155 today
curl -s $B/sitemap.xml | grep -c code-rules-test-library   # its pages, until step 7 unvets it
# Social card and canonical address on a library page.
curl -s $B/fabricahq/public-rules | grep -oE '<(link rel="canonical"|meta property="og:[a-z:]+")[^>]*>'
# Static files: one copy for everyone, signed in or not, kept a year.
css=$(curl -s $B/ | grep -oE '/_static/[0-9a-f]+/generated/app\.css')
curl -sI $B$css | grep -iE '^(cache-control|x-cache)'
curl -sI -H 'Cookie: __Host-rulemart-session=anything' $B$css | grep -i x-cache   # Hit, not Miss
curl -sI $B/favicon.ico | grep -iE '^(HTTP|content-type)'
# Writes: an empty POST passes CloudFront's signing and reaches the function.
curl -si -X POST $B/sign-out | head -1              # 303
# New pages.
for p in /about /privacy /groups /search?q=retry /unvetted; do curl -s -o /dev/null -w "%{http_code} $p\n" $B$p; done
```

In a browser, signed in with GitHub:

1. **Sign in** from any page: GitHub shows Rulemart reading only public information, and returns to the page, with
   your avatar in the header. Reload: `x-cache: Miss from cloudfront` and `cache-control: private, no-store` every
   time. `/account` shows your GitHub ID, login, and avatar.
2. **Star** fabricahq/public-rules: the page says so, the count rises, and `/account/stars` lists it. Unstar it.
3. **List a library**: at `/list`, list `fabricahq/release-planner`, which has no release tags. Within seconds,
   `/account/listings` shows it failed, saying why, which proves the web function queued its check and the worker ran
   it. Remove it.
4. **Cart and checkout**, which need no sign-in: on a rule page of fabricahq/public-rules, Add to cart, Just this
   rule; on the library's Groups tab, tick Go and Add 1 group to cart. The header's badge says 2. `/cart` lists both,
   with the Prompt and Commands tabs filled in; Fork on the rule adds a `project add rule` line, and the Commands
   tab's footnote suggests `--ref release/<n>` without pinning anything. Copy copies the tab's text. Clear cart
   empties it at once and shows the empty state.
5. **Sign out**: the page says you're signed out, once.
6. Report this library on a library page opens a GitHub issue form with the library filled in.
7. With analytics on, the dashboard shows page views within a few minutes, and the browser console shows no
   content security policy error.

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
updates CONTRIBUTING.md, which says both libraries are vetted for local development: locally, list the test library
at `/list` and run `make worker`, or keep it vetted in a local, uncommitted change. Then release it with Release
Planner, which has no migration, pin it, and apply `web_lambda` and `worker_lambda` as in step 5.

Afterwards its pages answer 404 unless someone lists it, when they show under the unvetted warning; search, groups,
and the sitemap leave it out; carts holding its rules say it's no longer on Rulemart, or, if it's listed, that they
need confirming as unvetted, and checkout leaves them out until then; its stars stop showing. The worker stops
checking it unless it's listed. Nothing is deleted.

## 8. Later

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
  runs on the migrated schema, since every migration in v0.2.0 only adds, and pages then show no sign-in, stars,
  listing, or cart; the data stays. A function refuses only a schema older than its own.
- **Migrations** are forward-only: fix a bad one with a new one. Neon keeps 6 hours of history, so data damaged within
  that window can be restored from a branch at an earlier time, in Neon's console.
- **Sign-in**: empty `github_oauth.client_id` and apply `web_lambda`; pages offer no new sign-in. Visitors already
  signed in stay signed in until their session ends or they sign out.
- **Analytics**: empty `web_analytics.site_token` and apply `web_lambda`.
- **CloudFront**: remove `static_paths` or set `write_rate_limit = null`, and apply `cdn`. Going back to only GET and
  HEAD, or dropping the cookies from the key, needs v0.1.0 deployed first: v0.2.0 depends on both.
- **The whole site**: set `web_reserved_concurrency = 0`, regenerate, and apply `web_lambda`, as the stack's README
  describes; set it back to `null` to reopen.
- **infra-catalog**: fix forward with a new release; the units use v0.8.0's inputs, so pinning v0.7.0 back would fail
  to plan.
