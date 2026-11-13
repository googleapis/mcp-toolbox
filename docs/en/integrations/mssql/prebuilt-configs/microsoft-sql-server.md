---
title: "Microsoft SQL Server"
type: docs
description: "Details of the Microsoft SQL Server prebuilt configuration."
---

## Microsoft SQL Server

*   `--prebuilt` value: `mssql`
*   **Environment Variables:**
    *   `MSSQL_HOST`: (Optional) The hostname or IP address of the SQL Server instance.
    *   `MSSQL_PORT`: (Optional) The port number for the SQL Server instance.
    *   `MSSQL_DATABASE`: The name of the database to connect to.
    *   `MSSQL_USER`: The database username.
    *   `MSSQL_PASSWORD`: The password for the database user.
*   **Permissions:**
    *   Database-level permissions (e.g., `SELECT`, `INSERT`) are required to
        execute queries.
*   **Tools:**
    *   `execute_sql`: Executes a SQL query.
    *   `list_tables`: Lists tables in the database.
    *   `get_database_object_source_code`: Retrieves source code or metadata for database objects.
