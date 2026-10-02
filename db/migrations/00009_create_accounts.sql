-- +goose Up
-- Visitors sign in with GitHub. An account is one GitHub user, keyed by GitHub's numeric user ID, which never
-- changes, rather than the login, which its owner can rename and someone else can then take. It keeps only what pages
-- show of the user: the login and the avatar's address. A session is one signed-in browser: the cookie holds a random
-- token, and the table only its SHA-256, so the table alone can't sign anyone in.
--
-- The web function signs visitors in and out through rulemart_accounts_writer, a group role that can't log in, which
-- infrastructure creates with SQL, as it does the catalog's group roles, and makes rulemart_web a member of. This
-- migration never creates it, so a release can't apply its migrations until infrastructure has. The release still
-- running doesn't read these tables, so adding them changes nothing for it.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'rulemart_accounts_writer') THEN
        RAISE EXCEPTION 'role rulemart_accounts_writer does not exist: infrastructure creates it with SQL, as a plain NOLOGIN role, before this migration grants it access; migrations never create it (locally, make db does)';
    END IF;
    -- Every login that joins the group gets what it grants, so the group itself must not be a login.
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'rulemart_accounts_writer' AND rolcanlogin) THEN
        RAISE EXCEPTION 'role rulemart_accounts_writer can log in: infrastructure creates it with SQL as a plain NOLOGIN group role, and logins join it; recreate it with NOLOGIN';
    END IF;
    -- Grants can't narrow what a privileged role already holds, and its members would inherit it, so refuse one rather
    -- than give them a false boundary.
    IF EXISTS (
        SELECT FROM pg_roles
        WHERE rolname = 'rulemart_accounts_writer'
          AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolreplication)
    ) OR EXISTS (
        SELECT FROM pg_auth_members m JOIN pg_roles r ON r.oid = m.member WHERE r.rolname = 'rulemart_accounts_writer'
    ) THEN
        RAISE EXCEPTION 'role rulemart_accounts_writer is privileged: infrastructure creates it with SQL as a plain NOLOGIN role with no SUPERUSER, CREATEROLE, CREATEDB, BYPASSRLS, or REPLICATION and no role memberships, such as neon_superuser, which Neon gives roles made through its API or console; recreate it with SQL';
    END IF;
END
$$;
-- +goose StatementEnd

CREATE TABLE accounts (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    github_user_id bigint NOT NULL UNIQUE CHECK (github_user_id > 0),
    -- The login GitHub gave at the account's latest sign-in: letters, digits, hyphens, and an Enterprise Managed
    -- User's underscore. Logins aren't unique here: a renamed user's old login can belong to someone else by the time
    -- either signs in again.
    github_login text NOT NULL CHECK (github_login ~ '^[A-Za-z0-9_-]{1,39}$'),
    -- The avatar GitHub gave at the latest sign-in, on GitHub's avatar host, or empty to show the login's initial.
    avatar_url text NOT NULL CHECK (avatar_url = '' OR avatar_url LIKE 'https://avatars.githubusercontent.com/%'),
    created_at timestamptz NOT NULL DEFAULT now(),
    signed_in_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE sessions (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- The SHA-256 of the session cookie's token.
    token_hash bytea NOT NULL UNIQUE CHECK (length(token_hash) = 32),
    account_id bigint NOT NULL REFERENCES accounts ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    CHECK (expires_at > created_at)
);
CREATE INDEX sessions_account_id_idx ON sessions (account_id);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);

GRANT USAGE ON SCHEMA public TO rulemart_accounts_writer;
-- Signing in adds or updates an account, and deleting one is the visitor's own choice.
GRANT SELECT, INSERT, UPDATE, DELETE ON accounts TO rulemart_accounts_writer;
-- Sessions are added at sign-in and deleted at sign-out or once expired; none changes.
GRANT SELECT, INSERT, DELETE ON sessions TO rulemart_accounts_writer;

-- +goose Down
-- Up-only migration; no rollback defined.
