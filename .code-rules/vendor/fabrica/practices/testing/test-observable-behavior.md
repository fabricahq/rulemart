---
title: "Test observable behavior"
whenToRead: "Before planning, writing, changing, or reviewing automated tests, or refactoring code that has tests, such as deciding what a test should assert or which dependencies to replace with test doubles."
impact: "HIGH"
impactDescription: "Keeps tests useful during refactoring and focused on outcomes users depend on."
tags: "testing"
attribution: [{"url":"https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx","description":"Underlying TypeScript Style Guide material by mkosir, including its snapshot-testing guidance, adapted under MIT; copyright and permission notice retained in NOTICE.md."}]
---

## Test observable behavior

Test the behavior a unit exposes to its callers or users, not the private steps that implement it.
Arrange a controlled starting state, perform the behavior through the interface callers use, and assert the results that establish it.

### Implementation

- Assert results that callers or users can observe: return values, raised errors, stored state read back through the public interface, messages sent across a boundary, and what a user interface displays.
- Do not assert calls to private helpers, internal module structure, or intermediate state that callers cannot see.
- Replace a dependency with a test double only at a boundary the code does not own or cannot run in a test, such as a payment provider or an email service.
  Assert an interaction with that double only when the interaction is itself part of the contract, such as charging a card exactly once.
- In user interface tests, find elements the way users do, by visible text, label, or accessible role, and assert what the user sees.
  Avoid selectors tied to markup structure or styling classes.
- Use several assertions in one test when together they establish one behavior.
  Do not add assertions about incidental details that callers do not depend on.
- Avoid large snapshot assertions of rendered output or data structures; they assert every incidental detail, fail on harmless changes, and invite updating the snapshot without review.
  A small snapshot whose whole content is the contract, such as a critical design-system element or a serialized wire format, is acceptable.

A unit test of an internal module is appropriate when it tests that module's own contract, such as a pure helper with documented results.
The rule concerns the interface of the unit under test, not whether a package exports it.
Do not export a private helper solely so that a test can call it.

### Rationale

Implementation details change during refactoring even when behavior does not.
A test that asserts them breaks on harmless changes, which teaches people to update or ignore failing tests, and it can still pass when the observable behavior is wrong.
A test that asserts observable results fails only when something a caller or user depends on changes.

Background: [Testing philosophy](../../assets/testing-philosophy.md).

### Examples

#### Application: Asserting results instead of helper calls

A checkout function totals the items in a cart, applies a discount, and returns a receipt.

**Incorrect (counterexample):**

Assert that checkout calls a private `calculateTotal` helper exactly once.
Renaming or inlining the helper breaks the test even though checkout still works, and the test still passes if the total is wrong.

**Correct:**

Give checkout two items and a discount, then assert the total and the contents of the returned receipt.

#### Application: Replacing an external service

Checkout charges the customer through a payment provider.

**Incorrect (counterexample):**

Replace the internal pricing module with a mock that returns a fixed total, and assert that a payment request was sent.
The test no longer checks pricing, and it breaks if pricing moves to another module.

**Correct:**

Replace only the payment provider with a fake, run the real pricing code, and assert that the fake received exactly one charge for the correct amount.
Charging once is part of the contract, so asserting that interaction is appropriate.

#### Application: A snapshot of rendered output

A component renders a user's profile card.

**Incorrect (counterexample):**

Render the card and assert that its full markup matches a stored snapshot.
Changing a class name or wrapper element fails the test, and the usual fix is to update the snapshot without checking what changed.

**Correct:**

Assert the user's name, role, and avatar alternative text, which are what users rely on.

#### Application: A user interface test

A form shows an error message when a required field is left empty.

**Incorrect (counterexample):**

Find the field by its position in the markup, submit the form, and assert that a component's internal `hasError` flag is true.
Restructuring the markup or renaming the flag breaks the test, and the test still passes if no message appears.

**Correct:**

Find the field by its label, submit the form, and assert that the error message is visible.

### Validation

For each assertion, name what a caller or user would see go wrong if the assertion failed.
An assertion with no such answer checks an implementation detail.

Check that renaming, inlining, or splitting private helpers while preserving behavior would leave the test valid.

Asserting an interaction is not a violation when the interaction is part of the contract.
Testing an internal module through its own documented contract is not a violation either.
