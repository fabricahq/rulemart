-- ListLibraries returns the libraries vetted holds, and with include_unvetted, the ones a listing names too, each
-- with whether vetted holds it and how many current rules it holds.
-- name: ListLibraries :many
SELECT l.id, l.owner, l.name, l.description, l.owner_avatar_url,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
       (SELECT count(*) FROM rules r WHERE r.library_id = l.id AND r.retired_in_release_id IS NULL) AS rule_count
FROM libraries l
WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
   OR (@include_unvetted::boolean
       AND EXISTS (SELECT 1 FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id))
ORDER BY lower(l.owner), lower(l.name);

-- ListOwnerLibraries returns the libraries vetted holds whose owner is login, matched without regard to case, as
-- ListLibraries returns them.
-- name: ListOwnerLibraries :many
SELECT l.id, l.owner, l.name, l.description, l.owner_avatar_url,
       (SELECT count(*) FROM rules r WHERE r.library_id = l.id AND r.retired_in_release_id IS NULL) AS rule_count
FROM libraries l
WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[]) AND lower(l.owner) = lower(@login)
ORDER BY lower(l.owner), lower(l.name);

-- ListUnvettedLibraries returns the libraries a listing names that vetted doesn't hold, as ListLibraries returns the
-- vetted ones.
-- name: ListUnvettedLibraries :many
SELECT l.id, l.owner, l.name, l.description, l.owner_avatar_url,
       (SELECT count(*) FROM rules r WHERE r.library_id = l.id AND r.retired_in_release_id IS NULL) AS rule_count
FROM libraries l
WHERE NOT l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
  AND EXISTS (SELECT 1 FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id)
ORDER BY lower(l.owner), lower(l.name);

-- GetLibrary returns the library owner/name that vetted holds or a listing names, with whether vetted holds it, its
-- latest release, how many current rules it holds and in how many groups, and when it came to Rulemart: when its
-- listing was made, with the login of the account that made it, or when it was first ingested, without a listing.
-- name: GetLibrary :one
SELECT l.id, l.owner, l.name, l.description, l.owner_avatar_url, l.license_expression, l.license_file,
       latest.number AS latest_release, latest.tagged_at AS latest_tagged_at, current.rule_count, current.group_count,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
       coalesce(listing.github_login, '')::text AS added_by, coalesce(listing.created_at, l.created_at)::timestamptz AS added_at
FROM libraries l
LEFT JOIN LATERAL (
    SELECT a.github_login, s.created_at
    FROM listings s JOIN accounts a ON a.id = s.account_id
    WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id
) listing ON true
JOIN LATERAL (
    SELECT number, tagged_at FROM library_releases WHERE library_id = l.id ORDER BY number DESC LIMIT 1
) latest ON true
JOIN LATERAL (
    SELECT count(*) AS rule_count, count(DISTINCT group_id) AS group_count
    FROM rules WHERE library_id = l.id AND retired_in_release_id IS NULL
) current ON true
WHERE l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
  AND (
      l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
      OR EXISTS (SELECT 1 FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id)
  );

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
SELECT r.id, r.path, g.path AS group_path, v.title::text AS title, v.impact::text AS impact, v.major, v.minor, v.patch
FROM rules r
JOIN library_groups g ON g.id = r.group_id
JOIN rule_versions v ON v.rule_id = r.id AND v.html IS NOT NULL
WHERE r.library_id = @library_id
ORDER BY g.path, lower(v.title), r.path;

-- GetRule returns the rule at path, current or retired, matched without regard to case, preferring the rule spelled
-- exactly so, with its newest version: the current version while it's current, and the last once retired, and a
-- retired rule's retirement. Its html is the newest version's body. when_to_read_html is empty unless it was rendered
-- from the reading guidance the version holds now, since a release that didn't render it may have changed it since.
-- name: GetRule :one
SELECT r.id, r.path, g.path AS group_path, v.title, v.impact, v.when_to_read, v.when_to_read_html, v.html, v.major,
       v.minor, v.patch, v.release, v.published_at, coalesce(v.tags, '{}')::text[] AS tags, retired.number AS retired_in,
       retired.tagged_at AS retired_at, r.retirement_summaries
FROM rules r
JOIN library_groups g ON g.id = r.group_id
JOIN LATERAL (
    SELECT v.title, v.impact, v.when_to_read,
           coalesce(CASE WHEN v.rendered_when_to_read = v.when_to_read THEN v.when_to_read_html END, '')::text
               AS when_to_read_html,
           coalesce(v.html, v.retired_html) AS html, v.major, v.minor, v.patch, p.number AS release,
           p.tagged_at AS published_at, v.tags
    FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
    WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
) v ON true
LEFT JOIN library_releases retired ON retired.id = r.retired_in_release_id
WHERE r.library_id = @library_id AND lower(r.path) = lower(@path)
ORDER BY r.path = @path DESC, r.path
LIMIT 1;

