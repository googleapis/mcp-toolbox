---
title: "Getting Started"
type: docs
weight: 2
description: >
  Understand the core concepts of MCP Toolbox, explore integration strategies, and learn how to architect your AI agent connections.
---

Before you spin up your server and start writing code, it is helpful to understand the different ways you can utilize the Toolbox within your architecture.

This guide breaks down the core methodologies for using MCP Toolbox, how to think about your tool configurations, and the different ways your applications can connect to it.

## Prebuilt vs. Custom Configs

MCP Toolbox provides two main approaches for tools: **prebuilt** and **custom**.

[**Prebuilt tools**](../configuration/prebuilt-configs/_index.md) are ready to use out of
the box. For example, a tool like
[`postgres-execute-sql`](../../integrations/postgres/tools/postgres-execute-sql.md) has fixed parameters
and always works the same way, allowing the agent to execute arbitrary SQL.
While these are convenient, they are typically only safe when a developer is in
the loop (e.g., during prototyping, developing, or debugging).

For application use cases, you need to be wary of security risks such as prompt
injection or data poisoning. Allowing an LLM to execute arbitrary queries in
production is highly dangerous.

To secure your application, you should [**use custom tools**](../configuration/tools/_index.md) to suit your
specific schema and application needs. Creating a custom tool restricts the
agent's capabilities to only what is necessary. For example, you can use the
[`postgres-sql`](../../integrations/postgres/tools/postgres-sql.md) tool to define a specific action. This
typically involves:

*   **Prepared Statements:** Writing a SQL query ahead of time and letting the
    agent only fill in specific [basic parameters](../configuration/tools/_index.md#basic-parameters).
*   [**Bound Parameters:**](../connect-to/toolbox-sdks/python-sdk/core/index.md#option-a-binding-parameters-to-a-loaded-tool)
    Passing parameters directly to the underlying engine as bound variables
    rather than allowing the LLM to provide them.
*   **Secure Parameters:** Using mechanisms like [authenticated
    parameters](../configuration/tools/_index.md#authenticated-parameters) to restrict what data the agent can
    access based on the logged-in user.

By creating custom tools, you significantly reduce the attack surface and ensure
the agent operates within defined, safe boundaries.

---

## Build-Time vs. Runtime Implementation

A key architectural benefit of the MCP Toolbox is flexibility in *how* and *when* your AI clients learn about their available tools. Understanding this distinction helps you choose the right integration path.

### Build-Time
In this model, the available tools and their schemas are established when the client initializes.
*   **How it works:** The client launches or connects to the MCP Toolbox server, reads the available tools once, and keeps them static for the session.
*   [**Best for:** **IDEs and CLI tools**](../connect-to/_index.md)

### Runtime
In this model, your application dynamically requests the latest tools from the Toolbox server on the fly.
*   **How it works:** Your application code actively calls the server at runtime to fetch the latest toolsets and their schemas.
*   [**Best for:** **AI Agents and Custom Applications**](../connect-to/_index.md)

---

## How to connect

Being built on the Model Context Protocol (MCP), MCP Toolbox is framework-agnostic. You can connect to it in three main ways:

*   [**IDE Integrations:**](../connect-to/ides/_index.md) Connect your local Toolbox server directly to MCP-compatible development environments.
*   [**CLI Tools:**](../connect-to/gemini-cli/_index.md) Use command-line interfaces like the Gemini CLI to interact with your databases using natural language directly from your terminal.
*   [**MCP Client:**](../connect-to/mcp-client/_index.md) Connect to an MCP Client.
*   [**Application Integration (Client SDKs):**](../connect-to/toolbox-sdks/_index.md) If you are building custom AI agents, you can use our Client SDKs to pull tools directly into your application code. We provide native support for major orchestration frameworks including LangChain, LlamaIndex, Genkit, and more across Python, JavaScript/TypeScript, Go, and Java.

---

## Quickstarts
