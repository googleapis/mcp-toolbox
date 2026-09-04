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

# Drops the dataset evals/setup/bigquery.sh created. Nothing else can drop it:
# the source runs read-only, so no scenario is able to clean up after itself.
#
# Two entry points: EvalBench's tear_down_script hook, which normally does the
# deleting, and TEARDOWN_SWEEP, an age-bounded pass over the datasets a killed
# build never reached teardown for.
#
# not_found_ok because seeding is allowed to fail: EvalBench ignores the setup
# script's exit code and runs teardown in a finally regardless.

set -euo pipefail

# This script's own exit code is discarded too, so the marker is the only way a
# leaked dataset reaches report-failures. Ahead of the guards below, which would
# otherwise fail silently. Skipped on the sweep path, which runs outside a build.
if [[ -z "${TEARDOWN_SWEEP:-}" ]]; then
  trap '[ $? -eq 0 ] || touch "/workspace/failed-teardown-${TOOLBOX_PREBUILT}"' EXIT
fi

: "${BIGQUERY_PROJECT:?}"

# A flag rather than $1, because EvalBench passes the session directory there.
if [[ -n "${TEARDOWN_SWEEP:-}" ]]; then
  # No EVAL_RUN_ID to name a dataset with, so spare anything recent enough to
  # belong to a build still in flight. Builds time out at 2h.
  export SWEEP_AGE_HOURS="${TEARDOWN_SWEEP_AGE_HOURS:-6}"
  echo "sweeping toolbox_evals_* in ${BIGQUERY_PROJECT} older than ${SWEEP_AGE_HOURS}h"
else
  : "${EVAL_RUN_ID:?}"
  export BQ_DATASET="toolbox_evals_${EVAL_RUN_ID}"
  echo "deleting ${BIGQUERY_PROJECT}.${BQ_DATASET}"
fi

uv run --no-sync python - <<'PY'
import datetime
import os

from google.cloud import bigquery

project = os.environ["BIGQUERY_PROJECT"]
client = bigquery.Client(project=project)

dataset = os.environ.get("BQ_DATASET")
if dataset:
    targets = [f"{project}.{dataset}"]
else:
    cutoff = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(
        hours=float(os.environ["SWEEP_AGE_HOURS"])
    )
    # created is only on the full resource, not the list entry, so this costs a
    # get per candidate.
    targets = [
        item.reference
        for item in client.list_datasets(project)
        if item.dataset_id.startswith("toolbox_evals_")
        and client.get_dataset(item.reference).created < cutoff
    ]
    print(f"{len(targets)} dataset(s) to delete")

for target in targets:
    print(f"deleting {target}")
    client.delete_dataset(target, delete_contents=True, not_found_ok=True)
PY
