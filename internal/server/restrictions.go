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
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/log"
)

// sourcePresets maps preset names accepted in TOOLBOX_ALLOWED_SOURCES to the
// source types they expand to.
var sourcePresets = map[string][]string{
	"gcp-sources": {
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
	},
}

// checkRestrictions validates cfg against AllowedSources and DisallowedTools
// before any source opens a connection.
func checkRestrictions(ctx context.Context, cfg ServerConfig, l log.Logger) error {
	// rejectedSources maps each rejected source name to its type.
	rejectedSources := make(map[string]string)
	allowed := make(map[string]struct{}, len(cfg.AllowedSources))
	for _, s := range cfg.AllowedSources {
		s = strings.ToLower(strings.TrimSpace(s))
		if preset, ok := sourcePresets[s]; ok {
			for _, st := range preset {
				allowed[st] = struct{}{}
			}
		} else if s != "" {
			allowed[s] = struct{}{}
		}
	}
	if len(allowed) > 0 {
		l.InfoContext(ctx, fmt.Sprintf("Restricting source types to: %s", strings.Join(slices.Sorted(maps.Keys(allowed)), ", ")))

		for name, sc := range cfg.SourceConfigs {
			if _, ok := allowed[strings.ToLower(sc.SourceConfigType())]; !ok {
				rejectedSources[name] = sc.SourceConfigType()
			}
		}
	}

	disallowed := make(map[string]struct{}, len(cfg.DisallowedTools))
	for _, t := range cfg.DisallowedTools {
		if t = strings.ToLower(strings.TrimSpace(t)); t != "" {
			disallowed[t] = struct{}{}
		}
	}
	if len(disallowed) > 0 {
		l.InfoContext(ctx, fmt.Sprintf("Disallowing tool types: %s", strings.Join(slices.Sorted(maps.Keys(disallowed)), ", ")))
	}

	var toolViolations []string
	usedBy := make(map[string][]string) // rejected source name -> tool names
	if len(disallowed) > 0 || len(rejectedSources) > 0 {
		for _, name := range slices.Sorted(maps.Keys(cfg.ToolConfigs)) {
			tc := cfg.ToolConfigs[name]
			toolType := tc.ToolConfigType()
			if _, ok := disallowed[strings.ToLower(toolType)]; ok {
				toolViolations = append(toolViolations, fmt.Sprintf("%s (type: %s)", name, toolType))
				continue
			}
			if len(rejectedSources) == 0 {
				continue
			}
			// ToolConfig does not expose its source name, so initialize the
			// tool to read it. Tool initialization opens no connections, and
			// this only runs when config loading is already failing. Tools
			// that fail to initialize are skipped.
			t, err := tc.Initialize(ctx)
			if err != nil || t == nil {
				continue
			}
			if src := t.GetSourceName(); src != "" {
				if _, ok := rejectedSources[src]; ok {
					usedBy[src] = append(usedBy[src], name)
				}
			}
		}
	}

	if len(rejectedSources) == 0 && len(toolViolations) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("config contains disallowed sources or tools:")
	if len(rejectedSources) > 0 {
		b.WriteString("\n  sources not allowed by TOOLBOX_ALLOWED_SOURCES:")
		for _, name := range slices.Sorted(maps.Keys(rejectedSources)) {
			fmt.Fprintf(&b, "\n    - %s (type: %s)", name, rejectedSources[name])
			if toolNames := usedBy[name]; len(toolNames) > 0 {
				fmt.Fprintf(&b, ", used by tools: %s", strings.Join(toolNames, ", "))
			}
		}
	}
	if len(toolViolations) > 0 {
		b.WriteString("\n  tools disallowed by TOOLBOX_DISALLOWED_TOOLS:")
		for _, v := range toolViolations {
			fmt.Fprintf(&b, "\n    - %s", v)
		}
	}
	return errors.New(b.String())
}
