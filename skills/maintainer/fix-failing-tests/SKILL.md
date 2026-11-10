---
name: fix-failing-tests
description: >-
  Diagnose a failing test in the googleapis/mcp-toolbox repo and land a fix by
  reasoning from the actual error: read the failure, reproduce it, shrink it
  until the cause is forced into the open, then fix the cause. Use this whenever
  a test or CI job is red, a build breaks after a change, many packages fail at
  once, or a test passes locally but fails in CI.
---

# Diagnose and Fix Failing Tests

Scope: `googleapis/mcp-toolbox`

Debug the way a careful developer does. Read the real error, find out what
changed, reproduce it, shrink it until the cause is forced into the open, then
fix the cause rather than the symptom.


## The Evidence Rule

**Every factual claim MUST cite evidence**: a path with line numbers, a build ID,
a log excerpt, or a SHA. Otherwise mark it `[UNVERIFIED]` and say what would
confirm it.

**Open every line you cite, including this skill's.** Ensure all pointers are
correct.

## 1. Read the actual failure

How you arrived decides what evidence you already hold and how much of step 2
you can skip.

| You arrived from | What you have | Where that puts you |
|---|---|---|
| A gate failing on your PR | A diff and a known-good parent | Your change is the prime suspect, but confirm the same job is green on `main` before assuming it |
| A `periodic-failure` issue | A build link buried in a long thread | Take the newest comment's link. The title often describes a different, already-fixed failure |
| An issue someone filed | A claim, often without a log | Establish that it still fails before anything else. Reports go stale, and the fix may have landed already |
| `main` is red | A range of commits, none of them yours | Bisect over merges. Say you are inheriting it rather than absorbing it into your change |
| Fails locally, green in CI | A reproduction | Suspect your environment: stale build cache, leftover local state, missing env, different Go version |
| Green locally, fails in CI | Neither | Suspect what CI does differently: `-race`, another OS, concurrency, a clean checkout, no cached state |

Whatever the door, get to the real error before theorising. Do not work from the
job summary or the last line of the log. Find the first real error and read it in
full.

- **Did a test fail at all?** Some red jobs contain zero failing tests: a
  compile error before the test step, a coverage threshold, a separate lint job,
  or a shard that skipped itself. Confirm a real test failure before debugging
  one. [references/ci-map.md](references/ci-map.md) lists the jobs that go red with everything green.
- **Which error is the cause?** Compile errors cascade, so the first is the
  cause and the rest are consequences. Assertion failures do not cascade, so
  read them all: three unrelated ones mean something different from thirty
  identical ones.
- **What kind of failure is it?** An assertion, a panic, a timeout, and a
  connection error each imply a different investigation. A connection error in
  a test that never touches the network usually means the process under test
  died.
- **What exactly was expected versus produced?** Get the diff, the test name,
  and the subtest name. "The test failed" is not a starting point.

## 2. Establish what changed

A test that passes on `main` and fails on your branch is a different problem
from one failing everywhere.

- Check whether the same job is red on `main`. If it is, you are inheriting
  someone else's failure and should say so rather than absorbing it into your
  change.
- Diff your branch against the last green commit. Look for the smallest change
  that could plausibly reach the failing code, including indirect reach through
  a shared helper or an interface.
- "Nothing relevant changed" is a hypothesis, not a fact. Shared infrastructure
  and concurrent CI runs are changes too.

## 3. Reproduce it

A fix you cannot demonstrate is a guess. Find the smallest command that fails
reliably, and run it before and after your patch.

```bash
go test -race -v ./path/to/package/                  # one package
go test -race -run 'TestX/subtest' -v ./path/...     # one subtest
go test -race -count=20 -run 'TestX' ./path/...      # does it always fail?
```

Use `-race` locally, because CI does, and a race can be green without it.

If you cannot reproduce it, that is itself a finding. Say so explicitly, state
what you would need (credentials, a build log, a concurrent run), and lower your
confidence rather than proceeding as if you had confirmed the cause.

