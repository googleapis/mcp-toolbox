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

package cloudstoragecopyobject

import (
	"context"
	"fmt"
	"net/http"

	yaml "github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/tools/cloudstorage/cloudstoragecommon"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
)

const resourceType string = "cloud-storage-copy-object"

const (
	sourceBucketKey      = "source_bucket"
	sourceObjectKey      = "source_object"
	destinationBucketKey = "destination_bucket"
	destinationObjectKey = "destination_object"
)

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
	CopyObject(ctx context.Context, sourceBucket, sourceObject, destinationBucket, destinationObject string) (map[string]any, error)
}

type Config struct {
	tools.ConfigBase  `yaml:",inline"`
	Type              string  `yaml:"type" validate:"required"`
	Source            string  `yaml:"source" validate:"required"`
	SourceBucket      *string `yaml:"source_bucket,omitempty"`
	DestinationBucket *string `yaml:"destination_bucket,omitempty"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}
	if cfg.SourceBucket != nil && *cfg.SourceBucket == "" {
		return nil, fmt.Errorf("source_bucket cannot be empty for tool %q", cfg.Name)
	}
	if cfg.DestinationBucket != nil && *cfg.DestinationBucket == "" {
		return nil, fmt.Errorf("destination_bucket cannot be empty for tool %q", cfg.Name)
	}

	sourceObjectParam := parameters.NewStringParameter(sourceObjectKey, "Full source object name (path) within the source bucket, e.g. 'path/to/file.txt'.")
	destinationObjectParam := parameters.NewStringParameter(destinationObjectKey, "Full destination object name (path) within the destination bucket, e.g. 'path/to/file.txt'.")
	allParameters := parameters.Parameters{}
	if cfg.SourceBucket == nil {
		allParameters = append(allParameters, parameters.NewStringParameter(sourceBucketKey, "Name of the Cloud Storage bucket containing the source object."))
	}
	allParameters = append(allParameters, sourceObjectParam)
	if cfg.DestinationBucket == nil {
		allParameters = append(allParameters, parameters.NewStringParameter(destinationBucketKey, "Name of the Cloud Storage bucket to copy into."))
	}
	allParameters = append(allParameters, destinationObjectParam)

	return Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(cfg.Annotations, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: cfg.Description, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
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
	sourceBucket := cloudstoragecommon.ResolveString(t.Cfg.SourceBucket, mapParams, sourceBucketKey)
	if sourceBucket == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a non-empty string", sourceBucketKey), nil)
	}
	sourceObject, ok := mapParams[sourceObjectKey].(string)
	if !ok || sourceObject == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a non-empty string", sourceObjectKey), nil)
	}
	destinationBucket := cloudstoragecommon.ResolveString(t.Cfg.DestinationBucket, mapParams, destinationBucketKey)
	if destinationBucket == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a non-empty string", destinationBucketKey), nil)
	}
	destinationObject, ok := mapParams[destinationObjectKey].(string)
	if !ok || destinationObject == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a non-empty string", destinationObjectKey), nil)
	}

	resp, err := source.CopyObject(ctx, sourceBucket, sourceObject, destinationBucket, destinationObject)
	if err != nil {
		return nil, cloudstoragecommon.ProcessGCSError(err)
	}
	return resp, nil
}
