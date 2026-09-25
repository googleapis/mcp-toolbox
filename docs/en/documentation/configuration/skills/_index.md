---
title: "Agent Skills"
type: docs
weight: 12
description: >
  Generate an Agent Skill from a toolset, and serve an Agent Skill over MCP.
---

An [Agent Skill](https://agentskills.io/specification) is a directory of files. A
`SKILL.md` file at the root of that directory holds the skill's name and
description in YAML frontmatter. Toolbox works with skills in two directions:

- **[Generating Skills](./generating.md):** Convert a toolset or a
  [group](../groups/) into a skill package. The package contains a `SKILL.md` and
  one Node.js script for each tool.
- **[Serving Skills over MCP](./serving.md):** Publish a skill you already have
  through `skills/list` and `skills/get`, on the connection that serves your tools.

The two directions are independent. Use either one alone, or use both.
