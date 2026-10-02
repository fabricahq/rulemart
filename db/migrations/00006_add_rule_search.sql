-- +goose Up
-- What search matches and ranks a current rule by: its words, stemmed for English, weighted by where they appear.
-- The title weighs most (A), then the reading guidance and impact description that summarize the rule (B), then the
-- Markdown body (D). Search adds the rule's group names when it reads, with the canonical group list the running
-- release ships, so they aren't stored here.
--
-- Postgres computes the column from the row, so rows a release written before it stored get one when this migration
-- rewrites the table, and the release that's still running keeps writing rows without naming it. A tsvector holds at
-- most 1 MB, and a rule file may hold 1 MiB, so each part is cut to a length whose vector stays well under that
-- limit, whatever the text: otherwise one large rule would fail its library's ingestion. Search reads the first
-- 100,000 characters of a body, which is more than ten times the longest rule Rulemart knows.
ALTER TABLE rule_versions ADD COLUMN search_document tsvector GENERATED ALWAYS AS (
    setweight(to_tsvector('english', left(coalesce(title, ''), 1000)), 'A') ||
    setweight(to_tsvector('english', left(coalesce(when_to_read, '') || ' ' || coalesce(impact_description, ''), 10000)), 'B') ||
    setweight(to_tsvector('english', left(coalesce(markdown, ''), 100000)), 'D')
) STORED;

-- Only the current version of a current rule has content, and only those are searched.
CREATE INDEX rule_versions_search_document ON rule_versions USING gin (search_document) WHERE html IS NOT NULL;

-- rulemart_catalog_reader reads the new column through its SELECT on rule_versions, which 00003 grants, and
-- rulemart_catalog_writer never writes it: Postgres does. An index needs no grant.

-- +goose Down
-- Up-only migration; no rollback defined.
