---
title: "databaseinsights-get-advanced-time-series-wait-event-stats"
type: docs
weight: 4
description: >
  Fetches time-series wait event statistics for AlloyDB instances over time.
---

## About

The `databaseinsights-get-advanced-time-series-wait-event-stats` tool retrieves time-series wait event metrics (IO, Lock, CPU) for an AlloyDB instance over a specified period to visualize how contention evolves.

## Compatible Sources

{{< compatible-sources >}}

## Requirements

### IAM Permissions

- `databaseinsights.waitEventTimeSeries.fetch` on the target location/project, which is granted by:
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

Filter wait event history to a specific database.

### `username` (String, Optional)

Filter wait event history to a specific database user.

### `query_id` (String, Optional)

Breakdown wait events over time for a specific query hash.

### `view` (String, Optional)

Aggregation level (`WAIT_CLASS` or `WAIT_EVENT`). Defaults to `WAIT_CLASS`.

## Example

**YAML Configuration:**
```yaml
kind: tool
name: get_wait_event_time_series
type: databaseinsights-get-advanced-time-series-wait-event-stats
source: database-insights-source
```

**Sample CLI Invocation:**
```bash
./toolbox invoke get_advanced_time_series_wait_event_stats \
  --prebuilt alloydb-postgres-observability \
  '{"parent":"projects/PROJECT_ID/locations/REGION","full_resource_name":"//alloydb.googleapis.com/projects/PROJECT_ID/locations/REGION/clusters/CLUSTER_ID/instances/INSTANCE_ID","view":"WAIT_CLASS"}'
```

## Output Format

Returns a structured JSON object containing a `timeseries` dataset of time-series wait event points and `metadata`:

```json
{
  "timeseries": [
    {
      "groupbyFieldValues": [""]
    }
  ],
  "metadata": {
    "fields": [
      {"name": "rate(time_spent)", "type": "DOUBLE"}
    ]
  }
}
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
| :---------- | :------: | :----------: | :------------------------------------------------- |
| type        |  string  |     true     | Must be "databaseinsights-get-advanced-time-series-wait-event-stats". |
| source      |  string  |     true     | Name of the source the tool should execute on.     |
| description |  string  |    false     | Optional description override.                     |

## Additional Resources

*   [OneMCP Reference: get_advanced_time_series_wait_event_stats](https://docs.cloud.google.com/alloydb/docs/reference/mcp/databaseinsights/mcp/tools_list/get_advanced_time_series_wait_event_stats)
*   [Use Database Insights with Model Context Protocol (MCP)](https://cloud.google.com/alloydb/docs/ai/use-database-insights-mcp)

