---
title: "Resource Templates"
type: docs
weight: 3
description: >
  Dynamic file resource templates using RFC 6570 URI patterns.
---

Resource templates allow you to expose an entire directory tree of files dynamically without defining each file individually. Clients can discover available templates using `resources/templates/list` and read specific files using `resources/read` by supplying a constructed URI.

## Examples

### Project Documentation Template

You can configure a template dedicated to documentation files with a custom `mimeType`. Note that `allowedPaths` can also use relative paths (such as `./docs`), which are automatically resolved relative to the directory containing your configuration file:

```yaml
kind: resourceTemplate
name: project_docs
type: file
title: "Project Documentation"
description: "Markdown documentation for the project."
uriTemplate: "file:///docs/{path}"
allowedPaths:
  - "/docs"
mimeType: "text/markdown"
```

When queried via `resources/templates/list`, the response includes the defined `mimeType`:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "resourceTemplates": [
      {
        "name": "project_docs",
        "title": "Project Documentation",
        "uriTemplate": "file:///docs/{path}",
        "description": "Markdown documentation for the project.",
        "mimeType": "text/markdown",
        "annotations": {
          "priority": 1
        }
      }
    ]
  }
}
```

The client can then retrieve markdown documentation using `resources/read`:

```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "resources/read",
  "params": {
    "uri": "file:///docs/guide.md"
  }
}
```

## Reference

### Resource Template Schema

| **field**        | **type**                           | **required** | **description**                                                                                               |
|------------------|------------------------------------|--------------|---------------------------------------------------------------------------------------------------------------|
| `name`           | string                             | Yes          | Unique name for the resource template.                                                                        |
| `type`           | string                             | Yes          | Must be `"file"`.                                                                                             |
| `uriTemplate`    | string                             | Yes          | RFC 6570 URI template. Must contain the `{path}` variable (e.g., `file:///logs/{path}`).                     |
| `allowedPaths`   | []string                           | No           | Allowed directory trees on disk. Any path resolving outside these boundaries is blocked.                      |
| `maxSize`       | int64 / string                     | No           | Maximum allowed file size in bytes (defaults to 5MB / `5242880` bytes).                                       |
| `description`    | string                             | No           | A brief explanation of what the resource template exposes.                                                    |
| `title`          | string                             | No           | Human-readable title for client display.                                                                      |
| `mimeType`       | string                             | No           | Default MIME type for content returned by this template.                                                      |
| `annotations`    | [Annotations](../_index.md#annotations-schema) | No   | Metadata annotations (`priority`, `audience`, `lastModified`).                                                |

## Guardrails & Security Model

- **Strict `{path}` Variable Requirement**: The `uriTemplate` must follow the [RFC 6570](https://datatracker.ietf.org/doc/html/rfc6570) specification and **must contain only the `{path}` variable**. Any other template variables (e.g., `{id}`, `{filename}`) will fail validation at startup.
{{< notice caution >}}
**Global System Access by Default**: If `allowedPaths` is omitted, the template will have **no sandbox**. It will be able to read **any file on the entire system** that possesses an allowed extension (e.g. `.txt`, `.json`). It is highly recommended to always specify `allowedPaths` to sandbox the template to a specific directory tree.
{{< /notice >}}

- **Path Traversal Prevention**:
  - If `allowedPaths` is defined, the target path must resolve strictly within one of the specified allowed directories.
  - Path traversal attempts using relative segments (such as `../`) or symlink escapes will be blocked with a security violation error.
- **Hidden File Protection**:
  - If `allowedPaths` is omitted, access to hidden files and directories (files or directories beginning with `.`) is automatically blocked to prevent accidental exposure of `.env` files, `.ssh` keys, or `.git` directories.
  - If `allowedPaths` **is** specified, hidden files *are* permitted, provided they reside within the allowed directory boundaries.
- **Allowed Extensions**:
  - The requested URI and the resolved disk path must both have an allowed text file extension:
    - `.txt`, `.md`, `.csv`, `.json`, `.yaml`, `.yml`, `.xml`, `.sql`
  - Requests for files with unsupported extensions are rejected.
- **Regular Files Only**:
  - `resources/read` can only read regular files. If a client provides a URI that resolves to a directory, block device, socket, or pipe, an error is returned.
- **Size Limits & Truncation (`maxSize`)**:
  - If a matched file exceeds `maxSize` (defaults to 5MB / `5242880` bytes, configurable up to 1GB), Toolbox does not error. Instead, it reads up to `maxSize` bytes, safely cleans incomplete multi-byte UTF-8 sequences at the cut-off boundary, and appends a clear truncation notice:
    ```text
    ...[TRUNCATED BY SERVER: Payload exceeded <limit> byte safety limit]...
    ```
    This prevents memory exhaustion while still delivering the initial content of large files to the LLM.

