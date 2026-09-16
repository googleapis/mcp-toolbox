---
title: "FalkorDB"
type: docs
description: "Details of the FalkorDB prebuilt configuration."
---

## FalkorDB

*   `--prebuilt` value: `falkordb`
*   **Environment Variables:**
    *   `FALKORDB_HOST`: The host of the FalkorDB instance (e.g.,
        `127.0.0.1`).
    *   `FALKORDB_PORT`: The port of the FalkorDB instance (defaults to
        `6379`).
    *   `FALKORDB_GRAPH`: The name of the graph to operate on.
    *   `FALKORDB_USERNAME`: The username for the FalkorDB instance
        (optional).
    *   `FALKORDB_PASSWORD`: The password for the FalkorDB instance
        (optional).
*   **Permissions:**
    *   **Database-level permissions** are required to execute Cypher
        queries.
*   **Tools:**
    *   `execute_cypher`: Executes a Cypher query against the graph.
    *   `get_schema`: Retrieves the schema of the graph.
    *   `list_graphs`: Lists the graphs stored on the instance.
