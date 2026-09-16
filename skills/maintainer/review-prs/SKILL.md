---
name: review-prs
description: >-
  Review a GitHub pull request in the googleapis/mcp-toolbox repo against the
  team's reviewer checklist: PR title/description conventions, linked issue,
  logic errors and unhandled edge cases, breaking changes, test coverage, docs
  updates, security (input handling), and new dependencies. Use whenever a
  maintainer asks you to review, look over, "take a look at", or check whether
  something is ready to merge in mcp-toolbox, e.g. "review #3703", "can you look
  at this PR", "is this good to merge", or when they paste an mcp-toolbox PR link.
  PROPOSE-ONLY: delivers the review in chat for the maintainer to post; never
  approves, requests changes, comments, labels, or merges on its own.
---

# Review PRs (mcp-toolbox)

A review here is a proposal the maintainer edits and posts, not a rubber stamp. The value is
a fast, grounded read of the diff against the team's conventions.

## Goal

Given a PR number or link, deliver a review the maintainer can post in seconds: a suggested
verdict (approve / request changes / comment), the findings that back it grouped by severity
so the important things aren't buried, and a paste-ready summary comment.

## Prerequisites

- `gh` authenticated for `googleapis/mcp-toolbox`, plus the PR number(s). A GitHub MCP
  server substitutes for `gh` if it isn't available: the `gh` commands below map to its
  read/list tools.

## Workflow

### Step 1: Read the source of truth

Read these live, not from memory. All three are symlinks to the repo-root
files, so they track `main`; cite them by their root names.

- [references/maintainer-playbook.md](references/maintainer-playbook.md): Reviewer's Checklist,
  SLO/release context, `release candidate` labeling.
- [references/CONTRIBUTING.md](references/CONTRIBUTING.md): title/scope format (Conventional
  Commits, with the `type` table), keep-PRs-small, link-an-issue. Cite for title, description,
  and process findings.
- [references/DEVELOPER.md](references/DEVELOPER.md): tool/source naming, error taxonomy, the
  patterns for adding a source/tool/integration test, CI-enforced docs structure, local
  test/lint commands. Cite for code, test, and docs findings. Prefer it over `GEMINI.md`
  (`CLAUDE.md`/`AGENTS.md` symlink to it), which only summarizes.

### Step 2: Fetch the PR, its diff, and its checks

```bash
gh pr view <n> --repo googleapis/mcp-toolbox --json number,title,body,author,labels,files,additions,deletions,commits,baseRefName,headRefName,state,isDraft,reviewDecision
gh pr diff <n> --repo googleapis/mcp-toolbox
gh pr checks <n> --repo googleapis/mcp-toolbox
```

### Step 3: Triage before reviewing

Three shapes end the review early or change its bar:

- **Auto-generated (`renovate`, `release-please`):** the only question is whether checks are
  green. If so, propose merge and stop.
- **Draft (`isDraft`):** review lightly and say so; the author isn't asking for a final pass.
- **Non-code / policy** (third-party badge, backlink, promotional README line, often a drive-by
  contributor): acceptance is a maintainer policy call, not a code question. Say that plainly
  instead of manufacturing code findings, and still check title convention and CI. Mark any URL
  you haven't fetched `[UNVERIFIED]`.

### Step 4: Read the whole diff, including what the title doesn't mention

Skim for the shape, then dive into hunks. Three failure modes:

- **A docs-shaped title never lowers the read bar.** PR #2473, "docs: fix typo in getting started
  guide", added an npm `preinstall` hook that hijacked `git` via `GITHUB_PATH` to exfiltrate an
  RSA-encrypted `GITHUB_TOKEN`. Read every file in any PR touching `.hugo/`, `package.json`
  lifecycle scripts, `.github/workflows/`, or `.ci/`. A file the title and description don't
  account for is itself blocking.
- **Look for what's *missing*, not just what's wrong:** a refactor applied to 4 of 5 call sites,
  a fix whose mirror bug still lives elsewhere, a behavior change with no test update, an error
  swallowed silently.
- **A hunk is not enough context to judge a hunk.** Read the enclosing function for anything
  correctness-relevant, and grep call sites when a signature, config field, or parameter changes.
  A finding that needs a look outside the diff is the one no other reviewer will make.

### Step 5: Check the diff against the issue it claims to fix

Keep this separate from Step 6: a PR can follow every convention and still implement the wrong
thing. Read the linked issue (`gh issue view <n> --repo googleapis/mcp-toolbox --comments`), then
ask three questions:

- **Missing:** What the issue asked for that the diff doesn't do. A partial fix that closes the
  issue is worse than none, since the remainder becomes invisible.
