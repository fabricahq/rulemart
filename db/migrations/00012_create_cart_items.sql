-- +goose Up
-- Anyone signed in collects rules in a cart, then checks out: Rulemart writes a prompt that sets up their project to
-- import exactly those rules with Code Rules. An item is a whole library, one of its groups, or one of its rules,
-- named by its ID in the library, so it stays in the cart when its library releases again, and the cart can say when
-- a rule it holds retired, or a group it holds no longer has rules.
--
-- The web function adds and removes items through rulemart_accounts_writer, since a cart is an account's, and reads
-- them with the catalog through the same membership. The worker gets nothing: ingestion never reads carts. Every role
-- exists already, and the release still running doesn't read this table, so adding it changes nothing for it.

CREATE TABLE cart_items (
    -- Deleting an account empties its cart.
    account_id bigint NOT NULL REFERENCES accounts ON DELETE CASCADE,
    -- A library row keeps its id for as long as the catalog stores it, across ingestions and renames, so an item
    -- follows its library. Neither function deletes a library; if an operator does, its items go with it.
    library_id bigint NOT NULL REFERENCES libraries ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('library', 'group', 'rule')),
    -- The group's or rule's ID, as the library spells it, such as techs/go or techs/go/return-errors, and empty for a
    -- whole library. Code Rules' IDs are short paths; the bound keeps a crafted one from growing a row.
    path text NOT NULL CHECK ((kind = 'library') = (path = '') AND length(path) <= 1000),
    -- When the visitor confirmed adding the item from a library Rulemart doesn't vet, or NULL when its library was
    -- vetted. Checkout leaves out an unconfirmed item whose library isn't vetted, such as one that lost its vetting.
    unvetted_confirmed_at timestamptz,
    added_at timestamptz NOT NULL DEFAULT now(),
    -- A cart holds each item once, so adding it twice changes nothing.
    PRIMARY KEY (account_id, library_id, kind, path)
);
-- Deleting a library, which only an operator does, finds its items.
CREATE INDEX cart_items_library_id_idx ON cart_items (library_id);

-- Adding, confirming, and removing items, and reading the cart. Confirming is the only change to an item.
GRANT SELECT, INSERT, DELETE ON cart_items TO rulemart_accounts_writer;
GRANT UPDATE (unvetted_confirmed_at) ON cart_items TO rulemart_accounts_writer;

-- +goose Down
-- Up-only migration; no rollback defined.
