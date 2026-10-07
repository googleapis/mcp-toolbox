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

package gcs_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"io/fs"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/storage"
	"github.com/google/go-cmp/cmp"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/resources/gcs"
	"github.com/googleapis/mcp-toolbox/internal/server"
	"github.com/googleapis/mcp-toolbox/internal/testutils"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragecommon"
)

type mockObject struct {
	content     []byte
	contentType string
	updated     time.Time
}

type mockClient struct {
	objects   map[string]mockObject
	gotBucket string
	gotObject string
	gotOffset int64
	gotLength int64
}

func (m *mockClient) GetObjectMetadata(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
	obj, ok := m.objects[bucket+"/"+object]
	if !ok {
		return nil, storage.ErrObjectNotExist
	}
	return &storage.ObjectAttrs{
		Bucket:      bucket,
		Name:        object,
		Size:        int64(len(obj.content)),
		ContentType: obj.contentType,
		Updated:     obj.updated,
	}, nil
}

func (m *mockClient) NewRangeReader(ctx context.Context, bucket, object string, offset, length int64) (io.ReadCloser, error) {
	m.gotBucket = bucket
	m.gotObject = object
	m.gotOffset = offset
	m.gotLength = length

	obj, ok := m.objects[bucket+"/"+object]
	if !ok {
		return nil, storage.ErrObjectNotExist
	}
	data := obj.content
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}
	end := int64(len(data))
	if length >= 0 && offset+length < end {
		end = offset + length
	}
	return io.NopCloser(bytes.NewReader(data[offset:end])), nil
}

func TestParseFromYamlGCS(t *testing.T) {
	defaultPriority := 1.0
	customPriority := 0.95
	defaultMaxSize := int64(resources.DefaultMaxFileSize)
	customMaxSize := int64(10485760)
	csvMimeType := resources.InferMimeType("data_dictionary.csv")
	trueVal := true

	tcs := []struct {
		desc string
		in   string
		want server.ResourceConfigs
	}{
		{
			desc: "basic example",
			in: `
			kind: resource
			name: enterprise_data_dictionary
			type: gcs
			uri: "gs://corp-knowledge-base/catalogs/data_dictionary.csv"
			`,
			want: server.ResourceConfigs{
				"enterprise_data_dictionary": &gcs.Config{
					ResourceConfigBase: resources.ResourceConfigBase{
						ConfigBase: resources.ConfigBase{
							Name:        "enterprise_data_dictionary",
							Type:        "gcs",
							MimeType:    csvMimeType,
							Annotations: &resources.ResourceAnnotations{Priority: &defaultPriority},
						},
						URI: "gs://corp-knowledge-base/catalogs/data_dictionary.csv",
					},
					MaxSize: &defaultMaxSize,
				},
			},
		},
		{
			desc: "with maxSize, title, description, and annotations",
			in: `
			kind: resource
			name: enterprise_data_dictionary
			type: gcs
			title: "Enterprise Data Dictionary"
			description: "Master definitions for database tables and business entities."
			uri: "gs://corp-knowledge-base/catalogs/Data_Dictionary.csv"
			maxSize: 10485760
			annotations:
				priority: 0.95
				audience:
					- assistant
					- user
			`,
			want: server.ResourceConfigs{
				"enterprise_data_dictionary": &gcs.Config{
					ResourceConfigBase: resources.ResourceConfigBase{
						ConfigBase: resources.ConfigBase{
							Name:        "enterprise_data_dictionary",
							Type:        "gcs",
							Title:       "Enterprise Data Dictionary",
							Description: "Master definitions for database tables and business entities.",
							MimeType:    csvMimeType,
							Annotations: &resources.ResourceAnnotations{
								Priority: &customPriority,
								Audience: []resources.AudienceRole{resources.RoleAssistant, resources.RoleUser},
							},
						},
						URI: "gs://corp-knowledge-base/catalogs/Data_Dictionary.csv",
					},
					MaxSize: &customMaxSize,
				},
			},
		},
		{
			desc: "UI enabled resource with ui:// scheme",
			in: `
			kind: resource
			name: my-gcs-app
			type: gcs
			ui: true
			uri: "ui://ui-bucket/views/dashboard.html"
			csp:
				connectDomains:
					- "https://api.example.com"
			domain: "https://example.com"
			permissions:
				- camera
			`,
			want: server.ResourceConfigs{
				"my-gcs-app": &gcs.Config{
					ResourceConfigBase: resources.ResourceConfigBase{
						ConfigBase: resources.ConfigBase{
							Name:        "my-gcs-app",
							Type:        "gcs",
							MimeType:    "text/html;profile=mcp-app",
							UI:          true,
							Annotations: &resources.ResourceAnnotations{Priority: &defaultPriority},
							CSP: &resources.CSPConfig{
								ConnectDomains: []string{"https://api.example.com"},
							},
							Domain:      "https://example.com",
							Permissions: &resources.PermissionsConfig{Camera: &trueVal},
						},
						URI: "ui://ui-bucket/views/dashboard.html",
					},
					MaxSize: &defaultMaxSize,
				},
			},
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, got, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err != nil {
				t.Fatalf("unable to unmarshal: %s", err)
			}
			if diff := cmp.Diff(tc.want, got); diff != "" {
				t.Fatalf("incorrect parse (-want +got):\n%s", diff)
			}
		})
	}
}

