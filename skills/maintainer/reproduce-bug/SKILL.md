---
name: reproduce-bug
description: >-
  Reproduce a reported bug in googleapis/mcp-toolbox and decide whether it is
  real, delivering an evidence-backed verdict: confirmed, already fixed,
  misconfiguration, client-side, works as intended, not reproducible, or
  blocked. Use whenever a maintainer asks you to reproduce, verify, confirm, or
  investigate a bug report, asks "is this a real bug", "can you repro this",
  "does this still happen", or pastes an mcp-toolbox bug issue or stack trace to
  check. PROPOSE-ONLY: reproduces locally and reports; never comments on,
  labels, or closes the issue.
---

# Reproduce a Bug Report (mcp-toolbox)

Scope: `googleapis/mcp-toolbox`

About half of reported bugs are not bugs in Toolbox server code: they are
misconfiguration, a client-side problem, or something already fixed in a version
the reporter has not upgraded to. So the job is not "make it fail", it is to
reach a **defensible verdict at the lowest cost**. Work cheapest-first and stop
the moment the evidence settles it. Building a full reproduction for something a
one-line version check would have explained is the main way this goes wrong.

## The Evidence Rule

**Every factual claim MUST cite evidence**: a `file:line`, a commit SHA, a
version string, or the exact command and its output. Otherwise mark it
`[UNVERIFIED]` and say what would confirm it.

Cite `file:line` in **your findings**, where the claim is about the tree as it
is now. This skill's own pointers name files and symbols instead, since line
numbers go stale between releases while symbols survive refactors. Resolve one
with `grep -rn '<symbol>' --include="*.go"` and read it before citing a line.

Reproducing a bug proves it exists. Failing to reproduce proves much less, and
only counts if the attempt was valid, so word the two differently.

## Verdicts

Every run ends in exactly one:

| Verdict | Means |
|---|---|
| `confirmed` | Reproduced on current `main`. Real, still open. |
| `already-fixed` | Reproduces on their version, not on `main`. Needs a release note or a link to the fix. |
| `misconfiguration` | Their config or environment. |
| `client-side` | In an SDK, CLI, or LLM client, not the server. Often belongs in another repo. |
| `works-as-intended` | Deliberate behaviour, frequently a breaking change they have not read. |
| `not-reproducible` | The attempt was valid and it did not fail. |
| `blocked` | Needs infrastructure or credentials you do not have. Say exactly what. |

## 1. Pin the claim down

Read the issue and all comments before touching code.

```bash
gh issue view <n> --repo googleapis/mcp-toolbox --json number,title,body,author,labels,comments,createdAt
```

Extract, noting what is missing:

- **Version.** Highest-value field. `1.9.0+binary.linux.amd64` is usable;
  "latest", a `:latest` tag, or blank are not.
- **The full config.** Reporters routinely paste only `sources:` when the fault
  is in `tools:`.
- **The verbatim error**, not a paraphrase, with `--log-level DEBUG` output if
  it exists.
- **Transport and client.** Direct `curl`, an SDK, Gemini CLI, an IDE? The bug
  template's `Client` field is often an unfilled `<placeholder>`.
- **Expected versus actual**, separately. A report that says only what happened
  may be a disagreement about intent.

State the claim in one falsifiable sentence: *given config X, action Y produces
Z, and should produce W.* If you cannot write it, you do not yet know what to
reproduce, and that is the finding.

**Check it is not already answered.** Search duplicates and scan
`git log`/`git blame` for a fix that landed quietly. Closed issues carry almost
no signal: nearly all closed bugs here are closed `COMPLETED` whether or not
they were real, so read the thread. If a maintainer already diagnosed it, still
reach your own verdict, then say whether you agree.

## 2. Run the cheap discriminators

Check the report against these shapes before standing anything up. Each is
seconds of work; they are ranked by how often they fire.

| Shape | Recognise it by |
|---|---|
| **Wrong tool type** | A tool whose type is a *source* type, missing the `-sql` or `-execute-sql` suffix. Fails at startup, before any connection. |
| **Version skew** | An unknown-field or missing-feature error for something the docs describe. Docs track `main`; their binary lags. |
| **Client-side** | The failure surfaces in an SDK, CLI, or IDE frame, or against `/api/*` while MCP semantics are expected. |
| **Parameter semantics** | The placeholder sits in an identifier position (`FROM $1`) rather than a value position (`WHERE id = $1`). |
| **Deliberate behaviour change** | It broke at an upgrade, and a flag now gates what used to be on by default. |
| **Database-side** | The same statement with the same credentials fails outside Toolbox too. |
| **Credential escaping** | A password or `${ENV_VAR}` holding characters that need escaping. |

The table is only an index of what to suspect. When a row plausibly matches,
read that section of
[references/discriminators.md](references/discriminators.md) for how to confirm
it, the fix, and the traps, including which shapes are sometimes real bugs.

**Trace error text rather than matching it from memory**, since wording changes
between releases and their string is evidence about *their* build:

```bash
grep -rn '<distinctive fragment>' --include="*.go" internal/ cmd/   # where it is raised now
git log -S '<their exact error text>'                             # when the wording changed
```

