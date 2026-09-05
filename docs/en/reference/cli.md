---
title: "CLI"
type: docs
weight: 1
description: >
  This page describes the `toolbox` command-line options.
---

## Reference

| Flag (Short) | Flag (Long)                | Description                                                                                                                                                               | Default     |
|--------------|----------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------|
| `-a`         | `--address`                | Address of the interface the server will listen on.                                                                                                                       | `127.0.0.1` |
|              | `--disable-ext`            | Specifies MCP extension URIs disabled on this server.                                                                                                                     |             |
|              | `--disable-reload`         | Disables dynamic reloading config.                                                                                                                                        |             |
| `-h`         | `--help`                   | help for toolbox                                                                                                                                                          |             |
|              | `--http-max-request-bytes` | Maximum MCP HTTP request body size in bytes.                                                                                                                              | `10485760`  |
|              | `--ignore-unknown-tools`   | Log warnings and skip unknown/unsupported tool types instead of failing to start.                                                                                         |             |
|              | `--log-level`              | Specify the minimum level logged. Allowed: 'DEBUG', 'INFO', 'WARN', 'ERROR'.                                                                                              | `info`      |
|              | `--logging-format`         | Specify logging format to use. Allowed: 'standard' or 'JSON'.                                                                                                             | `standard`  |
|              | `--mcp-prm-file`           | Path to a manual Protected Resource Metadata (PRM) JSON file. If provided, overrides auto-generation for MCP Server-Wide Authentication.                                  |             |
| `-p`         | `--port`                   | Port the server will listen on.                                                                                                                                           | `5000`      |
|              | `--tls-cert`               | Path to the PEM-encoded TLS certificate file.                                                                                                                             |             |
|              | `--tls-key`                | Path to the PEM-encoded TLS private key file.                                                                                                                             |             |
|              | `--toolbox-url`            | Specifies the absolute Toolbox URL (e.g., `https://my-toolbox.example.com`). Used as the resource field in the MCP PRM file when MCP Auth is enabled. Falls back to `TOOLBOX_URL` environment variable. |             |
|              | `--prebuilt`               | Use one or more prebuilt tool configuration by source type. Optionally specify a toolset suffix (e.g., `<source>/<toolset>`) to load only that toolset. These prebuilt configs are intended for 'build-time' use cases, where agents are helping trusted developers build things. They are not secure enough for 'run time' use cases, where the agent will be talking to potentially untrusted developers. See [Prebuilt Tools Reference](../documentation/configuration/prebuilt-configs/_index.md) for allowed values. |             |
|              | `--stdio`                  | Listens via MCP STDIO instead of acting as a remote HTTP server.                                                                                                          |             |
|              | `--telemetry-gcp`          | Enable exporting directly to Google Cloud Monitoring.                                                                                                                     |             |
|              | `--telemetry-gcp-project`  | Google Cloud project ID used for `--telemetry-gcp`; defaults to `GOOGLE_CLOUD_PROJECT` if not set.                                                                        |             |
|              | `--telemetry-otlp`         | Enable exporting using OpenTelemetry Protocol (OTLP) to the specified endpoint (e.g. 'http://127.0.0.1:4318')                                                             |             |
|              | `--telemetry-service-name` | Sets the value of the service.name resource attribute for telemetry data.                                                                                                 | `toolbox`   |
|              | `--sql-commenter`          | Prepend SQLCommenter-format comments (traceparent, server, tool.name, db.system.name, client metadata from `_meta["dev.mcp-toolbox/telemetry"]`) to executed SQL.         |             |
|              | `--config`                 | File path specifying the tool configuration. Cannot be used with --configs or --config-folder.                                                                            |             |
|              | `--configs`                | Multiple file paths specifying tool configurations. Files will be merged. Cannot be used with --config or --config-folder.                                                |             |
|              | `--config-folder`          | Directory path containing YAML tool configuration files. All .yaml and .yml files in the directory will be loaded and merged. Cannot be used with --config or --configs.  |             |
|              | `--ui`                     | Launches the Toolbox UI web server.                                                                                                                                       |             |
|              | `--allowed-origins`        | Specifies a list of origins permitted to access this server for CORs access.                                                                                              | `*`         |
|              | `--allowed-hosts`          | Specifies a list of hosts permitted to access this server to prevent DNS rebinding attacks.                                                                               | `*`         |
|              | `--user-agent-metadata`    | Appends additional metadata to the User-Agent.                                                                                                                            |             |
|              | `--poll-interval`          | Specifies the polling frequency (seconds) for configuration file updates.                                                                                                 | `0`         |
|              | `--enable-draft-specs`     | Opt-in and test upcoming draft MCP specifications.                                                                                                                        | `false`     |
|              | `--tool-suggestions`       | Controls what an unknown-tool error discloses so an agent can self-correct: `full` (the nearest matching name and the available tool names for the connected toolset), `nearest` (the nearest matching name only), or `off` (neither). Applies to MCP `tools/call` only, where the connected toolset bounds what may be revealed; the `/api` endpoints have no toolset scope and never disclose tool names regardless of this setting. | `full`      |
| `-v`         | `--version`                | version for toolbox                                                                                                                                                       |             |

