-- name: UpsertLibrary :execrows
INSERT INTO libraries (host, host_repository_id, owner, name, description, owner_avatar_url, license_expression,
                       license_file, clone_url)
VALUES (@host, @host_repository_id, @owner, @name, @description, @owner_avatar_url, @license_expression,
        @license_file, @clone_url)
ON CONFLICT (host, host_repository_id) DO UPDATE SET
    owner = excluded.owner, name = excluded.name, description = excluded.description,
    owner_avatar_url = excluded.owner_avatar_url, license_expression = excluded.license_expression,
    license_file = excluded.license_file, clone_url = excluded.clone_url
WHERE (libraries.owner, libraries.name, libraries.description, libraries.owner_avatar_url,
       libraries.license_expression, libraries.license_file, libraries.clone_url)
    IS DISTINCT FROM (excluded.owner, excluded.name, excluded.description, excluded.owner_avatar_url,
       excluded.license_expression, excluded.license_file, excluded.clone_url);

-- name: GetLibraryID :one
SELECT id FROM libraries WHERE host = @host AND host_repository_id = @host_repository_id;

-- name: UpsertRelease :execrows
INSERT INTO library_releases (library_id, number, tag_object_id, commit_id, tagged_at, updates_shared_files)
VALUES (@library_id, @number, @tag_object_id, @commit_id, @tagged_at, @updates_shared_files)
ON CONFLICT (library_id, number) DO UPDATE SET
    tag_object_id = excluded.tag_object_id, commit_id = excluded.commit_id, tagged_at = excluded.tagged_at,
    updates_shared_files = excluded.updates_shared_files
WHERE (library_releases.tag_object_id, library_releases.commit_id, library_releases.tagged_at,
       library_releases.updates_shared_files)
    IS DISTINCT FROM (excluded.tag_object_id, excluded.commit_id, excluded.tagged_at, excluded.updates_shared_files);

-- name: ListReleaseIDs :many
SELECT id, number FROM library_releases WHERE library_id = @library_id;

-- name: DeleteReleasesExcept :execrows
DELETE FROM library_releases WHERE library_id = @library_id AND NOT (number = ANY (@numbers::integer[]));

-- name: UpsertGroup :execrows
INSERT INTO library_groups (library_id, path, name, description, when_to_read)
VALUES (@library_id, @path, @name, @description, @when_to_read)
ON CONFLICT (library_id, path) DO UPDATE SET
    name = excluded.name, description = excluded.description, when_to_read = excluded.when_to_read
WHERE (library_groups.name, library_groups.description, library_groups.when_to_read)
    IS DISTINCT FROM (excluded.name, excluded.description, excluded.when_to_read);

-- name: ListGroupIDs :many
SELECT id, path FROM library_groups WHERE library_id = @library_id;

-- name: DeleteGroupsExcept :execrows
DELETE FROM library_groups WHERE library_id = @library_id AND NOT (path = ANY (@paths::text[]));

-- name: UpsertRule :execrows
INSERT INTO rules (library_id, group_id, path, retired_in_release_id, replaced_by, retirement_summaries)
VALUES (@library_id, @group_id, @path, @retired_in_release_id, @replaced_by, @retirement_summaries)
ON CONFLICT (library_id, path) DO UPDATE SET
    group_id = excluded.group_id, retired_in_release_id = excluded.retired_in_release_id,
    replaced_by = excluded.replaced_by, retirement_summaries = excluded.retirement_summaries
WHERE (rules.group_id, rules.retired_in_release_id, rules.replaced_by, rules.retirement_summaries)
    IS DISTINCT FROM (excluded.group_id, excluded.retired_in_release_id, excluded.replaced_by,
       excluded.retirement_summaries);

-- name: ListRuleIDs :many
SELECT id, path FROM rules WHERE library_id = @library_id;

-- name: DeleteRulesExcept :execrows
DELETE FROM rules WHERE library_id = @library_id AND NOT (path = ANY (@paths::text[]));

-- name: ListVersionKeys :many
SELECT v.id, r.path, v.major, v.minor, v.patch
FROM rule_versions v
JOIN rules r ON r.id = v.rule_id
WHERE r.library_id = @library_id;

-- name: DeleteVersion :execrows
DELETE FROM rule_versions WHERE id = @id;

-- name: UpsertVersion :execrows
INSERT INTO rule_versions (library_id, rule_id, release_id, major, minor, patch, change, summaries,
                           title, impact, impact_description, when_to_read, markdown, html)
VALUES (@library_id, @rule_id, @release_id, @major, @minor, @patch, @change, @summaries,
        @title, @impact, @impact_description, @when_to_read, @markdown, @html)
ON CONFLICT (rule_id, major, minor, patch) DO UPDATE SET
    release_id = excluded.release_id, change = excluded.change, summaries = excluded.summaries, title = excluded.title, impact = excluded.impact,
    impact_description = excluded.impact_description, when_to_read = excluded.when_to_read,
    markdown = excluded.markdown, html = excluded.html
WHERE (rule_versions.release_id, rule_versions.change, rule_versions.summaries, rule_versions.title, rule_versions.impact,
       rule_versions.impact_description, rule_versions.when_to_read, rule_versions.markdown, rule_versions.html)
    IS DISTINCT FROM (excluded.release_id, excluded.change, excluded.summaries, excluded.title, excluded.impact,
       excluded.impact_description, excluded.when_to_read, excluded.markdown, excluded.html);

-- name: GetCheckpoint :many
-- One row per stored release of the library, or one row with a NULL number when it has none. missing_content reports
-- whether a release that stored content only on current versions left any version without it.
SELECT l.clone_url, r.number, r.tag_object_id,
       EXISTS (SELECT FROM rule_versions v WHERE v.library_id = l.id AND v.markdown IS NULL) AS missing_content
FROM libraries l
LEFT JOIN library_releases r ON r.library_id = l.id
WHERE l.host = @host AND l.host_repository_id = @host_repository_id;
