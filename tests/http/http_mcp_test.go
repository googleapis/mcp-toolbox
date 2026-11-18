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

package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/googleapis/mcp-toolbox/tests"
)

var (
	HttpSourceType = "http"
	HttpToolType   = "http"
)

// handler function for the test server
func multiTool(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	path = strings.TrimPrefix(path, "/") // Remove leading slash

	switch path {
	case "tool0":
		handleTool0(w, r)
	case "tool1":
		handleTool1(w, r)
	case "tool1id":
		handleTool1Id(w, r)
	case "tool1name":
		handleTool1Name(w, r)
	case "tool2":
		handleTool2(w, r)
	case "tool3":
		handleTool3(w, r)
	case "toolQueryTest":
		handleQueryTest(w, r)
	default:
		http.NotFound(w, r) // Return 404 for unknown paths
	}
}

// handleQueryTest simply returns the raw query string it received so the test
// can verify it's formatted correctly.
func handleQueryTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorMessage := fmt.Sprintf("expected GET method but got: %s", string(r.Method))
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)

	err := enc.Encode(r.URL.RawQuery)
	if err != nil {
		http.Error(w, "Failed to write response", http.StatusInternalServerError)
		return
	}
}

func handleTool0(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errorMessage := fmt.Sprintf("expected POST method but got: %s", string(r.Method))
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
	response := "hello world"
	err := json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
}

func handleTool1(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorMessage := fmt.Sprintf("expected GET method but got: %s", string(r.Method))
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}
	var requestBody map[string]interface{}
	bodyBytes, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		http.Error(w, "Bad Request: Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	err := json.Unmarshal(bodyBytes, &requestBody)
	if err != nil {
		errorMessage := fmt.Sprintf("Bad Request: Error unmarshalling request body: %s, Raw body: %s", err, string(bodyBytes))
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}

	name, ok := requestBody["name"].(string)
	if !ok || name == "" {
		http.Error(w, "Bad Request: Missing or invalid name", http.StatusBadRequest)
		return
	}

	if name == "Alice" {
		response := `[{"id":1,"name":"Alice"},{"id":3,"name":"Sid"}]`
		_, err := w.Write([]byte(response))
		if err != nil {
			http.Error(w, "Failed to write response", http.StatusInternalServerError)
		}
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleTool1Id(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorMessage := fmt.Sprintf("expected GET method but got: %s", string(r.Method))
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}

	id := r.URL.Query().Get("id")
	if id == "4" {
		response := `[{"id":4,"name":null}]`
		_, err := w.Write([]byte(response))
		if err != nil {
			http.Error(w, "Failed to write response", http.StatusInternalServerError)
		}
		return
	}
	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleTool1Name(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorMessage := fmt.Sprintf("expected GET method but got: %s", string(r.Method))
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}

	if !r.URL.Query().Has("name") {
		response := "[]"
		_, err := w.Write([]byte(response))
		if err != nil {
			http.Error(w, "Failed to write response", http.StatusInternalServerError)
		}
		return
	}

	http.Error(w, "Bad Request: Unexpected query parameter 'name'", http.StatusBadRequest)
}

func handleTool2(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorMessage := fmt.Sprintf("expected GET method but got: %s", string(r.Method))
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}
	email := r.URL.Query().Get("email")
	if email != "" {
		response := `[{"name":"Alice"}]`
		_, err := w.Write([]byte(response))
		if err != nil {
			http.Error(w, "Failed to write response", http.StatusInternalServerError)
		}
		return
	}

	http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
}

