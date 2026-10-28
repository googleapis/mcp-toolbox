# MCP Toolbox for Databases

A CLI tool for running a toolbox server.

## Installation

You can install the toolbox globally:

```bash
npm install -g @toolbox-sdk/server
```

Or run it directly using npx:

```bash
npx @toolbox-sdk/server
```

## Configuration

Toolbox supports two ways to define tools: **Prebuilt Sources** for instant database access and **Custom YAML** for tailored logic.

### 1. Prebuilt Tools
Use the `--prebuilt` flag to skip configuration files and instantly expose standard database operations.

*   **Usage:** `npx @toolbox-sdk/server --prebuilt <source> --stdio`
*   **Supported:** [See all supported datasource](https://mcp-toolbox.dev/documentation/configuration/prebuilt-configs/).
*   **Setup:** Requires relevant environment variables (e.g., `BIGQUERY_PROJECT` or `POSTGRES_HOST`).

### 2. Custom Tools (`tools.yaml`)
The toolbox requires a `tools.yaml` file in the current working directory to define sources, tools, prompts, resources, and groups.

*   **Usage:** The server loads `tools.yaml` automatically, or use `--config <path>`.
*   **Function:** Defines `sources` (connection details), `tools` (specific SQL/logic with descriptions for the LLM), prompts, and resources.

#### Example `tools.yaml`

```yaml
sources:
  my-pg-source:
    kind: postgres
    host: 127.0.0.1
    port: 5432
    database: toolbox_db
    user: postgres
    password: password
tools:
  search-hotels-by-name:
    kind: postgres-sql
    source: my-pg-source
    description: Search for hotels based on name.
    parameters:
      - name: name
        type: string
        description: The name of the hotel.
    statement: SELECT * FROM hotels WHERE name ILIKE '%' || $1 || '%';
prompts:
  code-review:
    description: "Asks the LLM to analyze code quality and suggest improvements."
    messages:
      - role: "user"
        content: "Please review the following code for quality, correctness, and potential improvements: \n\n{{.code}}"
    arguments:
      - name: "code"
        description: "The code to review"
        required: true
resources:
  db-schema-ddl:
    type: text
    description: "PostgreSQL DDL schema for customer tables."
    mimeType: text/x-sql
    text: |
      CREATE TABLE users (
        id SERIAL PRIMARY KEY,
        name VARCHAR(255) NOT NULL,
        email VARCHAR(255) UNIQUE NOT NULL
      );
```

To learn more on how to configure your toolbox, visit the [official docsite](https://mcp-toolbox.dev/documentation/configuration/).

## Platform Support

The toolbox automatically handles platform-specific binaries. Supported platforms include:
- macOS (arm64, x64)
- Linux (x64)
- Windows (x64, arm64)

## Resources

For more information, visit the 
- [MCP Toolbox repository](https://github.com/googleapis/mcp-toolbox)
- [Official Documentation](https://mcp-toolbox.dev/documentation)