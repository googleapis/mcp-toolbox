---
title: "Resource Templates"
type: docs
weight: 3
description: >
  Dynamic file resource templates using RFC 6570 URI patterns.
---

Resource templates allow you to expose an entire directory tree of files dynamically without defining each file individually. Clients can discover available templates using `resources/templates/list` and read specific files using `resources/read` by supplying a constructed URI.

## Examples

### Sandboxed Log Directory Template

Here is an example exposing an application's log directory:

```yaml
kind: resourceTemplate
name: app_logs
type: file
title: "Application Logs"
description: "Dynamically read application log files."
uriTemplate: "file:///var/log/my-app/{path}"
allowedPaths:
  - "/var/log/my-app"
```

When a client queries `resources/templates/list`, it receives `uriTemplate: "file:///var/log/my-app/{path}"`. The client can then call `resources/read` with:

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "resources/read",
  "params": {
    "uri": "file:///var/log/my-app/service-errors.log"
  }
}
```

### Documentation Directory with Relative Paths

You can use relative paths for `allowedPaths`. Relative paths are resolved relative to the directory containing your configuration file:

```yaml
kind: resourceTemplate
name: project_docs
type: file
title: "Project Documentation"
description: "Markdown documentation for the project."
uriTemplate: "file:///docs/{path}"
allowedPaths:
  - "./documentation"
mimeType: "text/markdown"
```

## Reference

### Resource Template Schema

| **field**        | **type**                           | **required** | **description**                                                                                               |
|------------------|------------------------------------|--------------|---------------------------------------------------------------------------------------------------------------|
| `name`           | string                             | Yes          | Unique name for the resource template.                                                                        |
| `type`           | string                             | Yes          | Must be `"file"`.                                                                                             |
| `uriTemplate`    | string                             | Yes          | RFC 6570 URI template. Must contain the `{path}` variable (e.g., `file:///logs/{path}`).                     |
| `allowedPaths`   | []string                           | No           | Allowed directory trees on disk. Any path resolving outside these boundaries is blocked.                      |
| `max_size`       | int64 / string                     | No           | Maximum allowed file size in bytes (defaults to 5MB / `5242880` bytes).                                       |
| `description`    | string                             | No           | A brief explanation of what the resource template exposes.                                                    |
| `title`          | string                             | No           | Human-readable title for client display.                                                                      |
| `mimeType`       | string                             | No           | Default MIME type for content returned by this template.                                                      |
| `annotations`    | [Annotations](./_index.md#annotations-schema) | No   | Metadata annotations (`priority`, `audience`, `lastModified`).                                                |

## Guardrails & Security Model

- **Strict `{path}` Variable Requirement**: The `uriTemplate` must follow the [RFC 6570](https://datatracker.ietf.org/doc/html/rfc6570) specification and **must contain only the `{path}` variable**. Any other template variables (e.g., `{id}`, `{filename}`) will fail validation at startup.
- **Path Traversal Prevention**:
  - If `allowedPaths` is defined, the target path must resolve strictly within one of the specified allowed directories.
  - Path traversal attempts using relative segments (such as `../`) or symlink escapes will be blocked with a security violation error.
- **Hidden File Protection**:
  - If `allowedPaths` is omitted, access to hidden files and directories (files or directories beginning with `.`) is automatically blocked.
- **Allowed Extensions**:
  - The requested URI and the resolved disk path must both have an allowed text file extension:
    - `.txt`, `.md`, `.csv`, `.json`, `.yaml`, `.yml`, `.xml`, `.sql`
  - Requests for files with unsupported extensions are rejected.
- **Regular Files Only**:
  - `resources/read` can only read regular files. If a client provides a URI that resolves to a directory, block device, socket, or pipe, an error is returned.
- **Size Limits & Truncation (`max_size`)**:
  - If a matched file exceeds `max_size` (defaults to 5MB / `5242880` bytes, configurable up to 1GB), Toolbox does not error. Instead, it reads up to `max_size` bytes, safely cleans incomplete multi-byte UTF-8 sequences at the cut-off boundary, and appends a clear truncation notice:
    ```text
    ...[TRUNCATED BY SERVER: Payload exceeded <limit> byte safety limit]...
    ```
    This prevents memory exhaustion while still delivering the initial content of large files to the LLM.

