-- +goose Up
-- Stars move from libraries to rules: anyone signed in stars a current rule of a vetted library, which their Starred
-- rules list, and every page that shows the rule counts. A library's total is the sum of its rules'. Pages count
-- stars as they read, rather than keeping a count that could drift.
--
-- stars, which 00011 created for libraries, is dropped: no release with 00011 was published, so production never
-- had it, and only a local or branch database can hold one of its rows. Such a database stops here instead of
-- losing them, so someone looks at it first.
--
-- The web function stars and unstars through rulemart_accounts_writer, since a star is an account's, and pages count
-- stars through rulemart_catalog_reader, as 00011 granted. The worker gets nothing: ingestion never reads who starred
-- what. The release still running reads neither table, since none with 00011 was published.

-- +goose StatementBegin
DO $$
BEGIN
    LOCK TABLE stars IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT FROM stars) THEN
        RAISE EXCEPTION 'stars holds rows, which dropping it would lose: no published release created stars, so this database holds stars from an unreleased build; delete them, or the database, before migrating';
    END IF;
END
$$;
-- +goose StatementEnd
DROP TABLE stars;

CREATE TABLE rule_stars (
    -- Deleting an account removes its stars, so its stars stop counting.
    account_id bigint NOT NULL REFERENCES accounts ON DELETE CASCADE,
    -- A rule row keeps its id for as long as the catalog stores it, across ingestions, retired or not. A star stays
    -- on the rule it was given to: once the rule is retired, pages count it toward the rule that replaced it. Only an
    -- ingestion that drops the rule from its library's history deletes it, and its stars go with it.
    rule_id bigint NOT NULL REFERENCES rules ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- An account stars a rule at most once, so starring twice changes nothing.
    PRIMARY KEY (account_id, rule_id)
);
-- Pages count each rule's stars, and deleting a rule finds its stars.
CREATE INDEX rule_stars_rule_id_idx ON rule_stars (rule_id);
-- Counting a rule's stars follows its replacements backwards: the retired rules whose retirement named it, and the
-- ones that named those.
CREATE INDEX rules_replaced_by_idx ON rules (library_id, replaced_by) WHERE replaced_by IS NOT NULL;

-- Starring and unstarring, and the account's own list. A star is never changed, only added or removed.
GRANT SELECT, INSERT, DELETE ON rule_stars TO rulemart_accounts_writer;
-- Pages count the stars of the rules they show.
GRANT SELECT ON rule_stars TO rulemart_catalog_reader;

-- +goose Down
-- Up-only migration; no rollback defined.
