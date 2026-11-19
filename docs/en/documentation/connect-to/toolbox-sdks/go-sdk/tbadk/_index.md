---
title: "ADK Package"
linkTitle: "ADK"
type: docs
weight: 1
description: >
  MCP Toolbox ADK for integrating functionalities of MCP Toolbox into your Agentic apps.
---

## Overview

The `tbadk` package provides a Go interface to the MCP Toolbox service, enabling you to load and invoke tools from your own applications.

## Installation

```bash
go get github.com/googleapis/mcp-toolbox-sdk-go/tbadk
```
This SDK is supported on Go version 1.24.4 and higher.

{{< notice note >}}
While the SDK itself is synchronous, you can execute its functions within goroutines to achieve asynchronous behavior.
{{< /notice >}}

{{< notice note >}}
**Breaking Change Notice**: As of version `0.6.0`, this repository has transitioned to a multi-module structure.
*   **For new versions (`v0.6.0`+)**: You must import specific modules (e.g., `go get github.com/googleapis/mcp-toolbox-sdk-go/tbadk`).
*   **For older versions (`v0.5.1` and below)**: The repository remains a single-module library (`go get github.com/googleapis/mcp-toolbox-sdk-go`).
*   Please update your imports and `go.mod` accordingly when upgrading.
{{< /notice >}}

## Quickstart

Here's a minimal example to get you started. Ensure your Toolbox service is
running and accessible.

```go
package main

import (
  "context"
  "fmt"
  "github.com/googleapis/mcp-toolbox-sdk-go/tbadk"
)

func quickstart() string {
  inputs := map[string]any{"location": "London"}
  client, err := tbadk.NewToolboxClient("http://localhost:5000")
  if err != nil {
    return fmt.Sprintln("Could not start Toolbox Client", err)
  }
  tool, err := client.LoadTool("get_weather", ctx)
  if err != nil {
    return fmt.Sprintln("Could not load Toolbox Tool", err)
  }
  // pass the tool.Context as ctx into the Run() method
  result, err := tool.Run(ctx, inputs)
  if err != nil {
    return fmt.Sprintln("Could not invoke tool", err)
  }
  return fmt.Sprintln(result["output"])
}

func main() {
  fmt.Println(quickstart())
}
```

## Usage

Import and initialize a Toolbox client, pointing it to the URL of your running
Toolbox service.

```go
import "github.com/googleapis/mcp-toolbox-sdk-go/tbadk"

client, err := tbadk.NewToolboxClient("http://localhost:5000")
```

All interactions for loading and invoking tools happen through this client.

{{< notice note >}}
For advanced use cases, you can provide an external custom `http.Client`
during initialization (e.g., `tbadk.NewToolboxClient(URL, core.WithHTTPClient(myClient)`). If you provide your own session, you are responsible for managing its lifecycle;
`ToolboxClient` *will not* close it.
{{< /notice >}}

{{< notice note >}}
If your connection URL contains query parameters (e.g., `http://localhost:5000?foo=bar`), the client automatically preserves them across API requests for parameter binding.
{{< /notice >}}

## Transport Protocols

The SDK supports multiple transport protocols for communicating with the Toolbox server. By default, the client uses the `2026-07-28` version of the **Model Context Protocol (MCP)**.

You can explicitly select a protocol using the `core.WithProtocol` option during client initialization. This is useful if you need to pin the client to a specific legacy version of MCP.

{{< notice note >}}
* **MCP Transports**: These options use the **Model Context Protocol over HTTP**.
{{< /notice >}}

### Supported Protocols

We currently support different versions of the MCP protocol.

| Constant | Description |
| :--- | :--- |
| `core.MCP` | **(Default)** Alias for the default MCP version (currently `2026-07-28`). |
| `core.MCPLatest` | Alias for the latest stable MCP version (currently `2026-07-28`). |
| `core.MCPDraft` | Alias for the upcoming draft MCP version (currently `2026-07-28`). |
| `core.MCPv20260728` | MCP Protocol version 2026-07-28. |
| `core.MCPv20251125` | MCP Protocol version 2025-11-25. |
| `core.MCPv20250618` | MCP Protocol version 2025-06-18. |
| `core.MCPv20250326` | MCP Protocol version 2025-03-26. |
| `core.MCPv20241105` | MCP Protocol version 2024-11-05. |

### Example

```go
import (
    "github.com/googleapis/mcp-toolbox-sdk-go/core"
    "github.com/googleapis/mcp-toolbox-sdk-go/tbadk"
)

// Initialize with the default MCP protocol (2026-07-28)
client, err := tbadk.NewToolboxClient(
    "http://localhost:5000",
)

// Initialize with the MCP Protocol 2025-03-26
client, err := tbadk.NewToolboxClient(
    "http://localhost:5000",
    core.WithProtocol(core.MCPv20250326),
)

```

## Loading Tools

You can load tools individually or in groups (toolsets) as defined in your
Toolbox service configuration. Loading a toolset is convenient when working with
multiple related functions, while loading a single tool offers more granular
control.

### Load a toolset

A toolset is a collection of related tools. You can load all tools in a toolset
or a specific one:

```go
// Load default toolset by providing an empty string as the name
tools, err := client.LoadToolset("", ctx)

// Load a specific toolset
tools, err := client.LoadToolset("my-toolset", ctx)
```

`LoadToolset` returns a slice of the ToolboxTool structs (`[]ToolboxTool`).


### Load a single tool

Loads a specific tool by its unique name. This provides fine-grained control.

```go
tool, err = client.LoadTool("my-tool", ctx)
```

## Invoking Tools

Once loaded, tools behave like Go structs. You invoke them using `Run` method
by passing arguments corresponding to the parameters defined in the tool's
configuration within the Toolbox service.

```go
tool, err = client.LoadTool("my-tool", ctx)
inputs := map[string]any{"location": "London"}
// Pass the tool.Context as ctx to the Run() function
result, err := tool.Run(ctx, inputs)
```

{{< notice tip >}}For a more comprehensive guide on setting up the Toolbox service itself, which
you'll need running to use this SDK, please refer to the [Toolbox Quickstart Guide](../../../../getting-started/local_quickstart_go.md).
{{< /notice >}}

## Client to Server Authentication

This section describes how to authenticate the ToolboxClient itself when
connecting to a Toolbox server instance that requires authentication. This is
crucial for securing your Toolbox server endpoint, especially when deployed on
platforms like Cloud Run, GKE,  or any environment where unauthenticated access is restricted.

