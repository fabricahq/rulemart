-- The cart: checkout resolves what a browser's cart names against the catalog, as the web function does, and writes
-- nothing. A library is vetted when vetted, the release's vetted keys as host:repository ID, holds its key.

-- FindCartLibraries returns the libraries full_names names, each as owner/name in lowercase, matched without regard to
-- case, that vetted holds or a listing names, with whether vetted holds it, and its latest release with the commit that
-- release's tag pointed to when it was ingested.
-- name: FindCartLibraries :many
SELECT l.id, l.owner, l.name, l.owner_avatar_url,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
       latest.number AS latest_release, latest.commit_id AS latest_commit
FROM libraries l
JOIN LATERAL (
    SELECT number, commit_id FROM library_releases WHERE library_id = l.id ORDER BY number DESC LIMIT 1
) latest ON true
WHERE l.host = @host
  AND lower(l.owner || '/' || l.name) = ANY (@full_names::text[])
  AND (
      l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
      OR EXISTS (SELECT 1 FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id)
  );

-- ListCartRules returns the rules, current and retired, of the groups groups names, each as the library's ID, a
-- slash, and the group's path in lowercase, of the libraries library_ids, with each rule's newest version: its title,
-- or '' when it has none, at most title_runes characters of it, since a library may write one of any length, and its
-- version, and for a retired rule, the release that retired it. They're in order of library, then group, then title and ID, as a
-- library's page lists them.
-- name: ListCartRules :many
SELECT r.library_id, r.path, g.path AS group_path, left(coalesce(v.title, ''), @title_runes::integer) AS title, v.major, v.minor, v.patch,
       coalesce(retired.number, 0)::integer AS retired_in
FROM rules r
JOIN library_groups g ON g.id = r.group_id
JOIN LATERAL (
    SELECT v.title, v.major, v.minor, v.patch
    FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
    WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
) v ON true
LEFT JOIN library_releases retired ON retired.id = r.retired_in_release_id
WHERE g.library_id = ANY (@library_ids::bigint[]) AND g.library_id::text || '/' || lower(g.path) = ANY (@groups::text[])
ORDER BY r.library_id, g.path, lower(v.title), r.path;
