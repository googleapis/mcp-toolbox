# build-with-mcp-toolbox

Skills for developers building applications with
[MCP Toolbox for Databases](https://mcp-toolbox.dev).

This plugin ships **skills only**. It does not bundle the Toolbox MCP server —
if you want to serve an existing `tools.yaml`, run the server directly
(`npx @toolbox-sdk/server --config tools.yaml`).

## Install

Not published yet. Claude Code installs plugins only through a marketplace, and
a plugin directory is not itself one, so there is no `/plugin install` path
until this is listed in a catalog.

To try it before then, wrap it in a throwaway marketplace outside the repo:

```bash
mkdir -p /tmp/toolbox-mkt/.claude-plugin
cat > /tmp/toolbox-mkt/.claude-plugin/marketplace.json <<'EOF'
{
  "name": "toolbox-local",
  "owner": { "name": "local" },
  "plugins": [{ "name": "build-with-mcp-toolbox", "source": "./plugin" }]
}
EOF
cp -R plugins/build-with-mcp-toolbox /tmp/toolbox-mkt/plugin

claude plugin marketplace add /tmp/toolbox-mkt
claude plugin install build-with-mcp-toolbox@toolbox-local
claude plugin details build-with-mcp-toolbox@toolbox-local
```

Validate the manifest with `claude plugin validate . --strict`.

Distribution is planned via the official Claude Code plugin catalog, using a
`git-subdir` source pointing at this directory so installs fetch only these
files rather than a checkout of the whole repository.

## Skills

- [`getting-started`](skills/getting-started/SKILL.md)

Each skill's `description` frontmatter is the authoritative statement of what it
covers and when it triggers.

## Versioning

The plugin version tracks the Toolbox server version — both are bumped by the
same release-please run, so the guidance in a skill matches the binary it
describes. Skills link to the versioned docsite (`mcp-toolbox.dev/v<version>/`)
rather than symlinking into the repo, so this directory stands alone.

## Contributing

Skills live in `skills/<skill-name>/SKILL.md`. When adding one:

- Link to the versioned docsite, never to `/dev/` or a moving branch path.
- Wrap any version-bearing string in
  `<!-- {x-release-please-start-version} -->` / `<!-- {x-release-please-end} -->`
  and add the file to `extraFiles` in `.github/release-please.yml`.
- Add a link to it under [Skills](#skills).
- Validate before pushing: `claude plugin validate . --strict`
