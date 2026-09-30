---
title: "Keep the SQL migration directory for migration files only"
whenToRead: "Before planning, adding, moving, or reviewing files in the directory a project uses for goose SQL migrations, or adding a Go migration."
impact: "MEDIUM"
impactDescription: "Documentation and Go source mixed into a SQL-only migration directory confuse migration discovery and hide which files are migrations."
tags: "goose, migrations, sql, project-structure"
---

## Keep the SQL migration directory for migration files only

When a project uses a SQL-only directory for goose migrations, keep only numbered `.sql` migration files in it.
Put documentation, helper programs, and Go migrations elsewhere.

### Implementation

- Keep migration documentation in a project guide or next to the migration directory, not inside it.
- When a data change needs logic SQL cannot express, put the Go migration in a separate package and register it through the project's migration command.
- Alternatively, run a one-off tool and follow it with a SQL migration that enforces the end state, keeping the history SQL-only.
- When SQL and Go migrations coexist, use one version sequence, and make sure the project's status and rollback commands include every registered migration.

This is an organization policy for projects that chose a SQL-only migration directory; goose itself also supports Go migrations registered in application code.

### Rationale

The migration directory is what goose scans to find migrations, and what reviewers read to see the schema's history.
Extra files make it harder to tell what is a migration, and Go migration files in a directory run by the plain goose CLI are not compiled in, so they can be skipped or reported as errors depending on how goose is invoked.

### Examples

**Incorrect (counterexample):**

```text
db/migrations/
  001_extensions.sql
  002_users.sql
  README.md
  embed.go
```

**Correct:**

```text
db/migrations/
  001_extensions.sql
  002_users.sql
```

### Validation

List the migration directory and check that it contains only numbered `.sql` files.
Run the project's migration status command and check that it reports every migration.

A project that deliberately keeps SQL and Go migrations together, with one version sequence and a custom goose binary, is not a violation.
