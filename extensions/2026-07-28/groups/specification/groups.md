# Groups Specification

**Extension Identifier:** `com.google.cloud/toolbox.v1`  
**Protocol Version:** `2026-07-28`  
**Schema Definition:** [`../schema/schema.ts`](../schema/schema.ts)

---

## 1. Overview & Motivation

A **Group** is a named collection that scopes MCP primitives together — currently tools and prompts. Toolbox serves each group on its own endpoint (`/mcp/{name}`), so connecting to that endpoint scopes `tools/list` and `prompts/list` to the group's contents.

That endpoint-per-group model has a discovery gap: a client must already know a group's name to connect to it, and once connected it can only see one group at a time. Base MCP has no method for asking a server "what collections do you offer?"

**Groups** close the gap with two server-scoped methods:

- **`groups/list`** — enumerate every named group with its `name` and `description`, so a client can choose one without prior configuration.
- **`groups/get`** — fetch a single group's tools and prompts together in one round trip, instead of connecting to that group's endpoint and issuing separate `tools/list` and `prompts/list` calls.

Both are scoped to the **server**, not to the endpoint they are called on. Calling `groups/get` on `/mcp/data_analyst` can return the contents of the `admin` group.

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

Extensions can be disabled on the server using the `--disable-ext com.google.cloud/toolbox.v1` CLI flag. With the extension disabled, `groups/list` and `groups/get` are unreachable even for an extension-aware client.

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

### 2.3 Availability Matrix

Both methods belong to the experimental Toolbox extension, not to the base MCP specification. A client on any earlier protocol version cannot reach them at all — the methods are not registered on those versions, so the request fails at method dispatch rather than at capability negotiation.

| Client's protocol version | Declares `com.google.cloud/toolbox.v1` | Result                                        |
| ------------------------- | -------------------------------------- | --------------------------------------------- |
| Earlier than `2026-07-28` | n/a                                    | `METHOD_NOT_FOUND` (-32601)                   |
| `2026-07-28`              | No                                     | `MISSING_REQUIRED_CLIENT_CAPABILITY` (-32021) |
| `2026-07-28`              | Yes                                    | Served                                        |

---

## 3. Protocol Methods & Behavior

### 3.1 Group Discovery (`groups/list`)

Returns every named group's `name` and `description`, sorted alphabetically by name.

- The default (nameless) group is **omitted** — it is not a configured collection but the implicit set of everything on the server. Reach it through `groups/get` instead.
- The method is **not paginated**. A server configures a bounded set of groups, so all of them come back in one response. There is no `cursor` request field and no `nextCursor` in the result.
- The response carries **no `ttlMs` / `cacheScope` hint**. It spans groups that may each configure a different TTL, so no single hint applies.

#### Example `groups/list` Request

