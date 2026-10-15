---
title: "Add operation and identifier context to errors at boundaries"
whenToRead: "Before writing, changing, or reviewing Go code that returns errors from database, network, filesystem, parsing, or cross-package calls."
impact: "MEDIUM"
impactDescription: "A bare error that reaches a log without context cannot be traced to the operation or record that failed."
tags: "go, errors, diagnostics, logging"
---

## Add operation and identifier context to errors at boundaries

When an error crosses a package, persistence, network, filesystem, or parsing boundary, wrap it with the operation that failed and the stable identifiers involved.

### Implementation

- Name the operation in domain terms, such as `promote document draft`, not the function that failed.
- Include identifiers that narrow the failure, such as record IDs, paths, routes, or query names.
- Use a consistent shape, such as `"<operation> key=%q: <specific failure>: <cause>"`; the `key=%q` form is a convention that keeps logs searchable.
- Wrap once per boundary; avoid chains like `promote draft: commit promotion: tx commit: commit: ...`.
- Follow Go's error string style: lowercase, no trailing punctuation.
- Keep error strings for logs and debugging, and map them to user-facing messages at the transport or UI boundary.
- Never include secrets, tokens, credentials, or raw request bodies.

### Rationale

Errors are usually logged by a caller far from where they happened.
A bare `return err` from a transaction commit produces a log line that says only what the driver saw, with no clue which operation or record was involved.
Adding context at each boundary lets one log line identify the failure without reproducing it.

### Examples

**Incorrect (counterexample):**

```go
if err := tx.Commit(ctx); err != nil {
	return err
}
```

The log shows a commit failure with no indication of which draft or document it belonged to.

**Correct:**

```go
if err := tx.Commit(ctx); err != nil {
	return fmt.Errorf("promote document draft documentID=%q versionID=%q: commit transaction: %v", documentID, versionID, err)
}
```

### Validation

Search boundary code for `return err` and bare sentinel returns, and check that each adds operation and identifier context or is returned from a function whose caller adds it.
Read a sample of production log lines for errors and check that each identifies the operation and record.

Returning an error unchanged from a small private helper whose caller adds context is not a violation.
