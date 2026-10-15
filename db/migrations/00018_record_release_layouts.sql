-- +goose Up
-- Syllabus moved a library's files from the repository's root into its Syllabus directory: the manifest is the
-- library section of .syllabus/config.yaml, groups live in .syllabus/rules/, and shared assets in .syllabus/assets/.
-- Rule IDs and the paths the catalog stores stay the same, but each release's files on GitHub are where that release
-- keeps them, so ingestion records each release's layout, and pages link its files there.
--
-- Every library stored before this migration keeps its files at the root, which the default records. The release
-- still running neither reads nor writes the column, and ingesting with it leaves the default.

-- The directory that holds the library's files: empty for the repository's root, or .syllabus, or .code-rules,
-- the Syllabus directory's name before Code Rules was renamed Syllabus.
ALTER TABLE library_releases ADD COLUMN layout text NOT NULL DEFAULT ''
    CHECK (layout IN ('', '.syllabus', '.code-rules'));

-- +goose Down
-- Up-only migration; no rollback defined.
