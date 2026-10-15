---
title: "mssql-get-database-object-source-code"
type: docs
weight: 1
description: >
  The "mssql-get-database-object-source-code" tool retrieves source code and metadata for objects in a SQL Server database.
---

## About

The `mssql-get-database-object-source-code` tool retrieves source code and
metadata for database objects in a SQL Server database, including stored
procedures, views, functions, triggers, linked servers, tables, and user-defined
assemblies.

The tool takes the following input parameters:

- **`namesOnly`** (boolean, optional): Set to `true` to return only object names
  instead of full object source code or metadata. Default: `false`.
- **`object_type`** (string, optional): Specify the database object type to
  retrieve. Allowed values: `all`, `functions`, `tables`, `procedures`, `views`,
  `triggers`, `linked_servers`, `assemblies`. Default: `all`.

## Compatible Sources

{{< compatible-sources others="integrations/cloud-sql-mssql">}}

## Example

```yaml
kind: tool
name: get_database_object_source_code
type: mssql-get-database-object-source-code
source: mssql-source
description: Executes a query to retrieve source code for all coding objects.
```

## Reference

| **field**   | **type** | **required** | **description**                                      |
|-------------|:--------:|:------------:|------------------------------------------------------|
| type        |  string  |     true     | Must be "mssql-get-database-object-source-code".     |
| source      |  string  |     true     | Name of the source the SQL should execute on.        |
| description |  string  |     true     | Description of the tool that is passed to the agent. |
