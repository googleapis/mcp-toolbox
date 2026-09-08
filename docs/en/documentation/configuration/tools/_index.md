---
title: "Tools"
type: docs
weight: 4
description: >
  Tools define actions an agent can take -- such as reading and writing to a
  source.
---

A tool represents an action your agent can take, such as running a SQL
statement. You can define Tools as a map with the `tool` kind in your
`tools.yaml` file. Typically, a tool will require a source to act on:

```yaml
kind: tool
name: search_flights_by_number
type: postgres-sql
source: my-pg-instance
statement: |
  SELECT * FROM flights
  WHERE airline = $1
  AND flight_number = $2
  LIMIT 10
description: |
  Use this tool to get information for a specific flight.
  Takes an airline code and flight number and returns info on the flight.
  Do NOT use this tool with a flight id. Do NOT guess an airline code or flight number.
  An airline code is a code for an airline service consisting of a two-character
  airline designator and followed by a flight number, which is a 1 to 4 digit number.
  For example, if given CY 0123, the airline is "CY", and flight_number is "123".
  Another example for this is DL 1234, the airline is "DL", and flight_number is "1234".
  If the tool returns more than one option choose the date closest to today.
  Example:
  {{
      "airline": "CY",
      "flight_number": "888",
  }}
  Example:
  {{
      "airline": "DL",
      "flight_number": "1234",
  }}
parameters:
  - name: airline
    type: string
    description: Airline unique 2 letter identifier
  - name: flight_number
    type: string
    description: 1 to 4 digit number
```

## Specifying Parameters

Parameters for each Tool will define what inputs the agent will need to provide
to invoke them. Parameters should be pass as a list of Parameter objects:

```yaml
parameters:
  - name: airline
    type: string
    description: Airline unique 2 letter identifier
  - name: flight_number
    type: string
    description: 1 to 4 digit number
```

### Basic Parameters

Basic parameters types include `string`, `integer`, `float`, `boolean` types. In
most cases, the description will be provided to the LLM as context on specifying
the parameter.

```yaml
parameters:
  - name: airline
    type: string
    description: Airline unique 2 letter identifier
```

| **field**      |    **type**    | **required** | **description**                                                                                                                                                                                                                        |
|----------------|:--------------:|:------------:|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| name           |     string     |     true     | Name of the parameter.                                                                                                                                                                                                                 |
| type           |     string     |     true     | Must be one of "string", "integer", "float", "boolean" "array"                                                                                                                                                                         |
| description    |     string     |     true     | Natural language description of the parameter to describe it to the agent.                                                                                                                                                             |
| default        | parameter type |    false     | Default value of the parameter. If provided, `required` will be `false`.                                                                                                                                                               |
| required       |      bool      |    false     | Indicate if the parameter is required. Default to `true`.                                                                                                                                                                              |
| allowedValues  |    []string    |    false     | Input value will be checked against this field. Regex is also supported.                                                                                                                                                               |
| excludedValues |    []string    |    false     | Input value will be checked against this field. Regex is also supported.                                                                                                                                                               |
| escape         |     string     |    false     | Only available for type `string`. Indicate the escaping delimiters used for the parameter. This field is intended to be used with templateParameters. Must be one of "single-quotes", "double-quotes", "backticks", "square-brackets". |
| minValue       |  int or float  |    false     | Only available for type `integer` and `float`. Indicate the minimum value allowed.                                                                                                                                                     |
| maxValue       |  int or float  |    false     | Only available for type `integer` and `float`. Indicate the maximum value allowed.                                                                                                                                                     |
| secure         |      bool      |    false     | When `true`, marks the parameter as secure (sensitive application parameter supplied out-of-band; isolated from LLM context). Secure parameters cannot have `default`, `authServices`, or `required: false`. Requires protocol version `2026-07-28` and the `com.google.cloud/toolbox.v1` extension. Defaults to `false`. |

### Optional Parameters

Parameters are **required by default**. Omitting `required` is the same as
writing `required: true`, so an agent that calls the tool without the argument
gets `parameter "airline" is required` back.

There are two ways to make a parameter optional, and they behave differently:

```yaml
parameters:
  # Optional with a fallback: omitted calls use "AA".
  - name: airline
    type: string
    description: Airline unique 2 letter identifier
    default: AA

  # Optional with no fallback: omitted calls pass no value for the parameter,
  # which a SQL statement binds as NULL.
  - name: seat_class
    type: string
    description: Seat class to filter by
    required: false
```

