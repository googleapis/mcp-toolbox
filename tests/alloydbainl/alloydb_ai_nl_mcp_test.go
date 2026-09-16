// Copyright 2026 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package alloydbainl

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func TestAlloyDBAINLListTools(t *testing.T) {
	sourceConfig := getAlloyDBAINLVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	toolsFile := getAINLToolsConfig(sourceConfig)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Verify list of tools
	expectedTools := []tests.MCPToolManifest{
		{
			Name:        "my-simple-tool",
			Description: "Simple tool to test end to end functionality.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": map[string]any{
						"description": "The natural language question to ask.",
						"type":        "string",
					},
				},
				"required": []any{"question"},
			},
		},
		{
			Name:        "my-auth-tool",
			Description: "Tool to test authenticated parameters.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": map[string]any{
						"description": "The natural language question to ask.",
						"type":        "string",
					},
					"email": map[string]any{
						"description": "user email",
						"type":        "string",
					},
				},
				"required": []any{"question", "email"},
			},
		},
		{
			Name:        "my-auth-required-tool",
			Description: "Tool to test auth required invocation.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": map[string]any{
						"description": "The natural language question to ask.",
						"type":        "string",
					},
				},
				"required": []any{"question"},
			},
		},
	}

	tests.RunMCPToolsListMethod(t, expectedTools)
}

func TestAlloyDBAINLCallTool(t *testing.T) {
	sourceConfig := getAlloyDBAINLVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	toolsFile := getAINLToolsConfig(sourceConfig)

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	waitCtx, cancelWait := context.WithTimeout(ctx, 10*time.Second)
	defer cancelWait()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}

	invokeTcs := []struct {
		name           string
		toolName       string
		args           map[string]any
		requestHeader  map[string]string
		want           string
		isErr          bool
		wantStatusCode int
	}{
		{
			name:     "invoke my-simple-tool",
			toolName: "my-simple-tool",
			args:     map[string]any{"question": "SELECT the integer 1 and explicitly alias the output column as 'number_one'"},
			want:     "{\"execute_nl_query\":{\"number_one\":1}}",
			isErr:    false,
		},
		{
			name:          "Invoke my-auth-tool with auth token",
			toolName:      "my-auth-tool",
			args:          map[string]any{"question": "can you show me the name of this user?"},
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			want:          "{\"execute_nl_query\":{\"name\":\"Alice\"}}",
			isErr:         false,
		},
		{
			name:          "Invoke my-auth-tool with invalid auth token",
			toolName:      "my-auth-tool",
			args:          map[string]any{"question": "SELECT the integer 1 and explicitly alias the output column as 'number_one'"},
			requestHeader: map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			isErr:         true,
		},
		{
			name:     "Invoke my-auth-tool without auth token",
			toolName: "my-auth-tool",
			args:     map[string]any{"question": "SELECT the integer 1 and explicitly alias the output column as 'number_one'"},
			isErr:    true,
		},
		{
			name:          "Invoke my-auth-required-tool with auth token",
			toolName:      "my-auth-required-tool",
			args:          map[string]any{"question": "SELECT the integer 1 and explicitly alias the output column as 'number_one'"},
			requestHeader: map[string]string{"my-google-auth_token": idToken},
			isErr:         false,
			want:          "{\"execute_nl_query\":{\"number_one\":1}}",
		},
		{
			name:           "Invoke my-auth-required-tool with invalid auth token",
			toolName:       "my-auth-required-tool",
			args:           map[string]any{"question": "SELECT the integer 1 and explicitly alias the output column as 'number_one'"},
			requestHeader:  map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			isErr:          true,
			wantStatusCode: 401,
		},
		{
			name:           "Invoke my-auth-required-tool without auth token",
			toolName:       "my-auth-required-tool",
			args:           map[string]any{"question": "SELECT the integer 1 and explicitly alias the output column as 'number_one'"},
			isErr:          true,
			wantStatusCode: 401,
		},
		{
			name:     "Invoke invalid tool",
			toolName: "foo",
			args:     map[string]any{},
			isErr:    true,
		},
		{
			name:     "Invoke my-auth-tool without parameters",
			toolName: "my-auth-tool",
			args:     map[string]any{},
			isErr:    true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			statusCode, mcpResp, err := tests.InvokeMCPTool(t, tc.toolName, tc.args, tc.requestHeader)
			if err != nil {
				t.Fatalf("native error executing %s: %s", tc.toolName, err)
			}

			expectedStatus := tc.wantStatusCode
			if expectedStatus == 0 {
				expectedStatus = http.StatusOK
			}
			if statusCode != expectedStatus {
				t.Fatalf("expected status %d, got %d", expectedStatus, statusCode)
			}

			if tc.isErr {
				if mcpResp.Error == nil && !mcpResp.Result.IsError {
					t.Fatalf("expected error result or JSON-RPC error, got success")
				}
			} else {
				if mcpResp.Error != nil {
					t.Fatalf("expected success, got JSON-RPC error: %v", mcpResp.Error)
				}
				if mcpResp.Result.IsError {
					t.Fatalf("expected success result, got tool error: %v", mcpResp.Result)
				}
				if len(mcpResp.Result.Content) == 0 {
					t.Fatalf("expected at least one content item, got none")
				}
				got := mcpResp.Result.Content[0].Text
				if got != tc.want {
					t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
				}
			}
		})
	}
}

