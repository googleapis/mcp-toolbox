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

//go:build !omni_only

package spanner

import (
	"context"
	"fmt"
	"regexp"
	"testing"
	"time"

	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

// setupSpannerPostgresqlMCPServer builds the PostgreSQL-dialect config and
// starts a Toolbox server serving it over the MCP endpoint.
func setupSpannerPostgresqlMCPServer(t *testing.T, ctx context.Context) {
	if SpannerProject == "" || SpannerInstance == "" {
		t.Skip("SPANNER_PROJECT or SPANNER_INSTANCE not set, skipping PostgreSQL tests")
	}

	adminClient, err := database.NewDatabaseAdminClient(ctx)
	if err != nil {
		t.Fatalf("unable to create admin client: %s", err)
	}
	t.Cleanup(func() { adminClient.Close() })

	dbName := SpannerPgDatabase
	if dbName == "" {
		dbName = "pg_test_database"
	}
	t.Logf("Using PostgreSQL database: %s", dbName)

	dbString := fmt.Sprintf("projects/%s/instances/%s/databases/%s", SpannerProject, SpannerInstance, dbName)

	// Set up table for semantic search
	vectorTableName, tearDownVectorTable := setupSpannerPgVectorTable(t, ctx, adminClient, dbString)
	t.Cleanup(func() { tearDownVectorTable(t) })

	insertStmt, searchStmt := getSpannerPgVectorSearchStmts(vectorTableName)

	config := map[string]any{
		"sources": map[string]any{
			"my-instance": map[string]any{
				"type":     "spanner",
				"project":  SpannerProject,
				"instance": SpannerInstance,
				"database": dbName,
				"dialect":  "postgresql",
			},
		},
		"tools": map[string]any{},
	}
	toolsConfig := tests.AddSemanticSearchConfig(t, config, SpannerToolType, insertStmt, searchStmt)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsConfig)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(func() {
		cmd.Close()
		cleanup()
	})

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

func TestSpannerPostgresqlMCPListTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	t.Cleanup(cancel)

	setupSpannerPostgresqlMCPServer(t, ctx)

	// This config registers only the semantic search tools.
	t.Run("verify tools/list registry returns complete manifest", func(t *testing.T) {
		tests.RunMCPToolsListMethod(t, keepSpannerTools("insert_docs", "search_docs"))
	})
}

func TestSpannerPostgresqlMCPCallTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	t.Cleanup(cancel)

	setupSpannerPostgresqlMCPServer(t, ctx)

	tests.RunSemanticSearchToolInvokeTest(t, "[]", "", "The quick brown fox", tests.WithMCPExec())
}