func handleTool3(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		errorMessage := fmt.Sprintf("expected GET method but got: %s", string(r.Method))
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}

	expectedHeaders := map[string]string{
		"Content-Type":    "application/json",
		"X-Custom-Header": "example",
		"X-Other-Header":  "test",
	}
	for header, expectedValue := range expectedHeaders {
		if r.Header.Get(header) != expectedValue {
			errorMessage := fmt.Sprintf("Bad Request: Missing or incorrect header: %s", header)
			http.Error(w, errorMessage, http.StatusBadRequest)
			return
		}
	}

	expectedQueryParams := map[string][]string{
		"id":      []string{"2", "1", "3"},
		"country": []string{"US"},
	}
	query := r.URL.Query()
	for param, expectedValueSlice := range expectedQueryParams {
		values, ok := query[param]
		if ok {
			if !reflect.DeepEqual(expectedValueSlice, values) {
				errorMessage := fmt.Sprintf("Bad Request: Incorrect query parameter: %s, actual: %s", param, query[param])
				http.Error(w, errorMessage, http.StatusBadRequest)
				return
			}
		} else {
			errorMessage := fmt.Sprintf("Bad Request: Missing query parameter: %s, actual: %s", param, query[param])
			http.Error(w, errorMessage, http.StatusBadRequest)
			return
		}
	}

	var requestBody map[string]interface{}
	bodyBytes, readErr := io.ReadAll(r.Body)
	if readErr != nil {
		http.Error(w, "Bad Request: Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	err := json.Unmarshal(bodyBytes, &requestBody)
	if err != nil {
		errorMessage := fmt.Sprintf("Bad Request: Error unmarshalling request body: %s, Raw body: %s", err, string(bodyBytes))
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}

	expectedBody := map[string]interface{}{
		"place":   "zoo",
		"animals": []any{"rabbit", "ostrich", "whale"},
	}

	if !reflect.DeepEqual(requestBody, expectedBody) {
		errorMessage := fmt.Sprintf("Bad Request: Incorrect request body. Expected: %v, Got: %v", expectedBody, requestBody)
		http.Error(w, errorMessage, http.StatusBadRequest)
		return
	}

	response := "hello world"
	err = json.NewEncoder(w).Encode(response)
	if err != nil {
		http.Error(w, "Failed to encode JSON", http.StatusInternalServerError)
		return
	}
}

func getHTTPToolsConfig(sourceConfig map[string]any, toolType string, jwksURL string) map[string]any {
	otherSourceConfig := make(map[string]any)
	for k, v := range sourceConfig {
		otherSourceConfig[k] = v
	}
	otherSourceConfig["headers"] = map[string]string{"X-Custom-Header": "unexpected", "Content-Type": "application/json"}
	otherSourceConfig["queryParams"] = map[string]any{"id": 1, "name": "Sid"}

	clientID := tests.ClientId
	if clientID == "" {
		clientID = "test-client-id"
	}

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance":    sourceConfig,
			"other-instance": otherSourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": clientID,
			},
			"my-generic-auth": map[string]any{
				"type":                "generic",
				"audience":            "test-audience",
				"authorizationServer": jwksURL,
			},
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"type":        toolType,
				"path":        "/tool0",
				"method":      "POST",
				"source":      "my-instance",
				"requestBody": "{}",
				"description": "Simple tool to test end to end functionality.",
			},
			"my-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"method":      "GET",
				"path":        "/tool1",
				"description": "some description",
				"queryParams": []parameters.Parameter{
					parameters.NewIntParameter("id", "user ID")},
				"bodyParams": []parameters.Parameter{parameters.NewStringParameter("name", "user name")},
				"requestBody": `{
"age": 36,
"name": "{{.name}}"
}
`,
				"headers": map[string]string{"Content-Type": "application/json"},
			},
			"my-tool-by-id": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"method":      "GET",
				"path":        "/tool1id",
				"description": "some description",
				"queryParams": []parameters.Parameter{
					parameters.NewIntParameter("id", "user ID")},
				"headers": map[string]string{"Content-Type": "application/json"},
			},
			"my-tool-by-name": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"method":      "GET",
				"path":        "/tool1name",
				"description": "some description",
				"queryParams": []parameters.Parameter{
					parameters.NewStringParameter("name", "user name", parameters.WithStringRequired(false))},
				"headers": map[string]string{"Content-Type": "application/json"},
			},
			"my-query-param-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"method":      "GET",
				"path":        "/toolQueryTest",
				"description": "Tool to test optional query parameters.",
				"queryParams": []parameters.Parameter{
					parameters.NewStringParameter("reqId", "required ID", parameters.WithStringRequired(true)),
					parameters.NewStringParameter("page", "optional page number", parameters.WithStringRequired(false)),
					parameters.NewStringParameter("filter", "optional filter string", parameters.WithStringRequired(false)),
				},
			},
			"my-auth-tool": map[string]any{
				"type":        toolType,
				"source":      "my-instance",
				"method":      "GET",
				"path":        "/tool2",
				"description": "some description",
				"requestBody": "{}",
				"queryParams": []parameters.Parameter{
					parameters.NewStringParameter("email", "some description", parameters.WithStringAuth(
						[]parameters.ParamAuthService{{Name: "my-google-auth", Field: "email"}})),
				},
			},
			"my-auth-required-tool": map[string]any{
				"type":         toolType,
				"source":       "my-instance",
				"method":       "POST",
				"path":         "/tool0",
				"description":  "some description",
				"requestBody":  "{}",
				"authRequired": []string{"my-google-auth"},
			},
			"my-auth-required-generic-tool": map[string]any{
				"type":         toolType,
				"source":       "my-instance",
				"method":       "POST",
				"path":         "/tool0",
				"description":  "some description",
				"requestBody":  "{}",
				"authRequired": []string{"my-generic-auth"},
			},
			"my-advanced-tool": map[string]any{
				"type":        toolType,
				"source":      "other-instance",
				"method":      "get",
				"path":        "/{{.path}}?id=2",
				"description": "some description",
				"headers": map[string]string{
					"X-Custom-Header": "example",
				},
				"pathParams": []parameters.Parameter{
					&parameters.StringParameter{
						CommonParameter: parameters.CommonParameter{Name: "path", Type: "string", Desc: "path param"},
					},
				},
				"queryParams": []parameters.Parameter{
					parameters.NewIntParameter("id", "user ID"), parameters.NewStringParameter("country", "country"),
				},
				"requestBody": `{
					"place": "zoo",
					"animals": {{json .animalArray }}
					}
					`,
				"bodyParams":   []parameters.Parameter{parameters.NewArrayParameter("animalArray", "animals in the zoo", parameters.NewStringParameter("animals", "desc"))},
				"headerParams": []parameters.Parameter{parameters.NewStringParameter("X-Other-Header", "custom header")},
			},
		},
	}
	return toolsFile
}

