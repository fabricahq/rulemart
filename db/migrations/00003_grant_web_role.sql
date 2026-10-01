-- +goose Up
-- The web function connects as rulemart_web, which may read only what the pages show and the schema version its
-- schema check reads. Infrastructure creates the role with SQL, as a plain LOGIN role: a role made through Neon's
-- API or console joins neon_superuser, which can read and write every table. This migration never creates it, so
-- a release can't apply its migrations until infrastructure has.

-- +goose StatementBegin
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'rulemart_web') THEN
        RAISE EXCEPTION 'role rulemart_web does not exist: infrastructure creates it with SQL, as a plain LOGIN role, before this migration grants it access; migrations never create it (locally, make db does)';
    END IF;
    -- Grants can't narrow what a privileged role already holds, so refuse one rather than give it a false boundary.
    IF EXISTS (
        SELECT FROM pg_roles
        WHERE rolname = 'rulemart_web'
          AND (rolsuper OR rolcreaterole OR rolcreatedb OR rolbypassrls OR rolreplication)
    ) OR EXISTS (
        SELECT FROM pg_auth_members m JOIN pg_roles r ON r.oid = m.member WHERE r.rolname = 'rulemart_web'
    ) THEN
        RAISE EXCEPTION 'role rulemart_web is privileged: it must be a plain LOGIN role with no SUPERUSER, CREATEROLE, CREATEDB, BYPASSRLS, or REPLICATION and no role memberships, such as neon_superuser, which Neon gives roles made through its API or console; recreate it with SQL';
    END IF;
END
$$;
-- +goose StatementEnd

GRANT USAGE ON SCHEMA public TO rulemart_web;
GRANT SELECT ON libraries, library_releases, library_groups, rules, rule_versions TO rulemart_web;
GRANT SELECT ON goose_db_version TO rulemart_web;

-- +goose Down
-- Up-only migration; no rollback defined.
