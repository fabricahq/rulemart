-- ListSitemapLibraries returns the vetted libraries, ordered by owner and name without regard to case, each with when
-- its latest release was tagged.
-- name: ListSitemapLibraries :many
SELECT l.id, l.owner, l.name, latest.tagged_at
FROM libraries l
JOIN LATERAL (
    SELECT tagged_at FROM library_releases WHERE library_id = l.id ORDER BY number DESC LIMIT 1
) latest ON true
WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
ORDER BY lower(l.owner), lower(l.name);

-- ListSitemapRules returns at most max_rules of the vetted libraries' current rules, ordered by library as
-- ListSitemapLibraries orders them, then by ID, each with when the release that published its current version was
-- tagged. Only a current rule's current version keeps its HTML.
-- name: ListSitemapRules :many
SELECT l.id AS library_id, r.path, rel.tagged_at
FROM libraries l
JOIN rules r ON r.library_id = l.id AND r.retired_in_release_id IS NULL
JOIN rule_versions v ON v.rule_id = r.id AND v.html IS NOT NULL
JOIN library_releases rel ON rel.id = v.release_id
WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
ORDER BY lower(l.owner), lower(l.name), r.path
LIMIT @max_rules::integer;
