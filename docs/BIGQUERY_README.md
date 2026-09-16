# BigQuery MCP Server

The BigQuery Model Context Protocol (MCP) Server enables AI-powered development tools to seamlessly connect, interact, and generate data insights with your BigQuery datasets and data using natural language commands.

## Features

An editor configured to use the BigQuery MCP server can use its AI capabilities to help you:

- **Natural Language to Data Analytics:** Easily find required BigQuery tables and ask analytical questions in plain English.
- **Seamless Workflow:** Stay within your CLI, eliminating the need to constantly switch to the GCP console for generating analytical insights.
- **Run Advanced Analytics:** Generate forecasts and perform contribution analysis using built-in advanced tools.

## Prerequisites

*   [Node.js](https://nodejs.org/) installed.
*   A Google Cloud project with the **BigQuery API** enabled.
*   Ensure [Application Default Credentials](https://cloud.google.com/docs/authentication/gcloud) are available in your environment.
*   IAM Permissions:
    *   BigQuery User (`roles/bigquery.user`)

## Install & Configuration

1. In the Antigravity MCP Store, click the "Install" button.
    > [!NOTE]
    > On first use, the installation process automatically downloads and uses
    > [MCP Toolbox](https://www.npmjs.com/package/@toolbox-sdk/server)
    > `>=0.26.0`. To update MCP Toolbox, use:
    > ```npm i -g @toolbox-sdk/server@latest```
    > To always run the latest version, update the MCP server configuration to use:
    > ```npx -y @toolbox-sdk/server@latest --prebuilt bigquery```.

2. Add the required inputs in the configuration pop-up, then click "Save". You can update this configuration at any time in the "Configure" tab.

> [!NOTE]
> If you encounter issues with Windows Defender blocking the execution, you may need to configure an allowlist. See [Configure exclusions for Microsoft Defender Antivirus](https://learn.microsoft.com/en-us/microsoft-365/security/defender-endpoint/configure-exclusions-microsoft-defender-antivirus?view=o365-worldwide) for more details.

You'll now be able to see all enabled tools in the "Tools" tab.


### Usage 

Once configured, the MCP server will automatically provide BigQuery capabilities to your AI assistant. You can:


*   **Find Data:**

    *   "Find tables related to PyPi downloads"
    *   "Find tables related to Google analytics data in the dataset bigquery-public-data"

*   **Generate Analytics and Insights:**

    *   "Using bigquery-public-data.pypi.file_downloads show me the top 10 downloaded pypi packages this month."
    *   "Using bigquery-public-data.pypi.file_downloads can you forecast downloads for the last four months of 2025 for package urllib3?"

## Server Capabilities

The BigQuery MCP server provides the following tools:

| Tool Name              | Description                                                     |
|:-----------------------|:----------------------------------------------------------------|
| `execute_sql`          | Executes a SQL query.                                           |
| `forecast`             | Forecast time series data.                                      |
| `get_dataset_info`     | Get dataset metadata.                                           |
| `get_table_info`       | Get table metadata.                                             |
| `list_dataset_ids`     | Lists dataset IDs in the database.                              |
| `list_table_ids`       | Lists table IDs in the database.                                |
| `analyze_contribution` | Perform contribution analysis, also called key driver analysis. |
| `search_catalog`       | Search for tables based on the provided query.                  |

## Custom MCP Server Configuration

The BigQuery MCP server is configured using environment variables.

```bash
export BIGQUERY_PROJECT="<your-gcp-project-id>"
export BIGQUERY_LOCATION="<your-dataset-location>"  # Optional
export BIGQUERY_READONLY="true"  # Optional: Enforces read-only mode across all tools
export BIGQUERY_WRITE_MODE="protected"  # Optional: allowed, blocked, or protected
export BIGQUERY_USE_CLIENT_OAUTH="true"  # Optional: true, false, or a custom header name
export BIGQUERY_SCOPES="<comma-separated-scopes>"  # Optional
export BIGQUERY_IMPERSONATE_SERVICE_ACCOUNT="<service-account-email>"  # Optional: Service account to impersonate
```

Add the following configuration to your MCP client (e.g., `settings.json` for Gemini CLI, `mcp_config.json` for Antigravity):

```json
{
  "mcpServers": {
    "bigquery": {
      "command": "npx",
      "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "bigquery", "--stdio"]
    }
  }
}
```

## Documentation

For more information, visit the [BigQuery documentation](https://cloud.google.com/bigquery/docs).
