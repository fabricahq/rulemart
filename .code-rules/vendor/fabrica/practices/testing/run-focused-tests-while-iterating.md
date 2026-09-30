---
title: "Run focused tests while iterating, and the full suite before finishing"
whenToRead: "Before running tests during implementation, debugging, or review, or before declaring a change complete."
impact: "LOW-MEDIUM"
impactDescription: "Running the whole suite on every edit slows feedback, while stopping after one passing test misses breakage elsewhere."
tags: "testing, workflow, feedback"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (use-fast-focused-test-tooling; MIT, notice retained in NOTICE.md): moved from the TypeScript group to testing, restructured to the rule template, and made the full-suite step explicit."
---

## Run focused tests while iterating, and the full suite before finishing

While changing a behavior, run the smallest set of tests that exercises it, such as one test file or one named test.
Before declaring the change complete, run the project's full checks and test suite.

### Implementation

- Use the test runner's single-file or name-filter option, such as `vitest run path/to/file.test.ts -t "name"` or `go test -run TestName ./pkg`.
- Editor integrations that run one test are an optional convenience.
- After the focused test passes, run the broader suite, linting, and type checks the project requires.
- A passing focused test shows only that the selected test passes; report it that way.

### Rationale

Fast feedback on one behavior shortens each edit-test cycle.
A change can still break other behavior, so the full suite is the check that the change is complete.

Background: [Testing philosophy](../../assets/testing-philosophy.md).

### Examples

**Incorrect (counterexample):**

Run only the edited test file, see it pass, and report the change as done.

**Correct:**

Run the edited test file while working, then run the full test suite and type check before reporting the change as done.

### Validation

Check that the final report of a change cites a full-suite run, not only a focused one.

Running only focused tests during iteration is not a violation.
