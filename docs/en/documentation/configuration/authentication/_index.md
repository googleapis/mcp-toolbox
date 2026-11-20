---
title: "Authentication"
type: docs
weight: 1
description: >
  AuthServices represent services that handle authentication and authorization.
---

AuthServices represent services that handle authentication and authorization. They support two distinct modes of operation:

### 1. Toolbox Native Authorization
Used for specific tools to enforce authorization or resolve parameters:
- [**Authorized Invocation**][auth-invoke]: A tool is validated by the auth service before it can be invoked. Toolbox will reject any calls that fail to validate or have an invalid token.
- [**Authenticated Parameters**][auth-params]: Replaces the value of a parameter with a field from an [OIDC][openid-claims] claim. Toolbox will automatically resolve the ID token provided by the client and replace the parameter in the tool call.

### 2. MCP Authorization
Used to secure the entire MCP server. The Model Context Protocol supports [MCP Authorization](https://modelcontextprotocol.io/docs/tutorials/security/authorization) to secure interactions between clients and servers. When enabled, all MCP endpoints require a valid token, and you can enforce granular tool-level scope authorization.

[openid-claims]: https://openid.net/specs/openid-connect-core-1_0.html#StandardClaims
[auth-invoke]: ../tools/_index.md#authorized-invocations-toolbox-native-authorization
[auth-params]: ../tools/_index.md#authenticated-parameters

## Example

The following configurations are placed at the top level of a `tools.yaml` file.

{{< notice tip >}}
If you are accessing Toolbox with multiple applications, each
 application should register their own Client ID even if they use the same
 "type" of auth provider.
{{< /notice >}}

```yaml
kind: authService
name: my_auth_app_1
type: google
clientId: ${YOUR_CLIENT_ID_1}
---
kind: authService
name: my_auth_app_2
type: google
clientId: ${YOUR_CLIENT_ID_2}
```

{{< notice tip >}}
Use environment variable replacement with the format ${ENV_NAME}
instead of hardcoding your secrets into the configuration file.
{{< /notice >}}

After you've configured an `authService`, you can use it:
- For **Toolbox Native Authorization** by referencing it in your tool configuration (using `authRequired` or `authService` in parameters).
- For **MCP Authorization** by setting `mcpEnabled: true` in the auth service configuration to secure the entire server.

## Specifying ID Tokens from Clients

After [configuring](#example) your `authService`, use a Toolbox SDK to
add your ID tokens to the header of a Tool invocation request. When specifying a
token you will provide a function (that returns an id). This function is called
when the tool is invoked. This allows you to cache and refresh the ID token as
needed.

The primary method for providing these getters is via the `auth_token_getters`
parameter when loading tools, or the `add_auth_token_getter`() /
`add_auth_token_getters()` methods on a loaded tool object.

### Specifying tokens during load

#### Python

Use the [Python SDK](https://github.com/googleapis/mcp-toolbox-sdk-python/tree/main).

{{< tabpane persist=header >}}
{{< tab header="Core" lang="Python" >}}
import asyncio
from toolbox_core import ToolboxClient

async def get_auth_token():
    # ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
    # This example just returns a placeholder. Replace with your actual token retrieval.
    return "YOUR_ID_TOKEN" # Placeholder

async def main():
    async with ToolboxClient("<http://127.0.0.1:5000>") as toolbox:
        auth_tool = await toolbox.load_tool(
            "get_sensitive_data",
            auth_token_getters={"my_auth_app_1": get_auth_token}
        )
        result = await auth_tool(param="value")
        print(result)

if **name** == "**main**":
    asyncio.run(main())
{{< /tab >}}
{{< tab header="LangChain" lang="Python" >}}
import asyncio
from toolbox_langchain import ToolboxClient

async def get_auth_token():
    # ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
    # This example just returns a placeholder. Replace with your actual token retrieval.
    return "YOUR_ID_TOKEN" # Placeholder

async def main():
    toolbox = ToolboxClient("<http://127.0.0.1:5000>")

    auth_tool = await toolbox.aload_tool(
        "get_sensitive_data",
        auth_token_getters={"my_auth_app_1": get_auth_token}
    )
    result = await auth_tool.ainvoke({"param": "value"})
    print(result)

if **name** == "**main**":
    asyncio.run(main())
{{< /tab >}}
{{< tab header="Llamaindex" lang="Python" >}}
import asyncio
from toolbox_llamaindex import ToolboxClient

async def get_auth_token():
    # ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
    # This example just returns a placeholder. Replace with your actual token retrieval.
    return "YOUR_ID_TOKEN" # Placeholder

async def main():
    toolbox = ToolboxClient("<http://127.0.0.1:5000>")

    auth_tool = await toolbox.aload_tool(
        "get_sensitive_data",
        auth_token_getters={"my_auth_app_1": get_auth_token}
    )
    # result = await auth_tool.acall(param="value")
    # print(result.content)

if **name** == "**main**":
    asyncio.run(main()){{< /tab >}}
{{< /tabpane >}}

#### Javascript/Typescript

Use the [JS SDK](https://github.com/googleapis/mcp-toolbox-sdk-js/tree/main).

```javascript
import { ToolboxClient } from '@toolbox-sdk/core';

async function getAuthToken() {
    // ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
    // This example just returns a placeholder. Replace with your actual token retrieval.
    return "YOUR_ID_TOKEN" // Placeholder
}

const URL = 'http://127.0.0.1:5000';
let client = new ToolboxClient(URL);
const authTool = await client.loadTool("my-tool", {"my_auth_app_1": getAuthToken});
const result = await authTool({param:"value"});
console.log(result);
print(result)
```

#### Go

Use the [Go SDK](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main).

```go
import "github.com/googleapis/mcp-toolbox-sdk-go/core"
import "fmt"

func getAuthToken() string {
  // ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
  // This example just returns a placeholder. Replace with your actual token retrieval.
  return "YOUR_ID_TOKEN" // Placeholder
}

func main() {
  URL := "http://127.0.0.1:5000"
  client, err := core.NewToolboxClient(URL)
  if err != nil {
    log.Fatalf("Failed to create Toolbox client: %v", err)
    }
  dynamicTokenSource := core.NewCustomTokenSource(getAuthToken)
  authTool, err := client.LoadTool(
    "my-tool",
    ctx,
    core.WithAuthTokenSource("my_auth_app_1", dynamicTokenSource))
  if err != nil {
    log.Fatalf("Failed to load tool: %v", err)
  }
  inputs := map[string]any{"param": "value"}
  result, err := authTool.Invoke(ctx, inputs)
  if err != nil {
    log.Fatalf("Failed to invoke tool: %v", err)
  }
  fmt.Println(result)
}
```

### Specifying tokens for existing tools

#### Python

Use the [Python
SDK](https://github.com/googleapis/mcp-toolbox-sdk-python/tree/main).

{{< tabpane persist=header >}}
{{< tab header="Core" lang="Python" >}}
tools = await toolbox.load_toolset()

# for a single token

authorized_tool = tools[0].add_auth_token_getter("my_auth", get_auth_token)

# OR, if multiple tokens are needed

authorized_tool = tools[0].add_auth_token_getters({
  "my_auth1": get_auth1_token,
  "my_auth2": get_auth2_token,
})
{{< /tab >}}
{{< tab header="LangChain" lang="Python" >}}
tools = toolbox.load_toolset()

# for a single token

authorized_tool = tools[0].add_auth_token_getter("my_auth", get_auth_token)

# OR, if multiple tokens are needed

authorized_tool = tools[0].add_auth_token_getters({
  "my_auth1": get_auth1_token,
  "my_auth2": get_auth2_token,
})
{{< /tab >}}
{{< tab header="Llamaindex" lang="Python" >}}
tools = toolbox.load_toolset()

# for a single token

authorized_tool = tools[0].add_auth_token_getter("my_auth", get_auth_token)

# OR, if multiple tokens are needed

authorized_tool = tools[0].add_auth_token_getters({
  "my_auth1": get_auth1_token,
  "my_auth2": get_auth2_token,
})
{{< /tab >}}
{{< /tabpane >}}

#### Javascript/Typescript

Use the [JS SDK](https://github.com/googleapis/mcp-toolbox-sdk-js/tree/main).

```javascript
const URL = 'http://127.0.0.1:5000';
let client = new ToolboxClient(URL);
let tool = await client.loadTool("my-tool")

// for a single token
const authorizedTool = tool.addAuthTokenGetter("my_auth", get_auth_token)

// OR, if multiple tokens are needed
const multiAuthTool = tool.addAuthTokenGetters({
    "my_auth_1": getAuthToken1,
    "my_auth_2": getAuthToken2,
})

```

#### Go

Use the [Go SDK](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main).

```go
import "github.com/googleapis/mcp-toolbox-sdk-go/core"

func main() {
  URL := "http://127.0.0.1:5000"
  client, err := core.NewToolboxClient(URL)
  if err != nil {
    log.Fatalf("Failed to create Toolbox client: %v", err)
  }
  tool, err := client.LoadTool("my-tool", ctx)
  if err != nil {
    log.Fatalf("Failed to load tool: %v", err)
  }
  dynamicTokenSource1 := core.NewCustomTokenSource(getAuthToken1)
  dynamicTokenSource2 := core.NewCustomTokenSource(getAuthToken1)

  // For a single token
  authTool, err := tool.ToolFrom(
    core.WithAuthTokenSource("my-auth", dynamicTokenSource1),
  )

  // OR, if multiple tokens are needed
  authTool, err := tool.ToolFrom(
    core.WithAuthTokenSource("my-auth_1", dynamicTokenSource1),
    core.WithAuthTokenSource("my-auth_2", dynamicTokenSource2),
  )
}
```

## Types of Auth Services
