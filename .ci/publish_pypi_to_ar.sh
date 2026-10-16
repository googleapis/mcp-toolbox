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
# Builds one toolbox-server wheel per supported platform and uploads them all
# to the OSS Exit Gate internal Artifact Registry. Exit Gate ships them
# externally to pypi.org once trigger_exit_gate.sh uploads the manifest.
#
# Required CWD: repo root.
#
# Binaries (toolbox.<os>.<arch>) must be pre-staged in the workspace from GCS by
# the earlier fetch-binaries step (in both promote.cloudbuild.yaml and
# pypi_retry.cloudbuild.yaml).
#
# AR doesn't support twine's `--skip-existing` flag. If this version already
# has wheels in AR (e.g., from a prior run where Exit Gate failed to externally
# publish and left the AR contents in place), twine will error. Manually clean
# the AR version before re-running:
#
#   gcloud artifacts versions delete <VERSION> \
#     --package=toolbox-server \
#     --repository=mcp-toolbox--pypi \
#     --location=us \
#     --project=oss-exit-gate-prod

set -eo pipefail

# OSS Exit Gate constants — owned by Exit Gate, not build configuration.
readonly AR_URL="https://us-python.pkg.dev/oss-exit-gate-prod/mcp-toolbox--pypi/"
readonly PYPI_DIR="pypi"
readonly BIN_DIR="${PYPI_DIR}/src/toolbox_server/bin"
readonly DIST_DIR="${PYPI_DIR}/dist"

# Install build + upload tooling. keyrings.google-artifactregistry-auth is the
# keyring backend that twine uses to obtain a short-lived AR token from ADC.
pip install --quiet --upgrade build twine keyring keyrings.google-artifactregistry-auth

# Start each build run with empty dist/ to make the final upload deterministic.
rm -rf "${DIST_DIR}"

# Builds one wheel.
#
# Args:
#   src:  go build output staged in the workspace (e.g., toolbox.linux.amd64)
#   plat: PEP 425 platform tag for the wheel (e.g., manylinux2014_x86_64)
#   dest: binary name inside the wheel's bin/ (toolbox or toolbox.exe)
build_wheel() {
  local src="$1" plat="$2" dest="$3"

  if [[ ! -f "${src}" ]]; then
    echo "ERROR: ${src} not found in workspace. Earlier build step must run first." >&2
    exit 1
  fi

  # Replace bin/ so a previous iteration's binary doesn't leak into this wheel.
  rm -rf "${BIN_DIR}"
  mkdir -p "${BIN_DIR}"
  cp "${src}" "${BIN_DIR}/${dest}"
  chmod +x "${BIN_DIR}/${dest}"

  # Also wipe setuptools' build/ staging tree. It caches the previously-copied
  # binary at build/lib/toolbox_server/bin/, which would otherwise be packaged
  # into this iteration's wheel alongside the new binary (e.g., the Windows
  # wheel would end up shipping both toolbox AND toolbox.exe).
  rm -rf "${PYPI_DIR}/build"

  (cd "${PYPI_DIR}" && TOOLBOX_PLATFORM="${plat}" python -m build --wheel)
}

build_wheel "toolbox.linux.amd64"   "manylinux2014_x86_64" "toolbox"
build_wheel "toolbox.darwin.arm64"  "macosx_11_0_arm64"    "toolbox"
build_wheel "toolbox.darwin.amd64"  "macosx_10_14_x86_64"  "toolbox"
build_wheel "toolbox.windows.amd64" "win_amd64"            "toolbox.exe"
build_wheel "toolbox.windows.arm64" "win_arm64"            "toolbox.exe"

python - "${AR_URL}" "${DIST_DIR}" <<'PY'
import glob
import os
import subprocess
import sys
import keyring
import requests

ar_url, dist_dir = sys.argv[1], sys.argv[2]
simple_url = ar_url.rstrip("/") + "/simple/toolbox-server/"
cred = keyring.get_credential(ar_url, None)
auth = (cred.username, cred.password) if cred else None
resp = requests.get(simple_url, auth=auth, timeout=30)
existing = resp.text if resp.status_code == 200 else ""

to_upload = []
for whl in sorted(glob.glob(os.path.join(dist_dir, "*.whl"))):
    fname = os.path.basename(whl)
    if fname in existing:
        print(f"Skipping {fname}: already in AR")
    else:
        to_upload.append(whl)

if to_upload:
    subprocess.run(
        ["twine", "upload", "--repository-url", ar_url, *to_upload],
        check=True,
    )
PY
