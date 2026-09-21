---
name: docsite-link-sweep
description: >-
  Sweep the googleapis/mcp-toolbox docs for broken and non-canonical links, report each
  finding with its cause, and apply the safe class of internal link fixes. Use when a
  maintainer asks for a link sweep or a docs health check, triages the weekly "Link Checker
  Report" issue, or after a docs reorg, page rename, or directory move. Example requests:
  "check the docs for broken links", "link sweep", "fix the dead links in docs/". The skill
  edits the working tree and makes one commit. It never pushes, never opens a PR, and never
  rewrites external links or ambiguous targets.
---

# Docsite Link Sweep (mcp-toolbox)

Two link checkers run on this repo, and each one misses what the other catches:

- **Lychee** resolves a link as a filesystem path or a network URL. It knows nothing about Hugo.
- **Hugo** resolves a `.md` link to a pretty URL and generates links from shortcodes. It never checks an external URL.

This skill covers the gap. It applies safe mechanical fixes to internal links. It reports external and ambiguous failures for a maintainer to decide.

## Prerequisites

- **Clean git working tree.** Commit or stash docs changes before you start.
- **`lychee`.** Install with `brew install lychee`, or run `docker run --rm -v "$PWD:/input" lycheeverse/lychee`. If lychee is absent, run the grep and build passes only.
- **`hugo`**, Extended v0.146.0 or later, for shortcode and build verification.

Default scope is `README.md` and `docs/en/`. For a PR, use the changed markdown files instead.

## References

- [`DEVELOPER.md`](references/DEVELOPER.md): canonical link rules.
- [`references/link-forms.md`](references/link-forms.md): path-to-URL mapping, version leaks, and Hugo traps.
- [`.lycheeignore`](https://github.com/googleapis/mcp-toolbox/blob/main/.lycheeignore): excluded domains and URLs. Every entry must carry a comment.
- [`link_checker.yaml`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/workflows/link_checker.yaml): the PR check. [`link_checker_report.yaml`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/workflows/link_checker_report.yaml): the weekly report.

## 1. Detect

Run lychee. The two `--exclude` patterns match the PR workflow:

```bash
lychee --quiet --no-progress --exclude '^neo4j\+.*' --exclude '^bolt://.*' README.md docs/

# For a PR, limit the scope to changed files:
git diff --name-only --diff-filter=ACMRT origin/main...HEAD -- '*.md'
```

Then grep for the structural problems lychee cannot see:

```bash
# Directory-style links. Hugo resolves these, lychee fails them.
grep -rnE "\]\(\.\.?/[^)]*\)" docs/en --include=*.md | grep -vE "\.md(#[^)]*)?\)"

# Site-absolute links. These leak across versioned deploys.
grep -rnE "\]\(/[^)]*\)" docs/en --include=*.md

# Hardcoded domain URLs.
grep -rn "https://mcp-toolbox.dev/" docs/en --include=*.md

# Section indexes with no `type: docs`. Docsy then renders no child links.
find docs/en -name _index.md \
  -not -path '*/tools/*' -not -path '*/samples/*' -not -path '*/prebuilt-configs/*' \
  -exec grep -L "^type: docs" {} +
```

The three excluded paths hold frontmatter-only wrapper files. `CLAUDE.md` requires them to stay minimal, so they are expected hits and not findings. Without the exclusions this check returns about 58 files instead of 1.

For a deep sweep, build the site and crawl the rendered HTML. This is the only pass that sees shortcode-generated links:

```bash
cd .hugo && hugo --minify --config hugo.cloudflare.toml
lychee --offline --base-url public public
```

## 2. Classify

Fix only safe, unambiguous internal links. Report everything else.

| Category | Action | Criteria |
|---|---|---|
| **Safe to fix** | Rewrite in place to a file-relative `.md` link | • Directory link<br>• Site-absolute `](/path/)`<br>• Hardcoded `https://mcp-toolbox.dev/...`<br>• Moved file with exactly one obvious git successor<br>• Renamed heading anchor |
| **Needs decision** | Report with a recommendation. Do not apply. | • Target is missing or deleted<br>• Several candidate targets after a split or reorg<br>• Link points into an `ignoreFiles` path (see `hugo.toml`)<br>• `_index.md` has no `type: docs` |
| **External** | Report `file:line`, URL, and status | • External URL returns 404, 403, or 500 |
| **Ignore-worthy** | Propose a commented `.lycheeignore` regex | • Endpoint is auth-walled, rate-limited, or flaky |

**Canonical link rule:** use a file-relative path that ends in `.md`, for example `[Example](../folder/file.md)`. Never use a site-absolute `/...` path or a directory `/.../` path.

## 3. Verify

Check every modified file against both checkers:

```bash
lychee --quiet --no-progress --offline <modified-files>
cd .hugo && hugo --environment development
```

## 4. Commit and report

```bash
git checkout -b docs/fix-docsite-links
git commit -am "docs: fix broken docsite links"
```

Stop there. Print the report, then give the maintainer the push and PR commands to run.

## Rules

- **Verify both ways.** Every fix must pass offline `lychee` and `hugo --environment development`.
- **Never guess an external URL.** If an external link is dead, report it. Do not substitute a replacement.
- **Never invent a missing target.** A missing internal page belongs in "Needs your decision".
- **Canonical format only.** Use file-relative `.md`. Never add an internal link to `.lycheeignore`.
- **Propose only.** Never push a branch and never open a PR.

## Output format

```text
## Docsite link sweep: <scope>, <X> findings
Checked: lychee (<status>) | Hugo build (<status>) | <N> files changed

**Fixed and verified** (<count>)
| file:line | was | now | reason |

**Needs your decision** (<count>)
| file:line | target | problem | recommendation |

**External, report only** (<count>)
| file:line | url | status |

**Proposed .lycheeignore entries** (<count>)
| pattern | reason |

**Structural** (<count>)
- <file>: <issue>

**Apply:**
git push -u origin docs/fix-docsite-links
gh pr create --title "docs: fix broken docsite links"
```