func getMCPHTTPSourceConfig(t *testing.T) map[string]any {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Logf("Warning: error getting ID token: %s. Using dummy token.", err)
		idToken = "dummy-token"
	}
	idToken = "Bearer " + idToken

	return map[string]any{
		"type":                 HttpSourceType,
		"headers":              map[string]string{"Authorization": idToken},
		"allowPrivateNetworks": true,
	}
}

func TestHTTPListTools(t *testing.T) {
	// Start a test server with multiTool handler
	server := httptest.NewServer(http.HandlerFunc(multiTool))
	defer server.Close()

	sourceConfig := getMCPHTTPSourceConfig(t)
	sourceConfig["baseUrl"] = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up generic auth mock server (copied from legacy test)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to create RSA private key: %v", err)
	}
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"issuer":   "https://example.com",
				"jwks_uri": "http://" + r.Host + "/jwks",
			})
			return
		}
		if r.URL.Path == "/jwks" {
			options := jwkset.JWKOptions{
				Metadata: jwkset.JWKMetadataOptions{
					KID: "test-key-id",
				},
			}
			jwk, _ := jwkset.NewJWKFromKey(privateKey.Public(), options)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"keys": []jwkset.JWKMarshal{jwk.Marshal()},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer jwksServer.Close()

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-instance": sourceConfig,
		},
		"tools": map[string]any{
			"my-simple-tool": map[string]any{
				"type":        HttpToolType,
				"path":        "/tool0",
				"method":      "POST",
				"source":      "my-instance",
				"requestBody": "{}",
				"description": "Simple tool to test end to end functionality.",
			},
			"my-tool": map[string]any{
				"type":        HttpToolType,
				"source":      "my-instance",
				"method":      "GET",
				"path":        "/tool1",
				"description": "some description",
				"queryParams": []parameters.Parameter{
					parameters.NewIntParameter("id", "user ID")},
				"bodyParams": []parameters.Parameter{parameters.NewStringParameter("name", "user name")},
				"requestBody": `{
"age": 36,
"name": "{{.name}}"
}
`,
				"headers": map[string]string{"Content-Type": "application/json"},
			},
		},
	}

	// Start the toolbox server.
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	// Wait for server ready
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	expectedTools := []tests.MCPToolManifest{
		{
			Name:        "my-simple-tool",
			Description: "Simple tool to test end to end functionality.",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{}, "required": []any{}},
		},
		{
			Name:        "my-tool",
			Description: "some description",
			InputSchema: map[string]any{
				"type":     "object",
				"required": []any{"id", "name"},
				"properties": map[string]any{
					"id": map[string]any{
						"type":        "integer",
						"description": "user ID",
					},
					"name": map[string]any{
						"type":        "string",
						"description": "user name",
					},
				},
			},
		},
	}

	tests.RunMCPToolsListMethod(t, expectedTools)
}

