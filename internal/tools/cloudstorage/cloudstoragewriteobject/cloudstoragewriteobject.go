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

package cloudstoragewriteobject

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

const resourceType string = "cloud-storage-write-object"

const (
	bucketKey      = "bucket"
	objectKey      = "object"
	contentKey     = "content"
	contentTypeKey = "content_type"
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
	WriteObject(ctx context.Context, bucket, object, content, contentType string) (map[string]any, error)
}

type Config struct {
	tools.ConfigBase `yaml:",inline"`
	Type             string  `yaml:"type" validate:"required"`
	Source           string  `yaml:"source" validate:"required"`
	Bucket           *string `yaml:"bucket,omitempty"`
}

var _ tools.ToolConfig = Config{}

func (cfg Config) ToolConfigType() string {
	return resourceType
}

func (cfg Config) Initialize(context.Context) (tools.Tool, error) {
	if cfg.Description == "" {
		return nil, fmt.Errorf("description is required for tool %q", cfg.Name)
	}
	if cfg.Bucket != nil && *cfg.Bucket == "" {
		return nil, fmt.Errorf("bucket cannot be empty for tool %q", cfg.Name)
	}

	objectParam := parameters.NewStringParameter(objectKey, "Full object name (path) within the bucket, e.g. 'path/to/file.txt'.")
	contentParam := parameters.NewStringParameter(contentKey, "Text content to write to the Cloud Storage object.")
	contentTypeParam := parameters.NewStringParameter(contentTypeKey, "MIME type to record on the written object. When empty, Cloud Storage auto-detects from the first 512 bytes of content.", parameters.WithStringDefault(""))
	allParameters := parameters.Parameters{}
	if cfg.Bucket == nil {
		allParameters = append(allParameters, parameters.NewStringParameter(bucketKey, "Name of the Cloud Storage bucket to write into."))
	}
	allParameters = append(allParameters, objectParam, contentParam, contentTypeParam)

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
	bucket := cloudstoragecommon.ResolveString(t.Cfg.Bucket, mapParams, bucketKey)
	if bucket == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a non-empty string", bucketKey), nil)
	}
	object, ok := mapParams[objectKey].(string)
	if !ok || object == "" {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a non-empty string", objectKey), nil)
	}
	content, ok := mapParams[contentKey].(string)
	if !ok {
		return nil, util.NewAgentError(fmt.Sprintf("invalid or missing '%s' parameter; expected a string", contentKey), nil)
	}
	contentType, _ := mapParams[contentTypeKey].(string)

	resp, err := source.WriteObject(ctx, bucket, object, content, contentType)
	if err != nil {
		return nil, cloudstoragecommon.ProcessGCSError(err)
	}
	return resp, nil
}
