---
title: About Rulemart
heading: About Rulemart
description: Why Rulemart exists, what Code Rules is, and how Rulemart helps you pick the rules your project follows.
eyebrow: About
---

**Why Rulemart.** Everyone writes software with agents now, and agents need guidance to write it well. Most teams give that
guidance through skills, which are coarse: hard to version, hard to share across teams, and all-or-nothing when
something changes.

Rulemart exists so teams can share what they've learned about writing good code as rules, inside their organization or
with everyone. Adopt the rules that fit your project, in whatever subset you want. When a coding session goes wrong,
turn the retro into a rule. The feedback loop gets shorter, within a team and across the community, and more code comes
out right the first time.

A rule in an agent's context doesn't guarantee it follows every rule to the letter; you still validate the work. But an
agent told exactly how you want code written is far more likely to write it that way.

**What.** The mechanism is [Code Rules]({{.CodeRulesURL}}), [Fabrica](https://fabricahq.com)'s open-source package
manager for engineering rules you give to AI. Rules are Markdown files that live in a project or are published in
versioned libraries on GitHub; a project imports the ones it wants and updates them when it chooses.

Teams needed a way to browse those rules, see what changed, and pick which to adopt. Rulemart is that: a visual
interface over Code Rules libraries. Collect rules in a cart and check out with a prompt for your coding agent, or the
commands to run yourself. The libraries it shows by default are vetted by Fabrica; read
[how vetting works]({{.VettingHref}}).

Rulemart is free and open source [on GitHub]({{.RepositoryURL}}). We hope you find it useful, we welcome thoughtful
contributions, and we especially want [your feedback]({{.FeedbackHref}}).
