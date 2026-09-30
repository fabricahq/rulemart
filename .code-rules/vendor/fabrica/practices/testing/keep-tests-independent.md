---
title: "Keep tests independent"
whenToRead: "Before planning, writing, debugging, or reviewing automated tests that share data, global state, or services, such as tests against a shared database, browser tests that run in parallel workers, tests that change configuration or the clock, or tests that call external services."
impact: "MEDIUM-HIGH"
impactDescription: "Tests that depend on each other or on live services fail intermittently or pass by accident, which makes every failure harder to trust and diagnose."
tags: "testing"
---

## Keep tests independent

Make each test's outcome depend only on the state it arranges itself.
A test should pass when run alone, in any order, and alongside the rest of the suite.

### Implementation

- Create the data each test reads in that test or its setup, rather than relying on data another test creates or leaves behind.
- When tests share a database or other store, give created records unique identifiers and remove them afterward, or isolate each test in its own transaction, schema, or database.
- Restore any global state a test changes, such as environment variables, feature flags, or shared singletons.
- Control time and randomness when results depend on them.
- Replace live external services with fakes at the boundary, so that results do not depend on another organization's availability, rate limits, or data.
- Put shared setup in fixtures or helper functions, never in module-level variables that one test writes and another reads.
- In browser tests, start each test from its own navigation, and assert on the records the test created rather than on counts that seeded data or a neighboring test could change.
- Do not use a serial execution mode, such as Playwright's `test.describe.serial`, to make dependent tests pass; merge them into one test or make each one self-sufficient.

A scenario whose steps must run in sequence, such as a multi-step user journey, belongs in one test rather than in a chain of tests that depend on each other's order.

A read-only fixture that no test modifies, such as a seeded reference dataset, can be shared.
A separately run check against a live sandbox service is also acceptable when it is kept out of the default suite and labeled as such.

### Rationale

Shared mutable state couples tests: one test's leftovers change another's result, so outcomes depend on order, timing, or which subset ran.
Those failures are intermittent and point away from the real cause.
A test can also pass only because an earlier test happened to create its data, and then fail when run alone.
Live services add failures that the code under test did not cause.

Background: [Testing philosophy](../../assets/testing-philosophy.md).

### Examples

#### Application: Tests chained through module state

**Incorrect (counterexample):**

```ts
let createdName: string;

test('should create a record', async ({ page }) => {
  await page.goto('/');
  createdName = await createRecord(page, 'Shared Record');
});

test('should archive the record', async ({ page }) => {
  await page.goto('/');
  await archiveRecord(page, createdName);
});
```

The second test fails when run alone, on retry, or in a different worker.

**Correct:**

```ts
test('should archive a record after creating it', async ({ page }) => {
  await page.goto('/');
  const name = `Archive journey ${test.info().testId}`;
  await createRecord(page, name);
  await archiveRecord(page, name);
});
```

One test owns its whole scenario, with data named uniquely for it.

#### Application: Tests that share a database

**Incorrect (counterexample):**

One test creates a user named `alice`.
A later test looks up `alice` and asserts her profile.
The second test fails when run alone or first, and two parallel runs can create conflicting records.

**Correct:**

Each test creates its own user with a unique identifier, asserts on that user, and removes it afterward.

#### Application: Tests that change global state

**Incorrect (counterexample):**

A test sets a global clock to a fixed date to check an expiry rule and never restores it.
Later tests that compute today's date pass or fail depending on whether they run after it.

**Correct:**

The test uses a controllable time source scoped to the test, so that no other test sees the fixed date.

#### Application: Tests that call an external service

**Incorrect (counterexample):**

A test calls a live currency-exchange service and asserts a converted amount.
It fails when the service is down or the rate changes, even though the conversion code is correct.

**Correct:**

A fake at the service boundary returns a fixed rate, and the test asserts the conversion for that rate.

### Validation

Run the test alone, then run the suite in a different order, using random ordering if the runner supports it.
If the suite normally runs in parallel, run it in parallel as well.
The results should match.

Check that each test's setup creates everything its assertions read, and that it restores any global state it changes.

Sharing a read-only fixture is not a violation.
