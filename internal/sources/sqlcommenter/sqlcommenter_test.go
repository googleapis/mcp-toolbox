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
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/googleapis/mcp-toolbox/internal/util"
)

// sqlCommenterCtx returns a context with sql-commenter enabled.
func sqlCommenterCtx() context.Context {
	return util.WithSQLCommenterEnabled(context.Background(), true)
}

func TestPrependComment_SQLCommenterDisabled(t *testing.T) {
	// SQL commenter not enabled in context — statement should be unchanged
	ctx := context.Background()
	ctx = util.WithUserAgent(ctx, "1.1.0")
	ctx = util.WithGenAIMetricAttrs(ctx, &util.GenAIMetricAttrs{
		ToolName: "search_hotels",
	})

	stmt := "SELECT * FROM users"
	result := PrependComment(ctx, stmt, "postgresql", nil)

	if result != stmt {
		t.Errorf("expected unchanged statement when sql-commenter disabled, got: %s", result)
	}
}

// TestPrependComment_SourceOverride verifies the priority between the global
// sql-commenter flag (from context) and the per-source `sqlCommenter` setting.
// The per-source value wins when set; otherwise the global flag is used.
func TestPrependComment_SourceOverride(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }
	stmt := "SELECT 1"

	cases := []struct {
		name           string
		global         bool
		sourceOverride *bool
		wantComment    bool
	}{
		{"global on, source on", true, boolPtr(true), true},
		{"global on, source unset", true, nil, true},
		{"global on, source off", true, boolPtr(false), false},
		{"global off, source on", false, boolPtr(true), true},
		{"global off, source unset", false, nil, false},
		{"global off, source off", false, boolPtr(false), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := util.WithSQLCommenterEnabled(context.Background(), tc.global)
			result := PrependComment(ctx, stmt, "postgresql", tc.sourceOverride)

			gotComment := strings.HasPrefix(result, "/*")
			if gotComment != tc.wantComment {
				t.Errorf("wantComment=%v, got result: %q", tc.wantComment, result)
			}
			if !tc.wantComment && result != stmt {
				t.Errorf("expected unchanged statement, got: %q", result)
			}
		})
	}
}

func TestPrependComment_EmptyContext(t *testing.T) {
	ctx := sqlCommenterCtx()
	stmt := "SELECT * FROM users"
	result := PrependComment(ctx, stmt, "", nil)

	// No attributes available, statement should be unchanged
	if result != stmt {
		t.Errorf("expected unchanged statement, got: %s", result)
	}
}

func TestPrependComment_OnlyDbSystemName(t *testing.T) {
	ctx := sqlCommenterCtx()
	stmt := "SELECT * FROM users"
	result := PrependComment(ctx, stmt, "postgresql", nil)

	expected := "/*db.system.name='postgresql'*/ SELECT * FROM users"
	if result != expected {
		t.Errorf("expected %s, got: %s", expected, result)
	}
}

func TestPrependComment_ServerSideAttributes(t *testing.T) {
	ctx := sqlCommenterCtx()
	ctx = util.WithUserAgent(ctx, "1.1.0")
	ctx = util.WithGenAIMetricAttrs(ctx, &util.GenAIMetricAttrs{
		ToolName: "search_hotels",
	})

	stmt := "SELECT * FROM hotels"
	result := PrependComment(ctx, stmt, "postgresql", nil)

	// Should contain server, tool.name, db.system.name
	if !strings.Contains(result, "/*") || !strings.Contains(result, "*/") {
		t.Errorf("expected SQL comment, got: %s", result)
	}
	if !strings.Contains(result, "db.system.name='postgresql'") {
		t.Errorf("missing db.system.name, got: %s", result)
	}
	if !strings.Contains(result, "server='"+url.QueryEscape("genai-toolbox/1.1.0")+"'") {
		t.Errorf("missing server, got: %s", result)
	}
	if !strings.Contains(result, "tool.name='search_hotels'") {
		t.Errorf("missing tool.name, got: %s", result)
	}
	// Comment should be prepended
	if !strings.HasPrefix(result, "/*") {
		t.Errorf("expected comment prepended to statement, got: %s", result)
	}
}

func TestPrependComment_FullAttributes(t *testing.T) {
	ctx := sqlCommenterCtx()
	ctx = util.WithUserAgent(ctx, "1.1.0")
	ctx = util.WithGenAIMetricAttrs(ctx, &util.GenAIMetricAttrs{
		ToolName: "search_user",
	})
	ctx = util.WithTelemetryAttributes(ctx, &util.TelemetryAttributes{
		ClientName:    "toolbox-langchain-python",
		ClientVersion: "v0.1.0",
		ClientModel:   "gemini-2.5-flash",
		ClientUserID:  "user-123",
		ClientAgentID: "agent-456",
	})

	stmt := "SELECT * FROM users"
	result := PrependComment(ctx, stmt, "postgresql", nil)

	// Verify all expected key='value' pairs are present
	expectedPairs := []string{
		"client='" + url.QueryEscape("toolbox-langchain-python/v0.1.0") + "'",
		"client.agent.id='agent-456'",
		"client.model='gemini-2.5-flash'",
		"client.user.id='user-123'",
		"db.system.name='postgresql'",
		"server='" + url.QueryEscape("genai-toolbox/1.1.0") + "'",
		"tool.name='search_user'",
	}
	for _, pair := range expectedPairs {
		if !strings.Contains(result, pair) {
			t.Errorf("missing pair %q in: %s", pair, result)
		}
	}
}