-- GetRulePath returns the library's spelling of its rule at path, current or retired, matched as GetRule matches it.
-- name: GetRulePath :one
SELECT r.path
FROM rules r
WHERE r.library_id = @library_id AND lower(r.path) = lower(@path)
ORDER BY r.path = @path DESC, r.path
LIMIT 1;

-- ListRuleLinks returns how every rule of the library was replaced: its retirement and replacement, the release that
-- added it with its first title, and its last title, in path order.
-- name: ListRuleLinks :many
SELECT r.path, retired.number AS retired_in, r.replaced_by, first.release AS first_release, first.title AS first_title,
       last.title AS last_title
FROM rules r
LEFT JOIN library_releases retired ON retired.id = r.retired_in_release_id
JOIN LATERAL (
    SELECT p.number AS release, v.title FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
    WHERE v.rule_id = r.id ORDER BY p.number LIMIT 1
) first ON true
JOIN LATERAL (
    SELECT v.title FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
    WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
) last ON true
WHERE r.library_id = @library_id
ORDER BY r.path COLLATE "C";

-- ListVersions returns each version of a rule, newest first, with whether it has its Markdown, which a release that
-- stored only current versions' content left out, and how many bytes it holds.
-- name: ListVersions :many
SELECT v.id, v.major, v.minor, v.patch, published.number AS release, v.change, v.summaries,
       published.tagged_at AS published_at, (v.markdown IS NOT NULL)::boolean AS has_markdown, coalesce(octet_length(v.markdown), 0)::integer AS markdown_bytes
FROM rule_versions v
JOIN library_releases published ON published.id = v.release_id
WHERE v.rule_id = @rule_id
ORDER BY published.number DESC;

-- ListRetiredRules returns the library's retired rules, in path order, each with its group, and its last version and
-- that version's title and impact.
-- name: ListRetiredRules :many
SELECT r.path, g.path AS group_path, retired.number AS retired_in, r.replaced_by, last.title, last.impact, last.major,
       last.minor, last.patch
FROM rules r
JOIN library_groups g ON g.id = r.group_id
JOIN library_releases retired ON retired.id = r.retired_in_release_id
JOIN LATERAL (
    SELECT v.title, v.impact, v.major, v.minor, v.patch FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
    WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
) last ON true
WHERE r.library_id = @library_id
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

-- ListLibraryGroups returns each group that holds current rules in a library vetted holds, or with include_unvetted,
-- in one a listing names too, with the library, whether vetted holds it, and how many of its current rules the group
-- holds, in group path order and then the library's owner and name.
-- name: ListLibraryGroups :many
SELECT g.path, l.owner, l.name, l.owner_avatar_url,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted, count(*) AS rule_count
FROM library_groups g
JOIN libraries l ON l.id = g.library_id
JOIN rules r ON r.group_id = g.id AND r.retired_in_release_id IS NULL
WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
   OR (@include_unvetted::boolean
       AND EXISTS (SELECT 1 FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id))
GROUP BY g.path, l.id
ORDER BY g.path, lower(l.owner), lower(l.name);

-- ListRuleAssets returns the assets the rule's page lists, each with the library release its copy is from, and whether
-- the catalog keeps its bytes, in path order.
-- name: ListRuleAssets :many
SELECT a.path, a.size, a.media_type, (a.content IS NOT NULL)::boolean AS kept, p.number AS release
FROM rule_assets ra
JOIN assets a ON a.id = ra.asset_id
JOIN library_releases p ON p.id = a.release_id
WHERE ra.rule_id = @rule_id
ORDER BY a.path COLLATE "C";

-- GetAssetHTML returns how a page shows the library's asset at path: empty unless it's Markdown or text the catalog
-- keeps.
-- name: GetAssetHTML :one
SELECT coalesce(html, '')::text AS html FROM assets WHERE library_id = @library_id AND path = @path;

-- GetAssetContent returns the media type and bytes of the library's asset at path, when the catalog keeps them.
-- name: GetAssetContent :one
SELECT a.media_type, a.content::bytea AS content
FROM assets a
WHERE a.library_id = @library_id AND a.path = @path AND a.content IS NOT NULL;

-- CountRulesListingAsset returns how many of the library's rules list its asset at path on their pages.
-- name: CountRulesListingAsset :one
SELECT count(*)
FROM rule_assets ra
JOIN assets a ON a.id = ra.asset_id
WHERE a.library_id = @library_id AND a.path = @path;

-- FirstRuleListingAsset returns the path of the first rule, in path order, whose page lists the library's asset at path.
-- name: FirstRuleListingAsset :one
SELECT r.path
FROM rule_assets ra
JOIN assets a ON a.id = ra.asset_id
JOIN rules r ON r.id = ra.rule_id
WHERE a.library_id = @library_id AND a.path = @path
ORDER BY r.path COLLATE "C"
LIMIT 1;
