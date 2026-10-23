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

package serverlessspark

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	dataproc "cloud.google.com/go/dataproc/v2/apiv1"
	"cloud.google.com/go/dataproc/v2/apiv1/dataprocpb"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/sources/serverlessspark"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/testing/protocmp"
)

var (
	serverlessSparkLocation       = os.Getenv("SERVERLESS_SPARK_LOCATION")
	serverlessSparkProject        = os.Getenv("SERVERLESS_SPARK_PROJECT")
	serverlessSparkServiceAccount = os.Getenv("SERVERLESS_SPARK_SERVICE_ACCOUNT")
)

const (
	batchURLPrefix = "https://console.cloud.google.com/dataproc/batches/"
	logsURLPrefix  = "https://console.cloud.google.com/logs/viewer?"
)

func getServerlessSparkVars(t *testing.T) map[string]any {
	switch "" {
	case serverlessSparkLocation:
		t.Fatal("'SERVERLESS_SPARK_LOCATION' not set")
	case serverlessSparkProject:
		t.Fatal("'SERVERLESS_SPARK_PROJECT' not set")
	case serverlessSparkServiceAccount:
		t.Fatal("'SERVERLESS_SPARK_SERVICE_ACCOUNT' not set")
	}

	return map[string]any{
		"type":     "serverless-spark",
		"project":  serverlessSparkProject,
		"location": serverlessSparkLocation,
	}
}

