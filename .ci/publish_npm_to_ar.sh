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
# Publishes the six @toolbox-sdk npm packages (5 platform-specific + 1 wrapper)
# to the OSS Exit Gate internal Artifact Registry. The Exit Gate then ships
# them externally to npmjs.org once trigger_exit_gate.sh uploads the manifest.
#
# Required CWD: repo root (so npm/server-*/ paths resolve).
#
# Binaries (toolbox.<os>.<arch>) are picked up from the workspace if present
# (versioned-release pipeline) and otherwise downloaded by each package's own
# prepack script from GCS by version (retry pipeline).
#
# Each publish is gated by an "already in AR?" check so the script is safe to
# re-run after a partial failure.

set -eo pipefail

# OSS Exit Gate constants — owned by Exit Gate, not build configuration.
readonly AR_REGISTRY="https://us-npm.pkg.dev/oss-exit-gate-prod/mcp-toolbox--npm/"
readonly AR_HOST="us-npm.pkg.dev"

# Write to $HOME so npm finds it as user config from inside any package dir.
# A .npmrc at /workspace would be ignored — npm's per-project .npmrc must sit
# next to package.json, and each platform package has its own package.json.
cat >> "$HOME/.npmrc" <<EOF
@toolbox-sdk:registry=${AR_REGISTRY}
//${AR_HOST}/:always-auth=true
EOF

# Run from $HOME so google-artifactregistry-auth finds the .npmrc above and
# writes the short-lived AR auth token into it.
(cd "$HOME" && npx --yes google-artifactregistry-auth)

# Publishes the npm package at npm/${pkg} to the Exit Gate AR, unless that
# exact version is already there (idempotency check makes retries safe).
# --registry is passed explicitly so we don't depend on scope-config
# resolution at publish time.
publish_pkg() {
  local pkg="$1"
  local npm_name="@toolbox-sdk/${pkg}"
  local version
  version=$(cd "npm/${pkg}" && node -p "require('./package.json').version")

  if npm view "${npm_name}@${version}" version --registry "${AR_REGISTRY}" 2>/dev/null | grep -q .; then
    echo "Skipping ${npm_name}@${version}: already in AR"
    return
  fi

  (cd "npm/${pkg}" && npm publish --registry "${AR_REGISTRY}")
}

# Stages a Cloud-Build-produced binary into a platform npm package and publishes
# it. Copying the binary into bin/ short-circuits the package's prepack
# download script (it skips when the binary already exists). When the binary
# isn't in the workspace (retry pipeline), prepack downloads it from GCS.
#
# Args:
#   pkg:  npm folder under npm/ (e.g., server-linux-x64)
#   src:  go build output in the workspace (e.g., toolbox.linux.amd64)
#   dest: binary name inside the package's bin/ (toolbox or toolbox.exe)
publish_platform() {
  local pkg="$1" src="$2" dest="$3"
  if [[ -f "${src}" ]]; then
    mkdir -p "npm/${pkg}/bin"
    cp "${src}" "npm/${pkg}/bin/${dest}"
    chmod +x "npm/${pkg}/bin/${dest}"
  fi
  publish_pkg "${pkg}"
}

publish_platform "server-linux-x64"    "toolbox.linux.amd64"   "toolbox"
publish_platform "server-darwin-arm64" "toolbox.darwin.arm64"  "toolbox"
publish_platform "server-darwin-x64"   "toolbox.darwin.amd64"  "toolbox"
publish_platform "server-win32-x64"    "toolbox.windows.amd64" "toolbox.exe"
publish_platform "server-win32-arm64"  "toolbox.windows.arm64" "toolbox.exe"

publish_pkg "server"
