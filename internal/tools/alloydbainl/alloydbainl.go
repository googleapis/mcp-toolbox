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

package alloydbainl

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"github.com/jackc/pgx/v5/pgxpool"
)

const resourceType string = "alloydb-ai-nl"

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
	PostgresPoolContext(context.Context) (*pgxpool.Pool, error)
	RunSQL(context.Context, string, []any) (any, error)
}

type Config struct {
	tools.ConfigBase   `yaml:",inline"`
	Type               string                `yaml:"type" validate:"required"`
	Source             string                `yaml:"source" validate:"required"`
	NLConfig           string                `yaml:"nlConfig" validate:"required"`
	NLConfigParameters parameters.Parameters `yaml:"nlConfigParameters"`
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
	numParams := len(cfg.NLConfigParameters)
	quotedNameParts := make([]string, 0, numParams)
	placeholderParts := make([]string, 0, numParams)

	for i, paramDef := range cfg.NLConfigParameters {
		name := paramDef.GetName()
		escapedName := strings.ReplaceAll(name, "'", "''") // Escape for SQL literal
		quotedNameParts = append(quotedNameParts, fmt.Sprintf("'%s'", escapedName))
		placeholderParts = append(placeholderParts, fmt.Sprintf("$%d", i+3)) // $1, $2 reserved
	}

	var stmt string
	if numParams > 0 {
		paramNamesSQL := fmt.Sprintf("ARRAY[%s]", strings.Join(quotedNameParts, ", "))
		paramValuesSQL := fmt.Sprintf("ARRAY[%s]", strings.Join(placeholderParts, ", "))
		// execute_nl_query is the AlloyDB AI function that executes the natural language query
		// The first parameter is the natural language query, which is passed as $1
		// The second parameter is the NLConfig, which is passed as a $2
		// The following params are the list of PSV values passed to the NLConfig
		// Example SQL statement being executed:
		// SELECT alloydb_ai_nl.execute_nl_query(nl_question => 'How many tickets do I have?', nl_config_id => 'cymbal_air_nl_config', param_names => ARRAY ['user_email'], param_values => ARRAY ['hailongli@google.com']);
		stmtFormat := "SELECT alloydb_ai_nl.execute_nl_query(nl_question => $1, nl_config_id => $2, param_names => %s, param_values => %s);"
		stmt = fmt.Sprintf(stmtFormat, paramNamesSQL, paramValuesSQL)
	} else {
		stmt = "SELECT alloydb_ai_nl.execute_nl_query(nl_question => $1, nl_config_id => $2);"
	}
	newQuestionParam := parameters.NewStringParameter(
		"question",                              // name
		"The natural language question to ask.", // description
	)

	cfg.NLConfigParameters = append([]parameters.Parameter{newQuestionParam}, cfg.NLConfigParameters...)

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: cfg.NLConfigParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			cfg.NLConfigParameters,
		),
		Statement: stmt,
	}, nil
}

// validate interface
var _ tools.Tool = Tool{}

type Tool struct {
	tools.BaseTool[Config]
	Statement string
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	sliceParams := params.AsSlice()
	allParamValues := make([]any, len(sliceParams)+1)
	allParamValues[0] = fmt.Sprintf("%s", sliceParams[0]) // nl_question
	allParamValues[1] = t.Cfg.NLConfig                    // nl_config
	for i, param := range sliceParams[1:] {
		allParamValues[i+2] = fmt.Sprintf("%s", param)
	}

	resp, err := source.RunSQL(ctx, t.Statement, allParamValues)
	if err != nil {
		return nil, util.NewClientServerError(fmt.Sprintf("error running SQL query: %v. Query: %v , Values: %v. Toolbox v0.19.0+ is only compatible with AlloyDB AI NL v1.0.3+. Please ensure that you are using the latest AlloyDB AI NL extension", err, t.Statement, allParamValues), http.StatusBadRequest, err)
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