Providing a `default` also makes the parameter optional — it overrides
`required: true` rather than conflicting with it. The full matrix:

| `required` | `default`             | Effective behavior                                              |
|:-----------|:----------------------|:----------------------------------------------------------------|
| omitted    | omitted               | **Required.** Calls that omit the argument are rejected.         |
| `true`     | omitted               | **Required.** Same as above.                                     |
| `false`    | omitted               | Optional; omitted calls pass no value (`NULL` in SQL).           |
| `true`     | a value               | **Optional**;  the `default` wins over `required: true`.         |
| omitted    | a value               | Optional; omitted calls use the default.                         |
| `false`    | a value               | Optional; omitted calls use the default.                         |

This distinction also reaches the agent: a parameter that is effectively
optional is advertised as not required in the tool manifest, so the model knows
it may omit the argument.

{{< notice warning >}}
**`default: null` does not make a parameter optional.** In YAML, `default:
null` (and the equivalent `default: ~`, or a `default:` key with nothing after
it) parses to a null value, which Toolbox cannot distinguish from the field
being absent altogether. The parameter therefore stays **required**, and calls
that omit the argument fail at invocation time with `parameter "..." is
required`.

If you want "optional, with no value when the caller omits it", write
`required: false` instead:

```yaml
# Does NOT work — the parameter is still required.
- name: seat_class
  type: string
  description: Seat class to filter by
  default: null

# Works.
- name: seat_class
  type: string
  description: Seat class to filter by
  required: false
```

Note that an explicit empty value *is* a real default: `default: ""` makes a
string parameter optional and substitutes the empty string. Only `null` is
ignored.
{{< /notice >}}

### Array Parameters

The `array` type is a list of items passed in as a single parameter.
To use the `array` type, you must also specify what kind of items are
in the list using the items field:

```yaml
parameters:
  - name: preferred_airlines
    type: array
    description: A list of airline, ordered by preference.
    items:
      name: name
      type: string
      description: Name of the airline.
statement: |
  SELECT * FROM airlines WHERE preferred_airlines = ANY($1);
```

| **field**      |     **type**     | **required** | **description**                                                            |
|----------------|:----------------:|:------------:|----------------------------------------------------------------------------|
| name           |      string      |     true     | Name of the parameter.                                                     |
| type           |      string      |     true     | Must be "array"                                                            |
| description    |      string      |     true     | Natural language description of the parameter to describe it to the agent. |
| default        |  parameter type  |    false     | Default value of the parameter. If provided, `required` will be `false`.   |
| required       |       bool       |    false     | Indicate if the parameter is required. Default to `true`.                  |
| allowedValues  |     []string     |    false     | Input value will be checked against this field. Regex is also supported.   |
| excludedValues |     []string     |    false     | Input value will be checked against this field. Regex is also supported.   |
| items          | parameter object |     true     | Specify a Parameter object for the type of the values in the array.        |

{{< notice note >}}
Items in array should not have a `default` or `required` value. If provided, it
will be ignored.
{{< /notice >}}

### Map Parameters

The map type is a collection of key-value pairs. It can be configured in two
ways:

- Generic Map: By default, it accepts values of any primitive type (string,
  integer, float, boolean), allowing for mixed data.
- Typed Map: By setting the valueType field, you can enforce that all values
  within the map must be of the same specified type.

#### Generic Map (Mixed Value Types)

This is the default behavior when valueType is omitted. It's useful for passing
a flexible group of settings.

```yaml
parameters:
  - name: execution_context
    type: map
    description: A flexible set of key-value pairs for the execution environment.
```

#### Typed Map

Specify valueType to ensure all values in the map are of the same type. An error
will be thrown in case of value type mismatch.

```yaml
parameters:
  - name: user_scores
    type: map
    description: A map of user IDs to their scores. All scores must be integers.
    valueType: integer # This enforces the value type for all entries.
```

### Secure Parameters

Secure parameters are designed for sensitive runtime context (such as an end-user `customer_id`, tenant identifier, or session token) that **AI agents (LLMs) should not control or see** and that should not be transmitted in plain text through prompt completion requests, model context windows, or standard server logs.

