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

package gcs

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"cloud.google.com/go/storage"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragecommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

const (
	ResourceType       = "gcs"
	DefaultMaxFileSize = resources.DefaultMaxFileSize // 5MB
	MaxAllowedFileSize = resources.MaxAllowedFileSize // 1GB
)

func init() {
	if !resources.Register(ResourceType, newConfig) {
		panic(fmt.Sprintf("resource type %q already registered", ResourceType))
	}
}

// Client defines the Cloud Storage operations used by GCS resources and templates.
type Client interface {
	GetObjectMetadata(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error)
	NewRangeReader(ctx context.Context, bucket, object string, offset, length int64) (io.ReadCloser, error)
}

type storageClient struct {
	client *storage.Client
}

func (s *storageClient) GetObjectMetadata(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
	return s.client.Bucket(bucket).Object(object).Attrs(ctx)
}

func (s *storageClient) NewRangeReader(ctx context.Context, bucket, object string, offset, length int64) (io.ReadCloser, error) {
	return s.client.Bucket(bucket).Object(object).NewRangeReader(ctx, offset, length)
}

var (
	clientMu     sync.Mutex
	sharedClient Client
)

// getSharedClient returns the singleton GCS Client initialized with ADC.
func getSharedClient(ctx context.Context) (Client, error) {
	clientMu.Lock()
	defer clientMu.Unlock()
	if sharedClient != nil {
		return sharedClient, nil
	}
	var opts []option.ClientOption
	if userAgent, err := util.UserAgentFromContext(ctx); err == nil && userAgent != "" {
		opts = append(opts, option.WithUserAgent(userAgent))
	}
	rawClient, err := storage.NewClient(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("unable to create storage.NewClient: %w", err)
	}
	sharedClient = &storageClient{client: rawClient}
	return sharedClient, nil
}

// SetClientForTest overrides the shared GCS Client for unit tests and returns a cleanup function.
func SetClientForTest(client Client) func() {
	clientMu.Lock()
	prev := sharedClient
	sharedClient = client
	clientMu.Unlock()
	return func() {
		clientMu.Lock()
		sharedClient = prev
		clientMu.Unlock()
	}
}

// newConfig creates and decodes a new static GCS resource config.
func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceConfig, error) {
	cfg := &Config{
		ResourceConfigBase: resources.ResourceConfigBase{
			ConfigBase: resources.ConfigBase{
				Name: name,
				Type: ResourceType,
			},
		},
	}
	if err := decoder.DecodeContext(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Config represents the uninitialized configuration for a static GCS resource.
type Config struct {
	resources.ResourceConfigBase `yaml:",inline"`
	MaxSize                      *int64 `yaml:"maxSize,omitempty"`
}

var _ resources.ResourceConfig = (*Config)(nil)
var _ resources.Resource = (*GCSResource)(nil)

// ResourceConfigType returns the resource type identifier.
func (c *Config) ResourceConfigType() string {
	return ResourceType
}

// SetDefaults applies system defaults for unspecified optional fields.
func (c *Config) SetDefaults() {
	c.ResourceConfigBase.SetDefaults()
	if c.MaxSize == nil {
		limit := int64(DefaultMaxFileSize)
		c.MaxSize = &limit
	}
	if c.MimeType == "" {
		if parsed, err := url.Parse(c.URI); err == nil && parsed.Path != "" {
			c.MimeType = resources.InferMimeType(parsed.Path)
		} else {
			c.MimeType = "text/plain"
		}
	}
}

// parseGCSURI parses and validates a gs://bucket/path/to/file (or ui://bucket/path/to/file when ui is true) URI.
func parseGCSURI(rawURI, name string, isUI bool) (bucket, objectPath string, err error) {
	parsed, err := url.Parse(rawURI)
	if err != nil {
		return "", "", fmt.Errorf("invalid URI %q for gcs resource %q: %w", rawURI, name, err)
	}
	scheme := strings.ToLower(parsed.Scheme)
	if isUI {
		if scheme != "ui" {
			return "", "", fmt.Errorf("invalid scheme for UI gcs resource %q: must be 'ui'", name)
		}
	} else if scheme != "gs" {
		return "", "", fmt.Errorf("invalid scheme for gcs resource %q: must be 'gs'", name)
	}
	bucket = strings.ToLower(parsed.Host)
	if bucket == "" {
		return "", "", fmt.Errorf("missing bucket in URI %q for gcs resource %q", rawURI, name)
	}

	// parsed.Path contains the full remaining path after the bucket, including any
	// nested directory prefixes and the filename (e.g. "/dir/subdir/file.csv").
	objectPath = strings.TrimPrefix(parsed.Path, "/")
	if objectPath == "" || strings.HasSuffix(objectPath, "/") {
		return "", "", fmt.Errorf("invalid object path in URI %q for gcs resource %q: must point to a file object", rawURI, name)
	}

	if resources.ContainsTraversal(objectPath) {
		return "", "", fmt.Errorf("security violation: object path %q in URI %q contains backward traversal components (..)", objectPath, rawURI)
	}

	if err := resources.ValidateExtension(objectPath); err != nil {
		return "", "", fmt.Errorf("invalid extension for gcs resource %q: %w", name, err)
	}

	return bucket, objectPath, nil
}

// Validate performs specific validation for a static GCS resource configuration.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.URI) == "" {
		return fmt.Errorf("missing required 'uri' field for gcs resource %q", c.Name)
	}

	if err := c.ResourceConfigBase.Validate(); err != nil {
		return err
	}

	if _, _, err := parseGCSURI(c.URI, c.Name, c.UI); err != nil {
		return err
	}

	if err := resources.ValidateMaxSize(c.MaxSize, "gcs resource", c.Name); err != nil {
		return err
	}
	return nil
}

