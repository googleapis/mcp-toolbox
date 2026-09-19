# Link forms, path mapping, and the traps

How a file path in `docs/en` becomes a URL on `mcp-toolbox.dev`, and the structures that make a
naive link checker wrong here. Config moves, so verify anything load-bearing against
`.hugo/hugo.toml` and the workflow files.

## File path to URL

`.hugo/hugo.toml` mounts `../docs/en` as `content`. With `defaultContentLanguage = "en"` and
`defaultContentLanguageInSubdir = false`, **no URL has an `/en/` or `/docs/` segment**. `uglyURLs`
is unset, so URLs are pretty.

| File | URL |
| --- | --- |
| `docs/en/documentation/foo.md` | `/documentation/foo/` |
| `docs/en/documentation/_index.md` | `/documentation/` |
| `docs/en/integrations/postgres/source.md` | `/integrations/postgres/source/` |

The `aliases:` frontmatter on the Knowledge Catalog pages confirms it: site-absolute, no `/en/`.

## The canonical link form

File-relative with the `.md` extension, per the authoritative `DEVELOPER.md`. One string satisfies
both checkers: lychee resolves it as a filesystem path, Hugo resolves it to the pretty URL.

- Directory-style (`](../mcp-apps/)`) renders in Hugo, fails lychee.
- Site-absolute (`](/reference/cli/)`) passes lychee, breaks on versioned deploys.

Counts drift, so run the skill's greps rather than trusting numbers here. Rough orientation at the
time of writing: file-relative `.md` leads directory-style by about 4:1, site-absolute links are in
single digits, `{{< relref >}}` is unused, and the only two `{{< ref >}}` usages are in the
Firestore validate-rules page.

## Why site-absolute links leak across versions

Hugo does **not** prefix a hand-written absolute markdown path with `baseURL`, and each deploy sets
a different base:

| Deploy | `HUGO_BASEURL` |
| --- | --- |
| push to `main` | `https://mcp-toolbox.dev/dev/` |
| release | `https://mcp-toolbox.dev/<version>/` and `https://mcp-toolbox.dev/` |
| PR preview | `/` |

So `](/reference/cli/)` on the `/dev/` build resolves to `mcp-toolbox.dev/reference/cli/`, the
*latest-release* docs rather than dev; every archived build leaks the same way. Absolute
`https://mcp-toolbox.dev/...` self-links fail in mirror image, an archived `/v1.5.0/` page silently
linking forward to current docs. No markdown file hardcodes a versioned path today; the risk is the
opposite, unversioned absolute links that always mean "latest".

## Links that exist only in rendered HTML

These shortcodes build `<a href>` from `.RelPermalink`, so the markdown holds no link. A
grep-based checker sees a page with no outbound links, and sees nothing when a target is deleted:
the shortcode just renders one row fewer.

| Shortcode | Roughly how widely used | What it generates |
| --- | --- | --- |
| `{{< compatible-sources >}}` | ~300 pages | source pages compatible with a tool |
| `{{< list-tools >}}` | ~50 pages | the tool pages under a source |
| `{{< samples-gallery >}}` | 1 | every page with `is_sample: true` |
| `{{< list-prebuilt-configs >}}`, `{{< list-db >}}` | 1 each | directory listings |
| `{{< include >}}`, `{{< regionInclude >}}` | ~9 | another file's body, links and all |

Also rendered rather than written:

- **`shared_tools` frontmatter** injects a managed database's inherited tool links, via
  `.hugo/layouts/partials/hooks/body-end.html`.
- **`.hugo/layouts/docs/redirect.html`** renders a meta-refresh page from `external_url`.
- **`llms.txt` and `llms-full.txt`** emit absolute `Permalink`s for every page, baking in the
  deploy-time baseURL. `CLAUDE.md` requires hand-updating the Diátaxis section in both layouts when
  a top-level docs section is added.

Building the site and crawling `public/` is the only way to check any of this.

## Traps

- **`ignoreFiles`.** `.hugo/hugo.toml` lists several `quickstart/` paths. They exist on disk, so
  lychee resolves links to them happily, but Hugo never builds them into pages. The link 404s live
  and passes CI forever.
- **Missing `type: docs`.** A section `_index.md` without it gets the Docsy default layout instead
  of `.hugo/layouts/docs/section.html`, which is what lists child pages. The result is chrome with
  zero child links: a dead end containing no broken link. This hit 47 integration index pages
  (issue #3752, fixed in `59fb2c42180`).
- **Leaf-to-section promotion.** When a page becomes a directory, `foo.md` turns into
  `foo/_index.md` and every link naming the old leaf breaks. See `c63efb0568c`, which rewrote
  `../configure.md` to `../configuration/_index.md`.
- **Two `getting-started` directories.** `docs/en/getting-started/` and
  `docs/en/documentation/getting-started/` both exist, so `../getting-started/` resolves differently
  depending on the linking file's depth. Always resolve against the real file.
- **Aliases get forgotten.** The Dataplex to Knowledge Catalog rename added `aliases:` across ~23
  files, but later moves (the Groups docs relocation, the SDK page redirects) added none. Treat a
  missing alias on a rename as an oversight to raise, not a decision already made.

## Historical failure modes worth grepping for

```bash
git log --oneline -i --grep='broken link' --grep='dead link' --grep='fix.*link' --grep='docsite link' -- docs/
```

Recurring shapes: docs reorgs that move directories, tool and source page renames, absolute GitHub
URLs replaced by relative paths (`adc67c12a94`), and site-absolute paths converted to relative
(`3f400efdaa6`).
