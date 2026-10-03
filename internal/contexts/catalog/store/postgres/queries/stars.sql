-- Stars: the web function stars and unstars rules for accounts, lists the rules an account's stars count toward, and
-- counts each rule's stars for the pages that show it. A library is vetted when vetted, the release's vetted keys as
-- host:repository ID, holds its key.
--
-- A star stays on the rule it was given to. While the rule is current, it counts toward the rule; once the rule is
-- retired, toward the current rule its chain of replacements reaches, in the same library: the rule its retirement
-- named, then while that one is retired, the rule that replaced it, and so on, for at most max_replacements rules, as
-- pages follow the chain. A rule's line is the rule and every retired rule whose chain reaches it: the retired rules
-- whose retirement named it, the ones that named those, and so on. Each rule names at most one replacement, so a line
-- is a tree, and no rule in it repeats.

-- StarRule stars the current rule at path, matched without regard to case and preferring the rule spelled exactly so,
-- in the library owner/name that vetted holds, matched without regard to case, for the account, and returns the
-- rule's id, and first, true when this star is the account's only one: it had none, and now has this one. It returns no
-- row when there's no such rule. A rule the account starred already keeps its star, and isn't first.
-- name: StarRule :one
WITH target AS (
    SELECT r.id FROM rules r JOIN libraries l ON l.id = r.library_id
    WHERE l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
      AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
      AND lower(r.path) = lower(@path) AND r.retired_in_release_id IS NULL
    ORDER BY r.path = @path DESC, r.path
    LIMIT 1
),
starred AS (
    INSERT INTO rule_stars (account_id, rule_id) SELECT @account_id::bigint, t.id FROM target t
    ON CONFLICT (account_id, rule_id) DO NOTHING
    RETURNING rule_id
)
-- The statement's own reads see rule_stars as it was before the insert.
SELECT t.id,
    (EXISTS (SELECT 1 FROM starred)
        AND NOT EXISTS (SELECT 1 FROM rule_stars s WHERE s.account_id = @account_id::bigint))::boolean AS first
FROM target t;

-- UnstarRule removes the account's stars from the line of the current rule StarRule finds, so none of them counts
-- toward it any more, and returns how many rules it found: none when there's no such rule.
-- name: UnstarRule :one
WITH RECURSIVE target AS (
    SELECT r.id, r.library_id, r.path FROM rules r JOIN libraries l ON l.id = r.library_id
    WHERE l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
      AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
      AND lower(r.path) = lower(@path) AND r.retired_in_release_id IS NULL
    ORDER BY r.path = @path DESC, r.path
    LIMIT 1
),
line AS (
    SELECT t.id, t.library_id, t.path, 0 AS steps FROM target t
    UNION ALL
    SELECT p.id, p.library_id, p.path, line.steps + 1
    FROM line JOIN rules p ON p.library_id = line.library_id AND p.replaced_by = line.path
    WHERE line.steps < @max_replacements::integer
),
unstarred AS (
    DELETE FROM rule_stars s USING line WHERE s.rule_id = line.id AND s.account_id = @account_id::bigint
)
SELECT count(*) FROM target;

-- IsRuleStarred reports whether one of the account's stars counts toward the current rule at path in the library
-- owner/name, both matched as StarRule matches them: whether it starred a rule of the rule's line.
-- name: IsRuleStarred :one
WITH RECURSIVE target AS (
    SELECT r.id, r.library_id, r.path FROM rules r JOIN libraries l ON l.id = r.library_id
    WHERE l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name)
      AND lower(r.path) = lower(@path) AND r.retired_in_release_id IS NULL
    ORDER BY r.path = @path DESC, r.path
    LIMIT 1
),
line AS (
    SELECT t.id, t.library_id, t.path, 0 AS steps FROM target t
    UNION ALL
    SELECT p.id, p.library_id, p.path, line.steps + 1
    FROM line JOIN rules p ON p.library_id = line.library_id AND p.replaced_by = line.path
    WHERE line.steps < @max_replacements::integer
)
SELECT EXISTS (
    SELECT 1 FROM rule_stars s JOIN line ON line.id = s.rule_id WHERE s.account_id = @account_id::bigint
)::boolean AS starred;

-- CountRuleStars counts, for each of the rules rule_ids names, the accounts whose stars count toward it: those that
-- starred a rule of its line. It returns a row only for a rule with a star.
-- name: CountRuleStars :many
WITH RECURSIVE line AS (
    SELECT r.id AS rule_id, r.id, r.library_id, r.path, 0 AS steps FROM rules r WHERE r.id = ANY (@rule_ids::bigint[])
    UNION ALL
    SELECT line.rule_id, p.id, p.library_id, p.path, line.steps + 1
    FROM line JOIN rules p ON p.library_id = line.library_id AND p.replaced_by = line.path
    WHERE line.steps < @max_replacements::integer
)
SELECT line.rule_id::bigint AS rule_id, count(DISTINCT s.account_id) AS stars
FROM line JOIN rule_stars s ON s.rule_id = line.id
GROUP BY line.rule_id;