func setupServerlessSparkTest(t *testing.T) context.Context {
	t.Helper()
	sourceConfig := getServerlessSparkVars(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	t.Cleanup(cancel)

	toolsFile := map[string]any{
		"sources": map[string]any{
			"my-spark": sourceConfig,
		},
		"authServices": map[string]any{
			"my-google-auth": map[string]any{
				"type":     "google",
				"clientId": tests.ClientId,
			},
		},
		"tools": map[string]any{
			"list-batches": map[string]any{
				"type":   "serverless-spark-list-batches",
				"source": "my-spark",
			},
			"list-batches-with-auth": map[string]any{
				"type":         "serverless-spark-list-batches",
				"source":       "my-spark",
				"authRequired": []string{"my-google-auth"},
			},
			"list-sessions": map[string]any{
				"type":   "serverless-spark-list-sessions",
				"source": "my-spark",
			},
			"list-sessions-with-auth": map[string]any{
				"type":         "serverless-spark-list-sessions",
				"source":       "my-spark",
				"authRequired": []string{"my-google-auth"},
			},
			"get-batch": map[string]any{
				"type":   "serverless-spark-get-batch",
				"source": "my-spark",
			},
			"get-batch-with-auth": map[string]any{
				"type":         "serverless-spark-get-batch",
				"source":       "my-spark",
				"authRequired": []string{"my-google-auth"},
			},
			"get-session": map[string]any{
				"type":   "serverless-spark-get-session",
				"source": "my-spark",
			},
			"get-session-with-auth": map[string]any{
				"type":         "serverless-spark-get-session",
				"source":       "my-spark",
				"authRequired": []string{"my-google-auth"},
			},
			"cancel-batch": map[string]any{
				"type":   "serverless-spark-cancel-batch",
				"source": "my-spark",
			},
			"cancel-batch-with-auth": map[string]any{
				"type":         "serverless-spark-cancel-batch",
				"source":       "my-spark",
				"authRequired": []string{"my-google-auth"},
			},
			"create-pyspark-batch": map[string]any{
				"type":   "serverless-spark-create-pyspark-batch",
				"source": "my-spark",
				"environmentConfig": map[string]any{
					"executionConfig": map[string]any{
						"serviceAccount": serverlessSparkServiceAccount,
					},
				},
			},
			"create-pyspark-batch-2-3": map[string]any{
				"type":          "serverless-spark-create-pyspark-batch",
				"source":        "my-spark",
				"runtimeConfig": map[string]any{"version": "2.3"},
				"environmentConfig": map[string]any{
					"executionConfig": map[string]any{
						"serviceAccount": serverlessSparkServiceAccount,
					},
				},
			},
			"create-pyspark-batch-with-auth": map[string]any{
				"type":         "serverless-spark-create-pyspark-batch",
				"source":       "my-spark",
				"authRequired": []string{"my-google-auth"},
			},
			"create-spark-batch": map[string]any{
				"type":   "serverless-spark-create-spark-batch",
				"source": "my-spark",
				"environmentConfig": map[string]any{
					"executionConfig": map[string]any{
						"serviceAccount": serverlessSparkServiceAccount,
					},
				},
			},
			"create-spark-batch-2-3": map[string]any{
				"type":          "serverless-spark-create-spark-batch",
				"source":        "my-spark",
				"runtimeConfig": map[string]any{"version": "2.3"},
				"environmentConfig": map[string]any{
					"executionConfig": map[string]any{
						"serviceAccount": serverlessSparkServiceAccount,
					},
				},
			},
			"create-spark-batch-with-auth": map[string]any{
				"type":         "serverless-spark-create-spark-batch",
				"source":       "my-spark",
				"authRequired": []string{"my-google-auth"},
			},
			"get-session-template": map[string]any{
				"kind":   "serverless-spark-get-session-template",
				"source": "my-spark",
			},
			"get-session-template-with-auth": map[string]any{
				"kind":         "serverless-spark-get-session-template",
				"source":       "my-spark",
				"authRequired": []string{"my-google-auth"},
			},
		},
	}

	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(func() {
		cmd.Stop()
		waitCtx, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if err := cmd.Wait(waitCtx); err != nil {
			t.Errorf("stop toolbox: %v", err)
		}
		cmd.Close()
	})

	waitCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	return ctx
}

func runServerlessSparkCallTests(t *testing.T, ctx context.Context) {
	endpoint := fmt.Sprintf("%s-dataproc.googleapis.com:443", serverlessSparkLocation)
	batchClient, err := dataproc.NewBatchControllerClient(ctx, option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("failed to create dataproc batch client: %v", err)
	}
	defer batchClient.Close()

	sessionTemplateClient, err := dataproc.NewSessionTemplateControllerClient(ctx, option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("failed to create dataproc session template client: %v", err)
	}
	defer sessionTemplateClient.Close()

	sessionClient, err := dataproc.NewSessionControllerClient(ctx, option.WithEndpoint(endpoint))
	if err != nil {
		t.Fatalf("failed to create dataproc session client: %v", err)
	}
	defer sessionClient.Close()

	t.Run("list-batches", func(t *testing.T) {
		// list-batches is sensitive to state changes, so this test must run sequentially.
		t.Run("success", func(t *testing.T) {
			runListBatchesTest(t, batchClient, ctx)
		})
		t.Run("errors", func(t *testing.T) {
			t.Parallel()
			tcs := []struct {
				name     string
				toolName string
				request  map[string]any
				wantCode int
				wantMsg  string
			}{
				{
					name:     "zero page size",
					toolName: "list-batches",
					request:  map[string]any{"pageSize": 0},
					wantCode: http.StatusOK,
					wantMsg:  "pageSize must be positive: 0",
				},
				{
					name:     "negative page size",
					toolName: "list-batches",
					request:  map[string]any{"pageSize": -1},
					wantCode: http.StatusOK,
					wantMsg:  "pageSize must be positive: -1",
				},
			}
			for _, tc := range tcs {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					testError(t, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
				})
			}
		})
		t.Run("auth", func(t *testing.T) {
			t.Parallel()
			runAuthTest(t, "list-batches-with-auth", map[string]any{"pageSize": 1}, http.StatusOK)
		})
	})

	t.Run("list-sessions", func(t *testing.T) {
		t.Run("success", func(t *testing.T) {
			// Just verify it runs without error, as we might not have sessions.
			runListSessionsTest(t, sessionClient, ctx)
		})
		t.Run("errors", func(t *testing.T) {
			t.Parallel()
			tcs := []struct {
				name     string
				toolName string
				request  map[string]any
				wantCode int
				wantMsg  string
			}{
				{
					name:     "zero page size",
					toolName: "list-sessions",
					request:  map[string]any{"pageSize": 0},
					wantCode: http.StatusOK,
					wantMsg:  "pageSize must be positive: 0",
				},
				{
					name:     "negative page size",
					toolName: "list-sessions",
					request:  map[string]any{"pageSize": -1},
					wantCode: http.StatusOK,
					wantMsg:  "pageSize must be positive: -1",
				},
			}
			for _, tc := range tcs {
				t.Run(tc.name, func(t *testing.T) {
					t.Parallel()
					testError(t, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
				})
			}
		})
		t.Run("auth", func(t *testing.T) {
			t.Parallel()
			runAuthTest(t, "list-sessions-with-auth", map[string]any{"pageSize": 1}, http.StatusOK)
		})
	})

	// The following tool tests are independent and can run in parallel with each other.
	t.Run("parallel-tool-tests", func(t *testing.T) {
		t.Run("get-batch", func(t *testing.T) {
			t.Parallel()
			fullName := listBatchesRpc(t, batchClient, ctx, "", 1, true)[0].Name
			t.Run("success", func(t *testing.T) {
				t.Parallel()
				runGetBatchTest(t, batchClient, ctx, fullName)
			})
			t.Run("errors", func(t *testing.T) {
				t.Parallel()
				missingBatchFullName := fmt.Sprintf("projects/%s/locations/%s/batches/INVALID_BATCH", serverlessSparkProject, serverlessSparkLocation)
				tcs := []struct {
					name     string
					toolName string
					request  map[string]any
					wantCode int
					wantMsg  string
				}{
					{
						name:     "missing batch",
						toolName: "get-batch",
						request:  map[string]any{"name": "INVALID_BATCH"},
						wantCode: http.StatusOK,
						wantMsg:  fmt.Sprintf("error processing GCP request: failed to get batch: rpc error: code = NotFound desc = Not found: Batch projects/%s/locations/%s/batches/INVALID_BATCH", serverlessSparkProject, serverlessSparkLocation),
					},
					{
						name:     "full batch name",
						toolName: "get-batch",
						request:  map[string]any{"name": missingBatchFullName},
						wantCode: http.StatusOK,
						wantMsg:  fmt.Sprintf("name must be a short batch name without '/': %s", missingBatchFullName),
					},
				}
				for _, tc := range tcs {
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						testError(t, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
					})
				}
			})
			t.Run("auth", func(t *testing.T) {
				t.Parallel()
				runAuthTest(t, "get-batch-with-auth", map[string]any{"name": shortName(fullName)}, http.StatusOK)
			})
		})

		t.Run("get-session", func(t *testing.T) {
			t.Parallel()
			// Try to find an existing session to test success case
			sessions := listSessionsRpc(t, sessionClient, ctx, "", 1, false)
			if len(sessions) > 0 {
				fullName := sessions[0].Name
				t.Run("success", func(t *testing.T) {
					t.Parallel()
					runGetSessionTest(t, sessionClient, ctx, fullName)
				})
				t.Run("auth", func(t *testing.T) {
					t.Parallel()
					runAuthTest(t, "get-session-with-auth", map[string]any{"name": shortName(fullName)}, http.StatusOK)
				})
			}

			t.Run("errors", func(t *testing.T) {
				t.Parallel()
				missingSessionFullName := fmt.Sprintf("projects/%s/locations/%s/sessions/INVALID_SESSION", serverlessSparkProject, serverlessSparkLocation)
				tcs := []struct {
					name     string
					toolName string
					request  map[string]any
					wantCode int
					wantMsg  string
				}{
					{
						name:     "missing session",
						toolName: "get-session",
						request:  map[string]any{"name": "INVALID_SESSION"},
						wantCode: http.StatusOK,
						wantMsg:  fmt.Sprintf("Not found: Session projects/%s/locations/%s/sessions/INVALID_SESSION", serverlessSparkProject, serverlessSparkLocation),
					},
					{
						name:     "full session name",
						toolName: "get-session",
						request:  map[string]any{"name": missingSessionFullName},
						wantCode: http.StatusOK,
						wantMsg:  fmt.Sprintf("name must be a short session name without '/': %s", missingSessionFullName),
					},
				}
				for _, tc := range tcs {
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						testError(t, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
					})
				}
			})
		})

		t.Run("get-session-template", func(t *testing.T) {
			t.Parallel()
			// Try to find an existing session template to test success case
			sessionTemplates := listSessionTemplatesRpc(t, sessionTemplateClient, ctx, "", 1, false)
			if len(sessionTemplates) > 0 {
				fullName := sessionTemplates[0].Name
				t.Run("success", func(t *testing.T) {
					t.Parallel()
					runGetSessionTemplateTest(t, sessionTemplateClient, ctx, fullName)
				})
				t.Run("auth", func(t *testing.T) {
					t.Parallel()
					runAuthTest(t, "get-session-template-with-auth", map[string]any{"name": shortName(fullName)}, http.StatusOK)
				})
			}

			t.Run("errors", func(t *testing.T) {
				t.Parallel()
				missingSessionTemplateFullName := fmt.Sprintf("projects/%s/locations/%s/sessionTemplates/INVALID_SESSION_TEMPLATE", serverlessSparkProject, serverlessSparkLocation)
				tcs := []struct {
					name     string
					toolName string
					request  map[string]any
					wantCode int
					wantMsg  string
				}{
					{
						name:     "missing session template",
						toolName: "get-session-template",
						request:  map[string]any{"name": "INVALID_SESSION_TEMPLATE"},
						wantCode: http.StatusOK,
						wantMsg:  fmt.Sprintf("Not found: Session Template projects/%s/locations/%s/sessionTemplates/INVALID_SESSION_TEMPLATE", serverlessSparkProject, serverlessSparkLocation),
					},
					{
						name:     "full session name",
						toolName: "get-session-template",
						request:  map[string]any{"name": missingSessionTemplateFullName},
						wantCode: http.StatusOK,
						wantMsg:  fmt.Sprintf("name must be a short session template name without '/': %s", missingSessionTemplateFullName),
					},
				}
				for _, tc := range tcs {
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						testError(t, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
					})
				}
			})
		})

		t.Run("create-pyspark-batch", func(t *testing.T) {
			t.Parallel()

			t.Run("success", func(t *testing.T) {
				t.Parallel()
				piPy := "file:///usr/lib/spark/examples/src/main/python/pi.py"
				tcs := []struct {
					name           string
					toolName       string
					request        map[string]any
					waitForSuccess bool
					validate       func(t *testing.T, b *dataprocpb.Batch)
				}{
					{
						name:           "no params",
						toolName:       "create-pyspark-batch",
						waitForSuccess: true,
						request:        map[string]any{"mainFile": piPy},
					},
					// Tests below are just verifying options are set correctly on created batches,
					// they don't need to wait for success.
					{
						name:     "with arg",
						toolName: "create-pyspark-batch",
						request:  map[string]any{"mainFile": piPy, "args": []string{"100"}},
						validate: func(t *testing.T, b *dataprocpb.Batch) {
							if !cmp.Equal(b.GetPysparkBatch().Args, []string{"100"}) {
								t.Errorf("unexpected args: got %v, want %v", b.GetPysparkBatch().Args, []string{"100"})
							}
						},
					},
					{
						name:     "version",
						toolName: "create-pyspark-batch",
						request:  map[string]any{"mainFile": piPy, "version": "2.2"},
						validate: func(t *testing.T, b *dataprocpb.Batch) {
							v := b.GetRuntimeConfig().GetVersion()
							if v != "2.2" {
								t.Errorf("unexpected version: got %v, want 2.2", v)
							}
						},
					},
					{
						name:     "version param overrides tool",
						toolName: "create-pyspark-batch-2-3",
						request:  map[string]any{"mainFile": piPy, "version": "2.2"},
						validate: func(t *testing.T, b *dataprocpb.Batch) {
							v := b.GetRuntimeConfig().GetVersion()
							if v != "2.2" {
								t.Errorf("unexpected version: got %v, want 2.2", v)
							}
						},
					},
				}
				for _, tc := range tcs {
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						runCreateSparkBatchTest(t, batchClient, ctx, tc.toolName, tc.request, tc.waitForSuccess, tc.validate)
					})
				}
			})

			t.Run("auth", func(t *testing.T) {
				t.Parallel()
				// Batch creation succeeds even with an invalid main file, but will fail quickly once running.
				runAuthTest(t, "create-pyspark-batch-with-auth", map[string]any{"mainFile": "file:///placeholder"}, http.StatusOK)
			})

			t.Run("errors", func(t *testing.T) {
				t.Parallel()
				tcs := []struct {
					name    string
					request map[string]any
					wantMsg string
				}{
					{
						name:    "missing main file",
						request: map[string]any{},
						wantMsg: `parameter "mainFile" is required`,
					},
				}
				for _, tc := range tcs {
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						testError(t, "create-pyspark-batch", tc.request, http.StatusOK, tc.wantMsg)
					})
				}
			})
		})

		t.Run("create-spark-batch", func(t *testing.T) {
			t.Parallel()

			t.Run("success", func(t *testing.T) {
				t.Parallel()
				tcs := []struct {
					name           string
					toolName       string
					request        map[string]any
					waitForSuccess bool
					validate       func(t *testing.T, b *dataprocpb.Batch)
				}{
					{
						name:           "main class",
						toolName:       "create-spark-batch",
						waitForSuccess: true,
						request:        javaReq(map[string]any{}),
					},
					{
						// spark-examples.jar doesn't have a Main-Class, so pick an arbitrary other
						// jar that does. Note there's a chance a subminor release of 2.2 will
						// upgrade Spark and its dependencies, causing a failure. If that happens,
						// find the new ivy jar filename and use that. The alternative would be to
						// pin a subminor version, but that's guaranteed to be GC'ed after 1 year,
						// whereas 2.2 is old enough it's unlikely to see a Spark version bump.
						name:           "main jar",
						toolName:       "create-spark-batch",
						waitForSuccess: true,
						request: map[string]any{
							"version":     "2.2",
							"mainJarFile": "file:///usr/lib/spark/jars/ivy-2.5.2.jar",
							"args":        []string{"-version"},
						},
					},
					// Tests below are just verifying options are set correctly on created batches,
					// they don't need to wait for success.
					{
						name:     "with arg",
						toolName: "create-spark-batch",
						request:  javaReq(map[string]any{"args": []string{"100"}}),
						validate: func(t *testing.T, b *dataprocpb.Batch) {
							if !cmp.Equal(b.GetSparkBatch().Args, []string{"100"}) {
								t.Errorf("unexpected args: got %v, want %v", b.GetSparkBatch().Args, []string{"100"})
							}
						},
					},
					{
						name:     "version",
						toolName: "create-spark-batch",
						request:  javaReq(map[string]any{"version": "2.2"}),
						validate: func(t *testing.T, b *dataprocpb.Batch) {
							v := b.GetRuntimeConfig().GetVersion()
							if v != "2.2" {
								t.Errorf("unexpected version: got %v, want 2.2", v)
							}
						},
					},
					{
						name:     "version param overrides tool",
						toolName: "create-spark-batch-2-3",
						request:  javaReq(map[string]any{"version": "2.2"}),
						validate: func(t *testing.T, b *dataprocpb.Batch) {
							v := b.GetRuntimeConfig().GetVersion()
							if v != "2.2" {
								t.Errorf("unexpected version: got %v, want 2.2", v)
							}
						},
					},
				}
				for _, tc := range tcs {
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						runCreateSparkBatchTest(t, batchClient, ctx, tc.toolName, tc.request, tc.waitForSuccess, tc.validate)
					})
				}
			})

			t.Run("auth", func(t *testing.T) {
				t.Parallel()
				// Batch creation succeeds even with an invalid main file, but will fail quickly once running.
				runAuthTest(t, "create-spark-batch-with-auth", map[string]any{"mainJarFile": "file:///placeholder"}, http.StatusOK)
			})

			t.Run("errors", func(t *testing.T) {
				t.Parallel()
				tcs := []struct {
					name    string
					request map[string]any
					wantMsg string
				}{
					{
						name:    "no main jar or main class",
						request: map[string]any{},
						wantMsg: "must provide either mainJarFile or mainClass",
					},
					{
						name: "both main jar and main class",
						request: map[string]any{
							"mainJarFile": "my.jar",
							"mainClass":   "com.example.MyClass",
						},
						wantMsg: "cannot provide both mainJarFile and mainClass",
					},
					{
						name: "main class without jar files",
						request: map[string]any{
							"mainClass": "com.example.MyClass",
						},
						wantMsg: "jarFiles is required when mainClass is provided",
					},
				}
				for _, tc := range tcs {
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						testError(t, "create-spark-batch", tc.request, http.StatusOK, tc.wantMsg)
					})
				}
			})
		})

		t.Run("cancel-batch", func(t *testing.T) {
			t.Parallel()
			t.Run("success", func(t *testing.T) {
				t.Parallel()
				tcs := []struct {
					name         string
					getBatchName func(t *testing.T) string
				}{
					{
						name: "running batch",
						getBatchName: func(t *testing.T) string {
							return createBatch(t, batchClient, ctx)
						},
					},
					{
						name: "succeeded batch",
						getBatchName: func(t *testing.T) string {
							return listBatchesRpc(t, batchClient, ctx, "state = SUCCEEDED", 1, true)[0].Name
						},
					},
				}

				for _, tc := range tcs {
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						runCancelBatchTest(t, batchClient, ctx, tc.getBatchName(t))
					})
				}
			})
			t.Run("errors", func(t *testing.T) {
				t.Parallel()
				// Find a batch that's already completed.
				completedBatchOp := listBatchesRpc(t, batchClient, ctx, "state = SUCCEEDED", 1, true)[0].Operation
				fullOpName := fmt.Sprintf("projects/%s/locations/%s/operations/%s", serverlessSparkProject, serverlessSparkLocation, shortName(completedBatchOp))
				tcs := []struct {
					name     string
					toolName string
					request  map[string]any
					wantCode int
					wantMsg  string
				}{
					{
						name:     "missing op parameter",
						toolName: "cancel-batch",
						request:  map[string]any{},
						wantCode: http.StatusOK,
						wantMsg:  `parameter "operation" is required`,
					},
					{
						name:     "nonexistent op",
						toolName: "cancel-batch",
						request:  map[string]any{"operation": "INVALID_OPERATION"},
						wantCode: http.StatusOK,
						wantMsg:  "error processing GCP request: failed to cancel operation: rpc error: code = NotFound desc = Operation not found",
					},
					{
						name:     "full op name",
						toolName: "cancel-batch",
						request:  map[string]any{"operation": fullOpName},
						wantCode: http.StatusOK,
						wantMsg:  fmt.Sprintf("operation must be a short operation name without '/': %s", fullOpName),
					},
				}
				for _, tc := range tcs {
					t.Run(tc.name, func(t *testing.T) {
						t.Parallel()
						testError(t, tc.toolName, tc.request, tc.wantCode, tc.wantMsg)
					})
				}
			})
			t.Run("auth", func(t *testing.T) {
				t.Parallel()
				runAuthTest(t, "cancel-batch-with-auth", map[string]any{"operation": "INVALID_OPERATION"}, http.StatusOK)
			})
		})
	})
}

