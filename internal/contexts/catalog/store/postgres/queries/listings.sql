-- Listings: the web function adds, removes, and retries them for their accounts; the worker resolves and checks them.
-- A library is vetted when vetted, the release's vetted keys as host:repository ID, holds its key.

-- LockListings holds, until the transaction ends, the lock every listing takes, so the limits a listing checks can't
-- change before it's added.
-- name: LockListings :exec
SELECT pg_advisory_xact_lock(4_812_337_015);

-- FindListingConflict returns what already stands in the way of listing the repository owner/name, matched without
-- regard to case: a listing by that name, or a library by that name that a listing names or vetted holds. It also
-- returns the library a listing by that name names, or else the library by that name, as the host spells it now, or
-- empty names when the catalog stores neither.
-- name: FindListingConflict :one
WITH named AS (
    SELECT s.host_repository_id FROM listings s
    WHERE s.host = @host AND lower(s.owner) = lower(@owner) AND lower(s.name) = lower(@name)
),
library AS (
    SELECT l.owner, l.name, l.host_repository_id FROM libraries l
    WHERE l.host = @host AND (
        (lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name))
        OR l.host_repository_id IN (SELECT n.host_repository_id FROM named n)
    )
    ORDER BY l.host_repository_id IN (SELECT n.host_repository_id FROM named n) DESC
    LIMIT 1
)
SELECT (
           EXISTS (SELECT 1 FROM named)
           OR EXISTS (
               SELECT 1 FROM library b JOIN listings s ON s.host = @host AND s.host_repository_id = b.host_repository_id
           )
       )::boolean AS listed,
       EXISTS (SELECT 1 FROM library b WHERE @host || ':' || b.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
       coalesce((SELECT b.owner FROM library b), '')::text AS library_owner,
       coalesce((SELECT b.name FROM library b), '')::text AS library_name;

-- CountUnvettedListings counts the listings that vetted doesn't hold: the account's, and every account's. A listing
-- the worker hasn't resolved yet counts as unvetted.
-- name: CountUnvettedListings :one
SELECT count(*) FILTER (WHERE account_id = @account_id::bigint) AS account_listings, count(*) AS all_listings
FROM listings
WHERE NOT coalesce(host || ':' || host_repository_id = ANY (@vetted::text[]), false);

-- name: CreateListing :one
INSERT INTO listings (account_id, host, owner, name) VALUES (@account_id::bigint, @host, @owner, @name) RETURNING id;

-- ListAccountListings returns the account's listings, newest first, each with whether vetted holds it, and the
-- library it names when the catalog stores it.
-- name: ListAccountListings :many
SELECT s.id, s.owner, s.name, coalesce(s.host_repository_id, '')::text AS host_repository_id, s.created_at,
       s.requested_at, s.checked_at, s.failure,
       coalesce(s.host || ':' || s.host_repository_id = ANY (@vetted::text[]), false)::boolean AS vetted,
       (l.id IS NOT NULL)::boolean AS ingested, coalesce(l.owner, '')::text AS library_owner,
       coalesce(l.name, '')::text AS library_name, coalesce(l.owner_avatar_url, '')::text AS library_avatar_url
FROM listings s
LEFT JOIN libraries l ON l.host = s.host AND l.host_repository_id = s.host_repository_id
WHERE s.account_id = @account_id::bigint
ORDER BY s.created_at DESC, s.id DESC;

-- name: DeleteListing :execrows
DELETE FROM listings WHERE id = @id AND account_id = @account_id::bigint;

-- RetryListing asks the worker to check the account's listing again, when its last check failed.
-- name: RetryListing :execrows
UPDATE listings SET requested_at = now(), failure = NULL
WHERE id = @id AND account_id = @account_id::bigint AND failure IS NOT NULL;

-- GetListing returns a listing as the worker checks it, with whether the catalog stores the library it names.
-- name: GetListing :one
SELECT s.id, s.host, s.owner, s.name, coalesce(s.host_repository_id, '')::text AS host_repository_id,
       EXISTS (
           SELECT 1 FROM libraries l WHERE l.host = s.host AND l.host_repository_id = s.host_repository_id
       )::boolean AS ingested
FROM listings s
WHERE s.id = @id;

-- name: ResolveListing :execrows
UPDATE listings SET host_repository_id = @host_repository_id::text WHERE id = @id;

-- RecordListingCheck records that a check of the listing finished, and why it failed, or NULL when it didn't.
-- name: RecordListingCheck :execrows
UPDATE listings SET checked_at = now(), failure = sqlc.narg(failure) WHERE id = @id;

-- ListListingsToCheck returns the listings the hourly poll checks, in the order they were listed: every one vetted
-- doesn't hold, except one whose check failed before its library ever ingested, which waits for its lister to try
-- again.
-- name: ListListingsToCheck :many
SELECT s.id
FROM listings s
WHERE NOT coalesce(s.host || ':' || s.host_repository_id = ANY (@vetted::text[]), false)
  AND NOT (
      s.failure IS NOT NULL
      AND NOT EXISTS (SELECT 1 FROM libraries l WHERE l.host = s.host AND l.host_repository_id = s.host_repository_id)
  )
ORDER BY s.id;
