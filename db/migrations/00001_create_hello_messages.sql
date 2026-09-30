-- +goose Up
CREATE TABLE hello_messages (
    id          bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    message_id  text NOT NULL UNIQUE,
    text        text NOT NULL,
    source      text NOT NULL,
    sent_at     timestamptz NOT NULL,
    received_at timestamptz NOT NULL DEFAULT now()
);

-- +goose Down
-- Up-only migration; no rollback defined.
