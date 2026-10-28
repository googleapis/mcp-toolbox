---
title: "File"
type: docs
weight: 2
description: >
  Direct file resources exposed from the local filesystem.
---

File resources allow you to expose specific files from your local filesystem as read-only MCP resources. This is useful for sharing living configuration files, database schemas, local documentation, or static data files with Large Language Models.

## Examples

### Relative Path Resource

When using relative paths, the path is resolved relative to the directory containing your configuration file.

```yaml
kind: resource
name: database_schema
type: file
description: "Production PostgreSQL schema definition."
path: "./schema/schema.sql"
```

### Absolute Path with Custom URI and Metadata

You can supply an absolute path and override the URI, title, and MIME type:

```yaml
kind: resource
name: api_reference
type: file
title: "OpenAPI Specification"
description: "Complete REST API contract in JSON format."
path: "/var/www/docs/openapi.json"
uri: "file:///docs/openapi.json"
mimeType: "application/json"
annotations:
  priority: 0.9
  audience:
    - assistant
    - user
```

### Custom Size Limit

By default, files up to 5MB can be read. You can configure a smaller or larger limit using `maxSize`:

```yaml
kind: resource
name: large_data_catalog
type: file
description: "Large data catalog CSV."
path: "./data/catalog.csv"
maxSize: 10485760 # 10MB limit
```

## Reference

### File Resource Schema

| **field**     | **type**                           | **required** | **description**                                                                                               |
|---------------|------------------------------------|--------------|---------------------------------------------------------------------------------------------------------------|
| `name`        | string                             | Yes          | Unique name for the resource.                                                                                 |
| `type`        | string                             | Yes          | Must be `"file"`.                                                                                             |
| `path`        | string                             | Yes          | Path to the file on disk (relative to the config directory or absolute).                                      |
| `uri`         | string                             | No           | Unique URI identifying the resource. Defaults to `file:///{normalized_path}` if omitted.                       |
| `maxSize`    | int64 / string                     | No           | Maximum allowed file size in bytes (defaults to 5MB / `5242880` bytes).                                       |
| `description` | string                             | No           | A brief explanation of what the file contains.                                                                |
| `title`       | string                             | No           | Human-readable title for client display.                                                                      |
| `mimeType`    | string                             | No           | MIME type of the content. Auto-detected from file extension or content if omitted.                           |
| `annotations` | [Annotations](../_index.md#annotations-schema) | No   | Metadata annotations (`priority`, `audience`, `lastModified`).                                                |

## Guardrails & Security Model

- **Allowed File Extensions**: To prevent accidental exposure of binaries, executables, or sensitive credentials, Toolbox restricts file resources to known text formats:
  - `.txt`, `.md`, `.csv`, `.json`, `.yaml`, `.yml`, `.xml`, `.sql`
  Files with unsupported extensions are rejected during startup validation.
- **Boot-Time Existence Verification**: The file specified by `path` **must exist** and be readable when the server boots. If the file is missing or inaccessible, the server will fail to start.
- **Regular Files Only**: The target must be a regular file. Directories, sockets, character/block devices, and named pipes are rejected at startup and runtime.
- **Directory Traversal and Sandboxing**:
  - Relative paths are anchored to the directory containing the configuration file.
  - Symlinks are resolved at boot time and evaluated to verify that they do not escape the configuration base directory.
- **Time-of-Check to Time-of-Use (TOCTOU) Protection**: On every `resources/read` request, Toolbox verifies that:
  - The file has not been swapped with a symlink.
  - The underlying inode/file identity matches what was opened.
  - The file extension has not changed post-boot.
- **Dynamic Last-Modified & Size**:
  - In `resources/list`, `size` is reported directly from disk `FileInfo.Size()`.
  - In `annotations.lastModified`, the timestamp is dynamically generated on each query from the file's current disk modification time (in RFC 3339 format).
- **Size Limit & Truncation (`maxSize`)**: If a file exceeds `maxSize` (defaults to 5MB / `5242880` bytes, configurable up to 1GB), Toolbox does not error. Instead, it reads up to `maxSize` bytes, safely cleans incomplete multi-byte UTF-8 sequences at the cut-off boundary, and appends a clear truncation notice:
  ```text
  ...[TRUNCATED BY SERVER: Payload exceeded <limit> byte safety limit]...
  ```
  This prevents memory exhaustion while still delivering the initial content of large files to the LLM.

