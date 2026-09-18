---
title: "spanner-update-database-schema"
type: docs
weight: 1
description: >
  A "spanner-update-database-schema" tool executes DDL statements to modify the schema of a Cloud Spanner database.
---

## About

A `spanner-update-database-schema` tool executes DDL (Data Definition Language) statements to modify the schema of a Cloud Spanner database, such as creating, altering, or dropping tables, indexes, views, or change streams.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: update_database_schema
type: spanner-update-database-schema
source: my-spanner-instance
description: Use this tool to execute DDL statements to update the database schema.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "spanner-update-database-schema".          |
| source      |  string  |     true     | Name of the Spanner source to execute DDL against. |
| description |  string  |     true     | Description of the tool passed to the LLM.         |
