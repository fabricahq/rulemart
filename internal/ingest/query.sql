-- name: UpsertLibrary :execrows
INSERT INTO libraries (github_id, owner, name, description, owner_avatar_url, license_expression, license_file)
VALUES (@github_id, @owner, @name, @description, @owner_avatar_url, @license_expression, @license_file)
ON CONFLICT (github_id) DO UPDATE SET
    owner = excluded.owner, name = excluded.name, description = excluded.description,
    owner_avatar_url = excluded.owner_avatar_url, license_expression = excluded.license_expression,
    license_file = excluded.license_file
WHERE (libraries.owner, libraries.name, libraries.description, libraries.owner_avatar_url,
       libraries.license_expression, libraries.license_file)
    IS DISTINCT FROM (excluded.owner, excluded.name, excluded.description, excluded.owner_avatar_url,
       excluded.license_expression, excluded.license_file);

-- name: UpsertRelease :execrows
INSERT INTO library_releases (library_id, number, commit_id, tagged_at)
VALUES (@library_id, @number, @commit_id, @tagged_at)
ON CONFLICT (library_id, number) DO UPDATE SET commit_id = excluded.commit_id, tagged_at = excluded.tagged_at
WHERE (library_releases.commit_id, library_releases.tagged_at) IS DISTINCT FROM (excluded.commit_id, excluded.tagged_at);

-- name: DeleteReleasesExcept :execrows
DELETE FROM library_releases WHERE library_id = @library_id AND NOT (number = ANY (@numbers::integer[]));

-- name: UpsertGroup :execrows
INSERT INTO library_groups (library_id, group_id, name, description, when_to_read)
VALUES (@library_id, @group_id, @name, @description, @when_to_read)
ON CONFLICT (library_id, group_id) DO UPDATE SET
    name = excluded.name, description = excluded.description, when_to_read = excluded.when_to_read
WHERE (library_groups.name, library_groups.description, library_groups.when_to_read)
    IS DISTINCT FROM (excluded.name, excluded.description, excluded.when_to_read);

-- name: DeleteGroupsExcept :execrows
DELETE FROM library_groups WHERE library_id = @library_id AND NOT (group_id = ANY (@group_ids::text[]));

-- name: UpsertRule :execrows
INSERT INTO rules (library_id, rule_id, group_id, retired_in, replaced_by)
VALUES (@library_id, @rule_id, @group_id, @retired_in, @replaced_by)
ON CONFLICT (library_id, rule_id) DO UPDATE SET
    group_id = excluded.group_id, retired_in = excluded.retired_in, replaced_by = excluded.replaced_by
WHERE (rules.group_id, rules.retired_in, rules.replaced_by)
    IS DISTINCT FROM (excluded.group_id, excluded.retired_in, excluded.replaced_by);

-- name: DeleteRulesExcept :execrows
DELETE FROM rules WHERE library_id = @library_id AND NOT (rule_id = ANY (@rule_ids::text[]));

-- name: ListVersionKeys :many
SELECT rule_id, release, major, minor, patch FROM rule_versions WHERE library_id = @library_id;

-- name: DeleteVersion :execrows
DELETE FROM rule_versions WHERE library_id = @library_id AND rule_id = @rule_id AND release = @release;

-- name: UpsertVersion :execrows
INSERT INTO rule_versions (library_id, rule_id, major, minor, patch, release, change, summaries,
                           title, impact, impact_description, when_to_read, markdown, html)
VALUES (@library_id, @rule_id, @major, @minor, @patch, @release, @change, @summaries,
        @title, @impact, @impact_description, @when_to_read, @markdown, @html)
ON CONFLICT (library_id, rule_id, major, minor, patch) DO UPDATE SET
    change = excluded.change, summaries = excluded.summaries, title = excluded.title, impact = excluded.impact,
    impact_description = excluded.impact_description, when_to_read = excluded.when_to_read,
    markdown = excluded.markdown, html = excluded.html
WHERE (rule_versions.change, rule_versions.summaries, rule_versions.title, rule_versions.impact,
       rule_versions.impact_description, rule_versions.when_to_read, rule_versions.markdown, rule_versions.html)
    IS DISTINCT FROM (excluded.change, excluded.summaries, excluded.title, excluded.impact,
       excluded.impact_description, excluded.when_to_read, excluded.markdown, excluded.html);
