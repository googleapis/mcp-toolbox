// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package util

const (
	VERSION_20241105 = "2024-11-05"
	VERSION_20250326 = "2025-03-26"
	VERSION_20250618 = "2025-06-18"
	VERSION_20251125 = "2025-11-25"
	VERSION_20260728 = "2026-07-28"
)

// LATEST_PROTOCOL_VERSION is the latest version of the MCP protocol supported.
// Update the version used in InitializeResponse when this value is updated.
const LATEST_PROTOCOL_VERSION = VERSION_20251125

// LATEST_PROTOCOL_VERSION_NONSTABLE is the latest version of the MCP protocol
// supported that includes non-stable version
const LATEST_PROTOCOL_VERSION_NONSTABLE = VERSION_20260728

// SUPPORTED_PROTOCOL_VERSIONS is the MCP protocol versions that are supported.
var SUPPORTED_PROTOCOL_VERSIONS = []string{
	VERSION_20241105,
	VERSION_20250326,
	VERSION_20250618,
	VERSION_20251125,
	VERSION_20260728,
}

var SUPPORTED_PROTOCOL_VERSIONS_NONSTABLE = []string{}

func GetSupportedVersions(enableDraft bool) []string {
	if enableDraft {
		return append(SUPPORTED_PROTOCOL_VERSIONS, SUPPORTED_PROTOCOL_VERSIONS_NONSTABLE...)
	}
	return SUPPORTED_PROTOCOL_VERSIONS
}

func GetLatestSupportedVersion(enableDraft bool) string {
	if enableDraft {
		return LATEST_PROTOCOL_VERSION_NONSTABLE
	}
	return LATEST_PROTOCOL_VERSION
}
