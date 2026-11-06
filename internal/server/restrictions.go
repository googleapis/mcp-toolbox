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

package server

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/log"
)

const gcpSourcesPreset = "gcp-sources"

// gcpSourceTypes lists the source types allowed by the gcp-sources preset.
var gcpSourceTypes = []string{
	"alloydb-admin",
	"alloydb-postgres",
	"bigquery",
	"bigtable",
	"cloud-gemini-data-analytics",
	"cloud-healthcare",
	"cloud-logging-admin",
	"cloud-monitoring",
	"cloud-sql-admin",
	"cloud-sql-mssql",
	"cloud-sql-mysql",
	"cloud-sql-postgres",
	"cloud-storage",
	"databaseinsights",
	"datalineage",
	"dataplex",
	"dataproc",
	"firestore",
	"looker",
	"serverless-spark",
	"spanner",
}

// checkRestrictions validates cfg against AllowedSources and DisallowedTools
// before any source opens a connection.
func checkRestrictions(ctx context.Context, cfg ServerConfig, l log.Logger) error {
	var violations []string

	if len(cfg.AllowedSources) > 0 {
		allowed := make(map[string]struct{}, len(cfg.AllowedSources))
		for _, s := range cfg.AllowedSources {
			s = strings.ToLower(strings.TrimSpace(s))
			if s == gcpSourcesPreset {
				for _, gcp := range gcpSourceTypes {
					allowed[gcp] = struct{}{}
				}
			} else if s != "" {
				allowed[s] = struct{}{}
			}
		}
		l.InfoContext(ctx, fmt.Sprintf("Restricting source types to: %s", strings.Join(slices.Sorted(maps.Keys(allowed)), ", ")))

		for _, name := range slices.Sorted(maps.Keys(cfg.SourceConfigs)) {
			sourceType := cfg.SourceConfigs[name].SourceConfigType()
			if _, ok := allowed[strings.ToLower(sourceType)]; !ok {
				violations = append(violations, fmt.Sprintf("source %q has type %q, which is not allowed by TOOLBOX_ALLOWED_SOURCES", name, sourceType))
			}
		}
	}

	if len(cfg.DisallowedTools) > 0 {
		disallowed := make(map[string]struct{}, len(cfg.DisallowedTools))
		for _, t := range cfg.DisallowedTools {
			if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
				disallowed[t] = struct{}{}
			}
		}
		if len(disallowed) > 0 {
			l.InfoContext(ctx, fmt.Sprintf("Disallowing tool types: %s", strings.Join(slices.Sorted(maps.Keys(disallowed)), ", ")))
		}

		for _, name := range slices.Sorted(maps.Keys(cfg.ToolConfigs)) {
			toolType := cfg.ToolConfigs[name].ToolConfigType()
			if _, ok := disallowed[strings.ToLower(toolType)]; ok {
				violations = append(violations, fmt.Sprintf("tool %q has type %q, which is disallowed by TOOLBOX_DISALLOWED_TOOLS", name, toolType))
			}
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("config contains disallowed sources or tools: %s", strings.Join(violations, "; "))
	}
	return nil
}
