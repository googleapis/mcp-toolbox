---
title: "trino-sql"
type: docs
weight: 1
description: >
  A "trino-sql" tool executes a pre-defined SQL statement against a Trino
  database.
---

## About

A `trino-sql` tool executes a pre-defined SQL statement against a Trino
database.
The specified SQL statement is executed as a [prepared statement][trino-prepare], and expects parameters in the SQL query to be in the form of placeholders `?`.

[trino-prepare]: https://trino.io/docs/current/sql/prepare.html

### User impersonation

Set `impersonateUser: true` to run the statement as a specific Trino user. When
enabled, the tool exposes an additional optional input parameter `trino_user`
whose value is forwarded as the `X-Trino-User` header for that statement only
(it is not bound into the SQL). If `trino_user` is omitted (or empty), the query
runs as the source's configured user. The connection pool's configured principal
(DSN `user` / `accessToken`) still authenticates the request, so that principal
must be authorized to impersonate on the Trino side.

## Compatible Sources

{{< compatible-sources >}}

## Example

> **Note:** This tool uses parameterized queries to prevent SQL injections.
> Query parameters can be used as substitutes for arbitrary expressions.
> Parameters cannot be used as substitutes for identifiers, column names, table
> names, or other parts of the query.

```yaml
kind: tool
name: search_orders_by_region
type: trino-sql
source: my-trino-instance
statement: |
  SELECT * FROM hive.sales.orders
  WHERE region = ?
  AND order_date >= DATE(?)
  LIMIT 10
description: |
  Use this tool to get information for orders in a specific region.
  Takes a region code and date and returns info on the orders.
  Do NOT use this tool with an order id. Do NOT guess a region code or date.
  A region code is a code for a geographic region consisting of two-character
  region designator and followed by optional subregion.
  For example, if given US-WEST, the region is "US-WEST".
  Another example for this is EU-CENTRAL, the region is "EU-CENTRAL".
  If the tool returns more than one option choose the date closest to today.
  Example:
  {{
      "region": "US-WEST",
      "order_date": "2024-01-01",
  }}
  Example:
  {{
      "region": "EU-CENTRAL",
      "order_date": "2024-01-15",
  }}
parameters:
  - name: region
    type: string
    description: Region unique identifier
  - name: order_date
    type: string
    description: Order date in YYYY-MM-DD format
```

### Example with Template Parameters

> **Note:** This tool allows direct modifications to the SQL statement,
> including identifiers, column names, and table names. **This makes it more
> vulnerable to SQL injections**. Using basic parameters only (see above) is
> recommended for performance and safety reasons. For more details, please check
> [templateParameters](../../../documentation/configuration/tools/_index.md#template-parameters).

```yaml
kind: tool
name: list_table
type: trino-sql
source: my-trino-instance
statement: |
  SELECT * FROM {{.tableName}}
description: |
  Use this tool to list all information from a specific table.
  Example:
  {{
      "tableName": "hive.sales.orders",
  }}
templateParameters:
  - name: tableName
    type: string
    description: Table to select from
```

## Reference

| **field**          |                   **type**                   | **required** | **description**                                                                                                                        |
|--------------------|:--------------------------------------------:|:------------:|----------------------------------------------------------------------------------------------------------------------------------------|
| type               |                    string                    |     true     | Must be "trino-sql".                                                                                                                   |
| source             |                    string                    |     true     | Name of the source the SQL should execute on.                                                                                          |
| description        |                    string                    |     true     | Description of the tool that is passed to the LLM.                                                                                     |
| statement          |                    string                    |     true     | SQL statement to execute on.                                                                                                           |
| parameters         |   [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters)    |    false     | List of [parameters](../../../documentation/configuration/tools/_index.md#specifying-parameters) that will be inserted into the SQL statement.                                          |
| templateParameters | [templateParameters](../../../documentation/configuration/tools/_index.md#template-parameters) |    false     | List of [templateParameters](../../../documentation/configuration/tools/_index.md#template-parameters) that will be inserted into the SQL statement before executing prepared statement. |
| impersonateUser    |                     bool                     |    false     | When true, adds an optional `trino_user` input parameter forwarded as the `X-Trino-User` header.                                        |
