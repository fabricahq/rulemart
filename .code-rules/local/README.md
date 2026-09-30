# Local rules

A **local group** belongs to this project. Its metadata and rules live here, and you edit them directly. It needs no library or source declaration.

A **library group** comes from a versioned Git repository. The project selects it in its configuration and imports it with sync. Change its source selection or upstream library, then sync again; keep project-specific changes local.

## When groups share an ID

Local and library definitions can contribute to the same group, such as techs/go:

- Their rules are combined. Adding a local rule does not replace an imported rule, even when their filenames match.
- Local _group.yaml metadata takes precedence over library metadata for the group's name, description, and reading cue.
- Use explicit exclusions or replacements in the project configuration to remove or override imported rules.

## Manage local guidance

Follow [the project guide](../README.md) to add groups and rules. Keep rules within their group's scope and follow the [rule authoring rubric](https://github.com/fabricahq/code-rules/blob/main/docs/src/content/docs/reference/rule-authoring.md).

Complete drafts before building. Run code-rules project build after local edits, or code-rules project sync after changing a library source. Read the resulting [resolved rules](../generated/RULES.md) when working on the project.
