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
*   **Permissions:**
    *   **Cloud Spanner Database Reader** (`roles/spanner.databaseReader`) to
        execute DQL queries and list tables.
    *   **Cloud Spanner Database User** (`roles/spanner.databaseUser`) to
        execute DML queries.
*   **Tools:**
    *   `execute_sql`: Execute read-write SQL statements that modify the database (DML), such as INSERT, UPDATE, DELETE, or table alterations. Do not use this tool for standard data queries or SELECT statements.
    *   `execute_sql_readonly`: Execute read-only SQL queries (DQL) such as SELECT statements, metadata inspections, or information_schema queries. Use this tool as the default for any data retrieval tasks.
    *   `list_tables`: Lists tables in the database.
    *   `list_graphs`: Lists graphs in the database.
    *   `search_catalog`: Searches for data assets in Knowledge Catalog (Dataplex).
