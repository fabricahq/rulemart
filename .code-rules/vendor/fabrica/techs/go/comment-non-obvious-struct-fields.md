---
title: "Comment struct fields whose meaning the type does not show"
whenToRead: "Before planning, writing, changing, or reviewing Go struct fields, especially fields whose nil or zero values, ownership, or relationships to other fields carry rules that other packages rely on."
impact: "MEDIUM"
impactDescription: "Invariants hidden in field meanings are rediscovered by trial and error, while filler comments teach readers to skip comments altogether."
tags: "go, comments, structs, invariants"
---

## Comment struct fields whose meaning the type does not show

Comment a struct field when its meaning includes a durable fact that its name and type cannot express, such as what a nil value means or which rule it takes part in.
Do not comment fields whose name and type already say everything.

### Implementation

Comment a field when at least one of these applies:

- **Nil or zero meaning:** a nil pointer or zero value means something specific, such as "the document has no saved versions".
- **Invariants and ownership:** the field takes part in a rule enforced elsewhere, such as "at most one draft per document" or "nil exactly when `LatestVersionID` is nil".
- **References across modules:** the field holds an identifier owned by another module or service under an explicit contract.
- **Role of a dependency:** in a service struct, what a dependency is used for here, beyond what its type says.

Also:

- Describe properties, not operations: "nil while the document has no saved versions" stays true, while "set by save-version" goes stale when the call flow changes.
  Behavior belongs in a comment on the operation.
- Put facts about the whole type, such as "versions are immutable", in the type's doc comment.
- Skip self-describing fields such as `Name` or `CreatedAt`.
- When a comment only restates the name, delete it, or rename the field.

### Rationale

Other packages build on what a field's values mean, not just its type.
When a nil pointer carries a special meaning, a reader without the comment treats it as "not loaded yet" and writes code that breaks the rule.
Comments that restate names add nothing and train readers to skip the comments that matter.

### Examples

**Incorrect (counterexample):**

```go
type Document struct {
	ID              string
	FolderID        string
	LatestVersion   *DocumentVersion
	LatestVersionID *string
	// CreatedAt is the creation timestamp.
	CreatedAt time.Time
}
```

`FolderID` and `LatestVersionID` carry rules the types cannot show, while the only comment repeats a field name.

**Correct:**

```go
type Document struct {
	ID string
	// FolderID is required; documents at the top level point at the scope's hidden root folder.
	FolderID string
	// LatestVersion is the loaded row for LatestVersionID; it is nil exactly when LatestVersionID is nil.
	LatestVersion *DocumentVersion
	// LatestVersionID is nil while the document has no saved versions.
	LatestVersionID *string
	CreatedAt       time.Time
}
```

### Validation

For each commented field, check that the comment would stay true if the methods that write the field changed but the field's meaning did not.
For each uncommented field with a pointer, zero-value, or cross-module meaning, check that its meaning is obvious from the name and type.

A self-describing field without a comment is not a violation.
