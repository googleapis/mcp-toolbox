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
`readOnly` to `true` to run it as a read-only transaction, so the database itself
rejects writes and locking reads. Set it to `false` to run the statement as DML,
which reports the number of affected rows instead of returning rows.

> **Note:** `readOnly` guards against accidental writes; it is not a security
> boundary. A read-only transaction does not stop DDL, which commits implicitly,
> so a statement that performs DDL before writing escapes it entirely. Because
> this tool runs whatever SQL it is given, enforce read-only in the database for
> untrusted or model-generated input — see [Read-Only Access][ro-access].

[ro-access]: ../source.md#read-only-access

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