func TestPrependComment_AlphabeticalOrder(t *testing.T) {
	ctx := sqlCommenterCtx()
	ctx = util.WithUserAgent(ctx, "1.0.0")
	ctx = util.WithGenAIMetricAttrs(ctx, &util.GenAIMetricAttrs{
		ToolName: "my_tool",
	})
	ctx = util.WithTelemetryAttributes(ctx, &util.TelemetryAttributes{
		ClientName:    "test-client",
		ClientVersion: "v1",
		ClientModel:   "model-x",
	})

	stmt := "SELECT 1"
	result := PrependComment(ctx, stmt, "postgresql", nil)

	// Extract the comment part
	commentStart := strings.Index(result, "/*")
	commentEnd := strings.Index(result, "*/")
	if commentStart == -1 || commentEnd == -1 {
		t.Fatalf("no comment found in: %s", result)
	}
	comment := result[commentStart+2 : commentEnd]
	parts := strings.Split(comment, ",")

	// Verify keys are sorted
	for i := 1; i < len(parts); i++ {
		prevKey := strings.SplitN(parts[i-1], "=", 2)[0]
		currKey := strings.SplitN(parts[i], "=", 2)[0]
		if prevKey > currKey {
			t.Errorf("keys not sorted: %s comes before %s", prevKey, currKey)
		}
	}
}

func TestPrependComment_URLEncoding(t *testing.T) {
	ctx := sqlCommenterCtx()
	ctx = util.WithTelemetryAttributes(ctx, &util.TelemetryAttributes{
		ClientName:    "my client/special",
		ClientVersion: "v1.0",
	})

	stmt := "SELECT 1"
	result := PrependComment(ctx, stmt, "", nil)

	// The client value "my client/special/v1.0" should be URL-encoded
	if !strings.Contains(result, "client='"+url.QueryEscape("my client/special/v1.0")+"'") {
		t.Errorf("expected URL-encoded client, got: %s", result)
	}
}

func TestPrependComment_PartialClientAttributes(t *testing.T) {
	ctx := sqlCommenterCtx()
	ctx = util.WithTelemetryAttributes(ctx, &util.TelemetryAttributes{
		ClientName: "test-client",
		// No version
	})

	stmt := "SELECT 1"
	result := PrependComment(ctx, stmt, "", nil)

	if !strings.Contains(result, "client='test-client'") {
		t.Errorf("expected client with name only, got: %s", result)
	}
}

func TestPrependComment_EmptyTelemetryAttributes(t *testing.T) {
	ctx := sqlCommenterCtx()
	ctx = util.WithTelemetryAttributes(ctx, &util.TelemetryAttributes{})

	stmt := "SELECT 1"
	result := PrependComment(ctx, stmt, "postgresql", nil)

	// Should only have db.system.name since all telemetry attrs are empty
	if !strings.Contains(result, "db.system.name='postgresql'") {
		t.Errorf("expected db.system.name, got: %s", result)
	}
}

func TestAppendLabels_Disabled(t *testing.T) {
	// SQL commenter not enabled in context — no labels should be produced
	ctx := context.Background()
	ctx = util.WithUserAgent(ctx, "1.1.0")
	ctx = util.WithGenAIMetricAttrs(ctx, &util.GenAIMetricAttrs{
		ToolName: "search_hotels",
	})

	if labels := AppendLabels(ctx, nil, "bigquery", nil); labels != nil {
		t.Errorf("expected nil labels when sql-commenter disabled, got: %v", labels)
	}
}

// TestLabels_SourceOverride verifies the priority between the global
// sql-commenter flag (from context) and the per-source `sqlCommenter`
// setting, mirroring the PrependComment behavior.
func TestAppendLabels_SourceOverride(t *testing.T) {
	boolPtr := func(b bool) *bool { return &b }

	cases := []struct {
		name           string
		global         bool
		sourceOverride *bool
		wantLabels     bool
	}{
		{"global on, source on", true, boolPtr(true), true},
		{"global on, source unset", true, nil, true},
		{"global on, source off", true, boolPtr(false), false},
		{"global off, source on", false, boolPtr(true), true},
		{"global off, source unset", false, nil, false},
		{"global off, source off", false, boolPtr(false), false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctx := util.WithSQLCommenterEnabled(context.Background(), tc.global)
			labels := AppendLabels(ctx, nil, "bigquery", tc.sourceOverride)

			gotLabels := len(labels) > 0
			if gotLabels != tc.wantLabels {
				t.Errorf("wantLabels=%v, got: %v", tc.wantLabels, labels)
			}
		})
	}
}

