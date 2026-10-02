# Rulemart

Rulemart helps you find and adopt [Code Rules](https://code-rules.fabricahq.com) libraries: the engineering rules
coding agents follow while they write and review your code. It will be at
[rulemart.fabricahq.com](https://rulemart.fabricahq.com).

This repository is the site's source. Right now it shows one vetted library, built from its Code Rules release
tags: the library's groups and rules, and each rule's current version and version history. Every hour, it checks
the library's release tags and ingests a new release. Browsing by technology, search, and adding libraries
come later.

```text
CloudFront -> web Lambda (Function URL) -> Neon Postgres
EventBridge schedule -> worker Lambda -> SQS, one job per vetted library -> worker Lambda -> Neon Postgres
```

Fabrica's private infrastructure repositories define and deploy the AWS resources. To build, test, or change
Rulemart, see [CONTRIBUTING.md](CONTRIBUTING.md). The decisions that shape it are in
[_internal/decisions.md](_internal/decisions.md).
