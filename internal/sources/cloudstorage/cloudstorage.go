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
	"errors"
	"fmt"
	"io"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"cloud.google.com/go/storage"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragecommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

const SourceType string = "cloud-storage"

// defaultMaxReadBytes caps the payload ReadObject will return per call,
// protecting the server from OOM and keeping LLM contexts manageable. Objects
// or ranges exceeding this are rejected with ErrReadSizeLimitExceeded.
const defaultMaxReadBytes int64 = 8 << 20 // 8 MiB

// validate interface
var _ sources.SourceConfig = Config{}

func init() {
	if !sources.Register(SourceType, newConfig) {
		panic(fmt.Sprintf("source type %q already registered", SourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (sources.SourceConfig, error) {
	actual := Config{Name: name}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Config struct {
	Name              string   `yaml:"name" validate:"required"`
	Type              string   `yaml:"type" validate:"required"`
	Project           string   `yaml:"project" validate:"required"`
	AllowedBuckets    []string `yaml:"allowedBuckets,omitempty"`
	AllowedLocalRoots []string `yaml:"allowedLocalRoots,omitempty"`
}

func (r Config) SourceConfigType() string {
	return SourceType
}

func (r Config) Initialize(ctx context.Context, tracer trace.Tracer, deferConnect bool) (sources.Source, error) {
	s := r.newSource(ctx, tracer)
	if deferConnect {
		return s, nil
	}
	if _, err := s.client(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (r Config) newSource(ctx context.Context, tracer trace.Tracer) *Source {
	return &Source{
		Config: r,
		conn:   sources.NewConnectOnce[*storage.Client](ctx, r.Name, SourceType, tracer),
	}
}

var _ sources.Source = &Source{}

type Source struct {
	Config
	conn *sources.ConnectOnce[*storage.Client]
}

func (s *Source) client(ctx context.Context) (*storage.Client, error) {
	return s.conn.Do(ctx, func(ctx context.Context) (*storage.Client, error) {
		client, err := initGCSClient(ctx, s.Project)
		if err != nil {
			return nil, fmt.Errorf("unable to create client: %w", err)
		}
		return client, nil
	})
}

func (s *Source) validateBucket(bucket string) error {
	if len(s.AllowedBuckets) == 0 {
		return nil
	}
	for _, b := range s.AllowedBuckets {
		if b == bucket {
			return nil
		}
	}
	return fmt.Errorf("bucket %q is not allowed by source %q configuration", bucket, s.Name)
}

// validateLocalPath enforces allowedLocalRoots. The path must sit under an
// allowed root both as written and after symlinks are resolved: the first check
// keeps the rejection message tied to what the caller actually asked for, and
// the second is what makes the root a real boundary, since a symlink planted
// under a root can otherwise point anywhere on the filesystem.
//
// The resolved comparison uses resolved roots as well, so an allowed root that
// is itself reached through a symlink (/tmp on macOS, a symlinked workspace)
// still matches.
func (s *Source) validateLocalPath(p string) error {
	clean, err := cloudstoragecommon.ValidateLocalPath(p)
	if err != nil {
		return err
	}
	if len(s.AllowedLocalRoots) == 0 {
		return nil
	}

	nameMatched := false
	for _, root := range s.AllowedLocalRoots {
		if isUnderRoot(clean, root) {
			nameMatched = true
			break
		}
	}
	if !nameMatched {
		return fmt.Errorf("local path %q is not under any allowed local roots for source %q", p, s.Name)
	}

	resolved, err := cloudstoragecommon.ResolveSymlinks(clean)
	if err != nil {
		return fmt.Errorf("local path %q cannot be resolved for source %q: %w", p, s.Name, err)
	}
	for _, root := range s.AllowedLocalRoots {
		// A root we cannot resolve authorizes nothing; skip it rather than
		// falling back to the name-level match we already passed.
		resolvedRoot, err := cloudstoragecommon.ResolveSymlinks(root)
		if err != nil {
			continue
		}
		if isUnderRoot(resolved, resolvedRoot) {
			return nil
		}
	}
	return fmt.Errorf("local path %q resolves through a symbolic link to a target outside the allowed local roots for source %q", p, s.Name)
}

func isUnderRoot(target, root string) bool {
	target = filepath.Clean(target)
	root = filepath.Clean(root)
	if target == root {
		return true
	}
	if root == string(filepath.Separator) {
		return true
	}
	sep := string(filepath.Separator)
	if !strings.HasSuffix(root, sep) {
		root += sep
	}
	return strings.HasPrefix(target, root)
}

func (s *Source) IsReadOnly() bool {
	return false
}

func (s *Source) SourceType() string {
	return SourceType
}

func (s *Source) ToConfig() sources.SourceConfig {
	return s.Config
}

// StorageClient reports the client once connected.
func (s *Source) StorageClient() *storage.Client {
	client, _ := s.conn.Get()
	return client
}

func (s *Source) GetProjectID() string {
	return s.Project
}

// ListObjects lists objects in a bucket with optional prefix and delimiter filtering.
// maxResults == 0 means return up to one page as returned by the GCS API. A non-empty
// pageToken resumes listing from a prior call. The returned map contains "objects"
// (raw *storage.ObjectAttrs entries as returned by the GCS client), "prefixes"
// (common prefixes when a delimiter is set), and "nextPageToken" (empty when
// there are no more results).
func (s *Source) ListObjects(ctx context.Context, bucket, prefix, delimiter string, maxResults int, pageToken string) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	it := client.Bucket(bucket).Objects(ctx, &storage.Query{
		Prefix:    prefix,
		Delimiter: delimiter,
	})
	// iterator.NewPager errors on pageSize <= 0; the tool layer already rejects
	// values above the GCS per-page cap of 1000, so any positive value is safe.
	ps := maxResults
	if ps <= 0 {
		ps = 1000
	}
	pager := iterator.NewPager(it, ps, pageToken)

	var attrsPage []*storage.ObjectAttrs
	nextPageToken, err := pager.NextPage(&attrsPage)
	if err != nil {
		return nil, fmt.Errorf("failed to list objects in bucket %q: %w", bucket, err)
	}

	objects := make([]*storage.ObjectAttrs, 0, len(attrsPage))
	prefixes := make([]string, 0)
	for _, attrs := range attrsPage {
		if attrs.Prefix != "" {
			prefixes = append(prefixes, attrs.Prefix)
			continue
		}
		objects = append(objects, attrs)
	}

	return map[string]any{
		"objects":       objects,
		"prefixes":      prefixes,
		"nextPageToken": nextPageToken,
	}, nil
}

// ReadObject fetches an object's bytes and returns a map with the UTF-8
// content, its content type, and the number of bytes read. offset and length
// follow storage.ObjectHandle.NewRangeReader semantics: length == -1 means
// "read to end of object"; a negative offset means "suffix from end" (in
// which case length must be -1). Reads larger than defaultMaxReadBytes are
// rejected with cloudstoragecommon.ErrReadSizeLimitExceeded so the caller can
// narrow the range. Objects whose bytes are not valid UTF-8 are rejected
// with cloudstoragecommon.ErrBinaryContent.
//
// TODO: MCP tool results only carry text today, so we gate this tool on
// utf8.Valid. When the toolbox supports non-text MCP content (embedded
// resources, images, blobs), expand this to detect content type and return
// binary payloads natively.
func (s *Source) ReadObject(ctx context.Context, bucket, object string, offset, length int64) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	reader, err := client.Bucket(bucket).Object(object).NewRangeReader(ctx, offset, length)
	if err != nil {
		return nil, fmt.Errorf("failed to open object %q in bucket %q: %w", object, bucket, err)
	}
	defer reader.Close()

	if remain := reader.Remain(); remain > defaultMaxReadBytes {
		return nil, fmt.Errorf("object %q: %d bytes exceeds %d byte limit: %w",
			object, remain, defaultMaxReadBytes,
			cloudstoragecommon.ErrReadSizeLimitExceeded)
	}

	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read object %q in bucket %q: %w", object, bucket, err)
	}

	if !utf8.Valid(data) {
		return nil, fmt.Errorf("object %q in bucket %q: %w", object, bucket,
			cloudstoragecommon.ErrBinaryContent)
	}

	return map[string]any{
		"content":     string(data),
		"contentType": reader.Attrs.ContentType,
		"size":        len(data),
	}, nil
}

// ListBuckets lists buckets in a project. When project is empty, the source's
// configured project is used. maxResults == 0 returns up to the GCS per-page
// default (1000). A non-empty pageToken resumes listing. The returned map
// contains "buckets" ([]*storage.BucketAttrs) and "nextPageToken" (empty when
// there are no more results).
func (s *Source) ListBuckets(ctx context.Context, project, prefix string, maxResults int, pageToken string) (map[string]any, error) {
	if project == "" {
		project = s.Project
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	it := client.Buckets(ctx, project)
	if prefix != "" {
		it.Prefix = prefix
	}
	ps := maxResults
	if ps <= 0 {
		ps = 1000
	}
	pager := iterator.NewPager(it, ps, pageToken)

	var buckets []*storage.BucketAttrs
	nextPageToken, err := pager.NextPage(&buckets)
	if err != nil {
		return nil, fmt.Errorf("failed to list buckets in project %q: %w", project, err)
	}
	return map[string]any{
		"buckets":       buckets,
		"nextPageToken": nextPageToken,
	}, nil
}

// CreateBucket creates a Cloud Storage bucket and returns its freshly-read
// metadata. When project is empty, the source's configured project is used.
// When location is empty, Cloud Storage applies its service default.
func (s *Source) CreateBucket(ctx context.Context, bucket, project, location string, uniformBucketLevelAccess bool) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	if project == "" {
		project = s.Project
	}
	attrs := &storage.BucketAttrs{Location: location}
	if uniformBucketLevelAccess {
		attrs.UniformBucketLevelAccess = storage.UniformBucketLevelAccess{Enabled: true}
	}

	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	bkt := client.Bucket(bucket)
	if err := bkt.Create(ctx, project, attrs); err != nil {
		return nil, fmt.Errorf("failed to create bucket %q in project %q: %w", bucket, project, err)
	}

	createdAttrs, err := bkt.Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for created bucket %q: %w", bucket, err)
	}
	return map[string]any{
		"bucket":   bucket,
		"created":  true,
		"metadata": createdAttrs,
	}, nil
}

// GetBucketMetadata returns raw bucket metadata from the Cloud Storage client.
func (s *Source) GetBucketMetadata(ctx context.Context, bucket string) (*storage.BucketAttrs, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	attrs, err := client.Bucket(bucket).Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for bucket %q: %w", bucket, err)
	}
	return attrs, nil
}

// GetBucketIAMPolicy returns bucket IAM bindings in a stable, agent-friendly
// shape while preserving conditional bindings when present.
func (s *Source) GetBucketIAMPolicy(ctx context.Context, bucket string) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	policy, err := client.Bucket(bucket).IAM().Policy(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get IAM policy for bucket %q: %w", bucket, err)
	}

	bindings := make([]map[string]any, 0)
	if policy != nil && policy.InternalProto != nil {
		for _, binding := range policy.InternalProto.Bindings {
			members := append([]string(nil), binding.Members...)
			sort.Strings(members)

			out := map[string]any{
				"role":    binding.Role,
				"members": members,
			}
			if binding.Condition != nil {
				out["condition"] = map[string]any{
					"title":       binding.Condition.Title,
					"description": binding.Condition.Description,
					"expression":  binding.Condition.Expression,
				}
			}
			bindings = append(bindings, out)
		}
	}
	sort.Slice(bindings, func(i, j int) bool {
		return fmt.Sprint(bindings[i]["role"]) < fmt.Sprint(bindings[j]["role"])
	})

	return map[string]any{
		"bucket":   bucket,
		"bindings": bindings,
	}, nil
}

// GetObjectMetadata returns the raw *storage.ObjectAttrs for an object, giving
// callers the full field set the GCS client exposes (name, size, contentType,
// hashes, timestamps, user metadata, etc.) without a curated subset.
func (s *Source) GetObjectMetadata(ctx context.Context, bucket, object string) (*storage.ObjectAttrs, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	attrs, err := client.Bucket(bucket).Object(object).Attrs(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get metadata for object %q in bucket %q: %w", object, bucket, err)
	}
	return attrs, nil
}

// DownloadObject streams a GCS object to destination on the local filesystem.
// Unlike ReadObject there is no size cap and no UTF-8 check — the bytes go to
// disk, not into the LLM context, so binary payloads are fine. When overwrite
// is false, a pre-existing destination returns cloudstoragecommon.ErrDestinationExists
// (mapped to AgentError so the caller can retry with overwrite=true).
func (s *Source) DownloadObject(ctx context.Context, bucket, object, destination string, overwrite bool) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	if err := s.validateLocalPath(destination); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	reader, err := client.Bucket(bucket).Object(object).NewReader(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to open object %q in bucket %q: %w", object, bucket, err)
	}
	defer reader.Close()

	flags := os.O_WRONLY | os.O_CREATE
	if overwrite {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_EXCL
	}
	f, err := os.OpenFile(destination, flags, 0o644)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("destination %q: %w", destination, cloudstoragecommon.ErrDestinationExists)
		}
		return nil, fmt.Errorf("failed to open destination %q: %w", destination, err)
	}

	n, err := io.Copy(f, reader)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("failed to write object %q to %q: %w", object, destination, err)
	}
	if err := f.Close(); err != nil {
		return nil, fmt.Errorf("failed to close destination %q: %w", destination, err)
	}

	return map[string]any{
		"destination": destination,
		"bytes":       n,
		"contentType": reader.Attrs.ContentType,
	}, nil
}

