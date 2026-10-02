# Slice 2: automatic updates

## Goal

When a vetted library publishes a release, Rulemart shows it within about an hour, without an operator. The
worker function checks each vetted library on a schedule, and ingests the ones whose release tags changed. It
connects as a login that can only write the catalog, so production ingestion no longer needs the database's owner.

Decisions marked **Proposed** are new in this slice and wait for review. **Existing** ones are already in
[decisions.md](../decisions.md) or slice 1.

## Scope

**Trigger.** **Proposed:** the existing EventBridge schedule invokes the worker function instead of the web
function, with the same `{"source": "schedule"}` event. **Decided:** it runs hourly, `rate(1 hour)`, instead of
every 10 minutes. The worker sends one job per library in the vetted list its release embeds to the existing jobs
queue, and returns. Why:

- The schedule, the queue, its dead-letter queue, and their alarms already exist.
- The web function stays a page reader: it links no go-git (**Existing**), and loses its queue permission.
- Hourly, as Josh decided: each job reads the stored tags from Postgres, which wakes Neon's compute from scale to
  zero. Every 10 minutes kept it awake about half the time, about $12 a month; hourly costs about $3. A release
  shows within about an hour, plus ingestion. If updates need to be faster, the check can compare the listed tags
  with a fingerprint kept outside Postgres, so an unchanged library never wakes the database.

**Jobs.** **Proposed:** a job names one library by its code host and the host's repository ID, the key vetting
uses (**Existing**): `{"host": "github", "repositoryID": "1398540739"}`. The worker refuses a job with unknown
fields, or for a library its release doesn't vet. A job holds no URL, so a queued message can't point the worker
at another repository. One job per library keeps failures apart: an unreachable or broken library fails only its
own job.

**Checking for new releases cheaply.** **Proposed:** each job first compares the library's release tags with what
the catalog stored, and ingests only when they differ:

1. Read the clone URL ingestion last fetched from, and each stored release's tag object ID. Migration 00004 adds
   both columns.
2. List the remote's references with go-git, as `git ls-remote` does: one HTTPS request, with no objects fetched.
   Keep the `release/<number>` tags that Code Rules accepts, with their tag object IDs.
3. If the numbers and IDs match exactly, stop. Otherwise, or when the catalog has no clone URL or tag IDs for the
   library yet, look the repository up by ID in GitHub's API, and run slice 1's ingestion.

Why tag object IDs and not commit IDs: a release's record is in its tag's message, so a tag rewritten on the same
commit is still a change. Why the stored clone URL and not an API lookup on every check: GitHub allows 60
unauthenticated API requests an hour from one IP address, and Lambda functions share addresses, but listing
references over Git doesn't count against it. A renamed repository still answers at its old URL through GitHub's
redirect, and the next ingestion stores its new name and URL.

**Ingestion.** **Proposed:** the worker function ingests, through the queue's existing SQS trigger: one message per
invocation, at most two at once.

- **Idempotent.** Ingestion replaces a library's rows in one transaction, and changes nothing when the tags are
  unchanged (**Existing**). A duplicate job finds the tags unchanged and stops after the check.
- **Bounded.** Ingestion keeps slice 1's limits on what one library may hold in memory, about 1 GiB at most
  (**Existing**). The worker gets 2,048 MB of memory, so a library at the limits fails its job rather than the
  function. Its timeout is 120 seconds, and each job stops 10 seconds before that, so its transaction rolls back
  while the function still runs. The libraries Rulemart knows ingest in under two seconds. Three more bounds keep a
  hostile library within these, each **Proposed**:
  - Listing and fetching refuse more than 16 MiB of advertised references, which go-git would otherwise hold whole.
  - A rule gets one second to highlight its code, and code past that shows escaped and without highlighting, since
    chroma takes minutes on some inputs.
  - Ingestion checks the job's deadline between rules and between release records, and the releases of one fetch
    share their decoded Git trees.
- **Failures.** A failed job writes nothing, so the catalog keeps the last release it ingested. The worker reports
  the message as failed, SQS retries it after the visibility timeout of 720 seconds, six times the function's
  timeout as the module requires, and moves it to the dead-letter queue after five attempts (**Existing**). The
  dead-letter queue's alarm emails (**Existing**). The oldest-message alarm rises from 600 to 1,800 seconds, so one
  retried job doesn't set it off. Both alarms follow the visibility timeout, not the poll's rate. Scheduler retries a
  throttled poll for up to 50 minutes, so a busy moment doesn't skip an hour. A library that stays broken fails
  again every hour, and keeps the dead-letter alarm on until it's fixed or no longer vetted.

**Database access.** **Proposed**, following the split between infrastructure and migrations (**Existing**):

- Infrastructure creates `rulemart_catalog_writer`, a NOLOGIN group role, and `rulemart_worker`, a login role that's
  a member of it, with SQL. It writes the login's pooled connection string to `/rulemart/prod/worker-database-url`.
