# Rulemart

Rulemart helps you find and adopt [Code Rules](https://code-rules.fabricahq.com) libraries: the engineering rules
coding agents follow while they write and review your code. It will be at
[rulemart.fabricahq.com](https://rulemart.fabricahq.com).

This repository is the site's source. Right now it holds the walking skeleton: the full request path with no
product features, which proves the infrastructure end to end.

```text
CloudFront -> web Lambda (Function URL) -> SQS -> worker Lambda -> Neon Postgres
EventBridge Scheduler -> web Lambda, standing in for the library-release poller
```

Fabrica's private infrastructure repositories define and deploy the AWS resources. To build, test, or change
Rulemart, see [CONTRIBUTING.md](CONTRIBUTING.md). The decisions that shape it are in
[docs/decisions.md](docs/decisions.md).
