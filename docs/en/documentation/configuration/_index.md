---
title: "Configuration"
type: docs
weight: 3
description: >
  How to configure Toolbox's tools.yaml file.
---

The primary way to configure Toolbox is through the `tools.yaml` file. If you
have multiple files, you can tell toolbox which to load with the `--config
tools.yaml` flag.

### Using Environment Variables

To avoid hardcoding certain secret fields like passwords, usernames, API keys,
etc., you can use environment variables. Toolbox supports these forms:

| Syntax | Variable is unset | Variable is set to an empty string | Variable has a value |
| --- | --- | --- | --- |
| `${ENV_NAME}` | Error | Use the empty string | Use the value |
| `${ENV_NAME:default}` | Use `default` | Use the empty string | Use the value |
| `${ENV_NAME:-default}` | Use `default` | Use `default` | Use the value |
| `${ENV_NAME:?message}` | Error | Error | Use the value |

The message in `${ENV_NAME:?message}` is optional and is included in the
startup error when provided.

```yaml
user: ${USER_NAME}
password: ${PASSWORD}
```

A default value can be specified for an unset variable with
`${ENV_NAME:default}`. Use `${ENV_NAME:-default}` when an empty value should
also select the default.

```yaml
port: ${DB_PORT:3306}
host: ${DB_HOST:-localhost}
password: ${PASSWORD:?database password is required}
```

### Sources

The `source` kind of your `tools.yaml` defines what data source your
Toolbox should have access to. Most tools will have at least one source to
execute against.

```yaml
kind: source
name: my-pg-source
type: postgres
host: 127.0.0.1
port: 5432
database: toolbox_db
user: ${USER_NAME}
password: ${PASSWORD}
```

For more details on configuring different types of sources, see the
[Sources](./sources/_index.md).

### Tools

The `tool` kind of your `tools.yaml` defines the actions your agent can
take: what type of tool it is, which source(s) it affects, what parameters it
uses, etc.

```yaml
kind: tool
name: search-hotels-by-name
type: postgres-sql
source: my-pg-source
description: Search for hotels based on name.
parameters:
  - name: name
    type: string
    description: The name of the hotel.
statement: SELECT * FROM hotels WHERE name ILIKE '%' || $1 || '%';
```

For more details on configuring different types of tools, see the
[Tools](./tools/_index.md).

### Toolsets

The `toolset` kind of your `tools.yaml` allows you to define groups of tools
that you want to be able to load together. This can be useful for defining
different sets for different agents or different applications.

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

### Groups

The `group` kind scopes MCP primitives such as **tools** and **prompts**
together under one name, with a `description` used as group metadata. A toolset
is a tools-only group, so existing `kind: toolset` configs keep working
unchanged. See [Groups](./groups/_index.md) for details.

```yaml
kind: group
name: my_group
description: Tools and prompts for a specific task.
tools:
  - my_first_tool
prompts:
  - my_first_prompt
```

### Prompts

The `prompt` kind of your `tools.yaml` defines the templates containing
structured messages and instructions for interacting with language models.

```yaml
kind: prompt
name: code_review
description: "Asks the LLM to analyze code quality and suggest improvements."
messages:
  - content: "Please review the following code for quality, correctness, and potential improvements: \n\n{{.code}}"
arguments:
  - name: "code"
    description: "The code to review"
```

For more details on configuring different types of prompts, see the
[Prompts](./prompts/_index.md).

### Read-Only Configuration

Toolbox provides mechanisms to ensure data safety and prevent unintended modifications. Here is how you can configure read-only access and ensure safety:

#### Custom Tools and SQL Injection Protection

When creating custom tools (e.g., `postgres-sql` or `mysql-sql`), you should protect them from SQL injections by using parameterized queries. This ensures that the tools only execute the intended query structure and cannot be manipulated to perform data modification operations.

- **Always use placeholders** (like `$1`, `$2` for Postgres or `?` for MySQL) to pass parameters to your SQL statements.
- **Avoid constructing dynamic SQL** that interpolates user input directly.

**Example (Safe Parameterized Query):**

```yaml
kind: tool
name: search-hotels-by-name
type: postgres-sql
source: my-pg-source
description: Search for hotels based on name.
parameters:
  - name: name
    type: string
    description: The name of the hotel.
statement: SELECT * FROM hotels WHERE name ILIKE '%' || $1 || '%';
```

#### BigQuery Source Read-Only Mode

For BigQuery sources, you can configure read-only access at the source level. This provides a hard boundary at the source connection level, ensuring that no modification operations can be performed regardless of the tool configuration.

#### Database Permissions

You can further increase safety by making sure the credentials used by Toolbox only have read-only permissions to the database. This ensures that even if a tool is misconfigured or compromised, the database will reject any data modification attempts. This is the most effective way to enforce read-only behavior.

---

## Explore Configuration Modules
