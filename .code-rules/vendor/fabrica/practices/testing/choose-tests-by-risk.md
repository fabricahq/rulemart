---
title: "Choose tests by risk and cost"
whenToRead: "Before planning, writing, or reviewing the tests for a change, or deciding whether an existing test is worth keeping, such as when adding a feature, changing complex logic, or pruning a slow or brittle suite."
impact: "HIGH"
impactDescription: "Spreading test effort evenly or chasing coverage leaves risky behavior unguarded while low-value tests slow every change."
tags: "testing"
---

## Choose tests by risk and cost

Decide what to test by weighing how likely a bug is and how much it would cost against the cost of the test.
Every test you add should fail on a plausible real regression.

### Implementation

For each behavior a change adds or modifies, judge three factors:

- **Likelihood of bugs:** complex logic, such as parsing, state lifecycles, tree and graph building, synchronization, concurrency, and calculations, is more likely to be wrong than declarative wiring or simple delegation.
- **Cost of bugs:** behavior that can lose or corrupt data, affects security or permissions, cannot be undone, or must stay consistent between two implementations of one contract costs more when it fails.
- **Cost of tests:** unit tests are cheapest to write, run, and maintain; integration tests cost more; end-to-end tests through a user interface cost the most.

Test the behaviors where likelihood or cost of bugs is high.
Before writing a test, name the plausible regression it would catch.
If you cannot name one, do not write the test.

Do not write tests that restate the implementation, retest a framework or library, or exist only to raise a coverage number.
When you touch such a test, remove it after confirming that another test covers any real behavior it checked.

When a type, schema, or generated-output check can rule out a whole class of bug, prefer extending that check over writing many similar handwritten tests.

A change to low-likelihood, low-cost code, such as display copy or a configuration value validated elsewhere, may need no new test.

### Rationale

Tests let a team change code quickly because they catch defects before users do.
Each test also has a maintenance cost, so a test that cannot fail on a real regression slows the suite without adding safety.
Spreading effort evenly, or by coverage percentage, puts as many tests on trivial code as on the code most likely to break and most costly when it does.

Background: [Testing philosophy](../../assets/testing-philosophy.md).

### Examples

#### Application: Choosing tests for a new feature

A change adds an archive action that hides an item from active lists, keeps its history, and allows restore.
It also adds a label for the new action.

**Incorrect (counterexample):**

The change adds a UI test that checks the label renders, and a unit test that asserts the archive handler calls the store's `update` method once.
Neither test checks that restore brings back the item with its history intact, which is the behavior most likely to lose user data.
The handler test also breaks if the implementation is refactored, even when archiving still works.

**Correct:**

Test that archiving hides the item from active lists, that its history survives, and that restore returns it, at a layer that exercises real persistence.
The label needs no dedicated test.

#### Application: A test written for coverage

A coverage report flags an untested class whose constructor assigns fields and whose accessors return them.

**Incorrect (counterexample):**

Add a test that constructs the object and asserts that each accessor returns the value passed in.
It restates the implementation and would catch no plausible regression.

**Correct:**

Leave the class without a dedicated test, and let the tests of behavior that uses it cover it.

An accessor that derives a value, such as a display name that falls back to an email address when the name is blank, does warrant a test.
It contains logic that can be wrong.

### Validation

For each test in a change, state the plausible regression it would catch.
Breaking that behavior in a disposable checkout should make the test fail.

A changed behavior with high likelihood or cost of bugs and no test that would catch its failure is a violation.
Inspect what the change touches, such as stored data, permissions, or irreversible actions, to judge cost.

Low coverage by itself is not a violation, and neither is the absence of tests for simple delegation or declarative configuration.
