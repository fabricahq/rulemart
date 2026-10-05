---
title: About Rulemart
heading: About Rulemart
description: Why Rulemart exists, what Code Rules is, and how Rulemart helps you pick the rules your project follows.
eyebrow: About
---

Today, agents write almost all the code, but agents need guidance to write it well.

Most teams give that guidance through **skills**: a skill for writing good Go, a skill for designing a schema, a skill
for writing tests. Skills work, but as a delivery vehicle for guidance they're clumsy. A skill is coarse-grained: one
file holds many best practices, and not all of them apply every time. That makes a skill hard to update, because one
insight from a coding session has to be folded into the whole file, and hard to share, because whoever adopts it takes
everything or nothing. And while skills carry versions, people tend to disregard them and assume that latest is best.

The fix is to make guidance granular: one insight is one **rule**. A rule has its own versions, so an update can say
whether it changes the guidance or only improves it. Rules are organized into **groups** so humans and agents know when
they apply, and published in **libraries**, plain Git repositories, from which a project pulls exactly the rules it
wants.

That is the mental model [Code Rules]({{.CodeRulesURL}}) implements: [Fabrica](https://fabricahq.com)'s open-source
package manager for the rules you give your agents. A rule is a Markdown file that lives in a project or in a library;
a project imports the rules it wants and updates them when it chooses.

Rulemart exists so teams can share what they've learned about writing good code as rules, inside their organization or
with everyone. It's a visual interface over Code Rules libraries: browse the rules others have written, see what
changed between releases, collect the ones that fit your project, and check out with a prompt for your coding agent or
the commands to run yourself. When a coding session goes wrong, turn the retro into a rule. The feedback loop gets
shorter, within a team and across the community, and more code comes out right the first time.

A rule in an agent's context doesn't guarantee it follows every rule to the letter; you still validate the work. But an
agent told exactly how you want code written is far more likely to write it that way.

The libraries Rulemart shows by default are vetted by Fabrica; read [how vetting works]({{.VettingHref}}).

Rulemart is free and open source [on GitHub]({{.RepositoryURL}}). We hope you find it useful, we welcome thoughtful
contributions, and we especially want [your feedback]({{.FeedbackHref}}).
