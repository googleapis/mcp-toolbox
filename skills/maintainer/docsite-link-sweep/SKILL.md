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

Two checkers guard these docs and neither is sufficient alone. **lychee**, the repo's configured
checker, resolves links as filesystem paths and knows nothing about Hugo. **Hugo**, which builds
the site, resolves `.md` links to pretty URLs and generates whole classes of links from shortcodes,
but never checks an external URL. A link can pass one and break the other.

So the value of a sweep is not the lychee output. It is deciding which failures are real breakage,
which are the two checkers disagreeing, and which single rewrite satisfies both.

## Prerequisites

- **Hugo Extended v0.146.0+.** Without a build you cannot see shortcode-generated links.
- **lychee**, if available (`brew install lychee`, or `docker run --rm -v "$PWD:/input"
  lycheeverse/lychee`). Without it, run the grep and build passes only and say so in the report.
- **A clean tree.** Stop if docs have uncommitted changes: you cannot hand back a reviewable commit
  sitting on top of someone's work in progress.
- **A scope.** Default to `README.md` plus `docs/`, matching the weekly job.

## Workflow

### Step 1: Read the source of truth

- [`references/DEVELOPER.md`](references/DEVELOPER.md), "Link Checking and Fixing with Lychee":
  canonical link form, and when an ignore entry is legitimate. Cite it as `DEVELOPER.md`; the
  symlink path means nothing to a reader.
