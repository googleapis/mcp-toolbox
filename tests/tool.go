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

package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/server/mcp/jsonrpc"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// RunToolGet runs the tool get endpoint
func RunToolGetTest(t *testing.T) {
	// Test tool get endpoint
	tcs := []struct {
		name string
		api  string
		want map[string]any
	}{
		{
			name: "get my-simple-tool",
			api:  "http://127.0.0.1:5000/api/tool/my-simple-tool/",
			want: map[string]any{
				"my-simple-tool": map[string]any{
					"description":  "Simple tool to test end to end functionality.",
					"parameters":   []any{},
					"authRequired": []any{},
				},
			},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(tc.api)
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("response status code is not 200")
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["tools"]
			if !ok {
				t.Fatalf("unable to find tools in response body")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

func RunToolGetTestByName(t *testing.T, name string, want map[string]any) {
	// Test tool get endpoint
	tcs := []struct {
		name string
		api  string
		want map[string]any
	}{
		{
			name: fmt.Sprintf("get %s", name),
			api:  fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/", name),
			want: want,
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(tc.api)
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != 200 {
				t.Fatalf("response status code is not 200")
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["tools"]
			if !ok {
				t.Fatalf("unable to find tools in response body")
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// RunToolInvokeSimpleTest runs the tool invoke endpoint with no parameters
func RunToolInvokeSimpleTest(t *testing.T, name string, simpleWant string) {
	// Test tool invoke endpoint
	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   io.Reader
		want          string
		isErr         bool
	}{
		{
			name:          fmt.Sprintf("invoke %s", name),
			api:           fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", name),
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			want:          simpleWant,
			isErr:         false,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			// Send Tool invocation request
			resp, respBody := RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			// Check response body
			var body map[string]interface{}
			err := json.Unmarshal(respBody, &body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if !strings.Contains(got, tc.want) {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

func RunToolInvokeParametersTest(t *testing.T, name string, params []byte, simpleWant string) {
	// Test tool invoke endpoint
	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   io.Reader
		want          string
		isErr         bool
	}{
		{
			name:          fmt.Sprintf("invoke %s", name),
			api:           fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", name),
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer(params),
			want:          simpleWant,
			isErr:         false,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			// Send Tool invocation request
			resp, respBody := RunRequest(t, http.MethodPost, tc.api, tc.requestBody, tc.requestHeader)
			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			// Check response body
			var body map[string]interface{}
			err := json.Unmarshal(respBody, &body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}

			got, ok := body["result"].(string)
			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if !strings.Contains(got, tc.want) {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

// RunToolInvoke runs the tool invoke endpoint
func RunToolInvokeTest(t *testing.T, select1Want string, options ...InvokeTestOption) {
	// Resolve options
	// Default values for InvokeTestConfig
	configs := &InvokeTestConfig{
		myToolId3NameAliceWant:   "[{\"id\":1,\"name\":\"Alice\"},{\"id\":3,\"name\":\"Sid\"}]",
		myToolById4Want:          "[{\"id\":4,\"name\":null}]",
		myArrayToolWant:          "[{\"id\":1,\"name\":\"Alice\"},{\"id\":3,\"name\":\"Sid\"}]",
		nullWant:                 "[]",
		supportOptionalNullParam: true,
		supportArrayParam:        true,
		supportClientAuth:        false,
		supportSelect1Want:       true,
		supportSelect1Auth:       true,
	}

	// Apply provided options
	for _, option := range options {
		option(configs)
	}

	// Get ID token
	idToken, err := GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	// Test tool invoke endpoint
	invokeTcs := []struct {
		name              string
		toolName          string
		enabled           bool
		requestHeader     map[string]string
		args              map[string]any
		wantStatusCode    int
		wantStatusCodeMCP int
		wantBody          string
		wantBodyMCP       string
		wantContentErr    string
	}{
		{
			name:              "invoke my-simple-tool",
			toolName:          "my-simple-tool",
			enabled:           configs.supportSelect1Want,
			requestHeader:     map[string]string{},
			args:              map[string]any{},
			wantBody:          select1Want,
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
		},
		{
			name:              "invoke my-tool",
			toolName:          "my-tool",
			enabled:           true,
			requestHeader:     map[string]string{},
			args:              map[string]any{"id": 3, "name": "Alice"},
			wantBody:          configs.myToolId3NameAliceWant,
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
		},
		{
			name:              "invoke my-tool-by-id with nil response",
			toolName:          "my-tool-by-id",
			enabled:           true,
			requestHeader:     map[string]string{},
			args:              map[string]any{"id": 4},
			wantBody:          configs.myToolById4Want,
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
		},
		{
			name:              "invoke my-tool-by-name with empty slice response",
			toolName:          "my-tool-by-name",
			enabled:           configs.supportOptionalNullParam,
			requestHeader:     map[string]string{},
			args:              map[string]any{},
			wantBody:          configs.nullWant,
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
		},
		{
			name:              "Invoke my-tool without parameters",
			toolName:          "my-tool",
			enabled:           true,
			requestHeader:     map[string]string{},
			args:              map[string]any{},
			wantBody:          `{"error":"parameter \"id\" is required"}`,
			wantBodyMCP:       "[]",
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
			wantContentErr:    `parameter "id" is required`,
		},
		{
			name:              "Invoke my-tool with insufficient parameters",
			toolName:          "my-tool",
			enabled:           true,
			requestHeader:     map[string]string{},
			args:              map[string]any{"id": 1},
			wantBody:          `{"error":"parameter \"name\" is required"}`,
			wantBodyMCP:       "[]",
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
			wantContentErr:    `parameter "name" is required`,
		},
		{
			name:              "invoke my-array-tool",
			toolName:          "my-array-tool",
			enabled:           configs.supportArrayParam,
			requestHeader:     map[string]string{},
			args:              map[string]any{"idArray": []any{1, 2, 3}, "nameArray": []any{"Alice", "Sid", "RandomName"}, "cmdArray": []any{"HGETALL", "row3"}},
			wantBody:          configs.myArrayToolWant,
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
		},
		{
			name:              "Invoke my-auth-tool with auth token",
			toolName:          "my-auth-tool",
			enabled:           configs.supportSelect1Auth,
			requestHeader:     map[string]string{"my-google-auth_token": idToken},
			args:              map[string]any{},
			wantBody:          configs.myAuthToolWant,
			wantBodyMCP:       "[{\"name\":\"Alice\"}]",
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
		},
		{
			name:              "Invoke my-auth-tool with invalid auth token",
			toolName:          "my-auth-tool",
			enabled:           configs.supportSelect1Auth,
			requestHeader:     map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:              map[string]any{},
			wantBody:          "",
			wantBodyMCP:       "[]",
			wantStatusCode:    http.StatusUnauthorized,
			wantStatusCodeMCP: http.StatusOK,
			wantContentErr:    "missing or invalid authentication header",
		},
		{
			name:              "Invoke my-auth-tool without auth token",
			toolName:          "my-auth-tool",
			enabled:           true,
			requestHeader:     map[string]string{},
			args:              map[string]any{},
			wantBody:          "",
			wantBodyMCP:       "[]",
			wantStatusCode:    http.StatusUnauthorized,
			wantStatusCodeMCP: http.StatusOK,
			wantContentErr:    "missing or invalid authentication header",
		},
		{
			name:              "Invoke my-auth-required-tool with auth token",
			toolName:          "my-auth-required-tool",
			enabled:           configs.supportSelect1Auth,
			requestHeader:     map[string]string{"my-google-auth_token": idToken},
			args:              map[string]any{},
			wantBody:          select1Want,
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
		},
		{
			name:              "Invoke my-auth-required-tool with invalid auth token",
			toolName:          "my-auth-required-tool",
			enabled:           true,
			requestHeader:     map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:              map[string]any{},
			wantBody:          "",
			wantStatusCode:    http.StatusUnauthorized,
			wantStatusCodeMCP: http.StatusUnauthorized,
			wantContentErr:    "invalid token",
		},
		{
			name:              "Invoke my-auth-required-tool without auth token",
			toolName:          "my-auth-tool",
			enabled:           true,
			requestHeader:     map[string]string{},
			args:              map[string]any{},
			wantBody:          "",
			wantStatusCode:    http.StatusUnauthorized,
			wantStatusCodeMCP: http.StatusOK,
			wantContentErr:    "missing or invalid authentication header",
		},
		{
			name:              "Invoke my-client-auth-tool with auth token",
			toolName:          "my-client-auth-tool",
			enabled:           configs.supportClientAuth,
			requestHeader:     map[string]string{"Authorization": accessToken},
			args:              map[string]any{},
			wantBody:          select1Want,
			wantStatusCode:    http.StatusOK,
			wantStatusCodeMCP: http.StatusOK,
		},
		{
			name:              "Invoke my-client-auth-tool without auth token",
			toolName:          "my-client-auth-tool",
			enabled:           configs.supportClientAuth,
			requestHeader:     map[string]string{},
			args:              map[string]any{},
			wantStatusCode:    http.StatusUnauthorized,
			wantStatusCodeMCP: http.StatusOK,
			wantContentErr:    "missing access token",
		},
		{
			name:              "Invoke my-client-auth-tool with invalid auth token",
			toolName:          "my-client-auth-tool",
			enabled:           configs.supportClientAuth,
			requestHeader:     map[string]string{"Authorization": "Bearer invalid-token"},
			args:              map[string]any{},
			wantStatusCode:    http.StatusUnauthorized,
			wantStatusCodeMCP: http.StatusOK,
			wantContentErr:    "invalid token",
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.enabled {
				return
			}

			if configs.isMCP {
				// Invoke the tool via MCP protocol
				mcpStatusCode, mcpResp, err := InvokeMCPTool(t, tc.toolName, tc.args, tc.requestHeader)
				if err != nil {
					t.Fatalf("native error executing %s: %s", tc.toolName, err)
				}
				wantStatus := tc.wantStatusCodeMCP
				if mcpStatusCode != wantStatus {
					t.Fatalf("expected status %d, got %d", wantStatus, mcpStatusCode)
				}
				if wantStatus != http.StatusOK {
					return
				}

				if tc.wantContentErr != "" {
					AssertMCPError(t, mcpResp, tc.wantContentErr)
					return
				}
				if mcpResp.Result.IsError {
					t.Fatalf("%s returned error result: %v", tc.toolName, mcpResp.Result)
				}
				gotObj := getMCPResultText(t, mcpResp)
				gotBytes, err := json.Marshal(gotObj)
				if err != nil {
					t.Fatalf("error marshaling result object")
				}
				wantResultBody := tc.wantBody
				if tc.wantBodyMCP != "" {
					wantResultBody = tc.wantBodyMCP
				}
				if string(gotBytes) != wantResultBody {
					var gotJSON, wantJSON any
					errGot := json.Unmarshal(gotBytes, &gotJSON)
					errWant := json.Unmarshal([]byte(wantResultBody), &wantJSON)

					if errGot == nil && errWant == nil {
						if diff := cmp.Diff(wantJSON, gotJSON); diff != "" {
							t.Fatalf("unexpected JSON value mismatch (-want +got):\n%s\nRaw got: %s\nRaw want: %s", diff, string(gotBytes), wantResultBody)
						}
					} else {
						t.Fatalf("unexpected value: got %q, want %q", string(gotBytes), wantResultBody)
					}
				}
			} else {
				// Legacy REST path
				api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
				reqBytes, _ := json.Marshal(tc.args)
				resp, respBody := RunRequest(t, http.MethodPost, api, bytes.NewBuffer(reqBytes), tc.requestHeader)

				if resp.StatusCode != tc.wantStatusCode {
					t.Errorf("StatusCode mismatch: got %d, want %d", resp.StatusCode, tc.wantStatusCode)
				}

				if tc.wantBody == "" {
					return
				}

				var body map[string]interface{}
				err := json.Unmarshal(respBody, &body)
				if err != nil {
					t.Fatalf("error parsing response body: %s", err)
				}

				got, ok := body["result"].(string)
				if !ok {
					t.Fatalf("unable to find result in response body")
				}

				// Legacy REST assertion
				if got != tc.wantBody {
					var gotJSON, wantJSON any
					errGot := json.Unmarshal([]byte(got), &gotJSON)
					errWant := json.Unmarshal([]byte(tc.wantBody), &wantJSON)

					if errGot == nil && errWant == nil {
						if diff := cmp.Diff(wantJSON, gotJSON); diff != "" {
							t.Fatalf("unexpected JSON value mismatch (-want +got):\n%s\nRaw got: %s\nRaw want: %s", diff, got, tc.wantBody)
						}
					} else {
						t.Fatalf("unexpected value: got %q, want %q", got, tc.wantBody)
					}
				}
			}
		})
	}
}

// RunToolInvokeWithTemplateParameters runs tool invoke test cases with template parameters.
func RunToolInvokeWithTemplateParameters(t *testing.T, tableName string, options ...TemplateParamOption) {
	// Resolve options
	// Default values for TemplateParameterTestConfig
	configs := &TemplateParameterTestConfig{
		ddlWant:         "[]",
		selectAllWant:   "[{\"age\":21,\"id\":1,\"name\":\"Alex\"},{\"age\":100,\"id\":2,\"name\":\"Alice\"}]",
		selectId1Want:   "[{\"age\":21,\"id\":1,\"name\":\"Alex\"}]",
		selectNameWant:  "[{\"age\":21,\"id\":1,\"name\":\"Alex\"}]",
		selectEmptyWant: "[]",
		insert1Want:     "[]",

		nameFieldArray: `["name"]`,
		nameColFilter:  "name",
		createColArray: `["id INT","name VARCHAR(20)","age INT"]`,

		supportDdl:    true,
		supportInsert: true,
	}

	// Apply provided options
	for _, option := range options {
		option(configs)
	}

	selectOnlyNamesWant := "[{\"name\":\"Alex\"},{\"name\":\"Alice\"}]"

	parseJSONArray := func(s string) []any {
		var res []any
		_ = json.Unmarshal([]byte(s), &res)
		return res
	}

	// Test tool invoke endpoint
	invokeTcs := []struct {
		name           string
		enabled        bool
		ddl            bool
		insert         bool
		toolName       string
		requestHeader  map[string]string
		args           map[string]any
		want           string
		wantContentErr string
		isErr          bool
	}{
		{
			name:          "invoke create-table-templateParams-tool",
			ddl:           true,
			toolName:      "create-table-templateParams-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"tableName": tableName, "columns": parseJSONArray(configs.createColArray)},
			want:          configs.ddlWant,
			isErr:         false,
		},
		{
			name:          "invoke insert-table-templateParams-tool",
			insert:        true,
			toolName:      "insert-table-templateParams-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"tableName": tableName, "columns": []any{"id", "name", "age"}, "values": "1, 'Alex', 21"},
			want:          configs.insert1Want,
			isErr:         false,
		},
		{
			name:          "invoke insert-table-templateParams-tool",
			insert:        true,
			toolName:      "insert-table-templateParams-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"tableName": tableName, "columns": []any{"id", "name", "age"}, "values": "2, 'Alice', 100"},
			want:          configs.insert1Want,
			isErr:         false,
		},
		{
			name:          "invoke select-templateParams-tool",
			toolName:      "select-templateParams-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"tableName": tableName},
			want:          configs.selectAllWant,
			isErr:         false,
		},
		{
			name:          "invoke select-templateParams-combined-tool",
			toolName:      "select-templateParams-combined-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"id": 1, "tableName": tableName},
			want:          configs.selectId1Want,
			isErr:         false,
		},
		{
			name:          "invoke select-templateParams-combined-tool with no results",
			toolName:      "select-templateParams-combined-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"id": 999, "tableName": tableName},
			want:          configs.selectEmptyWant,
			isErr:         false,
		},
		{
			name:          "invoke select-fields-templateParams-tool",
			enabled:       configs.supportSelectFields,
			toolName:      "select-fields-templateParams-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"tableName": tableName, "fields": parseJSONArray(configs.nameFieldArray)},
			want:          selectOnlyNamesWant,
			isErr:         false,
		},
		{
			name:          "invoke select-filter-templateParams-combined-tool",
			toolName:      "select-filter-templateParams-combined-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"name": "Alex", "tableName": tableName, "columnFilter": configs.nameColFilter},
			want:          configs.selectNameWant,
			isErr:         false,
		},
		{
			name:          "invoke drop-table-templateParams-tool",
			ddl:           true,
			toolName:      "drop-table-templateParams-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"tableName": tableName},
			want:          configs.ddlWant,
			isErr:         false,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.enabled {
				return
			}
			// if test case is DDL and source support ddl test cases
			ddlAllow := !tc.ddl || (tc.ddl && configs.supportDdl)
			// if test case is insert statement and source support insert test cases
			insertAllow := !tc.insert || (tc.insert && configs.supportInsert)

			if ddlAllow && insertAllow {
				if configs.isMCP {
					toolName := tc.toolName
					args := tc.args
					if args == nil {
						args = make(map[string]any)
					}

					statusCode, mcpResp, err := InvokeMCPTool(t, toolName, args, tc.requestHeader)
					if statusCode != http.StatusOK {
						if tc.isErr {
							return
						}
						t.Fatalf("response status code is not 200, got %d, error: %v", statusCode, err)
					}

					if tc.wantContentErr != "" {
						AssertMCPError(t, mcpResp, tc.wantContentErr)
						return
					}

					if mcpResp.Result.IsError {
						if tc.isErr {
							return
						}
						t.Fatalf("%s returned error result: %v", tc.toolName, mcpResp.Result)
					}

					gotObj := getMCPResultText(t, mcpResp)
					gotBytes, _ := json.Marshal(gotObj)
					gotStr := string(gotBytes)

					if gotStr != tc.want {
						var gotJSON, wantJSON any
						errGot := json.Unmarshal([]byte(gotStr), &gotJSON)
						errWant := json.Unmarshal([]byte(tc.want), &wantJSON)

						if errGot == nil && errWant == nil {
							if diff := cmp.Diff(wantJSON, gotJSON); diff != "" {
								t.Fatalf("unexpected JSON value mismatch (-want +got):\n%s\nRaw got: %s\nRaw want: %s", diff, gotStr, tc.want)
							}
						} else {
							t.Fatalf("unexpected value: got %q, want %q", gotStr, tc.want)
						}
					}

				} else {
					// Legacy REST path
					api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
					reqBytes, _ := json.Marshal(tc.args)
					resp, respBody := RunRequest(t, http.MethodPost, api, bytes.NewBuffer(reqBytes), tc.requestHeader)
					if resp.StatusCode != http.StatusOK {
						if tc.isErr {
							return
						}
						t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
					}

					var body map[string]interface{}
					err := json.Unmarshal(respBody, &body)
					if err != nil {
						t.Fatalf("error parsing response body")
					}

					got, ok := body["result"].(string)
					if !ok {
						t.Fatalf("unable to find result in response body")
					}

					// Legacy REST assertion
					if got != tc.want {
						var gotJSON, wantJSON any
						errGot := json.Unmarshal([]byte(got), &gotJSON)
						errWant := json.Unmarshal([]byte(tc.want), &wantJSON)

						if errGot == nil && errWant == nil {
							if diff := cmp.Diff(wantJSON, gotJSON); diff != "" {
								t.Fatalf("unexpected JSON value mismatch (-want +got):\n%s\nRaw got: %s\nRaw want: %s", diff, got, tc.want)
							}
						} else {
							t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
						}
					}
				}
			}
		})
	}
}

func RunExecuteSqlToolInvokeTest(t *testing.T, createTableStatement, select1Want string, options ...ExecuteSqlOption) {
	// Resolve options
	// Default values for ExecuteSqlTestConfig
	configs := &ExecuteSqlTestConfig{
		select1Statement: `"SELECT 1"`,
		createWant:       "[]",
		dropWant:         "[]",
		selectEmptyWant:  "[]",
	}

	// Apply provided options
	for _, option := range options {
		option(configs)
	}

	// Get ID token
	idToken, err := GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Test tool invoke endpoint
	invokeTcs := []struct {
		name          string
		toolName      string
		requestHeader map[string]string
		args          map[string]any
		want          string
		isErr         bool
		isAgentErr    bool
	}{
		{
			name:          "invoke my-exec-sql-tool",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"sql": strings.Trim(configs.select1Statement, "\"")},
			want:          select1Want,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool create table",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"sql": strings.Trim(createTableStatement, "\"")},
			want:          configs.createWant,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool select table",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"sql": "SELECT * FROM t"},
			want:          configs.selectEmptyWant,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool drop table",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"sql": "DROP TABLE t"},
			want:          configs.dropWant,
			isErr:         false,
		},
		{
			name:          "invoke my-exec-sql-tool without body",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{},
			isAgentErr:    true,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			args:          map[string]any{"sql": strings.Trim(configs.select1Statement, "\"")},
			isErr:         false,
			want:          select1Want,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool with invalid auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			args:          map[string]any{"sql": strings.Trim(configs.select1Statement, "\"")},
			isErr:         true,
		},
		{
			name:          "Invoke my-auth-exec-sql-tool without auth token",
			toolName:      "my-auth-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"sql": strings.Trim(configs.select1Statement, "\"")},
			isErr:         true,
		},
		{
			name:          "invoke my-exec-sql-tool with invalid SELECT SQL",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"sql": "SELECT * FROM non_existent_table"},
			isAgentErr:    true,
		},
		{
			name:          "invoke my-exec-sql-tool with invalid ALTER SQL",
			toolName:      "my-exec-sql-tool",
			requestHeader: map[string]string{},
			args:          map[string]any{"sql": "ALTER TALE t ALTER COLUMN id DROP NOT NULL"},
			isAgentErr:    true,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			if configs.isMCP {
				toolName := tc.toolName
				args := tc.args
				if args == nil {
					args = make(map[string]any)
				}

				statusCode, mcpResp, err := InvokeMCPTool(t, toolName, args, tc.requestHeader)
				if statusCode != http.StatusOK {
					if tc.isErr || tc.isAgentErr {
						return
					}
					t.Fatalf("response status code is not 200, got %d: %v", statusCode, err)
				}
				if tc.isAgentErr {
					return
				}

				if mcpResp.Result.IsError {
					if tc.isAgentErr || tc.isErr {
						return
					}
					t.Fatalf("%s returned error result: %v", tc.toolName, mcpResp.Result)
				}

				gotObj := getMCPResultText(t, mcpResp)
				gotBytes, _ := json.Marshal(gotObj)
				gotStr := string(gotBytes)

				if strings.HasPrefix(strings.TrimSpace(tc.want), "[") || strings.HasPrefix(strings.TrimSpace(tc.want), "{") {
					// It looks like JSON, let's do JSON comparison
					var gotJSON, wantJSON any
					_ = json.Unmarshal([]byte(gotStr), &gotJSON)
					_ = json.Unmarshal([]byte(tc.want), &wantJSON)

					if diff := cmp.Diff(wantJSON, gotJSON); diff != "" {
						t.Fatalf("unexpected JSON value mismatch (-want +got):\n%s\nRaw got: %s\nRaw want: %s", diff, gotStr, tc.want)
					}
				} else {
					// Plain string, use strings.Contains as suggested by user
					if !strings.Contains(gotStr, tc.want) {
						t.Fatalf(`expected %q to contain %q`, gotStr, tc.want)
					}
				}

			} else {
				// Legacy REST path
				api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
				reqBytes, _ := json.Marshal(tc.args)
				resp, respBody := RunRequest(t, http.MethodPost, api, bytes.NewBuffer(reqBytes), tc.requestHeader)
				if resp.StatusCode != http.StatusOK {
					if tc.isErr {
						return
					}
					t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
				}
				if tc.isAgentErr {
					return
				}

				var body map[string]interface{}
				err := json.Unmarshal(respBody, &body)
				if err != nil {
					t.Fatalf("error parsing response body")
				}

				got, ok := body["result"].(string)
				if !ok {
					t.Fatalf("unable to find result in response body")
				}

				// Legacy REST assertion
				if got != tc.want {
					var gotJSON, wantJSON any
					errGot := json.Unmarshal([]byte(got), &gotJSON)
					errWant := json.Unmarshal([]byte(tc.want), &wantJSON)

					if errGot == nil && errWant == nil {
						if diff := cmp.Diff(wantJSON, gotJSON); diff != "" {
							t.Fatalf("unexpected JSON value mismatch (-want +got):\n%s\nRaw got: %s\nRaw want: %s", diff, got, tc.want)
						}
					} else {
						t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
					}
				}
			}
		})
	}
}

// RunMCPToolCallMethod runs the tool/call for mcp endpoint
func RunMCPToolCallMethod(t *testing.T, myFailToolWant, select1Want string, options ...McpTestOption) {
	// Resolve options
	// Default values for MCPTestConfig
	configs := &MCPTestConfig{
		myToolId3NameAliceWant: `{"jsonrpc":"2.0","id":"my-tool","result":{"content":[{"type":"text","text":"{\"id\":1,\"name\":\"Alice\"}"},{"type":"text","text":"{\"id\":3,\"name\":\"Sid\"}"}]}}`,
		mcpSelect1Want:         select1Want,
		supportClientAuth:      false,
		supportSelect1Auth:     true,
	}

	// Apply provided options
	for _, option := range options {
		option(configs)
	}

	sessionId := RunInitialize(t, "2024-11-05")

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	idToken, err := GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Test tool invoke endpoint
	invokeTcs := []struct {
		name           string
		api            string
		enabled        bool // switch to turn on/off the test case
		requestBody    jsonrpc.JSONRPCRequest
		requestHeader  map[string]string
		wantStatusCode int
		wantBody       string
	}{
		{
			name:          "MCP Invoke my-tool",
			api:           "http://127.0.0.1:5000/mcp",
			enabled:       true,
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "my-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name": "my-tool",
					"arguments": map[string]any{
						"id":   int(3),
						"name": "Alice",
					},
				},
			},
			wantStatusCode: http.StatusOK,
			wantBody:       configs.myToolId3NameAliceWant,
		},
		{
			name:          "MCP Invoke invalid tool",
			api:           "http://127.0.0.1:5000/mcp",
			enabled:       true,
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invalid-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "foo",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusOK,
			wantBody:       `{"jsonrpc":"2.0","id":"invalid-tool","error":{"code":-32602,"message":"invalid tool name: tool with name \"foo\" does not exist"}}`,
		},
		{
			name:          "MCP Invoke my-tool without parameters",
			api:           "http://127.0.0.1:5000/mcp",
			enabled:       true,
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke-without-parameter",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-tool",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusOK,
			wantBody:       `{"jsonrpc":"2.0","id":"invoke-without-parameter","result":{"content":[{"type":"text","text":"provided parameters were invalid: parameter \"id\" is required"}],"isError":true}}`,
		},
		{
			name:          "MCP Invoke my-tool with insufficient parameters",
			api:           "http://127.0.0.1:5000/mcp",
			enabled:       true,
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke-insufficient-parameter",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-tool",
					"arguments": map[string]any{"id": 1},
				},
			},
			wantStatusCode: http.StatusOK,
			wantBody:       `{"jsonrpc":"2.0","id":"invoke-insufficient-parameter","result":{"content":[{"type":"text","text":"provided parameters were invalid: parameter \"name\" is required"}],"isError":true}}`,
		},
		{
			name:          "MCP Invoke my-auth-required-tool",
			api:           "http://127.0.0.1:5000/mcp",
			enabled:       configs.supportSelect1Auth,
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-auth-required-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-auth-required-tool",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusOK,
			wantBody:       configs.mcpSelect1Want,
		},
		{
			name:          "MCP Invoke my-auth-required-tool with invalid auth token",
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-auth-required-tool with invalid token",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-auth-required-tool",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusUnauthorized,
			wantBody:       "{\"jsonrpc\":\"2.0\",\"id\":\"invoke my-auth-required-tool with invalid token\",\"error\":{\"code\":-32600,\"message\":\"unauthorized Tool call: Please make sure you specify correct auth headers: unauthorized\"}}",
		},
		{
			name:          "MCP Invoke my-auth-required-tool without auth token",
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-auth-required-tool without token",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-auth-required-tool",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusUnauthorized,
			wantBody:       "{\"jsonrpc\":\"2.0\",\"id\":\"invoke my-auth-required-tool without token\",\"error\":{\"code\":-32600,\"message\":\"unauthorized Tool call: Please make sure you specify correct auth headers: unauthorized\"}}",
		},

		{
			name:          "MCP Invoke my-client-auth-tool",
			enabled:       configs.supportClientAuth,
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{"Authorization": accessToken},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-client-auth-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-client-auth-tool",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusOK,
			wantBody:       "{\"jsonrpc\":\"2.0\",\"id\":\"invoke my-client-auth-tool\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"{\\\"f0_\\\":1}\"}]}}",
		},
		{
			name:          "MCP Invoke my-client-auth-tool without access token",
			enabled:       configs.supportClientAuth,
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-client-auth-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-client-auth-tool",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusUnauthorized,
			wantBody:       "{\"jsonrpc\":\"2.0\",\"id\":\"invoke my-client-auth-tool\",\"error\":{\"code\":-32600,\"message\":\"missing access token in the 'Authorization' header\"}",
		},
		{
			name:          "MCP Invoke my-client-auth-tool with invalid access token",
			enabled:       configs.supportClientAuth,
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{"Authorization": "Bearer invalid-token"},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-client-auth-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-client-auth-tool",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:    "MCP Invoke my-custom-client-auth-tool with custom access token",
			enabled: configs.supportClientAuth,
			api:     "http://127.0.0.1:5000/mcp",
			// Note: This assumes my-custom-client-auth-tool is configured to use X-Custom-Auth
			requestHeader: map[string]string{"X-Custom-Auth": accessToken},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-custom-client-auth-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-custom-client-auth-tool",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusOK,
			wantBody:       "{\"jsonrpc\":\"2.0\",\"id\":\"invoke my-custom-client-auth-tool\",\"result\":{\"content\":[{\"type\":\"text\",\"text\":\"{\\\"f0_\\\":1}\"}]}}",
		},
		{
			name:          "MCP Invoke my-custom-client-auth-tool without access token",
			enabled:       configs.supportClientAuth,
			api:           "http://127.0.0.1:5000/mcp",
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke my-custom-client-auth-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-custom-client-auth-tool",
					"arguments": map[string]any{},
				},
			},
			wantStatusCode: http.StatusUnauthorized,
		},
		{
			name:          "MCP Invoke my-fail-tool",
			api:           "http://127.0.0.1:5000/mcp",
			enabled:       true,
			requestHeader: map[string]string{},
			requestBody: jsonrpc.JSONRPCRequest{
				Jsonrpc: "2.0",
				Id:      "invoke-fail-tool",
				Request: jsonrpc.Request{
					Method: "tools/call",
				},
				Params: map[string]any{
					"name":      "my-fail-tool",
					"arguments": map[string]any{"id": 1},
				},
			},
			wantStatusCode: http.StatusOK,
			wantBody:       myFailToolWant,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			if !tc.enabled {
				return
			}
			reqMarshal, err := json.Marshal(tc.requestBody)
			if err != nil {
				t.Fatalf("unexpected error during marshaling of request body")
			}

			// add headers
			headers := map[string]string{}
			if sessionId != "" {
				headers["Mcp-Session-Id"] = sessionId
			}
			for key, value := range tc.requestHeader {
				headers[key] = value
			}

			httpResponse, respBody := RunRequest(t, http.MethodPost, tc.api, bytes.NewBuffer(reqMarshal), headers)

			// Check status code
			if httpResponse.StatusCode != tc.wantStatusCode {
				t.Errorf("StatusCode mismatch: got %d, want %d", httpResponse.StatusCode, tc.wantStatusCode)
			}

			// Check response body
			got := string(bytes.TrimSpace(respBody))
			if !strings.Contains(got, tc.wantBody) {
				t.Fatalf("Expected substring not found:\ngot:  %q\nwant: %q (to be contained within got)", got, tc.wantBody)
			}
		})
	}

	t.Run("secure parameters", func(t *testing.T) {
		RunMCPSecureToolInvokeTest(t, options...)
	})
}

func setupPostgresSchemas(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schemaName string) func() {
	createSchemaStmt := fmt.Sprintf("CREATE SCHEMA %s", schemaName)
	_, err := pool.Exec(ctx, createSchemaStmt)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return func() {
		dropSchemaStmt := fmt.Sprintf("DROP SCHEMA %s CASCADE", schemaName)
		_, err := pool.Exec(context.WithoutCancel(ctx), dropSchemaStmt)
		if err != nil {
			t.Fatalf("failed to drop schema: %v", err)
		}
	}
}

func RunPostgresListTablesTest(t *testing.T, tableNameParam, tableNameAuth, user string) {
	// TableNameParam columns to construct want
	paramTableColumns := fmt.Sprintf(`[
		{"data_type": "integer", "column_name": "id", "column_default": "nextval('%s_id_seq'::regclass)", "is_not_nullable": true, "ordinal_position": 1, "column_comment": null},
		{"data_type": "text", "column_name": "name", "column_default": null, "is_not_nullable": false, "ordinal_position": 2, "column_comment": null}
	]`, tableNameParam)

	// TableNameAuth columns to construct want
	authTableColumns := fmt.Sprintf(`[
		{"data_type": "integer", "column_name": "id", "column_default": "nextval('%s_id_seq'::regclass)", "is_not_nullable": true, "ordinal_position": 1, "column_comment": null},
		{"data_type": "text", "column_name": "name", "column_default": null, "is_not_nullable": false, "ordinal_position": 2, "column_comment": null},
		{"data_type": "text", "column_name": "email", "column_default": null, "is_not_nullable": false, "ordinal_position": 3, "column_comment": null}
	]`, tableNameAuth)

	const (
		// Template to construct detailed output want
		detailedObjectTemplate = `{
            "object_name": "%[1]s", "schema_name": "public",
            "object_details": {
                "owner": "%[3]s", "comment": null,
                "indexes": [{"is_primary": true, "is_unique": true, "index_name": "%[1]s_pkey", "index_method": "btree", "index_columns": ["id"], "index_definition": "CREATE UNIQUE INDEX %[1]s_pkey ON public.%[1]s USING btree (id)"}],
                "triggers": [], "columns": %[2]s, "object_name": "%[1]s", "object_type": "TABLE", "schema_name": "public",
                "constraints": [{"constraint_name": "%[1]s_pkey", "constraint_type": "PRIMARY KEY", "constraint_columns": ["id"], "constraint_definition": "PRIMARY KEY (id)", "foreign_key_referenced_table": null, "foreign_key_referenced_columns": null}]
            }
        }`

		// Template to construct simple output want
		simpleObjectTemplate = `{"object_name":"%s", "schema_name":"public", "object_details":{"name":"%s"}}`
	)

	// Helper to build json for detailed want
	getDetailedWant := func(tableName, columnJSON string) string {
		return fmt.Sprintf(detailedObjectTemplate, tableName, columnJSON, user)
	}

	// Helper to build template for simple want
	getSimpleWant := func(tableName string) string {
		return fmt.Sprintf(simpleObjectTemplate, tableName, tableName)
	}

	invokeTcs := []struct {
		name           string
		toolName       string
		requestHeader  map[string]string
		args           map[string]any
		wantStatusCode int
		want           string
		isAllTables    bool
		isAgentErr     bool
	}{
		{
			name:           "invoke list_tables all tables detailed output",
			toolName:       "list_tables",
			args:           map[string]any{"table_names": ""},
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s,%s]", getDetailedWant(tableNameAuth, authTableColumns), getDetailedWant(tableNameParam, paramTableColumns)),
			isAllTables:    true,
		},
		{
			name:           "invoke list_tables all tables simple output",
			toolName:       "list_tables",
			args:           map[string]any{"table_names": "", "output_format": "simple"},
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s,%s]", getSimpleWant(tableNameAuth), getSimpleWant(tableNameParam)),
			isAllTables:    true,
		},
		{
			name:           "invoke list_tables detailed output",
			toolName:       "list_tables",
			args:           map[string]any{"table_names": tableNameAuth},
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s]", getDetailedWant(tableNameAuth, authTableColumns)),
		},
		{
			name:           "invoke list_tables simple output",
			toolName:       "list_tables",
			args:           map[string]any{"table_names": tableNameAuth, "output_format": "simple"},
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s]", getSimpleWant(tableNameAuth)),
		},
		{
			name:           "invoke list_tables with invalid output format",
			toolName:       "list_tables",
			args:           map[string]any{"table_names": "", "output_format": "abcd"},
			wantStatusCode: http.StatusOK,
			isAgentErr:     true,
		},
		{
			name:           "invoke list_tables with malformed table_names parameter",
			toolName:       "list_tables",
			args:           map[string]any{"table_names": 12345, "output_format": "detailed"},
			wantStatusCode: http.StatusOK,
			isAgentErr:     true,
		},
		{
			name:           "invoke list_tables with multiple table names",
			toolName:       "list_tables",
			args:           map[string]any{"table_names": fmt.Sprintf("%s,%s", tableNameParam, tableNameAuth)},
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s,%s]", getDetailedWant(tableNameAuth, authTableColumns), getDetailedWant(tableNameParam, paramTableColumns)),
		},
		{
			name:           "invoke list_tables with non-existent table",
			toolName:       "list_tables",
			args:           map[string]any{"table_names": "non_existent_table"},
			wantStatusCode: http.StatusOK,
			want:           `[]`,
		},
		{
			name:           "invoke list_tables with one existing and one non-existent table",
			toolName:       "list_tables",
			args:           map[string]any{"table_names": fmt.Sprintf("%s,non_existent_table", tableNameParam)},
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s]", getDetailedWant(tableNameParam, paramTableColumns)),
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
			reqBytes, _ := json.Marshal(tc.args)
			resp, respBytes := RunRequest(t, http.MethodPost, api, bytes.NewBuffer(reqBytes), tc.requestHeader)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBytes))
			}

			if tc.wantStatusCode == http.StatusOK {

				var bodyWrapper map[string]json.RawMessage

				if err := json.Unmarshal(respBytes, &bodyWrapper); err != nil {
					t.Fatalf("error parsing response wrapper: %s, body: %s", err, string(respBytes))
				}

				resultJSON, ok := bodyWrapper["result"]
				if !ok {
					t.Fatal("unable to find 'result' in response body")
				}

				if tc.isAgentErr {
					return
				}

				var resultString string
				if err := json.Unmarshal(resultJSON, &resultString); err != nil {
					t.Fatalf("'result' is not a JSON-encoded string: %s", err)
				}

				var got, want []any

				if err := json.Unmarshal([]byte(resultString), &got); err != nil {
					t.Fatalf("failed to unmarshal actual result string: %v", err)
				}
				if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
					t.Fatalf("failed to unmarshal expected want string: %v", err)
				}

				// Checking only the default public schema where the test tables are created to avoid brittle tests.
				if tc.isAllTables {
					var filteredGot []any
					for _, item := range got {
						if tableMap, ok := item.(map[string]interface{}); ok {
							name, _ := tableMap["object_name"].(string)

							// Only keep the table if it matches expected test tables
							if name == tableNameParam || name == tableNameAuth {
								filteredGot = append(filteredGot, item)
							}
						}
					}
					got = filteredGot
				}

				sort.SliceStable(got, func(i, j int) bool {
					return fmt.Sprintf("%v", got[i]) < fmt.Sprintf("%v", got[j])
				})
				sort.SliceStable(want, func(i, j int) bool {
					return fmt.Sprintf("%v", want[i]) < fmt.Sprintf("%v", want[j])
				})

				if !reflect.DeepEqual(got, want) {
					t.Errorf("Unexpected result: got  %#v, want: %#v", got, want)
				}
			}
		})
	}
}

