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
- **License:** MIT, in [LICENSE.md](LICENSE.md).

Replace this copy with Code Rules' public parsing package once it ships, and delete this directory. Until then,
update it only by copying from Code Rules again, never by editing it here.
