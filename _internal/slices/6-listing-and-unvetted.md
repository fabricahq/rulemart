# Slice 6: listing a library, and the unvetted area

## Goal

Anyone signed in can list a public Code Rules library by its GitHub repository. Rulemart checks the repository and
ingests it within a minute, and the lister watches that happen on their listings page. A listed library is unvetted:
it stays out of normal browsing and search, and shows only in the unvetted area, reached from the bottom of the
libraries page, where every page warns that it hasn't been vetted. Vetting it is a reviewed change to
`catalog/vetted.yaml`, as before; the library then moves into normal browsing without being listed again.

Decisions marked **Decided** are Josh's: those dated 2026-10-05 were proposed in this slice and stand as built.
**Superseded** ones were proposed here and replaced by the slice named. **Existing** ones are already in
[decisions.md](../decisions.md) or an earlier slice.

## What a visitor can do

- **List a library.** "List a library" on the libraries page and the unvetted libraries page leads to `/list`, which
  says what may be listed, what Check does, and who vets libraries and how. Signed out, it asks the visitor to sign
  in and come back. Signed in, it asks for the repository, as `owner/name` or its GitHub address. Check shows the
  repository and what listing it does, or why it can't be listed, and a button that lists it.
- **Watch it being checked.** Listing leads to `/account/listings`, which shows each of the visitor's listings:
  checking, listed with a link to its page, vetted, or failed with the reason. A listing being checked says when it
  was asked for, and offers Refresh status.
- **Try again, or remove a listing.** A failed listing can be tried again. Any listing can be removed, after a page
  that says what removing it does: a listed library leaves Rulemart at once.
- **Browse unvetted libraries.** "View unvetted libraries" at the bottom of `/libraries` leads to `/unvetted`, which
  lists them, under the warning. Their pages are the same pages vetted libraries have, each with the warning.

## Decisions

### Visibility

- **Decided:** unvetted libraries are hidden from normal browsing, reached only through "View unvetted libraries" at
  the bottom of the libraries page; every unvetted page shows "This library has not been vetted. Tread carefully.";
  search covers vetted libraries only; unvetted pages carry `noindex`, and links to them `nofollow`.
- **Decided (Josh), 2026-10-05: a page finds a library that's vetted or listed, and nothing else.** The page reads already take the
  release's vetted keys; a library's own pages now also find a library a listing names, and say which it is. The home
  page, `/libraries`, `/groups`, a group's page, and search still read only vetted keys, so they never show an
  unvetted library, rule, or group. A library the catalog stores but nobody lists or vets, such as one whose listing
  was removed, is missing, as before.
- **Decided (Josh), 2026-10-05: links an unvetted library's author wrote in its rules carry `rel="nofollow ugc"`**, beside the page's
  own `nofollow`. An empty unvetted libraries page shows no warning, since it shows no library.
- **Decided (Josh), 2026-10-05: every unvetted page carries `<meta name="robots" content="noindex, nofollow">` and names no canonical
  address.** `nofollow` in the page's own robots tag covers every link on it, including "View on GitHub" and links in
  rule text, so a listing can't borrow Rulemart's reputation for the repository either. Google advises against a
  canonical address on a page it shouldn't index. Links to unvetted pages from other pages, on `/libraries`, the
  listings page, and `/list`, carry `rel="nofollow"`. Rulemart has no sitemap.
- **Decided (Josh), 2026-10-05: the warning is a band under the library's name, in the one amber the palette gains for it.** The
  palette was grays, plus the diff's green and red, which mean added and deleted. Amber reads as caution without
  reading as an error. It says the decided sentence, then why: anyone signed in can list a library, Rulemart hasn't
  reviewed its rules, and rules are instructions a coding agent follows, so read each before adopting it. The
  unvetted libraries page says the same about all of them.
- **Decided (Josh), 2026-10-05: unvetted pages are cached as vetted ones are, for a minute.** A removed listing's pages can show for
  up to a minute more.

### Listing

- **Decided:** anyone signed in can list a public library. New libraries are unvetted until vetted.
- **Decided (Josh), 2026-10-05: listing is two steps, a GET form and a POST with an empty body.** Writes are POSTs with an empty body
  and their input in the action's query string (**Existing**), so a form can't post what the visitor typed. The form
  submits with GET to `/list?repository=...`, which checks the input and shows what will happen, with a List button
  that posts to `/list?repository=owner/name`. The confirmation is also where the page says the library will be
  unvetted. `http.CrossOriginProtection` refuses a POST another site starts (**Existing**).