func waitForBatch(t *testing.T, client *dataproc.BatchControllerClient, parentCtx context.Context, batch string, desiredStates []dataprocpb.Batch_State, timeout time.Duration) {
	t.Logf("waiting %s for batch %s to reach one of %v", timeout, batch, desiredStates)
	ctx, cancel := context.WithTimeout(parentCtx, timeout)
	defer cancel()

	start := time.Now()
	lastLog := start
	for {
		select {
		case <-ctx.Done():
			t.Fatalf("timed out waiting for batch %s to reach one of %v", batch, desiredStates)
		default:
		}

		getReq := &dataprocpb.GetBatchRequest{Name: batch}
		batch, err := client.GetBatch(ctx, getReq)
		if err != nil {
			t.Fatalf("failed to get batch %s: %v", batch, err)
		}

		now := time.Now()
		if now.Sub(lastLog) >= 30*time.Second {
			t.Logf("%s: batch %s is in state %s after %s", t.Name(), batch.Name, batch.State, now.Sub(start))
			lastLog = now
		}

		if slices.Contains(desiredStates, batch.State) {
			return
		}

		if batch.State == dataprocpb.Batch_FAILED || batch.State == dataprocpb.Batch_CANCELLED || batch.State == dataprocpb.Batch_SUCCEEDED {
			t.Fatalf("batch op %s is in a terminal state %s, but wanted one of %v. State message: %s", batch.Name, batch.State, desiredStates, batch.StateMessage)
		}
		time.Sleep(2 * time.Second)
	}
}

