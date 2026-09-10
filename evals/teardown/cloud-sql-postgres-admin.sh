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

# Deletes the backups the cloud-sql-postgres-admin evalset creates. The toolbox
# has no delete tool, so this cannot be a scenario.
#
# Two entry points: EvalBench's tear_down_script hook, which normally does the
# deleting, and TEARDOWN_SWEEP, an age-bounded pass over whatever a killed eval
# left behind.
#
# Scope is deliberately narrow: one instance, ON_DEMAND only, so the automated
# backups that carry recovery value are never matched.

set -euo pipefail

: "${TEARDOWN_PROJECT:?}" "${TEARDOWN_INSTANCE:?}"

# A flag rather than $1, because EvalBench passes the session directory there.
if [[ -n "${TEARDOWN_SWEEP:-}" ]]; then
  # No marker to bound against, so spare anything recent enough to belong to a
  # build still in flight.
  cutoff=$(date -u -d "-${TEARDOWN_SWEEP_AGE_HOURS:-6} hours" +%Y-%m-%dT%H:%M:%SZ)
  filter="type=ON_DEMAND AND enqueuedTime<${cutoff}"
  echo "sweeping on-demand backups on ${TEARDOWN_INSTANCE} older than ${cutoff}"
else
  marker="/workspace/eval-start-${TOOLBOX_PREBUILT}"
  if [[ ! -f "${marker}" ]]; then
    echo "evals did not run; nothing to tear down"
    exit 0
  fi
  cutoff=$(cat "${marker}")
  filter="type=ON_DEMAND AND enqueuedTime>=${cutoff}"
  echo "deleting on-demand backups on ${TEARDOWN_INSTANCE} since ${cutoff}"
  # EvalBench discards this script's exit code, so the marker file is the only
  # way a leak reaches the build's report-failures step.
  trap '[ $? -eq 0 ] || touch "/workspace/failed-teardown-${TOOLBOX_PREBUILT}"' EXIT
fi

ids=$(gcloud sql backups list \
  --project="${TEARDOWN_PROJECT}" \
  --instance="${TEARDOWN_INSTANCE}" \
  --filter="${filter}" \
  --format='value(id)')

if [[ -z "${ids}" ]]; then
  echo "no backups to delete"
  exit 0
fi

# An unfinished backup cannot be deleted -- the API returns 400
# ERROR_INVALID_BACKUP_RUN_STATUS -- and an agent that starts one without
# waiting leaves exactly that. Observed runtime is about a minute.
delete_backup() {
  local id="$1" status
  for _ in $(seq 30); do
    status=$(gcloud sql backups describe "${id}" \
      --project="${TEARDOWN_PROJECT}" \
      --instance="${TEARDOWN_INSTANCE}" \
      --format='value(status)') || return 1
    case "${status}" in
      ENQUEUED | OVERDUE | RUNNING) sleep 10 ;;
      *) break ;;
    esac
  done

  # Not a failure, just early: the backup is still deletable, so leave it for a
  # later sweep rather than failing a build over it.
  case "${status}" in
    ENQUEUED | OVERDUE | RUNNING)
      echo "backup ${id} still ${status}; leaving it for the sweep"
      return 0
      ;;
  esac

  echo "deleting backup ${id}"
  gcloud sql backups delete "${id}" \
    --project="${TEARDOWN_PROJECT}" \
    --instance="${TEARDOWN_INSTANCE}" \
    --quiet
}

# The work is a function so that one bad id -- a NOT_FOUND from a teardown
# running concurrently, a backup wedged in an undeletable state -- does not take
# set -e and the rest of the list with it. Otherwise a permanently undeletable
# backup would block every id after it on every future sweep.
err=0
for id in ${ids}; do
  delete_backup "${id}" || { echo "could not delete backup ${id}"; err=1; }
done

exit "${err}"