func setUpPostgresViews(t *testing.T, ctx context.Context, pool *pgxpool.Pool, viewName string) func() {
	createView := fmt.Sprintf("CREATE VIEW %s AS SELECT 1 AS col", viewName)
	_, err := pool.Exec(ctx, createView)
	if err != nil {
		t.Fatalf("failed to create view: %v", err)
	}
	return func() {
		dropView := fmt.Sprintf("DROP VIEW %s", viewName)
		_, err := pool.Exec(context.WithoutCancel(ctx), dropView)
		if err != nil {
			t.Fatalf("failed to drop view: %v", err)
		}
	}
}

func RunPostgresListViewsTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	//adding this line temporarily
	viewName := "test_view_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	dropViewfunc1 := setUpPostgresViews(t, ctx, pool, viewName)
	defer dropViewfunc1()

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           string
	}{
		{
			name:           "invoke list_views with newly created view",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"view_name": "%s"}`, viewName))),
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf(`[{"schema_name":"public","view_name":"%s","owner_name":"postgres","definition":" SELECT 1 AS col;"}]`, viewName),
		},
		{
			name:           "invoke list_views with non-existent_view",
			requestBody:    bytes.NewBuffer([]byte(`{"view_name": "non_existent_view"}`)),
			wantStatusCode: http.StatusOK,
			want:           `[]`,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_views/invoke"
			resp, body := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)

			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(body))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(body, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got, want any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v", err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatalf("failed to unmarshal want string: %v", err)
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("Unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func RunPostgresListSchemasTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner string, uniqueID string) {
	schemaName := "test_schema_" + uniqueID
	cleanup := setupPostgresSchemas(t, ctx, pool, schemaName)
	defer cleanup()

	wantSchema := map[string]any{"functions": float64(0), "grants": map[string]any{}, "owner": owner, "schema_name": schemaName, "tables": float64(0), "views": float64(0)}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           []map[string]any
		compareSubset  bool
	}{
		{
			name:           "invoke list_schemas with schema_name",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"schema_name": "%s"}`, schemaName))),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantSchema},
		},
		// TODO: Re-enable this test case after this issue is fixed: https://github.com/googleapis/mcp-toolbox/issues/2562
		// {
		// 	name:           "invoke list_schemas with owner name",
		// 	requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"owner": "%s"}`, owner))),
		// 	wantStatusCode: http.StatusOK,
		// 	want:           []map[string]any{wantSchema},
		// 	compareSubset:  true,
		// },
		{
			name:           "invoke list_schemas with limit 1",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"schema_name": "%s","limit": 1}`, schemaName))),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantSchema},
		},
		{
			name:           "invoke list_schemas with non-existent schema",
			requestBody:    bytes.NewBuffer([]byte(`{"schema_name": "non_existent_schema"}`)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_schemas/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v", err)
			}

			if tc.compareSubset {
				// Assert that the 'wantTrigger' is present in the 'got' list.
				found := false
				for _, resultSchema := range got {
					if resultSchema["schema_name"] == wantSchema["schema_name"] {
						found = true
						if diff := cmp.Diff(wantSchema, resultSchema); diff != "" {
							t.Errorf("Mismatch in fields for the expected trigger (-want +got):\n%s", diff)
						}
						break
					}
				}
				if !found {
					t.Errorf("Expected schema '%+v' not found in the list of all schemas.", wantSchema)
				}
			} else {
				if diff := cmp.Diff(tc.want, got); diff != "" {
					t.Errorf("Unexpected result (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func RunPostgresDatabaseOverviewTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	const api = "http://127.0.0.1:5000/api/tool/database_overview/invoke"
	requestBody := bytes.NewBuffer([]byte(`{}`))

	resp, respBody := RunRequest(t, http.MethodPost, api, requestBody, nil)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, http.StatusOK, string(respBody))
	}

	var bodyWrapper struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
		t.Fatalf("error decoding response wrapper: %v, body: %s", err, string(respBody))
	}

	var resultString string
	if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
		resultString = string(bodyWrapper.Result)
	}

	var got []map[string]any
	if err := json.Unmarshal([]byte(resultString), &got); err != nil {
		t.Fatalf("failed to unmarshal nested result string: %v, result string: %s", err, resultString)
	}

	if len(got) != 1 {
		t.Fatalf("Expected exactly one row in the result, got %d", len(got))
	}

	resultRow := got[0]

	// Define expected keys based on the SELECT statement
	expectedKeys := []string{
		"pg_version",
		"is_replica",
		"uptime",
		"max_connections",
		"current_connections",
		"active_connections",
		"pct_connections_used",
	}

	for _, key := range expectedKeys {
		if _, ok := resultRow[key]; !ok {
			t.Errorf("Missing expected key in result: %s", key)
		}
	}

	// Check types of the fields. JSON numbers are unmarshalled into float64.
	if _, ok := resultRow["pg_version"].(string); !ok {
		t.Errorf("Expected 'pg_version' to be a string, got %T", resultRow["pg_version"])
	}
	if _, ok := resultRow["is_replica"].(bool); !ok {
		t.Errorf("Expected 'is_replica' to be a bool, got %T", resultRow["is_replica"])
	}
	if _, ok := resultRow["uptime"].(string); !ok {
		t.Errorf("Expected 'uptime' to be a string, got %T", resultRow["uptime"])
	}
	if _, ok := resultRow["max_connections"].(float64); !ok {
		t.Errorf("Expected 'max_connections' to be a number (float64), got %T", resultRow["max_connections"])
	}
	if _, ok := resultRow["current_connections"].(float64); !ok {
		t.Errorf("Expected 'current_connections' to be a number (float64), got %T", resultRow["current_connections"])
	}
	if _, ok := resultRow["active_connections"].(float64); !ok {
		t.Errorf("Expected 'active_connections' to be a number (float64), got %T", resultRow["active_connections"])
	}
	if _, ok := resultRow["pct_connections_used"].(float64); !ok {
		t.Errorf("Expected 'pct_connections_used' to be a number (float64), got %T", resultRow["pct_connections_used"])
	}

	// Basic sanity checks on values
	if maxConn, ok := resultRow["max_connections"].(float64); ok {
		if maxConn <= 0 {
			t.Errorf("Expected 'max_connections' to be positive, got %f", maxConn)
		}
	}

	if pctUsed, ok := resultRow["pct_connections_used"].(float64); ok {
		if pctUsed < 0 || pctUsed > 100 {
			t.Errorf("Expected 'pct_connections_used' to be between 0 and 100, got %f", pctUsed)
		}
	}
}

func setupPostgresTrigger(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schemaName, tableName, functionName, triggerName string) func() {
	t.Helper()

	createSchemaStmt := fmt.Sprintf("CREATE SCHEMA %s", schemaName)
	if _, err := pool.Exec(ctx, createSchemaStmt); err != nil {
		t.Fatalf("failed to create schema %s: %v", schemaName, err)
	}

	createTableStmt := fmt.Sprintf("CREATE TABLE %s.%s (id SERIAL PRIMARY KEY, name TEXT)", schemaName, tableName)
	if _, err := pool.Exec(ctx, createTableStmt); err != nil {
		t.Fatalf("failed to create table %s.%s: %v", schemaName, tableName, err)
	}

	createFunctionStmt := fmt.Sprintf(`
	CREATE OR REPLACE FUNCTION %s.%s() RETURNS TRIGGER AS $$
	BEGIN
		RETURN NEW;
	END;
	$$ LANGUAGE plpgsql;
