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
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/tests"
	"github.com/testcontainers/testcontainers-go"
	tclog "github.com/testcontainers/testcontainers-go/log"
	"github.com/testcontainers/testcontainers-go/wait"
)

// The emulator test covers the tools whose behavior fake-gcs-server models:
// object list/read/metadata/download/upload/write/copy/delete and bucket
// create/metadata/delete. Real-GCP coverage in cloud_storage_integration_test.go
// exercises the remaining features: move_object (fake-gcs-server lacks the
// objects.move endpoint), get_bucket_iam_policy, bucket location and uniform
// bucket-level access, project-scoped list_buckets, and real authentication.
const (
	fakeGCSImage    = "fsouza/fake-gcs-server:1.56.1"
	fakeGCSPort     = "4443"
	emulatorProject = "toolbox-emulator-project"
)

// setupFakeGCSContainer starts fake-gcs-server in memory mode, schedules
// container termination via t.Cleanup, configures its external URL and public
// host to match the mapped host:port, and returns that host:port.
func setupFakeGCSContainer(ctx context.Context, t *testing.T) string {
	t.Helper()

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        fakeGCSImage,
			Cmd:          []string{"-scheme", "http", "-port", fakeGCSPort, "-backend", "memory"},
			ExposedPorts: []string{fakeGCSPort + "/tcp"},
			WaitingFor: wait.ForHTTP("/_internal/healthcheck").
				WithPort(fakeGCSPort + "/tcp").
				WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
		Logger:  tclog.TestLogger(t),
	})
	testcontainers.CleanupContainer(t, container)
	if err != nil {
		t.Fatalf("failed to start fake-gcs-server container: %s", err)
	}

	endpoint, err := container.PortEndpoint(ctx, fakeGCSPort+"/tcp", "")
	if err != nil {
		t.Fatalf("failed to get fake-gcs-server endpoint: %s", err)
	}

	// fake-gcs-server matches path-style object requests against its public
	// host and builds resumable-upload URLs from its external URL. Point both
	// at the mapped endpoint so uploads and reads route properly.
	cfg := fmt.Sprintf(`{"externalUrl": "http://%s", "publicHost": %q}`, endpoint, endpoint)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://"+endpoint+"/_internal/config", strings.NewReader(cfg))
	if err != nil {
		t.Fatalf("failed to build fake-gcs-server config request: %s", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed to configure fake-gcs-server: %s", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("configuring fake-gcs-server returned status %d", resp.StatusCode)
	}
	return endpoint
}

func getCloudStorageEmulatorToolsConfig() map[string]any {
	tool := func(toolType, description string) map[string]any {
		return map[string]any{
			"type":        toolType,
			"source":      "my_instance",
			"description": description,
		}
	}
	return map[string]any{
		"sources": map[string]any{
			"my_instance": map[string]any{
				"type":    CloudStorageSourceType,
				"project": emulatorProject,
			},
		},
		"tools": map[string]any{
			"my_list_objects":        tool("cloud-storage-list-objects", "List objects in a Cloud Storage bucket."),
			"my_read_object":         tool("cloud-storage-read-object", "Read a Cloud Storage object."),
			"my_get_object_metadata": tool("cloud-storage-get-object-metadata", "Get metadata for a Cloud Storage object."),
			"my_download_object":     tool("cloud-storage-download-object", "Download a Cloud Storage object to a local file."),
			"my_upload_object":       tool("cloud-storage-upload-object", "Upload a local file to a Cloud Storage object."),
			"my_write_object":        tool("cloud-storage-write-object", "Write text content to a Cloud Storage object."),
			"my_copy_object":         tool("cloud-storage-copy-object", "Copy a Cloud Storage object."),
			"my_delete_object":       tool("cloud-storage-delete-object", "Delete a Cloud Storage object."),
			"my_create_bucket":       tool("cloud-storage-create-bucket", "Create a Cloud Storage bucket."),
			"my_get_bucket_metadata": tool("cloud-storage-get-bucket-metadata", "Get metadata for a Cloud Storage bucket."),
			"my_delete_bucket":       tool("cloud-storage-delete-bucket", "Delete an empty Cloud Storage bucket."),
		},
	}
}

func TestCloudStorageEmulatorToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	endpoint := setupFakeGCSContainer(ctx, t)
	// STORAGE_EMULATOR_HOST routes both the test's own storage.Client and the
	// source client initialized by the Toolbox server to fake-gcs-server
	// without requiring Google Cloud credentials.
	t.Setenv("STORAGE_EMULATOR_HOST", "http://"+endpoint)

	client := newGCSTestClient(t)

	bucketName := testBucketPrefix + "emu-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
	seedCloudStorageTestBucket(t, ctx, client, emulatorProject, bucketName)

	cmd, cleanup, err := tests.StartCmd(ctx, getCloudStorageEmulatorToolsConfig(), "--enable-api")
	if err != nil {
		t.Fatalf("command initialization returned an error: %s", err)
	}
	t.Cleanup(cleanup)

	waitCtx, waitCancel := context.WithTimeout(ctx, 10*time.Second)
	defer waitCancel()
	out, err := testutils.WaitForString(waitCtx, regexp.MustCompile(`Server ready to serve`), cmd.Out)
	if err != nil {
		t.Logf("toolbox command logs: \n%s", out)
		t.Fatalf("toolbox didn't start successfully: %s", err)
	}

	invokeOK := func(t *testing.T, toolName, body string, want ...string) string {
		t.Helper()
		result, status := invokeTool(t, toolName, body)
		if status != http.StatusOK {
			t.Fatalf("unexpected status %d: %s", status, result)
		}
		for _, w := range want {
			if !strings.Contains(result, w) {
				t.Errorf("expected result to contain %q, got %s", w, result)
			}
		}
		return result
	}

	t.Run("list_objects with prefix", func(t *testing.T) {
		invokeOK(t, "my_list_objects",
			fmt.Sprintf(`{"bucket": %q, "prefix": "seed/"}`, bucketName),
			helloObject, jsonObject)
	})

	t.Run("list_objects with delimiter returns prefixes", func(t *testing.T) {
		invokeOK(t, "my_list_objects",
			fmt.Sprintf(`{"bucket": %q, "prefix": "seed/", "delimiter": "/"}`, bucketName),
			helloObject, `"seed/nested/"`)
	})

	t.Run("read_object full object", func(t *testing.T) {
		result := invokeOK(t, "my_read_object", fmt.Sprintf(`{"bucket": %q, "object": %q}`, bucketName, helloObject))
		if got := extractStringField(t, result, "content"); got != helloBody {
			t.Errorf("content = %q, want %q (raw %s)", got, helloBody, result)
		}
		if got := extractStringField(t, result, "contentType"); got != "text/plain" {
			t.Errorf("contentType = %q, want text/plain (raw %s)", got, result)
		}
	})

	t.Run("read_object byte range", func(t *testing.T) {
		result := invokeOK(t, "my_read_object",
			fmt.Sprintf(`{"bucket": %q, "object": %q, "range": "bytes=0-4"}`, bucketName, helloObject))
		if got := extractStringField(t, result, "content"); got != "hello" {
			t.Errorf("content = %q, want hello (raw %s)", got, result)
		}
	})

	t.Run("read_object oversize returns agent error", func(t *testing.T) {
		invokeOK(t, "my_read_object",
			fmt.Sprintf(`{"bucket": %q, "object": %q}`, bucketName, largeObject), "size limit")
	})

	t.Run("read_object binary returns agent error", func(t *testing.T) {
		invokeOK(t, "my_read_object",
			fmt.Sprintf(`{"bucket": %q, "object": %q}`, bucketName, binaryObject), "UTF-8")
	})

	t.Run("read_object nonexistent object returns error", func(t *testing.T) {
		invokeOK(t, "my_read_object",
			fmt.Sprintf(`{"bucket": %q, "object": "does/not/exist.bin"}`, bucketName), "does/not/exist.bin")
	})

	t.Run("get_object_metadata", func(t *testing.T) {
		invokeOK(t, "my_get_object_metadata",
			fmt.Sprintf(`{"bucket": %q, "object": %q}`, bucketName, helloObject),
			`"Name":"`+helloObject+`"`, `"Size":11`)
	})

	t.Run("download_object writes expected bytes", func(t *testing.T) {
		dest := filepath.Join(t.TempDir(), "downloaded.txt")
		invokeOK(t, "my_download_object",
			fmt.Sprintf(`{"bucket": %q, "object": %q, "destination": %q}`, bucketName, downloadObject, dest))
		got, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("failed to read downloaded file: %v", err)
		}
		if string(got) != downloadBody {
			t.Errorf("downloaded content = %q, want %q", string(got), downloadBody)
		}
	})

	t.Run("upload_object stores file content", func(t *testing.T) {
		src := filepath.Join(t.TempDir(), "upload.csv")
		content := "a,b\n1,2\n"
		if err := os.WriteFile(src, []byte(content), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		obj := "uploaded/data.csv"
		invokeOK(t, "my_upload_object",
			fmt.Sprintf(`{"bucket": %q, "object": %q, "source": %q, "content_type": "text/csv"}`, bucketName, obj, src))
		got, attrs := readGCSObject(t, ctx, client, bucketName, obj)
		if got != content {
			t.Errorf("uploaded content = %q, want %q", got, content)
		}
		if attrs.ContentType != "text/csv" {
			t.Errorf("ContentType = %q, want text/csv", attrs.ContentType)
		}
	})

	t.Run("write_object stores content", func(t *testing.T) {
		obj := "written/explicit.txt"
		content := "written from tool"
		invokeOK(t, "my_write_object",
			fmt.Sprintf(`{"bucket": %q, "object": %q, "content": %q, "content_type": "text/plain"}`, bucketName, obj, content))
		got, attrs := readGCSObject(t, ctx, client, bucketName, obj)
		if got != content {
			t.Errorf("written content = %q, want %q", got, content)
		}
		if attrs.ContentType != "text/plain" {
			t.Errorf("ContentType = %q, want text/plain", attrs.ContentType)
		}
	})

	t.Run("copy_object creates destination and keeps source", func(t *testing.T) {
		dest := "copied/hello.txt"
		invokeOK(t, "my_copy_object",
			fmt.Sprintf(`{"source_bucket": %q, "source_object": %q, "destination_bucket": %q, "destination_object": %q}`,
				bucketName, helloObject, bucketName, dest))
		if got, _ := readGCSObject(t, ctx, client, bucketName, dest); got != helloBody {
			t.Errorf("copied content = %q, want %q", got, helloBody)
		}
		if !gcsObjectExists(t, ctx, client, bucketName, helloObject) {
			t.Errorf("expected source object %q to remain after copy", helloObject)
		}
	})

	t.Run("delete_object removes object", func(t *testing.T) {
		obj := "delete/delete-me.txt"
		writeGCSObject(t, ctx, client, bucketName, obj, "text/plain", "delete me")
		invokeOK(t, "my_delete_object",
			fmt.Sprintf(`{"bucket": %q, "object": %q}`, bucketName, obj), `"deleted":true`)
		if gcsObjectExists(t, ctx, client, bucketName, obj) {
			t.Errorf("expected object %q to be deleted", obj)
		}
	})

	t.Run("bucket create, metadata and delete", func(t *testing.T) {
		bucket := testBucketPrefix + "emu-bkt-" + strings.ReplaceAll(uuid.New().String(), "-", "")[:16]
		registerGCSBucketCleanup(t, client, bucket)

		invokeOK(t, "my_create_bucket", fmt.Sprintf(`{"bucket": %q}`, bucket), bucket, `"created":true`)
		if !gcsBucketExists(t, ctx, client, bucket) {
			t.Fatalf("expected bucket %q to exist after create", bucket)
		}
		invokeOK(t, "my_get_bucket_metadata", fmt.Sprintf(`{"bucket": %q}`, bucket), `"Name":"`+bucket+`"`)
		invokeOK(t, "my_delete_bucket", fmt.Sprintf(`{"bucket": %q}`, bucket), `"deleted":true`)
		if gcsBucketExists(t, ctx, client, bucket) {
			t.Errorf("expected bucket %q to be deleted", bucket)
		}
	})

	t.Run("MCP tools/call read_object", func(t *testing.T) {
		statusCode, mcpResp, err := tests.InvokeMCPTool(t, "my_read_object", map[string]any{
			"bucket": bucketName,
			"object": helloObject,
		}, nil)
		if err != nil {
			t.Fatalf("InvokeMCPTool failed: %v", err)
		}
		if statusCode != http.StatusOK {
			t.Fatalf("expected HTTP 200, got %d", statusCode)
		}
		if mcpResp.Error != nil {
			t.Fatalf("unexpected JSON-RPC error: %v", mcpResp.Error)
		}
		if mcpResp.Result.IsError || len(mcpResp.Result.Content) == 0 {
			t.Fatalf("unexpected MCP tool result: %+v", mcpResp.Result)
		}
		if got := extractStringField(t, mcpResp.Result.Content[0].Text, "content"); got != helloBody {
			t.Errorf("MCP read_object content = %q, want %q", got, helloBody)
		}
	})
}
