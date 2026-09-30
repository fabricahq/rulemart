---
title: "State what the product does not do"
whenToRead: "Before writing or reviewing a README's description of what a product can do, such as its features, supported platforms and services, status, or comparisons with alternatives."
impact: "MEDIUM"
impactDescription: "Readers who find an unstated limit after adopting a product lose time and trust, and a README that lists only strengths reads as marketing."
tags: "documentation, readme"
---

## State what the product does not do

Tell readers the product's limits that would change their decision to adopt it: what it deliberately leaves to other tools, the platforms and services it does not support, and features that are planned but not available.
Claim only what the current release does.

### Implementation

- For a primary product, add a short "What *product* doesn't do" section, with one bold lead per limit and, where known, what to use instead.
- For a secondary product, add a short "Limits" list, or state a single limit in the opening paragraph.
- Describe planned features as planned, without promising dates, and keep them out of feature lists.
- Before stating that a feature is missing, check the default branch and open pull requests.
  Leave out a gap that work in progress is about to close, and ask the owner when you cannot tell.
- When comparing with alternatives, describe the difference in approach fairly, including when an alternative is the better fit.
- Leave minor limits to the reference documentation.

### Rationale

A limit a reader discovers after investing in setup costs far more than one they read before starting.
Stating limits also makes the rest of the README credible: readers trust the claims of a page that is candid about gaps.
Missing-feature limits go stale fastest, so a limit that is about to be closed misleads readers as much as an overstated feature.

### Examples

**Incorrect (counterexample):**

```md
## Features

- Works with any CI system
- Uploads build artifacts to every release
```

The product runs only on GitHub Actions, and artifact upload is planned, not released.

**Correct:**

```md
## Limits

- Runs only on GitHub Actions. GitLab CI is not supported.
- Does not upload build artifacts yet. Support is planned.
```

### Validation

Compare each capability the README claims with the current release, and confirm it works as described.
Check each stated gap against the default branch and open pull requests, and confirm it is still expected to hold.
List the limits that would stop a typical reader from adopting the product, such as an unsupported platform, and check that the README states each one.

A limit covered in the reference documentation but not the README is not a violation when it would not change a typical reader's decision.
