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

package sqlcommenter

import (
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"

	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
)

// PrependComment prepends a SQLCommenter-format comment to the given SQL statement.
// It gathers attributes from the context (trace, server, client, tool metadata)
// and the provided dbSystemName, then prepends them as key='value' pairs sorted
// alphabetically.
//
// sourceOverride is the per-source `sqlCommenter` setting from tools.yaml. When
// non-nil it takes priority over the global sql-commenter flag; when nil the
// global flag (from context) is used.
func PrependComment(ctx context.Context, statement string, dbSystemName string, sourceOverride *bool) string {
	// Per-source config wins when set; otherwise fall back to the global flag.
	enabled := util.SQLCommenterEnabledFromContext(ctx)
	if sourceOverride != nil {
		enabled = *sourceOverride
	}

	// Only prepend SQL comments when sql-commenter is enabled
	if !enabled {
		return statement
	}

	pairs := collectAttributes(ctx, dbSystemName)
	if len(pairs) == 0 {
		return statement
	}

	// Sort keys alphabetically
	keys := make([]string, 0, len(pairs))
	for k := range pairs {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build comment in SQLCommenter format: key='url_encoded_value'
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		encodedKey := url.QueryEscape(k)
		encodedVal := url.QueryEscape(pairs[k])
		parts = append(parts, fmt.Sprintf("%s='%s'", encodedKey, encodedVal))
	}

	comment := strings.Join(parts, ",")
	return "/*" + comment + "*/ " + statement
}

// collectAttributes gathers all available SQLCommenter attributes from context.
func collectAttributes(ctx context.Context, dbSystemName string) map[string]string {
	attrs := make(map[string]string)

	// traceparent from OTel span context
	spanCtx := trace.SpanFromContext(ctx).SpanContext()
	if spanCtx.IsValid() {
		traceparent := fmt.Sprintf("00-%s-%s-%s",
			spanCtx.TraceID().String(),
			spanCtx.SpanID().String(),
			spanCtx.TraceFlags().String(),
		)
		attrs["traceparent"] = traceparent
	}

	// server from UserAgent context
	if ua, err := util.UserAgentFromContext(ctx); err == nil && ua != "" {
		attrs["server"] = ua
	}

	// db.system.name from parameter
	if dbSystemName != "" {
		attrs["db.system.name"] = dbSystemName
	}

	// tool.name from GenAIMetricAttrs
	if genAI := util.GenAIMetricAttrsFromContext(ctx); genAI != nil {
		if genAI.ToolName != "" {
			attrs["tool.name"] = genAI.ToolName
		}
	}

	// Client attributes from TelemetryAttributes
	if ta := util.TelemetryAttributesFromContext(ctx); ta != nil {
		// Combined client = name/version
		if ta.ClientName != "" && ta.ClientVersion != "" {
			attrs["client"] = ta.ClientName + "/" + ta.ClientVersion
		} else if ta.ClientName != "" {
			attrs["client"] = ta.ClientName
		} else if ta.ClientVersion != "" {
			attrs["client"] = ta.ClientVersion
		}

		if ta.ClientModel != "" {
			attrs["client.model"] = ta.ClientModel
		}
		if ta.ClientUserID != "" {
			attrs["client.user.id"] = ta.ClientUserID
		}
		if ta.ClientAgentID != "" {
			attrs["client.agent.id"] = ta.ClientAgentID
		}
	}

	return attrs
}

// AppendLabels is the job-label counterpart to PrependComment for sources
// that attach metadata to jobs natively instead of embedding SQL comments
// (e.g. BigQuery). It merges the SQLCommenter attributes into the given job
// labels and returns the result; labels already present always win on key
// collisions, so tool-supplied labels are never overwritten. Labels surface
// in the source's own job metadata (for BigQuery: INFORMATION_SCHEMA.JOBS
// and billing exports), so no query text parsing is needed to recover them.
//
// The attribute set matches PrependComment exactly. Attribute keys and
// values are machine-derived (e.g. client "name/version", model
// "gemini-2.5-flash") and routinely contain characters BigQuery rejects
// with an invalid-label error, so both are sanitized to the label
// constraints: only lowercase letters, digits, underscores, and dashes, at
// most 63 characters each, and keys must begin with a letter. Attribute
// names map punctuation to underscores, e.g. tool.name becomes tool_name.
// The 64-labels-per-job limit is deliberately left to the API, which
// rejects excess labels with a clear error rather than silently dropping
// telemetry.
//
// sourceOverride behaves as in PrependComment: when non-nil it takes
// priority over the global sql-commenter flag from the context. Returns the
// input labels unchanged (including nil) when the commenter is disabled or
// no attributes are available.
func AppendLabels(ctx context.Context, labels map[string]string, dbSystemName string, sourceOverride *bool) map[string]string {
	enabled := util.SQLCommenterEnabledFromContext(ctx)
	if sourceOverride != nil {
		enabled = *sourceOverride
	}
	if !enabled {
		return labels
	}

	pairs := collectAttributes(ctx, dbSystemName)
	if len(pairs) == 0 {
		return labels
	}

	merged := make(map[string]string, len(labels)+len(pairs))
	for k, v := range pairs {
		key := sanitizeLabelKey(k)
		if key == "" {
			continue
		}
		merged[key] = sanitizeLabelPart(v)
	}
	for k, v := range labels {
		merged[k] = v
	}
	return merged
}

// maxLabelLength is the maximum length of a BigQuery label key or value.
const maxLabelLength = 63

// sanitizeLabelPart lowercases s, replaces every character outside
// [a-z0-9_-] with an underscore, and truncates the result to
// maxLabelLength. The output is always plain ASCII, so byte-wise
// truncation cannot split a character. This is the full rule for label
// values, which may be empty and have no leading-character requirement
// (BigQuery accepts values beginning with an underscore, dash, or digit);
// keys need sanitizeLabelKey on top.
func sanitizeLabelPart(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	out := b.String()
	if len(out) > maxLabelLength {
		out = out[:maxLabelLength]
	}
	return out
}

// sanitizeLabelKey sanitizes s for use as a BigQuery label key. Keys must
// begin with a lowercase letter, so a leading non-letter is prefixed with
// "x". An empty input yields an empty result, which callers should treat
// as an unusable key.
func sanitizeLabelKey(s string) string {
	out := sanitizeLabelPart(s)
	if out == "" {
		return ""
	}
	if out[0] < 'a' || out[0] > 'z' {
		out = "x" + out
		if len(out) > maxLabelLength {
			out = out[:maxLabelLength]
		}
	}
	return out
}
