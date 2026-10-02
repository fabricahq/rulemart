# Release policy

Release Planner's agent reads this file before every release. Rulemart is a website, so a release is the pair of Lambda functions that serve it, not a library anyone imports. Treat the contract below as what deployments and visitors depend on, and flag uncertainty instead of inventing compatibility guarantees.

## Breaking changes

A change is breaking when it changes any of these in a way that forces Fabrica's infrastructure repository to change before or while it deploys the release:

- The release files: `web.zip` and `worker.zip`, `SHA256SUMS`, and `manifest.json`, including their names and format
- Each function's runtime, architecture, and handler
- The environment variables the functions read, and the AWS permissions they need
- The Postgres roles the functions connect as or migrations grant to, which infrastructure creates
- The schema older releases need: a migration that removes a table, column, or grant an older release uses, so rolling back to that release no longer works

Migrations run before a release is published, while the previous release still serves the site, so every migration must work with the previous release. One that breaks it is a bug to fix before releasing, not a breaking change to announce: add the new shape in one release, and remove the old one in a later release, once no release that might run or be rolled back to uses it.

For visitors, removing or moving a page's URL is breaking too, because people and search engines link to it.

Tests, CI, the Makefile, and internal docs aren't part of the contract.

## Choosing a version

Versions follow [SemVer 2.0.0](https://semver.org/), with Git tags `vMAJOR.MINOR.PATCH`. Releases v0.0.1 and v0.0.2 predate Release Planner, so the next release is newer than v0.0.2.

Before 1.0.0:

- Minor: any breaking change or new feature
- Patch: bug fixes, documentation, and internal changes

From 1.0.0:

- Major: any breaking change
- Minor: new features, such as new pages or catalog data
- Patch: bug fixes, documentation, and internal changes

## Who reads the release notes

- Fabrica's operators, who decide whether to deploy a release by pinning it in the infrastructure repository
- People following Rulemart's development

## Order of the release notes

1. New features
2. Improvements
3. Bug fixes
4. Breaking changes, with what the infrastructure repository must change before deploying

## Always and never

- Always say when a release adds migrations, and whether any of them changes a table the previous release reads.
- Never mention dependency updates unless they fix a security issue.
