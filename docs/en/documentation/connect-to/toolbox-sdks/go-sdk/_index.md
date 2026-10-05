---
title: "Go"
type: docs
weight: 3
description: >
  Go SDKs to connect to the MCP Toolbox server.
---

## Overview

The MCP Toolbox service provides a centralized way to manage and expose tools
(like API connectors, database query tools, etc.) for use by GenAI applications.

The Go SDK act as clients for that service. They handle the communication needed to:

* Fetch tool definitions from your running Toolbox instance.
* Provide convenient Go structs representing those tools.
* Invoke the tools (calling the underlying APIs/services configured in Toolbox).
* Handle authentication, parameter binding, and secure parameters as needed.

By using the SDK, you can easily leverage your Toolbox-managed tools directly
within your Go applications or AI orchestration frameworks.

{{< notice note >}}
[Secure parameters](../../../configuration/tools/_index.md#secure-parameters) are supported starting in:
* `core` >= `v1.2.0`
* `tbadk` >= `v1.2.0`
* `tbgenkit` >= `v0.10.0`
* Server requirement: MCP protocol version `2026-07-28` or newer with the `com.google.cloud/toolbox.v1` extension.
{{< /notice >}}

<a class="btn btn-primary" href="https://go.mcp-toolbox.dev" role="button"><i class="fa-brands fa-golang"></i> Go API Reference</a>

## Which Package Should I Use?

Choosing the right package depends on how you are building your application:

- [**`core`**](core/):
  This is a framework-agnostic way to connect tools to popular frameworks
  like Google GenAI, LangChain, etc.

- [**`tbadk`**](tbadk/):
  This package provides a way to connect tools to ADK Go.

- [**`tbgenkit`**](tbgenkit/):
  This package provides functionality to convert the Tool fetched using the core package
  into a Genkit Go compatible tool.

## Available Packages

This repository hosts the following Go packages. See the package-specific
README for detailed installation and usage instructions:

| Package | Target Use Case | Integration | Path | Details (README) |
| :------ | :----------| :---------- | :---------------------- | :---------- |
| [`core`](core/) | Framework-agnostic / Custom applications | Use directly / Custom | `core/` | 📄 [View README](https://github.com/googleapis/mcp-toolbox-sdk-go/blob/main/core/README.md) |
| [`tbadk`](tbadk/) | ADK Go | Use directly | `tbadk/` | 📄 [View README](https://github.com/googleapis/mcp-toolbox-sdk-go/blob/main/tbadk/README.md) |
| [`tbgenkit`](tbgenkit/) | Genkit Go | Along with core | `tbgenkit/` | 📄 [View README](https://github.com/googleapis/mcp-toolbox-sdk-go/blob/main/tbgenkit/README.md) |

## Getting Started

To get started using Toolbox tools with an application, follow these general steps:

1. **Set up and Run the Toolbox Service:**

    Before using the SDKs, you need the MCP Toolbox server running. Follow
    the instructions here: [**Toolbox Getting Started
    Guide**](../../../../documentation/getting-started/local_quickstart_go.md)

2. **Install the Appropriate SDK:**

    Choose the package based on your needs (see "[Which Package Should I Use?](#which-package-should-i-use)" above)
    Use this command to install the SDK module

    ```bash
    # For the core, framework-agnostic SDK
    go get github.com/googleapis/mcp-toolbox-sdk-go
    ```

{{< notice note >}}
Source code for [Go-sdk](https://github.com/googleapis/mcp-toolbox-sdk-go)
{{< /notice >}}