- Migration 00005 checks that the group role exists and is plain, as 00003 checks the reader. It grants it exactly
  what ingestion uses: `USAGE` on the `public` schema; `SELECT`, `INSERT`, and `UPDATE` on `libraries`, since
  ingestion never deletes a library; `SELECT`, `INSERT`, `UPDATE`, and `DELETE` on `library_releases`,
  `library_groups`, `rules`, and `rule_versions`; and `SELECT` on `goose_db_version`, for the schema check. It
  grants nothing on `hello_messages`, and nothing that changes the schema.
- The worker connects as `rulemart_worker`, and reads no other parameter. Only migrations still need the owner.

**What the worker logs.** **Proposed:** the worker writes JSON lines with Go's `slog`, under snake_case keys that
mean the same on every line, so CloudWatch Logs Insights can group by them. It never logs a connection string or a
token.

- One line per job: `ingested library`, `library unchanged`, or `job failed`, each with `outcome` (`ingested`,
  `unchanged`, or `failed`), `message_id`, and `duration_ms`, from the job's start to its end, and with `host` and
  `repository` once the job names a library. `ingested library` adds `full_name`, `releases`, `rules`,
  `rows_changed`, and how long listing the tags and ingesting took, in `list_ms` and `ingest_ms`. `library
  unchanged` adds `list_ms`, and `job failed` adds `error`.
- One `batch processed` line per SQS batch, with `jobs`, the count of each outcome under its name, and
  `duration_ms`.
- One `poll queued` line per scheduled invocation, with `libraries`, `queued`, `queue_failures`, and `duration_ms`.

Failures and p95 job duration by library, in `/aws/lambda/rulemart-worker` with the time range set to the last
week. `strcontains` returns 1 or 0, so its sum counts the failed jobs:

```text
filter ispresent(outcome)
| stats sum(strcontains(outcome, "failed")) as failures, pct(duration_ms, 95) as p95_ms, count(*) as jobs by host, repository
| sort failures desc, p95_ms desc
```

**Operator command.** **Proposed:** `cmd/ingest <repository URL>` stays, for local development and backfills,
including libraries not yet vetted. `make ingest` connects as `rulemart_worker` locally, as `make web` connects as
`rulemart_web`, and `make db` creates both new roles. A production backfill sets
`DATABASE_URL_PARAMETER=/rulemart/prod/worker-database-url`. `make worker` runs the worker locally: one poll, with a
queue in memory in place of SQS, through the same handler as on Lambda.

**The skeleton's leftovers.** **Proposed:**

- The web function stops acknowledging the schedule's event, and answers only Function URL requests. The
  infrastructure change moves the schedule to the worker before it deploys this web function.
- The worker's placeholder handler, which left every message for the dead-letter queue, goes.
- `hello_messages` stays until the release after this one. Production still runs the skeleton, which writes it,
  and migrations run before the functions change, so dropping it now would break the running release
  (**Existing**: make a breaking change in two releases).
- In infrastructure, the web function loses its queue URL and permission, and reads `rulemart_web`'s parameter, the
  switch slice 1 left for the release that grants `rulemart_catalog_reader`. The CDN allows only `GET` and `HEAD`,
  since `POST /enqueue` is gone. Every unit is still used, so none is deleted.

## Verification

- **Update tests:** integration tests against Postgres and Git repositories built in the test, through the update
  operation, the worker's handler, and the migrations. They cover:
  - a new release, and a rewritten release tag, which ingestion picks up
  - unchanged tags, which stop after the check without fetching
  - updating twice, which changes nothing the second time
  - a library whose remote is unreachable, which fails its job and changes nothing
  - a malformed new release, which fails its job and leaves the last ingested release
  - a library that isn't vetted, and a malformed job, which the worker refuses
  - the worker's login, which can ingest and nothing more, and a missing grant, which fails ingestion with
    permission denied and writes nothing
  - a missing or privileged `rulemart_catalog_writer`, which the migration refuses
- **Real data, locally:** `make worker` against `fabricahq/code-rules-test-library`: the first run ingests it, and
  the second finds nothing to do. `make ingest` ingests `fabricahq/public-rules`, release 1 with 127 rules, as
  `rulemart_worker`. Vetting public-rules is a separate decision, so this slice doesn't add it to
  `catalog/vetted.yaml`.
- **After deployment:** the worker's logs show a check of each vetted library every hour, and the dead-letter
  queue stays empty. Publishing a release of the test library shows it on its page within about an hour.

## Not in this slice

A GitHub token for the worker, and webhooks instead of polling. Checks faster than hourly, through a tag
fingerprint kept outside Postgres. Refreshing a library's description, avatar, or
name between releases. Backing off from a library that keeps failing. Ingesting libraries that aren't vetted,
which waits for the unvetted area. Dropping `hello_messages`, in the next release. Browsing by technology or
practice, search, and more than one library (slice 3). The Library releases tab and version comparison (slice 4).
