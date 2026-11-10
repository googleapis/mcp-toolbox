---
title: "databaseinsights-get-advanced-time-series-query-stats"
type: docs
weight: 3
description: >
  Fetches time-series performance trends for queries executed on AlloyDB instances.
---

## About

The `databaseinsights-get-advanced-time-series-query-stats` tool retrieves historical time-series performance metrics for AlloyDB queries to help analyze latency trends over time and identify performance spikes.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

### IAM Permissions

- `databaseinsights.queryTimeSeries.fetch` on the target location/project, which is granted by:
  - **Database Insights Viewer** (`roles/databaseinsights.viewer`)
  - **Monitoring Viewer** (`roles/monitoring.viewer`)

## Parameters

### `parent` (String, Required)

Project and location in the format `projects/{project_id}/locations/{location}`.

### `full_resource_name` (String, Required)

The full resource identifier for the database instance (e.g., `//alloydb.googleapis.com/projects/{project_id}/locations/{location}/clusters/{cluster_id}/instances/{instance_id}`).

### `start_time` (String, Optional)

Beginning of the time interval in RFC3339 format.

### `end_time` (String, Optional)

End of the time interval in RFC3339 format.

### `database` (String, Optional)

Filter results to a specific database.

### `username` (String, Optional)

Filter results to a specific database user.

### `query_id` (String, Optional)

Fetch history for a single specific query hash.

## Example

**YAML Configuration:**
```yaml
kind: tool
name: get_query_time_series
type: databaseinsights-get-advanced-time-series-query-stats
source: database-insights-source
```

**Sample CLI Invocation:**
```bash
./toolbox invoke get_advanced_time_series_query_stats \
  --prebuilt alloydb-postgres-observability \
  '{"parent":"projects/PROJECT_ID/locations/REGION","full_resource_name":"//alloydb.googleapis.com/projects/PROJECT_ID/locations/REGION/clusters/CLUSTER_ID/instances/INSTANCE_ID"}'
```

## Output Format

Returns a structured JSON object containing a `timeseries` array of interval data points and schema `metadata`:

```json
{
  "timeseries": [
    {
      "groupbyFieldValues": [""],
      "values": [
        {
          "interval": {
            "endTime": "2026-08-11T18:46:30Z"
          },
          "value": [796.93, 0]
        }
      ]
    }
  ],
  "metadata": {
    "fields": [
      {"name": "rate(execution_time)", "type": "DOUBLE"},
      {"name": "rate(wait_time)", "type": "DOUBLE"}
    ]
  }
}
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| :---------- | :------: | :----------: | :------------------------------------------------- |
| type        |  string  |     true     | Must be "databaseinsights-get-advanced-time-series-query-stats". |
| source      |  string  |     true     | Name of the source the tool should execute on.     |
| description |  string  |    false     | Optional description override.                     |

## Additional Resources

*   [OneMCP Reference: get_advanced_time_series_query_stats](https://docs.cloud.google.com/alloydb/docs/reference/mcp/databaseinsights/mcp/tools_list/get_advanced_time_series_query_stats)
*   [Use Database Insights with Model Context Protocol (MCP)](https://cloud.google.com/alloydb/docs/ai/use-database-insights-mcp)

