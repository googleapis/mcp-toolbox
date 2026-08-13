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

package spannerupdatedatabaseschema

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "spanner-update-database-schema"

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
	UpdateDatabaseDdl(ctx context.Context, statements []string, tokenString string) error
	UseClientAuthorization() bool
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	allParameters := parameters.Parameters{
		parameters.NewArrayParameter(
			"statements",
			"A list of DDL statements to execute against the Cloud Spanner database (e.g. CREATE TABLE, ALTER TABLE, CREATE INDEX, DROP TABLE).",
			parameters.NewStringParameter("statement", "A single DDL statement to execute.", parameters.WithStringRequired(true)),
			parameters.WithArrayRequired(true),
		),
	}

	if cfg.Description == "" {
		cfg.Description = "Executes DDL (Data Definition Language) statements to modify the Spanner database schema, such as creating, altering, or dropping tables, indexes, views, or change streams. Do not use this tool for DML (INSERT, UPDATE, DELETE) or DQL (SELECT) queries; use execute_sql or execute_sql_readonly instead."
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
		),
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
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

func (t Tool) RequiresClientAuthorization(source sources.Source) (bool, error) {
	s, ok := source.(compatibleSource)
	if !ok {
		return false, fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return s.UseClientAuthorization(), nil
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}

	paramsMap := params.AsMap()
	rawStmts, exists := paramsMap["statements"]
	if !exists || rawStmts == nil {
		return nil, util.NewAgentError("missing required parameter 'statements'", nil)
	}

	var statements []string
	switch v := rawStmts.(type) {
	case []any:
		for _, elem := range v {
			str, ok := elem.(string)
			if !ok {
				return nil, util.NewAgentError(fmt.Sprintf("invalid element type in 'statements': expected string, got %T", elem), nil)
			}
			if str == "" {
				return nil, util.NewAgentError("empty statement is not allowed", nil)
			}
			statements = append(statements, str)
		}
	case []string:
		for _, str := range v {
			if str == "" {
				return nil, util.NewAgentError("empty statement is not allowed", nil)
			}
			statements = append(statements, str)
		}
	case string:
		if v == "" {
			return nil, util.NewAgentError("empty statement is not allowed", nil)
		}
		statements = []string{v}
	default:
		return nil, util.NewAgentError(fmt.Sprintf("invalid type for 'statements': expected array of strings, got %T", rawStmts), nil)
	}

	if len(statements) == 0 {
		return nil, util.NewAgentError("statements array must contain at least one non-empty DDL statement", nil)
	}

	var tokenStr string
	var err error
	if source.UseClientAuthorization() {
		tokenStr, err = accessToken.ParseBearerToken()
		if err != nil {
			return nil, util.NewClientServerError("failed to parse access token", http.StatusUnauthorized, err)
		}
	}

	if err := source.UpdateDatabaseDdl(ctx, statements, tokenStr); err != nil {
		return nil, util.ProcessGcpError(err)
	}

	return map[string]any{
		"message":             "Database schema updated successfully",
		"statements_executed": len(statements),
	}, nil
}
