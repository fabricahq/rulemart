-- +goose Up
-- The cart moves into the visitor's browser, where it needs no account: pages paint it from localStorage, and
-- checkout sends its keys to the web function, which resolves them against the catalog and writes nothing. Nothing
-- reads cart_items any more.
--
-- cart_items, which 00012 created, is dropped: no release with 00012 was published, so production never had it, and
-- only a local or branch database can hold one of its rows. Such a database stops here instead of losing them, so
-- someone looks at it first, as 00013 does for stars. The grants to rulemart_accounts_writer go with the table.

-- +goose StatementBegin
DO $$
BEGIN
    LOCK TABLE cart_items IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT FROM cart_items) THEN
        RAISE EXCEPTION 'cart_items holds rows, which dropping it would lose: no published release created cart_items, so this database holds carts from an unreleased build; delete them, or the database, before migrating';
    END IF;
END
$$;
-- +goose StatementEnd
DROP TABLE cart_items;

-- +goose Down
-- Up-only migration; no rollback defined.
