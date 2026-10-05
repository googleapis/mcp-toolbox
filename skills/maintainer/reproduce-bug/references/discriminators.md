# Discriminator playbook

One section per false-alarm shape, ranked by how often it fires. Each gives the
confirmation step, the fix, and the traps.

The shapes are stable, since they are ways people misread a tool rather than
facts about the code. **The error wording is not stable.** Any error text quoted
here is an illustration, not a fingerprint. Confirm it against the tree with the
trace recipe in `SKILL.md` before quoting it to a reporter, and never conclude
"that string does not exist, so the report is bogus": it may be their version's
wording. Facts about *released* versions are frozen and safe to rely on; facts
about `main` drift.

## Wrong tool type

The most common misconfiguration. A source type is used where a tool type
belongs, usually `mysql` instead of `mysql-sql`, because the reporter copied the
source documentation page into their `tools:` block.

**Confirm.** Look up what is actually registered rather than trusting a list:

```bash
grep -rn 'resourceType string = ' --include="*.go" internal/tools/ | grep -i mysql
grep -rn 'SourceType string = ' --include="*.go" internal/sources/ | grep -i mysql
```

Tool types are registered in `internal/tools/**` and source types in
`internal/sources/**`, each via a `Register` call in `init()`. If a value only
appears under `internal/sources/`, it is not a tool type.

The raising sites are `ErrUnknownToolType` (`internal/tools/tools.go`) and the
`unknown source type` error in `internal/sources/sources.go`. Wording has
changed at least once, so match on the reporter's text, not on a remembered
string.

**Fix.** Change the value, not the field name. This fails before any connection,
so it reproduces at rung 1 with no database.

**Trap.** In legacy nested configs the field is spelled `kind:`, and that is
correct for that format. Do not tell a reporter to rename `kind` to `type`. See
the format mapping in [repro-env.md](repro-env.md).

## Version skew

Docs track `main`, releases lag, so a reporter follows current documentation
against an older binary and hits a field or flag that does not exist yet.

**Confirm.**

```bash
git log -S '<their exact error text>'                    # when did this wording exist
git log --oneline v<their-version>..HEAD -- <path>       # did the feature or fix land after them
```

An unknown-field error naming a field that *is* documented is the signature.

**Fix.** Point at the release that contains it, or note it is unreleased. This
is `already-fixed`, not `confirmed`, and it needs a release note rather than
code.

**Trap.** Trust their `--version` string over their prose. "Latest" and a
`:latest` container tag are not versions. See the compatibility table in
[repro-env.md](repro-env.md) for what changed when.

## Client-side

The failure is in an SDK, CLI, IDE extension, or the LLM itself, and the server
is behaving correctly.

**Confirm.** Replay the same call as raw JSON-RPC against `/mcp` with `curl`
(commands in [repro-env.md](repro-env.md)). A correct server response settles
it: the fault is above the transport.

**Traps.**
- An agent-fixable failure (bad SQL, missing record) returns **HTTP 200** with
  `isError: true` in the result, which reporters read as the server swallowing
  their error. An infrastructure or auth failure returns a **4XX/5XX** JSON-RPC
  error instead, so a non-200 is not automatically wrong. Check which category
  applies before judging the response: see the error taxonomy in
  [repro-env.md](repro-env.md).
- A request to `/api/*` judged against MCP expectations is a category error.
- The bug may belong in an SDK repo. Say which one rather than closing flatly.

## Parameter semantics

Parameters bind *values*. A placeholder in an identifier position never worked
and is not a regression.

**Confirm.** Locate the placeholder in the statement.

- Value position, `WHERE id = $1`: supported.
- Identifier position, `FROM $1` or `DESCRIBE $1`: works as intended. Template
  parameters are the supported mechanism.

**Trap.** Placeholder syntax is dialect-specific (`?` for MySQL and SQLite,
`$1` for Postgres, `@name` for MSSQL and BigQuery). The wrong style for the
dialect is a *different* finding from the wrong position. Confirm the dialect's
style from its tool implementation under `internal/tools/` before calling it.

## Deliberate behaviour change

Something that worked stopped working at an upgrade because a flag now gates it.

**Confirm.** Diff their flags against the release notes for their version bump,
and find the commit with `git log --oneline --grep='feat!' v<theirs>..HEAD`.

The known instance is `--enable-api`: `/api` was ungated before v0.31.0, off by
default from v0.31.0, and the *status code when off changed between releases*
(bare 404 on v0.31.0, 410 with an explanatory message after). Match on the
endpoint plus the upgrade, never on the status code.

**Trap.** A gate that surfaces as a bare 404 with no explanation is a real
usability bug even when the gating is intended. Record it under `secondary`.

## Database-side

Permissions, a missing extension, a firewall, or a maintenance state. Outside
Toolbox entirely.

**Confirm.** Ask whether the identical statement with the identical credentials
succeeds through `psql`, `mysql`, or the provider console. If it fails there
too, Toolbox is the messenger.

**Trap.** Toolbox mishandling a database error *is* a Toolbox bug. Returning a
silent nil instead of surfacing a permission error has been a real defect. The
question is not "did the database fail" but "did Toolbox report the failure
faithfully".

## Credential escaping

Passwords with reserved characters, or `${ENV_VAR}` substitution.

**Confirm.** Retry with a trivial alphanumeric password. If it succeeds, the
issue is escaping rather than authentication.

**Trap.** Do **not** auto-dismiss this shape. Both connection-string escaping
and environment-variable substitution have been genuine bugs. "Their password
had a special character" is the start of the investigation, not the verdict.
Never paste a real credential into a config, a command, or the report.
