---
title: "Spanner (GoogleSQL dialect)"
type: docs
description: "Details of the Spanner (GoogleSQL dialect) prebuilt configuration."
---

## Spanner (GoogleSQL dialect)

*   `--prebuilt` value: `spanner`
*   **Environment Variables:**
    *   `SPANNER_PROJECT`: The GCP project ID.
    *   `SPANNER_INSTANCE`: The Spanner instance ID.
    *   `SPANNER_DATABASE`: The Spanner database ID.
    *   `SPANNER_READONLY`: Optional. Set to `true` to enforce read-only execution at the database protocol level and suppress write-capable tools. Default: `false`.
*   **Permissions:**
    *   **Cloud Spanner Database Reader** (`roles/spanner.databaseReader`) to
        execute DQL queries, list tables, and get database DDL.
    *   **Cloud Spanner Database User** (`roles/spanner.databaseUser`) to
        execute DML queries.
    *   **Cloud Spanner Database Admin** (`roles/spanner.databaseAdmin`) to
        execute DDL schema updates (`update_database_schema`).
*   **Tools:**
    *   `execute_sql`: Execute read-write SQL statements that modify the database (DML), such as INSERT, UPDATE, DELETE, or table alterations. Do not use this tool for standard data queries or SELECT statements.
    *   `execute_sql_readonly`: Execute read-only SQL queries (DQL) such as SELECT statements, metadata inspections, or information_schema queries. Use this tool as the default for any data retrieval tasks.
    *   `list_tables`: Lists tables in the database.
    *   `list_graphs`: Lists graphs in the database.
    *   `search_catalog`: Searches for data assets in Knowledge Catalog (Dataplex).
    *   `update_database_schema`: Execute DDL statements to modify the Spanner database schema.
    *   `get_database_ddl`: Retrieve the DDL statements that define the schema of the Cloud Spanner database.
