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

package looker

import (
	"testing"

	"github.com/googleapis/mcp-toolbox/tests"
)

func lookerExpectedTools(t *testing.T) []tests.MCPToolManifest {
	t.Helper()
	var expected []tests.MCPToolManifest
	add := func(name string, manifests map[string]any) {
		manifest := manifests[name].(map[string]any)
		properties := map[string]any{}
		required := []any{}
		for _, raw := range manifest["parameters"].([]any) {
			p := raw.(map[string]any)
			properties[p["name"].(string)] = lookerParameterSchema(p)
			if p["required"] == true {
				required = append(required, p["name"])
			}
		}
		expected = append(expected, tests.MCPToolManifest{Name: name, Description: manifest["description"].(string), InputSchema: map[string]any{"type": "object", "properties": properties, "required": required}})
	}
	add("get_models",
		map[string]any{
			"get_models": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	add("get_explores",
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
	add("get_explore",
		map[string]any{
			"get_explore": map[string]any{
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
						"description":  "The explore to get metadata for.",
						"name":         "explore",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	add("get_dimensions",
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
	add("get_field_value_suggestions",
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
	add("get_measures",
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
	add("get_parameters",
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
	add("get_filters",
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
	add("query",
		map[string]any{
			"query": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore (required if query_id is not provided).",
						"name":         "model",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore to be queried (required if query_id is not provided).",
						"name":         "explore",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The fields to be retrieved (required if query_id is not provided).",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be returned in the query",
							"name":         "field",
							"required":     false,
							"type":         "string",
						},
						"name":     "fields",
						"required": false,
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
						"description":  "Optional unique identifier (numeric ID or slug) of a saved Looker query to run. If provided, model, explore, and fields are not required.",
						"name":         "query_id",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"default":      "json",
						"description":  "The output format: 'json' (raw row array) or 'json_detail' (includes fields, pivots, and data). Default is 'json'.",
						"name":         "result_format",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"default":      "",
						"description":  "Optional visualization configuration JSON string to associate with the query response.",
						"name":         "vis_config",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Optional flag indicating whether to generate drill links in the query result.",
						"name":         "generate_drill_links",
						"required":     false,
						"type":         "boolean",
					},
				},
			},
		},
	)
	add("query_sql",
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
	add("query_url",
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
	add("get_lookml_tests",
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
	add("run_lookml_tests",
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
	add("create_view_from_table",
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
	add("get_looks",
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
	add("make_look",
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
	add("get_dashboards",
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
	add("run_dashboard",
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
	add("make_dashboard",
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
	add("import_dashboard_from_lookml",
		map[string]any{
			"import_dashboard_from_lookml": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The LookML YAML definition of the dashboard to import",
						"name":         "lookml",
						"required":     true,
						"type":         "string",
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
	add("add_dashboard_filter",
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
	add("add_dashboard_element",
		map[string]any{
			"add_dashboard_element": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore (required if type is 'vis' or 'data').",
						"name":         "model",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore to be queried (required if type is 'vis' or 'data').",
						"name":         "explore",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The fields to be retrieved (required if type is 'vis' or 'data').",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be returned in the query",
							"name":         "field",
							"required":     false,
							"type":         "string",
						},
						"name":     "fields",
						"required": false,
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
					map[string]any{
						"authServices": []any{},
						"description":  "The type of dashboard element: 'vis' (visualization), 'data' (data table), or 'text' (text/markdown tile).",
						"name":         "type",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The text, Markdown, HTML, or Slate JSON body content for 'text' tiles.",
						"name":         "body_text",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The title text for 'text' tiles.",
						"name":         "title_text",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The subtitle text displayed below the title.",
						"name":         "subtitle_text",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "A JSON string containing properties for rich text (Slate) elements (e.g. '{\"format\":\"slate\"}').",
						"name":         "rich_content_json",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The note text attached to the tile.",
						"name":         "note_text",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Where the note appears on the tile: 'above', 'below', or 'hover'.",
						"name":         "note_display",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The display state of the note: 'expanded' or 'collapsed'.",
						"name":         "note_state",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Whether to hide the tile title.",
						"name":         "title_hidden",
						"required":     false,
						"type":         "boolean",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The auto-refresh interval for the tile.",
						"name":         "refresh_interval",
						"required":     false,
						"type":         "string",
					},
				},
			},
		},
	)
	add("conversational_analytics",
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
	add("health_pulse",
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
	add("health_analyze",
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
	add("health_vacuum",
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
	add("dev_mode",
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
	add("get_projects",
		map[string]any{
			"get_projects": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	add("get_project_files",
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
	add("get_project_file",
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
	add("create_project_file",
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
	add("update_project_file",
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
	add("delete_project_file",
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
	add("get_project_directories",
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
	add("create_project_directory",
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
	add("delete_project_directory",
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
	add("validate_project",
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
	add("generate_embed_url",
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
	add("get_connections",
		map[string]any{
			"get_connections": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	add("get_connection_schemas",
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
	add("get_connection_databases",
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
	add("get_connection_tables",
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
	add("get_connection_table_columns",
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
	add("list_git_branches",
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
	add("get_dashboard",
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
	add("update_dashboard_element",
		map[string]any{
			"update_dashboard_element": map[string]any{
				"description":  "Simple tool to test end to end functionality.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "The model containing the explore (required if type is 'vis' or 'data').",
						"name":         "model",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The explore to be queried (required if type is 'vis' or 'data').",
						"name":         "explore",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The fields to be retrieved (required if type is 'vis' or 'data').",
						"items": map[string]any{
							"authServices": []any{},
							"description":  "A field to be returned in the query",
							"name":         "field",
							"required":     false,
							"type":         "string",
						},
						"name":     "fields",
						"required": false,
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
					map[string]any{
						"authServices": []any{},
						"description":  "The type of dashboard element: 'vis' (visualization), 'data' (data table), or 'text' (text/markdown tile).",
						"name":         "type",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The text, Markdown, HTML, or Slate JSON body content for 'text' tiles.",
						"name":         "body_text",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The title text for 'text' tiles.",
						"name":         "title_text",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The subtitle text displayed below the title.",
						"name":         "subtitle_text",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "A JSON string containing properties for rich text (Slate) elements (e.g. '{\"format\":\"slate\"}').",
						"name":         "rich_content_json",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The note text attached to the tile.",
						"name":         "note_text",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Where the note appears on the tile: 'above', 'below', or 'hover'.",
						"name":         "note_display",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The display state of the note: 'expanded' or 'collapsed'.",
						"name":         "note_state",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Whether to hide the tile title.",
						"name":         "title_hidden",
						"required":     false,
						"type":         "boolean",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "The auto-refresh interval for the tile.",
						"name":         "refresh_interval",
						"required":     false,
						"type":         "string",
					},
				},
			},
		},
	)
	add("create_dashboard_layout",
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
	add("update_dashboard_layout_component",
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

	expected = append(expected, lookerAgentAndGitTools()...)
	return expected
}
func lookerParameterSchema(p map[string]any) map[string]any {
	schema := map[string]any{}
	for _, key := range []string{"type", "description", "default", "additionalProperties"} {
		if v, ok := p[key]; ok {
			schema[key] = v
		}
	}
	if v, ok := p["items"]; ok {
		item := lookerParameterSchema(v.(map[string]any))
		// MCP exposes defaults on tool arguments, not on array item schemas.
		delete(item, "default")
		schema["items"] = item
	}
	return schema
}

func lookerAgentAndGitTools() []tests.MCPToolManifest {
	property := func(kind, description string) map[string]any {
		return map[string]any{"type": kind, "description": description}
	}
	project := property("string", "The project_id")
	agentID := property("string", "The ID of the agent.")
	agentID["default"] = ""
	agentProperties := func() map[string]any {
		result := map[string]any{
			"name":             property("string", "The name of the agent."),
			"description":      property("string", "The description of the agent."),
			"instructions":     property("string", "The instructions (system prompt) for the agent."),
			"code_interpreter": map[string]any{"type": "boolean", "description": "Optional. Enables Code Interpreter for this Agent.", "default": false},
			"sources":          map[string]any{"type": "array", "description": `Optional. A list of JSON-encoded data sources for the agent (e.g., [{"model": "my_model", "explore": "my_explore"}]).`, "items": map[string]any{"type": "object", "description": "A JSON-encoded source object with 'model' and 'explore' keys.", "additionalProperties": map[string]any{"type": "string"}}},
		}
		for _, name := range []string{"name", "description", "instructions"} {
			result[name].(map[string]any)["default"] = ""
		}
		return result
	}
	update := agentProperties()
	update["agent_id"] = agentID
	definitions := []struct {
		name       string
		properties map[string]any
		required   []any
	}{
		{"create_agent", agentProperties(), []any{}},
		{"update_agent", update, []any{}},
		{"get_agent", map[string]any{"agent_id": agentID}, []any{}},
		{"delete_agent", map[string]any{"agent_id": agentID}, []any{}},
		{"list_agents", map[string]any{}, []any{}},
		{"get_git_branch", map[string]any{"project_id": project}, []any{"project_id"}},
		{"create_git_branch", map[string]any{"project_id": project, "branch": property("string", "The git branch to create"), "ref": property("string", "The ref to use as the start of a new branch. Defaults to HEAD of current branch if not specified.")}, []any{"project_id", "branch"}},
		{"delete_git_branch", map[string]any{"project_id": project, "branch": property("string", "The git branch to delete")}, []any{"project_id", "branch"}},
		{"switch_git_branch", map[string]any{"project_id": project, "branch": property("string", "The git branch to switch to"), "ref": property("string", "The ref to switch the branch to using `reset --hard`.")}, []any{"project_id", "branch"}},
	}
	expected := make([]tests.MCPToolManifest, 0, len(definitions))
	for _, d := range definitions {
		if ref, ok := d.properties["ref"]; ok {
			ref.(map[string]any)["default"] = ""
		}
		expected = append(expected, tests.MCPToolManifest{Name: d.name, Description: "Simple tool to test end to end functionality.", InputSchema: map[string]any{"type": "object", "properties": d.properties, "required": d.required}})
	}
	return expected
}
