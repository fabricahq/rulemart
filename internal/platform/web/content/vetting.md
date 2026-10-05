---
title: Library vetting · Rulemart
heading: Library vetting
description: What vetting a Code Rules library means, what an unvetted library is, and how to get a library vetted.
eyebrow: About
---

## Why do we need to vet libraries? {#why-vetting}

Rules are instructions a coding agent follows, so when you add a rule (or skill) to your project, you are taking guidance
written by someone else and asking your agent to execute it.

At worst, this guidance could be malicious and lead to destructive actions, exfiltration of data, or injection of known security
vulnerabilities.

That's why Rulemart manually vets each library submission.

## What changes when a library becomes vetted? {#vetting}

Rulemart's lists, groups, and search show only the libraries that [Fabrica](https://fabricahq.com) has vetted, each marked with a check,
unless you choose _{{.UnvettedOptInLabel}}_ beside the list. That choice stays in the page's address, and its links keep it, so the default view never changes.

## What are the limitations of vetting? {#limitations}

Vetting means only that a human at Fabrica has manually inspected a library and judged it worth publishing on Rulemart. Vetting does not
cover ongoing updates, so a malicious user could present a legitimate library, have it vetted, and then publish malicious rules.

If vetting becomes labor-intensive in the future, we can add automatic rule scanning, but we don't currently support that.

## Who can submit a library for review? {#unvetted}

Anyone signed in with GitHub can list a public repository that publishes a Code Rules library. Until Fabrica vets it, a
listed library appears under <a href="{{.UnvettedHref}}" rel="nofollow">unvetted libraries</a>, and in the lists,
groups, and search only once you include unvetted libraries, tagged Unvetted. While unvetted, every page containing
the library says "{{.UnvettedWarningText}}", and search engines are asked not to index it.

## How to submit a library for consideration {#get-vetted}

1. Release the library with Code Rules, which tags each release, as
   [Code Rules' guide to libraries]({{.CodeRulesLibrariesURL}}) explains.
2. {{if .CanList}}[List it on Rulemart]({{.ListHref}}){{else}}List it on Rulemart{{end}}, so its pages show, under a
   warning, and Rulemart keeps them current.
3. [Ask to vet a library]({{.AskToVetURL}}) on GitHub. 
4. Fabrica approves a library by updating [catalog/vetted.yaml]({{.VettedFileURL}}) in Rulemart's repository, so the file's history shows when each library was
   vetted, and why. When Fabrica vets the library, the next Rulemart release shows it across the site.

## Report a problem {#report}

Each library's page has **Report this library,** for rules that are harmful, misleading, or not what they claim. For
anything else, [report a problem]({{.ReportFormsURL}}). Reports are public GitHub issues, so leave out anything
private.