## 4. Isolate

Shrink the failure until only the cause is left. Each run should answer one
question, so change one variable at a time.

| Axis | Run it both ways | What a difference tells you |
|---|---|---|
| Alone vs in suite | the single test, then the whole package | passing alone means shared state or ordering, not bad logic |
| With vs without your change | stash it and re-run | separates your regression from a pre-existing failure |
| Repeated | `-count=20` | distinguishes deterministic from intermittent |
| Order | `-shuffle=on` | exposes tests that depend on their neighbours |
| Race detector | with and without `-race` | isolates a genuine data race |
| Scope | one package, then many | one package is local logic, many is a shared cause |

Alone versus in-suite is the highest-information split: it separates "this test
is wrong" from "this test is a victim".

When many packages fail at once, resist debugging the first one alphabetically.
Look for the single shared thing they all touch, fix that, and re-run. The list
usually collapses. Usual candidates: a fake or helper nearly every package
imports, a shared struct compared field-by-field in table tests, a signature
every caller depends on, or registration via a blank import.

## 5. Explain it before you fix it

State one hypothesis as a prediction: *if this is the cause, then X should be
true.* Then check X. If you cannot phrase a prediction, you do not have a
hypothesis yet and should go back to isolating.

Before writing a patch, be able to answer:

- Why does it fail **now**, when it passed before?
- Why does it fail **here** and not in the tests that pass?
- Does the cause explain **every** symptom, including the ones you set aside?

An explanation that covers only some of the evidence is usually the wrong one.

Suspect a bystander when the symptom sits far from the cause. A shared deadline
that also bounds the process under test kills it on overrun, blaming whichever
test was running. A fixture collision can block rather than error, surfacing as
a downstream timeout. Teardown on an expired context fails silently, leaving
stale state for the next run.

## 6. Fix the cause

Decide what is actually wrong before editing.

- **The code is wrong.** The test caught a real defect. Fix the code.
- **The test is wrong.** It encoded an assumption that no longer holds. Change
  it, and say plainly what behaviour changed and why that is acceptable.
- **The conditions are wrong.** The test and code are both fine, but the
  environment is shared, slow, or ordered unfavourably. Fix the conditions.

Fix at the level the cause lives. A shared helper that breaks many packages gets
one edit there, not a hundred local patches.

Never loosen an assertion, add a retry, or extend a timeout to make red go away
unless you can explain why the original was wrong. Those are the three edits
that hide a real bug while looking like a fix.

Skipping is a last resort: only for a diagnosed intermittent failure that is
blocking others, and only with a tracking issue and re-enable condition in the
skip reason, since a bare "flaky" is how tests stay skipped for years. Never
skip a data race, a deterministic failure, or one you cannot explain, because an
unexplained failure is the most likely to be a real bug.

## 7. Verify

- Re-run the exact command that failed, and confirm it now passes.
- Re-run the wider package or suite, to check you did not move the failure.
- For anything intermittent, repeat it. A single green run proves nothing when
  the failure was one-in-five to begin with.
- Say what you did **not** verify.

## Output format

Report the diagnosis, then the diff, then anything left open.

```json
{
  "failure": "<test or job, exact name>",
  "kind": "assertion | compile | panic | timeout | connection | not-a-test-failure",
  "scope": "<how many packages, and what they share>",
  "deterministic": true,
  "reproduced": "<the exact command, or why not>",
  "evidence": ["<path:line or log excerpt> <what it shows>"],
  "root_cause": "<one sentence that explains every symptom>",
  "fix_level": "code | test | conditions",
  "action": "fix | escalate | no-change",
  "confidence": "high | medium | low, and why",
  "residual_risk": "<what this patch does not cover, and what you did not verify>"
}
```

State `residual_risk` honestly. Claiming certainty you do not have is worse than
a hedged claim, because the next person will trust it.

## Reference

[references/ci-map.md](references/ci-map.md) covers where tests run, how to get the logs, and the jobs
that go red with every test passing. Read it in step 1.
