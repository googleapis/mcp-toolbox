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

package file

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"mime"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/resources"
)

const (
	defaultMaxFileSize = 5 * 1024 * 1024 // 5MB
	resourceType       = "file"
)

func init() {
	if !resources.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("resource type %q already registered", resourceType))
	}
	if !resources.RegisterTemplate(resourceType, newTemplateConfig) {
		panic(fmt.Sprintf("resource template type %q already registered", resourceType))
	}
}

// newConfig creates and decodes a new file resource config.
func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceConfig, error) {
	cfg := &Config{
		ResourceConfigBase: resources.ResourceConfigBase{
			ConfigBase: resources.ConfigBase{
				Name: name,
				Type: resourceType,
			},
		},
		baseDir: resources.GetBaseDirFromContext(ctx),
	}
	if err := decoder.DecodeContext(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// newTemplateConfig creates and decodes a new file template config.
func newTemplateConfig(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceTemplateConfig, error) {
	cfg := &TemplateConfig{
		ResourceTemplateConfigBase: resources.ResourceTemplateConfigBase{
			ConfigBase: resources.ConfigBase{
				Name: name,
				Type: resourceType,
			},
		},
		baseDir: resources.GetBaseDirFromContext(ctx),
	}
	if err := decoder.DecodeContext(ctx, cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Config represents the configuration for a file resource.
type Config struct {
	resources.ResourceConfigBase `yaml:",inline"`
	Path                         string `yaml:"path" validate:"required"`
	MaxSize                      *int64 `yaml:"maxSize,omitempty"`
	baseDir                      string
}

var _ resources.ResourceConfig = &Config{}
var _ resources.Resource = &FileResource{}

// ResourceConfigType returns the resource type identifier.
func (c *Config) ResourceConfigType() string {
	return resourceType
}

var allowedExts = map[string]bool{
	".txt": true, ".md": true, ".csv": true, ".json": true,
	".yaml": true, ".yml": true, ".xml": true, ".sql": true,
	".html": true, ".htm": true, ".js": true, ".css": true, ".svg": true,
}

// validateExtension checks if a file extension is allowed.
func validateExtension(path string) error {
	ext := strings.ToLower(filepath.Ext(path))
	if !allowedExts[ext] {
		return fmt.Errorf("file extension %q is not allowed", ext)
	}
	return nil
}

// Validate performs specific validation including URI scheme and file size limits.
func (c *Config) Validate() error {
	if err := c.ResourceConfigBase.Validate(); err != nil {
		return err
	}
	parsed, _ := url.Parse(c.URI)
	if !c.UI && parsed.Scheme != "file" {
		return fmt.Errorf("invalid scheme for file resource %q: must be 'file'", c.Name)
	}

	if c.MaxSize != nil {
		if *c.MaxSize <= 0 {
			return fmt.Errorf("file resource %q maxSize must be greater than 0", c.Name)
		} else if *c.MaxSize > 1024*1024*1024 {
			return fmt.Errorf("file resource %q maxSize cannot exceed 1GB", c.Name)
		}
	}
	return nil
}

// containsTraversal checks if any component of the path is a backward traversal
func containsTraversal(p string) bool {
	// Check for URL-encoded traversal attempts to prevent evasion
	decoded, err := url.PathUnescape(p)
	if err == nil {
		p = decoded
	}

	// Convert any backslashes to forward slashes for unified checking
	p = strings.ReplaceAll(p, "\\", "/")

	parts := strings.Split(p, "/")
	for _, part := range parts {
		if part == ".." {
			return true
		}
	}
	return false
}

// Initialize validates the configuration and initializes the file resource.
func (c *Config) Initialize(ctx context.Context) (resources.Resource, error) {
	if c.MaxSize == nil {
		limit := int64(defaultMaxFileSize)
		c.MaxSize = &limit
	}

	var absPath string
	var resolvedBaseDir string
	var isRelative bool

	if filepath.IsAbs(c.Path) {
		absPath = filepath.Clean(c.Path)
		isRelative = false
	} else {
		if !filepath.IsLocal(c.Path) {
			return nil, fmt.Errorf("relative path %q is unsafe", c.Path)
		}
		isRelative = true
		baseDir := c.baseDir
		if baseDir == "" {
			baseDir = resources.GetBaseDirFromContext(ctx)
		}
		if baseDir == "" {
			baseDir = "."
		}
		resolvedBaseDir = baseDir
		absPath = filepath.Clean(filepath.Join(baseDir, c.Path))
	}

	abs, err := filepath.Abs(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path for %q: %w", absPath, err)
	}
	absPath = abs

	if c.Annotations == nil {
		c.Annotations = &resources.ResourceAnnotations{}
	}

	if c.MimeType == "" {
		ext := strings.ToLower(filepath.Ext(absPath))
		c.MimeType = mime.TypeByExtension(ext)
		if c.MimeType == "" {
			c.MimeType = "text/plain"
		}
	}

	if isRelative && resolvedBaseDir != "" {
		absBase, err := filepath.Abs(resolvedBaseDir)
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for base directory: %w", err)
		}
		resolvedBase, err := filepath.EvalSymlinks(absBase)
		if err != nil {
			if !os.IsNotExist(err) {
				return nil, fmt.Errorf("failed to evaluate symlinks for base directory: %w", err)
			}
			resolvedBaseDir = absBase
		} else {
			resolvedBaseDir = resolvedBase
		}
	}

	// Security check for extension on the requested path
	if err := validateExtension(absPath); err != nil {
		return nil, fmt.Errorf("invalid extension for resource %q: %w", c.Name, err)
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %q (files must exist at boot time to prevent dead URIs)", absPath)
		}
		return nil, fmt.Errorf("failed to evaluate symlinks for resource %q: %w", c.Name, err)
	}
	absPath = resolvedPath

	if isRelative && resolvedBaseDir != "" {
		rel, err := filepath.Rel(resolvedBaseDir, absPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("security violation: resolved path %q escapes base directory %q", absPath, resolvedBaseDir)
		}
	}

	if err := validateExtension(absPath); err != nil {
		return nil, fmt.Errorf("invalid extension for resource %q: %w", c.Name, err)
	}

	info, err := os.Stat(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file %q for resource %q: %w", absPath, c.Name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path %q for resource %q is not a regular file (devices, pipes, sockets are blocked)", absPath, c.Name)
	}

	f, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %q (missing read permissions?): %w", absPath, err)
	}
	f.Close()

	size := info.Size()
	if size > *c.MaxSize {
		size = *c.MaxSize
	}
	return &FileResource{
		Config:          *c,
		Size:            size,
		absPath:         absPath,
		resolvedBaseDir: resolvedBaseDir,
		isRelative:      isRelative,
	}, nil
}

