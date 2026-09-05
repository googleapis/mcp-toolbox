---
title: "Spanner (PostgreSQL dialect)"
type: docs
description: "Details of the Spanner (PostgreSQL dialect) prebuilt configuration."
---

## Spanner (PostgreSQL dialect)

*   `--prebuilt` value: `spanner-postgres`
*   **Environment Variables:**
    *   `SPANNER_PROJECT`: The GCP project ID.
    *   `SPANNER_INSTANCE`: The Spanner instance ID.
    *   `SPANNER_DATABASE`: The Spanner database ID.
    *   `SPANNER_READONLY`: Optional. Set to `true` to enforce read-only execution at the database protocol level and suppress write-capable tools. Default: `false`.
*   **Permissions:**
    *   **Cloud Spanner Database Reader** (`roles/spanner.databaseReader`) to
        execute DQL queries and list tables.
    *   **Cloud Spanner Database User** (`roles/spanner.databaseUser`) to
        execute DML queries.
*   **Tools:**
    *   `execute_sql`: Executes a DML SQL query using the PostgreSQL interface
        for Spanner.
    *   `execute_sql_readonly`: Execute read-only SQL queries (DQL) such as SELECT statements, metadata inspections, or information_schema queries.
    *   `list_tables`: Lists tables in the database.
    *   `search_catalog`: Searches for data assets in Knowledge Catalog (Dataplex).
