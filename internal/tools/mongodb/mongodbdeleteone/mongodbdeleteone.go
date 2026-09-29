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
// See the License for the language governing permissions and
// limitations under the License.
package mongodbdeleteone

import (
	"context"
	"fmt"
	"net/http"
	"slices"

	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/mongodb/mongodbcommon"
)

const resourceType string = "mongodb-delete-one"

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
	MongoClient() *mongo.Client
	DeleteOne(context.Context, string, string, string) (any, error)
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

	allParameters := slices.Concat(cfg.FilterParams)

	if err := mongodbcommon.ValidateCollectionConfig(cfg.Collection, cfg.CollectionAllowedValues); err != nil {
		return nil, err
	}
	allParameters = mongodbcommon.WithRuntimeCollectionParam(cfg.Collection, cfg.CollectionAllowedValues, allParameters)

	if err := parameters.CheckDuplicateParameters(allParameters); err != nil {
		return nil, err
	}

	paramManifest := allParameters.Manifest()
	if paramManifest == nil {
		paramManifest = make([]parameters.ParameterManifest, 0)
	}

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
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

	filterString, err := parameters.PopulateTemplateWithJSON("MongoDBDeleteOneFilter", t.Cfg.FilterPayload, paramsMap)
	if err != nil {
		return nil, util.NewAgentError("error populating filter", err)
	}
	resp, err := source.DeleteOne(ctx, filterString, t.Cfg.Database, collection)
	if err != nil {
		return nil, util.ProcessGeneralError(err)
	}
	return resp, nil
}
