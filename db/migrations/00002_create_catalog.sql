-- +goose Up
-- The catalog: what each library's release/<n> tags published. Ingestion owns every row here. It rewrites a
-- library's rows from its tags in one transaction, upserting on each table's natural key, so a row that still
-- exists keeps its id across ingestions.

-- A library is a public repository on a code host that publishes Code Rules release tags.
CREATE TABLE libraries (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    -- The code host, and the host's ID for the repository, which survives renames and transfers. GitHub's is its
    -- numeric repository ID, written in decimal.
    host               text NOT NULL CHECK (host IN ('github')),
    host_repository_id text NOT NULL CHECK (host_repository_id <> ''),
    -- The repository's current owner and name, as the host spells them.
    owner              text NOT NULL CHECK (owner <> ''),
    name               text NOT NULL CHECK (name <> ''),
    description        text NOT NULL,
    -- Empty when the host reported no avatar on its avatar host.
    owner_avatar_url   text NOT NULL,
    -- The license rule-library.yaml declares at the latest library release: an SPDX expression, a license file, or
    -- both. Both are NULL when it declares none.
    license_expression text,
    license_file       text,
    UNIQUE (host, host_repository_id)
);

-- Hosts treat owner and repository names case-insensitively, and so do page URLs.
CREATE UNIQUE INDEX libraries_host_owner_name_key ON libraries (host, lower(owner), lower(name));

-- One release/<n> tag.
CREATE TABLE library_releases (
    id                   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    library_id           bigint NOT NULL REFERENCES libraries ON DELETE CASCADE,
    number               integer NOT NULL CHECK (number > 0),
    -- The commit the tag points to.
    commit_id            text NOT NULL,
    -- When the tag was made, from its tagger line.
    tagged_at            timestamptz NOT NULL,
    UNIQUE (library_id, number)
);

-- A group that holds or held a rule, as its _group.yaml describes it at the latest library release that has it.
CREATE TABLE library_groups (
    id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    library_id   bigint NOT NULL REFERENCES libraries ON DELETE CASCADE,
    -- The Code Rules group ID, such as techs/go or practices/testing.
    path         text NOT NULL,
    name         text NOT NULL,
    description  text NOT NULL,
    when_to_read text NOT NULL,
    UNIQUE (library_id, path)
);

-- Every rule a library release has published, including retired ones.
CREATE TABLE rules (
    id                    bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    library_id            bigint NOT NULL REFERENCES libraries ON DELETE CASCADE,
    group_id              bigint NOT NULL REFERENCES library_groups,
    -- The Code Rules rule ID: the rule's path without .md, such as practices/testing/verify-retry-limits.
    path                  text NOT NULL,
    -- The library release that retired the rule; NULL while the rule is current.
    retired_in_release_id bigint REFERENCES library_releases,
    -- The path of the rule that replaced a retired rule, when the retirement named one.
    replaced_by           text,
    UNIQUE (library_id, path),
    CHECK (replaced_by IS NULL OR retired_in_release_id IS NOT NULL)
);

-- Each version a library release published of a rule.
CREATE TABLE rule_versions (
    id                 bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    rule_id            bigint NOT NULL REFERENCES rules ON DELETE CASCADE,
    -- The library release that published this version.
    release_id         bigint NOT NULL REFERENCES library_releases,
    major              integer NOT NULL CHECK (major >= 0),
    minor              integer NOT NULL CHECK (minor >= 0),
    patch              integer NOT NULL CHECK (patch >= 0),
    change             text NOT NULL CHECK (change IN ('new', 'major', 'minor', 'patch')),
    -- One summary per change note, in note order.
    summaries          text[] NOT NULL CHECK (cardinality(summaries) > 0),
    -- The rule as this version published it. Only the current version of a current rule has content, and then it
    -- has all of it: the metadata, the original Markdown document, and the HTML Rulemart shows.
    title              text,
    impact             text,
    impact_description text,
    when_to_read       text,
    markdown           text,
    html               text,
    UNIQUE (rule_id, major, minor, patch),
    UNIQUE (rule_id, release_id),
    CHECK (num_nulls(title, impact, impact_description, when_to_read, markdown, html) IN (0, 6))
);

CREATE UNIQUE INDEX rule_versions_one_with_content ON rule_versions (rule_id) WHERE html IS NOT NULL;

-- +goose Down
-- Up-only migration; no rollback defined.