func TestHTTPCallTool(t *testing.T) {
	// Start a test server with multiTool handler
	server := httptest.NewServer(http.HandlerFunc(multiTool))
	defer server.Close()

	sourceConfig := getMCPHTTPSourceConfig(t)
	sourceConfig["baseUrl"] = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up generic auth mock server (needed for config generation)
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to create RSA private key: %v", err)
	}
	jwksServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/.well-known/openid-configuration" {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"issuer":   "https://example.com",
				"jwks_uri": "http://" + r.Host + "/jwks",
			})
			return
		}
		if r.URL.Path == "/jwks" {
			options := jwkset.JWKOptions{
				Metadata: jwkset.JWKMetadataOptions{
					KID: "test-key-id",
				},
			}
			jwk, _ := jwkset.NewJWKFromKey(privateKey.Public(), options)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"keys": []jwkset.JWKMarshal{jwk.Marshal()},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer jwksServer.Close()

	toolsFile := getHTTPToolsConfig(sourceConfig, HttpToolType, jwksServer.URL)

	// Start the toolbox server.
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	defer cleanup()

	// Wait for server ready
	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	// Run Generic Auth Tests
	runGenericAuthMCPInvokeTest(t, privateKey)

	// Run Advanced Tool Tests
	runAdvancedHTTPMCPInvokeTest(t)

	// Run Query Parameter Tests
	runQueryParamMCPInvokeTest(t)

	// Run URL Parameter Binding Tests
	runURLParamMCPBindingTest(t)

	// Use shared helper for standard database tools
	t.Run("use shared RunMCPToolInvokeTest", func(t *testing.T) {
		tests.RunMCPToolInvokeTest(t, `"hello world"`,
			tests.WithMyToolId3NameAliceWant(`{"id":1,"name":"Alice"}`),
			tests.WithMyToolById4Want(`{"id":4,"name":null}`),
			tests.WithNullWant("[]"),
		)
	})
}

func runURLParamMCPBindingTest(t *testing.T) {
	t.Run("tools/list with URL param filters schema", func(t *testing.T) {
		headers := tests.NewMCPRequestHeader(t, nil)
		req := tests.MCPListToolsRequest{
			Jsonrpc: "2.0",
			Id:      "1",
			Method:  "tools/list",
		}
		reqBody, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("failed to marshal list tools request: %v", err)
		}

		resp, respBody := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/mcp?id=4", bytes.NewBuffer(reqBody), headers)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var mcpResp tests.MCPListToolsResponse
		if err := json.Unmarshal(respBody, &mcpResp); err != nil {
			t.Fatalf("failed to parse list tools response: %v\nraw body: %s", err, string(respBody))
		}

		var myToolByID *tests.MCPToolManifest
		for _, tool := range mcpResp.Result.Tools {
			if tool.Name == "my-tool-by-id" {
				myToolByID = &tool
				break
			}
		}

		if myToolByID == nil {
			t.Fatalf("tool my-tool-by-id not found in tools list")
		}

		properties, ok := myToolByID.InputSchema["properties"].(map[string]any)
		if !ok {
			t.Fatalf("missing or invalid properties in inputSchema: %v", myToolByID.InputSchema)
		}

		if len(properties) != 0 {
			t.Fatalf("expected properties to be empty (filtered by URL params), got: %v", properties)
		}
	})

	t.Run("tools/call with URL param injects bound parameter", func(t *testing.T) {
		headers := tests.NewMCPRequestHeader(t, nil)
		// We call "my-tool-by-id" without passing any "id" argument in the JSON-RPC request body,
		// because "id=4" is passed as a URL query param.
		req := tests.NewMCPCallToolRequest("2", "my-tool-by-id", map[string]any{})
		reqBody, err := json.Marshal(req)
		if err != nil {
			t.Fatalf("failed to marshal call tool request: %v", err)
		}

		resp, respBody := tests.RunRequest(t, http.MethodPost, "http://127.0.0.1:5000/mcp?id=4", bytes.NewBuffer(reqBody), headers)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(respBody))
		}

		var mcpResp tests.MCPCallToolResponse
		if err := json.Unmarshal(respBody, &mcpResp); err != nil {
			t.Fatalf("failed to parse call tool response: %v\nraw body: %s", err, string(respBody))
		}

		if mcpResp.Result.IsError {
			t.Fatalf("expected success, got error: %v", mcpResp.Result)
		}

		if len(mcpResp.Result.Content) == 0 {
			t.Fatalf("expected content in result, got empty: %v", mcpResp.Result)
		}

		gotText := mcpResp.Result.Content[0].Text
		wantText := `{"id":4,"name":null}`
		if !strings.Contains(gotText, wantText) {
			t.Fatalf("unexpected response text: got %q, want it to contain %q", gotText, wantText)
		}
	})
}

