# Looker MCP Server

The Looker Model Context Protocol (MCP) Server gives AI-powered development tools the ability to work with your Looker instance. It supports exploring models, running queries, managing dashboards, and more.

## Features

An editor configured to use the Looker MCP server can use its AI capabilities to help you:

- **Explore Models** - Get models, explores, dimensions, measures, filters, and parameters
- **Run Queries** - Execute Looker queries, generate SQL, and create query URLs
- **Manage Dashboards** - Create, run, and modify dashboards
- **Manage Looks** - Search for and run saved looks
- **Health Checks** - Analyze instance health and performance

## Prerequisites

*   [Node.js](https://nodejs.org/) installed.
*   Access to a Looker instance.
*   API Credentials (`Client ID` and `Client Secret`) or OAuth configuration.

## Install & Configuration

1. In the Antigravity MCP Store, click the "Install" button.
    > [!NOTE]
    > On first use, the installation process automatically downloads and uses
    > [MCP Toolbox](https://www.npmjs.com/package/@toolbox-sdk/server)
    > `>=0.26.0`. To update MCP Toolbox, use:
    > ```npm i -g @toolbox-sdk/server@latest```
    > To always run the latest version, update the MCP server configuration to use:
    > ```npx -y @toolbox-sdk/server@latest --prebuilt looker,looker-dev```.

2. Add the required inputs for your [instance](https://docs.cloud.google.com/looker/docs/set-up-and-administer-looker) in the configuration pop-up, then click "Save". You can update this configuration at any time in the "Configure" tab.

You'll now be able to see all enabled tools in the "Tools" tab.

> [!NOTE]
> If you encounter issues with Windows Defender blocking the execution, you may need to configure an allowlist. See [Configure exclusions for Microsoft Defender Antivirus](https://learn.microsoft.com/en-us/microsoft-365/security/defender-endpoint/configure-exclusions-microsoft-defender-antivirus?view=o365-worldwide) for more details.

## Usage

Once configured, the MCP server will automatically provide Looker capabilities to your AI assistant. You can:

*   "Find explores in the 'ecommerce' model."
*   "Run a query to show total sales by month."
*   "Create a new dashboard named 'Sales Overview'."

## Server Capabilities

The Looker MCP server provides a wide range of tools. Here are some of the key capabilities:

| Tool Name               | Description                                               |
|:------------------------|:----------------------------------------------------------|
| `get_models`            | Retrieves the list of LookML models.                      |
| `get_explores`          | Retrieves the list of explores defined in a LookML model. |
| `get_explore`           | Retrieves detailed metadata and filters for an explore.   |
| `query`                 | Run a query against the LookML model.                     |
| `query_sql`             | Generate the SQL that Looker would run.                   |
| `run_look`              | Runs a saved look.                                        |
| `run_dashboard`         | Runs all tiles in a dashboard.                            |
| `make_dashboard`        | Creates a new dashboard.                                  |
| `add_dashboard_element` | Adds a tile to a dashboard.                               |
| `health_pulse`          | Checks the status of the Looker instance.                 |
| `dev_mode`              | Toggles development mode.                                 |
| `get_projects`          | Lists LookML projects.                                    |

## Custom MCP Server Configuration

The MCP server is configured using environment variables.

```bash
export LOOKER_BASE_URL="<your-looker-instance-url>"  # e.g. `https://looker.example.com`. You may need to add the port, i.e. `:19999`.
export LOOKER_CLIENT_ID="<your-looker-client-id>"
export LOOKER_CLIENT_SECRET="<your-looker-client-secret>"
export LOOKER_VERIFY_SSL="true" # Optional, defaults to true
export LOOKER_SHOW_HIDDEN_MODELS="true" # Optional, defaults to true
export LOOKER_SHOW_HIDDEN_EXPLORES="true" # Optional, defaults to true
export LOOKER_SHOW_HIDDEN_FIELDS="true" # Optional, defaults to true
```

Add the following configuration to your MCP client (e.g., `settings.json` for Gemini CLI, `mcp_config.json` for Antigravity):

```json
{
  "mcpServers": {
    "looker": {
      "command": "npx",
      "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "looker,looker-dev", "--stdio"],
      "env": {
        "LOOKER_BASE_URL": "https://your.looker.instance.com",
        "LOOKER_CLIENT_ID": "your-client-id",
        "LOOKER_CLIENT_SECRET": "your-client-secret"
      }
    }
  }
}
```

## Documentation

For more information, visit the [Looker documentation](https://cloud.google.com/looker).