// UploadObject streams a local file into a GCS object. When contentType is
// empty, mime.TypeByExtension is consulted; if inference still fails the
// writer's ContentType is left unset so GCS content-sniffs the first 512 bytes.
// The returned contentType is the post-Close value from w.Attrs(), i.e. what
// GCS actually recorded.
func (s *Source) UploadObject(ctx context.Context, bucket, object, source, contentType string) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	if err := s.validateLocalPath(source); err != nil {
		return nil, err
	}
	f, err := os.Open(source)
	if err != nil {
		return nil, fmt.Errorf("failed to open source %q: %w", source, err)
	}
	defer f.Close()

	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(source))
	}

	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	w := client.Bucket(bucket).Object(object).NewWriter(ctx)
	if contentType != "" {
		w.ContentType = contentType
	}

	n, err := io.Copy(w, f)
	if err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("failed to copy %q to object %q in bucket %q: %w", source, object, bucket, err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize upload of %q to %q/%q: %w", source, bucket, object, err)
	}

	attrs := w.Attrs()
	finalContentType := ""
	if attrs != nil {
		finalContentType = attrs.ContentType
	}
	return map[string]any{
		"bucket":      bucket,
		"object":      object,
		"bytes":       n,
		"contentType": finalContentType,
	}, nil
}