- [`.lycheeignore`](https://github.com/googleapis/mcp-toolbox/blob/main/.lycheeignore): what is
  already excluded, and why. Every entry carries a comment, and yours must too.
- [`link_checker.yaml`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/workflows/link_checker.yaml)
  (per PR, changed files) and
  [`link_checker_report.yaml`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/workflows/link_checker_report.yaml)
  (weekly, over `README.md` and `docs/`, files the "Link Checker Report" issue). Take the lychee
  args from the file, not from this skill. **Both have been disabled before**, so confirm they are
  running rather than assuming CI caught anything:

  ```bash
  gh api repos/googleapis/mcp-toolbox/actions/workflows --paginate \
    -q '.workflows[] | select(.path|test("link")) | .path + "  " + .state'
  ```

  When they are disabled, this sweep is the repo's only link checking and nothing has been checked
  since they were turned off. Say so in the report.
- [`references/link-forms.md`](references/link-forms.md): path-to-URL mapping, the shortcodes that
  generate links, and the traps that make a naive checker wrong here.

### Step 2: Run the configured check yourself

```bash
lychee --quiet --no-progress --exclude '^neo4j\+.*' --exclude '^bolt://.*' README.md docs/

# PR-scoped: the file list the PR job builds
git diff --name-only --diff-filter=ACMRT origin/main...HEAD -- '*.md'
```

Reproduce before fixing. A finding that will not reproduce locally is usually an external flake or
a cache artifact, so it belongs in the external bucket rather than the fix list.

### Step 3: Find what lychee structurally cannot

```bash
# Directory-style relative links: Hugo resolves them, lychee cannot.
grep -rnE "\]\(\.\.?/[^)]*\)" docs/en --include=*.md | grep -vE "\.md(#[^)]*)?\)"

# Site-absolute links: leak out of /dev/ and /vX.Y.Z/ builds to the latest-release docs.
grep -rnE "\]\(/[^)]*\)" docs/en --include=*.md

# Absolute self-links: the same version leak, and they 404 before a page ships to root.
grep -rn "https://mcp-toolbox.dev/" docs/en --include=*.md

# Section indexes missing `type: docs` render with no child links: a dead end
# containing no broken link (issue #3752).
find docs/en -name _index.md -exec grep -L "^type: docs" {} +
```

The last one lists candidates, not defects, and matches dozens of files. A page setting
`no_list: true` or rendering its own listing shortcode is deliberate. Open each hit and confirm the
built page has no way down to its children before reporting it.

Then build and crawl, the only way to see shortcode-generated links:

```bash
cd .hugo && hugo --minify --config hugo.cloudflare.toml
lychee --offline --base-url public public   # run `lychee --help`; this flag has been renamed across versions
```

Delete a page that `{{< list-tools >}}` or `{{< compatible-sources >}}` feeds and the shortcode
renders one row fewer: no error, no broken link, just missing content. `link-forms.md` has the
full set.

### Step 4: Classify before you touch anything

Every finding goes in exactly one bucket. Only the first is yours to fix.

**Safe to fix.** Mechanical, one correct answer, verifiable both ways:

1. Directory-style relative link → the same target in `.md` form.
2. Site-absolute `](/some/path/)` → file-relative `.md` path.
3. Absolute `https://mcp-toolbox.dev/...` self-link → file-relative `.md` path.
4. A moved target where `git log --diff-filter=D --name-only` or `git log --follow` names exactly
   one successor.
5. Anchor drift where the heading was renamed in this repo and the new heading is unambiguous.

**Needs a decision.** Report with a recommendation, do not apply:

- A target that exists nowhere: writing the page and deleting the link are both defensible.
- A move with more than one plausible successor, such as a page split in two.
- A link into a path listed in `ignoreFiles` in
  [`.hugo/hugo.toml`](https://github.com/googleapis/mcp-toolbox/blob/main/.hugo/hugo.toml). The file
  exists on disk so lychee is happy, but Hugo never builds it and the link 404s on the site.
- A section confirmed dead for want of `type: docs`. Give the frontmatter patch and let the
  maintainer apply it; it changes how the whole page renders, not just a link.

**External.** Report the status code and `file:line`, and propose only a direction. Guessing a
replacement URL swaps a visibly broken link for a plausible-looking wrong one.

**Ignore-worthy.** Rate-limited, auth-walled, or local-only URLs. Propose a `.lycheeignore` entry
with its comment. Last resort: an ignore entry hides the link from every future sweep.

### Step 5: Rewrite to the form that satisfies both checkers

File-relative, with the `.md` extension: lychee finds the physical file and Hugo resolves the same
string to the pretty URL. Compute the path from the *linking file's* directory, remembering that
`docs/en` is mounted at the site root with no `/en/` and no `/docs/` segment.

Two traps produce a wrong-but-plausible path, both detailed in `link-forms.md`: `_index.md` is the
section itself rather than a sibling, and `getting-started` names two different directories.

### Step 6: Verify every fix both ways

```bash
lychee --quiet --no-progress --offline <the files you changed>   # lychee finds the file
cd .hugo && hugo --environment development                       # Hugo builds with no ref errors
```

Then confirm the rendered `href` in `public/` points where you intended. A relative path can be
wrong by one directory level and still resolve to a real file.

If a fix touches a renamed or moved page, raise whether `aliases:` frontmatter belongs at the new
location so the old URL keeps working. The repo has the pattern but most moves forget it, so treat
a missing alias as an oversight rather than a decision already made.

### Step 7: Land the change, and stop

- Branch `docs/fix-docsite-links`, scoped further if the sweep was.
- One commit, `docs: fix broken docsite links`, safe-class fixes only: no drive-by wording edits,
  no reformatting, nothing from the decision bucket.
- Report the branch name and the command to run. Never `git push` or `gh pr create`.

## Rules

- **Verify both ways or do not ship it.** A change not confirmed against lychee *and* a Hugo build
  is a finding, not a fix.
- **Never rewrite an external link**, and never invent an internal target. A destination that does
  not exist goes in the decision bucket even when the intended page seems obvious.
- **One reason per finding**, with `file:line`. "Broken" is not a reason; "directory-style link,
  lychee cannot resolve it" is.
- **Never ignore an internal link.** Ignore entries are for external URLs only, always commented.
- **Disclose the edges of the sweep**: the scope, whether you built the site or only grepped, and
  anything you skipped. Silence about coverage reads as "I checked everything."
- **Mark anything unverified** `[UNVERIFIED]` rather than asserting it.

## Output format

```text
## Docsite link sweep: <scope>, <X> findings
Checked: lychee over <scope> | built site: <yes/no> | <N> files changed

**Fixed and verified** (<n>)
| file:line | was | now | why it broke |

**Needs your decision** (<n>)
| file:line | target | the problem | recommendation |

**External, report only** (<n>)
| file:line | url | status |

**Proposed .lycheeignore entries** (<n>)
| pattern | why |

**Structural** (<n>)
- <file>: <e.g. missing `type: docs`, renders no child links>

**Apply:**
git push -u origin docs/fix-docsite-links
gh pr create --title "docs: fix broken docsite links"
```

Omit empty buckets, except **Needs your decision**, which is stated even when empty: "nothing here
needs a judgment call" tells the maintainer the branch is safe to skim rather than audit. For large
sweeps, lead with the per-bucket counts and fix in batches.