{{< notice note >}}
Secure parameters should be used for client-supplied runtime values (such as `customer_id` or end-user context). Database credentials (such as service account passwords or API keys) should be configured directly in the Data Source configuration rather than passed as per-request tool parameters.
{{< /notice >}}

To configure a parameter as secure, set the `secure` field to `true` in your tool's parameter definition:

```yaml
kind: tool
name: search_secure_data
type: postgres-sql
source: my-pg-instance
statement: |
  SELECT * FROM sessions WHERE customer_id = $1 AND session_token = $2
parameters:
  - name: customer_id
    type: string
    description: Sensitive customer identifier supplied out-of-band by the calling application
    secure: true
  - name: session_token
    type: string
    description: Sensitive session token supplied out-of-band by the calling application
    secure: true
```

#### How Secure Parameters Work

When a parameter is marked as `secure: true`:

1. **LLM Schema Isolation:** The parameter is published in `secureInputSchema` rather than `inputSchema` in the MCP manifest (`tools/list`). Toolbox SDKs and frameworks automatically strip it from public tool signatures, docstrings, and function declarations (such as Gemini declarations in ADK or parameter schemas in LangChain, LlamaIndex, and Genkit). The LLM never sees, requests, or hallucinates the parameter.
2. **Out-of-Band Wire Transmission:** The host application binds the value in code. When the tool is executed, the parameter is transmitted out-of-band in the dedicated `secureArguments` field of the MCP `tools/call` JSON-RPC request, completely isolated from model `arguments`.
3. **Fail-Closed Security & Prompt Injection Defense:**
   * **Client SDK Fast-Fail:** Toolbox SDKs validate locally that all required secure parameters are bound before sending any request over the wire, raising an immediate client-side error if any are missing.
   * **Prompt Injection Defense:** If a prompt-injected LLM or malicious client attempts to supply a secure parameter in standard `arguments`, the server detects the parameter collision and returns a tool execution error (`isError: true`), preventing the model from overriding secure values.
   * **Protocol Enforcement:** If standard parameters are passed in `secureArguments` or required secure parameters are missing, the server rejects the request with a JSON-RPC protocol error (`-32602` `INVALID_PARAMS`). If an un-negotiated client attempts to invoke a secure tool directly, the server rejects the call with JSON-RPC error `-32021` (or `-32602` `INVALID_PARAMS` on protocol versions prior to `2026-07-28` where secure tools are completely hidden).
   * **Dedicated APIs & Mutual Exclusivity:** SDKs provide dedicated methods (`bind_secure_param` / `bindSecureParam` / `WithBindSecureParam*`) that cannot be cross-mixed with regular parameter binding methods (`bind_param` / `bindParam` / `WithBindParam*`), throwing clear guidance errors if mismatched.

{{< notice note >}}
Secure parameters are always required and cannot be optional. A parameter cannot have `secure: true` alongside `authServices`, `default`, or `required: false`.
{{< /notice >}}

#### SDK Usage Examples

Below is how you bind secure parameters across our official SDKs:

{{< tabpane persist=header >}}
{{< tab header="Python" lang="python" >}}
from toolbox_core import ToolboxClient

async with ToolboxClient("http://127.0.0.1:5000") as toolbox:
    # Option A: Bind when loading tool or toolset
    bound_tool = await toolbox.load_tool(
        "search_secure_data",
        secure_params={"customer_id": "cust_12345", "session_token": "token-xyz"}
    )
    tools = await toolbox.load_toolset(
        "my-toolset",
        secure_params={"customer_id": "cust_12345"}
    )

    # Option B: Bind on an un-bound loaded tool (returns a new immutable tool instance)
    raw_tool = await toolbox.load_tool("search_secure_data")
    single_bound = raw_tool.bind_secure_param("customer_id", "cust_12345")
    multi_bound = raw_tool.bind_secure_params({
        "customer_id": "cust_12345",
        "session_token": "token-xyz",
    })

    # Option C: Dynamic callable (evaluated per invocation)
    dynamic_tool = raw_tool.bind_secure_param("customer_id", lambda: get_current_user_id())

    # Execute tool (LLM only supplies standard arguments, secure parameters attached automatically)
    result = await multi_bound()
{{< /tab >}}
{{< tab header="JavaScript / TypeScript" lang="javascript" >}}
import { ToolboxClient } from '@toolbox-sdk/core';

const client = new ToolboxClient("http://127.0.0.1:5000");

