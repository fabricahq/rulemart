-- +goose Up
-- GitHub sends each delivery of the GitHub App's webhook with a unique ID, its X-GitHub-Delivery header, which a
-- redelivery keeps, and signs its body, but not the ID. The webhook records each delivery it acts on, in the
-- transaction that acts on it, and ignores one whose ID or body it recorded, so the same delivery sent again, by GitHub
-- or by anyone who copied it, changes nothing; the body's digest recognizes a copy sent under a new ID. Each delivery
-- forgets the ones recorded more than a week before it, in the same transaction, so a delivery is remembered longer
-- than the three days GitHub lets one be redelivered.
--
-- The web function writes it through rulemart_accounts_writer, as it does the installations the webhook changes. The
-- release still running doesn't read it.
CREATE TABLE github_deliveries (
    delivery_id text PRIMARY KEY CHECK (length(delivery_id) BETWEEN 1 AND 100),
    -- The SHA-256 of the delivery's body, as it arrived.
    body_sha256 bytea NOT NULL UNIQUE CHECK (length(body_sha256) = 32),
    -- When the webhook acted on it, by the web function's clock.
    applied_at timestamptz NOT NULL
);
-- Each delivery forgets the oldest.
CREATE INDEX github_deliveries_applied_at_idx ON github_deliveries (applied_at);

-- Forgetting the oldest reads when each was applied.
GRANT SELECT, INSERT, DELETE ON github_deliveries TO rulemart_accounts_writer;

-- +goose Down
-- Up-only migration; no rollback defined.
