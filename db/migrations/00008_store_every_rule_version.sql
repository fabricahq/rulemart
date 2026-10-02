-- +goose Up
-- Every rule version keeps the rule as it published it, so pages can compare two versions' text and show a retired
-- rule by its last title: its title, impact, impact description, reading guidance, and whole Markdown file. Only the
-- current version of a current rule has the HTML its page shows, as before, which still marks the current version. A
-- retired rule's last version has the HTML of its body in a column of its own, retired_html, so its page can show it
-- while every read that finds current rules by their HTML still finds only current ones.
--
-- This only relaxes what 00002 checked, so the release that's still running, which stores content only on current
-- versions, keeps writing rows that pass. Rows it stored before this migration keep older versions without content
-- until ingestion runs again: the worker ingests a library again when any of its versions lacks content.
ALTER TABLE rule_versions DROP CONSTRAINT rule_versions_check;

-- A version has all of its content or none of it: none only in rows a release before this one stored.
ALTER TABLE rule_versions ADD CONSTRAINT rule_versions_content_check
    CHECK (num_nulls(title, impact, impact_description, when_to_read, markdown) IN (0, 5));

-- The current version's HTML is its Markdown's body, so it never stands alone.
ALTER TABLE rule_versions ADD CONSTRAINT rule_versions_html_check CHECK (html IS NULL OR markdown IS NOT NULL);

-- A retired rule's last body, rendered. A version is current or retired, never both, and its HTML renders its
-- Markdown. Adding a nullable column without a default changes no row.
ALTER TABLE rule_versions ADD COLUMN retired_html text;
ALTER TABLE rule_versions ADD CONSTRAINT rule_versions_retired_html_check
    CHECK (retired_html IS NULL OR (html IS NULL AND markdown IS NOT NULL));

-- rulemart_catalog_reader reads the content and retired_html through its SELECT on rule_versions, which 00003 grants,
-- and rulemart_catalog_writer writes them through its INSERT and UPDATE, which 00005 grants.

-- +goose Down
-- Up-only migration; no rollback defined.
