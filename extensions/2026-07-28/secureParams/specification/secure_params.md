# Secure Parameters Specification

**Extension Identifier:** `com.google.cloud/toolbox.v1`  
**Protocol Version:** `2026-07-28`  
**Schema Definition:** [`../schema/schema.ts`](../schema/schema.ts)

---

## 1. Overview & Motivation

In agentic AI systems, tools frequently require sensitive runtime parameters (such as an end-user `customer_id`, tenant identifier, session token, or user account ID) to isolate data and enforce security boundaries.

Allowing the Large Language Model (LLM) to supply or view these sensitive parameters introduces critical security vulnerabilities:
- **Prompt Injection & Overriding:** An adversary could manipulate the model via prompt injection to alter a `customer_id` and access other tenants' data.
- **Context Window Leakage:** Sensitive user identifiers and tokens are sent in plaintext to third-party LLM providers, appearing in context windows and prompt completion logs.

**Secure Parameters** address this by separating standard tool parameters from secure parameters at the protocol level:
- **Standard Parameters (`inputSchema` / `arguments`):** Advertised to the LLM agent and populated by the model during tool invocation.
- **Secure Parameters (`secureInputSchema` / `secureArguments`):** Hidden from the LLM agent and passed out-of-band directly by the host application.

---

## 2. Server Capability Discovery & Negotiation

### 2.1 Server Discovery (`server/discover`)

The server advertises enabled MCP extensions in the `capabilities.extensions` object during discovery on protocol version `2026-07-28`:

```json
{
  "jsonrpc": "2.0",
  "id": "discover-1",
  "result": {
    "resultType": "complete",
    "supportedVersions": ["2026-07-28", "2025-11-25", "2025-06-18", "2025-03-26", "2024-11-05"],
    "capabilities": {
      "extensions": {
        "com.google.cloud/toolbox.v1": {}
      },
      "tools": { "listChanged": false },
      "prompts": { "listChanged": false }
    },
    "_meta": {
      "io.modelcontextprotocol/serverInfo": {
        "name": "Toolbox",
        "version": "1.0.0"
      }
    }
  }
}
```

Extensions can be disabled on the server using the `--disable-ext com.google.cloud/toolbox.v1` CLI flag.

### 2.2 Client Capability Declaration

Clients indicate support for the `com.google.cloud/toolbox.v1` extension by advertising the extension capability within `_meta["io.modelcontextprotocol/clientCapabilities"].extensions` in request metadata:

```json
{
  "_meta": {
    "io.modelcontextprotocol/protocolVersion": "2026-07-28",
    "io.modelcontextprotocol/clientInfo": {
      "name": "MyApplicationClient",
      "version": "1.0.0"
    },
    "io.modelcontextprotocol/clientCapabilities": {
      "extensions": {
        "com.google.cloud/toolbox.v1": {}
      }
    }
  }
}
```

---

## 3. Protocol Methods & Behavior

### 3.1 Tool Discovery (`tools/list` and `groups/get`)

When a client calls `tools/list` or `groups/get`:

- **When Client Supports `com.google.cloud/toolbox.v1`:**
  - Tools defining secure parameters are included in the returned `tools` array.
  - Non-secure parameters are placed in standard `inputSchema`.
  - Secure parameters are separated and placed in `secureInputSchema`.
  - Parameters bound via URL parameters are omitted from both schemas.
- **When Client Does NOT Support `com.google.cloud/toolbox.v1` (or on Legacy Protocols `< 2026-07-28`):**
  - Tools with secure parameters are **excluded (filtered out)** from the `tools` list in both `tools/list` and `groups/get` to prevent unsupported invocations.

#### Example `tools/list` Response

```json
{
  "jsonrpc": "2.0",
  "id": "list-1",
  "result": {
    "resultType": "complete",
    "tools": [
      {
        "name": "search_customer_records",
        "description": "Searches customer records by query filter.",
        "inputSchema": {
          "type": "object",
          "properties": {
            "query": {
              "type": "string",
              "description": "The search query string."
            }
          },
          "required": ["query"]
        },
        "secureInputSchema": {
          "type": "object",
          "properties": {
            "customer_id": {
              "type": "string",
              "description": "Sensitive customer identifier supplied out-of-band by the calling application."
            }
          },
          "required": ["customer_id"]
        }
      }
    ],
    "ttlMs": 300000,
    "cacheScope": "public",
    "_meta": {
      "io.modelcontextprotocol/serverInfo": {
        "name": "Toolbox",
        "version": "1.0.0"
      }
    }
  }
}
```

---

### 3.2 Tool Execution (`tools/call`)

