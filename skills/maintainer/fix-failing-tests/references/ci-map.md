# Where tests run, and how to see the failure

Read this in step 1, when you need the real error.

## Two tiers

| Tier | Where | Runner | Run it locally |
|---|---|---|---|
| **unit** | `internal/`, `cmd/` | GitHub Actions, `.github/workflows/tests.yml`, matrix of ubuntu + macos + windows, `-race` on all three, `fail-fast: false` | `go test -race -v ./cmd/... ./internal/...` |
| **integration** | `tests/` | Cloud Build, `.ci/integration.cloudbuild.yaml`, change-gated shards against real cloud infrastructure | usually not reproducible without credentials |

Unit tests are fast, hermetic and deterministic, so an intermittent unit failure
is unusual and normally means shared state, time dependence, or a goroutine leak
caught by `-race`. Integration tests are slow and share live infrastructure
between concurrent builds, so intermittent failures there are ordinary.

`tests.yml` runs `go build -v ./...` before any test step, and a job stops at
its first failing step. A compile error therefore produces a red job with no
test output at all.

## Red job, green tests

These fail without a single failing test, which makes them the most misread
failures in the repo.

| Job | Signature | What it is |
|---|---|---|
| Build | No test output; the job dies early | Compile error before the test step. Read the **first** error, not the last: they cascade |
| Unit coverage gate | All pass, Linux job red, log says coverage below 40% | The gate drops `internal/server/config.go` from `coverage.out`, then fails under 40% |
| Lint | Tests green, red on a job that ran no tests | `.github/workflows/lint.yml` is separate: `go mod tidy` with `git diff --exit-code`, then `golangci-lint`. Fix with `goimports -w . && go mod tidy && golangci-lint run`. There is no standalone `gofmt` or `go vet` step |
| Integration coverage gate | All pass, shard still red | Most shards call `.ci/test_with_coverage.sh`, which fails below 50% |
| Skipped shard | Green but suspiciously fast; "No relevant changes ... Skipping shard" | The shard opted out via `detect-changes`. It never ran, so green is not a signal |
| Silently skipped suite | Suite passes, but a required secret was missing | Some suites `t.Skip` on missing env where most `t.Fatal`. A green shard can mean zero coverage |


## Getting integration logs

However you arrived (a PR gate, a filed issue, a red `main`, or the automated
reporter), the integration log lives on Cloud Build and you have to go get it.

Scheduled failures arrive through
`.github/workflows/cloud_build_failure_reporter.yml`, driven by
`schedule_reporter.yml` daily at 06:00 UTC, which opens or comments on an issue
labelled `periodic-failure`. Two things to know about that path: it never closes
an issue (when the check passes it just comments "Tests are passing"), so one
thread accumulates unrelated failures for weeks, and the
title describes whichever failure came first. Always take the newest comment's
build link. For a PR gate you already have the build link from the check itself,
so skip straight to the log.

Builds run on a private `workerPools/integration-testing` pool, so VPC and quota
problems appear as connection errors inside otherwise healthy tests. That is
infrastructure, not a failing test.

## Test conventions

Unit tests are near-uniformly **table-driven subtests compared with `go-cmp`**,
with hand-written fakes in `internal/testutils/mocks.go` rather than generated
mocks. Match that style; do not introduce another assertion library. There are
no golden files or `testdata/` directories, so nothing needs regenerating.

Integration tests share helpers in `tests/server.go`, `tests/tool.go` and
`tests/common.go`, which compile into every shard, so an edit there reaches all
of them.
