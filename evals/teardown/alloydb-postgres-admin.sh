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

# Deletes the users, instances, and clusters the alloydb-postgres-admin evalset creates.
#
# Two entry points: EvalBench's tear_down_script hook, which normally does the
# deleting, and TEARDOWN_SWEEP, an age-bounded pass over whatever a killed eval
# left behind.

set -euo pipefail

: "${TEARDOWN_PROJECT:?}" "${TEARDOWN_REGION:?}" "${TEARDOWN_CLUSTER:?}" "${TEARDOWN_USER:?}" "${TEARDOWN_INSTANCE:?}"

if [[ -n "${TEARDOWN_SWEEP:-}" ]]; then
  echo "sweeping eval resources in project ${TEARDOWN_PROJECT}, region ${TEARDOWN_REGION}, cluster ${TEARDOWN_CLUSTER}, user ${TEARDOWN_USER}"
else
  marker="/workspace/eval-start-${TOOLBOX_PREBUILT}"
  if [[ ! -f "${marker}" ]]; then
    echo "evals did not run; nothing to tear down"
    exit 0
  fi
  echo "tearing down eval resources created since $(cat "${marker}")"
  # EvalBench discards this script's exit code, so the marker file is the only
  # way a leak reaches the build's report-failures step.
  trap '[ $? -eq 0 ] || touch "/workspace/failed-teardown-${TOOLBOX_PREBUILT}"' EXIT
fi

err=0

# 1. Delete test user
delete_user() {
  local user="$1"
  if gcloud alloydb users describe "${user}" \
    --project="${TEARDOWN_PROJECT}" \
    --region="${TEARDOWN_REGION}" \
    --cluster="${TEARDOWN_CLUSTER}" > /dev/null 2>&1; then
    echo "deleting test user ${user}"
    gcloud alloydb users delete "${user}" \
      --project="${TEARDOWN_PROJECT}" \
      --region="${TEARDOWN_REGION}" \
      --cluster="${TEARDOWN_CLUSTER}" \
      --quiet || return 1
  fi
}

# 2. Delete test instance (wait if still creating)
delete_instance() {
  local instance="$1" status
  if ! gcloud alloydb instances describe "${instance}" \
    --project="${TEARDOWN_PROJECT}" \
    --region="${TEARDOWN_REGION}" \
    --cluster="${TEARDOWN_CLUSTER}" > /dev/null 2>&1; then
    return 0
  fi

  for _ in $(seq 30); do
    status=$(gcloud alloydb instances describe "${instance}" \
      --project="${TEARDOWN_PROJECT}" \
      --region="${TEARDOWN_REGION}" \
      --cluster="${TEARDOWN_CLUSTER}" \
      --format='value(state)') || return 1
    case "${status}" in
      CREATING) sleep 10 ;;
      *) break ;;
    esac
  done

  case "${status}" in
    CREATING)
      echo "instance ${instance} still ${status}; leaving it for sweep"
      return 0
      ;;
  esac

  echo "deleting test instance ${instance}"
  gcloud alloydb instances delete "${instance}" \
    --project="${TEARDOWN_PROJECT}" \
    --region="${TEARDOWN_REGION}" \
    --cluster="${TEARDOWN_CLUSTER}" \
    --quiet
}

# 3. Delete test cluster (wait if still creating)
delete_cluster() {
  local cluster="$1" status
  if ! gcloud alloydb clusters describe "${cluster}" \
    --project="${TEARDOWN_PROJECT}" \
    --region="${TEARDOWN_REGION}" > /dev/null 2>&1; then
    return 0
  fi

  for _ in $(seq 30); do
    status=$(gcloud alloydb clusters describe "${cluster}" \
      --project="${TEARDOWN_PROJECT}" \
      --region="${TEARDOWN_REGION}" \
      --format='value(state)') || return 1
    case "${status}" in
      CREATING) sleep 10 ;;
      *) break ;;
    esac
  done

  case "${status}" in
    CREATING)
      echo "cluster ${cluster} still ${status}; leaving it for sweep"
      return 0
      ;;
  esac

  echo "deleting test cluster ${cluster}"
  gcloud alloydb clusters delete "${cluster}" \
    --project="${TEARDOWN_PROJECT}" \
    --region="${TEARDOWN_REGION}" \
    --force \
    --quiet
}

delete_user "${TEARDOWN_USER}" || { echo "could not delete test user ${TEARDOWN_USER}"; err=1; }
delete_instance "${TEARDOWN_INSTANCE}" || { echo "could not delete test instance ${TEARDOWN_INSTANCE}"; err=1; }
delete_cluster "${TEARDOWN_CLUSTER}" || { echo "could not delete test cluster ${TEARDOWN_CLUSTER}"; err=1; }

exit "${err}"
