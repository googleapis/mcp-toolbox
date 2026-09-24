# Reproduction environment (mcp-toolbox)

Verified mechanics for standing up a reproduction. Commands were run against
`v1.9.0`. Flags and kinds drift, so if something here disagrees with
`go run . --help` or the source, trust the repo.

**Everything below describes current `main` unless marked otherwise.** When you
reproduce on a reporter's older version, read
[Version compatibility](#version-compatibility) first. Flag names, the config
format, and error text have all changed, so a recipe from this file can fail on
their binary for reasons that have nothing to do with their bug.

## Build and run

General dev setup, cross-compilation, and linting are in
[DEVELOPER.md](DEVELOPER.md). Below is only what a reproduction needs.

```bash
go run . --help                         # every flag, authoritative
go run .                                # serve on 127.0.0.1:5000, reads ./tools.yaml
go build -o toolbox && ./toolbox --version
```

`--version` prints `<version>+<buildType>.<GOOS>.<GOARCH>[.<sha>]`, where
buildType is `dev`, `binary`, or `container` (`cmd/root.go`, `semanticVersion`). A reporter
saying "dev" built from source; "binary" or "container" came from a release.

Subcommands: `invoke`, `serve`, `migrate`, `skills-generate` (`cmd/root.go`).

## Flags that matter for reproduction

Defined in `cmd/internal/flags.go`:

- **Config** (mutually exclusive): `--config`, `--configs`, `--config-folder`,
  `--prebuilt`. The names `--tools-file`, `--tools-files`, and `--tools-folder`
  are deprecated aliases that still work, so an old bug report using them is not
  itself the fault. Default when nothing is passed: `tools.yaml`.
- **Serve**: `--address` (`127.0.0.1`), `--port` (`5000`), `--stdio`, `--ui`,
  `--enable-api`, `--allowed-origins` (`*`), `--allowed-hosts` (`*`),
  `--tls-cert`, `--tls-key`, `--disable-ext`.
- **Diagnostic**: `--log-level` (ask reporters for `--log-level DEBUG` output),
  `--logging-format`.

## Config format

Current format is flat, multi-document YAML: one `kind:` per document, naming
the resource class (`source`, `tool`, `group`), with `type:` as the
discriminator inside it (`internal/server/config.go`, `UnmarshalPrimitiveConfig`).

The legacy nested form (top-level `sources:`, `tools:`, `toolsets:`,
`authServices:`, `prompts:`, `groups:`) is still parsed and converted
(`cmd/internal/config.go`, `ConvertConfig` and `nestedFormatKey`), and `kind: toolset` folds into
`kind: group`. A reporter using the nested form is not out of date in a way that
breaks anything, so do not chase it as the cause.

**The two forms spell the discriminator differently, and this trips up almost
every older report.** In the nested form the per-entry field is `kind:`; in the
flat form that same value is `type:`, because `kind:` is taken by the resource
class. These are equivalent:

```yaml
# nested (legacy)             # flat (current)
sources:                      kind: source
  my-db:                      name: my-db
    kind: mysql               type: mysql
```

So when a report says `kind: mysql` under a `tools:` block, the fault is the
*value* (`mysql` is a source type, the tool type is `mysql-sql`), not the field
name. Do not tell a reporter to rename `kind` to `type` as the fix.

Convert a nested config to flat with `go run . migrate --config tools.yaml`.

## Zero-infrastructure repro: SQLite

The `sqlite` source needs no credentials, no server, and no CGO (pure-Go driver
`modernc.org/sqlite`, `internal/sources/sqlite/sqlite.go`). Fields:
`name`, `type`, `database` (all required), `sqlCommenter`. `database` accepts
`:memory:`. Tool types: `sqlite-sql`, `sqlite-execute-sql`, with `?` positional
placeholders.

This config plus the command below is a complete, verified reproduction:

```yaml
kind: source
name: repro-db
type: sqlite
database: ":memory:"
---
kind: tool
name: repro_tool
type: sqlite-sql
source: repro-db
description: Minimal repro
statement: SELECT ? AS echo;
parameters:
  - name: echo
    type: string
    description: value to echo
```

```bash
$ go run . invoke --config repro.yaml repro_tool '{"echo":"hello"}'
[
  {
    "echo": "hello"
  }
]
```

Swap in the reporter's tool definition, keeping their parameter shapes and
statement structure. This isolates config parsing, type validation, parameter
binding, manifest generation, and the invocation path from anything
database-specific.

No toolset is needed: every tool lands in an automatically created `default`
group, which is enough for `invoke` and `/mcp`. Add an explicit `kind: group`
document only when the report is about toolset or group behaviour itself.

A larger worked example lives at `tests/conformance/tools.yaml`.

## Three ways to invoke a tool without an LLM

**1. `invoke` subcommand.** No server, fastest loop. Params are a JSON string
(`cmd/internal/invoke/command.go`):

```bash
go run . invoke --config repro.yaml repro_tool '{"echo":"hello"}'
```

**2. `/mcp` JSON-RPC.** Always mounted (`internal/server/server.go`, the `/mcp` mount). Use
this when the report involves a client, since it is the transport real clients
use, and it shows the exact error envelope they saw:

```bash
go run . --config repro.yaml --port 5111 &
curl -s -X POST http://127.0.0.1:5111/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}'
curl -s -X POST http://127.0.0.1:5111/mcp -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"repro_tool","arguments":{"echo":"hi"}}}'
# {"jsonrpc":"2.0","id":1,"result":{"content":[{"type":"text","text":"{\"echo\":\"hi\"}"}]}}
```

Error shape decides the client-side verdict (`internal/util/errors.go`). An
`AgentError`, anything the agent could fix such as bad SQL or a missing record,
returns **HTTP 200 with `isError: true`**, which reporters misread as the server
swallowing their failure. A `ClientServerError`, infrastructure or auth, returns
a **4XX/5XX JSON-RPC error**. Check which category applies before calling either
wrong: [DEVELOPER.md](DEVELOPER.md), "Tool Invocation & Error Handling".

**3. `/api` REST.** Disabled by default. On current `main`, without
`--enable-api` every `/api/*` path returns **HTTP 410** with a message pointing
at `/mcp` (`internal/server/server.go`, the `cfg.EnableAPI` branch). On **v0.31.0 only** the same
gate returned a bare **404**, because `/api` was simply left unmounted with no
fallback handler, so a v0.31.0 report of "404 on /api" is this gate too. Either
status after upgrading is the gate, not a regression. Routes
(`internal/server/api.go`, `apiRouter`):
`GET /api/toolset`, `GET /api/toolset/{name}`, `GET /api/tool/{name}`,
`POST /api/tool/{name}/invoke`. The invoke body is a bare JSON object of
parameter name to value.

**UI**: `go run . --config repro.yaml --ui`, then `http://localhost:5000/ui`.

## Tests

Test commands and the integration-test environment setup live in
[DEVELOPER.md](DEVELOPER.md), "Testing". Three things it does not say that
matter when reproducing:

- Integration tests read credentials at package init and call `t.Fatal` on a
  missing variable, so they fail rather than skip.
- `tests/sqlite` is the exception, creating a temporary database when
  `SQLITE_DATABASE` is unset, so it runs anywhere.
- There is no Makefile and no docker-compose, so a local container for a real
  engine has to be started by hand.

## Testing against the reporter's version

```bash
git log --oneline v1.8.0..HEAD -- internal/tools/postgres/   # did a fix land since?
```

Build an old version in a throwaway worktree rather than checking out the tag in
place, so the working tree is left alone:

```bash
git worktree add /tmp/tb-v1.8.0 v1.8.0 --detach
cd /tmp/tb-v1.8.0 && go build -o toolbox && ./toolbox --version
# when finished
git worktree remove /tmp/tb-v1.8.0 --force
```

Released binaries, no build required:

```bash
curl -O https://storage.googleapis.com/mcp-toolbox-for-databases/v1.8.0/darwin/arm64/toolbox
```

Containers: `us-central1-docker.pkg.dev/database-toolbox/toolbox/toolbox:1.8.0`.

### Building when the working tree is broken

Maintainer checkouts are dirty constantly, and an unrelated local edit will fail
the build in a way that looks like the reported bug. Build from a clean worktree
at `HEAD` instead of reverting anything:

```bash
git worktree add /tmp/tb-verify HEAD --detach
cd /tmp/tb-verify && go run . invoke --config repro.yaml repro_tool '{}'
git worktree remove /tmp/tb-verify --force
```

Never `git checkout`, `restore`, or `stash` the user's changes to get a build.

## Version compatibility

Mechanics that changed under reporters' feet. Check the reporter's version
against this before concluding their setup is wrong, and confirm flag names
against their binary's own `--help` rather than this file.

| Area | Then | Now |
|---|---|---|
| Config flag | `--tools-file` was the only name before **v0.31.0**; `--config` errors there | `--config` (plus `--configs`, `--config-folder`); `--tools-file` still works as a deprecated alias |
| Config format | Nested only before **v0.30.0**, so the flat recipe in this file will not parse on an older binary | Flat multi-document, nested still accepted |
| `/api` gate | Ungated before **v0.31.0**; bare 404 when off in **v0.31.0** | 410 with an explanatory message from **v0.32.0** |
| Unknown tool type | `"<x>" is not a valid kind of tool` before **v0.7.0** | `unknown tool type: "<x>"` |

When a report quotes an error string that does not exist in the current tree,
that is version skew, not a phantom. Find when the wording changed with
`git log -S '<their exact error text>'`.

## What genuinely needs real infrastructure

Engine-specific SQL behaviour, driver and connection-string handling, IAM and
ADC, dialect differences, and connection pooling. Everything upstream of the
connection, including config parsing, kind validation, parameter binding, the
tool manifest, and protocol shape, reproduces on SQLite.