`, schemaName, functionName)
	if _, err := pool.Exec(ctx, createFunctionStmt); err != nil {
		t.Fatalf("failed to create function %s.%s: %v", schemaName, functionName, err)
	}

	createTriggerStmt := fmt.Sprintf(`
	CREATE TRIGGER %s
	AFTER INSERT ON %s.%s
	FOR EACH ROW
	EXECUTE FUNCTION %s.%s();
`, triggerName, schemaName, tableName, schemaName, functionName)
	if _, err := pool.Exec(ctx, createTriggerStmt); err != nil {
		t.Fatalf("failed to create trigger %s: %v", triggerName, err)
	}

	return func() {
		dropSchemaStmt := fmt.Sprintf("DROP SCHEMA %s CASCADE", schemaName)
		if _, err := pool.Exec(context.WithoutCancel(ctx), dropSchemaStmt); err != nil {
			t.Fatalf("failed to drop schema %s: %v", schemaName, err)
		}
	}
}

func RunPostgresListTriggersTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	uniqueID := strings.ReplaceAll(uuid.New().String(), "-", "")
	schemaName := "test_schema_" + uniqueID
	tableName := "test_table_" + uniqueID
	functionName := "test_func_" + uniqueID
	triggerName := "test_trigger_" + uniqueID

	cleanup := setupPostgresTrigger(t, ctx, pool, schemaName, tableName, functionName, triggerName)
	defer cleanup()

	// Definition can vary slightly based on server version/settings, so we fetch it to compare.
	var expectedDef string
	getDefQuery := fmt.Sprintf("SELECT pg_get_triggerdef(oid) FROM pg_trigger WHERE tgname = '%s'", triggerName)
	err := pool.QueryRow(ctx, getDefQuery).Scan(&expectedDef)
	if err != nil {
		t.Fatalf("failed to fetch trigger definition: %v", err)
	}

	wantTrigger := map[string]any{
		"trigger_name":     triggerName,
		"schema_name":      schemaName,
		"table_name":       tableName,
		"status":           "ENABLED",
		"timing":           "AFTER",
		"events":           "INSERT",
		"activation_level": "ROW",
		"function_name":    functionName,
		"definition":       expectedDef,
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           []map[string]any
		compareSubset  bool
	}{
		{
			name:           "list all triggers (expecting the one we created)",
			requestBody:    bytes.NewBuffer([]byte(`{}`)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantTrigger},
			compareSubset:  true, // avoid test flakiness in race condition
		},
		{
			name:           "filter by trigger_name",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"trigger_name": "%s"}`, triggerName))),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantTrigger},
		},
		{
			name:           "filter by schema_name",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"schema_name": "%s"}`, schemaName))),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantTrigger},
		},
		{
			name:           "filter by table_name",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"table_name": "%s"}`, tableName))),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantTrigger},
		},
		{
			name:           "filter by non-existent trigger_name",
			requestBody:    bytes.NewBuffer([]byte(`{"trigger_name": "non_existent_trigger"}`)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
		{
			name:           "filter by non-existent schema_name",
			requestBody:    bytes.NewBuffer([]byte(`{"schema_name": "non_existent_schema"}`)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
		{
			name:           "filter by non-existent table_name",
			requestBody:    bytes.NewBuffer([]byte(`{"table_name": "non_existent_table"}`)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_triggers/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v, content: %s", err, resultString)
			}

			if tc.compareSubset {
				// Assert that the 'wantTrigger' is present in the 'got' list.
				found := false
				for _, resultTrigger := range got {
					if resultTrigger["trigger_name"] == wantTrigger["trigger_name"] {
						found = true
						if diff := cmp.Diff(wantTrigger, resultTrigger); diff != "" {
							t.Errorf("Mismatch in fields for the expected trigger (-want +got):\n%s", diff)
						}
						break
					}
				}
				if !found {
					t.Errorf("Expected trigger '%s' not found in the list of all triggers.", triggerName)
				}
			} else {
				if diff := cmp.Diff(tc.want, got); diff != "" {
					t.Errorf("Unexpected result (-want +got):\n%s", diff)
				}
			}
		})
	}
}

func setupPostgresPublicationTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tableName string, pubName string) func(t *testing.T) {
	t.Helper()
	if _, err := pool.Exec(ctx, fmt.Sprintf("DROP PUBLICATION IF EXISTS %s;", pubName)); err != nil {
		t.Errorf("unable to drop publication %s: %v", pubName, err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s;", tableName)); err != nil {
		t.Errorf("unable to drop table %s: %v", tableName, err)
	}
	createTableStmt := fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT);", tableName)
	if _, err := pool.Exec(ctx, createTableStmt); err != nil {
		t.Fatalf("unable to create table %s: %v", tableName, err)
	}

	createPubStmt := fmt.Sprintf("CREATE PUBLICATION %s FOR TABLE %s;", pubName, tableName)
	if _, err := pool.Exec(ctx, createPubStmt); err != nil {
		if _, dropErr := pool.Exec(ctx, fmt.Sprintf("DROP TABLE IF EXISTS %s;", tableName)); dropErr != nil {
			t.Errorf("unable to drop table after failing to create publication: %v", dropErr)
		}
		t.Fatalf("unable to create publication %s: %v", pubName, err)
	}

	return func(t *testing.T) {
		t.Helper()
		if _, err := pool.Exec(context.WithoutCancel(ctx), fmt.Sprintf("DROP PUBLICATION IF EXISTS %s;", pubName)); err != nil {
			t.Errorf("unable to drop publication %s: %v", pubName, err)
		}
		if _, err := pool.Exec(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE IF EXISTS %s;", tableName)); err != nil {
			t.Errorf("unable to drop table %s: %v", tableName, err)
		}
	}
}

func RunPostgresListPublicationTablesTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	table1Name := "pub_table_1"
	pub1Name := "pub_1"

	table2Name := "pub_table_2"
	pub2Name := "pub_2"

	cleanup := setupPostgresPublicationTable(t, ctx, pool, table1Name, pub1Name)
	defer cleanup(t)
	cleanup2 := setupPostgresPublicationTable(t, ctx, pool, table2Name, pub2Name)
	defer cleanup2(t)

	// Fetch the current user to match the publication_owner
	var currentUser string
	err := pool.QueryRow(ctx, "SELECT current_user;").Scan(&currentUser)
	if err != nil {
		t.Fatalf("unable to fetch current user: %v", err)
	}

	wantTable1 := map[string]any{
		"publication_name":     pub1Name,
		"schema_name":          "public",
		"table_name":           table1Name,
		"publishes_all_tables": false,
		"publishes_inserts":    true,
		"publishes_updates":    true,
		"publishes_deletes":    true,
		"publishes_truncates":  true,
		"publication_owner":    currentUser,
	}

	wantTable2 := map[string]any{
		"publication_name":     pub2Name,
		"schema_name":          "public",
		"table_name":           table2Name,
		"publishes_all_tables": false,
		"publishes_inserts":    true,
		"publishes_updates":    true,
		"publishes_deletes":    true,
		"publishes_truncates":  true,
		"publication_owner":    currentUser,
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           []map[string]any
	}{
		{
			name:           "list all publication tables",
			requestBody:    bytes.NewBufferString(`{}`),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantTable1, wantTable2},
		},
		{
			name:           "list all tables for the created publication",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"publication_names": "%s"}`, pub1Name)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantTable1},
		},
		{
			name:           "filter by table_name",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_names": "%s, %s"}`, table1Name, table2Name)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantTable1, wantTable2},
		},
		{
			name:           "filter by schema_name and table_name",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_names": "public", "table_name": "%s , %s"}`, table1Name, table2Name)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantTable1, wantTable2},
		},
		{
			name:           "invoke list_publication_tables with non-existent table",
			requestBody:    bytes.NewBufferString(`{"table_names": "non_existent_table"}`),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
		{
			name:           "invoke list_publication_tables with non-existent publication",
			requestBody:    bytes.NewBufferString(`{"publication_names": "non_existent_pub"}`),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_publication_tables/invoke"

			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v, content: %s", err, resultString)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func RunPostgresListActiveQueriesTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	type queryListDetails struct {
		ProcessId        any    `json:"pid"`
		User             string `json:"user"`
		Datname          string `json:"datname"`
		ApplicationName  string `json:"application_name"`
		ClientAddress    string `json:"client_addr"`
		State            string `json:"state"`
		WaitEventType    string `json:"wait_event_type"`
		WaitEvent        string `json:"wait_event"`
		BackendStart     any    `json:"backend_start"`
		TransactionStart any    `json:"xact_start"`
		QueryStart       any    `json:"query_start"`
		QueryDuration    any    `json:"query_duration"`
		Query            string `json:"query"`
	}

	testUUID := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	tc2AppName := fmt.Sprintf("test_sleep_tc2_%s", testUUID)
	tc3AppName := fmt.Sprintf("test_sleep_tc3_%s", testUUID)

	tc3Query := fmt.Sprintf("SELECT set_config('application_name', '%s', false), pg_sleep(10);", tc3AppName)
	singleQueryWanted := queryListDetails{
		ApplicationName: tc3AppName,
		Query:           tc3Query,
	}

	invokeTcs := []struct {
		name                string
		toolName            string
		args                map[string]any
		appName             string
		clientSleepSecs     int
		waitSecsBeforeCheck int
		wantStatusCode      int
		want                []queryListDetails
	}{
		// exclude background monitoring apps such as "wal_uploader"
		{
			name:                "invoke list_active_queries when the system is idle",
			toolName:            "list_active_queries",
			args:                map[string]any{"exclude_application_names": "wal_uploader"},
			clientSleepSecs:     0,
			waitSecsBeforeCheck: 0,
			wantStatusCode:      http.StatusOK,
			want:                []queryListDetails{},
		},
		{
			name:                "invoke list_active_queries when there is 1 ongoing but lower than the threshold",
			toolName:            "list_active_queries",
			args:                map[string]any{"min_duration": "100 seconds", "exclude_application_names": "wal_uploader"},
			appName:             tc2AppName,
			clientSleepSecs:     3,
			waitSecsBeforeCheck: 1,
			wantStatusCode:      http.StatusOK,
			want:                []queryListDetails{},
		},
		{
			name:                "invoke list_active_queries when 1 ongoing query should show up",
			toolName:            "list_active_queries",
			args:                map[string]any{"min_duration": "1 seconds", "exclude_application_names": "wal_uploader"},
			appName:             tc3AppName,
			clientSleepSecs:     10,
			waitSecsBeforeCheck: 5,
			wantStatusCode:      http.StatusOK,
			want:                []queryListDetails{singleQueryWanted},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var wg sync.WaitGroup
			defer wg.Wait()

			queryCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			if tc.clientSleepSecs > 0 {
				wg.Add(1)

				go func() {
					defer wg.Done()

					err := pool.Ping(ctx)
					if err != nil {
						t.Errorf("unable to connect to test database: %s", err)
						return
					}
					query := fmt.Sprintf("SELECT set_config('application_name', '%s', false), pg_sleep(%d);", tc.appName, tc.clientSleepSecs)
					_, err = pool.Exec(queryCtx, query)
					if err != nil && !errors.Is(err, context.Canceled) {
						t.Errorf("Executing 'SELECT pg_sleep' failed: %s", err)
					}
				}()
			}

			if tc.waitSecsBeforeCheck > 0 {
				time.Sleep(time.Duration(tc.waitSecsBeforeCheck) * time.Second)
			}

			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", tc.toolName)
			reqBytes, _ := json.Marshal(tc.args)
			resp, respBody := RunRequest(t, http.MethodPost, api, bytes.NewBuffer(reqBytes), nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var details []queryListDetails
			if err := json.Unmarshal([]byte(resultString), &details); err != nil {
				t.Fatalf("failed to unmarshal nested ObjectDetails string: %v", err)
			}

			// Ensure queries with excluded application names are not returned.
			if excludeApp, ok := tc.args["exclude_application_names"].(string); ok && excludeApp != "" {
				for _, d := range details {
					if d.ApplicationName == excludeApp {
						t.Errorf("found excluded application_name %q in active queries: %#v", excludeApp, d)
					}
				}
			}

			// Filter to active queries that belong to this test case.
			got := []queryListDetails{}
			if tc.appName == "" {
				got = details
			} else {
				for _, d := range details {
					if d.ApplicationName == tc.appName {
						got = append(got, d)
					}
				}
			}

			if diff := cmp.Diff(tc.want, got, cmp.Comparer(func(a, b queryListDetails) bool {
				return a.ApplicationName == b.ApplicationName && a.Query == b.Query
			})); diff != "" {
				t.Errorf("Unexpected active queries (-want +got):\n%s", diff)
			}
		})
	}
}

func RunPostgresListAvailableExtensionsTest(t *testing.T) {
	invokeTcs := []struct {
		name           string
		api            string
		requestBody    io.Reader
		wantStatusCode int
	}{
		{
			name:           "invoke list_available_extensions output",
			api:            "http://127.0.0.1:5000/api/tool/list_available_extensions/invoke",
			wantStatusCode: http.StatusOK,
			requestBody:    bytes.NewBuffer([]byte(`{}`)),
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, respBody := RunRequest(t, http.MethodPost, tc.api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			// Intentionally not adding the output check as output depends on the postgres instance used where the the functional test runs.
			// Adding the check will make the test flaky.
		})
	}
}

func RunPostgresListInstalledExtensionsTest(t *testing.T) {
	invokeTcs := []struct {
		name           string
		api            string
		requestBody    io.Reader
		wantStatusCode int
	}{
		{
			name:           "invoke list_installed_extensions output",
			api:            "http://127.0.0.1:5000/api/tool/list_installed_extensions/invoke",
			wantStatusCode: http.StatusOK,
			requestBody:    bytes.NewBuffer([]byte(`{}`)),
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, bodyBytes := RunRequest(t, http.MethodPost, tc.api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			// Intentionally not adding the output check as output depends on the postgres instance used where the the functional test runs.
			// Adding the check will make the test flaky.
		})
	}
}

