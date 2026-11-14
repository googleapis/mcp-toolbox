---
title: "clickhouse-execute-sql"
type: docs
weight: 1
description: >
  A "clickhouse-execute-sql" tool executes a SQL statement against a ClickHouse
  database.
---

## About

A `clickhouse-execute-sql` tool executes a SQL statement against a ClickHouse
database.

`clickhouse-execute-sql` takes an input parameter `sql` and runs the SQL
statement against the specified `source`. This tool includes query logging
capabilities for monitoring and debugging purposes.

The optional `params` input binds
[query parameters](https://clickhouse.com/docs/sql-reference/syntax#defining-and-using-query-parameters)
on the server: each `{name:Type}` placeholder in the statement gets the value
of `params.name`, written in ClickHouse's text format for its type (for example
`"42"`, `"2024-01-31"` or `"['a','b']"`). ClickHouse parses each value as its
declared type, so a value never becomes part of the SQL text. Identifiers can be
bound the same way, for example `{table:Identifier}`.

> **Note:** This tool is intended for developer assistant workflows with
> human-in-the-loop and shouldn't be used for production agents.


## Compatible Sources

{{< compatible-sources >}}

## Parameters

| **parameter** | **type** | **required** | **description**                                                         |
|---------------|:--------:|:------------:|-------------------------------------------------------------------------|
| sql           |  string  |     true     | The SQL statement to execute against the database                       |
| params        |   map    |    false     | Query parameters by name, each a string in ClickHouse's text format     |


## Example

```yaml
kind: tool
name: execute_sql_tool
type: clickhouse-execute-sql
source: my-clickhouse-instance
description: Use this tool to execute SQL statements against ClickHouse.
```

## Reference

| **field**   | **type** | **required** | **description**                                       |
|-------------|:--------:|:------------:|-------------------------------------------------------|
| type        |  string  |     true     | Must be "clickhouse-execute-sql".                     |
| source      |  string  |     true     | Name of the ClickHouse source to execute SQL against. |
| description |  string  |     true     | Description of the tool that is passed to the LLM.    |
