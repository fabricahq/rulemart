---
title: "Separate pure computation from effects"
whenToRead: "Before planning, writing, changing, or reviewing functions that transform data, especially ones that also read global state, mutate their inputs, or perform I/O such as network, storage, or UI updates."
impact: "MEDIUM"
impactDescription: "Computation mixed with hidden state and side effects is hard to test and reuse, and changes data its callers do not expect to change."
tags: "code-design, purity, side-effects, testability"
attribution:
  - url: https://github.com/mkosir/typescript-style-guide/blob/86bebd58a987e23277dba02028c0ee2d6ffb5073/website/src/pages/index.mdx
    description: "Adapted from mkosir TypeScript Style Guide guidance (keep-functions-pure-and-focused; MIT, notice retained in NOTICE.md): moved from the TypeScript group to code design, restructured to the rule template with a code example."
---

## Separate pure computation from effects

Give each data transformation one responsibility, and make its result depend only on its explicit inputs, without mutating them, reading hidden mutable state, or causing effects.
Keep effects, such as network requests, storage, and UI updates, in the code that calls the computation.

### Implementation

- Pass the values a computation needs, such as a locale, the current time, or configuration, as parameters instead of reading them from globals.
- Return new values instead of mutating inputs.
- Put I/O at the edges: load data, call the pure function, then save or render the result.
- A function whose job is an effect, such as `saveDraft`, does not need to be pure; the aim is to separate the parts that can be.

### Rationale

A pure function's behavior is fully described by its inputs and output, so it can be tested with plain values, reused in other contexts, and called in any order.
Hidden inputs make the same call return different results, and hidden mutations change data far from the function that caused it.

This practice applies in any language; the TypeScript example illustrates it.

### Examples

**Incorrect (counterexample):**

```ts
let currentLocale = 'en-US';

function formatLabels(items: Array<Item>): Array<string> {
  items.sort((a, b) => a.name.localeCompare(b.name, currentLocale));
  return items.map((item) => `${item.name} (${item.count.toLocaleString(currentLocale)})`);
}
```

The result depends on a global, and the caller's array is reordered.

**Correct:**

```ts
function formatLabels(items: ReadonlyArray<Item>, locale: string): Array<string> {
  return [...items]
    .sort((a, b) => a.name.localeCompare(b.name, locale))
    .map((item) => `${item.name} (${item.count.toLocaleString(locale)})`);
}
```

### Validation

Check that data transformations take their inputs as parameters, leave them unchanged, and perform no I/O.
Test such functions with plain values and no mocks.

A function whose purpose is an effect, such as writing to storage, is not a violation.