func setupPostgresIndex(t *testing.T, ctx context.Context, pool *pgxpool.Pool, schemaName string, tableName string) func(t *testing.T) {
	t.Helper()
	createSchemaStmt := fmt.Sprintf("CREATE SCHEMA IF NOT EXISTS %s;", schemaName)
	if _, err := pool.Exec(ctx, createSchemaStmt); err != nil {
		t.Fatalf("unable to create schema %s: %v", schemaName, err)
	}

	fullTableName := fmt.Sprintf("%s.%s", schemaName, tableName)
	createTableStmt := fmt.Sprintf("CREATE TABLE %s (id SERIAL PRIMARY KEY, name TEXT, email TEXT);", fullTableName)
	if _, err := pool.Exec(ctx, createTableStmt); err != nil {
		t.Fatalf("unable to create table %s: %v", fullTableName, err)
	}

	// Create a unique index on email
	index1Stmt := fmt.Sprintf("CREATE UNIQUE INDEX %s_email_idx ON %s (email);", tableName, fullTableName)
	if _, err := pool.Exec(ctx, index1Stmt); err != nil {
		t.Fatalf("unable to create index %s_email_idx: %v", tableName, err)
	}

	// Create a non-unique index on name
	index2Stmt := fmt.Sprintf("CREATE INDEX %s_name_idx ON %s (name);", tableName, fullTableName)
	if _, err := pool.Exec(ctx, index2Stmt); err != nil {
		t.Fatalf("unable to create index %s_name_idx: %v", tableName, err)
	}

	return func(t *testing.T) {
		t.Helper()
		if _, err := pool.Exec(context.WithoutCancel(ctx), fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE;", schemaName)); err != nil {
			t.Errorf("unable to drop schema: %v", err)
		}
	}
}

func RunPostgresListIndexesTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	schemaName := "testschema_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableName := "table1_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	cleanup := setupPostgresIndex(t, ctx, pool, schemaName, tableName)
	defer cleanup(t)

	// Primary key index
	wantIndexPK := map[string]any{
		"schema_name":      schemaName,
		"table_name":       tableName,
		"index_name":       tableName + "_pkey",
		"index_type":       "btree",
		"is_unique":        true,
		"is_primary":       true,
		"is_used":          false,
		"index_definition": fmt.Sprintf("CREATE UNIQUE INDEX %s_pkey ON %s.%s USING btree (id)", tableName, schemaName, tableName),
		// Size and scan counts can vary, so omitting them from strict checks or using ranges might be better in real tests.
	}
	// Email unique index
	wantIndexEmail := map[string]any{
		"schema_name":      schemaName,
		"table_name":       tableName,
		"index_name":       tableName + "_email_idx",
		"index_type":       "btree",
		"is_unique":        true,
		"is_primary":       false,
		"is_used":          false,
		"index_definition": fmt.Sprintf("CREATE UNIQUE INDEX %s_email_idx ON %s.%s USING btree (email)", tableName, schemaName, tableName),
	}
	// Name non-unique index
	wantIndexName := map[string]any{
		"schema_name":      schemaName,
		"table_name":       tableName,
		"index_name":       tableName + "_name_idx",
		"index_type":       "btree",
		"is_unique":        false,
		"is_primary":       false,
		"is_used":          false,
		"index_definition": fmt.Sprintf("CREATE INDEX %s_name_idx ON %s.%s USING btree (name)", tableName, schemaName, tableName),
	}

	allWantIndexes := []map[string]any{wantIndexEmail, wantIndexName, wantIndexPK}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           []map[string]any
	}{
		// List all indexes is skipped because the output might include indexes for other database tables
		// defined outside of this test, which could make the test flaky.
		{
			name:           "list_indexes for a specific schema and table",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s", "table_name": "%s"}`, schemaName, tableName)),
			wantStatusCode: http.StatusOK,
			want:           allWantIndexes,
		},
		{
			name:           "list_indexes for a specific schema",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s"}`, schemaName)),
			wantStatusCode: http.StatusOK,
			want:           allWantIndexes,
		},
		{
			name:           "list_indexes with non-existent schema",
			requestBody:    bytes.NewBufferString(`{"schema_name": "non_existent_schema"}`),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
		{
			name:           "list_indexes with non-existent table in existing schema",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s", "table_name": "non_existent_table"}`, schemaName)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
		{
			name:           "list_indexes filter by index name",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s", "table_name": "%s", "index_name": "%s"}`, schemaName, tableName, tableName+"_email_idx")),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantIndexEmail},
		},
		{
			name:           "list_indexes filter by non-existent index name",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s", "table_name": "%s", "index_name": "non_existent_idx"}`, schemaName, tableName)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_indexes/invoke"

			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v, resultString: %s", err, resultString)
			}
			// Normalize got by removing fields that are hard to predict (like size)
			for _, index := range got {
				delete(index, "index_size_bytes")
				delete(index, "index_scans")
				delete(index, "tuples_read")
				delete(index, "tuples_fetched")
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func setupListSequencesTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (string, func(t *testing.T)) {
	sequenceName := "list_sequences_seq1_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createSequence1Stmt := fmt.Sprintf("CREATE SEQUENCE %s INCREMENT 1 START 1;", sequenceName)

	_, err := pool.Exec(ctx, createSequence1Stmt)
	if err != nil {
		t.Fatalf("unable to create sequence %s: %s", sequenceName, err)
	}
	return sequenceName, func(t *testing.T) {
		_, err := pool.Exec(ctx, fmt.Sprintf("DROP SEQUENCE IF EXISTS %s;", sequenceName))
		if err != nil {
			t.Errorf("unable to drop sequences: %v", err)
		}
	}
}

func RunPostgresListSequencesTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	sequenceName, teardown := setupListSequencesTest(t, ctx, pool)
	defer teardown(t)

	wantSequence := map[string]any{
		"sequence_name":  sequenceName,
		"schema_name":    "public",
		"sequence_owner": "postgres",
		"data_type":      "bigint",
		"start_value":    float64(1),
		"min_value":      float64(1),
		"max_value":      float64(9223372036854775807),
		"increment_by":   float64(1),
		"last_value":     nil,
	}

	invokeTcs := []struct {
		name           string
		api            string
		requestBody    io.Reader
		wantStatusCode int
		want           []map[string]any
	}{
		{
			name:           "invoke list_sequences",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"sequence_name": "%s"}`, sequenceName)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantSequence},
		},
		{
			name:           "invoke list_sequences with non-existent sequence",
			requestBody:    bytes.NewBufferString(`{"sequence_name": "non_existent_sequence"}`),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_sequences/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v", err)
			}

			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Errorf("Unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func RunPostgresListTableSpacesTest(t *testing.T) {
	invokeTcs := []struct {
		name           string
		api            string
		requestBody    io.Reader
		wantStatusCode int
	}{
		{
			name:           "invoke list_tablespaces output",
			api:            "http://127.0.0.1:5000/api/tool/list_tablespaces/invoke",
			wantStatusCode: http.StatusOK,
			requestBody:    bytes.NewBuffer([]byte(`{}`)),
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, respBody := RunRequest(t, http.MethodPost, tc.api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(respBody))
			}

			// Intentionally not adding the output check as output depends on the postgres instance used where the the functional test runs.
			// Adding the check will make the test flaky.
		})
	}
}

func RunPostgresListPgSettingsTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	targetSetting := "maintenance_work_mem"
	var name, setting, unit, shortDesc, source, contextVal string

	// We query the raw pg_settings to get the data needed to reconstruct the logic
	// defined in your listPgSettingQuery.
	err := pool.QueryRow(ctx, `
		SELECT name, setting, unit, short_desc, source, context 
		FROM pg_settings 
		WHERE name = $1
	`, targetSetting).Scan(&name, &setting, &unit, &shortDesc, &source, &contextVal)

	if err != nil {
		t.Fatalf("Setup failed: could not fetch postgres setting '%s': %v", targetSetting, err)
	}

	// Replicate the SQL CASE logic for 'requires_restart' field
	requiresRestart := "No"
	switch contextVal {
	case "postmaster":
		requiresRestart = "Yes"
	case "sighup":
		requiresRestart = "No (Reload sufficient)"
	}

	expectedObject := map[string]interface{}{
		"name":             name,
		"current_value":    setting,
		"unit":             unit,
		"short_desc":       shortDesc,
		"source":           source,
		"requires_restart": requiresRestart,
	}
	expectedJSON, _ := json.Marshal([]interface{}{expectedObject})

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           string
	}{
		{
			name:           "invoke list_pg_settings with specific setting",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"setting_name": "%s"}`, targetSetting))),
			wantStatusCode: http.StatusOK,
			want:           string(expectedJSON),
		},
		{
			name:           "invoke list_pg_settings with non-existent setting",
			requestBody:    bytes.NewBuffer([]byte(`{"setting_name": "non_existent_config_xyz"}`)),
			wantStatusCode: http.StatusOK,
			want:           `[]`,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_pg_settings/invoke"
			resp, body := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)

			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(body))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(body, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got, want any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v", err)
			}
			if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
				t.Fatalf("failed to unmarshal want string: %v", err)
			}

			if diff := cmp.Diff(want, got); diff != "" {
				t.Errorf("Unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

// RunPostgresDatabaseStatsTest tests the database_stats tool by comparing API results
// against a direct query to the database.
func RunPostgresListDatabaseStatsTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	dbName1 := "test_db_stats_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	dbOwner1 := "test_user_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	dbName2 := "test_db_stats_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	dbOwner2 := "test_user_" + strings.ReplaceAll(uuid.NewString(), "-", "")

	cleanup1 := setUpDatabase(t, ctx, pool, dbName1, dbOwner1)
	defer cleanup1()
	cleanup2 := setUpDatabase(t, ctx, pool, dbName2, dbOwner2)
	defer cleanup2()

	requiredKeys := map[string]bool{
		"database_name":      true,
		"database_owner":     true,
		"default_tablespace": true,
		"is_connectable":     true,
	}

	db1Want := map[string]interface{}{
		"database_name":      dbName1,
		"database_owner":     dbOwner1,
		"default_tablespace": "pg_default",
		"is_connectable":     true,
	}

	db2Want := map[string]interface{}{
		"database_name":      dbName2,
		"database_owner":     dbOwner2,
		"default_tablespace": "pg_default",
		"is_connectable":     true,
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           []map[string]interface{}
	}{
		{
			name:           "invoke database_stats filtering by specific database name",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"database_name": "%s"}`, dbName1))),
			wantStatusCode: http.StatusOK,
			want:           []map[string]interface{}{db1Want},
		},
		{
			name:           "invoke database_stats filtering by specific owner",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"database_owner": "%s"}`, dbOwner2))),
			wantStatusCode: http.StatusOK,
			want:           []map[string]interface{}{db2Want},
		},
		{
			name:           "filter by tablespace",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"default_tablespace": "pg_default", "database_name": "%s"}`, dbName1))),
			wantStatusCode: http.StatusOK,
			want:           []map[string]interface{}{db1Want},
		},
		{
			name:           "sort by size",
			requestBody:    bytes.NewBuffer([]byte(fmt.Sprintf(`{"sort_by": "size", "database_name": "%s"}`, dbName2))),
			wantStatusCode: http.StatusOK,
			want:           []map[string]interface{}{db2Want},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_database_stats/invoke"
			resp, body := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)

			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(body))
			}
			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(body, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []map[string]interface{}
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v", err)
			}

			// Configuration for comparison
			opts := []cmp.Option{
				// Ensure consistent order based on name for comparison
				cmpopts.SortSlices(func(a, b map[string]interface{}) bool {
					return a["database_name"].(string) < b["database_name"].(string)
				}),

				// Ignore Volatile Keys which change in every run and only compare the keys in 'requiredKeys'
				cmpopts.IgnoreMapEntries(func(key string, _ interface{}) bool {
					return !requiredKeys[key]
				}),

				// Ignore Irrelevant Databases
				cmpopts.IgnoreSliceElements(func(v map[string]interface{}) bool {
					name, ok := v["database_name"].(string)
					if !ok {
						return true
					}
					return name != dbName1 && name != dbName2
				}),
			}

			if diff := cmp.Diff(tc.want, got, opts...); diff != "" {
				t.Errorf("Unexpected result (-want +got):\n%s", diff)
			}
		})
	}
}

func setUpDatabase(t *testing.T, ctx context.Context, pool *pgxpool.Pool, dbName, dbOwner string) func() {
	_, err := pool.Exec(ctx, fmt.Sprintf("CREATE ROLE %s LOGIN PASSWORD 'password';", dbOwner))
	if err != nil {
		_, _ = pool.Exec(ctx, fmt.Sprintf("DROP ROLE %s;", dbOwner))
		t.Fatalf("failed to create %s: %v", dbOwner, err)
	}
	_, err = pool.Exec(ctx, fmt.Sprintf("GRANT %s TO current_user;", dbOwner))
	if err != nil {
		t.Fatalf("failed to grant %s to current_user: %v", dbOwner, err)
	}
	_, err = pool.Exec(ctx, fmt.Sprintf("CREATE DATABASE %s OWNER %s;", dbName, dbOwner))
	if err != nil {
		t.Fatalf("failed to create %s: %v", dbName, err)
	}
	return func() {
		_, _ = pool.Exec(context.WithoutCancel(ctx), fmt.Sprintf("DROP DATABASE IF EXISTS %s;", dbName))
		_, _ = pool.Exec(context.WithoutCancel(ctx), fmt.Sprintf("DROP ROLE IF EXISTS %s;", dbOwner))
	}
}

func setupPostgresRoles(t *testing.T, ctx context.Context, pool *pgxpool.Pool) (string, string, string, func(t *testing.T)) {
	t.Helper()
	suffix := strings.ReplaceAll(uuid.New().String(), "-", "")

	adminUser := "test_role_admin_" + suffix
	superUser := "test_role_super_" + suffix
	normalUser := "test_role_normal_" + suffix

	createAdminStmt := fmt.Sprintf("CREATE ROLE %s NOLOGIN;", adminUser)
	if _, err := pool.Exec(ctx, createAdminStmt); err != nil {
		t.Fatalf("unable to create role %s: %v", adminUser, err)
	}

	createSuperUserStmt := fmt.Sprintf("CREATE ROLE %s LOGIN CREATEDB;", superUser)
	if _, err := pool.Exec(ctx, createSuperUserStmt); err != nil {
		t.Fatalf("unable to create role %s: %v", superUser, err)
	}

	createNormalUserStmt := fmt.Sprintf("CREATE ROLE %s LOGIN;", normalUser)
	if _, err := pool.Exec(ctx, createNormalUserStmt); err != nil {
		t.Fatalf("unable to create role %s: %v", normalUser, err)
	}

	// Establish Relationships (Admin -> Superuser -> Normal)
	if _, err := pool.Exec(ctx, fmt.Sprintf("GRANT %s TO %s;", adminUser, superUser)); err != nil {
		t.Fatalf("unable to grant %s to %s: %v", adminUser, superUser, err)
	}
	if _, err := pool.Exec(ctx, fmt.Sprintf("GRANT %s TO %s;", superUser, normalUser)); err != nil {
		t.Fatalf("unable to grant %s to %s: %v", superUser, normalUser, err)
	}

	return adminUser, superUser, normalUser, func(t *testing.T) {
		t.Helper()
		cleanupCtx := context.WithoutCancel(ctx)
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DROP ROLE IF EXISTS %s;", normalUser))
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DROP ROLE IF EXISTS %s;", superUser))
		_, _ = pool.Exec(cleanupCtx, fmt.Sprintf("DROP ROLE IF EXISTS %s;", adminUser))
	}
}

func RunPostgresListRolesTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	adminUser, superUser, normalUser, cleanup := setupPostgresRoles(t, ctx, pool)
	defer cleanup(t)

	wantAdmin := map[string]any{
		"role_name":           adminUser,
		"connection_limit":    float64(-1),
		"is_superuser":        false,
		"inherits_privileges": true,
		"can_create_roles":    false,
		"can_create_db":       false,
		"can_login":           false,
		"is_replication_role": false,
		"bypass_rls":          false,
		"direct_members":      []any{superUser},
		"member_of":           []any{},
	}

	wantSuperUser := map[string]any{
		"role_name":           superUser,
		"connection_limit":    float64(-1),
		"is_superuser":        false,
		"inherits_privileges": true,
		"can_create_roles":    false,
		"can_create_db":       true,
		"can_login":           true,
		"is_replication_role": false,
		"bypass_rls":          false,
		"direct_members":      []any{normalUser},
		"member_of":           []any{adminUser},
	}

	wantNormalUser := map[string]any{
		"role_name":           normalUser,
		"connection_limit":    float64(-1),
		"is_superuser":        false,
		"inherits_privileges": true,
		"can_create_roles":    false,
		"can_create_db":       false,
		"can_login":           true,
		"is_replication_role": false,
		"bypass_rls":          false,
		"direct_members":      []any{},
		"member_of":           []any{superUser},
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           []map[string]any
	}{
		{
			name:           "list_roles with filter for created roles",
			requestBody:    bytes.NewBufferString(`{"role_name": "test_role_"}`),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantAdmin, wantNormalUser, wantSuperUser},
		},
		{
			name:           "list_roles filter specific role",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"role_name": "%s"}`, superUser)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{wantSuperUser},
		},
		{
			name:           "list_roles non-existent role",
			requestBody:    bytes.NewBufferString(`{"role_name": "non_existent_role_xyz"}`),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_roles/invoke"

			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v, resultString: %s", err, resultString)
			}

			gotMap := make(map[string]map[string]any)
			for _, role := range got {
				// Remove fields that change every run
				delete(role, "oid")
				delete(role, "valid_until")

				if name, ok := role["role_name"].(string); ok {
					gotMap[name] = role
				}
			}

			// Check that every role in 'want' exists in 'got' and matches
			for _, wantRole := range tc.want {
				roleName, _ := wantRole["role_name"].(string)

				gotRole, exists := gotMap[roleName]
				if !exists {
					t.Errorf("Expected role %q was not found in the response", roleName)
					continue
				}

				if diff := cmp.Diff(wantRole, gotRole); diff != "" {
					t.Errorf("Role %q mismatch (-want +got):\n%s", roleName, diff)
				}
			}

			// Verify that if want is nil/empty, got is also empty
			if len(tc.want) == 0 && len(got) != 0 {
				t.Errorf("Expected empty result, but got %d roles", len(got))
			}
		})
	}
}

