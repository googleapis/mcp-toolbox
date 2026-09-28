---
title: "ADK"
type: docs
weight: 1
description: >
  MCP Toolbox SDK for integrating functionalities of MCP Toolbox into your ADK apps.
---

## Overview

The `@toolbox-sdk/adk` package provides a Javascript interface to the MCP Toolbox service, enabling you to load and invoke tools from your own applications.

## Supported Environments

This SDK is a standard Node.js package built with TypeScript, ensuring broad compatibility with the modern JavaScript ecosystem.

- Node.js: Actively supported on Node.js v18.x and higher. The package is compatible with both modern ES Module (import) and legacy CommonJS (require).
- TypeScript: The SDK is written in TypeScript and ships with its own type declarations, providing a first-class development experience with autocompletion and type-checking out of the box.
- JavaScript: Fully supports modern JavaScript in Node.js environments.

## Installation

```bash
npm install @toolbox-sdk/adk
```

## Quickstart


1. **Start the Toolbox Service**
   - Make sure the MCP Toolbox service is running. See the [Toolbox Getting Started Guide](../../../../introduction/_index.md#getting-started).

2. **Minimal Example**

Here's a minimal example to get you started. Ensure your Toolbox service is running and accessible.

```javascript

import { ToolboxClient } from '@toolbox-sdk/adk';
const URL = 'http://127.0.0.1:5000'; // Replace with your Toolbox service URL
const client = new ToolboxClient(URL);

async function quickstart() {  
  try {  
      const tools = await client.loadToolset();  
      // Use tools  
  } catch (error) {  
      console.error("unable to load toolset:", error.message);  
  }  
}  
quickstart();  
```

{{< notice note>}}
 This guide uses modern ES Module (`import`) syntax. If your project uses CommonJS, you can import the library using require: `const { ToolboxClient } = require('@toolbox-sdk/adk')`;.
{{< /notice >}}

## Usage

Import and initialize a Toolbox client, pointing it to the URL of your running Toolbox service.

```javascript
import { ToolboxClient } from '@toolbox-sdk/adk';

// Replace with the actual URL where your Toolbox service is running
const URL = 'http://127.0.0.1:5000';

let client = new ToolboxClient(URL);
const tools = await client.loadToolset();

// Use the client and tools as per requirement
```

All interactions for loading and invoking tools happen through this client.

{{< notice note>}}
Closing the `ToolboxClient` also closes the underlying network session shared by all tools loaded from that client. As a result, any tool instances you have loaded will cease to function and will raise an error if you attempt to invoke them after the client is closed.
{{< /notice >}}

{{< notice note>}}
For advanced use cases, you can provide an external `AxiosInstance` during initialization (e.g., `ToolboxClient(url, my_session)`).
{{< /notice >}}

## Transport Protocols

The SDK supports multiple transport protocols to communicate with the Toolbox server. You can specify the protocol version during client initialization.

### Available Protocols

We currently support different versions of the MCP protocol.

- `Protocol.MCP`: The default protocol version (currently aliases to `MCP_v20260728`).
- `Protocol.MCP_LATEST`: Alias for the latest stable MCP version (currently aliases to `MCP_v20260728`).
- `Protocol.MCP_DRAFT`: Alias for the upcoming draft MCP version (currently aliases to `MCP_v20260728`).
- `Protocol.MCP_v20260728`: July 2026 version (2026-07-28).
- `Protocol.MCP_v20251125`: November 2025 version.
- `Protocol.MCP_v20250618`: June 2025 version.
- `Protocol.MCP_v20250326`: March 2025 version.
- `Protocol.MCP_v20241105`: Use this for compatibility with older MCP servers (November 2024 version).

### Specifying a Protocol

You can explicitly set the protocol by passing the `protocol` argument to the `ToolboxClient` constructor.

```javascript
import { ToolboxClient, Protocol } from '@toolbox-sdk/adk';

const URL = 'http://127.0.0.1:5000';

// Initialize with a specific protocol version
const client = new ToolboxClient(URL, null, null, Protocol.MCP_v20241105);

const tools = await client.loadToolset();
```

## Loading Tools

You can load tools individually or in groups (toolsets) as defined in your Toolbox service configuration. Loading a toolset is convenient when working with multiple related functions, while loading a single tool offers more granular control.

### Load a toolset

A toolset is a collection of related tools. You can load all tools in a toolset or a specific one:

```javascript
// Load all tools
const tools = await toolbox.loadToolset()

// Load a specific toolset
const tools = await toolbox.loadToolset("my-toolset")
```

### Load a single tool

Loads a specific tool by its unique name. This provides fine-grained control.

```javascript
const tool = await toolbox.loadTool("my-tool")
```

## Invoking Tools

Once loaded, tools behave like awaitable JS functions. You invoke them using `await` and pass arguments corresponding to the parameters defined in the tool's configuration within the Toolbox service.

```javascript
const tool = await client.loadTool("my-tool")
const result = await tool.runAsync(args: {a: 5, b: 2})
```

{{< notice tip>}}
For a more comprehensive guide on setting up the Toolbox service itself, which you'll need running to use this SDK, please refer to the [Toolbox Quickstart Guide](../../../../getting-started/local_quickstart_js.md).
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
make calls (like `load_tool`) will likely fail with `Unauthorized` errors.

### How it works

The `ToolboxClient` allows you to specify functions that dynamically generate
HTTP headers for every request sent to the Toolbox server. The most common use
case is to add an [Authorization
header](https://developer.mozilla.org/en-US/docs/Web/HTTP/Reference/Headers/Authorization)
with a bearer token (e.g., a Google ID token).

These header-generating functions are called just before each request, ensuring
that fresh credentials or header values can be used.

### Configuration

You can configure these dynamic headers as seen below:

```javascript
import { ToolboxClient } from '@toolbox-sdk/adk';
import {getGoogleIdToken} from '@toolbox-sdk/core/auth'

const URL = 'http://127.0.0.1:5000';
const getGoogleIdTokenGetter = () => getGoogleIdToken(URL);
const client = new ToolboxClient(URL, null, {"Authorization": getGoogleIdTokenGetter});

// Use the client as usual
```

### Authenticating with Google Cloud Servers

For Toolbox servers hosted on Google Cloud (e.g., Cloud Run) and requiring
`Google ID token` authentication, the helper module
[auth_methods](https://github.com/googleapis/mcp-toolbox-sdk-js/blob/main/packages/toolbox-core/src/toolbox_core/authMethods.ts) provides utility functions.

### Step by Step Guide for Cloud Run

1. **Configure Permissions**: [Grant](https://cloud.google.com/run/docs/securing/managing-access#service-add-principals) the `roles/run.invoker` IAM role on the Cloud
   Run service to the principal. This could be your `user account email` or a
   `service account`.
2. **Configure Credentials**
    - Local Development: Set up
   [ADC](https://cloud.google.com/docs/authentication/set-up-adc-local-dev-environment).
    - Google Cloud Environments: When running within Google Cloud (e.g., Compute
      Engine, GKE, another Cloud Run service, Cloud Functions), ADC is typically
      configured automatically, using the environment's default service account.
3. **Connect to the Toolbox Server**

    ```javascript
    import { ToolboxClient } from '@toolbox-sdk/adk';
    import {getGoogleIdToken} from '@toolbox-sdk/core/auth'

    const URL = 'http://127.0.0.1:5000';
    const getGoogleIdTokenGetter = () => getGoogleIdToken(URL);
    const client = new ToolboxClient(URL, null, {"Authorization": getGoogleIdTokenGetter});

    // Use the client as usual
    ```

## Authenticating Tools

{{< notice note>}}
**Always use HTTPS** to connect your application with the Toolbox service, especially in **production environments** or whenever the communication involves **sensitive data** (including scenarios where tools require authentication tokens). Using plain HTTP lacks encryption and exposes your application and data to significant security risks, such as eavesdropping and tampering.
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

The Toolbox service enables secure tool usage through **Authenticated Parameters**. For detailed information on how these mechanisms work within the Toolbox service and how to configure them, please refer to [Toolbox Service Documentation - Authenticated Parameters](../../../../configuration/tools/_index.md#authenticated-parameters)

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

You must provide the SDK with a function (sync or async) that returns the
necessary token when called. The implementation depends on your application's
authentication flow (e.g., retrieving a stored token, initiating an OAuth flow).

{{< notice note>}}
The name used when registering the getter function with the SDK (e.g., `"my_api_token"`) must exactly match the `name` of the corresponding `authService` defined in the tool's configuration within the Toolbox service.
{{< /notice >}}

```javascript

async function getAuthToken() {
    // ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
    // This example just returns a placeholder. Replace with your actual token retrieval.
    return "YOUR_ID_TOKEN" // Placeholder
}    
```
{{< notice tip>}}
Your token retriever function is invoked every time an authenticated parameter requires a token for a tool call. Consider implementing caching logic within this function to avoid redundant token fetching or generation, especially for tokens with longer validity periods or if the retrieval process is resource-intensive.
{{< /notice >}}

#### Option A: Add Authentication to a Loaded Tool

You can add the token retriever function to a tool object *after* it has been
loaded. This modifies the specific tool instance.

```javascript
const URL = 'http://127.0.0.1:5000';
let client = new ToolboxClient(URL);
let tool = await client.loadTool("my-tool")

const authTool = tool.addAuthTokenGetter("my_auth", get_auth_token)  // Single token

// OR

const multiAuthTool = tool.addAuthTokenGetters({
    "my_auth_1": getAuthToken1,
    "my_auth_2": getAuthToken2,
})  // Multiple tokens
```

#### Option B: Add Authentication While Loading Tools

You can provide the token retriever(s) directly during the `loadTool` or
`loadToolset` calls. This applies the authentication configuration only to the
tools loaded in that specific call, without modifying the original tool objects
if they were loaded previously.

```javascript
const authTool = await toolbox.loadTool("toolName", {"myAuth": getAuthToken})

// OR

const authTools = await toolbox.loadToolset({"myAuth": getAuthToken})
```

{{< notice note>}}
Adding auth tokens during loading only affect the tools loaded within that call.
{{< /notice >}}

### Complete Authentication Example

```javascript
import { ToolboxClient } from '@toolbox-sdk/adk';

async function getAuthToken() {
    // ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
    // This example just returns a placeholder. Replace with your actual token retrieval.
    return "YOUR_ID_TOKEN" // Placeholder
}

const URL = 'http://127.0.0.1:5000';
let client = new ToolboxClient(URL);
const tool = await client.loadTool("my-tool");
const authTool = tool.addAuthTokenGetters({"my_auth": getAuthToken});
const result = await authTool.runAsync(args: {input:"some input"});
console.log(result);
```

## Binding Parameter Values

The SDK allows you to pre-set, or "bind", values for specific tool parameters
before the tool is invoked or even passed to an LLM. These bound values are
fixed and will not be requested or modified by the LLM during tool use.

### Why Bind Parameters?

- **Protecting sensitive information:**  API keys, secrets, etc.
- **Enforcing consistency:** Ensuring specific values for certain parameters.
- **Pre-filling known data:**  Providing defaults or context.

{{< notice note>}}
The parameter names used for binding (e.g., `"api_key"`) must exactly match the parameter names defined in the tool's configuration within the Toolbox service.
{{< /notice >}}

{{< notice note>}}
You do not need to modify the tool's configuration in the Toolbox service to
> bind parameter values using the SDK.
{{< /notice >}}

### Option A: Binding Parameters to a Loaded Tool

Bind values to a tool object *after* it has been loaded. This modifies the
specific tool instance.

```javascript

import { ToolboxClient } from '@toolbox-sdk/adk';

const URL = 'http://127.0.0.1:5000';
let client = new ToolboxClient(URL);
const tool = await client.loadTool("my-tool");

const boundTool = tool.bindParam("param", "value");

// OR

const boundTool = tool.bindParams({"param": "value"});
```

### Option B: Binding Parameters While Loading Tools

Specify bound parameters directly when loading tools. This applies the binding
only to the tools loaded in that specific call.

```javascript
const boundTool = await client.loadTool("my-tool", null, {"param": "value"})

// OR

const boundTools = await client.loadToolset(null, {"param": "value"})
```

{{< notice note>}}
Bound values during loading only affect the tools loaded in that call.
{{< /notice >}}

### Binding Dynamic Values

Instead of a static value, you can bind a parameter to a synchronous or
asynchronous function. This function will be called *each time* the tool is
invoked to dynamically determine the parameter's value at runtime.

```javascript

async function getDynamicValue() {
    // Logic to determine the value
    return "dynamicValue";
}

const dynamicBoundTool = tool.bindParam("param", getDynamicValue)
```

{{< notice note>}}
You don't need to modify tool configurations to bind parameter values.
{{< /notice >}}

## Secure Parameters

{{< notice note >}}
Secure parameters are supported starting in `@toolbox-sdk/adk` version `1.2.0` (with `@toolbox-sdk/core` >= `1.2.0`) and require MCP protocol version `2026-07-28` or newer with the `com.google.cloud/toolbox.v1` extension. For server configuration details, see [Secure Parameters](../../../../configuration/tools/_index.md#secure-parameters).
{{< /notice >}}

Secure parameters are designed for sensitive runtime values (such as an end-user `customer_id`, tenant identifier, or secret tokens) that LLMs must not see or control.

* **Schema Isolation:** When loaded tools are passed to an ADK `LlmAgent`, secure parameters are completely omitted from the tool parameter declarations (`tool.declaration`), ensuring the LLM never prompts for or hallucinates them.
* **Prompt Injection Defense:** If an agent attempts to call a tool with a secure parameter supplied in standard arguments, execution fails immediately.
* **Fast-Fail Validation:** Missing required secure parameters are validated locally before invocation, preventing unwanted network calls.
* **Binding Methods:** Secure parameters can be pre-bound when loading tools or bound to tool instances:

```javascript
import { ToolboxClient } from '@toolbox-sdk/adk';

const client = new ToolboxClient("http://127.0.0.1:5000");

// Option A: Pre-bind secure parameters during loadTool / loadToolset
const boundTool = await client.loadTool(
    "search_secure_data",
    null, // authTokenGetters
    null, // boundParams
    { customer_id: "cust_12345" } // secureParams
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

// Option C: Dynamic callable (evaluated per invocation)
const dynamicTool = rawTool.bindSecureParam("auth_token", async () => fetchAuthToken());
```

### Cross-Binding Guidance & Mutual Exclusivity

* Calling `tool.bindParam()` on a secure parameter throws an error:  
  `Error: parameter '<name>' is a secure parameter; use bindSecureParam/bindSecureParams instead`
* Calling `tool.bindSecureParam()` on a regular parameter throws an error:  
  `Error: parameter '<name>' is a regular parameter; use bindParam/bindParams instead`

### Accessing Core Tool

To inspect or manipulate the underlying `@toolbox-sdk/core` tool instance from an ADK tool, call `tool.getCoreTool()`.


# Using with ADK

ADK JS:

```javascript
import {FunctionTool, InMemoryRunner, LlmAgent} from '@google/adk';
import {Content} from '@google/genai';
import {ToolboxClient} from '@toolbox-sdk/core'

const toolboxClient = new ToolboxClient("http://127.0.0.1:5000");
const loadedTools = await toolboxClient.loadToolset();

export const rootAgent = new LlmAgent({
  name: 'weather_time_agent',
  model: 'gemini-3-flash-preview',
  description:
    'Agent to answer questions about the time and weather in a city.',
  instruction:
    'You are a helpful agent who can answer user questions about the time and weather in a city.',
  tools: loadedTools,
});

async function main() {
  const userId = 'test_user';
  const appName = rootAgent.name;
  const runner = new InMemoryRunner({agent: rootAgent, appName});
  const session = await runner.sessionService.createSession({
    appName,
    userId,
  });

  const prompt = 'What is the weather in New York? And the time?';
  const content: Content = {
    role: 'user',
    parts: [{text: prompt}],
  };
  console.log(content);
  for await (const e of runner.runAsync({
    userId,
    sessionId: session.id,
    newMessage: content,
  })) {
    if (e.content?.parts?.[0]?.text) {
      console.log(`${e.author}: ${JSON.stringify(e.content, null, 2)}`);
    }
  }
}

main().catch(console.error);
```