- **Decided (Josh), 2026-10-05: the web function checks only what it can without GitHub; the worker checks the rest.** At `/list`, the
  web function checks that the input names a GitHub repository, and the limits below, and that no listing or visible
  library has that name. It takes `owner/name`, or an address: `https://` or `http://`, `github.com/` or
  `www.github.com/`, or a page inside the repository, such as `.../tree/main`, whose owner and name it takes. One
  trailing `.git` is dropped from every form, since GitHub refuses a repository name ending in `.git`, so
  `owner/name.git` can't slip past the duplicate check, in any case; a name still ending in `.git` is refused, as are
  owner names GitHub refuses, starting or ending with a hyphen or holding two in a row, and the first segments GitHub
  keeps for its own pages, such as `settings` or `orgs`, from a short list in the domain. It never
  calls GitHub: unauthenticated, GitHub allows 60 API requests an hour from an IP address that Lambda functions
  share, and the web function would need its own token. The worker looks the repository up, which also catches a
  missing or private one, and ingests it, which catches one that isn't a Code Rules library.
- **Decided (Josh), 2026-10-05: the web function queues the listing's job at once, and the hourly poll queues it again if that
  failed.** The web function regains `sqs:SendMessage` on the jobs queue, which slice 2 removed, so a listing is
  checked within seconds. Listing commits first, and a failure to queue is logged, not shown: the listing waits for
  the next poll, within the hour, and the listings page says so after three minutes.
- **Decided (Josh), 2026-10-05: a job names a listing only by its ID.** `{"listing": 42}` joins `{"host": "github", "repositoryID":
  "..."}`. The worker reads what to check from the listing's row, which only a signed-in visitor's POST writes, so a
  queued message still can't point it at an arbitrary repository (**Existing**, extended).
- **Decided (Josh), 2026-10-05: a listing is keyed by the repository's ID once the worker resolves it**, as vetting is. The name the
  lister gave stays for the listings page. A listing is unique by that name, without regard to case, and by the
  repository ID, so a renamed repository can't be listed twice under two names: the second fails as already listed.
- **Decided (Josh), 2026-10-05: a failed listing doesn't reserve its repository.** When another account lists a repository whose only
  listing failed before its library ever ingested, listing it removes the failed one, which only its lister could
  see. Otherwise one bad first check, or one account listing a name early, would keep everyone else out. A failed
  listing says so on its lister's page, rather than vanishing unexplained. A refusal says whose listing stands in the
  way: the visitor's own, with a link to their listings; another's being checked, how long ago by the same three
  minutes the lister's page uses; or another's that's listed, with a link to its page, `nofollow`.
- **Decided (Josh), 2026-10-05: what a lister sees.** Checking, while the worker hasn't finished; Listed, with a link, once ingested;
  Vetted, once the release's `vetted.yaml` names it; Failed, with the reason, when it never ingested; and Listed with
  the last check's failure when a later check failed, while its pages keep the last release ingested, as a vetted
  library's do. A check its lister asked for after another started records its own result, and the older check
  records nothing, so a slow check can't overwrite a newer one. A reason says what the repository got wrong, as a sentence, such as "GitHub has no public
  repository by this name" or "The repository has no release/<number> tags", never where it happened or a database
  error, which only the worker's log has. Raw repository IDs aren't shown.
- **Decided (Josh), 2026-10-05: the listings page never reloads itself.** QA found a five-second reload reset keyboard focus and scroll,
  which is hostile to keyboard and screen reader users. A listing being checked says when it was asked for, from
  `requested_at`, which a retry sets, and offers Refresh status, a link that reloads the page. Three minutes after it was
  asked for, it says the check is taking longer than usual and Rulemart checks it again within the hour: once queued,
  a check takes seconds, so by then it waits for the poll.

### Who listed it, and removing it

- **Superseded by [slice R6](15-library-and-rule-pages.md): a listing records the account that listed it, and pages don't name it.** The repository's owner answers
  for its rules, and their name and avatar are on every page; naming the lister would add personal data to public
  pages for little trust. The lister sees their listings on `/account/listings`, which the account menu and account
  page link to.
- **Decided (Josh), 2026-10-05: the lister can remove their listing at any time, after a page that says what that does.** Removing a
  listed library takes its pages away, so `/account/listings/remove?listing=N` asks first, by state: a listed library
  leaves Rulemart; a vetted one stays, and only the listing goes; a failed or unchecked one is forgotten. Its button
  posts to the same address with an empty body. Retrying a listing that isn't failing, from a page left open, says
  there's nothing to try again. The notice after removing says what removing did, by the listing's state. Signed out, removing or retrying leads to sign-in, as listing does. It deletes the row; the library leaves the unvetted
  area, and the worker stops checking it. Its catalog rows stay, since neither function can delete a library, and a
  later listing reuses them without ingesting again. Removing a vetted library's listing changes nothing visible.
