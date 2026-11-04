---
title: "AlloyDB Postgres Advisor"
type: docs
description: "Details of the AlloyDB Postgres Advisor prebuilt configuration."
---

## AlloyDB Postgres Advisor

*   `--prebuilt` value: `alloydb-postgres-advisor`
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
    *   Database-level `SELECT` permission on `google_db_advisor` views.

