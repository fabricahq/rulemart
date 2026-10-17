---
title: "Separate package documentation from file headers"
whenToRead: "Before writing, changing, or reviewing a comment at the top of a Go file, above the package clause, or a package's doc.go."
impact: "LOW"
impactDescription: "A file comment attached to the package clause becomes the package's documentation, and a detached package comment is lost from go doc."
tags: "go, comments, package-docs, godoc"
---

## Separate package documentation from file headers

Attach a comment directly to the `package` clause only when it documents the whole package, and start it with `Package <name>`.
Separate a comment about one file from the `package` clause with a blank line.

### Implementation

- For package documentation, put the comment directly above `package`, starting with `Package <name> ...`, in `doc.go` or the package's central file.
- For a file header, leave exactly one blank line between the comment and `package`, and do not start it with `Package <name>`.
- Keep package documentation in one file per package.
- When there is no package-level documentation to add, write a file header rather than an accidental package doc.

### Rationale

Go treats a comment immediately before the `package` clause as package documentation, shown by `go doc` and pkg.go.dev.
A comment about one file attached there becomes the package's description, and if several files do it, their comments are combined.
A package comment separated by a blank line is not documentation at all.

### Examples

#### Application: A file header

**Incorrect (counterexample):**

```go
// Projection helpers for mock fixture JSON.
package devseed
```

The comment describes one file, but Go treats it as the package's documentation.

**Correct:**

```go
// Projection helpers for mock fixture JSON.

package devseed
```

#### Application: Package documentation

**Incorrect (counterexample):**

```go
// Package devseed builds the sample data used in local development.

package devseed
```

The blank line detaches the package comment, so `go doc` shows none.

**Correct:**

```go
// Package devseed builds the sample data used in local development.
package devseed
```

### Validation

Run `go doc ./path/to/package` and check that it shows the intended package description and no file-specific text.
Search for comments attached to `package` that do not start with `Package <name>`.

A file header separated from `package` by a blank line is not a violation.
