# Extension Version: 2026-07-28

This directory contains schemas and specifications for the `2026-07-28` version of the experimental MCP Toolbox extension.

## Extension Identifier

The identifier for this extension version is **`com.google.cloud/toolbox.v1`**.

## Supported Capabilities

- **[Secure Parameters](./secureParams/specification/secure_params.md)**: Separates sensitive runtime parameters from LLM visibility via `secureInputSchema` and `secureArguments`.  
  Schema: [`secureParams/schema/schema.ts`](./secureParams/schema/schema.ts)
- **[Groups](./groups/specification/groups.md)**: Exposes a server's configured groups over MCP through the `groups/list` and `groups/get` methods.  
  Schema: [`groups/schema/schema.ts`](./groups/schema/schema.ts)
