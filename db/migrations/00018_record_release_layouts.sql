-- +goose Up
-- Trail Guide moved a library's files from the repository's root into its Trail Guide directory: the manifest is the
-- library section of .trailguide/config.yaml, groups live in .trailguide/rules/, and shared assets in .trailguide/assets/.
-- Rule IDs and the paths the catalog stores stay the same, but each release's files on GitHub are where that release
-- keeps them, so ingestion records each release's layout, and pages link its files there.
--
-- Every library stored before this migration keeps its files at the root, which the default records. The release
-- still running neither reads nor writes the column, and ingesting with it leaves the default.

-- The directory that holds the library's files: empty for the repository's root, or .trailguide, or .code-rules,
-- the Trail Guide directory's name before Code Rules was renamed Trail Guide.
ALTER TABLE library_releases ADD COLUMN layout text NOT NULL DEFAULT ''
    CHECK (layout IN ('', '.trailguide', '.code-rules'));

-- +goose Down
-- Up-only migration; no rollback defined.
