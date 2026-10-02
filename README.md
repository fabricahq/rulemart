# Rulemart

Rulemart helps you find and adopt [Code Rules](https://code-rules.fabricahq.com) libraries: the engineering rules
coding agents follow while they write and review your code. It will be at
[rulemart.fabricahq.com](https://rulemart.fabricahq.com).

This repository is the site's source. It shows the vetted libraries, built from their Code Rules release tags: each
library's groups, rules, and releases, each rule's current version and version history, what changed between two
releases or two versions of a rule, every library's rules by technology or practice, and search across them. Visitors
can sign in with GitHub, and browsing needs no account. Every hour, it checks each library's release tags and ingests a new release.
Adding libraries comes later.

```text
CloudFront -> web Lambda (Function URL) -> Neon Postgres
EventBridge schedule -> worker Lambda -> SQS, one job per vetted library -> worker Lambda -> Neon Postgres
```

Fabrica's private infrastructure repositories define and deploy the AWS resources. To build, test, or change
Rulemart, see [CONTRIBUTING.md](CONTRIBUTING.md). The decisions that shape it are in
[_internal/decisions.md](_internal/decisions.md).
