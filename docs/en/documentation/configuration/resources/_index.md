---
title: "Resources"
type: docs
weight: 9
description: >
   Resources allow servers to provide read-only data, files, and contextual content to language models and MCP clients.
---

A `resource` represents read-only data or content that can be discovered and retrieved by MCP clients to provide contextual information to Large Language Models (LLMs).

{{< notice note >}}
You can use [Groups](../groups/) to organize resources and resource templates into collections. When you connect to a group's endpoint /mcp/{name}, resources/list and resources/templates/list return only the items in that group, and resources/read strictly enforces that requested URIs belong to that group. The default endpoint /mcp provides access to all resources.
{{< /notice >}}

Resources are analogous to file attachments or contextual snippets: they allow the model to inspect data (such as documentation, schema definitions or log files) without needing to invoke executable tools. The Toolbox server implements the following methods from the [Model Context Protocol (MCP)](https://modelcontextprotocol.io/docs/concepts/resources) specification:

- `resources/list`: Discovers all concrete/direct resources defined on the server.
- `resources/templates/list`: Discovers dynamic resource templates with parameterized URI templates.
- `resources/read`: Retrieves the content and MIME type of a specific resource by its URI.

Toolbox supports two kinds of resources:
1. **Static Resources (`kind: resource`)**: Fixed entities with a static URI (such as in-memory text snippets or specific disk files).
2. **Resource Templates (`kind: resourceTemplate`)**: Dynamic resources that match URI patterns (such as exposing any log file within a directory tree using `{path}`).

```yaml
kind: resource
name: database_schema_ddl
type: text
description: "Core table definitions and constraints."
uri: "schema://database/ddl"
mimeType: "text/x-sql"
text: |
  CREATE TABLE customers (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    email VARCHAR(255) UNIQUE NOT NULL
  );
---
kind: resource
name: database_schema
type: file
description: "Application database schema definition."
path: "./schema.sql"
uri: "file:///app/schema.sql"
mimeType: "text/plain"
---
kind: resourceTemplate
name: app_logs
type: file
description: "Application runtime log files."
uriTemplate: "file:///logs/{path}"
allowedPaths:
  - "./logs"
```

## Resource Schema (`kind: resource`)

| **field**     | **type**                               | **required** | **description**                                                                                              |
|---------------|----------------------------------------|--------------|--------------------------------------------------------------------------------------------------------------|
| `name`        | string                                 | Yes          | Unique identifier for the resource.                                                                          |
| `type`        | string                                 | Yes          | The type of resource. Supported types: `"text"` and `"file"`.                                                |
| `uri`         | string                                 | No           | Unique URI for the resource. Defaults to `text://{name}` for text resources, or `file:///{normalized_path}` for file resources. |
| `description` | string                                 | No           | A brief explanation of what the resource contains.                                                           |
| `title`       | string                                 | No           | Human-readable title for the resource.                                                                       |
| `mimeType`    | string                                 | No           | The MIME type of the content. Defaults to `text/plain` for text; auto-detected from extension or content for files; defaults to `text/html;profile=mcp-app` when `ui: true`. |
| `ui`          | bool                                   | No           | Set to `true` to designate this resource as an interactive UI app. See [MCP Apps](../mcp-apps/).             |
| `annotations` | [Annotations](#annotations-schema)     | No           | Metadata annotations describing priority, audience, and modification time.                                   |

## Resource Template Schema (`kind: resourceTemplate`)

| **field**        | **type**                               | **required** | **description**                                                                                              |
|------------------|----------------------------------------|--------------|--------------------------------------------------------------------------------------------------------------|
| `name`           | string                                 | Yes          | Unique identifier for the resource template.                                                                 |
| `type`           | string                                 | Yes          | The type of resource template. Supported type: `"file"`.                                                     |
| `uriTemplate`    | string                                 | Yes          | An RFC 6570 URI template. Must contain the `{path}` template variable (e.g., `file:///logs/{path}`).        |
| `allowedPaths`   | []string                               | No           | Allowed base directories for filesystem sandboxing. Traversal attempts outside these paths are rejected.     |
| `maxSize`       | int64 / string                         | No           | Maximum allowed file size in bytes (e.g., `5242880` or `5MB`). Defaults to 5MB.                             |
| `description`    | string                                 | No           | A brief explanation of what the resource template exposes.                                                   |
| `title`          | string                                 | No           | Human-readable title for the resource template.                                                              |
| `mimeType`       | string                                 | No           | The default MIME type for content returned by this template. Defaults to `text/html;profile=mcp-app` when `ui: true`. |
| `ui`             | bool                                   | No           | Set to `true` to designate this resource template as an interactive UI app. See [MCP Apps](../mcp-apps/).    |
| `annotations`    | [Annotations](#annotations-schema)     | No           | Metadata annotations describing priority, audience, and modification time.                                   |

## Annotations Schema

Annotations provide hints to the client about how the resource content should be prioritized and treated.

| **field**      | **type**   | **required** | **description**                                                                                                              |
|----------------|------------|--------------|------------------------------------------------------------------------------------------------------------------------------|
| `priority`     | float      | No           | A number between `0.0` (lowest) and `1.0` (highest) indicating the relative importance of the resource. Defaults to `1.0`.    |
| `audience`     | []string   | No           | Roles that should receive this resource. Allowed values: `"user"`, `"assistant"`.                                           |
| `lastModified` | string     | No           | An RFC 3339 formatted timestamp indicating when the resource was last modified (computed dynamically for file resources).    |

## Types of Resources

Toolbox supports the following resource primitives:

- [**Text Resources**](./text.md): Static text content embedded directly in your configuration file.
- [**File Resources**](./file.md): Specific files stored on disk and served as read-only resources.
- [**Resource Templates**](./template.md): Parameterized URI templates that dynamically read matching files from sandboxed directories.
- [**MCP Apps**](../mcp-apps/): Interactive HTML web applications and tool visual interfaces for clients supporting the MCP Apps extension.
