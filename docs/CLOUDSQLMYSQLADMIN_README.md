# Cloud SQL for MySQL Admin MCP Server

The Cloud SQL for MySQL Model Context Protocol (MCP) Server gives AI-powered development tools the ability to work with your Google Cloud SQL for MySQL databases. It supports connecting to instances, exploring schemas, and running queries.

## Features

An editor configured to use the Cloud SQL for MySQL MCP server can use its AI capabilities to help you:

- **Provision & Manage Infrastructure** - Create and manage Cloud SQL instances and users

To connect to the database to explore and query data, search the MCP store for the Cloud SQL for MySQL MCP Server.

## Prerequisites

*   [Node.js](https://nodejs.org/) installed.
*   A Google Cloud project with the **Cloud SQL Admin API** enabled.
*   Ensure [Application Default Credentials](https://cloud.google.com/docs/authentication/gcloud) are available in your environment.
*   IAM Permissions:
  * Cloud SQL Viewer (`roles/cloudsql.viewer`)
  * Cloud SQL Admin (`roles/cloudsql.admin`)

## Install & Configuration

In the Antigravity MCP Store, click the "Install" button.

> [!NOTE]
> On first use, the installation process automatically downloads and uses
> [MCP Toolbox](https://www.npmjs.com/package/@toolbox-sdk/server)
> `>=0.26.0`. To update MCP Toolbox, use:
> ```npm i -g @toolbox-sdk/server@latest```
> To always run the latest version, update the MCP server configuration to use:
> ```npx -y @toolbox-sdk/server@latest --prebuilt cloud-sql-mysql-admin```.

You'll now be able to see all enabled tools in the "Tools" tab.

> [!NOTE]
> If you encounter issues with Windows Defender blocking the execution, you may need to configure an allowlist. See [Configure exclusions for Microsoft Defender Antivirus](https://learn.microsoft.com/en-us/microsoft-365/security/defender-endpoint/configure-exclusions-microsoft-defender-antivirus?view=o365-worldwide) for more details.

## Usage

Once configured, the MCP server will automatically provide Cloud SQL for MySQL capabilities to your AI assistant. You can:

   * "Create a new Cloud SQL for MySQL instance named 'e-commerce-prod' in the 'my-gcp-project' project."
   * "Create a new user named 'analyst' with read access to all tables."

## Server Capabilities

The Cloud SQL for MySQL MCP server provides the following tools:

| Tool Name            | Description                                            |
|:---------------------|:-------------------------------------------------------|
| `create_instance`    | Create an instance (PRIMARY, READ-POOL, or SECONDARY). |
| `create_user`        | Create BUILT-IN or IAM-based users for an instance.    |
| `get_instance`       | Get details about an instance.                         |
| `get_user`           | Get details about a user in an instance.               |
| `list_instances`     | List instances in a given project and location.        |
| `list_users`         | List users in a given project and location.            |
| `wait_for_operation` | Poll the operations API until the operation is done.   |

## Custom MCP Server Configuration

The MCP server is configured using environment variables.

```bash
export CLOUD_SQL_MYSQL_PROJECT="<your-gcp-project-id>"
export CLOUD_SQL_MYSQL_READONLY="true"  # Optional: `true`, `false`. Defaults to `false`. When set to `true`, write-capable admin tools (create_instance, create_user, create_database, clone_instance, create_backup, restore_backup) are suppressed.
```

Add the following configuration to your MCP client (e.g., `settings.json` for Gemini CLI, `mcp_config.json` for Antigravity):

```json
{
  "mcpServers": {
    "cloud-sql-mysql-admin": {
      "command": "npx",
      "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "cloud-sql-mysql-admin", "--stdio"]
    }
  }
}
```

## Documentation

For more information, visit the [Cloud SQL for MySQL documentation](https://cloud.google.com/sql/docs/mysql).
