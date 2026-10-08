---
title: "MCP Apps"
type: docs
weight: 10
description: >
  Interactive HTML UI resources and tool visual interfaces for MCP clients supporting the MCP Apps extension.
---

The [**MCP Apps**](https://github.com/modelcontextprotocol/ext-apps) extension (`io.modelcontextprotocol/ui`) allows MCP servers to serve interactive web applications (HTML/CSS/JS) directly as resources and bind them to tools. When supported by an MCP client, the client can render an interactive user interface alongside or in place of standard tool outputs. For the official protocol specification, schemas, and client SDKs, refer to the [modelcontextprotocol/ext-apps](https://github.com/modelcontextprotocol/ext-apps) repository.

{{< notice note >}}
Support for the MCP Apps extension is advertised via `io.modelcontextprotocol/ui` in server capabilities exclusively for MCP protocol version `2026-07-28`. Older protocol versions (`2024-11-05`, `2025-03-26`, `2025-06-18`, and `2025-11-25`) do not support extensions and will not advertise UI capabilities or tool UI metadata.
{{< /notice >}}

## Defining a UI Resource

In Toolbox, UI capabilities are configured directly on standard [resources](../resources/) (`kind: resource` or `kind: resourceTemplate`) by setting `ui: true`.

Any type of resource can function as an interactive MCP App simply by setting `ui: true`. When `ui: true` is set:
- `mimeType` must be `text/html;profile=mcp-app` (defaults automatically if omitted; explicit non-conforming types are rejected).
- `uri` defaults to `ui://{name}` if omitted.
- Security policies (CSP), device permissions, application domain, and container display settings can be configured.

```yaml
kind: resource
name: customer_dashboard
type: file
path: "./ui/dashboard.html"
description: "Interactive customer metrics dashboard."
ui: true
domain: "https://example.com"
prefersBorder: true
csp:
  connectDomains:
    - "https://api.example.com"
  resourceDomains:
    - "https://cdn.example.com"
permissions:
  - clipboardWrite
```

### UI Resource Configuration

When `ui: true` is enabled on `kind: resource` or `kind: resourceTemplate`, the following fields can be configured:

| **field**       | **type**                                      | **required** | **description**                                                                                           |
|-----------------|-----------------------------------------------|--------------|-----------------------------------------------------------------------------------------------------------|
| `ui`            | bool                                          | Yes          | Set to `true` to designate this resource as an interactive MCP UI application.                            |
| `mimeType`      | string                                        | No           | MIME type of the UI resource. Must be `text/html;profile=mcp-app` (defaults automatically if omitted).  |
| `domain`        | string                                        | No           | Application domain. Must be a valid absolute URI with an `http` or `https` scheme (e.g., `https://example.com`). |
| `csp`           | [CSPConfig](#content-security-policy-csp)     | No           | Content Security Policy restricting the domains that the UI app can communicate with or load assets from.|
| `permissions`   | []string                                      | No           | List of browser device permissions requested by the app (e.g., `camera`, `microphone`, `geolocation`, `clipboardWrite`). |
| `prefersBorder` | bool                                          | No           | Suggests whether the host client should render a visible border around the app container.                |

### Content Security Policy (CSP)

To safeguard client environments from untrusted origins, Toolbox allows you to define a strict Content Security Policy for your UI resources. All origins must include a valid protocol scheme (`http` or `https`) and host:

| **field**         | **type** | **description**                                                                         |
|-------------------|----------|-----------------------------------------------------------------------------------------|
| `connectDomains`  | []string | Allowed origins for fetch and XMLHttpRequest connections (`connect-src`).               |
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

[Tools](../tools/) can link directly to a UI resource so clients know which visual interface to render when executing the tool.

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
| `resource`     | string    | Yes          | The `name` of a configured `resource` or `resourceTemplate` providing the UI for this tool (must have `ui: true`).        |
| `visibility`   | []string  | No           | Controls who can see the tool. Allowed values are `model` and `app`. Defaults to `["model", "app"]` if omitted.          |

#### Visibility Options

- `model`: The tool is advertised to LLMs for automated tool calling.
- `app`: The tool is exposed to the interactive UI application for direct invocation.
- Both (`["model", "app"]`): The default setting, allowing both the model and the UI app to call the tool.

## Global Availability & Group Scoping

Unlike standard tools, prompts, or resources that are scoped to specific [Groups](../groups/), **UI resources are strictly global and cannot be scoped to groups**:

1. **Server-Wide Tool References**: Tools in any group can link to UI resources without needing to declare the UI resource in that group.
2. **UI Resource Requirement**: Toolbox validates that every `ui.resource` referenced by a tool exists and is explicitly configured as a UI resource (`ui: true`). Referencing a standard (non-UI) resource or template will fail startup validation with an error:
   ```text
   resource "<name>" referenced by tool "<tool>" is not a UI resource (ui: true is required)
   ```
3. **Forbidden in Groups**: UI resources and UI resource templates (`ui: true`) **cannot** be included in `groups[].resources` or `groups[].resourceTemplates`. Attempting to add a UI resource to a group will fail startup validation with an error:
   ```text
   UI resource "<name>" cannot be included in group "<group>": UI resources are globally accessible and cannot be scoped to groups
   ```
4. **Omitted from Resource Listing**: UI resources (`ui: true`) are intentionally excluded from `resources/list` and `resources/templates/list` across all endpoints (including the default `/mcp` group endpoint) so they do not clutter standard LLM context. Clients obtain the UI resource URI directly from the tool's manifest metadata and retrieve it via `resources/read`.

## Capability Negotiation & Graceful Degradation

MCP Apps employs client capability negotiation to ensure backwards compatibility with standard text-only MCP clients:

1. **Client Advertising**: During initialization, clients that support interactive apps advertise the extension in their initialization parameters:
   ```json
   {
     "capabilities": {
       "extensions": {
         "io.modelcontextprotocol/ui": {
           "mimeTypes": ["text/html;profile=mcp-app"]
         }
       }
     }
   }
   ```
2. **Graceful Degradation**:
   - When a client **supports** the UI extension, Toolbox advertises UI metadata on tools in `tools/list` under `_meta.ui`:
     ```json
     {
       "name": "view_customer_dashboard",
       "description": "Retrieves customer records and displays an interactive dashboard.",
       "_meta": {
         "ui": {
           "resourceUri": "ui://customer_dashboard",
           "visibility": ["model", "app"]
         }
       }
     }
     ```
   - When a client **does not support** the UI extension (or omits `io.modelcontextprotocol/ui`), Toolbox automatically strips `_meta.ui` from tool definitions, serving the tool as a standard text-based tool.
   - All tools continue to return standard structured text output in their `content` array regardless of UI mode.

## Reading UI Resources

When a client or host application requests a UI resource via `resources/read`, Toolbox delivers the HTML document along with the declared security policies in the content item's `_meta.ui`:

```json
{
  "contents": [
    {
      "uri": "ui://customer_dashboard",
      "mimeType": "text/html;profile=mcp-app",
      "text": "<!DOCTYPE html><html>...</html>",
      "_meta": {
        "ui": {
          "csp": {
            "connectDomains": ["https://api.example.com"],
            "resourceDomains": ["https://cdn.example.com"]
          },
          "permissions": {
            "clipboardWrite": {}
          },
          "domain": "https://example.com",
          "prefersBorder": true
        }
      }
    }
  ]
}
```

The host application uses this `_meta.ui` payload to configure iframe sandbox policies, enforce Content Security Policy headers, and apply frame boundary styling.
