---
title: "databaseinsights-get-index-recommendations"
type: docs
weight: 5
description: >
  Fetches index recommendations for specified query IDs across databases on AlloyDB instances.
---

## About

The `databaseinsights-get-index-recommendations` tool retrieves index advisor recommendations for specified query IDs across databases. It identifies missing indexes, provides exact SQL creation commands, and calculates estimated performance gains.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

### IAM Permissions

- `databaseinsights.indexRecommendations.batchQuery`, `databaseinsights.recommendations.query`, and `databaseinsights.resourceRecommendations.query` on the target location/project, which are granted by:
  - **Database Insights Viewer** (`roles/databaseinsights.viewer`)
  - **Monitoring Viewer** (`roles/monitoring.viewer`)

## Parameters

### `parent` (String, Required)

Project and location in the format `projects/{project_id}/locations/{location}`.

### `full_resource_name` (String, Required)

The full resource identifier for the database instance (e.g., `//alloydb.googleapis.com/projects/{project_id}/locations/{location}/clusters/{cluster_id}/instances/{instance_id}`).

### `database_query_ids` (Array of Objects, Optional)

A list of objects specifying database names and target query IDs (e.g., `[{"database": "postgres", "query_ids": ["1106633582131931382"]}]`).

## Example

**YAML Configuration:**
```yaml
kind: tool
name: get_index_recommendations
type: databaseinsights-get-index-recommendations
source: database-insights-source
```

**Sample CLI Invocation:**
```bash
./toolbox invoke get_index_recommendations \
  --prebuilt alloydb-postgres-observability \
  '{"parent":"projects/PROJECT_ID/locations/REGION","full_resource_name":"//alloydb.googleapis.com/projects/PROJECT_ID/locations/REGION/clusters/CLUSTER_ID/instances/INSTANCE_ID","database_query_ids":[{"database":"postgres","query_ids":["1106633582131931382"]}]}'
```

## Output Format

Returns index advisor recommendations (`sql_command`, `schema`, `relation`, `columns`, `estimated_storage_size_bytes`) and predicted query performance improvements:

```json
{
  "full_resource_name": "//alloydb.googleapis.com/clusters/mg-obs/instances/mg-obs-primary",
  "database_index_recommendations": [
    {
      "database": "postgres",
      "index_recommendations": [
        {
          "sql_command": "CREATE INDEX ON \"public\".\"orders\"(\"o_orderdate\")",
          "schema": "\"public\"",
          "relation": "\"orders\"",
          "columns": [
            "\"o_orderdate\""
          ],
          "estimated_storage_size_bytes": "26124288",
          "impacted_query_ids": [],
          "impacted_queries_count": 2
        }
      ],
      "recommendation_time": {
        "seconds": "1773214452"
      },
      "query_improvements": [
        {
          "key": "-9180744549948249982",
          "value": {
            "query_id": "-9180744549948249982",
            "index_recommendation_ids": [
              "0"
            ],
            "current_total_execution_duration": {
              "nanos": 25125664
            },
            "estimated_new_total_execution_duration": {
              "nanos": 17142
            }
          }
        }
      ]
    }
  ]
}
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| :---------- | :------: | :----------: | :------------------------------------------------- |
| type        |  string  |     true     | Must be "databaseinsights-get-index-recommendations". |
| source      |  string  |     true     | Name of the source the tool should execute on.     |
| description |  string  |    false     | Optional description override.                     |

## Additional Resources

*   [OneMCP Reference: get_index_recommendations](https://docs.cloud.google.com/alloydb/docs/reference/mcp/databaseinsights/mcp/tools_list/get_index_recommendations)
*   [Use Database Insights with Model Context Protocol (MCP)](https://cloud.google.com/alloydb/docs/ai/use-database-insights-mcp)

