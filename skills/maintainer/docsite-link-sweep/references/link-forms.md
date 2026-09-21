# Link forms, path mapping, and traps

How a docs path becomes a URL in `mcp-toolbox`, and the link traps that follow from it.

## Path to URL mapping

`.hugo/hugo.toml` mounts `../docs/en` as `content` at the site root, and sets
`defaultContentLanguageInSubdir = false`.

- A rendered URL contains no `/en/` prefix and no `/docs/` prefix.
- URLs are pretty paths. `docs/en/integrations/postgres/source.md` becomes `/integrations/postgres/source/`.

## Canonical link form

Use a file-relative path that ends in `.md`, for example `[Postgres](../postgres/source.md)`.

- **Directory style** (`../postgres/`): Hugo resolves it. Lychee fails it.
- **Site-absolute** (`/integrations/postgres/source/`): lychee passes it. It breaks versioned builds.

### Why a site-absolute link leaks

Hugo does not prefix a hand-written absolute path with `baseURL`. The same page deploys to three
different roots:

- A merge to `main` deploys to `https://mcp-toolbox.dev/dev/`.
- A release deploys to `https://mcp-toolbox.dev/<version>/`.
- A PR preview deploys to `/`.

A link such as `](/reference/cli/)` always resolves to `mcp-toolbox.dev/reference/cli/`. It sends
the reader out of `/dev/` or out of an archived release, and into the latest release.

## Shortcode-generated links

Some links exist only in the HTML that Hugo renders:

- `{{< compatible-sources >}}`: the sources that support a tool.
- `{{< list-tools >}}`: the tools under a source.
- `{{< samples-gallery >}}`, `{{< list-prebuilt-configs >}}`, `{{< list-db >}}`: directory listings.
- `{{< include >}}`, `{{< regionInclude >}}`: inlined external content.
- `shared_tools` frontmatter: injected tool links for a managed database.

To check these links, build the site and crawl `public/` with lychee.

## Common traps

- **`ignoreFiles` in `hugo.toml`.** A listed file exists on disk, so lychee passes it. Hugo does not render it, so the live page returns 404.
- **No `type: docs`.** An `_index.md` without `type: docs` falls back to the standard Docsy layout, which renders no child page links.
- **Leaf promoted to section.** When `foo.md` becomes `foo/_index.md`, every old link to `foo.md` breaks.
- **Two `getting-started` directories.** Both `docs/en/getting-started/` and `docs/en/documentation/getting-started/` exist. Resolve a relative path against the actual target file.
- **Missing alias.** When you rename or move a page, add an `aliases:` frontmatter entry. Without it, external bookmarks go dead.
