---
name: getting-started
description: >-
  Set up MCP Toolbox for Databases from zero: install the server, choose
  prebuilt vs custom tools, write a tools.yaml, run and verify it, then connect
  it to an agent over MCP or the Python/JS/Go SDKs. Use when someone wants to
  start with MCP Toolbox, connect an agent or LLM to a database (Postgres,
  MySQL, BigQuery, Spanner, AlloyDB, SQLite, Looker, Neo4j and ~30 more), asks
  how to get started with Toolbox, is writing their first tools.yaml, or is
  deciding how their agent should reach their database.
---

# Get started with MCP Toolbox for Databases

Toolbox is a server that sits between an agent and a database. You declare tools
in YAML; Toolbox serves them over MCP and through language SDKs, and owns the
connection pooling, auth, and parameter binding.

Work the steps in order. Step 0 shapes everything after it.

## Step 0: Settle two choices

**Which data source?** Toolbox ships integrations for ~40 sources. Confirm the
user's before writing config: the source `type` and the tool `type` both depend
on it.

**Prebuilt or custom tools?** The choice people most often get wrong.

| | Prebuilt (`--prebuilt`) | Custom (`tools.yaml`) |
|---|---|---|
| What the agent gets | Generic tools: `execute_sql`, `list_tables`, … | Only the tools you declare |
| SQL | Agent writes arbitrary SQL at runtime | You write the SQL; agent supplies typed parameters |
| Set up | Env vars, no config file | You author the config |
| Fits | Exploring a database, IDE/CLI assistants, demos | Applications, anything reaching production |

Follow the **Fits** row. What custom buys you is the whole reason to run Toolbox
instead of handing an LLM a database credential: a fixed statement with typed
parameters.

The two compose: `--prebuilt postgres --config tools.yaml` loads both. Names
must not collide across them or startup fails.

## Step 1: Install the server

`npx` is the fastest way to a running server and needs no install. Prefer the
binary or container for anything long-lived.

<!-- {x-release-please-start-version} -->
```bash
# No install (Node.js required)
npx @toolbox-sdk/server --config tools.yaml

# Homebrew
brew install mcp-toolbox

# Binary. Pick your OS/arch: linux/amd64, darwin/arm64, darwin/amd64, windows/amd64, windows/arm64
export OS="darwin/arm64"
curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.9.0/$OS/toolbox
chmod +x toolbox
```
<!-- {x-release-please-end} -->

Container images are at
`us-central1-docker.pkg.dev/database-toolbox/toolbox/toolbox`. Full matrix,
signature verification, and `go install` are in the [README][readme].

## Step 2: Configure

### Prebuilt path

No file. Export the source's env vars and name the source:

```bash
export POSTGRES_HOST=127.0.0.1 POSTGRES_PORT=5432
export POSTGRES_DATABASE=mydb POSTGRES_USER=myuser POSTGRES_PASSWORD=...
toolbox --prebuilt postgres
```

`toolbox --help` prints the authoritative list of prebuilt sources; read it from
the binary rather than guessing a name. `--prebuilt` repeats or takes a
comma-separated list, and `source/toolset` selects a subset
(e.g. `--prebuilt postgres,bigquery`, `--prebuilt sqlite/sqlite_database_tools`).

### Custom path

`tools.yaml` is a multi-document YAML file, documents separated by `---`. Every
document carries a `kind`: `source`, `tool`, `toolset`, `authService`,
`embeddingModel`, `prompt`, or `group`.

```yaml
kind: source
name: my-pg-source
type: postgres
host: 127.0.0.1
port: 5432
database: ${POSTGRES_DATABASE}
user: ${POSTGRES_USER}
password: ${POSTGRES_PASSWORD}
---
kind: tool
name: search_hotels_by_name
type: postgres-sql
source: my-pg-source
description: Search for hotels based on name.
parameters:
  - name: name
    type: string
    description: The name of the hotel.
statement: SELECT * FROM hotels WHERE name ILIKE '%' || $1 || '%';
---
kind: toolset
name: my-toolset
tools:
  - search_hotels_by_name
```

Each rule below is a common first-run failure:

- **A tool's `source` must match a source's `name`.** That is the only link
  between them.
- **Use `${ENV_VAR}`, never literal secrets.** `${VAR:default}` supplies a
  fallback.
