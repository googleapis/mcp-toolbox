// Copyright 2025 Google LLC
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

package looker

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/tests"

	"github.com/looker-open-source/sdk-codegen/go/rtl"
	v4 "github.com/looker-open-source/sdk-codegen/go/sdk/v4"
)

var (
	LookerSourceType   = "looker"
	LookerBaseUrl      = os.Getenv("LOOKER_BASE_URL")
	LookerVerifySsl    = os.Getenv("LOOKER_VERIFY_SSL")
	LookerClientId     = os.Getenv("LOOKER_CLIENT_ID")
	LookerClientSecret = os.Getenv("LOOKER_CLIENT_SECRET")
	LookerProject      = os.Getenv("LOOKER_PROJECT")
	LookerLocation     = os.Getenv("LOOKER_LOCATION")
)

func getLookerVars(t *testing.T) map[string]any {
	switch "" {
	case LookerBaseUrl:
		t.Fatal("'LOOKER_BASE_URL' not set")
	case LookerVerifySsl:
		t.Fatal("'LOOKER_VERIFY_SSL' not set")
	case LookerClientId:
		t.Fatal("'LOOKER_CLIENT_ID' not set")
	case LookerClientSecret:
		t.Fatal("'LOOKER_CLIENT_SECRET' not set")
	case LookerProject:
		t.Fatal("'LOOKER_PROJECT' not set")
	case LookerLocation:
		t.Fatal("'LOOKER_LOCATION' not set")
	}

	return map[string]any{
		"type":          LookerSourceType,
		"base_url":      LookerBaseUrl,
		"verify_ssl":    (LookerVerifySsl == "true"),
		"client_id":     LookerClientId,
		"client_secret": LookerClientSecret,
		"project":       LookerProject,
		"location":      LookerLocation,
	}
}

