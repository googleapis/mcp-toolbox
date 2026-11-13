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

package alloydbomni

import (
	"context"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/tests"
)

// getAlloyDBOmniMCPExpectedTools returns the MCP manifests for the alloydb-omni
// prebuilt tools: the Postgres prebuilt tools, with the descriptions the omni
// prebuilt uses, plus the omni-only tools.
func getAlloyDBOmniMCPExpectedTools() []tests.MCPToolManifest {
	// Descriptions that differ from the postgres test config.
	omniDescriptions := map[string]string{
		"list_active_queries":       "List the top N (default 50) currently running queries (state='active') from pg_stat_activity, ordered by longest-running first. Returns pid, user, database, application_name, client_addr, state, wait_event_type/wait_event, backend/xact/query start times, computed query_duration, and the SQL text.",
		"list_available_extensions": "Discover all PostgreSQL extensions available for installation on this server, returning name, default_version, and description.",
		"list_installed_extensions": "List all installed PostgreSQL extensions with their name, version, schema, owner, and description.",
		"list_tables":               "Lists detailed schema information (object type, columns, constraints, indexes, triggers, owner, comment) as JSON for user-created tables (ordinary or partitioned). Filters by a comma-separated list of names. If names are omitted, lists all tables in user schemas.",
	}
	// Tools in the alloydb-omni prebuilt that the postgres one doesn't have.
	omniOnly := []tests.MCPToolManifest{
		{
			Name:        "execute_sql",
			Description: "Use this tool to execute a single SQL statement.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"sql": map[string]any{
						"type":        "string",
						"description": "The sql to execute.",
					},
				},
				"required": []any{"sql"},
			},
		},
		{
			Name:        "get_query_plan",
			Description: "Generate a PostgreSQL EXPLAIN plan in JSON format for a single SQL statement\u2014without executing it. This returns the optimizer's estimated plan, costs, and rows (no ANALYZE, no extra options).",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The SQL statement for which you want to generate plan (omit the EXPLAIN keyword).",
					},
				},
				"required": []any{"query"},
			},
		},
		{
			Name:        "list_autovacuum_configurations",
			Description: "List PostgreSQL autovacuum-related configurations (name and current setting) from pg_settings.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []any{},
			},
		},
		{
			Name:        "list_columnar_configurations",
			Description: "List AlloyDB Omni columnar-related configurations (name and current setting) from pg_settings.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []any{},
			},
		},
		{
			Name:        "list_columnar_recommended_columns",
			Description: "Lists columns that AlloyDB Omni recommends adding to the columnar engine to improve query performance.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []any{},
			},
		},
		{
			Name:        "list_invalid_indexes",
			Description: "Lists all invalid PostgreSQL indexes which are taking up disk space but are unusable by the query planner. Typically created by failed CREATE INDEX CONCURRENTLY operations.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []any{},
			},
		},
		{
			Name:        "list_memory_configurations",
			Description: "List PostgreSQL memory-related configurations (name and current setting) from pg_settings.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []any{},
			},
		},
		{
			Name:        "list_replication_slots",
			Description: "List key details for all PostgreSQL replication slots (e.g., type, database, active status) and calculates the size of the outstanding WAL that is being prevented from removal by the slot.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{},
				"required":   []any{},
			},
		},
		{
			Name:        "list_top_bloated_tables",
			Description: "List the top tables by dead-tuple (approximate bloat signal), returning schema, table, live/dead tuples, percentage, and last vacuum/analyze times.\n",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"limit": map[string]any{
						"type":        "integer",
						"description": "The maximum number of results to return.",
						"default":     float64(50),
					},
				},
				"required": []any{},
			},
		},
	}

	expected := tests.GetPostgresPrebuiltMCPExpectedTools()
	for i := range expected {
		if desc, ok := omniDescriptions[expected[i].Name]; ok {
			expected[i].Description = desc
		}
	}
	return append(expected, omniOnly...)
}

func TestAlloyDBOmniMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// The prebuilt source connects to the database on startup, so this needs
	// the container too.
	setupAlloyDBOmniTest(t, ctx, true, false)

	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, getAlloyDBOmniMCPExpectedTools())
	})
}

func TestAlloyDBOmniMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	pool, uniqueID := setupAlloyDBOmniTest(t, ctx, true, false)
	runAlloyDBOmniTests(t, ctx, pool, uniqueID, tests.WithMCPExec())
}

func TestAlloyDBOmniMCPColumnar(t *testing.T) {
	skipUnlessColumnar(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	setupAlloyDBOmniTest(t, ctx, true, true)
	runAlloyDBOmniColumnarTests(t, tests.WithMCPExec())
}