// RunMySQLListTablesTest run tests against the mysql-list-tables tool
func RunMySQLListTablesTest(t *testing.T, databaseName, tableNameParam, tableNameAuth, expectedOwner string, opts ...ToolExecOption) {
	config := &ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	var ownerWant any
	if expectedOwner == "" {
		ownerWant = nil
	} else {
		ownerWant = expectedOwner
	}

	type tableInfo struct {
		ObjectName    string `json:"object_name"`
		SchemaName    string `json:"schema_name"`
		ObjectDetails string `json:"object_details"`
	}

	type column struct {
		DataType        string `json:"data_type"`
		ColumnName      string `json:"column_name"`
		ColumnComment   string `json:"column_comment"`
		ColumnDefault   any    `json:"column_default"`
		IsNotNullable   int    `json:"is_not_nullable"`
		OrdinalPosition int    `json:"ordinal_position"`
	}

	type objectDetails struct {
		Owner       any      `json:"owner"`
		Columns     []column `json:"columns"`
		Comment     string   `json:"comment"`
		Indexes     []any    `json:"indexes"`
		Triggers    []any    `json:"triggers"`
		Constraints []any    `json:"constraints"`
		ObjectName  string   `json:"object_name"`
		ObjectType  string   `json:"object_type"`
		SchemaName  string   `json:"schema_name"`
	}

	paramTableWant := objectDetails{
		ObjectName: tableNameParam,
		SchemaName: databaseName,
		ObjectType: "TABLE",
		Owner:      ownerWant,
		Columns: []column{
			{DataType: "int", ColumnName: "id", IsNotNullable: 1, OrdinalPosition: 1},
			{DataType: "varchar(255)", ColumnName: "name", OrdinalPosition: 2},
		},
		Indexes:     []any{map[string]any{"index_columns": []any{"id"}, "index_name": "PRIMARY", "is_primary": float64(1), "is_unique": float64(1)}},
		Triggers:    []any{},
		Constraints: []any{map[string]any{"constraint_columns": []any{"id"}, "constraint_name": "PRIMARY", "constraint_type": "PRIMARY KEY", "foreign_key_referenced_columns": any(nil), "foreign_key_referenced_table": any(nil), "constraint_definition": ""}},
	}

	authTableWant := objectDetails{
		ObjectName: tableNameAuth,
		SchemaName: databaseName,
		ObjectType: "TABLE",
		Owner:      ownerWant,
		Columns: []column{
			{DataType: "int", ColumnName: "id", IsNotNullable: 1, OrdinalPosition: 1},
			{DataType: "varchar(255)", ColumnName: "name", OrdinalPosition: 2},
			{DataType: "varchar(255)", ColumnName: "email", OrdinalPosition: 3},
		},
		Indexes:     []any{map[string]any{"index_columns": []any{"id"}, "index_name": "PRIMARY", "is_primary": float64(1), "is_unique": float64(1)}},
		Triggers:    []any{},
		Constraints: []any{map[string]any{"constraint_columns": []any{"id"}, "constraint_name": "PRIMARY", "constraint_type": "PRIMARY KEY", "foreign_key_referenced_columns": any(nil), "foreign_key_referenced_table": any(nil), "constraint_definition": ""}},
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           any
		isSimple       bool
		isAllTables    bool
	}{
		{
			name:           "invoke list_tables for all tables detailed output",
			requestBody:    bytes.NewBufferString(`{"table_names":""}`),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant, paramTableWant},
			isAllTables:    true,
		},
		{
			name:           "invoke list_tables detailed output",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_names": "%s"}`, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant},
		},
		{
			name:           "invoke list_tables simple output",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_names": "%s", "output_format": "simple"}`, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []map[string]any{{"name": tableNameAuth}},
			isSimple:       true,
		},
		{
			name:           "invoke list_tables with multiple table names",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_names": "%s,%s"}`, tableNameParam, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant, paramTableWant},
		},
		{
			name:           "invoke list_tables with one existing and one non-existent table",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_names": "%s,non_existent_table"}`, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{authTableWant},
		},
		{
			name:           "invoke list_tables with non-existent table",
			requestBody:    bytes.NewBufferString(`{"table_names": "non_existent_table"}`),
			wantStatusCode: http.StatusOK,
			want:           []objectDetails{},
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var resultString string

			if config.isMCP {
				reqBytes, _ := io.ReadAll(tc.requestBody)
				var args map[string]any
				if len(reqBytes) > 0 {
					_ = json.Unmarshal(reqBytes, &args)
				}
				if args == nil {
					args = make(map[string]any)
				}

				statusCode, mcpResp, err := InvokeMCPTool(t, "list_tables", args, nil)
				if statusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, err: %v", statusCode, tc.wantStatusCode, err)
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				gotObj := getMCPResultText(t, mcpResp)
				if len(gotObj) == 0 {
					resultString = "null"
				} else {
					gotBytes, _ := json.Marshal(gotObj)
					resultString = string(gotBytes)
				}
			} else {
				const api = "http://127.0.0.1:5000/api/tool/list_tables/invoke"
				resp, body := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
				if resp.StatusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(body))
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				var bodyWrapper struct {
					Result json.RawMessage `json:"result"`
				}
				if err := json.Unmarshal(body, &bodyWrapper); err != nil {
					t.Fatalf("error decoding response wrapper: %v", err)
				}

				if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
					resultString = string(bodyWrapper.Result)
				}
			}

			var got any
			if tc.isSimple {
				var tables []tableInfo
				if err := json.Unmarshal([]byte(resultString), &tables); err != nil && resultString != "null" {
					t.Fatalf("failed to unmarshal outer JSON array into []tableInfo: %v", err)
				}
				details := []map[string]any{}
				for _, table := range tables {
					var d map[string]any
					if err := json.Unmarshal([]byte(table.ObjectDetails), &d); err != nil {
						t.Fatalf("failed to unmarshal nested ObjectDetails string: %v", err)
					}
					details = append(details, d)
				}
				got = details
			} else {
				var tables []tableInfo
				if err := json.Unmarshal([]byte(resultString), &tables); err != nil && resultString != "null" {
					t.Fatalf("failed to unmarshal outer JSON array into []tableInfo: %v", err)
				}
				details := []objectDetails{}
				for _, table := range tables {
					var d objectDetails
					if err := json.Unmarshal([]byte(table.ObjectDetails), &d); err != nil {
						t.Fatalf("failed to unmarshal nested ObjectDetails string: %v", err)
					}
					details = append(details, d)
				}
				got = details
			}

			opts := []cmp.Option{
				cmpopts.SortSlices(func(a, b objectDetails) bool { return a.ObjectName < b.ObjectName }),
				cmpopts.SortSlices(func(a, b column) bool { return a.ColumnName < b.ColumnName }),
				cmpopts.SortSlices(func(a, b map[string]any) bool { return a["name"].(string) < b["name"].(string) }),
			}

			// Checking only the current database where the test tables are created to avoid brittle tests.
			if tc.isAllTables {
				filteredGot := []objectDetails{}
				if got != nil {
					for _, item := range got.([]objectDetails) {
						if item.SchemaName == databaseName {
							filteredGot = append(filteredGot, item)
						}
					}
				}
				got = filteredGot
			}

			if diff := cmp.Diff(tc.want, got, opts...); diff != "" {
				t.Errorf("Unexpected result: got %#v, want: %#v", got, tc.want)
			}
		})
	}
}

func RunMySQLListActiveQueriesTest(t *testing.T, ctx context.Context, pool *sql.DB, opts ...ToolExecOption) {
	config := &ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	type queryListDetails struct {
		ProcessId       any    `json:"process_id"`
		Query           string `json:"query"`
		TrxStarted      any    `json:"trx_started"`
		TrxDuration     any    `json:"trx_duration_seconds"`
		TrxWaitDuration any    `json:"trx_wait_duration_seconds"`
		QueryTime       any    `json:"query_time"`
		TrxState        string `json:"trx_state"`
		ProcessState    string `json:"process_state"`
		User            string `json:"user"`
		TrxRowsLocked   any    `json:"trx_rows_locked"`
		TrxRowsModified any    `json:"trx_rows_modified"`
		Db              string `json:"db"`
	}

	testUUID := strings.ReplaceAll(uuid.New().String(), "-", "")[:8]
	tc2QueryTag := fmt.Sprintf("test_mysql_sleep_tc2_%s", testUUID)
	tc3QueryTag := fmt.Sprintf("test_mysql_sleep_tc3_%s", testUUID)
	tc4QueryTag := fmt.Sprintf("test_mysql_sleep_tc4_%s", testUUID)

	makeQuery := func(sleepSecs int, tag string) string {
		return fmt.Sprintf("SELECT sleep(%d), '%s'", sleepSecs, tag)
	}

	makeWanted := func(query string) queryListDetails {
		return queryListDetails{
			ProcessId:       any(nil),
			Query:           query,
			TrxStarted:      any(nil),
			TrxDuration:     any(nil),
			TrxWaitDuration: any(nil),
			QueryTime:       any(nil),
			TrxState:        "",
			ProcessState:    "User sleep",
			User:            "",
			TrxRowsLocked:   any(nil),
			TrxRowsModified: any(nil),
			Db:              "",
		}
	}

	tc3Wanted := makeWanted(makeQuery(10, tc3QueryTag))
	tc4Wanted := makeWanted(makeQuery(10, tc4QueryTag))

	invokeTcs := []struct {
		name                string
		requestBody         io.Reader
		queryTag            string
		clientSleepSecs     int
		numClients          int
		waitSecsBeforeCheck int
		wantStatusCode      int
		want                []queryListDetails
	}{
		{
			name:                "invoke list_active_queries when the system is idle",
			requestBody:         bytes.NewBufferString(`{}`),
			queryTag:            "",
			clientSleepSecs:     0,
			waitSecsBeforeCheck: 0,
			wantStatusCode:      http.StatusOK,
			want:                []queryListDetails{},
		},
		{
			name:                "invoke list_active_queries when there is 1 ongoing but lower than the threshold",
			requestBody:         bytes.NewBufferString(`{"min_duration_secs": 100}`),
			queryTag:            tc2QueryTag,
			clientSleepSecs:     3,
			waitSecsBeforeCheck: 1,
			wantStatusCode:      http.StatusOK,
			want:                []queryListDetails{},
		},
		{
			// The threshold has to sit below waitSecsBeforeCheck: the wait starts
			// before the query does, and processlist.time truncates to whole
			// seconds, so an equal threshold loses the row to rounding.
			name:                "invoke list_active_queries when 1 ongoing query should show up",
			requestBody:         bytes.NewBufferString(`{"min_duration_secs": 3}`),
			queryTag:            tc3QueryTag,
			clientSleepSecs:     10,
			waitSecsBeforeCheck: 5,
			wantStatusCode:      http.StatusOK,
			want:                []queryListDetails{tc3Wanted},
		},
		{
			name:                "invoke list_active_queries when 2 ongoing query should show up",
			requestBody:         bytes.NewBufferString(`{"min_duration_secs": 2}`),
			queryTag:            tc4QueryTag,
			clientSleepSecs:     10,
			numClients:          2,
			waitSecsBeforeCheck: 3,
			wantStatusCode:      http.StatusOK,
			want:                []queryListDetails{tc4Wanted, tc4Wanted},
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var wg sync.WaitGroup
			defer wg.Wait()

			queryCtx, cancel := context.WithCancel(ctx)
			defer cancel()

			if tc.clientSleepSecs > 0 {
				numClients := tc.numClients
				if numClients == 0 {
					numClients = 1
				}

				for i := 0; i < numClients; i++ {
					wg.Add(1)

					go func() {
						defer wg.Done()

						err := pool.PingContext(queryCtx)
						if err != nil {
							if !errors.Is(err, context.Canceled) {
								t.Errorf("unable to connect to test database: %s", err)
							}
							return
						}
						query := makeQuery(tc.clientSleepSecs, tc.queryTag)
						_, err = pool.ExecContext(queryCtx, query)
						if err != nil && !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "interrupted") {
							t.Errorf("Executing 'SELECT sleep' failed: %s", err)
						}
					}()
				}
			}

			if tc.waitSecsBeforeCheck > 0 {
				time.Sleep(time.Duration(tc.waitSecsBeforeCheck) * time.Second)
			}

			var resultString string

			if config.isMCP {
				reqBytes, _ := io.ReadAll(tc.requestBody)
				var args map[string]any
				if len(reqBytes) > 0 {
					_ = json.Unmarshal(reqBytes, &args)
				}
				if args == nil {
					args = make(map[string]any)
				}

				statusCode, mcpResp, err := InvokeMCPTool(t, "list_active_queries", args, nil)
				if statusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, err: %v", statusCode, tc.wantStatusCode, err)
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				gotObj := getMCPResultText(t, mcpResp)
				if len(gotObj) == 0 {
					resultString = "null"
				} else {
					gotBytes, _ := json.Marshal(gotObj)
					resultString = string(gotBytes)
				}
			} else {
				const api = "http://127.0.0.1:5000/api/tool/list_active_queries/invoke"
				resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
				if resp.StatusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				var bodyWrapper struct {
					Result json.RawMessage `json:"result"`
				}
				if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
					t.Fatalf("error decoding response wrapper: %v", err)
				}

				if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
					resultString = string(bodyWrapper.Result)
				}
			}

			details := []queryListDetails{}
			if resultString != "null" {
				if err := json.Unmarshal([]byte(resultString), &details); err != nil {
					t.Fatalf("failed to unmarshal nested ObjectDetails string: %v", err)
				}
			}

			// Filter to active queries that belong to this test case.
			got := details
			if tc.queryTag != "" {
				got = []queryListDetails{}
				for _, d := range details {
					if strings.Contains(d.Query, tc.queryTag) {
						got = append(got, d)
					}
				}
			}

			if diff := cmp.Diff(tc.want, got, cmp.Comparer(func(a, b queryListDetails) bool {
				return strings.TrimSuffix(strings.TrimSpace(a.Query), ";") == strings.TrimSuffix(strings.TrimSpace(b.Query), ";") && a.ProcessState == b.ProcessState
			})); diff != "" {
				t.Errorf("Unexpected active queries (-want +got):\n%s", diff)
			}
		})
	}
}

func RunMySQLListTablesMissingUniqueIndexes(t *testing.T, ctx context.Context, pool *sql.DB, databaseName string, opts ...ToolExecOption) {
	config := &ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	type listDetails struct {
		TableSchema string `json:"table_schema"`
		TableName   string `json:"table_name"`
	}

	nonUniqueKeyTableName := "t03_non_unqiue_key_table"
	noKeyTableName := "t04_no_key_table"
	nonUniqueKeyTableWant := listDetails{
		TableSchema: databaseName,
		TableName:   nonUniqueKeyTableName,
	}
	noKeyTableWant := listDetails{
		TableSchema: databaseName,
		TableName:   noKeyTableName,
	}

	invokeTcs := []struct {
		name                 string
		requestBody          io.Reader
		newTableName         string
		newTablePrimaryKey   bool
		newTableUniqueKey    bool
		newTableNonUniqueKey bool
		wantStatusCode       int
		want                 any
	}{
		{
			name:                 "invoke list_tables_missing_unique_indexes when nothing to be found",
			requestBody:          bytes.NewBufferString(`{}`),
			newTableName:         "",
			newTablePrimaryKey:   false,
			newTableUniqueKey:    false,
			newTableNonUniqueKey: false,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes pk table will not show",
			requestBody:          bytes.NewBufferString(`{}`),
			newTableName:         "t01",
			newTablePrimaryKey:   true,
			newTableUniqueKey:    false,
			newTableNonUniqueKey: false,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes uk table will not show",
			requestBody:          bytes.NewBufferString(`{}`),
			newTableName:         "t02",
			newTablePrimaryKey:   false,
			newTableUniqueKey:    true,
			newTableNonUniqueKey: false,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes non-unique key only table will show",
			requestBody:          bytes.NewBufferString(`{}`),
			newTableName:         nonUniqueKeyTableName,
			newTablePrimaryKey:   false,
			newTableUniqueKey:    false,
			newTableNonUniqueKey: true,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{nonUniqueKeyTableWant},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes table with no key at all will show",
			requestBody:          bytes.NewBufferString(`{}`),
			newTableName:         noKeyTableName,
			newTablePrimaryKey:   false,
			newTableUniqueKey:    false,
			newTableNonUniqueKey: false,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{nonUniqueKeyTableWant, noKeyTableWant},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes table w/ both pk & uk will not show",
			requestBody:          bytes.NewBufferString(`{}`),
			newTableName:         "t05",
			newTablePrimaryKey:   true,
			newTableUniqueKey:    true,
			newTableNonUniqueKey: false,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{nonUniqueKeyTableWant, noKeyTableWant},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes table w/ uk & nk will not show",
			requestBody:          bytes.NewBufferString(`{}`),
			newTableName:         "t06",
			newTablePrimaryKey:   false,
			newTableUniqueKey:    true,
			newTableNonUniqueKey: true,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{nonUniqueKeyTableWant, noKeyTableWant},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes table w/ pk & nk will not show",
			requestBody:          bytes.NewBufferString(`{}`),
			newTableName:         "t07",
			newTablePrimaryKey:   true,
			newTableUniqueKey:    false,
			newTableNonUniqueKey: true,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{nonUniqueKeyTableWant, noKeyTableWant},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes with a non-exist database, nothing to show",
			requestBody:          bytes.NewBufferString(`{"table_schema": "non-exist-database"}`),
			newTableName:         "",
			newTablePrimaryKey:   false,
			newTableUniqueKey:    false,
			newTableNonUniqueKey: false,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes with the right database, show everything",
			requestBody:          bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s"}`, databaseName)),
			newTableName:         "",
			newTablePrimaryKey:   false,
			newTableUniqueKey:    false,
			newTableNonUniqueKey: false,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{nonUniqueKeyTableWant, noKeyTableWant},
		},
		{
			name:                 "invoke list_tables_missing_unique_indexes with limited output",
			requestBody:          bytes.NewBufferString(`{"limit": 1}`),
			newTableName:         "",
			newTablePrimaryKey:   false,
			newTableUniqueKey:    false,
			newTableNonUniqueKey: false,
			wantStatusCode:       http.StatusOK,
			want:                 []listDetails{nonUniqueKeyTableWant},
		},
	}

	createTableHelper := func(t *testing.T, tableName, databaseName string, primaryKey, uniqueKey, nonUniqueKey bool, ctx context.Context, pool *sql.DB) func() {
		var stmt strings.Builder
		fmt.Fprintf(&stmt, "CREATE TABLE %s (", tableName)
		stmt.WriteString("c1 INT")
		if primaryKey {
			stmt.WriteString(" PRIMARY KEY")
		}
		stmt.WriteString(", c2 INT, c3 CHAR(8)")
		if uniqueKey {
			stmt.WriteString(", UNIQUE(c2)")
		}
		if nonUniqueKey {
			stmt.WriteString(", INDEX(c3)")
		}
		stmt.WriteString(")")

		if _, err := pool.ExecContext(ctx, stmt.String()); err != nil {
			t.Fatalf("failed executing %s: %v", stmt.String(), err)
		}

		return func() {
			if _, err := pool.ExecContext(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE %s", tableName)); err != nil {
				t.Errorf("failed to drop table %s: %v", tableName, err)
			}
		}
	}

	var cleanups []func()
	defer func() {
		for i := len(cleanups) - 1; i >= 0; i-- {
			cleanups[i]()
		}
	}()

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			if tc.newTableName != "" {
				cleanup := createTableHelper(t, tc.newTableName, databaseName, tc.newTablePrimaryKey, tc.newTableUniqueKey, tc.newTableNonUniqueKey, ctx, pool)
				cleanups = append(cleanups, cleanup)
			}

			var resultString string

			if config.isMCP {
				reqBytes, _ := io.ReadAll(tc.requestBody)
				var args map[string]any
				if len(reqBytes) > 0 {
					_ = json.Unmarshal(reqBytes, &args)
				}
				if args == nil {
					args = make(map[string]any)
				}

				statusCode, mcpResp, err := InvokeMCPTool(t, "list_tables_missing_unique_indexes", args, nil)
				if statusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, err: %v", statusCode, tc.wantStatusCode, err)
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				gotObj := getMCPResultText(t, mcpResp)
				if len(gotObj) == 0 {
					resultString = "null"
				} else {
					gotBytes, _ := json.Marshal(gotObj)
					resultString = string(gotBytes)
				}
			} else {
				const api = "http://127.0.0.1:5000/api/tool/list_tables_missing_unique_indexes/invoke"
				resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
				if resp.StatusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				var bodyWrapper struct {
					Result json.RawMessage `json:"result"`
				}
				if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
					t.Fatalf("error decoding response wrapper: %v", err)
				}

				if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
					resultString = string(bodyWrapper.Result)
				}
			}

			var got any
			details := []listDetails{}
			if resultString != "null" {
				if err := json.Unmarshal([]byte(resultString), &details); err != nil {
					t.Fatalf("failed to unmarshal nested listDetails string: %v", err)
				}
			}
			got = details

			if diff := cmp.Diff(tc.want, got, cmp.Comparer(func(a, b listDetails) bool {
				return a.TableSchema == b.TableSchema && a.TableName == b.TableName
			})); diff != "" {
				t.Errorf("Unexpected result: got %#v, want: %#v", got, tc.want)
			}
		})
	}
}