func setupLooker(t *testing.T, sourceConfig map[string]any) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	t.Cleanup(cancel)

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	var args []string

	// Write config into a file and pass it to command

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"get_models": map[string]any{
				"type":        "looker-get-models",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_explores": map[string]any{
				"type":        "looker-get-explores",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_explore": map[string]any{
				"type":        "looker-get-explore",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_dimensions": map[string]any{
				"type":        "looker-get-dimensions",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_field_value_suggestions": map[string]any{
				"type":        "looker-get-field-value-suggestions",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_measures": map[string]any{
				"type":        "looker-get-measures",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_filters": map[string]any{
				"type":        "looker-get-filters",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_parameters": map[string]any{
				"type":        "looker-get-parameters",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"query": map[string]any{
				"type":        "looker-query",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"query_sql": map[string]any{
				"type":        "looker-query-sql",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"query_url": map[string]any{
				"type":        "looker-query-url",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_looks": map[string]any{
				"type":        "looker-get-looks",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"make_look": map[string]any{
				"type":        "looker-make-look",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_dashboards": map[string]any{
				"type":        "looker-get-dashboards",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"run_dashboard": map[string]any{
				"type":        "looker-run-dashboard",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"import_dashboard_from_lookml": map[string]any{
				"type":        "looker-import-dashboard-from-lookml",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"make_dashboard": map[string]any{
				"type":        "looker-make-dashboard",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"add_dashboard_filter": map[string]any{
				"type":        "looker-add-dashboard-filter",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"add_dashboard_element": map[string]any{
				"type":        "looker-add-dashboard-element",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"conversational_analytics": map[string]any{
				"type":        "looker-conversational-analytics",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"health_pulse": map[string]any{
				"type":        "looker-health-pulse",
				"source":      "my-instance",
				"description": "Checks the health of a Looker instance by running a series of checks on the system.",
			},
			"health_analyze": map[string]any{
				"type":        "looker-health-analyze",
				"source":      "my-instance",
				"description": "Provides analysis of a Looker instance's projects, models, or explores.",
			},
			"health_vacuum": map[string]any{
				"type":        "looker-health-vacuum",
				"source":      "my-instance",
				"description": "Vacuums unused content from a Looker instance.",
			},
			"dev_mode": map[string]any{
				"type":        "looker-dev-mode",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_projects": map[string]any{
				"type":        "looker-get-projects",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_project_files": map[string]any{
				"type":        "looker-get-project-files",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_project_file": map[string]any{
				"type":        "looker-get-project-file",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"create_project_file": map[string]any{
				"type":        "looker-create-project-file",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"update_project_file": map[string]any{
				"type":        "looker-update-project-file",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"delete_project_file": map[string]any{
				"type":        "looker-delete-project-file",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_project_directories": map[string]any{
				"type":        "looker-get-project-directories",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"create_project_directory": map[string]any{
				"type":        "looker-create-project-directory",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"delete_project_directory": map[string]any{
				"type":        "looker-delete-project-directory",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"validate_project": map[string]any{
				"type":        "looker-validate-project",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"generate_embed_url": map[string]any{
				"type":        "looker-generate-embed-url",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_connections": map[string]any{
				"type":        "looker-get-connections",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_connection_schemas": map[string]any{
				"type":        "looker-get-connection-schemas",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_connection_databases": map[string]any{
				"type":        "looker-get-connection-databases",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_connection_tables": map[string]any{
				"type":        "looker-get-connection-tables",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_connection_table_columns": map[string]any{
				"type":        "looker-get-connection-table-columns",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_lookml_tests": map[string]any{
				"type":        "looker-get-lookml-tests",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"run_lookml_tests": map[string]any{
				"type":        "looker-run-lookml-tests",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"create_view_from_table": map[string]any{
				"type":        "looker-create-view-from-table",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"list_git_branches": map[string]any{
				"type":        "looker-list-git-branches",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_git_branch": map[string]any{
				"type":        "looker-get-git-branch",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"create_git_branch": map[string]any{
				"type":        "looker-create-git-branch",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"switch_git_branch": map[string]any{
				"type":        "looker-switch-git-branch",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"delete_git_branch": map[string]any{
				"type":        "looker-delete-git-branch",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"list_agents": map[string]any{
				"type":        "looker-list-agents",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_agent": map[string]any{
				"type":        "looker-get-agent",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"create_agent": map[string]any{
				"type":        "looker-create-agent",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"update_agent": map[string]any{
				"type":        "looker-update-agent",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"delete_agent": map[string]any{
				"type":        "looker-delete-agent",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"get_dashboard": map[string]any{
				"type":        "looker-get-dashboard",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"update_dashboard_element": map[string]any{
				"type":        "looker-update-dashboard-element",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"create_dashboard_layout": map[string]any{
				"type":        "looker-create-dashboard-layout",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
			"update_dashboard_layout_component": map[string]any{
				"type":        "looker-update-dashboard-layout-component",
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
			},
		},
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		cmd.Stop()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := cmd.Wait(shutdown); err != nil {
			t.Errorf("stop toolbox: %v", err)
		}
		cmd.Close()
	})

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

}

func runLookerCallTools(t *testing.T, options ...lookerTestOption) {
	randstr := rand.Text()[0:8]

	wantResult := "{\"connections\":[],\"label\":\"System Activity\",\"name\":\"system__activity\",\"project_name\":\"system__activity\"}"
	runLookerToolContains(t, "get_models", []byte(`{}`), wantResult, options...)

	wantResult = "{\"description\":\"Data about Look and dashboard usage, including frequency of views, favoriting, scheduling, embedding, and access via the API. Also includes details about individual Looks and dashboards.\",\"group_label\":\"System Activity\",\"hidden\":false,\"label\":\"Content Usage\",\"name\":\"content_usage\"}"
	runLookerToolContains(t, "get_explores", []byte(`{"model": "system__activity"}`), wantResult, options...)

	wantResult = "\"group_label\":\"System Activity\",\"hidden\":false,\"label\":\"Content Usage\",\"name\":\"content_usage\""
	runLookerToolContains(t, "get_explore", []byte(`{"model": "system__activity", "explore": "content_usage"}`), wantResult, options...)

	wantResult = "{\"description\":\"\",\"label\":\" Dashboard Linked Looks  ID\",\"label_short\":\" ID\",\"name\":\"_dashboard_linked_looks._id\",\"suggest_dimension\":\"_dashboard_linked_looks._id\",\"suggest_explore\":\"content_usage\",\"suggestable\":true,\"type\":\"string\"}"
	runLookerToolContains(t, "get_dimensions", []byte(`{"model": "system__activity", "explore": "content_usage"}`), wantResult, options...)

	// Verify the suggestions output structure is wrapped in a JSON Object
	wantResult = `{"suggestions":`
	runLookerToolContains(t, "get_field_value_suggestions", []byte(`{"model": "system__activity", "explore": "history", "field": "history.source"}`), wantResult, options...)

	// Verify that the suggestions list contains the expected values
	wantSuggestions := []string{"api4", "dashboard", "explore", "regenerator", "sqlrunner", "suggest"}
	testFieldValueSuggestions(t, "basic", []byte("{\"model\": \"system__activity\", \"explore\": \"history\", \"field\": \"history.source\"}"), wantSuggestions, options...)

	// Verify that search term filtering works
	wantResult = "{\"suggestions\":[\"api4\"]}"
	runLookerToolContains(t, "get_field_value_suggestions", []byte(`{"model": "system__activity", "explore": "history", "field": "history.source", "term": "ap"}`), wantResult, options...)

	// Verify that conditional filtering based on other fields works
	testFieldValueSuggestions(t, "with_filtering", []byte(`{"model": "system__activity", "explore": "history", "field": "history.source", "filters": {"history.status": "complete"}}`), wantSuggestions, options...)

	wantResult = "{\"description\":\"The total number of views via the Looker API\",\"label\":\"Content Usage API Total\",\"label_short\":\"API Total\",\"name\":\"content_usage.api_total\",\"suggestable\":false,\"type\":\"sum\"}"
	runLookerToolContains(t, "get_measures", []byte(`{"model": "system__activity", "explore": "content_usage"}`), wantResult, options...)

	wantResult = "[]"
	runLookerToolContains(t, "get_filters", []byte(`{"model": "system__activity", "explore": "content_usage"}`), wantResult, options...)

	wantResult = "[]"
	runLookerToolContains(t, "get_parameters", []byte(`{"model": "system__activity", "explore": "content_usage"}`), wantResult, options...)

	wantResult = "{\"look.count\":"
	runLookerToolContains(t, "query", []byte(`{"model": "system__activity", "explore": "look", "fields": ["look.count"]}`), wantResult, options...)
	testQueryById(t, wantResult, options...)

	wantResult = "SELECT"
	runLookerToolContains(t, "query_sql", []byte(`{"model": "system__activity", "explore": "look", "fields": ["look.count"]}`), wantResult, options...)

	wantResult = "system__activity"
	runLookerToolContains(t, "query_url", []byte(`{"model": "system__activity", "explore": "look", "fields": ["look.count"]}`), wantResult, options...)

	// A system that is just being used for testing has no looks or dashboards.
	// MCP serializes a nil []any as an empty content list.
	emptyResult := "null"
	if lookerMCPEnabled(options) {
		emptyResult = "[]"
	}
	wantResult = emptyResult
	runLookerToolContains(t, "get_looks", []byte(`{"title": "FOO", "desc": "BAR"}`), wantResult, options...)

	wantResult = emptyResult
	runLookerToolContains(t, "get_dashboards", []byte(`{"title": "FOO", "desc": "BAR"}`), wantResult, options...)

	wantResult = "the_look"
	runLookerToolContains(t, "get_projects", []byte(`{}`), wantResult, options...)

	wantResult = "order_items.view"
	runLookerToolContains(t, "get_project_files", []byte(`{"project_id": "the_look"}`), wantResult, options...)

	wantResult = "view"
	runLookerToolContains(t, "get_project_file", []byte(`{"project_id": "the_look", "file_path": "order_items.view.lkml"}`), wantResult, options...)

	wantResult = "dev"
	runLookerToolContains(t, "dev_mode", []byte(`{"devMode": true}`), wantResult, options...)

	wantResult = "created"
	runLookerToolContains(t, "create_project_file", []byte(fmt.Sprintf(`{"project_id": "the_look", "file_path": "foo%s.view.lkml", "file_content": "view"}`, randstr)), wantResult, options...)

	wantResult = "updated"
	runLookerToolContains(t, "update_project_file", []byte(fmt.Sprintf(`{"project_id": "the_look", "file_path": "foo%s.view.lkml", "file_content": "model"}`, randstr)), wantResult, options...)

	wantResult = "deleted"
	runLookerToolContains(t, "delete_project_file", []byte(fmt.Sprintf(`{"project_id": "the_look", "file_path": "foo%s.view.lkml"}`, randstr)), wantResult, options...)

	wantResult = "Created"
	runLookerToolContains(t, "create_project_directory", []byte(fmt.Sprintf(`{"project_id": "the_look", "directory_path": "views%s"}`, randstr)), wantResult, options...)

	wantResult = fmt.Sprintf("views%s", randstr)
	runLookerToolContains(t, "get_project_directories", []byte(`{"project_id": "the_look"}`), wantResult, options...)

	// Add test back when infrastructure for testing supports it.
	// wantResult = "{\"status\":  \"success\", \"message\": \"Triggered view generation for project the_look in folder views\"}"
	// runLookerToolContains(t, "create_view_from_table", []byte(`{"project_id": "the_look", "connection": "thelook", "tables": [{"schema": "demo_db", "table_name": "Employees"}]}`), wantResult, options...)

	wantResult = "Deleted"
	runLookerToolContains(t, "delete_project_directory", []byte(fmt.Sprintf(`{"project_id": "the_look", "directory_path": "views%s"}`, randstr)), wantResult, options...)

	wantResult = "\"errors\":[]"
	runLookerToolContains(t, "validate_project", []byte(`{"project_id": "the_look"}`), wantResult, options...)

	wantResult = "master"
	runLookerToolContains(t, "list_git_branches", []byte(`{"project_id": "the_look"}`), wantResult, options...)

	wantResult = fmt.Sprintf("test_branch_%s", randstr)
	runLookerToolContains(t, "create_git_branch", []byte(fmt.Sprintf(`{"project_id": "the_look", "branch": "test_branch_%s"}`, randstr)), wantResult, options...)

	time.Sleep(5 * time.Second)
	wantResult = "d2d4eafdf8932837b2a12b773282c165a43fb0c0"
	runLookerToolContains(t, "switch_git_branch", []byte(fmt.Sprintf(`{"project_id": "the_look", "branch": "test_branch_%s", "ref": "d2d4eafdf8932837b2a12b773282c165a43fb0c0"}`, randstr)), wantResult, options...)

	wantResult = fmt.Sprintf("test_branch_%s", randstr)
	runLookerToolContains(t, "get_git_branch", []byte(`{"project_id": "the_look"}`), wantResult, options...)

	wantResult = "dev-mike-deangelo-twqb"
	runLookerToolContains(t, "switch_git_branch", []byte(`{"project_id": "the_look", "branch": "dev-mike-deangelo-twqb"}`), wantResult, options...)

	wantResult = "Deleted"
	runLookerToolContains(t, "delete_git_branch", []byte(fmt.Sprintf(`{"project_id": "the_look", "branch": "test_branch_%s"}`, randstr)), wantResult, options...)

	wantResult = "[]"
	runLookerToolContains(t, "get_lookml_tests", []byte(`{"project_id": "the_look"}`), wantResult, options...)

	wantResult = "[]"
	runLookerToolContains(t, "run_lookml_tests", []byte(`{"project_id": "the_look"}`), wantResult, options...)

	wantResult = "production"
	runLookerToolContains(t, "dev_mode", []byte(`{"devMode": false}`), wantResult, options...)

	wantResult = "thelook"
	runLookerToolContains(t, "get_connections", []byte(`{}`), wantResult, options...)

	wantResult = "{\"name\":\"demo_db\",\"is_default\":true}"
	runLookerToolContains(t, "get_connection_schemas", []byte(`{"conn": "thelook"}`), wantResult, options...)

	wantResult = "[]"
	runLookerToolContains(t, "get_connection_databases", []byte(`{"conn": "thelook"}`), wantResult, options...)

	wantResult = "Employees"
	runLookerToolContains(t, "get_connection_tables", []byte(`{"conn": "thelook", "schema": "demo_db"}`), wantResult, options...)

	wantResult = "{\"column_name\":\"EmpID\",\"data_type_database\":\"int\",\"data_type_looker\":\"number\",\"sql_escaped_column_name\":\"EmpID\"}"
	runLookerToolContains(t, "get_connection_table_columns", []byte(`{"conn": "thelook", "schema": "demo_db", "tables": "Employees"}`), wantResult, options...)

	wantResult = "/login/embed?t=" // testing for specific substring, since url is dynamic
	runLookerToolContains(t, "generate_embed_url", []byte(`{"type": "dashboards", "id": "1"}`), wantResult, options...)

	wantResult = fmt.Sprintf("agent_%s", randstr)
	runLookerToolContains(t, "create_agent", []byte(fmt.Sprintf(`{"name": "agent_%s", "description": "test description", "instructions": "test instructions", "sources": [{"model": "the_look", "explore": "events"}]}`, randstr)), wantResult, options...)

	agentId, _ := findTestAgentId(t, fmt.Sprintf("agent_%s", randstr))

	wantResult = fmt.Sprintf("agent_%s", randstr)
	runLookerToolContains(t, "list_agents", []byte(`{}`), wantResult, options...)

	wantResult = fmt.Sprintf("agent_%s", randstr)
	runLookerToolContains(t, "get_agent", []byte(fmt.Sprintf(`{"agent_id": "%s"}`, agentId)), wantResult, options...)

	wantResult = fmt.Sprintf("agent_%s", randstr)
	runLookerToolContains(t, "update_agent", []byte(fmt.Sprintf(`{"agent_id": "%s", "sources": [{"model": "system__activity", "explore": "history"}]}`, agentId)), wantResult, options...)

	wantResult = ""
	runLookerToolContains(t, "delete_agent", []byte(fmt.Sprintf(`{"agent_id": "%s"}`, agentId)), wantResult, options...)

	runConversationalAnalytics(t, "system__activity", "content_usage", options...)

	deleteLook := testMakeLook(t, randstr, options...)
	defer deleteLook()

	dashboardId, deleteDashboard := testMakeDashboard(t, randstr, options...)
	defer deleteDashboard()
	testAddDashboardFilter(t, dashboardId, options...)
	testAddDashboardElement(t, dashboardId, options...)
	testAddDashboardSqlElement(t, dashboardId)
	testRunDashboard(t, dashboardId, options...)
	testDashboardLifecycle(t, dashboardId, options...)

	deleteImportedDashboard := testImportDashboardFromLookml(t, randstr, options...)
	defer deleteImportedDashboard()

	wantResult = "\"Connection\":\"thelook\""
	runLookerToolContains(t, "health_pulse", []byte(`{"action": "check_db_connections"}`), wantResult, options...)

	wantResult = "[]"
	runLookerToolContains(t, "health_pulse", []byte(`{"action": "check_schedule_failures"}`), wantResult, options...)

	testHealthPulseLegacyFeatures(t, options...)

	wantResult = "\"Project\":\"the_look\""
	runLookerToolContains(t, "health_analyze", []byte(`{"action": "projects"}`), wantResult, options...)

	wantResult = "\"Model\":\"the_look\""
	runLookerToolContains(t, "health_analyze", []byte(`{"action": "explores", "project": "the_look", "model": "the_look", "explore": "inventory_items"}`), wantResult, options...)

	wantResult = "\"Model\":\"the_look\""
	runLookerToolContains(t, "health_vacuum", []byte(`{"action": "models"}`), wantResult, options...)
}

// testFieldValueSuggestions asserts that get_field_value_suggestions returns at
// least the wanted values. Suggestions are drawn from live System Activity data,
// which only accumulates — running the LookML tests in this suite, for instance,
// adds "__lookml_test__" as a history source — so the exact list cannot be pinned.
func testFieldValueSuggestions(t *testing.T, name string, params []byte, want []string, options ...lookerTestOption) {
	t.Helper()
	t.Run("invoke get_field_value_suggestions/"+name, func(t *testing.T) {
		t.Helper()
		url := "http://127.0.0.1:5000/api/tool/get_field_value_suggestions/invoke"
		bodyBytes := invokeLookerTool(t, url, params, options...)

		result := string(bodyBytes)

		var got struct {
			Suggestions []string `json:"suggestions"`
		}
		if err := json.Unmarshal([]byte(result), &got); err != nil {
			t.Fatalf("error parsing result body: %v", err)
		}

		gotSuggestions := make(map[string]bool, len(got.Suggestions))
		for _, s := range got.Suggestions {
			gotSuggestions[s] = true
		}
		for _, w := range want {
			if !gotSuggestions[w] {
				t.Errorf("missing suggestion %q: got %v", w, got.Suggestions)
			}
		}
	})
}

func findTestAgentId(t *testing.T, name string) (string, error) {
	sdk := newLookerTestSDK(t)
	reqSearchAgents := v4.RequestSearchAgents{
		Name: &name,
	}
	agents, err := sdk.SearchAgents(reqSearchAgents, nil)
	if len(agents) == 0 {
		t.Fatalf("Failed to find agent %s", name)
	} else if len(agents) > 1 {
		t.Fatalf("Found more than one agent with name %s", name)
	}
	agentId := *agents[0].Id
	t.Logf("Found Agent Id %s", agentId)
	return agentId, err
}

func runConversationalAnalytics(t *testing.T, modelName, exploreName string, options ...lookerTestOption) {
	exploreRefsJSON := fmt.Sprintf(`[{"model":"%s","explore":"%s"}]`, modelName, exploreName)

	var refs []map[string]any
	if err := json.Unmarshal([]byte(exploreRefsJSON), &refs); err != nil {
		t.Fatalf("failed to unmarshal explore refs: %v", err)
	}

	testCases := []struct {
		name           string
		exploreRefs    []map[string]any
		wantStatusCode int
		wantInResult   string
		wantInError    string
	}{
		{
			name:           "invoke conversational analytics with explore",
			exploreRefs:    refs,
			wantStatusCode: http.StatusOK,
			wantInResult:   `Answer`,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			requestBodyMap := map[string]any{
				"user_query_with_context": "What is in the explore?",
				"explore_references":      tc.exploreRefs,
			}
			bodyBytes, err := json.Marshal(requestBodyMap)
			if err != nil {
				t.Fatalf("failed to marshal request body: %v", err)
			}
			url := "http://127.0.0.1:5000/api/tool/conversational_analytics/invoke"
			bodyBytes = invokeLookerTool(t, url, bodyBytes, options...)

			if tc.wantInResult != "" {
				got := string(bodyBytes)
				if !strings.Contains(got, tc.wantInResult) {
					t.Errorf("unexpected result: got %q, want to contain %q", got, tc.wantInResult)
				}
			}

			if tc.wantInError != "" {
				if !strings.Contains(string(bodyBytes), tc.wantInError) {
					t.Errorf("unexpected error message: got %q, want to contain %q", string(bodyBytes), tc.wantInError)
				}
			}
		})
	}
}

func newLookerTestSDK(t *testing.T) *v4.LookerSDK {
	t.Helper()
	cfg := rtl.ApiSettings{
		BaseUrl:      LookerBaseUrl,
		ApiVersion:   "4.0",
		VerifySsl:    LookerVerifySsl == "true",
		Timeout:      120,
		ClientId:     LookerClientId,
		ClientSecret: LookerClientSecret,
	}
	return v4.NewLookerSDK(rtl.NewAuthSession(cfg))
}

func testMakeLook(t *testing.T, randstr string, options ...lookerTestOption) func() {
	var id string
	t.Run("TestMakeLook", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"model": "system__activity", "explore": "look", "fields": ["look.count"], "title": "TestLook%s"}`, randstr))

		url := "http://127.0.0.1:5000/api/tool/make_look/invoke"
		bodyBytes := invokeLookerTool(t, url, reqBody, options...)

		var respBody map[string]interface{}
		result := string(bodyBytes)
		if err := json.Unmarshal([]byte(result), &respBody); err != nil {
			t.Fatalf("error parsing result body: %v", err)
		}

		var ok bool
		if id, ok = respBody["id"].(string); !ok || id == "" {
			t.Fatalf("didn't get TestLook id, got %s", string(bodyBytes))
		}
	})

	return func() {
		sdk := newLookerTestSDK(t)

		if _, err := sdk.DeleteLook(id, nil); err != nil {
			t.Fatalf("error deleting look: %v", err)
		}
		t.Logf("deleted Look %s", id)
	}
}

func testAddDashboardFilter(t *testing.T, dashboardId string, options ...lookerTestOption) {
	t.Run("TestAddDashboardFilter", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s", "model": "system__activity", "explore": "look", "dimension": "look.created_year", "name": "test_filter", "title": "TestDashboardFilter"}`, dashboardId))

		url := "http://127.0.0.1:5000/api/tool/add_dashboard_filter/invoke"
		bodyBytes := invokeLookerTool(t, url, reqBody, options...)

		t.Logf("got %s", string(bodyBytes))
	})
}

func testAddDashboardElement(t *testing.T, dashboardId string, options ...lookerTestOption) {
	t.Run("TestAddDashboardElement", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s", "model": "system__activity", "explore": "look", "fields": ["look.count"], "title": "TestDashboardElement"}`, dashboardId))

		url := "http://127.0.0.1:5000/api/tool/add_dashboard_element/invoke"
		bodyBytes := invokeLookerTool(t, url, reqBody, options...)

		t.Logf("got %s", string(bodyBytes))
	})
}

func testAddDashboardSqlElement(t *testing.T, dashboardId string) {
	t.Run("TestAddDashboardSqlElement", func(t *testing.T) {
		sdk := newLookerTestSDK(t)

		connections, err := sdk.AllConnections("name", nil)
		if err != nil || len(connections) == 0 {
			t.Fatalf("Failed to retrieve connections: %v", err)
		}
		connName := *connections[0].Name

		// Create a SQL query
		sqlQuery, err := sdk.CreateSqlQuery(v4.SqlQueryCreate{
			ConnectionName: stringPtr(connName),
			Sql:            stringPtr("SELECT 100 AS test_number"),
		}, nil)
		if err != nil {
			t.Fatalf("Failed to create SQL query: %v", err)
		}

		// Create dashboard element with SQL query
		reqBody := v4.WriteDashboardElement{
			DashboardId:   stringPtr(dashboardId),
			Type:          stringPtr("vis"),
			ResultMakerId: sqlQuery.ResultMakerId,
			Title:         stringPtr("SQL Runner - Nil Query"),
		}

		_, err = sdk.CreateDashboardElement(v4.RequestCreateDashboardElement{
			Body: reqBody,
		}, nil)
		if err != nil {
			t.Fatalf("Failed to create SQL dashboard element: %v", err)
		}
		t.Log("Successfully added SQL query element to dashboard")
	})
}

func testRunDashboard(t *testing.T, dashboardId string, options ...lookerTestOption) {
	t.Run("TestRunDashboardSqlRunnerVerification", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s"}`, dashboardId))
		url := "http://127.0.0.1:5000/api/tool/run_dashboard/invoke"
		bodyBytes := invokeLookerTool(t, url, reqBody, options...)

		gotResult := string(bodyBytes)

		expectedSubstrings := []string{
			"TestDashboardElement",
			"SQL Runner - Nil Query",
			`"element_type":"sql_query"`,
			`"query_status":"success"`,
			"test_number",
		}
		for _, substr := range expectedSubstrings {
			if !strings.Contains(gotResult, substr) {
				t.Errorf("expected result to contain %q, but got:\n%s", substr, gotResult)
			}
		}
	})
}

func testMakeDashboard(t *testing.T, randstr string, options ...lookerTestOption) (string, func()) {
	var id string
	t.Run("TestMakeDashboard", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"title": "TestDashboard%s"}`, randstr))

		url := "http://127.0.0.1:5000/api/tool/make_dashboard/invoke"
		bodyBytes := invokeLookerTool(t, url, reqBody, options...)

		var respBody map[string]interface{}
		result := string(bodyBytes)
		if err := json.Unmarshal([]byte(result), &respBody); err != nil {
			t.Fatalf("error parsing result body: %v", err)
		}

		var ok bool
		if id, ok = respBody["id"].(string); !ok || id == "" {
			t.Fatalf("didn't get TestDashboard id, got %s", string(bodyBytes))
		}
	})

	return id, func() {
		sdk := newLookerTestSDK(t)

		if _, err := sdk.DeleteDashboard(id, nil); err != nil {
			t.Fatalf("error deleting dashboard: %v", err)
		}
	}
}

func stringPtr(s string) *string {
	return &s
}

func testDashboardLifecycle(t *testing.T, dashboardId string, options ...lookerTestOption) {
	t.Run("TestDashboardLifecycle", func(t *testing.T) {
		// 1. Get Dashboard
		getDashboardUrl := "http://127.0.0.1:5000/api/tool/get_dashboard/invoke"
		reqBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s"}`, dashboardId))
		bodyBytes := invokeLookerTool(t, getDashboardUrl, reqBody, options...)

		resultStr := string(bodyBytes)

		var dashboard v4.Dashboard
		if err := json.Unmarshal([]byte(resultStr), &dashboard); err != nil {
			t.Fatalf("failed to unmarshal dashboard: %v. Result string: %s", err, resultStr)
		}

		if dashboard.Id == nil || *dashboard.Id != dashboardId {
			t.Fatalf("expected dashboard ID %s, got %v", dashboardId, dashboard.Id)
		}

		if dashboard.DashboardElements == nil || len(*dashboard.DashboardElements) == 0 {
			t.Fatalf("expected at least one dashboard element")
		}

		elements := *dashboard.DashboardElements
		elementId := *elements[0].Id
		t.Logf("Found dashboard element ID: %s", elementId)

		// 2. Update Dashboard Element
		updateElementUrl := "http://127.0.0.1:5000/api/tool/update_dashboard_element/invoke"
		newTitle := "Updated Title By Integration Test"
		updateBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s", "dashboard_element_id": "%s", "title": "%s", "model": "system__activity", "explore": "look", "fields": ["look.count"]}`, dashboardId, elementId, newTitle))
		invokeLookerTool(t, updateElementUrl, updateBody, options...)

		// Verify update via get_dashboard
		bodyBytes = invokeLookerTool(t, getDashboardUrl, reqBody, options...)
		resultStr = string(bodyBytes)
		if err := json.Unmarshal([]byte(resultStr), &dashboard); err != nil {
			t.Fatalf("failed to unmarshal dashboard: %v", err)
		}
		elements = *dashboard.DashboardElements
		if elements[0].Title == nil || *elements[0].Title != newTitle {
			t.Fatalf("expected element title %q, got %q", newTitle, *elements[0].Title)
		}

		// 3. Create Dashboard Layout (Tab)
		createLayoutUrl := "http://127.0.0.1:5000/api/tool/create_dashboard_layout/invoke"
		layoutLabel := "Tab 2"
		createLayoutBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s", "label": "%s"}`, dashboardId, layoutLabel))
		bodyBytes = invokeLookerTool(t, createLayoutUrl, createLayoutBody, options...)

		resultStr = string(bodyBytes)
		var createLayoutResult map[string]any
		if err := json.Unmarshal([]byte(resultStr), &createLayoutResult); err != nil {
			t.Fatalf("failed to unmarshal layout result: %v", err)
		}
		newLayoutId, ok := createLayoutResult["id"].(string)
		if !ok || newLayoutId == "" {
			t.Fatalf("failed to get new layout ID from result: %v", createLayoutResult)
		}
		t.Logf("Created new layout ID: %s", newLayoutId)

		// 4. Move Element to new layout
		bodyBytes = invokeLookerTool(t, getDashboardUrl, reqBody, options...)
		resultStr = string(bodyBytes)
		if err := json.Unmarshal([]byte(resultStr), &dashboard); err != nil {
			t.Fatalf("failed to unmarshal dashboard: %v", err)
		}

		if dashboard.DashboardLayouts == nil || len(*dashboard.DashboardLayouts) == 0 {
			t.Fatalf("expected dashboard layouts")
		}

		var componentId string
		for _, layout := range *dashboard.DashboardLayouts {
			if layout.DashboardLayoutComponents != nil {
				for _, comp := range *layout.DashboardLayoutComponents {
					if comp.DashboardElementId != nil && *comp.DashboardElementId == elementId {
						componentId = *comp.Id
						break
					}
				}
			}
			if componentId != "" {
				break
			}
		}

		if componentId == "" {
			t.Fatalf("failed to find layout component for element %s", elementId)
		}
		t.Logf("Found layout component ID: %s", componentId)

		// Move it
		updateComponentUrl := "http://127.0.0.1:5000/api/tool/update_dashboard_layout_component/invoke"
		updateComponentBody := []byte(fmt.Sprintf(`{"dashboard_layout_component_id": "%s", "dashboard_layout_id": "%s", "row": 0, "column": 0}`, componentId, newLayoutId))
		invokeLookerTool(t, updateComponentUrl, updateComponentBody, options...)

		// Verify move
		bodyBytes = invokeLookerTool(t, getDashboardUrl, reqBody, options...)
		resultStr = string(bodyBytes)
		if err := json.Unmarshal([]byte(resultStr), &dashboard); err != nil {
			t.Fatalf("failed to unmarshal dashboard: %v", err)
		}

		foundMoved := false
		for _, layout := range *dashboard.DashboardLayouts {
			if layout.Id != nil && *layout.Id == newLayoutId {
				if layout.DashboardLayoutComponents != nil {
					for _, comp := range *layout.DashboardLayoutComponents {
						if comp.DashboardElementId != nil && *comp.DashboardElementId == elementId {
							foundMoved = true
							if comp.Row == nil || *comp.Row != 0 {
								t.Errorf("expected row 0, got %v", comp.Row)
							}
							if comp.Column == nil || *comp.Column != 0 {
								t.Errorf("expected column 0, got %v", comp.Column)
							}
							break
						}
					}
				}
			}
		}
		if !foundMoved {
			t.Fatalf("failed to find element %s in target layout %s", elementId, newLayoutId)
		}
	})
}

func testQueryById(t *testing.T, wantResult string, options ...lookerTestOption) {
	t.Helper()
	sdk := newLookerTestSDK(t)
	fields := []string{"look.count"}
	savedQuery, err := sdk.CreateQuery(v4.WriteQuery{
		Model:  "system__activity",
		View:   "look",
		Fields: &fields,
	}, "", nil)
	if err != nil {
		t.Fatalf("failed to create saved query for query_id test: %v", err)
	}
	if savedQuery.Id == nil || *savedQuery.Id == "" {
		t.Fatalf("expected non-empty saved query Id")
	}
	runLookerToolContains(t, "query", []byte(fmt.Sprintf(`{"query_id": "%s"}`, *savedQuery.Id)), wantResult, options...)
}

func testImportDashboardFromLookml(t *testing.T, randstr string, options ...lookerTestOption) func() {
	var id string
	t.Run("TestImportDashboardFromLookml", func(t *testing.T) {
		lookml := fmt.Sprintf("- dashboard: test_imported_%s\n  title: TestImportedDashboard%s\n  layout: newspaper\n", randstr, randstr)
		reqMap := map[string]any{
			"lookml": lookml,
		}
		reqBody, err := json.Marshal(reqMap)
		if err != nil {
			t.Fatalf("error marshaling request body: %v", err)
		}

		url := "http://127.0.0.1:5000/api/tool/import_dashboard_from_lookml/invoke"
		bodyBytes := invokeLookerTool(t, url, reqBody, options...)
		var respBody map[string]any
		if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
			t.Fatalf("error parsing result body: %v", err)
		}

		var ok bool
		if id, ok = respBody["id"].(string); !ok || id == "" {
			t.Fatalf("didn't get imported dashboard id, got %s", string(bodyBytes))
		}
	})

	return func() {
		if id == "" {
			return
		}
		sdk := newLookerTestSDK(t)

		if _, err := sdk.DeleteDashboard(id, nil); err != nil {
			t.Fatalf("error deleting imported dashboard: %v", err)
		}
	}
}

// testHealthPulseLegacyFeatures accepts enabled features added by Looker upgrades,
// while rejecting malformed results and the SDK-error sentinel.
func testHealthPulseLegacyFeatures(t *testing.T, options ...lookerTestOption) {
	t.Helper()
	t.Run("invoke health_pulse/check_legacy_features", func(t *testing.T) {
		result := invokeLookerTool(t, "http://127.0.0.1:5000/api/tool/health_pulse/invoke", []byte(`{"action":"check_legacy_features"}`), options...)
		if err := validateLookerLegacyFeatures(result); err != nil {
			t.Fatal(err)
		}
	})
}

func validateLookerLegacyFeatures(result []byte) error {
	var features []map[string]string
	if err := json.Unmarshal(result, &features); err != nil {
		return fmt.Errorf("error parsing legacy features: %w", err)
	}
	if features == nil {
		return fmt.Errorf("expected JSON array for legacy features, got: %s", result)
	}
	for _, f := range features {
		if f["Feature"] == "" {
			return fmt.Errorf("expected non-empty Feature name in entry: %v", f)
		}
		if f["Feature"] == "Unable to pull legacy features due to SDK error" {
			return fmt.Errorf("unexpected SDK error checking legacy features: %s", result)
		}
	}
	return nil
}
