# Testing philosophy

This page explains the reasoning behind the rules in the Testing group.
It is background only; the obligations, exceptions, and checks are in the rules.

## Attribution

This philosophy is based on Yevgeniy Brikman's talk [Agility Requires Safety](https://www.ybrikman.com/blog/2016/02/14/agility-requires-safety/) (2016), given in NerdWallet's NerdTalks series; see also the [video](https://www.youtube.com/watch?v=4fKm6ImKml8) and [slides](https://www.slideshare.net/brikis98/agility-requires-safety).
The ideas below are paraphrased and extended for this library; the talk is his work.

## Speed is limited by safety

Automated tests work like a car's brakes.
A car can drive fast only because it can stop, and a team can change code quickly only when its tests stop a defect before it reaches users.
Brikman's point is that without automated tests, software should be assumed broken until someone proves otherwise.
With a test suite that runs on every change, each change proves the software still works, and the team learns the moment it breaks.

## The build tests itself

Every change runs the automated checks and tests before it merges.
A failing build is fixed quickly or the change that broke it is reverted, never left failing.
Small, frequent changes keep each failure easy to locate and undo.

## Fast feedback keeps the brakes in use

Brakes help only if you use them, and a team uses its tests only while they are fast enough to run on every change.
While changing a behavior, run the few tests that cover it for feedback in seconds; before finishing, run the full suite, because a change can break behavior far from where it was made.
Keeping the whole suite fast is part of keeping it useful.

## What to test is a trade-off

Testing effort is limited, so spend it where it prevents the most harm.
Three factors decide where:

- **Likelihood of bugs:** higher in complex logic, such as parsing, state lifecycles, tree and graph building, synchronization, and calculations, than in declarative wiring or simple delegation.
  Within any logic, bugs cluster at the edges where behavior changes, such as empty inputs, exact thresholds, and an item related to itself.
- **Cost of bugs:** higher where users can lose or corrupt data, where security or permissions are involved, where an action cannot be undone, or where two implementations of the same contract can silently drift apart.
- **Cost of tests:** unit tests are cheap to write and run, integration tests cost more, and end-to-end tests through a user interface cost the most to write, run, and maintain.

## Every escaped bug is a missing test

A bug that reached a user shows that the tests did not cover something that mattered.
Fixing it without a test leaves the gap open for the next change to reopen.
Writing the test first, and seeing it fail, proves the test detects the bug instead of passing by accident.

## Tests are code

Every test has a maintenance cost.
A test earns its place when it would fail on a plausible real regression.
Tests that restate the implementation, retest a framework, or exist only to raise a coverage number cost time without adding safety.
When a type system, schema, or generated-output check can rule out a whole class of bug, it is usually cheaper and more reliable than many handwritten tests.
A test's name is its failure message, so a name that states the behavior and the condition tells the reader what broke before they open the test.

## The test ladder

Test layers form a ladder from cheapest to most expensive: unit, integration, and end-to-end.
A behavior belongs on the lowest rung that can actually prove it.
Most tests end up as unit tests, some as integration tests, and a few as end-to-end tests that cover critical journeys.
A lower rung is not always enough: a fake can only prove what the fake does, so behavior enforced by a real database, file system, or browser needs a test that includes it.

## A flaky test weakens every test

A test that passes or fails depending on order, timing, or another test's leftovers teaches people to rerun failures instead of reading them.
Once failures are routinely dismissed, real failures are dismissed too, and the suite stops working as a brake.
Each test therefore arranges its own state and depends on nothing another test did.

## Tests survive refactoring

Tests that assert what callers and users observe stay valid when the implementation changes.
If a behavior-preserving refactor breaks many tests, those tests were checking implementation details.