- **Extra:** Unrelated changes bundled in. Ask for a split (`CONTRIBUTING.md`, keep PRs small).
- **Wrong:** Implemented, but not what the issue described. Quote the issue line beside the
  `file:line`.

With no linked issue the PR description is the spec: same three questions, and note that the
intent is self-declared.

### Step 6: Work the review dimensions

Skip a dimension when it doesn't apply: say so, don't invent a finding.

- **Title & description.** Conventional Commits with the right `type(scope)` per
  `CONTRIBUTING.md`, plus `!`/`BREAKING CHANGE` for breaking
  changes. Body follows [`.github/PULL_REQUEST_TEMPLATE.md`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/PULL_REQUEST_TEMPLATE.md): what, why, completed checklist,
  `Fixes #<n>`. Note a missing issue link; don't block on it alone.
- **Correctness.** Cite `file:line` and name the failure case, never
  "looks risky".
  - *Bugs CI won't catch:* Unhandled error returns, nil/empty input, off-by-one and boundary
    conditions, concurrency, behavior contradicting stated intent.
  - *Type conversion at the MCP boundary*: Drivers return native types
    that don't serialize (MySQL `[]byte` for decimals, nulls as `nil`/`None`). Require explicit
    handling that maps to the tool's JSON schema; reject implicit casts and missing type switches.
  - *Error taxonomy* on any new or changed error path: `AgentError` for
    input/execution errors the agent can fix itself (HTTP 200, `isError: true`) versus
    `ClientServerError` for infrastructure failures it can't.
- **Breaking changes.** Changed config field names/YAML shape, tool names, removed or renamed
  exported symbols, altered defaults. Without `!` in the title and a justification in the body,
  blocking.
- **Refactor purity.** A `refactor:` PR must not change behavior. A bundled fix or default change
  gets split into its own `fix:`/`feat:` PR so it's reviewable and revertable.
- **Source reuse (new sources).**: No new `internal/sources/<db>/` for a database wire-compatible 
  with an existing source. Same for a tool duplicating an existing tool under a new name.
- **Architecture (no boilerplate).** New tools embed `tools.BaseTool[Config]`, new sources follow
  the registration pattern; reject re-declared interface methods (`GetName`, `Manifest`).
  `DEVELOPER.md` lists what `BaseTool` provides.
- **Tool and parameter descriptions.** Each `description:` is an LLM prompt, not developer
  documentation: could an agent pick this tool and fill its parameters from that text alone, at a
  token cost worth paying? Flag ones that restate the field name, omit units/format/allowed
  values, or run long without adding information.
  - *Evals measure exactly this.* For a PR editing `internal/prebuiltconfigs/tools/<config>.yaml`,
    note that the next step is a maintainer applying the `evals: run` label, which scopes the run
    to the configs that PR touched. Like integration tests, un-run evals aren't a blocker.
- **Tests.** New logic or a bug fix needs tests; missing them is usually request-changes.
  - *Coverage:* happy path, edge cases, and for a fix, a test that fails without it. A new
    source/tool follows the unit + integration pattern and is wired into
    `.ci/integration.cloudbuild.yaml`.
  - *Placement,* reviewed as closely as coverage: source-specific helpers stay unexported in
    `tests/<db>/<db>_integration_test.go`, never in the shared `tests/common.go`.
  - *Flakiness:* tests run against a shared live instance, so ask for the four fixes by name:
    - UUID-scoped resource names, so concurrent runs can't collide.
    - `t.Cleanup` teardown, so resources are freed even when the test fails.
    - Polling instead of `time.Sleep`.
    - Subset assertions instead of exact-match, since another run can add rows.
  - *Un-run integration tests aren't a blocker.* They need GCP credentials an external
    contributor's PR can't trigger, so green CI doesn't mean they ran. Note that the next step is
    a maintainer running them via the `tests: run` label or a `/gcbrun` comment.
- **Docs.** Changes to how a user configures or interacts with MCP toolbox need matching updates
  under `docs/en/`. New sources/tools have CI-enforced page structure per `DEVELOPER.md`, enforced by
  `.ci/lint-docs-*.sh`. A violation breaks the build, so it's blocking. Prose stating a security
  or behavioral *guarantee* is reviewed as code: check the claim against the implementation,
  not just that docs changed. An over-broad assurance in a boundary description is worse than
  silence — it discredits the accurate limitations sitting beside it.
