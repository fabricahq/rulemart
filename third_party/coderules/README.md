# Code Rules parser (temporary copy)

This directory holds the part of [Code Rules](https://github.com/fabricahq/code-rules)' parser that Rulemart's
ingestion needs: release tags and their records, rule documents, group metadata, and the library manifest's license.
Rulemart reads libraries with the same rules Code Rules applies when it publishes them, so the two can't disagree
about what a library release contains.

- **Source:** `internal/rules` in fabricahq/code-rules at commit
  [`b6eaefc`](https://github.com/fabricahq/code-rules/tree/b6eaefc/internal/rules), from the rule-versioning work
  (slice 11).
- **Changes:** the package is renamed `coderules`, files and functions ingestion doesn't use are left out, and
  imports they needed are removed. The logic is unchanged.
- **License:** MIT, in [LICENSE.md](LICENSE.md).

Replace this copy with Code Rules' public parsing package once it ships, and delete this directory. Until then,
update it only by copying from Code Rules again, never by editing it here.