// Option A: Pre-bind when loading tool or toolset
const boundTool = await client.loadTool(
    "search_secure_data",
    null, // authTokenGetters
    null, // boundParams
    { customer_id: "cust_12345", session_token: "token-xyz" } // secureParams
);
const tools = await client.loadToolset(
    "my-toolset",
    null, // authTokenGetters
    null, // boundParams
    false, // strict (set to true to error if any tool lacks the bound params)
    { customer_id: "cust_12345" } // secureParams
);

// Option B: Bind on an un-bound loaded tool (returns a new immutable tool instance)
const rawTool = await client.loadTool("search_secure_data");
const singleBound = rawTool.bindSecureParam("customer_id", "cust_12345");
const multiBound = rawTool.bindSecureParams({
    customer_id: "cust_12345",
    session_token: "token-xyz",
});

// Option C: Dynamic function (evaluated per invocation)
const dynamicTool = rawTool.bindSecureParam("customer_id", async () => getCurrentUserId());

// Execute tool
const result = await multiBound();
{{< /tab >}}
{{< tab header="Go" lang="go" >}}
import (
    "context"
    "github.com/googleapis/mcp-toolbox-sdk-go/core"
)

ctx := context.Background()
client, err := core.NewToolboxClient("http://127.0.0.1:5000")

// Option A: Bind when loading tool or toolset
boundTool, err := client.LoadTool("search_secure_data", ctx,
    core.WithBindSecureParamString("customer_id", "cust_12345"),
    core.WithBindSecureParamString("session_token", "token-xyz"),
)
tools, err := client.LoadToolset("my-toolset", ctx,
    core.WithBindSecureParamString("customer_id", "cust_12345"),
)

// Option B: Set client-level defaults across all tools loaded by the client
clientWithDefaults, err := core.NewToolboxClient("http://127.0.0.1:5000",
    core.WithDefaultToolOptions(
        core.WithBindSecureParamString("customer_id", "cust_12345"),
    ),
)

// Option C: Bind on an un-bound loaded tool (returns a new immutable tool instance)
rawTool, err := client.LoadTool("search_secure_data", ctx)
postBoundTool, err := rawTool.ToolFrom(
    core.WithBindSecureParamString("customer_id", "cust_12345"),
    core.WithBindSecureParamString("session_token", "token-xyz"),
)

// Option D: Dynamic function (evaluated per invocation)
dynamicTool, err := rawTool.ToolFrom(
    core.WithBindSecureParamStringFunc("customer_id", func() (string, error) {
        return getCurrentUserID(), nil
    }),
    core.WithBindSecureParamString("session_token", "token-xyz"),
)

// Execute tool
result, err := boundTool.Invoke(ctx, map[string]any{})
{{< /tab >}}
{{< /tabpane >}}

