---
title: "Organize code by feature"
whenToRead: "Before planning, writing, moving, or reviewing where files live in a project, such as adding a feature, creating shared components or utilities, or choosing import paths."
impact: "MEDIUM"
impactDescription: "Code grouped by technical type scatters each feature across the tree, so changes touch many folders and shared folders fill with single-use code."
tags: "code-design, project-structure, colocation, imports"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (organize-projects-by-feature, colocate-code-by-feature, use-relative-imports-within-feature; MIT, notice retained in NOTICE.md): merged the colocation, feature-organization, and relative-import rules, moved them to code design, restructured to the rule template, and replaced the long example trees with one comparison."
---

## Organize code by feature

Group files by the feature they belong to, and keep code as close as possible to where it is used.
Move code to a shared location only when a second feature uses it.

### Implementation

- Give each feature, such as a page or domain area, its own folder with its components, hooks, utilities, API calls, and tests.
- Keep a shared folder, such as `common` or `shared`, for code that more than one feature actually uses, and keep it small.
- Move code from a feature folder to the shared folder when a second feature needs it, not in anticipation.
- In frameworks with file-based routing, keep route files thin and put the feature's implementation in its feature folder.
- Import files within the same feature with relative paths, so the feature can move without editing its internal imports.
  Import from other features and shared code with the project's path aliases.
- Let tooling sort imports rather than ordering them by hand.

### Rationale

Most changes affect one feature.
When a feature's files live together, a change stays in one folder, and deleting the feature removes one folder.
Grouping by technical type, such as all components in one folder and all hooks in another, spreads each change across the tree and makes shared folders a dumping ground.

This practice applies in any language; the TypeScript paths illustrate it.

### Examples

**Incorrect (counterexample):**

```text
src/
  components/ProductItem.tsx, ProductsStatistics.tsx, CheckoutForm.tsx
  hooks/useGetProducts.ts, useCheckout.ts
  utils/filterProductsByType.ts
```

Changing the products page touches three folders, and nothing shows which files belong to it.

**Correct:**

```text
src/
  common/components/Button.tsx
  modules/products/
    components/product-item.tsx, products-statistics.tsx
    api/use-get-products.ts
    utils/filter-products-by-type.ts
  modules/checkout/
    components/checkout-form.tsx
    api/use-checkout.ts
```

### Validation

For a typical change, check that most edited files are in one feature folder.
Check that shared folders contain only code used by more than one feature.

A small project with a few files in one folder is not a violation.