// FileResource handles reading content from a local file.
type FileResource struct {
	Config
	Size int64

	absPath         string
	resolvedBaseDir string
	isRelative      bool
}

// GetSize returns the configured maximum size of the file.
func (r *FileResource) GetSize() *int64 {
	if size, err := r.GetCurrentSize(); err == nil {
		return &size
	}
	return &r.Size // Fallback to the size calculated at initialization
}

// Read retrieves the file content.
func (r *FileResource) Read(ctx context.Context, params map[string]any) (any, error) {
	// Security check for extension on the resolved target
	if err := validateExtension(r.absPath); err != nil {
		return nil, fmt.Errorf("security violation: configured file extension not allowed for resource %q: %w", r.Name, err)
	}

	resolvedPath, err := filepath.EvalSymlinks(r.absPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("file not found: %q: %w", r.absPath, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("failed to evaluate symlinks for resource %q at runtime: %w", r.Name, err)
	}

	if r.isRelative && r.resolvedBaseDir != "" {
		resolvedBaseDir := r.resolvedBaseDir
		if resolved, err := filepath.EvalSymlinks(resolvedBaseDir); err == nil {
			resolvedBaseDir = resolved
		}
		rel, err := filepath.Rel(resolvedBaseDir, resolvedPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("security violation: resolved path %q escapes base directory %q at runtime", resolvedPath, resolvedBaseDir)
		}
	}

	if err := validateExtension(resolvedPath); err != nil {
		return nil, fmt.Errorf("security violation: file extension changed post-boot for resource %q: %w", r.Name, err)
	}

	statInfo, err := os.Lstat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to lstat file %q: %w", resolvedPath, err)
	}

	if statInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("security violation: TOCTOU symlink swap detected on %q", resolvedPath)
	}

	f, err := os.Open(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %q: %w", resolvedPath, err)
	}
	defer f.Close()

	openInfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat opened file %q: %w", resolvedPath, err)
	}

	if !os.SameFile(statInfo, openInfo) {
		return nil, fmt.Errorf("security violation: TOCTOU file swap detected on %q between stat and open", resolvedPath)
	}

	if !openInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("security violation: file %q was swapped with a non-regular file during read", resolvedPath)
	}

	limit := *r.MaxSize
	limitedReader := io.LimitReader(f, limit+1)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %q: %w", resolvedPath, err)
	}

	if int64(len(content)) > limit {
		truncated := content[:limit]
		for len(truncated) > 0 {
			r, size := utf8.DecodeLastRune(truncated)
			if r == utf8.RuneError && size == 1 {
				truncated = truncated[:len(truncated)-1]
			} else {
				break
			}
		}
		warning := fmt.Sprintf("\n\n...[TRUNCATED BY SERVER: Payload exceeded %d byte safety limit]...", limit)
		return string(truncated) + warning, nil
	}

	return string(content), nil
}

