---
title: "MCP Apps & UI Resources"
type: docs
weight: 4
description: >
  Interactive HTML UI resources and tool visual interfaces for MCP clients supporting the MCP Apps extension.
---

The **MCP Apps** extension (`io.modelcontextprotocol/ui`) allows MCP servers to serve interactive web applications (HTML/CSS/JS) directly as resources and bind them to tools. When supported by an MCP client, the client can render an interactive user interface alongside or in place of standard tool outputs.

{{< notice note >}}
Support for the MCP Apps extension is advertised via `io.modelcontextprotocol/ui` in server capabilities starting with MCP protocol version `2026-07-28`.
{{< /notice >}}

## Defining a UI Resource

In Toolbox, UI capabilities are configured directly on standard resources (`kind: resource` or `kind: resourceTemplate`) using the optional `ui:` field.

Any `file` or `text` resource can function as an interactive MCP App simply by serving HTML content (typically with `mimeType: text/html;profile=mcp-app`) and specifying the `ui:` block for security policies (CSP) and device permissions.

```yaml
kind: resource
name: customer_dashboard
type: file
path: "./ui/dashboard.html"
mimeType: "text/html;profile=mcp-app"
description: "Interactive customer metrics dashboard."
ui:
  prefersBorder: true
  csp:
    connectDomains:
      - "https://api.example.com"
    resourceDomains:
      - "https://cdn.example.com"
  permissions:
    - clipboardWrite
```

### UI Resource Schema

The `ui` field on `kind: resource` and `kind: resourceTemplate` supports the following options:

| **field**       | **type**                                      | **required** | **description**                                                                                           |
|-----------------|-----------------------------------------------|--------------|-----------------------------------------------------------------------------------------------------------|
| `csp`           | [CSPConfig](#content-security-policy-csp)     | No           | Content Security Policy restricting the domains that the UI app can communicate with or load assets from.|
| `permissions`   | []string                                      | No           | List of browser device permissions requested by the app (e.g., `camera`, `microphone`, `geolocation`, `clipboardWrite`). |
| `prefersBorder` | bool                                          | No           | Suggests whether the host client should render a visible border around the app container.                |

### Content Security Policy (CSP)

To safeguard client environments from untrusted origins, Toolbox allows you to define a strict Content Security Policy for your UI resources. All origins must include a valid protocol scheme (`http`, `https`, `ws`, or `wss`) and domain:

| **field**         | **type** | **description**                                                                         |
|-------------------|----------|-----------------------------------------------------------------------------------------|
| `connectDomains`  | []string | Allowed origins for fetch, XMLHttpRequest, and WebSocket connections (`connect-src`).   |
| `resourceDomains` | []string | Allowed origins for scripts, styles, images, and fonts (`script-src`, `img-src`, etc.). |
| `frameDomains`    | []string | Allowed origins for nested iframes (`frame-src`).                                       |
| `baseUriDomains`  | []string | Allowed origins for document base URIs (`base-uri`).                                    |

### Permissions

The `permissions` field specifies browser capabilities requested by the UI app. The following permissions are supported:

- `camera`: Access to video capture devices.
- `microphone`: Access to audio capture devices.
- `geolocation`: Access to geographic location data.
- `clipboardWrite`: Permission to write text and data to the system clipboard.

## Linking Tools to UI Resources

Tools can link directly to a UI resource so clients know which visual interface to render when executing the tool.

To associate a tool with a UI resource, add the `ui` block to the tool's configuration:

```yaml
kind: tool
name: view_customer_dashboard
type: postgres-sql
source: my-db
statement: "SELECT * FROM customers WHERE id = $1"
description: "Retrieves customer records and displays an interactive dashboard."
parameters:
  - name: customer_id
    type: integer
    description: "The unique ID of the customer."
ui:
  resource: customer_dashboard
  visibility:
    - model
    - app
```

### Tool UI Schema

| **field**      | **type**  | **required** | **description**                                                                                                            |
|----------------|-----------|--------------|----------------------------------------------------------------------------------------------------------------------------|
| `resource`     | string    | Yes          | The `name` of a configured `resource` or `resourceTemplate` providing the UI for this tool.                               |
| `visibility`   | []string  | No           | Controls who can see the tool. Allowed values are `model` and `app`. Defaults to `["model", "app"]` if omitted.          |

#### Visibility Options

- `model`: The tool is advertised to LLMs for automated tool calling.
- `app`: The tool is exposed to the interactive UI application for direct invocation.
- Both (`["model", "app"]`): The default setting, allowing both the model and the UI app to call the tool.

## Group Scoping & Validation

When using [Groups](../groups/), Toolbox enforces strict consistency between tools and their associated UI resources:

1. **Existence Check**: Toolbox verifies during server startup that every `ui.resource` referenced by a tool exists. If the resource is missing, server initialization fails.
2. **Group Boundary Enforcement**: If a tool is included in a group (`kind: group`), its referenced `ui.resource` **must also be included in the same group**. Attempting to configure a group with a tool whose UI resource is not in the group will fail validation:

```yaml
kind: group
name: analytics_group
tools:
  - view_customer_dashboard
resources:
  - customer_dashboard # Required because view_customer_dashboard references it
```