```json
{
  "jsonrpc": "2.0",
  "id": "groups-list-1",
  "method": "groups/list",
  "params": {
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

#### Example `groups/list` Response

```json
{
  "jsonrpc": "2.0",
  "id": "groups-list-1",
  "result": {
    "resultType": "complete",
    "groups": [
      {
        "name": "admin",
        "description": "Administrative operations."
      },
      {
        "name": "data_analyst",
        "description": "Tools and prompts for exploratory data analysis."
      }
    ],
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

### 3.2 Group Contents (`groups/get`)

Takes a group `name` and returns that group's tools and prompts together, along with the group's cache hints.

- The group's `description` is **intentionally omitted** from the result; it is exposed only through `groups/list`.
- `ttlMs` and `cacheScope` are the group's own configured values — the same hints `tools/list` and `prompts/list` return when called on that group's endpoint. They default to `300000` (5 minutes) and `"public"`.
- An **omitted or empty `name`** resolves to the default (nameless) group, which holds every tool and prompt on the server. This mirrors the `/api/toolset` REST endpoint called without a toolset name. Since `groups/list` omits the default group, this is the only way to reach it over MCP.
- An **unrecognized `name`** returns `INVALID_PARAMS` (-32602).
- Tools are serialized exactly as `tools/list` serializes them. Because reaching `groups/get` at all requires declaring `com.google.cloud/toolbox.v1`, tools defining secure parameters are always included, with their sensitive parameters split into `secureInputSchema`. See the [Secure Parameters specification](../../secureParams/specification/secure_params.md).

#### Example `groups/get` Request

```json
{
  "jsonrpc": "2.0",
  "id": "groups-get-1",
  "method": "groups/get",
  "params": {
    "name": "data_analyst",
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

#### Example `groups/get` Response

```json
{
  "jsonrpc": "2.0",
  "id": "groups-get-1",
  "result": {
    "resultType": "complete",
    "name": "data_analyst",
    "ttlMs": 300000,
    "cacheScope": "public",
    "tools": [
      {
        "name": "list_tables",
        "description": "List tables in the database.",
        "inputSchema": {
          "type": "object",
          "properties": {},
          "required": []
        }
      },
      {
        "name": "execute_sql",
        "description": "Run a SQL query.",
        "inputSchema": {
          "type": "object",
          "properties": {
            "sql": { "type": "string" }
          },
          "required": ["sql"]
        }
      }
    ],
    "prompts": [
      {
        "name": "summarize_results",
        "description": "Summarize query results."
      }
    ],
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

### 3.3 Transport Headers

Over the HTTP transport, requests carry `Mcp-Method` and `Mcp-Name` headers that MUST agree with the request body. For `groups/list`, `Mcp-Name` MUST be empty. For `groups/get`, `Mcp-Name` MUST equal `params.name` — including the empty string when resolving the default group. A mismatch returns `HEADER_MISMATCH` (-32020). The stdio transport has no headers and skips this check.

---

## 4. Configuration Constraints

Groups are declared as `kind: group` documents in the Toolbox configuration:

- **`name`** is required and unique across both `kind: group` and `kind: toolset` documents; a collision is a startup error.
- **`description`** is optional and is surfaced only through `groups/list`. A `description` written on a `kind: toolset` is dropped with a warning, because a toolset is a tools-only group without one.
- **`ttlMs`** defaults to `300000`; **`cacheScope`** defaults to `"public"`. Both are returned by `groups/get`.
- Every `kind: toolset` loads as a tools-only group and is therefore visible to `groups/list` and `groups/get`, with an empty `prompts` array.

---

## 5. Error Handling Matrix

| Error Type | Protocol Level / Result | Error Code | Error Message | Condition |
|---|---|---|---|---|
| Method Not Found | JSON-RPC Error | `-32601` (`METHOD_NOT_FOUND`) | `invalid method groups/list` | Calling `groups/list` or `groups/get` on a protocol version earlier than `2026-07-28`. |
| Missing Client Capability | JSON-RPC Error | `-32021` (`MISSING_REQUIRED_CLIENT_CAPABILITY`) | `missing required client capability: method "groups/list" requires com.google.cloud/toolbox.v1 extension which is not supported by the client` | Calling either method on protocol `2026-07-28` without declaring `com.google.cloud/toolbox.v1` in client capabilities, or against a server started with `--disable-ext com.google.cloud/toolbox.v1`. |
| Group Not Found | JSON-RPC Error | `-32602` (`INVALID_PARAMS`) | `invalid group name: group with name "<name>" does not exist` | `groups/get` was called with a `name` that is not a configured group. |
| Missing Request Metadata | JSON-RPC Error | `-32602` (`INVALID_PARAMS`) | `_meta error: missing required fields in request metadata` | The request omitted required `_meta` fields (protocol version, client info, or client capabilities). |
| Header Mismatch | JSON-RPC Error | `-32020` (`HEADER_MISMATCH`) | `Mcp-Name header value '<header>' does not match body value '<body>'` | Over HTTP, `Mcp-Method` or `Mcp-Name` disagrees with the request body. |
| Malformed Request | JSON-RPC Error | `-32600` (`INVALID_REQUEST`) | `invalid mcp groups list request: <parse error>` for `groups/list`; `invalid mcp groups/get request: <parse error>` for `groups/get` | The request body could not be parsed into the method's request shape. |
