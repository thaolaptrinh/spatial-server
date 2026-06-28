# DBML

Author, review, explain, and convert DBML schemas using the official DBML specification.

## Installation

```bash
npx skills add thaolaptrinh/skills --skill dbml
```

Installs the skill into your environment. It activates automatically when a task involves DBML.

### Quick Start

Run the install command, then open a session and ask it to work with DBML.

Example prompt:

> Write a DBML schema for an ecommerce store with users, orders, and order_items. Include indexes on every foreign key.

## When to Use

- Authoring a new `.dbml` schema from scratch
- Reviewing an existing schema for correctness, naming, indexes, and normalization
- Converting between SQL and DBML (PostgreSQL, MySQL, MSSQL, Oracle, Snowflake)
- Explaining a construct, its syntax, or whether it is supported
- Debugging a parser error by mapping the message to a cause and fix

## Capabilities

| Capability | Covers |
| --- | --- |
| Author & explain | Every DBML construct with exact syntax and status tagging |
| Relationships | Operators, cardinality, composite refs, referential actions |
| Design & review | Naming, organization, normalization, enums, indexes, review checklist |
| Conversion | SQL → DBML (DDL + live connector) and DBML → SQL, with fidelity and loss lists |
| Troubleshooting | Parser error codes, common mistakes, FAQ |

## Examples

The `examples/` directory contains parser-validated schemas you can copy and adapt:

- `basic/` — Minimal baseline
- `blog/`, `crm/`, `ecommerce/`, `hospital/` — Domain schemas
- `enterprise/` — Multi-file module system (`use`/`reuse`, `TablePartial`)
- `conversion/` — SQL ↔ DBML pairs
- `review/` — Broken → fixed pairs

See [`examples/`](examples/).

## Resources

`resources/` holds the knowledge loaded on demand — syntax reference, design guidance, conversion details, and troubleshooting. See [`resources/decision-guide.md`](resources/decision-guide.md) for task routing.

## Limitations

- DBML is database-agnostic. Rendering (ERDs, images, PDFs) is done by dbdiagram.io / dbdocs.io, not this skill.
- No BigQuery DDL import (live-connector only). No Snowflake SQL export.
- Single schema level only (`schema.table`); nested schemas are unsupported.
- No type validation — types are opaque strings carried verbatim.
- No language-level views, procedures, triggers, or sequences (silently dropped on SQL import).
