---
title: About Rulemart
heading: About Rulemart
description: Why Rulemart exists, what Code Rules is, and how Rulemart helps you pick the rules your project follows.
eyebrow: About
---

Today, agents write almost all the code, but agents need guidance to write it well.

Most teams give that guidance through **skills**: a skill for writing good Go, a skill for designing a schema, a skill
for writing tests. Skills work, but as a delivery vehicle for guidance they're clumsy. A skill is coarse-grained, one
file holding many best practices that don't all apply every time, so it's hard to update with a single insight, hard to
adopt in part, and versioned in name only.

The fix is to make guidance granular: one insight is one **rule**.

## Rules are small

A rule describes a single best practice: what to do, when it applies, and what evidence shows it was followed. An agent
reads only the rules that apply to the work in front of it.

## Rules are versioned

Each rule carries its own version. An update says whether it changes the guidance, so work that followed the old
version may no longer comply, or only improves it.

## Rules are updatable

When a coding session goes wrong, turn the retro into a new rule or a new version of one, without touching anything
else. The feedback loop gets shorter, within a team and across the community, and more code comes out right the first
time.

## Rules are organized

Rules live in **groups**, by technology or practice, so humans and agents know when to read them.

## Rules are shareable

Rules are published in **libraries**, plain Git repositories, from which a project imports exactly the rules it wants,
inside an organization or with everyone.

That is the mental model [Code Rules]({{.CodeRulesURL}}) implements: [Fabrica](https://fabricahq.com)'s open-source
package manager for the rules you give your agents. A rule is a Markdown file that lives in a project or in a library;
a project imports the rules it wants and updates them when it chooses.

Rulemart is a visual interface over Code Rules libraries: browse the rules others have written, see what changed
between releases, collect the ones that fit your project, and check out with a prompt for your coding agent or the
commands to run yourself. The libraries it shows by default are vetted by Fabrica; read
[how vetting works]({{.VettingHref}}).

A rule in an agent's context doesn't guarantee it follows every rule to the letter; you still validate the work. But an
agent told exactly how you want code written is far more likely to write it that way.

Rulemart is free and open source [on GitHub]({{.RepositoryURL}}). We hope you find it useful, we welcome thoughtful
contributions, and we especially want [your feedback]({{.FeedbackHref}}).
