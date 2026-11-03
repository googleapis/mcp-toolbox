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

package bigquerygettableinfo

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/server"
	bigqueryds "github.com/googleapis/mcp-toolbox/internal/sources/bigquery"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/api/option"
)

func TestParseFromYamlBigQueryGetTableInfo(t *testing.T) {
	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	tcs := []struct {
		desc string
		in   string
		want server.ToolConfigs
	}{
		{
			desc: "basic example",
			in: `
            kind: tool
            name: example_tool
            type: bigquery-get-table-info
            source: my-instance
            description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:   "bigquery-get-table-info",
					Source: "my-instance",
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			// Parse contents
			_, _, _, got, _, _, _, _, err := server.UnmarshalPrimitiveConfig(ctx, testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse: diff %v", diff)
			}
		})
	}
}

func TestToCompactTableMetadata(t *testing.T) {
	if got := toCompactTableMetadata(nil); got != nil {
		t.Fatalf("expected nil for nil input, got %+v", got)
	}

	md := &bigqueryapi.TableMetadata{
		Name:        "",
		Location:    "US",
		Description: "",
		FullID:      "my-proj:my_ds.my_tbl",
		NumRows:     42,
		Schema: bigqueryapi.Schema{
			// Minimal leaf field with all optional attributes at zero values,
			// plus non-nil empty PolicyTags and RangeElementType pointers.
			{
				Name:             "id",
				Type:             bigqueryapi.IntegerFieldType,
				PolicyTags:       &bigqueryapi.PolicyTagList{Names: []string{}},
				RangeElementType: &bigqueryapi.RangeElementType{Type: ""},
			},
			// Nil FieldSchema entry should be safely skipped without panicking.
			nil,
			// Populated field exercising all non-zero FieldSchema attributes and nested Schema.
			{
				Name:                   "details",
				Description:            "nested record",
				Repeated:               true,
				Required:               false,
				Type:                   bigqueryapi.RecordFieldType,
				PolicyTags:             &bigqueryapi.PolicyTagList{Names: []string{"projects/p/locations/us/taxonomies/1/policyTags/2"}},
				MaxLength:              100,
				Precision:              10,
				Scale:                  2,
				DefaultValueExpression: "'default'",
				Collation:              "und:ci",
				RangeElementType:       &bigqueryapi.RangeElementType{Type: bigqueryapi.DateFieldType},
				RoundingMode:           bigqueryapi.RoundHalfEven,
				TimestampPrecision:     12,
				Schema: bigqueryapi.Schema{
					{
						Name:     "sub_field",
						Type:     bigqueryapi.StringFieldType,
						Required: true,
					},
				},
			},
		},
	}

	compact := toCompactTableMetadata(md)
	rawBytes, err := json.Marshal(compact)
	if err != nil {
		t.Fatalf("failed to marshal compactTableMetadata: %v", err)
	}
	jsonStr := string(rawBytes)

	// Verify exact top-level prefix expected by integration tests.
	wantPrefix := `{"Name":"","Location":"US","Description":"","Schema":[{"Name":"id","Type":"INTEGER"},`
	if !strings.HasPrefix(jsonStr, wantPrefix) {
		t.Errorf("marshaled JSON does not start with expected prefix.\ngot:  %s\nwant prefix: %s", jsonStr, wantPrefix)
	}

	// Verify the minimal "id" field has only Name and Type and omits all 13 zero-valued attributes.
	var parsed map[string]any
	if err := json.Unmarshal(rawBytes, &parsed); err != nil {
		t.Fatalf("failed to unmarshal JSON: %v", err)
	}
	if parsed["FullID"] != "my-proj:my_ds.my_tbl" {
		t.Errorf("expected FullID to be preserved, got %v", parsed["FullID"])
	}
	if parsed["NumRows"] != float64(42) {
		t.Errorf("expected NumRows=42 to be preserved, got %v", parsed["NumRows"])
	}

	schemaSlice, ok := parsed["Schema"].([]any)
	if !ok || len(schemaSlice) != 2 {
		t.Fatalf("expected 2 schema elements, got %v", parsed["Schema"])
	}

	idField, ok := schemaSlice[0].(map[string]any)
	if !ok {
		t.Fatalf("expected map for schema[0], got %T", schemaSlice[0])
	}
	wantIDField := map[string]any{
		"Name": "id",
		"Type": "INTEGER",
	}
	if diff := cmp.Diff(wantIDField, idField); diff != "" {
		t.Errorf("schema[0] mismatch (-want +got):\n%s", diff)
	}

	detailsField, ok := schemaSlice[1].(map[string]any)
	if !ok {
		t.Fatalf("expected map for schema[1], got %T", schemaSlice[1])
	}
	wantDetailsField := map[string]any{
		"Name":                   "details",
		"Description":            "nested record",
		"Repeated":               true,
		"Type":                   "RECORD",
		"PolicyTags":             map[string]any{"Names": []any{"projects/p/locations/us/taxonomies/1/policyTags/2"}},
		"MaxLength":              float64(100),
		"Precision":              float64(10),
		"Scale":                  float64(2),
		"DefaultValueExpression": "'default'",
		"Collation":              "und:ci",
		"RangeElementType":       map[string]any{"Type": "DATE"},
		"RoundingMode":           "ROUND_HALF_EVEN",
		"TimestampPrecision":     float64(12),
		"Schema": []any{
			map[string]any{
				"Name":     "sub_field",
				"Type":     "STRING",
				"Required": true,
			},
		},
	}
	if diff := cmp.Diff(wantDetailsField, detailsField); diff != "" {
		t.Errorf("schema[1] mismatch (-want +got):\n%s", diff)
	}
}

func TestInvokeBigQueryGetTableInfo(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/projects/test-project/datasets/test_ds/tables/test_tbl") {
			resp := map[string]any{
				"kind":     "bigquery#table",
				"id":       "test-project:test_ds.test_tbl",
				"location": "US",
				"type":     "TABLE",
				"schema": map[string]any{
					"fields": []map[string]any{
						{
							"name": "id",
							"type": "INTEGER",
							"mode": "NULLABLE",
						},
						{
							"name":        "tags",
							"type":        "STRING",
							"mode":        "REPEATED",
							"description": "tag list",
						},
					},
				},
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer mockServer.Close()

	ctx, err := testutils.ContextWithNewLogger()
	if err != nil {
		t.Fatalf("failed to create context: %v", err)
	}

	bqClient, err := bigqueryapi.NewClient(ctx, "test-project", option.WithEndpoint(mockServer.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("failed to create BigQuery client: %v", err)
	}
	defer bqClient.Close()

	src := &bigqueryds.Source{
		Config: bigqueryds.Config{
			Project: "test-project",
		},
		Client: bqClient,
	}

	cfg := Config{
		ConfigBase: tools.ConfigBase{
			Name:        "get_table_info",
			Description: "Get table info",
		},
		Type:   "bigquery-get-table-info",
		Source: "my-bq-source",
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}

	params, err := tool.GetParameters(src)
	if err != nil {
		t.Fatalf("failed to get parameters: %v", err)
	}
	paramVals, err := parameters.ParseParams(params, map[string]any{
		"project": "test-project",
		"dataset": "test_ds",
		"table":   "test_tbl",
	}, nil)
	if err != nil {
		t.Fatalf("failed to parse parameters: %v", err)
	}

	res, tbErr := tool.Invoke(ctx, src, paramVals, "")
	if tbErr != nil {
		t.Fatalf("unexpected error invoking tool: %v", tbErr)
	}

	resBytes, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("failed to marshal Invoke result: %v", err)
	}

	wantSub := `"Schema":[{"Name":"id","Type":"INTEGER"},{"Name":"tags","Description":"tag list","Repeated":true,"Type":"STRING"}]`
	if !strings.Contains(string(resBytes), wantSub) {
		t.Errorf("expected marshaled result to contain %s, got %s", wantSub, string(resBytes))
	}
}
