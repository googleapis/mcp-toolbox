---
title: "oracle-list-tables"
type: docs
weight: 1
description: > 
   Lists all tables in the current user's schema
---

## About

An `oracle-sql` tool executes a pre-defined SQL statement against an
Oracle database.

The specified SQL statement is executed using [prepared statements][oracle-stmt]
for security and performance. It expects parameter placeholders in the SQL query
to be in the native Oracle format (e.g., `:1`, `:2`).

By default, the statement runs as a query. Set `readOnly` to `true` to run it as
a read-only transaction, so the database itself rejects writes and locking reads.
Set it to `false` to run the statement as DML, which reports the number of
affected rows instead of returning rows.

[oracle-stmt]: https://docs.oracle.com/javase/tutorial/jdbc/basics/prepared.html

## Compatible Sources

{{< compatible-sources >}}

## Example

> **Note:** This tool uses parameterized queries to prevent SQL injections.
> Query parameters can be used as substitutes for arbitrary expressions.
> Parameters cannot be used as substitutes for identifiers, column names, table
> names, or other parts of the query.

```yaml
kind: tool
name: list_tables
type: oracle-sql
source: my-oracle-instance
statement: |
  SELECT table_name from user_tables;
description: |
  Lists all table names in the current user's schema.
```
