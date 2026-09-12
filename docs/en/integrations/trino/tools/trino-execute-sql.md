---
title: "trino-execute-sql"
type: docs
weight: 1
description: >
  A "trino-execute-sql" tool executes a SQL statement against a Trino
  database.
---

## About

A `trino-execute-sql` tool executes a SQL statement against a Trino
database.

`trino-execute-sql` takes one input parameter `sql` and run the sql
statement against the `source`.

> **Note:** This tool is intended for developer assistant workflows with
> human-in-the-loop and shouldn't be used for production agents.

### User impersonation

Set `impersonateUser: true` to run each statement as a specific Trino user. When
enabled, the tool exposes an additional optional input parameter `trino_user`
whose value is forwarded as the `X-Trino-User` header for that statement only. If
`trino_user` is omitted (or empty), the query runs as the source's configured
user. The connection pool's configured principal (DSN `user` / `accessToken`)
still authenticates the request, so that principal must be authorized to
impersonate on the Trino side.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: execute_sql_tool
type: trino-execute-sql
source: my-trino-instance
description: Use this tool to execute sql statement.
```

## Reference

| **field**   |                  **type**                  | **required** | **description**                                                                                  |
|-------------|:------------------------------------------:|:------------:|--------------------------------------------------------------------------------------------------|
| type            |                 string                 |     true     | Must be "trino-execute-sql".                                                                     |
| source          |                 string                 |     true     | Name of the source the SQL should execute on.                                                    |
| description     |                 string                 |     true     | Description of the tool that is passed to the LLM.                                               |
| impersonateUser |                  bool                  |    false     | When true, adds an optional `trino_user` input parameter forwarded as the `X-Trino-User` header. |
