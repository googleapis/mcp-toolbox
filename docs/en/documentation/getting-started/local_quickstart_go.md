---
title: "Go Quickstart (Local)"
type: docs
weight: 4
description: >
  How to get started running MCP Toolbox locally with [Go](https://github.com/googleapis/mcp-toolbox-sdk-go), PostgreSQL, and orchestration frameworks such as [LangChain Go](https://tmc.github.io/langchaingo/docs/), [GenkitGo](https://genkit.dev/go/docs/get-started-go/), [Go GenAI](https://github.com/googleapis/go-genai) and [OpenAI Go](https://github.com/openai/openai-go).
sample_filters: ["Go", "Quickstart", "Local", "ADK", "LangChain", "OpenAI", "Genkit", "Google GenAI"]
is_sample: true
---

## Before you begin

This guide assumes you have already done the following:

1. Installed [Go (v1.24.2 or higher)].
1. Installed [PostgreSQL 16+ and the `psql` client][install-postgres].

[Go (v1.24.2 or higher)]: https://go.dev/doc/install
[install-postgres]: https://www.postgresql.org/download/

### Cloud Setup (Optional)

{{< regionInclude "quickstart/shared/cloud_setup.md" "cloud_setup" >}}

## Step 1: Set up your database

{{< regionInclude "quickstart/shared/database_setup.md" "database_setup" >}}

## Step 2: Install and configure MCP Toolbox

{{< regionInclude "quickstart/shared/configure_toolbox.md" "configure_toolbox" >}}

## Step 3: Connect your agent to MCP Toolbox

In this section, we will write and run an agent that will load the Tools
from MCP Toolbox.

1. Initialize a go module:

    ```bash
    go mod init main
    ```

1. In a new terminal, install the Go SDK Module:
    {{< notice warning >}}
Breaking Change Notice: As of version `0.6.0`, this SDK has transitioned to a multi-module structure.
* For new versions (`v0.6.0`+): You must import specific modules (e.g., `go get github.com/googleapis/mcp-toolbox-sdk-go/core`).
* For older versions (`v0.5.1` and below): The SDK remains a single-module library (`go get github.com/googleapis/mcp-toolbox-sdk-go`).
* Please update your imports and `go.mod` accordingly when upgrading.
    {{< /notice >}}

   {{< tabpane persist=header >}}
    {{< tab header="LangChain Go" lang="bash" >}}
    go get github.com/googleapis/mcp-toolbox-sdk-go/core
    {{< /tab >}}

    {{< tab header="Genkit Go" lang="bash" >}}
    go get github.com/googleapis/mcp-toolbox-sdk-go/core
    go get github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit
    {{< /tab >}}

    {{< tab header="Go GenAI" lang="bash" >}}
    go get github.com/googleapis/mcp-toolbox-sdk-go/core
    {{< /tab >}}

    {{< tab header="OpenAI Go" lang="bash" >}}
    go get github.com/googleapis/mcp-toolbox-sdk-go/core
    {{< /tab >}}

    {{< tab header="ADK Go" lang="bash" >}}
    go get github.com/googleapis/mcp-toolbox-sdk-go/core
    go get github.com/googleapis/mcp-toolbox-sdk-go/tbadk
    {{< /tab >}}
    {{< /tabpane >}}

2. Create a new file named `hotelagent.go` and copy the following code to create
   an agent:

    {{< tabpane persist=header >}}
{{% tab header="LangChain Go" text=true %}}

> The `langchaingo` library is currently not compatible with modern Google AI models (such as Gemini 2.5+ and 3.0+) because it lacks support for `thought_signature` on multi-turn function calling. This sample code is currently untested with available Gemini models.

{{< highlight go >}}
{{< include "quickstart/go/langchain/quickstart.go" >}}
{{< /highlight >}}

{{% /tab %}}

{{< tab header="Genkit Go" lang="go" >}}

{{< include "quickstart/go/genkit/quickstart.go" >}}

{{< /tab >}}

{{< tab header="Go GenAI" lang="go" >}}

{{< include "quickstart/go/genAI/quickstart.go" >}}

{{< /tab >}}

{{< tab header="OpenAI Go" lang="go" >}}

{{< include "quickstart/go/openAI/quickstart.go" >}}

{{< /tab >}}

{{< tab header="ADK Go" lang="go" >}}

{{< include "quickstart/go/adkgo/quickstart.go" >}}

{{< /tab >}}
{{< /tabpane >}}

1. Ensure all dependencies are installed:

    ```sh
    go mod tidy
    ```

2. Run your agent, and observe the results:

    ```sh
    go run hotelagent.go
    ```

{{< notice info >}}
For more information, visit the [Go SDK
repo](https://github.com/googleapis/mcp-toolbox-sdk-go).
{{</ notice >}}
