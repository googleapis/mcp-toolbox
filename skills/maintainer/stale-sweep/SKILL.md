---
name: stale-sweep
description: >-
  Sweep the googleapis/mcp-toolbox repo for issues and PRs with no real activity in
  N days (default 60), sort each by whose silence it is (the author's, ours, or
  nobody's), and draft the nudge or close comment. Use whenever a maintainer asks for
  a stale sweep, backlog cleanup, or an SLO check, e.g. "stale sweep", "find issues
  with no activity in 30 days", "what's gone quiet", "what's rotting in the backlog",
  "draft close comments for the stale ones", or during the weekly open-issues review.
  PROPOSE-ONLY: delivers the sweep in chat for the maintainer to apply; never
  comments, labels, closes, or merges on its own.
---

# Stale Sweep (mcp-toolbox)

This repo runs no stale bot (`.github/` has no stale workflow), so closing is always a deliberate
maintainer act. The value of a sweep is therefore not the list of old things, which anyone can get
by sorting on a date. It's the judgment about which silences belong to the contributor, which belong
to us, and which are the backlog working as intended.

## Goal

Given a window (default 60 days), deliver a sweep the maintainer can work through in one sitting:
every candidate sorted into close / nudge-author / on-us / leave-alone, each with the date and
reason that put it there, plus a paste-ready comment wherever one is warranted.

## Prerequisites

- `gh` authenticated for `googleapis/mcp-toolbox`. A GitHub MCP server substitutes: the commands
  below map to its read/list tools.
- A window. If the maintainer didn't give one, default to 60 days and say that's what you used.

## Workflow

### Step 1: Read the source of truth

Read these live, not from memory.

- [references/maintainer-playbook.md](references/maintainer-playbook.md): **authoritative** for the
  >60-day silence rule on `status: waiting for response` / `status: feedback wanted`, the SLO
  response and closure targets, and the canonical comment templates.
- `gh label list --repo googleapis/mcp-toolbox --limit 200`: **there is no `stale` label**, so never
  propose one. This workflow's vocabulary is `status: waiting for response`, `status: feedback
  wanted`, `duplicate`, and `wontfix`.
- [`.github/blunderbuss.yml`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/blunderbuss.yml):
  which `product:` label routes to which team, needed to name an owner on anything blocked on us.

### Step 2: Pull the candidates

```bash
CUTOFF=$(date -d '60 days ago' +%F 2>/dev/null || date -v-60d +%F)   # GNU, then BSD/macOS

gh search issues --repo googleapis/mcp-toolbox --state open --updated "<$CUTOFF" \
  --limit 100 --json number,title,url,updatedAt,createdAt,labels,author,commentsCount

gh search prs --repo googleapis/mcp-toolbox --state open --updated "<$CUTOFF" \
  --limit 100 --json number,title,url,updatedAt,labels,author,isDraft
```

`gh search` defaults to 30 results, so always pass `--limit`.

### Step 3: Don't let `updatedAt` decide

`updatedAt` is not a silence measure. Label edits, bot comments, renovate rebases, and
cross-references all bump it, so an item a bot has kept warm never enters the candidate list at all
despite nobody having looked at it in months. Get the real last-human-touch before judging anything:

```bash
gh issue view <n> --repo googleapis/mcp-toolbox --json comments,labels,assignees,createdAt \
  --jq '.comments[-3:] | map({author: .author.login, at: .createdAt, body: .body[0:200]})'
```

Discount label-only churn and bot comments (`gemini-code-assist`, `renovate`, release automation,
the Cloud Build failure reporter). What's left is the last substantive human comment, and that date
drives every call below.

When the sweep is scoped to one area rather than the whole repo, widen the search window and filter
on last human comment yourself, so bot-warmed items can't hide behind the cutoff.

### Step 4: Check whether it already resolved itself

The highest-value step, and the one a stale bot structurally cannot do. Before drafting any nudge or
close, check whether the thing fixed itself while nobody was looking:

- `git log --oneline --since=<created date> --grep=<term>`, plus a grep for the feature or symbol.
- For a bug, trace the code path and see whether the defect is still there.
- Search for a newer issue that superseded it.

Either outcome leaves the stale buckets entirely, and both beat silence followed by a timeout:

- **Fixed:** close as fixed in `<sha>`, shipped in vX.Y.Z, citing the commit. That's a real answer,
  not a stale close.
- **Superseded:** `duplicate` plus a link to the newer issue.

### Step 5: Sort by whose silence it is

Stale is not the same as closeable. Put every remaining candidate in exactly one bucket.

**Blocked on the author.** Carries `status: waiting for response` or `status: feedback wanted`, or
the last comment is a maintainer question that never got a reply.

- Past 60 days of silence: propose the close, per the playbook rule.
- Under 60 days: propose a nudge that restates the specific outstanding question and names a close
  date.

**Blocked on us.** A report filed with enough information and no maintainer reply, or a contributor
PR sitting on review or on maintainer-triggered CI.

- **Never propose closing these.** Timing out work that stalled on our side is the failure mode that
  costs contributor trust.
- Each one is an SLO miss, so the action is internal: name the owning team from the `product:` label
  per `blunderbuss.yml`, and give the SLO clock from the playbook table.
- Surface them loudly rather than letting them sit below the close list.

**Blocked on nobody.** Triaged feature requests in the backlog, `status: help wanted`,
`good first issue`, p2/p3 nice-to-haves.

- Silence is the expected state here, not rot. Leave them alone.
- Flag one only when it's gone genuinely obsolete (targets a removed source, or a design the project
  moved past) or when the labels are wrong.

When the bucket is a real toss-up, ask. A wrong nudge is cheap; a wrong close is not.

### Step 6: Apply the PR-specific rules

PRs go stale for reasons issues don't:

- **Auto-generated** (`renovate`, `release-please`): never nudge a bot, and draft no comment. Checks
  green means merge; superseded by a newer bot PR means close.
- **Draft** (`isDraft`): silence is expected. Leave alone unless it's months old, and then only a
  light "still planning to pick this up?".
- **Blocked on maintainer-triggered CI**: PRs from external forks can't run the integration tests,
  which need a maintainer's `tests: run` label or a `/gcbrun` comment. This is an on-us item wearing
  contributor-silence clothing, so check it before reading quiet as abandoned.
- **CLA, conflicts, or red CI**: nudgeable, but name the actual blocker instead of "any update?".
  When a CLA check fails though the human author has signed, the usual cause is an AI co-author
  trailer; suggest squashing to a single human-authored commit.
- **Approved and unmerged**: not stale, just unmerged. Surface separately so it gets merged.

### Step 7: Draft the comments

Adapt the playbook's templates rather than inventing wording. Two shapes:

**Nudge**, naming the specific unanswered thing and a date:

```text
Hi @<author>, following up here. We're still blocked on <the specific thing: the repro steps, the Toolbox version, a reply to the question in <link>>. If we don't hear back in the next 14 days we'll close this out, though you're welcome to comment or reopen any time after that.
```

**Close**, giving the reason, reopenability, and thanks:

```text
Closing this out: we weren't able to move forward without <the missing thing>, and it's been quiet for <N> days. That's not a judgment on the report, so please comment or reopen if you're still hitting this and we'll pick it back up. Thanks for taking the time to file it!
```

Keep two things out of both. A form-letter tone, since these are hand-sent precisely so they don't
read like a bot. And any phrasing that puts the delay on the contributor when part of it was ours.

## Rules

- **Propose only.** Never run `gh issue close/comment/edit`, `gh pr close/comment/merge`, or apply a
  label. Deliver the sweep in chat.
- **Never close what's blocked on us.** The only valid outputs there are a reply, a review, or a
  `tests: run`.
- **Only real labels**: every one must appear in `gh label list`, and no `stale` label exists.
- **Show the last human comment date** for every candidate, never `updatedAt`, so the maintainer can
  check your arithmetic.
- **Ground the call.** A close cites the playbook rule and the silence duration; an "already fixed"
  cites a commit SHA or `file:line`; a routing claim cites `blunderbuss.yml`. Mark anything you
  couldn't verify `[UNVERIFIED]` rather than asserting it.
- **Disclose the edges of the sweep**: the window, the result cap if you hit it, and anything you
  skipped. Silence about coverage reads as "I checked everything".

## Output format

```text
## Stale sweep: <N>-day window (cutoff <YYYY-MM-DD>), <X> candidates

**Already resolved, close with an answer** (<n>)
| # | title | resolved by | last human |

**Close (author silent >60d)** (<n>)
| # | title | last human | what we asked for |

**Nudge author** (<n>)
| # | title | last human | the specific ask |

**On us (SLO miss, do not close)** (<n>)
| # | title | waiting since | owning team | SLO status |

**PRs (merge or close)** (<n>)
| # | title | why |

**Leave alone** (<n>)
- #<n>: <why the silence is fine>

**Draft comments**
### #<n>: <close | nudge | resolved>
<paste-ready body>

**Apply:**
gh issue comment <n> --repo googleapis/mcp-toolbox --body "..."
gh issue close <n> --repo googleapis/mcp-toolbox --comment "..."
```

- **Empty buckets:** omit them, except **On us**, which is always stated even when empty. "Nothing
  is rotting on our side" is the line the maintainer most wants confirmed.
- **Large sweeps:** classify in batches so items don't bleed together, and lead with the counts per
  bucket. The maintainer decides how deep to go from those counts.
