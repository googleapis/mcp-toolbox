---
title: "AlloyDB Postgres Observability"
type: docs
description: "Details of the AlloyDB Postgres Observability prebuilt configuration combining Cloud Monitoring and Database Insights."
---

## AlloyDB Postgres Observability

*   `--prebuilt` value: `alloydb-postgres-observability`
*   **Permissions:**
    *   **Monitoring Viewer** (`roles/monitoring.viewer`) is required on the project to view system and query time-series metrics.
    *   **Database Insights Viewer** (`roles/databaseinsights.viewer`) is required to retrieve aggregated query statistics, wait events, historical trends, and index advisor recommendations.
*   **Tools:**
    *   `get_system_metrics`: Fetches system level cloud monitoring data (timeseries metrics) for an AlloyDB instance using a PromQL query.
    *   `get_query_metrics`: Fetches query level cloud monitoring data (timeseries metrics) for queries running in an AlloyDB instance using a PromQL query.
    *   `get_advanced_aggregated_query_stats`: Fetches aggregated query execution statistics for a requested AlloyDB instance within a specified time period. Supports filtering by database name, database user, and a specific query ID, as well as pagination. Returns statistics including query ID, database name, total execution time (`sum(execution_time)`), average execution time (`avg(execution_time)`), total execution count (`sum(count)`), total wait time (`sum(wait_time)`), and normalized query text. Requires Advanced Query Insights to be enabled.
    *   `get_advanced_aggregated_wait_event_stats`: Fetches aggregated wait event statistics to identify performance bottlenecks for a requested AlloyDB instance within a specified time period. Supports filtering by database name, database user, and a specific query ID, selecting the aggregation level (by wait class or granular wait event), as well as pagination. Returns statistics including wait class or event name, total time spent (`sum(time_spent)`), average time spent (`avg(time_spent)`), and total wait count (`sum(count)`). Requires Advanced Query Insights to be enabled.
    *   `get_advanced_time_series_query_stats`: Fetches time-series history of query execution statistics to analyze trends and spikes for a requested AlloyDB instance within a specified time period. Supports filtering by database name, database user, and a specific query ID. Returns time-series data including rates of execution time (`rate(execution_time)`) and wait time (`rate(wait_time)`). Requires Advanced Query Insights to be enabled.
    *   `get_advanced_time_series_wait_event_stats`: Fetches time-series history of wait event statistics to analyze contention trends for a requested AlloyDB instance within a specified time period. Supports filtering by database name, database user, and a specific query ID, as well as selecting the aggregation level (by wait class or granular wait event). Returns time-series data including rate of time spent (`rate(time_spent)`) grouped by wait class or event. Requires Advanced Query Insights to be enabled.
    *   `get_index_recommendations`: Fetches index advisor suggestions to optimize performance for a requested AlloyDB instance. Supports requesting recommendations for specific databases and a list of query IDs. Returns index recommendations including SQL commands (`CREATE INDEX`), target schema, relation, and columns, estimated storage size, and predicted query performance improvements (current vs estimated execution duration).
*   **Available Toolsets (Filtered Groups):**
    *   `alloydb_postgres_cloud_monitoring_tools`: Exposes only the 2 Cloud Monitoring metrics tools (`get_system_metrics`, `get_query_metrics`).
    *   `alloydb_postgres_database_insights_tools`: Exposes only the 5 Database Insights diagnostic tools (`get_advanced_...` and `get_index_recommendations`).
    *   `alloydb_postgres_observability_tools`: Exposes all 7 observability tools.

## Additional Resources

These tools are also available via the remote Google Cloud OneMCP server for AlloyDB. For detailed reference documentation on remote OneMCP integration and index advisor capabilities, see:

*   [Monitor AlloyDB using the Database Insights MCP server](https://cloud.google.com/alloydb/docs/ai/use-database-insights-mcp)
*   [AlloyDB Database Insights MCP Reference](https://cloud.google.com/alloydb/docs/reference/mcp/databaseinsights/mcp)



