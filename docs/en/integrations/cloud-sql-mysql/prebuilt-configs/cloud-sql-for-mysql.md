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
    *   **Cloud SQL Viewer** (`roles/cloudsql.viewer`), **Cloud SQL Editor**
        (`roles/cloudsql.editor`), or **Cloud SQL Admin**
        (`roles/cloudsql.admin`) is required for the instance management
        tools, depending on the tool. See the Cloud SQL for MySQL Admin
        prebuilt config for the per-tool breakdown.
    *   **Monitoring Viewer** (`roles/monitoring.viewer`) is required on the
        project for `get_system_metrics` and `get_query_metrics`.
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
    *   `create_backup`: Creates a backup on a Cloud SQL instance.
    *   `restore_backup`: Restores a backup of a Cloud SQL instance.
    *   `clone_instance`: Creates a clone for an existing Cloud SQL for MySQL
        instance.
    *   `list_instances`: Lists Cloud SQL instances in a project.
    *   `create_instance`: Creates a new Cloud SQL for MySQL instance.
    *   `create_database`: Creates a new database in a Cloud SQL instance.
    *   `get_system_metrics`: Fetches system level cloud monitoring data
        (timeseries metrics) for a MySQL instance using a PromQL query.
    *   `create_user`: Creates a new user in a Cloud SQL instance.
    *   `get_query_metrics`: Fetches query level cloud monitoring data
        (timeseries metrics) for queries running in a MySQL instance using a
        PromQL query.
    *   `wait_for_operation`: Waits for a Cloud SQL operation to complete.
    *   `get_instance`: Gets information about a Cloud SQL instance.
    *   `list_databases`: Lists all databases for a Cloud SQL instance.