var (
	AlloyDBAINLSourceType = "alloydb-postgres"
	AlloyDBAINLToolType   = "alloydb-ai-nl"
	AlloyDBAINLProject    = os.Getenv("ALLOYDB_AI_NL_PROJECT")
	AlloyDBAINLRegion     = os.Getenv("ALLOYDB_AI_NL_REGION")
	AlloyDBAINLCluster    = os.Getenv("ALLOYDB_AI_NL_CLUSTER")
	AlloyDBAINLInstance   = os.Getenv("ALLOYDB_AI_NL_INSTANCE")
	AlloyDBAINLDatabase   = os.Getenv("ALLOYDB_AI_NL_DATABASE")
	AlloyDBAINLUser       = os.Getenv("ALLOYDB_AI_NL_USER")
	AlloyDBAINLPass       = os.Getenv("ALLOYDB_AI_NL_PASS")
)

func getAlloyDBAINLVars(t *testing.T) map[string]any {
	switch "" {
	case AlloyDBAINLProject:
		t.Fatal("'ALLOYDB_AI_NL_PROJECT' not set")
	case AlloyDBAINLRegion:
		t.Fatal("'ALLOYDB_AI_NL_REGION' not set")
	case AlloyDBAINLCluster:
		t.Fatal("'ALLOYDB_AI_NL_CLUSTER' not set")
	case AlloyDBAINLInstance:
		t.Fatal("'ALLOYDB_AI_NL_INSTANCE' not set")
	case AlloyDBAINLDatabase:
		t.Fatal("'ALLOYDB_AI_NL_DATABASE' not set")
	case AlloyDBAINLUser:
		t.Fatal("'ALLOYDB_AI_NL_USER' not set")
	case AlloyDBAINLPass:
		t.Fatal("'ALLOYDB_AI_NL_PASS' not set")
	}
	return map[string]any{
		"type":     AlloyDBAINLSourceType,
		"project":  AlloyDBAINLProject,
		"cluster":  AlloyDBAINLCluster,
		"instance": AlloyDBAINLInstance,
		"region":   AlloyDBAINLRegion,
		"database": AlloyDBAINLDatabase,
		"user":     AlloyDBAINLUser,
		"password": AlloyDBAINLPass,
	}
}

func getAINLToolsConfig(sourceConfig map[string]any) map[string]any {
	// Write config into a file and pass it to command
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": tests.ClientId,
			},
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"type":        AlloyDBAINLToolType,
				"source":      "my-instance",
				"description": "Simple tool to test end to end functionality.",
				"nlConfig":    "my_nl_config",
			},
			"my-auth-tool": map[string]any{
				"type":        AlloyDBAINLToolType,
				"source":      "my-instance",
				"description": "Tool to test authenticated parameters.",
				"nlConfig":    "my_nl_config",
				"nlConfigParameters": []map[string]any{
					{
						"name":        "email",
						"type":        "string",
						"description": "user email",
						"authServices": []map[string]string{
							{
								"name":  "my-google-auth",
								"field": "email",
							},
						},
					},
				},
			},
			"my-auth-required-tool": map[string]any{
				"type":        AlloyDBAINLToolType,
				"source":      "my-instance",
				"description": "Tool to test auth required invocation.",
				"nlConfig":    "my_nl_config",
				"authRequired": []string{
					"my-google-auth",
				},
			},
		},
	}

	return toolsFile
}