When executing a tool that defines secure parameters:

1. **Extension Check (`2026-07-28`):** If the tool defines secure parameters and the client did not declare `com.google.cloud/toolbox.v1` in `_meta["io.modelcontextprotocol/clientCapabilities"].extensions`, execution is rejected with JSON-RPC error `-32021` (`MISSING_REQUIRED_CLIENT_CAPABILITY`).
2. **Legacy Protocol Check (`< 2026-07-28`):** If a client invokes a tool defining secure parameters over a legacy MCP protocol version (`2024-11-05`, `2025-03-26`, `2025-06-18`, `2025-11-25`), execution is rejected with JSON-RPC error `-32602` (`INVALID_PARAMS`): `invalid tool name: tool with name "<name>" does not exist`.
3. **Argument Routing Validation:**
   - Secure parameters MUST NOT be passed in `arguments`. If present in `arguments`, returns JSON-RPC error `-32602` (`INVALID_PARAMS`).
   - Non-secure parameters MUST NOT be passed in `secureArguments`. If present in `secureArguments`, returns JSON-RPC error `-32602` (`INVALID_PARAMS`).
4. **URL Parameter Binding Interaction:**
   - If a parameter is bound via URL query parameters, it is auto-populated by the server.
   - If a client supplies an argument in either `arguments` or `secureArguments` that is also bound by URL parameters, execution returns a tool execution error (`IsError: true`): `parameter "<param_name>" is bound by URL and cannot be provided in client arguments`.
5. **Execution & Required Parameter Validation:**
   - The server validates and merges `arguments` and `secureArguments` internally.
   - If a required secure parameter is missing (and not bound via URL), execution returns a tool execution error (`IsError: true`): `provided parameters were invalid: parameter "<param_name>" is required`.

#### Example `tools/call` Request

```json
{
  "jsonrpc": "2.0",
  "id": "call-1",
  "method": "tools/call",
  "params": {
    "name": "search_customer_records",
    "arguments": {
      "query": "recent transactions"
    },
    "secureArguments": {
      "customer_id": "cust_12345"
    },
    "_meta": {
      "io.modelcontextprotocol/protocolVersion": "2026-07-28",
      "io.modelcontextprotocol/clientInfo": {
        "name": "MyApplicationClient",
        "version": "1.0.0"
      },
      "io.modelcontextprotocol/clientCapabilities": {
        "extensions": {
          "com.google.cloud/toolbox.v1": {}
        }
      }
    }
  }
}
```

---

## 4. Configuration Constraints

When configuring parameters in a tool definition YAML:
- **Required by Default:** Secure parameters (`secure: true`) are always required and cannot be made optional.
- **Mutual Exclusivity:** A parameter cannot specify `secure: true` alongside `authServices`, `default`, or `required: false`.

---

## 5. Error Handling Matrix

| Error Type | Protocol Level / Result | Error Code | Error Message | Condition |
|---|---|---|---|---|
| Missing Client Capability | JSON-RPC Error | `-32021` (`MISSING_REQUIRED_CLIENT_CAPABILITY`) | `missing required client capability: tool "<name>" requires com.google.cloud/toolbox.v1 extension which is not supported by the client` | Invoking a tool requiring secure parameters on protocol `2026-07-28` without declaring `com.google.cloud/toolbox.v1` in client capabilities. |
| Tool Not Found (Legacy Protocols) | JSON-RPC Error | `-32602` (`INVALID_PARAMS`) | `invalid tool name: tool with name "<name>" does not exist` | Invoking a tool requiring secure parameters on legacy protocol versions (`< 2026-07-28`). |
| Secure Parameter in Standard Arguments | JSON-RPC Error | `-32602` (`INVALID_PARAMS`) | `parameter "<param_name>" is secure and must not be passed in standard arguments` | A parameter defined with `secure: true` was passed in `arguments`. |
| Non-Secure Parameter in Secure Arguments | JSON-RPC Error | `-32602` (`INVALID_PARAMS`) | `parameter "<param_name>" is not secure and must not be passed in secureArguments` | A parameter NOT defined with `secure: true` was passed in `secureArguments`. |
| Bound Parameter Override | Tool Execution Error (`IsError: true`) | N/A | `parameter "<param_name>" is bound by URL and cannot be provided in client arguments` | A parameter bound via URL query parameters was also passed in `arguments` or `secureArguments`. |
| Missing Required Secure Parameter | Tool Execution Error (`IsError: true`) | N/A | `provided parameters were invalid: parameter "<param_name>" is required` | A required secure parameter was not provided in `secureArguments` (and not bound by URL). |