// createBatch creates a test batch and immediately returns the batch name, without waiting for the
// batch to start or complete.
func createBatch(t *testing.T, client *dataproc.BatchControllerClient, ctx context.Context) string {
	parent := fmt.Sprintf("projects/%s/locations/%s", serverlessSparkProject, serverlessSparkLocation)
	req := &dataprocpb.CreateBatchRequest{
		Parent: parent,
		Batch: &dataprocpb.Batch{
			BatchConfig: &dataprocpb.Batch_SparkBatch{
				SparkBatch: &dataprocpb.SparkBatch{
					Driver: &dataprocpb.SparkBatch_MainClass{
						MainClass: "org.apache.spark.examples.SparkPi",
					},
					JarFileUris: []string{
						"file:///usr/lib/spark/examples/jars/spark-examples.jar",
					},
					Args: []string{"1000"},
				},
			},
			EnvironmentConfig: &dataprocpb.EnvironmentConfig{
				ExecutionConfig: &dataprocpb.ExecutionConfig{
					ServiceAccount: serverlessSparkServiceAccount,
				},
			},
		},
	}

	createOp, err := client.CreateBatch(ctx, req)
	if err != nil {
		t.Fatalf("failed to create batch: %v", err)
	}
	meta, err := createOp.Metadata()
	if err != nil {
		t.Fatalf("failed to get batch metadata: %v", err)
	}

	// Wait for the batch to become at least PENDING; it typically takes >10s to go from PENDING to
	// RUNNING, giving the cancel batch tests plenty of time to cancel it before it completes.
	waitForBatch(t, client, ctx, meta.Batch, []dataprocpb.Batch_State{dataprocpb.Batch_PENDING, dataprocpb.Batch_RUNNING}, 1*time.Minute)
	return meta.Batch
}

