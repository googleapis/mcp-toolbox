---
title: "looker-query-sql"
type: docs
weight: 1
description: >
  "looker-query-sql" generates a sql query using the Looker
  semantic model.
---

## About

The `looker-query-sql` generates a sql query using the Looker
semantic model.

`looker-query-sql` takes ten parameters:

1. the `model`
2. the `explore`
3. the `fields` list
4. an optional set of `filters`
5. an optional `filter_expression`
6. an optional `dynamic_fields`
7. an optional set of `pivots`
8. an optional set of `sorts`
9. an optional `limit`
10. an optional `tz`

Starting in Looker v25.18, these queries can be identified in Looker's
System Activity. In the History explore, use the field API Client Name
to find MCP Toolbox queries.

## Compatible Sources

{{< compatible-sources >}}

## Example

```yaml
kind: tool
name: query_sql
type: looker-query-sql
source: looker-source
description: |
  This tool generates the underlying SQL query that Looker would execute
  against the database for a given set of parameters. It is useful for
  understanding how Looker translates a request into SQL.

  Parameters:
  All parameters for this tool are identical to those of the `query` tool.
  This includes `model`, `explore`, `fields` (required),
  and optional parameters like `pivots`, `filters`, `filter_expression`, `dynamic_fields`, `sorts`, `limit`, and `tz`.

  Output:
  The result of this tool is the raw SQL text.
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "looker-query-sql"                         |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |
