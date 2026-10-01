-- +goose Up
-- The catalog: what each library's release/<n> tags published. Ingestion owns every row here, and rewrites a
-- library's rows from its tags in one transaction.

-- A library is a public GitHub repository that publishes Code Rules release tags.
CREATE TABLE libraries (
    -- GitHub's repository ID, which survives renames and transfers.
    github_id          bigint PRIMARY KEY CHECK (github_id > 0),
    -- The repository's current owner and name, as GitHub spells them.
    owner              text NOT NULL CHECK (owner <> ''),
    name               text NOT NULL CHECK (name <> ''),
    description        text NOT NULL,
    -- Empty when GitHub reported no avatar on its avatar host.
    owner_avatar_url   text NOT NULL,
    -- The license rule-library.yaml declares at the latest library release: an SPDX expression, a license file, or
    -- both. Both are NULL when it declares none.
    license_expression text,
    license_file       text
);

-- GitHub treats owner and repository names case-insensitively, and so do page URLs.
CREATE UNIQUE INDEX libraries_owner_name_key ON libraries (lower(owner), lower(name));

-- One release/<n> tag.
CREATE TABLE library_releases (
    library_id bigint NOT NULL REFERENCES libraries ON DELETE CASCADE,
    number     integer NOT NULL CHECK (number > 0),
    -- The commit the tag points to.
    commit_id  text NOT NULL,
    -- When the tag was made, from its tagger line.
    tagged_at  timestamptz NOT NULL,
    PRIMARY KEY (library_id, number)
);

-- A group that holds at least one current rule, as its _group.yaml describes it at the latest library release.
CREATE TABLE library_groups (
    library_id   bigint NOT NULL REFERENCES libraries ON DELETE CASCADE,
    -- Such as techs/go or practices/testing.
    group_id     text NOT NULL,
    name         text NOT NULL,
    description  text NOT NULL,
    when_to_read text NOT NULL,
    PRIMARY KEY (library_id, group_id)
);

-- Every rule a library release has published, including retired ones.
CREATE TABLE rules (
    library_id  bigint NOT NULL REFERENCES libraries ON DELETE CASCADE,
    -- The library rule ID: the rule's path without .md, such as practices/testing/verify-retry-limits.
    rule_id     text NOT NULL,
    group_id    text NOT NULL,
    -- The library release that retired the rule; NULL while the rule is current.
    retired_in  integer,
    -- The rule that replaced a retired rule, when the retirement named one.
    replaced_by text,
    PRIMARY KEY (library_id, rule_id),
    FOREIGN KEY (library_id, retired_in) REFERENCES library_releases (library_id, number),
    CHECK (replaced_by IS NULL OR retired_in IS NOT NULL)
);

-- Each version a library release published of a rule.
CREATE TABLE rule_versions (
    library_id         bigint NOT NULL,
    rule_id            text NOT NULL,
    major              integer NOT NULL CHECK (major >= 0),
    minor              integer NOT NULL CHECK (minor >= 0),
    patch              integer NOT NULL CHECK (patch >= 0),
    -- The library release that published this version.
    release            integer NOT NULL,
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
    PRIMARY KEY (library_id, rule_id, major, minor, patch),
    UNIQUE (library_id, rule_id, release),
    FOREIGN KEY (library_id, rule_id) REFERENCES rules ON DELETE CASCADE,
    FOREIGN KEY (library_id, release) REFERENCES library_releases (library_id, number),
    CHECK (num_nulls(title, impact, impact_description, when_to_read, markdown, html) IN (0, 6))
);

CREATE UNIQUE INDEX rule_versions_one_with_content ON rule_versions (library_id, rule_id) WHERE html IS NOT NULL;

-- +goose Down
-- Up-only migration; no rollback defined.
