---
title: "AlloyDB Postgres"
type: docs
description: "Details of the AlloyDB Postgres prebuilt configuration."
---

## AlloyDB Postgres

*   `--prebuilt` value: `alloydb-postgres`
*   **Environment Variables:**
    *   `ALLOYDB_POSTGRES_PROJECT`: The GCP project ID.
    *   `ALLOYDB_POSTGRES_REGION`: The region of your AlloyDB instance.
    *   `ALLOYDB_POSTGRES_CLUSTER`: The ID of your AlloyDB cluster.
    *   `ALLOYDB_POSTGRES_INSTANCE`: The ID of your AlloyDB instance.
    *   `ALLOYDB_POSTGRES_DATABASE`: The name of the database to connect to.
    *   `ALLOYDB_POSTGRES_USER`: (Optional) The database username. Defaults to
        IAM authentication if unspecified.
    *   `ALLOYDB_POSTGRES_PASSWORD`: (Optional) The password for the database
        user. Defaults to IAM authentication if unspecified.
    *   `ALLOYDB_POSTGRES_IP_TYPE`: (Optional) The IP type i.e. "Public" or
        "Private" (Default: Public).
    *   `ALLOYDB_POSTGRES_READONLY`: (Optional) When set to `true`, enforces read-only execution at the database session level (`alloydb_session_read_only=locked`) and suppresses write-capable tools. If you encounter an unsupported parameter error, make sure your AlloyDB instance has Postgres version 17+. Default: `false`.
*   **Permissions:**
    *   **AlloyDB Client** (`roles/alloydb.client`) to connect to the instance.
    *   Database-level permissions (e.g., `SELECT`, `INSERT`) are required to
        execute queries.
    *   **AlloyDB Viewer** (`roles/alloydb.viewer`) is required for the `list`
        and `get` cluster, instance, and user tools.
    *   **AlloyDB Admin** (`roles/alloydb.admin`) is required for the `create`
        tools.
    *   **Monitoring Viewer** (`roles/monitoring.viewer`) is required on the
        project for `get_system_metrics` and `get_query_metrics`.
*   **Tools:**
    *   `execute_sql`: Executes a SQL query.
    *   `list_tables`: Lists tables in the database.
    *   `list_active_queries`: Lists ongoing queries.
    *   `list_available_extensions`: Discover all PostgreSQL extensions available for installation.
    *   `list_installed_extensions`: List all installed PostgreSQL extensions.
    *   `long_running_transactions`: Identifies and lists database transactions that exceed a specified time limit.
    *   `list_locks`: Identifies all locks held by active processes.
    *   `replication_stats`: Lists each replica's process ID and sync state.
    *   `list_autovacuum_configurations`: Lists autovacuum configurations in the
        database.
    *   `list_memory_configurations`: Lists memory-related configurations in the
        database.
    *   `list_top_bloated_tables`: List top bloated tables in the database.
    *   `list_replication_slots`: Lists replication slots in the database.
    *   `list_invalid_indexes`: Lists invalid indexes in the database.
    *   `get_query_plan`: Generate the execution plan of a statement.
    *   `list_views`: Lists views in the database from pg_views with a default
        limit of 50 rows. Returns schemaname, viewname and the ownername.
    *   `list_schemas`: Lists schemas in the database.
    *   `database_overview`: Fetches the current state of the PostgreSQL server.
    *   `list_triggers`: Lists triggers in the database.
    *   `list_indexes`: List available user indexes in a PostgreSQL database.
    *   `list_sequences`: List sequences in a PostgreSQL database.
    *   `list_query_stats`: Lists query statistics.
    *   `get_column_cardinality`: Gets column cardinality.
    *   `list_table_stats`: Lists table statistics.
    *   `list_publication_tables`: List publication tables in a PostgreSQL database.
    *   `list_tablespaces`: Lists tablespaces in the database.
    *   `list_pg_settings`: List configuration parameters for the PostgreSQL server.
    *   `list_database_stats`: Lists the key performance and activity statistics for
        each database in the AlloyDB instance.
    *   `list_roles`: Lists all the user-created roles in PostgreSQL database.
    *   `list_stored_procedure`: Lists stored procedures.
    *   `get_system_metrics`: Fetches system level cloud monitoring data
        (timeseries metrics) for an AlloyDB instance using a PromQL query.
    *   `list_clusters`: Lists all AlloyDB clusters in a project.
    *   `create_cluster`: Creates a new AlloyDB cluster.
    *   `list_users`: Lists all database users within an AlloyDB cluster.
    *   `create_user`: Creates a new database user in an AlloyDB cluster.
    *   `get_user`: Gets information about a specified database user in an
        AlloyDB cluster.
    *   `wait_for_operation`: Polls the operations API to track the status of
        long-running operations.
    *   `get_query_metrics`: Fetches query level cloud monitoring data
        (timeseries metrics) for queries running in an AlloyDB instance using a
        PromQL query.
    *   `get_instance`: Gets information about a specified AlloyDB instance.
    *   `get_cluster`: Gets information about a specified AlloyDB cluster.
    *   `create_instance`: Creates a new AlloyDB instance within a cluster.
    *   `list_instances`: Lists all instances within an AlloyDB cluster.
