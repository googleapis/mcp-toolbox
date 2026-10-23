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

package cloudstorage

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"google.golang.org/api/iterator"
)

var (
	CloudStorageSourceType = "cloud-storage"
	CloudStorageProject    = os.Getenv("CLOUD_STORAGE_PROJECT")
)

const (
	helloObject    = "seed/hello.txt"
	jsonObject     = "seed/nested/data.json"
	largeObject    = "seed/large.bin"
	binaryObject   = "seed/binary.bin"
	downloadObject = "seed/download.txt"
	helloBody      = "hello world"
	jsonBody       = `{"foo":"bar"}`
	downloadBody   = "download-me"
	// largeObjectSize is > the 8 MiB read cap so we can assert the size-limit
	// agent-error path on the read_object tool.
	largeObjectSize = (8 << 20) + 1024
)

// cloudStorageCleanupTimeout bounds bucket cleanup, which runs on a context
// detached from the (possibly cancelled) test context.
const cloudStorageCleanupTimeout = 2 * time.Minute

func getCloudStorageVars(t *testing.T) map[string]any {
	if CloudStorageProject == "" {
		t.Fatal("'CLOUD_STORAGE_PROJECT' not set")
	}
	return map[string]any{
		"type":    CloudStorageSourceType,
		"project": CloudStorageProject,
	}
}

// cloudStorageTestEnv holds the resources shared by a test run.
type cloudStorageTestEnv struct {
	client                *storage.Client
	bucket                string
	configuredDownloadDir string
	toolsFile             map[string]any
}

// setupCloudStorageTest creates the client and the seeded test bucket and
// returns them with the tools file. The bucket is deleted and the client
// closed on cleanup.
func setupCloudStorageTest(t *testing.T, ctx context.Context) cloudStorageTestEnv {
	t.Helper()
	sourceConfig := getCloudStorageVars(t)

	client, err := storage.NewClient(ctx)
	if err != nil {
		t.Fatalf("unable to create Cloud Storage client: %s", err)
	}
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Logf("failed to close Cloud Storage client: %v", err)
		}
	})

	// Bucket names must be globally unique and match [a-z0-9_.-]{3,63}.
	bucketName := "toolbox-it-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:20]
	t.Logf("Using test bucket %q", bucketName)

	setupCloudStorageTestData(t, ctx, client, CloudStorageProject, bucketName)

	configuredDownloadDir := t.TempDir()
	return cloudStorageTestEnv{
		client:                client,
		bucket:                bucketName,
		configuredDownloadDir: configuredDownloadDir,
		toolsFile:             getCloudStorageToolsConfig(sourceConfig, bucketName, configuredDownloadDir),
	}
}

type cloudStorageTransport struct {
	isMCP bool
}

func (tr cloudStorageTransport) startServer(t *testing.T, ctx context.Context, toolsFile map[string]any) {
	t.Helper()
	var args []string
	if !tr.isMCP {
		args = append(args, "--enable-api")
	}
	cmd, cleanup, err := tests.StartCmd(ctx, toolsFile, args...)
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)
	t.Cleanup(cmd.Close)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}
}

