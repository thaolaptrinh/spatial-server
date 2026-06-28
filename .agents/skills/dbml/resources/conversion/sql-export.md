# DBML → SQL Export

Get DBML *out to* SQL. Export targets = **`{ dbml, mysql, postgres, json, mssql, oracle }`** — **there is NO Snowflake exporter** (`'snowflake'` silently returns `""`). For the import direction see `sql-import.md`; for round-trip/loss see `fidelity.md`.

---

## API & CLI
```js
const { exporter, ModelExporter, Parser } = require('@dbml/core');
exporter.export(dbmlString, 'postgres')                 // string → string
ModelExporter.export(databaseModel, 'mysql')            // model → string
// To get a model from DBML (e.g. to validate before exporting):
const db = new Parser().parse(dbmlString, 'dbmlv2');    // → Database (throws on error)
// options: { includeRecords: true }  (default true; Records → INSERT)
```
CLI: `dbml2sql file.dbml --postgres -o out.sql` (default dialect = **postgres**; flags: `--mysql --postgres --mssql --oracle`; no Snowflake). Multifile DBML: pass the **entry** file; `use`/`reuse` resolve automatically.

### ⚠️ Exit code gotcha (critical)
**`dbml2sql` always exits 0, even on hard parse errors.** Errors print to stdout as `ERROR: <file>(line,col): <msg>` and are written to `./dbml-error.log`. **Do not gate CI/scripts on the exit code** — parse the output or check the log.

### ⚠️ Duplicate/overlapping refs block output
"References with same endpoints exist" is emitted as an error and the file produces **no SQL**. Two refs between the same column pair (even opposite direction) trip it.

### Flags that DO NOT exist
`--config`, `--include`, `--out-dir`, `--snowflake` (on `dbml2sql`). `-o/--out-file` is single-file only (`--out-dir` does not exist).

---

## What survives export (all SQL dialects)
| DBML | → SQL |
|---|---|
| tables, columns, types | `CREATE TABLE` (types **verbatim**) |
| `[pk]` / composite PK index | `PRIMARY KEY` |
| `[unique]`, `not null` | column constraints |
| `default:` (number/string/expression) | `DEFAULT` (`DEFAULT NULL` skipped as redundant) |
| `[increment]` | dialect-specific identity |
| `ref: > / < / -` | `ALTER TABLE … ADD FOREIGN KEY` |
| `delete:` / `update:` | `ON DELETE` / `ON UPDATE` (**`ON UPDATE` dropped on Oracle**) |
| Ref names | `CONSTRAINT "name"` |
| indexes | `CREATE INDEX` (composite-PK indexes excluded; MySQL/MSSQL auto-name unnamed `<table>_index_<n>`) |
| `Enum` | dialect-mapped (see below) |
| `Checks` | `CHECK (…)` |
| table/column `note:` | `COMMENT` / `sp_addextendedproperty` |
| `Records` | `INSERT` (wrapped in constraint-deferral scaffolding) |

### Enum export per dialect
| Dialect | Enum → |
|---|---|
| Postgres | `CREATE TYPE "x" AS ENUM (...)` |
| MySQL | inline `ENUM (...)` on the column |
| MSSQL | `CHECK (col IN (...))` |
| Oracle | `CHECK (col IN (...))` |

### `increment` export per dialect
Postgres `GENERATED … AS IDENTITY` · MySQL `AUTO_INCREMENT` · MSSQL `IDENTITY(1,1)` · Oracle `GENERATED AS IDENTITY` (suppresses `NOT NULL`/`DEFAULT`).

### Type fidelity (verbatim — NO type mapping)
`integer`→`integer`, `decimal(10,2)`→`decimal(10,2)`, custom `job_status`→`job_status`. **Exceptions:** MySQL `varchar`→`varchar(255)`, MSSQL `varchar`→`nvarchar(255)`; Postgres uppercases increment-column type names to match `SERIAL`/`IDENTITY` built-ins. Non-builtin types with spaces/uppercase get double-quoted (Postgres).

---

## ❌ Lost on DBML→SQL (canonical enrichment-loss list — all dialectes)

DBML enrichment with **no SQL equivalent** is dropped on SQL export (kept on DBML→DBML). This is the canonical list; `fidelity.md` references it rather than restating it:

`Project` block · `TableGroup` · sticky notes · `DiagramView` · aliases (`as`) · enum-value notes · colors (`headercolor`, group `color`, ref `color`) · `//` comments.

---

## `<>` many-to-many synthesizes a junction table
`Ref: a.id <> b.id` → a generated `a_b` junction table with two FKs (all dialects). Oracle also emits `GRANT REFERENCES … TO PUBLIC` for cross-schema refs.

## Records surprise
By default `Records` become real `INSERT` statements wrapped in deferral scaffolding (Postgres `BEGIN; SET CONSTRAINTS ALL DEFERRED; … COMMIT;`; MySQL `SET FOREIGN_KEY_CHECKS=0/1`; MSSQL `NOCHECK CONSTRAINT ALL`/`WITH CHECK CHECK`; Oracle `SET CONSTRAINTS ALL DEFERRED` + `INSERT ALL … SELECT FROM dual`). For pure DDL, pass `includeRecords: false`.

## Decisions before exporting
- Dialects: `--mysql --postgres --mssql --oracle`. **No Snowflake.**
- Decide on Records: default → `INSERT` (with deferral scaffolding); suppress for pure DDL.
- **Don't gate on exit code** — check `dbml-error.log`.
- Review losses: the enrichment-loss list applies (Project/TableGroup/sticky-notes/aliases/colors/enum-value notes gone).
