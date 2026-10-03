-- The dashboard: the libraries a visitor and their organizations publish, and the current versions of the libraries
-- their projects import, which update counts compare with.

-- ListOwnedLibraries returns the libraries vetted holds or a listing names whose owner is one of owners, in lowercase,
-- each with whether vetted holds it, how many current rules it holds, and when it came to Rulemart: when its listing
-- was made, or without one, when it was first ingested.
-- name: ListOwnedLibraries :many
SELECT l.id, l.owner, l.name, l.owner_avatar_url,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
       (SELECT count(*) FROM rules r WHERE r.library_id = l.id AND r.retired_in_release_id IS NULL) AS rule_count,
       coalesce(listing.created_at, l.created_at)::timestamptz AS added_at
FROM libraries l
LEFT JOIN LATERAL (
    SELECT s.created_at FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id
) listing ON true
WHERE lower(l.owner) = ANY (@owners::text[])
  AND (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]) OR listing.created_at IS NOT NULL)
ORDER BY lower(l.owner), lower(l.name);

-- ListCurrentRuleIDs returns the IDs of the current rules of the libraries library_ids names, by library.
-- name: ListCurrentRuleIDs :many
SELECT r.id, r.library_id FROM rules r
WHERE r.library_id = ANY (@library_ids::bigint[]) AND r.retired_in_release_id IS NULL;

-- ListNamedLibraries returns the libraries vetted holds or a listing names whose owner/name, in lowercase, is one of
-- names, each with whether vetted holds it.
-- name: ListNamedLibraries :many
SELECT l.id, l.owner, l.name, l.owner_avatar_url,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted
FROM libraries l
WHERE lower(l.owner || '/' || l.name) = ANY (@names::text[])
  AND (
      l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
      OR EXISTS (SELECT 1 FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id)
  )
ORDER BY lower(l.owner), lower(l.name);

-- ListRuleStates returns every rule of the libraries library_ids names, current or retired, by library, with a current
-- rule's current version, and zeros for a retired one.
-- name: ListRuleStates :many
SELECT r.library_id, r.path, (r.retired_in_release_id IS NOT NULL)::boolean AS retired,
       coalesce(v.major, 0)::integer AS major, coalesce(v.minor, 0)::integer AS minor, coalesce(v.patch, 0)::integer AS patch
FROM rules r
LEFT JOIN rule_versions v ON v.rule_id = r.id AND v.html IS NOT NULL AND r.retired_in_release_id IS NULL
WHERE r.library_id = ANY (@library_ids::bigint[])
ORDER BY r.library_id, r.path;
