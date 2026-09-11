---
title: "mssql-list-indexes"
type: docs
weight: 1
description: >
  The "mssql-list-indexes" tool lists indexes in a SQL Server database.
---

## About

The `mssql-list-indexes` tool lists user indexes in a SQL Server database,
excluding system schemas (`sys`, `INFORMATION_SCHEMA`) and heaps.

`mssql-list-indexes` lists detailed information as JSON for each index. The tool
takes the following input parameters:

- **`schema_name`** (string, optional): A text to filter results by schema name. The input is used within a `LIKE` clause. Default: `""`.
- **`table_name`** (string, optional): A text to filter results by table name. The input is used within a `LIKE` clause. Default: `""`.
- **`index_name`** (string, optional): A text to filter results by index name. The input is used within a `LIKE` clause. Default: `""`.
- **`only_unused`** (boolean, optional): If true, returns only indexes with no recorded reads since the last SQL Server restart. Default: `false`.
- **`limit`** (integer, optional): The maximum number of rows to return. Default: `50`.

## Compatible Sources

{{< compatible-sources others="integrations/cloud-sql-mssql">}}

## Requirements

The index usage columns (`user_reads`, `user_updates`, `last_user_seek`,
`last_user_scan`, and `is_used`) are read from `sys.dm_db_index_usage_stats`.
Querying that dynamic management view requires the `VIEW SERVER STATE` permission
on SQL Server, or `VIEW DATABASE STATE` on Azure SQL Database. The remaining
structural columns are read from catalog views, which need no special permission.

## Example

```yaml
kind: tool
name: list_indexes
type: mssql-list-indexes
source: mssql-source
description: |
  Lists user indexes in a SQL Server database, excluding system schemas. For each
  index, the following properties are returned: schema name, table name, index
  name, index type (e.g. CLUSTERED/NONCLUSTERED), whether it is unique, whether it
  backs a primary key, whether it is disabled, the filter definition, the key
  columns, the included columns, the number of user reads (seeks + scans + lookups)
  and user updates recorded by sys.dm_db_index_usage_stats since the last restart,
  and a boolean indicating whether the index has been used at least once. Index
  usage statistics reset on SQL Server restart.
```

The response is a json array with the following elements:

```json
{
  "schema_name": "schema name",
  "table_name": "table name",
  "index_name": "index name",
  "index_type": "index type (e.g. CLUSTERED, NONCLUSTERED)",
  "is_unique": "boolean indicating if the index is unique",
  "is_primary": "boolean indicating if the index backs a primary key",
  "is_disabled": "boolean indicating if the index is disabled",
  "filter_definition": "filter expression for a filtered index, or null",
  "key_columns": "comma-separated key columns in key order, with ASC/DESC",
  "included_columns": "comma-separated non-key included columns",
  "user_reads": "seeks + scans + lookups recorded since the last restart",
  "user_updates": "updates recorded since the last restart",
  "last_user_seek": "timestamp of the last user seek, or null",
  "last_user_scan": "timestamp of the last user scan, or null",
  "is_used": "boolean indicating if the index has been read at least once"
}
```

## Reference

| **field**   | **type** | **required** | **description**                                      |
|-------------|:--------:|:------------:|------------------------------------------------------|
| type        |  string  |     true     | Must be "mssql-list-indexes".                        |
| source      |  string  |     true     | Name of the source the SQL should execute on.        |
| description |  string  |    false     | Description of the tool that is passed to the agent. |
