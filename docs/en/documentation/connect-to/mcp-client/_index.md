---
title: "MCP Client"
type: docs
weight: 1
description: >
  How to connect to Toolbox from a MCP Client.
---

## Toolbox SDKs vs Model Context Protocol (MCP)

Toolbox supports connections via the [Model
Context Protocol (MCP)](https://modelcontextprotocol.io/). However, Toolbox has
several features which are not supported in the MCP specification (such as
Authenticated Parameters and Authorized invocation).

We recommend using the native Toolbox Client SDKs over MCP clients to leverage these features.
The Toolbox SDKs can be combined with MCP clients in many cases.

### Protocol Versions

Toolbox currently supports the following versions of MCP specification:

* [2026-07-28](https://modelcontextprotocol.io/specification/2026-07-28)
* [2025-11-25](https://modelcontextprotocol.io/specification/2025-11-25)
* [2025-06-18](https://modelcontextprotocol.io/specification/2025-06-18)
* [2025-03-26](https://modelcontextprotocol.io/specification/2025-03-26)
* [2024-11-05](https://modelcontextprotocol.io/specification/2024-11-05)

### Secure Parameters and MCP Clients

[Secure Parameters](../../configuration/tools/_index.md#secure-parameters) are supported on MCP protocol version `2026-07-28` or newer via the `com.google.cloud/toolbox.v1` extension.

* **Extension Negotiation:** MCP clients declare support for this extension in `params._meta["io.modelcontextprotocol/clientCapabilities"].extensions["com.google.cloud/toolbox.v1"]`.
* **Manifests (`tools/list`):** When negotiated, tools define application-controlled parameters under `secureInputSchema` alongside regular model parameters in `inputSchema`.
* **Invocations (`tools/call`):** Secure parameters are passed out-of-band in `params.secureArguments`, while model-generated arguments are passed in `params.arguments`.
* **Fallback for Generic MCP Clients:** Tools that require secure parameters are **automatically filtered out** from `tools/list` responses for clients that have not negotiated the `com.google.cloud/toolbox.v1` extension (or clients using protocol versions prior to `2026-07-28`). Attempting to call a secure tool directly without negotiated capability returns:
  * On protocol `2026-07-28`: `MissingRequiredClientCapabilityError` (JSON-RPC code `-32021`).
  * On protocol versions prior to `2026-07-28`: `ToolNotFoundError` / `INVALID_PARAMS` (JSON-RPC code `-32602`, "tool does not exist"), as secure tools are completely hidden from older protocols.
* **Injection Defense:** If a client or model attempts to pass a secure parameter inside standard `arguments`, the server rejects the parameter and returns a tool execution error (`isError: true`).

### Toolbox AuthZ/AuthN Not Supported by MCP

The auth implementation in Toolbox is not supported in MCP's auth specification.
This includes:

* [Authenticated Parameters](../../configuration/tools/_index.md#authenticated-parameters)
* [Authorized Invocations](../../configuration/tools/_index.md#authorized-invocations)

## Connecting to Toolbox with an MCP client

### Before you begin

{{< notice note >}}
MCP is only compatible with Toolbox version 0.3.0 and above.
{{< /notice >}}

1. [Install](../../introduction/_index.md#installing-the-server)
   Toolbox version 0.3.0+.

1. Make sure you've set up and initialized your database.

1. [Set up](../../configuration/_index.md) your `tools.yaml` file.

### Connecting via Standard Input/Output (stdio)

Toolbox supports the
[stdio](https://modelcontextprotocol.io/docs/concepts/transports#standard-input%2Foutput-stdio)
transport protocol. Users that wish to use stdio will have to include the
`--stdio` flag when running Toolbox.

```bash
./toolbox --stdio
```

When running with stdio, Toolbox will listen via stdio instead of acting as a
remote HTTP server. Logs will be set to the `warn` level by default. `debug` and
`info` logs are not supported with stdio.

{{< notice note >}}
Toolbox enables dynamic reloading by default. To disable, use the
`--disable-reload` flag.
{{< /notice >}}

### Connecting via HTTP

Toolbox supports the HTTP transport protocol with and without SSE.

{{< tabpane text=true >}} {{% tab header="HTTP with SSE (deprecated)" lang="en" %}}
Add the following configuration to your MCP client configuration:

```bash
{
  "mcpServers": {
    "toolbox": {
      "type": "sse",
      "url": "http://127.0.0.1:5000/mcp/sse",
    }
  }
}
```

If you would like to connect to a specific toolset, replace `url` with
`"http://127.0.0.1:5000/mcp/{toolset_name}/sse"`.

HTTP with SSE is only supported in version `2024-11-05` and is currently
deprecated.
{{% /tab %}} {{% tab header="Streamable HTTP" lang="en" %}}
Add the following configuration to your MCP client configuration:

```bash
{
  "mcpServers": {
    "toolbox": {
      "type": "http",
      "url": "http://127.0.0.1:5000/mcp",
    }
  }
}
```

If you would like to connect to a specific toolset, replace `url` with
`"http://127.0.0.1:5000/mcp/{toolset_name}"`.
{{% /tab %}} {{< /tabpane >}}

### Using the MCP Inspector with Toolbox

Use MCP [Inspector](https://github.com/modelcontextprotocol/inspector) for
testing and debugging Toolbox server.

{{< tabpane text=true >}}
{{% tab header="STDIO" lang="en" %}}

1. Run Inspector with Toolbox as a subprocess:

   ```bash
   npx @modelcontextprotocol/inspector ./toolbox --stdio
   ```

1. For `Transport Type` dropdown menu, select `STDIO`.

1. In `Command`, make sure that it is set to :`./toolbox` (or the correct path
   to where the Toolbox binary is installed).

1. In `Arguments`, make sure that it's filled with `--stdio`.

1. Click the `Connect` button. It might take awhile to spin up Toolbox. Voila!
   You should be able to inspect your toolbox tools!
{{% /tab %}}
{{% tab header="HTTP with SSE (deprecated)" lang="en" %}}
1. [Run Toolbox](../../introduction/_index.md#running-the-server).

1. In a separate terminal, run Inspector directly through `npx`:

    ```bash
    npx @modelcontextprotocol/inspector
    ```

1. For `Transport Type` dropdown menu, select `SSE`.

1. For `URL`, type in `http://127.0.0.1:5000/mcp/sse` to use all tool or
   `http//127.0.0.1:5000/mcp/{toolset_name}/sse` to use a specific toolset.

1. Click the `Connect` button. Voila! You should be able to inspect your toolbox
   tools!
{{% /tab %}}
{{% tab header="Streamable HTTP" lang="en" %}}
1. [Run Toolbox](../../introduction/_index.md#running-the-server).

1. In a separate terminal, run Inspector directly through `npx`:

    ```bash
    npx @modelcontextprotocol/inspector
    ```

1. For `Transport Type` dropdown menu, select `Streamable HTTP`.

1. For `URL`, type in `http://127.0.0.1:5000/mcp` to use all tool or
   `http//127.0.0.1:5000/mcp/{toolset_name}` to use a specific toolset.

1. Click the `Connect` button. Voila! You should be able to inspect your toolbox
   tools!
{{% /tab %}} {{< /tabpane >}}

### Tested Clients

| Client             | SSE Works  | MCP Config Docs                                                                 |
|--------------------|------------|---------------------------------------------------------------------------------|
| Claude Desktop     | ✅         | <https://modelcontextprotocol.io/quickstart/user#1-download-claude-for-desktop> |
| MCP Inspector      | ✅         | <https://github.com/modelcontextprotocol/inspector>                             |
| Cursor             | ✅         | <https://docs.cursor.com/context/model-context-protocol>                        |
| Windsurf           | ✅         | <https://docs.windsurf.com/windsurf/cascade/mcp#model-context-protocol-mcp>     |
| VS Code (Insiders) | ✅         | <https://code.visualstudio.com/docs/copilot/chat/mcp-servers>                   |
