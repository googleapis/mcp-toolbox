---
title: "Connect to Toolbox"
type: docs
weight: 4
description: >
  Learn how to connect your applications, AI agents, CLIs, and IDEs to MCP Toolbox.
---

Once your MCP Toolbox server is configured and running, the next step is putting those tools to work. Because MCP Toolbox is built on the Model Context Protocol (MCP), it acts as a universal control plane that can be consumed by a wide variety of clients.

Choose your connection method below based on your use case:

## Client SDKs (Application Integration)

If you are building custom AI agents or orchestrating multi-step workflows in code, use our officially supported Client SDKs. These SDKs allow your application to fetch tool schemas and execute queries dynamically at runtime.

*   **[Python SDKs](toolbox-sdks/python-sdk/_index.md)**: Connect using our Core SDK, or leverage native integrations for LangChain, LlamaIndex, and the Agent Development Kit (ADK).
*   **[JavaScript / TypeScript SDKs](toolbox-sdks/javascript-sdk/_index.md)**: Build Node.js applications using our Core SDK or ADK integrations.
*   **[Go SDKs](toolbox-sdks/go-sdk/_index.md)**: Build highly concurrent agents with our Go Core SDK, or use our integrations for Genkit and ADK.
*   **[Java SDKs](toolbox-sdks/java-sdk/_index.md)**: Build enterprise Java applications and agent integrations using our Java Core SDK.

## MCP Clients & CLIs

You do not need to build a full application to use the Toolbox. You can interact with your configured databases and execute tools directly from your terminal using MCP-compatible command-line clients.

* **[MCP Client](mcp-client/_index.md)**: Connect to an MCP client.

*   **[Gemini CLI](gemini-cli/_index.md)**: Explore how to use the Gemini CLI and its available datacloud extensions to manage and query your data using natural language commands right from your terminal.

## IDE Integrations

By connecting the Toolbox directly to an MCP-compatible IDE, your AI coding assistant gains real-time access to your database schemas, allowing it to write perfectly tailored queries and application code.

*   **[IDEs](ides/_index.md)**: Guide for connecting your IDE to AlloyDB instances.

## Available Connection Methods