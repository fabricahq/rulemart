---
title: About Rulemart
heading: About Rulemart
description: Why Rulemart exists, what Code Rules is, and how Rulemart helps you pick the rules your project follows.
eyebrow: About
---

Today, agents write almost all the code, but agents need guidance to write it well.

Most teams give that guidance through **skills**: a skill for writing good Go, a skill for designing a database schema, a skill
for writing tests. Skills work, but they're not an ideal delivery vehicle for guidance.

The root problem is that skills are coarse-grained. They tend to pack many insights and practices into a single artifact. As a result,
skills are easy to share but awkward to fully understand. When authors update skills, the skill may now be in a state where its users want
some of its guidance, but not all of it. Finally, skills are technically versioned, but rarely do those versions convey breaking changes explicitly.

## Introducing Rules

The fix is to make guidance granular by capturing one insight as one **rule**. 

### Rules are small

A rule describes a single best practice: what to do, when it applies, and what evidence shows it was followed. An agent
reads only the rules that apply to the work in front of it.

### Rules are versioned

Each rule carries its own version. An update says whether it changes the guidance, so work that followed the old
version may no longer comply, or only improves it.

### Rules are shareable

Rules are published in **libraries**, plain Git repositories, from which a project imports exactly the rules it wants,
inside an organization or with everyone.

## Introducing Code Rules

We wrote [Code Rules]({{.CodeRulesURL}}), an MIT-licensed open source tool to implement this concept of rules. Code Rules
is a package manager for your agent rules.

## Introducing Rulemart

Rulemart is a way to visually browse the rules available from many Code Rules libraries so you can pick and choose exactly the rules you want,
add them to your project, and get clean, versioned updates as their authors make changes (or you can fork the rules to maintain
them yourself).

Rulemart is free and open source [on GitHub]({{.RepositoryURL}}). Anyone is free to publish their own Code Rules library on Rulemart so that 
we can all benefit from your insights about how best to guide agents. Note that all new libraries are [manually vetted]({{.VettingHref}}) to
ensure that Rulemart users are embedding safe guidance in their projects.

We hope you find Rulemart useful, we welcome thoughtful contributions, and we especially welcome [your feedback]({{.FeedbackHref}}) on how to make it more useful.