func TestFailParseFromYamlGCS(t *testing.T) {
	tcs := []struct {
		desc string
		in   string
		err  string
	}{
		{
			desc: "extra unknown field",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/file.txt"
			foo: bar
			`,
			err: "unknown field \"foo\"",
		},
		{
			desc: "missing required uri field",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			`,
			err: "missing required 'uri' field",
		},
		{
			desc: "missing required uri field when ui is true (no default ui://<name>)",
			in: `
			kind: resource
			name: missing-uri-app
			type: gcs
			ui: true
			`,
			err: "missing required 'uri' field",
		},
		{
			desc: "invalid scheme file://",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "file://my-bucket/file.txt"
			`,
			err: "must be 'gs'",
		},
		{
			desc: "invalid scheme https://",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "https://storage.googleapis.com/my-bucket/file.txt"
			`,
			err: "must be 'gs'",
		},
		{
			desc: "ui is true with non-ui scheme gs://",
			in: `
			kind: resource
			name: invalid-app
			type: gcs
			ui: true
			uri: "gs://ui-bucket/views/dashboard.html"
			`,
			err: "must be 'ui'",
		},
		{
			desc: "ui is false with ui:// scheme",
			in: `
			kind: resource
			name: invalid-gcs
			type: gcs
			uri: "ui://ui-bucket/views/dashboard.html"
			`,
			err: "scheme 'ui' is only permitted when 'ui' is true",
		},
		{
			desc: "missing bucket",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs:///file.txt"
			`,
			err: "missing bucket",
		},
		{
			desc: "missing object path",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket"
			`,
			err: "must point to a file object",
		},
		{
			desc: "ui:// without object path",
			in: `
			kind: resource
			name: no-object-app
			type: gcs
			ui: true
			uri: "ui://no-object-app"
			`,
			err: "must point to a file object",
		},
		{
			desc: "directory object path ending with slash",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/folder/"
			`,
			err: "must point to a file object",
		},
		{
			desc: "path traversal in uri",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/folder/../secret.txt"
			`,
			err: "backward traversal",
		},
		{
			desc: "disallowed extension .env",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/.env"
			`,
			err: "file extension \".env\" is not allowed",
		},
		{
			desc: "disallowed extension .key",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/private.key"
			`,
			err: "file extension \".key\" is not allowed",
		},
		{
			desc: "disallowed extension .pem",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/cert.pem"
			`,
			err: "file extension \".pem\" is not allowed",
		},
		{
			desc: "disallowed extension .exe",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/binary.exe"
			`,
			err: "file extension \".exe\" is not allowed",
		},
		{
			desc: "maxSize zero",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/file.txt"
			maxSize: 0
			`,
			err: "must be greater than 0",
		},
		{
			desc: "maxSize negative",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/file.txt"
			maxSize: -50
			`,
			err: "must be greater than 0",
		},
		{
			desc: "maxSize exceeds 1GB",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/file.txt"
			maxSize: 2000000000
			`,
			err: "cannot exceed 1GB",
		},
		{
			desc: "maxSize type string",
			in: `
			kind: resource
			name: my-gcs
			type: gcs
			uri: "gs://my-bucket/file.txt"
			maxSize: 50MB
			`,
			err: "cannot unmarshal",
		},
	}

	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			_, _, _, _, _, _, _, _, err := server.UnmarshalPrimitiveConfig(context.Background(), testutils.FormatYaml(tc.in))
			if err == nil {
				t.Fatalf("expected parsing to fail with %q, got nil", tc.err)
			}
			if !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("unexpected error: got %q, want substring %q", err.Error(), tc.err)
			}
		})
	}
}

func TestGCSResource_InitializeAndRead(t *testing.T) {
	updatedTime := time.Date(2026, 9, 10, 15, 30, 0, 0, time.UTC)
	htmlContent := "<html><body><h1>GCS App</h1></body></html>"
	mc := &mockClient{
		objects: map[string]mockObject{
			"corp-bucket/catalogs/dict.csv": {
				content:     []byte("id,name\n1,Alice\n2,Bob\n"),
				contentType: "text/csv",
				updated:     updatedTime,
			},
			"corp-bucket/2026/09/reports/daily/summary.json": {
				content:     []byte(`{"status":"ok"}`),
				contentType: "application/json",
				updated:     updatedTime,
			},
			"ui-bucket/views/dashboard.html": {
				content:     []byte(htmlContent),
				contentType: "text/html",
				updated:     updatedTime,
			},
			"corp-bucket/logs/large.txt": {
				content:     []byte(strings.Repeat("A", 100)),
				contentType: "text/plain",
				updated:     updatedTime,
			},
			"corp-bucket/logs/utf8_boundary.txt": {
				content:     []byte("a€b"),
				contentType: "text/plain",
				updated:     updatedTime,
			},
			"corp-bucket/logs/empty.txt": {
				content:     []byte(""),
				contentType: "text/plain",
				updated:     updatedTime,
			},
			"corp-bucket/logs/binary_disguised.txt": {
				content:     []byte{0xff, 0xfe, 0x00, 0x80, 0x99},
				contentType: "application/octet-stream",
				updated:     updatedTime,
			},
			"corp-bucket/logs/large_binary.txt": {
				content:     bytes.Repeat([]byte{0xff}, 10),
				contentType: "application/octet-stream",
				updated:     updatedTime,
			},
		},
	}

	cleanup := gcs.SetClientForTest(mc)
	defer cleanup()

	limit20 := int64(20)
	limit5 := int64(5)
	limit3 := int64(3)
	trueVal := true

	tcs := []struct {
		desc             string
		cfg              *gcs.Config
		wantInitErrIs    error
		wantSize         int64
		wantMimePrefix   string
		wantLastModified string
		wantUIMetadata   any
		wantReadErrIs    error
		wantContent      string
		wantBucket       string
		wantObject       string
		wantRangeOffset  int64
		wantRangeLength  int64
	}{
		{
			desc: "successful read and metadata population",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name: "dict",
						Type: "gcs",
					},
					URI: "gs://corp-bucket/catalogs/dict.csv",
				},
			},
			wantSize:         22,
			wantMimePrefix:   "text/csv",
			wantLastModified: "2026-09-10T15:30:00Z",
			wantContent:      "id,name\n1,Alice\n2,Bob\n",
			wantBucket:       "corp-bucket",
			wantObject:       "catalogs/dict.csv",
			wantRangeOffset:  0,
			wantRangeLength:  resources.DefaultMaxFileSize + 1,
		},
		{
			desc: "explicit mimeType and lastModified are preserved",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name:     "dict-custom-meta",
						Type:     "gcs",
						MimeType: "text/plain",
						Annotations: &resources.ResourceAnnotations{
							LastModified: "2025-01-01T00:00:00Z",
						},
					},
					URI: "gs://corp-bucket/catalogs/dict.csv",
				},
			},
			wantSize:         22,
			wantMimePrefix:   "text/plain",
			wantLastModified: "2025-01-01T00:00:00Z",
			wantContent:      "id,name\n1,Alice\n2,Bob\n",
			wantBucket:       "corp-bucket",
			wantObject:       "catalogs/dict.csv",
			wantRangeOffset:  0,
			wantRangeLength:  resources.DefaultMaxFileSize + 1,
		},
		{
			desc: "deeply nested object path read",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name: "nested-summary",
						Type: "gcs",
					},
					URI: "gs://corp-bucket/2026/09/reports/daily/summary.json",
				},
			},
			wantSize:         15,
			wantMimePrefix:   "application/json",
			wantLastModified: "2026-09-10T15:30:00Z",
			wantContent:      `{"status":"ok"}`,
			wantBucket:       "corp-bucket",
			wantObject:       "2026/09/reports/daily/summary.json",
			wantRangeOffset:  0,
			wantRangeLength:  resources.DefaultMaxFileSize + 1,
		},
		{
			desc: "UI enabled resource read with ui:// scheme and UI metadata",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name:   "my-gcs-app",
						Type:   "gcs",
						UI:     true,
						Domain: "https://example.com",
						CSP: &resources.CSPConfig{
							ConnectDomains: []string{"https://api.example.com"},
						},
						Permissions: &resources.PermissionsConfig{
							Camera: &trueVal,
						},
					},
					URI: "ui://ui-bucket/views/dashboard.html",
				},
			},
			wantSize:         int64(len(htmlContent)),
			wantMimePrefix:   "text/html;profile=mcp-app",
			wantLastModified: "2026-09-10T15:30:00Z",
			wantUIMetadata: resources.ResourceUIMetadata{
				Domain: "https://example.com",
				CSP: &resources.CSPConfig{
					ConnectDomains: []string{"https://api.example.com"},
				},
				Permissions: map[string]any{
					"camera": map[string]any{},
				},
			},
			wantContent:     htmlContent,
			wantBucket:      "ui-bucket",
			wantObject:      "views/dashboard.html",
			wantRangeOffset: 0,
			wantRangeLength: resources.DefaultMaxFileSize + 1,
		},
		{
			desc: "range-based truncation with custom maxSize",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name: "large-log",
						Type: "gcs",
					},
					URI: "gs://corp-bucket/logs/large.txt",
				},
				MaxSize: &limit20,
			},
			wantSize:         20,
			wantMimePrefix:   "text/plain",
			wantLastModified: "2026-09-10T15:30:00Z",
			wantContent:      strings.Repeat("A", 20) + "\n\n...[TRUNCATED BY SERVER: Payload exceeded 20 byte safety limit]...",
			wantBucket:       "corp-bucket",
			wantObject:       "logs/large.txt",
			wantRangeOffset:  0,
			wantRangeLength:  21,
		},
		{
			desc: "multi-byte UTF-8 rune split at maxSize boundary is safely truncated",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name: "utf8-log",
						Type: "gcs",
					},
					URI: "gs://corp-bucket/logs/utf8_boundary.txt",
				},
				MaxSize: &limit3, // Cuts 3-byte '€' in "a€b"
			},
			wantSize:         3,
			wantMimePrefix:   "text/plain",
			wantLastModified: "2026-09-10T15:30:00Z",
			wantContent:      "a\n\n...[TRUNCATED BY SERVER: Payload exceeded 3 byte safety limit]...",
			wantBucket:       "corp-bucket",
			wantObject:       "logs/utf8_boundary.txt",
			wantRangeOffset:  0,
			wantRangeLength:  4,
		},
		{
			desc: "empty 0-byte object succeeds",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name: "empty-log",
						Type: "gcs",
					},
					URI: "gs://corp-bucket/logs/empty.txt",
				},
			},
			wantSize:         0,
			wantMimePrefix:   "text/plain",
			wantLastModified: "2026-09-10T15:30:00Z",
			wantContent:      "",
			wantBucket:       "corp-bucket",
			wantObject:       "logs/empty.txt",
			wantRangeOffset:  0,
			wantRangeLength:  resources.DefaultMaxFileSize + 1,
		},
		{
			desc: "binary payload rejected with ErrBinaryContent",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name: "binary-log",
						Type: "gcs",
					},
					URI: "gs://corp-bucket/logs/binary_disguised.txt",
				},
			},
			wantSize:         5,
			wantMimePrefix:   "text/plain",
			wantLastModified: "2026-09-10T15:30:00Z",
			wantReadErrIs:    cloudstoragecommon.ErrBinaryContent,
		},
		{
			desc: "binary payload exceeding maxSize rejected with ErrBinaryContent",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name: "large-binary-log",
						Type: "gcs",
					},
					URI: "gs://corp-bucket/logs/large_binary.txt",
				},
				MaxSize: &limit5,
			},
			wantSize:         5,
			wantMimePrefix:   "text/plain",
			wantLastModified: "2026-09-10T15:30:00Z",
			wantReadErrIs:    cloudstoragecommon.ErrBinaryContent,
		},
		{
			desc: "non-existent object fails at Initialize with fs.ErrNotExist",
			cfg: &gcs.Config{
				ResourceConfigBase: resources.ResourceConfigBase{
					ConfigBase: resources.ConfigBase{
						Name: "missing",
						Type: "gcs",
					},
					URI: "gs://corp-bucket/logs/does_not_exist.txt",
				},
			},
			wantInitErrIs: fs.ErrNotExist,
		},
	}

	ctx := context.Background()
	for _, tc := range tcs {
		t.Run(tc.desc, func(t *testing.T) {
			tc.cfg.SetDefaults()
			if err := tc.cfg.Validate(); err != nil {
				t.Fatalf("Validate failed: %v", err)
			}

			res, err := tc.cfg.Initialize(ctx)
			if tc.wantInitErrIs != nil {
				if err == nil || !errors.Is(err, tc.wantInitErrIs) {
					t.Fatalf("Initialize error = %v, want errors.Is(%v)", err, tc.wantInitErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("Initialize failed: %v", err)
			}

			if got := *res.GetSize(); got != tc.wantSize {
				t.Errorf("GetSize() = %d, want %d", got, tc.wantSize)
			}
			if got := res.GetMimeType(); !strings.HasPrefix(got, tc.wantMimePrefix) {
				t.Errorf("GetMimeType() = %q, want prefix %q", got, tc.wantMimePrefix)
			}
			if got := res.GetAnnotations().LastModified; got != tc.wantLastModified {
				t.Errorf("LastModified = %q, want %q", got, tc.wantLastModified)
			}
			if diff := cmp.Diff(tc.wantUIMetadata, res.GetResourceUIMetadata()); diff != "" {
				t.Errorf("GetResourceUIMetadata() mismatch (-want +got):\n%s", diff)
			}
			if res.ToConfig() == nil {
				t.Errorf("expected non-nil ToConfig()")
			}

			content, err := res.Read(ctx, nil)
			if tc.wantReadErrIs != nil {
				if err == nil || !errors.Is(err, tc.wantReadErrIs) {
					t.Fatalf("Read error = %v, want errors.Is(%v)", err, tc.wantReadErrIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("Read failed: %v", err)
			}

			strContent := content.(string)
			if !utf8.ValidString(strContent) {
				t.Errorf("expected valid UTF-8 string, got %q", strContent)
			}
			if strContent != tc.wantContent {
				t.Errorf("Read() content = %q, want %q", strContent, tc.wantContent)
			}
			if mc.gotBucket != tc.wantBucket || mc.gotObject != tc.wantObject {
				t.Errorf("forwarded bucket/object = %q/%q, want %q/%q", mc.gotBucket, mc.gotObject, tc.wantBucket, tc.wantObject)
			}
			if mc.gotOffset != tc.wantRangeOffset || mc.gotLength != tc.wantRangeLength {
				t.Errorf("NewRangeReader offset/length = %d/%d, want %d/%d", mc.gotOffset, mc.gotLength, tc.wantRangeOffset, tc.wantRangeLength)
			}
		})
	}
}
