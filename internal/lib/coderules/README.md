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
- **The Trail Guide directory's layout and the library section:** `library_manifest.go` and `skill_levels.go` are
  copied from fabricahq/trailguide at [v0.5.0](https://github.com/fabricahq/trailguide/tree/v0.5.0), commit
  `57346a9692e33737d510b77309969a353f0d187f`: `Upgrade`, `RepositoryPath`, `InSnapshotNames`, `LibraryPath`, and their
  constants from `internal/librarytree/layout.go`; the library section's parser from `internal/rules/licenses.go`,
  with its `termsPath` renamed `libraryTermsPath` beside the earlier layout's; skill levels from
  `internal/rules/skill_levels.go` and `validateCuratedSkill` from `internal/rules/curated_skills.go`, with
  `sourceNamePattern` from `internal/rules/configuration.go`; and, from `internal/skills/skills.go`, the levels,
  renamed `levelRequired`, `levelRecommended`, `levelOffered`, and `skillLevelNames`, `Directory`, renamed
  `skillsDirectory`, `ValidName`, renamed `validSkillName`, and `ValidateID`, renamed `validateSkillID`. The `decode`
  helpers are spelled as this copy names them. Its `ParseLibraryManifest` reads only the configuration's
  `schemaVersion` field, the names of its `sources`, and its `library` field, where Trail Guide parses the whole
  configuration, whose other fields configure the repository as a project. So a curated skill must name a declared
  source, but the source itself isn't validated, and `Library` has no `Curated` field, which needs each source's
  repository and ref. The skill levels are parsed, and Rulemart ignores them.
- **License:** MIT, in [LICENSE.md](LICENSE.md).

Replace this copy with Code Rules' public parsing package once it ships, and delete this directory. Until then,
update it only by copying from Code Rules again, never by editing it here.
