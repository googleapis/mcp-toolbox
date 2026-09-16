---
title: "Cloud SQL for PostgreSQL Admin"
type: docs
description: "Details of the Cloud SQL for PostgreSQL Admin prebuilt configuration."
---

## Cloud SQL for PostgreSQL Admin

*   `--prebuilt` value: `cloud-sql-postgres-admin`
*   **Environment Variables:**
    *   `CLOUD_SQL_POSTGRES_PROJECT`: (Optional) The GCP project ID to use as the default for Cloud SQL infrastructure tools.
    *   `CLOUD_SQL_POSTGRES_READONLY`: (Optional) When set to `true`, suppresses write-capable admin tools. Default: `false`.
*   **Permissions:**
    *   **Cloud SQL Viewer** (`roles/cloudsql.viewer`): Provides read-only
        access to resources.
        * `get_instance`
        * `list_instances`
        * `list_databases`
        * `wait_for_operation`
    *   **Cloud SQL Editor** (`roles/cloudsql.editor`): Provides permissions to
        manage existing resources.
        * All `viewer` tools
        * `create_database`
        * `create_backup`
    *   **Cloud SQL Admin** (`roles/cloudsql.admin`): Provides full control over
        all resources.
        * All `editor` and `viewer` tools
        * `create_instance`
        * `create_user`
        * `clone_instance`
        * `restore_backup`
*   **Tools:**
    *   `create_instance`: Creates a new Cloud SQL for PostgreSQL instance.
    *   `get_instance`: Gets information about a Cloud SQL instance.
    *   `list_instances`: Lists Cloud SQL instances in a project.
    *   `create_database`: Creates a new database in a Cloud SQL instance.
    *   `list_databases`: Lists all databases for a Cloud SQL instance.
    *   `create_user`: Creates a new user in a Cloud SQL instance.
    *   `wait_for_operation`: Waits for a Cloud SQL operation to complete.
    *   `clone_instance`: Creates a clone for an existing Cloud SQL for PostgreSQL instance.
    *   `postgres_upgrade_precheck`: Performs a precheck for a major version upgrade of a Cloud SQL for PostgreSQL instance.
    *   `create_backup`: Creates a backup on a Cloud SQL instance.
    *   `restore_backup`: Restores a backup of a Cloud SQL instance.
