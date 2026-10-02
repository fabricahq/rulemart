<h1 align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="brand/logos/rulemart-horizontal-white.svg">
    <source media="(prefers-color-scheme: light)" srcset="brand/logos/rulemart-horizontal-dark.svg">
    <img alt="Rulemart" src="brand/logos/rulemart-horizontal-dark.svg" width="240">
  </picture>
</h1>

<h3 align="center">Agent coding best practices, off the shelf.</h3>

<p align="center">
  Find the engineering rules other teams wrote for their coding agents, see who publishes them, and add them to your
  project with one prompt, so your agent follows proven practices from the first line it writes.
</p>

<p align="center">
  <a href="https://github.com/fabricahq/rulemart/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/fabricahq/rulemart/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-blue.svg"></a>
</p>

<p align="center">
  <a href="https://rulemart.fabricahq.com">Rulemart</a> ·
  <a href="#quick-start">Quick start</a> ·
  <a href="#list-your-library">List your library</a> ·
  <a href="https://code-rules.fabricahq.com">Code Rules docs</a>
</p>

## Why Rulemart?

Coding agents write code fast, but not the way your team would: they skip the error handling you expect, test the
wrong things, and repeat mistakes you have already corrected in review. [Code Rules](https://code-rules.fabricahq.com)
fixes that by giving your agent your team's rules before it writes code. But writing a good set of rules from scratch,
for every technology you use, is slow work that other teams have already done.

Rulemart is the catalog of those rules. Teams publish their rules as Code Rules libraries on GitHub, and Rulemart
shows each library's rules, who publishes it, and how each rule changed over time.

| Without Rulemart | With Rulemart |
| --- | --- |
| Write every rule yourself | Pick rules other teams proved |
| Agent skills bundle many practices in one file | Each rule is one practice, so you adopt exactly what fits |
| Skills change under you, with no versions | Each rule is versioned, and updates show what changed |
| Skills live in one project or one person's folder | Libraries are shared across projects and teams, with their source recorded |

## Quick start

You need a Git repository and a coding agent that can run commands in it, such as Claude Code or Codex. The agent
installs [Code Rules](https://code-rules.fabricahq.com/start-here/install/) 0.2.0 or later, after asking you, if
it's missing.

1. Open [rulemart.fabricahq.com](https://rulemart.fabricahq.com) and search for what your project uses, or browse
   **Techs** and **Practices**.
2. Open a rule to read it, its versions, and what changed between them. Its library's page shows the library's
   groups, every rule, and its releases.
3. Sign in with GitHub, and add what you want to your cart: a single rule, a group of rules, or a whole library.
4. Open the cart and check out. Copy the prompt it gives you.
5. Paste the prompt into your coding agent, in your project's repository. The agent sets up Code Rules, adds each
   library to `.code-rules/config.yaml` pinned to the release you saw, runs `code-rules project sync` and
   `code-rules project check`, and points your agent instructions, such as `AGENTS.md`, at the rules.

Your project now has `.code-rules/generated/RULES.md`, the index your agent reads before it works, listing the rules
you picked. [Code Rules' guide](https://code-rules.fabricahq.com) covers what comes next: adding your own rules,
overriding a library's, and updating to newer versions.

## List your library

Anyone can list a public Code Rules library on Rulemart.

1. Release your library with Code Rules, which tags each release, as
   [Code Rules' guide to libraries](https://code-rules.fabricahq.com/concepts/libraries/) explains.
2. Choose **List your library** on Rulemart's home page, sign in with GitHub, and enter the repository as
   `owner/name`.
3. Rulemart reads the library's release tags, usually within seconds, and shows its pages under **Unvetted
   libraries**, with a warning. **Your listings** shows how that went, and Rulemart checks for new releases every
   hour from then on.
4. To show the library across the site, in its lists, groups, and search,
   [ask Fabrica to vet it](https://github.com/fabricahq/rulemart/issues/new?template=ask-to-vet-a-library.yml).

## How it works

Rulemart builds every page from the libraries' release tags on GitHub. Every hour, it checks each library for a new
release and reads what changed, so each rule's version history matches what Code Rules installs.

Fabrica vets the libraries Rulemart shows by default. Vetting is a reviewed change to
[catalog/vetted.yaml](catalog/vetted.yaml), so its history shows when each library was vetted, and why. A listed
library that isn't vetted appears only under Unvetted libraries: every page warns that it hasn't been vetted, search
engines are asked not to index it, and adding its rules to the cart takes a second confirmation. Checkout then has
your agent review its rules and wait for your approval before adding them.

## Learn more

- [Code Rules documentation](https://code-rules.fabricahq.com): rules, groups, libraries, and the `code-rules`
  command.
- [_internal/decisions.md](_internal/decisions.md): the decisions that shape Rulemart.
- [_internal/realignment.md](_internal/realignment.md): the plan that brings the site to its intended design.

## Contributing

Issues and pull requests are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) explains how to run Rulemart locally, test
it, and release it.

## License

[MIT](LICENSE).