// GetAnnotations returns the resource annotations, dynamically computing the LastModified timestamp.
func (r *FileResource) GetAnnotations() *resources.ResourceAnnotations {
	var ret resources.ResourceAnnotations
	if r.Annotations != nil {
		ret = *r.Annotations
	}

	resolvedPath := r.absPath
	if resolved, err := filepath.EvalSymlinks(r.absPath); err == nil {
		resolvedPath = resolved
	}

	if r.isRelative && r.resolvedBaseDir != "" {
		resolvedBaseDir := r.resolvedBaseDir
		if resolved, err := filepath.EvalSymlinks(resolvedBaseDir); err == nil {
			resolvedBaseDir = resolved
		}
		rel, err := filepath.Rel(resolvedBaseDir, resolvedPath)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return &ret
		}
	}

	if info, err := os.Stat(resolvedPath); err == nil && info.Mode().IsRegular() {
		ret.LastModified = info.ModTime().Format(time.RFC3339)
	}

	return &ret
}

// ToConfig returns the original configuration for this resource.
func (r *FileResource) ToConfig() resources.ResourceConfig {
	return &r.Config
}

// GetCurrentSize returns the actual size of the file on disk.
func (r *FileResource) GetCurrentSize() (int64, error) {
	resolvedPath := r.absPath
	if resolved, err := filepath.EvalSymlinks(r.absPath); err == nil {
		resolvedPath = resolved
	}

	info, err := os.Stat(resolvedPath)
	if err != nil {
		return 0, fmt.Errorf("failed to stat file for size: %w", err)
	}
	if !info.Mode().IsRegular() {
		return 0, fmt.Errorf("not a regular file")
	}

	size := info.Size()
	if size > *r.MaxSize {
		size = *r.MaxSize
	}
	return size, nil
}

// TemplateConfig represents the configuration for a file resource template.
type TemplateConfig struct {
	resources.ResourceTemplateConfigBase `yaml:",inline"`
	AllowedPaths                         []string `yaml:"allowedPaths,omitempty"`
	MaxSize                              *int64   `yaml:"maxSize,omitempty"`
	baseDir                              string
}

var _ resources.ResourceTemplateConfig = (*TemplateConfig)(nil)

// ResourceTemplateConfigType returns the resource template type identifier.

func (c *TemplateConfig) ResourceTemplateConfigType() string {
	return resourceType
}

