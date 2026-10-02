-- name: ListLibraries :many
SELECT l.id, l.owner, l.name, l.description, l.owner_avatar_url,
       (SELECT count(*) FROM rules r WHERE r.library_id = l.id AND r.retired_in_release_id IS NULL) AS rule_count
FROM libraries l
WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
ORDER BY lower(l.owner), lower(l.name);

-- GetLibrary returns the vetted library owner/name, with its latest release, and how many current rules it holds and
-- in how many groups.
-- name: GetLibrary :one
SELECT l.id, l.owner, l.name, l.description, l.owner_avatar_url, l.license_expression, l.license_file,
       latest.number AS latest_release, latest.tagged_at AS latest_tagged_at, current.rule_count, current.group_count
FROM libraries l
JOIN LATERAL (
    SELECT number, tagged_at FROM library_releases WHERE library_id = l.id ORDER BY number DESC LIMIT 1
) latest ON true
JOIN LATERAL (
    SELECT count(*) AS rule_count, count(DISTINCT group_id) AS group_count
    FROM rules WHERE library_id = l.id AND retired_in_release_id IS NULL
) current ON true
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

-- GetRule returns the rule at path, current or retired, with its newest version: the current version while it's
-- current, and the last once retired. A retired rule also has its retirement, and the newest title of the rule that
-- replaced it, when the retirement named one.
-- name: GetRule :one
SELECT r.id, r.path, g.path AS group_path, v.title, v.impact, v.when_to_read, v.html, v.major, v.minor, v.patch,
       v.release, v.published_at, retired.number AS retired_in, retired.tagged_at AS retired_at,
       r.retirement_summaries, r.replaced_by, replacement.title AS replacement_title,
       replacement.retired_in AS replacement_retired_in
FROM rules r
JOIN library_groups g ON g.id = r.group_id
JOIN LATERAL (
    SELECT v.title, v.impact, v.when_to_read, v.html, v.major, v.minor, v.patch, p.number AS release,
           p.tagged_at AS published_at
    FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
    WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
) v ON true
LEFT JOIN library_releases retired ON retired.id = r.retired_in_release_id
LEFT JOIN LATERAL (
    SELECT newest.title, replaced_retired.number AS retired_in
    FROM rules other
    JOIN LATERAL (
        SELECT v.title FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
        WHERE v.rule_id = other.id ORDER BY p.number DESC LIMIT 1
    ) newest ON true
    LEFT JOIN library_releases replaced_retired ON replaced_retired.id = other.retired_in_release_id
    WHERE other.library_id = r.library_id AND other.path = r.replaced_by
) replacement ON true
WHERE r.library_id = @library_id AND r.path = @path;

-- ListVersions returns each version of a rule, newest first, with whether it has its Markdown, which a release that
-- stored only current versions' content left out, and how many bytes it holds.
-- name: ListVersions :many
SELECT v.id, v.major, v.minor, v.patch, published.number AS release, v.change, v.summaries,
       published.tagged_at AS published_at, (v.markdown IS NOT NULL)::boolean AS has_markdown, coalesce(octet_length(v.markdown), 0)::integer AS markdown_bytes
FROM rule_versions v
JOIN library_releases published ON published.id = v.release_id
WHERE v.rule_id = @rule_id
ORDER BY published.number DESC;

-- ListRetiredRules returns the library's retired rules, in path order, each with its last version and that version's
-- title.
-- name: ListRetiredRules :many
SELECT r.path, retired.number AS retired_in, r.replaced_by, last.title, last.major, last.minor, last.patch
FROM rules r
JOIN library_releases retired ON retired.id = r.retired_in_release_id
JOIN LATERAL (
    SELECT v.title, v.major, v.minor, v.patch FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
    WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
) last ON true
WHERE r.library_id = @library_id
ORDER BY r.path COLLATE "C";

-- ListReplacedRules returns the retired rules whose retirement named the rule at path as their replacement, in path
-- order, each with its last version's title.
-- name: ListReplacedRules :many
SELECT r.path, retired.number AS retired_in, last.title
FROM rules r
JOIN library_releases retired ON retired.id = r.retired_in_release_id
JOIN LATERAL (
    SELECT v.title FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
    WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
) last ON true
WHERE r.library_id = @library_id AND r.replaced_by = @path::text
ORDER BY r.path COLLATE "C";

-- name: ListReleases :many
SELECT number, tagged_at, updates_shared_files FROM library_releases WHERE library_id = @library_id ORDER BY number;

-- ListRuleHistories returns every version of every rule in the library, current or retired: one row per version, by
-- rule path, in code point order as Code Rules' release notes list rules, and then release, with the rule's retirement, whether the version has its Markdown, which a release that
-- stored only current versions' content left out, and how many bytes it holds.
-- name: ListRuleHistories :many
SELECT r.path, retired.number AS retired_in, r.replaced_by, r.retirement_summaries, v.id, v.major, v.minor, v.patch,
       published.number AS release, published.tagged_at AS published_at, v.change, v.summaries, v.title,
       (v.markdown IS NOT NULL)::boolean AS has_markdown, coalesce(octet_length(v.markdown), 0)::integer AS markdown_bytes
FROM rules r
JOIN rule_versions v ON v.rule_id = r.id
JOIN library_releases published ON published.id = v.release_id
LEFT JOIN library_releases retired ON retired.id = r.retired_in_release_id
WHERE r.library_id = @library_id
ORDER BY r.path COLLATE "C", published.number;

-- ListMarkdown returns the Markdown of the rule versions ids names that have it.
-- name: ListMarkdown :many
SELECT id, markdown::text AS markdown FROM rule_versions WHERE id = ANY (@ids::bigint[]) AND markdown IS NOT NULL;

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
-- text. A rule matches through its search document and its group's names together, so one word can match its text and
-- another its group: a canonical group's name on the list, whose IDs and names canonical_ids and canonical_names hold
-- in step, and the name part of any group's ID, but never what its library calls the group. Equal ranks keep a stable
-- order: by title, the library's owner and name, then rule ID.
-- name: SearchRules :many
WITH search AS (
    SELECT websearch_to_tsquery('english', @query::text) AS query
),
documents AS (
    SELECT v.id, v.search_document || setweight(to_tsvector('english',
               coalesce((@canonical_names::text[])[array_position(@canonical_ids::text[], g.path)], '') || ' ' ||
               split_part(g.path, '/', 2)), 'B') AS document
    FROM rule_versions v
    JOIN rules r ON r.id = v.rule_id
    JOIN library_groups g ON g.id = r.group_id
    JOIN libraries l ON l.id = r.library_id
    WHERE v.html IS NOT NULL AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
)
SELECT l.owner, l.name, l.owner_avatar_url, r.path, g.path AS group_path, v.title::text AS title,
       v.impact::text AS impact, v.when_to_read::text AS when_to_read, v.major, v.minor, v.patch,
       count(*) OVER () AS total
FROM documents d
CROSS JOIN search
JOIN rule_versions v ON v.id = d.id
JOIN rules r ON r.id = v.rule_id
JOIN library_groups g ON g.id = r.group_id
JOIN libraries l ON l.id = r.library_id
WHERE d.document @@ search.query
ORDER BY ts_rank(d.document, search.query) DESC, lower(v.title), lower(l.owner), lower(l.name), r.path
LIMIT @max_results;
