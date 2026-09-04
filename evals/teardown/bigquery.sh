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

# Drops the dataset evals/setup/bigquery.sh created. Nothing else can: the
# source runs read-only, so no scenario is able to clean up after itself.
#
# not_found_ok covers both the skipped-run case and the second harness in a
# multi-harness run, whose setup recreates what the first one's teardown removed.

set -euo pipefail

: "${BIGQUERY_PROJECT:?}" "${EVAL_RUN_ID:?}"

export BQ_DATASET="toolbox_evals_${EVAL_RUN_ID}"

# EvalBench discards this script's exit code, so the marker file is the only way
# a leaked dataset reaches the build's report-failures step.
trap '[ $? -eq 0 ] || touch "/workspace/failed-teardown-${TOOLBOX_PREBUILT}"' EXIT

echo "deleting ${BIGQUERY_PROJECT}.${BQ_DATASET}"

uv run --no-sync python - <<'PY'
import os

from google.cloud import bigquery

project = os.environ["BIGQUERY_PROJECT"]

bigquery.Client(project=project).delete_dataset(
    f"{project}.{os.environ['BQ_DATASET']}",
    delete_contents=True,
    not_found_ok=True,
)
PY
