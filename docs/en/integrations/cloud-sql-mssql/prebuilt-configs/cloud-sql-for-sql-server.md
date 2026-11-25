---
title: "Cloud SQL for SQL Server"
type: docs
description: "Details of the Cloud SQL for SQL Server prebuilt configuration."
---

## Cloud SQL for SQL Server

*   `--prebuilt` value: `cloud-sql-mssql`
*   **Environment Variables:**
    *   `CLOUD_SQL_MSSQL_PROJECT`: The GCP project ID.
    *   `CLOUD_SQL_MSSQL_REGION`: The region of your Cloud SQL instance.
    *   `CLOUD_SQL_MSSQL_INSTANCE`: The ID of your Cloud SQL instance.
    *   `CLOUD_SQL_MSSQL_DATABASE`: The name of the database to connect to.
    *   `CLOUD_SQL_MSSQL_USER`: The database username.
    *   `CLOUD_SQL_MSSQL_PASSWORD`: The password for the database user.
    *   `CLOUD_SQL_MSSQL_IP_TYPE`: (Optional) The IP type i.e. "Public" or
        "Private" (Default: Public).
*   **Permissions:**
    *   **Cloud SQL Client** (`roles/cloudsql.client`) to connect to the
        instance.
    *   Database-level permissions (e.g., `SELECT`, `INSERT`) are required to
        execute queries.
    *   **Cloud SQL Viewer** (`roles/cloudsql.viewer`), **Cloud SQL Editor**
        (`roles/cloudsql.editor`), or **Cloud SQL Admin**
        (`roles/cloudsql.admin`) is required for the instance management
        tools, depending on the tool. See the Cloud SQL for SQL Server Admin
        prebuilt config for the per-tool breakdown.
    *   **Monitoring Viewer** (`roles/monitoring.viewer`) is required on the
        project for `get_system_metrics`.
*   **Tools:**
    *   `execute_sql`: Executes a SQL query.
    *   `list_tables`: Lists tables in the database.
    *   `get_system_metrics`: Fetches system level cloud monitoring data
        (timeseries metrics) for a SQL Server instance using a PromQL query.
    *   `restore_backup`: Restores a backup of a Cloud SQL instance.
    *   `clone_instance`: Creates a clone for an existing Cloud SQL for SQL
        Server instance.
    *   `get_instance`: Gets information about a Cloud SQL instance.
    *   `list_instances`: Lists Cloud SQL instances in a project.
    *   `create_database`: Creates a new database in a Cloud SQL instance.
    *   `list_databases`: Lists all databases for a Cloud SQL instance.
    *   `create_user`: Creates a new user in a Cloud SQL instance.
    *   `create_backup`: Creates a backup on a Cloud SQL instance.
    *   `create_instance`: Creates a new Cloud SQL for SQL Server instance.
    *   `wait_for_operation`: Waits for a Cloud SQL operation to complete.
