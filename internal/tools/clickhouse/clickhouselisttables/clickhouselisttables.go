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

package clickhouse

import (
	"context"
	"fmt"
	"net/http"
	"regexp"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const listTablesType string = "clickhouse-list-tables"
const databaseKey string = "database"

// validIdentifier matches the ClickHouse unquoted identifier grammar: a letter
// or underscore followed by letters, digits, or underscores. The `database`
// parameter is interpolated directly into a `SHOW TABLES FROM` statement and
// cannot be bound as a positional value, so restricting it to this character
// set is the only safe option short of refactoring to use ClickHouse's
// `system.tables` view.
var validIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

func init() {
	if !tools.Register(listTablesType, newListTablesConfig) {
		panic(fmt.Sprintf("tool type %q already registered", listTablesType))
	}
}

func newListTablesConfig(ctx context.Context, name string, decoder *yaml.Decoder) (tools.ToolConfig, error) {
	actual := Config{ConfigBase: tools.ConfigBase{Name: name}}
	if err := decoder.DecodeContext(ctx, &actual); err != nil {
		return nil, err
	}
	return actual, nil
}

type compatibleSource interface {
	RunSQL(context.Context, string, parameters.ParamValues) (any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string                `yaml:"type" validate:"required"`
	Source           string                `yaml:"source" validate:"required"`
	Parameters       parameters.Parameters `yaml:"parameters"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return listTablesType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	databaseParameter := parameters.NewStringParameter(databaseKey, "The database to list tables from.")
	params := parameters.Parameters{databaseParameter}

	allParameters, paramManifest, err := parameters.ProcessParameters(nil, params)
	if err != nil {
		return nil, err
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewReadOnlyAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: paramManifest, AuthRequired: cfg.AuthRequired},
			allParameters,
		),
	}, nil
}

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
	mapParams := params.AsMap()
	database, ok := mapParams[databaseKey].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a string", databaseKey), nil)
	}

	// The database name is interpolated directly into the `SHOW TABLES FROM`
	// statement. Reject anything that is not a plain identifier so that a
	// caller cannot smuggle additional clauses (LIKE, LIMIT, FORMAT,
	// INTO OUTFILE, ...), quoted identifiers, or other expressions through
	// this parameter. Without this check, an MCP client (or a prompt-injected
	// LLM) could escape the intended scope of the tool and read arbitrary
	// system tables, switch the output format to one the calling layer cannot
	// parse safely, or chain a SHOW with an `INTO OUTFILE` write.
	if !validIdentifier.MatchString(database) {
		return nil, util.NewAgentError(fmt.Sprintf("invalid '%s' parameter %q: must be a plain identifier matching %s", databaseKey, database, validIdentifier.String()), nil)
	}
	query := fmt.Sprintf("SHOW TABLES FROM %s", database)

	out, err := source.RunSQL(ctx, query, nil)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}

	res, ok := out.([]any)
	if !ok {
		return nil, util.NewClientServerError("unable to convert result to list", http.StatusInternalServerError, nil)
	}

	var tables []map[string]any
	for _, item := range res {
		tableMap, ok := item.(map[string]any)
		if !ok {
			return nil, util.NewClientServerError(fmt.Sprintf("unexpected type in result: got %T, want map[string]any", item), http.StatusInternalServerError, nil)
		}
		tableMap["database"] = database
		tables = append(tables, tableMap)
	}
	return tables, nil
}
