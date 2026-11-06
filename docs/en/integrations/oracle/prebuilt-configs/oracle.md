---
title: "Oracle"
type: docs
description: "Details of the Oracle prebuilt configuration."
---

## Oracle

*   `--prebuilt` value: `oracledb`
*   **Environment Variables:**
   
    *   `ORACLE_CONNECTION_STRING`: The connection string for the Oracle server (e.g., "hostname:port/servicename"). Required if not using `ORACLE_TNS_ALIAS`.
    *   `ORACLE_TNS_ALIAS`: The TNS connection alias. Required if not using `ORACLE_CONNECTION_STRING`.
    *   `ORACLE_TNS_ADMIN`: The directory containing `tnsnames.ora`, `sqlnet.ora`, and/or wallet credentials files (used with `ORACLE_USE_OCI=true`).
    *   `ORACLE_USERNAME`: The database username. Required unless `ORACLE_USE_OCI=true` and passwordless wallet/external authentication is configured.
    *   `ORACLE_PASSWORD`: The database user's password. Required unless `ORACLE_USE_OCI=true` and passwordless wallet/external authentication is configured.
    *   `ORACLE_WALLET`: The path to the Oracle DB Wallet folder for the pure-Go driver (`ORACLE_USE_OCI=false`).
    *   `ORACLE_USE_OCI`: A boolean flag (`true` or `false`) indicating whether to use the OCI-based driver. Setting to `true` is required for passwordless Oracle Wallet / external authentication (SEPS) and requires the Oracle Instant Client libraries to be installed.
*   **Permissions:**
    *   Database-level permissions (e.g., `SELECT`, `INSERT`) are required to execute queries.
    *   For queries on DBA views like `dba_data_files` and `dba_free_space`, access typically requires elevated database privileges (like `SELECT_CATALOG_ROLE` or direct grants) that a standard user may not have.
*   **Tools:**
    *   `execute_sql`: Executes a SQL query.
    *   `list_tables`: Lists tables in the database.
    *   `list_active_sessions`: Lists active database sessions.
    *   `get_query_plan`: Generate a full execution plan for a single SQL statement.
    *   `list_top_sql_by_resource`: Lists top SQL statements by resource usage.
    *   `list_tablespace_usage`: Lists tablespace usage.
    *   `list_invalid_objects`: Lists invalid objects.

## Examples

### Example 1: Standard Username/Password Authentication
This is the standard authentication method using a direct database connection string along with a database username and password.

```bash
export ORACLE_CONNECTION_STRING="db-host:1521/ORCL"
export ORACLE_USERNAME="myuser"
export ORACLE_PASSWORD="mypassword"
export ORACLE_USE_OCI=false

# Start toolbox
./toolbox --prebuilt oracledb -a 0.0.0.0 --port 9200
```

### Example 2: Passwordless Oracle Wallet Authentication (SEPS/External Auth)
This method is used when you want to connect passwordlessly using an Oracle Wallet (Secure External Password Store or SSL client certificates). It requires `ORACLE_USE_OCI=true` and the Oracle Instant Client libraries installed on the target host/container.

```bash
export ORACLE_USE_OCI=true
export ORACLE_TNS_ADMIN="/oracle/database/wallet"
export ORACLE_TNS_ALIAS="MY_DB_ALIAS"
# Leave ORACLE_USERNAME and ORACLE_PASSWORD empty or unset

# Start toolbox
./toolbox --prebuilt oracledb -a 0.0.0.0 --port 9200
```
