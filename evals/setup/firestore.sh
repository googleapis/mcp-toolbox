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

set -euo pipefail

: "${FIRESTORE_PROJECT:?}"
FIRESTORE_DATABASE="${FIRESTORE_DATABASE:-"(default)"}"
COLLECTION="eval_customers"

echo "Seeding test documents in Firestore project ${FIRESTORE_PROJECT}, database ${FIRESTORE_DATABASE}..."

TOKEN=$(gcloud auth print-access-token)
BASE_URL="https://firestore.googleapis.com/v1/projects/${FIRESTORE_PROJECT}/databases/${FIRESTORE_DATABASE}/documents/${COLLECTION}"

seed_doc() {
  local doc_id="$1"
  local json_payload="$2"

  echo "Creating document ${COLLECTION}/${doc_id}..."
  curl -s -f -X PATCH \
    -H "Authorization: Bearer ${TOKEN}" \
    -H "Content-Type: application/json" \
    -d "${json_payload}" \
    "${BASE_URL}/${doc_id}" > /dev/null
}

# Used for get_documents and query_collection
seed_doc "customer_alpha" '{
  "fields": {
    "name": {"stringValue": "Stark Industries"},
    "status": {"stringValue": "active"},
    "tier": {"stringValue": "gold"}
  }
}'

# Used for query_collection
seed_doc "customer_beta" '{
  "fields": {
    "name": {"stringValue": "Oscorp"},
    "status": {"stringValue": "inactive"},
    "tier": {"stringValue": "silver"}
  }
}'

# Used for delete_documents
seed_doc "temp_eval_doc" '{
  "fields": {
    "name": {"stringValue": "Temporary Doc"},
    "status": {"stringValue": "transient"}
  }
}'

echo "Firestore eval state successfully seeded."