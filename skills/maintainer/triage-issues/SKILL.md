---
name: triage-issues
description: >-
  Triage GitHub issues in the googleapis/mcp-toolbox repo: propose the correct
  labels (type / priority / product / status), check for duplicates, verify a bug
  has enough info to act on, and draft a triage comment. Use whenever a maintainer
  asks you to triage, label, categorize, prioritize, or "look at" an issue (or a
  batch) in mcp-toolbox, e.g. "triage #3648", "what labels should this get", "is
  this a dup", triage a batch of issue numbers, or when they paste an mcp-toolbox
  issue link. PROPOSE-ONLY: delivers the triage in chat for the maintainer to apply;
  never edits labels, comments, closes, or assigns on its own.
---

# Triage Issues (mcp-toolbox)

Triage here is mostly manual **labeling**. The `product:` label auto-routes to the owning team
via `.github/blunderbuss.yml`. Propose only, never mutating the issue; when a call isn't obvious,
ask rather than guess.

## Goal

Given an issue number (or several), deliver a triage the maintainer can apply in seconds: labels
across the four axes (each with a one-line *why*), likely duplicates, missing bug info, and a
draft comment when one adds value.

## Prerequisites

- `gh` authenticated for `googleapis/mcp-toolbox`, plus the issue number(s). A GitHub MCP server
  substitutes for `gh` if it isn't available: the `gh` commands below map to its read/list tools.

## Guidance

**Read source-of-truth live, not from memory** (labels/routing drift):
- `gh label list --repo googleapis/mcp-toolbox --limit 200`: the only valid label names. Never propose one not listed.
- [`.github/blunderbuss.yml`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/blunderbuss.yml): which `product:` labels route to which team.
- [`.github/ISSUE_TEMPLATE/bug_report.yml`](https://github.com/googleapis/mcp-toolbox/blob/main/.github/ISSUE_TEMPLATE/bug_report.yml): required bug fields (for the completeness check).
- [references/maintainer-playbook.md](references/maintainer-playbook.md): the **authoritative**
  taxonomy, priority/status definitions, SLO targets, and comment templates. Read it to classify;
  this skill only adds how to apply them in a propose-only workflow.

Fetch the issue:
```bash
gh issue view <n> --repo googleapis/mcp-toolbox --json number,title,body,author,labels,comments,createdAt
```

**Check bot-noise first.** Automated reports (e.g. "Cloud Build Failure Reporter ... failed",
bot author) just need `periodic-failure`, so skip the rest of the workflow.

**The four axes** (the playbook has the definitions; below is only how to *apply* them here). Skip
an axis when it doesn't apply: say so, don't force a label.

- **`type:`**: the template pre-applies `type: bug`/`type: question`; trust it, but correct it
  when the body disagrees (a "bug" asking for new behavior is a feature request) and say why.
- **`product:`**: one per data source; **highest-leverage label**, since it drives team routing.
  Infer from the source/tool named (e.g. "looker run_dashboard tool" → `product: looker`). Don't
  force one on core/product-agnostic issues; note it has no product owner. If a real data source
  has no matching label yet, say so and flag it for `.github/labels.yaml` rather than mis-routing.
- **`priority:`**: **bugs are only ever p0 or p1** (never p2/p3); feature requests span p0-p3.
  Match the issue to the playbook's priority examples and SLO table, state the reason, and let the
  maintainer override.
- **`status:`** (optional): `waiting for response` when a bug lacks info to act, `feedback wanted`
  when waiting on community/author, `help wanted` for community-open work. See the playbook for
  exact semantics and the >60-day-silence close rule.

**Community labels** (standalone, not `status:`-prefixed): `good first issue` for well-scoped,
approachable issues; `ready for work` for triaged issues actionable now. Propose `good first issue`
alongside `help wanted` when the fix is small and self-contained.

**Assignment** (not a label): rarely needed, since `product:` auto-routes to the team. Propose an
assignee only when an external contributor volunteers (assign them, to avoid duplicate work) or a
maintainer is picking it up; otherwise leave it unassigned so contributors know it's open.

**Duplicates.** `gh issue list --repo googleapis/mcp-toolbox --state all --search "<key terms>" --limit 20`
with distinctive terms (tool name, error string). If it's a known issue, propose `duplicate` +
close: link and reference the original, and thank the reporter (template below).

**Investigate before deferring (bugs).** Before proposing `waiting for response`, try to reproduce
by tracing the code, and check `git log`/`git blame` for a fix that already landed silently. When
the call hinges on whether the bug is real, hand off to the `reproduce-bug` skill, which runs the
cheap discriminators (wrong tool type, version skew, client-side) and can reproduce most reports on
SQLite with no credentials; its verdict sharpens both `priority:` and `status:`. If
it's already fixed, propose `duplicate` + close referencing the commit rather than asking for info.
If you can root-cause it, include the `file:line` — it sharpens the priority call. Only fall back to
`waiting for response` when reproduction genuinely isn't possible, and then ask *specific*,
actionable questions, not "please provide more info". Compare against `bug_report.yml` required
fields (version, env, expected vs. current, repro) to list exactly what's missing; this feeds the
label and the draft comment.

**Feature requests & questions.** For a feature request, search the codebase for an existing or
partial implementation before drafting the acknowledgment — it may already be possible (answer and
close) or a duplicate. For a `type: question`, answer directly from the code with references rather
than only labeling it.

**Draft comment.** Use the playbook's canonical templates verbatim ("Needs More Information" for a
bug missing repro/details, "Acknowledging a Feature Request" for an FR), filling in specifics like
the exact missing fields; don't invent wording. For a duplicate:

```text
Thanks for reporting this! This is a duplicate of #<original>, so I'm closing this in favor of that issue. Please follow along there for updates.
```

## Rules

- **Propose only.** Never run `gh issue edit/comment/close` or assign. Deliver in chat.
- **Only real labels**: every proposed label must appear in `gh label list`.
- **When type/product is a genuine toss-up, ask.** A confident wrong `product:` mis-routes.
- **Cite what backs a claim.** Routing/label claims cite the config (e.g. "routes to
  `toolbox-looker-team` per `blunderbuss.yml`"); codebase claims (root cause, "already fixed",
  "already implemented") cite `file:line` or a commit SHA, or are marked `[UNVERIFIED]`.

## Output format

```text
## Issue #<n>: <title>

**Labels:**
- `type: <x>`: <why>
- `product: <x>`: <why + routing, or "no product owner: core/...">
- `priority: <x>`: <why>
- `status: <x>`: <why, if any>

**Duplicates:** <#nums, or "none for terms: ...">
**Completeness (bugs):** <what's missing, or "complete">
**Draft comment:** <paste-ready, or "none needed">

**Apply:** gh issue edit <n> --repo googleapis/mcp-toolbox --add-label "type: ...,product: ...,priority: ..."
```

Include the ready-to-run `gh issue edit` line so the maintainer applies with one paste, but leave
running it to them. For a batch, one block per issue plus a one-line summary table.
