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

# Deletes documents in eval_customers created by the firestore evalset.
#
# Two entry points:
# 1. EvalBench's tear_down_script hook via evals/teardown/dispatch.sh
# 2. TEARDOWN_SWEEP: age-bounded garbage collection of leaked eval data.

set -euo pipefail

# Fall back to FIRESTORE_PROJECT or PROJECT_ID if TEARDOWN_PROJECT is unset
TEARDOWN_PROJECT="${TEARDOWN_PROJECT:-${FIRESTORE_PROJECT:-${PROJECT_ID:-}}}"
: "${TEARDOWN_PROJECT:?Neither TEARDOWN_PROJECT, FIRESTORE_PROJECT, nor PROJECT_ID is set}"

TEARDOWN_DATABASE="${TEARDOWN_DATABASE:-${FIRESTORE_DATABASE:-"(default)"}}"
COLLECTION="eval_customers"

TOKEN=$(gcloud auth print-access-token)
BASE_URL="https://firestore.googleapis.com/v1/projects/${TEARDOWN_PROJECT}/databases/${TEARDOWN_DATABASE}/documents/${COLLECTION}"

# A flag rather than $1, because EvalBench passes the session directory there.
if [[ -n "${TEARDOWN_SWEEP:-}" ]]; then
  cutoff=$(date -u -d "-${TEARDOWN_SWEEP_AGE_HOURS:-6} hours" +%Y-%m-%dT%H:%M:%SZ)
  echo "sweeping documents in ${COLLECTION} older than ${cutoff}"
else
  marker="/workspace/eval-start-${TOOLBOX_PREBUILT}"
  if [[ ! -f "${marker}" ]]; then
    echo "evals did not run; nothing to tear down"
    exit 0
  fi
  cutoff=$(cat "${marker}")
  echo "deleting documents in ${COLLECTION} created since ${cutoff}"
  # EvalBench discards this script's exit code, so the marker file is the only
  # way a leak reaches the build's report-failures step.
  trap '[ $? -eq 0 ] || touch "/workspace/failed-teardown-${TOOLBOX_PREBUILT}"' EXIT
fi

# Fetch documents in the collection and filter by createTime against cutoff
docs=$(curl -s -H "Authorization: Bearer ${TOKEN}" "${BASE_URL}?pageSize=300" | python3 -c '
import sys, json

try:
    data = json.load(sys.stdin)
    cutoff = sys.argv[1]
    is_sweep = sys.argv[2] == "1"

    for doc in data.get("documents", []):
        create_time = doc.get("createTime", "")
        name = doc.get("name", "")
        if is_sweep and create_time < cutoff:
            print(name)
        elif not is_sweep and create_time >= cutoff:
            print(name)
except Exception:
    pass
' "${cutoff}" "$([[ -n "${TEARDOWN_SWEEP:-}" ]] && echo 1 || echo 0)")

if [[ -z "${docs}" ]]; then
  echo "no documents to delete"
  exit 0
fi

# Delete matching documents individually
err=0
for doc_path in ${docs}; do
  echo "deleting document ${doc_path}"
  curl -s -f -X DELETE \
    -H "Authorization: Bearer ${TOKEN}" \
    "https://firestore.googleapis.com/v1/${doc_path}" > /dev/null || {
      echo "could not delete document ${doc_path}"
      err=1
    }
done

echo "Teardown completed successfully."
exit "${err}"