-- +goose Up
-- Anyone signed in can star a vetted library: a star is one account's mark on one library, which its account page
-- lists, and which pages count. Pages count stars as they read, rather than keeping a count that could drift.
--
-- The web function stars and unstars through rulemart_accounts_writer, since a star is an account's, and pages count
-- stars through rulemart_catalog_reader. The worker gets nothing: ingestion never reads who starred what. Every role
-- exists already, and the release still running doesn't read this table, so adding it changes nothing for it.

CREATE TABLE stars (
    -- Deleting an account removes its stars, so its stars stop counting.
    account_id bigint NOT NULL REFERENCES accounts ON DELETE CASCADE,
    -- A library row keeps its id for as long as the catalog stores it, across ingestions and renames, so a star
    -- follows the library. Neither function deletes a library; if an operator does, its stars go with it.
    library_id bigint NOT NULL REFERENCES libraries ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- An account stars a library at most once, so starring twice changes nothing.
    PRIMARY KEY (account_id, library_id)
);
-- Pages count each library's stars.
CREATE INDEX stars_library_id_idx ON stars (library_id);

-- Starring and unstarring, and the account's own list. A star is never changed, only added or removed.
GRANT SELECT, INSERT, DELETE ON stars TO rulemart_accounts_writer;
-- Pages count the stars of the libraries they show.
GRANT SELECT ON stars TO rulemart_catalog_reader;

-- +goose Down
-- Up-only migration; no rollback defined.
