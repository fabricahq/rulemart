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

-- UpsertVersion stores a version; rendered_when_to_read records the reading guidance when_to_read_html was rendered
-- from, which is when_to_read whenever this release writes both.
-- name: UpsertVersion :execrows
INSERT INTO rule_versions (library_id, rule_id, release_id, major, minor, patch, change, summaries,
                           title, impact, impact_description, when_to_read, tags, markdown, html,
                           when_to_read_html, rendered_when_to_read, retired_html)
VALUES (@library_id, @rule_id, @release_id, @major, @minor, @patch, @change, @summaries,
        @title, @impact, @impact_description, @when_to_read, @tags, @markdown, @html,
        @when_to_read_html, @rendered_when_to_read, @retired_html)
ON CONFLICT (rule_id, major, minor, patch) DO UPDATE SET
    release_id = excluded.release_id, change = excluded.change, summaries = excluded.summaries, title = excluded.title, impact = excluded.impact,
    impact_description = excluded.impact_description, when_to_read = excluded.when_to_read, tags = excluded.tags,
    markdown = excluded.markdown, html = excluded.html, when_to_read_html = excluded.when_to_read_html,
    rendered_when_to_read = excluded.rendered_when_to_read, retired_html = excluded.retired_html
WHERE (rule_versions.release_id, rule_versions.change, rule_versions.summaries, rule_versions.title, rule_versions.impact,
       rule_versions.impact_description, rule_versions.when_to_read, rule_versions.tags, rule_versions.markdown,
       rule_versions.html, rule_versions.when_to_read_html, rule_versions.rendered_when_to_read, rule_versions.retired_html)
    IS DISTINCT FROM (excluded.release_id, excluded.change, excluded.summaries, excluded.title, excluded.impact,
       excluded.impact_description, excluded.when_to_read, excluded.tags, excluded.markdown, excluded.html,
       excluded.when_to_read_html, excluded.rendered_when_to_read, excluded.retired_html);

-- name: UpsertAsset :execrows
INSERT INTO assets (library_id, path, release_id, size, media_type, content, html)
VALUES (@library_id, @path, @release_id, @size, @media_type, @content, @html)
ON CONFLICT (library_id, path) DO UPDATE SET
    release_id = excluded.release_id, size = excluded.size, media_type = excluded.media_type, content = excluded.content,
    html = excluded.html
WHERE (assets.release_id, assets.size, assets.media_type, assets.content, assets.html)
    IS DISTINCT FROM (excluded.release_id, excluded.size, excluded.media_type, excluded.content, excluded.html);

-- name: ListAssetIDs :many
SELECT id, path FROM assets WHERE library_id = @library_id;

-- DeleteRuleAssetsExcept deletes which assets the library's rules list, except the pairs of a rule and an asset that
-- rule_ids and asset_ids hold at the same positions.
-- name: DeleteRuleAssetsExcept :execrows
DELETE FROM rule_assets ra
WHERE ra.library_id = @library_id
  AND (ra.rule_id, ra.asset_id) NOT IN (
      SELECT r.rule_id, a.asset_id
      FROM unnest(@rule_ids::bigint[]) WITH ORDINALITY AS r (rule_id, n)
      JOIN unnest(@asset_ids::bigint[]) WITH ORDINALITY AS a (asset_id, n) ON a.n = r.n
  );

-- InsertRuleAssets records that the rules rule_ids list the assets asset_ids, at the same positions, unless they do.
-- name: InsertRuleAssets :execrows
INSERT INTO rule_assets (library_id, rule_id, asset_id)
SELECT @library_id, r.rule_id, a.asset_id
FROM unnest(@rule_ids::bigint[]) WITH ORDINALITY AS r (rule_id, n)
JOIN unnest(@asset_ids::bigint[]) WITH ORDINALITY AS a (asset_id, n) ON a.n = r.n
ON CONFLICT DO NOTHING;

-- name: DeleteAssetsExcept :execrows
DELETE FROM assets WHERE library_id = @library_id AND NOT (path = ANY (@paths::text[]));

-- name: GetCheckpoint :many
-- One row per stored release of the library, or one row with a NULL number when it has none, each saying whether a
-- release that stored content only on current versions left any version without it, or a retired rule's last version
-- without its body's HTML, whether a current version's reading guidance lacks the HTML rendered from it, and whether a
-- release that read neither tags nor assets left a version with content without its tags.
SELECT l.clone_url, r.number, r.tag_object_id,
       (EXISTS (SELECT FROM rule_versions v WHERE v.library_id = l.id AND v.markdown IS NULL) OR EXISTS (
           SELECT FROM rules retired
           JOIN LATERAL (
               SELECT v.retired_html FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
               WHERE v.rule_id = retired.id ORDER BY p.number DESC LIMIT 1
           ) last ON true
           WHERE retired.library_id = l.id AND retired.retired_in_release_id IS NOT NULL AND last.retired_html IS NULL
       ))::boolean AS missing_content,
       EXISTS (
           SELECT FROM rule_versions v
           WHERE v.library_id = l.id AND v.html IS NOT NULL AND v.rendered_when_to_read IS DISTINCT FROM v.when_to_read
       ) AS unrendered,
       EXISTS (
           SELECT FROM rule_versions v WHERE v.library_id = l.id AND v.markdown IS NOT NULL AND v.tags IS NULL
       ) AS missing_assets
FROM libraries l
LEFT JOIN library_releases r ON r.library_id = l.id
WHERE l.host = @host AND l.host_repository_id = @host_repository_id;
