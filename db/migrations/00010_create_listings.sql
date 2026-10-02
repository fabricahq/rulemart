-- +goose Up
-- Anyone signed in can list a public library. A listing is one account's request that Rulemart show a repository's
-- library: the web function adds it, and the worker resolves the repository's ID on its code host, ingests it, and
-- records how each check went. Pages show a library a listing names as unvetted until catalog/vetted.yaml vets it.
--
-- The web function writes listings through rulemart_accounts_writer, since a listing is an account's, and pages read
-- them through rulemart_catalog_reader. The worker resolves and checks them through rulemart_catalog_writer, and may
-- change only what a check finds. Every role exists already, and the release still running doesn't read this table,
-- so adding it changes nothing for it.

CREATE TABLE listings (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- The account that listed the library. Deleting the account removes its listings, so its listings stop counting
    -- toward Rulemart's limit, and a vetted library stays vetted, since vetting is catalog/vetted.yaml's.
    account_id bigint NOT NULL REFERENCES accounts ON DELETE CASCADE,
    host text NOT NULL CHECK (host IN ('github')),
    -- The repository's owner and name as the lister gave them, which GitHub's names allow.
    owner text NOT NULL CHECK (owner ~ '^[A-Za-z0-9]([A-Za-z0-9-]{0,37}[A-Za-z0-9])?$'),
    name text NOT NULL CHECK (name ~ '^[A-Za-z0-9._-]{1,100}$' AND name NOT IN ('.', '..') AND lower(name) NOT LIKE '%.git'),
    -- The host's ID for the repository, once the worker has looked it up: the key vetting and the catalog use.
    host_repository_id text CHECK (host_repository_id ~ '^[1-9][0-9]*$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    -- When the listing last asked for a check: when it was listed, or when its lister last asked to try again.
    requested_at timestamptz NOT NULL DEFAULT now(),
    -- When the worker last finished checking the listing, or NULL until it first does.
    checked_at timestamptz,
    -- Why the last check failed, in words the lister can act on, or NULL when it succeeded or hasn't run.
    failure text CHECK (failure <> '' AND length(failure) <= 1000),
    CHECK (failure IS NULL OR checked_at IS NOT NULL)
);

-- A repository is listed once, by the name its lister gave, without regard to case as GitHub treats names, and by
-- its ID once resolved, so a renamed repository can't be listed again under its new name.
CREATE UNIQUE INDEX listings_host_owner_name_key ON listings (host, lower(owner), lower(name));
CREATE UNIQUE INDEX listings_host_repository_id_key ON listings (host, host_repository_id);
CREATE INDEX listings_account_id_idx ON listings (account_id);

-- When each listing was listed or tried again, kept for a day, which bounds how often an account, and every account
-- together, makes the worker check a listing. A request outlives a deleted account, unlinked, so deleting an account
-- doesn't reset the hourly count across accounts.
CREATE TABLE listing_requests (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    account_id bigint REFERENCES accounts ON DELETE SET NULL,
    requested_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX listing_requests_requested_at_idx ON listing_requests (requested_at);
CREATE INDEX listing_requests_account_id_idx ON listing_requests (account_id, requested_at);

-- Listing, removing a listing, and asking the worker to try one again, within the request limits.
GRANT SELECT, INSERT, DELETE ON listing_requests TO rulemart_accounts_writer;
GRANT SELECT, INSERT, DELETE ON listings TO rulemart_accounts_writer;
GRANT UPDATE (requested_at, failure) ON listings TO rulemart_accounts_writer;
-- Pages find the libraries listings name.
GRANT SELECT ON listings TO rulemart_catalog_reader;
-- The worker records what each check finds, and nothing else: it can't list a library or change who listed one.
GRANT SELECT ON listings TO rulemart_catalog_writer;
GRANT UPDATE (host_repository_id, checked_at, failure) ON listings TO rulemart_catalog_writer;

-- +goose Down
-- Up-only migration; no rollback defined.
