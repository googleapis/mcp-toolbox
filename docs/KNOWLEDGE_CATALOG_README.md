# Knowledge Catalog MCP Server

The Knowledge Catalog (formerly known as Dataplex) Model Context Protocol (MCP) Server gives AI-powered development tools the ability to work with your Google Cloud Knowledge Catalog. It supports searching and looking up entries, managing Data Products and Data Assets, and searching data quality scans.

## Features

An editor configured to use the Knowledge Catalog MCP server can use its AI capabilities to help you:

- **Search Catalog** - Search for entries in Knowledge Catalog
- **Explore Metadata** - Lookup specific entries and list/retrieve Data Products and Data Assets
- **Data Quality** - Search for data quality scans

## Prerequisites

*   [Node.js](https://nodejs.org/) installed.
*   A Google Cloud project with the **Dataplex API** enabled.
*   Ensure [Application Default Credentials](https://cloud.google.com/docs/authentication/gcloud) are available in your environment.
*   IAM Permissions:
    *   Dataplex Viewer (`roles/dataplex.viewer`) or equivalent permissions to read catalog entries.

## Install & Configuration

1. In the Antigravity MCP Store, click the "Install" button.
    > [!NOTE]
    > On first use, the installation process automatically downloads and uses
    > [MCP Toolbox](https://www.npmjs.com/package/@toolbox-sdk/server)
    > `>=0.26.0`. To update MCP Toolbox, use:
    > ```npm i -g @toolbox-sdk/server@latest```
    > To always run the latest version, update the MCP server configuration to use:
    > ```npx -y @toolbox-sdk/server@latest --prebuilt dataplex```.

2. Add the required inputs in the configuration pop-up, then click "Save". You can update this configuration at any time in the "Configure" tab.

You'll now be able to see all enabled tools in the "Tools" tab.

> [!NOTE]
> If you encounter issues with Windows Defender blocking the execution, you may need to configure an allowlist. See [Configure exclusions for Microsoft Defender Antivirus](https://learn.microsoft.com/en-us/microsoft-365/security/defender-endpoint/configure-exclusions-microsoft-defender-antivirus?view=o365-worldwide) for more details.

## Usage

Once configured, the MCP server will automatically provide Knowledge Catalog capabilities to your AI assistant. You can:

*   "Search for entries related to 'sales' in Knowledge Catalog."
*   "Look up details for the entry 'projects/my-project/locations/us-central1/entryGroups/my-group/entries/my-entry'."
*   "List all Data Products."
*   "Get details of the Data Product 'projects/my-project/locations/us-central1/dataProducts/my-product'."
*   "List Data Assets for the Data Product 'projects/my-project/locations/us-central1/dataProducts/my-product'."
*   "Get details of the Data Asset 'projects/my-project/locations/us-central1/dataProducts/my-product/dataAssets/my-asset'."
*   "Create a new Data Product named 'my-product' with owner 'user@example.com'."
*   "Update the display name of Data Product 'my-product' to 'Updated Product'."
*   "Create a new Data Asset under 'my-product' with resource '//bigquery.googleapis.com/projects/my-project/datasets/my-dataset/tables/my-table'."
*   "Update the labels of Data Asset 'my-asset' under Data Product 'my-product' to have 'env: prod'."
*   "Update aspects of Data Product 'my-product' to set overview content to 'New description'."

## Server Capabilities

The Knowledge Catalog MCP server provides the following tools:

| Tool Name                     | Description                                                                                                                  |
|:------------------------------|:-----------------------------------------------------------------------------------------------------------------------------|
| `search_entries`              | Search for entries in Knowledge Catalog.                                                                                     |
| `lookup_entry`                | Retrieve specific subset of metadata (for example, schema, usage, business overview, and contacts) of a specific data asset. |
| `lookup_context`              | Retrieve rich metadata regarding one or more data assets along with their relationships.                                     |
| `search_dq_scans`             | Search for Data Quality scans.                                                                                               |
| `list_data_products`          | List Data Products for the current project.                                                                                  |
| `get_data_product`            | Retrieve a specific Data Product.                                                                                            |
| `list_data_assets`            | List Data Assets under a Data Product.                                                                                       |
| `get_data_asset`              | Retrieve specific metadata regarding a Data Asset.                                                                           |
| `create_data_product`         | Create a new Data Product.                                                                                                   |
| `update_data_product`         | Update an existing Data Product.                                                                                             |
| `create_data_asset`           | Create a new Data Asset.                                                                                                     |
| `update_data_asset`           | Update an existing Data Asset.                                                                                               |
| `update_data_product_aspects` | Update aspects (like documentation or contracts) on a Data Product Entry.                                                          |
| `get_operation`               | Retrieve the status of a long-running operation.                                                                             |

## Custom MCP Server Configuration

The MCP server is configured using environment variables.

```bash
export DATAPLEX_PROJECT="<your-gcp-project-id>"
```

Add the following configuration to your MCP client (e.g., `settings.json` for Gemini CLI, `mcp_config.json` for Antigravity):

```json
{
  "mcpServers": {
    "dataplex": {
      "command": "npx",
      "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "dataplex", "--stdio"],
      "env": {
        "DATAPLEX_PROJECT": "your-project-id"
      }
    }
  }
}
```

## Documentation

For more information, visit the [Knowledge Catalog documentation](https://cloud.google.com/dataplex/docs).
