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
	"testing"
	"time"

	"github.com/googleapis/mcp-toolbox/tests"
)

// getCloudStorageMCPManifests returns the expected tools/list entries for the
// tools in getCloudStorageToolsConfig. Configured variants omit the
// parameters baked into their config.
func getCloudStorageMCPManifests() []tests.MCPToolManifest {
	// my_copy_object
	copyObjectSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"destination_bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket to copy into.",
			},
			"destination_object": map[string]any{
				"type":        "string",
				"description": "Full destination object name (path) within the destination bucket, e.g. 'path/to/file.txt'.",
			},
			"source_bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket containing the source object.",
			},
			"source_object": map[string]any{
				"type":        "string",
				"description": "Full source object name (path) within the source bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"source_bucket", "source_object", "destination_bucket", "destination_object"},
	}
	// my_copy_object_configured
	copyObjectConfiguredSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"destination_object": map[string]any{
				"type":        "string",
				"description": "Full destination object name (path) within the destination bucket, e.g. 'path/to/file.txt'.",
			},
			"source_object": map[string]any{
				"type":        "string",
				"description": "Full source object name (path) within the source bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"source_object", "destination_object"},
	}
	// my_create_bucket
	createBucketSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket to create.",
			},
			"location": map[string]any{
				"type":        "string",
				"description": "Location for the bucket, e.g. 'US', 'EU', or 'us-central1'. Omit to use the Cloud Storage service default.",
			},
			"project": map[string]any{
				"type":        "string",
				"description": "Project ID to create the bucket in. When empty, the source's configured project is used.",
				"default":     "",
			},
			"uniform_bucket_level_access": map[string]any{
				"type":        "boolean",
				"description": "Whether to enable uniform bucket-level access on the bucket.",
				"default":     false,
			},
		},
		"required": []any{"bucket"},
	}
	// my_create_bucket_configured
	createBucketConfiguredSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket to create.",
			},
		},
		"required": []any{"bucket"},
	}
	// my_delete_bucket
	deleteBucketSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the empty Cloud Storage bucket to delete.",
			},
		},
		"required": []any{"bucket"},
	}
	// my_delete_object
	deleteObjectSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket containing the object to delete.",
			},
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"bucket", "object"},
	}
	// my_delete_object_configured, my_get_object_metadata_configured
	objectOnlySchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"object"},
	}
	// my_download_object
	downloadObjectSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket containing the object.",
			},
			"destination": map[string]any{
				"type":        "string",
				"description": "Absolute local filesystem path where the object will be written. Relative paths and paths containing '..' are rejected.",
			},
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
			"overwrite": map[string]any{
				"type":        "boolean",
				"description": "If true, overwrite the destination when it already exists. If false (default), the tool returns an error when the destination exists.",
				"default":     false,
			},
		},
		"required": []any{"bucket", "object", "destination"},
	}
	// my_download_object_configured
	downloadObjectConfiguredSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"destination": map[string]any{
				"type":        "string",
				"description": "Relative path under the configured destination_dir where the object will be written. Absolute paths and paths that escape destination_dir are rejected.",
			},
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"object", "destination"},
	}
	// my_get_bucket_iam_policy
	getBucketIamPolicySchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket whose IAM policy should be returned.",
			},
		},
		"required": []any{"bucket"},
	}
	// my_get_bucket_iam_policy_configured, my_get_bucket_metadata_configured
	noParamsSchema := map[string]any{
		"type":       "object",
		"properties": map[string]any{},
		"required":   []any{},
	}
	// my_get_bucket_metadata
	getBucketMetadataSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket to inspect.",
			},
		},
		"required": []any{"bucket"},
	}
	// my_get_object_metadata
	getObjectMetadataSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket containing the object.",
			},
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"bucket", "object"},
	}
	// my_list_buckets
	listBucketsSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of buckets to return per page. A value of 0 uses the API default (1000); negative values and values above 1000 are rejected.",
				"default":     float64(0),
			},
			"page_token": map[string]any{
				"type":        "string",
				"description": "A previously-returned page token for retrieving the next page of results.",
				"default":     "",
			},
			"prefix": map[string]any{
				"type":        "string",
				"description": "Filter results to buckets whose names begin with this prefix.",
				"default":     "",
			},
			"project": map[string]any{
				"type":        "string",
				"description": "Project ID to list buckets in. When empty, the source's configured project is used.",
				"default":     "",
			},
		},
		"required": []any{},
	}
	// my_list_buckets_configured
	listBucketsConfiguredSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of buckets to return per page. A value of 0 uses the API default (1000); negative values and values above 1000 are rejected.",
				"default":     float64(0),
			},
			"page_token": map[string]any{
				"type":        "string",
				"description": "A previously-returned page token for retrieving the next page of results.",
				"default":     "",
			},
		},
		"required": []any{},
	}
	// my_list_objects
	listObjectsSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket to list objects from.",
			},
			"delimiter": map[string]any{
				"type":        "string",
				"description": "Delimiter used to group object names (typically '/'). When set, common prefixes are returned as 'prefixes'.",
				"default":     "",
			},
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of objects to return per page. A value of 0 uses the API default (1000); negative values and values above 1000 are rejected.",
				"default":     float64(0),
			},
			"page_token": map[string]any{
				"type":        "string",
				"description": "A previously-returned page token for retrieving the next page of results.",
				"default":     "",
			},
			"prefix": map[string]any{
				"type":        "string",
				"description": "Filter results to objects whose names begin with this prefix.",
				"default":     "",
			},
		},
		"required": []any{"bucket"},
	}
	// my_list_objects_configured
	listObjectsConfiguredSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"max_results": map[string]any{
				"type":        "integer",
				"description": "Maximum number of objects to return per page. A value of 0 uses the API default (1000); negative values and values above 1000 are rejected.",
				"default":     float64(0),
			},
			"page_token": map[string]any{
				"type":        "string",
				"description": "A previously-returned page token for retrieving the next page of results.",
				"default":     "",
			},
		},
		"required": []any{},
	}
	// my_move_object
	moveObjectSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket containing the object to move.",
			},
			"destination_object": map[string]any{
				"type":        "string",
				"description": "Full destination object name (path) within the same bucket, e.g. 'path/to/file.txt'.",
			},
			"source_object": map[string]any{
				"type":        "string",
				"description": "Full source object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"bucket", "source_object", "destination_object"},
	}
	// my_move_object_configured
	moveObjectConfiguredSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"destination_object": map[string]any{
				"type":        "string",
				"description": "Full destination object name (path) within the same bucket, e.g. 'path/to/file.txt'.",
			},
			"source_object": map[string]any{
				"type":        "string",
				"description": "Full source object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"source_object", "destination_object"},
	}
	// my_read_object
	readObjectSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket containing the object.",
			},
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
			"range": map[string]any{
				"type":        "string",
				"description": "Optional HTTP byte range, e.g. 'bytes=0-999' (first 1000 bytes), 'bytes=-500' (last 500 bytes), or 'bytes=500-' (from byte 500 to end). Empty reads the full object.",
				"default":     "",
			},
		},
		"required": []any{"bucket", "object"},
	}
	// my_read_object_configured
	readObjectConfiguredSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
			"range": map[string]any{
				"type":        "string",
				"description": "Optional HTTP byte range, e.g. 'bytes=0-999' (first 1000 bytes), 'bytes=-500' (last 500 bytes), or 'bytes=500-' (from byte 500 to end). Empty reads the full object.",
				"default":     "",
			},
		},
		"required": []any{"object"},
	}
	// my_upload_object
	uploadObjectSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket to upload into.",
			},
			"content_type": map[string]any{
				"type":        "string",
				"description": "MIME type to record on the uploaded object. When empty, it is inferred from the source file's extension; if that fails, Cloud Storage auto-detects from the first 512 bytes of content.",
				"default":     "",
			},
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
			"source": map[string]any{
				"type":        "string",
				"description": "Absolute local filesystem path of the file to upload. Relative paths and paths containing '..' are rejected.",
			},
		},
		"required": []any{"bucket", "object", "source"},
	}
	// my_upload_object_configured
	uploadObjectConfiguredSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content_type": map[string]any{
				"type":        "string",
				"description": "MIME type to record on the uploaded object. When empty, it is inferred from the source file's extension; if that fails, Cloud Storage auto-detects from the first 512 bytes of content.",
				"default":     "",
			},
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
			"source": map[string]any{
				"type":        "string",
				"description": "Absolute local filesystem path of the file to upload. Relative paths and paths containing '..' are rejected.",
			},
		},
		"required": []any{"object", "source"},
	}
	// my_write_object
	writeObjectSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bucket": map[string]any{
				"type":        "string",
				"description": "Name of the Cloud Storage bucket to write into.",
			},
			"content": map[string]any{
				"type":        "string",
				"description": "Text content to write to the Cloud Storage object.",
			},
			"content_type": map[string]any{
				"type":        "string",
				"description": "MIME type to record on the written object. When empty, Cloud Storage auto-detects from the first 512 bytes of content.",
				"default":     "",
			},
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"bucket", "object", "content"},
	}
	// my_write_object_configured
	writeObjectConfiguredSchema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"content": map[string]any{
				"type":        "string",
				"description": "Text content to write to the Cloud Storage object.",
			},
			"content_type": map[string]any{
				"type":        "string",
				"description": "MIME type to record on the written object. When empty, Cloud Storage auto-detects from the first 512 bytes of content.",
				"default":     "",
			},
			"object": map[string]any{
				"type":        "string",
				"description": "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.",
			},
		},
		"required": []any{"object", "content"},
	}

	return []tests.MCPToolManifest{
		{Name: "my_copy_object", Description: "Copy a Cloud Storage object.", InputSchema: copyObjectSchema},
		{Name: "my_create_bucket", Description: "Create a Cloud Storage bucket.", InputSchema: createBucketSchema},
		{Name: "my_delete_bucket", Description: "Delete an empty Cloud Storage bucket.", InputSchema: deleteBucketSchema},
		{Name: "my_delete_object", Description: "Delete a Cloud Storage object.", InputSchema: deleteObjectSchema},
		{Name: "my_download_object", Description: "Download a Cloud Storage object to a local file.", InputSchema: downloadObjectSchema},
		{Name: "my_get_bucket_iam_policy", Description: "Get the IAM policy for a Cloud Storage bucket.", InputSchema: getBucketIamPolicySchema},
		{Name: "my_get_bucket_metadata", Description: "Get metadata for a Cloud Storage bucket.", InputSchema: getBucketMetadataSchema},
		{Name: "my_get_object_metadata", Description: "Get metadata for a Cloud Storage object.", InputSchema: getObjectMetadataSchema},
		{Name: "my_list_buckets", Description: "List Cloud Storage buckets in the project.", InputSchema: listBucketsSchema},
		{Name: "my_list_objects", Description: "List objects in a Cloud Storage bucket.", InputSchema: listObjectsSchema},
		{Name: "my_move_object", Description: "Move a Cloud Storage object within the same bucket.", InputSchema: moveObjectSchema},
		{Name: "my_read_object", Description: "Read a Cloud Storage object.", InputSchema: readObjectSchema},
		{Name: "my_upload_object", Description: "Upload a local file to a Cloud Storage object.", InputSchema: uploadObjectSchema},
		{Name: "my_write_object", Description: "Write text content to a Cloud Storage object.", InputSchema: writeObjectSchema},
		{Name: "my_copy_object_configured", Description: "Copy a Cloud Storage object between configured buckets.", InputSchema: copyObjectConfiguredSchema},
		{Name: "my_create_bucket_configured", Description: "Create a Cloud Storage bucket with configured settings.", InputSchema: createBucketConfiguredSchema},
		{Name: "my_delete_object_configured", Description: "Delete an object from a configured Cloud Storage bucket.", InputSchema: objectOnlySchema},
		{Name: "my_download_object_configured", Description: "Download a Cloud Storage object with configured storage settings.", InputSchema: downloadObjectConfiguredSchema},
		{Name: "my_get_bucket_iam_policy_configured", Description: "Get the IAM policy for a configured Cloud Storage bucket.", InputSchema: noParamsSchema},
		{Name: "my_get_bucket_metadata_configured", Description: "Get metadata for a configured Cloud Storage bucket.", InputSchema: noParamsSchema},
		{Name: "my_get_object_metadata_configured", Description: "Get object metadata from a configured Cloud Storage bucket.", InputSchema: objectOnlySchema},
		{Name: "my_list_buckets_configured", Description: "List Cloud Storage buckets in a configured project.", InputSchema: listBucketsConfiguredSchema},
		{Name: "my_list_objects_configured", Description: "List objects in a configured Cloud Storage bucket.", InputSchema: listObjectsConfiguredSchema},
		{Name: "my_move_object_configured", Description: "Move a Cloud Storage object within a configured bucket.", InputSchema: moveObjectConfiguredSchema},
		{Name: "my_read_object_configured", Description: "Read an object from a configured Cloud Storage bucket.", InputSchema: readObjectConfiguredSchema},
		{Name: "my_upload_object_configured", Description: "Upload a local file to a configured Cloud Storage bucket.", InputSchema: uploadObjectConfiguredSchema},
		{Name: "my_write_object_configured", Description: "Write text content to a configured Cloud Storage bucket.", InputSchema: writeObjectConfiguredSchema},
	}
}

func TestCloudStorageMCPToolEndpoints(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	env := setupCloudStorageTest(t, ctx)

	tr := cloudStorageTransport{isMCP: true}
	tr.startServer(t, ctx, env.toolsFile)

	tests.RunMCPToolsListMethod(t, getCloudStorageMCPManifests())

	runCloudStorageTests(ctx, t, tr, env)
}