## Sub Commands

<details>
<summary><code>invoke</code></summary>

Executes a tool directly with the provided parameters. This is useful for testing tool configurations and parameters without needing a full client setup.

**Syntax:**

```bash
toolbox invoke <tool-name> [params]
```

**Arguments:**

- `tool-name`: The name of the tool to execute (as defined in your configuration).
- `params`: (Optional) A JSON string containing the parameters for the tool.

For more detailed instructions, see [Invoke Tools via CLI](../documentation/configuration/tools/invoke_tool.md).

</details>

<details>
<summary><code>migrate</code></summary>

Rewrites configuration files from the legacy nested format (top-level `sources:`, `tools:`, `toolsets:` maps) into the flat format, where each resource is its own YAML document with a `kind` field.

This subcommand also converts `toolset` primitive into `group` primitive.

**Syntax:**

```bash
toolbox migrate --config <path>
```

**Flags:**

- `--config`: (Optional) Path to a configuration file to migrate. Defaults to `tools.yaml` when no other config flag is set.
- `--configs`: (Optional) Comma-separated list of configuration files to migrate.
- `--config-folder`: (Optional) Path to a folder whose `.yaml` and `.yml` files should be migrated.
- `--dry-run`: (Optional) Print the migrated configuration to stdout instead of writing it.

`--config`, `--configs`, and `--config-folder` are mutually exclusive. Each file is rewritten in place, with the original kept alongside it with `.bak` appended (e.g., `tools.yaml.bak`); a file that needs no changes is left as is. Comments other than top-level ones are not preserved, so review the result before deleting the backup.

</details>

<details>
<summary><code>skills-generate</code></summary>

Generates a skill package from a specified toolset or group. Each tool in the collection will have a corresponding Node.js execution script in the generated skill.

**Syntax:**

```bash
toolbox skills-generate --name <name> --description <description> --toolset <toolset> --output-dir <output>
```

**Flags:**

- `--name`: (Optional) Name of the generated skill. When multiple toolsets are generated because `--toolset` is omitted, this name acts as a prefix for each skill folder (e.g., `<name>-<toolset>`). When omitted in a single-skill mode, the name defaults, in order, to: the `--group` name, then the `--toolset` name, then the single `--prebuilt` config name; any other case requires `--name`.
- `--description`: (Optional) Description of the generated skill. When a group defines its own `description`, that takes precedence and `--description` acts as a fallback.
- `--group`: (Optional) Name of the group to convert into a single skill. Uses the group's `description`, falling back to `--description`. Mutually exclusive with `--toolset`.
- `--toolset`: (Optional) Name of the toolset to convert into a skill. If not provided, one skill will be generated for every custom toolset defined. If no custom toolsets are defined, it defaults to a single skill containing all tools.
- `--output-dir`: (Optional) Directory to output generated skills (default: "skills").
- `--license-header`: (Optional) Optional license header to prepend to generated node scripts.
- `--additional-notes`: (Optional) Additional notes to add under the Usage section of the generated SKILL.md.
- `--invocation-mode`: (Optional) Invocation mode for the generated scripts: 'binary' or 'npx' (default: "npx").
- `--toolbox-version`: (Optional) Version of @toolbox-sdk/server to use for npx approach (defaults to current toolbox version).

For more detailed instructions, see [Generate Agent Skills](../documentation/configuration/skills/_index.md).

</details>

## Examples

### Hardening Toolbox

Toolbox is designed for flexibility, but security should not be ignored—even in
local development. When exposing the server to a network or running it alongside
a web browser, use these configurations to protect your data and system.

#### Host Validation & DNS Rebinding Protection
The `--allowed-hosts` flag controls which Host headers the server accepts.
Restricting this is the primary defense against DNS Rebinding attacks.

* Flag: `--allowed-hosts`
* Local Development: Set to localhost or 127.0.0.1.
* Production: Set to your specific FQDN (e.g., toolbox.example.com).
* Example:
  ```
  ./toolbox --allowed-hosts="localhost,127.0.0.1"
  ```


{{< notice tip >}}
**The "Local" Fallacy:** Using `--allowed-hosts="*"` is unsafe even on localhost. A
malicious website can trick your browser into making requests to `127.0.0.1`,
effectively bypassing the browser's security to control your local Toolbox.
{{< /notice >}}