func RunMySQLListTableStatsTest(t *testing.T, ctx context.Context, pool *sql.DB, databaseName string, tableNameParam string, tableNameAuth string, opts ...ToolExecOption) {
	config := &ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}
	type tableStatsDetails struct {
		TableSchema    string `json:"table_schema"`
		TableName      string `json:"table_name"`
		DataSize       any    `json:"size_MB"`
		RowCount       any    `json:"row_count"`
		TotalLatency   any    `json:"total_latency_secs"`
		RowsFetched    any    `json:"rows_fetched"`
		RowsInserted   any    `json:"rows_inserted"`
		RowsUpdated    any    `json:"rows_updated"`
		RowsDeleted    any    `json:"rows_deleted"`
		IOReads        any    `json:"io_reads"`
		IOReadLatency  any    `json:"io_read_latency"`
		IOWriteLatency any    `json:"io_write_latency"`
		IOMiscLatency  any    `json:"io_misc_latency"`
	}

	paramTableEntryWanted := tableStatsDetails{
		TableSchema:    databaseName,
		TableName:      tableNameParam,
		DataSize:       any(nil),
		RowCount:       any(nil),
		TotalLatency:   any(nil),
		RowsFetched:    any(nil),
		RowsInserted:   any(nil),
		RowsUpdated:    any(nil),
		RowsDeleted:    any(nil),
		IOReads:        any(nil),
		IOReadLatency:  any(nil),
		IOWriteLatency: any(nil),
		IOMiscLatency:  any(nil),
	}

	authTableEntryWanted := tableStatsDetails{
		TableSchema:    databaseName,
		TableName:      tableNameAuth,
		DataSize:       any(nil),
		RowCount:       any(nil),
		TotalLatency:   any(nil),
		RowsFetched:    any(nil),
		RowsInserted:   any(nil),
		RowsUpdated:    any(nil),
		RowsDeleted:    any(nil),
		IOReads:        any(nil),
		IOReadLatency:  any(nil),
		IOWriteLatency: any(nil),
		IOMiscLatency:  any(nil),
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           any
	}{
		{
			name:           "invoke list_table_stats with no arguments, expected 2 results",
			requestBody:    bytes.NewBufferString(`{}`),
			wantStatusCode: http.StatusOK,
			want:           []tableStatsDetails{paramTableEntryWanted, authTableEntryWanted},
		},
		{
			name:           "invoke list_table_stats with schema other than connected to, expected log error and nil results",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s"}`, "somerandomdb_xyx")),
			wantStatusCode: http.StatusInternalServerError,
			want:           []tableStatsDetails{},
		},
		{
			name:           "invoke list_table_stats on 1 database and all tables, expected to have 2 result",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s"}`, databaseName)),
			wantStatusCode: http.StatusOK,
			want:           []tableStatsDetails{paramTableEntryWanted, authTableEntryWanted},
		},
		{
			name:           "invoke list_table_stats on 1 database and 1 specific table name, expected to have 1 result",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s", "table_name": "%s"}`, databaseName, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []tableStatsDetails{authTableEntryWanted},
		},
		{
			name:           "invoke list_table_stats on 1 non-exist table on 1 database, expected to have 0 result",
			requestBody:    bytes.NewBufferString(`{"table_name": "non_existent_table"}`),
			wantStatusCode: http.StatusOK,
			want:           []tableStatsDetails{},
		},
	}

	// Generating additional stats for tableNameParam
	for i := 0; i < 3; i++ {
		selectStmt := fmt.Sprintf("SELECT * FROM %s", tableNameParam)
		if _, err := pool.ExecContext(ctx, selectStmt); err != nil {
			t.Logf("warning: unable to execute select: %v", err)
		}
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var resultString string

			if config.isMCP {
				reqBytes, _ := io.ReadAll(tc.requestBody)
				var args map[string]any
				if len(reqBytes) > 0 {
					_ = json.Unmarshal(reqBytes, &args)
				}
				if args == nil {
					args = make(map[string]any)
				}

				statusCode, mcpResp, err := InvokeMCPTool(t, "list_table_stats", args, nil)

				// For the error case (expecting 500 in REST), we expect 200 OK in MCP with IsError=true
				expectedStatus := tc.wantStatusCode
				if tc.wantStatusCode == http.StatusInternalServerError {
					expectedStatus = http.StatusOK
				}

				if statusCode != expectedStatus {
					t.Fatalf("wrong status code: got %d, want %d, err: %v", statusCode, expectedStatus, err)
				}

				if tc.wantStatusCode == http.StatusInternalServerError {
					if !mcpResp.Result.IsError {
						t.Fatalf("expected error result for list_table_stats")
					}
					return // Error case, no need to check result body
				}

				if mcpResp.Result.IsError {
					t.Fatalf("list_table_stats returned error result: %v", mcpResp.Result)
				}

				gotObj := getMCPResultText(t, mcpResp)
				if len(gotObj) == 0 {
					resultString = "null"
				} else {
					gotBytes, _ := json.Marshal(gotObj)
					resultString = string(gotBytes)
				}
			} else {
				const api = "http://127.0.0.1:5000/api/tool/list_table_stats/invoke"
				resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
				if resp.StatusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				var bodyWrapper struct {
					Result json.RawMessage `json:"result"`
				}
				if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
					t.Fatalf("error decoding response wrapper: %v", err)
				}

				if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
					resultString = string(bodyWrapper.Result)
				}
			}

			var got any
			details := []tableStatsDetails{}
			if resultString != "null" {
				if err := json.Unmarshal([]byte(resultString), &details); err != nil {
					t.Fatalf("failed to unmarshal outer JSON array into []tableInfo: %v", err)
				}
			}
			got = details

			// The tool sorts on measured total_latency, so which table comes back
			// first varies between runs. Compare as a set.
			if diff := cmp.Diff(tc.want, got,
				cmpopts.SortSlices(func(a, b tableStatsDetails) bool {
					if a.TableSchema != b.TableSchema {
						return a.TableSchema < b.TableSchema
					}
					return a.TableName < b.TableName
				}),
				cmp.Comparer(func(a, b tableStatsDetails) bool {
					return a.TableSchema == b.TableSchema && a.TableName == b.TableName
				})); diff != "" {
				t.Errorf("Unexpected result: got %#v, want: %#v", got, tc.want)
			}
		})
	}
}

func RunMySQLListTableFragmentationTest(t *testing.T, databaseName, tableNameParam, tableNameAuth string, opts ...ToolExecOption) {
	config := &ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	type tableFragmentationDetails struct {
		TableSchema             string `json:"table_schema"`
		TableName               string `json:"table_name"`
		DataSize                any    `json:"data_size"`
		IndexSize               any    `json:"index_size"`
		DataFree                any    `json:"data_free"`
		FragmentationPercentage any    `json:"fragmentation_percentage"`
	}

	paramTableEntryWanted := tableFragmentationDetails{
		TableSchema:             databaseName,
		TableName:               tableNameParam,
		DataSize:                any(nil),
		IndexSize:               any(nil),
		DataFree:                any(nil),
		FragmentationPercentage: any(nil),
	}
	authTableEntryWanted := tableFragmentationDetails{
		TableSchema:             databaseName,
		TableName:               tableNameAuth,
		DataSize:                any(nil),
		IndexSize:               any(nil),
		DataFree:                any(nil),
		FragmentationPercentage: any(nil),
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		want           any
	}{
		{
			name:           "invoke list_table_fragmentation on all, no data_free threshold, expected to have 2 results",
			requestBody:    bytes.NewBufferString(`{"data_free_threshold_bytes": 0}`),
			wantStatusCode: http.StatusOK,
			want:           []tableFragmentationDetails{authTableEntryWanted, paramTableEntryWanted},
		},
		{
			name:           "invoke list_table_fragmentation on all, no data_free threshold, limit to 1, expected to have 1 results",
			requestBody:    bytes.NewBufferString(`{"data_free_threshold_bytes": 0, "limit": 1}`),
			wantStatusCode: http.StatusOK,
			want:           []tableFragmentationDetails{authTableEntryWanted},
		},
		{
			name:           "invoke list_table_fragmentation on all databases and 1 specific table name, no data_free threshold, expected to have 1 result",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_name": "%s","data_free_threshold_bytes": 0}`, tableNameAuth)),
			wantStatusCode: http.StatusOK,
			want:           []tableFragmentationDetails{authTableEntryWanted},
		},
		{
			name:           "invoke list_table_fragmentation on 1 database and 1 specific table name, no data_free threshold, expected to have 1 result",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s", "table_name": "%s", "data_free_threshold_bytes": 0}`, databaseName, tableNameParam)),
			wantStatusCode: http.StatusOK,
			want:           []tableFragmentationDetails{paramTableEntryWanted},
		},
		{
			name:           "invoke list_table_fragmentation on 1 database and 1 specific table name, high data_free threshold, expected to have 0 result",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s", "table_name": "%s", "data_free_threshold_bytes": 1000000000}`, databaseName, tableNameParam)),
			wantStatusCode: http.StatusOK,
			want:           []tableFragmentationDetails{},
		},
		{
			name:           "invoke list_table_fragmentation on 1 non-exist database, no data_free threshold, expected to have 0 result",
			requestBody:    bytes.NewBufferString(`{"table_schema": "non_existent_database", "data_free_threshold_bytes": 0}`),
			wantStatusCode: http.StatusOK,
			want:           []tableFragmentationDetails{},
		},
		{
			name:           "invoke list_table_fragmentation on 1 non-exist table, no data_free threshold, expected to have 0 result",
			requestBody:    bytes.NewBufferString(`{"table_name": "non_existent_table", "data_free_threshold_bytes": 0}`),
			wantStatusCode: http.StatusOK,
			want:           []tableFragmentationDetails{},
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var resultString string

			if config.isMCP {
				reqBytes, _ := io.ReadAll(tc.requestBody)
				var args map[string]any
				if len(reqBytes) > 0 {
					_ = json.Unmarshal(reqBytes, &args)
				}
				if args == nil {
					args = make(map[string]any)
				}

				statusCode, mcpResp, err := InvokeMCPTool(t, "list_table_fragmentation", args, nil)
				if statusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, err: %v", statusCode, tc.wantStatusCode, err)
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				gotObj := getMCPResultText(t, mcpResp)
				if len(gotObj) == 0 {
					resultString = "null"
				} else {
					gotBytes, _ := json.Marshal(gotObj)
					resultString = string(gotBytes)
				}
			} else {
				const api = "http://127.0.0.1:5000/api/tool/list_table_fragmentation/invoke"
				resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
				if resp.StatusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				var bodyWrapper struct {
					Result json.RawMessage `json:"result"`
				}
				if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
					t.Fatalf("error decoding response wrapper: %v", err)
				}

				if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
					resultString = string(bodyWrapper.Result)
				}
			}

			var got any
			details := []tableFragmentationDetails{}
			if resultString != "null" {
				if err := json.Unmarshal([]byte(resultString), &details); err != nil {
					t.Fatalf("failed to unmarshal outer JSON array into []tableInfo: %v", err)
				}
			}
			got = details

			if diff := cmp.Diff(tc.want, got, cmp.Comparer(func(a, b tableFragmentationDetails) bool {
				return a.TableSchema == b.TableSchema && a.TableName == b.TableName
			})); diff != "" {
				t.Errorf("Unexpected result: got %#v, want: %#v", got, tc.want)
			}
		})
	}
}

