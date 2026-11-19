---
title: "Bigtable"
type: docs
description: "Details of the Bigtable prebuilt configuration."
---

## Bigtable

*   `--prebuilt` value: `bigtable`
*   **Environment Variables:**
    *   `BIGTABLE_PROJECT`: The GCP project ID.
    *   `BIGTABLE_INSTANCE`: The Bigtable instance ID.
*   **Permissions:**
    *   **Bigtable Reader** (`roles/bigtable.reader`) to execute SQL queries, list schemas, and get or list instances, clusters, tables, and views.
    *   **Bigtable Administrator** (`roles/bigtable.admin`) to create, update, or delete instances, clusters, tables, logical views, and materialized views.
*   **Tools:**
    *   `execute_sql`: Use this tool to execute a GoogleSQL query on Bigtable.
    *   `list_schemas`: List all Bigtable schemas, including tables with column family definitions, logical views, and materialized views.
    *   `list_tables`: List all Bigtable tables in the instance.
    *   `get_table`: Get details of a Bigtable table.
    *   `create_table`: Create a new Bigtable table.
    *   `update_table`: Update an existing Bigtable table's configuration.
    *   `delete_table`: Delete a Bigtable table.
    *   `list_instances`: List all Bigtable instances in the project.
    *   `get_instance`: Get details of a Bigtable instance.
    *   `create_instance`: Create a new Bigtable instance.
    *   `update_instance`: Update an existing Bigtable instance.
    *   `delete_instance`: Delete a Bigtable instance.
    *   `list_clusters`: List all Bigtable clusters in the instance.
    *   `get_cluster`: Get details of a Bigtable cluster.
    *   `create_cluster`: Create a new Bigtable cluster in an instance.
    *   `update_cluster`: Update the number of nodes in a Bigtable cluster.
    *   `delete_cluster`: Delete a Bigtable cluster.
    *   `list_logical_views`: List all Bigtable logical views in the instance.
    *   `get_logical_view`: Get details of a Bigtable logical view.
    *   `create_logical_view`: Create a new Bigtable logical view.
    *   `update_logical_view`: Update an existing Bigtable logical view.
    *   `delete_logical_view`: Delete a Bigtable logical view.
    *   `list_materialized_views`: List all existing Bigtable materialized views.
    *   `get_materialized_view`: Get information about an existing Bigtable materialized view.
    *   `create_materialized_view`: Create a new Bigtable materialized view.
    *   `update_materialized_view`: Update an existing Bigtable materialized view.
    *   `delete_materialized_view`: Delete an existing Bigtable materialized view.
*   **Toolsets:**
    *   `admin`: Instance and cluster lifecycle operations (create, get, list, update, delete instances and clusters).
    *   `data`: Schema discovery (`list_schemas`), SQL query execution (`execute_sql`), and table management (create, get, list, update, delete tables).
    *   `views`: Logical view and materialized view management (create, get, list, update, delete views).
