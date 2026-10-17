---
title: "Give a quick start that runs as written"
whenToRead: "Before writing, changing, or reviewing install or getting-started instructions in a README, such as install commands, version numbers, setup steps, example commands, and sample output, or before a release that changes any of them."
impact: "MEDIUM-HIGH"
impactDescription: "The quick start is the first thing many users run; when it fails or installs an old version, they conclude the product does not work."
tags: "documentation, readme"
---

## Give a quick start that runs as written

Include a quick start that takes a new user from nothing to one real result in the fewest steps.
Every command in it must run as written against the current release, and every output shown must come from a real run.

### Implementation

- Place the quick start soon after the description, and after the problem statement for a primary product.
- Number the steps.
  Give each step one action and the command to paste.
- Describe each step by what it does, in terms the reader already knows.
  Name a generated file or component by what it is and does, such as "the GitHub Actions workflow that publishes releases", rather than by a name the reader has not met yet, such as "the Release workflow".
- Stop at the first real result: the smallest use that shows the product working.
  Link to the full setup guide for everything after that, such as options, permissions, and continuous integration.
- Make commands paste cleanly.
  Leave out the shell prompt in `sh` blocks, and use a `console` block when you show a command with its output.
- Install the latest release by default.
  When a step pins a version, use the current release or a placeholder such as `<version>`, and update it whenever you release.
- Copy output from a real run, trimmed if long.
  Do not write output by hand.
- When a step cannot be a command, such as changing a repository setting, describe it in a sentence and link to the detailed instructions.

### Rationale

A reader who tries the product usually starts with the quick start.
A command that fails, an example that installs a version three releases old, or output that does not match what they see ends the trial and makes them doubt the rest of the documentation.
Commands in a README have no test suite by default, so they drift unless someone runs them.

### Examples

The current release of `tally` is v0.4.0.

**Incorrect (counterexample):**

````md
1. Install:

   ```sh
   $ curl -fsSL https://tally.example.com/install.sh | sh -s -- --version v0.1.0
   ```
````

The `$` breaks pasting, and the example installs an old release.

**Correct:**

````md
1. Install the latest release:

   ```sh
   curl -fsSL https://tally.example.com/install.sh | sh
   ```

2. Import last month's bill and see spending by team:

   ```console
   $ tally report --month last
   TEAM       SPEND
   payments   $4,210
   search     $2,875
   ```
````

The output is copied from a run against v0.4.0.

### Validation

Run each command in a fresh environment, such as a new temporary directory or clean checkout, using the current release.
Compare the output with what the README shows.
Check every version number in the README against the latest release tag.
Read each step's description without its command, and check that it names what the step produces in terms a new reader knows.
When the first real result depends on a service you cannot use during the check, such as a hosted account, run every step you can and say which steps you could not run.

A pinned version is not a violation when the step's purpose is pinning and the version matches the current release.