- **Decided (Josh), 2026-10-05: deleting an account removes its listings**, which reverses slice 5's proposal that a listed library
  should stay listed. Kept listings would still count toward Rulemart's cap of 500, and signing in again starts a new
  account with a fresh allowance of 5, so one GitHub user could list, delete, and repeat until nobody could list.
  Removing them keeps "Rulemart deletes everything it keeps about you" true too. A vetted library stays vetted,
  since vetting is `vetted.yaml`'s; an unvetted one leaves the site until someone lists it again. The account page
  says so before deleting. Codex's review found this.
- **Decided (Josh), 2026-10-05: an operator removes an abusive listing with SQL**, `DELETE FROM listings WHERE id = ...`, as the
  database's owner. A report button can come later.

### Failures

- **Decided (Josh), 2026-10-05: a listed library's failure is the lister's to see, not an alarm.** A job for a listing records why its
  check failed on the listing and succeeds, so SQS doesn't retry it or move it to the dead-letter queue, whose alarm
  is for Rulemart's own failures. A failure Rulemart caused, such as the database being unreachable or GitHub's API
  refusing a request, still fails the job, as a vetted library's failure does (**Existing**).
- **Decided (Josh), 2026-10-05: a listing that never ingested is checked again by itself only for a day, and only once GitHub has
  confirmed its repository.** A failure to fetch it may be GitHub's for a moment, and its failure can't be told apart
  from the repository's reliably, so the hourly poll tries it again for a day after it was listed or retried; one
  GitHub has no public repository for, or that another listing names, isn't, since checking it costs an API call and
  the answer won't change. Its lister can try again at any time, within the request limits. Devin's review found the
  first version stranded a listing whose first fetch failed for a moment. A listing that
  ingested is checked every hour, failing or not, as a vetted library is, and its pages keep the last good release.

### Vetting

- **Existing: vetting is a reviewed change to `catalog/vetted.yaml`**, by host and repository ID, which ships with a
  release. An unvetted library becomes vetted when the release that adds it deploys, without being listed again: its
  catalog rows are the same, and pages decide vetted or not from the release's list as they read. Its listing stays,
  showing Vetted to its lister, and stops counting toward their limit. The worker checks it as a vetted library, and no
  longer as a listing. Removing it from `vetted.yaml` returns it to the unvetted area if it's listed, or hides it.
- **Decided (Josh), 2026-10-05: an operator reads the repository ID to vet from the listing's row**, `host_repository_id`. The
  listings page doesn't show it: it means nothing to a lister.

### Limits and abuse

- **Decided:** keep ingestion's size and memory caps (**Existing**); never execute library content; limit how many
  libraries one account can list.
- **Decided (Josh), 2026-10-05: an account holds at most 5 unvetted listings, and Rulemart at most 500.** Vetted listings count toward
  neither. Both are checked in the transaction that lists, under an advisory lock, so two requests at once can't pass
  either. Removing a listing frees its place. The global cap bounds the unvetted area's size, the worker's hourly
  work, and what someone with many GitHub accounts can add. At the cap, `/list` says Rulemart isn't taking new
  listings.
- **Existing: ingestion never executes library content.** It reads Git objects with go-git, parses YAML and Markdown,
  and renders rules with raw HTML escaped and dangerous links dropped, under the pages' content security policy. This
  slice adds nothing that runs a library's files.
- **Decided (Josh), 2026-10-05: an account lists or retries at most 20 times a day, and every account together at most 100 times an
  hour.** Each listing and retry queues a check, and one that isn't resolved yet costs a GitHub API call, so the caps
  on listings held don't bound the work: removing and listing again, or retrying, would. `listing_requests` keeps
  each request for a day, and it outlives a deleted account, unlinked, so the hourly cap holds however many
  accounts ask. Past either, `/list` and Try again say when to come back. A CloudFront WAF rate rule on `POST` is
  the next step if abuse appears, as slice 5 says.

### The worker, GitHub, and cost

- **Decided (Josh), 2026-10-05: the hourly poll queues a job for each vetted library and each listing to check**: every listing that
  isn't vetted, except one that failed without ever ingesting, as above. Reading the listings wakes Neon on the poll, which the
  jobs did anyway.
- **Decided (Josh), 2026-10-05: the worker can authenticate to GitHub's API with a token from SSM.** `GITHUB_TOKEN_PARAMETER` names a
  SecureString holding a fine-grained token with read access to public repositories only, beside `GITHUB_TOKEN` for
  local use. Unauthenticated, GitHub allows 60 requests an hour per IP address, which Lambda functions share. The
  worker calls the API once per new listing, and once per library whose tags changed; listing tags uses Git, which
  isn't counted. Without the token, the worker works as before, and a rate-limited lookup fails its job for SQS to
  retry.
