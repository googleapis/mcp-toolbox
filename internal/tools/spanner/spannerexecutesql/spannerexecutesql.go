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

package spannerexecutesql

import (
	"context"
	"fmt"
	"net/http"

	"cloud.google.com/go/spanner"
	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "spanner-execute-sql"

func init() {
	if !tools.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("tool type %q already registered", resourceType))
	}
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{ConfigBase: tools.ConfigBase{Name: name}}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type compatibleSource interface {
	SpannerClient() *spanner.Client
	DatabaseDialect() string
	RunSQL(context.Context, bool, string, map[string]any) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	ReadOnly         *bool                  `yaml:"readOnly,omitempty"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(ctx context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	if cfg.ReadOnly != nil {
		l, err := util.LoggerFromContext(ctx)
		if err == nil {
			l.WarnContext(ctx, fmt.Sprintf("[DEPRECATED] The 'readOnly' field on tool %q is deprecated and will be removed in a future release. Please configure 'readOnly: true' on the Spanner source instead.", cfg.Name))
		}
	}

	if cfg.ReadOnly != nil && cfg.Annotations != nil && cfg.Annotations.ReadOnlyHint != nil {
		if *cfg.ReadOnly != *cfg.Annotations.ReadOnlyHint {
			return nil, fmt.Errorf("conflict in tool %q: legacy 'readOnly' (%v) and 'annotations.readOnlyHint' (%v) cannot have different values", cfg.Name, *cfg.ReadOnly, *cfg.Annotations.ReadOnlyHint)
		}
	}

	sqlParameter := parameters.NewStringParameter("sql", "The sql to execute.")
	params := parameters.Parameters{sqlParameter}

	defaultAnnotations := tools.NewDestructiveAnnotations
	if cfg.ReadOnly != nil && *cfg.ReadOnly {
		defaultAnnotations = tools.NewReadOnlyAnnotations
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, defaultAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: params.Manifest(), AuthRequired: cfg.AuthRequired},
			params,
		),
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()
	sql, ok := paramsMap["sql"].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("unable to get cast %s", paramsMap["sql"]), nil)
	}

	// Log the query executed for debugging.
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, util.NewClientServerError("error getting logger", http.StatusInternalServerError, err)
	}
	logger.DebugContext(ctx, fmt.Sprintf("executing `%s` tool query: %s", resourceType, sql))
	readOnly := false
	if t.Cfg.ReadOnly != nil {
		readOnly = *t.Cfg.ReadOnly
	} else if ann := t.GetAnnotations(s); ann != nil && ann.ReadOnlyHint != nil {
		readOnly = *ann.ReadOnlyHint
	}
	resp, err := source.RunSQL(ctx, readOnly, sql, nil)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ToConfig() tools.ToolConfig {
	return t.Cfg
}

func (t Tool) ValidateSource(source sources.Source) error {
	_, ok := source.(compatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return nil
}
