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
| Skills help only when the agent picks one for a task | Rules apply to every change, so code comes out right the first time and review checks the same standard |

## Quick start

You need a Git repository and a coding agent that can run commands in it, such as Claude Code or Codex, or a
terminal to run them yourself. The prompt installs [Code Rules](https://code-rules.fabricahq.com/start-here/install/)
0.2.0 or later if the project doesn't use it yet.

1. Open [rulemart.fabricahq.com](https://rulemart.fabricahq.com) and search for what your project uses, or browse
   **Techs** and **Practices**.
2. Open a rule to read it, its versions, and what changed between them. Its library's page shows the library's
   groups, every rule, and its releases.
3. Add what you want to your cart, which needs no account: a single rule, or a rule's whole group. Picking the
   groups on a library's page adds several at once.
4. Open the cart. Choose for each rule whether it stays in sync with its library or is forked into your project,
   and enter your project's GitHub repository if you like, so the prompt names it. Signed in, pick one of your
   projects instead, each listed with the libraries it uses.
5. Copy the prompt into your coding agent, in your project's repository, or copy the commands and run them yourself.
   They set up Code Rules if the project needs it, add each library with `code-rules project add library`, fork
   rules with `code-rules project add rule --from`, and sync. The prompt also points your agent instructions, such as
   `AGENTS.md`, at the rules.

Checkout pins nothing by default: the rules you keep in sync move to newer versions only when your project runs
`code-rules project update`, which previews each change first. To hold a library at one release instead, the Commands
tab's footnote says how to add `--ref release/<number>` to its `add library` command.

Your project now has `.code-rules/generated/RULES.md`, the index your agent reads before it works, listing the rules
you picked. [Code Rules' guide](https://code-rules.fabricahq.com) covers what comes next: adding your own rules,
overriding a library's, and updating to newer versions.

## List your library

A library's maintainers can list it on Rulemart: anyone with write access to its public repository on GitHub.

1. Release your library with Code Rules, which tags each release, as
   [Code Rules' guide to libraries](https://code-rules.fabricahq.com/concepts/libraries/) explains.
2. Choose **List your library** on Rulemart's home page and sign in with GitHub. Pick the library from the
   repositories you and your organizations own, or enter the GitHub URL of one you have write access to.
3. Rulemart reads the library's release tags, usually within seconds, while a checklist shows its progress, then
   shows its pages under **Unvetted libraries**, with a warning. Rulemart checks for new releases every hour from then
   on.
4. To show the library in lists, groups, and search by default,
   [ask Fabrica to vet it](https://github.com/fabricahq/rulemart/issues/new?template=ask-to-vet-a-library.yml).

## How it works

Rulemart builds every page from the libraries' release tags on GitHub. Every hour, it checks each library for a new
release and reads what changed, so each rule's version history matches what Code Rules installs.

Fabrica vets the libraries Rulemart shows by default. Vetting is a reviewed change to
[catalog/vetted.yaml](catalog/vetted.yaml), so its history shows when each library was vetted, and why. A listed
library that isn't vetted appears under Unvetted libraries, and in lists, groups, and search only for a visitor who
chooses **Include unvetted libraries**, tagged Unvetted. Every page of it warns that it hasn't been vetted, search
engines are asked not to index it, and adding its rules to the cart takes a second confirmation. Checkout then has
your agent review its rules and wait for your approval before adding them, pinned to the commit it reviewed.

## Learn more

- [Code Rules documentation](https://code-rules.fabricahq.com): rules, groups, libraries, and the `code-rules`
  command.
- [_internal/decisions.md](_internal/decisions.md): the decisions that shape Rulemart.

## Contributing

Issues and pull requests are welcome. [CONTRIBUTING.md](CONTRIBUTING.md) explains how to run Rulemart locally, test
it, and release it.

## License

[MIT](LICENSE).
