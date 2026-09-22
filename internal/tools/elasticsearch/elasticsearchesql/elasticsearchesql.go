// Copyright 2025 Google LLC
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

package elasticsearchesql

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/tools"
)

const resourceType string = "elasticsearch-esql"

func init() {
	if !tools.Register(resourceType, newConfig) {
		panic(fmt.Sprintf("tool type %q already registered", resourceType))
	}
}

type compatibleSource interface {
	RunSQL(ctx context.Context, format, query string, params []map[string]any) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Query            string                 `yaml:"query" validate:"required"`
	Format           string                 `yaml:"format"`
	Timeout          int                    `yaml:"timeout"`
	Parameters       parameters.Parameters  `yaml:"parameters"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

var _ tools.ToolConfig = Config{}

func (c Config) ToolConfigType() string {
	return resourceType
}

func newConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{ConfigBase: tools.ConfigBase{Name: name}}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type Tool struct {
	tools.BaseTool[Config]
}

var _ tools.Tool = Tool{}

func (c Config) Initialize(context.Context) (tools.Tool, error) {
	if c.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", c.Name)
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			c,
			tools.GetAnnotationsOrDefault(c.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: c.Description, Parameters: c.Parameters.Manifest(), AuthRequired: c.AuthRequired},
			c.Parameters,
		),
	}, nil
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

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	var cancel context.CancelFunc
	if t.Cfg.Timeout > 0 {
		ctx, cancel = context.WithTimeout(ctx, time.Duration(t.Cfg.Timeout)*time.Second)
		defer cancel()
	} else {
		ctx, cancel = context.WithTimeout(ctx, time.Minute)
		defer cancel()
	}

	query := t.Cfg.Query
	paramMap := params.AsMap()

	var paramsList []map[string]any
	for _, param := range t.Cfg.Parameters {
		if param.GetType() == "array" {
			return nil, util.NewAgentError("array parameters are not supported yet", nil)
		}

		// ES|QL requires an array of single-key objects for named parameters
		if val, ok := paramMap[param.GetName()]; ok {
			paramsList = append(paramsList, map[string]any{param.GetName(): val})
		}
	}

	resp, err := source.RunSQL(ctx, t.Cfg.Format, query, paramsList)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil
}
