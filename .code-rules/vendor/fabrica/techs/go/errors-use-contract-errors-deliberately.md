---
title: "Expose error identity only for contract errors"
whenToRead: "Before defining sentinel or typed errors in Go, or writing, changing, or reviewing code that wraps errors with %w or checks them with errors.Is or errors.As."
impact: "MEDIUM"
impactDescription: "Wrapping implementation errors with %w makes driver and library errors part of a package's API, so callers depend on details that change with the implementation."
tags: "go, errors, api-design, wrapping"
---

## Expose error identity only for contract errors

Let callers inspect an error's identity, through `%w`, `errors.Is`, or `errors.As`, only when the error is part of the package's contract.
Translate implementation errors, such as database driver errors, into contract errors or wrap them with `%v`.

### Implementation

- Define a contract error only when a caller maps it to behavior, such as an HTTP status, a UI state, a retry, or a recovery path.
- Wrap contract errors with `%w`, such as `fmt.Errorf("load document id=%q: %w", id, store.ErrNotFound)`.
- At package boundaries, translate implementation errors, such as `pgx.ErrNoRows`, `sql.ErrNoRows`, or `*fs.PathError`, into the package's contract errors when they have a meaning callers need.
- Wrap other internal failures with `%v`, so the text is kept for logs but the identity is not exposed.
- Choose contract errors by meaning, such as not found, forbidden, conflict, or invalid input.

### Rationale

`%w` makes the wrapped error visible to `errors.Is` and `errors.As`, so callers can and will depend on it.
A driver error exposed this way ties every caller to the current database library, and swapping the driver silently changes behavior that callers branch on.
Translating at the boundary keeps the contract stable while the implementation changes.

### Examples

**Incorrect (counterexample):**

```go
document, err := queries.GetDocument(ctx, documentID)
if err != nil {
	return Document{}, fmt.Errorf("load document documentID=%q: %w", documentID, err)
}
```

Callers can now test for `pgx.ErrNoRows`, which makes the driver's error part of this package's API.

**Correct:**

```go
document, err := queries.GetDocument(ctx, documentID)
if errors.Is(err, pgx.ErrNoRows) {
	return Document{}, fmt.Errorf("load document documentID=%q: %w", documentID, store.ErrNotFound)
}
if err != nil {
	return Document{}, fmt.Errorf("load document documentID=%q: %v", documentID, err)
}
```

The missing row becomes the store's `ErrNotFound`, and other driver errors keep their text without exposing their identity.

### Validation

Search package boundaries for `%w` and check that each wrapped error is a documented contract error.
Check that callers branch only on contract errors, never on driver or library errors from another layer.

Wrapping with `%w` inside one package, where the error does not cross its boundary, is not a violation.