func runCancelBatchTest(t *testing.T, client *dataproc.BatchControllerClient, ctx context.Context, batchName string) {
	// First get the batch details directly from the Go proto API.
	batch, err := client.GetBatch(ctx, &dataprocpb.GetBatchRequest{Name: batchName})
	if err != nil {
		t.Fatalf("failed to get batch: %s", err)
	}

	request := map[string]any{"operation": shortName(batch.Operation)}
	result := invokeMCPResult(t, "cancel-batch", request, nil)
	if want := fmt.Sprintf("Cancelled [%s].", shortName(batch.Operation)); result != want {
		t.Fatalf("cancellation result = %q, want %q", result, want)
	}

	if batch.State != dataprocpb.Batch_SUCCEEDED {
		waitForBatch(t, client, ctx, batchName, []dataprocpb.Batch_State{dataprocpb.Batch_CANCELLING, dataprocpb.Batch_CANCELLED}, 2*time.Minute)
	}
}

// runListBatchesTest invokes the running list-batches tool and ensures it returns the correct
// number of results. It can run successfully against any GCP project that contains at least 2 total
// Serverless Spark batches.
func runListBatchesTest(t *testing.T, client *dataproc.BatchControllerClient, ctx context.Context) {
	batch2 := listBatchesRpc(t, client, ctx, "", 2, true)
	batch20 := listBatchesRpc(t, client, ctx, "", 20, false)

	tcs := []struct {
		name     string
		filter   string
		pageSize int
		numPages int
		want     []serverlessspark.Batch
	}{
		{name: "one page", pageSize: 2, numPages: 1, want: batch2},
		{name: "two pages", pageSize: 1, numPages: 2, want: batch2},
		{name: "20 batches", pageSize: 20, numPages: 1, want: batch20},
		{name: "omit page size", numPages: 1, want: batch20},
		{
			name:     "filtered",
			filter:   "state = SUCCEEDED",
			pageSize: 2,
			numPages: 1,
			want:     listBatchesRpc(t, client, ctx, "state = SUCCEEDED", 2, true),
		},
		{
			name:     "empty",
			filter:   "state = SUCCEEDED AND state = FAILED",
			pageSize: 1,
			numPages: 1,
			want:     nil,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			var actual []serverlessspark.Batch
			var pageToken string
			for i := 0; i < tc.numPages; i++ {
				request := map[string]any{
					"filter":    tc.filter,
					"pageToken": pageToken,
				}
				if tc.pageSize > 0 {
					request["pageSize"] = tc.pageSize
				}

				result := invokeMCPResult(t, "list-batches", request, nil)

				var listResponse serverlessspark.ListBatchesResponse
				if err := json.Unmarshal([]byte(result), &listResponse); err != nil {
					t.Fatalf("error unmarshalling result: %s", err)
				}
				actual = append(actual, listResponse.Batches...)
				pageToken = listResponse.NextPageToken
			}

			if !reflect.DeepEqual(actual, tc.want) {
				t.Fatalf("unexpected batches: got %+v, want %+v", actual, tc.want)
			}

			// want has URLs because it's created from Batch instances by the same utility function
			// used by the tool internals. Double-check that the URLs are reasonable.
			for _, batch := range tc.want {
				if !strings.HasPrefix(batch.ConsoleURL, batchURLPrefix) {
					t.Errorf("unexpected consoleUrl in batch: %#v", batch)
				}
				if !strings.HasPrefix(batch.LogsURL, logsURLPrefix) {
					t.Errorf("unexpected logsUrl in batch: %#v", batch)
				}
			}
		})
	}
}