{{< notice note >}}
Secure parameters require MCP protocol version `2026-07-28` and the `com.google.cloud/toolbox.v1` extension. For more details on extension capabilities and client requirements, see the [Extension README](https://github.com/googleapis/mcp-toolbox/blob/main/extensions/2026-07-28/README.md).

For in-depth framework integration guides, see:
* [Python SDK Secure Parameters](../../connect-to/toolbox-sdks/python-sdk/core/index.md#secure-parameters) ([ADK](../../connect-to/toolbox-sdks/python-sdk/adk/index.md#secure-parameters) | [LangChain](../../connect-to/toolbox-sdks/python-sdk/langchain/index.md#secure-parameters) | [LlamaIndex](../../connect-to/toolbox-sdks/python-sdk/llamaindex/index.md#secure-parameters))
* [JavaScript / TypeScript SDK Secure Parameters](../../connect-to/toolbox-sdks/javascript-sdk/core/index.md#secure-parameters) ([ADK](../../connect-to/toolbox-sdks/javascript-sdk/adk/index.md#secure-parameters))
* [Go SDK Secure Parameters](../../connect-to/toolbox-sdks/go-sdk/core/_index.md#secure-parameters) ([ADK](../../connect-to/toolbox-sdks/go-sdk/tbadk/_index.md#secure-parameters) | [Genkit](../../connect-to/toolbox-sdks/go-sdk/tbgenkit/_index.md#secure-parameters))
{{< /notice >}}

### Authenticated Parameters

Authenticated parameters are automatically populated with user
information decoded from [ID
tokens](../authentication/_index.md#specifying-id-tokens-from-clients) that are passed in
request headers. They do not take input values in request bodies like other
parameters. To use authenticated parameters, you must configure the tool to map
the required [authService](../authentication/_index.md) to specific claims within the
user's ID token.

```yaml
kind: tool
name: search_flights_by_user_id
type: postgres-sql
source: my-pg-instance
statement: |
  SELECT * FROM flights WHERE user_id = $1
parameters:
  - name: user_id
    type: string
    description: Auto-populated from Google login
    authServices:
      # Refer to one of the `authService` defined
      - name: my-google-auth
        # `sub` is the OIDC claim field for user ID
        field: sub
```

| **field** | **type** | **required** | **description**                                                                  |
|-----------|:--------:|:------------:|----------------------------------------------------------------------------------|
| name      |  string  |     true     | Name of the [authServices](../authentication/_index.md) used to verify the OIDC auth token. |
| field     |  string  |     true     | Claim field decoded from the OIDC token used to auto-populate this parameter.    |

### Template Parameters

Template parameters types include `string`, `integer`, `float`, `boolean` types.
In most cases, the description will be provided to the LLM as context on
specifying the parameter. Template parameters will be inserted into the SQL
statement before executing the prepared statement. They will be inserted without
quotes, so to insert a string using template parameters, quotes must be
explicitly added within the string.

Template parameter arrays can also be used similarly to basic parameters, and array
items must be strings. Once inserted into the SQL statement, the outer layer of
quotes will be removed. Therefore to insert strings into the SQL statement, a
set of quotes must be explicitly added within the string.

{{< notice warning >}}
Because template parameters can directly replace identifiers, column names, and
table names, they are prone to SQL injections. Basic parameters are preferred
for performance and safety reasons.
{{< /notice >}}

{{< notice tip >}}
To minimize SQL injection risk when using template parameters, always provide
the `allowedValues` field within the parameter to restrict inputs.

Alternatively, for `string` type parameters, you can use the `escape` field to
add delimiters to the identifier, though please note that escaping alone does
not fully secure the parameter.

For `integer` or `float` type parameters, you can use `minValue` and `maxValue`
to define the allowable range.
{{< /notice >}}

```yaml
kind: tool
name: select_columns_from_table
type: postgres-sql
source: my-pg-instance
statement: |
  SELECT {{array .columnNames}} FROM {{.tableName}}
description: |
  Use this tool to list all information from a specific table.
  Example:
  {{
      "tableName": "flights",
      "columnNames": ["id", "name"]
  }}
templateParameters:
  - name: tableName
    type: string
    description: Table to select from
  - name: columnNames
    type: array
    description: The columns to select
    items:
      name: column
      type: string
      description: Name of a column to select
      escape: double-quotes # with this, the statement will resolve to `SELECT "id", "name" FROM flights`
```

| **field**      |     **type**     |  **required**   | **description**                                                                     |
|----------------|:----------------:|:---------------:|-------------------------------------------------------------------------------------|
| name           |      string      |      true       | Name of the template parameter.                                                     |
| type           |      string      |      true       | Must be one of "string", "integer", "float", "boolean", "array"                     |
| description    |      string      |      true       | Natural language description of the template parameter to describe it to the agent. |
| default        |  parameter type  |      false      | Default value of the parameter. If provided, `required` will be `false`.            |
| required       |       bool       |      false      | Indicate if the parameter is required. Default to `true`.                           |
| allowedValues  |     []string     |      false      | Input value will be checked against this field. Regex is also supported.            |
| excludedValues |     []string     |      false      | Input value will be checked against this field. Regex is also supported.            |
| items          | parameter object | true (if array) | Specify a Parameter object for the type of the values in the array (string only).   |

## Tool-Level Scopes (MCP Authorization)

The Model Context Protocol supports [MCP Authorization](https://modelcontextprotocol.io/docs/tutorials/security/authorization) to secure interactions between clients and servers. When using MCP Authorization in Toolbox, you can enforce granular tool-level scope authorization by specifying the `scopesRequired` field in the tool configuration.

For detailed information on how to configure this and examples, please see the [Generic OIDC Auth](../authentication/generic.md#tool-level-scopes) documentation.

## Authorized Invocations (Toolbox Native Authorization)

You can require an authorization check for any Tool invocation request by
specifying an `authRequired` field. Specify a list of
[authServices](../authentication/_index.md) defined in the previous section.

```yaml
kind: tool
name: search_all_flight
type: postgres-sql
source: my-pg-instance
statement: |
  SELECT * FROM flights
# A list of `authService` defined previously
authRequired:
  - my-google-auth
  - other-auth-service
```

## Tool Annotations

Tool annotations provide semantic metadata that helps MCP clients understand tool
behavior. These hints enable clients to make better decisions about tool usage
and provide appropriate user experiences.

### Available Annotations

| **annotation**     |  **type**   | **default** | **description**                                                        |
|--------------------|:-----------:|:-----------:|------------------------------------------------------------------------|
| readOnlyHint       |    bool     |    false    | Tool only reads data, no modifications to the environment.             |
| destructiveHint    |    bool     |    true     | Tool may create, update, or delete data.                               |
| idempotentHint     |    bool     |    false    | Repeated calls with same arguments have no additional effect.          |
| openWorldHint      |    bool     |    true     | Tool interacts with external entities beyond its local environment.    |

### Specifying Annotations

Annotations can be specified in YAML tool configuration:

```yaml
kind: tool
name: my_query_tool
type: mongodb-find-one
source: my-mongodb
description: Find a single document
database: mydb
collection: users
annotations:
  readOnlyHint: true
  idempotentHint: true
```

### Default Annotations

If not specified, tools use sensible defaults based on their operation type:

- **Read operations** (find, aggregate, list): `readOnlyHint: true`
- **Write operations** (insert, update, delete): `destructiveHint: true`, `readOnlyHint: false`

### MCP Client Response

Annotations appear in the `tools/list` MCP response:

```json
{
  "name": "my_query_tool",
  "description": "Find a single document",
  "annotations": {
    "readOnlyHint": true
  }
}
```

## URL Parameter Binding

You can bind specific arguments to tools at the transport level using URL query parameters. This allows you to restrict clients to specific database instances, projects, or environments dynamically without modifying the server configuration.

For a comprehensive guide, see the [URL Parameter Binding](./url_parameter_binding.md) documentation.

## Tool UI Metadata (MCP Apps)

Tools can be associated with an interactive visual interface provided by an MCP resource (such as an interactive dashboard, custom form, or chart). This capability is part of the [MCP Apps](../resources/apps.md) extension.

UI capabilities are configured directly on standard resources (`kind: resource` or `kind: resourceTemplate`) using the optional `ui:` field. Tools reference the target resource by its `name`.

To link a tool to a UI resource, specify the `ui` configuration block:

```yaml
kind: tool
name: view_customer_dashboard
type: postgres-sql
source: my-pg-instance
statement: "SELECT * FROM customers WHERE id = $1"
description: "Retrieves customer information and opens the interactive dashboard."
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

| **field**      | **type**  | **required** | **description**                                                                                                            |
|----------------|-----------|--------------|----------------------------------------------------------------------------------------------------------------------------|
| `resource`     | string    | Yes          | The `name` of a configured `resource` or `resourceTemplate` providing the UI for this tool.                               |
| `visibility`   | []string  | No           | Controls who can see the tool. Allowed values are `model` and `app`. Defaults to `["model", "app"]` if omitted.          |

When serving `tools/list` responses, Toolbox automatically resolves the `resource` name into the target resource's full URI (`resourceUri`) in the tool manifest's `_meta.ui` block. For details on defining UI resources, Content Security Policy, and device permissions, see the [MCP Apps & UI Resources](../resources/apps.md) documentation.

## Using tools with MCP Toolbox Client SDKs

Once your tools are defined in your configuration, you can retrieve them directly from your application code.

Here is how to load and invoke your tools across our supported languages:

### Python

```python
# Loading a single tool
tool = await toolbox.load_tool("my-tool")

# Invoke the tool
result = await tool("foo", bar="baz")

```

### Javascript/Typescript

```javascript
// Loading a single tool
const tool = await client.loadTool("my-tool")

// Invoke the tool
const result = await tool({a: 5, b: 2})
```

### Go

```go
// Loading a single tool
tool, err = client.LoadTool("my-tool", ctx)

// Invoke the tool
inputs := map[string]any{"location": "London"}
result, err := tool.Invoke(ctx, inputs)
```


To see all supported sources and the specific tools they unlock, explore the full list of our [Integrations](../../../integrations/_index.md).
