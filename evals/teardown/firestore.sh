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

# Deletes documents in the eval_customers collection created during the firestore evalset.
#
# Two entry points: EvalBench's tear_down_script hook (via evals/teardown/dispatch.sh),
# and TEARDOWN_SWEEP, an age-bounded pass over whatever a killed eval left behind.

set -euo pipefail

: "${TEARDOWN_PROJECT:?}"
TEARDOWN_DATABASE="${TEARDOWN_DATABASE:-"(default)"}"
COLLECTION="eval_customers"

# When not running a sweep, ensure evals actually ran before attempting deletion.
if [[ -z "${TEARDOWN_SWEEP:-}" ]]; then
  marker="/workspace/eval-start-${TOOLBOX_PREBUILT}"
  if [[ ! -f "${marker}" ]]; then
    echo "evals did not run; nothing to tear down"
    exit 0
  fi
  # EvalBench discards this script's exit code, so the marker file is the only
  # way a leak reaches the build's report-failures step.
  trap '[ $? -eq 0 ] || touch "/workspace/failed-teardown-${TOOLBOX_PREBUILT}"' EXIT
fi

echo "Deleting eval documents in collection '${COLLECTION}' on ${TEARDOWN_PROJECT}/${TEARDOWN_DATABASE}..."

gcloud firestore bulk-delete \
  --project="${TEARDOWN_PROJECT}" \
  --database="${TEARDOWN_DATABASE}" \
  --collection-ids="${COLLECTION}" \
  --quiet

echo "Teardown completed successfully."