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

package trinoexecutesql

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "trino-execute-sql"

// impersonationParamName is the tool input parameter that supplies the Trino
// user to impersonate. It is exposed only when impersonateUser is enabled and
// is forwarded as the X-Trino-User header rather than bound into the SQL.
const impersonationParamName string = "trino_user"

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
	TrinoDB() *sql.DB
	RunSQL(context.Context, string, []any) (any, error)
	RunSQLAsUser(context.Context, string, string, []any) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                 `yaml:"type" validate:"required"`
	Source           string                 `yaml:"source" validate:"required"`
	ImpersonateUser  bool                   `yaml:"impersonateUser"`
	Annotations      *tools.ToolAnnotations `yaml:"annotations,omitempty"`
}

// validate interface
var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	sqlParameter := parameters.NewStringParameter("sql", "The SQL query to execute against the Trino database.")
	params := parameters.Parameters{sqlParameter}
	if cfg.ImpersonateUser {
		params = append(params, parameters.NewStringParameter(impersonationParamName, "The Trino user to impersonate for this query (forwarded as the X-Trino-User header). Optional; if omitted the query runs as the source's configured user.", parameters.WithStringDefault("")))
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
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

	paramsMap := params.AsMap()
	sql, ok := paramsMap["sql"].(string)
	if !ok {
		return nil, util.NewAgentError("unable to cast the `sql` input parameter into string", nil)
	}

	if t.Cfg.ImpersonateUser {
		user, ok := paramsMap[impersonationParamName].(string)
		if !ok {
			return nil, util.NewAgentError(fmt.Sprintf("unable to cast the `%s` input parameter into string", impersonationParamName), nil)
		}
		res, err := source.RunSQLAsUser(ctx, user, sql, []any{})
		if err != nil {
			return nil, util.ProcessGeneralError(err)
		}
		return res, nil
	}

	res, err := source.RunSQL(ctx, sql, []any{})
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return res, nil
}
