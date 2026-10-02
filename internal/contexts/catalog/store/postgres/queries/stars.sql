-- Stars: the web function stars and unstars libraries for accounts, and lists an account's stars. A library is vetted
-- when vetted, the release's vetted keys as host:repository ID, holds its key.

-- StarLibrary stars the library owner/name for the account, matched without regard to case, when vetted holds it, and
-- returns it as the host spells it now, or no row when vetted holds no library by that name. A library the account
-- starred already keeps its star.
-- name: StarLibrary :one
WITH library AS (
    SELECT l.id, l.owner, l.name, l.owner_avatar_url FROM libraries l
    WHERE l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
      AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
),
starred AS (
    INSERT INTO stars (account_id, library_id) SELECT @account_id::bigint, b.id FROM library b
    ON CONFLICT (account_id, library_id) DO NOTHING
)
SELECT b.owner, b.name, b.owner_avatar_url FROM library b;

-- UnstarLibrary removes the account's star from the library owner/name, matched without regard to case, and
-- returns how many libraries have that name: none when the catalog has no such library.
-- name: UnstarLibrary :one
WITH library AS (
    SELECT l.id FROM libraries l
    WHERE l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
),
unstarred AS (
    DELETE FROM stars s USING library b WHERE s.library_id = b.id AND s.account_id = @account_id::bigint
)
SELECT count(*) FROM library;

-- IsStarred reports whether the account starred the library owner/name, matched without regard to case.
-- name: IsStarred :one
SELECT EXISTS (
    SELECT 1 FROM stars s JOIN libraries l ON l.id = s.library_id
    WHERE s.account_id = @account_id::bigint
      AND l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
)::boolean AS starred;

-- ListAccountStars returns the libraries the account starred, most recently starred first, each as the libraries'
-- list shows it, with whether vetted holds it, and whether a listing names it.
-- name: ListAccountStars :many
SELECT l.owner, l.name, l.description, l.owner_avatar_url,
       (SELECT count(*) FROM rules r WHERE r.library_id = l.id AND r.retired_in_release_id IS NULL) AS rule_count,
       (SELECT count(*) FROM stars a WHERE a.library_id = l.id) AS star_count,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
       EXISTS (SELECT 1 FROM listings g WHERE g.host = l.host AND g.host_repository_id = l.host_repository_id)::boolean
           AS listed,
       s.created_at AS starred_at
FROM stars s
JOIN libraries l ON l.id = s.library_id
WHERE s.account_id = @account_id::bigint
ORDER BY s.created_at DESC, lower(l.owner), lower(l.name);
