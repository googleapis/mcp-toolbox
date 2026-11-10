# MCP Toolbox Agent Skills

This directory holds [Agent Skills](https://agentskills.io/specification) for the
MCP Toolbox project — self-contained instructions that teach a coding agent
(Claude Code, Cursor, Gemini CLI, Codex, and others) how to perform specific
workflows against this repo.

Skills are grouped by audience:

- `maintainer/` — skills for maintaining the toolbox itself (e.g. `triage-issues`,
  `review-prs`, `stale-sweep`).

## Installing

Skills install with [`npx skills`](https://www.npmjs.com/package/skills)
([source](https://github.com/vercel-labs/skills)), the open Agent Skills CLI. It
works with any supported agent and needs only Node.js (no global install
required).

By default it installs into the **current project** (`.agents/skills/`, symlinked
into each detected agent's directory). Add `-g` to install **globally** into your
home directory so the skill is available across all projects.

### Install every skill

```bash
# Interactive: pick which skills and agents to install
npx skills add googleapis/mcp-toolbox

# Non-interactive: install all skills for all detected agents
npx skills add googleapis/mcp-toolbox -y
```

### Install all maintainer skills

Point at the category subdirectory:

```bash
npx skills add googleapis/mcp-toolbox/skills/maintainer
```

### Install a specific skill

Point directly at the skill's folder:

```bash
# Shorthand
npx skills add googleapis/mcp-toolbox/skills/maintainer/triage-issues

# Or the full GitHub URL
npx skills add https://github.com/googleapis/mcp-toolbox/tree/main/skills/maintainer/triage-issues
```

From a source that contains several skills, you can also select by name instead
of a subpath:

```bash
npx skills add googleapis/mcp-toolbox --skill triage-issues   # one skill
npx skills add googleapis/mcp-toolbox --skill '*'             # all skills
```

### Install globally

Any of the commands above take `-g` to install into your home directory instead
of the current project:

```bash
npx skills add googleapis/mcp-toolbox/skills/maintainer/triage-issues -g
```

## Managing installed skills

```bash
npx skills list                 # list installed skills
npx skills update               # update all installed skills to latest
npx skills update triage-issues # update a specific skill
npx skills remove triage-issues # remove a skill
```

`update` re-fetches only skills whose upstream content has changed and refreshes
the whole skill folder (including bundled `references/`), so keep installed
copies read-only — local edits are overwritten on update.
