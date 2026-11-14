---
title: "looker-create-view-from-table"
type: docs
weight: 1
description: >
  A "looker-create-view-from-table" tool generates boilerplate LookML views directly from the database schema.
---

## About

A `looker-create-view-from-table` tool triggers the automatic generation of LookML view files based on database tables.

`looker-create-view-from-table` accepts `project_id`, `connection`, and `tables` parameters, along with optional `database` and `folder_name` parameters. Each entry in `tables` requires `table_name` and accepts optional `schema`, `primary_key`, `base_view`, and `columns` fields.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: create_view_from_table
type: looker-create-view-from-table
source: looker-source
description: |
  This tool generates boilerplate LookML views directly from the database schema.
  It does not create model or explore files, only view files in the specified folder.

  Prerequisite: The Looker session must be in Development Mode. Use `dev_mode: true` first.

  Parameters:
  - project_id (required): The unique ID of the LookML project.
  - connection (required): The database connection name.
  - database (optional): The BigQuery project or database where the target table resides. May be required even when using the connection's default database.
  - tables (required): A list of objects to generate views for. Each object requires `table_name` (note: table names are case-sensitive) and accepts optional `schema`, `primary_key`, `base_view`, and `columns` (array of objects with `column_name`).
  - folder_name (optional): The folder to place the view files in (defaults to 'views').

  Output:
  A confirmation message upon successful view generation, or an error message if the operation fails.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-create-view-from-table".           |
| source      |  string  |     true     | Name of the source Looker instance.                |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
