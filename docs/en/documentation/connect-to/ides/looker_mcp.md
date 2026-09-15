---
title: "Looker using MCP"
type: docs
weight: 2
description: >
  Connect your IDE to Looker using Toolbox.
---

[Model Context Protocol (MCP)](https://modelcontextprotocol.io/introduction) is
an open protocol for connecting Large Language Models (LLMs) to data sources
like Postgres. This guide covers how to use [MCP Toolbox for Databases][toolbox]
to expose your developer assistant tools to a Looker instance:

* [Gemini-CLI][gemini-cli]
* [Cursor][cursor]
* [Windsurf][windsurf] (Codium)
* [Visual Studio Code][vscode] (Copilot)
* [Cline][cline] (VS Code extension)
* [Claude desktop][claudedesktop]
* [Claude code][claudecode]
* [Antigravity][antigravity]

[toolbox]: https://github.com/googleapis/mcp-toolbox
[gemini-cli]: #configure-your-mcp-client
[cursor]: #configure-your-mcp-client
[windsurf]: #configure-your-mcp-client
[vscode]: #configure-your-mcp-client
[cline]: #configure-your-mcp-client
[claudedesktop]: #configure-your-mcp-client
[claudecode]: #configure-your-mcp-client
[antigravity]: #connect-with-antigravity

## Set up Looker

1. Get a Looker Client ID and Client Secret. Follow the directions
   [here](https://cloud.google.com/looker/docs/api-auth#authentication_with_an_sdk).

1. Have the base URL of your Looker instance available. It is likely
   something like `https://looker.example.com`. In some cases the API is
   listening at a different port, and you will need to use
   `https://looker.example.com:19999` instead.

## Connect with Antigravity

You can connect Looker to Antigravity in the following ways:

* Using the MCP Store
* Using a custom configuration

{{< notice note >}}
You don't need to download the MCP Toolbox binary to use these methods.
{{< /notice >}}

{{< tabpane text=true >}}
{{% tab header="MCP Store" lang="en" %}}
The most straightforward way to connect to Looker in Antigravity is by using the built-in MCP Store.

1. Open Antigravity and open the editor's agent panel.
1. Click the **"..."** icon at the top of the panel and select **MCP Servers**.
1. Locate **Looker** in the list of available servers and click Install.
1. Follow the on-screen prompts to securely link your accounts where applicable.

After you install Looker in the MCP Store, resources and tools from the server are automatically available to the editor. 

{{% /tab %}}
{{% tab header="Custom config" lang="en" %}}
 To connect to a custom MCP server, follow these steps:

1. Open Antigravity and navigate to the MCP store using the **"..."** drop-down at the top of the editor's agent panel.
1. To open the **mcp_config.json** file, click **MCP Servers** and then click **Manage MCP Servers > View raw config**.
1. Add the following configuration, replace the environment variables with your values, and save.
    
    ```json
    {
      "mcpServers": {
        "looker": {
          "command": "npx",
          "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "looker", "--stdio"],
          "env": {
              "LOOKER_BASE_URL": "https://looker.example.com",
              "LOOKER_CLIENT_ID": "your-client-id",
              "LOOKER_CLIENT_SECRET": "your-client-secret"
          }
        }
      }
    }
    ```
   If you want to access tools to develop LookML as well as query data
   and create content, change the following line
   ```json
          "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "looker", "--stdio"],
   ```
   to
   ```json
          "args": ["-y", "@toolbox-sdk/server", "--prebuilt", "looker,looker-dev", "--stdio"],
   ```

{{% /tab %}}
{{< /tabpane >}}

## Install MCP Toolbox

1. Download the latest version of Toolbox as a binary. Select the [correct
   binary](https://github.com/googleapis/mcp-toolbox/releases) corresponding
   to your OS and CPU architecture. You are required to use Toolbox version
   v0.10.0+:

   <!-- {x-release-please-start-version} -->

{{< tabpane persist=header >}}
{{< tab header="linux/amd64" lang="bash" >}}
curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.11.0/linux/amd64/toolbox
{{< /tab >}}

{{< tab header="darwin/arm64" lang="bash" >}}
curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.11.0/darwin/arm64/toolbox
{{< /tab >}}

{{< tab header="darwin/amd64" lang="bash" >}}
curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.11.0/darwin/amd64/toolbox
{{< /tab >}}

{{< tab header="windows/amd64" lang="bash" >}}
curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.11.0/windows/amd64/toolbox.exe
{{< /tab >}}

{{< tab header="windows/arm64" lang="bash" >}}
curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.11.0/windows/arm64/toolbox.exe
{{< /tab >}}
{{< /tabpane >}}
    <!-- {x-release-please-end} -->

1. Make the binary executable:

    ```bash
    chmod +x toolbox
    ```

1. Verify the installation:

    ```bash
    ./toolbox --version
    ```

## Configure your MCP Client

{{< tabpane text=true >}}
{{% tab header="Gemini-CLI" lang="en" %}}

1. Install
   [Gemini-CLI](https://github.com/google-gemini/gemini-cli#install-globally-with-npm).
1. Create a directory `.gemini` in your home directory if it doesn't exist.
1. Create the file `.gemini/settings.json` if it doesn't exist.
1. Add the following configuration, or add the mcpServers stanza if you already
   have a `settings.json` with content. Replace the path to the toolbox
   executable and the environment variables with your values, and save:

    ```json
    {
      "mcpServers": {
        "looker-toolbox": {
          "command": "./PATH/TO/toolbox",
          "args": ["--stdio", "--prebuilt", "looker"],
          "env": {
            "LOOKER_BASE_URL": "https://looker.example.com",
            "LOOKER_CLIENT_ID": "",
            "LOOKER_CLIENT_SECRET": "",
            "LOOKER_VERIFY_SSL": "true"
          }
        }
      }
    }
    ```
    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

1. Start Gemini-CLI with the `gemini` command and use the command `/mcp` to see
   the configured MCP tools.
{{% /tab %}}

{{% tab header="Claude code" lang="en" %}}

1. Install [Claude
   Code](https://docs.anthropic.com/en/docs/agents-and-tools/claude-code/overview).
1. Create a `.mcp.json` file in your project root if it doesn't exist.
1. Add the following configuration, replace the environment variables with your
   values, and save:

    ```json
    {
      "mcpServers": {
        "looker-toolbox": {
          "command": "./PATH/TO/toolbox",
          "args": ["--stdio", "--prebuilt", "looker"],
          "env": {
            "LOOKER_BASE_URL": "https://looker.example.com",
            "LOOKER_CLIENT_ID": "",
            "LOOKER_CLIENT_SECRET": "",
            "LOOKER_VERIFY_SSL": "true"
          }
        }
      }
    }
    ```
    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

1. Restart Claude Code to apply the new configuration.
{{% /tab %}}

{{% tab header="Claude desktop" lang="en" %}}

1. Open [Claude desktop](https://claude.ai/download) and navigate to Settings.
1. Under the Developer tab, tap Edit Config to open the configuration file.
1. Add the following configuration, replace the environment variables with your
   values, and save:

    ```json
    {
      "mcpServers": {
        "looker-toolbox": {
          "command": "./PATH/TO/toolbox",
          "args": ["--stdio", "--prebuilt", "looker"],
          "env": {
            "LOOKER_BASE_URL": "https://looker.example.com",
            "LOOKER_CLIENT_ID": "",
            "LOOKER_CLIENT_SECRET": "",
            "LOOKER_VERIFY_SSL": "true"
          }
        }
      }
    }
    ```
    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

1. Restart Claude desktop.
1. From the new chat screen, you should see a hammer (MCP) icon appear with the
   new MCP server available.
{{% /tab %}}

{{% tab header="Cline" lang="en" %}}

1. Open the [Cline](https://github.com/cline/cline) extension in VS Code and tap
   the **MCP Servers** icon.
1. Tap Configure MCP Servers to open the configuration file.
1. Add the following configuration, replace the environment variables with your
   values, and save:

    ```json
    {
      "mcpServers": {
        "looker-toolbox": {
          "command": "./PATH/TO/toolbox",
          "args": ["--stdio", "--prebuilt", "looker"],
          "env": {
            "LOOKER_BASE_URL": "https://looker.example.com",
            "LOOKER_CLIENT_ID": "",
            "LOOKER_CLIENT_SECRET": "",
            "LOOKER_VERIFY_SSL": "true"
          }
        }
      }
    }
    ```
    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

1. You should see a green active status after the server is successfully
   connected.
{{% /tab %}}

{{% tab header="Cursor" lang="en" %}}

1. Create a `.cursor` directory in your project root if it doesn't exist.
1. Create a `.cursor/mcp.json` file if it doesn't exist and open it.
1. Add the following configuration, replace the environment variables with your
   values, and save:

    ```json
    {
      "mcpServers": {
        "looker-toolbox": {
          "command": "./PATH/TO/toolbox",
          "args": ["--stdio", "--prebuilt", "looker"],
          "env": {
            "LOOKER_BASE_URL": "https://looker.example.com",
            "LOOKER_CLIENT_ID": "",
            "LOOKER_CLIENT_SECRET": "",
            "LOOKER_VERIFY_SSL": "true"
          }
        }
      }
    }
    ```
    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

1. Open [Cursor](https://www.cursor.com/) and navigate to **Settings > Cursor
   Settings > MCP**. You should see a green active status after the server is
   successfully connected.
{{% /tab %}}

{{% tab header="Visual Studio Code (Copilot)" lang="en" %}}

1. Open [VS Code](https://code.visualstudio.com/docs/copilot/overview) and
   create a `.vscode` directory in your project root if it doesn't exist.
1. Create a `.vscode/mcp.json` file if it doesn't exist and open it.
1. Add the following configuration, replace the environment variables with your
   values, and save:

    ```json
    {
      "servers": {
        "looker-toolbox": {
          "command": "./PATH/TO/toolbox",
          "args": ["--stdio", "--prebuilt", "looker"],
          "env": {
            "LOOKER_BASE_URL": "https://looker.example.com",
            "LOOKER_CLIENT_ID": "",
            "LOOKER_CLIENT_SECRET": "",
            "LOOKER_VERIFY_SSL": "true"
          }
        }
      }
    }
    ```
    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```

{{% /tab %}}

{{% tab header="Windsurf" lang="en" %}}

1. Open [Windsurf](https://docs.codeium.com/windsurf) and navigate to the
   Cascade assistant.
1. Tap on the hammer (MCP) icon, then Configure to open the configuration file.
1. Add the following configuration, replace the environment variables with your
   values, and save:

    ```json
    {
      "mcpServers": {
        "looker-toolbox": {
          "command": "./PATH/TO/toolbox",
          "args": ["--stdio", "--prebuilt", "looker"],
          "env": {
            "LOOKER_BASE_URL": "https://looker.example.com",
            "LOOKER_CLIENT_ID": "",
            "LOOKER_CLIENT_SECRET": "",
            "LOOKER_VERIFY_SSL": "true"
          }
        }
      }
    }
    ```
    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```
    
    If you want to access tools to develop LookML as well as query data
    and create content, change the following line
    ```json
          "args": ["--stdio", "--prebuilt", "looker"],
    ```
    to
    ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
    ```
    

{{% /tab %}}
{{< /tabpane >}}

## Use Tools

Your AI tool is now connected to Looker using MCP. Try asking your AI
assistant to list models, explores, dimensions, and measures. Run a
query, retrieve the SQL for a query, and run a saved Look.

The full tool list is available in the [Prebuilt Tools Reference](../../configuration/prebuilt-configs/_index.md#looker).

The following tools are available to the LLM:

### Looker Model and Query Tools

These tools are used to get information about a Looker model
and execute queries against that model.

1. **get_models**: list the LookML models in Looker
1. **get_explores**: list the explores in a given model
1. **get_dimensions**: list the dimensions in a given explore
1. **get_measures**: list the measures in a given explore
1. **get_filters**: list the filters in a given explore
1. **get_parameters**: list the parameters in a given explore
1. **query**: Run a query and return the data
1. **query_sql**: Return the SQL generated by Looker for a query
1. **query_url**: Return a link to the query in Looker for further exploration

### Looker Content Tools

These tools get saved content (Looks and Dashboards) from a Looker
instance and create new saved content.

1. **get_looks**: Return the saved Looks that match a title or description
1. **run_look**: Run a saved Look and return the data
1. **make_look**: Create a saved Look in Looker and return the URL
1. **get_dashboards**: Return the saved dashboards that match a title or
   description
1. **run_dashboard**: Run the queries associated with a dashboard and return the
   data
1. **make_dashboard**: Create a saved dashboard in Looker and return the URL
1. **add_dashboard_element**: Add a tile to a dashboard
1. **add_dashboard_filter**: Add a filter to a dashboard
1. **generate_embed_url**: Generate an embed url for content

### Looker Instance Health Tools

These tools offer the same health check algorithms that the popular
CLI [Henry](https://github.com/looker-open-source/henry) offers.

1. **health_pulse**: Check the health of a Looker intance
1. **health_analyze**: Analyze the usage of a Looker object
1. **health_vacuum**: Find LookML elements that might be unused

### LookML Authoring Tools

These tools allow enable the caller to write and modify LookML files
as well as get the database schema needed to write LookML effectively.

1. **dev_mode**: Activate dev mode.
1. **get_projects**: Get the list of LookML projects
1. **get_project_files**: Get the list of files in a project
1. **get_project_file**: Get the contents of a file in a project
1. **create_project_file**: Create a file in a project
1. **update_project_file**: Update the contents of a file in a project
1. **delete_project_file**: Delete a file in a project
1. **get_project_directories**: Retrieves a list of project directories for a given LookML project.
1. **create_project_directory**: Creates a new directory within a specified LookML project.
1. **delete_project_directory**: Deletes a directory from a specified LookML project.
1. **validate_project**: Check the syntax of a LookML project.
1. **get_connections**: Get the list of connections
1. **get_connection_schemas**: Get the list of schemas for a connection
1. **get_connection_databases**: Get the list of databases for a connection
1. **get_connection_tables**: Get the list of tables for a connection
1. **get_connection_table_columns**: Get the list of columns for a table in a connection
1. **get_lookml_tests**: Retrieves a list of available LookML tests for a project.
1. **run_lookml_tests**: Executes specific LookML tests within a project.
1. **create_view_from_table**: Generates boilerplate LookML views directly from the database schema.
1. **list_git_branches**: List the available git branches of a LookML project.
1. **get_git_branch**: Get the current git branch of a LookML project.
1. **create_git_branch**: Create a new git branch for a LookML project.
1. **switch_git_branch**: Switch the git branch of a LookML project.
1. **delete_git_branch**: Delete a git branch of a LookML project.

{{< notice note >}}
Prebuilt tools are pre-1.0, so expect some tool changes between versions. LLMs
will adapt to the tools available, so this shouldn't affect most users.
{{< /notice >}}
