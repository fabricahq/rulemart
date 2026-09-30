---
title: "Reproduce bugs with regression tests"
whenToRead: "Before planning, diagnosing, fixing, or reviewing the fix for a behavior defect, such as a reported bug, a failing production case, or an edge case that escaped the tests."
impact: "HIGH"
impactDescription: "Prevents a fixed bug from returning unnoticed in a later change."
tags: "testing"
---

## Reproduce bugs with regression tests

Before fixing a behavior defect, write a test that reproduces the reported failure and confirm that it fails.
Then apply the fix, confirm that the test passes, and keep the test with the change.

### Implementation

Write the test against an interface callers use, at the lowest layer that faithfully reproduces the failure.
Use an integration or UI test when a unit test cannot reproduce it, such as a defect in how components are wired together.
Do not force a pure-function extraction solely to satisfy this rule.

Confirm that the test fails for the reported reason, not because of an unrelated setup error or missing dependency.
For timing- or order-dependent bugs, control the clock, scheduling, or input order so that the test fails every time.

Name the test for the behavior it protects, not for a private helper or an incident number.
After the fix, also run related valid inputs, so that the fix does not break behavior next to the bug.

If a reliable reproduction is not yet possible, such as during an urgent production fix, record what was verified and the remaining coverage gap.
Do not claim that a test written after the fix, and never run against the old behavior, proves the regression was reproduced.

This rule applies to defects in observable behavior.
A change with no behavior to observe, such as correcting a typo in a comment, needs no regression test.

### Rationale

A test written after a fix may never have been able to fail.
Running it against the unfixed code first proves that it detects the bug.
A later change that reintroduces the bug then fails the test instead of reaching users again.

Background: [Testing philosophy](../../assets/testing-philosophy.md).

### Examples

#### Application: A logic bug a unit test can reproduce

Sorting names is supposed to ignore case, but `Beta` sorts before `alpha`.

**Incorrect (counterexample):**

Fix the comparison, then add a test containing only lowercase names.
The test would have passed before the fix, so it does not guard the bug.

**Correct:**

Before fixing, sort `alpha`, `Beta`, and `gamma`, and expect `alpha`, `Beta`, `gamma`.
Confirm that the test fails with the case-sensitive comparison, which puts `Beta` first.
Then fix the comparison and confirm that the test passes.

#### Application: A bug that appears only when components are combined

A detail page shows the previous record after the user navigates to another one, because the page passes a stale identifier to its data request.
Unit tests of the page and of the data request each pass.

**Incorrect (counterexample):**

Add another unit test that calls the data request with the correct identifier.
It passes before the fix, because the defect is in how the page supplies the identifier, not in the request.

**Correct:**

Write an integration or UI test that navigates from one record to another and asserts the second record's details.
Confirm that it shows the first record's details before the fix.

### Validation

Look for evidence that the test failed for the original symptom before the fix and passes after it.
Examples of evidence are the failing output recorded in the change description, or a commit that adds the test before the commit that adds the fix.
Check that the test asserts the reported symptom rather than a nearby behavior.

A fix without a new test does not violate this rule when it changes no observable behavior, or when it records why a reliable reproduction was not possible and what remains untested.