// Initialize validates the configuration, fetches GCS object attributes, and initializes the GCSResource.
func (c *Config) Initialize(ctx context.Context) (resources.Resource, error) {
	bucket, objectPath, err := parseGCSURI(c.URI, c.Name, c.UI)
	if err != nil {
		return nil, err
	}

	client, err := getSharedClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCS client for resource %q: %w", c.Name, err)
	}

	attrs, err := client.GetObjectMetadata(ctx, bucket, objectPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get attributes for object %q in bucket %q for resource %q: %w", objectPath, bucket, c.Name, wrapGCSError(err))
	}

	if c.Annotations == nil {
		c.Annotations = &resources.ResourceAnnotations{}
	}
	if c.Annotations.LastModified == "" && !attrs.Updated.IsZero() {
		c.Annotations.LastModified = attrs.Updated.UTC().Format(time.RFC3339)
	}

	size := attrs.Size
	if size > *c.MaxSize {
		size = *c.MaxSize
	}

	return &GCSResource{
		Config:     *c,
		client:     client,
		bucket:     bucket,
		objectPath: objectPath,
		size:       size,
		attrs:      attrs,
	}, nil
}

// GCSResource represents the initialized static GCS resource.
type GCSResource struct {
	Config
	// Cloud Storage client
	client Client
	// Parsed GCS Bucket
	bucket string
	// Parsed GCS Object Path
	objectPath string
	// Size of content
	size int64
	// attrs contains the object's GCS metadata (ContentType, Updated timestamp, etc.) fetched during initialization.
	attrs *storage.ObjectAttrs
}

// GetSize returns the size of the GCS resource (capped at MaxSize).
func (r *GCSResource) GetSize() *int64 {
	return &r.size
}

// GetAnnotations returns the resource annotations, including LastModified from GCS metadata if not explicitly set.
func (r *GCSResource) GetAnnotations() *resources.ResourceAnnotations {
	var ret resources.ResourceAnnotations
	if r.Annotations != nil {
		ret = *r.Annotations
	}
	if ret.LastModified == "" && r.attrs != nil && !r.attrs.Updated.IsZero() {
		ret.LastModified = r.attrs.Updated.UTC().Format(time.RFC3339)
	}
	return &ret
}

// ToConfig returns the original configuration for this resource.
func (r *GCSResource) ToConfig() resources.ResourceConfig {
	return &r.Config
}

// Read retrieves the GCS object content using a range reader capped at MaxSize.
func (r *GCSResource) Read(ctx context.Context, params map[string]any) (any, error) {
	if err := resources.ValidateExtension(r.objectPath); err != nil {
		return nil, fmt.Errorf("security violation: configured file extension not allowed for resource %q: %w", r.Name, err)
	}
	return readGCSObject(ctx, r.client, r.bucket, r.objectPath, *r.MaxSize)
}

// readGCSObject reads up to limit+1 bytes from a GCS object using NewRangeReader,
// validates that the payload is valid UTF-8 text, and truncates at limit if exceeded.
func readGCSObject(ctx context.Context, client Client, bucket, objectPath string, limit int64) (string, error) {
	reader, err := client.NewRangeReader(ctx, bucket, objectPath, 0, limit+1)
	if err != nil {
		if isRangeNotSatisfiable(err) {
			return "", nil
		}
		return "", fmt.Errorf("failed to open object %q in bucket %q: %w", objectPath, bucket, wrapGCSError(err))
	}
	defer reader.Close()

	data, err := io.ReadAll(reader)
	if err != nil {
		return "", fmt.Errorf("failed to read object %q in bucket %q: %w", objectPath, bucket, wrapGCSError(err))
	}

	// If the payload exceeds limit, trim any incomplete trailing multi-byte rune
	// (at most 3 bytes) at the cut boundary before validating UTF-8 so valid
	// multi-byte characters split at the limit are not misclassified as binary content.
	checkSlice := data
	if int64(len(data)) > limit {
		checkSlice = data[:limit]
		for i := 0; i < 3 && len(checkSlice) > 0; i++ {
			r, size := utf8.DecodeLastRune(checkSlice)
			if r == utf8.RuneError && size == 1 {
				checkSlice = checkSlice[:len(checkSlice)-1]
			} else {
				break
			}
		}
	}

	if !utf8.Valid(checkSlice) {
		return "", fmt.Errorf("object %q in bucket %q: %w", objectPath, bucket,
			cloudstoragecommon.ProcessGCSError(cloudstoragecommon.ErrBinaryContent))
	}

	return resources.TruncateUTF8(data, limit), nil
}

func isRangeNotSatisfiable(err error) bool {
	var gErr *googleapi.Error
	if errors.As(err, &gErr) && gErr.Code == http.StatusRequestedRangeNotSatisfiable {
		return true
	}
	return false
}

func wrapGCSError(err error) error {
	if err == nil {
		return nil
	}
	processed := cloudstoragecommon.ProcessGCSError(err)
	if errors.Is(err, storage.ErrObjectNotExist) || errors.Is(err, storage.ErrBucketNotExist) {
		return fmt.Errorf("%w: %w", processed, fs.ErrNotExist)
	}
	var gErr *googleapi.Error
	if errors.As(err, &gErr) && gErr.Code == http.StatusNotFound {
		return fmt.Errorf("%w: %w", processed, fs.ErrNotExist)
	}
	return processed
}
