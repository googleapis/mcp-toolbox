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
	"bytes"
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

func TestLooker(t *testing.T) {
	sourceConfig := getLookerVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	testLogger, err := log.NewStdLogger(os.Stdout, os.Stderr, "info")
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	ctx = util.WithLogger(ctx, testLogger)

	args := []string{"--enable-api"}

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
	defer cleanup()

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	tests.RunToolGetTestByName(t, "get_models",
		map[string]any{
			"get_models": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_explores",
		map[string]any{
			"get_explores": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explores.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_dimensions",
		map[string]any{
			"get_dimensions": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore containing the fields.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_field_value_suggestions",
		map[string]any{
			"get_field_value_suggestions": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore containing the fields.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The name of the field to get suggestions for.",
						"name":         "field",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Optional search term pattern.",
						"name":         "term",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"default":              map[string]any{},
						"description":          "Optional filters to enable conditional suggestions (restricting suggestions based on other field values).",
						"name":                 "filters",
						"required":             false,
						"type":                 "object",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_measures",
		map[string]any{
			"get_measures": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore containing the fields.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_parameters",
		map[string]any{
			"get_parameters": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore containing the fields.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_filters",
		map[string]any{
			"get_filters": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore containing the fields.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "query",
		map[string]any{
			"query": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore to be queried.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The fields to be retrieved.",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be returned in the query",
							"name":         "field",
							"required":     true,
							"type":         "string",
						},
						"name":     "fields",
						"required": true,
						"type":     "array",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The filters for the query. Keys are fully-qualified field names (e.g. \"view.field\") and values are filter expressions or parameter values. Pass values bare — do not wrap them in extra quote characters. For LookML `parameter` fields, use the raw allowed_value (e.g. `first_touch`), not `\"first_touch\"`. To retrieve valid filter values for a suggestible field, use the 'get_field_value_suggestions' tool.",
						"name":                 "filters",
						"required":             false,
						"default":              map[string]any{},
						"type":                 "object",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query pivots (must be included in fields as well).",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a pivot in the query",
							"name":         "pivot_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "pivots",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The sorts like \"field.id desc 0\".",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a sort in the query",
							"name":         "sort_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "sorts",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The row limit.",
						"name":         "limit",
						"required":     false,
						"type":         "integer",
						"default":      float64(500),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query timezone.",
						"name":         "tz",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "An optional filter expression string.",
						"name":         "filter_expression",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"default":      []any{},
						"description":  "An optional array of dynamic fields (table calculations, custom measures, custom dimensions).",
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"description":          "A dynamic field definition",
							"name":                 "dynamic_field",
							"required":             false,
							"type":                 "object",
						},
						"name":     "dynamic_fields",
						"required": false,
						"type":     "array",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "query_sql",
		map[string]any{
			"query_sql": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore to be queried.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The fields to be retrieved.",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be returned in the query",
							"name":         "field",
							"required":     true,
							"type":         "string",
						},
						"name":     "fields",
						"required": true,
						"type":     "array",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The filters for the query. Keys are fully-qualified field names (e.g. \"view.field\") and values are filter expressions or parameter values. Pass values bare — do not wrap them in extra quote characters. For LookML `parameter` fields, use the raw allowed_value (e.g. `first_touch`), not `\"first_touch\"`. To retrieve valid filter values for a suggestible field, use the 'get_field_value_suggestions' tool.",
						"name":                 "filters",
						"required":             false,
						"default":              map[string]any{},
						"type":                 "object",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query pivots (must be included in fields as well).",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a pivot in the query",
							"name":         "pivot_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "pivots",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The sorts like \"field.id desc 0\".",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a sort in the query",
							"name":         "sort_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "sorts",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The row limit.",
						"name":         "limit",
						"required":     false,
						"type":         "integer",
						"default":      float64(500),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query timezone.",
						"name":         "tz",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "An optional filter expression string.",
						"name":         "filter_expression",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"default":      []any{},
						"description":  "An optional array of dynamic fields (table calculations, custom measures, custom dimensions).",
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"description":          "A dynamic field definition",
							"name":                 "dynamic_field",
							"required":             false,
							"type":                 "object",
						},
						"name":     "dynamic_fields",
						"required": false,
						"type":     "array",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "query_url",
		map[string]any{
			"query_url": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore to be queried.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The fields to be retrieved.",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be returned in the query",
							"name":         "field",
							"required":     true,
							"type":         "string",
						},
						"name":     "fields",
						"required": true,
						"type":     "array",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The filters for the query. Keys are fully-qualified field names (e.g. \"view.field\") and values are filter expressions or parameter values. Pass values bare — do not wrap them in extra quote characters. For LookML `parameter` fields, use the raw allowed_value (e.g. `first_touch`), not `\"first_touch\"`. To retrieve valid filter values for a suggestible field, use the 'get_field_value_suggestions' tool.",
						"name":                 "filters",
						"required":             false,
						"default":              map[string]any{},
						"type":                 "object",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query pivots (must be included in fields as well).",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a pivot in the query",
							"name":         "pivot_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "pivots",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The sorts like \"field.id desc 0\".",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a sort in the query",
							"name":         "sort_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "sorts",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The row limit.",
						"name":         "limit",
						"required":     false,
						"type":         "integer",
						"default":      float64(500),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query timezone.",
						"name":         "tz",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "An optional filter expression string.",
						"name":         "filter_expression",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"default":      []any{},
						"description":  "An optional array of dynamic fields (table calculations, custom measures, custom dimensions).",
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"description":          "A dynamic field definition",
							"name":                 "dynamic_field",
							"required":             false,
							"type":                 "object",
						},
						"name":     "dynamic_fields",
						"required": false,
						"type":     "array",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The visualization config for the query",
						"name":                 "vis_config",
						"required":             false,
						"type":                 "object",
						"default":              map[string]any{},
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_lookml_tests",
		map[string]any{
			"get_lookml_tests": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The unique ID of the LookML project.",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Optional ID of the file to filter tests by. This must be the complete file path from the project root (e.g., 'models/my_model.model.lkml').",
						"name":         "file_id",
						"required":     false,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "run_lookml_tests",
		map[string]any{
			"run_lookml_tests": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project to run LookML tests for.",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Optional id of the file to run tests for.",
						"name":         "file_id",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Optional name of the test to run.",
						"name":         "test",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Optional name of the model to run tests for.",
						"name":         "model",
						"required":     false,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "create_view_from_table",
		map[string]any{
			"create_view_from_table": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project to create the view in.",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The database connection name.",
						"name":         "connection",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The tables to generate views for.\n\t\tEach item must be a map with:\n\t\t- schema (string, required)\n\t\t- table_name (string, required)\n\t\t- primary_key (string, optional)\n\t\t- base_view (boolean, optional)\n\t\t- columns (array of objects, optional): Each object must have 'column_name' (string).",
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"description":          "Table definition.",
							"name":                 "table",
							"required":             true,
							"type":                 "object",
						},
						"name":     "tables",
						"required": true,
						"type":     "array",
					},
					map[string]any{
						"authServices": []any{},
						"default":      "views",
						"description":  "The folder to place the view files in (e.g., 'views').",
						"name":         "folder_name",
						"required":     false,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_looks",
		map[string]any{
			"get_looks": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The title of the look.",
						"name":         "title",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The description of the look.",
						"name":         "desc",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The number of looks to fetch. Default 100",
						"name":         "limit",
						"required":     false,
						"type":         "integer",
						"default":      float64(100),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The number of looks to skip before fetching. Default 0",
						"name":         "offset",
						"required":     false,
						"type":         "integer",
						"default":      float64(0),
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "make_look",
		map[string]any{
			"make_look": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore to be queried.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The fields to be retrieved.",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be returned in the query",
							"name":         "field",
							"required":     true,
							"type":         "string",
						},
						"name":     "fields",
						"required": true,
						"type":     "array",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The filters for the query. Keys are fully-qualified field names (e.g. \"view.field\") and values are filter expressions or parameter values. Pass values bare — do not wrap them in extra quote characters. For LookML `parameter` fields, use the raw allowed_value (e.g. `first_touch`), not `\"first_touch\"`. To retrieve valid filter values for a suggestible field, use the 'get_field_value_suggestions' tool.",
						"name":                 "filters",
						"required":             false,
						"default":              map[string]any{},
						"type":                 "object",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query pivots (must be included in fields as well).",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a pivot in the query",
							"name":         "pivot_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "pivots",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The sorts like \"field.id desc 0\".",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a sort in the query",
							"name":         "sort_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "sorts",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The row limit.",
						"name":         "limit",
						"required":     false,
						"type":         "integer",
						"default":      float64(500),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query timezone.",
						"name":         "tz",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "An optional filter expression string.",
						"name":         "filter_expression",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"default":      []any{},
						"description":  "An optional array of dynamic fields (table calculations, custom measures, custom dimensions).",
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"description":          "A dynamic field definition",
							"name":                 "dynamic_field",
							"required":             false,
							"type":                 "object",
						},
						"name":     "dynamic_fields",
						"required": false,
						"type":     "array",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The title of the Look",
						"name":         "title",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The description of the Look",
						"name":         "description",
						"required":     false,
						"default":      "",
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The folder id where the Look will be created. Leave blank to use the user's personal folder",
						"name":         "folder",
						"required":     false,
						"default":      "",
						"type":         "string",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The visualization config for the query",
						"name":                 "vis_config",
						"required":             false,
						"type":                 "object",
						"default":              map[string]any{},
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_dashboards",
		map[string]any{
			"get_dashboards": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The title of the dashboard.",
						"name":         "title",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The description of the dashboard.",
						"name":         "desc",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The number of dashboards to fetch. Default 100",
						"name":         "limit",
						"required":     false,
						"type":         "integer",
						"default":      float64(100),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The number of dashboards to skip before fetching. Default 0",
						"name":         "offset",
						"required":     false,
						"type":         "integer",
						"default":      float64(0),
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "run_dashboard",
		map[string]any{
			"run_dashboard": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the dashboard to run.",
						"name":         "dashboard_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "make_dashboard",
		map[string]any{
			"make_dashboard": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The title of the Dashboard",
						"name":         "title",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The description of the Dashboard",
						"name":         "description",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The folder id where the Dashboard will be created. Leave blank to use the user's personal folder",
						"name":         "folder",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "add_dashboard_filter",
		map[string]any{
			"add_dashboard_filter": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the dashboard where this filter will exist",
						"name":         "dashboard_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The name of the Dashboard Filter",
						"name":         "name",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The title of the Dashboard Filter",
						"name":         "title",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The filter_type of the Dashboard Filter: date_filter, number_filter, string_filter, field_filter (default field_filter)",
						"name":         "filter_type",
						"required":     false,
						"type":         "string",
						"default":      "field_filter",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The default_value of the Dashboard Filter (optional)",
						"name":         "default_value",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The model of a field type Dashboard Filter (required if type field)",
						"name":         "model",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore of a field type Dashboard Filter (required if type field)",
						"name":         "explore",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The dimension of a field type Dashboard Filter (required if type field)",
						"name":         "dimension",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The Dashboard Filter should allow multiple values (default true)",
						"name":         "allow_multiple_values",
						"required":     false,
						"type":         "boolean",
						"default":      true,
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The Dashboard Filter is required to run dashboard (default false)",
						"name":         "required",
						"required":     false,
						"type":         "boolean",
						"default":      false,
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "add_dashboard_element",
		map[string]any{
			"add_dashboard_element": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore to be queried.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The fields to be retrieved.",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be returned in the query",
							"name":         "field",
							"required":     true,
							"type":         "string",
						},
						"name":     "fields",
						"required": true,
						"type":     "array",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The filters for the query. Keys are fully-qualified field names (e.g. \"view.field\") and values are filter expressions or parameter values. Pass values bare — do not wrap them in extra quote characters. For LookML `parameter` fields, use the raw allowed_value (e.g. `first_touch`), not `\"first_touch\"`. To retrieve valid filter values for a suggestible field, use the 'get_field_value_suggestions' tool.",
						"name":                 "filters",
						"required":             false,
						"default":              map[string]any{},
						"type":                 "object",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query pivots (must be included in fields as well).",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a pivot in the query",
							"name":         "pivot_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "pivots",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The sorts like \"field.id desc 0\".",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a sort in the query",
							"name":         "sort_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "sorts",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The row limit.",
						"name":         "limit",
						"required":     false,
						"type":         "integer",
						"default":      float64(500),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query timezone.",
						"name":         "tz",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "An optional filter expression string.",
						"name":         "filter_expression",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"default":      []any{},
						"description":  "An optional array of dynamic fields (table calculations, custom measures, custom dimensions).",
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"description":          "A dynamic field definition",
							"name":                 "dynamic_field",
							"required":             false,
							"type":                 "object",
						},
						"name":     "dynamic_fields",
						"required": false,
						"type":     "array",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the dashboard where this tile will exist",
						"name":         "dashboard_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The title of the Dashboard Element",
						"name":         "title",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The visualization config for the query",
						"name":                 "vis_config",
						"required":             false,
						"type":                 "object",
						"default":              map[string]any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  `An array of dashboard filters like [{"dashboard_filter_name": "name", "field": "view_name.field_name"}, ...]`,
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"description":          `A dashboard filter like {"dashboard_filter_name": "name", "field": "view_name.field_name"}`,
							"name":                 "dashboard_filter",
							"required":             false,
							"type":                 "object",
							"default":              map[string]any{},
						},
						"name":     "dashboard_filters",
						"required": false,
						"type":     "array",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "conversational_analytics",
		map[string]any{
			"conversational_analytics": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The user's question, potentially including conversation history and system instructions for context.",
						"name":         "user_query_with_context",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "An Array of at least one and up to 5 explore references like [{'model': 'MODEL_NAME', 'explore': 'EXPLORE_NAME'}]",
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"name":                 "explore_reference",
							"description":          "An explore reference like {'model': 'MODEL_NAME', 'explore': 'EXPLORE_NAME'}",
							"required":             true,
							"type":                 "object",
						},
						"name":     "explore_references",
						"required": true,
						"type":     "array",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "health_pulse",
		map[string]any{
			"health_pulse": map[string]any{
				"description":  "Checks the health of a Looker instance by running a series of checks on the system.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The health check to run. Can be either: `check_db_connections`, `check_dashboard_performance`,`check_dashboard_errors`,`check_explore_performance`,`check_schedule_failures`, or `check_legacy_features`",
						"name":         "action",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "health_analyze",
		map[string]any{
			"health_analyze": map[string]any{
				"description":  "Provides analysis of a Looker instance's projects, models, or explores.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The analysis to run. Can be 'projects', 'models', or 'explores'.",
						"name":         "action",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The Looker project to analyze (optional).",
						"name":         "project",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The Looker model to analyze (optional).",
						"name":         "model",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The Looker explore to analyze (optional).",
						"name":         "explore",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The timeframe in days to analyze.",
						"name":         "timeframe",
						"required":     false,
						"type":         "integer",
						"default":      float64(90),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The minimum number of queries for a model or explore to be considered used.",
						"name":         "min_queries",
						"required":     false,
						"type":         "integer",
						"default":      float64(0),
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "health_vacuum",
		map[string]any{
			"health_vacuum": map[string]any{
				"description":  "Vacuums unused content from a Looker instance.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The vacuum action to run. Can be 'models', or 'explores'.",
						"name":         "action",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The Looker project to vacuum (optional).",
						"name":         "project",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The Looker model to vacuum (optional).",
						"name":         "model",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The Looker explore to vacuum (optional).",
						"name":         "explore",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The timeframe in days to analyze.",
						"name":         "timeframe",
						"required":     false,
						"type":         "integer",
						"default":      float64(90),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The minimum number of queries for a model or explore to be considered used.",
						"name":         "min_queries",
						"required":     false,
						"type":         "integer",
						"default":      float64(1),
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "dev_mode",
		map[string]any{
			"dev_mode": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Whether to set Dev Mode.",
						"name":         "devMode",
						"required":     false,
						"type":         "boolean",
						"default":      true,
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_projects",
		map[string]any{
			"get_projects": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_project_files",
		map[string]any{
			"get_project_files": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project containing the files",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_project_file",
		map[string]any{
			"get_project_file": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project containing the files",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The path of the file within the project",
						"name":         "file_path",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "create_project_file",
		map[string]any{
			"create_project_file": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project containing the files",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The path of the file within the project",
						"name":         "file_path",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The content of the file",
						"name":         "file_content",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "update_project_file",
		map[string]any{
			"update_project_file": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project containing the files",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The path of the file within the project",
						"name":         "file_path",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The content of the file",
						"name":         "file_content",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "delete_project_file",
		map[string]any{
			"delete_project_file": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project containing the files",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The path of the file within the project",
						"name":         "file_path",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_project_directories",
		map[string]any{
			"get_project_directories": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "create_project_directory",
		map[string]any{
			"create_project_directory": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The path to create in the project",
						"name":         "directory_path",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "delete_project_directory",
		map[string]any{
			"delete_project_directory": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The path to delete in the project",
						"name":         "directory_path",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "validate_project",
		map[string]any{
			"validate_project": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the project to validate",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "generate_embed_url",
		map[string]any{
			"generate_embed_url": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Type of Looker content to embed (e.g. dashboards, looks, query-visualizations, or explores).",
						"name":         "type",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the content to embed.",
						"name":         "id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_connections",
		map[string]any{
			"get_connections": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_connection_schemas",
		map[string]any{
			"get_connection_schemas": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The connection containing the schemas.",
						"name":         "conn",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The optional database to search",
						"name":         "db",
						"required":     false,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_connection_databases",
		map[string]any{
			"get_connection_databases": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The connection containing the databases.",
						"name":         "conn",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_connection_tables",
		map[string]any{
			"get_connection_tables": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The connection containing the tables.",
						"name":         "conn",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The optional database to search",
						"name":         "db",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The schema containing the tables.",
						"name":         "schema",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_connection_table_columns",
		map[string]any{
			"get_connection_table_columns": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The connection containing the tables.",
						"name":         "conn",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The optional database to search",
						"name":         "db",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The schema containing the tables.",
						"name":         "schema",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "A comma separated list of tables containing the columns.",
						"name":         "tables",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "list_git_branches",
		map[string]any{
			"list_git_branches": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The project_id",
						"name":         "project_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "get_dashboard",
		map[string]any{
			"get_dashboard": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The id of the dashboard to retrieve.",
						"name":         "dashboard_id",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "update_dashboard_element",
		map[string]any{
			"update_dashboard_element": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore.",
						"name":         "model",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore to be queried.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The fields to be retrieved.",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be returned in the query",
							"name":         "field",
							"required":     true,
							"type":         "string",
						},
						"name":     "fields",
						"required": true,
						"type":     "array",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The filters for the query. Keys are fully-qualified field names (e.g. \"view.field\") and values are filter expressions or parameter values. Pass values bare — do not wrap them in extra quote characters. For LookML `parameter` fields, use the raw allowed_value (e.g. `first_touch`), not `\"first_touch\"`. To retrieve valid filter values for a suggestible field, use the 'get_field_value_suggestions' tool.",
						"name":                 "filters",
						"required":             false,
						"default":              map[string]any{},
						"type":                 "object",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query pivots (must be included in fields as well).",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a pivot in the query",
							"name":         "pivot_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "pivots",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The sorts like \"field.id desc 0\".",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be used as a sort in the query",
							"name":         "sort_field",
							"required":     false,
							"type":         "string",
						},
						"name":     "sorts",
						"required": false,
						"type":     "array",
						"default":  []any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The row limit.",
						"name":         "limit",
						"required":     false,
						"type":         "integer",
						"default":      float64(500),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The query timezone.",
						"name":         "tz",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "An optional filter expression string.",
						"name":         "filter_expression",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"default":      []any{},
						"description":  "An optional array of dynamic fields (table calculations, custom measures, custom dimensions).",
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"description":          "A dynamic field definition",
							"name":                 "dynamic_field",
							"required":             false,
							"type":                 "object",
						},
						"name":     "dynamic_fields",
						"required": false,
						"type":     "array",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the dashboard containing the element.",
						"name":         "dashboard_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the dashboard element to update.",
						"name":         "dashboard_element_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The new title of the element.",
						"name":         "title",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"additionalProperties": true,
						"authServices":         []any{},
						"description":          "The new visualization configuration.",
						"name":                 "vis_config",
						"required":             false,
						"type":                 "object",
						"default":              map[string]any{},
					},
					map[string]any{
						"authServices": []any{},
						"description":  `An array of dashboard filters like [{"dashboard_filter_name": "name", "field": "view_name.field_name"}, ...]`,
						"items": map[string]any{
							"additionalProperties": true,
							"authServices":         []any{},
							"description":          `A dashboard filter like {"dashboard_filter_name": "name", "field": "view_name.field_name"}`,
							"name":                 "dashboard_filter",
							"required":             false,
							"type":                 "object",
							"default":              map[string]any{},
						},
						"name":     "dashboard_filters",
						"required": false,
						"type":     "array",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "create_dashboard_layout",
		map[string]any{
			"create_dashboard_layout": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the dashboard to create the layout for.",
						"name":         "dashboard_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The label (title) of the layout/tab.",
						"name":         "label",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The type of layout (e.g. 'newspaper', 'grid'). Defaults to 'newspaper'.",
						"name":         "type",
						"required":     false,
						"type":         "string",
						"default":      "newspaper",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Whether this layout should be active.",
						"name":         "active",
						"required":     false,
						"type":         "boolean",
						"default":      false,
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "update_dashboard_layout_component",
		map[string]any{
			"update_dashboard_layout_component": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the dashboard layout component to update.",
						"name":         "dashboard_layout_component_id",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The ID of the target dashboard layout (tab) to move the component to.",
						"name":         "dashboard_layout_id",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The new row position.",
						"name":         "row",
						"required":     false,
						"type":         "integer",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The new column position.",
						"name":         "column",
						"required":     false,
						"type":         "integer",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The new width.",
						"name":         "width",
						"required":     false,
						"type":         "integer",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The new height.",
						"name":         "height",
						"required":     false,
						"type":         "integer",
					},
				},
			},
		},
	)

	randstr := rand.Text()[0:8]

	wantResult := "{\"connections\":[],\"label\":\"System Activity\",\"name\":\"system__activity\",\"project_name\":\"system__activity\"}"
	tests.RunToolInvokeSimpleTest(t, "get_models", wantResult)

	wantResult = "{\"description\":\"Data about Look and dashboard usage, including frequency of views, favoriting, scheduling, embedding, and access via the API. Also includes details about individual Looks and dashboards.\",\"group_label\":\"System Activity\",\"label\":\"Content Usage\",\"name\":\"content_usage\"}"
	tests.RunToolInvokeParametersTest(t, "get_explores", []byte(`{"model": "system__activity"}`), wantResult)

	wantResult = "{\"description\":\"\",\"label\":\" Dashboard Linked Looks  ID\",\"label_short\":\" ID\",\"name\":\"_dashboard_linked_looks._id\",\"suggest_dimension\":\"_dashboard_linked_looks._id\",\"suggest_explore\":\"content_usage\",\"suggestable\":true,\"type\":\"string\"}"
	tests.RunToolInvokeParametersTest(t, "get_dimensions", []byte(`{"model": "system__activity", "explore": "content_usage"}`), wantResult)

	// Verify the suggestions output structure is wrapped in a JSON Object
	wantResult = `{"suggestions":`
	tests.RunToolInvokeParametersTest(t, "get_field_value_suggestions", []byte(`{"model": "system__activity", "explore": "history", "field": "history.source"}`), wantResult)

	// Verify that the suggestions list contains the expected values
	wantSuggestions := []string{"api4", "dashboard", "explore", "merge_query", "regenerator", "sqlrunner", "suggest"}
	testFieldValueSuggestions(t, "basic", []byte("{\"model\": \"system__activity\", \"explore\": \"history\", \"field\": \"history.source\"}"), wantSuggestions)

	// Verify that search term filtering works
	wantResult = "{\"suggestions\":[\"api4\"]}"
	tests.RunToolInvokeParametersTest(t, "get_field_value_suggestions", []byte(`{"model": "system__activity", "explore": "history", "field": "history.source", "term": "ap"}`), wantResult)

	// Verify that conditional filtering based on other fields works
	testFieldValueSuggestions(t, "with_filtering", []byte(`{"model": "system__activity", "explore": "history", "field": "history.source", "filters": {"history.status": "complete"}}`), wantSuggestions)

	wantResult = "{\"description\":\"The total number of views via the Looker API\",\"label\":\"Content Usage API Total\",\"label_short\":\"API Total\",\"name\":\"content_usage.api_total\",\"suggestable\":false,\"type\":\"sum\"}"
	tests.RunToolInvokeParametersTest(t, "get_measures", []byte(`{"model": "system__activity", "explore": "content_usage"}`), wantResult)

	wantResult = "[]"
	tests.RunToolInvokeParametersTest(t, "get_filters", []byte(`{"model": "system__activity", "explore": "content_usage"}`), wantResult)

	wantResult = "[]"
	tests.RunToolInvokeParametersTest(t, "get_parameters", []byte(`{"model": "system__activity", "explore": "content_usage"}`), wantResult)

	wantResult = "{\"look.count\":"
	tests.RunToolInvokeParametersTest(t, "query", []byte(`{"model": "system__activity", "explore": "look", "fields": ["look.count"]}`), wantResult)

	wantResult = "SELECT"
	tests.RunToolInvokeParametersTest(t, "query_sql", []byte(`{"model": "system__activity", "explore": "look", "fields": ["look.count"]}`), wantResult)

	wantResult = "system__activity"
	tests.RunToolInvokeParametersTest(t, "query_url", []byte(`{"model": "system__activity", "explore": "look", "fields": ["look.count"]}`), wantResult)

	// A system that is just being used for testing has no looks or dashboards
	wantResult = "null"
	tests.RunToolInvokeParametersTest(t, "get_looks", []byte(`{"title": "FOO", "desc": "BAR"}`), wantResult)

	wantResult = "null"
	tests.RunToolInvokeParametersTest(t, "get_dashboards", []byte(`{"title": "FOO", "desc": "BAR"}`), wantResult)

	wantResult = "the_look"
	tests.RunToolInvokeSimpleTest(t, "get_projects", wantResult)

	wantResult = "order_items.view"
	tests.RunToolInvokeParametersTest(t, "get_project_files", []byte(`{"project_id": "the_look"}`), wantResult)

	wantResult = "view"
	tests.RunToolInvokeParametersTest(t, "get_project_file", []byte(`{"project_id": "the_look", "file_path": "order_items.view.lkml"}`), wantResult)

	wantResult = "dev"
	tests.RunToolInvokeParametersTest(t, "dev_mode", []byte(`{"devMode": true}`), wantResult)

	wantResult = "created"
	tests.RunToolInvokeParametersTest(t, "create_project_file", []byte(fmt.Sprintf(`{"project_id": "the_look", "file_path": "foo%s.view.lkml", "file_content": "view"}`, randstr)), wantResult)

	wantResult = "updated"
	tests.RunToolInvokeParametersTest(t, "update_project_file", []byte(fmt.Sprintf(`{"project_id": "the_look", "file_path": "foo%s.view.lkml", "file_content": "model"}`, randstr)), wantResult)

	wantResult = "deleted"
	tests.RunToolInvokeParametersTest(t, "delete_project_file", []byte(fmt.Sprintf(`{"project_id": "the_look", "file_path": "foo%s.view.lkml"}`, randstr)), wantResult)

	wantResult = "Created"
	tests.RunToolInvokeParametersTest(t, "create_project_directory", []byte(fmt.Sprintf(`{"project_id": "the_look", "directory_path": "views%s"}`, randstr)), wantResult)

	wantResult = fmt.Sprintf("views%s", randstr)
	tests.RunToolInvokeParametersTest(t, "get_project_directories", []byte(`{"project_id": "the_look"}`), wantResult)

	// Add test back when infrastructure for testing supports it.
	// wantResult = "{\"status\":  \"success\", \"message\": \"Triggered view generation for project the_look in folder views\"}"
	// tests.RunToolInvokeParametersTest(t, "create_view_from_table", []byte(`{"project_id": "the_look", "connection": "thelook", "tables": [{"schema": "demo_db", "table_name": "Employees"}]}`), wantResult)

	wantResult = "Deleted"
	tests.RunToolInvokeParametersTest(t, "delete_project_directory", []byte(fmt.Sprintf(`{"project_id": "the_look", "directory_path": "views%s"}`, randstr)), wantResult)

	wantResult = "\"errors\":[]"
	tests.RunToolInvokeParametersTest(t, "validate_project", []byte(`{"project_id": "the_look"}`), wantResult)

	wantResult = "master"
	tests.RunToolInvokeParametersTest(t, "list_git_branches", []byte(`{"project_id": "the_look"}`), wantResult)

	wantResult = fmt.Sprintf("test_branch_%s", randstr)
	tests.RunToolInvokeParametersTest(t, "create_git_branch", []byte(fmt.Sprintf(`{"project_id": "the_look", "branch": "test_branch_%s"}`, randstr)), wantResult)

	time.Sleep(5 * time.Second)
	wantResult = "d2d4eafdf8932837b2a12b773282c165a43fb0c0"
	tests.RunToolInvokeParametersTest(t, "switch_git_branch", []byte(fmt.Sprintf(`{"project_id": "the_look", "branch": "test_branch_%s", "ref": "d2d4eafdf8932837b2a12b773282c165a43fb0c0"}`, randstr)), wantResult)

	wantResult = fmt.Sprintf("test_branch_%s", randstr)
	tests.RunToolInvokeParametersTest(t, "get_git_branch", []byte(`{"project_id": "the_look"}`), wantResult)

	wantResult = "dev-mike-deangelo-twqb"
	tests.RunToolInvokeParametersTest(t, "switch_git_branch", []byte(`{"project_id": "the_look", "branch": "dev-mike-deangelo-twqb"}`), wantResult)

	wantResult = "Deleted"
	tests.RunToolInvokeParametersTest(t, "delete_git_branch", []byte(fmt.Sprintf(`{"project_id": "the_look", "branch": "test_branch_%s"}`, randstr)), wantResult)

	wantResult = "[]"
	tests.RunToolInvokeParametersTest(t, "get_lookml_tests", []byte(`{"project_id": "the_look"}`), wantResult)

	wantResult = "[]"
	tests.RunToolInvokeParametersTest(t, "run_lookml_tests", []byte(`{"project_id": "the_look"}`), wantResult)

	wantResult = "production"
	tests.RunToolInvokeParametersTest(t, "dev_mode", []byte(`{"devMode": false}`), wantResult)

	wantResult = "thelook"
	tests.RunToolInvokeSimpleTest(t, "get_connections", wantResult)

	wantResult = "{\"name\":\"demo_db\",\"is_default\":true}"
	tests.RunToolInvokeParametersTest(t, "get_connection_schemas", []byte(`{"conn": "thelook"}`), wantResult)

	wantResult = "[]"
	tests.RunToolInvokeParametersTest(t, "get_connection_databases", []byte(`{"conn": "thelook"}`), wantResult)

	wantResult = "Employees"
	tests.RunToolInvokeParametersTest(t, "get_connection_tables", []byte(`{"conn": "thelook", "schema": "demo_db"}`), wantResult)

	wantResult = "{\"column_name\":\"EmpID\",\"data_type_database\":\"int\",\"data_type_looker\":\"number\",\"sql_escaped_column_name\":\"EmpID\"}"
	tests.RunToolInvokeParametersTest(t, "get_connection_table_columns", []byte(`{"conn": "thelook", "schema": "demo_db", "tables": "Employees"}`), wantResult)

	wantResult = "/login/embed?t=" // testing for specific substring, since url is dynamic
	tests.RunToolInvokeParametersTest(t, "generate_embed_url", []byte(`{"type": "dashboards", "id": "1"}`), wantResult)

	wantResult = fmt.Sprintf("agent_%s", randstr)
	tests.RunToolInvokeParametersTest(t, "create_agent", []byte(fmt.Sprintf(`{"name": "agent_%s", "description": "test description", "instructions": "test instructions", "sources": [{"model": "the_look", "explore": "events"}]}`, randstr)), wantResult)

	agentId, _ := findTestAgentId(t, fmt.Sprintf("agent_%s", randstr))

	wantResult = fmt.Sprintf("agent_%s", randstr)
	tests.RunToolInvokeParametersTest(t, "list_agents", []byte(`{}`), wantResult)

	wantResult = fmt.Sprintf("agent_%s", randstr)
	tests.RunToolInvokeParametersTest(t, "get_agent", []byte(fmt.Sprintf(`{"agent_id": "%s"}`, agentId)), wantResult)

	wantResult = fmt.Sprintf("agent_%s", randstr)
	tests.RunToolInvokeParametersTest(t, "update_agent", []byte(fmt.Sprintf(`{"agent_id": "%s", "sources": [{"model": "system__activity", "explore": "history"}]}`, agentId)), wantResult)

	wantResult = ""
	tests.RunToolInvokeParametersTest(t, "delete_agent", []byte(fmt.Sprintf(`{"agent_id": "%s"}`, agentId)), wantResult)

	runConversationalAnalytics(t, "system__activity", "content_usage")

	deleteLook := testMakeLook(t, randstr)
	defer deleteLook()

	dashboardId, deleteDashboard := testMakeDashboard(t, randstr)
	defer deleteDashboard()
	testAddDashboardFilter(t, dashboardId)
	testAddDashboardElement(t, dashboardId)
	testAddDashboardSqlElement(t, dashboardId)
	testRunDashboard(t, dashboardId)
	testDashboardLifecycle(t, dashboardId)

	wantResult = "\"Connection\":\"thelook\""
	tests.RunToolInvokeParametersTest(t, "health_pulse", []byte(`{"action": "check_db_connections"}`), wantResult)

	wantResult = "[]"
	tests.RunToolInvokeParametersTest(t, "health_pulse", []byte(`{"action": "check_schedule_failures"}`), wantResult)

	wantResult = "[{\"Feature\":\"Unsupported in Looker (Google Cloud core)\"}]"
	tests.RunToolInvokeParametersTest(t, "health_pulse", []byte(`{"action": "check_legacy_features"}`), wantResult)

	wantResult = "\"Project\":\"the_look\""
	tests.RunToolInvokeParametersTest(t, "health_analyze", []byte(`{"action": "projects"}`), wantResult)

	wantResult = "\"Model\":\"the_look\""
	tests.RunToolInvokeParametersTest(t, "health_analyze", []byte(`{"action": "explores", "project": "the_look", "model": "the_look", "explore": "inventory_items"}`), wantResult)

	wantResult = "\"Model\":\"the_look\""
	tests.RunToolInvokeParametersTest(t, "health_vacuum", []byte(`{"action": "models"}`), wantResult)
}

// testFieldValueSuggestions asserts that get_field_value_suggestions returns at
// least the wanted values. Suggestions are drawn from live System Activity data,
// which only accumulates — running the LookML tests in this suite, for instance,
// adds "__lookml_test__" as a history source — so the exact list cannot be pinned.
func testFieldValueSuggestions(t *testing.T, name string, params []byte, want []string) {
	t.Helper()
	t.Run("invoke get_field_value_suggestions/"+name, func(t *testing.T) {
		t.Helper()
		url := "http://127.0.0.1:5000/api/tool/get_field_value_suggestions/invoke"
		resp, bodyBytes := tests.RunRequest(t, http.MethodPost, url, bytes.NewBuffer(params), nil)

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, http.StatusOK, string(bodyBytes))
		}

		var respBody map[string]any
		if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
			t.Fatalf("error parsing response body: %v", err)
		}

		result, ok := respBody["result"].(string)
		if !ok {
			t.Fatalf("unable to find result in response body: %s", string(bodyBytes))
		}

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

func runConversationalAnalytics(t *testing.T, modelName, exploreName string) {
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
			resp, bodyBytes := tests.RunRequest(t, http.MethodPost, url, bytes.NewBuffer(bodyBytes), nil)

			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, tc.wantStatusCode, string(bodyBytes))
			}

			if tc.wantInResult != "" {
				var respBody map[string]interface{}
				if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
					t.Fatalf("error parsing response body: %v", err)
				}
				got, ok := respBody["result"].(string)
				if !ok {
					t.Fatalf("unable to find result in response body")
				}
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

func testMakeLook(t *testing.T, randstr string) func() {
	var id string
	t.Run("TestMakeLook", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"model": "system__activity", "explore": "look", "fields": ["look.count"], "title": "TestLook%s"}`, randstr))

		url := "http://127.0.0.1:5000/api/tool/make_look/invoke"
		resp, bodyBytes := tests.RunRequest(t, http.MethodPost, url, bytes.NewBuffer(reqBody), nil)

		if resp.StatusCode != 200 {
			t.Fatalf("unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, 200, string(bodyBytes))
		}

		var respBody map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
			t.Fatalf("error parsing response body: %v", err)
		}

		result := respBody["result"].(string)
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

func testAddDashboardFilter(t *testing.T, dashboardId string) {
	t.Run("TestAddDashboardFilter", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s", "model": "system__activity", "explore": "look", "dimension": "look.created_year", "name": "test_filter", "title": "TestDashboardFilter"}`, dashboardId))

		url := "http://127.0.0.1:5000/api/tool/add_dashboard_filter/invoke"
		resp, bodyBytes := tests.RunRequest(t, http.MethodPost, url, bytes.NewBuffer(reqBody), nil)

		if resp.StatusCode != 200 {
			t.Fatalf("unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, 200, string(bodyBytes))
		}

		t.Logf("got %s", string(bodyBytes))
	})
}

func testAddDashboardElement(t *testing.T, dashboardId string) {
	t.Run("TestAddDashboardElement", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s", "model": "system__activity", "explore": "look", "fields": ["look.count"], "title": "TestDashboardElement"}`, dashboardId))

		url := "http://127.0.0.1:5000/api/tool/add_dashboard_element/invoke"
		resp, bodyBytes := tests.RunRequest(t, http.MethodPost, url, bytes.NewBuffer(reqBody), nil)

		if resp.StatusCode != 200 {
			t.Fatalf("unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, 200, string(bodyBytes))
		}

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

func testRunDashboard(t *testing.T, dashboardId string) {
	t.Run("TestRunDashboardSqlRunnerVerification", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s"}`, dashboardId))
		url := "http://127.0.0.1:5000/api/tool/run_dashboard/invoke"
		resp, bodyBytes := tests.RunRequest(t, http.MethodPost, url, bytes.NewBuffer(reqBody), nil)
		if resp.StatusCode != 200 {
			t.Fatalf("unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, 200, string(bodyBytes))
		}

		var respBody map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
			t.Fatalf("error parsing response body: %v", err)
		}

		gotResult, ok := respBody["result"].(string)
		if !ok {
			t.Fatalf("unable to find result in response body")
		}

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

func testMakeDashboard(t *testing.T, randstr string) (string, func()) {
	var id string
	t.Run("TestMakeDashboard", func(t *testing.T) {
		reqBody := []byte(fmt.Sprintf(`{"title": "TestDashboard%s"}`, randstr))

		url := "http://127.0.0.1:5000/api/tool/make_dashboard/invoke"
		resp, bodyBytes := tests.RunRequest(t, http.MethodPost, url, bytes.NewBuffer(reqBody), nil)

		if resp.StatusCode != 200 {
			t.Fatalf("unexpected status code: got %d, want %d. Body: %s", resp.StatusCode, 200, string(bodyBytes))
		}

		var respBody map[string]interface{}
		if err := json.Unmarshal(bodyBytes, &respBody); err != nil {
			t.Fatalf("error parsing response body: %v", err)
		}

		result := respBody["result"].(string)
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

func testDashboardLifecycle(t *testing.T, dashboardId string) {
	t.Run("TestDashboardLifecycle", func(t *testing.T) {
		// 1. Get Dashboard
		getDashboardUrl := "http://127.0.0.1:5000/api/tool/get_dashboard/invoke"
		reqBody := []byte(fmt.Sprintf(`{"dashboard_id": "%s"}`, dashboardId))
		resp, bodyBytes := tests.RunRequest(t, http.MethodPost, getDashboardUrl, bytes.NewBuffer(reqBody), nil)
		if resp.StatusCode != 200 {
			t.Fatalf("get_dashboard failed: %d, body: %s", resp.StatusCode, string(bodyBytes))
		}

		var respMap map[string]any
		if err := json.Unmarshal(bodyBytes, &respMap); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		resultStr, ok := respMap["result"].(string)
		if !ok {
			t.Fatalf("result field missing or not string")
		}

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
		resp, bodyBytes = tests.RunRequest(t, http.MethodPost, updateElementUrl, bytes.NewBuffer(updateBody), nil)
		if resp.StatusCode != 200 {
			t.Fatalf("update_dashboard_element failed: %d, body: %s", resp.StatusCode, string(bodyBytes))
		}

		// Verify update via get_dashboard
		resp, bodyBytes = tests.RunRequest(t, http.MethodPost, getDashboardUrl, bytes.NewBuffer(reqBody), nil)
		if resp.StatusCode != 200 {
			t.Fatalf("get_dashboard failed: %d, body: %s", resp.StatusCode, string(bodyBytes))
		}
		if err := json.Unmarshal(bodyBytes, &respMap); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		resultStr = respMap["result"].(string)
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
		resp, bodyBytes = tests.RunRequest(t, http.MethodPost, createLayoutUrl, bytes.NewBuffer(createLayoutBody), nil)
		if resp.StatusCode != 200 {
			t.Fatalf("create_dashboard_layout failed: %d, body: %s", resp.StatusCode, string(bodyBytes))
		}

		if err := json.Unmarshal(bodyBytes, &respMap); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		resultStr = respMap["result"].(string)
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
		resp, bodyBytes = tests.RunRequest(t, http.MethodPost, getDashboardUrl, bytes.NewBuffer(reqBody), nil)
		if resp.StatusCode != 200 {
			t.Fatalf("get_dashboard failed: %d, body: %s", resp.StatusCode, string(bodyBytes))
		}
		if err := json.Unmarshal(bodyBytes, &respMap); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		resultStr = respMap["result"].(string)
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
		resp, bodyBytes = tests.RunRequest(t, http.MethodPost, updateComponentUrl, bytes.NewBuffer(updateComponentBody), nil)
		if resp.StatusCode != 200 {
			t.Fatalf("update_dashboard_layout_component failed: %d, body: %s", resp.StatusCode, string(bodyBytes))
		}

		// Verify move
		resp, bodyBytes = tests.RunRequest(t, http.MethodPost, getDashboardUrl, bytes.NewBuffer(reqBody), nil)
		if resp.StatusCode != 200 {
			t.Fatalf("get_dashboard failed: %d, body: %s", resp.StatusCode, string(bodyBytes))
		}
		if err := json.Unmarshal(bodyBytes, &respMap); err != nil {
			t.Fatalf("failed to unmarshal response: %v", err)
		}
		resultStr = respMap["result"].(string)
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
