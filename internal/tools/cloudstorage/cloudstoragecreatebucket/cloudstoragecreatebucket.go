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

package cloudstoragecreatebucket

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

const resourceType string = "cloud-storage-create-bucket"

const (
	projectKey                  = "project"
	bucketKey                   = "bucket"
	locationKey                 = "location"
	uniformBucketLevelAccessKey = "uniform_bucket_level_access"
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
	CreateBucket(ctx context.Context, bucket, project, location string, uniformBucketLevelAccess bool) (map[string]any, error)
}

type Config struct {
	tools.ConfigBase         `yaml:",inline"`
	Type                     string  `yaml:"type" validate:"required"`
	Source                   string  `yaml:"source" validate:"required"`
	Project                  *string `yaml:"project,omitempty"`
	Location                 *string `yaml:"location,omitempty"`
	UniformBucketLevelAccess *bool   `yaml:"uniform_bucket_level_access,omitempty"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}

	bucketParam := parameters.NewStringParameter(bucketKey, "Name of the Cloud Storage bucket to create.")
	allParameters := parameters.Parameters{bucketParam}
	if cfg.Project == nil {
		allParameters = append(allParameters, parameters.NewStringParameter(projectKey, "Project ID to create the bucket in. When empty, the source's configured project is used.", parameters.WithStringDefault("")))
	}
	if cfg.Location == nil {
		allParameters = append(allParameters, parameters.NewStringParameter(locationKey, "Location for the bucket, e.g. 'US', 'EU', or 'us-central1'. Omit to use the Cloud Storage service default.", parameters.WithStringRequired(false)))
	}
	if cfg.UniformBucketLevelAccess == nil {
		allParameters = append(allParameters, parameters.NewBooleanParameter(uniformBucketLevelAccessKey, "Whether to enable uniform bucket-level access on the bucket.", parameters.WithBooleanDefault(false)))
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
	bucket, ok := mapParams[bucketKey].(string)
	if !ok || bucket == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a non-empty string", bucketKey), nil)
	}
	project := cloudstoragecommon.ResolveString(t.Cfg.Project, mapParams, projectKey)
	location := cloudstoragecommon.ResolveString(t.Cfg.Location, mapParams, locationKey)
	uniformBucketLevelAccess := cloudstoragecommon.ResolveBool(t.Cfg.UniformBucketLevelAccess, mapParams, uniformBucketLevelAccessKey)

	resp, err := source.CreateBucket(ctx, bucket, project, location, uniformBucketLevelAccess)
	if err != nil {
		return nil, cloudstoragecommon.ProcessGCSError(err)
	}
	return resp, nil
}
