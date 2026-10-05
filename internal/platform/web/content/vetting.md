---
title: Library vetting · Rulemart
heading: Library vetting
description: What vetting a Code Rules library means, what an unvetted library is, and how to get a library vetted.
eyebrow: About
lede: Which libraries Rulemart shows, what vetting one means, and how to get yours vetted.
---

## What vetting means {#vetting}

Rules are instructions a coding agent follows, so Rulemart's lists, groups, and search show only the libraries Fabrica
has vetted, each marked with a check, until you choose {{.UnvettedOptInLabel}} beside the list. That choice stays in
the page's address, and its links keep it, so the default view never changes. Vetting is a reviewed change to
[catalog/vetted.yaml]({{.VettedFileURL}}) in Rulemart's repository, so the file's history shows when each library was
vetted, and why.

Vetting covers a library's future releases too, which Rulemart shows without another review. It means Fabrica chose to
show the library, not that it checked every rule: read the rules you adopt, as you would any code you add to your
project.

## Unvetted libraries {#unvetted}

Anyone signed in with GitHub can list a public repository that publishes a Code Rules library. Until Fabrica vets it, a
listed library appears under <a href="{{.UnvettedHref}}" rel="nofollow">unvetted libraries</a>, and in the lists,
groups, and search only once you include unvetted libraries, tagged Unvetted. Every page of it says
"{{.UnvettedWarningText}}", search engines are asked not to index it, and adding its rules to your cart takes a second
confirmation. Checkout then has your coding agent review its rules and wait for your approval before adding it, pinned
to the commit it reviewed.

## Get a library vetted {#get-vetted}

1. Release the library with Code Rules, which tags each release, as
   [Code Rules' guide to libraries]({{.CodeRulesLibrariesURL}}) explains.
2. {{if .CanList}}[List it on Rulemart]({{.ListHref}}){{else}}List it on Rulemart{{end}}, so its pages show, under a
   warning, and Rulemart keeps them current.
3. [Ask to vet a library]({{.AskToVetURL}}) on GitHub. When Fabrica vets it, the next Rulemart release shows it across
   the site.

## Report a problem {#report}

Each library's page has Report this library, for rules that are harmful, misleading, or not what they claim. For
anything else, [report a problem]({{.ReportFormsURL}}). Reports are public GitHub issues, so leave out anything
private.
