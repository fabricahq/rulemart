---
title: "Cover empty inputs and boundaries"
whenToRead: "Before planning, writing, changing, or reviewing code or tests that process collections, check ranges or thresholds, compute positions, or relate two items, such as validation limits, pagination, sorting, or moving items in a list or tree."
impact: "MEDIUM"
impactDescription: "Catches bugs at the edges, which tests that use only ordinary inputs never reach."
tags: "testing"
---

## Cover empty inputs and boundaries

Test each input the contract treats differently, and assert the intended result for each one.
These inputs include empty and single-item collections, first and last positions, exact thresholds, and an item related to itself.

### Implementation

Look for cases in three places:

- **Collections:** empty and single-item inputs.
  An empty input often means there is nothing to loop over or renumber, and a single item skips the logic that runs between items.
- **Positions and thresholds:** the first and last positions, each exact threshold, and the nearest meaningful values on either side.
- **Relationships:** the same item on both sides, and a missing counterpart, such as a root item that has no parent.

Use the implementation to find boundaries the contract leaves implicit, but choose and assert cases through the interface callers use.
When an edge case shows that the contract never decided the result, decide it, write the answer into the contract, enforce it in the implementation, and test that enforcement.

Cover each boundary at the lowest test layer that can prove it, usually a unit test.
Do not repeat edge-case variants in UI or end-to-end tests unless the boundary exists only at that layer.
Keep each case cheap: a table-driven or parameterized test can hold one row per boundary.
Be most thorough where a wrong edge result loses or corrupts data.

Choose representative inputs for each behavior the contract defines, and test the boundaries between those behaviors.
Avoid additional cases unless they exercise a distinct requirement or plausible failure.
An input the contract or type system rules out needs no test, such as an empty list passed to a function that only accepts a non-empty type.

When an edge-case bug gets past the tests, reproduce it with a failing test before fixing it, and keep that test.

### Rationale

Bugs often cluster at these edges because each one takes a different path through the code.
Edge cases fail in three ways: a crash, a misleading error, or a silently wrong result.
A test that only checks for "no crash" misses the last two, and a suite that only uses ordinary inputs never reaches the edge paths at all.
Asserting through the caller-facing contract, rather than the current implementation's branches, keeps these tests valid when the implementation is refactored.

Background: [Testing philosophy](../../assets/testing-philosophy.md).

### Examples

These examples describe tests in prose because the practice applies in any language.

#### Application: A range with inclusive bounds

A function accepts retry counts from 1 through 3 and rejects other counts with a documented error.

**Incorrect (counterexample):**

The only test uses a count of 2 and asserts that it is accepted.
The test still passes if the upper check is written as `count < 3`, which wrongly rejects 3, or the lower check as `count >= 0`, which wrongly accepts 0.

**Correct:**

Test 0, 1, 3, and 4, and assert the documented acceptance or error for each.
1 and 3 prove the inclusive bounds, and 0 and 4 prove the rejections.

Testing 2 is optional for boundary coverage, because 1 and 3 already represent the accepted range.
Add it when a value inside the range exercises a distinct requirement or plausible failure.

#### Application: An item related to itself

A function moves an item to just before a target item in a list.

**Incorrect (counterexample):**

The tests only reorder three distinct items.
If the function removes the moved item before looking up the target, moving an item before itself fails with a misleading "target not found" error, and no test reaches that path.

**Correct:**

Also move an item before itself and assert the documented result, such as an unchanged list.
Move the only item in a list, and move items to the first and last positions.

#### Application: A contract that never decided the edge

A function answers whether folder A is inside folder B, and its documentation does not say whether a folder is inside itself.

**Incorrect (counterexample):**

A test for A inside A asserts whatever the implementation happens to return.
The test locks in an accident: a later change that "fixes" the answer breaks the test, and nobody can tell which result callers depend on.

**Correct:**

Decide the answer from what callers need.
For example, if callers use the answer to reject moving a folder into its own subtree, A must count as inside A.
Document that result, enforce it in the implementation, and assert it in the test.

### Validation

Before judging coverage, read the contract: the documentation, types, and input validation of the code under test.
List the boundaries it defines and match each one to a test that asserts the intended result, not only the absence of a crash.

Optionally, to confirm that a boundary test works, change the comparison at that boundary in a disposable checkout, such as `<=` to `<`.
The test should fail.

A missing case is not a violation when the contract treats the input the same as a tested case and it exercises no distinct requirement or plausible failure, or when the contract or type system rules the input out.
