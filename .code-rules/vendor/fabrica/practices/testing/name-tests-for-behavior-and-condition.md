---
title: "Name tests for the behavior and the condition"
whenToRead: "Before writing, changing, or reviewing test names or descriptions in any test framework."
impact: "LOW"
impactDescription: "Vague test names make failures hard to interpret and hide which behavior broke."
tags: "testing, naming, conventions"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (use-clear-should-when-test-descriptions; MIT, notice retained in NOTICE.md): moved from the TypeScript group to testing, generalized beyond one naming template, and restructured to the rule template."
---

## Name tests for the behavior and the condition

Name each test for the result it expects and the condition that produces it, such as "should return an empty list when the filter matches nothing".

### Implementation

- Include the expected behavior and the situation, in the project's chosen form, such as `it('should ... when ...')` or `TestParseDate_RejectsInvalidMonth`.
- Name the observable result, not the function or implementation step being exercised.
- Enforce the chosen form with a lint rule when the test framework's plugin supports one, such as Vitest's `valid-title` with `mustMatch`.

This is a naming convention; the exact form is a project choice, but each test should state behavior and condition.

### Rationale

A failing test's name is often the first thing a developer reads.
A name that states the behavior and condition says what broke without opening the test, and makes gaps visible when reading the list of tests.

Background: [Testing philosophy](../../assets/testing-philosophy.md).

### Examples

**Incorrect (counterexample):**

```ts
it('parseDate works');
it('after title is confirmed user description is rendered');
```

**Correct:**

```ts
it('should return the date formatted as YYYY-MM when the input is an ISO date');
it('should render the user description when the title is confirmed');
```

### Validation

Read the test names in a file and check that each states an expected result and the condition that produces it.

A different consistent form, such as Go's `TestXxx_Condition` names, is not a violation.
