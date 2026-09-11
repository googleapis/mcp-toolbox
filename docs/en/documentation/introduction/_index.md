---
title: "Introduction"
type: docs
weight: 1
description: >
  An introduction to MCP Toolbox for Databases.
---

MCP Toolbox for Databases is an open source Model Context Protocol (MCP) server that connects your AI agents, IDEs, and applications directly to your enterprise databases.

It serves a **dual purpose**:
1. **Ready-to-use MCP Server (aka ['Build-Time'](../getting-started/#build-time)):** Instantly connect Gemini CLI, Google Antigravity, Claude Code, Codex, or other MCP clients to your databases using our *prebuilt generic tools*. Talk to your data, explore schemas, and generate code without writing boilerplate.
2. **Custom Tools Framework (aka ['Run-Time'](../getting-started/#runtime)):** A robust framework to build specialized, highly secure AI tools for your production agents. Define structured queries, semantic search, and NL2SQL capabilities safely and easily.

{{< notice tip >}}
**Repository Name Update:** The GitHub repository for this project has been officially renamed from `genai-toolbox` to `mcp-toolbox`. We recommend updating your local Git remote URL for consistency:
`git remote set-url origin https://github.com/googleapis/mcp-toolbox.git`
{{< /notice >}}

{{< notice note >}}
This document has been updated to support the flat configuration file format. To
view documentation with original configuration file format, please navigate to the
top-right menu and select versions v0.26.0 or older.
{{< /notice >}}

## Why MCP Toolbox?

- **Out-of-the-Box Database Access:** Prebuilt generic tools for instant data exploration (e.g., `list_tables`, `execute_sql`) directly from your IDE or CLI.
- **Custom Tools Framework:** Build production-ready tools with your own predefined logic, ensuring safety through Restricted Access, Structured Queries, and Semantic Search.
- **Simplified Development:** Integrate tools into your Agent Development Kit (ADK), LangChain, LlamaIndex, or custom agents in less than 10 lines of code.
- **Better Performance:** Handles connection pooling, integrated auth (IAM), and end-to-end observability (OpenTelemetry) out of the box.
- **Enhanced Security**: Integrated authentication for more secure access to your data.
- **End-to-end Observability**: Out of the box metrics and tracing with built-in support for OpenTelemetry.

{{< notice note >}}
This solution was originally named “Gen AI Toolbox for
Databases” as its initial development predated MCP, but was renamed to align
with the added MCP compatibility.
{{< /notice >}}

## General Architecture

Toolbox sits between your application's orchestration framework and your
database, providing a control plane that is used to modify, distribute, or
invoke tools. It simplifies the management of your tools by providing you with a
centralized location to store and update tools, allowing you to share tools
between agents and applications and update those tools without necessarily
redeploying your application.

![architecture](./architecture.png)

## Getting Started

### Quickstart: Running Toolbox using NPX

#### Ready-to-use MCP tools

Add the following to your client's MCP configuration file (usually `mcp.json` or `claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "toolbox-postgres": {
      "command": "npx",
      "args": [
        "-y",
        "@toolbox-sdk/server",
        "--prebuilt=postgres"
      ]
    }
  }
}
```

Set the appropriate environment variables to connect, see the [Prebuilt Tools Reference](https://mcp-toolbox.dev/documentation/configuration/prebuilt-configs/).

When you run Toolbox with a `--prebuilt=<database>` flag, you instantly get access to standard tools to interact with that database. 

Supported databases currently include:
- **Google Cloud:** AlloyDB, BigQuery, Cloud SQL (PostgreSQL, MySQL, SQL Server), Spanner, Firestore, Knowledge Catalog (formerly known as Dataplex).
- **Other Databases:** PostgreSQL, MySQL, SQL Server, Oracle, MongoDB, Redis, Elasticsearch, CockroachDB, ClickHouse, Couchbase, Neo4j, Snowflake, Trino, and more.

For a full list of available tools and their capabilities across all supported databases, see the [Prebuilt Tools Reference](https://mcp-toolbox.dev/documentation/configuration/prebuilt-configs/).

#### Custom Tools

You can run Toolbox directly with a [configuration file](../configuration/_index.md):

```sh
npx @toolbox-sdk/server --config tools.yaml
```

{{< notice note >}}
This method is optimized for convenience rather than performance. For a more standard and reliable installation, please use the binary or container image as described in [Install Toolbox](#install-toolbox).
{{< /notice >}}

### Install Toolbox

For the latest version, check the [releases page][releases] and use the
following instructions for your OS and CPU architecture.

[releases]: https://github.com/googleapis/mcp-toolbox/releases

<!-- {x-release-please-start-version} -->
{{< tabpane text=true >}}
{{% tab header="Binary" lang="en" %}}
{{< tabpane text=true >}}
{{% tab header="Linux (AMD64)" lang="en" %}}
To install Toolbox as a binary on Linux (AMD64):

```sh
# see releases page for other versions
export VERSION=1.11.0
curl -L -o toolbox https://storage.googleapis.com/mcp-toolbox-for-databases/v$VERSION/linux/amd64/toolbox
chmod +x toolbox
```

{{% /tab %}}
{{% tab header="macOS (Apple Silicon)" lang="en" %}}
To install Toolbox as a binary on macOS (Apple Silicon):

```sh
# see releases page for other versions
export VERSION=1.11.0
curl -L -o toolbox https://storage.googleapis.com/mcp-toolbox-for-databases/v$VERSION/darwin/arm64/toolbox
chmod +x toolbox
```

{{% /tab %}}
{{% tab header="macOS (Intel)" lang="en" %}}
To install Toolbox as a binary on macOS (Intel):

```sh
# see releases page for other versions
export VERSION=1.11.0
curl -L -o toolbox https://storage.googleapis.com/mcp-toolbox-for-databases/v$VERSION/darwin/amd64/toolbox
chmod +x toolbox
```

{{% /tab %}}
{{% tab header="Windows (Command Prompt)" lang="en" %}}
To install Toolbox as a binary on Windows (Command Prompt):

```cmd
:: see releases page for other versions
set VERSION=1.11.0
curl -o toolbox.exe "https://storage.googleapis.com/mcp-toolbox-for-databases/v%VERSION%/windows/amd64/toolbox.exe"
```

{{% /tab %}}
{{% tab header="Windows (PowerShell)" lang="en" %}}
To install Toolbox as a binary on Windows (PowerShell):

```powershell
# see releases page for other versions
$VERSION = "1.11.0"
curl.exe -o toolbox.exe "https://storage.googleapis.com/mcp-toolbox-for-databases/v$VERSION/windows/amd64/toolbox.exe"
```

{{% /tab %}}
{{% tab header="Windows ARM64 (Command Prompt)" lang="en" %}}
To install Toolbox as a binary on Windows ARM64 (Command Prompt):

```cmd
:: see releases page for other versions
set VERSION=1.11.0
curl -o toolbox.exe "https://storage.googleapis.com/mcp-toolbox-for-databases/v%VERSION%/windows/arm64/toolbox.exe"
```

{{% /tab %}}
{{% tab header="Windows ARM64 (PowerShell)" lang="en" %}}
To install Toolbox as a binary on Windows ARM64 (PowerShell):

```powershell
# see releases page for other versions
$VERSION = "1.11.0"
curl.exe -o toolbox.exe "https://storage.googleapis.com/mcp-toolbox-for-databases/v$VERSION/windows/arm64/toolbox.exe"
```

{{% /tab %}}
{{< /tabpane >}}
{{% /tab %}}
{{% tab header="Container image" lang="en" %}}
You can also install Toolbox as a container:

```sh
# see releases page for other versions
export VERSION=1.11.0
docker pull us-central1-docker.pkg.dev/database-toolbox/toolbox/toolbox:$VERSION
```

{{% /tab %}}
{{% tab header="Homebrew" lang="en" %}}
To install Toolbox using Homebrew on macOS or Linux:

```sh
brew install mcp-toolbox
```

{{% /tab %}}
{{% tab header="Compile from source" lang="en" %}}

To install from source, ensure you have the latest version of
[Go installed](https://go.dev/doc/install), and then run the following command:

```sh
go install github.com/googleapis/mcp-toolbox@v1.11.0
```

{{% /tab %}}
{{< /tabpane >}}
<!-- {x-release-please-end} -->

### Run Toolbox

[Configure](../configuration/_index.md) a `tools.yaml` to define your tools, and then
execute `toolbox` to start the server:

```sh
./toolbox --config "tools.yaml"
```

{{< notice note >}}
Toolbox enables dynamic reloading by default. To disable, use the
`--disable-reload` flag.
{{< /notice >}}

#### Launching Toolbox UI

To launch Toolbox's interactive UI, use the `--ui` flag. This allows you to test
tools and toolsets with features such as authorized parameters. To learn more,
visit [Toolbox UI](../configuration/toolbox-ui/index.md).

```sh
./toolbox --ui
```

#### Homebrew Users

If you installed Toolbox using Homebrew, the `toolbox` binary is available in
your system path. You can start the server with the same command:

```sh
toolbox --config "tools.yaml"
```

You can use `toolbox help` for a full list of flags! To stop the server, send a
terminate signal (`ctrl+c` on most platforms).

For more detailed documentation on deploying to different environments, check
out the resources in the [Deploy section](../../documentation/deploy-to/_index.md)

### Integrating your application

Once your server is up and running, you can load the tools into your
application. See below the list of Client SDKs for using various frameworks:

#### Python

{{< tabpane text=true persist=header >}}
{{% tab header="Core" lang="en" %}}

Once you've installed the [Toolbox Core
SDK](https://pypi.org/project/toolbox-core/), you can load
tools:

{{< highlight python >}}
from toolbox_core import ToolboxClient

# update the url to point to your server
async with ToolboxClient("http://127.0.0.1:5000") as client:

    # these tools can be passed to your application!
    tools = await client.load_toolset("toolset_name")
{{< /highlight >}}

For more detailed instructions on using the Toolbox Core SDK, see the
[README](https://github.com/googleapis/mcp-toolbox-sdk-python/blob/main/packages/toolbox-core/README.md).

{{% /tab %}}
{{% tab header="LangChain" lang="en" %}}

Once you've installed the [Toolbox LangChain
SDK](https://pypi.org/project/toolbox-langchain/), you can load
tools:

{{< highlight python >}}
from toolbox_langchain import ToolboxClient

# update the url to point to your server
async with ToolboxClient("http://127.0.0.1:5000") as client:

    # these tools can be passed to your application!
    tools = client.load_toolset()
{{< /highlight >}}

For more detailed instructions on using the Toolbox LangChain SDK, see the
[README](https://github.com/googleapis/mcp-toolbox-sdk-python/blob/main/packages/toolbox-langchain/README.md).

{{% /tab %}}
{{% tab header="Llamaindex" lang="en" %}}

Once you've installed the [Toolbox Llamaindex
SDK](https://github.com/googleapis/genai-toolbox-llamaindex-python), you can load
tools:

{{< highlight python >}}
from toolbox_llamaindex import ToolboxClient

# update the url to point to your server
async with ToolboxClient("http://127.0.0.1:5000") as client:

# these tools can be passed to your application

  tools = client.load_toolset()
{{< /highlight >}}

For more detailed instructions on using the Toolbox Llamaindex SDK, see the
[README](https://github.com/googleapis/genai-toolbox-llamaindex-python/blob/main/README.md).

{{% /tab %}}
{{< /tabpane >}}

#### Javascript/Typescript

Once you've installed the [Toolbox Core
SDK](https://www.npmjs.com/package/@toolbox-sdk/core), you can load
tools:

{{< tabpane text=true persist=header >}}
{{% tab header="Core" lang="en" %}}

{{< highlight javascript >}}
import { ToolboxClient } from '@toolbox-sdk/core';

// update the url to point to your server
const URL = 'http://127.0.0.1:5000';
let client = new ToolboxClient(URL);

// these tools can be passed to your application!
const toolboxTools = await client.loadToolset('toolsetName');
{{< /highlight >}}

For more detailed instructions on using the Toolbox Core SDK, see the
[README](https://github.com/googleapis/mcp-toolbox-sdk-js/blob/main/packages/toolbox-core/README.md).

{{% /tab %}}
{{% tab header="LangChain/Langraph" lang="en" %}}

{{< highlight javascript >}}
import { ToolboxClient } from '@toolbox-sdk/core';

// update the url to point to your server
const URL = 'http://127.0.0.1:5000';
let client = new ToolboxClient(URL);

// these tools can be passed to your application!
const toolboxTools = await client.loadToolset('toolsetName');

// Define the basics of the tool: name, description, schema and core logic
const getTool = (toolboxTool) => tool(currTool, {
    name: toolboxTool.getName(),
    description: toolboxTool.getDescription(),
    schema: toolboxTool.getParamSchema()
});

// Use these tools in your Langchain/Langraph applications
const tools = toolboxTools.map(getTool);
{{< /highlight >}}

For more detailed instructions on using the Toolbox Core SDK, see the
[README](https://github.com/googleapis/mcp-toolbox-sdk-js/blob/main/packages/toolbox-core/README.md).

{{% /tab %}}
{{% tab header="Genkit" lang="en" %}}

{{< highlight javascript >}}
import { ToolboxClient } from '@toolbox-sdk/core';
import { genkit } from 'genkit';

// Initialise genkit
const ai = genkit({
    plugins: [
        googleAI({
            apiKey: process.env.GEMINI_API_KEY || process.env.GOOGLE_API_KEY
        })
    ],
    model: googleAI.model('gemini-2.0-flash'),
});

// update the url to point to your server
const URL = 'http://127.0.0.1:5000';
let client = new ToolboxClient(URL);

// these tools can be passed to your application!
const toolboxTools = await client.loadToolset('toolsetName');

// Define the basics of the tool: name, description, schema and core logic
const getTool = (toolboxTool) => ai.defineTool({
    name: toolboxTool.getName(),
    description: toolboxTool.getDescription(),
    schema: toolboxTool.getParamSchema()
}, toolboxTool)

// Use these tools in your Genkit applications
const tools = toolboxTools.map(getTool);
{{< /highlight >}}

For more detailed instructions on using the Toolbox Core SDK, see the
[README](https://github.com/googleapis/mcp-toolbox-sdk-js/blob/main/packages/toolbox-core/README.md).

{{% /tab %}}
{{% tab header="LlamaIndex" lang="en" %}}

{{< highlight javascript >}}
import { ToolboxClient } from '@toolbox-sdk/core';
import { tool } from "llamaindex";

// update the url to point to your server
const URL = 'http://127.0.0.1:5000';
let client = new ToolboxClient(URL);

// these tools can be passed to your application!
const toolboxTools = await client.loadToolset('toolsetName');

// Define the basics of the tool: name, description, schema and core logic
const getTool = (toolboxTool) => tool({
    name: toolboxTool.getName(),
    description: toolboxTool.getDescription(),
    parameters: toolboxTool.getParamSchema(),
    execute: toolboxTool
});;

// Use these tools in your LlamaIndex applications
const tools = toolboxTools.map(getTool);

{{< /highlight >}}

For more detailed instructions on using the Toolbox Core SDK, see the
[README](https://github.com/googleapis/mcp-toolbox-sdk-js/blob/main/packages/toolbox-core/README.md).

{{% /tab %}}
{{% tab header="ADK TS" lang="en" %}}

{{< highlight javascript >}}
import { ToolboxClient } from '@toolbox-sdk/adk';

// Replace with the actual URL where your Toolbox service is running
const URL = 'http://127.0.0.1:5000';

let client = new ToolboxClient(URL);
const tools = await client.loadToolset();

// Use the client and tools as per requirement

{{< /highlight >}}

For detailed samples on using the Toolbox JS SDK with ADK JS, see the [README.](https://github.com/googleapis/mcp-toolbox-sdk-js/tree/main/packages/toolbox-adk/README.md)

{{% /tab %}}
{{< /tabpane >}}


#### Go

{{< tabpane text=true persist=header >}}
{{% tab header="Core" lang="en" %}}

Once you've installed the [Go Core SDK](https://pkg.go.dev/github.com/googleapis/mcp-toolbox-sdk-go/core), you can load
tools:

{{< highlight go >}}
package main

import (
	"context"
	"log"

	"github.com/googleapis/mcp-toolbox-sdk-go/core"
)

func main() {
	// update the url to point to your server
	URL := "http://127.0.0.1:5000"
	ctx := context.Background()

	client, err := core.NewToolboxClient(URL)
	if err != nil {
		log.Fatalf("Failed to create Toolbox client: %v", err)
	}

	// Framework agnostic tools
	tools, err := client.LoadToolset("toolsetName", ctx)
	if err != nil {
		log.Fatalf("Failed to load tools: %v", err)
	}
}
{{< /highlight >}}

{{% /tab %}}
{{% tab header="LangChain Go" lang="en" %}}

Once you've installed the [Go Core SDK](https://pkg.go.dev/github.com/googleapis/mcp-toolbox-sdk-go/core), you can load
tools:

{{< highlight go >}}
package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/googleapis/mcp-toolbox-sdk-go/core"
	"github.com/tmc/langchaingo/llms"
)

func main() {
	// Make sure to add the error checks
	// update the url to point to your server
	URL := "http://127.0.0.1:5000"
	ctx := context.Background()

	client, err := core.NewToolboxClient(URL)
	if err != nil {
		log.Fatalf("Failed to create Toolbox client: %v", err)
	}

	// Framework agnostic tool
	tool, err := client.LoadTool("toolName", ctx)
	if err != nil {
		log.Fatalf("Failed to load tools: %v", err)
	}

	// Fetch the tool's input schema
	inputschema, err := tool.InputSchema()
	if err != nil {
		log.Fatalf("Failed to fetch inputSchema: %v", err)
	}

	var paramsSchema map[string]any
	_ = json.Unmarshal(inputschema, &paramsSchema)

	// Use this tool with LangChainGo
	langChainTool := llms.Tool{
		Type: "function",
		Function: &llms.FunctionDefinition{
			Name:        tool.Name(),
			Description: tool.Description(),
			Parameters:  paramsSchema,
		},
	}
}
{{< /highlight >}}

For end-to-end samples on using the Toolbox Go SDK with LangChain Go, see the [module's samples](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main/core/samples)

{{% /tab %}}
{{% tab header="Genkit Go" lang="en" %}}

Once you've installed the [Go TBGenkit SDK](https://pkg.go.dev/github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit), you can load
tools:

{{< highlight go >}}
package main
import (
	"context"
	"encoding/json"
	"log"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/genkit"
	"github.com/googleapis/mcp-toolbox-sdk-go/core"
	"github.com/googleapis/mcp-toolbox-sdk-go/tbgenkit"
	"github.com/invopop/jsonschema"
)

func main() {
	// Make sure to add the error checks
	// Update the url to point to your server
	URL := "http://127.0.0.1:5000"
	ctx := context.Background()
	g, err := genkit.Init(ctx)

	client, err := core.NewToolboxClient(URL)
	if err != nil {
		log.Fatalf("Failed to create Toolbox client: %v", err)
	}

	// Framework agnostic tool
	tool, err := client.LoadTool("toolName", ctx)
	if err != nil {
		log.Fatalf("Failed to load tools: %v", err)
	}

	// Convert the tool using the tbgenkit package
 	// Use this tool with Genkit Go
	genkitTool, err := tbgenkit.ToGenkitTool(tool, g)
	if err != nil {
		log.Fatalf("Failed to convert tool: %v\n", err)
	}
}
{{< /highlight >}}
For end-to-end samples on using the Toolbox Go SDK with Genkit Go, see the [module's samples](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main/tbgenkit/samples)

{{% /tab %}}
{{% tab header="Go GenAI" lang="en" %}}

Once you've installed the [Go Core SDK](https://pkg.go.dev/github.com/googleapis/mcp-toolbox-sdk-go/core), you can load
tools:

{{< highlight go >}}
package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/googleapis/mcp-toolbox-sdk-go/core"
	"google.golang.org/genai"
)

func main() {
	// Make sure to add the error checks
	// Update the url to point to your server
	URL := "http://127.0.0.1:5000"
	ctx := context.Background()

	client, err := core.NewToolboxClient(URL)
	if err != nil {
		log.Fatalf("Failed to create Toolbox client: %v", err)
	}

	// Framework agnostic tool
	tool, err := client.LoadTool("toolName", ctx)
	if err != nil {
		log.Fatalf("Failed to load tools: %v", err)
	}

	// Fetch the tool's input schema
	inputschema, err := tool.InputSchema()
	if err != nil {
		log.Fatalf("Failed to fetch inputSchema: %v", err)
	}

	var schema *genai.Schema
	_ = json.Unmarshal(inputschema, &schema)

	funcDeclaration := &genai.FunctionDeclaration{
		Name:        tool.Name(),
		Description: tool.Description(),
		Parameters:  schema,
	}

	// Use this tool with Go GenAI
	genAITool := &genai.Tool{
		FunctionDeclarations: []*genai.FunctionDeclaration{funcDeclaration},
	}
}
{{< /highlight >}}
For end-to-end samples on using the Toolbox Go SDK with Go GenAI, see the [module's samples](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main/core/samples)

{{% /tab %}}

{{% tab header="OpenAI Go" lang="en" %}}

Once you've installed the [Go Core SDK](https://pkg.go.dev/github.com/googleapis/mcp-toolbox-sdk-go/core), you can load
tools:

{{< highlight go >}}
package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/googleapis/mcp-toolbox-sdk-go/core"
	openai "github.com/openai/openai-go"
)

func main() {
	// Make sure to add the error checks
	// Update the url to point to your server
	URL := "http://127.0.0.1:5000"
	ctx := context.Background()

	client, err := core.NewToolboxClient(URL)
	if err != nil {
		log.Fatalf("Failed to create Toolbox client: %v", err)
	}

	// Framework agnostic tool
	tool, err := client.LoadTool("toolName", ctx)
	if err != nil {
		log.Fatalf("Failed to load tools: %v", err)
	}

	// Fetch the tool's input schema
	inputschema, err := tool.InputSchema()
	if err != nil {
		log.Fatalf("Failed to fetch inputSchema: %v", err)
	}

	var paramsSchema openai.FunctionParameters
	_ = json.Unmarshal(inputschema, &paramsSchema)

	// Use this tool with OpenAI Go
	openAITool := openai.ChatCompletionToolParam{
		Function: openai.FunctionDefinitionParam{
			Name:        tool.Name(),
			Description: openai.String(tool.Description()),
			Parameters:  paramsSchema,
		},
	}
}
{{< /highlight >}}
For end-to-end samples on using the Toolbox Go SDK with OpenAI Go, see the [module's samples](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main/core/samples)

{{% /tab %}}

{{% tab header="ADK Go" lang="en" %}}

Once you've installed the [Go TBADK SDK](https://pkg.go.dev/github.com/googleapis/mcp-toolbox-sdk-go/tbadk), you can load
tools:

{{< highlight go >}}
package main

import (
  	"context"
  	"fmt"
  	"github.com/googleapis/mcp-toolbox-sdk-go/tbadk"
)

func main() {
  	// Make sure to add the error checks
	// Update the url to point to your server
	URL := "http://127.0.0.1:5000"
	ctx := context.Background()
  	client, err := tbadk.NewToolboxClient(URL)
  	if err != nil {
		return fmt.Sprintln("Could not start Toolbox Client", err)
  	}

  	// Use this tool with ADK Go
  	tool, err := client.LoadTool("toolName", ctx)
  	if err != nil {
		return fmt.Sprintln("Could not load Toolbox Tool", err)
  	}
}

{{< /highlight >}}

For end-to-end samples on using the Toolbox Go SDK with ADK Go, see the [module's samples](https://github.com/googleapis/mcp-toolbox-sdk-go/tree/main/tbadk/samples)

{{% /tab %}}
{{< /tabpane >}}

For more detailed instructions on using the Toolbox Go SDK, see the
[README](https://github.com/googleapis/mcp-toolbox-sdk-go/blob/main/core/README.md).

For more details, see the [Agent Skills guide](https://mcp-toolbox.dev/documentation/configuration/skills/).

## Supported MCP Version

Toolbox is fully compatible with the Model Context Protocol (MCP) and maintains support for multiple protocol revisions to ensure seamless integration with most MCP clients.

### Stable Releases
The following official MCP specification versions are currently supported for production use:

* `2026-07-28`
* `2025-11-25`
* `2025-06-18`
* `2025-03-26`
* `2024-11-05`

{{< notice note >}}
If no protocol version is negotiated or provided, the Toolbox server defaults to
the legacy custom transport protocol established on `2024-11-05`, which does not
require initialization.
{{< /notice >}}

### Draft Specifications
We actively develop against upcoming protocol specifications. You can opt-in to
test these forthcoming revisions before their official release using the
`--enable-draft-specs` flag during server startup.

To test these draft specifications, enable the startup flag and use this version
string during negotiation:
* No supported draft right now

Description: Enables experimental support for upcoming draft MCP specifications,
allowing you to test new schema standards and transport adjustments before they
become stable.

{{< notice note >}}
Once the draft specification is finalized and released as a stable version, the
draft implementation will be permanently removed. The flag itself will not be
removed, but its functionality will remain dormant (having no effect on the
server) until a new draft specification becomes available.

There will be no automatic redirects or backwards compatibility from the draft
spec to the stable release. Developers must manually migrate their clients to
the stable version once it is available. Do not use this flag in production
environments.
{{< /notice >}}
