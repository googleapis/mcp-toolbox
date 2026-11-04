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
	"sort"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/log"
)

// configRestrictions holds the source and tool type restrictions from
// ServerConfig. A nil allowedSources means no source restriction. A non-nil but
// empty allowedSources allows no source types.
type configRestrictions struct {
	allowedSources  map[string]struct{}
	disallowedTools map[string]struct{}
}

// newTypeSet builds a set from a list of types. It trims whitespace around each
// entry and drops empty entries. It returns nil when the list has no entries,
// which means the list is not set.
func newTypeSet(entries []string) map[string]struct{} {
	if len(entries) == 0 {
		return nil
	}
	set := make(map[string]struct{})
	for _, entry := range entries {
		entry = strings.ToLower(strings.TrimSpace(entry))
		if entry == "" {
			continue
		}
		set[entry] = struct{}{}
	}
	return set
}

// newConfigRestrictions reads the source and tool restrictions from the
// AllowedSources and DisallowedTools fields of cfg.
func newConfigRestrictions(cfg ServerConfig) configRestrictions {
	return configRestrictions{
		allowedSources:  newTypeSet(cfg.AllowedSources),
		disallowedTools: newTypeSet(cfg.DisallowedTools),
	}
}

// active reports whether any restriction is set.
func (r configRestrictions) active() bool {
	return r.allowedSources != nil || len(r.disallowedTools) > 0
}

// validate checks the source and tool types in cfg against the restrictions.
// It runs on the raw configs, before any source opens a connection. It returns
// one error that lists every violation in sorted order.
func (r configRestrictions) validate(cfg ServerConfig) error {
	var violations []string

	if r.allowedSources != nil {
		names := make([]string, 0, len(cfg.SourceConfigs))
		for name := range cfg.SourceConfigs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			sc := cfg.SourceConfigs[name]
			if sc == nil {
				continue
			}
			sourceType := sc.SourceConfigType()
			if _, ok := r.allowedSources[strings.ToLower(sourceType)]; !ok {
				violations = append(violations, fmt.Sprintf("source %q has type %q, which is not allowed by TOOLBOX_ALLOWED_SOURCES", name, sourceType))
			}
		}
	}

	if len(r.disallowedTools) > 0 {
		names := make([]string, 0, len(cfg.ToolConfigs))
		for name := range cfg.ToolConfigs {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			tc := cfg.ToolConfigs[name]
			if tc == nil {
				continue
			}
			toolType := tc.ToolConfigType()
			if _, ok := r.disallowedTools[strings.ToLower(toolType)]; ok {
				violations = append(violations, fmt.Sprintf("tool %q has type %q, which is disallowed by TOOLBOX_DISALLOWED_TOOLS", name, toolType))
			}
		}
	}

	if len(violations) > 0 {
		return fmt.Errorf("config contains disallowed sources or tools: %s", strings.Join(violations, "; "))
	}
	return nil
}

// checkRestrictions logs any active source and tool restrictions and validates
// cfg against them. It must run before any source opens a connection. When no
// restriction is set, it does nothing.
func checkRestrictions(ctx context.Context, cfg ServerConfig, l log.Logger) error {
	r := newConfigRestrictions(cfg)
	if !r.active() {
		return nil
	}
	if r.allowedSources != nil {
		l.InfoContext(ctx, fmt.Sprintf("Restricting source types to: %s", strings.Join(sortedKeys(r.allowedSources), ", ")))
	}
	if len(r.disallowedTools) > 0 {
		l.InfoContext(ctx, fmt.Sprintf("Disallowing tool types: %s", strings.Join(sortedKeys(r.disallowedTools), ", ")))
	}
	return r.validate(cfg)
}

// sortedKeys returns the keys of set in sorted order.
func sortedKeys(set map[string]struct{}) []string {
	keys := make([]string, 0, len(set))
	for k := range set {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
