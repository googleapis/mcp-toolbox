# AlloyDB for PostgreSQL MCP Server

The AlloyDB Model Context Protocol (MCP) Server gives AI-powered development tools the ability to work with your Google Cloud AlloyDB for PostgreSQL resources. It supports full lifecycle control, from exploring schemas and running queries to monitoring your database.

## Features

An editor configured to use the AlloyDB MCP server can use its AI capabilities to help you:

- **Explore Schemas and Data** - List tables, get table details, and view data
- **Execute SQL** - Run SQL queries directly from your editor
- **Monitor Performance** - View active queries, query plans, and other performance metrics (via observability tools)
- **Manage Extensions** - List available and installed PostgreSQL extensions

For AlloyDB infrastructure management, search the MCP store for the AlloyDB for PostgreSQL Admin MCP Server.

## Prerequisites

*   [Node.js](https://nodejs.org/) installed.
*   A Google Cloud project with the **AlloyDB API** enabled.
*   Ensure [Application Default Credentials](https://cloud.google.com/docs/authentication/gcloud) are available in your environment.
*   IAM Permissions:
    *   AlloyDB Client (`roles/alloydb.client`) (for connecting and querying)
    *   Service Usage Consumer (`roles/serviceusage.serviceUsageConsumer`)

> **Note:** If your AlloyDB instance uses private IPs, you must run the MCP server in the same Virtual Private Cloud (VPC) network.

## Install & Configuration

1. In the Antigravity MCP Store, click the "Install" button.
    > [!NOTE]
    > On first use, the installation process automatically downloads and uses
    > [MCP Toolbox](https://www.npmjs.com/package/@toolbox-sdk/server)
    > `>=0.26.0`. To update MCP Toolbox, use:
    > ```npm i -g @toolbox-sdk/server@latest```
    > To always run the latest version, update the MCP server configuration to use:
    > ```npx -y @toolbox-sdk/server@latest --prebuilt alloydb-postgres```.

2. Add the required inputs for your [cluster](https://docs.cloud.google.com/alloydb/docs/cluster-list) in the configuration pop-up, then click "Save". You can update this configuration at any time in the "Configure" tab.

> [!NOTE]
> If you encounter issues with Windows Defender blocking the execution, you may need to configure an allowlist. See [Configure exclusions for Microsoft Defender Antivirus](https://learn.microsoft.com/en-us/microsoft-365/security/defender-endpoint/configure-exclusions-microsoft-defender-antivirus?view=o365-worldwide) for more details.

You'll now be able to see all enabled tools in the "Tools" tab.

## Usage

Once configured, the MCP server will automatically provide AlloyDB capabilities to your AI assistant. You can:

*   "Show me all tables in the 'orders' database."
*   "What are the columns in the 'products' table?"
*   "How many orders were placed in the last 30 days?"

## Server Capabilities

The AlloyDB MCP server provides the following tools:

| Tool Name                        | Description                                                |
|:---------------------------------|:-----------------------------------------------------------|
| `list_tables`                    | Lists detailed schema information for user-created tables. |
| `execute_sql`                    | Executes a SQL query.                                      |
| `list_active_queries`            | List currently running queries.                            |
| `list_available_extensions`      | List available extensions for installation.                |
| `list_installed_extensions`      | List installed extensions.                                 |
| `get_query_plan`                 | Get query plan for a SQL statement.                        |
| `list_autovacuum_configurations` | List autovacuum configurations and their values.           |
| `list_memory_configurations`     | List memory configurations and their values.               |
| `list_top_bloated_tables`        | List top bloated tables.                                   |
| `list_replication_slots`         | List replication slots.                                    |
| `list_invalid_indexes`           | List invalid indexes.                                      |

## Custom MCP Server Configuration

The AlloyDB MCP server is configured using environment variables.

```bash
export ALLOYDB_POSTGRES_PROJECT="<your-gcp-project-id>"
export ALLOYDB_POSTGRES_REGION="<your-alloydb-region>"
export ALLOYDB_POSTGRES_CLUSTER="<your-alloydb-cluster-id>"
export ALLOYDB_POSTGRES_INSTANCE="<your-alloydb-instance-id>"
export ALLOYDB_POSTGRES_DATABASE="<your-database-name>"
export ALLOYDB_POSTGRES_USER="<your-database-user>"  # Optional
export ALLOYDB_POSTGRES_PASSWORD="<your-database-password>"  # Optional
export ALLOYDB_POSTGRES_IP_TYPE="PUBLIC"  # Optional: `PUBLIC`, `PRIVATE`, `PSC`. Defaults to `PUBLIC`.
export ALLOYDB_POSTGRES_READONLY="true"  # Optional: Restricts tools and enforces read-only session locking on the database connection. Defaults to `false`.
```

Add the following configuration to your MCP client (e.g., `settings.json` for Gemini CLI, `mcp_config.json` for Antigravity):

```json
{
  "mcpServers": {
    "alloydb-postgres": {
      "command": "npx",
      "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "alloydb-postgres", "--stdio"]
    }
  }
}
```

## Documentation

For more information, visit the [AlloyDB for PostgreSQL documentation](https://cloud.google.com/alloydb/docs).