- **Write `description` for the model, not for a human skimming the file.** It
  is what the model reads to decide whether to call the tool, and it affects
  agent behaviour more than anything else in the config.
- **Bind parameters, never concatenate.** They bind positionally into
  `statement` using the driver's placeholder syntax (`$1` for Postgres, `?` for
  MySQL/SQLite). Never string-concatenate user input.
- **A `toolset` exposes a named subset to a client.** Without one, everything is
  served.

**Legacy format:** older configs use top-level `sources:` / `tools:` maps
instead of `kind:`. Both still load. Run `toolbox migrate` to convert.

## Step 3: Run it

```bash
toolbox --config tools.yaml
```

Defaults to `127.0.0.1:5000` with dynamic config reload on. Flags you will
actually reach for:

| Flag | Effect |
|---|---|
| `--port`, `--address` | Move off `127.0.0.1:5000` |
| `--stdio` | Serve MCP over stdio instead of HTTP. Required by clients that spawn the server |
| `--ui` | Serve the Toolbox UI at `/ui` for browsing and running tools |
| `--log-level DEBUG` | First thing to try when a connection fails |
| `--disable-reload` | Turn off config hot-reload |

Endpoints: `/mcp` (MCP, always on), `/healthz`, `/ui` (with `--ui`), and `/api`
(only with `--enable-api`).

## Step 4: Verify before wiring an agent

Do this before touching agent code: it separates a config problem from an agent
problem.

```bash
# Runs the tool directly against the database. No server, no agent.
toolbox invoke search_hotels_by_name '{"name": "Hilton"}' --config tools.yaml
```

Works with `--prebuilt` too. If this fails, the problem is credentials,
connectivity, or SQL. Fix it here. If it succeeds and the agent still gets
nothing, the problem is the client wiring in Step 5.

## Step 5: Connect

**As an MCP server.** Point any MCP client at `http://127.0.0.1:5000/mcp`, or
have it spawn `toolbox --config tools.yaml --stdio`. Check what the server is
actually serving before you blame the client:

```bash
curl -s -X POST http://127.0.0.1:5000/mcp \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
```

No initialize handshake is required. Swap the body for
`"method":"tools/call","params":{"name":"<tool>","arguments":{}}` to run a tool
over the wire.

**From application code**, when the agent is something you are writing:

| Language | Install | Notes |
|---|---|---|
| Python | `pip install toolbox-core` | `toolbox-langchain`, `toolbox-llamaindex`, or `google-adk[toolbox]` for framework bindings |
| JS/TS | `npm install @toolbox-sdk/core` | `@toolbox-sdk/adk` for ADK |
| Go | `go get github.com/googleapis/mcp-toolbox-sdk-go/core` | Modular since v0.6.0 |

The SDK loads a toolset by name and hands the framework native tool objects, so
you do not hand-write tool schemas. See the [Python][py], [JS][js], and
[Go][go] quickstarts for runnable agents.

## When it does not work

- **Server will not start.** Almost always the config. The error names the
  document and key. Check `kind` is present on every document and that each
  tool's `source` matches a declared source name.
- **Agent sees no tools.** You exposed a `toolset` the client did not ask for,
  or the client is pointed at `/` instead of `/mcp`.
- **Agent sees tools but picks wrong.** The `description` fields are the fix,
  not the tool logic.
- **Connection refused from a container.** `127.0.0.1` is the default bind. Use
  `--address 0.0.0.0`.

## Reference

<!-- {x-release-please-start-version} -->
[readme]: https://github.com/googleapis/mcp-toolbox/blob/v1.9.0/README.md
[py]: https://mcp-toolbox.dev/v1.9.0/documentation/getting-started/local_quickstart/
[js]: https://mcp-toolbox.dev/v1.9.0/documentation/getting-started/local_quickstart_js/
[go]: https://mcp-toolbox.dev/v1.9.0/documentation/getting-started/local_quickstart_go/

- [Configuration reference](https://mcp-toolbox.dev/v1.9.0/documentation/configuration/)
- [Prebuilt configs](https://mcp-toolbox.dev/v1.9.0/documentation/configuration/prebuilt-configs/)
- [All sources and tools](https://mcp-toolbox.dev/v1.9.0/integrations/)
- [Deploying Toolbox](https://mcp-toolbox.dev/v1.9.0/documentation/deploy-to/)
<!-- {x-release-please-end} -->