func listBatchesRpc(t *testing.T, client *dataproc.BatchControllerClient, ctx context.Context, filter string, n int, exact bool) []serverlessspark.Batch {
	parent := fmt.Sprintf("projects/%s/locations/%s", serverlessSparkProject, serverlessSparkLocation)
	req := &dataprocpb.ListBatchesRequest{
		Parent:   parent,
		PageSize: 2,
		OrderBy:  "create_time desc",
	}
	if filter != "" {
		req.Filter = filter
	}

	it := client.ListBatches(ctx, req)
	pager := iterator.NewPager(it, n, "")
	var batchPbs []*dataprocpb.Batch
	_, err := pager.NextPage(&batchPbs)
	if err != nil {
		t.Fatalf("failed to list batches: %s", err)
	}
	if exact && len(batchPbs) != n {
		t.Fatalf("expected exactly %d batches, got %d", n, len(batchPbs))
	}
	if !exact && (len(batchPbs) == 0 || len(batchPbs) > n) {
		t.Fatalf("expected between 1 and %d batches, got %d", n, len(batchPbs))
	}
	batches, err := serverlessspark.ToBatches(batchPbs)
	if err != nil {
		t.Fatalf("failed to convert batches to JSON: %v", err)
	}

	return batches
}

func runAuthTest(t *testing.T, toolName string, request map[string]any, wantStatus int) {
	idToken, err := tests.GetGoogleIdToken(t)
	if err != nil {
		t.Fatalf("error getting Google ID token: %s", err)
	}
	tcs := []struct {
		name       string
		headers    map[string]string
		wantStatus int
	}{
		{
			name:       "valid auth token",
			headers:    map[string]string{"my-google-auth_token": idToken},
			wantStatus: wantStatus,
		},
		{
			name:       "invalid auth token",
			headers:    map[string]string{"my-google-auth_token": "INVALID_TOKEN"},
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "no auth token",
			headers:    nil,
			wantStatus: http.StatusUnauthorized,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			status, response, err := tests.InvokeMCPTool(t, toolName, request, tc.headers)
			if status != tc.wantStatus {
				t.Fatalf("tools/call status = %d, want %d: %v", status, tc.wantStatus, err)
			}
			if status == http.StatusOK {
				if err != nil || response == nil || response.Error != nil {
					t.Fatalf("authenticated tools/call failed: response=%+v, error=%v", response, err)
				}
				// A nonexistent cancellation operation still verifies valid auth, but must
				// report the expected tool error rather than silently passing any error.
				if toolName == "cancel-batch-with-auth" {
					if !response.Result.IsError {
						t.Fatal("expected cancellation error")
					}
					tests.AssertMCPError(t, response, "Operation not found")
				} else if response.Result.IsError {
					t.Fatalf("authenticated tool returned an error: %+v", response.Result)
				}
			}
		})
	}
}

func runGetBatchTest(t *testing.T, client *dataproc.BatchControllerClient, ctx context.Context, fullName string) {
	// First get the batch details directly from the Go proto API.
	req := &dataprocpb.GetBatchRequest{
		Name: fullName,
	}
	rawWantBatchPb, err := client.GetBatch(ctx, req)
	if err != nil {
		t.Fatalf("failed to get batch: %s", err)
	}

	// Trim unknown fields from the proto by marshalling and unmarshalling.
	jsonBytes, err := protojson.Marshal(rawWantBatchPb)
	if err != nil {
		t.Fatalf("failed to marshal batch to JSON: %s", err)
	}
	var wantBatchPb dataprocpb.Batch
	if err := protojson.Unmarshal(jsonBytes, &wantBatchPb); err != nil {
		t.Fatalf("error unmarshalling result: %s", err)
	}

	tcs := []struct {
		name      string
		batchName string
		want      *dataprocpb.Batch
	}{
		{
			name:      "found batch",
			batchName: shortName(fullName),
			want:      &wantBatchPb,
		},
	}

	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			request := map[string]any{"name": tc.batchName}
			result := invokeMCPResult(t, "get-batch", request, nil)
			var wrappedResult map[string]any
			if err := json.Unmarshal([]byte(result), &wrappedResult); err != nil {
				t.Fatalf("error unmarshalling result: %s", err)
			}
			consoleURL, ok := wrappedResult["consoleUrl"].(string)
			if !ok || !strings.HasPrefix(consoleURL, batchURLPrefix) {
				t.Errorf("unexpected consoleUrl: %v", consoleURL)
			}
			logsURL, ok := wrappedResult["logsUrl"].(string)
			if !ok || !strings.HasPrefix(logsURL, logsURLPrefix) {
				t.Errorf("unexpected logsUrl: %v", logsURL)
			}
			batchJSON, err := json.Marshal(wrappedResult["batch"])
			if err != nil {
				t.Fatalf("failed to marshal batch: %v", err)
			}

			// Unmarshal JSON to proto for proto-aware deep comparison.
			var batch dataprocpb.Batch
			if err := protojson.Unmarshal(batchJSON, &batch); err != nil {
				t.Fatalf("error unmarshalling batch from wrapped result: %s", err)
			}

			ignoreFields := protocmp.IgnoreFields(&dataprocpb.RuntimeInfo{}, "approximate_usage")
			if !cmp.Equal(&batch, tc.want, protocmp.Transform(), ignoreFields) {
				diff := cmp.Diff(&batch, tc.want, protocmp.Transform(), ignoreFields)
				t.Errorf("GetBatch() returned diff (-got +want):\n%s", diff)
			}
		})
	}
}

