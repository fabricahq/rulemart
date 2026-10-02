-- name: ListLibraries :many
SELECT l.id, l.owner, l.name, l.description, l.owner_avatar_url,
       (SELECT count(*) FROM rules r WHERE r.library_id = l.id AND r.retired_in_release_id IS NULL) AS rule_count
FROM libraries l
WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
ORDER BY lower(l.owner), lower(l.name);

-- name: GetLibrary :one
SELECT l.id, l.owner, l.name, l.description, l.owner_avatar_url, l.license_expression, l.license_file,
       latest.number AS latest_release, latest.tagged_at AS latest_tagged_at
FROM libraries l
JOIN LATERAL (
    SELECT number, tagged_at FROM library_releases WHERE library_id = l.id ORDER BY number DESC LIMIT 1
) latest ON true
WHERE l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
  AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[]);

-- ListGroups returns the groups that hold current rules: a group whose rules are all retired stays in the catalog,
-- but not on the library's page.
-- name: ListGroups :many
SELECT g.path, g.description, g.when_to_read, current.rule_count
FROM library_groups g
JOIN LATERAL (
    SELECT count(*) AS rule_count FROM rules r WHERE r.group_id = g.id AND r.retired_in_release_id IS NULL
) current ON current.rule_count > 0
WHERE g.library_id = @library_id
ORDER BY g.path;

-- name: ListCurrentRules :many
SELECT r.path, g.path AS group_path, v.title::text AS title, v.impact::text AS impact, v.major, v.minor, v.patch
FROM rules r
JOIN library_groups g ON g.id = r.group_id
JOIN rule_versions v ON v.rule_id = r.id AND v.html IS NOT NULL
WHERE r.library_id = @library_id
ORDER BY g.path, lower(v.title), r.path;

-- name: GetRule :one
SELECT r.id, r.path, g.path AS group_path, v.title::text AS title, v.impact::text AS impact,
       v.when_to_read::text AS when_to_read, v.html::text AS html, v.major, v.minor, v.patch,
       published.number AS release, published.tagged_at AS published_at
FROM rules r
JOIN library_groups g ON g.id = r.group_id
JOIN rule_versions v ON v.rule_id = r.id AND v.html IS NOT NULL
JOIN library_releases published ON published.id = v.release_id
WHERE r.library_id = @library_id AND r.path = @path;

-- name: ListVersions :many
SELECT v.major, v.minor, v.patch, published.number AS release, v.change, v.summaries, published.tagged_at AS published_at
FROM rule_versions v
JOIN library_releases published ON published.id = v.release_id
WHERE v.rule_id = @rule_id
ORDER BY published.number DESC;
