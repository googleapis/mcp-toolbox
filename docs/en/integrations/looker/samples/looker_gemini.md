---
title: "Quickstart (MCP with Looker and Gemini-CLI)"
type: docs
weight: 2
description: >
  How to get started running Toolbox with Gemini-CLI and Looker as the source.
sample_filters: ["Gemini CLI", "Looker"]
is_sample: true
---

## Overview

[Model Context Protocol](https://modelcontextprotocol.io) is an open protocol
that standardizes how applications provide context to LLMs. Check out this page
on how to [connect to Toolbox via MCP](../../../documentation/connect-to/mcp-client/_index.md).

## Step 1: Get a Looker Client ID and Client Secret

The Looker Client ID and Client Secret can be obtained from the Users page of
your Looker instance. Refer to the documentation
[here](https://cloud.google.com/looker/docs/api-auth#authentication_with_an_sdk).
You may need to ask an administrator to get the Client ID and Client Secret
for you.

## Step 2: Install and configure Toolbox

In this section, we will download Toolbox and run the Toolbox server.

1. Download the latest version of Toolbox as a binary:

    {{< notice tip >}}
   Select the
   [correct binary](https://github.com/googleapis/mcp-toolbox/releases)
   corresponding to your OS and CPU architecture.
    {{< /notice >}}
    <!-- {x-release-please-start-version} -->
    ```bash
    export OS="linux/amd64" # one of linux/amd64, darwin/arm64, darwin/amd64, windows/amd64, or windows/arm64
    curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.11.0/$OS/toolbox
    ```
    <!-- {x-release-please-end} -->

1. Make the binary executable:

    ```bash
    chmod +x toolbox
    ```

1. Edit the file `~/.gemini/settings.json` and add the following
   to the list of mcpServers. Use the Client Id and Client Secret
   you obtained earlier. The name of the server - here
   `looker-toolbox` - can be anything meaningful to you.

   ```json
      "mcpServers": {
        "looker-toolbox": {
          "command": "/path/to/toolbox",
          "args": ["--stdio", "--prebuilt", "looker"],
          "env": {
            "LOOKER_BASE_URL": "https://looker.example.com",
            "LOOKER_CLIENT_ID": "",
            "LOOKER_CLIENT_SECRET": "",
            "LOOKER_VERIFY_SSL": "true"
          }
        }
      }
   ```

   In some instances you may need to append `:19999` to
   the LOOKER_BASE_URL.

   If you want to access tools to develop LookML as well as query data
   and create content, change the following line
   ```json
          "args": ["--stdio", "--prebuilt", "looker"],
   ```
   to
   ```json
          "args": ["--stdio", "--prebuilt", "looker,looker-dev"],
   ```

## Step 3: Start Gemini-CLI

1. Run Gemini-CLI:

    ```bash
    npx https://github.com/google-gemini/gemini-cli
    ```

1. Type `y` when it asks to download.

1. Log into Gemini-CLI

1. Enter the command `/mcp` and you should see a list of
   available tools like

   ```
    ℹ Configured MCP servers:

      🟢 looker-toolbox - Ready (10 tools)
        - looker-toolbox__get_models
        - looker-toolbox__query
        - looker-toolbox__get_looks
        - looker-toolbox__get_measures
        - looker-toolbox__get_filters
        - looker-toolbox__get_parameters
        - looker-toolbox__get_explores
        - looker-toolbox__query_sql
        - looker-toolbox__get_dimensions
        - looker-toolbox__run_look
        - looker-toolbox__query_url
    ```

1. Start exploring your Looker instance with commands like
   `Find an explore to see orders` or `show me my current
   inventory broken down by item category`.

1. Gemini will prompt you for your approval before using
   a tool. You can approve all the tools at once or
   one at a time.
