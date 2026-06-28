# Best Practices — Maintainability, Readability, Consistency

How to keep a DBML schema clean as it grows. Cross-cutting principles; for naming specifics see `naming.md`, for data shape see `normalization.md`, for an audit see `review.md`.

---

## Maintainability

- **Treat DBML as the source of truth; generated SQL as a derived artifact.** Keep enrichment (notes, groups, sticky notes) in `.dbml` — it won't survive a SQL detour (→ `conversion/fidelity.md`).
- **Split large schemas by domain** (`auth.dbml`, `billing.dbml`, `main.dbml`) and compose with `use`/`reuse`. Keep a single **entry file** (`main.dbml`) as the composition root (→ `syntax/advanced.md`).
- **Reuse repeated column bundles** (audit columns, soft-delete columns) via `TablePartial` injected with `~name` rather than copy-pasting (→ `syntax/advanced.md`).
- **Order tables top-down by dependency** inside a file: referenced (parent) tables before referencing (child) tables. Not required (refs resolve regardless), but it reads better and matches ERD layout intent.
- **Don't rely on `dbml2sql` exit codes** in CI — it always exits 0 even on error (→ `conversion/sql-export.md`).

## Readability

- **Document the *why*, not the *what*.** `email varchar [unique]` already says what; the note should add "login identity, case-insensitive" (the why).
- **One note per concept.** Don't repeat the same note on column + index + table.
- **Keep notes short.** Multi-line block notes (`'''…'''`) for tables that need it; inline `'…'` otherwise.
- **Use `TableGroup` and sticky notes purposefully** to segment domains and capture cross-table rationale — not as clutter.
- **Colors sparingly:** use `TableGroup color` / sticky-note color to visually segment domains; a rainbow diagram is noise.

## Consistency

- **Consistent type spelling.** DBML does no type mapping — `int` ≠ `integer` ≠ `INT` for rendering purposes; pick one and use it everywhere.
- **Consistent FK naming** (`<target>_id`) and a consistent PK convention (`id`).
- **Consistent operator direction** for the common FK case (`child.parent_id > parent.id`).

---

## Documentation practices

DBML has rich, free-form documentation constructs. Use them to make the schema self-explaining.

| Need | Use |
|---|---|
| What a table represents / business meaning | `Note: '...'` (block) inside the table |
| A subtle column (units, format, derivation) | `[note: 'amount in minor units (cents)']` |
| A non-obvious enum value | `pending [note: 'awaiting payment capture']` |
| A non-obvious index (esp. expression) | `(...)[note: 'case-insensitive login lookup']` |
| Cross-table rationale / TODOs / design decisions | **Sticky note** `Note name [color:] { '...' }` |
| Diagram grouping | `TableGroup` |

### What does NOT count as documentation
- `//` and `/* */` comments are **trivia** — not stored as notes, don't travel to exports or the model. Use `note:`/`Note:` for anything that should persist.
- `Project.note` is a one-line project description, not per-entity docs.

### Survives export?
Notes survive **DBML→DBML** round-trips. On **DBML→SQL**, table/column notes become `COMMENT`/extended-property statements (Postgres/MySQL/Oracle) or `sp_addextendedproperty` (MSSQL); **enum-value notes and sticky notes are lost**. Don't put critical info only in places that vanish on export.

### Generating documentation (the DBML-native way)
There is **no DBML "render" command** — DBML produces a model, not a document. To "generate docs" the DBML-native way, **enrich the schema** (`Project.note`, table/column `note:`, `TableGroup`, sticky notes, `Records`) and hand the `.dbml` to a **renderer**: **dbdocs.io** renders DBML into browsable documentation; **dbdiagram.io** renders ERDs. DBML itself is database-agnostic and does not draw or export images/PDFs (those are Tool-only features — → `capabilities.md`).
