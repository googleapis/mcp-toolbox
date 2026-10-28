// Copyright 2025 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package createbatch

import (
	"context"
	"fmt"
	"net/http"

	dataprocpb "cloud.google.com/go/dataproc/v2/apiv1/dataprocpb"
	"github.com/googleapis/mcp-toolbox/internal/sources"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"github.com/googleapis/mcp-toolbox/internal/util"
	"github.com/googleapis/mcp-toolbox/internal/util/parameters"
	"google.golang.org/protobuf/proto"
)

type BatchBuilder interface {
	Parameters() parameters.Parameters
	BuildBatch(parameters.ParamValues) (*dataprocpb.Batch, error)
}

func NewTool(cfg Config, originalCfg tools.ToolConfig, builder BatchBuilder) (*Tool, error) {
	desc := cfg.Description
	if desc == "" {
		desc = fmt.Sprintf("Creates a Serverless Spark (aka Dataproc Serverless) %s operation.", cfg.Type)
	}

	allParameters := builder.Parameters()

	return &Tool{
		BaseTool: tools.NewBaseTool(
			cfg,
			tools.GetAnnotationsOrDefault(nil, tools.NewDestructiveAnnotations),
			tools.Manifest{Description: desc, Parameters: allParameters.Manifest(), AuthRequired: cfg.AuthRequired},
			allParameters,
		),
		originalConfig: originalCfg,
		Builder:        builder,
	}, nil
}

type Tool struct {
	tools.BaseTool[Config]
	originalConfig tools.ToolConfig
	Builder        BatchBuilder
}

func (t Tool) GetSourceName() string {
	return t.Cfg.Source
}

func (t Tool) ValidateSource(source sources.Source) error {
	_, ok := source.(compatibleSource)
	if !ok {
		return fmt.Errorf("invalid source for %q tool: source %q is not a compatible type", t.Cfg.Type, t.Cfg.Source)
	}
	return nil
}

func (t *Tool) Invoke(ctx context.Context, s sources.Source, params parameters.ParamValues, accessToken tools.AccessToken) (any, util.ToolboxError) {
	source, ok := s.(compatibleSource)
	if !ok {
		return nil, util.NewClientServerError("source used is not compatible with the tool", http.StatusInternalServerError, nil)
	}
	batch, err := t.Builder.BuildBatch(params)
	if err != nil {
		if tbErr, ok := err.(util.ToolboxError); ok {
			return nil, tbErr
		}
		return nil, util.NewAgentError("failed to build batch", err)
	}

	if t.Cfg.RuntimeConfig != nil {
		batch.RuntimeConfig = proto.Clone(t.Cfg.RuntimeConfig).(*dataprocpb.RuntimeConfig)
	}

	if t.Cfg.EnvironmentConfig != nil {
		batch.EnvironmentConfig = proto.Clone(t.Cfg.EnvironmentConfig).(*dataprocpb.EnvironmentConfig)
	}

	// Common override for version if present in params
	paramMap := params.AsMap()
	if version, ok := paramMap["version"].(string); ok && version != "" {
		if batch.RuntimeConfig == nil {
			batch.RuntimeConfig = &dataprocpb.RuntimeConfig{}
		}
		batch.RuntimeConfig.Version = version
	}

	resp, err := source.CreateBatch(ctx, batch)
	if err != nil {
		return nil, util.ProcessGcpError(err)
	}
	return resp, nil
}

func (t *Tool) ToConfig() tools.ToolConfig {
	return t.originalConfig
}

func (t Tool) EmbedParams(ctx context.Context, paramValues parameters.ParamValues, pMgr tools.PrimitiveManagerI) (parameters.ParamValues, error) {
	newParamValues, err := parameters.EmbedParams(ctx, t.StaticParameters, paramValues, pMgr, nil)
	if err != nil {
		return nil, util.NewClientServerError(fmt.Sprintf("error embedding parameters: %v", err), http.StatusInternalServerError, err)
	}
	return newParamValues, nil
}