// Validate performs template-specific validation including URI scheme checks.
func (c *TemplateConfig) Validate() error {
	if err := c.ResourceTemplateConfigBase.Validate(); err != nil {
		return err
	}
	parsed, _ := url.Parse(strings.ReplaceAll(c.URITemplate, "{path}", "path"))
	if !c.UI && parsed.Scheme != "file" {
		return fmt.Errorf("invalid scheme for file resource template %q: must be 'file'", c.Name)
	}

	if c.MaxSize != nil {
		if *c.MaxSize <= 0 {
			return fmt.Errorf("file resource template %q maxSize must be greater than 0", c.Name)
		} else if *c.MaxSize > 1024*1024*1024 {
			return fmt.Errorf("file resource template %q maxSize cannot exceed 1GB", c.Name)
		}
	}
	return nil
}

// Initialize validates the configuration and initializes the file resource template.
func (c *TemplateConfig) Initialize(ctx context.Context) (resources.ResourceTemplate, error) {
	if c.MaxSize == nil {
		limit := int64(defaultMaxFileSize)
		c.MaxSize = &limit
	}

	// Validate and resolve allowed paths if specified
	var unresolvedAllowedPaths []string
	var resolvedAllowedPaths []string
	baseDir := c.baseDir
	if baseDir == "" {
		baseDir = resources.GetBaseDirFromContext(ctx)
	}
	if baseDir == "" {
		baseDir = "."
	}

	for _, p := range c.AllowedPaths {
		// Resolve relative allowedPaths against the config file's base directory
		if !filepath.IsAbs(p) {
			p = filepath.Join(baseDir, p)
		}

		abs, err := filepath.Abs(filepath.Clean(p))
		if err != nil {
			return nil, fmt.Errorf("failed to get absolute path for allowed path %q: %w", p, err)
		}

		unresolvedAllowedPaths = append(unresolvedAllowedPaths, abs)

		resolved, err := filepath.EvalSymlinks(abs)
		if err != nil {
			if os.IsNotExist(err) {
				// If directory doesn't exist yet, we still track its intended absolute path
				resolvedAllowedPaths = append(resolvedAllowedPaths, abs)
			} else {
				return nil, fmt.Errorf("failed to evaluate symlinks for allowed path %q: %w", abs, err)
			}
		} else {
			resolvedAllowedPaths = append(resolvedAllowedPaths, resolved)
		}
	}

	return &FileTemplate{
		TemplateConfig:         *c,
		unresolvedAllowedPaths: unresolvedAllowedPaths,
		resolvedAllowedPaths:   resolvedAllowedPaths,
	}, nil
}

// FileTemplate handles reading content from a file template URI.
type FileTemplate struct {
	TemplateConfig
	unresolvedAllowedPaths []string
	resolvedAllowedPaths   []string
}

var _ resources.ResourceTemplate = (*FileTemplate)(nil)

