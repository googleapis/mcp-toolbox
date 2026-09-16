// Copyright 2024 Google LLC
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

package tools

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/embeddingmodels"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

// ToolConfigFactory defines the signature for a function that creates and
// decodes a specific tool's configuration. It takes the context, the tool's
// name, and a YAML decoder to parse the config.
type ToolConfigFactory func(ctx context.Context, name string, decoder *yaml.Decoder) (ToolConfig, error)

var toolRegistry = make(map[string]ToolConfigFactory)

// Register allows individual tool packages to register their configuration
// factory function. This is typically called from an init() function in the
// tool's package. It associates a 'type' string with a function that can
// produce the specific ToolConfig type. It returns true if the registration was
// successful, and false if a tool with the same type was already registered.
func Register(resourceType string, factory ToolConfigFactory) bool {
	if _, exists := toolRegistry[resourceType]; exists {
		// Tool with this type already exists, do not overwrite.
		return false
	}
	toolRegistry[resourceType] = factory
	return true
}

var ErrUnknownToolType = fmt.Errorf("unknown tool type")

// DecodeConfig looks up the registered factory for the given type and uses it
// to decode the tool configuration.
func DecodeConfig(ctx context.Context, resourceType string, name string, decoder *yaml.Decoder) (ToolConfig, error) {
	factory, found := toolRegistry[resourceType]
	if !found {
		return nil, fmt.Errorf("%w: %q", ErrUnknownToolType, resourceType)
	}
	toolConfig, err := factory(ctx, name, decoder)
	if err != nil {
		return nil, fmt.Errorf("unable to parse tool %q as type %q: %w", name, resourceType, err)
	}
	return toolConfig, nil
}

type ToolConfig interface {
	ToolConfigType() string
	Initialize(context.Context) (Tool, error)
}

// https://modelcontextprotocol.io/specification/2025-06-18/schema#toolannotations
type ToolAnnotations struct {
	DestructiveHint *bool `json:"destructiveHint,omitempty" yaml:"destructiveHint,omitempty"`
	IdempotentHint  *bool `json:"idempotentHint,omitempty" yaml:"idempotentHint,omitempty"`
	OpenWorldHint   *bool `json:"openWorldHint,omitempty" yaml:"openWorldHint,omitempty"`
	ReadOnlyHint    *bool `json:"readOnlyHint,omitempty" yaml:"readOnlyHint,omitempty"`
}

// NewReadOnlyAnnotations creates default annotations for a read-only tool.
// Use this for tools that only query/fetch data without side effects.
func NewReadOnlyAnnotations() *ToolAnnotations {
	readOnly := true
	return &ToolAnnotations{ReadOnlyHint: &readOnly}
}

// NewDestructiveAnnotations creates default annotations for a destructive tool.
// Use this for tools that create, update, or delete data.
func NewDestructiveAnnotations() *ToolAnnotations {
	readOnly := false
	destructive := true
	return &ToolAnnotations{
		ReadOnlyHint:    &readOnly,
		DestructiveHint: &destructive,
	}
}

// NewWriteAnnotations creates default annotations for a non-destructive write
// tool: ReadOnlyHint is false, DestructiveHint is left unset.
func NewWriteAnnotations() *ToolAnnotations {
	readOnly := false
	return &ToolAnnotations{ReadOnlyHint: &readOnly}
}

// GetAnnotationsOrDefault returns the provided annotations if non-nil,
// otherwise returns the result of calling defaultFn.
func GetAnnotationsOrDefault(annotations *ToolAnnotations, defaultFn func() *ToolAnnotations) *ToolAnnotations {
	if annotations != nil {
		return annotations
	}
	return defaultFn()
}

type AccessToken string

func (token AccessToken) ParseBearerToken() (string, error) {
	headerParts := strings.Split(string(token), " ")
	if len(headerParts) != 2 || strings.ToLower(headerParts[0]) != "bearer" {
		return "", util.NewClientServerError("authorization header must be in the format 'Bearer <token>'", http.StatusUnauthorized, nil)
	}
	return headerParts[1], nil
}

type Tool interface {
	GetName() string
	GetSourceName() string
	GetDescription() string
	GetAuthRequired() []string
	GetAnnotations(sources.Source) *ToolAnnotations
	GetToolUIMetadata() *ToolUIMetadata
	Invoke(context.Context, sources.Source, parameters.ParamValues, AccessToken) (any, util.ToolboxError)
	EmbedParams(context.Context, parameters.ParamValues, PrimitiveManagerI) (parameters.ParamValues, error)
	Manifest(sources.Source) (Manifest, error)
	StaticManifest() Manifest
	Authorized([]string) bool
	RequiresClientAuthorization(sources.Source) (bool, error)
	ToConfig() ToolConfig
	GetAuthTokenHeaderName(sources.Source) (string, error)
	GetParameters(sources.Source) (parameters.Parameters, error)
	GetScopesRequired() []string
	ValidateSource(sources.Source) error
	HasSecureParams() bool
}

// PrimitiveManagerI defines the minimal view of the primitives.PrimitiveManager
// that the Tool package needs.
// This is implemented to prevent import cycles.
type PrimitiveManagerI interface {
	GetSource(string) (sources.Source, bool)
	GetEmbeddingModel(string) (embeddingmodels.EmbeddingModel, bool)
}

// Manifest is the representation of tools sent to Client SDKs.
type Manifest struct {
	Description  string                         `json:"description"`
	Parameters   []parameters.ParameterManifest `json:"parameters"`
	AuthRequired []string                       `json:"authRequired"`
}

// Helper function that returns if a tool invocation request is authorized
func IsAuthorized(authRequiredSources []string, verifiedAuthServices []string) bool {
	if len(authRequiredSources) == 0 {
		// no authorization requirement
		return true
	}
	for _, a := range authRequiredSources {
		if slices.Contains(verifiedAuthServices, a) {
			return true
		}
	}
	return false
}

// ToolMeta is the read-only view BaseTool needs of any tool's Config. Tools
// satisfy it for free by embedding ConfigBase.
type ToolMeta interface {
	GetName() string
	GetDescription() string
	GetAuthRequired() []string
	GetScopesRequired() []string
	GetToolUIMetadata() *ToolUIMetadata
}

// ToolVisibility defines the visibility of a tool in the UI.
type ToolVisibility string

const (
	VisibilityModel ToolVisibility = "model"
	VisibilityApp   ToolVisibility = "app"
)

// ToolUIMetadata defines the UI-specific metadata for a tool with UI Resource
type ToolUIMetadata struct {
	Resource   string           `yaml:"resource" validate:"required"`
	Visibility []ToolVisibility `yaml:"visibility,omitempty" validate:"omitempty,dive,oneof=model app"`
}

// ConfigBase owns the YAML fields that every tool's Config shares and that
// BaseTool reads through.
// Description is eagerly defaulted by the tool's Initialize (many prebuilt
// configs omit description: and rely on a canned per-tool string), so
// post-Initialize ConfigBase.Description holds the resolved value.
type ConfigBase struct {
	Name           string          `yaml:"name"           validate:"required"`
	Description    string          `yaml:"description"`
	AuthRequired   []string        `yaml:"authRequired"`
	ScopesRequired []string        `yaml:"scopesRequired"`
	UI             *ToolUIMetadata `yaml:"ui,omitempty"`
}

func (c ConfigBase) GetName() string             { return c.Name }
func (c ConfigBase) GetDescription() string      { return c.Description }
func (c ConfigBase) GetAuthRequired() []string   { return c.AuthRequired }
func (c ConfigBase) GetScopesRequired() []string { return c.ScopesRequired }
func (c ConfigBase) GetToolUIMetadata() *ToolUIMetadata {
	if c.UI == nil {
		return nil
	}
	if len(c.UI.Visibility) == 0 {
		ui := *c.UI
		ui.Visibility = []ToolVisibility{VisibilityModel, VisibilityApp}
		return &ui
	}
	return c.UI
}

// BaseTool provides default implementations of various methods on the Tool
// interface. Tools embed BaseTool to drop their boilerplate and override
// only methods that need custom behavior.
type BaseTool[T ToolMeta] struct {
	Cfg              T
	annotations      *ToolAnnotations
	metadata         Manifest
	StaticParameters parameters.Parameters
	hasSecureParams  bool
}

// NewBaseTool constructs a BaseTool from a resolved Config (typically the
// per-tool Config after Initialize has filled in defaults), the resolved
// annotations, the precomputed Manifest, and the tool's static parameters.
func NewBaseTool[T ToolMeta](cfg T, annotations *ToolAnnotations, metadata Manifest, staticParameters parameters.Parameters) BaseTool[T] {
	var hasSecureParams bool
	for _, p := range staticParameters {
		if p != nil && p.GetSecure() {
			hasSecureParams = true
			break
		}
	}
	return BaseTool[T]{
		Cfg:              cfg,
		annotations:      annotations,
		metadata:         metadata,
		StaticParameters: staticParameters,
		hasSecureParams:  hasSecureParams,
	}
}

func (b BaseTool[T]) GetName() string                                  { return b.Cfg.GetName() }
func (b BaseTool[T]) GetDescription() string                           { return b.Cfg.GetDescription() }
func (b BaseTool[T]) GetAuthRequired() []string                        { return b.Cfg.GetAuthRequired() }
func (b BaseTool[T]) HasSecureParams() bool                            { return b.hasSecureParams }
func (b BaseTool[T]) GetScopesRequired() []string                      { return b.Cfg.GetScopesRequired() }
func (b BaseTool[T]) GetAnnotations(_ sources.Source) *ToolAnnotations { return b.annotations }

// Manifest returns the precomputed metadata. It and GetParameters stay trivial
// and never call each other: embedded methods have no virtual dispatch, so a
// BaseTool method calling another would miss a concrete tool's override.
func (b BaseTool[T]) Manifest(_ sources.Source) (Manifest, error) {
	return b.metadata, nil
}

// StaticManifest returns the manifest baked at Initialize, with no source
// resolution. Dynamic tools override Manifest/GetParameters to refine params
// against a live source, but not this method, so it always reaches the baked
// skeleton — used for offline generation (e.g. skills) where no source exists.
func (b BaseTool[T]) StaticManifest() Manifest {
	return b.metadata
}

func (b BaseTool[T]) GetParameters(_ sources.Source) (parameters.Parameters, error) {
	return b.StaticParameters, nil
}

func (b BaseTool[T]) Authorized(verifiedAuthServices []string) bool {
	return IsAuthorized(b.Cfg.GetAuthRequired(), verifiedAuthServices)
}

func (b BaseTool[T]) RequiresClientAuthorization(_ sources.Source) (bool, error) {
	return false, nil
}

func (b BaseTool[T]) GetAuthTokenHeaderName(_ sources.Source) (string, error) {
	return "Authorization", nil
}

func (b BaseTool[T]) EmbedParams(ctx context.Context, paramValues parameters.ParamValues, pMgr PrimitiveManagerI) (parameters.ParamValues, error) {
	return parameters.EmbedParams(ctx, b.StaticParameters, paramValues, pMgr, nil)
}

// ShouldSuppress returns true if the tool should be suppressed from registration
// when its data source is in read-only mode, and logs relevant warnings or info messages.
// By default, if the source is read-only and the tool's ReadOnlyHint is explicitly false,
// the tool is suppressed. Unannotated tools (ReadOnlyHint == nil) or read-only tools
// (ReadOnlyHint == true) are not suppressed.
func ShouldSuppress(ctx context.Context, t Tool, src sources.Source) bool {
	if t == nil || src == nil || !src.IsReadOnly() {
		return false
	}

	toolName := t.GetName()
	l, _ := util.LoggerFromContext(ctx)

	ann := t.GetAnnotations(src)
	if ann != nil && ann.ReadOnlyHint != nil {
		if !*ann.ReadOnlyHint {
			if l != nil {
				l.InfoContext(ctx, fmt.Sprintf("Suppressing write-capable tool %q bound to read-only source", toolName))
			}
			return true
		}
		return false
	}

	if l != nil {
		l.WarnContext(ctx, fmt.Sprintf("Tool %q lacks ReadOnlyHint annotation; executing this tool may fail if it attempts write operations. If this tool is meant to be read-only, please add 'readOnlyHint: true' to its annotations. Otherwise, add 'readOnlyHint: false' to suppress it in read-only mode and save agent context window.", toolName))
	}

	return false
}

func (b BaseTool[T]) GetToolUIMetadata() *ToolUIMetadata {
	return b.Cfg.GetToolUIMetadata()
}
