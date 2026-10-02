-- +goose Up
-- The worker function ingests libraries through rulemart_catalog_writer, a group role that can't log in. It may read
-- and write the catalog's rows and read the schema version its schema check reads, and nothing else: it never deletes
-- a library, and it can't change the schema. Migrations grant to that role, never to a login, so infrastructure can
-- replace or rotate the worker's login, rulemart_worker, without a migration. Infrastructure creates the group role
-- with SQL, as a plain NOLOGIN role, and makes the login a member of it, as it does rulemart_catalog_reader. This
-- migration never creates it, so a release can't apply its migrations until infrastructure has.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'rulemart_catalog_writer') THEN
        RAISE EXCEPTION 'role rulemart_catalog_writer does not exist: infrastructure creates it with SQL, as a plain NOLOGIN role, before this migration grants it access; migrations never create it (locally, make db does)';
    END IF;
    -- Every login that joins the group gets what it grants, so the group itself must not be a login.
    IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'rulemart_catalog_writer' AND rolcanlogin) THEN
        RAISE EXCEPTION 'role rulemart_catalog_writer can log in: infrastructure creates it with SQL as a plain NOLOGIN group role, and logins join it; recreate it with NOLOGIN';
    END IF;
    -- Grants can't narrow what a privileged role already holds, and its members would inherit it, so refuse one rather
    -- than give them a false boundary.
    IF EXISTS (
        SELECT FROM pg_roles
        WHERE rolname = 'rulemart_catalog_writer'
          AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolreplication)
    ) OR EXISTS (
        SELECT FROM pg_auth_members m JOIN pg_roles r ON r.oid = m.member WHERE r.rolname = 'rulemart_catalog_writer'
    ) THEN
        RAISE EXCEPTION 'role rulemart_catalog_writer is privileged: infrastructure creates it with SQL as a plain NOLOGIN role with no SUPERUSER, CREATEROLE, CREATEDB, BYPASSRLS, or REPLICATION and no role memberships, such as neon_superuser, which Neon gives roles made through its API or console; recreate it with SQL';
    END IF;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO rulemart_catalog_writer;
-- Ingestion upserts a library on its host and repository ID, and never deletes one.
GRANT SELECT, INSERT, UPDATE ON libraries TO rulemart_catalog_writer;
-- Ingestion replaces everything below a library, deleting what its tags no longer publish.
GRANT SELECT, INSERT, UPDATE, DELETE ON library_releases, library_groups, rules, rule_versions TO rulemart_catalog_writer;
GRANT SELECT ON goose_db_version TO rulemart_catalog_writer;

-- +goose Down
-- Up-only migration; no rollback defined.
