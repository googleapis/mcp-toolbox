---
title: "mysql-show-query-stats"
type: docs
weight: 1
description: >
  A "mysql-show-query-stats" tool report query execution statistics including execution count, total and average latency, max latency, total rows examined, full table scans, and inefficient index usage for all queries on a specified database or all databases as requested.
---

## About
`mysql-show-query-stats` tool retrieves a database level query statistics from performance_schema.events_statements_summary_by_digest to identify slow and inefficient queries and provides necessary data to perform effective query tuning.

`mysql-show-query-stats` outputs detailed query statistics including total latency, average latency, maximum latency, total rows sent, total rows examined, full table scan count, inefficient index usage count and last executed timestamp. The output format is JSON array of top 10 queries ranked by total latency.

## Compatible Sources

{{< compatible-sources others="integrations/cloud-sql-mysql">}}

## Requirements

- `performance_schema` should be turned ON for this tool to work. 

## Parameters

This tool takes 2 optional input parameters:
- `table_schema` (optional): The target database for query statistics. If not specified the results will be displayed for all databases. 
- `limit` (optional): Max rows to return, default 10.
- `connected_schema` (optional): The database user is connected to, the value is set from env variable `CLOUD_SQL_MYSQL_DATABASE` or `MYSQL_DATABASE`.

## Example

```yaml
kind: tool
name: show_query_stats
type: mysql-show-query-stats
source: my-mysql-instance
description: Shows query execution statistics including execution count, total and average latency, max latency, total rows examined, full table scans, and inefficient index usage for all queries on a specified database or from all databases. Results are limited to 10 by default.
```

## Output Format

The response is a json array with the following fields:
```json
[
  {
  "table_schema": "The schema/database this table belongs to",
  "table_name": "Name of this table",
  "exec_count": "Number of times query is executed",
  "total_latency_ms": "total latency in milli seconds",
  "average_latency_ms": "average latency in milli seconds",
  "max_latency_ms": "maximum latency in milli seconds",
  "total_rows_sent": "total number of rows sent",
  "sum_rows_examined": "total number of rows examined",
  "sum_no_index_used": "count of full table scan",
  "sum_no_good_index_used": "count of inefficient index use",
  "last_seen": "time when query was last seen"
  }
]
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "mysql-show-query-stats".                  |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |

