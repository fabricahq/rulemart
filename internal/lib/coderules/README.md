# Code Rules parser (temporary copy)

This directory holds the part of [Code Rules](https://github.com/fabricahq/code-rules)' parser that Rulemart needs:
release tags and their records, rule documents, group metadata, and the library manifest's license, which ingestion
reads, and the canonical group list, which the pages read. Rulemart reads libraries with the same rules Code Rules
applies when it publishes them, so the two can't disagree about what a library release contains.

- **Source:** `internal/rules` in fabricahq/code-rules at
  [v0.2.0](https://github.com/fabricahq/code-rules/tree/v0.2.0/internal/rules), commit
  `6a63c7173bb5ae8b37cf22f30b8a4aede1d6b435`. The files ingestion uses are unchanged there since commit `b6eaefc`,
  from the rule-versioning work, which they were first copied from.
- **Changes:** the package is renamed `coderules`, files and functions Rulemart doesn't use are left out, and
  imports they needed are removed. The logic is unchanged.
- **The Trail Guide directory's layout:** `library_manifest.go` is copied from fabricahq/trailguide#126's branch, commit
  `9d11c960575f49727804da70169c0612e31d3def`: `RepositoryPath`, `LibraryPath`, and their constants from
  `internal/librarytree/layout.go`, and the library section's parser from `internal/rules/licenses.go`, with that
  branch's `decode` helpers spelled as this copy names them, and its `termsPath` renamed `libraryTermsPath` beside
  the earlier layout's. Its `ParseLibraryManifest` reads only the configuration's `schemaVersion` and `library`
  fields, where Trail Guide parses the whole configuration, whose other fields configure the repository as a project.
  Its names and messages say Trail Guide and `.trailguide/`, as fabricahq/trailguide#138 renamed them there.
- **License:** MIT, in [LICENSE.md](LICENSE.md).

Replace this copy with Code Rules' public parsing package once it ships, and delete this directory. Until then,
update it only by copying from Code Rules again, never by editing it here.
