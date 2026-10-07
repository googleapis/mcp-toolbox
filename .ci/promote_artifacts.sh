#!/usr/bin/env bash
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
#
# Polls for BCID attestations on the staged container image and promotes the
# verified artifact from the staging Artifact Registry repository to the
# production repository via the Artifact Registry Promote API.
#
# Usage:
#   bash .ci/promote_artifacts.sh [PROJECT_ID] [LOCATION] [STAGING_REPO] [PROD_REPO] [PACKAGE_NAME] [VERSION]

set -eo pipefail

readonly PROJECT_ID="${1:-${PROJECT_ID:?PROJECT_ID must be set}}"
readonly LOCATION="${2:-${LOCATION:?LOCATION must be set}}"
readonly STAGING_REPO="${3:-${STAGING_REPO:?STAGING_REPO must be set}}"
readonly PROD_REPO="${4:-${PROD_REPO:?PROD_REPO must be set}}"
readonly PACKAGE_NAME="${5:-${PACKAGE_NAME:?PACKAGE_NAME must be set}}"
readonly VERSION="${6:-${VERSION:-$(cat ./cmd/version.txt)}}"

STAGING_IMAGE_URI="${LOCATION}-docker.pkg.dev/${PROJECT_ID}/${STAGING_REPO}/${PACKAGE_NAME}"

echo "Resolving image digest for ${STAGING_IMAGE_URI}:${VERSION}..."
IMAGE_DIGEST=$(gcloud artifacts docker tags list "${STAGING_IMAGE_URI}" \
  --project="${PROJECT_ID}" \
  --filter="tag ~ /tags/(v?${VERSION#v})$" \
  --format="value(version)" 2>/dev/null | head -n 1 || true)
IMAGE_DIGEST="${IMAGE_DIGEST##*/}"

if [[ -z "${IMAGE_DIGEST}" ]]; then
  for CANDIDATE_TAG in "v${VERSION#v}" "${VERSION#v}"; do
    TAG_RESP=$(curl -fsSL \
      -H "Authorization: Bearer $(gcloud auth print-access-token)" \
      "https://artifactregistry.googleapis.com/v1/projects/${PROJECT_ID}/locations/${LOCATION}/repositories/${STAGING_REPO}/packages/${PACKAGE_NAME}/tags/${CANDIDATE_TAG}" 2>/dev/null || true)
    IMAGE_DIGEST=$(echo "${TAG_RESP}" | jq -r '.version // empty' | awk -F'/' '{print $NF}')
    [[ -n "${IMAGE_DIGEST}" ]] && break
  done
fi

if [[ -z "${IMAGE_DIGEST}" ]]; then
  echo "ERROR: Could not resolve image digest for ${STAGING_IMAGE_URI}:${VERSION}" >&2
  exit 1
fi

echo "Resolved staging image digest: ${IMAGE_DIGEST}"

echo "Polling for BCID attestations on ${STAGING_IMAGE_URI}@${IMAGE_DIGEST}..."
POLL_START=$SECONDS
POLL_TIMEOUT="${POLL_TIMEOUT:-600}"
ATTESTATION_FOUND=false
TARGET_VERSION="projects/${PROJECT_ID}/locations/${LOCATION}/repositories/${STAGING_REPO}/packages/${PACKAGE_NAME}/versions/${IMAGE_DIGEST}"

