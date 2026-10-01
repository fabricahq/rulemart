# Rulemart agent guide

Rulemart is a catalog of public [Code Rules](https://code-rules.fabricahq.com)
libraries. See [CONTRIBUTING.md](CONTRIBUTING.md) to build, test, migrate, and
release it, and [_internal/decisions.md](_internal/decisions.md) for the
decisions that shape it. Run `make db`, then `make check`, to vet and test the
code.

README tier: secondary. The site is the product's landing page; the README
serves people working on this repository.

## Engineering rules

Before planning, implementing, reviewing, testing, or debugging a change:

1. Read `.code-rules/generated/RULES.md` and follow its instructions to
   select relevant groups and read their rules in full, including linked
   files and additional index pages.
2. Follow the applicable rules and their exceptions while doing the work.
3. Before finishing, check your work against those rules and run the
   relevant validation. Briefly report what you verified and any gaps.

If required rule files are unavailable or give conflicting instructions,
report the issue rather than silently skipping them or choosing a policy.

To add, change, or update rules, follow [.code-rules/README.md](.code-rules/README.md).

<!-- release-planner:begin v0.5.0 sha256:5911c1f9642cb6bb -->
## Releases

This repository publishes releases with [Release Planner](https://github.com/fabricahq/release-planner) v0.5.0. When asked to make a release, draft, revise, or correct release notes, or retry a failed release, print the release procedure and follow it:

```sh
release-planner guide
```

First check that `release-planner version` prints `v0.5.0`. If it doesn't, or `release-planner` isn't installed, install that version:

```sh
curl -fsSL https://raw.githubusercontent.com/fabricahq/release-planner/v0.5.0/install.sh | sh -s -- --version v0.5.0
```

Read `.release-planner/policy.md` first for this repository's release policy. You prepare the release pull request; the maintainer approves the release by merging it. Never tag, publish, or merge.
<!-- release-planner:end -->
