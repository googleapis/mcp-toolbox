#!/bin/bash
# Copyright 2026 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Runs one prebuilt config's evalset against each harness in EVAL_HARNESSES,
# inside the EvalBench image. Everything per-database comes from the calling
# Cloud Build step's env, including EVAL_ENV_PREFIX, the prefix its settings
# share.
#
# Database is a step because that is where credentials live; harness is a loop
# here so adding one does not duplicate those credential blocks.

set -euo pipefail

# Both early exits stay above the trap, which needs TOOLBOX_PREBUILT.

# detect-changes writes this when the pull request lacks the 'evals: run' label.
if [[ -f /workspace/skip-evals ]]; then
  echo "no 'evals: run' label; skipping"
  [[ "${1:-}" == "--check-any" ]] && exit 1
  exit 0
fi

# Paths that put every step in scope. internal/tools/ is here because most tool
# descriptions default in Go rather than in the prebuilt config, so the change an
# eval is meant to measure often touches no path this script can attribute to a
# single database.
common_pattern='(^|/)evals/(run_configs|model_configs|setup|teardown)/|(^|/)internal/tools/|\.ci/evals\.cloudbuild\.yaml|\.ci/run_evals\.sh'

# --check-any: true when any step's evals would run, so build-toolbox can skip
# the compile.
if [[ "${1:-}" == "--check-any" ]]; then
  [[ -s /workspace/changed_files.txt ]] || exit 0
  grep -qE "${common_pattern}" /workspace/changed_files.txt && exit 0
  # Every step's evalset and prebuilt config, read back out of the build config.
  targets=$(sed -n -e 's|.*EVAL_DATASET=\([^"]*\).*|\1|p' \
                   -e 's|.*TOOLBOX_PREBUILT=\([^"]*\).*|internal/prebuiltconfigs/tools/\1.yaml|p' \
                   /workspace/.ci/evals.cloudbuild.yaml)
  grep -qxF "${targets}" /workspace/changed_files.txt && exit 0
  exit 1
fi

# The step is allowFailure, so a database that fails does not cancel the others
# mid-run. This marker is what the build's last step fails on.
trap '[ $? -eq 0 ] || touch "/workspace/failed-${TOOLBOX_PREBUILT}"' EXIT

# Empty values fail quietly, which is why they are checked: ALLOW_LOOSE blanks
# an undefined substitution, empty postgres credentials degrade to IAM auth so
# every tool disappears, and an empty EVAL_REPORTING_PROJECT falls back to the
# build project.
required=("EVAL_GCP_PROJECT_REGION" "EVAL_REPORTING_PROJECT" "EVAL_HARNESSES")
for prefix in ${EVAL_ENV_PREFIX}; do
  matched=($(compgen -v "${prefix}_" || true))
  if [ ${#matched[@]} -eq 0 ]; then
    echo "EVAL_ENV_PREFIX=${prefix} matched no environment variables" >&2
    exit 1
  fi
  required+=("${matched[@]}")
done

for var in "${required[@]}"; do
  if [ -z "${!var:-}" ]; then
    echo "missing required environment variable: ${var}" >&2
    exit 1
  fi
done

# Exact-line matches, since git diff emits repo-relative paths. Only a non-empty
# list can narrow the run: empty means a scheduled build, or a checkout the diff
# failed against.
if [[ -s /workspace/changed_files.txt ]] &&
   ! grep -qxF -e "${EVAL_DATASET}" \
               -e "internal/prebuiltconfigs/tools/${TOOLBOX_PREBUILT}.yaml" \
               /workspace/changed_files.txt &&
   ! grep -qE "${common_pattern}" /workspace/changed_files.txt; then
  echo "no changes affecting ${TOOLBOX_PREBUILT}; skipping"
  exit 0
fi

echo "prebuilt config: ${TOOLBOX_PREBUILT}"
echo "harnesses:       ${EVAL_HARNESSES}"
"${TOOLBOX_BIN}" --version  # built on Debian, exec'd here on Ubuntu

# EvalBench resolves config paths against its own working directory. -T so a
# rerun copies onto the existing directory instead of nesting inside it.
cp -rT /workspace/evals /evalbench/evals

# Resolve and check every harness before running any, so a typo fails in
# seconds rather than after the first one has burned an hour.
harnesses=(${EVAL_HARNESSES})
for harness in "${harnesses[@]}"; do
  if [ ! -f "evals/model_configs/${harness}.yaml" ]; then
    echo "no such model config: evals/model_configs/${harness}.yaml" >&2
    exit 1
  fi
done

ulimit -n 4096

# Per-build resource name for the setup and teardown scripts, so concurrent
# builds cannot collide. Hyphens go because BigQuery dataset ids reject them;
# exported because the evalset loader expands ${EVAL_RUN_ID} too.
EVAL_RUN_ID="${BUILD_ID:-local}"
export EVAL_RUN_ID="${EVAL_RUN_ID//-/_}"

# Lower bound for teardown, so it cannot reach resources that predate this build.
# Its absence is also how teardown knows the evals never ran.
date -u +%Y-%m-%dT%H:%M:%SZ > "/workspace/eval-start-${TOOLBOX_PREBUILT}"

# One process per harness: pyaml_env resolves !ENV at load time, so a single
# process could not vary EVAL_MODEL_CONFIG between runs.
overall_exit=0
for harness in "${harnesses[@]}"; do
  echo "::: ${TOOLBOX_PREBUILT} x ${harness}"
  export EVAL_MODEL_CONFIG="evals/model_configs/${harness}.yaml"

  set +e
  uv run --no-sync evalbench/evalbench.py \
    --experiment_config="evals/run_configs/toolbox.yaml"
  eval_exit=$?
  set -e

  # Keep going so one broken harness does not hide the others' results, but
  # still fail the step.
  if [ $eval_exit -ne 0 ]; then
    echo "harness ${harness} failed with exit ${eval_exit}" >&2
    overall_exit=$eval_exit
  fi

  # Reporters write under the working directory, not the shared /workspace
  # volume. Move rather than copy so the next harness starts from an empty
  # results dir.
  if [ -d /evalbench/results ]; then
    dest="/workspace/results/${TOOLBOX_PREBUILT}"
    mkdir -p "${dest}"
    mv -T /evalbench/results "${dest}/${harness}"
  fi
done

exit $overall_exit
