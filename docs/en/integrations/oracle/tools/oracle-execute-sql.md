---
title: "oracle-execute-sql"
type: docs
weight: 1
description: > 
  An "oracle-execute-sql" tool executes a SQL statement against an Oracle database.
---

## About

An `oracle-execute-sql` tool executes a SQL statement against an Oracle
database.

`oracle-execute-sql` takes one input parameter `sql` and runs the sql
statement against the `source`.

> **Note:** This tool is intended for developer assistant workflows with
> human-in-the-loop and shouldn't be used for production agents.

By default the statement runs as given, so the tool can both read and write. Set
`readOnly` to `true` to run statements inside an Oracle read-only transaction,
which makes the database reject writes and locking reads such as
`SELECT ... FOR UPDATE` with `ORA-01456`. Set it to `false` to run the statement
as DML and report the number of affected rows instead of returning rows.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: execute_sql_tool
type: oracle-execute-sql
source: my-oracle-instance
description: Use this tool to execute sql statement.
```

A tool that may only read:

```yaml
kind: tool
name: query_sql_tool
type: oracle-execute-sql
source: my-oracle-instance
readOnly: true
description: Use this tool to run a read-only sql statement.
```
