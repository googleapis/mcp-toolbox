---
title: "Serving Skills over MCP"
type: docs
weight: 2
description: >
  Publish an Agent Skill through the io.modelcontextprotocol/skills extension, so a
  connected client discovers the skill and reads its files.
---

## Overview

Toolbox serves an Agent Skill as a set of ordinary [resources](../resources/). The
`io.modelcontextprotocol/skills` extension adds two methods on top of them:

- `skills/list` returns every skill the server declares.
- `skills/get` returns one skill, addressed by the URI of its `SKILL.md`. An
  unknown URI returns error code `-32602`.

Both methods return metadata only. Each entry carries the `SKILL.md` frontmatter,
and the URI, `sha256:` digest, and byte size of every file in the skill. The client
reads the content of a file with `resources/read`, and compares the bytes against
the digest.

A client that does not negotiate the extension still sees the same files as
ordinary resources. Toolbox advertises extensions only for MCP protocol version
`2026-07-28`.

## The skill:// convention

A resource joins a skill through its URI. Give the resource a URI of the form
`skill://<skill-path>/<file>`:

- The resource at `skill://<skill-path>/SKILL.md` declares a skill.
- Every other resource under the same `skill://<skill-path>/` prefix is a file of
  that skill.

A `skill://` resource with no `SKILL.md` at its skill path joins no skill.
Toolbox serves it as an ordinary resource, and reports no error.

The skill path holds one or more segments. `skill://guide/SKILL.md` and
`skill://team/guide/SKILL.md` both declare a skill.

The frontmatter `name` must equal the last segment of the skill path.
`skill://team/guide/SKILL.md` therefore requires `name: guide`.

Only concrete resources (`kind: resource`) become files of a skill. A
[resource template](../resources/template/) accepts a `skill://` URI template, but
Toolbox does not count the files it matches as members of a skill.

## Example

Declare one `kind: resource` entry for each file of the skill:

```yaml
kind: resource
name: deploy-runbook
type: file
description: "How to deploy the billing service."
path: "./skills/deploy-runbook/SKILL.md"
uri: "skill://deploy-runbook/SKILL.md"
---
kind: resource
name: deploy-runbook-rollback
type: file
description: "Rollback steps for the billing service."
path: "./skills/deploy-runbook/reference/rollback.md"
uri: "skill://deploy-runbook/reference/rollback.md"
```

The `SKILL.md` on disk opens with YAML frontmatter, and its `name` matches the
skill path:

```markdown
---
name: deploy-runbook
description: "How to deploy the billing service, and how to roll it back."
---

Read `reference/rollback.md` before you start a rollback.
```

`skills/list` then returns one entry:

```json
{
  "skills": [
    {
      "uri": "skill://deploy-runbook/SKILL.md",
      "frontmatter": {
        "name": "deploy-runbook",
        "description": "How to deploy the billing service, and how to roll it back."
      },
      "resources": [
        {
          "uri": "skill://deploy-runbook/SKILL.md",
          "digest": "sha256:...",
          "size": 184
        },
        {
          "uri": "skill://deploy-runbook/reference/rollback.md",
          "digest": "sha256:...",
          "size": 2210
        }
      ]
    }
  ]
}
```

## Configuration

