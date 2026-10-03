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
# Binaries (toolbox.<os>.<arch>) must be pre-staged in the workspace by an
# earlier Cloud Build step. The retry pipeline downloads them from GCS first.
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
pip install --index-url https://us-central1-python.pkg.dev/${PROJECT_ID}/pypi-remote/simple/ --quiet build==1.6.1 twine==7.0.0 keyring==25.7.0 keyrings.google-artifactregistry-auth==1.1.2

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

twine upload \
  --repository-url "${AR_URL}" \
  "${DIST_DIR}"/*.whl
