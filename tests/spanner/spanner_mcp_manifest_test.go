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

package spanner

import (
	"github.com/googleapis/mcp-toolbox/tests"
)

// sqlParamSchema is the input schema shared by the spanner-execute-sql tools.
func sqlParamSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{"sql": map[string]any{"type": "string", "description": "The sql to execute."}},
		"required":   []any{"sql"},
	}
}

// noParamSchema is the input schema of a spanner-sql tool that takes no parameters.
func noParamSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{},
	}
}

// searchCatalogSchema is the input schema shared by the spanner-search-catalog tools.
func searchCatalogSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"prompt": map[string]any{"type": "string", "description": "Prompt representing search intention. Do not rewrite the prompt."},
			"databaseIds": map[string]any{
				"type": "array", "description": "Array of database IDs.", "default": []any{},
				"items": map[string]any{"type": "string", "description": "The IDs of the spanner database."},
			},
			"projectIds": map[string]any{
				"type": "array", "description": "Array of project IDs.", "default": []any{},
				"items": map[string]any{"type": "string", "description": "The IDs of the GCP project."},
			},
			"types": map[string]any{
				"type": "array", "description": "Array of data types to filter by.", "default": []any{},
				"items": map[string]any{"type": "string", "description": "The type of the data. Accepted values are: SERVICE, DATABASE, TABLE, VIEW."},
			},
			// Numbers in the served schema arrive as float64 once decoded.
			"pageSize": map[string]any{"type": "integer", "description": "Number of results in the search page.", "default": float64(5)},
		},
		"required": []any{"prompt"},
	}
}

// getSpannerMCPExpectedTools returns the manifest tools/list serves for the
// GoogleSQL test config: the shared base tools, the subset of the shared
// template parameter tools this source registers, and the Spanner-specific
// tools.
func getSpannerMCPExpectedTools() []tests.MCPToolManifest {
	expected := tests.GetBaseMCPExpectedTools()

	// Spanner registers a third, read-only execute-sql tool alongside the two
	// the shared helper describes.
	expected = append(expected,
		tests.MCPToolManifest{Name: "my-exec-sql-tool", Description: "Tool to execute sql", InputSchema: sqlParamSchema()},
		tests.MCPToolManifest{Name: "my-exec-sql-tool-read-only", Description: "Tool to execute sql", InputSchema: sqlParamSchema()},
		tests.MCPToolManifest{Name: "my-auth-exec-sql-tool", Description: "Tool to execute sql", InputSchema: sqlParamSchema()},
	)

	// addTemplateParamConfig in this package registers five of the seven shared
	// template parameter tools; it has no DDL tools, which is also why the
	// legacy test passes DisableDdlTest.
	for _, tool := range tests.GetTemplateParamMCPExpectedTools() {
		switch tool.Name {
		case "create-table-templateParams-tool", "drop-table-templateParams-tool":
			continue
		}
		expected = append(expected, tool)
	}

	expected = append(expected,
		tests.MCPToolManifest{Name: "access-schema", Description: "Tool to access information schema.", InputSchema: noParamSchema()},
		tests.MCPToolManifest{Name: "access-schema-read-only", Description: "Tool to access information schema in read-only mode.", InputSchema: noParamSchema()},
		tests.MCPToolManifest{
			Name:        "list-tables-tool",
			Description: "Lists tables with their schema information",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"table_names":   map[string]any{"type": "string", "default": "", "description": "Optional: A comma-separated list of table names. If empty, details for all tables in user-accessible schemas will be listed."},
					"output_format": map[string]any{"type": "string", "default": "detailed", "description": "Optional: Use 'simple' to return table names only or use 'detailed' to return the full information schema."},
				},
				"required": []any{},
			},
		},
		tests.MCPToolManifest{
			Name:        "list-graphs-tool",
			Description: "Lists graphs with their schema information",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"graph_names":   map[string]any{"type": "string", "default": "", "description": "Optional: A comma-separated list of graph names. If empty, details for all graphs in user-accessible schemas will be listed."},
					"output_format": map[string]any{"type": "string", "default": "detailed", "description": "Optional: Use 'simple' to return graph names only or use 'detailed' to return the full information schema."},
				},
				"required": []any{},
			},
		},
		tests.MCPToolManifest{Name: "my-search-catalog-tool", Description: "Searches for data assets in catalog", InputSchema: searchCatalogSchema()},
		tests.MCPToolManifest{Name: "my-auth-search-catalog-tool", Description: "Searches for data assets in catalog", InputSchema: searchCatalogSchema()},
		tests.MCPToolManifest{Name: "my-client-auth-search-catalog-tool", Description: "Searches for data assets in catalog", InputSchema: searchCatalogSchema()},
		tests.MCPToolManifest{
			Name:        "insert_docs",
			Description: "Stores content and its vector embedding into the documents table.",
			InputSchema: map[string]any{
				"type": "object",
				// text_to_embed is generated by the embedding model, so it is
				// not part of the served schema.
				"properties": map[string]any{"content": map[string]any{"type": "string", "description": "The text content associated with the vector."}},
				"required":   []any{"content"},
			},
		},
		tests.MCPToolManifest{
			Name:        "search_docs",
			Description: "Finds the most semantically similar document to the query vector.",
			InputSchema: map[string]any{
				"type":       "object",
				"properties": map[string]any{"query": map[string]any{"type": "string", "description": "The text content to search for."}},
				"required":   []any{"query"},
			},
		},
	)

	return expected
}

// keepSpannerTools filters the GoogleSQL manifest down to the named tools. The
// PostgreSQL and Omni configs register subsets of the same tool types, so their
// expectations are derived from the one set rather than restated.
func keepSpannerTools(names ...string) []tests.MCPToolManifest {
	keep := make(map[string]bool, len(names))
	for _, n := range names {
		keep[n] = true
	}
	var out []tests.MCPToolManifest
	for _, tool := range getSpannerMCPExpectedTools() {
		if keep[tool.Name] {
			out = append(out, tool)
		}
	}
	return out
}
