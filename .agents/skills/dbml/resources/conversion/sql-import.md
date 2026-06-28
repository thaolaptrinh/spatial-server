# SQL → DBML Import

Get SQL *into* DBML. **Two completely separate import paths — never conflate them.** For the lossy/round-trip picture see `fidelity.md`; for the export direction see `sql-export.md`. Tooling boundaries live in `capabilities.md`.

---

## Path 1: DDL-file import — `@dbml/core` `importer.import(sql, format)`

Parses a `.sql` text blob via **ANTLR grammars**. **5 dialects only.**

| `format` | Dialect |
|---|---|
| `'postgres'` | PostgreSQL |
| `'mysql'` | MySQL |
| `'mssql'` | SQL Server |
| `'snowflake'` | Snowflake |
| `'oracle'` | Oracle |
| `'dbml'` | (DBML→DBML via **deprecated** PEG parser — prefer `'dbmlv2'`; → `fidelity.md`) |

```js
const { importer } = require('@dbml/core');
const dbml = importer.import(sqlString, 'postgres');   // → DBML text
// options: { includeRecords: true }  (default true → INSERT rows become Records)
```
CLI: `sql2dbml file.sql --postgres -o out.dbml`

## Path 2: Live-DB import — `@dbml/connector`

Connects to a **running database** via native drivers and introspects `INFORMATION_SCHEMA`. **6 dialects, including BigQuery.**

```js
const { connector } = require('@dbml/connector');
const { importer } = require('@dbml/core');
const schemaJson = await connector.fetchSchemaJson(connStr, 'postgres'); // or mysql/mssql/snowflake/bigquery/oracle
const dbml = importer.generateDbml(schemaJson);
```
CLI: `db2dbml postgres "postgresql://..." -o out.dbml`

## ❗ BigQuery is connector-only
There is **no DDL-file import** for BigQuery (no grammar; `ImportFormat` has no `bigquery`). The repo's `ddl_samples/bigquery.sql` is an orphan red herring. To get BigQuery into DBML you must connect live. BigQuery connector returns **no FKs, no checks, no increment** (BigQuery doesn't support them natively).

---

## What converts (DDL path) — per construct

| SQL | → DBML | Notes |
|---|---|---|
| `CREATE TABLE` + columns | `Table` + columns | types largely **verbatim** |
| `PRIMARY KEY` (single) | `[pk]` | |
| `PRIMARY KEY` (multi) | `indexes { (...) [pk] }` | |
| `FOREIGN KEY` / `REFERENCES` | `Ref … [delete: …]` | inline + table-level; `MATCH` clauses lost |
| `UNIQUE` (single) | `[unique]` | |
| `UNIQUE` (multi) | `indexes { (...) [unique] }` | |
| `CREATE INDEX` / `KEY` | `indexes` | **errors on Snowflake** (grammar has no rule → `no viable alternative`; not silent) |
| `AUTO_INCREMENT`/`IDENTITY`/`SERIAL`/`AUTOINCREMENT` | `[increment]` | dialect-specific detection |
| `NOT NULL` | `[not null]` | |
| `DEFAULT` | `[default: …]` (typed) | |
| `CHECK` | table/field `checks` | **errors on Snowflake** (not supported; not silent) |
| table/column comments | `note:` | Postgres/MySQL/Oracle `COMMENT`; MSSQL `sp_addextendedproperty` |
| `CREATE TYPE … AS ENUM` (Postgres) | `Enum` | |
| MySQL `ENUM('a','b')` | synthetic `Enum "<table>_<col>_enum"` | column type renamed to it |
| MySQL `SET('a','b')` | kept as the **literal type** `SET('a','b')` | only `ENUM` becomes a DBML `Enum`; `SET` is not converted |
| `INSERT INTO … VALUES` | `Records` | only when `includeRecords` (default on) |
| `ALTER TABLE ADD CONSTRAINT` | applied to existing table | only `ADD CONSTRAINT` (FK/CHECK/PK/UNIQUE); most ALTERs dropped |

---

## ⚠️ Silently dropped — NO error, NO output (DDL path)

These have no DBML representation and vanish without warning:
- **Views, materialized views, sequences, triggers, stored procedures/functions** (a `CREATE FUNCTION` produces nothing).
- **Most `ALTER TABLE`** sub-commands (RENAME, ADD/DROP COLUMN, type changes, owner, tablespace…).
- **Generated/computed columns** — the column survives but the **generation expression is lost** (MSSQL stuffs it into the type string as `AS (expr) PERSISTED`).
- **MySQL `UNSIGNED`/`ZEROFILL`**, charsets/collations.
- Schema/tablespace/role/policy/extension DDL.

Because drops are silent, **diff table/enum counts against the source** to catch what's missing.

---

## ⚠️ Parse errors (NOT silent) — Snowflake

Unlike the silent drops above, these **throw a `no viable alternative` error** and abort the import on Snowflake:
- **`CREATE INDEX`** (secondary indexes) — the Snowflake grammar has no rule for it.
- **`CHECK` constraints** (inline or `CONSTRAINT … CHECK`).

If your Snowflake DDL contains these, **strip them before importing** (Snowflake import supports tables, PK, UNIQUE, FK, defaults, comments — but not secondary indexes or CHECK).

---

## No type validation
A bogus type (`Intdsfsd`) passes through verbatim. Good for fidelity, bad if you expected canonicalization.

---

## After-import audit
- Match the dialect flag (`--postgres`/`--mysql`/`--mssql`/`--snowflake`/`--oracle`). Wrong dialect = wrong/missing output.
- **BigQuery:** use the live connector (`db2dbml bigquery "<conn>"`).
- Count tables/enums vs source; re-add views/functions/triggers as notes (silently dropped).
- Check generated-column expressions and MySQL `UNSIGNED` — both lost.
- Snowflake: strip `CREATE INDEX` and `CHECK` first (they error, not drop).
- Pass `includeRecords: false` if you don't want sample `INSERT` rows as Records.
