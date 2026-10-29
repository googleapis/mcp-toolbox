---
title: "Cloud SQL for PostgreSQL"
type: docs
description: "Details of the Cloud SQL for PostgreSQL prebuilt configuration."
---

## Cloud SQL for PostgreSQL

*   `--prebuilt` value: `cloud-sql-postgres`
*   **Environment Variables:**
    *   `CLOUD_SQL_POSTGRES_PROJECT`: The GCP project ID.
    *   `CLOUD_SQL_POSTGRES_REGION`: The region of your Cloud SQL instance.
    *   `CLOUD_SQL_POSTGRES_INSTANCE`: The ID of your Cloud SQL instance.
    *   `CLOUD_SQL_POSTGRES_DATABASE`: The name of the database to connect to.
    *   `CLOUD_SQL_POSTGRES_USER`: (Optional) The database username. Defaults to
        IAM authentication if unspecified.
    *   `CLOUD_SQL_POSTGRES_PASSWORD`: (Optional) The password for the database
        user. Defaults to IAM authentication if unspecified.
    *   `CLOUD_SQL_POSTGRES_IP_TYPE`: (Optional) The IP type i.e. "Public" or
        "Private" (Default: Public).
    *   `CLOUD_SQL_POSTGRES_READONLY`: (Optional) When set to `true`, enforces read-only execution at the database session level (`cloudsql_session_read_only=locked`) and suppresses write-capable tools. Default: `false`.
*   **Permissions:**
    *   **Cloud SQL Client** (`roles/cloudsql.client`) to connect to the
        instance.
    *   Database-level permissions (e.g., `SELECT`, `INSERT`) are required to
        execute queries.
    *   **Cloud SQL Viewer** (`roles/cloudsql.viewer`), **Cloud SQL Editor**
        (`roles/cloudsql.editor`), or **Cloud SQL Admin**
        (`roles/cloudsql.admin`) is required for the instance management
        tools, depending on the tool. See the Cloud SQL for PostgreSQL Admin
        prebuilt config for the per-tool breakdown.
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
        each database in the postgreSQL instance.
    *   `list_roles`: Lists all the user-created roles in PostgreSQL database.
    *   `list_stored_procedure`: Lists stored procedures.
    *   `list_databases`: Lists all databases for a Cloud SQL instance.
    *   `create_backup`: Creates a backup on a Cloud SQL instance.
    *   `postgres_upgrade_precheck`: Performs a precheck for a major version
        upgrade of a Cloud SQL for PostgreSQL instance.
    *   `create_instance`: Creates a new Cloud SQL for PostgreSQL instance.
    *   `wait_for_operation`: Waits for a Cloud SQL operation to complete.
    *   `get_system_metrics`: Fetches system level cloud monitoring data
        (timeseries metrics) for a Postgres instance using a PromQL query.
    *   `restore_backup`: Restores a backup of a Cloud SQL instance.
    *   `list_instances`: Lists Cloud SQL instances in a project.
    *   `create_database`: Creates a new database in a Cloud SQL instance.
    *   `create_user`: Creates a new user in a Cloud SQL instance.
    *   `get_query_metrics`: Fetches query level cloud monitoring data
        (timeseries metrics) for queries running in a Postgres instance using a
        PromQL query.
    *   `clone_instance`: Creates a clone for an existing Cloud SQL for
        PostgreSQL instance.
    *   `get_instance`: Gets information about a Cloud SQL instance.
    *   `define_spec`: Defines a new vector specification and generates SQL
        recommendations for setting up embeddings and vector indexes.
    *   `modify_spec`: Modifies an existing vector specification and
        recalculates its SQL recommendations.
    *   `apply_spec`: Executes the SQL recommendations for a vector
        specification to finalize the vector search setup.
    *   `generate_query`: Generates an optimized SQL query for vector search
        based on a vector specification.
    *   `delete_spec`: Deletes an existing vector specification.
    *   `list_specs`: Lists the vector specifications defined for a table and
        column.
    *   `get_spec`: Gets the details of an existing vector specification.
    *   `improve_query_recall`: Generates SQL recommendations to improve vector
        search recall for an existing index.
