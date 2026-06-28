# Parser Errors — code → meaning → fix

`@dbml/parse` emits `CompileError`s with a numeric `CompileErrorCode`. `@dbml/core` surfaces them via `CompilerError.diags[]` with `{ message, location:{start:{line,column}}, code }`. Legacy/SQL PEG errors instead carry `expected`/`found` text. For worked broken→fixed examples see `common-mistakes.md`.

---

## Top errors you will actually hit

| Code group / message | Meaning | Cause | Fix |
|---|---|---|---|
| `INVALID_REF_FIELD` — *"A Ref field must be a binary relationship"* | invalid relationship operator | `><`, `-<`, `>-`, or a malformed endpoint | use only `- < > <>` (→ `relationships.md`) |
| `UNEQUAL_FIELDS_BINARY_REF` — *"Unequal fields in ref endpoints"* | composite tuple size mismatch | `a.(x,y) > b.(p)` | make both tuples equal length |
| `REF_REDEFINED` / `SAME_ENDPOINT` — *"References with same endpoints exist"* | duplicate ref | two refs between the same column pair | define each relationship once (→ `relationships.md`) |
| `UNKNOWN_COLUMN_SETTING` — *"Unknown column setting 'X'"* | invented column setting | `default_expr`, `using`, etc. | use the real settings (→ `common-mistakes.md`) |
| `UNKNOWN_INDEX_SETTING` — *"Unknown index setting 'using'"* | wrong index-method keyword | `[using: btree]` | `[type: btree]` |
| `UNSUPPORTED` — *"Nested schema is not supported"* | schema has >1 segment | `Table a.b.c` | single-level `schema.table` only |
| `INVALID_CUSTOM_*` — *"A Custom element can only appear in a Project"* | enrichment in wrong place | `headercolor:` inside a Table body | put `headercolor` in the header `[...]` |
| `EMPTY_TABLE` — *"Table must have at least one field"* | table with no columns | `Table t { }` | add ≥1 column |
| `DUPLICATE_COLUMN_NAME` — *"Field X existed in table"* | repeated column | two `id int` | unique column names |
| `INVALID_PROJECT_FIELD` | non `key: value` in Project | `Project p { Note {} }` | use `key: 'value'` + lowercase `note:` |
| `NONEXISTENT_MODULE` — *"Failed to resolve the non-existent file"* | missing import target | `use * from './missing'` | fix the relative path |
| *"These fields must be some inline settings optionally ended with a setting list"* | malformed column/index line | multiple columns on one line, or `index {…}` glued to a column | one item per line |
| `EMPTY_ENUM` — *"An Enum must have only a field"* | enum values on one line | `Enum s { a b }` | one value per line |
| `BINDING_ERROR` — *"Can't find table …"* / *"Can't find field …"* | dangling ref | `ref: > ghost.id` | fix/remove the ref (→ `relationships.md`) |
| *"Can't find primary or composite key in table"* | ref auto-PK target lacks a PK | `Ref: child.parent_id > parent` with no PK on `parent` | add a PK to the target |
| *"Expect a newline between use specifiers"* | comma-separated `use` specifiers | `use { table a, table b } from './x'` | one specifier per line |

---

## Error-code ranges (full enum in `@dbml/parse` `core/types/errors.ts`)

| Range | Category | Examples |
|---|---|---|
| 1000–1999 | lexer / tokens / symbols | `UNKNOWN_SYMBOL`, `UNEXPECTED_TOKEN`, `INVALID_OPERAND`, `MISSING_SPACES` |
| 3000–3999 | per-construct validation | table/column/enum/ref/note/indexes/project/tablepartial/checks/records/use/diagramview + their `*_CONTEXT`/`UNKNOWN_*`/`DUPLICATE_*`/`INVALID_*` codes |
| 4000 | binding | `BINDING_ERROR`, `NONEXISTENT_MODULE` |
| 5000 | structural | `UNSUPPORTED`, `CIRCULAR_REF`, `SAME_ENDPOINT`, `UNEQUAL_FIELDS_BINARY_REF`, `CONFLICTING_SETTING`, `TABLE_REAPPEAR_IN_TABLEGROUP` |

---

## Throw vs collected (nuance)

`@dbml/core` (`Parser.parse('dbmlv2')`) **throws** `CompilerError` for many errors (invalid ref ops, duplicate refs, dangling refs, unknown settings). A few diagnostics are **collected but non-fatal** — e.g. nested schema emits `UNSUPPORTED` yet still returns a model. When programmatically checking validity, **inspect `.diags`, don't rely solely on "did it throw."** The CLI prints errors to stdout + `dbml-error.log` but **always exits 0** (→ `conversion/sql-export.md`).

`@dbml/core` is **all-or-nothing per model**: the first dangling ref / missing field / duplicate **throws** and aborts the build. (The `@dbml/parse` layer is more forgiving — it collects diagnostics.)

---

## Repair patterns (by symptom)

| Symptom in the error | Likely cause | Repair |
|---|---|---|
| *"binary relationship"* / *"invalid operator"* | Mermaid op `><`/`-<`/`>-` | use `- < > <>`; many-to-many → junction table |
| *"Unknown column setting"* / *"Unknown index setting"* | `default_expr`, `using`, or other invented name | use the real setting name |
| *"must have only one field"* / *"only a field"* | multiple items on one line | one item per line |
| *"A Custom element can only appear in a Project"* | `headercolor:` (or color) inside a body | move it to the header `[...]` |
| *"Nested schema is not supported"* | `a.b.c` | collapse to single-level `schema.table` |
| *"same endpoints exist"* | duplicate ref (possibly reversed) | keep one ref per column pair |
| *"Can't find table/field"* | dangling ref | fix or remove the ref |
| *"Can't find primary or composite key"* | ref target has no PK | add a PK to the target |
| *"Failed to resolve the non-existent file"* | bad `use` path | fix the relative path |
| PEG-style *"expected … found …"* | legacy `'dbml'` format or SQL import | use `'dbmlv2'`; match the SQL dialect |
