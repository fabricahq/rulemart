---
title: "Open with what the product does for the reader"
whenToRead: "Before writing or reviewing the opening of a README, such as its title, tagline, first paragraphs, badges, and any notices or links placed above the description."
impact: "MEDIUM"
impactDescription: "Readers who cannot tell from the first screen what a product would do for them leave or misjudge it, and those who stay follow setup steps without knowing whether the product fits their needs."
tags: "documentation, readme"
---

## Open with what the product does for the reader

Open the README by saying, in plain words, what the reader gets from the product, what it works on, and which part of the job it takes off their hands.
Put nothing before that description that assumes the reader already knows the product, such as install steps, notices, pointers to the documentation, or tables.

### Implementation

- Lead with the outcome for the reader, addressed as "you", in everyday words.
- Say what the product acts on in concrete terms, such as GitHub releases, container images, or cloud bills.
- In a sentence or two, say who does what: the steps the product handles, and what stays with the reader.
- Leave the product's implementation shape, such as "a command-line tool and a generated workflow", to the sections on how it works.
  Name the shape up front only when it is what the reader is choosing, as with a library or an SDK.
- When the product has a documentation site, keep the README's description consistent with how the site's landing page describes the product.
  Reuse its framing rather than inventing a second one.
- Follow with a short paragraph that says who the product is for and names hard constraints that decide whether a reader can use it, such as supported platforms, required services, or a hosting provider.
- For a primary product, the tagline and promise make this description.
  For a secondary product, the opening paragraph makes it.
- A short status notice, such as a pre-release warning, can come directly after the description.

### Rationale

A reader deciding whether to keep reading wants to know what the product would change for them.
A description of its mechanism is accurate but makes the reader work out the benefit, and a list of components gives them nothing to picture.
Two descriptions of one product, one in the README and one on the documentation site, make readers wonder whether they describe the same thing.

### Examples

**Incorrect (counterexample):**

```md
# Shipwright

Shipwright is a command-line tool and a generated GitHub Actions workflow that deploy container images to Fly.io, evaluate health checks, and roll back the previous image on failure.
```

The sentence is accurate, but it describes Shipwright's parts and leaves the reader to work out what they would gain.
A common variant of the same mistake opens with where the documentation lives, before any description at all.

**Correct:**

```md
# Shipwright

Shipwright gets your containers onto Fly.io without the late-night rollbacks. When you merge to main, it deploys the new image, watches its health checks, and puts the previous image back on its own if they fail. You decide what merges; Shipwright handles the rest.

It is for teams that deploy to Fly.io from GitHub. It runs in GitHub Actions only.
```

A Go library that opens with "`retry` is a Go library for retrying failed calls with backoff" needs no change: the reader is choosing a library, so its shape is part of what it does for them.

### Validation

Read only the title and the opening paragraphs, or the title block for a primary product.
Someone unfamiliar with the product should be able to answer four questions: what do I get, what does it work on, what do I still do myself, and can I use it in my setup?
When the product has a documentation site, compare the opening with the site's landing page and check that both describe the product the same way.

Badges above the description are not a violation.
Neither is a link to the documentation after the description.