No grep hit means version skew, not an invented error: their wording predates
`HEAD`, and `git log -S` dates the build and finds what replaced it.

One question ranks above every shape: **does the failure happen before or after
the source connects?** Config parsing, type validation, and manifest generation
run before any connection, so they reproduce with zero infrastructure no matter
which exotic database the report names.

## 3. Climb only as far as you need

Start at the lowest rung that could settle the claim.

| Rung | Cost | Settles |
|---|---|---|
| **0. Read the code** | seconds | Works-as-intended, obvious misconfiguration, already-fixed. Trace the path, check `git log`. |
| **1. Parse their config** | seconds | Config schema, type validation, field names. Feed their YAML to the binary, read the startup error. |
| **2. SQLite, no infrastructure** | a minute | Parameter binding, manifest shape, invocation path, protocol and error envelopes. Covers most reports. |
| **3. Local container** | minutes | Driver behaviour, connection strings, engine-specific SQL. |
| **4. Real cloud instance** | slow, needs credentials | IAM and ADC, managed-service behaviour. Prefer `blocked` with a precise ask over a half-done attempt. |

Rung 2 is the workhorse and the one most often skipped by mistake: a bug
reported against BigQuery or AlloyDB frequently reproduces on SQLite whenever
the fault is upstream of the connection.

Reproduce on **their** version first when they gave one, then on `main`. That
comparison separates `confirmed` from `already-fixed`, and it is the difference
between a fix and a release note. Their version is a different environment, not
just older code: flag names and the accepted config format have both changed, so
match the form their version accepts rather than the recipe this skill hands
you, or the run fails for reasons unrelated to the bug and reads like a
reproduction. [references/repro-env.md](references/repro-env.md) has the
compatibility table and the commands.

## 4. Shrink it, then prove the cause

Cut everything not required: one tool, one parameter, SQLite if the engine is
not the point. Each thing you remove that keeps it failing is a thing the bug
does not depend on, which is what turns a reproduction into a root cause. Report
the smallest failing case, not the reporter's original.

**Then flip the one thing you claim is the cause and show the failure moves.** A
reproduction plus a code read tells you what is broken; changing the suspected
cause and watching the error change is what proves it. This matters most for
`misconfiguration`, where you are about to tell someone their config is at
fault. Landing on a *different* error, a connection refused say, is a pass: it
shows the original fault was the only one in the way.

If it does **not** fail, establish that your attempt was valid before concluding
anything: version, config form, flags, transport, parameter values. An attempt
that silently differed from theirs is not evidence. Turn each gap you could not
close into a precise question for the reporter.

## Rules

- **Propose only.** Never run `gh issue comment`, `edit`, or `close`. Deliver
  the verdict and any draft comment in chat.
- **Work in a scratch directory**, never committing a reproduction config. When
  the working tree is too dirty to build, use a throwaway worktree
  (`git worktree add /tmp/tb-verify HEAD --detach`), remove it after, and note
  that you tested `HEAD` rather than their tree. Never `checkout`, `restore`, or
  `stash` the user's changes to get a build.
- **Never paste a reporter's credentials** anywhere. Substitute placeholders.
- **Do not ask for what you can derive.** A round-trip costs the reporter days.
- **Separate the bug from the message.** A confusing error that led someone to
  file a wrong report deserves its own issue even when the verdict is
  `misconfiguration`.
- **Leave the door open.** Greet the reporter, name the concrete cause, link the
  fix or docs, invite them to reopen. Never "invalid" or "wontfix". Reporters
  cannot always reopen, so a wrong close strands them.

## Output format

```json
{
  "issue": "<number and title>",
  "claim": "<the falsifiable one-sentence restatement>",
  "verdict": "confirmed | already-fixed | misconfiguration | client-side | works-as-intended | not-reproducible | blocked",
  "rung": "0 read | 1 config | 2 sqlite | 3 container | 4 cloud",
  "versions_tested": ["<version> <what it showed>", "<version> not tested: <why>"],
  "repro": "<smallest failing command and config, or why none>",
  "cause_proven_by": "<the change that moved the failure, or null>",
  "evidence": ["<file:line, SHA, or command output> <what it shows>"],
  "root_cause": "<one sentence explaining every symptom, or null>",
  "secondary": "<separate issue worth filing, e.g. a misleading error, or null>",
  "missing_info": ["<specific question for the reporter>"],
  "confidence": "high | medium | low, and why",
  "not_verified": "<what you did not test>"
}
```

Use `[]` for a list with genuinely nothing in it; that is a result, not a gap.
Record a version you deliberately skipped in `versions_tested` with the reason,
rather than dropping it and implying you covered it. Follow the block with the
smallest reproduction as a paste-ready snippet, and a draft comment when one
adds value.

## Reference

- [references/discriminators.md](references/discriminators.md): per-shape
  playbook for step 2.
- [references/repro-env.md](references/repro-env.md): commands, flags, the
  SQLite recipe, the version compatibility table, and older-version builds.

Related: `triage-issues` for labeling and priority once the verdict is known,
`fix-failing-tests` when the reproduction is a failing test rather than a report.
