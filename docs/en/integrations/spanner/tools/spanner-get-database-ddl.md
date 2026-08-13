---
title: "spanner-get-database-ddl"
type: docs
weight: 1
description: >
  A "spanner-get-database-ddl" tool retrieves the DDL statements that define the schema of a Cloud Spanner database.
---

## About

A `spanner-get-database-ddl` tool retrieves the DDL (Data Definition Language) statements that define the schema of a Cloud Spanner database, including table, index, view, and constraint definitions.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: get_database_ddl
type: spanner-get-database-ddl
source: my-spanner-instance
description: Use this tool to retrieve DDL statements defining the database schema.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "spanner-get-database-ddl".                |
| source      |  string  |     true     | Name of the Spanner source to retrieve DDL from.   |
| description |  string  |     true     | Description of the tool passed to the LLM.         |
