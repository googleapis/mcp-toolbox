---
title: "Agent Skills"
type: docs
weight: 12
description: >
  How to generate agent skills from a toolset.
---

The `skills-generate` command allows you to convert a **toolset** or a **[group](../groups/)** into an **Agent Skill**. The generated skill contains metadata and execution scripts for all tools in the collection, complying with the [Agent Skill specification](https://agentskills.io/specification).

## Before you begin

1. Make sure you have the `toolbox` executable in your PATH.
2. Make sure you have [Node.js](https://nodejs.org/) installed on your system.

## Generating a Skill from a Toolset

A skill package consists of a `SKILL.md` file (with required YAML frontmatter) and a set of Node.js scripts. Each tool defined in your toolset maps to a corresponding script in the generated Node.js scripts (`.js`) that work across different platforms (Linux, macOS, Windows).


### Command Usage

The basic syntax for the command is:

```bash
toolbox <tool-source> skills-generate \
  --name <skill-name> \
  --group <group-name> \
  --toolset <toolset-name> \
  --description <description> \
  --output-dir <output-directory> \
  --license-header <license-header> \
  --additional-notes <additional-notes>
```

- `<tool-source>`: Can be `--config`, `--configs`, `--config-folder`, and `--prebuilt`. See the [CLI Reference](../../../reference/cli.md) for details.
- `--name`: (Optional) Name of the generated skill. When multiple toolsets are generated because `--toolset` is omitted, this name acts as a prefix for each skill folder (e.g., `<name>-<toolset>`). When omitted in a single-skill mode, the name defaults, in order, to: the `--group` name, then the `--toolset` name, then the single `--prebuilt` config name. Any other case (a custom `--config`, or multiple `--prebuilt` configs) requires `--name`.
- `--description`: (Optional) Description of the generated skill. When a [group](../groups/) defines its own `description`, that takes precedence and `--description` acts as a fallback for groups without one.
- `--group`: (Optional) Name of the [group](../groups/) to convert into a single skill. Uses the group's `description`, falling back to `--description`. Mutually exclusive with `--toolset`.
- `--toolset`: (Optional) Name of the toolset to convert into a skill. If not provided, one skill will be generated for every custom toolset defined. If no custom toolsets are defined, it defaults to a single skill containing all tools.
- `--output-dir`: (Optional) Directory to output generated skills (default: "skills").
- `--license-header`: (Optional) Optional license header to prepend to generated node scripts.
- `--additional-notes`: (Optional) Additional notes to add under the Usage section of the generated SKILL.md.
- `--invocation-mode`: (Optional) Invocation mode for the generated scripts: 'binary' or 'npx' (default: "npx").
- `--toolbox-version`: (Optional) Version of @toolbox-sdk/server to use for npx approach (defaults to current toolbox version).

{{< notice note >}}
**Note:** The `<skill-name>` must follow the Agent Skill [naming convention](https://agentskills.io/specification): it must contain only lowercase alphanumeric characters and hyphens, cannot start or end with a hyphen, and cannot contain consecutive hyphens (e.g., `my-skill`, `data-processing`).
{{< /notice >}}

### Example: Custom Tools File

1. Create a `tools.yaml` file with a toolset and some tools:

   ```yaml
   tools:
     tool_a:
       description: "First tool"
       run:
         command: "echo 'Tool A'"
     tool_b:
       description: "Second tool"
       run:
         command: "echo 'Tool B'"
   toolsets:
     my_toolset:
       tools:
         - tool_a
         - tool_b
   ```

2. Generate the skill:

   ```bash
   toolbox --config tools.yaml skills-generate \
     --name "my-skill" \
     --toolset "my_toolset" \
     --description "A skill containing multiple tools" \
     --output-dir "generated-skills"
   ```

3. The generated skill directory structure:

   ```text
   generated-skills/
   └── my-skill/
       ├── SKILL.md
       ├── assets/
       │   └── tools.yaml
       └── scripts/
           ├── tool_a.js
           └── tool_b.js
   ```

   In this example, the skill contains two Node.js scripts (`tool_a.js` and `tool_b.js`), each mapping to a tool in the original toolset.

### Example: Generating from a Group

The `--group` flag generates a **single** skill from one named [group](../groups/), scoping the skill to that group's tools. This differs from `--toolset`, and the two flags are **mutually exclusive** — passing both is an error.

Description resolution follows the group's metadata:

- If the group defines a `description`, the generated skill uses it — this takes **precedence** over `--description`.
- If the group has no `description`, the `--description` flag is used as a fallback.

When `--name` is omitted, the skill is named after the group (e.g., `data_analyst`):

```bash
toolbox --config tools.yaml skills-generate \
  --group "data_analyst" \
  --description "Fallback description if the group has none"
```

{{< notice note >}}
Sourcing the description from the group keeps the skill's description and the group's metadata in sync from a single place. To control the generated skill's description, set it on the group rather than passing `--description`.
{{< /notice >}}

### Example: Prebuilt Configuration

You can also generate skills from prebuilt configurations. When exactly one `--prebuilt` config is provided, `--name` is optional and defaults to the prebuilt config name, so no naming flags are required:

```bash
toolbox --prebuilt alloydb-postgres-admin skills-generate \
  --description "skill for performing administrative operations on alloydb"
```

## Installing the Generated Skill in Gemini CLI

Once you have generated a skill, you can install it into the Gemini CLI using the `gemini skills install` command.

### Installation Command

Provide the path to the directory containing the generated skill:

```bash
gemini skills install /path/to/generated-skills/my-skill
```

Alternatively, use ~/.gemini/skills as the `--output-dir` to generate the skill straight to the Gemini CLI.