#### Cross-Origin Resource Sharing (CORS)
The `--allowed-origins` flag dictates which web applications (frontends) are
permitted to communicate with your Toolbox API.

* Flag: `--allowed-origins`
* Recommendation: Avoid `*` in any environment containing sensitive data. Explicitly list your trusted frontend URLs.
* Example: 
  ```
  ./toolbox --allowed-origins="https://my-mcp-ui.internal.com"
  ```

#### Transport Layer Security (TLS/HTTPS)
By default, traffic is unencrypted (HTTP). In production or shared networks, you must enable TLS to prevent Man-in-the-Middle (MitM) attacks and packet sniffing.

* Flag: `--tls-cert` and `--tls-key` (Both cert and key files are required for
  TLS activation)
* Protocol: Toolbox enforces TLS 1.2 as a minimum version to ensure modern encryption standards.
* Use Case: Use Certbot for public domains or mkcert for locally-trusted development certificates.
* Example:
  ```
  ./toolbox --tls-cert=cert.pem --tls-key=key.pem
  ```


### Transport Configuration

**Server Settings:**

- `--address`, `-a`: Server listening address (default: "127.0.0.1")
- `--port`, `-p`: Server listening port (default: 5000)

**STDIO:**

- `--stdio`: Run in MCP STDIO mode instead of HTTP server

#### Usage Examples

```bash
# Basic server with custom port configuration
./toolbox --config "tools.yaml" --port 8080

# Server with prebuilt + custom tools configurations
./toolbox --config tools.yaml --prebuilt alloydb-postgres

# Server with multiple prebuilt tools configurations
./toolbox --prebuilt alloydb-postgres,alloydb-postgres-admin
# OR
./toolbox --prebuilt alloydb-postgres --prebuilt alloydb-postgres-admin

# Server filtering a prebuilt configuration to load only a specific toolset
./toolbox --prebuilt alloydb-postgres/monitor
```

### Tool Configuration Sources

The CLI supports multiple mutually exclusive ways to specify tool configurations:

**Single File:** (default)

- `--config`: Path to a single YAML configuration file (default: `tools.yaml`)

**Multiple Files:**

- `--configs`: Comma-separated list of YAML files to merge

**Directory:**

- `--config-folder`: Directory containing YAML files to load and merge

**Prebuilt Configurations:**

- `--prebuilt`: Use one or more predefined configurations for specific database types (e.g.,
  'bigquery', 'postgres', 'spanner'), optionally appending a toolset name to filter the loaded tools (e.g., `alloydb-postgres/monitor`). These prebuilt configs are intended for 'build-time' use cases, where agents are helping trusted developers build things. They are not secure enough for 'run time' use cases, where the agent will be talking to potentially untrusted developers. See [Prebuilt Tools 
  Reference](../documentation/configuration/prebuilt-configs/_index.md) for allowed values.

{{< notice tip >}}
The CLI enforces mutual exclusivity between configuration source flags,
preventing simultaneous use of the file-based options ensuring only one of
`--config`, `--configs`, or `--config-folder` is
used at a time.
{{< /notice >}}

### Hot Reload

Toolbox supports two methods for detecting configuration changes: **Push**
(event-driven) and **Poll** (interval-based). To completely disable all hot
reloading, use the `--disable-reload` flag.

* **Push (Default):** Toolbox uses a highly efficient push system that listens
  for instant OS-level file events to reload configurations the moment you save.
* **Poll (Fallback):** Alternatively, you can use the
  `--poll-interval=<seconds>` flag to actively check for updates at a set
  cadence. Unlike the push system, polling "pulls" the file status manually,
  which is a great fallback for network drives or container volumes where OS
  events might get dropped. Set the interval to `0` to disable the polling
  system.

### Toolbox UI

To launch Toolbox's interactive UI, use the `--ui` flag. This allows you to test
tools and toolsets with features such as authorized parameters. To learn more,
visit [Toolbox UI](../documentation/configuration/toolbox-ui/index.md).

### Disabling MCP Extensions

By default, Toolbox advertises support for its own custom MCP extensions (e.g., `com.google.cloud/toolbox.v1`) during the client discovery phase. This extension signals to clients that they can leverage Toolbox-specific features that fall outside the official MCP specification (see the [Extension README](https://github.com/googleapis/mcp-toolbox/blob/main/extensions/2026-07-28/README.md) for a list of currently supported capabilities).

Disabling an extension removes it from the server's advertised capabilities. To disable specific extensions on the server, pass their URIs via the `--disable-ext` flag:

```bash
# Disable the Toolbox v1 extension
./toolbox --disable-ext com.google.cloud/toolbox.v1
```