A skill uses two fields of the [resource schema](../resources/#resource-schema-kind-resource).
The **required** column below applies to a skill, not to resources in general.

| **field**   | **type** | **required** | **description**                                                                                                                                       |
|-------------|----------|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------|
| `uri`       | string   | Yes          | Address the resource as `skill://<skill-path>/<file>`. A URI that ends in `/SKILL.md` declares a skill. Other `skill://` URIs are files of a skill.  |
| `dynamic`   | bool     | No           | Set to `true` on a `SKILL.md` resource to publish the dynamic marker in place of digests. See [Dynamic skills](#dynamic-skills). Defaults to `false`. |

A `type: file` resource is the usual choice, because it serves a file from disk.

## Requirements

Toolbox validates every skill at startup. A skill that breaks one of these rules
fails the load, and the error names the skill and the file:

- The `SKILL.md` opens with `---` on the first line, and closes the frontmatter
  with `---` on a line of its own.
- The frontmatter parses as YAML.
- The frontmatter defines `name` and `description`.
- `name` equals the last segment of the skill path.
- `name` holds 1 to 64 characters, and uses only lowercase letters, digits, and
  hyphens. It does not start or end with a hyphen, and holds no two hyphens
  together.
- `description` holds at most 1024 characters.
- The skill holds at most 512 files.
- The files of the skill total at most 16 MiB.
- No two resources declare the same `skill://` URI.

A `file` resource serves only these extensions: `.txt`, `.md`, `.csv`, `.json`,
`.yaml`, `.yml`, `.xml`, `.sql`, `.html`, `.htm`, `.js`, `.css`, `.svg`. A
resource with any other extension fails the load, and `dynamic: true` does not
change this.

A skill can therefore hold files on disk that Toolbox cannot serve. Declare the
files it can serve, and set [`dynamic: true`](#dynamic-skills) on the `SKILL.md`.
The entry then publishes the dynamic marker in place of a file list that would
otherwise be incomplete.

A large file does not fail the load. It truncates. The
[`maxSize`](../resources/file/) of a `file` resource defaults to 5 MB. Toolbox
reads `maxSize` bytes, appends a truncation notice to the content, and computes
the digest over those bytes. The catalogue reports no truncation, so raise
`maxSize` when a file of the skill is larger.

## Freshness

Toolbox computes the digests for each request, and does not cache them for the
life of the process. Edit a file, and the next `skills/list` publishes the new
digest. The specification treats a changed digest as a normal condition: the host
requests a fresh entry, and the user approves the new content.

If you add or remove a file of the skill, you change the configuration. Reload
the server.

A client can still hold an older catalogue. Both results carry `ttlMs: 300000`
and `cacheScope: public`, so a client can reuse the catalogue for 5 minutes.
`public` also lets a shared proxy serve that catalogue to a different client.

## Nested skills

A skill path can contain another skill. `skill://guide/SKILL.md` and
`skill://guide/api/SKILL.md` declare two skills, and Toolbox lists both.

The specification requires a complete file list, so a file of the inner skill also
belongs to the outer skill. Both entries list that file.

## Dynamic skills

Set `dynamic: true` on a `SKILL.md` resource when the content of the skill changes
faster than a host's load cycle. The entry then publishes the string `"dynamic"`
in place of the file list.

{{< notice warning >}}
A dynamic skill publishes no digests, so a client cannot verify the bytes it
fetched against the catalogue. Use `dynamic: true` only when a stable digest is
impossible.
{{< /notice >}}

Three rules apply to the marker:

- `dynamic` is valid only on a `SKILL.md` resource. Any other resource fails the
  load.
- The 512-file limit does not apply, because a dynamic skill publishes no file
  list. The 16 MiB limit still bounds the read of the `SKILL.md`.
- A nested dynamic skill makes every enclosing skill dynamic. The outer skill
  contains the inner files, so its own digests cannot be stable either.

## Resource names

`resources/list` publishes the frontmatter `name` and `description` of a
`SKILL.md`, not the config `name` and `description`. Toolbox also reports the
`mimeType` of a `SKILL.md` as `text/markdown`. The `title` and the annotations
come from the config.

Toolbox logs a warning at startup for each `SKILL.md` whose config name differs
from its frontmatter name. The catalogue is correct either way. Name the resource
after the skill, because a group lists its resources by config name.

Toolbox logs a second warning at startup when two skills share a frontmatter
`name`. The warning names both URIs, because a host must distinguish them.

A host reaches a skill through `skills/list` and the URI. No code path routes on
the resource name.

## Groups

The skills catalogue is server-wide. [Groups](../groups/) do not scope
`skills/list` or `skills/get`, and the endpoint of the request does not change the
result. Groups do scope `resources/list` and `resources/read`, which return the
file content.

Treat the frontmatter of a skill as visible to every client of the server.

## Disabling the extension

Pass the extension URI to `--disable-ext`:

```bash
./toolbox --disable-ext io.modelcontextprotocol/skills
```

The server then does not answer `skills/list` or `skills/get` on any endpoint,
and removes the extension from its advertised capabilities. The files stay
readable as ordinary resources. See
[Disabling MCP Extensions](../../../reference/cli.md#disabling-mcp-extensions).

Toolbox still validates every skill at startup. A skill that breaks one of the
[requirements](#requirements) fails the load, with or without the extension.