// WriteObject writes text content directly into a GCS object. When contentType
// is empty, the writer's ContentType is left unset so Cloud Storage detects it
// from the first 512 bytes. The returned contentType is the post-Close value
// from w.Attrs(), i.e. what GCS actually recorded.
func (s *Source) WriteObject(ctx context.Context, bucket, object, content, contentType string) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	w := client.Bucket(bucket).Object(object).NewWriter(ctx)
	if contentType != "" {
		w.ContentType = contentType
	}

	n, err := io.WriteString(w, content)
	if err != nil {
		_ = w.Close()
		return nil, fmt.Errorf("failed to write content to object %q in bucket %q: %w", object, bucket, err)
	}
	if err := w.Close(); err != nil {
		return nil, fmt.Errorf("failed to finalize write to %q/%q: %w", bucket, object, err)
	}

	attrs := w.Attrs()
	finalContentType := ""
	if attrs != nil {
		finalContentType = attrs.ContentType
	}
	return map[string]any{
		"bucket":      bucket,
		"object":      object,
		"bytes":       n,
		"contentType": finalContentType,
	}, nil
}

// CopyObject copies an object to a destination object. The destination may be
// in the same bucket or a different bucket. Existing destination objects are
// replaced, matching Cloud Storage's copy semantics without preconditions.
func (s *Source) CopyObject(ctx context.Context, sourceBucket, sourceObject, destinationBucket, destinationObject string) (map[string]any, error) {
	if err := s.validateBucket(sourceBucket); err != nil {
		return nil, err
	}
	if err := s.validateBucket(destinationBucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	src := client.Bucket(sourceBucket).Object(sourceObject)
	dst := client.Bucket(destinationBucket).Object(destinationObject)

	attrs, err := dst.CopierFrom(src).Run(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to copy %q/%q to %q/%q: %w", sourceBucket, sourceObject, destinationBucket, destinationObject, err)
	}

	return map[string]any{
		"sourceBucket":      sourceBucket,
		"sourceObject":      sourceObject,
		"destinationBucket": destinationBucket,
		"destinationObject": destinationObject,
		"bytes":             attrs.Size,
		"contentType":       attrs.ContentType,
	}, nil
}

// MoveObject atomically renames or moves an object within the same bucket using
// Cloud Storage's native move API. Cross-bucket moves should be modeled as
// CopyObject followed by DeleteObject.
func (s *Source) MoveObject(ctx context.Context, bucket, sourceObject, destinationObject string) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	attrs, err := client.Bucket(bucket).Object(sourceObject).Move(ctx, storage.MoveObjectDestination{Object: destinationObject})
	if err != nil {
		return nil, fmt.Errorf("failed to move %q to %q in bucket %q: %w", sourceObject, destinationObject, bucket, err)
	}

	return map[string]any{
		"bucket":            bucket,
		"sourceObject":      sourceObject,
		"destinationObject": destinationObject,
		"bytes":             attrs.Size,
		"contentType":       attrs.ContentType,
	}, nil
}

