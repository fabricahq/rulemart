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
-- latest release, and how many current rules it holds and in how many groups.
-- name: GetLibrary :one
SELECT l.id, l.owner, l.name, l.description, l.owner_avatar_url, l.license_expression, l.license_file,
       latest.number AS latest_release, latest.tagged_at AS latest_tagged_at, current.rule_count, current.group_count,
       (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted
FROM libraries l
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
       v.minor, v.patch, v.release, v.published_at, retired.number AS retired_in, retired.tagged_at AS retired_at,
       r.retirement_summaries
FROM rules r
JOIN library_groups g ON g.id = r.group_id
JOIN LATERAL (
    SELECT v.title, v.impact, v.when_to_read,
           coalesce(CASE WHEN v.rendered_when_to_read = v.when_to_read THEN v.when_to_read_html END, '')::text
               AS when_to_read_html,
           coalesce(v.html, v.retired_html) AS html, v.major, v.minor, v.patch, p.number AS release,
           p.tagged_at AS published_at
    FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
    WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
) v ON true
LEFT JOIN library_releases retired ON retired.id = r.retired_in_release_id
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

-- ListGroupRules returns the current rules of the group at path in every vetted library that has it, in the library's
-- owner and name order, then title order.
-- name: ListGroupRules :many
SELECT l.owner, l.name, l.owner_avatar_url, r.id, r.path, v.title::text AS title, v.impact::text AS impact,
       v.major, v.minor, v.patch
FROM library_groups g
JOIN libraries l ON l.id = g.library_id
JOIN rules r ON r.group_id = g.id
JOIN rule_versions v ON v.rule_id = r.id AND v.html IS NOT NULL
WHERE g.path = @path AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
ORDER BY lower(l.owner), lower(l.name), lower(v.title), r.path;

-- SearchRules returns the vetted libraries' current rules that hold at least one of the find terms and none of the
-- exclude terms, best first, skipping skip of them and returning at most max_results, each with how many matched in
-- all and how many of those hold every find term, and with its reading guidance's HTML as GetRule returns it. Each term is in websearch_to_tsquery's syntax, which accepts any
-- text, and a term of only stop words, such as "the", is ignored.
--
-- A term matches a rule by its text, its library's owner and name, or its group's names: a canonical group's name on
-- the list, whose IDs and names canonical_ids and canonical_names hold in step, and the name part of any group's ID,
-- but never what its library calls the group. A term's identifier query, the term's alternatives that joined words
-- with -, /, or :, such as keep tests independent from keep-tests-independent, also matches the words of the rule's
-- source-qualified ID, owner/name:rule-ID, whose rule ID starts with its group's; an empty one matches no ID.
--
-- Rules that hold every term come first, then those that hold some, each by score. Each term scores by the best
-- place it matches: the title 1, the group or the IDs 0.8, the reading guidance or impact description 0.5, and anywhere
-- else, the body or the library's name, 0.1. A rule's score is its terms' average, scaled by the square of the share
-- of terms it holds. A title made mostly of the terms it matches adds up to 0.25, so "Verify retry limits" outranks a
-- longer title for retry. Equal scores fall back to ts_rank, then title, the library's owner and name, and rule ID, so
-- the order is stable.
-- name: SearchRules :many
WITH find_terms AS (
    SELECT i AS ordinal, (@find_terms::text[])[i] AS query, (@find_identifier_terms::text[])[i] AS identifier_query
    FROM generate_subscripts(@find_terms::text[], 1) AS i
),
find AS (
    SELECT t.ordinal, websearch_to_tsquery('english', t.query) AS query,
           websearch_to_tsquery('english', t.identifier_query) AS identifier_query,
           tsvector_to_array(to_tsvector('english', t.query)) AS lexemes
    FROM find_terms t
    WHERE numnode(websearch_to_tsquery('english', t.query)) > 0
),
exclude_terms AS (
    SELECT (@exclude_terms::text[])[i] AS query, (@exclude_identifier_terms::text[])[i] AS identifier_query
    FROM generate_subscripts(@exclude_terms::text[], 1) AS i
),
exclude AS (
    SELECT websearch_to_tsquery('english', t.query) AS query,
           websearch_to_tsquery('english', t.identifier_query) AS identifier_query
    FROM exclude_terms t
    WHERE numnode(websearch_to_tsquery('english', t.query)) > 0
),
search AS (
    SELECT websearch_to_tsquery('english', array_to_string(@find_terms::text[], ' or ')) AS any_query,
           (SELECT count(*) FROM find)::float AS terms
),
documents AS (
    SELECT v.id,
           v.search_document || setweight(to_tsvector('english', l.owner || ' ' || l.name), 'D') AS text,
           ts_filter(v.search_document, '{a}') AS title,
           to_tsvector('english', coalesce((@canonical_names::text[])[array_position(@canonical_ids::text[], g.path)], '')
               || ' ' || split_part(g.path, '/', 2)) AS group_names,
           to_tsvector('english', translate(l.owner || ' ' || l.name || ' ' || r.path, '/-:', '   ')) AS identifiers
    FROM rule_versions v
    JOIN rules r ON r.id = v.rule_id
    JOIN library_groups g ON g.id = r.group_id
    JOIN libraries l ON l.id = r.library_id
    WHERE v.html IS NOT NULL AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
),
places AS (
    SELECT d.id, f.ordinal, f.lexemes,
           CASE WHEN d.title @@ f.query THEN 1.0
                WHEN d.group_names @@ f.query OR (numnode(f.identifier_query) > 0 AND d.identifiers @@ f.identifier_query) THEN 0.8
                WHEN ts_filter(d.text, '{b}') @@ f.query THEN 0.5
                WHEN d.text @@ f.query THEN 0.1
           END AS score
    FROM documents d
    CROSS JOIN find f
),
scored AS (
    SELECT d.id, d.text || setweight(d.group_names, 'B') AS document,
           array(SELECT p.ordinal FROM places p WHERE p.id = d.id AND p.score IS NULL ORDER BY p.ordinal)::int[] AS missing,
           (SELECT coalesce(sum(p.score), 0) FROM places p WHERE p.id = d.id) AS term_score,
           (SELECT count(*) FILTER (WHERE w.lexeme IN (
                       SELECT unnest(p.lexemes) FROM places p WHERE p.id = d.id AND p.score = 1.0
                   ))::float / greatest(count(*), 1)
            FROM unnest(d.title) w) AS title_share
    FROM documents d
    WHERE NOT EXISTS (
        SELECT 1 FROM exclude e
        WHERE d.text @@ e.query OR d.group_names @@ e.query
           OR (numnode(e.identifier_query) > 0 AND d.identifiers @@ e.identifier_query)
    )
),
ranked AS (
    SELECT s.id, s.missing,
           power((search.terms - cardinality(s.missing)) / search.terms, 2) * s.term_score / search.terms
               + 0.25 * s.title_share AS score,
           ts_rank(s.document, search.any_query) AS text_rank
    FROM scored s
    CROSS JOIN search
    WHERE cardinality(s.missing) < search.terms
)
SELECT l.owner, l.name, l.owner_avatar_url, r.id, r.path, g.path AS group_path, v.title::text AS title,
       v.impact::text AS impact, v.when_to_read::text AS when_to_read,
       coalesce(CASE WHEN v.rendered_when_to_read = v.when_to_read THEN v.when_to_read_html END, '')::text AS when_to_read_html,
       v.major, v.minor, v.patch, ranked.missing, count(*) OVER () AS total,
       count(*) FILTER (WHERE cardinality(ranked.missing) = 0) OVER () AS complete
FROM ranked
JOIN rule_versions v ON v.id = ranked.id
JOIN rules r ON r.id = v.rule_id
JOIN library_groups g ON g.id = r.group_id
JOIN libraries l ON l.id = r.library_id
ORDER BY cardinality(ranked.missing) > 0, ranked.score DESC, ranked.text_rank DESC, lower(v.title), lower(l.owner), lower(l.name), r.path
LIMIT @max_results OFFSET @skip;

-- CountSearchableTerms counts the terms that hold a word search looks for, rather than only stop words, such as
-- "the", or punctuation.
-- name: CountSearchableTerms :one
SELECT count(*) FROM unnest(@terms::text[]) AS t(query)
WHERE numnode(websearch_to_tsquery('english', t.query)) > 0;

-- ListCountedRuleIDs returns the IDs of the current rules of the libraries vetted holds, in the group at group_path,
-- or in every group when it's empty: the rules whose stars ListRules counts, since a rule of a library that isn't
-- vetted, or a retired rule, has none.
-- name: ListCountedRuleIDs :many
SELECT r.id
FROM rules r
JOIN libraries l ON l.id = r.library_id
JOIN library_groups g ON g.id = r.group_id
WHERE r.retired_in_release_id IS NULL AND l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
  AND (@group_path::text = '' OR g.path = @group_path::text);

-- ListRules returns one page of a list of rules across libraries: a group's rules, every rule, or a search's matches,
-- as domain.RuleList describes them, after its filters, in its order, skipping skip of them and returning at most
-- max_results. A library is in the list when vetted holds it, or, with include_unvetted, when a listing names it. The
-- list holds the current rules of the group at group_path, or of every group when it's empty, and their retired rules
-- too with include_retired, each by its newest version. With match_all, it holds every one of them; otherwise those
-- that hold at least one of the find terms and none of the exclude terms, matched and scored as SearchRules does.
--
-- Each rule's stars are the ones star_rule_ids and star_counts, in step, give it, which CountRuleStars counted; a rule
-- they don't name has none. The filters keep the rules of the libraries libraries names, in lowercase as owner/name,
-- or of every library when it's empty; of the impact band impact, high for CRITICAL and HIGH and medium for the rest,
-- or of any when it's empty; with at least min_stars stars; and of the kind of group kind, techs or practices, or of
-- both when it's empty.
--
-- Rules that hold every find term come first. Then order_by orders them: best by score as SearchRules ranks them,
-- stars by stars, and new by when the release that first published each rule was tagged, newest first. A retired rule
-- comes after the current rules it ties with, and then rules fall to best's text rank, stars, the libraries first_owner
-- owns, owner, name, title, and rule ID, so the order is stable. Each group's rules then come together, in the order
-- of each group's first rule.
--
-- Every row also says how many rules pass the filters, how many of those hold every find term, and in how many
-- libraries; how many rules of its group pass them; and, the same on every row, how many rules the list holds before
-- its filters, and the libraries those come from, in step, first_owner's first and then by owner and name, each with
-- whether vetted holds it and how many of the rules it holds.
-- name: ListRules :many
WITH find_terms AS (
    SELECT i AS ordinal, (@find_terms::text[])[i] AS query, (@find_identifier_terms::text[])[i] AS identifier_query
    FROM generate_subscripts(@find_terms::text[], 1) AS i
),
find AS (
    SELECT t.ordinal, websearch_to_tsquery('english', t.query) AS query,
           websearch_to_tsquery('english', t.identifier_query) AS identifier_query,
           tsvector_to_array(to_tsvector('english', t.query)) AS lexemes
    FROM find_terms t
    WHERE numnode(websearch_to_tsquery('english', t.query)) > 0
),
exclude_terms AS (
    SELECT (@exclude_terms::text[])[i] AS query, (@exclude_identifier_terms::text[])[i] AS identifier_query
    FROM generate_subscripts(@exclude_terms::text[], 1) AS i
),
exclude AS (
    SELECT websearch_to_tsquery('english', t.query) AS query,
           websearch_to_tsquery('english', t.identifier_query) AS identifier_query
    FROM exclude_terms t
    WHERE numnode(websearch_to_tsquery('english', t.query)) > 0
),
search AS (
    SELECT websearch_to_tsquery('english', array_to_string(@find_terms::text[], ' or ')) AS any_query,
           (SELECT count(*) FROM find)::float AS terms
),
stars AS (
    SELECT (@star_rule_ids::bigint[])[i] AS rule_id, (@star_counts::integer[])[i] AS stars
    FROM generate_subscripts(@star_rule_ids::bigint[], 1) AS i
),
documents AS (
    SELECT r.id, newest.id AS version_id, (r.retired_in_release_id IS NOT NULL)::boolean AS retired,
           (l.host || ':' || l.host_repository_id = ANY (@vetted::text[]))::boolean AS vetted,
           newest.search_document || setweight(to_tsvector('english', l.owner || ' ' || l.name), 'D') AS text,
           ts_filter(newest.search_document, '{a}') AS title,
           to_tsvector('english', coalesce((@canonical_names::text[])[array_position(@canonical_ids::text[], g.path)], '')
               || ' ' || split_part(g.path, '/', 2)) AS group_names,
           to_tsvector('english', translate(l.owner || ' ' || l.name || ' ' || r.path, '/-:', '   ')) AS identifiers
    FROM rules r
    JOIN library_groups g ON g.id = r.group_id
    JOIN libraries l ON l.id = r.library_id
    JOIN LATERAL (
        SELECT v.id, v.search_document FROM rule_versions v JOIN library_releases p ON p.id = v.release_id
        WHERE v.rule_id = r.id ORDER BY p.number DESC LIMIT 1
    ) newest ON true
    WHERE (r.retired_in_release_id IS NULL OR @include_retired::boolean)
      AND (@group_path::text = '' OR g.path = @group_path::text)
      AND (
          l.host || ':' || l.host_repository_id = ANY (@vetted::text[])
          OR (@include_unvetted::boolean
              AND EXISTS (SELECT 1 FROM listings s WHERE s.host = l.host AND s.host_repository_id = l.host_repository_id))
      )
),
places AS (
    SELECT d.id, f.ordinal, f.lexemes,
           CASE WHEN d.title @@ f.query THEN 1.0
                WHEN d.group_names @@ f.query OR (numnode(f.identifier_query) > 0 AND d.identifiers @@ f.identifier_query) THEN 0.8
                WHEN ts_filter(d.text, '{b}') @@ f.query THEN 0.5
                WHEN d.text @@ f.query THEN 0.1
           END AS score
    FROM documents d
    CROSS JOIN find f
),
scored AS (
    SELECT d.id, d.text || setweight(d.group_names, 'B') AS document,
           array(SELECT p.ordinal FROM places p WHERE p.id = d.id AND p.score IS NULL ORDER BY p.ordinal)::int[] AS missing,
           (SELECT coalesce(sum(p.score), 0) FROM places p WHERE p.id = d.id) AS term_score,
           (SELECT count(*) FILTER (WHERE w.lexeme IN (
                       SELECT unnest(p.lexemes) FROM places p WHERE p.id = d.id AND p.score = 1.0
                   ))::float / greatest(count(*), 1)
            FROM unnest(d.title) w) AS title_share
    FROM documents d
    WHERE NOT EXISTS (
        SELECT 1 FROM exclude e
        WHERE d.text @@ e.query OR d.group_names @@ e.query
           OR (numnode(e.identifier_query) > 0 AND d.identifiers @@ e.identifier_query)
    )
),
ranked AS (
    SELECT s.id, s.missing,
           CASE WHEN search.terms = 0 THEN 0
                ELSE power((search.terms - cardinality(s.missing)) / search.terms, 2) * s.term_score / search.terms
                    + 0.25 * s.title_share
           END::float AS score,
           CASE WHEN search.terms = 0 THEN 0 ELSE ts_rank(s.document, search.any_query) END::float AS text_rank
    FROM scored s
    CROSS JOIN search
    WHERE @match_all::boolean OR (search.terms > 0 AND cardinality(s.missing) < search.terms)
),
base AS (
    SELECT rk.id, rk.missing, rk.score, rk.text_rank, d.retired, d.vetted, l.id AS library_id, l.owner, l.name,
           l.owner_avatar_url, r.path, g.path AS group_path, r.replaced_by, v.title, v.impact, v.major, v.minor,
           v.patch, coalesce(st.stars, 0) AS stars, first.tagged_at AS first_published_at
    FROM ranked rk
    JOIN documents d ON d.id = rk.id
    JOIN rule_versions v ON v.id = d.version_id
    JOIN rules r ON r.id = rk.id
    JOIN library_groups g ON g.id = r.group_id
    JOIN libraries l ON l.id = r.library_id
    LEFT JOIN stars st ON st.rule_id = rk.id
    JOIN LATERAL (
        SELECT p.tagged_at FROM rule_versions f JOIN library_releases p ON p.id = f.release_id
        WHERE f.rule_id = r.id ORDER BY p.number LIMIT 1
    ) first ON true
),
library_counts AS (
    SELECT b.owner, b.name, b.owner_avatar_url, b.vetted, count(*) AS rules,
           row_number() OVER (ORDER BY lower(b.owner) = lower(@first_owner::text) DESC, lower(b.owner), lower(b.name))
               AS position
    FROM base b
    GROUP BY b.library_id, b.owner, b.name, b.owner_avatar_url, b.vetted
),
facets AS (
    SELECT (SELECT count(*) FROM base) AS unfiltered,
           coalesce(array_agg(c.owner ORDER BY c.position), '{}')::text[] AS library_owners,
           coalesce(array_agg(c.name ORDER BY c.position), '{}')::text[] AS library_names,
           coalesce(array_agg(c.owner_avatar_url ORDER BY c.position), '{}')::text[] AS library_avatar_urls,
           coalesce(array_agg(c.vetted ORDER BY c.position), '{}')::boolean[] AS library_vetted,
           coalesce(array_agg(c.rules ORDER BY c.position), '{}')::bigint[] AS library_rules
    FROM library_counts c
),
filtered AS (
    SELECT b.* FROM base b
    WHERE (cardinality(@libraries::text[]) = 0 OR lower(b.owner || '/' || b.name) = ANY (@libraries::text[]))
      AND (@impact::text = '' OR (@impact::text = 'high') = (coalesce(b.impact, '') IN ('CRITICAL', 'HIGH')))
      AND b.stars >= @min_stars::integer
      AND (@kind::text = '' OR b.group_path LIKE @kind::text || '/%')
),
positioned AS (
    SELECT f.*, row_number() OVER (
               ORDER BY cardinality(f.missing) > 0,
                        CASE @order_by::text
                            WHEN 'best' THEN f.score
                            WHEN 'stars' THEN f.stars::float
                            WHEN 'new' THEN extract(epoch FROM f.first_published_at)::float
                        END DESC,
                        f.retired,
                        CASE WHEN @order_by::text = 'best' THEN f.text_rank END DESC,
                        f.stars DESC,
                        lower(f.owner) = lower(@first_owner::text) DESC, lower(f.owner), lower(f.name),
                        lower(coalesce(f.title, '')), f.path
           ) AS position
    FROM filtered f
),
grouped AS (
    SELECT p.*, min(p.position) OVER (PARTITION BY p.group_path) AS group_position,
           count(*) OVER (PARTITION BY p.group_path) AS group_rules
    FROM positioned p
)
SELECT gr.owner, gr.name, gr.owner_avatar_url, gr.vetted, gr.id, gr.path, gr.group_path,
       coalesce(gr.title, '')::text AS title, coalesce(gr.impact, '')::text AS impact, gr.major, gr.minor, gr.patch,
       gr.retired, coalesce(gr.replaced_by, '')::text AS replaced_by,
       coalesce(replacement.title, '')::text AS replacement_title, gr.stars::integer AS stars, gr.missing,
       gr.group_rules, count(*) OVER () AS total, count(*) FILTER (WHERE cardinality(gr.missing) = 0) OVER () AS complete,
       (SELECT count(DISTINCT f.library_id) FROM filtered f) AS libraries,
       facets.unfiltered, facets.library_owners, facets.library_names, facets.library_avatar_urls,
       facets.library_vetted, facets.library_rules
FROM grouped gr
CROSS JOIN facets
LEFT JOIN LATERAL (
    SELECT v.title FROM rules rr
    JOIN rule_versions v ON v.rule_id = rr.id
    JOIN library_releases p ON p.id = v.release_id
    WHERE rr.library_id = gr.library_id AND rr.path = gr.replaced_by
    ORDER BY p.number DESC LIMIT 1
) replacement ON true
ORDER BY gr.group_position, gr.position
LIMIT @max_results OFFSET @skip;
