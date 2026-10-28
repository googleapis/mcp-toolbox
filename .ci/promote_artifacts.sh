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

if [[ ! "${VERSION}" =~ ^v?[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "ERROR: Invalid VERSION '${VERSION}'; expected vX.Y.Z or X.Y.Z" >&2
  exit 1
fi

STAGING_IMAGE_URI="${LOCATION}-docker.pkg.dev/${PROJECT_ID}/${STAGING_REPO}/${PACKAGE_NAME}"

echo "Resolving image digest for ${STAGING_IMAGE_URI}:${VERSION#v}..."
IMAGE_DIGEST=$(gcloud artifacts docker tags list "${STAGING_IMAGE_URI}" \
  --project="${PROJECT_ID}" \
  --filter="tag ~ \"/tags/${VERSION#v}\$\"" \
  --format="value(version)")
IMAGE_DIGEST="${IMAGE_DIGEST##*/}"

if [[ -z "${IMAGE_DIGEST}" ]]; then
  echo "ERROR: Could not resolve image digest for ${STAGING_IMAGE_URI}:${VERSION#v}" >&2
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
    --format="value(name)" | head -n 1)

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

readonly ACCESS_TOKEN=$(gcloud auth print-access-token)

upsert_tag() {
  local tag_id="$1"
  local repo="${2:-${STAGING_REPO}}"
  local digest="${3:-${IMAGE_DIGEST}}"
  local encoded_package="${PACKAGE_NAME//\//%2F}"
  local image_uri="${LOCATION}-docker.pkg.dev/${PROJECT_ID}/${repo}/${PACKAGE_NAME}"
  local tag_parent="projects/${PROJECT_ID}/locations/${LOCATION}/repositories/${repo}/packages/${encoded_package}"
  local target_version_name="${tag_parent}/versions/${digest}"
  local existing_digest

  existing_digest=$(gcloud artifacts docker tags list "${image_uri}" \
    --project="${PROJECT_ID}" \
    --filter="tag ~ \"/tags/${tag_id}\$\"" \
    --format="value(version)" | head -n 1)
  existing_digest="${existing_digest##*/}"

  if [[ "${existing_digest}" == "${digest}" ]]; then
    echo "Tag ${tag_id} on ${image_uri} already points to ${digest}."
    return 0
  fi

  local tag_url method
  if [[ -z "${existing_digest}" ]]; then
    echo "Creating tag ${tag_id} on ${image_uri}@${digest}..."
    tag_url="https://artifactregistry.googleapis.com/v1/${tag_parent}/tags?tagId=${tag_id}"
    method="POST"
  else
    echo "Updating tag ${tag_id} on ${image_uri} from ${existing_digest} to ${digest}..."
    tag_url="https://artifactregistry.googleapis.com/v1/${tag_parent}/tags/${tag_id}?updateMask=version"
    method="PATCH"
  fi

  local resp http_code body
  resp=$(curl -sS --retry 3 -w "\n%{http_code}" -X "${method}" \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    "${tag_url}" \
    -d "{\"name\": \"${tag_parent}/tags/${tag_id}\", \"version\": \"${target_version_name}\"}")
  http_code=$(echo "${resp}" | tail -n 1)
  body=$(echo "${resp}" | sed '$d')
  if [[ "${http_code}" -lt 200 || "${http_code}" -ge 300 ]]; then
    echo "ERROR: Failed to ${method} tag ${tag_id} (HTTP ${http_code}): ${body}" >&2
    exit 1
  fi
}

tag_with_children() {
  local prefix="$1" repo="$2" digest="$3" suffix="$4" require_children="${5:-true}" children child
  local encoded_package="${PACKAGE_NAME//\//%2F}"
  upsert_tag "${prefix}-${suffix}" "${repo}" "${digest}"
  children=$(curl -fsSL --retry 3 \
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    "https://artifactregistry.googleapis.com/v1/projects/${PROJECT_ID}/locations/${LOCATION}/repositories/${repo}/dockerImages/${encoded_package}@${digest}" \
    | jq -r '.imageManifests[]?.digest // empty | select(startswith("sha256:"))') || true
  if [[ -z "${children}" ]]; then
    if [[ "${require_children}" == "true" ]]; then
      echo "ERROR: No child manifests found for ${repo}/${PACKAGE_NAME}@${digest}" >&2
      exit 1
    fi
    echo "WARNING: No child manifests found for ${repo}/${PACKAGE_NAME}@${digest}; skipping child tags." >&2
    return 0
  fi
  for child in ${children}; do
    local child_sha="${child#sha256:}"
    upsert_tag "${prefix}-${child_sha:0:12}" "${repo}" "${child}"
  done
}

PROD_IMAGE_URI="${LOCATION}-docker.pkg.dev/${PROJECT_ID}/${PROD_REPO}/${PACKAGE_NAME}"
EXISTING_PROD_DIGEST=$(gcloud artifacts docker tags list "${PROD_IMAGE_URI}" \
  --project="${PROJECT_ID}" \
  --filter="tag ~ \"/tags/${VERSION#v}\$\"" \
  --format="value(version)")
EXISTING_PROD_DIGEST="${EXISTING_PROD_DIGEST##*/}"

if [[ -n "${EXISTING_PROD_DIGEST}" && "${EXISTING_PROD_DIGEST}" != "${IMAGE_DIGEST}" ]]; then
  echo "ERROR: Refusing to overwrite existing prod tag for ${VERSION} (existing digest ${EXISTING_PROD_DIGEST} != staged digest ${IMAGE_DIGEST})" >&2
  exit 1
fi

EXISTING_LATEST_DIGEST=""
if [[ "${PUSH_LATEST:-false}" == "true" ]]; then
  EXISTING_LATEST_DIGEST=$(gcloud artifacts docker tags list "${PROD_IMAGE_URI}" \
    --project="${PROJECT_ID}" \
    --filter="tag ~ \"/tags/latest\$\"" \
    --format="value(version)" | head -n 1)
  EXISTING_LATEST_DIGEST="${EXISTING_LATEST_DIGEST##*/}"

  echo "Tagging ${STAGING_IMAGE_URI}@${IMAGE_DIGEST} as latest..."
  upsert_tag "latest"
fi

if [[ "${EXISTING_PROD_DIGEST}" == "${IMAGE_DIGEST}" ]]; then
  echo "Image ${PACKAGE_NAME}@${IMAGE_DIGEST} already promoted to ${PROD_REPO}; skipping promoteArtifact call."
else
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
    -H "Authorization: Bearer ${ACCESS_TOKEN}" \
    -H "Content-Type: application/json" \
    "${PROMOTE_URL}" \
    -d "${PAYLOAD}")

  echo "Promote API response: ${RESP}"
  if [[ "$(echo "${RESP}" | jq -r '.error // empty')" != "" ]]; then
    echo "ERROR: Promote API request failed: ${RESP}" >&2
    exit 1
  fi

  OP_NAME=$(echo "${RESP}" | jq -r '.name // empty')
  if [[ -z "${OP_NAME}" ]]; then
    echo "ERROR: Promote API response did not contain an operation name: ${RESP}" >&2
    exit 1
  fi

  echo "Waiting for promotion operation ${OP_NAME} to complete..."
  OP_START=$SECONDS
  OP_COMPLETED=false
  while (( SECONDS - OP_START < 300 )); do
    OP_STATUS=$(curl -fsSL --retry 3 \
      -H "Authorization: Bearer ${ACCESS_TOKEN}" \
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

echo "Verifying prod tag ${PROD_REPO}/${PACKAGE_NAME}:${VERSION#v} -> ${IMAGE_DIGEST}..."
PROD_DIGEST=$(curl -fsSL --retry 3 \
  -H "Authorization: Bearer ${ACCESS_TOKEN}" \
  "https://artifactregistry.googleapis.com/v1/projects/${PROJECT_ID}/locations/${LOCATION}/repositories/${PROD_REPO}/packages/${PACKAGE_NAME}/tags/${VERSION#v}" \
  | jq -r '.version // empty' | awk -F'/' '{print $NF}')
if [[ "${PROD_DIGEST}" != "${IMAGE_DIGEST}" ]]; then
  echo "ERROR: prod tag ${VERSION#v} resolves to '${PROD_DIGEST}', expected ${IMAGE_DIGEST}" >&2
  exit 1
fi

echo "Tagging ${PROD_IMAGE_URI}@${IMAGE_DIGEST} (and platform manifests) as public-image..."
tag_with_children "public-image" "${PROD_REPO}" "${IMAGE_DIGEST}" "${VERSION#v}" "true"

if [[ "${PUSH_LATEST:-false}" == "true" ]]; then
  upsert_tag "latest" "${PROD_REPO}" "${IMAGE_DIGEST}"
  if [[ -n "${EXISTING_LATEST_DIGEST}" && "${EXISTING_LATEST_DIGEST}" != "${IMAGE_DIGEST}" ]]; then
    echo "Marking previous latest image (${EXISTING_LATEST_DIGEST}) as deprecated..."
    SHORT_LATEST="${EXISTING_LATEST_DIGEST#sha256:}"
    tag_with_children "deprecated-public-image" "${PROD_REPO}" "${EXISTING_LATEST_DIGEST}" "${SHORT_LATEST:0:12}" "false"
  fi
fi
