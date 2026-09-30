---
title: "Test at the lowest layer that proves the behavior"
whenToRead: "Before planning, writing, or reviewing a test and choosing whether it should be a unit, integration, or end-to-end test, including regression tests for bug fixes."
impact: "MEDIUM-HIGH"
impactDescription: "Tests at too high a layer make the suite slow, flaky, and imprecise, while tests at too low a layer replace the behavior they claim to prove with a fake."
tags: "testing"
---

## Test at the lowest layer that proves the behavior

Prove each behavior with the cheapest kind of test that actually exercises it.
Use a higher layer only for behavior that lower layers cannot prove.

### Implementation

Test layers, from cheapest to most expensive:

- **Unit:** pure logic with no I/O, such as parsing, calculations, decisions, and data transformations.
- **Integration:** behavior that depends on a real dependency's semantics, such as database constraints, transactions, ordering, access-control policies, or file system behavior.
- **End-to-end:** the assembled application driven through its user interface or public entry point.

Choose the lowest layer that includes the component that enforces the behavior.
A unit test with a fake cannot prove a database constraint or an access-control policy the database enforces, so test that behavior against the real dependency.

When decision logic is tangled into a UI component, request handler, or script, extract it into a function that a unit test can call.
Do not contort code solely to move a test down a layer; when the behavior only exists in the composition, test the composition.

Reserve end-to-end tests for critical user journeys, for composition that unit and integration tests each pass but that has broken before, and for regressions no lower layer can reproduce.
Do not use them for styling, copy, every create-read-update-delete permutation, or edge-case variants a lower layer can prove.
Do not re-prove the same behavior at every layer.

### Rationale

Lower layers run faster, fail more deterministically, and point directly at the broken code.
Higher layers take longer to run, fail for more incidental reasons, and cost more to maintain, so a suite built on them becomes slow enough that people stop running it.
Each layer proves only what it exercises, though: a fake proves the fake's behavior, not the real dependency's.
Choosing the lowest layer that includes the enforcing component gets the most safety for the cost.

Background: [Testing philosophy](../../assets/testing-philosophy.md).

### Examples

#### Application: Decision logic inside a user interface

A list lets users drop an item before, inside, or after a folder, depending on where in the row the pointer is released.

**Incorrect (counterexample):**

A browser test drags items to several pointer positions to check each zone and the thresholds between them.
It is slow, can fail on pixel rounding, and a failure does not show whether the calculation or the drag handling broke.

**Correct:**

Extract the calculation into a function that maps a pointer offset to a drop position, and unit-test each zone and threshold.
If drag-and-drop is a critical journey, one end-to-end test confirms that dropping an item works as a whole.

#### Application: Behavior enforced by a database

Users must not read or change another tenant's records, and the database enforces this with access-control policies.

**Incorrect (counterexample):**

A unit test runs the service against an in-memory fake store that filters by tenant.
It proves the fake filters correctly, not that the database policies do.
A browser test that checks user B cannot open user A's page only proves that a page renders.

**Correct:**

An integration test against the real database creates two tenants, then attempts reads, updates, and deletes across them and asserts each is denied.
It also confirms that the owning tenant can perform the same operations, so the denials are not vacuous.

An end-to-end test for sign-in is appropriate even though parts of sign-in have unit tests.
The behavior spans the browser, cookies, and the server session together, and no lower layer includes all three.

### Validation

For each test in a change, check whether a lower layer could exercise the same behavior with its real semantics.
If it could, the test belongs there.

Check that the test's layer includes the component that enforces the behavior.
If a fake replaces that component, the test is too low.

An end-to-end test that passes through logic unit-tested elsewhere is not a violation when its purpose is the journey or the composition.
