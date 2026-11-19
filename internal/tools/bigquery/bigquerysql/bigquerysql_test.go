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

package bigquerysql

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	bigqueryapi "cloud.google.com/go/bigquery"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/log"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	bqutil "github.com/googleapis/mcp-toolbox/internal/tools/bigquery/bigquerycommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	bigqueryrestapi "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

func TestParseFromYamlBigQuery(t *testing.T) {
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
            type: bigquery-sql
            source: my-instance
            description: some description
            statement: |
                SELECT * FROM SQL_STATEMENT;
            parameters:
                - name: country
                  type: string
                  description: some description
            `,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:      "bigquery-sql",
					Source:    "my-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description"),
					},
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

func TestParseFromYamlWithTemplateBigQuery(t *testing.T) {
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
            type: bigquery-sql
            source: my-instance
            description: some description
            statement: |
                SELECT * FROM SQL_STATEMENT;
            parameters:
                - name: country
                  type: string
                  description: some description
            templateParameters:
                - name: tableName
                  type: string
                  description: The table to select hotels from.
                - name: fieldArray
                  type: array
                  description: The columns to return for the query.
                  items:
                        name: column
                        type: string
                        description: A column name that will be returned from the query.
            `,
			want: server.ToolConfigs{
				"example_tool": Config{
					ConfigBase: tools.ConfigBase{
						Name:         "example_tool",
						Description:  "some description",
						AuthRequired: []string{},
					},
					Type:      "bigquery-sql",
					Source:    "my-instance",
					Statement: "SELECT * FROM SQL_STATEMENT;\n",
					Parameters: []parameters.Parameter{
						parameters.NewStringParameter("country", "some description"),
					},
					TemplateParameters: []parameters.Parameter{
						parameters.NewStringParameter("tableName", "The table to select hotels from."),
						parameters.NewArrayParameter("fieldArray", "The columns to return for the query.", parameters.NewStringParameter("column", "A column name that will be returned from the query.")),
					},
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
func TestBuildQueryParameters(t *testing.T) {
	required := false
	paramsMetadata := parameters.Parameters{
		&parameters.StringParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_string",
				Type:     parameters.TypeString,
				Required: &required,
			},
		},
		&parameters.IntParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_int",
				Type:     parameters.TypeInt,
				Required: &required,
			},
		},
		&parameters.FloatParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_float",
				Type:     parameters.TypeFloat,
				Required: &required,
			},
		},
		&parameters.BooleanParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_bool",
				Type:     parameters.TypeBool,
				Required: &required,
			},
		},
		&parameters.ArrayParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_array",
				Type:     parameters.TypeArray,
				Required: &required,
			},
			Items: parameters.NewStringParameter("item", ""),
		},
	}

	paramsMap := map[string]any{
		// All are omitted
	}
	statement := "SELECT @opt_string, @opt_int, @opt_float, @opt_bool, @opt_array"

	gotHigh, gotLow, err := buildQueryParameters(paramsMetadata, paramsMap, statement)
	if err != nil {
		t.Fatalf("buildQueryParameters failed: %v", err)
	}

	wantHigh := []bigqueryapi.QueryParameter{
		{Name: "opt_string", Value: bigqueryapi.NullString{Valid: false}},
		{Name: "opt_int", Value: bigqueryapi.NullInt64{Valid: false}},
		{Name: "opt_float", Value: bigqueryapi.NullFloat64{Valid: false}},
		{Name: "opt_bool", Value: bigqueryapi.NullBool{Valid: false}},
		{Name: "opt_array", Value: []string(nil)},
	}

	if diff := cmp.Diff(wantHigh, gotHigh); diff != "" {
		t.Errorf("High-level parameters mismatch (-want +got):\n%s", diff)
	}

	// For low-level, we check the NullFields slice
	for i, p := range gotLow {
		foundNull := false
		for _, field := range p.ParameterValue.NullFields {
			if field == "Value" {
				foundNull = true
				break
			}
		}
		if !foundNull {
			t.Errorf("Low-level parameter %d (%s) NullFields does not contain 'Value', want true", i, p.Name)
		}
	}

	// Verify one non-null case
	paramsMapFull := map[string]any{
		"opt_string": "hello",
	}
	gotHighFull, gotLowFull, _ := buildQueryParameters(paramsMetadata, paramsMapFull, statement)

	if gotHighFull[0].Value != "hello" {
		t.Errorf("Expected string value 'hello', got %v", gotHighFull[0].Value)
	}
	if len(gotLowFull[0].ParameterValue.NullFields) > 0 {
		t.Error("Expected low-level NullFields to be empty for non-null value")
	}
	if gotLowFull[0].ParameterValue.Value != "hello" {
		t.Errorf("Expected low-level string value 'hello', got %s", gotLowFull[0].ParameterValue.Value)
	}
}