func javaReq(req map[string]any) map[string]any {
	merged := map[string]any{
		"mainClass": "org.apache.spark.examples.SparkPi",
		"jarFiles":  []string{"file:///usr/lib/spark/examples/jars/spark-examples.jar"},
	}
	maps.Copy(merged, req)
	return merged
}

func runCreateSparkBatchTest(
	t *testing.T,
	client *dataproc.BatchControllerClient,
	ctx context.Context,
	toolName string,
	request map[string]any,
	waitForSuccess bool,
	validate func(t *testing.T, b *dataprocpb.Batch),
) {
	result := invokeMCPResult(t, toolName, request, nil)

	var resultMap map[string]any
	if err := json.Unmarshal([]byte(result), &resultMap); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}
	consoleURL, ok := resultMap["consoleUrl"].(string)
	if !ok || !strings.HasPrefix(consoleURL, batchURLPrefix) {
		t.Errorf("unexpected consoleUrl: %v", consoleURL)
	}
	logsURL, ok := resultMap["logsUrl"].(string)
	if !ok || !strings.HasPrefix(logsURL, logsURLPrefix) {
		t.Errorf("unexpected logsUrl: %v", logsURL)
	}
	metaMap, ok := resultMap["opMetadata"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected opMetadata: %v", metaMap)
	}
	metaJson, err := json.Marshal(metaMap)
	if err != nil {
		t.Fatalf("failed to marshal op metadata to JSON: %s", err)
	}
	var meta dataprocpb.BatchOperationMetadata
	if err := json.Unmarshal([]byte(metaJson), &meta); err != nil {
		t.Fatalf("failed to unmarshal result: %v", err)
	}

	if validate != nil {
		b, err := client.GetBatch(ctx, &dataprocpb.GetBatchRequest{Name: meta.Batch})
		if err != nil {
			t.Fatalf("failed to get batch %s: %s", meta.Batch, err)
		}
		validate(t, b)
	}

	if waitForSuccess {
		waitForBatch(t, client, ctx, meta.Batch, []dataprocpb.Batch_State{dataprocpb.Batch_SUCCEEDED}, 5*time.Minute)
	}
}

func testError(t *testing.T, toolName string, request map[string]any, wantCode int, wantMsg string) {
	t.Helper()
	status, response, err := tests.InvokeMCPTool(t, toolName, request, nil)
	if err != nil || status != wantCode {
		t.Fatalf("tools/call status = %d, want %d: %v", status, wantCode, err)
	}
	if response.Error != nil || !response.Result.IsError {
		t.Fatalf("expected a tool error, got %+v", response)
	}

	tests.AssertMCPError(t, response, wantMsg)
}

func invokeMCPResult(t *testing.T, toolName string, request map[string]any, headers map[string]string) string {
	t.Helper()
	status, response, err := tests.InvokeMCPTool(t, toolName, request, headers)
	if err != nil || status != http.StatusOK {
		t.Fatalf("tools/call status = %d: %v", status, err)
	}
	text, err := mcpResultText(response)
	if err != nil {
		t.Fatalf("tools/call result: %v", err)
	}
	return text
}

func mcpResultText(response *tests.MCPCallToolResponse) (string, error) {
	if response == nil {
		return "", fmt.Errorf("missing response")
	}
	if response.Error != nil || response.Result.IsError {
		return "", fmt.Errorf("tool returned an error: %+v", response)
	}
	content := response.Result.Content
	if len(content) != 1 || content[0].Type != "text" {
		return "", fmt.Errorf("unexpected content: %+v", content)
	}
	return content[0].Text, nil
}

func shortName(fullName string) string {
	parts := strings.Split(fullName, "/")
	return parts[len(parts)-1]
}

func runListSessionsTest(t *testing.T, client *dataproc.SessionControllerClient, ctx context.Context) {
	expected := listSessionsRpc(t, client, ctx, "", 20, false)

	request := map[string]any{
		"pageSize": 20,
	}
	result := invokeMCPResult(t, "list-sessions", request, nil)

	var listResponse serverlessspark.ListSessionsResponse
	if err := json.Unmarshal([]byte(result), &listResponse); err != nil {
		t.Fatalf("error unmarshalling result: %s", err)
	}

	if len(expected) == 0 {
		if len(listResponse.Sessions) != 0 {
			t.Errorf("expected 0 sessions, got %d", len(listResponse.Sessions))
		}
		return
	}

	if !reflect.DeepEqual(listResponse.Sessions, expected) {
		t.Fatalf("unexpected sessions: got %+v, want %+v", listResponse.Sessions, expected)
	}

	for _, session := range listResponse.Sessions {
		if !strings.Contains(session.ConsoleURL, "/dataproc/interactive/") {
			t.Errorf("unexpected consoleUrl in session: %#v", session)
		}
		if !strings.HasPrefix(session.LogsURL, logsURLPrefix) {
			t.Errorf("unexpected logsUrl in session: %#v", session)
		}
	}
}

