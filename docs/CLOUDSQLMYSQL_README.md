# Cloud SQL for MySQL MCP Server

The Cloud SQL for MySQL Model Context Protocol (MCP) Server gives AI-powered development tools the ability to work with your Google Cloud SQL for MySQL databases. It supports connecting to instances, exploring schemas, and running queries.

## Features

An editor configured to use the Cloud SQL for MySQL MCP server can use its AI capabilities to help you:

- **Query Data** - Execute SQL queries and analyze query plans
- **Explore Schema** - List tables and view schema details
- **Database Maintenance** - Check for fragmentation and missing indexes
- **Monitor Performance** - View active queries, query stats and list all locks

For Cloud SQL infrastructure management, search the MCP store for the Cloud SQL for MySQL Admin MCP Server.

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
    > ```npx -y @toolbox-sdk/server@latest --prebuilt cloud-sql-mysql```.

2. Add the required inputs for your [instance](https://cloud.google.com/sql/docs/mysql/instance-info) in the configuration pop-up, then click "Save". You can update this configuration at any time in the "Configure" tab.

You'll now be able to see all enabled tools in the "Tools" tab.

> [!NOTE]
> If you encounter issues with Windows Defender blocking the execution, you may need to configure an allowlist. See [Configure exclusions for Microsoft Defender Antivirus](https://learn.microsoft.com/en-us/microsoft-365/security/defender-endpoint/configure-exclusions-microsoft-defender-antivirus?view=o365-worldwide) for more details.

## Usage

Once configured, the MCP server will automatically provide Cloud SQL for MySQL capabilities to your AI assistant. You can:

*   "Show me the schema for the 'orders' table."
*   "List the top 10 active queries."
*   "Check for tables missing unique indexes."

## Server Capabilities

The Cloud SQL for MySQL MCP server provides the following tools:

| Tool Name                            | Description                                                             |
|:-------------------------------------|:------------------------------------------------------------------------|
| `execute_sql`                        | Use this tool to execute SQL.                                           |
| `list_active_queries`                | Lists top N ongoing queries from processlist and innodb_trx.            |
| `get_query_plan`                     | Provide information about how MySQL executes a SQL statement (EXPLAIN). |
| `list_tables`                        | Lists detailed schema information for user-created tables.              |
| `list_tables_missing_unique_indexes` | Find tables that do not have primary or unique key constraint.          |
| `list_table_fragmentation`           | List table fragmentation in MySQL.                                      |
| `list_table_stats`                   | List table statistics in MySQL.                                         |
| `list_all_locks`                     | List all active locks in MySQL.                                         |
| `show_query_stats`                   | Show query statistics in MySQL.                                         |

## Custom MCP Server Configuration

The MCP server is configured using environment variables.

```bash
export CLOUD_SQL_MYSQL_PROJECT="<your-gcp-project-id>"
export CLOUD_SQL_MYSQL_REGION="<your-cloud-sql-region>"
export CLOUD_SQL_MYSQL_INSTANCE="<your-cloud-sql-instance-id>"
export CLOUD_SQL_MYSQL_DATABASE="<your-database-name>"
export CLOUD_SQL_MYSQL_USER="<your-database-user>"  # Optional
export CLOUD_SQL_MYSQL_PASSWORD="<your-database-password>"  # Optional
export CLOUD_SQL_MYSQL_IP_TYPE="PUBLIC"  # Optional: `PUBLIC`, `PRIVATE`, `PSC`. Defaults to `PUBLIC`.
export CLOUD_SQL_MYSQL_READONLY="true"  # Optional: Restricts tools and enforces read-only session locking on the database connection. Defaults to `false`.
```

Add the following configuration to your MCP client (e.g., `settings.json` for Gemini CLI, `mcp_config.json` for Antigravity):

```json
{
  "mcpServers": {
    "cloud-sql-mysql": {
      "command": "npx",
      "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "cloud-sql-mysql", "--stdio"],
      "env": {
        "CLOUD_SQL_MYSQL_PROJECT": "your-project-id",
        "CLOUD_SQL_MYSQL_REGION": "your-region",
        "CLOUD_SQL_MYSQL_INSTANCE": "your-instance-id",
        "CLOUD_SQL_MYSQL_DATABASE": "your-database-name",
        "CLOUD_SQL_MYSQL_USER": "your-username",
        "CLOUD_SQL_MYSQL_PASSWORD": "your-password"
      }
    }
  }
}
```

## Documentation

For more information, visit the [Cloud SQL for MySQL documentation](https://cloud.google.com/sql/docs/mysql).
