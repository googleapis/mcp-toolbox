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

package dgraph

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	DgraphSourceType = "dgraph"
	DgraphApiKey     = "api-key"
	DgraphUrl        = os.Getenv("DGRAPH_URL")
)

const dgraphImage = "dgraph/standalone:v25.5.1"

func setupDgraphContainer(ctx context.Context, t *testing.T) string {
	t.Helper()

	req := testcontainers.ContainerRequest{
		Image:        dgraphImage,
		ExposedPorts: []string{"8080/tcp"},
		WaitingFor: wait.ForHTTP("/health").WithPort("8080/tcp").
			WithResponseMatcher(func(body io.Reader) bool {
				var health []struct {
					Status string `json:"status"`
				}
				if err := json.NewDecoder(body).Decode(&health); err != nil || len(health) == 0 {
					return false
				}
				for _, h := range health {
					if h.Status != "healthy" {
						return false
					}
				}
				return true
			}).
			WithStartupTimeout(3 * time.Minute),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})

	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("failed to start Dgraph container: %s", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get Dgraph container host: %s", err)
	}
	mappedPort, err := container.MappedPort(ctx, "8080/tcp")
	if err != nil {
		t.Fatalf("failed to get Dgraph container mapped port: %s", err)
	}
	return fmt.Sprintf("http://%s:%s", host, mappedPort.Port())
}

func useDgraph(ctx context.Context, t *testing.T) {
	t.Helper()
	if DgraphUrl != "" {
		return
	}
	t.Logf("DGRAPH_URL not set; starting ephemeral %s container", dgraphImage)
	url := setupDgraphContainer(ctx, t)
	// Restore the global so repeated runs (-count>1) start a fresh container.
	origUrl := DgraphUrl
	t.Cleanup(func() { DgraphUrl = origUrl })
	DgraphUrl = url
}

func getDgraphVars(t *testing.T) map[string]any {
	if DgraphUrl == "" {
		t.Fatal("'DGRAPH_URL' not set")
	}
	return map[string]any{
		"type":      DgraphSourceType,
		"dgraphUrl": DgraphUrl,
		"apiKey":    DgraphApiKey,
	}
}

func TestDgraphToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	useDgraph(ctx, t)
	sourceConfig := getDgraphVars(t)

	args := []string{"--enable-api"}

	// Write config into a file and pass it to command
	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-dgraph-instance": sourceConfig,
		},
		"tools": map[string]any{
			"my-simple-dql-tool": map[string]any{
				"type":        "dgraph-dql",
				"source":      "my-dgraph-instance",
				"description": "Simple tool to test end to end functionality.",
				"statement":   "{result(func: uid(0x0)) {constant: math(1)}}",
				"isQuery":     true,
				"timeout":     "20s",
				"parameters":  []any{},
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

	// Test tool get endpoint
	tcs := []struct {
		name string
		api  string
		want map[string]any
	}{
		{
			name: "get my-simple-tool",
			api:  "http://127.0.0.1:5000/api/tool/my-simple-dql-tool/",
			want: map[string]any{
				"my-simple-dql-tool": map[string]any{
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

	// Test tool invoke endpoint
	invokeTcs := []struct {
		name        string
		api         string
		requestBody io.Reader
		want        string
	}{
		{
			name:        "invoke my-simple-dql-tool",
			api:         "http://127.0.0.1:5000/api/tool/my-simple-dql-tool/invoke",
			requestBody: bytes.NewBuffer([]byte(`{}`)),
			want:        "{\"result\":[{\"constant\":1}]}",
		},
	}
	for _, tc := range invokeTcs {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Post(tc.api, "application/json", tc.requestBody)
			if err != nil {
				t.Fatalf("error when sending a request: %s", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Fatalf("response status code is not 200, got %d: %s", resp.StatusCode, string(bodyBytes))
			}

			var body map[string]interface{}
			err = json.NewDecoder(resp.Body).Decode(&body)
			if err != nil {
				t.Fatalf("error parsing response body")
			}
			got, ok := body["result"].(string)

			if !ok {
				t.Fatalf("unable to find result in response body")
			}

			if got != tc.want {
				t.Fatalf("unexpected value: got %q, want %q", got, tc.want)
			}
		})
	}
}