-- ListAccountRuleStars returns each current rule of a library vetted holds that the account's stars count toward,
-- once, most recently starred first: when the account last starred it or a rule of its line. starred_as is the ID of
-- the retired rule of its line the account starred most recently, or empty when the account starred the rule itself.
-- A star that counts toward no such rule, such as one on a rule retired without a replacement, isn't listed.
-- name: ListAccountRuleStars :many
WITH RECURSIVE heirs AS (
    SELECT s.rule_id AS starred_id, s.created_at, r.id, r.library_id, r.replaced_by, r.retired_in_release_id, 0 AS steps
    FROM rule_stars s JOIN rules r ON r.id = s.rule_id
    WHERE s.account_id = @account_id::bigint
    UNION ALL
    SELECT h.starred_id, h.created_at, n.id, n.library_id, n.replaced_by, n.retired_in_release_id, h.steps + 1
    FROM heirs h JOIN rules n ON n.library_id = h.library_id AND n.path = h.replaced_by
    WHERE h.retired_in_release_id IS NOT NULL AND h.steps < @max_replacements::integer
),
counted AS (
    SELECT h.id AS rule_id, max(h.created_at) AS starred_at, bool_or(h.starred_id = h.id) AS direct,
           (array_agg(h.starred_id ORDER BY h.created_at DESC, h.starred_id))[1] AS last_starred_id
    FROM heirs h
    WHERE h.retired_in_release_id IS NULL
    GROUP BY h.id
)
SELECT l.owner, l.name, l.owner_avatar_url, r.id, r.path, g.path AS group_path, v.title::text AS title,
       v.impact::text AS impact, v.major, v.minor, v.patch,
       (CASE WHEN c.direct THEN '' ELSE starred.path END)::text AS starred_as, c.starred_at::timestamptz AS starred_at
FROM counted c
JOIN rules r ON r.id = c.rule_id
JOIN rules starred ON starred.id = c.last_starred_id
JOIN library_groups g ON g.id = r.group_id
JOIN libraries l ON l.id = r.library_id
JOIN rule_versions v ON v.rule_id = r.id AND v.html IS NOT NULL
WHERE l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
ORDER BY c.starred_at DESC, lower(l.owner), lower(l.name), r.path;

-- ListUncountedRuleStars returns the account's stars that count toward no current rule of a library vetted holds, most
-- recently starred first: a star on a rule retired without a replacement, or whose chain of replacements passes
-- max_replacements, or on a rule of a library that isn't vetted now. Each comes with its rule's library, whether vetted
-- holds it and whether a listing names it, its ID, its newest version's title, empty when the catalog has none, and
-- whether it's retired, with its replacement.
-- name: ListUncountedRuleStars :many
WITH RECURSIVE heirs AS (
    SELECT s.rule_id AS starred_id, r.id, r.library_id, r.replaced_by, r.retired_in_release_id, 0 AS steps
    FROM rule_stars s JOIN rules r ON r.id = s.rule_id
    WHERE s.account_id = @account_id::bigint
    UNION ALL
    SELECT h.starred_id, n.id, n.library_id, n.replaced_by, n.retired_in_release_id, h.steps + 1
    FROM heirs h JOIN rules n ON n.library_id = h.library_id AND n.path = h.replaced_by
    WHERE h.retired_in_release_id IS NOT NULL AND h.steps < @max_replacements::integer
),
counted AS (
    SELECT DISTINCT h.starred_id
    FROM heirs h JOIN libraries l ON l.id = h.library_id
    WHERE h.retired_in_release_id IS NULL AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
)
SELECT l.owner, l.name, l.owner_avatar_url,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
       EXISTS (SELECT 1 FROM listings ls WHERE ls.host = l.host AND ls.host_repository_id = l.host_repository_id)::boolean AS listed,
       r.path, (r.retired_in_release_id IS NOT NULL)::boolean AS retired, coalesce(r.replaced_by, '')::text AS replaced_by,
       coalesce((
           SELECT v.title FROM rule_versions v WHERE v.rule_id = r.id AND v.title IS NOT NULL
           ORDER BY v.major DESC, v.minor DESC, v.patch DESC LIMIT 1
       ), '')::text AS title,
       s.created_at::timestamptz AS starred_at
FROM rule_stars s
JOIN rules r ON r.id = s.rule_id
JOIN libraries l ON l.id = r.library_id
WHERE s.account_id = @account_id::bigint AND s.rule_id NOT IN (SELECT starred_id FROM counted)
ORDER BY s.created_at DESC, lower(l.owner), lower(l.name), r.path;

-- RemoveRuleStar removes the account's star on the rule at path in the library owner/name, current or retired, vetted
-- or not, both matched without regard to case, and returns how many it removed.
-- name: RemoveRuleStar :execrows
DELETE FROM rule_stars s
USING rules r, libraries l
WHERE s.rule_id = r.id AND r.library_id = l.id AND s.account_id = @account_id::bigint
  AND l.host = @host AND lower(l.owner) = lower(@owner) AND lower(l.name) = lower(@name) AND lower(r.path) = lower(@path);
