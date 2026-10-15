# Authored rules

An **authored group** belongs to this project. Its metadata and rules live here, and you edit them directly. It needs no source declaration. Supporting files that several rules share live in .syllabus/assets/; a rule such as techs/go/errors.md links to one as ../../../assets/diagram.svg.

A **library group** comes from a versioned Git repository. The project selects it in its configuration and imports it with sync. Change its source selection or upstream library, then sync again; keep project-specific changes here.

## When groups share an ID

Authored and library definitions can contribute to the same group, such as techs/go:

- Their rules are combined. Adding an authored rule does not replace an imported rule, even when their filenames match.
- Authored _group.yaml metadata takes precedence over library metadata for the group's name, description, and reading cue.
- Use explicit exclusions or replacements in the project configuration to remove or override imported rules.

## Manage authored rules

Follow [the project guide](../README.md) to add groups and rules. Keep rules within their group's scope and follow the [rule authoring rubric](https://github.com/fabricahq/code-rules/blob/main/docs/src/content/docs/reference/rule-authoring.md).

Complete drafts before building. Run syllabus project build after editing these files, or syllabus project sync after changing a library source. Read the resulting [resolved rules](../generated/RULES.md) when working on the project.