func RunMySQLGetQueryPlanTest(t *testing.T, ctx context.Context, pool *sql.DB, databaseName, tableNameParam string, opts ...ToolExecOption) {
	config := &ToolExecConfig{}
	for _, opt := range opts {
		opt(config)
	}

	query := fmt.Sprintf("SELECT * FROM %s", tableNameParam)

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		checkResult    func(t *testing.T, result any)
	}{
		{
			name:           "invoke get_query_plan with valid query",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"sql_statement": "%s"}`, query)),
			wantStatusCode: http.StatusOK,
			checkResult: func(t *testing.T, result any) {
				resultMap, ok := result.(map[string]any)
				if !ok {
					t.Fatalf("result should be a map, got %T", result)
				}
				if _, ok := resultMap["query_block"]; !ok {
					t.Errorf("result should contain 'query_block', got %v", resultMap)
				}
			},
		},
		{
			name:           "invoke get_query_plan with invalid query",
			requestBody:    bytes.NewBufferString(`{"sql_statement": "SELECT * FROM non_existent_table"}`),
			wantStatusCode: http.StatusOK,
			checkResult:    nil,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			var resultString string

			if config.isMCP {
				reqBytes, _ := io.ReadAll(tc.requestBody)
				var args map[string]any
				if len(reqBytes) > 0 {
					_ = json.Unmarshal(reqBytes, &args)
				}
				if args == nil {
					args = make(map[string]any)
				}

				statusCode, mcpResp, err := InvokeMCPTool(t, "get_query_plan", args, nil)
				if statusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, err: %v", statusCode, tc.wantStatusCode, err)
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				gotObj := getMCPResultText(t, mcpResp)
				if len(gotObj) == 0 {
					resultString = "null"
				} else {
					if len(gotObj) == 1 {
						if str, ok := gotObj[0].(string); ok {
							resultString = str
						} else {
							gotBytes, _ := json.Marshal(gotObj[0])
							resultString = string(gotBytes)
						}
					} else {
						gotBytes, _ := json.Marshal(gotObj)
						resultString = string(gotBytes)
					}
				}
			} else {
				const api = "http://127.0.0.1:5000/api/tool/get_query_plan/invoke"
				resp, respBytes := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
				if resp.StatusCode != tc.wantStatusCode {
					t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBytes))
				}
				if tc.wantStatusCode != http.StatusOK {
					return
				}

				var bodyWrapper map[string]json.RawMessage

				if err := json.Unmarshal(respBytes, &bodyWrapper); err != nil {
					t.Fatalf("error parsing response wrapper: %s, body: %s", err, string(respBytes))
				}

				resultJSON, ok := bodyWrapper["result"]
				if !ok {
					t.Fatal("unable to find 'result' in response body")
				}

				if err := json.Unmarshal(resultJSON, &resultString); err != nil {
					if string(resultJSON) == "null" {
						resultString = "null"
					} else {
						t.Fatalf("'result' is not a JSON-encoded string: %s", err)
					}
				}
			}

			if tc.checkResult != nil {
				var got map[string]any
				if err := json.Unmarshal([]byte(resultString), &got); err != nil && resultString != "null" {
					t.Fatalf("failed to unmarshal actual result string: %v", err)
				}
				tc.checkResult(t, got)
			}
		})
	}
}

func RunMySQLShowQueryStats(t *testing.T, ctx context.Context, pool *sql.DB, databaseName string) {

	// Generating stats for query
	selectStmt := "SELECT 1"
	if _, err := pool.ExecContext(ctx, selectStmt); err != nil {
		t.Logf("warning: unable to execute select: %v", err)
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		wantError      string
	}{
		{
			name:           "list query stats with default limit",
			requestBody:    bytes.NewBufferString(`{}`),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list query stats with custom limit",
			requestBody:    bytes.NewBufferString(`{"limit": 1}`),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list query stats for specific database",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s"}`, databaseName)),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list query stats with with schema other than connected schema, expected error",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s"}`, "some_random_db_name_foo_bar")),
			wantStatusCode: http.StatusOK,
			wantError:      "SCHEMA_MATCH_FAILED",
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/show_query_stats/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			if tc.wantError != "" {
				var got map[string]any
				if err := json.Unmarshal([]byte(resultString), &got); err != nil {
					t.Fatalf("failed to unmarshal error result: %v, resultString: %s", err, resultString)
				}
				errVal, ok := got["error"].(string)
				if !ok {
					t.Fatalf("expected error key in response, got: %v", got)
				}
				if !strings.Contains(errVal, tc.wantError) {
					t.Fatalf("expected error containing %q, got: %q", tc.wantError, errVal)
				}
				return
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v, resultString: %s", err, resultString)
			}

			//verify if results have expected fields
			if len(got) > 0 {
				requiredFields := []string{"db", "query", "execution_count", "total_latency_ms", "average_latency_ms", "max_latency_ms", "total_rows_sent", "total_rows_examined", "full_table_scan_count", "inefficient_index_used_count", "last_executed"}
				for _, field := range requiredFields {
					if _, ok := got[0][field]; !ok {
						t.Errorf("missing expected field: %s in result: %v", field, got[0])
					}
				}
			}

		})
	}
}

func RunMySQLListAllLocks(t *testing.T, ctx context.Context, pool *sql.DB, databaseName string) {

	// Create table and lock the table for test
	testTableName := "test_list_table_stats_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createTableStmt := fmt.Sprintf(`
        CREATE TABLE %s (
            id INT NOT NULL AUTO_INCREMENT PRIMARY KEY,
            name VARCHAR(100),
            email VARCHAR(100)
        )
    `, testTableName)

	if _, err := pool.ExecContext(ctx, createTableStmt); err != nil {
		t.Fatalf("unable to create test table: %s", err)
	}
	defer func() {
		dropTableStmt := fmt.Sprintf("DROP TABLE IF EXISTS %s", testTableName)
		if _, err := pool.ExecContext(ctx, dropTableStmt); err != nil {
			t.Logf("warning: unable to drop test table: %v", err)
		}
	}()

	// insert sample data
	insertStmt := fmt.Sprintf(`
        INSERT INTO %s (id, name, email) VALUES
        (1, 'Alice', 'alice@example.com')
    `, testTableName)

	if _, err := pool.ExecContext(ctx, insertStmt); err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	// Generating stats for query
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		tx, err := pool.BeginTx(ctx, nil)
		if err != nil {
			t.Logf("warning: unable to begin transaction: %v", err)
			return
		}
		defer func() {
			if err := tx.Rollback(); err != nil {
				t.Logf("warning: unable to rollback transaction: %v", err)
			}
		}()

		selectStmt := fmt.Sprintf("SELECT * FROM %s WHERE id = 1 FOR UPDATE", testTableName)
		if _, err := tx.ExecContext(ctx, selectStmt); err != nil {
			t.Logf("warning: unable to execute select: %v", err)
			return
		}
		// Hold the lock for a while to allow tests to run
		time.Sleep(10 * time.Second)
	}()
	// Give it a moment to actually acquire the lock
	time.Sleep(1 * time.Second)

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		wantError      string
	}{
		{
			name:           "list all locks with default limit",
			requestBody:    bytes.NewBufferString(`{}`),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list all locks with custom limit",
			requestBody:    bytes.NewBufferString(`{"limit": 1}`),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list all locks for specific database",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s"}`, databaseName)),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list all locks with with schema other than connected schema, expected error",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_schema": "%s"}`, "some_random_db_name_foo_bar")),
			wantStatusCode: http.StatusOK,
			wantError:      "SCHEMA_MATCH_FAILED",
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_all_locks/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			if tc.wantError != "" {
				var got map[string]any
				if err := json.Unmarshal([]byte(resultString), &got); err != nil {
					t.Fatalf("failed to unmarshal error result: %v, resultString: %s", err, resultString)
				}
				errVal, ok := got["error"].(string)
				if !ok {
					t.Fatalf("expected error key in response, got: %v", got)
				}
				if !strings.Contains(errVal, tc.wantError) {
					t.Fatalf("expected error containing %q, got: %q", tc.wantError, errVal)
				}
				return
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v, resultString: %s", err, resultString)
			}

			//verify if results have expected fields
			if len(got) > 0 {
				requiredFields := []string{"thread_id", "process_id", "table_schema", "table_name", "lock_type", "lock_mode", "lock_status", "transaction_state", "current_operation", "query"}
				for _, field := range requiredFields {
					if _, ok := got[0][field]; !ok {
						t.Errorf("missing expected field: %s in result: %v", field, got[0])
					}
				}
			}

		})
	}
	wg.Wait()
}

// RunMSSQLListTablesTest run tests againsts the mssql-list-tables tools.
func RunMSSQLListTablesTest(t *testing.T, tableNameParam, tableNameAuth string) {
	// TableNameParam columns to construct want.
	const paramTableColumns = `[
        {"column_name": "id", "data_type": "INT", "column_ordinal_position": 1, "is_not_nullable": true},
        {"column_name": "name", "data_type": "VARCHAR(255)", "column_ordinal_position": 2, "is_not_nullable": false}
    ]`

	// TableNameAuth columns to construct want
	const authTableColumns = `[
		{"column_name": "id", "data_type": "INT", "column_ordinal_position": 1, "is_not_nullable": true},
		{"column_name": "name", "data_type": "VARCHAR(255)", "column_ordinal_position": 2, "is_not_nullable": false},
		{"column_name": "email", "data_type": "VARCHAR(255)", "column_ordinal_position": 3, "is_not_nullable": false}
    ]`

	const (
		// Template to construct detailed output want.
		detailedObjectTemplate = `{
            "schema_name": "dbo",
            "object_name": "%[1]s",
            "object_details": {
                "owner": "dbo",
                "triggers": [],
                "columns": %[2]s,
                "object_name": "%[1]s",
                "object_type": "TABLE",
                "schema_name": "dbo"
            }
        }`

		// Template to construct simple output want
		simpleObjectTemplate = `{"object_name":"%s", "schema_name":"dbo", "object_details":{"name":"%s"}}`
	)

	// Helper to build json for detailed want
	getDetailedWant := func(tableName, columnJSON string) string {
		return fmt.Sprintf(detailedObjectTemplate, tableName, columnJSON)
	}

	// Helper to build template for simple want
	getSimpleWant := func(tableName string) string {
		return fmt.Sprintf(simpleObjectTemplate, tableName, tableName)
	}

	invokeTcs := []struct {
		name           string
		api            string
		requestBody    string
		wantStatusCode int
		want           string
		isAllTables    bool
		isAgentErr     bool
	}{
		{
			name:           "invoke list_tables for all tables detailed output",
			api:            "http://127.0.0.1:5000/api/tool/list_tables/invoke",
			requestBody:    `{"table_names": ""}`,
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s,%s]", getDetailedWant(tableNameAuth, authTableColumns), getDetailedWant(tableNameParam, paramTableColumns)),
			isAllTables:    true,
		},
		{
			name:           "invoke list_tables for all tables simple output",
			api:            "http://127.0.0.1:5000/api/tool/list_tables/invoke",
			requestBody:    `{"table_names": "", "output_format": "simple"}`,
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s,%s]", getSimpleWant(tableNameAuth), getSimpleWant(tableNameParam)),
			isAllTables:    true,
		},
		{
			name:           "invoke list_tables detailed output",
			api:            "http://127.0.0.1:5000/api/tool/list_tables/invoke",
			requestBody:    fmt.Sprintf(`{"table_names": "%s"}`, tableNameAuth),
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s]", getDetailedWant(tableNameAuth, authTableColumns)),
		},
		{
			name:           "invoke list_tables simple output",
			api:            "http://127.0.0.1:5000/api/tool/list_tables/invoke",
			requestBody:    fmt.Sprintf(`{"table_names": "%s", "output_format": "simple"}`, tableNameAuth),
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s]", getSimpleWant(tableNameAuth)),
		},
		{
			name:           "invoke list_tables with invalid output format",
			api:            "http://127.0.0.1:5000/api/tool/list_tables/invoke",
			requestBody:    `{"table_names": "", "output_format": "abcd"}`,
			wantStatusCode: http.StatusOK,
			isAgentErr:     true,
		},
		{
			name:           "invoke list_tables with malformed table_names parameter",
			api:            "http://127.0.0.1:5000/api/tool/list_tables/invoke",
			requestBody:    `{"table_names": 12345, "output_format": "detailed"}`,
			wantStatusCode: http.StatusOK,
			isAgentErr:     true,
		},
		{
			name:           "invoke list_tables with multiple table names",
			api:            "http://127.0.0.1:5000/api/tool/list_tables/invoke",
			requestBody:    fmt.Sprintf(`{"table_names": "%s,%s"}`, tableNameParam, tableNameAuth),
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s,%s]", getDetailedWant(tableNameAuth, authTableColumns), getDetailedWant(tableNameParam, paramTableColumns)),
		},
		{
			name:           "invoke list_tables with non-existent table",
			api:            "http://127.0.0.1:5000/api/tool/list_tables/invoke",
			requestBody:    `{"table_names": "non_existent_table"}`,
			wantStatusCode: http.StatusOK,
			want:           `[]`,
		},
		{
			name:           "invoke list_tables with one existing and one non-existent table",
			api:            "http://127.0.0.1:5000/api/tool/list_tables/invoke",
			requestBody:    fmt.Sprintf(`{"table_names": "%s,non_existent_table"}`, tableNameParam),
			wantStatusCode: http.StatusOK,
			want:           fmt.Sprintf("[%s]", getDetailedWant(tableNameParam, paramTableColumns)),
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, respBytes := RunRequest(t, http.MethodPost, tc.api, bytes.NewBuffer([]byte(tc.requestBody)), nil)

			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("response status code is not %d, got %d: %s", tc.wantStatusCode, resp.StatusCode, string(respBytes))
			}

			if tc.wantStatusCode == http.StatusOK {
				var bodyWrapper map[string]json.RawMessage

				if err := json.Unmarshal(respBytes, &bodyWrapper); err != nil {
					t.Fatalf("error parsing response wrapper: %s, body: %s", err, string(respBytes))
				}

				resultJSON, ok := bodyWrapper["result"]
				if !ok {
					t.Fatal("unable to find 'result' in response body")
				}

				var resultString string

				if tc.isAgentErr {
					return
				}

				if err := json.Unmarshal(resultJSON, &resultString); err != nil {
					if string(resultJSON) == "null" {
						resultString = "null"
					} else {
						t.Fatalf("'result' is not a JSON-encoded string: %s", err)
					}
				}

				var got, want []any

				if err := json.Unmarshal([]byte(resultString), &got); err != nil {
					t.Fatalf("failed to unmarshal actual result string: %v", err)
				}
				if err := json.Unmarshal([]byte(tc.want), &want); err != nil {
					t.Fatalf("failed to unmarshal expected want string: %v", err)
				}

				for _, item := range got {
					itemMap, ok := item.(map[string]any)
					if !ok {
						continue
					}

					detailsStr, ok := itemMap["object_details"].(string)
					if !ok {
						continue
					}

					var detailsMap map[string]any
					if err := json.Unmarshal([]byte(detailsStr), &detailsMap); err != nil {
						t.Fatalf("failed to unmarshal nested object_details string: %v", err)
					}

					// clean unpredictable fields
					delete(detailsMap, "constraints")
					delete(detailsMap, "indexes")

					itemMap["object_details"] = detailsMap
				}

				// Checking only the default dbo schema where the test tables are created to avoid brittle tests.
				if tc.isAllTables {
					var filteredGot []any
					for _, item := range got {
						if tableMap, ok := item.(map[string]interface{}); ok {
							if schema, ok := tableMap["schema_name"]; ok && schema == "dbo" {
								filteredGot = append(filteredGot, item)
							}
						}
					}
					got = filteredGot
				}

				sort.SliceStable(got, func(i, j int) bool {
					return fmt.Sprintf("%v", got[i]) < fmt.Sprintf("%v", got[j])
				})
				sort.SliceStable(want, func(i, j int) bool {
					return fmt.Sprintf("%v", want[i]) < fmt.Sprintf("%v", want[j])
				})

				if !reflect.DeepEqual(got, want) {
					gotJSON, _ := json.MarshalIndent(got, "", "  ")
					wantJSON, _ := json.MarshalIndent(want, "", "  ")
					t.Errorf("Unexpected result:\ngot:\n%s\n\nwant:\n%s", string(gotJSON), string(wantJSON))
				}
			}
		})
	}
}

func CreateAndLockPostgresTable(t *testing.T, ctx context.Context, pool *pgxpool.Pool, tableName string) func() {
	_, err := pool.Exec(ctx, fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (id INT PRIMARY KEY)", pgx.Identifier{tableName}.Sanitize()))
	if err != nil {
		t.Fatalf("unable to create table: %s", err)
	}

	tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		t.Fatalf("unable to create transaction: %s", err)
	}
	if _, err := tx.Exec(ctx, fmt.Sprintf("LOCK TABLE %s IN ACCESS EXCLUSIVE MODE", pgx.Identifier{tableName}.Sanitize())); err != nil {
		t.Fatalf("unable to acquire lock: %s", err)
	}

	return func() {
		if err := tx.Rollback(context.WithoutCancel(ctx)); err != nil {
			t.Fatalf("unable to rollback transaction: %s", err)
		}
		if _, err := pool.Exec(context.WithoutCancel(ctx), fmt.Sprintf("DROP TABLE IF EXISTS %s", pgx.Identifier{tableName}.Sanitize())); err != nil {
			t.Fatalf("unable to drop table: %s", err)
		}
	}
}

// RunPostgresListLocksTest runs tests for the postgres list-locks tool
func RunPostgresListLocksTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {

	// Create and lock a test table
	cleanup := CreateAndLockPostgresTable(t, ctx, pool, "test_postgres_list_locks_table")
	defer cleanup()

	type lockDetails struct {
		Pid     any    `json:"pid"`
		Usename string `json:"usename"`
		Query   string `json:"query"`
		TrxID   string `json:"trxid"`
		Locks   string `json:"locks"`
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		expectResults  bool
	}{
		{
			name:           "invoke list_locks with no arguments",
			requestBody:    bytes.NewBuffer([]byte(`{}`)),
			wantStatusCode: http.StatusOK,
			expectResults:  true,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_locks/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []lockDetails
			if resultString != "null" {
				if err := json.Unmarshal([]byte(resultString), &got); err != nil {
					t.Fatalf("failed to unmarshal result: %v, result string: %s", err, resultString)
				}
			}
			// Verify that we got results if expected
			if tc.expectResults && len(got) == 0 {
				t.Errorf("expected results but got none")
			}
		})
	}
}

// RunPostgresLongRunningTransactionsTest runs tests for the postgres long-running-transactions tool
func RunPostgresLongRunningTransactionsTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	type transactionDetails struct {
		Pid               any    `json:"pid"`
		Usename           string `json:"usename"`
		Database          string `json:"database"`
		ApplicationName   string `json:"application_name"`
		XactStart         any    `json:"xact_start"`
		XactDurationSecs  any    `json:"xact_duration_secs"`
		IdleInTransaction string `json:"idle_in_transaction"`
		Query             string `json:"query"`
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
	}{
		{
			name:           "invoke long_running_transactions with default threshold",
			requestBody:    bytes.NewBuffer([]byte(`{}`)),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "invoke long_running_transactions with custom threshold",
			requestBody:    bytes.NewBuffer([]byte(`{"min_transaction_duration_secs": 3600}`)),
			wantStatusCode: http.StatusOK,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/long_running_transactions/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []transactionDetails
			if resultString != "null" {
				if err := json.Unmarshal([]byte(resultString), &got); err != nil {
					t.Fatalf("failed to unmarshal result: %v, result string: %s", err, resultString)
				}
			}

			// Verify that if results exist, they have the expected structure
			for _, tx := range got {
				if tx.XactDurationSecs == nil {
					t.Errorf("transaction duration should not be null for long-running transactions")
				}
			}
		})
	}
}

// RunPostgresReplicationStatsTest runs tests for the postgres replication-stats tool
func RunPostgresReplicationStatsTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	type replicationStats struct {
		ClientAddr          string `json:"client_addr"`
		Username            string `json:"usename"`
		ApplicationName     string `json:"application_name"`
		ClientHostname      string `json:"client_hostname"`
		BackendStart        any    `json:"backend_start"`
		State               string `json:"state"`
		SyncState           string `json:"sync_state"`
		ReplyTime           any    `json:"reply_time"`
		FlushLsn            string `json:"flush_lsn"`
		ReplayLsn           string `json:"replay_lsn"`
		WriteLag            any    `json:"write_lag"`
		FlushLag            any    `json:"flush_lag"`
		ReplayLag           any    `json:"replay_lag"`
		SyncPriority        any    `json:"sync_priority"`
		ReplicationSlotName any    `json:"slot_name"`
		IsStreaming         bool   `json:"is_streaming"`
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
	}{
		{
			name:           "invoke replication_stats with no arguments",
			requestBody:    bytes.NewBuffer([]byte(`{}`)),
			wantStatusCode: http.StatusOK,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/replication_stats/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []replicationStats
			if resultString != "null" {
				if err := json.Unmarshal([]byte(resultString), &got); err != nil {
					t.Fatalf("failed to unmarshal result: %v, result string: %s", err, resultString)
				}
			}

			// Verify that if results exist, they have the expected structure
			for _, stat := range got {
				if stat.State == "" {
					t.Errorf("replication state should not be empty")
				}
			}
		})
	}
}

func RunPostgresGetColumnCardinalityTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	schemaName := "testschema_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	tableName := "table1_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	cleanup := setupPostgresSchemas(t, ctx, pool, schemaName)
	defer cleanup()

	// Create table with multiple columns
	createTableStmt := fmt.Sprintf(`
		CREATE TABLE %s.%s (
			id SERIAL PRIMARY KEY,
			email VARCHAR(100) UNIQUE,
			name VARCHAR(50),
			status VARCHAR(20),
			created_at TIMESTAMP
		)
	`, schemaName, tableName)

	if _, err := pool.Exec(ctx, createTableStmt); err != nil {
		t.Fatalf("unable to create table: %s", err)
	}

	// Insert larger sample data to ensure statistics are collected
	insertStmt := fmt.Sprintf(`
		INSERT INTO %s.%s (email, name, status, created_at) VALUES
		('user1@example.com', 'Alice', 'active', NOW()),
		('user2@example.com', 'Bob', 'inactive', NOW()),
		('user3@example.com', 'Charlie', 'active', NOW()),
		('user4@example.com', 'David', 'active', NOW()),
		('user5@example.com', 'Eve', 'inactive', NOW()),
		('user6@example.com', 'Frank', 'active', NOW()),
		('user7@example.com', 'Grace', 'inactive', NOW()),
		('user8@example.com', 'Henry', 'active', NOW()),
		('user9@example.com', 'Ivy', 'active', NOW()),
		('user10@example.com', 'Jack', 'inactive', NOW())
	`, schemaName, tableName)

	if _, err := pool.Exec(ctx, insertStmt); err != nil {
		t.Fatalf("unable to insert data: %s", err)
	}

	// Run ANALYZE to update statistics
	analyzeStmt := fmt.Sprintf(`ANALYZE %s.%s`, schemaName, tableName)
	if _, err := pool.Exec(ctx, analyzeStmt); err != nil {
		t.Fatalf("unable to run ANALYZE: %s", err)
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		shouldHaveData bool // Whether we expect data in the response
	}{
		{
			name:           "get cardinality for a specific column",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s", "table_name": "%s", "column_name": "email"}`, schemaName, tableName)),
			wantStatusCode: http.StatusOK,
			shouldHaveData: true,
		},
		{
			name:           "get cardinality for all columns",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s", "table_name": "%s"}`, schemaName, tableName)),
			wantStatusCode: http.StatusOK,
			shouldHaveData: true,
		},
		{
			name:           "get cardinality with non-existent column",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s", "table_name": "%s", "column_name": "non_existent"}`, schemaName, tableName)),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "get cardinality with non-existent schema",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "non_existent_schema", "table_name": "%s"}`, tableName)),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/get_column_cardinality/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v", err)
			}

			// Verify that we got the expected data presence
			if tc.shouldHaveData {
				if len(got) == 0 {
					t.Logf("warning: expected data but got empty result. This can happen if pg_stats is not populated yet.")
					return
				}

				// Verify column names and cardinality values
				for _, row := range got {
					columnName, ok := row["column_name"].(string)
					if !ok {
						t.Fatalf("column_name is not a string: %v", row["column_name"])
					}

					// Check that estimated_cardinality is present and is a number
					cardinality, ok := row["estimated_cardinality"]
					if !ok {
						t.Fatalf("estimated_cardinality is missing for column %s", columnName)
					}

					// Convert to float64 for numeric checks
					cardinalityFloat, ok := cardinality.(float64)
					if !ok {
						t.Fatalf("estimated_cardinality is not a number: %v", cardinality)
					}

					// Cardinality should be >= 0
					if cardinalityFloat < 0 {
						t.Errorf("cardinality for column %s is negative: %v", columnName, cardinalityFloat)
					}
				}
			} else {
				if len(got) != 0 {
					t.Errorf("expected no data but got: %v", got)
				}
			}
		})
	}
}

func createPostgresExtension(t *testing.T, ctx context.Context, pool *pgxpool.Pool, extensionName string) func() {
	createExtensionCmd := fmt.Sprintf("CREATE EXTENSION IF NOT EXISTS %s", extensionName)
	_, err := pool.Exec(ctx, createExtensionCmd)
	if err != nil {
		t.Fatalf("failed to create extension: %v", err)
	}
	return func() {
		dropExtensionCmd := fmt.Sprintf("DROP EXTENSION IF EXISTS %s", extensionName)
		_, err := pool.Exec(context.WithoutCancel(ctx), dropExtensionCmd)
		if err != nil {
			t.Fatalf("failed to drop extension: %v", err)
		}
	}
}

func RunPostgresListQueryStatsTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	// Insert a simple query by running a SELECT statement
	// This will record statistics in pg_stat_statements
	selectStmt := "SELECT 1 as test_query"
	if _, err := pool.Exec(ctx, selectStmt); err != nil {
		t.Logf("warning: unable to execute test query: %s", err)
	}

	dropExtensionFunc := createPostgresExtension(t, ctx, pool, "pg_stat_statements")
	defer dropExtensionFunc()

	type queryStatDetails struct {
		Datname        string `json:"datname"`
		Query          string `json:"query"`
		Calls          any    `json:"calls"`
		TotalExecTime  any    `json:"total_exec_time"`
		MinExecTime    any    `json:"min_exec_time"`
		MaxExecTime    any    `json:"max_exec_time"`
		MeanExecTime   any    `json:"mean_exec_time"`
		Rows           any    `json:"rows"`
		SharedBlksHit  any    `json:"shared_blks_hit"`
		SharedBlksRead any    `json:"shared_blks_read"`
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
	}{
		{
			name:           "list query stats with default limit",
			requestBody:    bytes.NewBufferString(`{}`),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list query stats with custom limit",
			requestBody:    bytes.NewBufferString(`{"limit": 10}`),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list query stats for specific database",
			requestBody:    bytes.NewBufferString(`{"database_name": "postgres"}`),
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "list query stats with non-existent database name",
			requestBody:    bytes.NewBufferString(`{"database_name": "non_existent_db_xyz"}`),
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_query_stats/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []map[string]any
			if err := json.Unmarshal([]byte(resultString), &got); err != nil {
				t.Fatalf("failed to unmarshal nested result string: %v, resultString: %s", err, resultString)
			}

			// For databases with pg_stat_statements available, verify response structure
			if len(got) > 0 {
				// Verify the response has the expected fields
				requiredFields := []string{"datname", "query", "calls", "total_exec_time", "min_exec_time", "max_exec_time", "mean_exec_time", "rows", "shared_blks_hit", "shared_blks_read"}
				for _, field := range requiredFields {
					if _, ok := got[0][field]; !ok {
						t.Errorf("missing expected field: %s in result: %v", field, got[0])
					}
				}

				// Verify data types
				var stat queryStatDetails
				statData, _ := json.Marshal(got[0])
				if err := json.Unmarshal(statData, &stat); err != nil {
					t.Logf("warning: unable to unmarshal query stat: %v", err)
				}

				// Verify that results are ordered by total_exec_time (descending)
				if len(got) > 1 {
					for i := 0; i < len(got)-1; i++ {
						currentTime, ok1 := got[i]["total_exec_time"].(float64)
						nextTime, ok2 := got[i+1]["total_exec_time"].(float64)
						if ok1 && ok2 && currentTime < nextTime {
							t.Logf("warning: results may not be ordered by total_exec_time descending: %f vs %f", currentTime, nextTime)
						}
					}
				}
			}
		})
	}
}

