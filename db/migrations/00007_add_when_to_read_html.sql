-- +goose Up
-- A current rule version's reading guidance as its page shows it: when_to_read_html is the HTML ingestion renders it
-- to, as it renders the body, so inline Markdown such as code shows as the body's does. rendered_when_to_read is the
-- reading guidance that HTML was rendered from.
--
-- The release that's still running keeps writing when_to_read without either column, so a library release it ingests
-- could change the reading guidance and leave the HTML of the old one. Pages show the HTML only while
-- rendered_when_to_read matches when_to_read, and show when_to_read as text otherwise, and the worker ingests a
-- library again while any current version's doesn't match, so rows stored before this migration, or by the running
-- release after it, get their HTML within an hour of this release's deployment.
--
-- Adding nullable columns without a default changes no row, so this takes a brief exclusive lock and no rewrite.
ALTER TABLE rule_versions ADD COLUMN when_to_read_html text, ADD COLUMN rendered_when_to_read text;

-- rulemart_catalog_reader reads the new columns through its SELECT on rule_versions, which 00003 grants, and
-- rulemart_catalog_writer writes them through its INSERT and UPDATE, which 00005 grants.

-- +goose Down
-- Up-only migration; no rollback defined.
