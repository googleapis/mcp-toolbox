---
name: docsite-link-sweep
description: >-
  Sweep the googleapis/mcp-toolbox docs for broken and non-canonical links, report each
  finding with the reason it breaks, and apply the safe class of internal link fixes. Use
  when a maintainer asks for a link sweep or docs health check, triages the weekly "Link
  Checker Report" issue, or after a docs reorg, page rename, or directory move, e.g. "check
  the docs for broken links", "link sweep", "fix the dead links in docs/". Edits the working
  tree and leaves a commit; never pushes, never opens a PR, and never rewrites external links
  or ambiguous targets.
---

# Docsite Link Sweep (mcp-toolbox)

Sweeps documentation for broken, dead, and non-canonical links. **Lychee** checks links as filesystem paths or network URLs; **Hugo** resolves `.md` links to pretty URLs and renders shortcodes.

This skill applies safe, mechanical internal link fixes (converting to canonical file-relative `.md` links) and reports external or ambiguous failures for maintainer review.

## Prerequisites

- **Clean git working tree**: Ensure no uncommitted docs changes before starting.
- **Tools**:
  - `lychee` (`brew install lychee` or `docker run --rm -v "$PWD:/input" lycheeverse/lychee`). If unavailable, run grep and build passes only.
  - `hugo` (Extended v0.146.0+) for shortcode and build verification.
- **Default scope**: `README.md` and `docs/en/` (or changed markdown files for a PR).

## Workflow

### 1. References
- [`DEVELOPER.md`](references/DEVELOPER.md): Canonical link rules.
- [`.lycheeignore`](https://github.com/googleapis/mcp-toolbox/blob/main/.lycheeignore): Excluded domains and URLs (every entry must have a comment).
- Workflows: [`link_checker.yaml`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/workflows/link_checker.yaml) (PR checks) and [`link_checker_report.yaml`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/workflows/link_checker_report.yaml) (weekly report).
- [`references/link-forms.md`](references/link-forms.md): URL mapping, version leaks, and Hugo traps.

### 2. Detection

**Lychee check:**
```bash
# Repo-wide (adjust scope as needed)
lychee --quiet --no-progress --exclude '^neo4j\+.*' --exclude '^bolt://.*' README.md docs/

# PR-scoped check (changed files only)
git diff --name-only --diff-filter=ACMRT origin/main...HEAD -- '*.md'
```

**Find links Lychee misses (structural & format issues):**
```bash
# Directory-style links (Hugo resolves, Lychee fails):
grep -rnE "\]\(\.\.?/[^)]*\)" docs/en --include=*.md | grep -vE "\.md(#[^)]*)?\)"

# Site-absolute links (leak across versioned deploys):
grep -rnE "\]\(/[^)]*\)" docs/en --include=*.md

# Hardcoded domain URLs:
grep -rn "https://mcp-toolbox.dev/" docs/en --include=*.md

# Section indexes missing `type: docs` (renders with no child links):
find docs/en -name _index.md -exec grep -L "^type: docs" {} +
```

**Verify shortcode-rendered links (deep sweep):**
```bash
cd .hugo && hugo --minify --config hugo.cloudflare.toml
lychee --offline --base-url public public
```

### 3. Classification & Action

Fix only safe, unambiguous internal links. Report everything else.

| Category | Action | Criteria |
|---|---|---|
| **Safe to fix** | Rewrite in-place to file-relative `.md` | • Directory link → file-relative `.md`<br>• Site-absolute `](/path/)` → file-relative `.md`<br>• Hardcoded domain `https://mcp-toolbox.dev/...` → file-relative `.md`<br>• Moved file with exactly one obvious git successor<br>• Renamed heading anchor drift |
| **Needs decision** | Report with recommendation; do not apply | • Target missing or deleted<br>• Multiple candidate targets after a split/reorg<br>• Links pointing to `ignoreFiles` paths (see `hugo.toml`)<br>• Missing `type: docs` on `_index.md` |
| **External** | Report `file:line`, URL, status | • 404 / 403 / 500 external URLs |
| **Ignore-worthy** | Propose commented `.lycheeignore` regex | • Auth-walled, rate-limited, or flakey endpoints |

**Canonical Link Rule:** Always link using file-relative paths ending in `.md` (e.g. `[Example](../folder/file.md)`). Never use site-absolute `/...` or directory `/.../` paths.

### 4. Verification

Verify all modified files against both checkers:
```bash
# 1. Verify filesystem paths resolve
lychee --quiet --no-progress --offline <modified-files>

# 2. Verify Hugo builds without ref errors
cd .hugo && hugo --environment development
```

### 5. Commit and Report

- Create a scoped branch: `git checkout -b docs/fix-docsite-links`
- Commit verified fixes: `git commit -am "docs: fix broken docsite links"`
- Never `git push` or run `gh pr create`. Output the report and prompt the maintainer with push/PR commands.

## Rules

- **Verify both ways**: Every fix must pass offline `lychee` and `hugo --environment development`.
- **Never guess external URLs**: If an external link is dead, report it rather than substituting a guess.
- **Never invent missing targets**: Missing internal pages belong in "Needs your decision".
- **Canonical format only**: File-relative `.md`. Never add internal links to `.lycheeignore`.
- **Propose-only**: Never push branches or open PRs automatically.

## Output Format

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
- <file>: <issue description>

**Apply:**
git push -u origin docs/fix-docsite-links
gh pr create --title "docs: fix broken docsite links"
```