while (( SECONDS - POLL_START < POLL_TIMEOUT )); do
  OCCURRENCES=$(gcloud artifacts attachments list \
    --project="${PROJECT_ID}" \
    --location="${LOCATION}" \
    --repository="${STAGING_REPO}" \
    --filter="target=\"${TARGET_VERSION}\" AND type=\"application/vnd.in-toto.verification_summary+dsse\"" \
    --format="value(name)" 2>/dev/null | head -n 1 || true)

  if [[ -z "${OCCURRENCES}" ]]; then
    OCCURRENCES=$(curl -fsSL -G \
      -H "Authorization: Bearer $(gcloud auth print-access-token)" \
      --data-urlencode "filter=target=\"${TARGET_VERSION}\" AND type=\"application/vnd.in-toto.verification_summary+dsse\"" \
      "https://artifactregistry.googleapis.com/v1/projects/${PROJECT_ID}/locations/${LOCATION}/repositories/${STAGING_REPO}/attachments" 2>/dev/null | jq -r '.attachments[0].name // empty' || true)
  fi

  if [[ -z "${OCCURRENCES}" ]]; then
    OCCURRENCES=$(curl -fsSL -G \
      -H "Authorization: Bearer $(gcloud auth print-access-token)" \
      --data-urlencode "filter=kind=\"ATTESTATION\" AND resourceUrl=\"https://${STAGING_IMAGE_URI}@${IMAGE_DIGEST}\"" \
      "https://containeranalysis.googleapis.com/v1/projects/${PROJECT_ID}/occurrences" 2>/dev/null | jq -r '.occurrences[0].name // empty' || true)
  fi

  if [[ -n "${OCCURRENCES}" ]]; then
    echo "Found BCID attestation for ${STAGING_IMAGE_URI}@${IMAGE_DIGEST}: ${OCCURRENCES}"
    ATTESTATION_FOUND=true
    break
  fi

  echo "Waiting 15s for BCID attestation generation..."
  sleep 15
done

if [[ "${ATTESTATION_FOUND}" != "true" ]]; then
  echo "ERROR: Timed out waiting for BCID attestations on ${STAGING_IMAGE_URI}@${IMAGE_DIGEST}" >&2
  exit 1
fi

echo "Promoting ${PACKAGE_NAME}@${IMAGE_DIGEST} from ${STAGING_REPO} to ${PROD_REPO}..."
PROMOTE_URL="https://artifactregistry.googleapis.com/v1/projects/${PROJECT_ID}/locations/${LOCATION}/repositories/${PROD_REPO}:promoteArtifact"
PAYLOAD=$(cat <<EOF
{
  "source_repository": "projects/${PROJECT_ID}/locations/${LOCATION}/repositories/${STAGING_REPO}",
  "source_version": "projects/${PROJECT_ID}/locations/${LOCATION}/repositories/${STAGING_REPO}/packages/${PACKAGE_NAME}/versions/${IMAGE_DIGEST}",
  "attachment_behavior": "PUBLIC_BCID_VSA_ONLY",
  "include_all_tags": true,
  "overwrite_tags": true
}
EOF
)

RESP=$(curl -sSL -X POST \
  -H "Authorization: Bearer $(gcloud auth print-access-token)" \
  -H "Content-Type: application/json" \
  "${PROMOTE_URL}" \
  -d "${PAYLOAD}")

echo "Promote API response: ${RESP}"
if [[ "$(echo "${RESP}" | jq -r '.error // empty')" != "" ]]; then
  echo "ERROR: Promote API request failed: ${RESP}" >&2
  exit 1
fi

OP_NAME=$(echo "${RESP}" | jq -r '.name // empty')
if [[ -n "${OP_NAME}" ]]; then
  echo "Waiting for promotion operation ${OP_NAME} to complete..."
  OP_START=$SECONDS
  OP_COMPLETED=false
  while (( SECONDS - OP_START < 300 )); do
    OP_STATUS=$(curl -fsSL \
      -H "Authorization: Bearer $(gcloud auth print-access-token)" \
      -H "Content-Type: application/json" \
      "https://artifactregistry.googleapis.com/v1/${OP_NAME}")

    if [[ "$(echo "${OP_STATUS}" | jq -r '.done // false')" == "true" ]]; then
      if [[ "$(echo "${OP_STATUS}" | jq -r '.error // empty')" != "" ]]; then
        echo "ERROR: Artifact promotion failed: ${OP_STATUS}" >&2
        exit 1
      fi
      echo "Artifact promotion completed successfully."
      OP_COMPLETED=true
      break
    fi

    echo "Promotion in progress, waiting 5s..."
    sleep 5
  done

  if [[ "${OP_COMPLETED}" != "true" ]]; then
    echo "ERROR: Timed out waiting for promotion operation ${OP_NAME}" >&2
    exit 1
  fi
fi
