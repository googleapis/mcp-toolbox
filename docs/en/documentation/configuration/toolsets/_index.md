---
title: "Toolsets"
type: docs
weight: 5
description: >
  Toolsets allow you to define logical groups of tools to load together for specific agents or applications.
---

A Toolset allows you to logically group multiple tools together so they can be loaded and managed as a single unit. You can define Toolsets as documents in your `tools.yaml` file.

This is especially useful when you are building a system with multiple AI agents or applications, where each agent only needs access to a specific subset of tools to perform its specialized tasks safely and efficiently.

{{< notice tip >}}
Try organizing your toolsets by the agent's persona or app feature (e.g., `data_analyst_set` vs `customer_support_set`). This keeps your client-side code clean and ensures an agent isn't distracted by tools it doesn't need.
{{< /notice >}}

{{< notice note >}}
A toolset is a tools-only [Group](../groups/), and Toolbox now loads every `kind: toolset` as a group. Existing toolsets keep working, with three exceptions: a `description` written on a toolset is dropped (with a warning), unrecognized fields are rejected at startup, and reusing one name for both a `kind: toolset` and a `kind: group` is now a duplicate-name error. We recommend migrating to `kind: group` — even for tools-only collections — because a group lets you attach a `description` (surfaced via `groups/list`) and scope other MCP primitives such as **prompts** alongside your **tools**. Run `toolbox migrate` to convert automatically.
{{< /notice >}}

## Defining Toolsets

In your configuration file, define each toolset by providing a unique `name` and a list of `tools` that belong to that group..

```yaml
kind: toolset
name: my_first_toolset
tools:
  - my_first_tool
  - my_second_tool
---
kind: toolset
name: my_second_toolset
tools:
  - my_second_tool
  - my_third_tool
```

## Using toolsets with MCP Toolbox Client SDKs

Once your toolsets are defined in your configuration, you can retrieve them directly from your application code. If you request a toolset without specifying a name, the SDKs will default to loading every tool available on the server.

Here is how to load your toolsets across our supported languages:

### Python

```python
# Load all tools available on the server
all_tools = client.load_toolset()

# Load only the tools defined in 'my_second_toolset'
my_second_toolset = client.load_toolset("my_second_toolset")
```

### Javascript/Typescript

```javascript
// Load all tools available on the server
const allTools = await client.loadToolset()

// Load only the tools defined in 'my_second_toolset'
const mySecondToolset = await client.loadToolset("my_second_toolset")
```

### Go

```go
// Load all tools available on the server
allTools, err := client.LoadToolset("", ctx)

// Load only the tools defined in 'my_second_toolset'
mySecondToolset, err := client.LoadToolset("my-toolset", ctx)
```