- **Cost.** Each listing checked hourly is one SQS message, one invocation, one tag listing over HTTPS, and one
  Postgres read: half a second to a second at 2,048 MB, as the vetted libraries' jobs take locally. At the cap of 500,
  about 360,000 jobs a month: roughly $5 to $10 of Lambda, under $0.50 of SQS, and Neon awake a few more minutes an
  hour, with the worker's two concurrent jobs. An unchanged library costs no API call. If the cap rises,
  the next step is checking unvetted libraries less often than vetted ones.

### Routes

- **Superseded by [slice R7](16-dashboard-and-add-a-library.md): `/list`, `/unvetted`, and `/account/listings`.** One-segment paths can't hide a library's
  `/{owner}/{repo}`, and paths under `/account` can't either (**Existing**). `/libraries/unvetted` could: GitHub has
  an account named `libraries`. Removing and trying again are `POST /account/listings/remove?listing=42` and `POST
  /account/listings/retry?listing=42`. Signing in may return to `/account/listings`, the one page under `/account/`
  besides the account page that it may return to, since the rest are the flow's callback and actions that take POST.

### Data and roles

- **Decided (Josh), 2026-10-05: migration 00010 adds `listings` and `listing_requests`**: for a listing, the account that listed it; the host and the
  `owner` and `name` the lister gave; the repository ID once resolved; when it was listed, last asked for a check,
  and last checked; and why the last check failed. For a request, the account and when, for a day. It only adds
  tables, which the release still running doesn't read.
- **Decided (Josh), 2026-10-05: no new role.** Slice 5 said to grant new tables the web function writes to `rulemart_accounts_writer`,
  and a listing is an account's. It gets `SELECT`, `INSERT`, and `DELETE`, and `UPDATE` of only `requested_at` and
  `failure`, to try again, and `SELECT`, `INSERT`, and `DELETE` on `listing_requests`. `rulemart_catalog_reader` gets `SELECT`, which pages read to find listed libraries.
  `rulemart_catalog_writer` gets `SELECT`, and `UPDATE` of only `host_repository_id`, `checked_at`, and `failure`: the
  worker can't change who listed a library, or list one. Neither catalog role gets anything new on `accounts`.

## Infrastructure

Nothing is applied. An infra-live pull request, stacked on slice 5's, carries the changes:

- `web_lambda`: `QUEUE_URL`, and `sqs:SendMessage` on the jobs queue.
- `worker_github_token`: a new `/rulemart/prod/worker-github-token` SecureString, holding a placeholder until set.
- `worker_lambda`: `GITHUB_TOKEN_PARAMETER`, and `ssm:GetParameter` on it, once `worker_github_token_set` is true.

### What Josh creates

A fine-grained personal access token, in GitHub's Settings, Developer settings, Personal access tokens, Fine-grained
tokens: resource owner fabricahq or yourself, "Public repositories" only, no permissions, and the longest expiry
GitHub allows, with a reminder to rotate it. Put it in `/rulemart/prod/worker-github-token`, then set
`worker_github_token_set = true`.

### Order

1. Release Rulemart with this slice. Migration 00010 needs no new role.
2. Pin the release and apply `web_lambda` and `worker_lambda`. Before the web function has `QUEUE_URL`, listings wait
   for the hourly poll.
3. Optionally, apply `worker_github_token`, set its value, set `worker_github_token_set = true`, and apply
   `worker_lambda` again.

## Verification

- **Migration tests:** 00010's grants, and what each role may and may not do with listings.
- **Store and app tests**, as the functions' roles: listing, its checks and limits, duplicates by name and ID,
  removing and retrying only one's own listings, the worker resolving, ingesting, and recording failures, and which
  listings the poll checks.
- **Page tests:** unvetted libraries missing from home, `/libraries`, `/groups`, group pages, and search; the
  unvetted area and every unvetted page with the warning, `noindex, nofollow`, and no canonical address; links to them
  `nofollow`; the listing form's steps, sign-in, limits, and errors; the listings page's states; and removing and
  retrying.
- **Worker tests:** listing jobs, the poll's listings, and failures recorded rather than retried.
- **In a browser, locally:** both vetted libraries ingested, two real unvetted libraries listed as a test user and
  ingested by `make worker`, one repository that isn't a library failing, at 1280 and 390 pixels, light and dark.

## Not in this slice

Stars, the cart, and checkout, which names unvetted rules to the agent. Reporting a listing. Listing from another
code host. Showing who listed a library. Checking unvetted libraries less often than vetted ones. A WAF rate rule.
