---
title: "bigquery-execute-sql"
type: docs
weight: 1
description: >
  A "bigquery-execute-sql" tool executes a SQL statement against BigQuery.
---

## About

A `bigquery-execute-sql` tool executes a SQL statement against BigQuery.

`bigquery-execute-sql` accepts the following parameters:

- **`sql`** (required): The GoogleSQL statement to execute.
- **`dry_run`** (optional): If set to `true`, the query is validated but not
  run, returning information about the execution instead. Defaults to `false`.

The behavior of this tool is influenced by the combination of the `readOnly`
and `writeMode` settings on its `bigquery` source:

| `readOnly` | `writeMode` | Tool Behavior | MCP Tool Annotations |
| :--- | :--- | :--- | :--- |
| `false` *(default)* | `allowed` *(default)* | All SQL statements are permitted. | *(Default / none)* |
| `true` | `blocked` *(default if `readOnly: true`)* | Only `SELECT` statements are allowed. Any other type of statement (e.g., `INSERT`, `UPDATE`, `CREATE`) will be rejected. | `readOnlyHint: true` |
| `true` | `protected` | Enables session-based execution. `SELECT` statements can be used on all tables, while write operations are allowed only for the session's temporary dataset (e.g., `CREATE TEMP TABLE ...`). | `readOnlyHint: true` |

> **Note:** Conflicting configurations, such as `readOnly: true` combined with
> `writeMode: allowed`, are rejected during server startup.

The tool's behavior is influenced by the `allowedDatasets` restriction on the
`bigquery` source. Similar to `writeMode`, this setting provides an additional
layer of security by controlling which datasets can be accessed:

- **Without `allowedDatasets` restriction:** The tool can execute any valid
  GoogleSQL query.
- **With `allowedDatasets` restriction:** Before execution, the tool performs a
  dry run to analyze the query. It will reject the query if it explicitly
  references any table outside the allowed `datasets` list.

  **Authorized views are supported.** If the dry run reports that the query
  reads a table outside the allowed list, but that table is not named anywhere
  in the SQL text, the access is treated as an
  [authorized view](https://cloud.google.com/bigquery/docs/authorized-views)
  and permitted. This lets you expose a curated view in an allowed dataset that
  reads from restricted source data. Note that all table names in the query
  must be fully qualified (`dataset.table` or `project.dataset.table`) to be
  eligible for authorized view exemptions, as unqualified table names resolved
  via default datasets cannot be statically verified against the allowed list.

  To keep the analysis sound, the following operations remain disallowed:

  - **Dataset-level operations** (e.g., `CREATE SCHEMA`, `ALTER SCHEMA`).
  - **Unanalyzable operations** where the accessed tables cannot be determined
    statically (e.g., `EXECUTE IMMEDIATE`, `CREATE PROCEDURE`,
    `CREATE FUNCTION`, `CREATE TABLE FUNCTION`, `CALL`).
  - **Session variable assignments** (e.g., `SET @@dataset_id = ...`).
  - **Federated queries** via `EXTERNAL_QUERY`.
  - **Region-level `INFORMATION_SCHEMA` views** (only dataset-scoped views such
    as `<dataset>.INFORMATION_SCHEMA.TABLES` are allowed).

> **Note:** This tool is intended for developer assistant workflows with
> human-in-the-loop and shouldn't be used for production agents.


## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: execute_sql_tool
type: bigquery-execute-sql
source: my-bigquery-source
description: Use this tool to execute sql statement.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "bigquery-execute-sql".                    |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
