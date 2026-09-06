---
title: "SQL Commenter"
type: docs
weight: 10
description: >
  Propagate application context into database query logs by prepending SQLCommenter-format comments to executed SQL statements.
---

[SQLCommenter](https://google.github.io/sqlcommenter/) is an open-source
convention that propagates application context into the database by prepending a
structured comment to every SQL statement before it is executed. The comment is
stripped before query planning, so it has no effect on results — but it shows up
verbatim in database query logs and slow-query logs, letting you correlate a
specific SQL statement back to the MCP client, LLM tool invocation, model, user,
agent, and distributed trace that triggered it.

This closes the observability gap between the application-level traces emitted by
Toolbox and the database-level logs emitted by your underlying database engine.

## Enabling SQL Commenter

SQL Commenter is opt-in and disabled by default. Enable it on a per-source basis by setting the `sqlCommenter` field to `true` in a source's configuration file:

```yaml
sources:
  - name: my-pg-source
    type: postgres
    # ...
    sqlCommenter: true
```

If you are exporting telemetry using OpenTelemetry, the `traceparent` attribute embedded in the SQL comment will automatically be part of the same distributed trace exported by Toolbox:

```bash
./toolbox --telemetry-otlp="127.0.0.1:4553"
```

## Supported Sources

SQL Commenter is supported on the following database sources:

* `alloydb-postgres`
* `bigquery` (as [job labels](../../integrations/bigquery/source.md#sql-commenter-job-labels))
* `cloud-sql-postgres`
* `cloud-sql-mysql`
* `postgres`
* `mysql`
* `sqlite`

## Comment Format

When enabled, Toolbox prepends a SQLCommenter-format comment to every SQL
statement executed against a supported source. Keys are sorted alphabetically,
values are URL-encoded, and pairs are joined by commas inside a `/* … */`
block. For example:

```sql
/*client='toolbox-langchain-python%2Fv0.1.0',client.model='gemini-2.5-flash',db.system.name='postgresql',server='genai-toolbox%2F1.1.0',tool.name='search_user',traceparent='00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01'*/ SELECT * FROM users WHERE id = $1;
```

## Attributes

| **Attribute**       | **Source**                                                                                       | **Description**                                                                                |
|---------------------|--------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `traceparent`       | Active OpenTelemetry span context.                                                               | W3C Trace Context header tying the SQL statement to the distributed trace for this invocation. |
| `server`            | Toolbox build (`genai-toolbox/<version>`).                                                       | Identifies the Toolbox server name and version that issued the query.                          |
| `tool.name`         | The tool being invoked.                                                                          | The tool whose execution triggered the SQL.                                                    |
| `db.system.name`    | The database engine (e.g. `postgresql`, `mysql`, `sqlite`).                                      | Identifies the database backend.                                                               |
| `client`            | MCP client `params._meta["dev.mcp-toolbox/telemetry"]["client.name"]` and/or `["client.version"]` (joined by `/`). | Identifies the ADK or application that initiated the MCP request.                              |
| `client.model`      | MCP client `params._meta["dev.mcp-toolbox/telemetry"]["client.model"]`.                                 | The LLM model that produced the tool call.                                                     |
| `client.user.id`    | MCP client `params._meta["dev.mcp-toolbox/telemetry"]["client.user.id"]`.                               | End-user identifier supplied by the client.                                                    |
| `client.agent.id`   | MCP client `params._meta["dev.mcp-toolbox/telemetry"]["client.agent.id"]`.                              | Agent identifier supplied by the client.                                                       |

Client-supplied attributes (`client`, `client.model`, `client.user.id`,
`client.agent.id`) are populated from the MCP request's
`params._meta["dev.mcp-toolbox/telemetry"]` field. Attributes that are not
provided by the client or not applicable in the current context are omitted
from the comment.

## Populating Client Attributes from SDKs

Toolbox SDKs that support the `dev.mcp-toolbox/telemetry` meta field will
populate `client.*` attributes automatically. With the Python SDK, you can
attach per-tool attributes such as model name, user ID, and agent ID using
`TelemetryAttributes`. See [Per-call Telemetry Attributes](../../connect-to/toolbox-sdks/python-sdk/core/#per-call-telemetry-attributes)
for details.

{{< notice tip >}}
The comment is plain text in your database logs. To follow a slow query back to
its trace, take the `traceparent` value and search your tracing backend for the
matching trace ID (the second segment of the W3C `traceparent`).
{{< /notice >}}
