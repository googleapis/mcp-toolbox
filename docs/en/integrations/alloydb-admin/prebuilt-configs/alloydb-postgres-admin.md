---
title: "AlloyDB Postgres Admin"
type: docs
description: "Details of the AlloyDB Postgres Admin prebuilt configuration."
---

## AlloyDB Postgres Admin

*   `--prebuilt` value: `alloydb-postgres-admin`
*   **Environment Variables:**
    *   `ALLOYDB_POSTGRES_PROJECT`: (Optional) The GCP project ID to use as the default for AlloyDB infrastructure tools.
    *   `ALLOYDB_POSTGRES_READONLY`: (Optional) When set to `true`, suppresses write-capable admin tools (e.g., `create_cluster`, `create_instance`, `create_user`). Default: `false`.
*   **Permissions:**
    *   **AlloyDB Viewer** (`roles/alloydb.viewer`) is required for `list` and
        `get` tools.
    *   **AlloyDB Admin** (`roles/alloydb.admin`) is required for `create` tools.
*   **Tools:**
    *   `create_cluster`: Creates a new AlloyDB cluster.
    *   `list_clusters`: Lists all AlloyDB clusters in a project.
    *   `get_cluster`: Gets information about a specified AlloyDB cluster.
    *   `create_instance`: Creates a new AlloyDB instance within a cluster.
    *   `list_instances`: Lists all instances within an AlloyDB cluster.
    *   `get_instance`: Gets information about a specified AlloyDB instance.
    *   `create_user`: Creates a new database user in an AlloyDB cluster.
    *   `list_users`: Lists all database users within an AlloyDB cluster.
    *   `get_user`: Gets information about a specified database user in an
        AlloyDB cluster.
    *   `wait_for_operation`: Polls the operations API to track the status of
        long-running operations.
