# Rulemart agent guide

Rulemart is a catalog of public [Code Rules](https://code-rules.fabricahq.com)
libraries. See [CONTRIBUTING.md](CONTRIBUTING.md) to build, test, migrate, and
release it, and [docs/decisions.md](docs/decisions.md) for the decisions that
shape it. Run `make db`, then `make check`, to vet and test the code.

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