func listSessionsRpc(t *testing.T, client *dataproc.SessionControllerClient, ctx context.Context, filter string, n int, exact bool) []serverlessspark.Session {
	parent := fmt.Sprintf("projects/%s/locations/%s", serverlessSparkProject, serverlessSparkLocation)
	req := &dataprocpb.ListSessionsRequest{
		Parent:   parent,
		PageSize: int32(n),
	}
	if filter != "" {
		req.Filter = filter
	}

	it := client.ListSessions(ctx, req)
	pager := iterator.NewPager(it, n, "")
	var sessionPbs []*dataprocpb.Session
	_, err := pager.NextPage(&sessionPbs)
	if err != nil {
		t.Fatalf("failed to list sessions: %s", err)
	}
	if exact && len(sessionPbs) != n {
		t.Fatalf("expected exactly %d sessions, got %d", n, len(sessionPbs))
	}
	if !exact && (len(sessionPbs) == 0 || len(sessionPbs) > n) {
		t.Fatalf("expected between 1 and %d sessions, got %d", n, len(sessionPbs))
	}
	sessions, err := serverlessspark.ToSessions(sessionPbs)
	if err != nil {
		t.Fatalf("failed to convert sessions to JSON: %v", err)
	}

	return sessions
}

func runGetSessionTest(t *testing.T, client *dataproc.SessionControllerClient, ctx context.Context, fullName string) {
	req := &dataprocpb.GetSessionRequest{
		Name: fullName,
	}
	rawWantSessionPb, err := client.GetSession(ctx, req)
	if err != nil {
		t.Fatalf("failed to get session: %s", err)
	}

	jsonBytes, err := protojson.Marshal(rawWantSessionPb)
	if err != nil {
		t.Fatalf("failed to marshal session to JSON: %s", err)
	}
	var wantSessionPb dataprocpb.Session
	if err := protojson.Unmarshal(jsonBytes, &wantSessionPb); err != nil {
		t.Fatalf("error unmarshalling result: %s", err)
	}

	request := map[string]any{"name": shortName(fullName)}
	result := invokeMCPResult(t, "get-session", request, nil)
	var wrappedResult map[string]any
	if err := json.Unmarshal([]byte(result), &wrappedResult); err != nil {
		t.Fatalf("error unmarshalling result: %s", err)
	}

	sessionJSON, err := json.Marshal(wrappedResult["session"])
	if err != nil {
		t.Fatalf("failed to marshal session: %v", err)
	}

	var session dataprocpb.Session
	if err := protojson.Unmarshal(sessionJSON, &session); err != nil {
		t.Fatalf("error unmarshalling session from wrapped result: %s", err)
	}

	if !cmp.Equal(&session, &wantSessionPb, protocmp.Transform()) {
		diff := cmp.Diff(&session, &wantSessionPb, protocmp.Transform())
		t.Errorf("GetSession() returned diff (-got +want):\n%s", diff)
	}
}

func listSessionTemplatesRpc(t *testing.T, client *dataproc.SessionTemplateControllerClient, ctx context.Context, filter string, n int, exact bool) []serverlessspark.SessionTemplate {
	parent := fmt.Sprintf("projects/%s/locations/%s", serverlessSparkProject, serverlessSparkLocation)
	req := &dataprocpb.ListSessionTemplatesRequest{
		Parent:   parent,
		PageSize: int32(n),
	}
	if filter != "" {
		req.Filter = filter
	}

	it := client.ListSessionTemplates(ctx, req)
	pager := iterator.NewPager(it, n, "")
	var sessionTemplatePbs []*dataprocpb.SessionTemplate
	_, err := pager.NextPage(&sessionTemplatePbs)
	if err != nil {
		t.Fatalf("failed to list session templates: %s", err)
	}
	if exact && len(sessionTemplatePbs) != n {
		t.Fatalf("expected exactly %d templates, got %d", n, len(sessionTemplatePbs))
	}
	if !exact && (len(sessionTemplatePbs) == 0 || len(sessionTemplatePbs) > n) {
		t.Fatalf("expected between 1 and %d session templates, got %d", n, len(sessionTemplatePbs))
	}
	sessionTemplates, err := serverlessspark.ToSessionTemplates(sessionTemplatePbs)
	if err != nil {
		t.Fatalf("failed to convert session templates to JSON: %v", err)
	}

	return sessionTemplates
}

func runGetSessionTemplateTest(t *testing.T, client *dataproc.SessionTemplateControllerClient, ctx context.Context, fullName string) {
	req := &dataprocpb.GetSessionTemplateRequest{
		Name: fullName,
	}
	rawWantSessionTemplatePb, err := client.GetSessionTemplate(ctx, req)
	if err != nil {
		t.Fatalf("failed to get session template: %s", err)
	}

	jsonBytes, err := protojson.Marshal(rawWantSessionTemplatePb)
	if err != nil {
		t.Fatalf("failed to marshal session template to JSON: %s", err)
	}
	var wantSessionTemplatePb dataprocpb.SessionTemplate
	if err := protojson.Unmarshal(jsonBytes, &wantSessionTemplatePb); err != nil {
		t.Fatalf("error unmarshalling result: %s", err)
	}

	request := map[string]any{"name": shortName(fullName)}
	result := invokeMCPResult(t, "get-session-template", request, nil)
	var wrappedResult map[string]any
	if err := json.Unmarshal([]byte(result), &wrappedResult); err != nil {
		t.Fatalf("error unmarshalling result: %s", err)
	}

	sessionTemplateJSON, err := json.Marshal(wrappedResult["sessionTemplate"])
	if err != nil {
		t.Fatalf("failed to marshal session template: %v", err)
	}

	var sessionTemplate dataprocpb.SessionTemplate
	if err := protojson.Unmarshal(sessionTemplateJSON, &sessionTemplate); err != nil {
		t.Fatalf("error unmarshalling session template from wrapped result: %s", err)
	}

	if !cmp.Equal(&sessionTemplate, &wantSessionTemplatePb, protocmp.Transform()) {
		diff := cmp.Diff(&sessionTemplate, &wantSessionTemplatePb, protocmp.Transform())
		t.Errorf("GetSessionTemplate() returned diff (-got +want):\n%s", diff)
	}
}