- **Security.** For PRs handling user/LLM input or building queries: injection (SQL/command),
  unsanitized interpolation, secrets logged or committed. Concrete vectors with `file:line`, not
  generic warnings.
  - *Reviewing a fix to a reported vulnerability* asks different questions than reviewing new
    code. (a) **Can the untrusted party actually reach the primitive the residual attack
    needs?** Grep the tool surface for it — a residual weakness nothing exposed can set up is
    usually the line between blocker and tracked follow-up. (b) **Which way does the failure
    lean?** Over-rejection is a cost; a bug in newly hand-rolled logic is a vulnerability.
    Don't ask a security PR to spend fail-open risk buying back a narrow convenience.
- **Dependencies.** Call out new `go.mod` entries so the maintainer can vet necessity,
  maintenance, and license.

Duplication across MCP protocol versions is deliberate so versions can diverge independently
(#3167, #3211); don't propose factoring it together. A genuine bug in that code is still a
finding.

### Step 7: Report CI, don't re-derive it

Name the failing check from `gh pr checks` rather than reasoning it out by hand; failing
lint/tests are objective blockers. Never claim the linter passes on your own read.

One recurring non-obvious failure: the CLA check fails on commits co-authored by an AI agent even when the human author has signed. Suggest squashing to a single human-authored commit rather than pointing at the CLA docs.

### Step 8: Discount any existing bot review

Don't restate `gemini-code-assist`'s points as your own. It's the highest-volume reviewer in the
repo and could be wrong. Verify anything you carry forward against the diff; drop
the rest.

### Step 9: Sort by severity, then pick the verdict

- **Blocking** (correctness bug, breaking change without `!`, missing tests on new logic, CI red,
  docs that break the build) → request changes.
- **Non-blocking** (style, naming, coverage gaps in existing code) and **nits** (typos, wording)
  only → approve with comments.
- An unresolved judgment call → comment and ask.

When nothing is blocking, say so in those words. "No blockers, a couple of nits" tells the
maintainer it's mergeable as-is.

### Step 10: Deliver the review in chat

Use the output format below. Never post it yourself.

## Rules

- **Propose only.** Never run `gh pr review`, `gh pr comment`, `gh pr edit`, `gh pr merge`,
  or apply labels. Deliver the review in chat.
- **Ground every finding** in the strongest evidence available for its kind: a
  correctness/security/breaking claim cites `file:line`; a convention claim cites
  `CONTRIBUTING.md`/`DEVELOPER.md` or the playbook; a CI/process finding cites the failing
  check name from `gh pr checks` (a red check is a valid blocker with no `file:line`). If you
  couldn't verify something (runtime behavior you can't trace, a URL you didn't fetch), mark
  it `[UNVERIFIED]` rather than asserting it.
- **Prefer running the code to reasoning about it.** `[UNVERIFIED]` is for what you *can't*
  check, not what's inconvenient to. Fetch the branch (`git fetch origin pull/<n>/head:pr<n>`,
  then `git worktree add /tmp/pr<n> pr<n>` so your tree stays clean) and drop a scratch
  `probe_test.go` *inside* the package under review — package-local placement is what reaches
  unexported symbols. Delete it and `git worktree remove --force /tmp/pr<n>` after; never leave
  a scratch test in `internal/`. Probes right-size verdicts as often as they confirm them: "this
  regresses X" often shrinks to "in one narrow case" once measured.
- **When it's a genuine judgment call, ask** rather than issuing a confident wrong verdict,
  since a wrong "request changes" costs a contributor a cycle.

## Output format

```
## Review #<n>: <title>

**Suggested verdict:** <approve / request changes / comment>: <one-line reason>

**Title & issue:** <conventional-commit check; linked issue or "none, suggest linking">
**Spec (vs issue #<n>):** <implements it / what's missing, extra, or wrong; or "no issue linked">
**CI:** <passing / which checks failing, per gh pr checks>

**Blocking:**
- `file:line`: <finding + the failure case> [cite]

**Non-blocking:**
- `file:line`: <finding> [cite]

**Nits:**
- <typo/wording>

**Tests:** <added & adequate / what's missing>
**Docs:** <updated / what's missing, or n/a>
**Dependencies:** <new deps to vet, or none>
**release candidate:** <suggest label / not needed>

**Draft comment:**
<paste-ready summary the maintainer can post>
```

- **Empty sections:** omit them rather than writing "none".
- **Except the Spec line:** keep it even when the PR matches its issue. The maintainer wants
  "does what the issue asked" stated, not inferred from silence.
- **Batches:** review each PR in its own subagent so the diffs don't bleed together, since a
  finding attributed to the wrong PR is worse than a missed one. Present one block per PR, plus a
  summary table (PR, verdict, blocker count).