// invokeTool calls a tool and returns its result text and the HTTP status.
// Over REST this is the `result` string, which carries agent errors as an
// `{"error": ...}` payload; on non-200 responses the full body is returned
// instead. Over MCP it is the text of the result (an isError result carries
// the error message), or the message of a JSON-RPC error.
func (tr cloudStorageTransport) invokeTool(t *testing.T, ctx context.Context, toolName string, args map[string]any) (string, int) {
	t.Helper()
	url := fmt.Sprintf("http://127.0.0.1:5000/api/tool/%s/invoke", toolName)
	var reqBody any = args
	headers := map[string]string{"Content-Type": "application/json"}
	if tr.isMCP {
		url = "http://127.0.0.1:5000/mcp"
		reqBody = tests.NewMCPCallToolRequest(uuid.New().String(), toolName, args)
		headers = tests.NewMCPRequestHeader(t, nil)
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		t.Fatalf("unable to marshal request body: %s", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(reqBytes))
	if err != nil {
		t.Fatalf("unable to create request: %s", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("unable to send request: %s", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("unable to read response body: %s", err)
	}
	if resp.StatusCode != http.StatusOK {
		return string(bodyBytes), resp.StatusCode
	}

	if !tr.isMCP {
		var body map[string]any
		if err := json.Unmarshal(bodyBytes, &body); err != nil {
			t.Fatalf("failed to parse response JSON: %s (body=%s)", err, string(bodyBytes))
		}
		result, _ := body["result"].(string)
		return result, resp.StatusCode
	}

	var mcpResp tests.MCPCallToolResponse
	if err := json.Unmarshal(bodyBytes, &mcpResp); err != nil {
		t.Fatalf("failed to parse MCP response JSON: %s (body=%s)", err, string(bodyBytes))
	}
	if mcpResp.Error != nil {
		return mcpResp.Error.Message, resp.StatusCode
	}
	texts := make([]string, 0, len(mcpResp.Result.Content))
	for _, content := range mcpResp.Result.Content {
		texts = append(texts, content.Text)
	}
	if mcpResp.Result.IsError || len(texts) == 1 {
		return strings.Join(texts, ""), resp.StatusCode
	}
	// Results split into one content block per element are joined back
	// into a JSON array so they match the REST result.
	return "[" + strings.Join(texts, ",") + "]", resp.StatusCode
}

func TestCloudStorageToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env := setupCloudStorageTest(t, ctx)

	tr := cloudStorageTransport{}
	tr.startServer(t, ctx, env.toolsFile)

	tests.RunToolGetTestByName(t, "my_list_objects",
		map[string]any{
			"my_list_objects": map[string]any{
				"description":  "List objects in a Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket to list objects from.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Filter results to objects whose names begin with this prefix.",
						"name":         "prefix",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Delimiter used to group object names (typically '/'). When set, common prefixes are returned as 'prefixes'.",
						"name":         "delimiter",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Maximum number of objects to return per page. A value of 0 uses the API default (1000); negative values and values above 1000 are rejected.",
						"name":         "max_results",
						"required":     false,
						"type":         "integer",
						"default":      float64(0),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "A previously-returned page token for retrieving the next page of results.",
						"name":         "page_token",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_read_object",
		map[string]any{
			"my_read_object": map[string]any{
				"description":  "Read a Cloud Storage object.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket containing the object.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Optional HTTP byte range, e.g. 'bytes=0-999' (first 1000 bytes), 'bytes=-500' (last 500 bytes), or 'bytes=500-' (from byte 500 to end). Empty reads the full object.",
						"name":         "range",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)

	tests.RunToolGetTestByName(t, "my_list_buckets",
		map[string]any{
			"my_list_buckets": map[string]any{
				"description":  "List Cloud Storage buckets in the project.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Project ID to list buckets in. When empty, the source's configured project is used.",
						"name":         "project",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Filter results to buckets whose names begin with this prefix.",
						"name":         "prefix",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Maximum number of buckets to return per page. A value of 0 uses the API default (1000); negative values and values above 1000 are rejected.",
						"name":         "max_results",
						"required":     false,
						"type":         "integer",
						"default":      float64(0),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "A previously-returned page token for retrieving the next page of results.",
						"name":         "page_token",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_create_bucket",
		map[string]any{
			"my_create_bucket": map[string]any{
				"description":  "Create a Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket to create.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Project ID to create the bucket in. When empty, the source's configured project is used.",
						"name":         "project",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Location for the bucket, e.g. 'US', 'EU', or 'us-central1'. Omit to use the Cloud Storage service default.",
						"name":         "location",
						"required":     false,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Whether to enable uniform bucket-level access on the bucket.",
						"name":         "uniform_bucket_level_access",
						"required":     false,
						"type":         "boolean",
						"default":      false,
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_get_bucket_metadata",
		map[string]any{
			"my_get_bucket_metadata": map[string]any{
				"description":  "Get metadata for a Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket to inspect.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_get_bucket_iam_policy",
		map[string]any{
			"my_get_bucket_iam_policy": map[string]any{
				"description":  "Get the IAM policy for a Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket whose IAM policy should be returned.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_delete_bucket",
		map[string]any{
			"my_delete_bucket": map[string]any{
				"description":  "Delete an empty Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the empty Cloud Storage bucket to delete.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_get_object_metadata",
		map[string]any{
			"my_get_object_metadata": map[string]any{
				"description":  "Get metadata for a Cloud Storage object.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket containing the object.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_download_object",
		map[string]any{
			"my_download_object": map[string]any{
				"description":  "Download a Cloud Storage object to a local file.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket containing the object.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Absolute local filesystem path where the object will be written. Relative paths and paths containing '..' are rejected.",
						"name":         "destination",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "If true, overwrite the destination when it already exists. If false (default), the tool returns an error when the destination exists.",
						"name":         "overwrite",
						"required":     false,
						"type":         "boolean",
						"default":      false,
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_upload_object",
		map[string]any{
			"my_upload_object": map[string]any{
				"description":  "Upload a local file to a Cloud Storage object.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket to upload into.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Absolute local filesystem path of the file to upload. Relative paths and paths containing '..' are rejected.",
						"name":         "source",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "MIME type to record on the uploaded object. When empty, it is inferred from the source file's extension; if that fails, Cloud Storage auto-detects from the first 512 bytes of content.",
						"name":         "content_type",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_write_object",
		map[string]any{
			"my_write_object": map[string]any{
				"description":  "Write text content to a Cloud Storage object.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket to write into.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Text content to write to the Cloud Storage object.",
						"name":         "content",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "MIME type to record on the written object. When empty, Cloud Storage auto-detects from the first 512 bytes of content.",
						"name":         "content_type",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_copy_object",
		map[string]any{
			"my_copy_object": map[string]any{
				"description":  "Copy a Cloud Storage object.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket containing the source object.",
						"name":         "source_bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full source object name (path) within the source bucket, e.g. 'path/to/file.txt'.",
						"name":         "source_object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket to copy into.",
						"name":         "destination_bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full destination object name (path) within the destination bucket, e.g. 'path/to/file.txt'.",
						"name":         "destination_object",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_move_object",
		map[string]any{
			"my_move_object": map[string]any{
				"description":  "Move a Cloud Storage object within the same bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket containing the object to move.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full source object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "source_object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full destination object name (path) within the same bucket, e.g. 'path/to/file.txt'.",
						"name":         "destination_object",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_delete_object",
		map[string]any{
			"my_delete_object": map[string]any{
				"description":  "Delete a Cloud Storage object.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket containing the object to delete.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)

	// Tools with parameters baked into their config drop those parameters from
	// the schema exposed to the LLM. The assertions below confirm each
	// configured parameter is absent from the manifest.
	tests.RunToolGetTestByName(t, "my_list_objects_configured",
		map[string]any{
			"my_list_objects_configured": map[string]any{
				"description":  "List objects in a configured Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Maximum number of objects to return per page. A value of 0 uses the API default (1000); negative values and values above 1000 are rejected.",
						"name":         "max_results",
						"required":     false,
						"type":         "integer",
						"default":      float64(0),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "A previously-returned page token for retrieving the next page of results.",
						"name":         "page_token",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_list_buckets_configured",
		map[string]any{
			"my_list_buckets_configured": map[string]any{
				"description":  "List Cloud Storage buckets in a configured project.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Maximum number of buckets to return per page. A value of 0 uses the API default (1000); negative values and values above 1000 are rejected.",
						"name":         "max_results",
						"required":     false,
						"type":         "integer",
						"default":      float64(0),
					},
					map[string]any{
						"authServices": []any{},
						"description":  "A previously-returned page token for retrieving the next page of results.",
						"name":         "page_token",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_read_object_configured",
		map[string]any{
			"my_read_object_configured": map[string]any{
				"description":  "Read an object from a configured Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Optional HTTP byte range, e.g. 'bytes=0-999' (first 1000 bytes), 'bytes=-500' (last 500 bytes), or 'bytes=500-' (from byte 500 to end). Empty reads the full object.",
						"name":         "range",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_get_object_metadata_configured",
		map[string]any{
			"my_get_object_metadata_configured": map[string]any{
				"description":  "Get object metadata from a configured Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_get_bucket_metadata_configured",
		map[string]any{
			"my_get_bucket_metadata_configured": map[string]any{
				"description":  "Get metadata for a configured Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_get_bucket_iam_policy_configured",
		map[string]any{
			"my_get_bucket_iam_policy_configured": map[string]any{
				"description":  "Get the IAM policy for a configured Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters":   []any{},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_create_bucket_configured",
		map[string]any{
			"my_create_bucket_configured": map[string]any{
				"description":  "Create a Cloud Storage bucket with configured settings.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Name of the Cloud Storage bucket to create.",
						"name":         "bucket",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_download_object_configured",
		map[string]any{
			"my_download_object_configured": map[string]any{
				"description":  "Download a Cloud Storage object with configured storage settings.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Relative path under the configured destination_dir where the object will be written. Absolute paths and paths that escape destination_dir are rejected.",
						"name":         "destination",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_upload_object_configured",
		map[string]any{
			"my_upload_object_configured": map[string]any{
				"description":  "Upload a local file to a configured Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Absolute local filesystem path of the file to upload. Relative paths and paths containing '..' are rejected.",
						"name":         "source",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "MIME type to record on the uploaded object. When empty, it is inferred from the source file's extension; if that fails, Cloud Storage auto-detects from the first 512 bytes of content.",
						"name":         "content_type",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_write_object_configured",
		map[string]any{
			"my_write_object_configured": map[string]any{
				"description":  "Write text content to a configured Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Text content to write to the Cloud Storage object.",
						"name":         "content",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "MIME type to record on the written object. When empty, Cloud Storage auto-detects from the first 512 bytes of content.",
						"name":         "content_type",
						"required":     false,
						"type":         "string",
						"default":      "",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_copy_object_configured",
		map[string]any{
			"my_copy_object_configured": map[string]any{
				"description":  "Copy a Cloud Storage object between configured buckets.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Full source object name (path) within the source bucket, e.g. 'path/to/file.txt'.",
						"name":         "source_object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full destination object name (path) within the destination bucket, e.g. 'path/to/file.txt'.",
						"name":         "destination_object",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_move_object_configured",
		map[string]any{
			"my_move_object_configured": map[string]any{
				"description":  "Move a Cloud Storage object within a configured bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Full source object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "source_object",
						"required":     true,
						"type":         "string",
					},
					map[string]any{
						"authServices": []any{},
						"description":  "Full destination object name (path) within the same bucket, e.g. 'path/to/file.txt'.",
						"name":         "destination_object",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)
	tests.RunToolGetTestByName(t, "my_delete_object_configured",
		map[string]any{
			"my_delete_object_configured": map[string]any{
				"description":  "Delete an object from a configured Cloud Storage bucket.",
				"authRequired": []any{},
				"parameters": []any{
					map[string]any{
						"authServices": []any{},
						"description":  "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
						"name":         "object",
						"required":     true,
						"type":         "string",
					},
				},
			},
		},
	)

	runCloudStorageTests(ctx, t, tr, env)
}

// runCloudStorageTests runs the Cloud Storage tool invocation checks.
func runCloudStorageTests(ctx context.Context, t *testing.T, tr cloudStorageTransport, env cloudStorageTestEnv) {
	client, bucketName := env.client, env.bucket
	runCloudStorageListObjectsTest(ctx, t, tr, bucketName)
	runCloudStorageReadObjectTest(ctx, t, tr, bucketName)
	runCloudStorageListBucketsTest(ctx, t, tr, bucketName)
	runCloudStorageGetObjectMetadataTest(ctx, t, tr, bucketName)
	runCloudStorageConfiguredParamsTest(ctx, t, tr, client, bucketName, env.configuredDownloadDir)
	bucketToolName := "toolbox-it-bucket-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:17]
	t.Cleanup(func() { cleanupGCSBucket(ctx, t, client, bucketToolName) })
	runCloudStorageCreateBucketTest(ctx, t, tr, client, bucketToolName)
	runCloudStorageGetBucketMetadataTest(ctx, t, tr, bucketToolName)
	runCloudStorageGetBucketIAMPolicyTest(ctx, t, tr, bucketToolName)
	runCloudStorageDeleteBucketTest(ctx, t, tr, client, bucketToolName)
	runCloudStorageDownloadObjectTest(ctx, t, tr, bucketName)
	runCloudStorageUploadObjectTest(ctx, t, tr, client, bucketName)
	runCloudStorageWriteObjectTest(ctx, t, tr, client, bucketName)
	runCloudStorageCopyObjectTest(ctx, t, tr, client, bucketName)
	runCloudStorageMoveObjectTest(ctx, t, tr, client, bucketName)
	runCloudStorageDeleteObjectTest(ctx, t, tr, client, bucketName)
}

func getCloudStorageToolsConfig(sourceConfig map[string]any, bucketName, configuredDownloadDir string) map[string]any {
	return map[string]any{
		"sources": map[string]any{
			"my_instance": sourceConfig,
		},
		"tools": map[string]any{
			"my_list_objects": map[string]any{
				"type":        "cloud-storage-list-objects",
				"source":      "my_instance",
				"description": "List objects in a Cloud Storage bucket.",
			},
			"my_read_object": map[string]any{
				"type":        "cloud-storage-read-object",
				"source":      "my_instance",
				"description": "Read a Cloud Storage object.",
			},
			"my_list_buckets": map[string]any{
				"type":        "cloud-storage-list-buckets",
				"source":      "my_instance",
				"description": "List Cloud Storage buckets in the project.",
			},
			"my_create_bucket": map[string]any{
				"type":        "cloud-storage-create-bucket",
				"source":      "my_instance",
				"description": "Create a Cloud Storage bucket.",
			},
			"my_get_bucket_metadata": map[string]any{
				"type":        "cloud-storage-get-bucket-metadata",
				"source":      "my_instance",
				"description": "Get metadata for a Cloud Storage bucket.",
			},
			"my_get_bucket_iam_policy": map[string]any{
				"type":        "cloud-storage-get-bucket-iam-policy",
				"source":      "my_instance",
				"description": "Get the IAM policy for a Cloud Storage bucket.",
			},
			"my_delete_bucket": map[string]any{
				"type":        "cloud-storage-delete-bucket",
				"source":      "my_instance",
				"description": "Delete an empty Cloud Storage bucket.",
			},
			"my_get_object_metadata": map[string]any{
				"type":        "cloud-storage-get-object-metadata",
				"source":      "my_instance",
				"description": "Get metadata for a Cloud Storage object.",
			},
			"my_download_object": map[string]any{
				"type":        "cloud-storage-download-object",
				"source":      "my_instance",
				"description": "Download a Cloud Storage object to a local file.",
			},
			"my_upload_object": map[string]any{
				"type":        "cloud-storage-upload-object",
				"source":      "my_instance",
				"description": "Upload a local file to a Cloud Storage object.",
			},
			"my_write_object": map[string]any{
				"type":        "cloud-storage-write-object",
				"source":      "my_instance",
				"description": "Write text content to a Cloud Storage object.",
			},
			"my_copy_object": map[string]any{
				"type":        "cloud-storage-copy-object",
				"source":      "my_instance",
				"description": "Copy a Cloud Storage object.",
			},
			"my_move_object": map[string]any{
				"type":        "cloud-storage-move-object",
				"source":      "my_instance",
				"description": "Move a Cloud Storage object within the same bucket.",
			},
			"my_delete_object": map[string]any{
				"type":        "cloud-storage-delete-object",
				"source":      "my_instance",
				"description": "Delete a Cloud Storage object.",
			},
			// Configured variants bake static parameter values into the tool
			// config so those parameters are removed from the schema and the
			// configured values are used at invocation time.
			"my_list_objects_configured": map[string]any{
				"type":        "cloud-storage-list-objects",
				"source":      "my_instance",
				"description": "List objects in a configured Cloud Storage bucket.",
				"bucket":      bucketName,
				"prefix":      "seed/",
				"delimiter":   "",
			},
			"my_list_buckets_configured": map[string]any{
				"type":        "cloud-storage-list-buckets",
				"source":      "my_instance",
				"description": "List Cloud Storage buckets in a configured project.",
				"project":     CloudStorageProject,
				"prefix":      "toolbox-it-",
			},
			"my_read_object_configured": map[string]any{
				"type":        "cloud-storage-read-object",
				"source":      "my_instance",
				"description": "Read an object from a configured Cloud Storage bucket.",
				"bucket":      bucketName,
			},
			"my_get_object_metadata_configured": map[string]any{
				"type":        "cloud-storage-get-object-metadata",
				"source":      "my_instance",
				"description": "Get object metadata from a configured Cloud Storage bucket.",
				"bucket":      bucketName,
			},
			"my_get_bucket_metadata_configured": map[string]any{
				"type":        "cloud-storage-get-bucket-metadata",
				"source":      "my_instance",
				"description": "Get metadata for a configured Cloud Storage bucket.",
				"bucket":      bucketName,
			},
			"my_get_bucket_iam_policy_configured": map[string]any{
				"type":        "cloud-storage-get-bucket-iam-policy",
				"source":      "my_instance",
				"description": "Get the IAM policy for a configured Cloud Storage bucket.",
				"bucket":      bucketName,
			},
			"my_create_bucket_configured": map[string]any{
				"type":                        "cloud-storage-create-bucket",
				"source":                      "my_instance",
				"description":                 "Create a Cloud Storage bucket with configured settings.",
				"project":                     CloudStorageProject,
				"location":                    "US",
				"uniform_bucket_level_access": true,
			},
			"my_download_object_configured": map[string]any{
				"type":            "cloud-storage-download-object",
				"source":          "my_instance",
				"description":     "Download a Cloud Storage object with configured storage settings.",
				"bucket":          bucketName,
				"destination_dir": configuredDownloadDir,
				"overwrite":       true,
			},
			"my_upload_object_configured": map[string]any{
				"type":        "cloud-storage-upload-object",
				"source":      "my_instance",
				"description": "Upload a local file to a configured Cloud Storage bucket.",
				"bucket":      bucketName,
			},
			"my_write_object_configured": map[string]any{
				"type":        "cloud-storage-write-object",
				"source":      "my_instance",
				"description": "Write text content to a configured Cloud Storage bucket.",
				"bucket":      bucketName,
			},
			"my_copy_object_configured": map[string]any{
				"type":               "cloud-storage-copy-object",
				"source":             "my_instance",
				"description":        "Copy a Cloud Storage object between configured buckets.",
				"source_bucket":      bucketName,
				"destination_bucket": bucketName,
			},
			"my_move_object_configured": map[string]any{
				"type":        "cloud-storage-move-object",
				"source":      "my_instance",
				"description": "Move a Cloud Storage object within a configured bucket.",
				"bucket":      bucketName,
			},
			"my_delete_object_configured": map[string]any{
				"type":        "cloud-storage-delete-object",
				"source":      "my_instance",
				"description": "Delete an object from a configured Cloud Storage bucket.",
				"bucket":      bucketName,
			},
		},
	}
}

// setupCloudStorageTestData creates the test bucket and seeds its objects. The
// cleanup that empties and deletes the bucket is registered first, so a
// partially seeded bucket is still removed.
func setupCloudStorageTestData(t *testing.T, ctx context.Context, client *storage.Client, project, bucket string) {
	t.Helper()
	t.Cleanup(func() { cleanupGCSBucket(ctx, t, client, bucket) })

	if err := client.Bucket(bucket).Create(ctx, project, &storage.BucketAttrs{Location: "US"}); err != nil {
		t.Fatalf("failed to create bucket %q: %v", bucket, err)
	}

	seeds := []struct {
		name, contentType, body string
	}{
		{helloObject, "text/plain", helloBody},
		{jsonObject, "application/json", jsonBody},
		{downloadObject, "text/plain", downloadBody},
		// An oversize object to exercise the read-size cap.
		{largeObject, "application/octet-stream", strings.Repeat("A", largeObjectSize)},
		// A small binary (non-UTF-8) object to exercise the ErrBinaryContent
		// path on read_object.
		{binaryObject, "application/octet-stream", string([]byte{0xff, 0xfe, 0xfd, 0xfc})},
	}
	for _, seed := range seeds {
		writeGCSObject(t, ctx, client, bucket, seed.name, seed.contentType, seed.body)
	}
}

func runCloudStorageListObjectsTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, bucket string) {
	fakeBucket := "toolbox-it-does-not-exist-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "list with prefix",
			args:           map[string]any{"bucket": bucket, "prefix": "seed/"},
			wantSubstrings: []string{helloObject, jsonObject},
		},
		{
			name:           "empty prefix and delimiter lists all objects",
			args:           map[string]any{"bucket": bucket, "prefix": "", "delimiter": ""},
			wantSubstrings: []string{helloObject, jsonObject},
		},
		{
			name:           "empty page_token behaves as first page",
			args:           map[string]any{"bucket": bucket, "prefix": "seed/", "page_token": ""},
			wantSubstrings: []string{helloObject, jsonObject},
		},
		{
			name:           "list with delimiter returns prefixes",
			args:           map[string]any{"bucket": bucket, "prefix": "seed/", "delimiter": "/"},
			wantSubstrings: []string{helloObject, `"seed/nested/"`},
		},
		{
			name:           "missing bucket parameter returns agent error",
			args:           map[string]any{},
			wantSubstrings: []string{"bucket"},
		},
		{
			name:           "max_results above 1000 returns agent error",
			args:           map[string]any{"bucket": bucket, "max_results": 1001},
			wantSubstrings: []string{"max_results", "1000"},
		},
		{
			name:           "negative max_results returns agent error",
			args:           map[string]any{"bucket": bucket, "max_results": -1},
			wantSubstrings: []string{"max_results", "must be"},
		},
		{
			name:           "nonexistent bucket returns error",
			args:           map[string]any{"bucket": fakeBucket},
			wantSubstrings: []string{fakeBucket},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_list_objects", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}

	// Pagination is inherently two-step (fetch page one, reuse its token for
	// page two), so it doesn't fit the single-request table above.
	t.Run("pagination via max_results and page_token", func(t *testing.T) {
		result, status := tr.invokeTool(t, ctx, "my_list_objects",
			map[string]any{"bucket": bucket, "prefix": "seed/", "max_results": 1})
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		token := extractStringField(t, result, "nextPageToken")
		if token == "" {
			t.Fatalf("expected non-empty nextPageToken, got %s", result)
		}

		result2, status := tr.invokeTool(t, ctx, "my_list_objects",
			map[string]any{"bucket": bucket, "prefix": "seed/", "page_token": token})
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result2)
		}
		combined := result + result2
		if !strings.Contains(combined, helloObject) || !strings.Contains(combined, jsonObject) {
			t.Errorf("expected both %q and %q across paginated results, got page1=%s page2=%s",
				helloObject, jsonObject, result, result2)
		}
	})
}

func runCloudStorageReadObjectTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, bucket string) {
	tcs := []struct {
		name            string
		args            map[string]any
		wantContent     string
		wantContentType string
		wantSubstrings  []string
	}{
		{
			name:            "read full object",
			args:            map[string]any{"bucket": bucket, "object": helloObject},
			wantContent:     helloBody,
			wantContentType: "text/plain",
		},
		{
			name:        "read range bytes=0-4",
			args:        map[string]any{"bucket": bucket, "object": helloObject, "range": "bytes=0-4"},
			wantContent: "hello",
		},
		{
			name:        "read suffix range bytes=-5",
			args:        map[string]any{"bucket": bucket, "object": helloObject, "range": "bytes=-5"},
			wantContent: "world",
		},
		{
			name:        "read open-ended range bytes=6-",
			args:        map[string]any{"bucket": bucket, "object": helloObject, "range": "bytes=6-"},
			wantContent: "world",
		},
		{
			name:        "oversize read narrowed by range succeeds",
			args:        map[string]any{"bucket": bucket, "object": largeObject, "range": "bytes=0-9"},
			wantContent: "AAAAAAAAAA",
		},
		{
			name:           "missing object parameter returns agent error",
			args:           map[string]any{"bucket": bucket},
			wantSubstrings: []string{"object"},
		},
		{
			name:           "nonexistent object returns error",
			args:           map[string]any{"bucket": bucket, "object": "does/not/exist.bin"},
			wantSubstrings: []string{"does/not/exist.bin"},
		},
		{
			name:           "invalid range returns agent error",
			args:           map[string]any{"bucket": bucket, "object": helloObject, "range": "garbage"},
			wantSubstrings: []string{"range"},
		},
		{
			name:           "oversize read returns agent error",
			args:           map[string]any{"bucket": bucket, "object": largeObject},
			wantSubstrings: []string{"size limit"},
		},
		{
			name:           "binary object returns agent error",
			args:           map[string]any{"bucket": bucket, "object": binaryObject},
			wantSubstrings: []string{"UTF-8"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_read_object", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			if tc.wantContent != "" {
				if got := extractStringField(t, result, "content"); got != tc.wantContent {
					t.Errorf("expected content %q, got %q (raw %s)", tc.wantContent, got, result)
				}
			}
			if tc.wantContentType != "" {
				if got := extractStringField(t, result, "contentType"); got != tc.wantContentType {
					t.Errorf("expected contentType %q, got %q (raw %s)", tc.wantContentType, got, result)
				}
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

// extractStringField pulls a top-level string field out of a JSON-encoded result
// string (the kind the tool invoke API wraps in the `result` property).
func extractStringField(t *testing.T, result, field string) string {
	t.Helper()
	var parsed map[string]any
	if err := json.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("failed to parse tool result JSON: %s (raw=%s)", err, result)
	}
	v, _ := parsed[field].(string)
	return v
}

func runCloudStorageListBucketsTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, bucket string) {
	tcs := []struct {
		name             string
		args             map[string]any
		wantSubstrings   []string
		unwantSubstrings []string
	}{
		{
			name:           "list with matching prefix finds the test bucket",
			args:           map[string]any{"prefix": bucket[:10]},
			wantSubstrings: []string{bucket},
		},
		{
			name:             "list with non-matching prefix omits the test bucket",
			args:             map[string]any{"prefix": "toolbox-it-definitely-not-a-real-prefix-"},
			unwantSubstrings: []string{bucket},
		},
		{
			name:           "explicit project override returns the test bucket",
			args:           map[string]any{"project": CloudStorageProject, "prefix": bucket[:10]},
			wantSubstrings: []string{bucket},
		},
		{
			name:           "max_results above 1000 returns agent error",
			args:           map[string]any{"max_results": 1001},
			wantSubstrings: []string{"max_results", "1000"},
		},
		{
			name:           "negative max_results returns agent error",
			args:           map[string]any{"max_results": -1},
			wantSubstrings: []string{"max_results", "must be"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_list_buckets", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
			for _, unwant := range tc.unwantSubstrings {
				if strings.Contains(result, unwant) {
					t.Errorf("did not expect result to contain %q, got %s", unwant, result)
				}
			}
		})
	}
}

func runCloudStorageCreateBucketTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, client *storage.Client, bucket string) {
	t.Run("create bucket with omitted location", func(t *testing.T) {
		args := map[string]any{"bucket": bucket, "uniform_bucket_level_access": true}
		result, status := tr.invokeTool(t, ctx, "my_create_bucket", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		for _, want := range []string{bucket, `"created":true`, `"metadata"`} {
			if !strings.Contains(result, want) {
				t.Errorf("expected result to contain %q, got %s", want, result)
			}
		}
		attrs, err := client.Bucket(bucket).Attrs(ctx)
		if err != nil {
			t.Fatalf("expected created bucket to exist: %v", err)
		}
		if attrs.Location != "US" {
			t.Errorf("bucket location = %q, want US", attrs.Location)
		}
		if !attrs.UniformBucketLevelAccess.Enabled {
			t.Errorf("expected uniform bucket-level access to be enabled")
		}
	})

	t.Run("missing bucket returns agent error", func(t *testing.T) {
		result, status := tr.invokeTool(t, ctx, "my_create_bucket", map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if !strings.Contains(result, "bucket") {
			t.Errorf("expected result to contain bucket error, got %s", result)
		}
	})
}

func runCloudStorageGetBucketMetadataTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, bucket string) {
	fakeBucket := "toolbox-it-does-not-exist-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "metadata for created bucket",
			args:           map[string]any{"bucket": bucket},
			wantSubstrings: []string{`"Name":"` + bucket + `"`, `"Location":"US"`, `"Enabled":true`},
		},
		{
			name:           "missing bucket returns agent error",
			args:           map[string]any{},
			wantSubstrings: []string{"bucket"},
		},
		{
			name:           "nonexistent bucket returns error",
			args:           map[string]any{"bucket": fakeBucket},
			wantSubstrings: []string{fakeBucket},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_get_bucket_metadata", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

func runCloudStorageGetBucketIAMPolicyTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, bucket string) {
	fakeBucket := "toolbox-it-does-not-exist-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:12]
	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "IAM policy for created bucket",
			args:           map[string]any{"bucket": bucket},
			wantSubstrings: []string{`"bucket":"` + bucket + `"`, `"bindings"`},
		},
		{
			name:           "missing bucket returns agent error",
			args:           map[string]any{},
			wantSubstrings: []string{"bucket"},
		},
		{
			name:           "nonexistent bucket returns error",
			args:           map[string]any{"bucket": fakeBucket},
			wantSubstrings: []string{fakeBucket},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_get_bucket_iam_policy", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

func runCloudStorageDeleteBucketTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, client *storage.Client, bucket string) {
	t.Run("delete empty bucket", func(t *testing.T) {
		args := map[string]any{"bucket": bucket}
		result, status := tr.invokeTool(t, ctx, "my_delete_bucket", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if !strings.Contains(result, `"deleted":true`) {
			t.Errorf("expected deleted confirmation, got %s", result)
		}
		if gcsBucketExists(t, ctx, client, bucket) {
			t.Errorf("expected bucket %q to be deleted", bucket)
		}
	})

	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "missing bucket returns agent error",
			args:           map[string]any{},
			wantSubstrings: []string{"bucket"},
		},
		{
			name:           "missing bucket in storage returns agent-visible error",
			args:           map[string]any{"bucket": bucket},
			wantSubstrings: []string{"bucket"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_delete_bucket", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

func runCloudStorageGetObjectMetadataTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, bucket string) {
	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "metadata for hello.txt",
			args:           map[string]any{"bucket": bucket, "object": helloObject},
			wantSubstrings: []string{`"ContentType":"text/plain"`, `"Size":11`, `"Name":"seed/hello.txt"`},
		},
		{
			name:           "metadata for JSON object",
			args:           map[string]any{"bucket": bucket, "object": jsonObject},
			wantSubstrings: []string{`"ContentType":"application/json"`},
		},
		{
			name:           "missing bucket returns agent error",
			args:           map[string]any{"object": "x"},
			wantSubstrings: []string{"bucket"},
		},
		{
			name:           "missing object returns agent error",
			args:           map[string]any{"bucket": bucket},
			wantSubstrings: []string{"object"},
		},
		{
			name:           "nonexistent object returns error",
			args:           map[string]any{"bucket": bucket, "object": "does/not/exist.bin"},
			wantSubstrings: []string{"does/not/exist.bin"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_get_object_metadata", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

// runCloudStorageConfiguredParamsTest exercises the tool variants that bake
// parameter values into their config. Each invocation omits the configured
// parameters (they are absent from the schema) and relies on the configured
// values instead. The seeded `bucket` is the configured bucket for the
// bucket-scoped tools.
func runCloudStorageConfiguredParamsTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, client *storage.Client, bucket, configuredDownloadDir string) {
	t.Run("list_objects uses configured bucket and prefix", func(t *testing.T) {
		result, status := tr.invokeTool(t, ctx, "my_list_objects_configured", map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		for _, want := range []string{helloObject, jsonObject} {
			if !strings.Contains(result, want) {
				t.Errorf("expected result to contain %q, got %s", want, result)
			}
		}
	})

	t.Run("list_buckets uses configured project and prefix", func(t *testing.T) {
		result, status := tr.invokeTool(t, ctx, "my_list_buckets_configured", map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if !strings.Contains(result, bucket) {
			t.Errorf("expected result to contain configured bucket %q, got %s", bucket, result)
		}
	})

	t.Run("read_object uses configured bucket", func(t *testing.T) {
		args := map[string]any{"object": helloObject}
		result, status := tr.invokeTool(t, ctx, "my_read_object_configured", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if !strings.Contains(result, helloBody) {
			t.Errorf("expected result to contain %q, got %s", helloBody, result)
		}
	})

	t.Run("get_object_metadata uses configured bucket", func(t *testing.T) {
		args := map[string]any{"object": helloObject}
		result, status := tr.invokeTool(t, ctx, "my_get_object_metadata_configured", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if !strings.Contains(result, `"ContentType":"text/plain"`) {
			t.Errorf("expected result to contain content type, got %s", result)
		}
	})

	t.Run("get_bucket_metadata uses configured bucket", func(t *testing.T) {
		result, status := tr.invokeTool(t, ctx, "my_get_bucket_metadata_configured", map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if !strings.Contains(result, `"Name":"`+bucket+`"`) {
			t.Errorf("expected result to contain bucket name %q, got %s", bucket, result)
		}
	})

	t.Run("get_bucket_iam_policy uses configured bucket", func(t *testing.T) {
		result, status := tr.invokeTool(t, ctx, "my_get_bucket_iam_policy_configured", map[string]any{})
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		for _, want := range []string{`"bucket":"` + bucket + `"`, `"bindings"`} {
			if !strings.Contains(result, want) {
				t.Errorf("expected result to contain %q, got %s", want, result)
			}
		}
	})

	t.Run("create_bucket uses configured project, location and uniform access", func(t *testing.T) {
		newBucket := "toolbox-it-cfg-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:17]
		t.Cleanup(func() { cleanupGCSBucket(ctx, t, client, newBucket) })

		args := map[string]any{"bucket": newBucket}
		result, status := tr.invokeTool(t, ctx, "my_create_bucket_configured", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		for _, want := range []string{newBucket, `"created":true`} {
			if !strings.Contains(result, want) {
				t.Errorf("expected result to contain %q, got %s", want, result)
			}
		}
		attrs, err := client.Bucket(newBucket).Attrs(ctx)
		if err != nil {
			t.Fatalf("expected created bucket to exist: %v", err)
		}
		if attrs.Location != "US" {
			t.Errorf("bucket location = %q, want US", attrs.Location)
		}
		if !attrs.UniformBucketLevelAccess.Enabled {
			t.Errorf("expected uniform bucket-level access to be enabled")
		}
	})

	t.Run("download_object uses configured bucket destination_dir and overwrite", func(t *testing.T) {
		relDest := filepath.Join("configured", "downloaded.txt")
		dest := filepath.Join(configuredDownloadDir, relDest)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.WriteFile(dest, []byte("pre-existing"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		args := map[string]any{"object": downloadObject, "destination": relDest}
		result, status := tr.invokeTool(t, ctx, "my_download_object_configured", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("failed to read downloaded file: %v", err)
		}
		if string(got) != downloadBody {
			t.Errorf("downloaded content = %q, want %q (raw %s)", string(got), downloadBody, result)
		}
	})

	t.Run("upload_object uses configured bucket", func(t *testing.T) {
		srcPath := filepath.Join(t.TempDir(), "configured-upload.txt")
		content := "configured upload"
		if err := os.WriteFile(srcPath, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		obj := "configured/upload.txt"
		args := map[string]any{"object": obj, "source": srcPath, "content_type": "text/plain"}
		result, status := tr.invokeTool(t, ctx, "my_upload_object_configured", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		got, attrs := readGCSObject(t, ctx, client, bucket, obj)
		if got != content {
			t.Errorf("uploaded content = %q, want %q", got, content)
		}
		if attrs.ContentType != "text/plain" {
			t.Errorf("GCS ContentType = %q, want text/plain (raw %s)", attrs.ContentType, result)
		}
	})

	t.Run("write_object uses configured bucket", func(t *testing.T) {
		obj := "configured/write.txt"
		content := "configured write"
		args := map[string]any{"object": obj, "content": content, "content_type": "text/plain"}
		result, status := tr.invokeTool(t, ctx, "my_write_object_configured", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		got, attrs := readGCSObject(t, ctx, client, bucket, obj)
		if got != content {
			t.Errorf("written content = %q, want %q", got, content)
		}
		if attrs.ContentType != "text/plain" {
			t.Errorf("GCS ContentType = %q, want text/plain (raw %s)", attrs.ContentType, result)
		}
	})

	t.Run("copy_object uses configured buckets", func(t *testing.T) {
		dest := "configured/copied.txt"
		args := map[string]any{"source_object": helloObject, "destination_object": dest}
		result, status := tr.invokeTool(t, ctx, "my_copy_object_configured", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		got, _ := readGCSObject(t, ctx, client, bucket, dest)
		if got != helloBody {
			t.Errorf("copied content = %q, want %q (raw %s)", got, helloBody, result)
		}
	})

	t.Run("move_object uses configured bucket", func(t *testing.T) {
		src := "configured/move-source.txt"
		dest := "configured/move-destination.txt"
		writeGCSObject(t, ctx, client, bucket, src, "text/plain", "configured move")
		args := map[string]any{"source_object": src, "destination_object": dest}
		result, status := tr.invokeTool(t, ctx, "my_move_object_configured", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		got, _ := readGCSObject(t, ctx, client, bucket, dest)
		if got != "configured move" {
			t.Errorf("moved content = %q, want configured move (raw %s)", got, result)
		}
		if gcsObjectExists(t, ctx, client, bucket, src) {
			t.Errorf("expected source object %q to be gone after move", src)
		}
	})

	t.Run("delete_object uses configured bucket", func(t *testing.T) {
		obj := "configured/delete.txt"
		writeGCSObject(t, ctx, client, bucket, obj, "text/plain", "configured delete")
		args := map[string]any{"object": obj}
		result, status := tr.invokeTool(t, ctx, "my_delete_object_configured", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if !strings.Contains(result, `"deleted":true`) {
			t.Errorf("expected deleted confirmation, got %s", result)
		}
		if gcsObjectExists(t, ctx, client, bucket, obj) {
			t.Errorf("expected object %q to be deleted", obj)
		}
	})
}

func runCloudStorageDownloadObjectTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, bucket string) {
	t.Run("happy path writes expected bytes", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "downloaded.txt")
		args := map[string]any{"bucket": bucket, "object": downloadObject, "destination": dest}
		result, status := tr.invokeTool(t, ctx, "my_download_object", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if ct := extractStringField(t, result, "contentType"); ct != "text/plain" {
			t.Errorf("contentType = %q, want text/plain (raw %s)", ct, result)
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("failed to read downloaded file: %v", err)
		}
		if string(got) != downloadBody {
			t.Errorf("downloaded content = %q, want %q", string(got), downloadBody)
		}
	})

	t.Run("overwrite=false on existing file returns agent error", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "existing.txt")
		if err := os.WriteFile(dest, []byte("pre-existing"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		args := map[string]any{"bucket": bucket, "object": downloadObject, "destination": dest}
		result, status := tr.invokeTool(t, ctx, "my_download_object", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if !strings.Contains(result, "overwrite") && !strings.Contains(result, "exists") {
			t.Errorf("expected error referencing overwrite/exists, got %s", result)
		}
		got, err := os.ReadFile(dest)
		if err != nil || string(got) != "pre-existing" {
			t.Errorf("destination was modified or unreadable: %q, %v", string(got), err)
		}
	})

	t.Run("overwrite=true replaces existing file", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "existing.txt")
		if err := os.WriteFile(dest, []byte("pre-existing"), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		args := map[string]any{"bucket": bucket, "object": downloadObject, "destination": dest, "overwrite": true}
		result, status := tr.invokeTool(t, ctx, "my_download_object", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("failed to read downloaded file: %v", err)
		}
		if string(got) != downloadBody {
			t.Errorf("downloaded content = %q, want %q", string(got), downloadBody)
		}
	})

	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "relative destination returns agent error",
			args:           map[string]any{"bucket": bucket, "object": downloadObject, "destination": "relative/out.bin"},
			wantSubstrings: []string{"destination"},
		},
		{
			name:           "destination with traversal returns agent error",
			args:           map[string]any{"bucket": bucket, "object": downloadObject, "destination": "/tmp/../etc/passwd"},
			wantSubstrings: []string{"destination"},
		},
		{
			name:           "missing destination returns agent error",
			args:           map[string]any{"bucket": bucket, "object": downloadObject},
			wantSubstrings: []string{"destination"},
		},
		{
			name:           "nonexistent object returns error",
			args:           map[string]any{"bucket": bucket, "object": "does/not/exist.bin", "destination": "/tmp/nope-should-not-be-created-" + uuid.New().String()[:8] + ".bin"},
			wantSubstrings: []string{"does/not/exist.bin"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_download_object", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

func runCloudStorageUploadObjectTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, client *storage.Client, bucket string) {
	// Seed a local file that the explicit-content-type and MIME-auto-detect
	// cases both read from.
	srcDir := t.TempDir()
	csvPath := filepath.Join(srcDir, "data.csv")
	if err := os.WriteFile(csvPath, []byte("a,b\n1,2\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	binPath := filepath.Join(srcDir, "blob.unknownext")
	if err := os.WriteFile(binPath, []byte("<html>hi</html>"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	t.Run("upload with explicit content_type", func(t *testing.T) {
		obj := "uploaded/explicit.bin"
		args := map[string]any{"bucket": bucket, "object": obj, "source": csvPath, "content_type": "application/octet-stream"}
		result, status := tr.invokeTool(t, ctx, "my_upload_object", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if ct := extractStringField(t, result, "contentType"); ct != "application/octet-stream" {
			t.Errorf("contentType = %q, want application/octet-stream (raw %s)", ct, result)
		}
		attrs, err := client.Bucket(bucket).Object(obj).Attrs(ctx)
		if err != nil {
			t.Fatalf("expected uploaded object to exist: %v", err)
		}
		if attrs.ContentType != "application/octet-stream" {
			t.Errorf("GCS ContentType = %q, want application/octet-stream", attrs.ContentType)
		}
	})

	t.Run("upload infers MIME from .csv extension", func(t *testing.T) {
		obj := "uploaded/inferred.csv"
		args := map[string]any{"bucket": bucket, "object": obj, "source": csvPath}
		result, status := tr.invokeTool(t, ctx, "my_upload_object", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		// mime.TypeByExtension(".csv") is "text/csv" on most systems but may
		// carry a charset suffix — assert containment rather than equality.
		if ct := extractStringField(t, result, "contentType"); !strings.Contains(ct, "csv") {
			t.Errorf("contentType = %q, want to contain 'csv' (raw %s)", ct, result)
		}
	})

	t.Run("upload with unknown extension lets GCS auto-detect", func(t *testing.T) {
		obj := "uploaded/unknown.bin"
		args := map[string]any{"bucket": bucket, "object": obj, "source": binPath}
		result, status := tr.invokeTool(t, ctx, "my_upload_object", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		attrs, err := client.Bucket(bucket).Object(obj).Attrs(ctx)
		if err != nil {
			t.Fatalf("expected uploaded object to exist: %v", err)
		}
		// GCS always records *something* — verify the bytes landed regardless
		// of whichever content type the server ended up storing.
		if attrs.Size != int64(len("<html>hi</html>")) {
			t.Errorf("object size = %d, want %d", attrs.Size, len("<html>hi</html>"))
		}
	})

	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "missing source returns agent error",
			args:           map[string]any{"bucket": bucket, "object": "uploaded/nope.bin"},
			wantSubstrings: []string{"source"},
		},
		{
			name:           "relative source returns agent error",
			args:           map[string]any{"bucket": bucket, "object": "uploaded/nope.bin", "source": "relative/path"},
			wantSubstrings: []string{"source"},
		},
		{
			name:           "nonexistent local source returns error",
			args:           map[string]any{"bucket": bucket, "object": "uploaded/nope.bin", "source": "/tmp/definitely-not-a-real-source-" + uuid.New().String()[:8]},
			wantSubstrings: []string{"source", "no such"},
		},
		{
			name:           "missing bucket returns agent error",
			args:           map[string]any{"object": "uploaded/nope.bin", "source": csvPath},
			wantSubstrings: []string{"bucket"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_upload_object", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

func runCloudStorageWriteObjectTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, client *storage.Client, bucket string) {
	tcs := []struct {
		name            string
		args            map[string]any
		object          string
		wantContent     string
		wantContentType string
		wantSize        int64
		wantSubstrings  []string
	}{
		{
			name:            "write object with explicit content_type",
			object:          "written/explicit.txt",
			wantContent:     "written from tool",
			wantContentType: "text/plain",
			wantSize:        int64(len("written from tool")),
		},
		{
			name:            "write empty object",
			object:          "written/empty.txt",
			wantContent:     "",
			wantContentType: "text/plain",
			wantSize:        0,
		},
		{
			name:           "missing bucket returns agent error",
			args:           map[string]any{"object": "written/nope.txt", "content": "x"},
			wantSubstrings: []string{"bucket"},
		},
		{
			name:           "missing object returns agent error",
			args:           map[string]any{"bucket": bucket, "content": "x"},
			wantSubstrings: []string{"object"},
		},
		{
			name:           "missing content returns agent error",
			args:           map[string]any{"bucket": bucket, "object": "written/nope.txt"},
			wantSubstrings: []string{"content"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			args := tc.args
			if args == nil {
				args = map[string]any{"bucket": bucket, "object": tc.object, "content": tc.wantContent, "content_type": tc.wantContentType}
			}
			result, status := tr.invokeTool(t, ctx, "my_write_object", args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			if tc.object != "" {
				if ct := extractStringField(t, result, "contentType"); ct != tc.wantContentType {
					t.Errorf("contentType = %q, want %q (raw %s)", ct, tc.wantContentType, result)
				}
				got, attrs := readGCSObject(t, ctx, client, bucket, tc.object)
				if got != tc.wantContent {
					t.Errorf("written content = %q, want %q", got, tc.wantContent)
				}
				if attrs.ContentType != tc.wantContentType {
					t.Errorf("GCS ContentType = %q, want %q", attrs.ContentType, tc.wantContentType)
				}
				if attrs.Size != tc.wantSize {
					t.Errorf("object size = %d, want %d", attrs.Size, tc.wantSize)
				}
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

func runCloudStorageCopyObjectTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, client *storage.Client, bucket string) {
	t.Run("copy creates destination and leaves source", func(t *testing.T) {
		dest := "copied/hello.txt"
		args := map[string]any{"source_bucket": bucket, "source_object": helloObject, "destination_bucket": bucket, "destination_object": dest}
		result, status := tr.invokeTool(t, ctx, "my_copy_object", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		got, attrs := readGCSObject(t, ctx, client, bucket, dest)
		if got != helloBody {
			t.Errorf("copied content = %q, want %q", got, helloBody)
		}
		if attrs.ContentType != "text/plain" {
			t.Errorf("copied ContentType = %q, want text/plain", attrs.ContentType)
		}
		if !gcsObjectExists(t, ctx, client, bucket, helloObject) {
			t.Errorf("expected source object %q to remain after copy", helloObject)
		}
	})

	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "missing source_bucket returns agent error",
			args:           map[string]any{"source_object": helloObject, "destination_bucket": bucket, "destination_object": "copied/nope.txt"},
			wantSubstrings: []string{"source_bucket"},
		},
		{
			name:           "missing source_object returns agent error",
			args:           map[string]any{"source_bucket": bucket, "destination_bucket": bucket, "destination_object": "copied/nope.txt"},
			wantSubstrings: []string{"source_object"},
		},
		{
			name:           "missing destination_bucket returns agent error",
			args:           map[string]any{"source_bucket": bucket, "source_object": helloObject, "destination_object": "copied/nope.txt"},
			wantSubstrings: []string{"destination_bucket"},
		},
		{
			name:           "missing destination_object returns agent error",
			args:           map[string]any{"source_bucket": bucket, "source_object": helloObject, "destination_bucket": bucket},
			wantSubstrings: []string{"destination_object"},
		},
		{
			name:           "nonexistent source object returns error",
			args:           map[string]any{"source_bucket": bucket, "source_object": "does/not/exist.txt", "destination_bucket": bucket, "destination_object": "copied/nope.txt"},
			wantSubstrings: []string{"does/not/exist.txt"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_copy_object", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

func runCloudStorageMoveObjectTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, client *storage.Client, bucket string) {
	t.Run("move atomically renames within bucket", func(t *testing.T) {
		src := "move/source.txt"
		dest := "move/destination.txt"
		writeGCSObject(t, ctx, client, bucket, src, "text/plain", "move me")
		args := map[string]any{"bucket": bucket, "source_object": src, "destination_object": dest}
		result, status := tr.invokeTool(t, ctx, "my_move_object", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		got, _ := readGCSObject(t, ctx, client, bucket, dest)
		if got != "move me" {
			t.Errorf("moved content = %q, want %q", got, "move me")
		}
		if gcsObjectExists(t, ctx, client, bucket, src) {
			t.Errorf("expected source object %q to be gone after move", src)
		}
	})

	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "missing bucket returns agent error",
			args:           map[string]any{"source_object": "move/nope.txt", "destination_object": "move/nope2.txt"},
			wantSubstrings: []string{"bucket"},
		},
		{
			name:           "missing source_object returns agent error",
			args:           map[string]any{"bucket": bucket, "destination_object": "move/nope2.txt"},
			wantSubstrings: []string{"source_object"},
		},
		{
			name:           "missing destination_object returns agent error",
			args:           map[string]any{"bucket": bucket, "source_object": "move/nope.txt"},
			wantSubstrings: []string{"destination_object"},
		},
		{
			name:           "nonexistent source object returns error",
			args:           map[string]any{"bucket": bucket, "source_object": "move/does-not-exist.txt", "destination_object": "move/nope2.txt"},
			wantSubstrings: []string{"move/does-not-exist.txt"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_move_object", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

func runCloudStorageDeleteObjectTest(ctx context.Context, t *testing.T, tr cloudStorageTransport, client *storage.Client, bucket string) {
	t.Run("delete removes object", func(t *testing.T) {
		obj := "delete/delete-me.txt"
		writeGCSObject(t, ctx, client, bucket, obj, "text/plain", "delete me")
		args := map[string]any{"bucket": bucket, "object": obj}
		result, status := tr.invokeTool(t, ctx, "my_delete_object", args)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		if !strings.Contains(result, `"deleted":true`) {
			t.Errorf("expected deleted confirmation, got %s", result)
		}
		if gcsObjectExists(t, ctx, client, bucket, obj) {
			t.Errorf("expected object %q to be deleted", obj)
		}
	})

	tcs := []struct {
		name           string
		args           map[string]any
		wantSubstrings []string
	}{
		{
			name:           "missing bucket returns agent error",
			args:           map[string]any{"object": "delete/nope.txt"},
			wantSubstrings: []string{"bucket"},
		},
		{
			name:           "missing object returns agent error",
			args:           map[string]any{"bucket": bucket},
			wantSubstrings: []string{"object"},
		},
		{
			name:           "missing object in storage returns agent-visible error",
			args:           map[string]any{"bucket": bucket, "object": "delete/does-not-exist.txt"},
			wantSubstrings: []string{"object", "does-not-exist"},
		},
	}
	for _, tc := range tcs {
		t.Run(tc.name, func(t *testing.T) {
			result, status := tr.invokeTool(t, ctx, "my_delete_object", tc.args)
			if status != http.StatusOK {
				t.Fatalf("unexpected status %d: %s", status, result)
			}
			for _, want := range tc.wantSubstrings {
				if !strings.Contains(result, want) {
					t.Errorf("expected result to contain %q, got %s", want, result)
				}
			}
		})
	}
}

func writeGCSObject(t *testing.T, ctx context.Context, client *storage.Client, bucket, object, contentType, body string) {
	t.Helper()
	w := client.Bucket(bucket).Object(object).NewWriter(ctx)
	w.ContentType = contentType
	if _, err := io.WriteString(w, body); err != nil {
		_ = w.Close()
		t.Fatalf("failed to write object %q: %v", object, err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close writer for object %q: %v", object, err)
	}
}

func readGCSObject(t *testing.T, ctx context.Context, client *storage.Client, bucket, object string) (string, *storage.ReaderObjectAttrs) {
	t.Helper()
	r, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		t.Fatalf("failed to open object %q: %v", object, err)
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("failed to read object %q: %v", object, err)
	}
	return string(data), &r.Attrs
}

func gcsObjectExists(t *testing.T, ctx context.Context, client *storage.Client, bucket, object string) bool {
	t.Helper()
	_, err := client.Bucket(bucket).Object(object).Attrs(ctx)
	if err == nil {
		return true
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
		return false
	}
	t.Fatalf("failed checking object %q existence: %v", object, err)
	return false
}

func gcsBucketExists(t *testing.T, ctx context.Context, client *storage.Client, bucket string) bool {
	t.Helper()
	_, err := client.Bucket(bucket).Attrs(ctx)
	if err == nil {
		return true
	}
	if errors.Is(err, storage.ErrBucketNotExist) {
		return false
	}
	t.Fatalf("failed checking bucket %q existence: %v", bucket, err)
	return false
}

// cleanupGCSBucket empties and deletes a bucket. It runs on a context detached
// from ctx, which may already be cancelled when cleanups run.
func cleanupGCSBucket(ctx context.Context, t *testing.T, client *storage.Client, bucket string) {
	t.Helper()
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cloudStorageCleanupTimeout)
	defer cancel()

	bkt := client.Bucket(bucket)
	it := bkt.Objects(cleanupCtx, nil)
	for {
		attrs, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			if errors.Is(err, storage.ErrBucketNotExist) {
				return
			}
			t.Logf("cleanup: failed listing bucket %q: %v", bucket, err)
			break
		}
		if delErr := bkt.Object(attrs.Name).Delete(cleanupCtx); delErr != nil {
			t.Logf("cleanup: failed deleting object %q from bucket %q: %v", attrs.Name, bucket, delErr)
		}
	}
	if err := bkt.Delete(cleanupCtx); err != nil && !errors.Is(err, storage.ErrBucketNotExist) {
		t.Logf("cleanup: failed deleting bucket %q: %v", bucket, err)
	}
}
