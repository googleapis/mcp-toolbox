---
title: "mysql-list-tables"
type: docs
weight: 1
description: >
  The "mysql-list-tables" tool lists schema information for all or specified tables in a MySQL database.
---

## About

The `mysql-list-tables` tool retrieves schema information for all or specified
tables in a MySQL database.

`mysql-list-tables` lists detailed schema information (object type, columns,
constraints, indexes, triggers, owner, comment) as JSON for user-created tables
(ordinary or partitioned) in the configured database. Filters by a
comma-separated list of names. If names are omitted, it lists all tables in that
database. If no database is configured, it includes tables across all user
schemas. The output format can be set
to `simple` which will return only the table names or `detailed` which is the
default.

The tool takes the following input parameters:

| Parameter       | Type   | Description                                                                                                                                                    | Required |
|:----------------|:-------|:---------------------------------------------------------------------------------------------------------------------------------------------------------------|:---------|
| `table_names`   | string | Filters by a comma-separated list of names in the configured database. If no database is configured, includes all user schemas. An empty string lists all tables. Default: `""` | No       |
| `output_format` | string | Indicate the output format of table schema. `simple` will return only the table names, `detailed` will return the full table information. Default: `detailed`. | No       |

## Compatible Sources

{{< compatible-sources others="integrations/cloud-sql-mysql">}}

## Example

```yaml
kind: tool
name: mysql_list_tables
type: mysql-list-tables
source: mysql-source
description: Use this tool to retrieve schema information for all or specified tables. Output format can be simple (only table names) or detailed.
```

## Reference

| **field**   | **type** | **required** | **description**                                      |
|-------------|:--------:|:------------:|------------------------------------------------------|
| type        |  string  |     true     | Must be "mysql-list-tables".                         |
| source      |  string  |     true     | Name of the source the SQL should execute on.        |
| description |  string  |     true     | Description of the tool that is passed to the agent. |
