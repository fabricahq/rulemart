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

-- ListVettedGroups returns each group that holds current rules in a vetted library, with the library and how many of
-- its current rules the group holds, in group path order and then the library's owner and name.
-- name: ListVettedGroups :many
SELECT g.path, l.owner, l.name, l.owner_avatar_url, count(*) AS rule_count
FROM library_groups g
JOIN libraries l ON l.id = g.library_id
JOIN rules r ON r.group_id = g.id AND r.retired_in_release_id IS NULL
WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
GROUP BY g.path, l.id
ORDER BY g.path, lower(l.owner), lower(l.name);

-- ListGroupRules returns the current rules of the group at path in every vetted library that has it, in the library's
-- owner and name order, then title order.
-- name: ListGroupRules :many
SELECT l.owner, l.name, l.owner_avatar_url, r.path, v.title::text AS title, v.impact::text AS impact,
       v.major, v.minor, v.patch
FROM library_groups g
JOIN libraries l ON l.id = g.library_id
JOIN rules r ON r.group_id = g.id
JOIN rule_versions v ON v.rule_id = r.id AND v.html IS NOT NULL
WHERE g.path = @path AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
ORDER BY lower(l.owner), lower(l.name), lower(v.title), r.path;

-- SearchRules returns the vetted libraries' current rules that match query, best first, at most max_results of them,
-- each with how many matched in all. query is what a visitor typed, in websearch_to_tsquery's syntax, which accepts any
-- text. A rule matches through its search document or through its group's names: a canonical group's name on the list,
-- whose IDs and names canonical_ids and canonical_names hold in step, and the name part of any group's ID, but never
-- what its library calls the group. It matches by text and by group name separately, so the first can use the search
-- documents' index. Equal ranks keep a stable order: by title, the library's owner and name, then rule ID.
-- name: SearchRules :many
WITH search AS (
    SELECT websearch_to_tsquery('english', @query::text) AS query
),
named_groups AS (
    SELECT g.id,
           setweight(to_tsvector('english',
               coalesce((@canonical_names::text[])[array_position(@canonical_ids::text[], g.path)], '') || ' ' ||
               split_part(g.path, '/', 2)), 'B') AS names
    FROM library_groups g
    JOIN libraries l ON l.id = g.library_id
    WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
),
matched AS (
    SELECT v.id
    FROM rule_versions v, search
    WHERE v.html IS NOT NULL AND v.search_document @@ search.query
    UNION
    SELECT v.id
    FROM named_groups g
    JOIN rules r ON r.group_id = g.id
    JOIN rule_versions v ON v.rule_id = r.id AND v.html IS NOT NULL
    CROSS JOIN search
    WHERE g.names @@ search.query
)
SELECT l.owner, l.name, l.owner_avatar_url, r.path, g.path AS group_path, v.title::text AS title,
       v.impact::text AS impact, v.when_to_read::text AS when_to_read, v.major, v.minor, v.patch,
       count(*) OVER () AS total
FROM matched m
JOIN rule_versions v ON v.id = m.id
JOIN rules r ON r.id = v.rule_id
JOIN library_groups g ON g.id = r.group_id
JOIN named_groups ng ON ng.id = g.id
JOIN libraries l ON l.id = r.library_id
CROSS JOIN search
ORDER BY ts_rank(v.search_document || ng.names, search.query) DESC, lower(v.title), lower(l.owner), lower(l.name),
         r.path
LIMIT @max_results;
