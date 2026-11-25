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
	"encoding/json"
	"fmt"

	dataprocpb "cloud.google.com/go/dataproc/v2/apiv1/dataprocpb"
	"github.com/goccy/go-yaml"
	"github.com/googleapis/mcp-toolbox/internal/tools"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

// unmarshalProto is a helper function to unmarshal a generic interface{} into a proto.Message.
func unmarshalProto(data any, m proto.Message) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal to JSON: %w", err)
	}
	return protojson.Unmarshal(jsonData, m)
}

type compatibleSource interface {
	CreateBatch(context.Context, *dataprocpb.Batch) (map[string]any, error)
}

// Config is a common config that can be used with any type of create batch tool. However, each tool
// will still need its own config type, embedding this Config, so it can provide a type-specific
// Initialize implementation.
type Config struct {
	tools.ConfigBase  `yaml:",inline"`
	Type              string                        `yaml:"type" validate:"required"`
	Source            string                        `yaml:"source" validate:"required"`
	RuntimeConfig     *dataprocpb.RuntimeConfig     `yaml:"runtimeConfig"`
	EnvironmentConfig *dataprocpb.EnvironmentConfig `yaml:"environmentConfig"`
}

// GetSourceName returns the name of the source the tool is configured to use.
// It is promoted to the tool-specific configs that embed this Config, so they
// satisfy tools.ToolConfig without redeclaring it.
func (cfg Config) GetSourceName() string {
	return cfg.Source
}

func NewConfig(ctx context.Context, name string, decoder *yaml.Decoder) (Config, error) {
	// Use a temporary struct to decode the YAML, so that we can handle the proto
	// conversion for RuntimeConfig and EnvironmentConfig.
	var ymlCfg struct {
		Name              string   `yaml:"name"`
		Type              string   `yaml:"type"`
		Source            string   `yaml:"source"`
		Description       string   `yaml:"description"`
		RuntimeConfig     any      `yaml:"runtimeConfig"`
		EnvironmentConfig any      `yaml:"environmentConfig"`
		AuthRequired      []string `yaml:"authRequired"`
	}

	if err := decoder.DecodeContext(ctx, &ymlCfg); err != nil {
		return Config{}, err
	}

	cfg := Config{
		ConfigBase: tools.ConfigBase{
			Name:         name,
			Description:  ymlCfg.Description,
			AuthRequired: ymlCfg.AuthRequired,
		},
		Type:   ymlCfg.Type,
		Source: ymlCfg.Source,
	}

	if ymlCfg.RuntimeConfig != nil {
		rc := &dataprocpb.RuntimeConfig{}
		if err := unmarshalProto(ymlCfg.RuntimeConfig, rc); err != nil {
			return Config{}, fmt.Errorf("failed to unmarshal runtimeConfig: %w", err)
		}
		cfg.RuntimeConfig = rc
	}

	if ymlCfg.EnvironmentConfig != nil {
		ec := &dataprocpb.EnvironmentConfig{}
		if err := unmarshalProto(ymlCfg.EnvironmentConfig, ec); err != nil {
			return Config{}, fmt.Errorf("failed to unmarshal environmentConfig: %w", err)
		}
		cfg.EnvironmentConfig = ec
	}

	return cfg, nil
}
