---
title: "bigtable-list-schemas"
type: docs
weight: 1
description: >
  The "bigtable-list-schemas" tool lists all Bigtable schemas, including table column families, logical views, and materialized views with dynamically resolved column schemas.
---

## About

The `bigtable-list-schemas` tool retrieves comprehensive schema information for all resources within a Google Cloud Bigtable instance. It provides Large Language Models (LLMs) with full visibility into:

- **Tables:** Table names along with their configured column family definitions (`FamilyList`).
- **Logical Views & Materialized Views:** View definitions (`LogicalViewID`, `MaterializedViewID`, and GoogleSQL `Query`) alongside dynamically resolved column schemas (`name` and `type`) extracted from the view query.

## Compatible Sources

{{< compatible-sources >}}

## Parameters

`bigtable-list-schemas` accepts the following parameters:

- **`limit`** (integer): Optional maximum number of tables to return (default: 20).

## Example

```yaml
kind: tool
name: bigtable_list_schemas
type: bigtable-list-schemas
source: my-bigtable-source
description: List all Bigtable tables, column families, and SQL view column schemas.
```

## Output Format

The tool returns a structured JSON payload containing `tables`, `logical_views`, and `materialized_views`:

```json
{
  "tables": [
    {
      "table_name": "users",
      "info": {
        "FamilyList": ["profile", "activity"]
      }
    }
  ],
  "logical_views": [
    {
      "LogicalViewID": "active_users_view",
      "Query": "SELECT _key, CAST(profile['name'] AS STRING) AS name FROM users",
      "columns": [
        {
          "name": "_key",
          "type": "BYTES"
        },
        {
          "name": "name",
          "type": "STRING"
        }
      ]
    }
  ],
  "materialized_views": []
}
```

## Reference

| **field** | **type** | **required** | **description** |
|-----------|----------|--------------|-----------------|
| `type` | string | true | Must be `bigtable-list-schemas`. |
| `source` | string | true | Name of the Bigtable source to execute on. |
| `description` | string | false | Custom description of the tool passed to the LLM. |
