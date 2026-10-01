-- +goose Up
-- The web function reads only what the pages show and the schema version its schema check reads, through
-- rulemart_catalog_reader, a group role that can't log in. Migrations grant to that role, never to a login, so
-- infrastructure can replace or rotate the web function's login, rulemart_web, without a migration. Infrastructure
-- creates the group role with SQL, as a plain NOLOGIN role, and makes the login a member of it: a role made through
-- Neon's API or console joins neon_superuser, which can read and write every table. This migration never creates it,
-- so a release can't apply its migrations until infrastructure has.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'rulemart_catalog_reader') THEN
        RAISE EXCEPTION 'role rulemart_catalog_reader does not exist: infrastructure creates it with SQL, as a plain NOLOGIN role, before this migration grants it access; migrations never create it (locally, make db does)';
    END IF;
    -- Every login that joins the group gets what it grants, so the group itself must not be a login.
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'rulemart_catalog_reader' AND rolcanlogin) THEN
        RAISE EXCEPTION 'role rulemart_catalog_reader can log in: infrastructure creates it with SQL as a plain NOLOGIN group role, and logins join it; recreate it with NOLOGIN';
    END IF;
    -- Grants can't narrow what a privileged role already holds, and its members would inherit it, so refuse one rather
    -- than give them a false boundary.
    IF EXISTS (
        SELECT FROM pg_roles
        WHERE rolname = 'rulemart_catalog_reader'
          AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolreplication)
    ) OR EXISTS (
        SELECT FROM pg_auth_members m JOIN pg_roles r ON r.oid = m.member WHERE r.rolname = 'rulemart_catalog_reader'
    ) THEN
        RAISE EXCEPTION 'role rulemart_catalog_reader is privileged: infrastructure creates it with SQL as a plain NOLOGIN role with no SUPERUSER, CREATEROLE, CREATEDB, BYPASSRLS, or REPLICATION and no role memberships, such as neon_superuser, which Neon gives roles made through its API or console; recreate it with SQL';
    END IF;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO rulemart_catalog_reader;
GRANT SELECT ON libraries, library_releases, library_groups, rules, rule_versions TO rulemart_catalog_reader;
GRANT SELECT ON goose_db_version TO rulemart_catalog_reader;

-- +goose Down
-- Up-only migration; no rollback defined.
