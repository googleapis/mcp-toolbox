---
title: "mysql-list-table-stats"
type: docs
weight: 1
description: >
  A "mysql-list-table-stats" tool report table statistics including table size, total latency, rows read, rows written, read and write latency for entire instance, a specified database, or a specified table.
---

## About

A `mysql-list-table-stats` tool generates table-level performance and resource consumption statistics to facilitate bottleneck identification and workload analysis.

`mysql-list-table-stats` outputs detailed table-level resource consumption including estimated row counts, table size, a complete breakdown of CRUD activity (rows fetched, inserted, updated, and deleted), and IO statistics such as total, read, write and miscellaneous latency. The output is a JSON formatted array of the top 10 MySQL tables ranked by total latency. 

Below are some use cases for `mysql-list-table-stats`
- **Finding hottest tables**: Identify tables with highest total latency, read or writes based on the `sort_by` column. 
- **Finding tables with most reads**: Identify tables with highest reads by sorting on `rows_fetched`. 
- **Monitoring growth**: Track `row_count` and `size_MB` of table over time to estimate growth."

## Compatible Sources

{{< compatible-sources others="integrations/cloud-sql-mysql">}}

## Requirements

- `performance_schema` should be turned ON for this tool to work. 

## Parameters

This tool takes 4 optional input parameters:

- `table_schema` (optional): The database where table stats check is to be
  executed. Check all tables visible to the current database if not specified.
- `table_name` (optional): Name of the table to be checked. Check all tables
  visible to the current user if not specified.
- `sort_by` (optional): The column to sort by. Valid values are `row_count`, `rows_fetched`, `rows_inserted`, `rows_updated`, `rows_deleted`, `total_latency_secs` (defaults to `total_latency_secs`)
- `limit` (optional): Max rows to return, default 10.

## Example

```yaml
kind: tool
name: list_table_stats
type: mysql-list-table-stats
source: my-mysql-instance
description: Display table statistics including table size, total latency, rows read, rows written, read and write latency for entire instance, a specified database, or a specified table. Specifying a database name or table name filters the output to that specific db or table. Results are limited to 10 by default.
```

## Output Format
 
The response is a json array with the following fields:
```json
[
  {
  "table_schema": "The schema/database this table belongs to",
  "table_name": "Name of this table",
  "size_MB": "Size of the table data in MB",
  "row_count": "Number of rows in the table",
  "total_latency_secs": "total latency in secs",
  "rows_fetched": "total number of rows fetched",
  "rows_inserted": "total number of rows inserted",
  "rows_updated": "total number of rows updated",
  "rows_deleted": "total number of rows deleted",
  "io_reads": "total number of io read requests",
  "io_read_latency": "io read latency in seconds",
  "io_write_latency": "io write latency in seconds",
  "io_misc_latency": "io misc latency in seconds",
  }
]
```

## Reference

| **field**   | **type** | **required** | **description**                                    |
|-------------|:--------:|:------------:|----------------------------------------------------|
| type        |  string  |     true     | Must be "mysql-list-table-stats".                  |
| source      |  string  |     true     | Name of the source the SQL should execute on.      |
| description |  string  |     true     | Description of the tool that is passed to the LLM. |