func TestAppendLabels_EmptyContext(t *testing.T) {
	ctx := sqlCommenterCtx()

	// No attributes available at all — nil map, not an empty one
	if labels := AppendLabels(ctx, nil, "", nil); labels != nil {
		t.Errorf("expected nil labels for empty context, got: %v", labels)
	}
}

func TestAppendLabels_FullAttributes(t *testing.T) {
	ctx := sqlCommenterCtx()
	ctx = util.WithUserAgent(ctx, "1.1.0")
	ctx = util.WithGenAIMetricAttrs(ctx, &util.GenAIMetricAttrs{
		ToolName: "search_user",
	})
	ctx = util.WithTelemetryAttributes(ctx, &util.TelemetryAttributes{
		ClientName:    "toolbox-langchain-python",
		ClientVersion: "v0.1.0",
		ClientModel:   "gemini-2.5-flash",
		ClientUserID:  "user-123",
		ClientAgentID: "agent-456",
	})

	labels := AppendLabels(ctx, nil, "bigquery", nil)

	// Attribute names map dots to underscores; values have characters
	// outside [a-z0-9_-] replaced with underscores.
	expected := map[string]string{
		"client":          "toolbox-langchain-python_v0_1_0",
		"client_agent_id": "agent-456",
		"client_model":    "gemini-2_5-flash",
		"client_user_id":  "user-123",
		"db_system_name":  "bigquery",
		"server":          "genai-toolbox_1_1_0",
		"tool_name":       "search_user",
	}
	if !reflect.DeepEqual(labels, expected) {
		t.Errorf("expected %v, got: %v", expected, labels)
	}
}

func TestSanitizeLabelKey(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "traceparent", "traceparent"},
		{"dots to underscores", "tool.name", "tool_name"},
		{"uppercase lowered", "Tool.Name", "tool_name"},
		{"dash kept", "client-id", "client-id"},
		{"leading digit prefixed", "0key", "x0key"},
		{"leading underscore prefixed", "_key", "x_key"},
		{"unicode replaced", "kéy", "k_y"},
		{"empty stays empty", "", ""},
		{"truncated to 63", strings.Repeat("a", 100), strings.Repeat("a", 63)},
		{"prefix respects max length", "0" + strings.Repeat("a", 63), "x0" + strings.Repeat("a", 61)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLabelKey(tc.in); got != tc.want {
				t.Errorf("sanitizeLabelKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestSanitizeLabelPart(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "search_user", "search_user"},
		{"slash and dots replaced", "genai-toolbox/1.1.0", "genai-toolbox_1_1_0"},
		{"uppercase lowered", "Test-Client", "test-client"},
		{"spaces replaced", "a b c", "a_b_c"},
		{"empty allowed", "", ""},
		{"leading digit kept", "00-abc", "00-abc"},
		// Unlike keys, values have no leading-character requirement:
		// BigQuery accepts values starting with an underscore or dash.
		{"leading underscore kept", "_internal", "_internal"},
		{"leading dash kept", "-flag", "-flag"},
		{"leading dot becomes underscore", ".hidden", "_hidden"},
		{"truncated to 63", strings.Repeat("v", 100), strings.Repeat("v", 63)},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sanitizeLabelPart(tc.in); got != tc.want {
				t.Errorf("sanitizeLabelPart(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestAppendLabels_MergePrecedence verifies that labels already present on
// the job are returned unchanged when the commenter is disabled and always
// win over commenter attributes on key collisions.
func TestAppendLabels_MergePrecedence(t *testing.T) {
	explicit := map[string]string{"mcp-toolbox-tool": "bigquery-execute-sql", "tool_name": "explicit-value"}

	// Disabled: input map returned unchanged.
	if got := AppendLabels(context.Background(), explicit, "bigquery", nil); !reflect.DeepEqual(got, explicit) {
		t.Errorf("expected input labels unchanged when disabled, got: %v", got)
	}

	// Enabled: commenter attributes merged in, explicit labels win.
	ctx := sqlCommenterCtx()
	ctx = util.WithGenAIMetricAttrs(ctx, &util.GenAIMetricAttrs{ToolName: "execute_sql"})
	got := AppendLabels(ctx, explicit, "bigquery", nil)
	if got["tool_name"] != "explicit-value" {
		t.Errorf("expected explicit label to win on collision, got: %q", got["tool_name"])
	}
	if got["mcp-toolbox-tool"] != "bigquery-execute-sql" {
		t.Errorf("expected existing label preserved, got: %q", got["mcp-toolbox-tool"])
	}
	if got["db_system_name"] != "bigquery" {
		t.Errorf("expected commenter attribute merged, got: %v", got)
	}
}
