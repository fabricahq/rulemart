# Rulemart

A catalog of public [Code Rules](https://code-rules.fabricahq.com) libraries. This branch holds the walking skeleton:
the full request path with no business logic, to prove the infrastructure end to end.

```text
CloudFront -> web Lambda (Function URL) -> SQS -> worker Lambda -> Neon Postgres
EventBridge Scheduler -> web Lambda, standing in for the library-release poller
```

- `cmd/web` answers through CloudFront and on the schedule, queueing a message either way.
- `cmd/worker` stores each queued message in Neon.
- `internal/hello` holds the shared message type and the Neon connection, read from an SSM parameter.

Fabrica's private infrastructure repositories define and deploy the AWS resources.

## Build and release

`make check` vets and tests the code. `make dist` builds each function under `cmd/` for `provided.al2023` on
arm64 and writes `dist/<function>.zip`, `SHA256SUMS`, and `manifest.json`. The ZIPs are reproducible.

CI builds the same assets on every push. Pushing a `v*` tag on a commit merged into `main` publishes them as a
GitHub release. A deployment pins each release's SHA-256 values, so publishing a release doesn't deploy it.
