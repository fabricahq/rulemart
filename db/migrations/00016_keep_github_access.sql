-- +goose Up
-- The dashboard reads the visitor's GitHub account: their organizations, the repositories of theirs and their
-- organizations' that publish a Code Rules library, and the projects that import one. Sign-in asks GitHub for
-- read:org, and the session keeps the OAuth token, sealed with AES-256-GCM under a key only the web function holds, so
-- reading the table alone reveals no token. Signing out deletes the session, and the token with it.
--
-- What a read found is kept per account, in github_snapshots, so pages read GitHub only at sign-in and when the visitor
-- refreshes. The GitHub App "Rulemart by Fabrica" reads private repositories where the visitor installs it, and
-- github_installations records which installations an account may read through. An account keeps its GitHub name too,
-- which the header shows. Deleting an account deletes all of it.
--
-- The web function writes all of it through rulemart_accounts_writer, as it does accounts and sessions. Neither catalog
-- role gets anything: the worker never reads an account's GitHub data. The release still running doesn't read the new
-- tables, and inserts sessions and accounts without the new columns, which are nullable or have defaults.

-- The name the visitor's GitHub profile shows, at their latest sign-in, or empty when they set none.
ALTER TABLE accounts ADD COLUMN github_name text NOT NULL DEFAULT '' CHECK (length(github_name) <= 255);

-- The session's GitHub token, sealed: a 12-byte nonce, then the ciphertext and its 16-byte tag, bound to the session's
-- token hash, so a sealed token copied to another session doesn't open. NULL when the session has none, such as a local
-- build's test user's.
ALTER TABLE sessions ADD COLUMN github_token bytea CHECK (length(github_token) BETWEEN 29 AND 1024);

-- How many times the account's access to private repositories has changed, or its snapshot was discarded for another
-- reason: installing the app, removing access, GitHub's webhook, and signing in each add one. A read notes it before
-- reading GitHub and keeps what it found only if it hasn't changed since, so a read that overlaps a removal can't bring
-- back what the removal discarded. A change adds one before it discards the snapshot, in the same transaction, so a
-- read's save that locks the account's row first is discarded after it.
ALTER TABLE accounts ADD COLUMN github_generation bigint NOT NULL DEFAULT 0;
-- When a read of the account's GitHub account last began, whether or not it succeeded, or NULL when none has since its
-- snapshot was last discarded. A read claims it before contacting GitHub, in one statement, only when it's NULL or a
-- minute old, so requests that arrive together read GitHub once between them.
ALTER TABLE accounts ADD COLUMN github_tried_at timestamptz;

CREATE TABLE github_snapshots (
    account_id bigint PRIMARY KEY REFERENCES accounts ON DELETE CASCADE,
    -- The organizations, publishable repositories, and projects the read found, as accounts' store encodes them. Only
    -- the account whose snapshot it is sees it, so it may name private repositories. When they were read, which pages
    -- show, is in the snapshot.
    snapshot jsonb NOT NULL CHECK (jsonb_typeof(snapshot) = 'object')
);

CREATE TABLE github_installations (
    account_id bigint NOT NULL REFERENCES accounts ON DELETE CASCADE,
    -- GitHub's ID for the installation of the GitHub App, which GitHub returned to the account after installing it,
    -- and Rulemart checked belongs to the account or an organization it owns.
    installation_id bigint NOT NULL CHECK (installation_id > 0),
    -- The GitHub account the app is installed on, the visitor's own or an organization's, as GitHub spelled it.
    github_account text NOT NULL CHECK (github_account ~ '^[A-Za-z0-9_-]{1,39}$'),
    created_at timestamptz NOT NULL DEFAULT now(),
    -- An organization's installation can serve each of its owners who connect it.
    PRIMARY KEY (account_id, installation_id)
);
-- GitHub's webhook names an installation that changed or was removed.
CREATE INDEX github_installations_installation_id_idx ON github_installations (installation_id);

-- A read replaces the snapshot, and a change to an installation discards it.
GRANT SELECT, INSERT, UPDATE, DELETE ON github_snapshots TO rulemart_accounts_writer;
-- Installing adds one, and removing access, or GitHub's webhook, deletes it; none changes.
GRANT SELECT, INSERT, DELETE ON github_installations TO rulemart_accounts_writer;

-- +goose Down
-- Up-only migration; no rollback defined.
