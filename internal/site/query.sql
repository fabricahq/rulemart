-- name: ListLibraries :many
SELECT l.github_id, l.owner, l.name, l.description, l.owner_avatar_url,
       (SELECT count(*) FROM rules r WHERE r.library_id = l.github_id AND r.retired_in IS NULL) AS rule_count
FROM libraries l
WHERE l.github_id = ANY (@vetted::bigint[])
ORDER BY lower(l.owner), lower(l.name);

-- name: GetLibrary :one
SELECT l.github_id, l.owner, l.name, l.description, l.owner_avatar_url, l.license_expression, l.license_file,
       latest.number AS latest_release, latest.tagged_at AS latest_tagged_at
FROM libraries l
JOIN LATERAL (
    SELECT number, tagged_at FROM library_releases WHERE library_id = l.github_id ORDER BY number DESC LIMIT 1
) latest ON true
WHERE lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name) AND l.github_id = ANY (@vetted::bigint[]);

-- name: ListGroups :many
SELECT g.group_id, g.name, g.description, g.when_to_read,
       (SELECT count(*) FROM rules r
        WHERE r.library_id = g.library_id AND r.group_id = g.group_id AND r.retired_in IS NULL) AS rule_count
FROM library_groups g
WHERE g.library_id = @library_id
ORDER BY g.group_id;

-- name: ListCurrentRules :many
SELECT v.rule_id, r.group_id, v.title::text AS title, v.impact::text AS impact, v.major, v.minor, v.patch
FROM rule_versions v
JOIN rules r ON r.library_id = v.library_id AND r.rule_id = v.rule_id
WHERE v.library_id = @library_id AND v.html IS NOT NULL
ORDER BY r.group_id, lower(v.title), v.rule_id;

-- name: GetRule :one
SELECT v.rule_id, r.group_id, g.name AS group_name, v.title::text AS title, v.impact::text AS impact,
       v.when_to_read::text AS when_to_read, v.html::text AS html, v.major, v.minor, v.patch, v.release,
       published.tagged_at AS published_at
FROM rule_versions v
JOIN rules r ON r.library_id = v.library_id AND r.rule_id = v.rule_id
JOIN library_groups g ON g.library_id = r.library_id AND g.group_id = r.group_id
JOIN library_releases published ON published.library_id = v.library_id AND published.number = v.release
WHERE v.library_id = @library_id AND v.rule_id = @rule_id AND v.html IS NOT NULL;

-- name: ListVersions :many
SELECT v.major, v.minor, v.patch, v.release, v.change, v.summaries, published.tagged_at AS published_at
FROM rule_versions v
JOIN library_releases published ON published.library_id = v.library_id AND published.number = v.release
WHERE v.library_id = @library_id AND v.rule_id = @rule_id
ORDER BY v.release DESC;
