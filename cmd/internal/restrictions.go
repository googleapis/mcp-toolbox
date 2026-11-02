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

package internal

import (
	"os"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/sources/alloydbpg"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudsqlmssql"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudsqlmysql"
	"github.com/googleapis/mcp-toolbox/internal/sources/cloudsqlpg"
	"github.com/googleapis/mcp-toolbox/internal/sources/spanner"
)

const (
	// allowedSourcesEnvVar holds a comma-separated allowlist of source types.
	// When it is unset or empty, every source type is allowed.
	allowedSourcesEnvVar = "TOOLBOX_ALLOWED_SOURCES"
	// disallowedToolsEnvVar holds a comma-separated denylist of tool types.
	// When it is unset or empty, every tool type is allowed.
	disallowedToolsEnvVar = "TOOLBOX_DISALLOWED_TOOLS"
	// gcpSourcesPreset is a TOOLBOX_ALLOWED_SOURCES entry that expands to
	// gcpSourceTypes.
	gcpSourcesPreset = "gcp-sources"
)

// gcpSourceTypes lists the source types allowed by the gcp-sources preset.
var gcpSourceTypes = []string{
	alloydbpg.SourceType,
	cloudsqlmssql.SourceType,
	cloudsqlmysql.SourceType,
	cloudsqlpg.SourceType,
	spanner.SourceType,
}

// loadRestrictionsFromEnv reads the source and tool type restrictions from
// TOOLBOX_ALLOWED_SOURCES and TOOLBOX_DISALLOWED_TOOLS into opts.Cfg. There
// are no flags for these restrictions.
func (opts *ToolboxOptions) loadRestrictionsFromEnv() {
	opts.Cfg.AllowedSources = expandSourcePresets(splitEnvList(os.Getenv(allowedSourcesEnvVar)))
	opts.Cfg.DisallowedTools = splitEnvList(os.Getenv(disallowedToolsEnvVar))
}

// splitEnvList splits a comma-separated env value. It returns nil when the
// value is empty or only whitespace.
func splitEnvList(v string) []string {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return strings.Split(v, ",")
}

// expandSourcePresets replaces each preset entry with the source types it
// stands for. Other entries are kept as they are, so presets and plain types
// can be combined.
func expandSourcePresets(entries []string) []string {
	if entries == nil {
		return nil
	}
	expanded := make([]string, 0, len(entries))
	for _, e := range entries {
		if strings.TrimSpace(e) == gcpSourcesPreset {
			expanded = append(expanded, gcpSourceTypes...)
			continue
		}
		expanded = append(expanded, e)
	}
	return expanded
}
