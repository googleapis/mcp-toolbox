---
title: "LangChain/LangGraph"
type: docs
weight: 3
description: >
  MCP Toolbox SDK for integrating functionalities of MCP Toolbox into your LangChain/LangGraph apps.
---

## Overview

The `toolbox-langchain` package provides a Python interface to the MCP Toolbox service, enabling you to load and invoke tools from your own applications.

## Installation

```bash
pip install toolbox-langchain
```
## Quickstart

Here's a minimal example to get you started using
[LangGraph](https://langchain-ai.github.io/langgraph/reference/prebuilt/#langgraph.prebuilt.chat_agent_executor.create_react_agent):

```py
from toolbox_langchain import ToolboxClient
from langchain_google_vertexai import ChatVertexAI
from langgraph.prebuilt import create_react_agent

async with ToolboxClient("http://127.0.0.1:5000") as toolbox:
    tools = toolbox.load_toolset()

    model = ChatVertexAI(model="gemini-3-flash-preview")
    agent = create_react_agent(model, tools)

    prompt = "How's the weather today?"

    for s in agent.stream({"messages": [("user", prompt)]}, stream_mode="values"):
        message = s["messages"][-1]
        if isinstance(message, tuple):
            print(message)
        else:
            message.pretty_print()
```
{{< notice tip >}}
For a complete, end-to-end example including setting up the service and using an SDK, see the full tutorial: [Toolbox Quickstart Tutorial](../../../../getting-started/local_quickstart.md)
{{< /notice >}}

## Usage

Import and initialize the toolbox client.

```py
from toolbox_langchain import ToolboxClient

# Replace with your Toolbox service's URL
async with ToolboxClient("http://127.0.0.1:5000") as toolbox:
```

## Transport Protocols

The SDK supports multiple transport protocols for communicating with the Toolbox server. By default, the client uses the latest supported version of the **Model Context Protocol (MCP)**.

You can explicitly select a protocol using the `protocol` option during client initialization. This is useful if you need to use the native Toolbox HTTP protocol or pin the client to a specific legacy version of MCP.

{{< notice note >}}
* **MCP Transports**: These options use the **Model Context Protocol over HTTP**.
{{< /notice >}}

### Supported Protocols

We currently support different versions of the MCP protocol.

| Constant | Description |
| :--- | :--- |
| `Protocol.MCP` | **(Default)** Alias for the default MCP version (currently `2026-07-28`). |
| `Protocol.MCP_LATEST` | Alias for the latest stable MCP version (currently `2026-07-28`). |
| `Protocol.MCP_DRAFT` | Alias for the upcoming draft MCP version (currently `2026-07-28`). |
| `Protocol.MCP_v20260728` | MCP Protocol version 2026-07-28. |
| `Protocol.MCP_v20251125` | MCP Protocol version 2025-11-25. |
| `Protocol.MCP_v20250618` | MCP Protocol version 2025-06-18. |
| `Protocol.MCP_v20250326` | MCP Protocol version 2025-03-26. |
| `Protocol.MCP_v20241105` | MCP Protocol version 2024-11-05. |

### Example

```py
from toolbox_langchain import ToolboxClient
from toolbox_core.protocol import Protocol

async with ToolboxClient("http://127.0.0.1:5000", protocol=Protocol.MCP) as toolbox:
    # Use client
    pass
```

If you want to pin the MCP Version 2025-03-26:

```py
from toolbox_langchain import ToolboxClient
from toolbox_core.protocol import Protocol

async with ToolboxClient("http://127.0.0.1:5000", protocol=Protocol.MCP_v20250326) as toolbox:
    # Use client
    pass
```

## Loading Tools

### Load a toolset

A toolset is a collection of related tools. You can load all tools in a toolset
or a specific one:

```py
# Load all tools
tools = toolbox.load_toolset()

# Load a specific toolset
tools = toolbox.load_toolset("my-toolset")
```

### Load a single tool

```py
tool = toolbox.load_tool("my-tool")
```

Loading individual tools gives you finer-grained control over which tools are
available to your LLM agent.

## Use with LangChain

LangChain's agents can dynamically choose and execute tools based on the user
input. Include tools loaded from the Toolbox SDK in the agent's toolkit:

```py
from langchain_google_vertexai import ChatVertexAI

model = ChatVertexAI(model="gemini-3-flash-preview")

# Initialize agent with tools
agent = model.bind_tools(tools)

# Run the agent
result = agent.invoke("Do something with the tools")
```

## Use with LangGraph

Integrate the Toolbox SDK with LangGraph to use Toolbox service tools within a
graph-based workflow. Follow the [official
guide](https://langchain-ai.github.io/langgraph/) with minimal changes.

### Represent Tools as Nodes

Represent each tool as a LangGraph node, encapsulating the tool's execution within the node's functionality:

```py
from toolbox_langchain import ToolboxClient
from langgraph.graph import StateGraph, MessagesState
from langgraph.prebuilt import ToolNode

# Define the function that calls the model
def call_model(state: MessagesState):
    messages = state['messages']
    response = model.invoke(messages)
    return {"messages": [response]}  # Return a list to add to existing messages

model = ChatVertexAI(model="gemini-3-flash-preview")
builder = StateGraph(MessagesState)
tool_node = ToolNode(tools)

builder.add_node("agent", call_model)
builder.add_node("tools", tool_node)
```

### Connect Tools with LLM

Connect tool nodes with LLM nodes. The LLM decides which tool to use based on
input or context. Tool output can be fed back into the LLM:

```py
from typing import Literal
from langgraph.graph import END, START
from langchain_core.messages import HumanMessage

# Define the function that determines whether to continue or not
def should_continue(state: MessagesState) -> Literal["tools", END]:
    messages = state['messages']
    last_message = messages[-1]
    if last_message.tool_calls:
        return "tools"  # Route to "tools" node if LLM makes a tool call
    return END  # Otherwise, stop

builder.add_edge(START, "agent")
builder.add_conditional_edges("agent", should_continue)
builder.add_edge("tools", 'agent')

graph = builder.compile()

graph.invoke({"messages": [HumanMessage(content="Do something with the tools")]})
```

## Manual usage

Execute a tool manually using the `invoke` method:

```py
result = tools[0].invoke({"name": "Alice", "age": 30})
```

This is useful for testing tools or when you need precise control over tool
execution outside of an agent framework.

## Client to Server Authentication

This section describes how to authenticate the ToolboxClient itself when
connecting to a Toolbox server instance that requires authentication. This is
crucial for securing your Toolbox server endpoint, especially when deployed on
platforms like Cloud Run, GKE, or any environment where unauthenticated access
is restricted.

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

The `ToolboxClient` allows you to specify functions (or coroutines for the async
client) that dynamically generate HTTP headers for every request sent to the
Toolbox server. The most common use case is to add an Authorization header with
a bearer token (e.g., a Google ID token).

These header-generating functions are called just before each request, ensuring
that fresh credentials or header values can be used.

### Configuration

You can configure these dynamic headers as follows:

```python
from toolbox_langchain import ToolboxClient

async with ToolboxClient(
    "toolbox-url", 
    client_headers={"header1": header1_getter, "header2": header2_getter, ...}
) as client:
```

### Authenticating with Google Cloud Servers

For Toolbox servers hosted on Google Cloud (e.g., Cloud Run) and requiring
`Google ID token` authentication, the helper module
[auth_methods](https://github.com/googleapis/mcp-toolbox-sdk-python/blob/main/packages/toolbox-core/src/toolbox_core/auth_methods.py) provides utility functions.

### Step by Step Guide for Cloud Run

1. **Configure Permissions**:
   [Grant](https://cloud.google.com/run/docs/securing/managing-access#service-add-principals)
   the `roles/run.invoker` IAM role on the Cloud
   Run service to the principal. This could be your `user account email` or a
   `service account`.
2. **Configure Credentials**
    - Local Development: Set up
   [ADC](https://cloud.google.com/docs/authentication/set-up-adc-local-dev-environment).
    - Google Cloud Environments: When running within Google Cloud (e.g., Compute
      Engine, GKE, another Cloud Run service, Cloud Functions), ADC is typically
      configured automatically, using the environment's default service account.
3. **Connect to the Toolbox Server**

    ```python
    from toolbox_langchain import ToolboxClient
    from toolbox_core import auth_methods

    auth_token_provider = auth_methods.aget_google_id_token(URL) # can also use sync method
    async with ToolboxClient(
        URL,
        client_headers={"Authorization": auth_token_provider},
    ) as client:
        tools = client.load_toolset()

        # Now, you can use the client as usual.
    ```


## Authenticating Tools

{{< notice info >}}
Always use HTTPS to connect your application with the Toolbox service, especially when using tools with authentication configured. Using HTTP exposes your application to serious security risks.
{{< /notice >}}

Some tools require user authentication to access sensitive data.

### Supported Authentication Mechanisms
Toolbox currently supports authentication using the [OIDC
protocol](https://openid.net/specs/openid-connect-core-1_0.html) with [ID
tokens](https://openid.net/specs/openid-connect-core-1_0.html#IDToken) (not
access tokens) for [Google OAuth
2.0](https://cloud.google.com/apigee/docs/api-platform/security/oauth/oauth-home).

### Configure Tools

Refer to [these
instructions](../../../../configuration/tools/_index.md#authenticated-parameters) on
configuring tools for authenticated parameters.

### Configure SDK

You need a method to retrieve an ID token from your authentication service:

```py
async def get_auth_token():
    # ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
    # This example just returns a placeholder. Replace with your actual token retrieval.
    return "YOUR_ID_TOKEN" # Placeholder
```

#### Add Authentication to a Tool

```py
async with ToolboxClient("http://127.0.0.1:5000") as toolbox:
    tools = toolbox.load_toolset()

    auth_tool = tools[0].add_auth_token_getter("my_auth", get_auth_token) # Single token

    multi_auth_tool = tools[0].add_auth_token_getters({"auth_1": get_auth_1}, {"auth_2": get_auth_2}) # Multiple tokens

    # OR

    auth_tools = [tool.add_auth_token_getter("my_auth", get_auth_token) for tool in tools]
```

#### Add Authentication While Loading

```py
auth_tool = toolbox.load_tool(auth_token_getters={"my_auth": get_auth_token})

auth_tools = toolbox.load_toolset(auth_token_getters={"my_auth": get_auth_token})
```
{{< notice note >}}
Adding auth tokens during loading only affect the tools loaded within that call.
{{< /notice >}}

### Complete Example

```py
import asyncio
from toolbox_langchain import ToolboxClient

async def get_auth_token():
    # ... Logic to retrieve ID token (e.g., from local storage, OAuth flow)
    # This example just returns a placeholder. Replace with your actual token retrieval.
    return "YOUR_ID_TOKEN" # Placeholder

async with ToolboxClient("http://127.0.0.1:5000") as toolbox:
    tool = toolbox.load_tool("my-tool")

    auth_tool = tool.add_auth_token_getter("my_auth", get_auth_token)
    result = auth_tool.invoke({"input": "some input"})
    print(result)
```

## Parameter Binding

Predetermine values for tool parameters using the SDK. These values won't be
modified by the LLM. This is useful for:

* **Protecting sensitive information:**  API keys, secrets, etc.
* **Enforcing consistency:** Ensuring specific values for certain parameters.
* **Pre-filling known data:**  Providing defaults or context.

### Binding Parameters to a Tool

```py
async with ToolboxClient("http://127.0.0.1:5000") as toolbox:
    tools = toolbox.load_toolset()

    bound_tool = tool[0].bind_param("param", "value") # Single param

    multi_bound_tool = tools[0].bind_params({"param1": "value1", "param2": "value2"}) # Multiple params

    # OR

    bound_tools = [tool.bind_param("param", "value") for tool in tools]
```

### Binding Parameters While Loading

```py
bound_tool = toolbox.load_tool("my-tool", bound_params={"param": "value"})

bound_tools = toolbox.load_toolset(bound_params={"param": "value"})
```
{{< notice note >}}
Bound values during loading only affect the tools loaded in that call.
{{< /notice >}}

### Binding Dynamic Values

Use a function to bind dynamic values:

```py
def get_dynamic_value():
  # Logic to determine the value
  return "dynamic_value"

dynamic_bound_tool = tool.bind_param("param", get_dynamic_value)
```
{{< notice note >}}
You don’t need to modify tool configurations to bind parameter values.
{{< /notice >}}

## Secure Parameters

{{< notice note >}}
Secure parameters are supported starting in `toolbox-langchain` version `1.4.0` (with `toolbox-core` >= `1.4.0`) and require MCP protocol version `2026-07-28` or newer with the `com.google.cloud/toolbox.v1` extension. For server configuration details, see [Secure Parameters](../../../../configuration/tools/_index.md#secure-parameters).
{{< /notice >}}

Secure parameters are designed for sensitive runtime values (such as an end-user `customer_id`, tenant identifier, or secret tokens) that LLMs must not see or control.

* **Schema Isolation:** Secure parameters are automatically excluded from LangChain's `tool.args_schema`, so models using `model.bind_tools(tools)` will never see or request them.
* **Prompt Injection Defense:** If a model attempts to supply a secure parameter in standard arguments, execution fails immediately.
* **Fast-Fail Validation:** Missing required secure parameters fail locally before invocation.
* **Binding While Loading or After:** You can provide secure parameters when loading tools or bind them to loaded tools (both synchronous and asynchronous clients supported):

```python
from toolbox_langchain import ToolboxClient

client = ToolboxClient("http://127.0.0.1:5000")

# Option A: Bind secure parameters when loading tools (sync or async)
bound_tool = client.load_tool("search_secure_data", secure_params={"customer_id": "cust_12345"})
tools = client.load_toolset("my-set", secure_params={"customer_id": "cust_12345"})

# Async client loading:
# bound_tool = await client.aload_tool("search_secure_data", secure_params={"customer_id": "cust_12345"})
# tools = await client.aload_toolset("my-set", secure_params={"customer_id": "cust_12345"})

# Option B: Bind secure parameters to an un-bound loaded tool (returns a new immutable tool)
raw_tool = client.load_tool("search_secure_data")
single_bound = raw_tool.bind_secure_param("customer_id", "cust_12345")
multi_bound = raw_tool.bind_secure_params({
    "customer_id": "cust_12345",
    "session_token": "token-xyz",
})

# Option C: Dynamic callable (evaluated per invocation)
dynamic_tool = raw_tool.bind_secure_param("customer_id", lambda: get_current_user_id())
```

### Cross-Binding Guidance & Mutual Exclusivity

* Calling `tool.bind_param()` on a secure parameter raises:  
  `ValueError: parameter '<name>' is a secure parameter; use bind_secure_param/bind_secure_params instead`
* Calling `tool.bind_secure_param()` on a regular parameter raises:  
  `ValueError: parameter '<name>' is a regular parameter; use bind_param/bind_params instead`


## Asynchronous Usage

For better performance through [cooperative
multitasking](https://en.wikipedia.org/wiki/Cooperative_multitasking), you can
use the asynchronous interfaces of the `ToolboxClient`.

{{< notice note >}}
Asynchronous interfaces like `aload_tool` and `aload_toolset` require an asynchronous environment. For guidance on running asynchronous Python programs, see [asyncio documentation](https://docs.python.org/3/library/asyncio-runner.html#running-an-asyncio-program).
{{< /notice >}}

```py
import asyncio
from toolbox_langchain import ToolboxClient

async def main():
    async with ToolboxClient("http://127.0.0.1:5000") as toolbox:
        tool = await client.aload_tool("my-tool")
        tools = await client.aload_toolset()
        response = await tool.ainvoke()

if __name__ == "__main__":
    asyncio.run(main())
```

## OpenTelemetry

The SDK supports OpenTelemetry tracing and metrics via the `toolbox-core` layer, following the [MCP Semantic Conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/mcp).

First install the telemetry extra from `toolbox-core`:

```bash
pip install toolbox-core[telemetry]
```

Then pass `telemetry_enabled=True` when creating your client:

```py
from toolbox_langchain import ToolboxClient

with ToolboxClient("http://127.0.0.1:5000", telemetry_enabled=True) as toolbox:
    tool = toolbox.load_tool("my-tool")
    result = tool.invoke({"param": "value"})
```

Configure your OpenTelemetry `TracerProvider` and `MeterProvider` before creating the client. See the [toolbox-core OpenTelemetry documentation](https://mcp-toolbox.dev/documentation/connect-to/toolbox-sdks/python-sdk/core/#opentelemetry) for a full setup example.

### Per-call Telemetry Attributes

Use `TelemetryAttributes` to attach model, user, and agent metadata to tool invocations:

```py
from toolbox_core import TelemetryAttributes
from toolbox_langchain import ToolboxClient

attrs = TelemetryAttributes(
    llm_model="gemini-3.6-flash",
    user_id="user-123",
    agent_id="agent-abc",
)

with ToolboxClient("http://127.0.0.1:5000") as toolbox:
    tools = toolbox.load_toolset("my-toolset", telemetry_attributes=attrs)

    tool = toolbox.load_tool("my-tool")
    instrumented_tool = tool.add_telemetry_attributes(attrs)
```

You can pass `telemetry_attributes` to `load_tool()` or `load_toolset()`, or call `add_telemetry_attributes()` on a loaded tool. See the [toolbox-core telemetry attributes documentation](https://mcp-toolbox.dev/documentation/connect-to/toolbox-sdks/python-sdk/core/#per-call-telemetry-attributes) for field details.
