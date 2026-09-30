# Rulemart agent guide

Rulemart is a catalog of public [Code Rules](https://code-rules.fabricahq.com)
libraries. See [README.md](README.md) for how it's built and released. Run
`make check` to vet and test the code.

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
