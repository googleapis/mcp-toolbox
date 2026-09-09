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

# EvalBench discards this script's exit code, so the marker is the only way a
# leaked dataset reaches report-failures. Ahead of the guards below so they
# cannot fail silently; skipped on the sweep path, which runs outside a build.
if [[ -z "${TEARDOWN_SWEEP:-}" ]]; then
  trap '[ $? -eq 0 ] || touch "/workspace/failed-teardown-${TOOLBOX_PREBUILT}"' EXIT
fi

: "${BIGQUERY_PROJECT:?}"

# A flag rather than $1, because EvalBench passes the session directory there.
if [[ -n "${TEARDOWN_SWEEP:-}" ]]; then
  # No EVAL_BQ_DATASET to name a dataset with, so spare anything recent enough
  # to belong to a build still in flight. Builds time out at 2h.
  export SWEEP_AGE_HOURS="${TEARDOWN_SWEEP_AGE_HOURS:-6}"
  echo "sweeping toolbox_evals_* in ${BIGQUERY_PROJECT} older than ${SWEEP_AGE_HOURS}h"
else
  : "${EVAL_BQ_DATASET:?}"
fi

uv run --no-sync python - <<'PY'
import datetime
import os
import sys

from google.cloud import bigquery

project = os.environ["BIGQUERY_PROJECT"]
client = bigquery.Client(project=project)

# Same condition the shell branched on, so the two cannot disagree.
if os.environ.get("TEARDOWN_SWEEP"):
    cutoff = datetime.datetime.now(datetime.timezone.utc) - datetime.timedelta(
        hours=float(os.environ["SWEEP_AGE_HOURS"])
    )

    # created is not on the list entry, so this costs a get per candidate. A
    # failed get just drops the candidate: it was never confirmed a target, and
    # the likely cause is another teardown deleting it first.
    def is_stale(reference):
        try:
            return client.get_dataset(reference).created < cutoff
        except Exception as exc:
            print(f"skipping {reference}: {exc}")
            return False

    targets = [
        item.reference
        for item in client.list_datasets(project)
        if item.dataset_id.startswith("toolbox_evals_") and is_stale(item.reference)
    ]
    print(f"{len(targets)} dataset(s) to delete")
else:
    targets = [f"{project}.{os.environ['EVAL_BQ_DATASET']}"]

# One undeletable dataset must not take the rest with it: the sweep rebuilds this
# list in the same order every run, so it would block everything behind it.
err = 0
for target in targets:
    print(f"deleting {target}")
    try:
        client.delete_dataset(target, delete_contents=True, not_found_ok=True)
    except Exception as exc:
        print(f"could not delete {target}: {exc}")
        err = 1

sys.exit(err)
PY
