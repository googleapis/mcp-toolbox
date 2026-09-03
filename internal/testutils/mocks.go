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

package testutils

import (
	"context"
	"fmt"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/prompts"
	"github.com/googleapis/mcp-toolbox/internal/resources"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.opentelemetry.io/otel/trace"
)

// MockSourceConfig is used to mock source config in tests
type MockSourceConfig struct {
	Name     string `yaml:"name"`
	Type     string `yaml:"type"`
	Foo      string `yaml:"foo"`
	ReadOnly bool   `yaml:"readOnly"`
}

func (m MockSourceConfig) SourceConfigType() string {
	if m.Type != "" {
		return m.Type
	}
	return "mock-source"
}

func (m MockSourceConfig) Initialize(ctx context.Context, tracer trace.Tracer) (sources.Source, error) {
	return MockSource{MockSourceConfig: m}, nil
}

// MockSource is used to mock source in tests
type MockSource struct {
	MockSourceConfig
}

func (s MockSource) IsReadOnly() bool {
	return s.ReadOnly
}

func (s MockSource) SourceType() string {
	if s.Type != "" {
		return s.Type
	}
	return "mock-source"
}

func (s MockSource) ToConfig() sources.SourceConfig {
	return s.MockSourceConfig
}

// MockToolConfig is used to mock tool config in tests
type MockToolConfig struct {
	tools.ConfigBase `yaml:",inline"`
	Source           string                 `yaml:"source"`
	Parameters       parameters.Parameters  `yaml:"parameters"`
	Type             string                 `yaml:"type"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

func (m MockToolConfig) ToolConfigType() string {
	if m.Type != "" {
		return m.Type
	}
	return "mock-tool"
}

func (m MockToolConfig) Initialize(context.Context) (tools.Tool, error) {
	return MockTool{
		BaseTool: tools.NewBaseTool(
			m, m.Annotations,
			tools.Manifest{Description: m.Description, Parameters: m.Parameters.Manifest(), AuthRequired: m.AuthRequired},
			m.Parameters,
		),
	}, nil
}

var _ tools.ToolConfig = MockToolConfig{}

// MockTool is used to mock tools in tests
type MockTool struct {
	tools.BaseTool[MockToolConfig]
	unauthorized               bool
	requireClientAuthorization bool
	ReturnParamsInInvoke       bool
}

var _ tools.Tool = MockTool{}

// NewMockTool creates a new mock prompt for testing.
func NewMockTool(name, desc, source string, params []parameters.Parameter, unauthorized, reqClientAutho bool) MockTool {
	mockConfig := MockToolConfig{
		ConfigBase: tools.ConfigBase{
			Name:        name,
			Description: desc,
		},
		Source:     source,
		Type:       "mock-tool",
		Parameters: params,
	}
	ctx := context.Background()
	t, _ := mockConfig.Initialize(ctx)
	mt := t.(MockTool)
	mt.unauthorized = unauthorized
	mt.requireClientAuthorization = reqClientAutho
	return mt
}

func (t MockTool) GetSourceName() string {
	return t.Cfg.Source
}

func (t MockTool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t MockTool) RequiresClientAuthorization(sources.Source) (bool, error) {
	// defaulted to false
	return t.requireClientAuthorization, nil
}

func (t MockTool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, token tools.AccessToken) (any, util.ToolboxError) {
	mock := []any{t.Cfg.Name}
	if t.ReturnParamsInInvoke && len(params) > 0 {
		for _, p := range params {
			mock = append(mock, p.Value)
		}
	}
	return mock, nil
}

func (t MockTool) Authorized(verifiedAuthServices []string) bool {
	// default to true
	return !t.unauthorized
}

func (t MockTool) ValidateSource(src sources.Source) error {
	if src == nil || src.SourceType() == "mock-source" {
		return nil
	}
	return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
}

// claims is a map of user info decoded from an auth token
func (t MockTool) ParseParams(data map[string]any, claimsMap map[string]map[string]any) (parameters.ParamValues, error) {
	return parameters.ParseParams(t.StaticParameters, data, claimsMap)
}

func (t MockTool) GetAuthTokenHeaderName(sources.Source) (string, error) {
	return "Authorization", nil
}

func (t MockTool) GetScopesRequired() []string {
	return nil
}

func (t MockTool) HasSecureParams() bool {
	for _, p := range t.StaticParameters {
		if p != nil && p.GetSecure() {
			return true
		}
	}
	return false
}

// MockPrompt is used to mock prompts in tests
type MockPrompt struct {
	Name        string
	Description string
	Args        prompts.Arguments
	manifest    prompts.Manifest
}

func (p MockPrompt) SubstituteParams(vals parameters.ParamValues) (any, error) {
	return []prompts.Message{
		{
			Role:    "user",
			Content: fmt.Sprintf("substituted %s", p.Name),
		},
	}, nil
}

func (p MockPrompt) ParseArgs(data map[string]any, claimsMap map[string]map[string]any) (parameters.ParamValues, error) {
	var params parameters.Parameters
	for _, arg := range p.Args {
		params = append(params, arg.Parameter)
	}
	return parameters.ParseParams(params, data, claimsMap)
}

func (p MockPrompt) Manifest() prompts.Manifest {
	var argManifests []parameters.ParameterManifest
	for _, arg := range p.Args {
		argManifests = append(argManifests, arg.Manifest())
	}
	return prompts.Manifest{
		Description: p.Description,
		Arguments:   argManifests,
	}
}

func (p MockPrompt) GetDesc() string {
	return p.Description
}

func (p MockPrompt) GetArguments() prompts.Arguments {
	return p.Args
}

func (p MockPrompt) ToConfig() prompts.PromptConfig {
	return nil
}

// NewMockPrompt creates a new mock prompt for testing.
func NewMockPrompt(name, desc string, args prompts.Arguments) MockPrompt {
	var argManifests []parameters.ParameterManifest
	for _, arg := range args {
		argManifests = append(argManifests, arg.Manifest())
	}
	manifest := prompts.Manifest{
		Description: desc,
		Arguments:   argManifests,
	}
	return MockPrompt{
		Name:        name,
		Description: desc,
		Args:        args,
		manifest:    manifest,
	}
}

// RegisterMockResource registers the mock resource type with the resources package.
func RegisterMockResource() {
	resources.Register("mock", func(ctx context.Context, name string, decoder *yaml.Decoder) (resources.ResourceConfig, error) {
		var cfg MockResourceConfig
		cfg.Name = name
		cfg.Type = "mock"
		if err := decoder.DecodeContext(ctx, &cfg); err != nil {
			return nil, err
		}
		return &cfg, nil
	})
}

// MockResourceConfig is a mock implementation of resources.ResourceConfig
type MockResourceConfig struct {
	resources.ResourceConfigBase `yaml:",inline"`
	Size                         *int64 `yaml:"-"`
}

func (m *MockResourceConfig) ResourceConfigType() string {
	return "mock"
}

func (m *MockResourceConfig) Initialize(ctx context.Context) (resources.Resource, error) {
	return MockResource{config: m}, nil
}

// MockResource is a mock implementation of resources.Resource
type MockResource struct {
	config *MockResourceConfig
}

func (m MockResource) GetTitle() string                               { return m.config.GetTitle() }
func (m MockResource) GetDescription() string                         { return m.config.GetDescription() }
func (m MockResource) GetMimeType() string                            { return m.config.GetMimeType() }
func (m MockResource) GetURI() string                                 { return m.config.GetURI() }
func (m MockResource) GetSize() *int64                                { return m.config.Size }
func (m MockResource) GetAnnotations() *resources.ResourceAnnotations { return m.config.Annotations }

func (m MockResource) Read(ctx context.Context, params map[string]any) (any, error) {
	return "mock resource data", nil
}

func (m MockResource) ToConfig() resources.ResourceConfig {
	return m.config
}

func (m MockResource) GetName() string {
	return m.config.Name
}

// MockResourceTemplateConfig is a mock implementation of resources.ResourceTemplateConfig
type MockResourceTemplateConfig struct {
	resources.ResourceTemplateConfigBase `yaml:",inline"`
}

func (m *MockResourceTemplateConfig) ResourceTemplateConfigType() string {
	if m.Type != "" {
		return m.Type
	}
	return "mock"
}

func (m *MockResourceTemplateConfig) Initialize(ctx context.Context) (resources.ResourceTemplate, error) {
	return MockResourceTemplate{config: m}, nil
}

// MockResourceTemplate is a mock implementation of resources.ResourceTemplate
type MockResourceTemplate struct {
	config *MockResourceTemplateConfig
}

func (m MockResourceTemplate) GetTitle() string       { return m.config.GetTitle() }
func (m MockResourceTemplate) GetDescription() string { return m.config.GetDescription() }
func (m MockResourceTemplate) GetMimeType() string    { return m.config.GetMimeType() }
func (m MockResourceTemplate) GetURITemplate() string { return m.config.GetURITemplate() }

func (m MockResourceTemplate) Read(ctx context.Context, params map[string]any) (any, error) {
	return "mock resource template data", nil
}

func (m MockResourceTemplate) ToConfig() resources.ResourceTemplateConfig {
	return m.config
}

func (m MockResourceTemplate) GetName() string {
	return m.config.Name
}

func NewMockResource(name, uri, title, description, mimeType string, size *int64, annotations *resources.ResourceAnnotations) MockResource {
	cfgBase := resources.ConfigBase{Name: name}
	if title != "" {
		cfgBase.Title = title
	}
	if description != "" {
		cfgBase.Description = description
	}
	if mimeType != "" {
		cfgBase.MimeType = mimeType
	}
	if annotations != nil {
		cfgBase.Annotations = annotations
	}

	resCfg := resources.ResourceConfigBase{
		ConfigBase: cfgBase,
		URI:        uri,
	}

	return MockResource{
		config: &MockResourceConfig{
			ResourceConfigBase: resCfg,
			Size:               size,
		},
	}
}

func NewMockResourceTemplate(name, uriTemplate, title, description, mimeType string, annotations *resources.ResourceAnnotations) MockResourceTemplate {
	cfgBase := resources.ConfigBase{Name: name}
	if title != "" {
		cfgBase.Title = title
	}
	if description != "" {
		cfgBase.Description = description
	}
	if mimeType != "" {
		cfgBase.MimeType = mimeType
	}
	if annotations != nil {
		cfgBase.Annotations = annotations
	}

	return MockResourceTemplate{
		config: &MockResourceTemplateConfig{
			ResourceTemplateConfigBase: resources.ResourceTemplateConfigBase{
				ConfigBase:  resources.ConfigBase{Name: name},
				URITemplate: uriTemplate,
			},
		},
	}
}

func (m MockResourceTemplate) GetAnnotations() *resources.ResourceAnnotations {
	return m.config.Annotations
}