func runGenericAuthMCPInvokeTest(t *testing.T, privateKey *rsa.PrivateKey) {
	// Generic Auth Success
	t.Run("invoke generic auth tool with valid token", func(t *testing.T) {
		// Generate valid token
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iss":   "https://example.com",
			"aud":   "test-audience",
			"scope": "read:files",
			"sub":   "test-user",
			"exp":   time.Now().Add(time.Hour).Unix(),
		})
		token.Header["kid"] = "test-key-id"
		signedString, err := token.SignedString(privateKey)
		if err != nil {
			t.Fatalf("failed to sign token: %v", err)
		}

		headers := map[string]string{"my-generic-auth_token": signedString}
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "my-auth-required-generic-tool", map[string]any{}, headers)
		if err != nil {
			t.Fatalf("native error executing %s: %s", "my-auth-required-generic-tool", err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", statusCode)
		}
		if mcpResp.Result.IsError {
			t.Fatalf("expected success, got error result: %v", mcpResp.Result)
		}
	})

	// Auth Failure: Invoke generic auth tool without token
	t.Run("invoke generic auth tool without token", func(t *testing.T) {
		statusCode, _, err := tests.InvokeMCPTool(t, "my-auth-required-generic-tool", map[string]any{}, nil)
		if err != nil {
			t.Fatalf("native error executing %s: %s", "my-auth-required-generic-tool", err)
		}
		if statusCode != http.StatusUnauthorized {
			t.Fatalf("expected status 401, got %d", statusCode)
		}
	})
}

func runQueryParamMCPInvokeTest(t *testing.T) {
	// Query Parameter Variations: Tests with optional parameters omitted or nil
	t.Run("invoke query-param-tool optional omitted", func(t *testing.T) {
		arguments := map[string]any{"reqId": "test1"}
		tests.RunMCPCustomToolCallMethod(t, "my-query-param-tool", arguments, `"reqId=test1"`)
	})

	t.Run("invoke query-param-tool some optional nil", func(t *testing.T) {
		arguments := map[string]any{"reqId": "test2", "page": "5", "filter": nil}
		tests.RunMCPCustomToolCallMethod(t, "my-query-param-tool", arguments, `"page=5\u0026reqId=test2"`) // 'filter' omitted!
	})

	t.Run("invoke query-param-tool some optional absent", func(t *testing.T) {
		arguments := map[string]any{"reqId": "test2", "page": "5"}
		tests.RunMCPCustomToolCallMethod(t, "my-query-param-tool", arguments, `"page=5\u0026reqId=test2"`) // 'filter' omitted!
	})

	t.Run("invoke query-param-tool required param nil", func(t *testing.T) {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "my-query-param-tool", map[string]any{"reqId": nil, "page": "1"}, nil)
		if err != nil {
			t.Fatalf("native error executing %s: %s", "my-query-param-tool", err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", statusCode)
		}
		tests.AssertMCPError(t, mcpResp, "required")
	})
}

func runAdvancedHTTPMCPInvokeTest(t *testing.T) {
	// Mock Server Error: Invoke tool with parameters that cause the mock server to return 400
	t.Run("invoke my-advanced-tool with wrong params causing mock 400", func(t *testing.T) {
		arguments := map[string]any{
			"animalArray":    []any{"rabbit", "ostrich", "whale"},
			"id":             4, // Expected 3 in mock!
			"path":           "tool3",
			"country":        "US",
			"X-Other-Header": "test",
		}
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "my-advanced-tool", arguments, nil)
		if err != nil {
			t.Fatalf("native error executing %s: %s", "my-advanced-tool", err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", statusCode)
		}
		tests.AssertMCPError(t, mcpResp, "unexpected status code")
	})

	// Advanced Tool Success
	t.Run("invoke my-advanced-tool successfully", func(t *testing.T) {
		arguments := map[string]any{
			"animalArray":    []any{"rabbit", "ostrich", "whale"},
			"id":             3,
			"path":           "tool3",
			"country":        "US",
			"X-Other-Header": "test",
		}
		tests.RunMCPCustomToolCallMethod(t, "my-advanced-tool", arguments, `"hello world"`)
	})
}
