---
title: "Express operations as meaningful steps"
whenToRead: "Before planning, writing, changing, or reviewing a function that coordinates multiple steps, such as parsing input, validating it, calling another operation, or constructing a result."
impact: "MEDIUM"
impactDescription: "Mixing orchestration with low-level details can hide important decisions and make behavior harder to verify or change."
tags: "code-design"
---

## Express operations as meaningful steps

Express an operation as a sequence of meaningful steps at a consistent level of abstraction.
Put detailed parsing, validation, and normalization behind named helpers when their contracts let readers understand the caller without opening the helpers.

### Implementation

- Make the main operation readable in execution order.
  Keep orchestration focused on domain steps, such as reading a manifest, validating declarations, and resolving files.
  Keep schema details and indexed error construction inside the step that owns them.
- Use names that identify domain values and their validation state, such as `sourceFiles`, `manifest`, and `declarations`.
  Introduce intermediate values when nesting forces readers to unpack several meaningful operations from the inside out.
  Do not create a temporary variable for every obvious expression.
- Extract a helper when it centralizes a repeated invariant or lets a reader skip meaningful implementation details.
  A helper earns its place that way, not by reducing line count.
  Give every named helper a concise contract describing its result, ordering, mutation, or failure behavior where relevant.
- Avoid pass-through wrappers whose names and interfaces say no more than the expression they wrap.
  Names such as `stepOne` or `processData` add navigation without helping readers understand the caller.
- Keep extracted helpers private to their module unless other modules need them.
- Carry diagnostic context with validated data when later steps need to report where an error came from.
  Preserve distinctions such as source-relative paths versus manifest field locations, declaration order versus sorted output, and absent configuration versus invalid configuration.
- Preserve behavior during extraction, including error precedence, duplicate handling, ordering, empty input, and mutation guarantees.
  Make intentional behavior changes explicit and verify them separately.
- Test through the interface callers use, so that tests stay valid when private helpers are renamed, combined, or split.
  Do not export helpers solely for tests, or add a matching test suite for every extracted function.

A short, cohesive function does not need extraction merely because it could be divided into smaller functions.

### Rationale

When a function interleaves its overall steps with field checks, error formatting, and data reshaping, a reader has to trace every detail to learn what the function does.
Important decisions, such as which errors take precedence or where ordering changes, get lost among incidental code, so they are easy to change by accident and hard to review.
Named steps with clear contracts let a reader understand the operation from the caller, then inspect each detail in the one place that owns it.
Extraction that does not hide a meaningful operation has the opposite effect: it adds indirection without adding understanding.

### Examples

This practice applies in any language.
The TypeScript examples illustrate the structure.

#### Application: A function that validates while it orchestrates

Both versions below load an export manifest, validate its declared file paths, check that the files exist, and return unique paths in code-unit order.
They use the same existing validation utilities: `object` rejects non-objects, `nonempty` rejects non-string or blank values, `relativePath` rejects paths escaping the source root, and `requiredFile` rejects missing files.
Each utility reports errors at the supplied location.
These are excerpts from a module that imports those utilities.

**Incorrect (counterexample):**

```ts
/** Return unique declared export paths in code-unit order; reject invalid manifests or missing files. */
export function collectExportPaths(
  sourceFiles: ReadonlyMap<string, string>,
): ReadonlyArray<string> {
  const manifest = object(
    JSON.parse(requiredFile(sourceFiles, 'exports.json', 'library')),
    'exports.json',
  );
  const values = manifest.files;
  if (!Array.isArray(values)) {
    throw new Error('exports.json: files must be an array');
  }
  const entries: ReadonlyArray<unknown> = values;
  const declarations = entries.map((value, index) => {
    const location = `exports.json: files[${index}]`;
    return {
      path: relativePath(nonempty(value, location), location),
      location,
    };
  });
  for (const { path, location } of declarations) {
    requiredFile(sourceFiles, path, location);
  }
  return [...new Set(declarations.map(({ path }) => path))].sort();
}
```

Manifest interpretation and per-field validation obscure the operation's four steps.
A reader has to parse the nested calls and the indexed error construction to see that the function loads, validates, checks, and deduplicates.

**Correct:**

```ts
/** A source-relative path paired with the manifest field that declared it. */
type DeclaredPath = {
  readonly path: string;
  readonly location: string;
};

/** Return unique declared export paths in code-unit order; reject invalid manifests or missing files. */
export function collectExportPaths(
  sourceFiles: ReadonlyMap<string, string>,
): ReadonlyArray<string> {
  const manifest = exportManifest(sourceFiles);
  const declarations = exportDeclarations(manifest);
  requireDeclaredFiles(sourceFiles, declarations);
  return [...new Set(declarations.map(({ path }) => path))].sort();
}

/** Load exports.json as an object; reject a missing file, invalid JSON, or a non-object value. */
function exportManifest(
  sourceFiles: ReadonlyMap<string, string>,
): Record<string, unknown> {
  const text = requiredFile(sourceFiles, 'exports.json', 'library');
  const value: unknown = JSON.parse(text);
  return object(value, 'exports.json');
}

/** Validate declared export paths in input order, retaining duplicates and indexed error locations. */
function exportDeclarations(
  manifest: Record<string, unknown>,
): ReadonlyArray<DeclaredPath> {
  const values = manifest.files;
  if (!Array.isArray(values)) {
    throw new Error('exports.json: files must be an array');
  }
  const entries: ReadonlyArray<unknown> = values;
  return entries.map((value, index) => {
    const location = `exports.json: files[${index}]`;
    const text = nonempty(value, location);
    return { path: relativePath(text, location), location };
  });
}

/** Reject the first declaration whose path is absent from the source files. */
function requireDeclaredFiles(
  sourceFiles: ReadonlyMap<string, string>,
  declarations: ReadonlyArray<DeclaredPath>,
): void {
  for (const { path, location } of declarations) {
    requiredFile(sourceFiles, path, location);
  }
}
```

The caller names the domain steps, and each private helper owns its details behind a stated contract.
The final deduplication and sort stay inline because their behavior is apparent at the call site.
Error precedence, duplicate handling, and ordering are unchanged from the first version.

#### Application: A short function that is already clear

A function returns a user's display name, falling back to the email address when the name is blank.

**Incorrect (counterexample):**

```ts
export function displayName(user: { name: string; email: string }): string {
  return chooseName(trimName(user.name), user.email);
}

function trimName(name: string): string {
  return name.trim();
}

function chooseName(name: string, fallback: string): string {
  return name === '' ? fallback : name;
}
```

The helpers are pass-through wrappers.
Their names say no more than the expressions they wrap, so a reader must open both to learn what the function does.

**Correct:**

```ts
/** Return the trimmed name, or the email address when the name is blank. */
export function displayName(user: { name: string; email: string }): string {
  const name = user.name.trim();
  return name === '' ? user.email : name;
}
```

The function performs two steps, but both are at the same level and readable at a glance, so it needs no extraction.

### Validation

Read the main operation without opening its helpers.
Its domain steps and their order should be understandable from the helpers' names and contracts.
Then inspect each helper: its implementation should match its contract and keep the relevant details in one place.

When refactoring toward this structure, run behavior tests covering valid input, absent and malformed configuration, precise error locations, duplicates, ordering, and missing referenced files as applicable.
The tests should check observable results, not the number or names of private helpers.

Function length, helper count, or the presence of a loop is not by itself evidence of a violation.
Identify the mixed responsibilities, repeated invariant, or obscured step that makes the code harder to understand or change.
