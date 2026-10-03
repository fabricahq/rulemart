-- +goose Up
-- A library's pages show more of it: each current rule's assets, the supporting files Code Rules copies with it, its
-- tags, and when the library came to Rulemart and who listed it.
--
-- Assets are a current rule's own files, in its asset directory, assets/<rule name>/ beside its file, as the release
-- that published its current version holds them, and the library-root assets/ files its text or Markdown files link
-- to, as the latest release holds them. Ingestion keeps each file's size and type, and the bytes of images and text
-- within caps, with the HTML a page shows for Markdown and text; it lists larger files with their size only.
--
-- Nothing here changes a row the release that's still running reads or writes: the new columns are nullable or have a
-- default, and it doesn't know the new tables. A library it stores, or stored before this migration, has versions
-- without tags, which tells the worker to ingest it again: ingestion writes tags, as an empty array when a rule lists
-- none, and assets in the same transaction, so a version without tags marks a library whose assets weren't read.

-- The topics a version's frontmatter lists, in its order, each once. NULL only in rows a release before this one
-- stored, which ingesting the library again fills in.
ALTER TABLE rule_versions ADD COLUMN tags text[];
ALTER TABLE rule_versions ADD CONSTRAINT rule_versions_tags_check CHECK (tags IS NULL OR markdown IS NOT NULL);

-- When the library came to Rulemart: when it was first ingested. A library already stored gets this migration's time,
-- shortly after Rulemart launched. now() is the transaction's start, the same for every row, so adding the column
-- rewrites no row.
ALTER TABLE libraries ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();

-- One asset of a library's current rules.
CREATE TABLE assets (
    id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    library_id bigint NOT NULL REFERENCES libraries ON DELETE CASCADE,
    -- The file's path in the repository: in a rule's asset directory, or under the library-root assets/.
    path       text NOT NULL CHECK (path <> ''),
    -- The library release whose commit the copy is from: the one that published its rule's current version, for a
    -- rule's own file, and the latest, for a shared one.
    release_id bigint NOT NULL,
    size       bigint NOT NULL CHECK (size >= 0),
    -- What ingestion found the file to be, which Rulemart serves an image it keeps as.
    media_type text NOT NULL CHECK (media_type <> ''),
    -- The file's bytes, when it's an image or text within the caps; NULL otherwise.
    content    bytea,
    -- How a page shows Markdown or text it has the content of: rendered, or as highlighted code.
    html       text,
    UNIQUE (library_id, path),
    UNIQUE (library_id, id),
    FOREIGN KEY (library_id, release_id) REFERENCES library_releases (library_id, id),
    CHECK (content IS NULL OR octet_length(content) = size),
    CHECK (html IS NULL OR content IS NOT NULL)
);

-- Which assets each current rule's page lists: its own files and the shared files it links to. A shared file is
-- listed by every rule that links to it.
CREATE TABLE rule_assets (
    library_id bigint NOT NULL,
    rule_id    bigint NOT NULL,
    asset_id   bigint NOT NULL,
    PRIMARY KEY (rule_id, asset_id),
    FOREIGN KEY (library_id, rule_id) REFERENCES rules (library_id, id) ON DELETE CASCADE,
    FOREIGN KEY (library_id, asset_id) REFERENCES assets (library_id, id) ON DELETE CASCADE
);

CREATE INDEX rule_assets_asset_id_idx ON rule_assets (asset_id);

-- Pages read assets as they read rule content, and ingestion replaces them as it does a library's other rows.
GRANT SELECT ON assets, rule_assets TO rulemart_catalog_reader;
GRANT SELECT, INSERT, UPDATE, DELETE ON assets, rule_assets TO rulemart_catalog_writer;

-- A library's About panel names who listed it, by the login the listing's account signed in with last.
GRANT SELECT (id, github_login) ON accounts TO rulemart_catalog_reader;

-- rulemart_catalog_reader reads tags and created_at through its SELECT on rule_versions and libraries, which 00003
-- grants, and rulemart_catalog_writer writes tags through its INSERT and UPDATE on rule_versions, which 00005 grants.
-- Ingestion never writes created_at.

-- +goose Down
-- Up-only migration; no rollback defined.
