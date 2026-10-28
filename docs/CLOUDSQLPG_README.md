# Cloud SQL for PostgreSQL MCP Server

The Cloud SQL for PostgreSQL Model Context Protocol (MCP) Server gives AI-powered development tools the ability to work with your Google Cloud SQL for PostgreSQL databases. It supports connecting to instances, exploring schemas, running queries, and analyzing performance.

## Features

An editor configured to use the Cloud SQL for PostgreSQL MCP server can use its AI capabilities to help you:

- **Query Data** - Execute SQL queries and analyze query plans
- **Explore Schema** - List tables, views, indexes, and triggers
- **Monitor Performance** - View active queries, bloat, and memory configurations
- **Manage Extensions** - List available and installed extensions

For Cloud SQL infrastructure management, search the MCP store for the Cloud SQL for PostgreSQL Admin MCP Server.

## Prerequisites

*   [Node.js](https://nodejs.org/) installed.
*   A Google Cloud project with the **Cloud SQL Admin API** enabled.
*   Ensure [Application Default Credentials](https://cloud.google.com/docs/authentication/gcloud) are available in your environment.
*   IAM Permissions:
    *   Cloud SQL Client (`roles/cloudsql.client`)

> **Note:** If your instance uses private IPs, you must run the MCP server in the same Virtual Private Cloud (VPC) network.

## Install & Configuration

1. In the Antigravity MCP Store, click the "Install" button.
    > [!NOTE]
    > On first use, the installation process automatically downloads and uses
    > [MCP Toolbox](https://www.npmjs.com/package/@toolbox-sdk/server)
    > `>=0.26.0`. To update MCP Toolbox, use:
    > ```npm i -g @toolbox-sdk/server@latest```
    > To always run the latest version, update the MCP server configuration to use:
    > ```npx -y @toolbox-sdk/server@latest --prebuilt cloud-sql-postgres```.

2. Add the required inputs for your [instance](https://cloud.google.com/sql/docs/postgres/instance-info) in the configuration pop-up, then click "Save". You can update this configuration at any time in the "Configure" tab.

You'll now be able to see all enabled tools in the "Tools" tab.

> [!NOTE]
> If you encounter issues with Windows Defender blocking the execution, you may need to configure an allowlist. See [Configure exclusions for Microsoft Defender Antivirus](https://learn.microsoft.com/en-us/microsoft-365/security/defender-endpoint/configure-exclusions-microsoft-defender-antivirus?view=o365-worldwide) for more details.

## Usage

Once configured, the MCP server will automatically provide Cloud SQL for PostgreSQL capabilities to your AI assistant. You can:

*   "Show me the top 5 bloated tables."
*   "List all installed extensions."
*   "Explain the query plan for SELECT * FROM users."

## Server Capabilities

The Cloud SQL for PostgreSQL MCP server provides the following tools:

| Tool Name                        | Description                                                    |
|:---------------------------------|:---------------------------------------------------------------|
| `execute_sql`                    | Use this tool to execute sql.                                  |
| `list_tables`                    | Lists detailed schema information for user-created tables.     |
| `list_active_queries`            | List the top N currently running queries.                      |
| `list_available_extensions`      | Discover all PostgreSQL extensions available for installation. |
| `list_installed_extensions`      | List all installed PostgreSQL extensions.                      |
| `list_autovacuum_configurations` | List PostgreSQL autovacuum-related configurations.             |
| `list_memory_configurations`     | List PostgreSQL memory-related configurations.                 |
| `list_top_bloated_tables`        | List the top tables by dead-tuple (approximate bloat signal).  |
| `list_replication_slots`         | List key details for all PostgreSQL replication slots.         |
| `list_invalid_indexes`           | Lists all invalid PostgreSQL indexes.                          |
| `get_query_plan`                 | Generate a PostgreSQL EXPLAIN plan in JSON format.             |
| `list_views`                     | Lists views in the database.                                   |
| `list_schemas`                   | Lists all schemas in the database.                             |
| `database_overview`              | Fetches the current state of the PostgreSQL server.            |
| `list_triggers`                  | Lists all non-internal triggers in a database.                 |
| `list_indexes`                   | Lists available user indexes in the database.                  |
| `list_sequences`                 | Lists sequences in the database.                               |
| `define_spec`                    | Defines a new vector specification for search workloads.       |
| `modify_spec`                    | Modifies an existing vector specification.                     |
| `apply_spec`                     | Executes SQL recommendations for a vector specification.       | 
| `generate_query`                 | Generates optimized SQL queries for vector searches.           |
| `list_specs`                     | Lists all vector specifications for a given table and column.  |
| `get_spec`                       | Retrieves a vector specification using its unique ID.          |
| `delete_spec`                    | Deletes a vector specification using its unique ID.            |
| `improve_query_recall`           | Improves query recall for vector search workloads.             |


## Custom MCP Server Configuration

The MCP server is configured using environment variables.

```bash
export CLOUD_SQL_POSTGRES_PROJECT="<your-gcp-project-id>"
export CLOUD_SQL_POSTGRES_REGION="<your-cloud-sql-region>"
export CLOUD_SQL_POSTGRES_INSTANCE="<your-cloud-sql-instance-id>"
export CLOUD_SQL_POSTGRES_DATABASE="<your-database-name>"
export CLOUD_SQL_POSTGRES_USER="<your-database-user>"  # Optional
export CLOUD_SQL_POSTGRES_PASSWORD="<your-database-password>"  # Optional
export CLOUD_SQL_POSTGRES_IP_TYPE="PUBLIC"  # Optional: `PUBLIC`, `PRIVATE`, `PSC`. Defaults to `PUBLIC`.
export CLOUD_SQL_POSTGRES_READONLY="true"  # Optional: Restricts tools and enforces read-only session locking on the database connection. Defaults to `false`.
```

Add the following configuration to your MCP client (e.g., `settings.json` for Gemini CLI, `mcp_config.json` for Antigravity):

```json
{
  "mcpServers": {
    "cloud-sql-postgres": {
      "command": "npx",
      "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "cloud-sql-postgres", "--stdio"],
      "env": {
        "CLOUD_SQL_POSTGRES_PROJECT": "your-project-id",
        "CLOUD_SQL_POSTGRES_REGION": "your-region",
        "CLOUD_SQL_POSTGRES_INSTANCE": "your-instance-id",
        "CLOUD_SQL_POSTGRES_DATABASE": "your-database-name",
        "CLOUD_SQL_POSTGRES_USER": "your-username",
        "CLOUD_SQL_POSTGRES_PASSWORD": "your-password"
      }
    }
  }
}
```

## Documentation

For more information, visit the [Cloud SQL for PostgreSQL documentation](https://cloud.google.com/sql/docs/postgres).
