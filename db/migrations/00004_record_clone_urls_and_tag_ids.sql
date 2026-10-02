-- +goose Up
-- What the worker compares with a library's remote to decide whether to ingest it again: where ingestion fetched the
-- library, and the tag object each release/<n> tag pointed to. Both are NULL in rows a release written before them
-- stored, until ingestion runs again, so the release that's still running keeps writing rows without them.

-- Where ingestion last fetched the library, such as https://github.com/owner/name.git.
ALTER TABLE libraries ADD COLUMN clone_url text CHECK (clone_url <> '');

-- The annotated tag object the release's tag pointed to. A release's record is in its tag's message, so a tag
-- rewritten on the same commit changes this, but not commit_id.
ALTER TABLE library_releases ADD COLUMN tag_object_id text CHECK (tag_object_id <> '');

-- +goose Down
-- Up-only migration; no rollback defined.
