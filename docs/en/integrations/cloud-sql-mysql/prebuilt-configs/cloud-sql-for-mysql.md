---
title: "Cloud SQL for MySQL"
type: docs
description: "Details of the Cloud SQL for MySQL prebuilt configuration."
---

## Cloud SQL for MySQL

*   `--prebuilt` value: `cloud-sql-mysql`
*   **Environment Variables:**
    *   `CLOUD_SQL_MYSQL_PROJECT`: The GCP project ID.
    *   `CLOUD_SQL_MYSQL_REGION`: The region of your Cloud SQL instance.
    *   `CLOUD_SQL_MYSQL_INSTANCE`: The ID of your Cloud SQL instance.
    *   `CLOUD_SQL_MYSQL_DATABASE`: The name of the database to connect to.
    *   `CLOUD_SQL_MYSQL_USER`: The database username.
    *   `CLOUD_SQL_MYSQL_PASSWORD`: The password for the database user.
    *   `CLOUD_SQL_MYSQL_IP_TYPE`: The IP type i.e. "Public" or "Private" (Default: Public).
    *   `CLOUD_SQL_MYSQL_READONLY`: (Optional) When set to `true`, enforces read-only execution on the database connection (`read_only_connection:true`) and suppresses write-capable tools. Default: `false`.
*   **Permissions:**
    *   **Cloud SQL Client** (`roles/cloudsql.client`) to connect to the
        instance.
    *   Database-level permissions (e.g., `SELECT`, `INSERT`) are required to
        execute queries.
*   **Tools:**
    *   `execute_sql`: Executes a SQL query.
    *   `list_tables`: Lists tables in the database.
    *   `get_query_plan`: Provides information about how MySQL executes a SQL
        statement.
    *   `list_active_queries`: Lists ongoing queries.
    *   `list_tables_missing_unique_indexes`: Looks for tables that do not have
        primary or unique key contraint.
    *   `list_table_fragmentation`: Displays table fragmentation in MySQL.
    *   `list_all_locks`: Lists all current locks on the database.
    *   `show_query_stats`: Show query execution statistics.
    *   `list_table_stats`: Displays table statistics in MySQL.