// DeleteObject deletes a GCS object.
func (s *Source) DeleteObject(ctx context.Context, bucket, object string) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	if err := client.Bucket(bucket).Object(object).Delete(ctx); err != nil {
		return nil, fmt.Errorf("failed to delete object %q in bucket %q: %w", object, bucket, err)
	}

	return map[string]any{
		"bucket":  bucket,
		"object":  object,
		"deleted": true,
	}, nil
}

// DeleteBucket deletes an empty Cloud Storage bucket.
func (s *Source) DeleteBucket(ctx context.Context, bucket string) (map[string]any, error) {
	if err := s.validateBucket(bucket); err != nil {
		return nil, err
	}
	client, err := s.client(ctx)
	if err != nil {
		return nil, err
	}
	if err := client.Bucket(bucket).Delete(ctx); err != nil {
		return nil, fmt.Errorf("failed to delete bucket %q: %w", bucket, err)
	}

	return map[string]any{
		"bucket":  bucket,
		"deleted": true,
	}, nil
}

func initGCSClient(ctx context.Context, project string) (*storage.Client, error) {
	userAgent, err := util.UserAgentFromContext(ctx)
	if err != nil {
		return nil, err
	}

	client, err := storage.NewClient(ctx, option.WithUserAgent(userAgent))
	if err != nil {
		return nil, fmt.Errorf("unable to create storage.NewClient for project %q: %w", project, err)
	}
	return client, nil
}