This client-to-server authentication ensures that the Toolbox server can verify
the identity of the client making the request before any tool is loaded or
called. It is different from [Authenticating Tools](#authenticating-tools),
which deals with providing credentials for specific tools within an already
connected Toolbox session.

### When is Client-to-Server Authentication Needed?

You'll need this type of authentication if your Toolbox server is configured to
deny unauthenticated requests. For example:

- Your Toolbox server is deployed on Cloud Run and configured to "Require authentication."
- Your server is behind an Identity-Aware Proxy (IAP) or a similar
  authentication layer.
- You have custom authentication middleware on your self-hosted Toolbox server.

Without proper client authentication in these scenarios, attempts to connect or
make calls (like `LoadTool`) will likely fail with `Unauthorized` errors.

### How it works

The `ToolboxClient` allows you to specify TokenSources that dynamically generate HTTP headers for
every request sent to the Toolbox server. The most common use case is to add an
Authorization header with a bearer token (e.g., a Google ID token).

These header-generating functions are called just before each request, ensuring
that fresh credentials or header values can be used.

### Configuration

You can configure these dynamic headers as seen below:


```go
import (
  "github.com/googleapis/mcp-toolbox-sdk-go/core"
  "github.com/googleapis/mcp-toolbox-sdk-go/tbadk"
)

tokenProvider := func() string {
  return "header3_value"
}

staticTokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "header2_value"})
dynamicTokenSource := core.NewCustomTokenSource(tokenProvider)

client, err := tbadk.NewToolboxClient(
  "toolbox-url",
  core.WithClientHeaderString("header1", "header1_value"),
  core.WithClientHeaderTokenSource("header2", staticTokenSource),
  core.WithClientHeaderTokenSource("header3", dynamicTokenSource),
)
```

### Authenticating with Google Cloud Servers

For Toolbox servers hosted on Google Cloud (e.g., Cloud Run) and requiring
`Google ID token` authentication, the helper module
[auth_methods](https://github.com/googleapis/mcp-toolbox-sdk-go/blob/main/core/auth.go) provides utility functions.

### Step by Step Guide for Cloud Run

1. **Configure Permissions**: [Grant](https://cloud.google.com/run/docs/securing/managing-access#service-add-principals) the `roles/run.Runr` IAM role on the Cloud
   Run service to the principal. This could be your `user account email` or a
   `service account`.
2. **Configure Credentials**
    - Local Development: Set up
   [ADC](https://cloud.google.com/docs/authentication/set-up-adc-local-dev-environment).
    - Google Cloud Environments: When running within Google Cloud (e.g., Compute
      Engine, GKE, another Cloud Run service, Cloud Functions), ADC is typically
      configured automatically, using the environment's default service account.
3. **Connect to the Toolbox Server**
    ```go
    import (
      "github.com/googleapis/mcp-toolbox-sdk-go/core"
      "github.com/googleapis/mcp-toolbox-sdk-go/tbadk"
      "context"
    )

    ctx := context.Background()

    token, err := core.GetGoogleIDToken(ctx, URL)

    client, err := tbadk.NewToolboxClient(
      URL,
      core.WithClientHeaderString("Authorization", token),
    )

    // Now, you can use the client as usual.
    ```

## Authenticating Tools

{{< notice warning >}} **Always use HTTPS** to connect your application with the Toolbox service,
especially in **production environments** or whenever the communication
involves **sensitive data** (including scenarios where tools require
authentication tokens). Using plain HTTP lacks encryption and exposes your
application and data to significant security risks, such as eavesdropping and tampering.
{{< /notice >}}

Tools can be configured within the Toolbox service to require authentication,
ensuring only authorized users or applications can invoke them, especially when
accessing sensitive data.

### When is Authentication Needed?

Authentication is configured per-tool within the Toolbox service itself. If a
tool you intend to use is marked as requiring authentication in the service, you
must configure the SDK client to provide the necessary credentials (currently
Oauth2 tokens) when invoking that specific tool.

### Supported Authentication Mechanisms

The Toolbox service enables secure tool usage through **Authenticated Parameters**.
For detailed information on how these mechanisms work within the Toolbox service and how to configure them, please refer to [Toolbox Service Documentation - Authenticated Parameters](../../../../configuration/tools/_index.md#authenticated-parameters).

### Step 1: Configure Tools in Toolbox Service

First, ensure the target tool(s) are configured correctly in the Toolbox service
to require authentication. Refer to the [Toolbox Service Documentation -
Authenticated
Parameters](../../../../configuration/tools/_index.md#authenticated-parameters)
for instructions.

### Step 2: Configure SDK Client

Your application needs a way to obtain the required Oauth2 token for the
authenticated user. The SDK requires you to provide a function capable of
retrieving this token *when the tool is invoked*.

#### Provide an ID Token Retriever Function

You must provide the SDK with a function that returns the
necessary token when called. The implementation depends on your application's
authentication flow (e.g., retrieving a stored token, initiating an OAuth flow).

{{< notice info >}}
The name used when registering the getter function with the SDK (e.g.,
`"my_api_token"`) must exactly match the `name` of the corresponding
`authService` defined in the tool's configuration within the Toolbox service.
{{< /notice >}}

```go
func getAuthToken() string {
  // ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
  // This example just returns a placeholder. Replace with your actual token retrieval.
  return "YOUR_ID_TOKEN" // Placeholder
}
```

{{< notice tip >}} Your token retriever function is invoked every time an authenticated parameter
requires a token for a tool call. Consider implementing caching logic within
this function to avoid redundant token fetching or generation, especially for
tokens with longer validity periods or if the retrieval process is resource-intensive.
{{< /notice >}}

#### Option A: Add Default Authentication to a Client

You can add default tool level authentication to a client.
Every tool / toolset loaded by the client will contain the auth token.

```go

ctx := context.Background()

client, err := tbadk.NewToolboxClient("http://127.0.0.1:5000",
  core.WithDefaultToolOptions(
    core.WithAuthTokenString("my-auth-1", "auth-value"),
  ),
)

AuthTool, err := client.LoadTool("my-tool", ctx)
```

#### Option B: Add Authentication to a Loaded Tool

You can add the token retriever function to a tool object *after* it has been
loaded. This modifies the specific tool instance.

```go

ctx := context.Background()

client, err := tbadk.NewToolboxClient("http://127.0.0.1:5000")

tool, err := client.LoadTool("my-tool", ctx)

AuthTool, err := tool.ToolFrom(
  core.WithAuthTokenSource("my-auth", headerTokenSource),
  core.WithAuthTokenString("my-auth-1", "value"),
  )
```

#### Option C: Add Authentication While Loading Tools

You can provide the token retriever(s) directly during the `LoadTool` or
`LoadToolset` calls. This applies the authentication configuration only to the
tools loaded in that specific call, without modifying the original tool objects
if they were loaded previously.

```go
AuthTool, err := client.LoadTool("my-tool", ctx, core.WithAuthTokenString("my-auth-1", "value"))

// or

AuthTools, err := client.LoadToolset(
  "my-toolset",
  ctx,
  core.WithAuthTokenString("my-auth-1", "value"),
)
```

{{< notice note >}}
Adding auth tokens during loading only affect the tools loaded within that call.
{{< /notice >}}

### Complete Authentication Example

```go
import "github.com/googleapis/mcp-toolbox-sdk-go/core"
import "fmt"

func getAuthToken() string {
  // ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
  // This example just returns a placeholder. Replace with your actual token retrieval.
  return "YOUR_ID_TOKEN" // Placeholder
}

func main() {
  ctx := context.Background()
  inputs := map[string]any{"input": "some input"}

  dynamicTokenSource := core.NewCustomTokenSource(getAuthToken)

  client, err := tbadk.NewToolboxClient("http://127.0.0.1:5000")
  tool, err := client.LoadTool("my-tool", ctx)
  AuthTool, err := tool.ToolFrom(core.WithAuthTokenSource("my_auth", dynamicTokenSource))

  result, err := AuthTool.Run(ctx, inputs)

  fmt.Println(result)
}
```

{{< notice note >}}An auth token getter for a specific name (e.g., "GOOGLE_ID") will replace any client header with the same name followed by "_token" (e.g.,"GOOGLE_ID_token").
{{< /notice >}}

## Binding Parameter Values

The SDK allows you to pre-set, or "bind", values for specific tool parameters
before the tool is invoked or even passed to an LLM. These bound values are
fixed and will not be requested or modified by the LLM during tool use.

### Why Bind Parameters?

- **Protecting sensitive information:**  API keys, secrets, etc.
- **Enforcing consistency:** Ensuring specific values for certain parameters.
- **Pre-filling known data:**  Providing defaults or context.

{{< notice info >}}
The parameter names used for binding (e.g., `"api_key"`) must exactly match the
parameter names defined in the tool's configuration within the Toolbox service.
{{< /notice >}}

{{< notice note >}}
You do not need to modify the tool's configuration in the Toolbox service to bind parameter values using the SDK.
{{< /notice >}}

#### Option A: Add Default Bound Parameters to a Client

You can add default tool level bound parameters to a client. Every tool / toolset
loaded by the client will have the bound parameter.

```go

ctx := context.Background()

client, err := tbadk.NewToolboxClient("http://127.0.0.1:5000",
  core.WithDefaultToolOptions(
    core.WithBindParamString("param1", "value"),
  ),
)

boundTool, err := client.LoadTool("my-tool", ctx)
```

### Option B: Binding Parameters to a Loaded Tool

Bind values to a tool object *after* it has been loaded. This modifies the
specific tool instance.

```go
client, err := tbadk.NewToolboxClient("http://127.0.0.1:5000")

tool, err := client.LoadTool("my-tool", ctx)

boundTool, err := tool.ToolFrom(
  core.WithBindParamString("param1", "value"),
  core.WithBindParamString("param2", "value")
  )
```

### Option C: Binding Parameters While Loading Tools

Specify bound parameters directly when loading tools. This applies the binding
only to the tools loaded in that specific call.

```go
boundTool, err := client.LoadTool("my-tool", ctx, core.WithBindParamString("param", "value"))

// OR

boundTool, err := client.LoadToolset("", ctx, core.WithBindParamString("param", "value"))
```

{{< notice note >}} 
Bound values during loading only affect the tools loaded in that call.
{{< /notice >}}

### Binding Dynamic Values

Instead of a static value, you can bind a parameter to a synchronous or
asynchronous function. This function will be called *each time* the tool is
invoked to dynamically determine the parameter's value at runtime.
Functions with the return type (data_type, error) can be provided.

```go
getDynamicValue := func() (string, error) { return "req-123", nil }

dynamicBoundTool, err := tool.ToolFrom(core.WithBindParamStringFunc("param", getDynamicValue))
```

{{< notice info >}} 
You don't need to modify tool configurations to bind parameter values.
{{< /notice >}}

## Secure Parameters

{{< notice note >}}
Secure parameters are supported starting in [`github.com/googleapis/mcp-toolbox-sdk-go/tbadk`](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main/tbadk) version `v1.2.0` (with `core` >= `v1.2.0`) and require MCP [protocol version `2026-07-28`](#supported-protocols) or newer with the [`com.google.cloud/toolbox.v1` extension](https://github.com/googleapis/mcp-toolbox/blob/main/extensions/2026-07-28/README.md). For server configuration details, see [Secure Parameters](../../../../configuration/tools/_index.md#secure-parameters).
{{< /notice >}}

Secure parameters are designed for sensitive runtime values (such as an end-user `customer_id`, tenant identifier, or secret tokens) that LLMs must not see, control, or hallucinate.

* **Schema Isolation:** When `ToolboxTool` is converted to an ADK tool or registered with an ADK agent, secure parameters are completely stripped from `tool.Declaration()`. The model never sees them, preventing credential exposure and prompt leakage.
* **Prompt Injection Defense:** If a model attempts to supply arguments containing a secure parameter name, execution fails immediately.
* **Fast-Fail Validation:** Missing required secure parameters are validated locally before invocation, preventing unwanted network calls.
* **Binding Methods:** Secure parameters can be set as client defaults, bound when loading tools, or bound on existing tool instances using `core.WithBindSecureParam*` options:

```go
import (
    "context"
    "github.com/googleapis/mcp-toolbox-sdk-go/core"
    "github.com/googleapis/mcp-toolbox-sdk-go/tbadk"
)

ctx := context.Background()

// Option A: Set default secure parameters across all tools loaded by the client
client, err := tbadk.NewToolboxClient("http://127.0.0.1:5000",
    core.WithDefaultToolOptions(
        core.WithBindSecureParamString("customer_id", "cust_12345"),
    ),
)

// Option B: Bind secure parameters when loading a tool or toolset
boundTool, err := client.LoadTool("search_secure_data", ctx,
    core.WithBindSecureParamString("customer_id", "cust_12345"),
)
tools, err := client.LoadToolset("my-toolset", ctx,
    core.WithBindSecureParamString("customer_id", "cust_12345"),
)

// Option C: Bind to an un-bound loaded tool (returns a new immutable tool)
rawTool, err := client.LoadTool("search_secure_data", ctx)
postBoundTool, err := rawTool.ToolFrom(
    core.WithBindSecureParamString("customer_id", "cust_12345"),
)

// Option D: Dynamic runtime resolution
dynamicTool, err := rawTool.ToolFrom(
    core.WithBindSecureParamStringFunc("auth_token", func() (string, error) {
        return fetchSessionToken(), nil
    }),
)
```

### Cross-Binding Guidance & Mutual Exclusivity

* Passing `core.WithBindParam*` for a parameter configured with `secure: true` returns:  
  `parameter "<name>" is a secure parameter; use WithBindSecureParam* instead`
* Passing `core.WithBindSecureParam*` for a regular parameter returns:  
  `parameter "<name>" is a regular parameter; use WithBindParam* instead`


## Default Parameters

Tools defined in the MCP Toolbox server can specify default values for their optional parameters. When invoking a tool using the SDK, if an input for a parameter with a default value is not provided, the SDK will automatically populate the request with the default value.

```go
tool, err = client.LoadTool("my-tool", ctx)

// If 'my-tool' has a parameter 'param2' with a default value of "default-value",
// we can omit 'param2' from the inputs.
inputs := map[string]any{"param1": "value"}

// The invocation will automatically use param2="default-value" if not provided
result, err := tool.Run(ctx, inputs)
```

## Using with ADK Go

After altering the tool to your needs, type-assert the ToolboxTool and pass it to the LLM agent.

### For a single tool

```go

toolboxtool, err = client.LoadTool("my-tool", ctx)

// <Bind parameters & add authentication here>

llmagent, err := llmagent.New(llmagent.Config{
  Name:        "assistant",
  Model:       model,
  Description: "Agent to answer questions.",
  Tools:       []tool.Tool{&toolboxtool},
})
```

### For a toolset

```go
toolboxtools, err := client.LoadToolset("", ctx)

// <Bind parameters & add authentication here>

toolsList := make([]tool.Tool, len(toolboxtools))
	for i := range toolboxtools {
		toolsList[i] = &toolboxtools[i]
	}

llmagent, err := llmagent.New(llmagent.Config{
  Name:        "assistant",
  Model:       model,
  Description: "Agent to answer questions.",
  Tools:       toolsList,
})

```

The reason we have to type assert it before passing it to ADK Go, is because it requires a generic `tool.Tool` interface. You can always convert it back to `ToolboxTool` format to access the specialized  methods.

# Using with Orchestration Frameworks

To see how the MCP Toolbox Go SDK works with orchestration frameworks, check out these end-to-end examples given below.

<details>
<summary>ADK Go</summary>

```go
//This sample contains a complete example on how to integrate MCP Toolbox Go SDK with ADK Go using the tbadk package.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/googleapis/mcp-toolbox-sdk-go/tbadk"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/genai"
)

func main() {
	genaiKey := os.Getenv("GEMINI_API_KEY")
	toolboxURL := "http://localhost:5000"
	ctx := context.Background()

	// Initialize MCP Toolbox client
	toolboxClient, err := tbadk.NewToolboxClient(toolboxURL)
	if err != nil {
		log.Fatalf("Failed to create MCP Toolbox client: %v", err)
	}

	toolsetName := "my-toolset"
	toolset, err := toolboxClient.LoadToolset(toolsetName, ctx)
	if err != nil {
		log.Fatalf("Failed to load MCP toolset '%s': %v\nMake sure your Toolbox server is running.", toolsetName, err)
	}

	// Create Gemini model
	model, err := gemini.NewModel(ctx, "gemini-2.5-flash", &genai.ClientConfig{
		APIKey: genaiKey,
	})
	if err != nil {
		log.Fatalf("Failed to create model: %v", err)
	}

	tools := make([]tool.Tool, len(toolset))
	for i := range toolset {
		tools[i] = &toolset[i]
	}

	llmagent, err := llmagent.New(llmagent.Config{
		Name:        "hotel_assistant",
		Model:       model,
		Description: "Agent to answer questions about hotels.",
		Tools:       tools,
	})
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}

	appName := "hotel_assistant"
	userID := "user-123"

	sessionService := session.InMemoryService()
	resp, err := sessionService.Create(ctx, &session.CreateRequest{
		AppName: appName,
		UserID:  userID,
	})
	if err != nil {
		log.Fatalf("Failed to create the session service: %v", err)
	}
	session := resp.Session

	r, err := runner.New(runner.Config{
		AppName:        appName,
		Agent:          llmagent,
		SessionService: sessionService,
	})
	if err != nil {
		log.Fatalf("Failed to create runner: %v", err)
	}

	query := "Find hotels with Basel in its name."

	fmt.Println(query)
	userMsg := genai.NewContentFromText(query, genai.RoleUser)

	streamingMode := agent.StreamingModeSSE
	for event, err := range r.Run(ctx, userID, session.ID(), userMsg, agent.RunConfig{
		StreamingMode: streamingMode,
	}) {
		if err != nil {
			fmt.Printf("\nAGENT_ERROR: %v\n", err)
		} else {
			if event.LLMResponse.Content != nil {
				for _, p := range event.LLMResponse.Content.Parts {
					if streamingMode != agent.StreamingModeSSE || event.LLMResponse.Partial {
						fmt.Print(p.Text)
					}
				}
			}
		}
	}
	fmt.Println()
}
```

</details>
