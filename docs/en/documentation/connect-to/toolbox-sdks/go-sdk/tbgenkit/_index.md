---
title: "Genkit Package"
linkTitle: "Genkit"
type: docs
weight: 3
description: >
  MCP Toolbox Genkit for integrating functionalities of MCP Toolbox into your Agentic apps.
---

## Overview

The `tbgenkit` package provides a Go interface to the MCP Toolbox service, enabling you to load and invoke tools from your own applications.

## Installation

```bash
go get github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit
```
This SDK is supported on Go version 1.24.4 and higher.

{{< notice note >}}
**Breaking Change Notice**: As of version `0.6.0`, this repository has transitioned to a multi-module structure.
*   **For new versions (`v0.6.0`+)**: You must import specific modules (e.g., `go get github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit`).
*   **For older versions (`v0.5.1` and below)**: The repository remains a single-module library (`go get github.com/googleapis/mcp-toolbox-sdk-go`).
*   Please update your imports and `go.mod` accordingly when upgrading.
{{< /notice >}}

## Quickstart

For more information on how to load a `ToolboxTool`, see [the core package](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main/core)

## Convert Toolbox Tool to a Genkit Tool

```go
"github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit"

func main() {
  // Assuming the toolbox tool is loaded
  // Make sure to add error checks for debugging
  ctx := context.Background()
  g, err := genkit.Init(ctx)

  genkitTool, err := tbgenkit.ToGenkitTool(toolboxTool, g)

}
```

## Secure Parameters

{{< notice note >}}
Secure parameters are supported starting in [`github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit`](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main/tbgenkit) version `v0.10.0` (with `core` >= `v1.2.0`) and require MCP protocol version `2026-07-28` or newer with the [`com.google.cloud/toolbox.v1` extension](https://github.com/googleapis/mcp-toolbox/blob/main/extensions/2026-07-28/README.md). For server configuration details, see [Secure Parameters](../../../../configuration/tools/_index.md#secure-parameters).
{{< /notice >}}

Secure parameters are designed for sensitive runtime values (such as an end-user `customer_id`, tenant identifier, or secret tokens) that LLMs must not see, control, or hallucinate.

* **Schema Isolation:** When converting a `ToolboxTool` into a Genkit tool via `tbgenkit.ToGenkitTool`, secure parameters are automatically excluded from the Genkit tool's input schema. Genkit models will never see or prompt for these parameters.
* **Pre-Binding Required:** Secure parameters must be bound to the `ToolboxTool` (either during loading or via `tool.ToolFrom(...)`) before conversion. All type-safe `core.WithBindSecureParam*` and `core.WithBindSecureParam*Func` options are supported.
* **Prompt Injection Defense:** If a model attempts to supply a secure parameter in standard inputs, execution fails immediately.
* **Fast-Fail Validation:** If any required secure parameter is not bound, execution fails locally before making network requests.

```go
package main

import (
    "context"
    "github.com/firebase/genkit/go/genkit"
    "github.com/googleapis/mcp-toolbox-sdk-go/core"
    "github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit"
)

func main() {
    ctx := context.Background()
    toolboxClient, err := core.NewToolboxClient("http://127.0.0.1:5000")

    // Load tool with secure parameter bound
    tool, err := toolboxClient.LoadTool("search_secure_data", ctx,
        core.WithBindSecureParamString("customer_id", "cust_12345"),
    )

    // Convert to Genkit tool
    g, err := genkit.Init(ctx)
    genkitTool, err := tbgenkit.ToGenkitTool(tool, g)
}
```


## Using with Orchestration Frameworks

To see how the MCP Toolbox Go SDK works with orchestration frameworks, check out these end-to-end examples given below.

<details>
<summary>Genkit Go</summary>

```go
//This sample contains a complete example on how to integrate MCP Toolbox Go SDK with Genkit Go using the tbgenkit package.
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/googleapis/mcp-toolbox-sdk-go/core"
	"github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/firebase/genkit/go/plugins/googlegenai"
)

func main() {
	ctx := context.Background()
	toolboxClient, err := core.NewToolboxClient("http://127.0.0.1:5000")
	if err != nil {
		log.Fatalf("Failed to create Toolbox client: %v", err)
	}

	// Load the tools using the MCP Toolbox SDK.
	tools, err := toolboxClient.LoadToolset("my-toolset", ctx)
	if err != nil {
		log.Fatalf("Failed to load tools: %v\nMake sure your Toolbox server is running and the tool is configured.", err)
	}

	// Initialize genkit
  g := genkit.Init(ctx,
		genkit.WithPlugins(&googlegenai.GoogleAI{}),
		genkit.WithDefaultModel("googleai/gemini-1.5-flash"),
	)

	// Convert your tool to a Genkit tool.
	genkitTools := make([]ai.Tool, len(tools))
	for i, tool := range tools {
		newTool, err := tbgenkit.ToGenkitTool(tool, g)
		if err != nil {
			log.Fatalf("Failed to convert tool: %v\n", err)
		}
		genkitTools[i] = newTool
	}

	toolRefs := make([]ai.ToolRef, len(genkitTools))

	for i, tool := range genkitTools {
		toolRefs[i] = tool
	}

	// Generate llm response using prompts and tools.
	resp, err := genkit.Generate(ctx, g,
		ai.WithPrompt("Find hotels in Basel with Basel in it's name."),
		ai.WithTools(toolRefs...),
	)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	fmt.Println(resp.Text())
}
```

</details>