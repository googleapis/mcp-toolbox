---
title: "Groups"
type: docs
weight: 6
description: >
  Groups let you scope MCP primitives such as tools and prompts together under a single name, with a description used as group metadata.
---

A Group is a single named collection that scopes MCP primitives together — currently **tools** and **prompts**, with more (such as resources) planned. Where a [Toolset](../toolsets/) groups only tools, a group bundles these primitives under one name and one MCP endpoint, and carries a `description` that describes the collection.

Connecting to a group's endpoint (`/mcp/{name}`) scopes the corresponding MCP list methods (such as `tools/list` and `prompts/list`) to that group. Groups are also introspectable over MCP through methods which are available on the `com.google.cloud/toolbox.v1` [extension](https://github.com/googleapis/mcp-toolbox/tree/main/extensions).

## Defining Groups

Declare a group as a `kind: group` document in your configuration file. A group has the following fields:

| Field         | Required | Description                                                                                    |
| ------------- | -------- | -----------------------------------------------------------------------------------------------|
| `name`        | Yes\*    | Unique name for the group. Used as the endpoint path (`/mcp/{name}`).                          |
| `description` | No       | Human-readable description of the group, surfaced via `groups/list`.                           |
| `tools`       | No       | List of tool names to include in the group.                                                    |
| `prompts`     | No       | List of prompt names to include in the group.                                                  |
| `ttlMs`       | No       | Time-to-live in milliseconds for cached group list responses. ([MCP TTL Spec](https://modelcontextprotocol.io/specification/2026-07-28/server/utilities/caching#time-to-live-ttl-field )). Defaults to `300000` (5 minutes).|
| `cacheScope`  | No       | Cache visibility for group list responses (either `public` or `private`). ([MCP Cache Scope Spec](https://modelcontextprotocol.io/specification/2026-07-28/server/utilities/caching#cache-scope-field)). Defaults to `public`.|

\* `name` is required for every named group. The single [default group](#the-default-group) omits it.

As Toolbox adds support for more MCP primitives, groups will gain corresponding fields (for example, `resources`).

```yaml
kind: group
name: data_analyst
description: Tools and prompts for exploratory data analysis.
tools:
  - list_tables
  - execute_sql
prompts:
  - summarize_results
---
kind: group
name: admin
description: Administrative operations.
tools:
  - create_user
  - list_users
ttlMs: 3600000
cacheScope: private
```

## The default group

A single **default (nameless) group** always exists and contains **all** configured primitives (every tool and prompt). Connecting to the default MCP endpoint (`/mcp`) returns everything.

You may declare a `kind: group` document with no `name` to set a `description` for the default group. Because the default group always contains everything, it **cannot** declare `tools`, `prompts`, or any other primitive list:

```yaml
kind: group
description: All tools and prompts available on this server.
```

## Validation rules

At startup, Toolbox validates groups:

- **Unique names.** A named group must have a unique name that satisfies the standard name rules (alphanumeric characters, underscores, and hyphens).
- **One default group.** Declaring more than one nameless group is an error.
- **Default group restrictions.** The default group may set only a `description`; declaring `tools`, `prompts`, or any other primitive list on it is an error.
- **No duplicate names across toolsets and groups.** A `kind: toolset` is parsed as a group, so defining the same name as both a `kind: toolset` and a `kind: group` is a duplicate-name error.
- **Valid parameters.** If specified, `ttlMs` must be non-negative (>=0), and `cacheScope` must be either `public` or `private`.

## Relationship to toolsets

Groups are a superset of toolsets: a toolset is equivalent to a tools-only group. Existing `kind: toolset` configurations still load — Toolbox parses every toolset as a group — but that parity is not exact, so review the differences below before assuming a toolset behaves as it did:

- A toolset has no `description` of its own, so one written on a `kind: toolset` is dropped rather than promoted, and Toolbox logs a warning. To give a collection a description, declare it as a `kind: group`.
- Unrecognized fields on a toolset are rejected at startup instead of being silently ignored.
- Declaring the same name as both a `kind: toolset` and a `kind: group` is a duplicate-name error; previously the group silently took precedence.

We recommend migrating to a `kind: group` even for tools-only collections, so the configuration matches what Toolbox actually loads and can grow to scope prompts (and, in the future, other primitives) alongside tools. See [Toolsets](../toolsets/) for more.

To convert existing toolsets to groups automatically, run the `migrate` command. It rewrites both nested `toolsets:` blocks and already-flat `kind: toolset` documents to `kind: group`:

```bash
toolbox migrate --config tools.yaml
```

Use `--dry-run` to preview the changes without writing them.
