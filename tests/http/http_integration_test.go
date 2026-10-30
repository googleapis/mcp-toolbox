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

package http

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/MicahParks/jwkset"
	"github.com/golang-jwt/jwt/v5"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
)

func getHTTPSourceConfig(t *testing.T) map[string]any {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting ID token: %s", err)
	}
	idToken = "Bearer " + idToken

	return map[string]any{
		"type":                 HttpSourceType,
		"headers":              map[string]string{"Authorization": idToken},
		"allowPrivateNetworks": true,
	}
}

func TestHttpToolEndpoints(t *testing.T) {
	// start a test server
	server := httptest.NewServer(http.HandlerFunc(multiTool))
	defer server.Close()

	sourceConfig := getHTTPSourceConfig(t)
	sourceConfig["baseUrl"] = server.URL
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	// Set up generic auth mock server
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

	args := []string{"--enable-api"}

	toolsFile := getHTTPToolsConfig(sourceConfig, HttpToolType, jwksServer.URL)
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

	// Run tests
	tests.RunToolGetTest(t)
	tests.RunToolInvokeTest(t, `"hello world"`, tests.DisableArrayTest())
	runAdvancedHTTPInvokeTest(t)
	runQueryParamInvokeTest(t)
	runGenericAuthInvokeTest(t, privateKey)
}

func runGenericAuthInvokeTest(t *testing.T, privateKey *rsa.PrivateKey) {
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

	api := "http://127.0.0.1:5000/api/tool/my-auth-required-generic-tool/invoke"

	// Test without auth header (should fail)
	t.Run("invoke generic auth tool without token", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, api, bytes.NewBuffer([]byte(`{}`)))
		req.Header.Add("Content-type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("unable to send request: %s", err)
		}
		defer resp.Body.Close()

		var body map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		errorStr, _ := body["error"].(string)
		statusStr, _ := body["status"].(string)
		if !strings.Contains(strings.ToLower(errorStr), "not authorized") && !strings.Contains(strings.ToLower(statusStr), "unauthorized") {
			bodyBytes, _ := json.Marshal(body)
			t.Fatalf("expected unauthorized error, got: %s", string(bodyBytes))
		}
	})

	// Test with valid token
	t.Run("invoke generic auth tool with valid token", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, api, bytes.NewBuffer([]byte(`{}`)))
		req.Header.Add("Content-type", "application/json")
		req.Header.Add("my-generic-auth_token", signedString)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("unable to send request: %s", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			bodyBytes, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected status 200, got %d: %s", resp.StatusCode, string(bodyBytes))
		}

		var body map[string]interface{}
		_ = json.NewDecoder(resp.Body).Decode(&body)
		got, ok := body["result"].(string)
		if !ok || got != `"hello world"` {
			bodyBytes, _ := json.Marshal(body)
			t.Fatalf("unexpected result: %s", string(bodyBytes))
		}
	})
}

// runQueryParamInvokeTest runs the tool invoke endpoint for the query param test tool
func runQueryParamInvokeTest(t *testing.T) {
	invokeTcs := []struct {
		name        string
		api         string
		requestBody io.Reader
		want        string
		isErr       bool
	}{
		{
			name:        "invoke query-param-tool (optional omitted)",
			api:         "http://127.0.0.1:5000/api/tool/my-query-param-tool/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"reqId": "test1"}`)),
			want:        `"reqId=test1"`,
		},
		{
			name:        "invoke query-param-tool (some optional nil)",
			api:         "http://127.0.0.1:5000/api/tool/my-query-param-tool/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"reqId": "test2", "page": "5", "filter": null}`)),
			want:        `"page=5\u0026reqId=test2"`, // 'filter' omitted
		},
		{
			name:        "invoke query-param-tool (some optional absent)",
			api:         "http://127.0.0.1:5000/api/tool/my-query-param-tool/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"reqId": "test2", "page": "5"}`)),
			want:        `"page=5\u0026reqId=test2"`, // 'filter' omitted
		},
		{
			name:        "invoke query-param-tool (required param nil)",
			api:         "http://127.0.0.1:5000/api/tool/my-query-param-tool/invoke",
			requestBody: bytes.NewBuffer([]byte(`{"reqId": null, "page": "1"}`)),
			want:        `{"error":"parameter \"reqId\" is required"}`,
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			// Send Tool invocation request
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody)
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

			// Check response body
			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body: %v", err)
			}
			got, ok := body["result"].(string)
			if !ok {
				bodyBytes, _ := json.Marshal(body)
				t.Fatalf("unable to find result in response body, got: %s", string(bodyBytes))
			}

			if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}

func runAdvancedHTTPInvokeTest(t *testing.T) {
	// Test HTTP tool invoke endpoint
	invokeTcs := []struct {
		name          string
		api           string
		requestHeader map[string]string
		requestBody   func() io.Reader
		want          string
		isAgentErr    bool
	}{
		{
			name:          "invoke my-advanced-tool",
			api:           "http://127.0.0.1:5000/api/tool/my-advanced-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: func() io.Reader {
				return bytes.NewBuffer([]byte(`{"animalArray": ["rabbit", "ostrich", "whale"], "id": 3, "path": "tool3", "country": "US", "X-Other-Header": "test"}`))
			},
			want:       `"hello world"`,
			isAgentErr: false,
		},
		{
			name:          "invoke my-advanced-tool with wrong params",
			api:           "http://127.0.0.1:5000/api/tool/my-advanced-tool/invoke",
			requestHeader: map[string]string{},
			requestBody: func() io.Reader {
				return bytes.NewBuffer([]byte(`{"animalArray": ["rabbit", "ostrich", "whale"], "id": 4, "path": "tool3", "country": "US", "X-Other-Header": "test"}`))
			},
			want:       "error processing request: unexpected status code: 400 (Bad Request)",
			isAgentErr: true,
		},
	}

	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(http.MethodPost, tc.api, tc.requestBody())
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

			// As you noted, the toolbox wraps errors in a 200 OK
			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("expected status 200 from toolbox, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			// Decode the response body into a map
			var body map[string]any
			if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
				t.Fatalf("failed to decode response: %v", err)
			}

			if tc.isAgentErr {
				resStr, ok := body["result"].(string)
				if !ok {
					t.Fatalf("expected 'result' field as string in response body, got: %v", body)
				}

				var resMap map[string]any
				if err := json.Unmarshal([]byte(resStr), &resMap); err != nil {
					t.Fatalf("failed to unmarshal result string: %v", err)
				}

				gotErr, ok := resMap["error"].(string)
				if !ok {
					t.Fatalf("expected 'error' field inside result, got: %v", resMap)
				}

				if !strings.Contains(gotErr, tc.want) {
					t.Fatalf("unexpected error message: got %q, want it to contain %q", gotErr, tc.want)
				}
			} else {
				got, ok := body["result"].(string)
				if !ok {
					resBytes, _ := json.Marshal(body["result"])
					got = string(resBytes)
				}

				if got != tc.want {
					t.Fatalf("unexpected result: got %q, want %q", got, tc.want)
				}
			}
		})
	}
}
