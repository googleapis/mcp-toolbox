// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package mongodbfind

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mongodb/mongodbcommon"
)

const resourceType string = "mongodb-find"

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
	Find(context.Context, string, string, string, *options.FindOptionsBuilder) ([]any, error)
}

type Config struct {
	tools.ConfigBase        `yaml:",inline"`
	Type                    string                `yaml:"type" validate:"required"`
	Source                  string                `yaml:"source" validate:"required"`
	Database                string                `yaml:"database" validate:"required"`
	Collection              string                `yaml:"collection"`
	CollectionAllowedValues []string              `yaml:"collectionAllowedValues"`
	FilterPayload           string                `yaml:"filterPayload" validate:"required"`
	FilterParams            parameters.Parameters `yaml:"filterParams"`
	ProjectPayload          string                `yaml:"projectPayload"`
	ProjectParams           parameters.Parameters `yaml:"projectParams"`
	SortPayload             string                `yaml:"sortPayload"`
	SortParams              parameters.Parameters `yaml:"sortParams"`
	Limit                   int64                 `yaml:"limit"`
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

	allParameters := slices.Concat(cfg.FilterParams, cfg.ProjectParams, cfg.SortParams)

	if err := mongodbcommon.ValidateCollectionConfig(cfg.Collection, cfg.CollectionAllowedValues); err != nil {
		return nil, err
	}
	allParameters = mongodbcommon.WithRuntimeCollectionParam(cfg.Collection, cfg.CollectionAllowedValues, allParameters)

	if err := parameters.CheckDuplicateParameters(allParameters); err != nil {
		return nil, err
	}

	if cfg.Limit <= 0 {
		return nil, fmt.Errorf("limit must be a positive number, but got %d", cfg.Limit)
	}

	paramManifest := allParameters.Manifest()
	if paramManifest == nil {
		paramManifest = make([]parameters.ParameterManifest, 0)
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

func getOptions(ctx context.Context, sortParameters parameters.Parameters, projectPayload string, limit int64, paramsMap map[string]any) (*options.FindOptionsBuilder, error) {
	logger, err := util.LoggerFromContext(ctx)
	if err != nil {
		return nil, err
	}

	opts := options.Find()

	sort := bson.M{}
	for _, p := range sortParameters {
		sort[p.GetName()] = paramsMap[p.GetName()]
	}
	opts = opts.SetSort(sort)

	if len(projectPayload) > 0 {

		result, err := parameters.PopulateTemplateWithJSON("MongoDBFindProjectString", projectPayload, paramsMap)

		if err != nil {
			return nil, fmt.Errorf("error populating project payload: %s", err)
		}

		var projection any
		err = bson.UnmarshalExtJSON([]byte(result), false, &projection)
		if err != nil {
			return nil, fmt.Errorf("error unmarshalling projection: %s", err)
		}

		opts = opts.SetProjection(projection)
		logger.DebugContext(ctx, fmt.Sprintf("Projection is set to %v", projection))
	}

	if limit > 0 {
		opts = opts.SetLimit(limit)
		logger.DebugContext(ctx, fmt.Sprintf("Limit is being set to %d", limit))
	}
	return opts, nil
}

func (t Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	paramsMap := params.AsMap()

	collection, tbErr := mongodbcommon.ResolveCollection(t.Cfg.Collection, paramsMap)
	if tbErr != nil {
		return nil, tbErr
	}

	filterString, err := parameters.PopulateTemplateWithJSON("MongoDBFindFilterString", t.Cfg.FilterPayload, paramsMap)
	if err != nil {
		return nil, util.NewAgentError("error populating filter", err)
	}
	opts, err := getOptions(ctx, t.Cfg.SortParams, t.Cfg.ProjectPayload, t.Cfg.Limit, paramsMap)
	if err != nil {
		return nil, util.NewAgentError("error populating options", err)
	}
	resp, err := source.Find(ctx, filterString, t.Cfg.Database, collection, opts)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil
}
