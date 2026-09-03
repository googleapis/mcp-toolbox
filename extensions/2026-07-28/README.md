# Extension Version: 2026-07-28

This directory contains the schemas and specifications for the `2026-07-28` version of the experimental MCP Toolbox extension.

## Extension Identifier

The identifier we are using for this extension version is `com.google.cloud/toolbox.v1`.

**Currently Supported Extensions:**
- **Secure Parameters**: The secure parameter feature is strictly tied to the latest `v20260728` MCP protocol and the `com.google.cloud/toolbox.v1` experimental extension.
- **Groups**: The `groups/list` and `groups/get` methods are strictly tied to the latest `v20260728` MCP protocol and the `com.google.cloud/toolbox.v1` experimental extension. Earlier protocol versions do not implement them (`METHOD_NOT_FOUND`), and a `v20260728` client that has not declared the extension gets `MISSING_REQUIRED_CLIENT_CAPABILITY`.  
  Specification: [`groups/specification/groups.md`](./groups/specification/groups.md) · Schema: [`groups/schema/schema.ts`](./groups/schema/schema.ts)