// RunPostgresListTableStatsTest runs tests for the postgres list-table-stats tool
func RunPostgresListTableStatsTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	type tableStatsDetails struct {
		SchemaName          string  `json:"schema_name"`
		TableName           string  `json:"table_name"`
		Owner               string  `json:"owner"`
		TotalSizeBytes      any     `json:"total_size_bytes"`
		SeqScan             any     `json:"seq_scan"`
		IdxScan             any     `json:"idx_scan"`
		IdxScanRatioPercent float64 `json:"idx_scan_ratio_percent"`
		LiveRows            any     `json:"live_rows"`
		DeadRows            any     `json:"dead_rows"`
		DeadRowRatioPercent float64 `json:"dead_row_ratio_percent"`
		NTupIns             any     `json:"n_tup_ins"`
		NTupUpd             any     `json:"n_tup_upd"`
		NTupDel             any     `json:"n_tup_del"`
		LastVacuum          any     `json:"last_vacuum"`
		LastAutovacuum      any     `json:"last_autovacuum"`
		LastAutoanalyze     any     `json:"last_autoanalyze"`
	}

	// Create a test table to generate statistics
	testTableName := "test_list_table_stats_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createTableStmt := fmt.Sprintf(`
        CREATE TABLE %s (
            id SERIAL PRIMARY KEY,
            name VARCHAR(100),
            email VARCHAR(100)
        )
    `, testTableName)

	if _, err := pool.Exec(ctx, createTableStmt); err != nil {
		t.Fatalf("unable to create test table: %s", err)
	}
	defer func() {
		dropTableStmt := fmt.Sprintf("DROP TABLE IF EXISTS %s", testTableName)
		if _, err := pool.Exec(ctx, dropTableStmt); err != nil {
			t.Logf("warning: unable to drop test table: %v", err)
		}
	}()

	// Insert some data to generate statistics
	insertStmt := fmt.Sprintf(`
        INSERT INTO %s (name, email) VALUES
        ('Alice', 'alice@example.com'),
        ('Bob', 'bob@example.com'),
        ('Charlie', 'charlie@example.com'),
        ('David', 'david@example.com'),
        ('Eve', 'eve@example.com')
    `, testTableName)

	if _, err := pool.Exec(ctx, insertStmt); err != nil {
		t.Fatalf("unable to insert test data: %s", err)
	}

	// Run some sequential scans to generate statistics
	for i := 0; i < 3; i++ {
		selectStmt := fmt.Sprintf("SELECT * FROM %s WHERE name = 'Alice'", testTableName)
		if _, err := pool.Exec(ctx, selectStmt); err != nil {
			t.Logf("warning: unable to execute select: %v", err)
		}
	}

	// Run ANALYZE to update statistics
	analyzeStmt := fmt.Sprintf("ANALYZE %s", testTableName)
	if _, err := pool.Exec(ctx, analyzeStmt); err != nil {
		t.Logf("warning: unable to run ANALYZE: %v", err)
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		shouldHaveData bool
		filterTable    bool
	}{
		{
			name:           "list table stats with no arguments (default limit)",
			requestBody:    bytes.NewBufferString(`{}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false, // may or may not have data depending on what's in the database
		},
		{
			name:           "list table stats with default limit",
			requestBody:    bytes.NewBufferString(`{"schema_name": "public"}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list table stats filtering by specific table",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"table_name": "%s"}`, testTableName)),
			wantStatusCode: http.StatusOK,
			shouldHaveData: true,
			filterTable:    true,
		},
		{
			name:           "list table stats with custom limit",
			requestBody:    bytes.NewBufferString(`{"limit": 10}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list table stats sorted by size",
			requestBody:    bytes.NewBufferString(`{"sort_by": "size", "limit": 5}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list table stats sorted by seq_scan",
			requestBody:    bytes.NewBufferString(`{"sort_by": "seq_scan", "limit": 5}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list table stats sorted by idx_scan",
			requestBody:    bytes.NewBufferString(`{"sort_by": "idx_scan", "limit": 5}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list table stats sorted by dead_rows",
			requestBody:    bytes.NewBufferString(`{"sort_by": "dead_rows", "limit": 5}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list table stats with non-existent table filter",
			requestBody:    bytes.NewBufferString(`{"table_name": "non_existent_table_xyz"}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list table stats with non-existent schema filter",
			requestBody:    bytes.NewBufferString(`{"schema_name": "non_existent_schema_xyz"}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list table stats with owner filter",
			requestBody:    bytes.NewBufferString(`{"owner": "postgres"}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_table_stats/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []tableStatsDetails
			if resultString != "null" {
				if err := json.Unmarshal([]byte(resultString), &got); err != nil {
					t.Fatalf("failed to unmarshal result: %v, result string: %s", err, resultString)
				}
			}

			// Verify expected data presence
			if tc.shouldHaveData {
				if len(got) == 0 {
					t.Fatalf("expected data but got empty result")
				}

				// Verify the test table is in results
				found := false
				for _, row := range got {
					if row.TableName == testTableName {
						found = true
						// Verify expected fields are present
						if row.SchemaName == "" {
							t.Errorf("schema_name should not be empty")
						}
						if row.Owner == "" {
							t.Errorf("owner should not be empty")
						}
						if row.TotalSizeBytes == nil {
							t.Errorf("total_size_bytes should not be null")
						}
						if row.LiveRows == nil {
							t.Errorf("live_rows should not be null")
						}
						break
					}
				}

				if !found {
					t.Errorf("test table %s not found in results", testTableName)
				}
			} else if tc.filterTable {
				// For filtered queries that shouldn't find anything
				if len(got) != 0 {
					t.Logf("warning: expected no data but got: %v", len(got))
				}
			}

			// Verify result structure and data types
			for _, stat := range got {
				// Verify schema_name and table_name are strings
				if stat.SchemaName == "" && stat.TableName != "" {
					t.Errorf("schema_name is empty for table %s", stat.TableName)
				}

				// Verify numeric fields are valid
				if stat.IdxScanRatioPercent < 0 || stat.IdxScanRatioPercent > 100 {
					t.Errorf("idx_scan_ratio_percent should be between 0 and 100, got %f", stat.IdxScanRatioPercent)
				}

				if stat.DeadRowRatioPercent < 0 || stat.DeadRowRatioPercent > 100 {
					t.Errorf("dead_row_ratio_percent should be between 0 and 100, got %f", stat.DeadRowRatioPercent)
				}
			}

			// Verify sorting for specific sort_by options
			if tc.name == "list table stats sorted by size" && len(got) > 1 {
				for i := 0; i < len(got)-1; i++ {
					current, ok1 := got[i].TotalSizeBytes.(float64)
					next, ok2 := got[i+1].TotalSizeBytes.(float64)
					if ok1 && ok2 && current < next {
						t.Logf("warning: results may not be sorted by total_size_bytes descending")
					}
				}
			}
		})
	}
}

// cleanupOldSchemas cleans up schemas that were created more than 1 hour ago
func cleanupOldSchemas(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	rows, err := pool.Query(ctx, "SELECT schema_name FROM information_schema.schemata WHERE schema_name LIKE 'test_proc_%'")
	if err != nil {
		return
	}
	defer rows.Close()

	oneHourAgo := time.Now().Add(-1 * time.Hour).Unix()

	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			continue
		}

		parts := strings.Split(name, "_")
		if len(parts) < 3 {
			continue
		}

		timestamp, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			continue
		}

		if timestamp < oneHourAgo {
			_, err := pool.Exec(ctx, fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", name))
			if err == nil {
				t.Logf("Cleaned up schema: %s", name)
			}
		}
	}
}

// RunPostgresListStoredProcedureTest runs tests for the postgres list-stored-procedure tool
func RunPostgresListStoredProcedureTest(t *testing.T, ctx context.Context, pool *pgxpool.Pool) {
	cleanupOldSchemas(t, ctx, pool)

	type storedProcedureDetails struct {
		SchemaName  string `json:"schema_name"`
		Name        string `json:"name"`
		Owner       string `json:"owner"`
		Language    string `json:"language"`
		Definition  string `json:"definition"`
		Description any    `json:"description"`
	}

	// Create test schema
	// Use this format: test_proc_<timestamp>_<uuid>
	now := time.Now().Unix()
	testSchemaName := fmt.Sprintf("test_proc_%d_%s", now, strings.ReplaceAll(uuid.New().String(), "-", "")[:8])
	createSchemaStmt := fmt.Sprintf("CREATE SCHEMA %s", testSchemaName)
	if _, err := pool.Exec(ctx, createSchemaStmt); err != nil {
		t.Fatalf("unable to create test schema: %v", err)
	}
	defer func() {
		dropSchemaStmt := fmt.Sprintf("DROP SCHEMA IF EXISTS %s CASCADE", testSchemaName)
		if _, err := pool.Exec(context.WithoutCancel(ctx), dropSchemaStmt); err != nil {
			t.Logf("warning: unable to drop test schema: %v", err)
		}
	}()

	// Create test procedures
	proc1Name := "test_proc_1_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createProc1Stmt := fmt.Sprintf(`
        CREATE PROCEDURE %s.%s(p_count INT)
        LANGUAGE plpgsql
        AS $$
        BEGIN
            INSERT INTO test_table VALUES (p_count);
            COMMIT;
        END;
        $$
    `, testSchemaName, proc1Name)

	if _, err := pool.Exec(ctx, createProc1Stmt); err != nil {
		t.Fatalf("unable to create test procedure 1: %v", err)
	}

	// Add a comment/description to the procedure
	commentStmt := fmt.Sprintf("COMMENT ON PROCEDURE %s.%s(INT) IS 'Test procedure that inserts a record'", testSchemaName, proc1Name)
	if _, err := pool.Exec(ctx, commentStmt); err != nil {
		t.Logf("warning: unable to add comment to procedure: %v", err)
	}

	// Create a second test procedure
	proc2Name := "test_proc_2_" + strings.ReplaceAll(uuid.New().String(), "-", "")
	createProc2Stmt := fmt.Sprintf(`
        CREATE PROCEDURE %s.%s()
        LANGUAGE plpgsql
        AS $$
        DECLARE
            v_count INT;
        BEGIN
            SELECT COUNT(*) INTO v_count FROM test_table;
            RAISE NOTICE 'Total records: %%', v_count;
        END;
        $$
    `, testSchemaName, proc2Name)

	if _, err := pool.Exec(ctx, createProc2Stmt); err != nil {
		t.Fatalf("unable to create test procedure 2: %v", err)
	}

	invokeTcs := []struct {
		name           string
		requestBody    io.Reader
		wantStatusCode int
		shouldHaveData bool
		expectedCount  int
		filterByRole   string
		filterBySchema string
	}{
		{
			name:           "list stored procedures with no arguments (default limit 20)",
			requestBody:    bytes.NewBufferString(`{}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false, // may or may not have data depending on what's in the database
		},
		{
			name:           "list stored procedures filtering by specific schema",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s"}`, testSchemaName)),
			wantStatusCode: http.StatusOK,
			shouldHaveData: true,
			expectedCount:  2,
			filterBySchema: testSchemaName,
		},
		{
			name:           "list stored procedures filtering by procedure owner (postgres)",
			requestBody:    bytes.NewBufferString(`{"role_name": "postgres"}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false, // might have procedures owned by postgres
		},
		{
			name:           "list stored procedures with custom limit",
			requestBody:    bytes.NewBufferString(`{"limit": 5}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list stored procedures filtering by schema and role",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s", "role_name": "postgres"}`, testSchemaName)),
			wantStatusCode: http.StatusOK,
			shouldHaveData: true,
			expectedCount:  2,
			filterBySchema: testSchemaName,
			filterByRole:   "postgres",
		},
		{
			name:           "list stored procedures with non-existent schema",
			requestBody:    bytes.NewBufferString(`{"schema_name": "non_existent_schema_xyz"}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list stored procedures with non-existent role",
			requestBody:    bytes.NewBufferString(`{"role_name": "non_existent_role_xyz"}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: false,
		},
		{
			name:           "list stored procedures with partial schema name match",
			requestBody:    bytes.NewBufferString(`{"schema_name": "test_proc"}`),
			wantStatusCode: http.StatusOK,
			shouldHaveData: true,
			expectedCount:  2,
		},
		{
			name:           "list stored procedures with limit 1",
			requestBody:    bytes.NewBufferString(fmt.Sprintf(`{"schema_name": "%s", "limit": 1}`, testSchemaName)),
			wantStatusCode: http.StatusOK,
			shouldHaveData: true,
			expectedCount:  1,
			filterBySchema: testSchemaName,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			const api = "http://127.0.0.1:5000/api/tool/list_stored_procedure/invoke"
			resp, respBody := RunRequest(t, http.MethodPost, api, tc.requestBody, nil)
			if resp.StatusCode != tc.wantStatusCode {
				t.Fatalf("wrong status code: got %d, want %d, body: %s", resp.StatusCode, tc.wantStatusCode, string(respBody))
			}
			if tc.wantStatusCode != http.StatusOK {
				return
			}

			var bodyWrapper struct {
				Result json.RawMessage `json:"result"`
			}
			if err := json.Unmarshal(respBody, &bodyWrapper); err != nil {
				t.Fatalf("error decoding response wrapper: %v", err)
			}

			var resultString string
			if err := json.Unmarshal(bodyWrapper.Result, &resultString); err != nil {
				resultString = string(bodyWrapper.Result)
			}

			var got []storedProcedureDetails
			if resultString != "null" {
				if err := json.Unmarshal([]byte(resultString), &got); err != nil {
					t.Fatalf("failed to unmarshal result: %v, result string: %s", err, resultString)
				}
			}

			// Verify expected data presence
			if tc.shouldHaveData {
				if len(got) == 0 {
					t.Fatalf("expected data but got empty result")
				}

				// If filtering by schema, verify all results are from that schema
				if tc.filterBySchema != "" {
					for _, proc := range got {
						if proc.SchemaName != tc.filterBySchema && !strings.Contains(proc.SchemaName, tc.filterBySchema) {
							t.Errorf("procedure schema %s does not match filter %s", proc.SchemaName, tc.filterBySchema)
						}
					}
				}

				// If filtering by role, verify all results are owned by that role
				if tc.filterByRole != "" {
					for _, proc := range got {
						if proc.Owner != tc.filterByRole {
							t.Errorf("procedure owner %s does not match filter %s", proc.Owner, tc.filterByRole)
						}
					}
				}

				// Verify expected count if specified
				if tc.expectedCount > 0 && len(got) != tc.expectedCount {
					t.Errorf("expected %d procedures but got %d", tc.expectedCount, len(got))
				}
			}

			// Verify result structure and data types
			for _, proc := range got {
				// Verify all required fields are present and non-empty
				if proc.SchemaName == "" {
					t.Errorf("schema_name should not be empty")
				}
				if proc.Name == "" {
					t.Errorf("procedure name should not be empty")
				}
				if proc.Owner == "" {
					t.Errorf("owner should not be empty")
				}
				if proc.Language == "" {
					t.Errorf("language should not be empty")
				}
				if proc.Definition == "" {
					t.Errorf("definition should not be empty")
				}

				// Verify definition contains CREATE PROCEDURE
				if !strings.Contains(strings.ToUpper(proc.Definition), "PROCEDURE") {
					t.Logf("warning: definition may not be a valid CREATE PROCEDURE statement: %s", proc.Definition)
				}

				// Verify language is a valid PostgreSQL language
				validLanguages := []string{"plpgsql", "sql", "c", "internal", "plperl", "pltcl", "plpython"}
				found := false
				for _, lang := range validLanguages {
					if proc.Language == lang {
						found = true
						break
					}
				}
				if !found {
					t.Logf("warning: language %s may not be a standard PostgreSQL language", proc.Language)
				}
			}

			// Verify results are sorted by schema_name and name
			if len(got) > 1 {
				for i := 0; i < len(got)-1; i++ {
					currentKey := fmt.Sprintf("%s.%s", got[i].SchemaName, got[i].Name)
					nextKey := fmt.Sprintf("%s.%s", got[i+1].SchemaName, got[i+1].Name)
					if currentKey > nextKey {
						t.Logf("warning: results may not be sorted by schema_name and name")
					}
				}
			}
		})
	}
}

func RunStatementToolsTest(t *testing.T, tools map[string]string) {
	for toolName, paramBody := range tools {
		t.Run(toolName, func(t *testing.T) {
			api := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
			req, err := http.NewRequest(http.MethodPost, api, bytes.NewBufferString(paramBody))
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}
		})
	}
}

// SearchCatalogTestParams defines parameters for RunSearchCatalogToolTest
type SearchCatalogTestParams struct {
	ContainerParamName string // "databaseIds" or "datasetIds"
	ContainerName      string // databaseName or datasetName
	ProjectID          string // SpannerProject or BigqueryProject
	TargetName         string // tableName
	WantKey            string // "DisplayName"
	AllowEmpty         bool   // true for Spanner, false for BigQuery
	CheckValue         bool   // true for BigQuery, false for Spanner
}

// RunSearchCatalogToolTest tests the search catalog tool for Spanner or BigQuery
func RunSearchCatalogToolTest(t *testing.T, params SearchCatalogTestParams) {
	// Get ID token
	idToken, err := GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	// Get access token
	accessToken, err := sources.GetIAMAccessToken(t.Context())
	if err != nil {
		t.Fatalf("error getting access token from ADC: %s", err)
	}
	accessToken = "Bearer " + accessToken

	// Test tool invoke endpoint
	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   io.Reader
		isErr         bool
	}{
		{
			name:          "invoke my-search-catalog-tool without body",
			api:           "http://127.0.0.1:5000/api/tool/my-search-catalog-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(`{}`)),
			isErr:         true,
		},
		{
			name:          "invoke my-search-catalog-tool",
			api:           "http://127.0.0.1:5000/api/tool/my-search-catalog-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"%s\":[\"%s\"]}", params.TargetName, params.ContainerParamName, params.ContainerName))),
			isErr:         false,
		},
		{
			name:          "Invoke my-auth-search-catalog-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"%s\":[\"%s\"]}", params.TargetName, params.ContainerParamName, params.ContainerName))),
			isErr:         false,
		},
		{
			name:          "Invoke my-auth-search-catalog-tool with correct project",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"projectIds\":[\"%s\"], \"%s\":[\"%s\"]}", params.TargetName, params.ProjectID, params.ContainerParamName, params.ContainerName))),
			isErr:         false,
		},
		{
			name:          "Invoke my-auth-search-catalog-tool with non-existent project",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"projectIds\":[\"%s-%s\"], \"%s\":[\"%s\"]}", params.TargetName, params.ProjectID, uuid.NewString(), params.ContainerParamName, params.ContainerName))),
			isErr:         true,
		},
		{
			name:          "Invoke my-auth-search-catalog-tool with invalid auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"%s\":[\"%s\"]}", params.TargetName, params.ContainerParamName, params.ContainerName))),
			isErr:         true,
		},
		{
			name:          "Invoke my-auth-search-catalog-tool without auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-auth-search-catalog-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"%s\":[\"%s\"]}", params.TargetName, params.ContainerParamName, params.ContainerName))),
			isErr:         true,
		},
		{
			name:          "Invoke my-client-auth-search-catalog-tool without auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-client-auth-search-catalog-tool/invoke",
			requestHeader: map[string]string{},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"%s\":[\"%s\"]}", params.TargetName, params.ContainerParamName, params.ContainerName))),
			isErr:         true,
		},
		{
			name:          "Invoke my-client-auth-search-catalog-tool with auth token",
			api:           "http://127.0.0.1:5000/api/tool/my-client-auth-search-catalog-tool/invoke",
			requestHeader: map[string]string{"Authorization": accessToken},
			requestBody:   bytes.NewBuffer([]byte(fmt.Sprintf("{\"prompt\":\"%s\", \"types\":[\"TABLE\"], \"%s\":[\"%s\"]}", params.TargetName, params.ContainerParamName, params.ContainerName))),
			isErr:         false,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
			if err != nil {
				t.Fatalf("unable to create request: %s", err)
			}
			req.Header.Add("Content-type", "application/json")
			for k, v := range tc.requestHeader {
				req.Header.Add(k, v)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("unable to send request: %s", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusOK {
				if tc.isErr {
					return
				}
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var result map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				t.Fatalf("error parsing response body: %s", err)
			}
			resultStr, ok := result["result"].(string)
			if !ok {
				if result["result"] == nil && tc.isErr {
					return
				}
				t.Fatalf("expected 'result' field to be a string, got %T", result["result"])
			}

			var errorCheck map[string]any
			if err := json.Unmarshal([]byte(resultStr), &errorCheck); err == nil {
				if _, hasError := errorCheck["error"]; hasError {
					if tc.isErr {
						return
					}
					t.Fatalf("unexpected error object in result: %s", resultStr)
				}
			}

			if tc.isErr && (resultStr == "" || resultStr == "[]") {
				return
			}

			var entries []any
			if err := json.Unmarshal([]byte(resultStr), &entries); err != nil {
				t.Fatalf("error unmarshalling result string: %v. Raw string: %s", err, resultStr)
			}

			if !tc.isErr {
				if len(entries) == 0 {
					if params.AllowEmpty {
						t.Logf("No entries found, skipping content verification")
						return
					}
					t.Fatalf("expected entries, but got 0")
				}

				if !params.AllowEmpty && len(entries) != 1 {
					t.Fatalf("expected exactly one entry, but got %d", len(entries))
				}

				entry, ok := entries[0].(map[string]interface{})
				if !ok {
					t.Fatalf("expected first entry to be a map, got %T", entries[0])
				}

				val, ok := entry[params.WantKey]
				if !ok {
					t.Fatalf("expected entry to have key '%s', but it was not found", params.WantKey)
				}

				if params.CheckValue {
					if val != params.TargetName {
						t.Fatalf("expected key '%s' to have value '%s', but got %s", params.WantKey, params.TargetName, val)
					}
				}
			} else {
				if len(entries) != 0 {
					t.Fatalf("expected 0 entries, but got %d", len(entries))
				}
			}
		})
	}
}