// Read retrieves the file content using template parameters.
func (r *FileTemplate) Read(ctx context.Context, params map[string]any) (any, error) {
	pathVal, ok := params["path"]
	if !ok {
		return nil, fmt.Errorf("missing 'path' parameter in template execution")
	}

	pathStr, ok := pathVal.(string)
	if !ok {
		return nil, fmt.Errorf("'path' parameter must be a string")
	}

	// Explicitly block backward traversal in the raw input
	if containsTraversal(pathStr) {
		return nil, fmt.Errorf("security violation: path %q contains backward traversal components (..)", pathStr)
	}

	// If the path is relative, reconstruct the full path
	if !filepath.IsAbs(pathStr) {
		uriTemplate := r.URITemplate
		if strings.Contains(uriTemplate, "{path}") {
			// Interpolate {path} back into the template to capture prefix AND suffix
			escapedPath := url.PathEscape(pathStr)
			fullURI := strings.Replace(uriTemplate, "{path}", escapedPath, 1)

			parsed, err := url.Parse(fullURI)
			if err != nil {
				return nil, fmt.Errorf("invalid URI reconstructed from template: %w", err)
			}

			base := parsed.Path

			// Handle Windows drive letter quirks exclusively on Windows:
			// If 2 slashes were used (e.g., file://C:/...), parsed.Host contains "C:".
			// If 3 slashes were used (e.g., file:///C:/...), parsed.Path contains "/C:/...".
			if runtime.GOOS == "windows" {
				if len(parsed.Host) == 2 && parsed.Host[1] == ':' && ((parsed.Host[0] >= 'a' && parsed.Host[0] <= 'z') || (parsed.Host[0] >= 'A' && parsed.Host[0] <= 'Z')) {
					base = parsed.Host + base
				} else if len(base) > 2 && base[0] == '/' && base[2] == ':' && ((base[1] >= 'a' && base[1] <= 'z') || (base[1] >= 'A' && base[1] <= 'Z')) {
					base = base[1:]
				}
			}
			pathStr = filepath.FromSlash(base)
		}
	}

	// If the client provides an absolute path like "/logs/server.txt",
	// filepath.Abs will evaluate it as the OS root (not relative to the sandbox).
	// This will cause the subsequent filepath.Rel sandbox check to fail if it's not actually within allowedPaths.
	absPath, err := filepath.Abs(filepath.Clean(pathStr))
	if err != nil {
		return nil, fmt.Errorf("invalid path %q: %w", pathStr, err)
	}

	checkSandbox := func(pathToCheck string, allowedPaths []string) error {
		if len(allowedPaths) > 0 {
			isAllowed := false
			for _, allowedDir := range allowedPaths {
				rel, err := filepath.Rel(allowedDir, pathToCheck)
				if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
					isAllowed = true
					break
				}
			}
			if !isAllowed {
				return fmt.Errorf("security violation: path %q is not within any allowedPaths", pathToCheck)
			}
		} else {
			parts := strings.Split(filepath.ToSlash(pathToCheck), "/")
			for _, part := range parts {
				if strings.HasPrefix(part, ".") && part != "." && part != ".." {
					return fmt.Errorf("security violation: access to hidden file or directory %q is blocked when allowedPaths is not specified", pathToCheck)
				}
			}
		}
		return nil
	}

	// First security check on the unresolved absolute path to prevent leaking existence
	// of files outside the sandbox during EvalSymlinks.
	if err := checkSandbox(absPath, r.unresolvedAllowedPaths); err != nil {
		return nil, err
	}

	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		// file does not exist
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("file not found: %q: %w", pathStr, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("failed to evaluate symlinks for %q: %w", absPath, err)
	}

	// Second security check on the resolved path to prevent symlink escape attacks
	if err := checkSandbox(resolvedPath, r.resolvedAllowedPaths); err != nil {
		return nil, err
	}

	// Security check for extension on BOTH the requested path and resolved target
	if err := validateExtension(pathStr); err != nil {
		return nil, fmt.Errorf("security violation: requested file extension not allowed: %w", err)
	}
	if err := validateExtension(resolvedPath); err != nil {
		return nil, fmt.Errorf("security violation: file extension not allowed: %w", err)
	}

	statInfo, err := os.Lstat(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to lstat file %q: %w", resolvedPath, err)
	}

	if statInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("security violation: TOCTOU symlink swap detected on %q", resolvedPath)
	}

	f, err := os.Open(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %q: %w", resolvedPath, err)
	}
	defer f.Close()

	openInfo, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("failed to stat opened file %q: %w", resolvedPath, err)
	}

	if !os.SameFile(statInfo, openInfo) {
		return nil, fmt.Errorf("security violation: TOCTOU file swap detected on %q between stat and open", resolvedPath)
	}

	if !openInfo.Mode().IsRegular() {
		return nil, fmt.Errorf("security violation: file %q was swapped with a non-regular file during read", resolvedPath)
	}

	limit := *r.MaxSize
	limitedReader := io.LimitReader(f, limit+1)
	content, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %q: %w", resolvedPath, err)
	}

	if int64(len(content)) > limit {
		truncated := content[:limit]
		for len(truncated) > 0 {
			r, size := utf8.DecodeLastRune(truncated)
			if r == utf8.RuneError && size == 1 {
				truncated = truncated[:len(truncated)-1]
			} else {
				break
			}
		}
		warning := fmt.Sprintf("\n\n...[TRUNCATED BY SERVER: Payload exceeded %d byte safety limit]...", limit)
		return string(truncated) + warning, nil
	}

	return string(content), nil
}

// ToConfig returns the original configuration for this template.
func (r *FileTemplate) ToConfig() resources.ResourceTemplateConfig {
	return &r.TemplateConfig
}
