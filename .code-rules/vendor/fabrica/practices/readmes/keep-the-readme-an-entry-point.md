---
title: "Keep the README an entry point, and link to the full documentation"
whenToRead: "Before adding or reviewing detailed material in a README, such as configuration tables, command references, FAQs, file formats, architecture notes, or build and contribution instructions, or when a README grows well past its quick start."
impact: "MEDIUM"
impactDescription: "A README that duplicates the manual drifts out of date with it, and buries the quick start under reference detail most readers do not need yet."
tags: "documentation, readme"
---

## Keep the README an entry point, and link to the full documentation

Keep in the README only what a reader needs to decide whether to use the product and to get a first result.
Put complete reference material in the documentation, and maintainer instructions in `CONTRIBUTING.md`, then link to both.

### Implementation

- **Keep in the README:** what the product is, the problem it solves, the quick start, a short summary of how it works, its limits, links into the documentation, a one- or two-sentence contributing pointer, and the license.
- **Move to the documentation:** every configuration key, every command and flag, full file formats, customization guides, troubleshooting, and FAQs beyond a few essential ones.
- **Move to `CONTRIBUTING.md`:** building from source, running tests and linters, the repository layout, and the release process.
  Create the file if the repository does not have one.
- Link each topic to its page on the published documentation site, rather than to the source folder of the site.
- Before removing a section from the README, confirm that the page it points to already covers everything the section said.
  Move anything missing to that page first, and update links elsewhere in the repository that point to the removed section.

When a product has no documentation site, a longer README is acceptable.
Still order it so the quick start comes before reference material, and keep maintainer instructions in `CONTRIBUTING.md`.

### Rationale

Material that lives in two places is updated in one.
Readers then find a README that disagrees with the documentation and cannot tell which is right.
Reference material also pushes the quick start and the product's value below the first few screens, where readers deciding whether to adopt the product never reach them.

### Examples

**Incorrect (counterexample):**

```md
## Configuration

| Key | Default | Meaning |
| --- | --- | --- |
| `region` | `us-east-1` | … |
| `currency` | `USD` | … |

## Commands

| Command | What it does |
| --- | --- |
| `init` | … |
| `report` | … |

## Building from source

Run `make build`, then …
```

The configuration and command tables repeat pages on the documentation site, and the build steps are for maintainers.

**Correct:**

```md
## Learn more

- [Configuration](https://tally.example.com/configuration/): every setting and its default
- [Commands](https://tally.example.com/reference/): every command and flag

## Contributing

Pull requests are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) explains how to build and test Tally.
```

### Validation

For each README section that holds a table of options or commands, or runs longer than a few paragraphs, check whether the documentation covers the same material.
If it does, the README should link to it instead.
Check that documentation links point to the published site.
When material was removed, check that its destination page contains it.

A short table that compares approaches or lists a few generated files a user must know about during the quick start is not a violation.
