---
title: "Match the README's presentation to the product's tier"
whenToRead: "Before writing, restructuring, or reviewing a README's overall layout and tone, such as its title block, badges, opening pitch, section headings, and feature descriptions."
impact: "MEDIUM"
impactDescription: "A flagship product presented like a manual loses readers before they see its value, and a supporting tool presented like a flagship overstates its importance and hides the facts its readers came for."
tags: "documentation, readme"
---

## Match the README's presentation to the product's tier

Present a primary product's README as a landing page, and a secondary product's README as plain information.
The product's owner decides its tier; the README follows that decision.

### Implementation

#### Find the tier

- A **primary** product is one the organization wants people to discover and adopt for its own sake: a flagship it would lead with in a launch announcement.
- A **secondary** product is everything else: a tool, library, plugin, integration, or content collection that readers usually reach through a primary product or a specific task.
- Use the tier the owner states, in the task or in the repository's agent instructions, such as a line in `AGENTS.md` that says `README tier: secondary`.
- If no tier is stated, ask the owner.
  When you cannot ask, write a secondary README and say in your summary or pull request that you assumed the secondary tier.
- Do not infer the tier from the repository's size, popularity, or how polished its current README is.

#### Primary: a landing page

- Open with a centered title block: the product name, a one-line tagline that says what the product is, a one- or two-sentence promise, a few status badges such as build and license, and a row of links to the documentation, the quick start, and the reference.
- Follow with the problem the reader has felt, in a few sentences, and how the product changes it.
  A before-and-after comparison, such as a short table, often shows the change better than a feature list.
- Add an image or terminal recording near the top when one shows the product working.
- Describe features as short jobs, one line each, with a bold lead.
- End with a short "Learn more" list that points into the documentation.

#### Secondary: plain information

- Open with a plain `#` title and one or two short paragraphs: what the product does for the reader, then who it is for and what it requires.
  Link the primary product it serves, if any.
- Keep badges to status facts, such as license and build.
  Leave out the centered title block, the tagline heading, the navigation row, and comparison tables.
- Use plain descriptive or question headings, such as "How it works" or "How do I set it up?"
- Write plainly and warmly, without hype.
  Speak to the reader's benefit in concrete terms, and replace claims such as "powerful" or "effortless" with what the product does.
- If you include a "Why use it?" section, start from what the reader is trying to get done.
  Then say how the product divides the work: what it automates, what it checks, and what it leaves to the reader.
  Keep comparisons with alternatives out of this section; state them fairly with the product's limits.

Both tiers carry the same substance: what the product does for the reader, a quick start, its limits, and links to the full documentation.
Only the presentation and length differ.

### Rationale

Readers of a primary product are often deciding whether to adopt it.
The README has to earn their attention quickly, so it leads with the problem, the promise, and proof that the product works.

Readers of a secondary product usually arrive with a task, often from a primary product's documentation.
Promotional presentation slows them down, and it blurs the organization's product line: when every repository presents itself as a flagship, readers cannot tell which products matter most.

The tier is a product decision, not a writing decision, so it belongs to the owner.

### Examples

#### Application: A secondary product with a flagship presentation

`lint-bridge` is a secondary product that runs a company's primary linter, Acme Lint, in continuous integration.

**Incorrect (counterexample):**

```md
<h1 align="center">lint-bridge</h1>
<h3 align="center">Supercharge your CI in seconds.</h3>
<p align="center"><a href="…">Docs</a> · <a href="#quick-start">Quick start</a> · <a href="…">Why lint-bridge?</a></p>

## Why lint-bridge?

Tired of slow, noisy pipelines? lint-bridge changes everything.
```

The title block competes with Acme Lint's own README, and the pitch never says what lint-bridge does.

**Correct:**

```md
# lint-bridge

lint-bridge puts [Acme Lint](https://example.com/acme-lint)'s findings where you already review code: it runs Acme Lint in GitHub Actions and posts each finding as a comment on the pull request.

It is for teams that already run Acme Lint locally.

## How do I set it up?
```

#### Application: A primary product presented like a manual

Tally is a company's primary product, a command-line tool that tracks cloud spending by team.

**Incorrect (counterexample):**

```md
# Tally

The full documentation is in `docs/`.

## Configuration

| Key | Default | Meaning |
| --- | --- | --- |
```

A reader deciding whether to adopt Tally meets a configuration table before learning what problem it solves.

**Correct:**

```md
<h1 align="center">Tally</h1>
<h3 align="center">Know what every team spends on the cloud.</h3>
<p align="center">Tally reads your cloud bills and attributes every dollar to the team that owns it, every day.</p>
<p align="center"><a href="https://tally.example.com">Docs</a> · <a href="#quick-start">Quick start</a> · <a href="https://tally.example.com/reference">Reference</a></p>

## Why Tally?

Finance sees one bill. Engineering sees hundreds of services. Nobody can say which team's work costs what.
```

### Validation

- Find the tier the owner stated.
  If none was stated, check that the change says which tier it assumed.
- For a primary README, check that the first screen has the tagline, the promise, and the links row, and that the problem statement comes before the quick start.
- For a secondary README, check for a plain `#` title, an opening of one or two short paragraphs, and no centered title block, navigation row, comparison table, or promotional adjectives.

A secondary README with a license badge and a build badge is not a violation.
Neither is a primary README without an image, or a primary README that is short.