func TestBuildQueryParameters_Types(t *testing.T) {
	// Mixed cases
	required := false
	paramsMetadata := parameters.Parameters{
		&parameters.StringParameter{CommonParameter: parameters.CommonParameter{Name: "s", Type: "string", Required: &required}},
		&parameters.IntParameter{CommonParameter: parameters.CommonParameter{Name: "i", Type: "integer", Required: &required}},
	}
	paramsMap := map[string]any{
		"s": "val",
		// i is omitted
	}
	statement := "SELECT @s, @i"

	gotHigh, gotLow, _ := buildQueryParameters(paramsMetadata, paramsMap, statement)

	expectedHigh := []bigqueryapi.QueryParameter{
		{Name: "s", Value: "val"},
		{Name: "i", Value: bigqueryapi.NullInt64{Valid: false}},
	}

	if diff := cmp.Diff(expectedHigh, gotHigh, cmp.AllowUnexported(bigqueryapi.NullInt64{})); diff != "" {
		t.Errorf("High-level parameters mismatch (-want +got):\n%s", diff)
	}

	if len(gotLow[0].ParameterValue.NullFields) > 0 {
		t.Error("Expected low-level NullFields to be empty for 's'")
	}
	foundNull := false
	for _, field := range gotLow[1].ParameterValue.NullFields {
		if field == "Value" {
			foundNull = true
			break
		}
	}
	if !foundNull {
		t.Error("Expected low-level NullFields to contain 'Value' for 'i'")
	}
}

func TestBuildQueryParameters_EdgeCases(t *testing.T) {
	required := false
	paramsMetadata := parameters.Parameters{
		&parameters.StringParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "user",
				Type:     parameters.TypeString,
				Required: &required,
			},
		},
		&parameters.StringParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "user_id",
				Type:     parameters.TypeString,
				Required: &required,
			},
		},
		&parameters.MapParameter{
			CommonParameter: parameters.CommonParameter{
				Name:     "opt_map",
				Type:     parameters.TypeMap,
				Required: &required,
			},
		},
	}

	paramsMap := map[string]any{
		"user_id": "123",
		// user is omitted, and opt_map is omitted
	}
	// "user" should NOT be identified as named because it's only a prefix of "user_id".
	statement := "SELECT @user_id, @opt_map"

	gotHigh, gotLow, err := buildQueryParameters(paramsMetadata, paramsMap, statement)
	if err != nil {
		t.Fatalf("buildQueryParameters failed: %v", err)
	}

	// 1. Check named parameter isolation
	// gotHigh[0] is "user"
	if gotHigh[0].Name != "" {
		t.Errorf("Expected 'user' to be positional (empty name), got %q", gotHigh[0].Name)
	}
	// gotHigh[1] is "user_id"
	if gotHigh[1].Name != "user_id" {
		t.Errorf("Expected 'user_id' to be named, got %q", gotHigh[1].Name)
	}

	// 2. Check TypeMap NULL handling
	// gotHigh[2] is "opt_map"
	if gotHigh[2].Value == nil || !reflect.ValueOf(gotHigh[2].Value).IsNil() {
		t.Errorf("Expected 'opt_map' Value to be a nil map, got %v", gotHigh[2].Value)
	}
	if gotLow[2].ParameterType.Type != "STRUCT" {
		t.Errorf("Expected low-level 'opt_map' type to be STRUCT, got %q", gotLow[2].ParameterType.Type)
	}
	foundNull := false
	for _, field := range gotLow[2].ParameterValue.NullFields {
		if field == "Value" {
			foundNull = true
			break
		}
	}
	if !foundNull {
		t.Error("Expected low-level 'opt_map' NullFields to contain 'Value'")
	}
}

func TestInvokeLogsDebugQuery(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"code":403,"message":"Access Denied"}}`, http.StatusForbidden)
	}))
	defer mockServer.Close()

	var outBuf, errBuf bytes.Buffer
	logger, err := log.NewStdLogger(&outBuf, &errBuf, "debug")
	if err != nil {
		t.Fatalf("failed to create debug logger: %v", err)
	}
	ctx := util.WithLogger(context.Background(), logger)

	bqClient, err := bigqueryapi.NewClient(ctx, "test-project", option.WithEndpoint(mockServer.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("failed to create mocked BigQuery client: %v", err)
	}
	defer bqClient.Close()

	restService, err := bigqueryrestapi.NewService(ctx, option.WithEndpoint(mockServer.URL), option.WithoutAuthentication())
	if err != nil {
		t.Fatalf("failed to create mocked BigQuery REST service: %v", err)
	}

	src := &bqutil.MockSource{Client: bqClient, Service: restService}
	cfg := Config{
		ConfigBase: tools.ConfigBase{Name: "my_bq_sql_tool", Description: "Run custom BigQuery SQL"},
		Type:       "bigquery-sql",
		Source:     "my-bq-source",
		Statement:  "SELECT * FROM `my_dataset.my_table` WHERE id = @id",
		Parameters: parameters.Parameters{parameters.NewIntParameter("id", "row id")},
	}
	tool, err := cfg.Initialize(ctx)
	if err != nil {
		t.Fatalf("failed to initialize tool: %v", err)
	}
	paramVals, err := parameters.ParseParams(cfg.Parameters, map[string]any{"id": 42}, nil)
	if err != nil {
		t.Fatalf("failed to parse parameters: %v", err)
	}

	_, tbErr := tool.Invoke(ctx, src, paramVals, "")
	if tbErr == nil {
		t.Fatal("expected error when dry run returns 403 Forbidden, got nil")
	}
	wantLog := "executing `bigquery-sql` tool query: SELECT * FROM `my_dataset.my_table` WHERE id = @id"
	if !strings.Contains(outBuf.String(), wantLog) {
		t.Errorf("expected debug log output to contain %q, got %q", wantLog, outBuf.String())
	}
}
